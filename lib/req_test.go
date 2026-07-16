// Copyright 2026 Blues Inc.  All rights reserved.
// Use of this source code is governed by licenses granted by the
// copyright holder including that found in the LICENSE file.

package notelib

import (
	"context"
	"testing"

	"github.com/blues/note-go/note"
	"github.com/blues/note-go/notecard"
	"github.com/stretchr/testify/require"
)

// TestVarShimRoundTrip exercises the var.set / var.get / var.delete shim that
// (*Notebox).Request layers over the underlying note.update / note.get /
// note.delete handlers.  It verifies that:
//
//   - Each scalar (text / value / flag) round-trips losslessly.
//   - The new values array round-trips losslessly.
//   - Mixed bodies (text + value + flag in the same var) round-trip.
//   - Zero scalar values are stripped from the stored body, matching the
//     notecard-side dbVarBody marshaler's skip-zero-values semantics.
//   - NotefileID defaults to vars.db when the request omits it.
//   - Explicit NotefileID (e.g. _fence.dbs) is honored end-to-end.
//   - var.delete removes the underlying note.
//
// Together these assertions lock in the behavioral contract that the var.*
// shim on this side mirrors the reqnote.c shim on the notecard side, so that
// var.* requests round-trip through Notehub on the way to / from a connected
// device without lossy schema rewrites.
func TestVarShimRoundTrip(t *testing.T) {
	ctx := context.Background()

	tmpDir := t.TempDir()
	FileSetStorageLocation(tmpDir)

	storage := FileStorageObject("test_var_shim")
	box, err := OpenEndpointNotebox(ctx, note.DefaultDeviceEndpointID, storage, true)
	require.NoError(t, err)

	ei := &NotefileEventInfo{}
	send := func(req notecard.Request) notecard.Request {
		return box.Request(ctx, ei, note.DefaultDeviceEndpointID, req)
	}

	// var.set name="numvar" value=42 -> body {value:42}
	rsp := send(notecard.Request{
		Req:   notecard.ReqVarSet,
		Name:  "numvar",
		Value: 42,
	})
	require.Empty(t, rsp.Err)

	// var.get name="numvar" -> rsp.Value=42, no other fields
	rsp = send(notecard.Request{
		Req:  notecard.ReqVarGet,
		Name: "numvar",
	})
	require.Empty(t, rsp.Err)
	require.Equal(t, float64(42), rsp.Value)
	require.Equal(t, "", rsp.Text)
	require.False(t, rsp.Flag)
	require.Nil(t, rsp.Values)
	// var.get's epilogue must clear the note-shaped scaffolding so the
	// response looks like a var, not a note.
	require.Empty(t, rsp.NoteID)
	require.Nil(t, rsp.Body)
	require.Zero(t, rsp.Time)
	require.False(t, rsp.Deleted)

	// var.set name="strvar" text="hello"
	rsp = send(notecard.Request{
		Req:  notecard.ReqVarSet,
		Name: "strvar",
		Text: "hello",
	})
	require.Empty(t, rsp.Err)

	rsp = send(notecard.Request{
		Req:  notecard.ReqVarGet,
		Name: "strvar",
	})
	require.Empty(t, rsp.Err)
	require.Equal(t, "hello", rsp.Text)
	require.Equal(t, float64(0), rsp.Value)

	// var.set name="boolvar" flag=true
	rsp = send(notecard.Request{
		Req:  notecard.ReqVarSet,
		Name: "boolvar",
		Flag: true,
	})
	require.Empty(t, rsp.Err)

	rsp = send(notecard.Request{
		Req:  notecard.ReqVarGet,
		Name: "boolvar",
	})
	require.Empty(t, rsp.Err)
	require.True(t, rsp.Flag)

	// var.set name="arrvar" values=[1.5, 2.5, 3.5]
	arr := []float64{1.5, 2.5, 3.5}
	rsp = send(notecard.Request{
		Req:    notecard.ReqVarSet,
		Name:   "arrvar",
		Values: &arr,
	})
	require.Empty(t, rsp.Err)

	rsp = send(notecard.Request{
		Req:  notecard.ReqVarGet,
		Name: "arrvar",
	})
	require.Empty(t, rsp.Err)
	require.NotNil(t, rsp.Values)
	require.Equal(t, []float64{1.5, 2.5, 3.5}, *rsp.Values)

	// Mixed scalars: text + value + flag in one var must all round-trip.
	rsp = send(notecard.Request{
		Req:   notecard.ReqVarSet,
		Name:  "mixed",
		Text:  "combo",
		Value: 99,
		Flag:  true,
	})
	require.Empty(t, rsp.Err)

	rsp = send(notecard.Request{
		Req:  notecard.ReqVarGet,
		Name: "mixed",
	})
	require.Empty(t, rsp.Err)
	require.Equal(t, "combo", rsp.Text)
	require.Equal(t, float64(99), rsp.Value)
	require.True(t, rsp.Flag)

	// Skip-zero-values: var.set with Value=0 stores nothing for value.
	// var.get returns the default-zero, indistinguishable from "never set".
	rsp = send(notecard.Request{
		Req:   notecard.ReqVarSet,
		Name:  "zero",
		Value: 0,
	})
	require.Empty(t, rsp.Err)

	rsp = send(notecard.Request{
		Req:  notecard.ReqVarGet,
		Name: "zero",
	})
	// note.get on a vars.db entry whose body is {} yields no error and
	// no fields populated.  The exact response shape (err vs. empty body)
	// depends on whether the underlying note exists at all -- when var.set
	// is called with only zero/empty fields, the note is still added with
	// an empty body, so the get succeeds and just returns zeros.
	require.Empty(t, rsp.Err)
	require.Equal(t, float64(0), rsp.Value)
	require.Equal(t, "", rsp.Text)
	require.False(t, rsp.Flag)
	require.Nil(t, rsp.Values)

	// NotefileID defaults to vars.db when the request omits it -- verify
	// by reading the note back via an explicit note.get against vars.db.
	rsp = send(notecard.Request{
		Req:   notecard.ReqVarSet,
		Name:  "defaultfile",
		Value: 7,
	})
	require.Empty(t, rsp.Err)

	rsp = send(notecard.Request{
		Req:        notecard.ReqNoteGet,
		NotefileID: note.HubDefaultVarNotefile,
		NoteID:     "defaultfile",
	})
	require.Empty(t, rsp.Err)
	require.NotNil(t, rsp.Body)
	require.Equal(t, float64(7), (*rsp.Body)["value"])

	// Explicit NotefileID is honored: write a fence-shaped note into
	// _fence.dbs and read it back via var.get with the same file override.
	fenceArr := []float64{42.36, -71.06, 42.37, -71.05, 42.35, -71.04}
	rsp = send(notecard.Request{
		Req:        notecard.ReqVarSet,
		Name:       "zoneA",
		NotefileID: note.FenceNotefile,
		Values:     &fenceArr,
	})
	require.Empty(t, rsp.Err)

	rsp = send(notecard.Request{
		Req:        notecard.ReqVarGet,
		Name:       "zoneA",
		NotefileID: note.FenceNotefile,
	})
	require.Empty(t, rsp.Err)
	require.NotNil(t, rsp.Values)
	require.Equal(t, fenceArr, *rsp.Values)

	// var.delete removes the note.  Subsequent var.get must report an error.
	rsp = send(notecard.Request{
		Req:  notecard.ReqVarDelete,
		Name: "numvar",
	})
	require.Empty(t, rsp.Err)

	rsp = send(notecard.Request{
		Req:  notecard.ReqVarGet,
		Name: "numvar",
	})
	require.NotEmpty(t, rsp.Err, "var.get of a deleted var should error")
}

// TestNotefileIDIsReservedWithExceptionsCarveOuts asserts the carve-out list
// in NotefileIDIsReservedWithExceptions stays in sync with the set of system
// notefiles the front end / device-side code expect to be writable without
// the req.Allow override.  In particular, _fence.dbs must be carved out so
// the var.* shim and the front-end notefile editor can author geofences
// without callers having to pass allow:true on every request.
func TestNotefileIDIsReservedWithExceptionsCarveOuts(t *testing.T) {
	for _, name := range []string{
		note.TrackNotefile,
		note.LogNotefile,
		note.HealthNotefile,
		note.HealthHostNotefile,
		note.NotecardRequestNotefile,
		note.NotecardResponseNotefile,
		note.TempNotefile,
		note.FenceNotefile,
	} {
		require.Falsef(t, NotefileIDIsReservedWithExceptions(name),
			"%s should be carved out of the reserved-name rejection", name)
	}

	// Sanity: arbitrary _-prefixed notefiles that aren't carved out are
	// still rejected -- the carve-out is a finite allowlist, not a blanket
	// "underscore is fine".
	for _, name := range []string{
		"_internal.dbs",
		"_state.qo",
		"_anything.qis",
	} {
		require.Truef(t, NotefileIDIsReservedWithExceptions(name),
			"%s should still be considered reserved", name)
	}

	// Sanity: arbitrary user notefiles (no leading underscore) are not
	// reserved at all.
	for _, name := range []string{
		"data.qo",
		"sensors.qo",
		"vars.db",
	} {
		require.Falsef(t, NotefileIDIsReservedWithExceptions(name),
			"%s is a user-data notefile and must not be considered reserved", name)
	}
}
