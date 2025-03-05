package notelib

import (
	"testing"

	"github.com/blues/note-go/note"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func isValidUUID(u string) bool {
	_, err := uuid.Parse(u)
	return err == nil
}

func TestGenerateEventUid(t *testing.T) {
	// nil events or events without a When field
	// always generate a UUID
	e := note.Event{}
	a := GenerateEventUid(nil)
	b := GenerateEventUid(&e)
	c := GenerateEventUid(&e)
	require.True(t, isValidUUID(a))
	require.True(t, isValidUUID(b))
	require.True(t, isValidUUID(c))
	require.NotEqual(t, a, b)
	require.NotEqual(t, b, c)

	// an event with When field set should always
	// generate a deterministic UID which will look
	// something like this:
	//
	// 33cdeccc-cebe-8032-9f1f-dbee7f5874cb
	e = note.Event{When: 1}
	b = GenerateEventUid(&e)
	c = GenerateEventUid(&e)
	require.Equal(t, b, c)
	require.Equal(t, b, "33cdeccc-cebe-8032-9f1f-dbee7f5874cb")

	// The `When`, `Body`, `Payload` and `Details` fields
	// are all used to construct the UID
	e = note.Event{
		When: 1,
		Body: &map[string]interface{}{
			"key": "value",
			"obj": map[string]int{
				"a": 1,
				"b": 2,
			},
			"list": []string{"a", "b", "c"},
		},
		Payload: []byte{1, 2, 3, 4, 5, 6, 7, 8, 9},
		Details: &map[string]interface{}{
			"key":  1,
			"key2": "value2",
		},
	}

	b = GenerateEventUid(&e)
	c = GenerateEventUid(&e)
	require.Equal(t, b, c)
	require.Equal(t, b, "edd13f0c-b9b8-836c-8c1c-631230e21e56")
}
