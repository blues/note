package notelib

import (
	"math"
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
	require.Equal(t, "33cdeccc-cebe-8032-9f1f-dbee7f5874cb", b)

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
	require.Equal(t, "edd13f0c-b9b8-836c-8c1c-631230e21e56", b)
}

func TestGenerateEventUidWhenMsAndSensor(t *testing.T) {
	// Legacy events (no SensorUID, no WhenMs) must keep their existing UID; adding the
	// new fields to the hash must not churn UIDs for events that don't set them.
	require.Equal(t, "33cdeccc-cebe-8032-9f1f-dbee7f5874cb",
		GenerateEventUid(&note.Event{When: 1}))

	// WhenMs is an epoch-millisecond timestamp and distinguishes events that
	// share the same whole-second When.
	base := note.Event{DeviceUID: "dev:1", When: 1, NotefileID: "f.qo"}
	withMs := base
	withMs.WhenMs = 1500
	require.NotEqual(t, GenerateEventUid(&base), GenerateEventUid(&withMs))

	// Same WhenMs is deterministic; a different WhenMs differs.
	withMsSame := base
	withMsSame.WhenMs = 1500
	require.Equal(t, GenerateEventUid(&withMs), GenerateEventUid(&withMsSame))
	withMsOther := base
	withMsOther.WhenMs = 1600
	require.NotEqual(t, GenerateEventUid(&withMs), GenerateEventUid(&withMsOther))

	// SensorUID distinguishes sensors reporting through the same gateway device with the
	// same captured time. Different SensorUID -> different UID; same -> deterministic.
	s1 := note.Event{DeviceUID: "dev:1", When: 1, SensorUID: "san:1"}
	s2 := note.Event{DeviceUID: "dev:1", When: 1, SensorUID: "foo:2"}
	require.NotEqual(t, GenerateEventUid(&s1), GenerateEventUid(&s2))
	require.Equal(t, GenerateEventUid(&s1),
		GenerateEventUid(&note.Event{DeviceUID: "dev:1", When: 1, SensorUID: "san:1"}))

	// DeviceUID and SensorUID must not ambiguously concatenate: ("dev:ab","") and
	// ("dev:a","b") are different events and must hash to different UIDs.
	require.NotEqual(t,
		GenerateEventUid(&note.Event{DeviceUID: "dev:ab", When: 1}),
		GenerateEventUid(&note.Event{DeviceUID: "dev:a", SensorUID: "b", When: 1}))

	// WhenMs alone is a complete captured timestamp.
	msOnly := note.Event{DeviceUID: "dev:1", WhenMs: 1500}
	require.Equal(t, GenerateEventUid(&msOnly),
		GenerateEventUid(&note.Event{DeviceUID: "dev:1", WhenMs: 1500}))
	require.True(t, isValidUUID(GenerateEventUid(&msOnly)))
}

func TestGenerateEventUidBatch(t *testing.T) {
	base := note.Event{
		DeviceUID:  "dev:1",
		SensorUID:  "san:1",
		NotefileID: "data.qo",
		WhenMs:     1800000000100,
		Body:       &map[string]interface{}{"temperature": 20},
	}
	batch := base
	batch.BulkReceived = 1800000000.123456
	batch.BulkTotal = 10
	batch.BulkNumber = 1
	batchUID := GenerateEventUid(&batch)
	require.NotEqual(t, GenerateEventUid(&base), batchUID)
	require.Equal(t, batchUID, GenerateEventUid(&batch), "the same batch row remains deterministic")

	for _, tc := range []struct {
		name   string
		change func(*note.Event)
	}{
		{"received", func(e *note.Event) {
			e.BulkReceived = math.Nextafter(batch.BulkReceived, math.Inf(1))
		}},
		{"total", func(e *note.Event) { e.BulkTotal = batch.BulkTotal + 1 }},
		{"number", func(e *note.Event) { e.BulkNumber = batch.BulkNumber + 1 }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			changed := batch
			tc.change(&changed)
			require.NotEqual(t, batchUID, GenerateEventUid(&changed),
				"batch metadata must distinguish otherwise identical samples")

			onlyField := base
			tc.change(&onlyField)
			require.NotEqual(t, GenerateEventUid(&base), GenerateEventUid(&onlyField),
				"each batch field must participate even when the other two are zero")
		})
	}
}
