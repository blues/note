// Copyright 2026 Blues Inc.  All rights reserved.
// Use of this source code is governed by licenses granted by the
// copyright holder including that found in the LICENSE file.

package notelib

import (
	"context"
	"testing"

	"github.com/blues/note-go/note"
	"github.com/golang/snappy"
	"github.com/stretchr/testify/require"
)

func TestJcWireLen(t *testing.T) {
	tests := []struct {
		name    string
		body    map[string]interface{}
		payload []byte
		want    uint32
	}{
		{"nothing", nil, nil, 0},
		{"empty body", map[string]interface{}{}, nil, 0},
		{"payload only", nil, []byte{1, 2, 3}, 3},
		// {"a\0051}
		{"flat", map[string]interface{}{"a": 1}, nil, 6},
		// {"b\005\010}
		{"true", map[string]interface{}{"b": true}, nil, 6},
		// {"a\003"b\0051\004c\0052}
		{"nested", map[string]interface{}{"a": map[string]interface{}{"b": 1}, "c": 2}, nil, 13},
		{"nested with payload", map[string]interface{}{"a": map[string]interface{}{"b": 1}, "c": 2}, []byte{1, 2, 3, 4, 5}, 18},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, jcWireLen(tc.body, tc.payload))
		})
	}
}

func TestBodyWireLen(t *testing.T) {
	ctx := context.Background()

	// The body's wire length is exactly the size of its jc-compressed form
	msg := notehubMessage{MessageType: msgNotefileAddNote, NotefileID: "data.qo"}
	require.NoError(t, msg.SetBody(map[string]interface{}{"temp": 12.5, "ok": true, "label": "hello"}))
	msg.SetPayload([]byte{1, 2, 3})
	wire, _, err := msgToWire(msg)
	require.NoError(t, err)

	decoded, _, err := msgFromWire(ctx, wire)
	require.NoError(t, err)
	require.Equal(t, len(msg.cf.Body), decoded.GetBodyWireLen())
	require.Equal(t, []byte{1, 2, 3}, decoded.GetPayload())

	// A message without a body has no body wire length
	msg = notehubMessage{MessageType: msgNotefileAddNote, NotefileID: "data.qo"}
	wire, _, err = msgToWire(msg)
	require.NoError(t, err)
	decoded, _, err = msgFromWire(ctx, wire)
	require.NoError(t, err)
	require.Zero(t, decoded.GetBodyWireLen())
	require.Zero(t, (&notehubMessage{}).GetBodyWireLen())
}

// captureEvents returns an event info whose event function records each event
func captureEvents(events *[]note.Event) *NotefileEventInfo {
	return &NotefileEventInfo{Fn: func(ctx context.Context, sess *HubSession, local bool, event *note.Event) error {
		*events = append(*events, *event)
		return nil
	}}
}

func TestWireLenIsConsumedByEvent(t *testing.T) {
	ctx := context.Background()
	var events []note.Event
	ei := captureEvents(&events)
	nf := CreateNotefile(false)

	// A length set just before the emitting call lands on that event, and is consumed
	ei.WireLen = 42
	_, err := nf.AddNote(ctx, ei, note.DefaultDeviceEndpointID, "a", note.Note{Body: map[string]interface{}{"x": 1}})
	require.NoError(t, err)
	require.Len(t, events, 1)
	require.Equal(t, uint32(42), events[0].WireLen)
	require.Zero(t, ei.WireLen)

	// It is not carried over to the next event
	_, err = nf.AddNote(ctx, ei, note.DefaultDeviceEndpointID, "b", note.Note{Body: map[string]interface{}{"x": 2}})
	require.NoError(t, err)
	require.Len(t, events, 2)
	require.Zero(t, events[1].WireLen)

	// It is consumed even when there is no event function
	ei.Fn = nil
	ei.WireLen = 7
	_, err = nf.AddNote(ctx, ei, note.DefaultDeviceEndpointID, "c", note.Note{Body: map[string]interface{}{"x": 3}})
	require.NoError(t, err)
	require.Zero(t, ei.WireLen)
}

func TestWireLenMerge(t *testing.T) {
	ctx := context.Background()
	var events []note.Event
	ei := captureEvents(&events)

	deviceHistory := func(when int64) *[]note.History {
		return &[]note.History{{EndpointID: note.DefaultDeviceEndpointID, When: when}}
	}

	// The hub already has a newer version of "conflict" than the one the device sends
	nf := CreateNotefile(false)
	nf.Notes["conflict"] = note.Note{
		Body:      map[string]interface{}{"hub": true},
		Updates:   2,
		Histories: &[]note.History{{EndpointID: note.DefaultHubEndpointID, When: 200}},
	}

	liveBody := map[string]interface{}{"temp": 12.5, "ok": true}
	livePayload := []byte{1, 2, 3, 4}
	sentBody := map[string]interface{}{"queued": "yes"}
	from := CreateNotefile(false)
	from.Notes["live"] = note.Note{Body: liveBody, Payload: livePayload, Histories: deviceHistory(100)}
	// Queue notes arrive as "sent" tombstones that still carry their body and payload
	from.Notes["sent"] = note.Note{Body: sentBody, Payload: livePayload, Sent: true, Deleted: true, Histories: deviceHistory(100)}
	// A new note with an empty history is measured like any other
	from.Notes["nohistory"] = note.Note{Body: liveBody, Histories: &[]note.History{}}
	from.Notes["bulk"] = note.Note{Body: map[string]interface{}{"format": 3}, Payload: []byte{9, 9}, Bulk: true, Histories: deviceHistory(100)}
	from.Notes["deleted"] = note.Note{Body: map[string]interface{}{"x": 1}, Deleted: true, Histories: deviceHistory(100)}
	conflictBody := map[string]interface{}{"device": true}
	from.Notes["conflict"] = note.Note{Body: conflictBody, Updates: 1, Histories: deviceHistory(100)}

	require.NoError(t, nf.MergeNotefile(ctx, ei, from))
	require.Zero(t, ei.WireLen)

	// Sent notes' events carry no NoteID, so they are keyed by their body
	lengths := map[string]uint32{}
	for _, event := range events {
		if event.NoteID == "" {
			require.Equal(t, sentBody, *event.Body)
			lengths["sent"] = event.WireLen
		} else {
			lengths[event.NoteID] = event.WireLen
		}
	}
	require.Len(t, lengths, 6)

	// Every note the device sent is measured, even one whose conflict the hub wins, except
	// deletions and bulk carriers (whose records are measured when they are expanded)
	require.NotZero(t, lengths["live"])
	require.Equal(t, jcWireLen(liveBody, livePayload), lengths["live"])
	require.Equal(t, jcWireLen(sentBody, livePayload), lengths["sent"])
	require.Equal(t, jcWireLen(liveBody, nil), lengths["nohistory"])
	require.Zero(t, lengths["bulk"])
	require.Zero(t, lengths["deleted"])
	require.Equal(t, jcWireLen(conflictBody, nil), lengths["conflict"])
}

func TestBulkBinLenDeltaIsRecordLength(t *testing.T) {
	ctx := context.Background()
	for _, format := range []uint32{BulkNoteFormatFlex, BulkNoteFormatFlexNano} {
		bulkBody := BulkBody{NoteFormat: format, NoteTemplate: `{"temp":12.1,"count":14,"label":"10"}`}
		bulkBodyJSON, err := note.JSONMarshal(bulkBody)
		require.NoError(t, err)

		// Each record is encoded with its own context, as the encoder accumulates its output
		records := [][]byte{}
		bin := []byte{}
		for i, data := range []map[string]interface{}{
			{"temp": 9.5, "count": 1, "label": "a"},
			{"temp": 10.5, "count": 2, "label": "a longer label"},
			{"count": 3},
		} {
			encodeCtx, err := BulkEncodeTemplate(bulkBodyJSON)
			require.NoError(t, err)
			record, err := encodeCtx.BulkEncodeNextEntry(ctx, data, []byte{byte(i)}, 1700000000+int64(i), 0, "", "", false)
			require.NoError(t, err)
			records = append(records, record)
			bin = append(bin, record...)
		}
		if format != BulkNoteFormatFlexNano {
			bin = snappy.Encode(nil, bin)
		}

		// Each successful decode consumes exactly one record's bytes, including when/where
		decodeCtx, err := BulkDecodeTemplate(ctx, bulkBodyJSON, bin)
		require.NoError(t, err)
		for i, record := range records {
			recordStart := decodeCtx.BinLen
			_, _, _, _, _, _, success := decodeCtx.BulkDecodeNextEntry(ctx)
			require.True(t, success, "format %d record %d", format, i)
			require.Equal(t, len(record), recordStart-decodeCtx.BinLen, "format %d record %d", format, i)
		}

		// A failed decode consumes nothing
		recordStart := decodeCtx.BinLen
		_, _, _, _, _, _, success := decodeCtx.BulkDecodeNextEntry(ctx)
		require.False(t, success)
		require.Equal(t, recordStart, decodeCtx.BinLen)
	}
}

func TestPacketWireDataLen(t *testing.T) {
	compressible := make([]byte, 200)
	incompressible := []byte{0x9f, 0x03, 0x77, 0x41, 0xe2}
	for _, encrypted := range []bool{false, true} {
		h := &PacketHandler{}
		h.Notecard.PacketDownlinkMtu = 1024
		h.Notecard.EncrAlg = "chacha20-poly1305"
		h.Notecard.EncrKey = make([]byte, 32)
		h.Notehub.MustEncrypt = encrypted

		// The data's length as sent is what crossed the wire, whether or not it was compressed
		for _, data := range [][]byte{compressible, incompressible} {
			wire, err := h.PacketToWire(Packet{MessagePort: 7, Data: data}, encrypted, false)
			require.NoError(t, err)
			msg, err := h.PacketFromWire(wire)
			require.NoError(t, err)
			require.Equal(t, data, msg.Data)
			wireDataLen := len(data)
			if compressed := snappy.Encode(nil, data); len(compressed) < len(data) {
				wireDataLen = len(compressed)
			}
			require.Equal(t, wireDataLen, msg.WireDataLen, "encrypted %v, %d bytes", encrypted, len(data))
		}
	}
}

// wireRequest returns a request as notehub decodes it from the wire
func wireRequest(t *testing.T, messageType string, notefileID string, noteID string, body map[string]interface{}, payload []byte) notehubMessage {
	msg := notehubMessage{MessageType: messageType, NotefileID: notefileID, NoteID: noteID}
	require.NoError(t, msg.SetBody(body))
	msg.SetPayload(payload)
	wire, _, err := msgToWire(msg)
	require.NoError(t, err)
	req, _, err := msgFromWire(context.Background(), wire)
	require.NoError(t, err)
	req.DeviceUID = "dev:ota"
	req.DeviceEndpointID = note.DefaultDeviceEndpointID
	return req
}

func TestWireLenHubTransactions(t *testing.T) {
	ctx := context.Background()
	FileSetStorageLocation(t.TempDir())
	// Open noteboxes are cached by storage object, so each run of the test gets its own
	storage := FileStorageObject("test_wirelen_hub_transactions_" + NewUUID())
	savedDiscover := fnDiscover
	fnDiscover = func(edgeUID string, deviceSN string, productUID string, appUID string, needHandlerInfo bool, hostname string, packetHandlerVersion string) (DiscoverInfo, error) {
		return DiscoverInfo{HubEndpointID: note.DefaultHubEndpointID, HubDeviceStorageObject: storage}, nil
	}
	t.Cleanup(func() { fnDiscover = savedDiscover })

	var events []note.Event
	eventFn := func(ctx context.Context, sess *HubSession, local bool, event *note.Event) error {
		events = append(events, *event)
		return nil
	}
	session := &HubSession{}
	payload := []byte{1, 2, 3, 4, 5}

	// Note add: the body as it was sent on the wire, plus the payload
	req := wireRequest(t, msgNotefileAddNote, "data.qo", "", map[string]interface{}{"temp": 12.5, "label": "hello"}, payload)
	require.NoError(t, hubNotefileAddNote(ctx, session, req, &notehubMessage{}, eventFn))
	require.Len(t, events, 1)
	require.Equal(t, uint32(req.GetBodyWireLen()+len(payload)), events[0].WireLen)

	// Note add with a template-encoded payload: only the payload is counted
	bulkBody := BulkBody{NoteFormat: BulkNoteFormatFlexNano, NoteTemplate: `{"temp":12.1,"count":14}`}
	bulkBodyJSON, err := note.JSONMarshal(bulkBody)
	require.NoError(t, err)
	records := []byte{}
	for i := 0; i < 3; i++ {
		encodeCtx, err := BulkEncodeTemplate(bulkBodyJSON)
		require.NoError(t, err)
		record, err := encodeCtx.BulkEncodeNextEntry(ctx, map[string]interface{}{"temp": 9.5, "count": i}, nil, 0, 0, "", "", false)
		require.NoError(t, err)
		records = append(records, record...)
	}
	var templateBody map[string]interface{}
	require.NoError(t, note.JSONUnmarshal(bulkBodyJSON, &templateBody))
	req = wireRequest(t, msgNotefileAddNote, "data.qo", "", templateBody, records)
	req.PowerSource = 0x00000001
	require.NoError(t, hubNotefileAddNote(ctx, session, req, &notehubMessage{}, eventFn))
	require.Len(t, events, 2)
	require.Contains(t, *events[1].Body, "decoded")
	require.Equal(t, uint32(len(records)), events[1].WireLen)

	// Note update: the same rule as note add
	req = wireRequest(t, msgNotefileAddNote, "state.db", "n1", map[string]interface{}{"on": true}, nil)
	require.NoError(t, hubNotefileAddNote(ctx, session, req, &notehubMessage{}, eventFn))
	req = wireRequest(t, msgNotefileUpdateNote, "state.db", "n1", map[string]interface{}{"on": false, "level": 7}, payload)
	require.NoError(t, hubNotefileUpdateNote(ctx, session, req, &notehubMessage{}, eventFn))
	require.Len(t, events, 4)
	require.Equal(t, note.EventUpdate, events[3].Req)
	require.Equal(t, uint32(req.GetBodyWireLen()+len(payload)), events[3].WireLen)
}

func TestWireLenWebRequest(t *testing.T) {
	ctx := context.Background()
	var got uint32
	savedWebRequest := fnHubWebRequest
	fnHubWebRequest = func(ctx context.Context, sess *HubSession, deviceUID string, productUID string, alias string, reqtype string, reqcontent string, reqoffset int, reqTotalPayloadLen int, reqRspMaxBytes int, target string, reqLabel string, reqwhen int64, bodyJSON []byte, payload []byte) (int, map[string][]string, []byte, []byte, error) {
		got = WireLenFromContext(ctx)
		return 200, nil, []byte("{}"), nil, nil
	}
	t.Cleanup(func() { fnHubWebRequest = savedWebRequest })

	webBody := map[string]interface{}{"temp": 12.5, "label": "hello"}
	webRequest := func(payload []byte, compressed bool, totalPayloadLen uint32) notehubMessage {
		notefile := CreateNotefile(false)
		_, err := notefile.AddNote(ctx, nil, note.DefaultDeviceEndpointID, "web", note.Note{Body: webBody})
		require.NoError(t, err)
		req := notehubMessage{MessageType: msgWebRequest, NoteID: "alias", NotefileID: "POST", SessionIDMismatch: compressed, HighPowerSecsTotal: totalPayloadLen}
		require.NoError(t, req.SetNotefile(notefile))
		req.SetPayload(payload)
		return req
	}

	// The body plus the payload as it was sent
	payload := []byte{1, 2, 3, 4, 5, 6, 7, 8}
	require.NoError(t, hubWebRequest(ctx, &HubSession{}, webRequest(payload, false, 0), &notehubMessage{}, nil))
	require.Equal(t, jcWireLen(webBody, payload), got)

	// A payload compressed on its own is counted as sent
	compressed := snappy.Encode(nil, make([]byte, 200))
	require.NoError(t, hubWebRequest(ctx, &HubSession{}, webRequest(compressed, true, 0), &notehubMessage{}, nil))
	require.Equal(t, jcWireLen(webBody, compressed), got)

	// A segmented upload counts its total payload
	require.NoError(t, hubWebRequest(ctx, &HubSession{}, webRequest(payload, false, 1000), &notehubMessage{}, nil))
	require.Equal(t, jcWireLen(webBody, nil)+1000, got)
}

// deviceChanges returns the changes to a device's notefile that it hasn't yet sent to notehub,
// as the Notecard sends them, and marks them as sent
func deviceChanges(t *testing.T, notefile *Notefile) *Notefile {
	chgfile, _, _, _, since, until, err := notefile.GetChanges(note.DefaultHubEndpointID, true, 0)
	require.NoError(t, err)
	require.NoError(t, notefile.UpdateChangeTracker(note.DefaultHubEndpointID, since, until))
	return chgfile
}

// externalizePayloads moves the payloads of the specified notes into the binary section of a
// message, appending them to xp as the Notecard does, and returns that section
func externalizePayloads(t *testing.T, xp []byte, notefile *Notefile, noteIDs ...string) []byte {
	for _, noteID := range noteIDs {
		xnote, present := notefile.Notes[noteID]
		require.True(t, present, noteID)
		require.NotEmpty(t, xnote.Payload, noteID)
		xnote.XPOff = uint32(len(xp))
		xnote.XPLen = uint32(len(xnote.Payload))
		xp = append(xp, xnote.Payload...)
		xnote.Payload = nil
		notefile.Notes[noteID] = xnote
	}
	return xp
}

func TestWireLenHubMergeTransactions(t *testing.T) {
	ctx := context.Background()
	FileSetStorageLocation(t.TempDir())
	// Open noteboxes are cached by storage object, so each run of the test gets its own
	storage := FileStorageObject("test_wirelen_hub_merge_" + NewUUID())
	savedDiscover := fnDiscover
	fnDiscover = func(edgeUID string, deviceSN string, productUID string, appUID string, needHandlerInfo bool, hostname string, packetHandlerVersion string) (DiscoverInfo, error) {
		return DiscoverInfo{HubEndpointID: note.DefaultHubEndpointID, HubDeviceStorageObject: storage}, nil
	}
	t.Cleanup(func() { fnDiscover = savedDiscover })

	// The notefiles that the device's notebox would already have synchronized
	box, err := OpenEndpointNotebox(ctx, note.DefaultHubEndpointID, storage, true)
	require.NoError(t, err)
	require.NoError(t, box.AddNotefile(ctx, "state.db", nil))
	require.NoError(t, box.AddNotefile(ctx, "data.qo", nil))
	require.NoError(t, box.Close(ctx))

	var events []note.Event
	eventFn := func(ctx context.Context, sess *HubSession, local bool, event *note.Event) error {
		events = append(events, *event)
		return nil
	}
	session := &HubSession{Active: true, HubSessionTicket: "ticket", DeviceEndpointID: note.DefaultDeviceEndpointID}
	session.Session.DeviceUID = "dev:wirelen"

	// send transmits a request on the wire as the Notecard does, and returns the events that notehub
	// emitted while processing it.  Queue notes' events carry no NoteID, so they are keyed by label.
	send := func(req notehubMessage) map[string]note.Event {
		events = nil
		wire, _, err := msgToWire(req)
		require.NoError(t, err)
		_, result, _, err := HubRequest(ctx, wire, eventFn, session)
		require.NoError(t, err)
		rsp, _, err := msgFromWire(ctx, result)
		require.NoError(t, err)
		require.Empty(t, rsp.Error)
		byNote := map[string]note.Event{}
		for _, event := range events {
			noteID := event.NoteID
			if noteID == "" {
				require.NotNil(t, event.Body)
				noteID, _ = (*event.Body)["label"].(string)
			}
			byNote[event.NotefileID+"/"+noteID] = event
		}
		require.Len(t, byNote, len(events))
		return byNote
	}

	// Notefile merge: a note whose payload is in the binary section of the message, and one without
	devState := CreateNotefile(false)
	n1Body := map[string]interface{}{"label": "n1", "temp": 12.5, "ok": true}
	n1Payload := []byte{1, 2, 3, 4, 5, 6}
	n2Body := map[string]interface{}{"label": "n2", "level": 7}
	_, err = devState.AddNote(ctx, nil, note.DefaultDeviceEndpointID, "n1", note.Note{Body: n1Body, Payload: n1Payload})
	require.NoError(t, err)
	_, err = devState.AddNote(ctx, nil, note.DefaultDeviceEndpointID, "n2", note.Note{Body: n2Body})
	require.NoError(t, err)
	chgState := deviceChanges(t, devState)
	xp := externalizePayloads(t, nil, chgState, "n1")
	req := notehubMessage{MessageType: msgNotefileMerge, NotefileID: "state.db"}
	require.NoError(t, req.SetNotefile(chgState))
	req.SetPayload(xp)
	got := send(req)
	require.Len(t, got, 2)
	require.Equal(t, n1Payload, got["state.db/n1"].Payload)
	require.Equal(t, jcWireLen(n1Body, n1Payload), got["state.db/n1"].WireLen)
	require.Equal(t, jcWireLen(n2Body, nil), got["state.db/n2"].WireLen)

	// Multi-notefile merge: queue notes sent with their payloads in the binary section and inline,
	// a new note whose payload follows the queue's in the binary section, and a deletion
	devQueue := CreateNotefile(true)
	q1Body := map[string]interface{}{"label": "q1", "temp": 9.5}
	q1Payload := []byte{10, 11, 12, 13}
	q2Body := map[string]interface{}{"label": "q2", "msg": "hello"}
	q2Payload := []byte{20, 21, 22}
	q1, err := devQueue.AddNote(ctx, nil, note.DefaultDeviceEndpointID, "", note.Note{Body: q1Body, Payload: q1Payload})
	require.NoError(t, err)
	_, err = devQueue.AddNote(ctx, nil, note.DefaultDeviceEndpointID, "", note.Note{Body: q2Body, Payload: q2Payload})
	require.NoError(t, err)
	n3Body := map[string]interface{}{"label": "n3", "on": false}
	n3Payload := []byte{30, 31, 32, 33, 34}
	_, err = devState.AddNote(ctx, nil, note.DefaultDeviceEndpointID, "n3", note.Note{Body: n3Body, Payload: n3Payload})
	require.NoError(t, err)
	require.NoError(t, devState.DeleteNote(ctx, nil, note.DefaultDeviceEndpointID, "n1"))
	chgQueue := deviceChanges(t, devQueue)
	chgState = deviceChanges(t, devState)
	require.Len(t, chgQueue.Notes, 2)
	require.Len(t, chgState.Notes, 2)
	require.True(t, chgState.Notes["n1"].Deleted)
	xp = externalizePayloads(t, nil, chgQueue, q1)
	xp = externalizePayloads(t, xp, chgState, "n3")
	require.Equal(t, uint32(len(q1Payload)), chgState.Notes["n3"].XPOff)
	req = notehubMessage{MessageType: msgNotefilesMerge}
	require.NoError(t, req.SetNotefiles(map[string]*Notefile{"data.qo": chgQueue, "state.db": chgState}))
	req.SetPayload(xp)
	got = send(req)
	require.Len(t, got, 4)
	require.Equal(t, q1Payload, got["data.qo/q1"].Payload)
	require.Equal(t, jcWireLen(q1Body, q1Payload), got["data.qo/q1"].WireLen)
	require.Equal(t, jcWireLen(q2Body, q2Payload), got["data.qo/q2"].WireLen)
	require.Equal(t, n3Payload, got["state.db/n3"].Payload)
	require.Equal(t, jcWireLen(n3Body, n3Payload), got["state.db/n3"].WireLen)

	// A deletion isn't measured
	require.Equal(t, note.EventDelete, got["state.db/n1"].Req)
	require.Zero(t, got["state.db/n1"].WireLen)
}
