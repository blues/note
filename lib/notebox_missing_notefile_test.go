package notelib

import (
	"context"
	"testing"

	"github.com/blues/note-go/note"
	"github.com/blues/note-go/notecard"
	"github.com/stretchr/testify/require"
)

// A notebox can list a notefile that has no instance for this endpoint, and so can't be
// opened. file.changes must skip it rather than fail the whole listing.
func TestFileChangesSkipsMissingNotefile(t *testing.T) {
	ctx := context.Background()
	FileSetStorageLocation(t.TempDir())

	box, err := OpenEndpointNotebox(ctx, note.DefaultHubEndpointID, FileStorageObject("missing_notefile_hub"), true)
	require.NoError(t, err)
	defer box.Close(ctx)

	const goodFile = "data.qi"
	const missingFile = "EL10653"
	require.NoError(t, box.AddNotefile(ctx, goodFile, nil))

	// List the missing notefile with a descriptor but no instance.
	boxfile := box.Notefile()
	boxfile.lock.Lock()
	boxfile.Notes[missingFile] = boxfile.Notes[goodFile]
	boxfile.lock.Unlock()
	require.Contains(t, box.Notefiles(false), missingFile)

	rsp := box.Request(ctx, nil, box.EndpointID(), notecard.Request{Req: notecard.ReqFileChanges})
	require.Empty(t, rsp.Err)
	require.NotNil(t, rsp.FileInfo)
	require.Contains(t, *rsp.FileInfo, goodFile)
	require.NotContains(t, *rsp.FileInfo, missingFile)
}
