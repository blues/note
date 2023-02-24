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
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"

	"github.com/blues/note-go/note"
	"github.com/blues/note/jsonxt"
	"github.com/golang/snappy"
)

// Debugging
const debugJSONBin = false
const debugBin = false

// Bulk note formats
const bulkNoteFormatOriginal = 1 // Original format
const bulkNoteFormatFlex = 2     // Different 'where', 'string', and 'payload' handling

// Bulk header byte flags
const bulkflagPayloadL = 0x01 // HL == 00:0byte, 01:1byte, 10:2bytes, 11:4bytes
const bulkflagPayloadH = 0x02
const bulkflagFlagsL = 0x04 // HL == 00:0byte, 01:1byte, 10:2bytes, 11:8bytes
const bulkflagFlagsH = 0x08
const bulkflagOLCL = 0x10 // HL == 00:0byte, 01:1byte, 10:2bytes, 11:4bytes
const bulkflagOLCH = 0x20

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
	// Fields only valid for bulkNoteFormatOriginal
	ORIGTemplatePayloadLen    int
	ORIGTemplatePayloadOffset int
	ORIGTemplateFlagsOffset   int
}

// The flags length is sizeof(int64)
const flagsLength = 8

// See if a floating value is ".1" - that is, between N.0 and N.2
func isPointOne(test float64, base float64) bool {
	return test > base && test < base+0.2
}

// BulkDecodeTemplate decodes the template
func BulkDecodeTemplate(templateBodyJSON []byte, compressedPayload []byte) (context BulkTemplateContext, err error) {

	body := BulkBody{}
	note.JSONUnmarshal(templateBodyJSON, &body)
	if body.NoteFormat != bulkNoteFormatOriginal && body.NoteFormat != bulkNoteFormatFlex {
		err = fmt.Errorf("bulk template: unrecognized format: %d", body.NoteFormat)
		return
	}

	// Set up the context
	context.NoteFormat = body.NoteFormat
	context.Template = body.NoteTemplate
	context.ORIGTemplatePayloadLen = body.NoteTemplatePayloadLen

	// Parse the template if this is the original format, because in that format entries were not self-describing
	if body.NoteFormat == bulkNoteFormatOriginal {
		err = parseORIGTemplate(&context)
		if err != nil {
			return
		}
	}

	// Decompress the payload
	context.Bin, err = snappy.Decode(nil, compressedPayload)
	context.BinLen = len(context.Bin)
	if err != nil {
		return
	}

	// Done
	//debugf("\n$$$ BULK DATA $$$: decompressed payload from %d to %d\n", len(compressedPayload), len(context.Bin))

	return
}

// Data extraction routines
func (context *BulkTemplateContext) binExtract(n int) (value []byte, success bool) {
	// Catch bogus values of n
	if n < 0 || (context.BinOffset+n) < 0 {
		if debugBin {
			debugf("bulk underrun: invalid value of n %d at offset %d \n", n, context.BinOffset)
		}
		context.BinUnderrun = true
	}

	if (context.BinOffset + n) > context.BinLen {
		if debugBin {
			debugf("bulk underrun: want %d at offset %d but record len is only %d\n", n, context.BinOffset, context.BinLen)
		}
		context.BinUnderrun = true
	}
	if context.BinUnderrun {
		return
	}
	value = context.Bin[context.BinOffset : context.BinOffset+n]
	context.BinOffset += n
	success = true
	return
}
func (context *BulkTemplateContext) binExtractInt8() (value int8) {
	bin, success := context.binExtract(1)
	if !success {
		return
	}
	return int8(bin[0])
}
func (context *BulkTemplateContext) binExtractUint8() (value uint8) {
	bin, success := context.binExtract(1)
	if !success {
		return
	}
	return uint8(bin[0])
}
func (context *BulkTemplateContext) binExtractInt16() (value int16) {
	bin, success := context.binExtract(2)
	if !success {
		return
	}
	value = int16(bin[0])
	value = value | (int16(bin[1]) << 8)
	return value
}
func (context *BulkTemplateContext) binExtractUint16() (value uint16) {
	bin, success := context.binExtract(2)
	if !success {
		return
	}
	value = uint16(bin[0])
	value = value | (uint16(bin[1]) << 8)
	return value
}
func (context *BulkTemplateContext) binExtractInt24() (value int32) {
	bin, success := context.binExtract(3)
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
func (context *BulkTemplateContext) binExtractUint24() (value uint32) {
	bin, success := context.binExtract(3)
	if !success {
		return
	}
	value = uint32(bin[0])
	value = value | (uint32(bin[1]) << 8)
	value = value | (uint32(bin[2]) << 16)
	return value
}
func (context *BulkTemplateContext) binExtractInt32() (value int32) {
	bin, success := context.binExtract(4)
	if !success {
		return
	}
	value = int32(bin[0])
	value = value | (int32(bin[1]) << 8)
	value = value | (int32(bin[2]) << 16)
	value = value | (int32(bin[3]) << 24)
	return value
}
func (context *BulkTemplateContext) binExtractUint32() (value uint32) {
	bin, success := context.binExtract(4)
	if !success {
		return
	}
	value = uint32(bin[0])
	value = value | (uint32(bin[1]) << 8)
	value = value | (uint32(bin[2]) << 16)
	value = value | (uint32(bin[3]) << 24)
	return value
}
func (context *BulkTemplateContext) binExtractInt64() (value int64) {
	bin, success := context.binExtract(8)
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
func (context *BulkTemplateContext) binExtractUint64() (value uint64) {
	bin, success := context.binExtract(8)
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
func (context *BulkTemplateContext) binExtractFloat16() (value float32) {
	bin, success := context.binExtract(2)
	if !success {
		return
	}
	uvalue := uint16(bin[0])
	uvalue = uvalue | (uint16(bin[1]) << 8)
	f16 := Float16(uvalue)
	return f16.Float32()
}
func (context *BulkTemplateContext) binExtractFloat32() (value float32) {
	bin, success := context.binExtract(4)
	if !success {
		return
	}
	bits := binary.LittleEndian.Uint32(bin)
	return math.Float32frombits(bits)
}
func (context *BulkTemplateContext) binExtractFloat64() (value float64) {
	bin, success := context.binExtract(8)
	if !success {
		return
	}
	bits := binary.LittleEndian.Uint64(bin)
	return math.Float64frombits(bits)
}
func (context *BulkTemplateContext) binExtractBytes(n int) (value []byte) {
	bin, success := context.binExtract(n)
	if !success {
		return
	}
	return bin
}
func (context *BulkTemplateContext) binExtractString(maxlen int) (value string) {
	var strbytes []byte
	if context.NoteFormat == bulkNoteFormatOriginal {
		for i := 0; i < maxlen; i++ {
			b := byte(context.binExtractUint8())
			if b != 0 {
				strbytes = append(strbytes, b)
			}
		}

	} else {
		stringLen := context.binExtractUint16()
		strbytes = context.binExtractBytes(int(stringLen))
	}
	value = string(strbytes)
	return
}

// BulkDecodeNextEntry extract a JSON object from the binary.  Note that both 'when' and 'wherewhen'
// returned in standard unix epoch seconds (not in nanoseconds)
func (context *BulkTemplateContext) BulkDecodeNextEntry() (body map[string]interface{}, payload []byte, when int64, where int64, wherewhen int64, olc string, success bool) {

	// Exit if nothing left
	if context.BinLen == 0 {
		return
	}

	// Trace
	if debugBin {
		debugf("%x\n", context.Bin)
	}

	// Extract the bin header, and process the variable-length area
	var flags uint64
	if context.NoteFormat == bulkNoteFormatOriginal {

		// If there was a payload, extract it
		if context.ORIGTemplatePayloadLen != 0 {
			context.BinOffset = context.ORIGTemplatePayloadOffset
			payload = context.binExtractBytes(context.ORIGTemplatePayloadLen)
		}

		// If there were flags, extract them
		if context.ORIGTemplateFlagsOffset != 0 {
			context.BinOffset = context.ORIGTemplateFlagsOffset
			flags = context.binExtractUint64()
		}

	} else {

		// Extract the header and variable-length area position
		context.BinOffset = 0
		binHeader := context.binExtractUint8()
		context.BinOffset = int(context.binExtractUint16())

		// Extract payload length, and skip by the payload
		payloadLen := 0
		switch binHeader & (bulkflagPayloadL | bulkflagPayloadH) {
		case bulkflagPayloadL:
			payloadLen = int(context.binExtractUint8())
		case bulkflagPayloadH:
			payloadLen = int(context.binExtractUint16())
		case bulkflagPayloadH | bulkflagPayloadL:
			payloadLen = int(context.binExtractUint32())
		}
		payload = context.binExtractBytes(payloadLen)

		// Extract flags length, and skip by the flags
		switch binHeader & (bulkflagFlagsL | bulkflagFlagsH) {
		case bulkflagFlagsL:
			flags = uint64(context.binExtractUint8())
		case bulkflagFlagsH:
			flags = uint64(context.binExtractUint16())
		case bulkflagFlagsH | bulkflagFlagsL:
			flags = context.binExtractUint64()
		}

		// Extract OLC, and skip by it
		olcLen := 0
		switch binHeader & (bulkflagOLCL | bulkflagOLCH) {
		case bulkflagOLCL:
			olcLen = int(context.binExtractUint8())
		case bulkflagOLCH:
			olcLen = int(context.binExtractUint16())
		case bulkflagOLCH | bulkflagOLCL:
			olcLen = int(context.binExtractUint32())
		}
		olc = string(context.binExtractBytes(olcLen))

	}

	// The position right now is the length of the record
	binRecLen := context.BinOffset

	// Reset to the beginning of the record
	context.BinOffset = 0
	if context.NoteFormat != bulkNoteFormatOriginal {
		context.BinOffset++    // BULKFLAGS header byte
		context.BinOffset += 2 // Offset to variable length portion
	}

	// All entries begin with these
	combinedWhen := context.binExtractInt64()
	where = context.binExtractInt64()

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

	// Generate an output body JSON string from the input, without even paying any attention at all
	// to the JSON hierarchy, arrays, or whatnot.
	bodyJSON := ""

	jsonReader := strings.NewReader(context.Template)
	dec := jsonxt.NewDecoder(jsonReader)
	dec.UseNumber()
	for {
		if debugJSONBin {
			debugf("%d:\n  %s\n", context.BinOffset, bodyJSON)
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
				bodyJSON += "\"" + context.binExtractString(strLen) + "\""
			}
		case jsonxt.Number:
			numberType, errInt := tt.Int64()
			if errInt == nil {
				// Integer
				switch numberType {
				case 11:
					bodyJSON += fmt.Sprintf("%d", context.binExtractInt8())
				case 12:
					bodyJSON += fmt.Sprintf("%d", context.binExtractInt16())
				case 13:
					bodyJSON += fmt.Sprintf("%d", context.binExtractInt24())
				case 21:
					bodyJSON += fmt.Sprintf("%d", context.binExtractUint8())
				case 22:
					bodyJSON += fmt.Sprintf("%d", context.binExtractUint16())
				case 23:
					bodyJSON += fmt.Sprintf("%d", context.binExtractUint24())
				case 1:
					fallthrough
				case 14:
					bodyJSON += fmt.Sprintf("%d", context.binExtractInt32())
				case 18:
					bodyJSON += fmt.Sprintf("%d", context.binExtractInt64())
				case 24:
					bodyJSON += fmt.Sprintf("%d", context.binExtractUint32())
				case 28:
					bodyJSON += fmt.Sprintf("%d", context.binExtractUint64())
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
						bodyJSON += fmt.Sprintf("%g", context.binExtractFloat16())
					} else if isPointOne(numberType, 14) {
						bodyJSON += fmt.Sprintf("%g", context.binExtractFloat32())
					} else if isPointOne(numberType, 18) || isPointOne(numberType, 1) {
						bodyJSON += fmt.Sprintf("%g", context.binExtractFloat64())
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

	// Unmarshal and remove empty fields
	jsonObj := map[string]interface{}{}
	if note.JSONUnmarshal([]byte(bodyJSON), &jsonObj) == nil {
		jsonObj = omitempty(jsonObj)
	}

	// Return the json object as the body
	body = jsonObj

	// Exit if we'd encountered underrun
	if context.BinUnderrun {
		return
	}

	// If the record length is 0 at this point, it's because there was no payload, no flags, and
	// no variable OLC buffer area following the binary record.  In this case, the actual
	// record length is the current position after parsing the data.
	if binRecLen == 0 {
		binRecLen = context.BinOffset
	}

	// Advance the context to the next entry
	success = true
	context.Bin = context.Bin[binRecLen:]
	context.BinLen = len(context.Bin)

	// Trace
	if debugBin {
		debugf("bin: done with %d-byte record (%d bytes remaining)\n", binRecLen, context.BinLen)
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
func parseORIGTemplate(context *BulkTemplateContext) (err error) {

	// All templates begin with time and location
	binLength := 0
	binLength += 8 // time
	binLength += 8 // location

	jsonReader := strings.NewReader(context.Template)
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
				debugf("TEMPLATE DELIM %s\n", fmt.Sprintf("%v", t))
			}
		case string:
			str := fmt.Sprintf("%s", t)
			if debugJSONBin {
				debugf("TEMPLATE string %s\n", str)
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
				debugf("TEMPLATE number\n")
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
				debugf("TEMPLATE bool\n")
			}
			boolPresent = true
		}

	}
	if err != nil {
		return
	}

	// Payload
	context.ORIGTemplatePayloadOffset = binLength
	binLength += context.ORIGTemplatePayloadLen

	// Flags
	if boolPresent {
		context.ORIGTemplateFlagsOffset = binLength
		binLength += flagsLength
	}

	return
}
