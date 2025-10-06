package notelib

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/blues/note-go/note"
	golc "github.com/google/open-location-code/go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const verboseBulkTest = false

func abs(n int64) int64 {
	if n < 0 {
		return -n
	}
	return n
}

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

	require.Equal(t, []byte{0x8, 0x63, 0x0, 0xb8, 0xd7, 0xbc, 0x58, 0x80, 0x7f, 0x0, 0x80, 0xff, 0x7f, 0x0, 0x0, 0x80, 0xff, 0xff, 0x7f, 0x0, 0x0, 0x0, 0x80, 0xff, 0xff, 0xff, 0x7f, 0x8, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x80, 0xf8, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0x7f, 0x0, 0xff, 0x0, 0x0, 0xff, 0xff, 0x0, 0x0, 0x0, 0xff, 0xff, 0xff, 0x0, 0x0, 0x0, 0x0, 0xff, 0xff, 0xff, 0xff, 0x0, 0x0, 0xf0, 0xff, 0x3f, 0x7c, 0x6f, 0xfc, 0x7e, 0xa, 0x76, 0x61, 0x6c, 0x75, 0x65, 0x53, 0x65, 0x70, 0x32, 0x33, 0x65, 0x8d, 0x92, 0x4e, 0xb1, 0x94, 0xe1, 0x7f, 0xed, 0x15, 0x3b, 0x1a, 0x99, 0x31, 0x66, 0x0, 0x35, 0x1},
		output)

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
	wantErr  bool
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
		template: `{"foo": 12}`,
		data:     map[string]any{"foo": 32767},
	},
	{
		template: `{"foo": 13}`,
		data:     map[string]any{"foo": 8388607},
	},
	{
		template: `{"foo": 14}`,
		data:     map[string]any{"foo": 2147483647},
	},
	{
		template: `{"foo": 15}`,
		data:     map[string]any{"foo": 549755813887},
	},
	{
		template: `{"foo": 16}`,
		data:     map[string]any{"foo": 140737488355327},
	},
	{
		template: `{"foo": 17}`,
		data:     map[string]any{"foo": 36028797018963967},
	},
	{
		template: `{"foo": 18}`,
		data:     map[string]any{"foo": 9223372036854775807},
	},
	{
		template: `{"foo": 21}`,
		data:     map[string]any{"foo": 255},
	},
	{
		template: `{"foo": 22}`,
		data:     map[string]any{"foo": 65535},
	},
	{
		template: `{"foo": 23}`,
		data:     map[string]any{"foo": 16777215},
	},
	{
		template: `{"foo": 24}`,
		data:     map[string]any{"foo": 4294967295},
	},
	{
		template: `{"foo": 25}`,
		data:     map[string]any{"foo": 1099511627775},
	},
	{
		template: `{"foo": 26}`,
		data:     map[string]any{"foo": 281474976710655},
	},
	{
		template: `{"foo": 27}`,
		data:     map[string]any{"foo": 72057594037927935},
	},
	{
		template: `{"foo": 28}`,
		data:     map[string]any{"foo": uint64(18446744073709551615)},
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
	{
		template: `{"arr": [11]}`,
		data:     map[string]any{"arr": []int{1, 2, 3, 4, 5}},
	},
	{
		template: `{"arr": [14.1]}`,
		data:     map[string]any{"arr": []float32{1.1, 2.2, 3.3, 4.4, 5.5}},
	},
	{
		template: `{"arr": [21]}`,
		data:     map[string]any{"arr": []uint{0, 128, 255}},
	},
	{
		template: `{"arr": [11]}`,
		data:     map[string]any{"arr": []any{"not-a-number"}},
		wantErr:  true,
	},
	/*{ // DAH 022825 - Variable length arrays of strings not (yet) supported
		template: `{"arr": ["string"]}`,
		data:     map[string]any{"arr": []string{"a", "b", "c"}},
	},*/
	{
		template: `{"arr": [11, 12]}`,
		data:     map[string]any{"arr": []int{1, 2}},
	},
}

func TestVariableLengthArrays(t *testing.T) {
	tests := []struct {
		name     string
		template string
		data     map[string]any
		wantErr  bool
	}{
		// Signed integer tests
		{
			name:     "int8 array with boundary values",
			template: `{"arr": [11]}`,
			data:     map[string]any{"arr": []int8{-128, -64, 0, 64, 127}},
		},
		{
			name:     "int16 array with boundary values",
			template: `{"arr": [12]}`,
			data:     map[string]any{"arr": []int16{-32768, -16384, 0, 16384, 32767}},
		},
		{
			name:     "int24 array with boundary values",
			template: `{"arr": [13]}`,
			data:     map[string]any{"arr": []int32{-8388608, -4194304, 0, 4194304, 8388607}},
		},
		{
			name:     "int32 array with boundary values",
			template: `{"arr": [14]}`,
			data:     map[string]any{"arr": []int32{-2147483648, -1073741824, 0, 1073741824, 2147483647}},
		},
		{
			name:     "int40 array with boundary values",
			template: `{"arr": [15]}`,
			data:     map[string]any{"arr": []int64{-549755813888, -274877906944, 0, 274877906944, 549755813887}},
		},
		{
			name:     "int48 array with boundary values",
			template: `{"arr": [16]}`,
			data:     map[string]any{"arr": []int64{-140737488355328, -70368744177664, 0, 70368744177664, 140737488355327}},
		},
		{
			name:     "int56 array with boundary values",
			template: `{"arr": [17]}`,
			data:     map[string]any{"arr": []int64{-36028797018963968, -18014398509481984, 0, 18014398509481984, 36028797018963967}},
		},
		{
			name:     "int64 array with boundary values",
			template: `{"arr": [18]}`,
			data:     map[string]any{"arr": []int64{-9223372036854775808, -4611686018427387904, 0, 4611686018427387904, 9223372036854775807}},
		},
		// Unsigned integer tests
		/*{ // DAH this test fails because MarshalJson converts an array of uint8 to character array: "Array and slice values encode as JSON arrays, except that []byte encodes as a base64-encoded string, and a nil slice encodes as the null JSON object."
			name:     "uint8 array with boundary values",
			template: `{"arr": [21]}`,
			data:     map[string]any{"arr": []uint8{0, 64, 128, 192, 255}},
		},*/
		{
			name:     "uint16 array with boundary values",
			template: `{"arr": [22]}`,
			data:     map[string]any{"arr": []uint16{0, 16384, 32768, 49152, 65535}},
		},
		{
			name:     "uint24 array with boundary values",
			template: `{"arr": [23]}`,
			data:     map[string]any{"arr": []uint32{0, 4194304, 8388608, 12582912, 16777215}},
		},
		{
			name:     "uint32 array with boundary values",
			template: `{"arr": [24]}`,
			data:     map[string]any{"arr": []uint32{0, 1073741824, 2147483648, 3221225472, 4294967295}},
		},
		{
			name:     "uint40 array with boundary values",
			template: `{"arr": [25]}`,
			data:     map[string]any{"arr": []uint64{0, 274877906944, 549755813888, 824633720832, 1099511627775}},
		},
		{
			name:     "uint48 array with boundary values",
			template: `{"arr": [26]}`,
			data:     map[string]any{"arr": []uint64{0, 70368744177664, 140737488355328, 211106232532992, 281474976710655}},
		},
		{
			name:     "uint56 array with boundary values",
			template: `{"arr": [27]}`,
			data:     map[string]any{"arr": []uint64{0, 18014398509481984, 36028797018963968, 54043195528445952, 72057594037927935}},
		},
		{
			name:     "uint64 array with boundary values",
			template: `{"arr": [28]}`,
			data:     map[string]any{"arr": []uint64{0, 4611686018427387904, 9223372036854775808, 13835058055282163712, 18446744073709551615}},
		},
		// Floating point tests
		{
			name:     "float16 array with boundary values",
			template: `{"arr": [12.1]}`,
			data:     map[string]any{"arr": []float32{-65504, -1.0, 0.0, 1.0, 65504}},
		},
		{
			name:     "float32 array with boundary values",
			template: `{"arr": [14.1]}`,
			data:     map[string]any{"arr": []float32{-3.4e38, -1.0, 0.0, 1.0, 3.4e38}},
		},
		{
			name:     "float64 array with boundary values",
			template: `{"arr": [18.1]}`,
			data:     map[string]any{"arr": []float64{-1.8e307, -1.0, 0.0, 1.0, 1.8e307}},
		},
		// Error cases
		{
			name:     "error on int8 overflow",
			template: `{"arr": [11]}`,
			data:     map[string]any{"arr": []int{128}},
			wantErr:  true,
		},
		{
			name:     "error on int8 underflow",
			template: `{"arr": [11]}`,
			data:     map[string]any{"arr": []int{-129}},
			wantErr:  true,
		},
		{
			name:     "error on uint8 overflow",
			template: `{"arr": [21]}`,
			data:     map[string]any{"arr": []int{256}},
			wantErr:  true,
		},
		{
			name:     "error on uint negative",
			template: `{"arr": [21]}`,
			data:     map[string]any{"arr": []int{-1}},
			wantErr:  true,
		},
		{
			name:     "multi-element template should not use variable length",
			template: `{"arr": [11, 11]}`,
			data:     map[string]any{"arr": []int{1, 2}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			templateBody := BulkBody{
				NoteFormat:   BulkNoteFormatFlexNano,
				NoteTemplate: tt.template,
			}
			templateJSON, err := note.JSONMarshal(templateBody)
			require.NoError(t, err)

			// Test encoding
			encodeCtx, err := BulkEncodeTemplate(templateJSON)
			require.NoError(t, err)

			output, err := encodeCtx.BulkEncodeNextEntry(tt.data, []byte{}, 0, 0, "", "", false)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)

			// Test decoding
			decodeCtx, err := BulkDecodeTemplate(templateJSON, output)
			require.NoError(t, err)

			body, payload, _, _, _, _, success := decodeCtx.BulkDecodeNextEntry()
			require.True(t, success)
			require.Empty(t, payload)

			// Compare original and decoded data
			expectedJSON, err := note.JSONMarshal(tt.data)
			require.NoError(t, err)
			actualJSON, err := note.JSONMarshal(body)
			require.NoError(t, err)
			assert.JSONEq(t, string(expectedJSON), string(actualJSON))
		})
	}
}

// haversineDistance calculates the distance between two points on Earth in meters
func haversineDistance(lat1, lon1, lat2, lon2 float64) float64 {
	const earthRadiusM = 6371000 // Earth radius in meters

	// Convert to radians
	lat1Rad := lat1 * math.Pi / 180
	lat2Rad := lat2 * math.Pi / 180
	deltaLat := (lat2 - lat1) * math.Pi / 180
	deltaLon := (lon2 - lon1) * math.Pi / 180

	a := math.Sin(deltaLat/2)*math.Sin(deltaLat/2) +
		math.Cos(lat1Rad)*math.Cos(lat2Rad)*
			math.Sin(deltaLon/2)*math.Sin(deltaLon/2)
	c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))

	return earthRadiusM * c
}

func TestBulkFlagNoVariable(t *testing.T) {
	// Test that bulkFlagNoVariable works correctly on decode with multiple records
	// This flag indicates no variable-length section, so no 2-byte offset is present
	// and no variable data (payload, flags, OLC) can be included

	templateBody := BulkBody{
		NoteFormat:   BulkNoteFormatFlexNano,
		NoteTemplate: `{"temp": 14.1, "humidity": 12, "pressure": 24}`,
	}
	templateJSON, err := note.JSONMarshal(templateBody)
	require.NoError(t, err)

	// Test with THREE records - each with bulkFlagNoVariable flag set (0x80)
	// Format per record:
	// - uint8 header with bulkFlagNoVariable (0x80)
	// - NO uint16 offset (since NO_VARIABLE flag is set)
	// - float32 temp value (4 bytes)
	// - int16 humidity value (2 bytes)
	// - uint32 pressure value (4 bytes)

	binaryData := []byte{
		// Record 1
		0x80, // Header: bulkFlagNoVariable (0x80) - no variable section
		// No offset bytes here due to NO_VARIABLE flag
		0x66, 0x66, 0x16, 0x41, // float32: 9.4 (temp)
		0x2A, 0x00, // int16: 42 (humidity)
		0x01, 0x02, 0x03, 0x04, // uint32: 0x04030201 = 67305985 (pressure)

		// Record 2
		0x80,                   // Header: bulkFlagNoVariable (0x80) - no variable section
		0x00, 0x00, 0x20, 0x41, // float32: 10.0 (temp)
		0x15, 0x00, // int16: 21 (humidity)
		0x05, 0x06, 0x07, 0x08, // uint32: 0x08070605 = 134678021 (pressure)

		// Record 3
		0x80,                   // Header: bulkFlagNoVariable (0x80) - no variable section
		0xCD, 0xCC, 0x4C, 0x40, // float32: 3.2 (temp)
		0x64, 0x00, // int16: 100 (humidity)
		0xFF, 0xFF, 0xFF, 0xFF, // uint32: 0xFFFFFFFF = 4294967295 (pressure)
	}

	// Test decoding all three records
	decodeCtx, err := BulkDecodeTemplate(templateJSON, binaryData)
	require.NoError(t, err)

	// Expected values for each record
	expectedRecords := []struct {
		temp     float64
		humidity float64
		pressure float64
	}{
		{9.4, 42, 67305985},
		{10.0, 21, 134678021},
		{3.2, 100, 4294967295},
	}

	for i, expected := range expectedRecords {
		body, payload, _, _, _, _, success := decodeCtx.BulkDecodeNextEntry()
		require.True(t, success, "Decoding record %d should succeed", i+1)
		require.Empty(t, payload, "No payload expected with NO_VARIABLE flag for record %d", i+1)
		require.NotNil(t, body, "Body should not be nil for record %d", i+1)

		// Check temperature value (might be json.Number due to UseNumber())
		var tempValue float64
		switch v := body["temp"].(type) {
		case float64:
			tempValue = v
		case json.Number:
			tempValue, err = v.Float64()
			require.NoError(t, err)
		default:
			t.Fatalf("Record %d: temp should be a float64 or json.Number, got type %T", i+1, body["temp"])
		}
		assert.InDelta(t, expected.temp, tempValue, 0.01, "Record %d: Temperature should be approximately %.1f", i+1, expected.temp)

		// Check humidity value (might be json.Number due to UseNumber())
		var humidityValue float64
		switch v := body["humidity"].(type) {
		case float64:
			humidityValue = v
		case json.Number:
			humidityValue, err = v.Float64()
			require.NoError(t, err)
		default:
			t.Fatalf("Record %d: humidity should be a float64 or json.Number, got type %T", i+1, body["humidity"])
		}
		assert.Equal(t, expected.humidity, humidityValue, "Record %d: Humidity should be %.0f", i+1, expected.humidity)

		// Check pressure value (might be json.Number due to UseNumber())
		var pressureValue float64
		switch v := body["pressure"].(type) {
		case float64:
			pressureValue = v
		case json.Number:
			pressureValue, err = v.Float64()
			require.NoError(t, err)
		default:
			t.Fatalf("Record %d: pressure should be a float64 or json.Number, got type %T", i+1, body["pressure"])
		}
		assert.Equal(t, expected.pressure, pressureValue, "Record %d: Pressure should be %.0f", i+1, expected.pressure)
	}

	// Check that there are no more records
	body, _, _, _, _, _, success := decodeCtx.BulkDecodeNextEntry()
	require.False(t, success, "Should have no more records after processing all three")
	require.Nil(t, body, "Body should be nil when no more records")

	// Test that encoding does NOT set the bulkFlagNoVariable flag
	// (it's a decode-only feature)
	encodeCtx, err := BulkEncodeTemplate(templateJSON)
	require.NoError(t, err)

	data := map[string]interface{}{
		"temp":     9.4,
		"humidity": 42,
		"pressure": 67305985,
	}

	encoded, err := encodeCtx.BulkEncodeNextEntry(data, []byte{}, 0, 0, "", "", false)
	require.NoError(t, err)

	// Verify the encoded data does NOT have bulkFlagNoVariable set
	require.Greater(t, len(encoded), 0, "Encoded data should not be empty")
	assert.Equal(t, uint8(0x00), encoded[0]&0x80, "bulkFlagNoVariable should NOT be set in encoded data")

	// Verify normal encoding includes the offset bytes
	assert.GreaterOrEqual(t, len(encoded), 3, "Encoded data should include header and offset")
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
		if test.wantErr {
			assert.Error(t, err)
			continue
		}
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

// TestOLCEncodingRoundTrip tests that OLC values round-trip correctly through encoding/decoding
func TestOLCEncodingRoundTrip(t *testing.T) {
	// Test location: Blues Inc headquarters in Boston (approximately)
	testLat := 42.3601
	testLon := -71.0589
	testOLC := golc.Encode(testLat, testLon, 12)

	tests := []struct {
		name             string
		templateField    string
		expectedMaxError float64 // in meters
	}{
		{
			name:             "OLC8 - Full 8-byte precision",
			templateField:    "_loc8",
			expectedMaxError: 0.1, // Perfect
		},
		{
			name:             "OLC7 - 7-byte precision (12 chars)",
			templateField:    "_loc7",
			expectedMaxError: 2, // ~1.7m measured
		},
		{
			name:             "OLC6 - 6-byte precision (10 chars)",
			templateField:    "_loc6",
			expectedMaxError: 20, // ~18m measured
		},
		{
			name:             "OLC5 - 5-byte precision (8 chars, coarse grid only)",
			templateField:    "_loc5",
			expectedMaxError: 130, // ~128m measured
		},
		{
			name:             "OLC4 - 4-byte precision (7 chars, truncated coarse)",
			templateField:    "_loc4",
			expectedMaxError: 1500, // ~1226m measured
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create template with the OLC field (use FlexNano to avoid compression)
			templateBody := BulkBody{
				NoteFormat:   BulkNoteFormatFlexNano,
				NoteTemplate: fmt.Sprintf(`{"%s": 18, "data": 14}`, tt.templateField),
			}
			templateJSON, err := note.JSONMarshal(templateBody)
			require.NoError(t, err)

			// Create test data
			testData := map[string]interface{}{
				"data": 42,
			}

			// Encode with OLC
			encodeCtx, err := BulkEncodeTemplate(templateJSON)
			require.NoError(t, err)

			output, err := encodeCtx.BulkEncodeNextEntry(testData, []byte{},
				1234567890, 1234567880, testOLC, "test-note", false)
			require.NoError(t, err)

			// Debug: print the output
			t.Logf("Output bytes: %v", output)

			// Decode - the output is not compressed, pass it directly
			decodeCtx, err := BulkDecodeTemplate(templateJSON, output)
			require.NoError(t, err)

			body, _, _, _, decodedOLC, _, success := decodeCtx.BulkDecodeNextEntry()
			require.True(t, success)

			// The OLC fields are extracted during decoding and removed from the body
			// They're returned as the OLC string (5th return value)

			// Verify the data field (handling json.Number type)
			dataValue := body["data"]
			switch v := dataValue.(type) {
			case json.Number:
				intVal, err := v.Int64()
				require.NoError(t, err)
				assert.Equal(t, int64(42), intVal)
			case int:
				assert.Equal(t, 42, v)
			case float64:
				assert.Equal(t, 42.0, v)
			default:
				t.Fatalf("Unexpected type for data value: %T", v)
			}

			// Verify OLC round-trip
			require.NotEmpty(t, decodedOLC, "Decoded OLC should not be empty for field %s", tt.templateField)

			t.Logf("Field: %s, Original OLC: %s, Decoded OLC: %s", tt.templateField, testOLC, decodedOLC)

			// Decode the OLC strings to lat/lon
			originalArea, err := golc.Decode(testOLC)
			require.NoError(t, err)
			originalLat, originalLon := originalArea.Center()

			decodedArea, err := golc.Decode(decodedOLC)
			require.NoError(t, err)
			decodedLat, decodedLon := decodedArea.Center()

			// Calculate distance between original and decoded
			distance := haversineDistance(originalLat, originalLon, decodedLat, decodedLon)

			t.Logf("Field: %s, Original OLC: %s, Decoded OLC: %s, Distance: %.2f meters",
				tt.templateField, testOLC, decodedOLC, distance)

			// Verify distance is within expected error bounds
			assert.LessOrEqual(t, distance, tt.expectedMaxError,
				"Distance %.2f meters exceeds maximum expected error %.2f meters for %s",
				distance, tt.expectedMaxError, tt.templateField)
		})
	}
}

// TestBulkTimeFieldType23 tests the type 23 encoding for _time field
// which provides 256-second granularity to save 1 byte for satellite
func TestBulkTimeFieldType23(t *testing.T) {
	// Simple test: create a template with _time field of type 23,
	// encode and decode it, verify the time is within expected range
	t.Run("type_23_time_field", func(t *testing.T) {
		// Template with _time field of type 23 (3-byte unsigned integer)
		templateJSON := `{"_time":23,"data":12.1}`

		bulkBody := BulkBody{}
		bulkBody.NoteFormat = BulkNoteFormatFlexNano
		bulkBody.NoteTemplate = templateJSON
		bulkBodyJSON, err := note.JSONMarshal(bulkBody)
		require.NoError(t, err)

		context, err := BulkEncodeTemplate(bulkBodyJSON)
		require.NoError(t, err)

		// Create test data
		data := map[string]interface{}{
			"data": 42.5,
		}

		// Get current time just before encoding
		timeBeforeEncode := time.Now().UTC().Unix()

		output, err := context.BulkEncodeNextEntry(data, []byte{}, 0, 0, "", "", false)
		require.NoError(t, err)

		// Decode
		context, err = BulkDecodeTemplate(bulkBodyJSON, output)
		require.NoError(t, err)

		body, _, when, _, _, _, success := context.BulkDecodeNextEntry()
		require.True(t, success)

		// The decoded time should be within 256*2 seconds of the current time
		// (256 for the granularity, *2 for margin)
		timeDiff := abs(when - timeBeforeEncode)
		require.LessOrEqual(t, timeDiff, int64(256*2),
			"Time difference %d should be within %d seconds", timeDiff, 256*2)

		// The time should have 256-second granularity
		require.Equal(t, int64(0), when%256,
			"Time %d should be a multiple of 256", when)

		// Check the data value
		if dataVal, ok := body["data"].(json.Number); ok {
			f, _ := dataVal.Float64()
			require.Equal(t, 42.5, f)
		} else {
			require.Equal(t, 42.5, body["data"])
		}
	})

	// Compare type 23 vs type 14 to show the space savings
	t.Run("type_23_vs_type_14_size", func(t *testing.T) {
		// Template with type 14 _time field (4 bytes)
		template14 := `{"_time":14,"sensor":12.1}`
		bulkBody14 := BulkBody{}
		bulkBody14.NoteFormat = BulkNoteFormatFlexNano
		bulkBody14.NoteTemplate = template14
		bulkBody14JSON, err := note.JSONMarshal(bulkBody14)
		require.NoError(t, err)

		// Template with type 23 _time field (3 bytes)
		template23 := `{"_time":23,"sensor":12.1}`
		bulkBody23 := BulkBody{}
		bulkBody23.NoteFormat = BulkNoteFormatFlexNano
		bulkBody23.NoteTemplate = template23
		bulkBody23JSON, err := note.JSONMarshal(bulkBody23)
		require.NoError(t, err)

		// Encode with both templates
		context14, err := BulkEncodeTemplate(bulkBody14JSON)
		require.NoError(t, err)
		context23, err := BulkEncodeTemplate(bulkBody23JSON)
		require.NoError(t, err)

		data := map[string]interface{}{
			"sensor": 25.5,
		}

		output14, err := context14.BulkEncodeNextEntry(data, []byte{}, 0, 0, "", "", false)
		require.NoError(t, err)
		output23, err := context23.BulkEncodeNextEntry(data, []byte{}, 0, 0, "", "", false)
		require.NoError(t, err)

		// Type 23 should use 1 byte less (3 bytes vs 4 bytes for time)
		require.Equal(t, len(output14)-1, len(output23),
			"Type 23 encoding should be 1 byte smaller than type 14")
	})
}
