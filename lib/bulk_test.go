package notelib

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"testing"

	"github.com/blues/note-go/note"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const verboseBulkTest = false

func TestBulkDecode(t *testing.T) {
	context, err := BulkDecodeTemplate(
		[]byte(`{"format":2,"storage":"file:data/test_note-qo.000","template_body":"{\"id\":24,\"description\":\"type_hint\"}","template_payload":1024}`),
		[]byte{167, 1, 152, 1, 38, 0, 2, 182, 219, 68, 209, 235, 39, 23, 137, 112, 31, 54, 63, 130, 138, 24, 0, 0, 0, 0, 13, 0, 49, 50, 56, 32, 98, 121, 116, 101, 32, 110, 111, 116, 101, 128, 1, 20, 254, 1, 0, 238, 1, 0},
	)
	require.NoError(t, err)

	body, payload, when, wherewhen, olc, _, success := context.BulkDecodeNextEntry()

	require.Equal(t, map[string]interface{}{"description": "128 byte note"}, body)
	require.Equal(t, bytes.Repeat([]byte{0}, 128), payload)
	require.Equal(t, int64(1668561471), when)
	require.Equal(t, int64(1668561470), wherewhen)
	require.Equal(t, "87JC9RF9+86XJ", olc)
	require.Equal(t, true, success)

	body, payload, when, wherewhen, olc, _, success = context.BulkDecodeNextEntry()

	require.Nil(t, body)
	require.Nil(t, payload)
	require.Equal(t, int64(0), when)
	require.Equal(t, int64(0), wherewhen)
	require.Equal(t, "", olc)
	require.Equal(t, false, success)
}

func TestBulkEncode(t *testing.T) {

	templateJSON := `{"sensor12":{"temp":12.1,"humidity":12.1},"mylist11":[{"val":11},{"val":11}],"mylist12":[{"val":12},{"val":12}],"mylist13":[{"val":13},{"val":13}],"mylist14":[{"val":14},{"val":14}],"mylist18":[{"val":18},{"val":18}],"mylist21":[{"val":21},{"val":21}],"mylist22":[{"val":22},{"val":22}],"mylist23":[{"val":23},{"val":23}],"mylist24":[{"val":24},{"val":24}],"sep14":"0","sensor14":{"temp":14.1,"humidity":14.1},"sep23":"0","sensor18":{"temp":18.1,"humidity":18.1},"bool1":true,"bool2":true,"bool3":true,"bool4":true,"bool5":true,"bool6":true,"bool7":true,"bool8":true,"bool9":true}`

	bulkBody := BulkBody{}
	bulkBody.NoteFormat = BulkNoteFormatFlexNano
	bulkBody.NoteTemplate = templateJSON
	bulkBodyJSON, err := note.JSONMarshal(bulkBody)
	require.NoError(t, err)

	context, err := BulkEncodeTemplate(bulkBodyJSON)
	require.NoError(t, err)

	// Note that some numeric values are in quotes to test that numeric env vars, which are always strings, are encoded correctly
	dataJSON := `{"beep":true,"sep12":"valueSep12","sensor12":{"hi":"there","temp":"-123.5","humidity":151.5},"sensor14":{"temp":1.9995117,"humidity":"1.6777216E+38"},"sensor18":{"humidity":"9.8765432109876e-307","temp":9.8765432109876e+307},"mylist11":[{"val":"-128"},{"val":127}],"mylist12":[{"val":-32768},{"val":"32767"}],"mylist13":[{"val":-8388608},{"val":"8388607"}],"mylist14":[{"val":"-2147483648"},{"val":2147483647}],"mylist18":[{"val":-9223372036854775800},{"val":"9223372036854775800"}],"mylist21":[{"val":"0"},{"val":255}],"mylist22":[{"val":0},{"val":"65535"}],"mylist23":[{"val":"0"},{"val":16777215}],"mylist24":[{"val":0},{"val":"4294967295"}],"sep23":"valueSep23","bool1":true,"bool2":false,"bool3":"true","bool4":"false","bool5":"1","bool6":1,"bool7":"0","bool8":0,"bool9":true}`
	var data map[string]interface{}
	err = note.JSONUnmarshal([]byte(dataJSON), &data)
	require.NoError(t, err)

	output, err := context.BulkEncodeNextEntry(data, []byte{}, 0, 0, "", "", false)
	require.NoError(t, err)

	if verboseBulkTest {
		fmt.Printf("Encoded output:\n%s\n", hex.Dump(output))
	}

	require.Equal(t, output, []byte{0x8, 0x63, 0x0, 0xb8, 0xd7, 0xbc, 0x58, 0x80, 0x7f, 0x0, 0x80, 0xff, 0x7f, 0x0, 0x0, 0x80, 0xff, 0xff, 0x7f, 0x0, 0x0, 0x0, 0x80, 0xff, 0xff, 0xff, 0x7f, 0x8, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x80, 0xf8, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0x7f, 0x0, 0xff, 0x0, 0x0, 0xff, 0xff, 0x0, 0x0, 0x0, 0xff, 0xff, 0xff, 0x0, 0x0, 0x0, 0x0, 0xff, 0xff, 0xff, 0xff, 0x0, 0x0, 0xf0, 0xff, 0x3f, 0x7c, 0x6f, 0xfc, 0x7e, 0xa, 0x76, 0x61, 0x6c, 0x75, 0x65, 0x53, 0x65, 0x70, 0x32, 0x33, 0x65, 0x8d, 0x92, 0x4e, 0xb1, 0x94, 0xe1, 0x7f, 0xed, 0x15, 0x3b, 0x1a, 0x99, 0x31, 0x66, 0x0, 0x35, 0x1})

	context, err = BulkDecodeTemplate(bulkBodyJSON, output)
	require.NoError(t, err)

	body, payload, _, _, _, _, success := context.BulkDecodeNextEntry()
	require.Equal(t, true, success)
	require.Equal(t, "", string(payload))

	bodyJSON, err := note.JSONMarshal(body)
	require.NoError(t, err)
	if verboseBulkTest {
		fmt.Printf("Round-trip:\n")
		fmt.Printf("  *** before ***\n%s\n", string(dataJSON))
		fmt.Printf("  *** after ***\n%s\n", string(bodyJSON))
	}
}

var bulkTests = []struct {
	template string
	data     map[string]any
}{
	{
		template: `{"foo": "string"}`,
		data:     map[string]any{"foo": "bar"},
	},
	{
		template: `{"foo": 11}`,
		data:     map[string]any{"foo": 2},
	},
	{
		template: `{"foo": 14.1}`,
		data:     map[string]any{"foo": 2.5},
	},
	{
		template: `{"arr": [11, 11, 11, 11]}`,
		data:     map[string]any{"arr": []int{1, 2, 3, 4}},
	},
	{
		template: `{"arr": [14.1, 14.1, 14.1, 14.1]}`,
		data:     map[string]any{"arr": []float32{1.1, 2.2, 3.3, 4.4}},
	},
	{
		template: `{"map": {"val": 12}}`,
		data:     map[string]any{"map": map[string]interface{}{"val": 2}},
	},
}

func TestBulkEncodeDecode(t *testing.T) {
	for _, test := range bulkTests {
		expectedJSON, err := note.JSONMarshal(test.data)
		require.NoError(t, err)

		if verboseBulkTest {
			fmt.Printf("Template:  %s\n", test.template)
			fmt.Printf("Data Before: %v JSON: %s\n", test.data, expectedJSON)
		}

		templateBody := BulkBody{}
		templateBody.NoteFormat = BulkNoteFormatFlexNano
		templateBody.NoteTemplate = test.template
		templateBodyJSON, err := note.JSONMarshal(templateBody)
		require.NoError(t, err)

		context, err := BulkEncodeTemplate(templateBodyJSON)
		assert.NoError(t, err)

		output, err := context.BulkEncodeNextEntry(test.data, []byte{}, 0, 0, "", "", false)
		assert.NoError(t, err)

		if verboseBulkTest {
			fmt.Printf("Binary: %v\n", output)
		}

		context, err = BulkDecodeTemplate(templateBodyJSON, output)
		assert.NoError(t, err)

		body, payload, _, _, _, _, success := context.BulkDecodeNextEntry()
		assert.Equal(t, true, success, "BulkDecodeNextEntry failed")
		assert.Equal(t, "", string(payload))

		// The contract appears to be that these should be equal from the JSON standpoint
		// They are not equal from the Go standpoint because numbers are represented as json.Number rather than their native types
		actualJSON, err := note.JSONMarshal(body)
		assert.NoError(t, err)

		if verboseBulkTest {
			fmt.Printf("Data After: %v JSON: %s\n\n", body, actualJSON)
		}

		assert.JSONEq(t, string(expectedJSON), string(actualJSON), "JSON strings should match after encoding and decoding using template %s", test.template)
	}
}
