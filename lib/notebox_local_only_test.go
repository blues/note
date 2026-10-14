package notelib

import (
	"context"
	"testing"

	"github.com/blues/note-go/note"
	"github.com/blues/note-go/notecard"
	"github.com/stretchr/testify/require"
)

func TestLocalOnlyNotefileStaysServerLocal(t *testing.T) {
	ctx := context.Background()
	FileSetStorageLocation(t.TempDir())

	box, err := OpenEndpointNotebox(ctx, note.DefaultHubEndpointID, FileStorageObject("local_only_hub"), true)
	require.NoError(t, err)
	defer box.Close(ctx)

	const syncedFile = "visible.db"
	const localFile = "server-state.dbx"
	require.NoError(t, box.AddNotefile(ctx, syncedFile, nil))
	require.NoError(t, box.AddNotefile(ctx, localFile, nil))

	// The parser is the only place that knows the "x" syntax. All sync code
	// consumes the two direction flags through this derived predicate.
	_, syncToHub, syncFromHub, _, _, err := NotefileAttributesFromID(localFile)
	require.NoError(t, err)
	require.False(t, syncToHub)
	require.False(t, syncFromHub)
	require.True(t, NotefileIsLocalOnly(localFile))
	require.False(t, NotefileIsLocalOnly(syncedFile))

	// Server-side note APIs retain normal read/write access to .dbx files.
	body := map[string]interface{}{"state": "server-only"}
	rsp := box.Request(ctx, nil, box.EndpointID(), notecard.Request{
		Req:        notecard.ReqNoteAdd,
		NotefileID: localFile,
		NoteID:     "private",
		Body:       &body,
	})
	require.Empty(t, rsp.Err)
	rsp = box.Request(ctx, nil, box.EndpointID(), notecard.Request{
		Req:        notecard.ReqNoteGet,
		NotefileID: localFile,
		NoteID:     "private",
	})
	require.Empty(t, rsp.Err)
	require.Equal(t, "server-only", (*rsp.Body)["state"])

	visibleBody := map[string]interface{}{"state": "syncable"}
	rsp = box.Request(ctx, nil, box.EndpointID(), notecard.Request{
		Req:        notecard.ReqNoteAdd,
		NotefileID: syncedFile,
		NoteID:     "visible",
		Body:       &visibleBody,
	})
	require.Empty(t, rsp.Err)

	// Changed-file discovery must never expose the server-local file.
	changed := box.GetChangedNotefiles(ctx, note.DefaultDeviceEndpointID)
	require.Contains(t, changed, syncedFile)
	require.NotContains(t, changed, localFile)
	session := HubSession{}
	sawNotefile(&session, box, "")
	require.Contains(t, session.Notefiles, syncedFile)
	require.NotContains(t, session.Notefiles, localFile)

	// The notebox catalog itself synchronizes, so both the global descriptor
	// and endpoint-instance descriptor for .dbx must be filtered from it.
	catalogChanges, numChanges, totalChanges, _, since, until, err := box.GetChanges(note.DefaultDeviceEndpointID, defaultMaxGetNoteboxChangesBatchSize)
	require.NoError(t, err)
	require.Positive(t, numChanges)
	require.Equal(t, numChanges, totalChanges)
	for _, noteID := range catalogChanges.NoteIDs(true) {
		_, _, notefileID := parseCompositeNoteID(noteID)
		require.NotEqual(t, localFile, notefileID)
	}
	require.NoError(t, box.UpdateChangeTracker(note.DefaultDeviceEndpointID, since, until))

	// A later .dbx-only catalog change is hidden while the catalog tracker still
	// advances past it, preventing an empty-sync loop.
	require.NoError(t, box.AddNotefile(ctx, "later.dbx", nil))
	catalogChanges, numChanges, totalChanges, _, since, until, err = box.GetChanges(note.DefaultDeviceEndpointID, defaultMaxGetNoteboxChangesBatchSize)
	require.NoError(t, err)
	require.Zero(t, numChanges)
	require.Zero(t, totalChanges)
	require.Empty(t, catalogChanges.NoteIDs(true))
	require.Greater(t, until, since)
	require.NoError(t, box.UpdateChangeTracker(note.DefaultDeviceEndpointID, since, until))

	// High-level file enumeration hides .dbx from a caller acting as the device,
	// but shows it to an API caller acting as the hub, because the API is the only
	// way these files can be managed.
	rsp = box.Request(ctx, nil, note.DefaultDeviceEndpointID, notecard.Request{Req: notecard.ReqFileChanges})
	require.Empty(t, rsp.Err)
	require.NotNil(t, rsp.FileInfo)
	require.Contains(t, *rsp.FileInfo, syncedFile)
	require.NotContains(t, *rsp.FileInfo, localFile)
	rsp = box.Request(ctx, nil, box.EndpointID(), notecard.Request{Req: notecard.ReqFileChanges})
	require.Empty(t, rsp.Err)
	require.NotNil(t, rsp.FileInfo)
	require.Contains(t, *rsp.FileInfo, syncedFile)
	require.Contains(t, *rsp.FileInfo, localFile)

	// An explicitly targeted server-side note.changes request remains usable.
	rsp = box.Request(ctx, nil, box.EndpointID(), notecard.Request{
		Req:        notecard.ReqNoteChanges,
		NotefileID: localFile,
	})
	require.Empty(t, rsp.Err)
	require.NotNil(t, rsp.Notes)
	require.Contains(t, *rsp.Notes, "private")

	// A notefile that does not synchronize from the hub can never have a note
	// waiting for the device, so no note in it may be reported as pending.  The
	// device has no tracker on such a file, which is exactly the condition under
	// which ChangePendingForEndpoint would otherwise report everything pending.
	rsp = box.Request(ctx, nil, box.EndpointID(), notecard.Request{
		Req:        notecard.ReqNotesGetL,
		NotefileID: localFile,
		Pending:    true,
	})
	require.Empty(t, rsp.Err)
	require.NotNil(t, rsp.Notes)
	require.False(t, (*rsp.Notes)["private"].Pending)
	rsp = box.Request(ctx, nil, box.EndpointID(), notecard.Request{
		Req:        notecard.ReqNotesGetL,
		NotefileID: syncedFile,
		Pending:    true,
	})
	require.Empty(t, rsp.Err)
	require.NotNil(t, rsp.Notes)
	require.True(t, (*rsp.Notes)["visible"].Pending)
}

func TestMergeNoteboxIgnoresLocalOnlyDescriptors(t *testing.T) {
	ctx := context.Background()
	FileSetStorageLocation(t.TempDir())

	hubBox, err := OpenEndpointNotebox(ctx, note.DefaultHubEndpointID, FileStorageObject("local_only_merge_hub"), true)
	require.NoError(t, err)
	defer hubBox.Close(ctx)

	deviceBox, err := OpenEndpointNotebox(ctx, note.DefaultDeviceEndpointID, FileStorageObject("local_only_merge_device"), true)
	require.NoError(t, err)
	defer deviceBox.Close(ctx)

	const localFile = "device-state.dbx"
	require.NoError(t, deviceBox.AddNotefile(ctx, localFile, nil))

	// Use the raw notebox notefile to simulate an older peer that still sends
	// .dbx descriptors. MergeNotebox must discard them defensively.
	incoming, _, totalChanges, _, _, _, err := deviceBox.Notefile().GetChanges(note.DefaultHubEndpointID, true, defaultMaxGetNoteboxChangesBatchSize)
	require.NoError(t, err)
	require.Positive(t, totalChanges)
	require.NoError(t, hubBox.MergeNotebox(ctx, incoming))
	require.False(t, hubBox.NotefileExists(localFile))
}

func TestNoteboxNotefileDescByPortIgnoresLocalOnlyNotefiles(t *testing.T) {
	descriptors := []note.NotefileDesc{
		{NotefileID: "server-state.dbx", TemplatePort: 10},
		{NotefileID: "visible.db", TemplatePort: 10},
	}

	info, present := NoteboxNotefileDescByPort(descriptors, 10)
	require.True(t, present)
	require.Equal(t, "visible.db", info.NotefileID)

	_, present = NoteboxNotefileDescByPort(descriptors[:1], 10)
	require.False(t, present)
}

func TestHubWireNotefileOperationsIgnoreLocalOnlyNotefiles(t *testing.T) {
	type handler func(context.Context, *HubSession, notehubMessage, *notehubMessage, EventFunc) error
	handlers := map[string]handler{
		"changes":        hubNotefileChanges,
		"merge":          hubNotefileMerge,
		"update tracker": hubNotefileUpdateChangeTracker,
		"live add":       hubNotefileAddNote,
		"live update":    hubNotefileUpdateNote,
		"live delete":    hubNotefileDeleteNote,
	}

	for name, handle := range handlers {
		t.Run(name, func(t *testing.T) {
			rsp := notehubMessage{}
			err := handle(context.Background(), &HubSession{}, notehubMessage{NotefileID: "server-state.dbx"}, &rsp, nil)
			require.NoError(t, err)
			require.Equal(t, notehubMessage{}, rsp)
		})
	}
}
