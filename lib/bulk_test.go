package notelib

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBulkDecode(t *testing.T) {
	context, err := BulkDecodeTemplate(
		[]byte(`{"format":2,"storage":"file:data/test_note-qo.000","template_body":"{\"id\":24,\"description\":\"type_hint\"}","template_payload":1024}`),
		[]byte{167, 1, 152, 1, 38, 0, 2, 182, 219, 68, 209, 235, 39, 23, 137, 112, 31, 54, 63, 130, 138, 24, 0, 0, 0, 0, 13, 0, 49, 50, 56, 32, 98, 121, 116, 101, 32, 110, 111, 116, 101, 128, 1, 20, 254, 1, 0, 238, 1, 0},
	)
	require.NoError(t, err)

	body, payload, when, where, wherewhen, olc, success := context.BulkDecodeNextEntry()

	require.Equal(t, map[string]interface{}{"description": "128 byte note"}, body)
	require.Equal(t, bytes.Repeat([]byte{0}, 128), payload)
	require.Equal(t, int64(1668561471), when)
	require.Equal(t, int64(1768369011698921609), where)
	require.Equal(t, int64(1668561470), wherewhen)
	require.Equal(t, "", olc)
	require.Equal(t, true, success)

	body, payload, when, where, wherewhen, olc, success = context.BulkDecodeNextEntry()

	require.Nil(t, body)
	require.Nil(t, payload)
	require.Equal(t, int64(0), when)
	require.Equal(t, int64(0), where)
	require.Equal(t, int64(0), wherewhen)
	require.Equal(t, "", olc)
	require.Equal(t, false, success)
}
