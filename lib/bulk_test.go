package notelib

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"hash/crc32"
	"math"
	"testing"
	"time"

	"github.com/blues/note-go/note"
	"github.com/golang/snappy"
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
	ctx := context.Background()
	context, err := BulkDecodeTemplate(
		ctx,
		[]byte(`{"format":2,"storage":"file:data/test_note-qo.000","template_body":"{\"id\":24,\"description\":\"type_hint\"}","template_payload":1024}`),
		[]byte{167, 1, 152, 1, 38, 0, 2, 182, 219, 68, 209, 235, 39, 23, 137, 112, 31, 54, 63, 130, 138, 24, 0, 0, 0, 0, 13, 0, 49, 50, 56, 32, 98, 121, 116, 101, 32, 110, 111, 116, 101, 128, 1, 20, 254, 1, 0, 238, 1, 0},
	)
	require.NoError(t, err)

	body, payload, when, wherewhen, olc, _, success := context.BulkDecodeNextEntry(ctx)

	require.Equal(t, map[string]interface{}{"description": "128 byte note"}, body)
	require.Equal(t, bytes.Repeat([]byte{0}, 128), payload)
	require.Equal(t, int64(1668561471), when)
	require.Equal(t, int64(1668561470), wherewhen)
	require.Equal(t, "87JC9RF9+86XJ", olc)
	require.Equal(t, true, success)

	body, payload, when, wherewhen, olc, _, success = context.BulkDecodeNextEntry(ctx)

	require.Nil(t, body)
	require.Nil(t, payload)
	require.Equal(t, int64(0), when)
	require.Equal(t, int64(0), wherewhen)
	require.Equal(t, "", olc)
	require.Equal(t, false, success)
}

func TestBulkEncode(t *testing.T) {
	ctx := context.Background()
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

	output, err := context.BulkEncodeNextEntry(ctx, data, []byte{}, 0, 0, "", "", false)
	require.NoError(t, err)

	if verboseBulkTest {
		fmt.Printf("Encoded output:\n%s\n", hex.Dump(output))
	}

	require.Equal(t, []byte{0x8, 0x63, 0x0, 0xb8, 0xd7, 0xbc, 0x58, 0x80, 0x7f, 0x0, 0x80, 0xff, 0x7f, 0x0, 0x0, 0x80, 0xff, 0xff, 0x7f, 0x0, 0x0, 0x0, 0x80, 0xff, 0xff, 0xff, 0x7f, 0x8, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x80, 0xf8, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0x7f, 0x0, 0xff, 0x0, 0x0, 0xff, 0xff, 0x0, 0x0, 0x0, 0xff, 0xff, 0xff, 0x0, 0x0, 0x0, 0x0, 0xff, 0xff, 0xff, 0xff, 0x0, 0x0, 0xf0, 0xff, 0x3f, 0x7c, 0x6f, 0xfc, 0x7e, 0xa, 0x76, 0x61, 0x6c, 0x75, 0x65, 0x53, 0x65, 0x70, 0x32, 0x33, 0x65, 0x8d, 0x92, 0x4e, 0xb1, 0x94, 0xe1, 0x7f, 0xed, 0x15, 0x3b, 0x1a, 0x99, 0x31, 0x66, 0x0, 0x35, 0x1},
		output)

	context, err = BulkDecodeTemplate(ctx, bulkBodyJSON, output)
	require.NoError(t, err)

	body, payload, _, _, _, _, success := context.BulkDecodeNextEntry(ctx)
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
	ctx := context.Background()

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

			output, err := encodeCtx.BulkEncodeNextEntry(ctx, tt.data, []byte{}, 0, 0, "", "", false)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)

			// Test decoding
			decodeCtx, err := BulkDecodeTemplate(ctx, templateJSON, output)
			require.NoError(t, err)

			body, payload, _, _, _, _, success := decodeCtx.BulkDecodeNextEntry(ctx)
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
	ctx := context.Background()

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
	decodeCtx, err := BulkDecodeTemplate(ctx, templateJSON, binaryData)
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
		body, payload, _, _, _, _, success := decodeCtx.BulkDecodeNextEntry(ctx)
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
	body, _, _, _, _, _, success := decodeCtx.BulkDecodeNextEntry(ctx)
	require.False(t, success, "Should have no more records after processing all three")
	require.Nil(t, body, "Body should be nil when no more records")

	// Test that encoding does NOT set the bulkFlagNoVariable flag.  This is deliberate and
	// load-bearing, not an oversight: Notecard firmware older than 2025-09 consumes the 2-byte
	// offset unconditionally and would misparse a collapsed record.  See
	// TestBulkEncoderStaysUncollapsed and BulkNoVariableEquivalent, which reconciles the two
	// encodings for checksum comparison without changing what we transmit.
	encodeCtx, err := BulkEncodeTemplate(templateJSON)
	require.NoError(t, err)

	data := map[string]interface{}{
		"temp":     9.4,
		"humidity": 42,
		"pressure": 67305985,
	}

	encoded, err := encodeCtx.BulkEncodeNextEntry(ctx, data, []byte{}, 0, 0, "", "", false)
	require.NoError(t, err)

	// Verify the encoded data does NOT have bulkFlagNoVariable set
	require.Greater(t, len(encoded), 0, "Encoded data should not be empty")
	assert.Equal(t, uint8(0x00), encoded[0]&0x80, "bulkFlagNoVariable should NOT be set in encoded data")

	// Verify normal encoding includes the offset bytes
	assert.GreaterOrEqual(t, len(encoded), 3, "Encoded data should include header and offset")
}

func TestBulkEncodeDecode(t *testing.T) {
	ctx := context.Background()
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

		output, err := context.BulkEncodeNextEntry(ctx, test.data, []byte{}, 0, 0, "", "", false)
		if test.wantErr {
			assert.Error(t, err)
			continue
		}
		assert.NoError(t, err)

		if verboseBulkTest {
			fmt.Printf("Binary: %v\n", output)
		}

		context, err = BulkDecodeTemplate(ctx, templateBodyJSON, output)
		assert.NoError(t, err)

		body, payload, _, _, _, _, success := context.BulkDecodeNextEntry(ctx)
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
		ctx := context.Background()
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

			output, err := encodeCtx.BulkEncodeNextEntry(ctx, testData, []byte{},
				1234567890, 1234567880, testOLC, "test-note", false)
			require.NoError(t, err)

			// Debug: print the output
			t.Logf("Output bytes: %v", output)

			// Decode - the output is not compressed, pass it directly
			decodeCtx, err := BulkDecodeTemplate(ctx, templateJSON, output)
			require.NoError(t, err)

			body, _, _, _, decodedOLC, _, success := decodeCtx.BulkDecodeNextEntry(ctx)
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

// A device-supplied _loc4 with its top bit set shifts into a negative OLC64; decoding it
// must yield no OLC rather than panic
func TestBulkDecodeNegativeOLC(t *testing.T) {
	ctx := context.Background()
	// The encoder fills _loc4 from the OLC argument, so encode the raw value under another
	// name- a single uint32 field has the same wire layout either way
	encoded := bulkEncodeOne(t, BulkNoteFormatFlexNano, `{"v":24}`, map[string]interface{}{"v": 0xFFFFFFFF})

	decodeCtx, err := BulkDecodeTemplate(ctx, bulkTemplateJSON(t, BulkNoteFormatFlexNano, `{"_loc4":24}`), encoded)
	require.NoError(t, err)

	var olc string
	var success bool
	require.NotPanics(t, func() {
		_, _, _, _, olc, _, success = decodeCtx.BulkDecodeNextEntry(ctx)
	})
	require.True(t, success)
	assert.Empty(t, olc)
}

// TestBulkTimeFieldType23 tests the type 23 encoding for _time field
// which provides 256-second granularity to save 1 byte for satellite
func TestBulkTimeFieldType23(t *testing.T) {
	ctx := context.Background()
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

		output, err := context.BulkEncodeNextEntry(ctx, data, []byte{}, 0, 0, "", "", false)
		require.NoError(t, err)

		// Decode
		context, err = BulkDecodeTemplate(ctx, bulkBodyJSON, output)
		require.NoError(t, err)

		body, _, when, _, _, _, success := context.BulkDecodeNextEntry(ctx)
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
		ctx := context.Background()
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

		output14, err := context14.BulkEncodeNextEntry(ctx, data, []byte{}, 0, 0, "", "", false)
		require.NoError(t, err)
		output23, err := context23.BulkEncodeNextEntry(ctx, data, []byte{}, 0, 0, "", "", false)
		require.NoError(t, err)

		// Type 23 should use 1 byte less (3 bytes vs 4 bytes for time)
		require.Equal(t, len(output14)-1, len(output23),
			"Type 23 encoding should be 1 byte smaller than type 14")
	})
}

// TestVariableLengthArrayMissingField is the regression suite for the
// walkArray bug where a variable-length-array template field whose value
// was missing from the body emitted zero bytes instead of a uint16(0)
// count. The decoder unconditionally reads a uint16 count for VLA fields,
// so the encoder skipping those 2 bytes shifted every subsequent field in
// the record.
//
// Pre-fix behavior: encode silently produces a too-short buffer; decode
// reads garbage values for every field after the first omitted VLA.
//
// Post-fix behavior: a missing body field encodes as count=0 with no
// elements — the same encoding as an explicit empty `[]` or `null`.
func TestVariableLengthArrayMissingField(t *testing.T) {
	ctx := context.Background()

	// (B) Minimal targeted byte-layout test.
	//
	// Template has two VLA fields; body omits the first. Inspect the raw
	// bytes to verify the count for the omitted field is present and zero,
	// and that the second field's data lands at the expected offset.
	t.Run("minimal_byte_layout", func(t *testing.T) {
		templateBody := BulkBody{
			NoteFormat:   BulkNoteFormatFlexNano,
			NoteTemplate: `{"a":[22],"b":[14.1]}`,
		}
		templateJSON, err := note.JSONMarshal(templateBody)
		require.NoError(t, err)

		// JSON-unmarshal so the body matches what wire.go produces (arrays
		// arrive as []interface{} of json.Number, not typed Go slices).
		var body map[string]interface{}
		err = note.JSONUnmarshal([]byte(`{"b":[1.5,2.5,3.5]}`), &body)
		require.NoError(t, err)

		encodeCtx, err := BulkEncodeTemplate(templateJSON)
		require.NoError(t, err)

		output, err := encodeCtx.BulkEncodeNextEntry(ctx, body, []byte{}, 0, 0, "", "", false)
		require.NoError(t, err)

		// Layout:
		//   [0]    BULKFLAGS                = 0x00
		//   [1-2]  variableLengthOffset LE  = 19 (0x13, 0x00)
		//   [3-4]  count for "a" LE         = 0  (0x00, 0x00)  <-- the fix
		//   [5-6]  count for "b" LE         = 3  (0x03, 0x00)
		//   [7-10]  float32 1.5             = 0x00 0x00 0xC0 0x3F
		//   [11-14] float32 2.5             = 0x00 0x00 0x20 0x40
		//   [15-18] float32 3.5             = 0x00 0x00 0x60 0x40
		expected := []byte{
			0x00,
			0x13, 0x00,
			0x00, 0x00,
			0x03, 0x00,
			0x00, 0x00, 0xC0, 0x3F,
			0x00, 0x00, 0x20, 0x40,
			0x00, 0x00, 0x60, 0x40,
		}
		require.Equal(t, expected, output,
			"missing VLA field must encode as uint16(0) count, not 0 bytes; pre-fix this is 17 bytes with the count for \"a\" missing entirely")

		// Round-trip: decoded "b" must come back as [1.5, 2.5, 3.5].
		decodeCtx, err := BulkDecodeTemplate(ctx, templateJSON, output)
		require.NoError(t, err)

		decoded, _, _, _, _, _, success := decodeCtx.BulkDecodeNextEntry(ctx)
		require.True(t, success)

		bArr, ok := decoded["b"].([]interface{})
		require.True(t, ok, "decoded \"b\" should be []interface{}, got %T", decoded["b"])
		require.Len(t, bArr, 3)
		for i, want := range []float64{1.5, 2.5, 3.5} {
			n, ok := bArr[i].(json.Number)
			require.True(t, ok, "element %d should be json.Number, got %T", i, bArr[i])
			got, err := n.Float64()
			require.NoError(t, err)
			assert.InDelta(t, want, got, 1e-6, "b[%d] mismatch", i)
		}
	})

	// (C) Missing-field shapes equivalence.
	//
	// Three bodies that should all encode identically and round-trip to the
	// same decoded form: field absent, field explicitly [], field explicitly
	// null. Pre-fix, the absent case produced a different (broken) encoding
	// from the empty-array case.
	t.Run("missing_empty_null_equivalence", func(t *testing.T) {
		templateBody := BulkBody{
			NoteFormat:   BulkNoteFormatFlexNano,
			NoteTemplate: `{"a":[22],"b":[14.1]}`,
		}
		templateJSON, err := note.JSONMarshal(templateBody)
		require.NoError(t, err)

		shapes := []struct {
			name string
			body string
		}{
			{"absent", `{"b":[1.5,2.5,3.5]}`},
			{"empty_array", `{"a":[],"b":[1.5,2.5,3.5]}`},
			{"explicit_null", `{"a":null,"b":[1.5,2.5,3.5]}`},
		}

		var firstOutput []byte
		var firstDecoded map[string]interface{}
		for i, s := range shapes {
			var body map[string]interface{}
			err = note.JSONUnmarshal([]byte(s.body), &body)
			require.NoError(t, err)

			encodeCtx, err := BulkEncodeTemplate(templateJSON)
			require.NoError(t, err)

			output, err := encodeCtx.BulkEncodeNextEntry(ctx, body, []byte{}, 0, 0, "", "", false)
			require.NoError(t, err, "encode %s", s.name)

			decodeCtx, err := BulkDecodeTemplate(ctx, templateJSON, output)
			require.NoError(t, err)

			decoded, _, _, _, _, _, success := decodeCtx.BulkDecodeNextEntry(ctx)
			require.True(t, success, "decode %s", s.name)

			if i == 0 {
				firstOutput = output
				firstDecoded = decoded
				continue
			}
			assert.Equal(t, firstOutput, output,
				"shape %q must produce identical bytes to %q", s.name, shapes[0].name)

			firstJSON, _ := note.JSONMarshal(firstDecoded)
			gotJSON, _ := note.JSONMarshal(decoded)
			assert.JSONEq(t, string(firstJSON), string(gotJSON),
				"shape %q must round-trip to the same body as %q", s.name, shapes[0].name)
		}
	})

	// (A) Canonical bat_power.qo round-trip — the symptom that surfaced this
	// bug. Multiple VLA fields, body omits two of them in the middle of the
	// record. Pre-fix, every field after the first omitted one decodes to
	// garbage. Post-fix, all values round-trip within float-rounding tolerance.
	t.Run("bat_power_round_trip", func(t *testing.T) {
		template := `{` +
			`"_sensors":"0",` +
			`"_sensor":[21],` +
			`"when_ms_16":[22],` +
			`"when_ms_24":[23],` +
			`"when_ms_32":[24],` +
			`"volts":[14.1],` +
			`"amps":[14.1],` +
			`"temp_shunt":[14.1],` +
			`"ah_in":[14.1],` +
			`"ah_out":[14.1],` +
			`"wh_in":[14.1],` +
			`"wh_out":[14.1]` +
			`}`

		// Body intentionally omits "when_ms_16" and "when_ms_32" — the two
		// VLA fields whose absence triggered the original symptom.
		bodyJSON := `{` +
			`"_sensors":",san:chq7w0f4g6hyg,san:chq7w0f4g6s6g",` +
			`"_sensor":[0,2,0,2,1,0,2,1],` +
			`"when_ms_24":[525,14024,119794,133463,135346,241365,252897,255012],` +
			`"volts":[11.5273494720459,11.5324068069458,11.52229976654053,` +
			`11.52399349212646,11.53074550628662,11.53410053253174,` +
			`11.52948474884033,11.52399349212646],` +
			`"amps":[0.7924892902374268,0.2885665595531464,0.7513015270233154,` +
			`0.293621838092804,0.5004115700721741,0.7854286432266235,` +
			`0.3026634156703949,0.5636722445487976],` +
			`"temp_shunt":[26.27883720397949,25.89171600341797,26.41076469421387,` +
			`25.96841049194336,26.01416969299316,26.53219795227051,` +
			`26.02609634399414,26.11377906799316],` +
			`"ah_in":[0,0,0,0,0,0,0,0],` +
			`"ah_out":[0.01893045762068325,0.05425670677855484,0.04454897559416051,` +
			`0.06378065062663091,0.07095306508612359,0.07081270608760681,` +
			`0.07329920617021135,0.08818646905299284],` +
			`"wh_in":[0,0,0,0,0,0,0,0],` +
			`"wh_out":[0.2182123688414992,0.6252877884332192,0.5135076226366643,` +
			`0.7350713939931611,0.8178172973719245,0.8162261413720263,` +
			`0.8447912482773979,1.016471664258873]` +
			`}`

		templateBody := BulkBody{
			NoteFormat:   BulkNoteFormatFlexNano,
			NoteTemplate: template,
		}
		templateJSON, err := note.JSONMarshal(templateBody)
		require.NoError(t, err)

		var body map[string]interface{}
		err = note.JSONUnmarshal([]byte(bodyJSON), &body)
		require.NoError(t, err)

		encodeCtx, err := BulkEncodeTemplate(templateJSON)
		require.NoError(t, err)

		output, err := encodeCtx.BulkEncodeNextEntry(ctx, body, []byte{}, 0, 0, "", "", false)
		require.NoError(t, err)

		// Sanity-check the encoded length. The 12 template fields produce:
		//   header (BULKFLAGS + variableLengthOffset)            =   3
		//   "_sensors": uint8 length (FlexNano) + 36 bytes        =  37
		//   "_sensor":  uint16 count + 8 * uint8                  =  10
		//   "when_ms_16": uint16 count = 0 (omitted in body)      =   2
		//   "when_ms_24": uint16 count + 8 * uint24               =  26
		//   "when_ms_32": uint16 count = 0 (omitted in body)      =   2
		//   "volts".."wh_out": 7 fields, each uint16 count + 8*f32
		//                              7 * (2 + 8*4) = 7 * 34     = 238
		// Total: 3 + 37 + 10 + 2 + 26 + 2 + 238 = 318.
		// Pre-fix this would be 314 (4 bytes short — both omitted VLA counts
		// are missing entirely).
		assert.Equal(t, 318, len(output),
			"pre-fix encoding is 4 bytes shorter because both omitted VLA counts are missing")

		decodeCtx, err := BulkDecodeTemplate(ctx, templateJSON, output)
		require.NoError(t, err)

		decoded, _, _, _, _, _, success := decodeCtx.BulkDecodeNextEntry(ctx)
		require.True(t, success)

		// _sensors is a fixed string — anchor the alignment.
		assert.Equal(t, ",san:chq7w0f4g6hyg,san:chq7w0f4g6s6g", decoded["_sensors"])

		// Helper to extract a numeric array from the decoded body.
		toFloats := func(t *testing.T, key string) []float64 {
			t.Helper()
			arr, ok := decoded[key].([]interface{})
			require.True(t, ok, "%s should be []interface{}, got %T", key, decoded[key])
			out := make([]float64, len(arr))
			for i, v := range arr {
				n, ok := v.(json.Number)
				require.True(t, ok, "%s[%d] should be json.Number, got %T", key, i, v)
				f, err := n.Float64()
				require.NoError(t, err)
				out[i] = f
			}
			return out
		}

		assert.Equal(t,
			[]float64{0, 2, 0, 2, 1, 0, 2, 1},
			toFloats(t, "_sensor"))

		assert.Equal(t,
			[]float64{525, 14024, 119794, 133463, 135346, 241365, 252897, 255012},
			toFloats(t, "when_ms_24"))

		// The float arrays should round-trip to within float32 precision.
		floatCases := []struct {
			key  string
			want []float64
		}{
			{"volts", []float64{11.5273494720459, 11.5324068069458, 11.52229976654053,
				11.52399349212646, 11.53074550628662, 11.53410053253174,
				11.52948474884033, 11.52399349212646}},
			{"amps", []float64{0.7924892902374268, 0.2885665595531464, 0.7513015270233154,
				0.293621838092804, 0.5004115700721741, 0.7854286432266235,
				0.3026634156703949, 0.5636722445487976}},
			{"temp_shunt", []float64{26.27883720397949, 25.89171600341797, 26.41076469421387,
				25.96841049194336, 26.01416969299316, 26.53219795227051,
				26.02609634399414, 26.11377906799316}},
			{"ah_out", []float64{0.01893045762068325, 0.05425670677855484, 0.04454897559416051,
				0.06378065062663091, 0.07095306508612359, 0.07081270608760681,
				0.07329920617021135, 0.08818646905299284}},
			{"wh_out", []float64{0.2182123688414992, 0.6252877884332192, 0.5135076226366643,
				0.7350713939931611, 0.8178172973719245, 0.8162261413720263,
				0.8447912482773979, 1.016471664258873}},
		}
		// float32 has ~7 decimal digits of precision; relative tolerance of
		// 1e-5 covers all values here including ones near 26 (so absolute
		// error up to ~3e-4).
		for _, fc := range floatCases {
			got := toFloats(t, fc.key)
			require.Len(t, got, len(fc.want), "wrong length for %s", fc.key)
			for i, want := range fc.want {
				tol := 1e-5 * math.Abs(want)
				if tol < 1e-6 {
					tol = 1e-6
				}
				assert.InDelta(t, want, got[i], tol,
					"%s[%d] mismatch (pre-fix this is a garbage value because the encoder skipped 4 bytes earlier in the stream)", fc.key, i)
			}
		}

		// "ah_in" and "wh_in" are arrays of zeros. omitempty doesn't strip
		// arrays, but the per-element zero-stripping inside arrays is not
		// performed either, so they round-trip as [0,0,...].
		assert.Equal(t, []float64{0, 0, 0, 0, 0, 0, 0, 0}, toFloats(t, "ah_in"))
		assert.Equal(t, []float64{0, 0, 0, 0, 0, 0, 0, 0}, toFloats(t, "wh_in"))

		// when_ms_16 and when_ms_32 round-trip as empty arrays (count=0,
		// no elements) since the body omitted them.
		if v, present := decoded["when_ms_16"]; present {
			arr, ok := v.([]interface{})
			require.True(t, ok)
			assert.Empty(t, arr, "when_ms_16 should round-trip as empty array")
		}
		if v, present := decoded["when_ms_32"]; present {
			arr, ok := v.([]interface{})
			require.True(t, ok)
			assert.Empty(t, arr, "when_ms_32 should round-trip as empty array")
		}
	})
}

// Missing values, and objects of another type, encode as empty ones, since the decoder reads every
// field the template defines
func TestBulkEncodeMissingValues(t *testing.T) {
	ctx := context.Background()
	payload := []byte{1, 2, 3}
	cases := []struct {
		name     string
		template string
		body     string // "" for no body
		empty    string // the same note with those values given as empty
		decoded  string
	}{
		{"no body", `{"seq":12,"sub":{"a":11},"pos":[14.1,14.1]}`, "", `{"seq":0,"sub":{},"pos":[]}`, `{"sub":{},"pos":[0,0]}`},
		{"missing object", `{"seq":12,"sub":{"a":11},"n":11}`, `{"seq":5,"n":1}`, `{"seq":5,"sub":{},"n":1}`, `{"seq":5,"sub":{},"n":1}`},
		{"null object", `{"seq":12,"sub":{"a":11},"n":11}`, `{"seq":5,"sub":null,"n":1}`, `{"seq":5,"sub":{},"n":1}`, `{"seq":5,"sub":{},"n":1}`},
		{"object of another type", `{"seq":12,"sub":{"a":11},"n":11}`, `{"seq":5,"sub":"x","n":1}`, `{"seq":5,"sub":{},"n":1}`, `{"seq":5,"sub":{},"n":1}`},
		{"missing nested object", `{"a":{"b":{"c":11}},"d":11}`, `{"a":{},"d":3}`, `{"a":{"b":{}},"d":3}`, `{"a":{"b":{}},"d":3}`},
		{"missing array", `{"pos":[14.1,14.1],"n":11}`, `{"n":1}`, `{"pos":[],"n":1}`, `{"pos":[0,0],"n":1}`},
		{"null array", `{"pos":[14.1,14.1],"n":11}`, `{"pos":null,"n":1}`, `{"pos":[],"n":1}`, `{"pos":[0,0],"n":1}`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			templateJSON := bulkTemplateJSON(t, BulkNoteFormatFlexNano, c.template)
			encode := func(bodyJSON string) []byte {
				var body map[string]interface{}
				if bodyJSON != "" {
					require.NoError(t, note.JSONUnmarshal([]byte(bodyJSON), &body))
				}
				encodeCtx, err := BulkEncodeTemplate(templateJSON)
				require.NoError(t, err)
				record, err := encodeCtx.BulkEncodeNextEntry(ctx, body, payload, 0, 0, "", "", false)
				require.NoError(t, err)
				return record
			}
			record := encode(c.body)
			require.Equal(t, encode(c.empty), record)

			decodeCtx, err := BulkDecodeTemplate(ctx, templateJSON, record)
			require.NoError(t, err)
			body, decodedPayload, _, _, _, _, ok := decodeCtx.BulkDecodeNextEntry(ctx)
			require.True(t, ok)
			bodyJSON, err := note.JSONMarshal(body)
			require.NoError(t, err)
			require.JSONEq(t, c.decoded, string(bodyJSON))
			require.Equal(t, payload, decodedPayload)
		})
	}
}

// ---------------------------------------------------------------------------------------------
// NO_VARIABLE equivalence
//
// The Notecard's encoder (notecard src/app/json.c, ConvertJSONToBinUsingTemplateHandler) ends
// with a "collapse" step: a record carrying no payload, no bool flags and no OLC has its 2-byte
// variableLengthOffset removed and BULKFLAG_NO_VARIABLE (0x80) set in the header instead.  This
// encoder deliberately never does that, because Notecard firmware older than 2025-09 consumes
// the offset unconditionally on decode and would misparse the shorter record.
//
// The two encoders therefore emit different bytes for identical content, and a device that
// checksums its own encoding can never match a checksum of ours.  BulkNoVariableEquivalent
// reconciles that without changing a single transmitted byte.
//
// The vectors below are written out BY HAND from the format the C encoder specifies -- they were
// not produced by running it.  That is deliberate rather than a shortcut: they never consult this
// package's encoder either, so they are an independent statement of the format and they fail if
// either side drifts.  What they do NOT prove is that the C encoder matches its own
// specification, so confirming them against a real Notecard -- a trace capture, or a harness
// linked against the firmware objects calling ConvertJSONToBinUsingTemplate -- would be a
// worthwhile follow-up.
//
// The CRCs assume notecard's crc32Hash (src/global/crchash.c) is plain CRC-32/IEEE, which is what
// Go's hash/crc32.ChecksumIEEE computes.  That reading of the C is unverified against hardware.
// ---------------------------------------------------------------------------------------------

// The env.template a radnote registers for satellite operation: four variable-length strings,
// no numerics, no booleans, no payload.  This is exactly the shape the Notecard collapses and
// this encoder does not.
const radnoteEnvTemplate = `{"sample_mins_ntn":"0","alarm_usv":"0","_sync_outbound_mins_ntn":"0","_sync_inbound_mins_ntn":"0"}`

func bulkTemplateJSON(t *testing.T, noteFormat uint32, template string) []byte {
	t.Helper()
	templateJSON, err := note.JSONMarshal(BulkBody{NoteFormat: noteFormat, NoteTemplate: template})
	require.NoError(t, err)
	return templateJSON
}

func bulkEncodeOne(t *testing.T, noteFormat uint32, template string, body map[string]interface{}) []byte {
	t.Helper()
	encodeCtx, err := BulkEncodeTemplate(bulkTemplateJSON(t, noteFormat, template))
	require.NoError(t, err)
	encoded, err := encodeCtx.BulkEncodeNextEntry(context.Background(), body, []byte{}, 0, 0, "", "", false)
	require.NoError(t, err)
	return encoded
}

// wireVector is one record as a NO_VARIABLE-capable Notecard encodes it, written out by hand
// from the FlexNano wire format rather than captured from a device: a header byte, then each
// field in template order, where a string is a 1-byte length followed by its bytes.  Deriving
// them by hand is the point -- nothing here consults our own encoder, so these vectors are an
// independent statement of what the format IS, and they fail if our encoder drifts from it.
// Confirming them against a real Notecard trace would strengthen them further; none has been
// captured yet.
type wireVector struct {
	name string
	// body is encoded against radnoteEnvTemplate unless template is set
	template string
	body     map[string]interface{}
	// wireBytes is the record as the C encoder in notecard src/app/json.c specifies it, hex
	// encoded, and wireCRC is crc32 over exactly those bytes
	wireBytes string
	wireCRC   uint32
	// collapses is whether the C encoder's collapse step applies to this record shape
	collapses bool
}

var wireVectors = []wireVector{{
	// The steady state of a provisioned radnote: three tiers unset ("-"), alarm disarmed
	name:      "radnote env unset",
	body:      map[string]interface{}{"sample_mins_ntn": "-", "alarm_usv": "0", "_sync_outbound_mins_ntn": "-", "_sync_inbound_mins_ntn": "-"},
	wireBytes: "80012d0130012d012d",
	wireCRC:   0x5c0cf8e2,
	collapses: true,
}, {
	name:      "radnote env configured",
	body:      map[string]interface{}{"sample_mins_ntn": "60", "alarm_usv": "5.0", "_sync_outbound_mins_ntn": "720", "_sync_inbound_mins_ntn": "1440"},
	wireBytes: "8002363003352e30033732300431343430",
	wireCRC:   0xb29447f6,
	collapses: true,
}, {
	// Env vars outside the template must not perturb the encoding -- the device checksums its
	// WHOLE env map through this template, so anything else it holds has to fall away
	name:      "env vars outside the template are ignored",
	body:      map[string]interface{}{"zz_other": "ignored", "sample_mins_ntn": "-", "_air_mins": "5", "alarm_usv": "0", "_sync_outbound_mins_ntn": "-", "_sync_inbound_mins_ntn": "-"},
	wireBytes: "80012d0130012d012d",
	wireCRC:   0x5c0cf8e2,
	collapses: true,
}, {
	// Notehub holding none of the templated names: every field encodes as a zero-length string
	name:      "all fields missing",
	body:      map[string]interface{}{},
	wireBytes: "8000000000",
	wireCRC:   0x77c4478f,
	collapses: true,
}, {
	// The degenerate case: the collapse leaves a single header byte
	name:      "empty template",
	template:  `{}`,
	body:      map[string]interface{}{},
	wireBytes: "80",
	wireCRC:   0x3fba6cad,
	collapses: true,
}, {
	// A bool sets BULKFLAG_FLAGSL, so the Notecard does NOT collapse -- and for these records
	// the two encoders already agree byte for byte, which is why the plain comparison works
	name:      "bool in template suppresses the collapse",
	template:  `{"enabled":true,"name":"0"}`,
	body:      map[string]interface{}{"enabled": true, "name": "x"},
	wireBytes: "040500017801",
	wireCRC:   0x7390c59a,
	collapses: false,
}, {
	// Numeric fields live in the fixed area and collapse just like strings
	name:      "numeric template",
	template:  `{"a":14,"b":14.1}`,
	body:      map[string]interface{}{"a": 7, "b": 0.5},
	wireBytes: "80070000000000003f",
	wireCRC:   0x3cff30c5,
	collapses: true,
}}

// TestBulkNoVariableEquivalentMatchesWireFormat is the anchor of this suite: it pins our output
// against the wire format itself, spelled out by hand in wireVectors.  For every record shape,
// exactly one of {what we transmit, the equivalent form} is byte-identical to what a
// NO_VARIABLE-capable device encodes -- so comparing a device checksum against both always has a
// chance of matching, and comparing against only one never did.
func TestBulkNoVariableEquivalentMatchesWireFormat(t *testing.T) {
	for _, v := range wireVectors {
		t.Run(v.name, func(t *testing.T) {
			template := v.template
			if template == "" {
				template = radnoteEnvTemplate
			}
			wantWire, err := hex.DecodeString(v.wireBytes)
			require.NoError(t, err)
			require.Equal(t, v.wireCRC, crc32.ChecksumIEEE(wantWire),
				"the vector bytes and vector CRC disagree; one of them was transcribed wrong")

			transmitted := bulkEncodeOne(t, BulkNoteFormatFlexNano, template, v.body)
			equivalent := BulkNoVariableEquivalent(BulkNoteFormatFlexNano, transmitted)

			// Whatever we transmit must stay decodable by pre-2025-09 firmware
			require.Equal(t, uint8(0), transmitted[0]&bulkflagNoVariable,
				"transmitted form must never set NO_VARIABLE")
			require.GreaterOrEqual(t, len(transmitted), 3, "transmitted form keeps its 2-byte offset")

			if !v.collapses {
				require.Nil(t, equivalent,
					"the Notecard did not collapse this record, so there is no equivalent form")
				require.Equal(t, wantWire, transmitted,
					"for an uncollapsed record the two encoders must already agree byte for byte")
				return
			}

			require.NotNil(t, equivalent, "the Notecard collapsed this record")
			require.Equal(t, wantWire, equivalent,
				"equivalent form must reproduce the Notecard's bytes exactly")
			require.Equal(t, v.wireCRC, crc32.ChecksumIEEE(equivalent),
				"this is the checksum the device sends us; it is what the comparison must accept")

			// And demonstrate the bug that motivated all of this: without the equivalent form
			// the comparison can never succeed for these records
			require.NotEqual(t, v.wireCRC, crc32.ChecksumIEEE(transmitted),
				"if these ever matched, the equivalent form would be unnecessary")
		})
	}
}

// TestBulkNoVariableEquivalentShape pins the transform itself, independent of any vector: the
// equivalent form is the transmitted form with the offset excised and NO_VARIABLE set, and
// nothing else moves.
func TestBulkNoVariableEquivalentShape(t *testing.T) {
	for _, v := range wireVectors {
		if !v.collapses {
			continue
		}
		t.Run(v.name, func(t *testing.T) {
			template := v.template
			if template == "" {
				template = radnoteEnvTemplate
			}
			transmitted := bulkEncodeOne(t, BulkNoteFormatFlexNano, template, v.body)
			equivalent := BulkNoVariableEquivalent(BulkNoteFormatFlexNano, transmitted)
			require.NotNil(t, equivalent)

			require.Len(t, equivalent, len(transmitted)-2, "exactly the 2-byte offset is dropped")
			require.Equal(t, transmitted[0]|bulkflagNoVariable, equivalent[0], "header gains only NO_VARIABLE")
			require.Equal(t, transmitted[3:], equivalent[1:], "field bytes are untouched")

			// The two forms are structurally distinguishable by bit 0x80, so accepting both can
			// never make a DIFFERENT environment look equal -- only a genuine CRC32 collision can
			require.NotEqual(t, transmitted[0]&bulkflagNoVariable, equivalent[0]&bulkflagNoVariable)
		})
	}
}

// TestBulkNoVariableEquivalentPreservesNoOmitEmpty covers the one header bit the Notecard
// deliberately does NOT count as variable data.  Its hasVariableData test looks at PAYLOAD,
// FLAGS and OLC only, so a noOmitEmpty record still collapses and keeps 0x40 alongside 0x80.
func TestBulkNoVariableEquivalentPreservesNoOmitEmpty(t *testing.T) {
	encodeCtx, err := BulkEncodeTemplate(bulkTemplateJSON(t, BulkNoteFormatFlexNano, radnoteEnvTemplate))
	require.NoError(t, err)
	transmitted, err := encodeCtx.BulkEncodeNextEntry(context.Background(),
		map[string]interface{}{"sample_mins_ntn": "-", "alarm_usv": "0",
			"_sync_outbound_mins_ntn": "-", "_sync_inbound_mins_ntn": "-"},
		[]byte{}, 0, 0, "", "", true)
	require.NoError(t, err)
	require.Equal(t, uint8(bulkflagNoOmitEmpty), transmitted[0], "noOmitEmpty must set 0x40 and nothing else")

	equivalent := BulkNoVariableEquivalent(BulkNoteFormatFlexNano, transmitted)
	require.NotNil(t, equivalent, "noOmitEmpty is not variable data, so this still collapses")
	require.Equal(t, uint8(bulkflagNoOmitEmpty|bulkflagNoVariable), equivalent[0],
		"the equivalent form must carry both 0x40 and 0x80")
}

// TestBulkNoVariableEquivalentDecodesIdentically is the content-equivalence check.  The decoder
// is an independent oracle: it understood bulkflagNoVariable long before this helper existed, so
// agreeing with it proves the helper rewrites framing without disturbing content.
func TestBulkNoVariableEquivalentDecodesIdentically(t *testing.T) {
	ctx := context.Background()

	for _, tc := range []struct {
		name     string
		template string
		body     map[string]interface{}
	}{
		{"strings", radnoteEnvTemplate, map[string]interface{}{
			"sample_mins_ntn":         "usb:5;high:15;normal:15;low:60;0",
			"alarm_usv":               "0.25",
			"_sync_outbound_mins_ntn": "2880",
			"_sync_inbound_mins_ntn":  "-",
		}},
		{"signed ints", `{"a":11,"b":12,"c":13,"d":14,"e":18}`, map[string]interface{}{
			"a": 100, "b": 30000, "c": 8000000, "d": 2000000000, "e": 90000000000,
		}},
		{"unsigned ints", `{"a":21,"b":22,"c":23,"d":24}`, map[string]interface{}{
			"a": 250, "b": 65000, "c": 16000000, "d": 4000000000,
		}},
		{"floats", `{"h":12.1,"f":14.1,"d":18.1}`, map[string]interface{}{
			"h": 1.5, "f": 2.25, "d": 3.125,
		}},
		{"mixed", `{"n":"0","i":14,"f":14.1}`, map[string]interface{}{
			"n": "mixed", "i": 7, "f": 0.5,
		}},
		{"empty string fields", radnoteEnvTemplate, map[string]interface{}{
			"sample_mins_ntn": "", "alarm_usv": "", "_sync_outbound_mins_ntn": "", "_sync_inbound_mins_ntn": "",
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			templateJSON := bulkTemplateJSON(t, BulkNoteFormatFlexNano, tc.template)
			encodeCtx, err := BulkEncodeTemplate(templateJSON)
			require.NoError(t, err)
			transmitted, err := encodeCtx.BulkEncodeNextEntry(ctx, tc.body, []byte{}, 0, 0, "", "", false)
			require.NoError(t, err)

			equivalent := BulkNoVariableEquivalent(BulkNoteFormatFlexNano, transmitted)
			require.NotNil(t, equivalent, "no payload, flags or OLC, so this has an equivalent form")

			decode := func(binaryData []byte) map[string]interface{} {
				decodeCtx, err := BulkDecodeTemplate(ctx, templateJSON, binaryData)
				require.NoError(t, err)
				body, _, _, _, _, _, success := decodeCtx.BulkDecodeNextEntry(ctx)
				require.True(t, success)
				return body
			}
			require.Equal(t, decode(transmitted), decode(equivalent),
				"both framings must decode to the same body")
		})
	}
}

// TestBulkNoVariableEquivalentRefuses covers every case where the Notecard would have left the
// record alone.  Returning a value in any of these would make the tolerant comparison accept a
// checksum no device ever computes, silently suppressing an environment update the device needs.
func TestBulkNoVariableEquivalentRefuses(t *testing.T) {
	ctx := context.Background()
	env := map[string]interface{}{"sample_mins_ntn": "-", "alarm_usv": "0",
		"_sync_outbound_mins_ntn": "-", "_sync_inbound_mins_ntn": "-"}
	encoded := bulkEncodeOne(t, BulkNoteFormatFlexNano, radnoteEnvTemplate, env)

	t.Run("original format never collapses", func(t *testing.T) {
		// Mirrors the C guard `if (binFormat != BULK_NOTEFORMAT_ORIGINAL)`
		require.Nil(t, BulkNoVariableEquivalent(BulkNoteFormatOriginal, encoded))
	})
	t.Run("too short to carry a header and offset", func(t *testing.T) {
		require.Nil(t, BulkNoVariableEquivalent(BulkNoteFormatFlexNano, nil))
		require.Nil(t, BulkNoVariableEquivalent(BulkNoteFormatFlexNano, []byte{}))
		require.Nil(t, BulkNoVariableEquivalent(BulkNoteFormatFlexNano, []byte{0x00}))
		require.Nil(t, BulkNoVariableEquivalent(BulkNoteFormatFlexNano, []byte{0x00, 0x00}))
	})
	t.Run("already collapsed", func(t *testing.T) {
		require.Nil(t, BulkNoVariableEquivalent(BulkNoteFormatFlexNano,
			append([]byte{bulkflagNoVariable}, encoded[3:]...)))
	})
	t.Run("payload present", func(t *testing.T) {
		encodeCtx, err := BulkEncodeTemplate(bulkTemplateJSON(t, BulkNoteFormatFlexNano, radnoteEnvTemplate))
		require.NoError(t, err)
		withPayload, err := encodeCtx.BulkEncodeNextEntry(ctx, env, []byte{1, 2, 3}, 0, 0, "", "", false)
		require.NoError(t, err)
		require.NotEqual(t, uint8(0), withPayload[0]&(bulkflagPayloadL|bulkflagPayloadH))
		require.Nil(t, BulkNoVariableEquivalent(BulkNoteFormatFlexNano, withPayload))
	})
	t.Run("bool flags present", func(t *testing.T) {
		withFlags := bulkEncodeOne(t, BulkNoteFormatFlexNano, `{"enabled":true,"name":"0"}`,
			map[string]interface{}{"enabled": true, "name": "x"})
		require.NotEqual(t, uint8(0), withFlags[0]&(bulkflagFlagsL|bulkflagFlagsH))
		require.Nil(t, BulkNoVariableEquivalent(BulkNoteFormatFlexNano, withFlags))
	})
	t.Run("unconvertible OLC occupies the variable area", func(t *testing.T) {
		// Only Flex carries an OLC; an OLC that will not pack into an int64 is appended to the
		// variable-length area, which sets BULKFLAG_OLCL and suppresses the collapse.  Verified
		// against the firmware: it emits header 0x10 for exactly this input.
		encodeCtx, err := BulkEncodeTemplate(bulkTemplateJSON(t, BulkNoteFormatFlex, radnoteEnvTemplate))
		require.NoError(t, err)
		withOLC, err := encodeCtx.BulkEncodeNextEntry(ctx, env, []byte{}, 0, 0, "not-an-olc", "", false)
		require.NoError(t, err)
		require.Equal(t, uint8(bulkflagOLCL), withOLC[0]&(bulkflagOLCL|bulkflagOLCH))
		require.Nil(t, BulkNoVariableEquivalent(BulkNoteFormatFlex, withOLC))
	})
}

// TestBulkNoVariableEquivalentFlexCollapseDecision checks the collapse PREDICATE for Flex, which
// packet.go will happily hand us because the note format is whatever the device registered.
//
// Only the predicate: the Flex records themselves are NOT byte-comparable with the Notecard's,
// for two reasons unrelated to NO_VARIABLE.  BulkEncodeNextEntry ignores its `when` argument and
// substitutes time.Now(), and this package's OLCToINT64 is missing the `plusCode` validity check
// that notecard src/app/olc64.c ends with -- so for an empty OLC the firmware writes -1 ("no
// location") where we write 6221821273427820543.  A Flex env template would therefore still
// mismatch after this fix.  In practice reqenv.c hardwires the env template to FlexNano, so the
// env path is unaffected.  See the residual-issues note on this change.
func TestBulkNoVariableEquivalentFlexCollapseDecision(t *testing.T) {
	ctx := context.Background()
	env := map[string]interface{}{"sample_mins_ntn": "-", "alarm_usv": "0",
		"_sync_outbound_mins_ntn": "-", "_sync_inbound_mins_ntn": "-"}

	for _, tc := range []struct {
		name          string
		olc           string
		wantCollapses bool
	}{
		// Firmware ground truth: header 0x80 (collapsed) for both of these
		{"no olc", "", true},
		{"olc that packs into an int64", "87JC9RF9+86XJ", true},
		// Firmware ground truth: header 0x10 (BULKFLAG_OLCL), not collapsed
		{"olc that does not pack", "not-an-olc", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			encodeCtx, err := BulkEncodeTemplate(bulkTemplateJSON(t, BulkNoteFormatFlex, radnoteEnvTemplate))
			require.NoError(t, err)
			transmitted, err := encodeCtx.BulkEncodeNextEntry(ctx, env, []byte{}, 0, 0, tc.olc, "", false)
			require.NoError(t, err)

			equivalent := BulkNoVariableEquivalent(BulkNoteFormatFlex, transmitted)
			if !tc.wantCollapses {
				require.Nil(t, equivalent)
				return
			}
			require.NotNil(t, equivalent)
			require.Len(t, equivalent, len(transmitted)-2)
			require.Equal(t, transmitted[0]|bulkflagNoVariable, equivalent[0])
			require.Equal(t, transmitted[3:], equivalent[1:])

			// Flex keeps when/olc64 in the fixed area, so the collapsed form must still decode.
			// Unlike FlexNano, a Flex payload is snappy-compressed in transit (bulk.go:186).
			decodeCtx, err := BulkDecodeTemplate(ctx,
				bulkTemplateJSON(t, BulkNoteFormatFlex, radnoteEnvTemplate),
				snappy.Encode(nil, equivalent))
			require.NoError(t, err)
			decoded, _, _, _, _, _, success := decodeCtx.BulkDecodeNextEntry(ctx)
			require.True(t, success, "the collapsed Flex form must decode")
			require.Equal(t, "-", decoded["sample_mins_ntn"],
				"the collapsed Flex form must decode to the right fields, not shifted by two bytes")
		})
	}
}

// TestBulkEncoderStaysUncollapsed is a tripwire, not a behavioural test.  Notecard firmware
// predating the 2025-09 NO_VARIABLE change consumes the 2-byte offset unconditionally on decode,
// so it would misparse a collapsed record and write garbage env values.  Anyone who later
// "fixes" this encoder to match the C side must fail here and go read why.  Changing it is a
// wire break that needs a firmware floor and a LoRa story first: loranote/App/binary.c is a
// separate decoder that has never supported NO_VARIABLE at all.
func TestBulkEncoderStaysUncollapsed(t *testing.T) {
	for _, noteFormat := range []uint32{BulkNoteFormatFlex, BulkNoteFormatFlexNano} {
		encoded := bulkEncodeOne(t, noteFormat, radnoteEnvTemplate,
			map[string]interface{}{"sample_mins_ntn": "-", "alarm_usv": "0",
				"_sync_outbound_mins_ntn": "-", "_sync_inbound_mins_ntn": "-"})
		require.Equal(t, uint8(0), encoded[0]&bulkflagNoVariable,
			"format %d: transmitted encoding must not set NO_VARIABLE -- older Notecards cannot decode it",
			noteFormat)
		require.GreaterOrEqual(t, len(encoded), 3,
			"format %d: the 2-byte variable-length offset must still be present", noteFormat)
	}
}
