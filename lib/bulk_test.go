package notelib

import (
	"bytes"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

const verboseBulkTest = false

func TestBulkDecode(t *testing.T) {
	context, err := BulkDecodeTemplate(
		[]byte(`{"format":2,"storage":"file:data/test_note-qo.000","template_body":"{\"id\":24,\"description\":\"type_hint\"}","template_payload":1024}`),
		[]byte{167, 1, 152, 1, 38, 0, 2, 182, 219, 68, 209, 235, 39, 23, 137, 112, 31, 54, 63, 130, 138, 24, 0, 0, 0, 0, 13, 0, 49, 50, 56, 32, 98, 121, 116, 101, 32, 110, 111, 116, 101, 128, 1, 20, 254, 1, 0, 238, 1, 0},
	)
	require.NoError(t, err)

	body, payload, when, where, wherewhen, olc, _, success := context.BulkDecodeNextEntry()

	require.Equal(t, map[string]interface{}{"description": "128 byte note"}, body)
	require.Equal(t, bytes.Repeat([]byte{0}, 128), payload)
	require.Equal(t, int64(1668561471), when)
	require.Equal(t, int64(1768369011698921609), where)
	require.Equal(t, int64(1668561470), wherewhen)
	require.Equal(t, "", olc)
	require.Equal(t, true, success)

	body, payload, when, where, wherewhen, olc, _, success = context.BulkDecodeNextEntry()

	require.Nil(t, body)
	require.Nil(t, payload)
	require.Equal(t, int64(0), when)
	require.Equal(t, int64(0), where)
	require.Equal(t, int64(0), wherewhen)
	require.Equal(t, "", olc)
	require.Equal(t, false, success)
}

func TestBulkEncode(t *testing.T) {

	templateJSON := `{"sensor1":{"temp":14.1,"humidity":14.1},"mylist":[{"val":14},{"val":14}],"sep14":"0","sensor2":{"temp":14.1,"humidity":14.1},"sep23":"0","sensor3":{"temp":14.1,"humidity":14.1}}`

	bulkBody := BulkBody{}
	bulkBody.NoteFormat = BulkNoteFormatFlexNano
	bulkBody.NoteTemplate = templateJSON
	bulkBodyJSON, err := json.Marshal(bulkBody)
	require.NoError(t, err)

	context, err := BulkEncodeTemplate(bulkBodyJSON)
	require.NoError(t, err)

	dataJSON := `{"beep":true,"sep12":"valueSep12","sensor1":{"hi":"there","temp":101.5,"humidity":151.5},"sensor2":{"temp":201.5,"humidity":201.5},"sensor3":{"humidity":302.3,"temp":305},"mylist":[{"val":9991},{"val":9992}],"sep23":"valueSep23"}`
	var data map[string]interface{}
	err = json.Unmarshal([]byte(dataJSON), &data)
	require.NoError(t, err)

	output, err := context.BulkEncodeNextEntry(data, []byte{}, 0, 0, "", "", false)
	require.NoError(t, err)

	require.Equal(t, output, []byte{0x0, 0x2f, 0x0, 0x0, 0x0, 0xcb, 0x42, 0x0, 0x80, 0x17, 0x43, 0x7, 0x27, 0x0, 0x0, 0x8, 0x27, 0x0, 0x0, 0x0, 0x0, 0x80, 0x49, 0x43, 0x0, 0x80, 0x49, 0x43, 0xa, 0x76, 0x61, 0x6c, 0x75, 0x65, 0x53, 0x65, 0x70, 0x32, 0x33, 0x0, 0x80, 0x98, 0x43, 0x66, 0x26, 0x97, 0x43})

	context, err = BulkDecodeTemplate(bulkBodyJSON, output)
	require.NoError(t, err)

	body, payload, _, _, _, _, _, success := context.BulkDecodeNextEntry()
	require.Equal(t, true, success)
	require.Equal(t, "", string(payload))

	bodyJSON, err := json.Marshal(body)
	require.NoError(t, err)
	if verboseBulkTest {
		fmt.Printf("Round-trip:\n")
		fmt.Printf("  *** before ***\n%s\n", string(dataJSON))
		fmt.Printf("  *** after ***\n%s\n", string(bodyJSON))
	}

}
