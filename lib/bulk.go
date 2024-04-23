// Copyright 2017 Blues Inc.  All rights reserved.
// Use of this source code is governed by licenses granted by the
// copyright holder including that found in the LICENSE file.

// Note that this package needs a forked version of the golang "json" package, modified for two requirements:
// 1. Because we need to decompose and reconstruct the JSON template in a linear manner, we need to parse
//	  it and reconstruct it with its delimiters in-place.  Unfortunately, the standard JSON Decoder suppresses
//	  commas and colons and quotes.	 And so jsonxt returns all delimiters, not just []{}
// 2. Because we are serializing the values without serializing the keys, we need to know in the Decoder
//	  when the string it is sending us is a key vs a value.	 And so we changed it (in a hacky way)
//	  to return keys as "quoted" strings, and values as standard unquoted strings.

package notelib

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/blues/note-go/note"
	"github.com/blues/note/jsonxt"
	"github.com/golang/snappy"
	golc "github.com/google/open-location-code/go"
	"github.com/valyala/fastjson"
)

// Debugging
const (
	debugJSONBin    = false
	debugBin        = false
	debugEncoding   = false
	debugDecompress = false
)

// Bulk note formats
const (
	BulkNoteFormatOriginal = 1 // Original format
	BulkNoteFormatFlex     = 2 // Different 'where', 'string', and 'payload' handling
	BulkNoteFormatFlexNano = 3 // Same as Flex but omits Where/When
)

// Bulk header byte flags
const (
	bulkflagPayloadL    = 0x01 // HL == 00:0byte, 01:1byte, 10:2bytes, 11:4bytes
	bulkflagPayloadH    = 0x02
	bulkflagFlagsL      = 0x04 // HL == 00:0byte, 01:1byte, 10:2bytes, 11:8bytes
	bulkflagFlagsH      = 0x08
	bulkflagOLCL        = 0x10 // HL == 00:0byte, 01:1byte, 10:2bytes, 11:4bytes
	bulkflagOLCH        = 0x20
	bulkflagNoOmitEmpty = 0x40
)

// Maximum number of flags supported
const maxBinFlags = 64

// BulkBody is the bulk data note body
type BulkBody struct {
	// The format of the encoded binary note
	NoteFormat uint32 `json:"format,omitempty"`
	// The JSON of the Note template for this notefile.	 Note that we keep this in text form because
	// the order of the fields is relevant to the binary encoding derived from the JSON, and the various
	// encoders/decoders in the data processing pipeline won't retain field order.
	NoteTemplate           string `json:"template_body,omitempty"`
	NoteTemplatePayloadLen int    `json:"template_payload,omitempty"`
}

// BulkTemplateContext is the context for the bulk template
type BulkTemplateContext struct {
	NoteFormat uint32
	Template   string
	Bin        []byte
	BinLen     int
	// Used while parsing a record
	BinOffset   int
	BinUnderrun bool
	// Fields only valid for BulkNoteFormatOriginal
	ORIGTemplatePayloadLen    int
	ORIGTemplatePayloadOffset int
	ORIGTemplateFlagsOffset   int
	// Fields used for encoding
	binFlags      uint64
	binFlagsFound uint32
	binDepth      uint32
	binError      error
	noteID        string
}

// The flags length is sizeof(int64)
const flagsLength = 8

// See if a floating value is ".1" - that is, between N.0 and N.2
func isPointOne(test float64, base float64) bool {
	return test > base && test < base+0.2
}

// BulkDecodeTemplate sets up a context for decoding by taking the template and a binary buffer to be decoded
func BulkDecodeTemplate(templateBodyJSON []byte, compressedPayload []byte) (tmplContext BulkTemplateContext, err error) {
	body := BulkBody{}
	err = note.JSONUnmarshal(templateBodyJSON, &body)
	if err != nil {
		err = fmt.Errorf("couldn't unmarshal bulk template: %w", err)
		return
	}
	if body.NoteFormat != BulkNoteFormatOriginal && body.NoteFormat != BulkNoteFormatFlex && body.NoteFormat != BulkNoteFormatFlexNano {
		err = fmt.Errorf("bulk template: unrecognized format: %d", body.NoteFormat)
		return
	}

	// Set up the context
	tmplContext.NoteFormat = body.NoteFormat
	tmplContext.Template = body.NoteTemplate
	tmplContext.ORIGTemplatePayloadLen = body.NoteTemplatePayloadLen

	// Parse the template if this is the original format, because in that format entries were not self-describing
	if body.NoteFormat == BulkNoteFormatOriginal {
		err = parseORIGTemplate(&tmplContext)
		if err != nil {
			return
		}
	}

	// 2024-04-11 NF-305 Deal with the odd situation where flexnano format is intentionally snappy-compressed
	// because it's being sent over cellular.  In this case, we had nowhere to store a flag saying "this is
	// compressed" and so we use an in-band method.
	isSnappyCompressed := false
	snappySig := []byte("snappy$")
	snappySigLen := len(snappySig)
	if len(compressedPayload) >= snappySigLen && bytes.Equal(snappySig, compressedPayload[:snappySigLen]) {
		isSnappyCompressed = true
		compressedPayload = compressedPayload[snappySigLen:]
	}

	// Decompress the payload if not flex nano, which is sent uncompressed
	if body.NoteFormat == BulkNoteFormatFlexNano && !isSnappyCompressed {
		tmplContext.Bin = compressedPayload
		tmplContext.BinLen = len(tmplContext.Bin)
	} else if len(compressedPayload) > 0 {
		tmplContext.Bin, err = snappy.Decode(nil, compressedPayload)
		tmplContext.BinLen = len(tmplContext.Bin)
		if err != nil {
			err = fmt.Errorf("bulk decode error '%s': template:%s payload:%v", err, string(templateBodyJSON), compressedPayload)
			return
		}
		if debugDecompress {
			logDebug(context.Background(), "\n$$$ BULK DATA $$$: decompressed payload from %d to %d", len(compressedPayload), len(tmplContext.Bin))
		}
	}

	return
}

// Data extraction routines
func (tmplContext *BulkTemplateContext) binExtract(n int) (value []byte, success bool) {
	// Catch bogus values of n
	if n < 0 || (tmplContext.BinOffset+n) < 0 {
		logWarn(context.Background(), "bulk underrun: invalid value of n %d at offset %d", n, tmplContext.BinOffset)
		tmplContext.BinUnderrun = true
	}

	if (tmplContext.BinOffset + n) > tmplContext.BinLen {
		logWarn(context.Background(), "bulk underrun: want %d at offset %d but record len is only %d", n, tmplContext.BinOffset, tmplContext.BinLen)
		tmplContext.BinUnderrun = true
	}
	if tmplContext.BinUnderrun {
		return
	}
	value = tmplContext.Bin[tmplContext.BinOffset : tmplContext.BinOffset+n]
	tmplContext.BinOffset += n
	success = true
	return
}

func (tmplContext *BulkTemplateContext) binExtractInt8() (value int8) {
	bin, success := tmplContext.binExtract(1)
	if !success {
		return
	}
	return int8(bin[0])
}

func (tmplContext *BulkTemplateContext) binExtractUint8() (value uint8) {
	bin, success := tmplContext.binExtract(1)
	if !success {
		return
	}
	return uint8(bin[0])
}

func (tmplContext *BulkTemplateContext) binExtractInt16() (value int16) {
	bin, success := tmplContext.binExtract(2)
	if !success {
		return
	}
	value = int16(bin[0])
	value = value | (int16(bin[1]) << 8)
	return value
}

func (tmplContext *BulkTemplateContext) binExtractUint16() (value uint16) {
	bin, success := tmplContext.binExtract(2)
	if !success {
		return
	}
	value = uint16(bin[0])
	value = value | (uint16(bin[1]) << 8)
	return value
}

func (tmplContext *BulkTemplateContext) binExtractInt24() (value int32) {
	bin, success := tmplContext.binExtract(3)
	if !success {
		return
	}
	value = int32(bin[0])
	value = value | (int32(bin[1]) << 8)
	msb := int8(bin[2])
	msbSignExtended := int32(msb)
	value = value | (msbSignExtended << 16)
	return value
}

func (tmplContext *BulkTemplateContext) binExtractUint24() (value uint32) {
	bin, success := tmplContext.binExtract(3)
	if !success {
		return
	}
	value = uint32(bin[0])
	value = value | (uint32(bin[1]) << 8)
	value = value | (uint32(bin[2]) << 16)
	return value
}

func (tmplContext *BulkTemplateContext) binExtractInt32() (value int32) {
	bin, success := tmplContext.binExtract(4)
	if !success {
		return
	}
	value = int32(bin[0])
	value = value | (int32(bin[1]) << 8)
	value = value | (int32(bin[2]) << 16)
	value = value | (int32(bin[3]) << 24)
	return value
}

func (tmplContext *BulkTemplateContext) binExtractUint32() (value uint32) {
	bin, success := tmplContext.binExtract(4)
	if !success {
		return
	}
	value = uint32(bin[0])
	value = value | (uint32(bin[1]) << 8)
	value = value | (uint32(bin[2]) << 16)
	value = value | (uint32(bin[3]) << 24)
	return value
}

func (tmplContext *BulkTemplateContext) binExtractInt64() (value int64) {
	bin, success := tmplContext.binExtract(8)
	if !success {
		return
	}
	value = int64(bin[0])
	value = value | (int64(bin[1]) << 8)
	value = value | (int64(bin[2]) << 16)
	value = value | (int64(bin[3]) << 24)
	value = value | (int64(bin[4]) << 32)
	value = value | (int64(bin[5]) << 40)
	value = value | (int64(bin[6]) << 48)
	value = value | (int64(bin[7]) << 56)
	return value
}

func (tmplContext *BulkTemplateContext) binExtractUint64() (value uint64) {
	bin, success := tmplContext.binExtract(8)
	if !success {
		return
	}
	value = uint64(bin[0])
	value = value | (uint64(bin[1]) << 8)
	value = value | (uint64(bin[2]) << 16)
	value = value | (uint64(bin[3]) << 24)
	value = value | (uint64(bin[4]) << 32)
	value = value | (uint64(bin[5]) << 40)
	value = value | (uint64(bin[6]) << 48)
	value = value | (uint64(bin[7]) << 56)
	return value
}

func (tmplContext *BulkTemplateContext) binExtractFloat16() (value float32) {
	bin, success := tmplContext.binExtract(2)
	if !success {
		return
	}
	uvalue := uint16(bin[0])
	uvalue = uvalue | (uint16(bin[1]) << 8)
	f16 := Float16(uvalue)
	return f16.Float32()
}

func (tmplContext *BulkTemplateContext) binExtractFloat32() (value float32) {
	bin, success := tmplContext.binExtract(4)
	if !success {
		return
	}
	bits := binary.LittleEndian.Uint32(bin)
	return math.Float32frombits(bits)
}

func (tmplContext *BulkTemplateContext) binExtractFloat64() (value float64) {
	bin, success := tmplContext.binExtract(8)
	if !success {
		return
	}
	bits := binary.LittleEndian.Uint64(bin)
	return math.Float64frombits(bits)
}

func (tmplContext *BulkTemplateContext) binExtractBytes(n int) (value []byte) {
	bin, success := tmplContext.binExtract(n)
	if !success {
		return
	}
	return bin
}

func (tmplContext *BulkTemplateContext) binExtractString(maxlen int) (value string) {
	var strbytes []byte
	if tmplContext.NoteFormat == BulkNoteFormatOriginal {
		for i := 0; i < maxlen; i++ {
			b := byte(tmplContext.binExtractUint8())
			if b != 0 {
				strbytes = append(strbytes, b)
			}
		}
	} else if tmplContext.NoteFormat == BulkNoteFormatFlexNano {
		stringLen := tmplContext.binExtractUint8()
		strbytes = tmplContext.binExtractBytes(int(stringLen))
	} else {
		stringLen := tmplContext.binExtractUint16()
		strbytes = tmplContext.binExtractBytes(int(stringLen))
	}
	// Escape any quotes in the string
	value = strings.TrimSuffix(strings.TrimPrefix(strconv.Quote(string(strbytes)), "\""), "\"")
	return
}

// BulkDecodeNextEntry extract a JSON object from the binary.  Note that both 'when' and 'wherewhen'
// returned in standard unix epoch seconds (not in nanoseconds)
func (tmplContext *BulkTemplateContext) BulkDecodeNextEntry() (body map[string]interface{}, payload []byte, when int64, wherewhen int64, olc string, noteID string, success bool) {

	// Exit if nothing left
	if tmplContext.BinLen == 0 {
		if debugBin {
			logDebug(context.Background(), "bulk: nothing left")
		}
		return
	}

	// Trace
	if debugBin {
		logDebug(context.Background(), "%x", tmplContext.Bin)
	}

	// Plug the noteID into the context for later substitution
	tmplContext.noteID = noteID

	// Extract the bin header, and process the variable-length area
	noOmitEmpty := false
	var flags uint64
	if tmplContext.NoteFormat == BulkNoteFormatOriginal {

		// If there was a payload, extract it
		if tmplContext.ORIGTemplatePayloadLen != 0 {
			tmplContext.BinOffset = tmplContext.ORIGTemplatePayloadOffset
			payload = tmplContext.binExtractBytes(tmplContext.ORIGTemplatePayloadLen)
		}

		// If there were flags, extract them
		if tmplContext.ORIGTemplateFlagsOffset != 0 {
			tmplContext.BinOffset = tmplContext.ORIGTemplateFlagsOffset
			flags = tmplContext.binExtractUint64()
		}

	} else {

		// Extract the header and variable-length area position
		tmplContext.BinOffset = 0
		binHeader := tmplContext.binExtractUint8()
		tmplContext.BinOffset = int(tmplContext.binExtractUint16())

		// Extract the No OmitEmpty flag
		noOmitEmpty = (binHeader & bulkflagNoOmitEmpty) != 0

		// Extract payload length, and skip by the payload
		payloadLen := 0
		switch binHeader & (bulkflagPayloadL | bulkflagPayloadH) {
		case bulkflagPayloadL:
			payloadLen = int(tmplContext.binExtractUint8())
		case bulkflagPayloadH:
			payloadLen = int(tmplContext.binExtractUint16())
		case bulkflagPayloadH | bulkflagPayloadL:
			payloadLen = int(tmplContext.binExtractUint32())
		}
		payload = tmplContext.binExtractBytes(payloadLen)

		// Extract flags length, and skip by the flags
		switch binHeader & (bulkflagFlagsL | bulkflagFlagsH) {
		case bulkflagFlagsL:
			flags = uint64(tmplContext.binExtractUint8())
		case bulkflagFlagsH:
			flags = uint64(tmplContext.binExtractUint16())
		case bulkflagFlagsH | bulkflagFlagsL:
			flags = tmplContext.binExtractUint64()
		}

		// Extract OLC, and skip by it
		olcLen := 0
		switch binHeader & (bulkflagOLCL | bulkflagOLCH) {
		case bulkflagOLCL:
			olcLen = int(tmplContext.binExtractUint8())
		case bulkflagOLCH:
			olcLen = int(tmplContext.binExtractUint16())
		case bulkflagOLCH | bulkflagOLCL:
			olcLen = int(tmplContext.binExtractUint32())
		}
		olc = string(tmplContext.binExtractBytes(olcLen))

	}

	// The position right now is the length of the record
	binRecLen := tmplContext.BinOffset

	// Reset to the beginning of the record
	tmplContext.BinOffset = 0
	if tmplContext.NoteFormat != BulkNoteFormatOriginal {
		tmplContext.BinOffset++    // BULKFLAGS header byte
		tmplContext.BinOffset += 2 // Offset to variable length portion
	}

	// All entries begin with these
	combinedWhen := int64(0)
	if tmplContext.NoteFormat != BulkNoteFormatFlexNano {
		combinedWhen = tmplContext.binExtractInt64()
		where := tmplContext.binExtractInt64()
		if where != 0 {
			olc = OLCFromINT64(where)
		}

		// Prior to 2021-08-26, When was stored in nanoseconds, with the low order 1000000000 ALWAYS being 0
		// because the notecard only measures time on 1-second granularity.  Thus, these bits were squandered.
		// Starting on the ddate above, we changed the semantics to mean that when the low order 1000000000 is
		// exactly 0, it means that wherewhen is not supplied.  Otherwise, it is the number of seconds prior
		// to "when" that represents the time when the location was measured, offset by 1.  Please refer to
		// the notecard repo src/app/json.c to see the code that encodes this field.
		when = int64((uint64(combinedWhen) / 1000000000))
		relativeWhereWhenOffsetSecs := int64(uint64(combinedWhen) % 1000000000)
		if when > 0 && relativeWhereWhenOffsetSecs > 0 {
			wherewhen = when - (relativeWhereWhenOffsetSecs - 1)
		}

	}

	// Generate an output body JSON string from the input, without even paying any attention at all
	// to the JSON hierarchy, arrays, or whatnot.
	bodyJSON := ""

	jsonReader := strings.NewReader(tmplContext.Template)
	dec := jsonxt.NewDecoder(jsonReader)
	dec.UseNumber()
	for {
		if debugJSONBin {
			logDebug(context.Background(), "%d:\n  %s", tmplContext.BinOffset, bodyJSON)
		}
		t, err := dec.Token()
		if err == io.EOF {
			break
		}
		// If invalid token (such as garbage in the template), bail
		if err != nil {
			break
		}
		switch tt := t.(type) {
		case jsonxt.Delim:
			bodyJSON += fmt.Sprintf("%v", t)
		case string:
			str := fmt.Sprintf("%s", t)
			if strings.HasPrefix(str, "\"") {
				bodyJSON += str
			} else {
				strLen := len(str)
				i, err2 := strconv.Atoi(str)
				if err2 == nil && i > 0 {
					strLen = i
				}
				bodyJSON += "\"" + tmplContext.binExtractString(strLen) + "\""
			}
		case jsonxt.Number:
			numberType, errInt := tt.Int64()
			if errInt == nil {
				// Integer
				switch numberType {
				case 11:
					bodyJSON += fmt.Sprintf("%d", tmplContext.binExtractInt8())
				case 12:
					bodyJSON += fmt.Sprintf("%d", tmplContext.binExtractInt16())
				case 13:
					bodyJSON += fmt.Sprintf("%d", tmplContext.binExtractInt24())
				case 21:
					bodyJSON += fmt.Sprintf("%d", tmplContext.binExtractUint8())
				case 22:
					bodyJSON += fmt.Sprintf("%d", tmplContext.binExtractUint16())
				case 23:
					bodyJSON += fmt.Sprintf("%d", tmplContext.binExtractUint24())
				case 1:
					fallthrough
				case 14:
					bodyJSON += fmt.Sprintf("%d", tmplContext.binExtractInt32())
				case 18:
					bodyJSON += fmt.Sprintf("%d", tmplContext.binExtractInt64())
				case 24:
					bodyJSON += fmt.Sprintf("%d", tmplContext.binExtractUint32())
				case 28:
					bodyJSON += fmt.Sprintf("%d", tmplContext.binExtractUint64())
				default:
					bodyJSON += "0"
				}
			} else {
				numberType, errFloat := tt.Float64()
				if errFloat != nil {
					bodyJSON += "0"
				} else {
					// Real
					if isPointOne(numberType, 12) {
						bodyJSON += fmt.Sprintf("%g", tmplContext.binExtractFloat16())
					} else if isPointOne(numberType, 14) {
						bodyJSON += fmt.Sprintf("%g", tmplContext.binExtractFloat32())
					} else if isPointOne(numberType, 18) || isPointOne(numberType, 1) {
						bodyJSON += fmt.Sprintf("%g", tmplContext.binExtractFloat64())
					} else {
						bodyJSON += "0"
					}
				}
			}
		case bool:
			if (flags & 0x01) != 0 {
				bodyJSON += "true"
			} else {
				bodyJSON += "false"
			}
			flags = flags >> 1
		}
	}

	// Unmarshal and remove empty fields unless requested not to
	jsonObj := map[string]interface{}{}
	if note.JSONUnmarshal([]byte(bodyJSON), &jsonObj) == nil {
		if !noOmitEmpty {
			jsonObj = omitempty(jsonObj)
		}
	}

	// Return the json object as the body
	body = jsonObj

	// Perform special processing of the body in which the developer can specify
	// special fields to override the 'where' and 'when'.  This will be used in
	// conjunction with the 'FlexNano' bin format used by loranote, in which case
	// on-air savings at the byte level is required and where we don't want to take
	// two int64's worth of space per binary record even in cases when the user
	// doesn't need time or location in their application.  Note that we use
	// these defensive ways of converting the fields just in case the user sets
	// the datatype of these fields to something unexpected.
	v, present := body["_note"]
	if present {
		delete(body, "_note")
		noteID = v.(string)
	}
	v, present = body["_time"]
	if present {
		delete(body, "_time")
		bodyWhen, _ := strconv.ParseFloat(fmt.Sprintf("%v", v), 64)
		when = int64(bodyWhen)
	}
	v, present = body["_lat"]
	if present {
		delete(body, "_lat")
		bodyLat, _ := strconv.ParseFloat(fmt.Sprintf("%v", v), 64)
		v, present = body["_lon"]
		if present {
			delete(body, "_lon")
			bodyLon, _ := strconv.ParseFloat(fmt.Sprintf("%v", v), 64)
			olc = golc.Encode(bodyLat, bodyLon, 12)
			v, present := body["_ltime"]
			if present {
				delete(body, "_ltime")
				bodyLocWhen, _ := strconv.ParseFloat(fmt.Sprintf("%v", v), 64)
				wherewhen = int64(bodyLocWhen)
			} else {
				wherewhen = when
			}
		}
	}

	// Exit if we'd encountered underrun
	if tmplContext.BinUnderrun {
		logDebug(context.Background(), "bin: bin underrun")
		return
	}

	// If the record length is 0 at this point, it's because there was no payload, no flags, and
	// no variable OLC buffer area following the binary record.  In this case, the actual
	// record length is the current position after parsing the data.
	if binRecLen == 0 {
		binRecLen = tmplContext.BinOffset
	}

	// Advance the context to the next entry
	success = true
	tmplContext.Bin = tmplContext.Bin[binRecLen:]
	tmplContext.BinLen = len(tmplContext.Bin)

	// Trace
	if debugBin {
		logDebug(context.Background(), "bin: done with %d-byte record (%d bytes remaining)", binRecLen, tmplContext.BinLen)
	}

	// Done
	return
}

// Eliminate fields from a JSON object in a way that simulates "omitempty" tags, recursively
func omitempty(in map[string]interface{}) (out map[string]interface{}) {
	out = in
	for key, value := range in {
		switch tv := value.(type) {
		case json.Number:
			valueAsInt64, err := tv.Int64()
			if err == nil && valueAsInt64 == 0 {
				valueAsFloat64, err := tv.Float64()
				if err == nil && valueAsFloat64 == 0 {
					delete(out, key)
				}
			}
		case float64:
			if value == 0.0 {
				delete(out, key)
			}
		case int:
			if value == 0 {
				delete(out, key)
			}
		case string:
			if value == "" {
				delete(out, key)
			}
		case bool:
			if value == false {
				delete(out, key)
			}
		case map[string]interface{}:
			out[key] = omitempty(value.(map[string]interface{}))
		}
	}
	return
}

// In the case of the ORIG format, get parameters about bin records from the template
// because in that format the bin records were not yet self-describing.
func parseORIGTemplate(tmplContext *BulkTemplateContext) (err error) {
	// All templates begin with time and location
	binLength := 0
	binLength += 8 // time
	binLength += 8 // location

	jsonReader := strings.NewReader(tmplContext.Template)
	dec := jsonxt.NewDecoder(jsonReader)
	dec.UseNumber()
	boolPresent := false

	for err == nil {
		t, err2 := dec.Token()
		if err2 == io.EOF {
			break
		}
		// If invalid token (such as garbage in the template), bail
		if err2 != nil {
			err = err2
			break
		}
		switch tt := t.(type) {
		case jsonxt.Delim:
			if debugJSONBin {
				logDebug(context.Background(), "TEMPLATE DELIM %s", fmt.Sprintf("%v", t))
			}
		case string:
			str := fmt.Sprintf("%s", t)
			if debugJSONBin {
				logDebug(context.Background(), "TEMPLATE string %s", str)
			}
			if !strings.HasPrefix(str, "\"") {
				i, err2 := strconv.Atoi(str)
				if err2 == nil && i > 0 {
					binLength += i
				} else {
					binLength += len(str)
				}
			}
		case jsonxt.Number:
			if debugJSONBin {
				logDebug(context.Background(), "TEMPLATE number")
			}
			numberType, errInt := tt.Int64()
			if errInt == nil {
				// Integer
				switch numberType {
				case 11:
					binLength++
				case 12:
					binLength += 2
				case 13:
					binLength += 3
				case 1:
					fallthrough
				case 14:
					binLength += 4
				case 18:
					binLength += 8
				default:
					err = fmt.Errorf("unrecognized JSON integer type")
				}
			} else {
				numberType, errFloat := tt.Float64()
				if errFloat != nil {
					err = fmt.Errorf("unrecognized JSON number")
				} else {
					// Real
					if isPointOne(numberType, 12) {
						binLength += 2
					} else if isPointOne(numberType, 14) {
						binLength += 4
					} else if isPointOne(numberType, 18) || isPointOne(numberType, 1) {
						binLength += 8
					} else {
						err = fmt.Errorf("unrecognized JSON real number type")
					}
				}
			}
		case bool:
			if debugJSONBin {
				logDebug(context.Background(), "TEMPLATE bool")
			}
			boolPresent = true
		}

	}
	if err != nil {
		return
	}

	// Payload
	tmplContext.ORIGTemplatePayloadOffset = binLength
	binLength += tmplContext.ORIGTemplatePayloadLen

	// Flags
	if boolPresent {
		tmplContext.ORIGTemplateFlagsOffset = binLength
		binLength += flagsLength //nolint ignore 'ineffectual assignment to binLength' since we may use it for future additions to the code
	}

	return
}

// BulkEncodeTemplate sets up a context for encoding by taking the template and preparing a binary output buffer
func BulkEncodeTemplate(templateBodyJSON []byte) (context BulkTemplateContext, err error) {
	body := BulkBody{}
	err = note.JSONUnmarshal(templateBodyJSON, &body)
	if err != nil {
		err = fmt.Errorf("couldn't unmarshal bulk template: %w", err)
		return
	}
	if body.NoteFormat != BulkNoteFormatFlex && body.NoteFormat != BulkNoteFormatFlexNano {
		err = fmt.Errorf("bulk template: unrecognized or unsupported format: %d", body.NoteFormat)
		return
	}

	// Set up the context
	context.NoteFormat = body.NoteFormat
	context.Template = body.NoteTemplate
	context.Bin = []byte{}

	// Done
	return
}

// BulkEncodeNextEntry encodes a JSON object using the template in the supplied contxt
//
// BULK_NOTEFORMAT_FLEX and BULK_NOTEFORMAT_NANOFLEX
//
//	uint8 header containing BULKFLAGS
//	uint16 header containing variableLengthOffset
//	If BULKFLAGS is not NANO, int64 CombinedWhen (the merger of 'when' and 'whereWhen')
//	If BULKFLAGS is not NANO, int64 WhereOLC64 (positive if validly parsed, else negative if parsing error)
//	N*[binary records as defined by template, with counted text strings encoded as uint16:len uint8[len] ]
//	(beginning of variable-length data)
//	If BULKFLAGS indicates it is present, [payloadLen] [payload]
//	If BULKFLAGS indicates it is present, [flagsLen] [flags]
//	If BULKFLAGS indicates it is present, [olclen] [olc]
func (tmplContext *BulkTemplateContext) BulkEncodeNextEntry(body map[string]interface{}, payload []byte, when int64, wherewhen int64, currentLocOLC string, noteID string, noOmitEmpty bool) (output []byte, err error) {

	// If the OLC can be converted to an int64, do so.
	currentLocOLC64 := OLCToINT64(currentLocOLC)

	// Exit if the payload is simply too large
	if tmplContext.NoteFormat == BulkNoteFormatOriginal {
		err = fmt.Errorf("format not supported")
		return
	}

	// Plug the noteID into the context for later substitution
	tmplContext.noteID = noteID

	// Prior to 2021-08-26, When was stored in nanoseconds, with the low order 1000000000 ALWAYS being 0.
	// Starting on that date, we changed the semantics to mean that the low order 1000000000 is 0, then
	// wherewhen is not supplied.  Otherwise, it is the number of seconds prior to "when" that represents
	// the time when the location was measured, offset by 1.
	currentTimeSecs := uint32(time.Now().UTC().Unix())
	relativeWhereWhenOffsetSecs := uint32(0)
	if wherewhen != 0 && uint32(wherewhen) <= currentTimeSecs {
		differenceSecs := currentTimeSecs - uint32(wherewhen)
		if differenceSecs < (1000000000 - 1) {
			relativeWhereWhenOffsetSecs = differenceSecs + 1
		}
	}
	combinedWhen := uint64((uint64(currentTimeSecs) * 1000000000) + uint64(relativeWhereWhenOffsetSecs))

	// For the new formats, append a flags byte as the very first thing.  We put this at the top
	// because we will later come back and modify it, and it's convenient to be at the start.
	binHeader := uint8(0)
	if noOmitEmpty {
		binHeader = bulkflagNoOmitEmpty
	}
	variableLengthOffset := uint16(0)
	err = tmplContext.binAppendUint8(binHeader) // BULKFLAGS
	if err == nil {
		err = tmplContext.binAppendUint16(variableLengthOffset)
	}

	// Append the time and location so long as we're not operating in nano mode
	if tmplContext.NoteFormat != BulkNoteFormatFlexNano {
		if err == nil {
			err = tmplContext.binAppendUint64(combinedWhen)
		}
		if err == nil {
			tmplContext.binAppendInt64(currentLocOLC64)
		}
	}
	if err != nil {
		return
	}

	// Parse the template
	var p fastjson.Parser
	var template *fastjson.Value
	template, err = p.Parse(tmplContext.Template)
	if err != nil {
		err = fmt.Errorf("bulk: parse error: %s", err)
		return
	}

	// Visit each of the values within the template
	var to *fastjson.Object
	to, err = template.Object()
	if err != nil {
		err = fmt.Errorf("bulk: template error: %s", err)
		return
	}
	if body != nil {
		err = tmplContext.walkObjectInto(0, to, body)
		if err != nil {
			err = fmt.Errorf("bulk: %s", err)
			return
		}
	}

	// Remember the beginning of the variable-length data because
	// on all versions, the payload begins just after the records.
	variableLengthOffset = uint16(len(tmplContext.Bin))

	// Append the payload
	payloadLength := uint32(len(payload))
	if payloadLength > 0 {
		if payloadLength < 256 {
			binHeader |= bulkflagPayloadL
			err = tmplContext.binAppendUint8(uint8(payloadLength))
		} else if payloadLength < 65536 {
			binHeader |= bulkflagPayloadH
			err = tmplContext.binAppendInt16(int16(payloadLength))
		} else {
			binHeader |= bulkflagPayloadL | bulkflagPayloadH
			err = tmplContext.binAppendInt32(int32(payloadLength))
		}
		if err == nil {
			err = tmplContext.binAppendUint8s(payload, payloadLength)
		}
	} else {
		err = tmplContext.binAppendUint8s(payload, payloadLength)
	}
	if err != nil {
		return
	}

	// Append the flags if they're present
	if tmplContext.binFlagsFound > 0 {
		if tmplContext.binFlagsFound <= 8 {
			binHeader |= bulkflagFlagsL
			err = tmplContext.binAppendUint8(uint8(tmplContext.binFlags))
		} else if tmplContext.binFlagsFound <= 16 {
			binHeader |= bulkflagFlagsH
			err = tmplContext.binAppendInt16(int16(tmplContext.binFlags))
		} else {
			binHeader |= bulkflagFlagsL | bulkflagFlagsH
			err = tmplContext.binAppendUint64(tmplContext.binFlags)
		}
		if err != nil {
			return
		}
	}

	// Append OLC if it wasn't able to be converted
	if tmplContext.NoteFormat != BulkNoteFormatFlexNano && currentLocOLC64 < 0 {
		olclen := uint32(len(currentLocOLC))
		if olclen > 0 {
			if olclen < 256 {
				binHeader |= bulkflagOLCL
				err = tmplContext.binAppendUint8(uint8(olclen))
				if err == nil {
					err = tmplContext.binAppendUint8s([]uint8(currentLocOLC), olclen)
				}
			} else if olclen < 65536 {
				binHeader |= bulkflagOLCH
				err = tmplContext.binAppendInt16(int16(olclen))
				if err == nil {
					err = tmplContext.binAppendUint8s([]uint8(currentLocOLC), olclen)
				}
			} else {
				binHeader |= bulkflagOLCL | bulkflagOLCH
				err = tmplContext.binAppendInt32(int32(olclen))
				if err == nil {
					err = tmplContext.binAppendUint8s([]uint8(currentLocOLC), olclen)
				}
			}
		}
	}
	if err != nil {
		return
	}

	// Re-insert the header byte and variableLengthOffset where it belongs
	if len(tmplContext.Bin) >= 3 {
		tmplContext.Bin[0] = binHeader
		tmplContext.Bin[1] = byte(variableLengthOffset & 0x0ff)
		tmplContext.Bin[2] = byte((variableLengthOffset >> 8) & 0x0ff)
	}

	// Done
	output = tmplContext.Bin
	return

}

func (tmplContext *BulkTemplateContext) binAppendBool(value bool) (err error) {
	if debugEncoding {
		logDebug(context.Background(), "append %d  BOOL = %v\n", tmplContext.binDepth, value)
	}
	if tmplContext.binFlagsFound >= maxBinFlags {
		return fmt.Errorf("too may flag fields in template (%d max)", maxBinFlags)
	}
	// The flags are stored least significant bit to highest
	if value {
		tmplContext.binFlags |= 1 << tmplContext.binFlagsFound
	}
	tmplContext.binFlagsFound++
	return nil
}

func (tmplContext *BulkTemplateContext) binAppendUint8(databyte uint8) (err error) {
	tmplContext.Bin = append(tmplContext.Bin, byte(databyte))
	return nil
}

func (tmplContext *BulkTemplateContext) binAppendInt8(value int8) (err error) {
	if debugEncoding {
		logDebug(context.Background(), "append %d  INT8 = %d", tmplContext.binDepth, value)
	}
	return tmplContext.binAppendUint8(uint8(value))
}

func (tmplContext *BulkTemplateContext) binAppendInt16(value int16) (err error) {
	if debugEncoding {
		logDebug(context.Background(), "append %d  INT16 = %d", tmplContext.binDepth, value)
	}
	err = tmplContext.binAppendUint8(uint8(value & 0xff))
	value = value >> 8
	if err == nil {
		err = tmplContext.binAppendUint8(uint8(value & 0xff))
	}
	return err
}
func (tmplContext *BulkTemplateContext) binAppendUint16(value uint16) (err error) {
	if debugEncoding {
		logDebug(context.Background(), "append %d  UINT16 = %d", tmplContext.binDepth, value)
	}
	err = tmplContext.binAppendUint8(uint8(value & 0xff))
	value = value >> 8
	if err == nil {
		err = tmplContext.binAppendUint8(uint8(value & 0xff))
	}
	return err
}

func (tmplContext *BulkTemplateContext) binAppendInt24(value int32) (err error) {
	if debugEncoding {
		logDebug(context.Background(), "append %d  INT24 = %d", tmplContext.binDepth, value)
	}
	err = tmplContext.binAppendUint8(uint8(value & 0xff))
	value = value >> 8
	if err == nil {
		err = tmplContext.binAppendUint8(uint8(value & 0xff))
	}
	value = value >> 8
	if err == nil {
		err = tmplContext.binAppendUint8(uint8(value & 0xff))
	}
	return err
}

func (tmplContext *BulkTemplateContext) binAppendUint24(value uint32) (err error) {
	if debugEncoding {
		logDebug(context.Background(), "append %d  UINT24 = %d", tmplContext.binDepth, value)
	}
	err = tmplContext.binAppendUint8(uint8(value & 0xff))
	value = value >> 8
	if err == nil {
		err = tmplContext.binAppendUint8(uint8(value & 0xff))
	}
	value = value >> 8
	if err == nil {
		err = tmplContext.binAppendUint8(uint8(value & 0xff))
	}
	return err
}

func (tmplContext *BulkTemplateContext) binAppendInt32(value int32) (err error) {
	if debugEncoding {
		logDebug(context.Background(), "append %d  INT32 = %d", tmplContext.binDepth, value)
	}
	err = tmplContext.binAppendUint8(uint8(value & 0xff))
	value = value >> 8
	if err == nil {
		err = tmplContext.binAppendUint8(uint8(value & 0xff))
	}
	value = value >> 8
	if err == nil {
		err = tmplContext.binAppendUint8(uint8(value & 0xff))
	}
	value = value >> 8
	if err == nil {
		err = tmplContext.binAppendUint8(uint8(value & 0xff))
	}
	return err
}

func (tmplContext *BulkTemplateContext) binAppendUint32(value uint32) (err error) {
	if debugEncoding {
		logDebug(context.Background(), "append %d  UINT32 = %d", tmplContext.binDepth, value)
	}
	err = tmplContext.binAppendUint8(uint8(value & 0xff))
	value = value >> 8
	if err == nil {
		err = tmplContext.binAppendUint8(uint8(value & 0xff))
	}
	value = value >> 8
	if err == nil {
		err = tmplContext.binAppendUint8(uint8(value & 0xff))
	}
	value = value >> 8
	if err == nil {
		err = tmplContext.binAppendUint8(uint8(value & 0xff))
	}
	return err
}

func (tmplContext *BulkTemplateContext) binAppendInt64(value int64) (err error) {
	if debugEncoding {
		logDebug(context.Background(), "append %d  INT64 = %d", tmplContext.binDepth, value)
	}
	err = tmplContext.binAppendUint8(uint8(value & 0xff))
	value = value >> 8
	if err == nil {
		err = tmplContext.binAppendUint8(uint8(value & 0xff))
	}
	value = value >> 8
	if err == nil {
		err = tmplContext.binAppendUint8(uint8(value & 0xff))
	}
	value = value >> 8
	if err == nil {
		err = tmplContext.binAppendUint8(uint8(value & 0xff))
	}
	value = value >> 8
	if err == nil {
		err = tmplContext.binAppendUint8(uint8(value & 0xff))
	}
	value = value >> 8
	if err == nil {
		err = tmplContext.binAppendUint8(uint8(value & 0xff))
	}
	value = value >> 8
	if err == nil {
		err = tmplContext.binAppendUint8(uint8(value & 0xff))
	}
	value = value >> 8
	if err == nil {
		err = tmplContext.binAppendUint8(uint8(value & 0xff))
	}
	return err
}

func (tmplContext *BulkTemplateContext) binAppendUint64(value uint64) (err error) {
	if debugEncoding {
		logDebug(context.Background(), "append %d  UINT64 = %d", tmplContext.binDepth, value)
	}
	err = tmplContext.binAppendUint8(uint8(value & 0xff))
	value = value >> 8
	if err == nil {
		err = tmplContext.binAppendUint8(uint8(value & 0xff))
	}
	value = value >> 8
	if err == nil {
		err = tmplContext.binAppendUint8(uint8(value & 0xff))
	}
	value = value >> 8
	if err == nil {
		err = tmplContext.binAppendUint8(uint8(value & 0xff))
	}
	value = value >> 8
	if err == nil {
		err = tmplContext.binAppendUint8(uint8(value & 0xff))
	}
	value = value >> 8
	if err == nil {
		err = tmplContext.binAppendUint8(uint8(value & 0xff))
	}
	value = value >> 8
	if err == nil {
		err = tmplContext.binAppendUint8(uint8(value & 0xff))
	}
	value = value >> 8
	if err == nil {
		err = tmplContext.binAppendUint8(uint8(value & 0xff))
	}
	return err
}

func (tmplContext *BulkTemplateContext) binAppendUint8s(data []uint8, templateLen uint32) (err error) {
	if debugEncoding {
		logDebug(context.Background(), "append %d  BYTES(%d bytes)", tmplContext.binDepth, templateLen)
	}
	datalen := uint32(len(data))
	for i := uint32(0); err == nil && i < templateLen; i++ {
		if i < datalen {
			err = tmplContext.binAppendUint8(data[i])
		} else {
			err = tmplContext.binAppendUint8(0)
		}
	}
	return err
}

func (tmplContext *BulkTemplateContext) binAppendString(p string) (err error) {
	if debugEncoding {
		logDebug(context.Background(), "append %d  STRING = %s", tmplContext.binDepth, p)
	}
	actualLen := uint32(len(p))
	if tmplContext.NoteFormat == BulkNoteFormatFlexNano {
		err = tmplContext.binAppendUint8(uint8(actualLen))
		if err == nil {
			err = tmplContext.binAppendUint8s([]uint8(p), actualLen)
		}
	} else {
		err = tmplContext.binAppendUint16(uint16(actualLen))
		if err == nil {
			err = tmplContext.binAppendUint8s([]uint8(p), actualLen)
		}
	}
	return err
}

func (tmplContext *BulkTemplateContext) binAppendReal16(number float32) (err error) {
	if debugEncoding {
		logDebug(context.Background(), "append %d  REAL16 = %f", tmplContext.binDepth, number)
	}
	value := Fromfloat32(number)
	err = tmplContext.binAppendUint8(uint8(value & 0xff))
	value = value >> 8
	if err == nil {
		err = tmplContext.binAppendUint8(uint8(value & 0xff))
	}
	return err
}

func (tmplContext *BulkTemplateContext) binAppendReal32(number float32) (err error) {
	if debugEncoding {
		logDebug(context.Background(), "(appending %d REAL32 = %f as float32)", tmplContext.binDepth, number)
	}
	return tmplContext.binAppendUint32(math.Float32bits(number))
}

func (tmplContext *BulkTemplateContext) binAppendReal64(number float64) (err error) {
	if debugEncoding {
		logDebug(context.Background(), "(appending %d REAL64 = %f as float64)", tmplContext.binDepth, float32(number))
	}
	return tmplContext.binAppendUint64(math.Float64bits(number))
}

// Get a value
func (tmplContext *BulkTemplateContext) processTemplateValue(level int, v *fastjson.Value, do map[string]interface{}, dok string, dov interface{}) (err error) {

	if debugEncoding {
		fmt.Printf("%sprocessTemplateValue %s\n", strings.Repeat("  ", level), dok)
	}
	beforeLen := len(tmplContext.Bin)

	switch v.Type() {

	case fastjson.TypeTrue:
		var dv bool

		if debugEncoding {
			// Needed this printf to discover numeric type = json.Number
			fmt.Printf("%s type %T\n", dok, dov)
		}

		s, isString := dov.(string)
		b, isBool := dov.(bool)
		i, isInt := dov.(int)
		// Numeric values unmarshalled into json.Number
		n, isNumber := dov.(json.Number)

		if isBool {
			dv = b
		} else if isString {
			if s == "true" {
				dv = true
			} else {
				i, err := strconv.Atoi(s)
				if err == nil && i > 0 {
					dv = true
				}
			}
		} else if isInt {
			dv = (i > 0)
		} else if isNumber {
			i, err := n.Int64()
			if err == nil {
				dv = (i > 0)
			}
		}

		if debugEncoding {
			fmt.Printf("%sEMIT BOOL %t\n", strings.Repeat("  ", level), dv)
		}
		err = tmplContext.binAppendBool(dv)

	case fastjson.TypeString:
		newStringBytes, _ := v.StringBytes()
		format := string(newStringBytes)
		var dv string
		dv, _ = dov.(string)
		if dok == "_note" {
			dv = tmplContext.noteID
		}
		if debugEncoding {
			fmt.Printf("%sEMIT STRING %s format %s\n", strings.Repeat("  ", level), dv, format)
		}
		err = tmplContext.binAppendString(dv)

	case fastjson.TypeNumber:
		format := v.GetFloat64()
		s, isString := dov.(string)
		var isInt bool
		var ivalue int64
		if isString {
			// We do auto-coercision of strings to numbers so that we can parse
			// values in environment variables for _env.dbi
			// In Go playground it was found that ParseInt returns an error if
			// it encounters a decimal point, so if there is no error we use its
			// returned integer value in preference to casting the fvalue, since
			// float64 cannot encode the entire range of int64 integers.
			ivalue, err = strconv.ParseInt(s, 10, 64)
			if err == nil {
				isInt = true
			}
			f64, err := strconv.ParseFloat(s, 64)
			if err != nil {
				dov = 0
			} else {
				dov = f64
			}
		}
		fvalue, _ := dov.(float64)
		if !isInt {
			ivalue = int64(fvalue)
		}
		_, isJsonNumber := dov.(json.Number)
		if isJsonNumber {
			fvalue, _ = dov.(json.Number).Float64()
			ivalue, err = dov.(json.Number).Int64()
			if err != nil {
				if debugEncoding {
					fmt.Printf("%s(can't extract value as int)\n", strings.Repeat("  ", level))
				}
				ivalue = int64(fvalue)
			}
		}
		if dok == "_time" {
			fvalue = float64(time.Now().UTC().Unix())
			ivalue = int64(time.Now().UTC().Unix())
		}
		if debugEncoding {
			if isJsonNumber {
				fmt.Printf("%sEMIT %T %f %d format %f\n", strings.Repeat("  ", level), dov, fvalue, ivalue, format)
			} else {
				fmt.Printf("%sEMIT %T %f %d format %f\n", strings.Repeat("  ", level), dov, fvalue, ivalue, format)
			}
		}
		if isPointOne(format, 18) || isPointOne(format, 1) { // 8-byte float64
			err = tmplContext.binAppendReal64(fvalue)
		} else if isPointOne(format, 14) { // 4-byte float32
			err = tmplContext.binAppendReal32(float32(fvalue))
		} else if isPointOne(format, 12) { // 2-byte float16
			err = tmplContext.binAppendReal16(float32(fvalue))
		} else if format == 18 { // 8-byte int
			err = tmplContext.binAppendInt64(int64(ivalue))
		} else if format == 28 { // 8-byte uint
			err = tmplContext.binAppendUint64(uint64(ivalue))
		} else if format == 14 || format == 1 { // 4-byte int
			if ivalue > 2147483647 || ivalue < -2147483648 {
				err = fmt.Errorf("number out of range of 4-byte int")
			} else {
				err = tmplContext.binAppendInt32(int32(ivalue))
			}
		} else if format == 24 { // 4-byte uint
			if ivalue > 4294967295 || ivalue < 0 {
				err = fmt.Errorf("number out of range of 4-byte unsigned int")
			} else {
				err = tmplContext.binAppendUint32(uint32(ivalue))
			}
		} else if format == 13 { // 3-byte int
			if ivalue > 8388607 || ivalue < -8388608 {
				err = fmt.Errorf("number out of range of 3-byte int")
			} else {
				err = tmplContext.binAppendInt24(int32(ivalue))
			}
		} else if format == 23 { // 3-byte uint
			if ivalue > 16777215 || ivalue < 0 {
				err = fmt.Errorf("number out of range of 3-byte unsigned int")
			} else {
				err = tmplContext.binAppendUint24(uint32(ivalue))
			}
		} else if format == 12 { // 2-byte int
			if ivalue > 32767 || ivalue < -32768 {
				err = fmt.Errorf("number out of range of 2-byte int")
			} else {
				err = tmplContext.binAppendInt16(int16(ivalue))
			}
		} else if format == 22 { // 2-byte uint
			if ivalue > 65535 || ivalue < 0 {
				err = fmt.Errorf("number out of range of 2-byte unsigned int")
			} else {
				err = tmplContext.binAppendUint16(uint16(ivalue))
			}
		} else if format == 11 { // 1-byte int
			if ivalue > 127 || ivalue < -128 {
				err = fmt.Errorf("number out of range of 1-byte int")
			} else {
				err = tmplContext.binAppendInt8(int8(ivalue))
			}
		} else if format == 21 { // 1-byte uint
			if ivalue > 255 || ivalue < 0 {
				err = fmt.Errorf("number out of range of 1-byte unsigned int")
			} else {
				err = tmplContext.binAppendUint8(uint8(ivalue))
			}
		} else {
			err = fmt.Errorf("unrecognized template field type indicator: %f", format)
		}

	case fastjson.TypeObject:
		o, _ := v.Object()
		subObject, testOk := dov.(map[string]interface{})
		if dov == nil || !testOk || subObject == nil {
			return
		}
		err = tmplContext.walkObjectInto(level, o, subObject)

	case fastjson.TypeArray:
		a, _ := v.Array()
		subArray, testOk := dov.([]interface{})
		if dov == nil || !testOk || subArray == nil {
			return
		}
		err = tmplContext.walkArray(level, a, subArray)
	}

	if debugEncoding {
		if err != nil {
			fmt.Printf("%s^^ %s\n", strings.Repeat("  ", level), err)
		}
		fmt.Printf("%s^^ %d bytes appended\n", strings.Repeat("  ", level), len(tmplContext.Bin)-beforeLen)
	}

	return
}

// Walk an object array (the only type of array supported)
func (tmplContext *BulkTemplateContext) walkArray(level int, a []*fastjson.Value, da []interface{}) (err error) {

	if a == nil {
		return
	}
	if len(a) == 0 {
		return
	}

	switch a[0].Type() {
	case fastjson.TypeObject:
		for i := 0; i < len(a); i++ {
			do := map[string]interface{}{}
			if i < len(da) {
				v, testOK := da[i].(map[string]interface{})
				if testOK && v != nil {
					do = v
				}
			}
			err = tmplContext.processTemplateValue(level+1, a[i], do, "", do)
		}
	}
	return
}

// Decode an object
func (tmplContext *BulkTemplateContext) walkObjectInto(level int, o *fastjson.Object, do map[string]interface{}) (err error) {
	o.Visit(func(k []byte, v *fastjson.Value) {
		key := string(k)
		err := tmplContext.processTemplateValue(level+1, v, do, key, do[key])
		if tmplContext.binError == nil {
			tmplContext.binError = err
		}
	})
	return tmplContext.binError
}
