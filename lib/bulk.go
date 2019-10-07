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
	"github.com/blues/hub/jsonxt"
	"github.com/golang/snappy"
	"io"
	"math"
	"strings"
)

// Debugging
const debugJSONBin = false

// BulkNoteFormatV1 is the first and only version
const BulkNoteFormatV1 = 0x00000001

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
	Template              string
	TemplatePayloadLen    int
	TemplatePayloadOffset int
	TemplateFlagsOffset   int
	Payload               []byte
	PayloadEntries        int
	PayloadEntryLength    int
}

// The flags length is sizeof(int64)
const flagsLength = 8

// BulkDecodeTemplate decodes the template
func BulkDecodeTemplate(templateBodyJSON []byte, compressedPayload []byte) (context BulkTemplateContext, entries int, err error) {

	body := BulkBody{}
	json.Unmarshal(templateBodyJSON, &body)
	if body.NoteFormat != BulkNoteFormatV1 {
		err = fmt.Errorf("bulk template: unrecognized format: %d", body.NoteFormat)
		return
	}

	// Parse the template
	context.Template = body.NoteTemplate
	context.TemplatePayloadLen = body.NoteTemplatePayloadLen
	err = parseTemplate(&context)
	if err != nil {
		return
	}

	// Decompress the payload
	context.Payload, err = snappy.Decode(nil, compressedPayload)
	if err != nil {
		return
	}
	context.PayloadEntries = len(context.Payload) / context.PayloadEntryLength
	entries = context.PayloadEntries

	// Done
	debugf("\n$$$ BULK DATA $$$: decompressed payload from %d to %d (%d entries at %d/entry)\n",
		len(compressedPayload), len(context.Payload), len(context.Payload)/context.PayloadEntryLength, context.PayloadEntryLength)

	return
}

// Data extraction routines
func binExtractInt32(bin []byte) int32 {
	var value int32
	value = int32(bin[0])
	value = value | (int32(bin[1]) << 8)
	value = value | (int32(bin[2]) << 16)
	value = value | (int32(bin[3]) << 24)
	return value
}
func binExtractInt64(bin []byte) int64 {
	var value int64
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
func binExtractString(bin []byte) string {
	s := ""
	for i := 0; i < len(bin); i++ {
		if bin[i] != 0 {
			s += string(bin[i])
		}
	}
	return s
}
func binExtractFloat64(bin []byte) float64 {
	bits := binary.LittleEndian.Uint64(bin)
	return math.Float64frombits(bits)
}
func binExtractBytes(bin []byte) []byte {
	return bin
}

// BulkDecodeEntry extract a JSON object from the binary
func BulkDecodeEntry(context *BulkTemplateContext, i int) (body map[string]interface{}, payload []byte, when int64, where int64) {

	// Get the binary to be decoded
	off := i * context.PayloadEntryLength
	bin := context.Payload[off : off+context.PayloadEntryLength]

	// If there were flags, extract them
	flags := int64(0)
	if context.TemplateFlagsOffset != 0 {
		flags = binExtractInt64(bin[context.TemplateFlagsOffset : context.TemplateFlagsOffset+flagsLength])
	}

	// If there was a payload, extract it
	if context.TemplatePayloadLen != 0 {
		payload = binExtractBytes(bin[context.TemplatePayloadOffset : context.TemplatePayloadOffset+context.TemplatePayloadLen])
	}

	// All entries begin with these
	binOffset := 0
	when = binExtractInt64(bin[binOffset : binOffset+8])
	binOffset += 8
	where = binExtractInt64(bin[binOffset : binOffset+8])
	binOffset += 8

	// Generate an output body JSON string from the input, without even paying any attention at all
	// to the JSON hierarchy, arrays, or whatnot.
	bodyJSON := ""

	jsonReader := strings.NewReader(context.Template)
	dec := jsonxt.NewDecoder(jsonReader)
	dec.UseNumber()
	for {
		if debugJSONBin {
			debugf("%d/%d:\n  %s\n", binOffset, context.PayloadEntryLength, bodyJSON)
		}
		t, err := dec.Token()
		if err == io.EOF {
			break
		}
		// If invalid token (such as garbage in the template), bail
		if err != nil {
			break
		}
		switch t.(type) {
		case jsonxt.Delim:
			bodyJSON += fmt.Sprintf("%v", t)
		case string:
			str := fmt.Sprintf("%s", t)
			if strings.HasPrefix(str, "\"") {
				bodyJSON += str
			} else {
				strLen := len(str)
				bodyJSON += "\"" + binExtractString(bin[binOffset:binOffset+strLen]) + "\""
				binOffset += strLen
			}
		case jsonxt.Number:
			_, errInt := t.(jsonxt.Number).Int64()
			if errInt == nil {
				bodyJSON += fmt.Sprintf("%d", binExtractInt32(bin[binOffset:binOffset+4]))
				binOffset += 4
			} else {
				_, errFloat := t.(jsonxt.Number).Float64()
				if errFloat == nil {
					bodyJSON += fmt.Sprintf("%f", binExtractFloat64(bin[binOffset:binOffset+8]))
					binOffset += 8
				} else {
					bodyJSON += "0"
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

	// Unmarshal into an object
	jsonObj := map[string]interface{}{}
	if json.Unmarshal([]byte(bodyJSON), &jsonObj) == nil {
		jsonObj = omitempty(jsonObj)
	}

	// Return the json object as the body
	body = jsonObj

	// Done
	return

}

// Eliminate fields from a JSON object in a way that simulates "omitempty" tags, recursively
func omitempty(in map[string]interface{}) (out map[string]interface{}) {
	out = in
	for key, value := range in {
		switch value.(type) {
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

// Get the entry len by doing a pass over the template
func parseTemplate(context *BulkTemplateContext) (err error) {

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
		switch t.(type) {
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
				binLength += len(fmt.Sprintf("%s", t))
			}
		case jsonxt.Number:
			if debugJSONBin {
				debugf("TEMPLATE number\n")
			}
			_, errInt := t.(jsonxt.Number).Int64()
			if errInt == nil {
				binLength += 4 // int32
			} else {
				_, errFloat := t.(jsonxt.Number).Float64()
				if errFloat == nil {
					binLength += 8 // float64
				} else {
					err = fmt.Errorf("unrecognized JSON number")
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
	context.TemplatePayloadOffset = binLength
	binLength += context.TemplatePayloadLen

	// Flags
	if boolPresent {
		context.TemplateFlagsOffset = binLength
		binLength += flagsLength
	}

	// Done
	context.PayloadEntryLength = binLength
	return
}
