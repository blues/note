// Copyright 2017 Blues Inc.  All rights reserved.
// Use of this source code is governed by licenses granted by the
// copyright holder including that found in the LICENSE file.

package notelib

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/blues/note-go/note"
	"github.com/blues/note-go/notecard"
	golc "github.com/google/open-location-code/go"
)

// Request performs a local operation using the JSON API
func (box *Notebox) Request(ctx context.Context, ei *NotefileEventInfo, endpointID string, req notecard.Request) (rsp notecard.Request) {
	var err error

	// Debug
	if debugRequest {
		reqJSON, _ := note.JSONMarshal(req)
		logDebug(ctx, ">> %s", string(reqJSON))
	}

	// Extract the request ID, which will be used to correlate requests with responses
	rsp.RequestID = req.RequestID

	// Handle legacy which used "files." instead of "file.".  This was changed 2019-11-18 and
	// can be removed after we ship because the compat was just until we distributed new notecard fw.
	req.Req = strings.Replace(req.Req, "files.", "file.", -1)

	// Pre-process var.set/var.get/var.delete as syntactic sugar over note.update/note.get/
	// note.delete on the vars.db notefile, mirroring the notecard-side shim in reqnote.c.
	// Each rewrites NotefileID (defaulting to vars.db), moves Name -> NoteID, and re-routes
	// req.Req to the underlying note.* request.  var.set additionally packs the scalar
	// (value/flag/text) and array (values) fields into a body map, preserving the same
	// "skip zero values" semantics the notecard marshaler enforces -- a var.set carrying
	// only text+value produces a body with only those keys, byte-equivalent to what a
	// connected device would have stored.  var.get sets a flag so the post-switch epilogue
	// can hoist body fields back to top-level rsp fields.
	isVarGet := false
	switch req.Req {
	case notecard.ReqVarSet:
		if req.NotefileID == "" {
			req.NotefileID = note.HubDefaultVarNotefile
		}
		req.NoteID = req.Name
		req.Name = ""
		body := map[string]interface{}{}
		if req.Text != "" {
			body["text"] = req.Text
			req.Text = ""
		}
		if req.Values != nil && len(*req.Values) > 0 {
			body["values"] = *req.Values
			req.Values = nil
		}
		if req.Flag {
			body["flag"] = true
			req.Flag = false
		}
		if req.Value != 0.0 {
			body["value"] = req.Value
			req.Value = 0.0
		}
		req.Body = &body
		req.Req = notecard.ReqNoteUpdate
	case notecard.ReqVarGet:
		if req.NotefileID == "" {
			req.NotefileID = note.HubDefaultVarNotefile
		}
		req.NoteID = req.Name
		req.Name = ""
		req.Req = notecard.ReqNoteGet
		isVarGet = true
	case notecard.ReqVarDelete:
		if req.NotefileID == "" {
			req.NotefileID = note.HubDefaultVarNotefile
		}
		req.NoteID = req.Name
		req.Name = ""
		req.Req = notecard.ReqNoteDelete
	}

	// Dispatch based on request type
	switch req.Req {

	default:
		rsp.Err = fmt.Sprintf("unknown request type: %s", req.Req)

	case notecard.ReqFileSet:
		fallthrough
	case notecard.ReqFileAdd:
		if req.FileInfo == nil || len(*req.FileInfo) == 0 {
			rsp.Err = "no notefiles were specified"
			break
		}

		for notefileID, notefileInfo := range *req.FileInfo {
			if !NotefileIDIsReservedWithExceptions(notefileID) || req.Allow {
				err = box.AddNotefile(ctx, notefileID, &notefileInfo)
				if err != nil && rsp.Err == "" {
					rsp.Err = fmt.Sprintf("error adding notefile: %s", err)
					break
				}
			}
		}

	case notecard.ReqFileDelete:
		if req.Files == nil || len(*req.Files) == 0 {
			rsp.Err = "no notefiles were specified"
			break
		}
		deleteFiles := *req.Files
		for i := range deleteFiles {
			if !NotefileIDIsReservedWithExceptions(deleteFiles[i]) || req.Allow {
				err = box.DeleteNotefile(ctx, deleteFiles[i])
				if err != nil && rsp.Err == "" {
					rsp.Err = fmt.Sprintf("error deleting %s: %s", deleteFiles[i], err)
					break
				}
			}
		}

	case notecard.ReqNoteAdd:
		if req.NotefileID == "" {
			req.NotefileID = note.HubDefaultInboundNotefile
		}
		// Make sure that the file exists
		if NotefileIDIsReservedWithExceptions(req.NotefileID) && !req.Allow {
			rsp.Err = "reserved notefile name"
			break
		}

		exists := box.NotefileExists(req.NotefileID)
		if !exists {
			err = box.AddNotefile(ctx, req.NotefileID, nil)
			if err != nil {
				rsp.Err = fmt.Sprintf("%s", err)
				break
			}
		}

		// Check the noteID
		isQueue, toHub, fromHub, _, _, _ := NotefileAttributesFromID(req.NotefileID)
		if req.NoteID == "" {
			if !isQueue {
				rsp.Err = "note ID is required when using a database notefile"
				break
			}
		} else {
			if isQueue {
				rsp.Err = "note ID should not be specified when using a queue notefile"
				break
			}
		}
		// If this is an inbound queue, and if the user has not already specified an
		// explicit 'when' time to be assigned to the note, make sure that we assign
		// the new note a creation time that is either now or later than the creation
		// time assigned to any note still pending to be downloaded in the queue.
		// This is to ensure that a note.get on the notecard will receive the notes
		// in FIFO order, even at the cost of having a burst of notes being created
		// in the future.  It's a tradeoff, but because of the importance of FIFO
		// behavior, FIFO wins over time-correctness.
		if isQueue && fromHub && req.Time == 0 {
			openfile, file, err := box.OpenNotefile(ctx, req.NotefileID)
			if err == nil {
				_, mostRecentWhen, err := file.GetMostRecentNoteID()
				if err == nil && mostRecentWhen >= time.Now().UTC().Unix() {
					req.Time = mostRecentWhen + 1
				}
				openfile.Close(ctx)
			}
		}

		// Add the note
		xnote := note.Note{}
		annotateNoteWithWhereWhen(&xnote, req)
		if req.Payload != nil {
			xnote.Payload = *req.Payload
		}
		if req.Body != nil {
			xnote.Body = *req.Body
		}
		if !req.Allow {
			err = box.checkAddNoteLimits(ctx, req.NotefileID, xnote)
		}

		if err == nil {
			err = box.AddNoteWithHistory(ctx, ei, endpointID, req.NotefileID, req.NoteID, xnote)
		}
		if err != nil {
			rsp.Err = fmt.Sprintf("%s", err)
			break
		}
		// If this is an outbound queue, purge ALL tombstones from the
		// file that would normally have been purged during sync/merge
		if isQueue && toHub {
			openfile, file, err := box.OpenNotefile(ctx, req.NotefileID)
			if err == nil {
				file.PurgeTombstones("*")
				openfile.Close(ctx)
			}
		}

	case notecard.ReqNoteUpdate:
		if req.NotefileID == "" {
			rsp.Err = "no notefile specified"
			break
		}
		if NotefileIDIsReservedWithExceptions(req.NotefileID) && !req.Allow {
			rsp.Err = "reserved notefile name"
			break
		}
		if !box.NotefileExists(req.NotefileID) {
			err = box.AddNotefile(ctx, req.NotefileID, nil)
			if err != nil {
				rsp.Err = fmt.Sprintf("%s", err)
				break
			}
		}
		if req.NoteID == "" {
			rsp.Err = "no note ID specified"
			break
		}
		var xnote note.Note
		xnote, err = box.GetNote(ctx, req.NotefileID, req.NoteID)
		if err != nil {
			annotateNoteWithWhereWhen(&xnote, req)
			if req.Payload != nil {
				xnote.Payload = *req.Payload
			}
			if req.Body != nil {
				xnote.Body = *req.Body
			}
			err = nil
			if !req.Allow {
				err = box.checkAddNoteLimits(ctx, req.NotefileID, xnote)
			}
			if err == nil {
				err = box.AddNoteWithHistory(ctx, ei, endpointID, req.NotefileID, req.NoteID, xnote)
			}
			if err != nil {
				rsp.Err = fmt.Sprintf("error adding note: %s", err)
				break
			}
			break
		}
		annotateNoteWithWhereWhen(&xnote, req)
		if req.Payload != nil {
			xnote.Payload = *req.Payload
		} else {
			xnote.Payload = nil
		}
		if req.Body != nil {
			xnote.Body = *req.Body
		} else {
			xnote.Body = nil
		}
		err = box.UpdateNote(ctx, ei, endpointID, req.NotefileID, req.NoteID, xnote)
		if err != nil {
			rsp.Err = fmt.Sprintf("error updating note: %s", err)
			break
		}

	case notecard.ReqNoteDelete:
		if req.NotefileID == "" {
			rsp.Err = "no notefile specified"
			break
		}
		if NotefileIDIsReservedWithExceptions(req.NotefileID) && !req.Allow {
			rsp.Err = "reserved notefile name"
			break
		}
		if req.NoteID == "" {
			rsp.Err = "no note ID specified"
			break
		}
		err = box.DeleteNote(ctx, ei, endpointID, req.NotefileID, req.NoteID)
		if err != nil {
			rsp.Err = fmt.Sprintf("error deleting note: %s", err)
			break
		}

	case notecard.ReqNoteGet:
		if req.NotefileID == "" {
			rsp.Err = "no notefile specified"
			break
		}
		isQueue, _, syncFromHub, _, _, _ := NotefileAttributesFromID(req.NotefileID)
		if req.NoteID == "" {
			if !isQueue {
				rsp.Err = "no note ID specified"
				break
			}
		}
		openfile, file, err2 := box.OpenNotefile(ctx, req.NotefileID)
		if err2 != nil {
			rsp.Err = fmt.Sprintf("%s", err2)
			break
		}
		// Handle queues by getting the LRU note.  Note that we allow either inbound
		// or outbound queues to be accessed this way via API, explicitly to allow
		// the API to be used to manage queues in either direction.
		if isQueue && req.NoteID == "" {
			req.NoteID, err = file.GetLeastRecentNoteID()
			if err != nil {
				rsp.Err = fmt.Sprintf("%s", err)
				openfile.Close(ctx)
				break
			}
			openfile.Close(ctx)
		}
		// Get the note
		var xnote note.Note
		xnote, err = file.GetNote(req.NoteID)
		if err != nil {
			rsp.Err = fmt.Sprintf("error getting note: %s", err)
			openfile.Close(ctx)
			break
		}
		rsp.NoteID = req.NoteID
		if !isQueue && xnote.Deleted {
			if !req.Deleted {
				rsp.Err = fmt.Sprintf("note has been deleted: %s "+note.ErrNoteNoExist, req.NoteID)
				openfile.Close(ctx)
				break
			}
			rsp.Deleted = true
		}
		if xnote.Body != nil {
			rsp.Body = &xnote.Body
		}
		if len(xnote.Payload) != 0 {
			payload := xnote.GetPayload()
			rsp.Payload = &payload
		}
		rsp.Time = xnote.When()
		rsp.Latitude, rsp.Longitude, rsp.LocationTime = Where(xnote)
		if req.Edge {
			rsp.Pending = syncFromHub && file.ChangePendingForEndpoint(note.DefaultDeviceEndpointID, xnote)
			rsp.Edge = xnote.GetEndpointID() == note.DefaultDeviceEndpointID
		}
		if req.Delete {
			_ = box.DeleteNote(ctx, ei, endpointID, req.NotefileID, req.NoteID)
		}
		openfile.Close(ctx)

	case notecard.ReqFileGetL:
		fallthrough
	case notecard.ReqFileChanges:
		fallthrough
	case notecard.ReqFileChangesPending:
		var notefiles []string
		changes := 0
		total := 0
		showOnlyTotal := false

		// Special way of requesting to see if there is stuff pending to be uploaded
		if req.Req == notecard.ReqFileChangesPending {
			req.Pending = true
		}
		if req.Pending {
			req.TrackerID = "^"
		}

		// Special way, for use by support, to get a list of all notefiles
		// and their info such as templates.
		if req.Full {
			allNotefiles := box.NoteboxNotefileDesc()
			rsp.FileDesc = &allNotefiles
			break
		}

		// If no tracker, generate the entire list of notefiles
		tracker := req.TrackerID
		if tracker == "" {

			// Use the special reserved name, which (for internal use only) means "no tracker"
			showOnlyTotal = true
			tracker = ReservedIDDelimiter
			notefiles = box.Notefiles(false)
		} else {

			// Handle special case of "^" meaning things to be downloaded
			if req.TrackerID == "^" {
				req.TrackerID = note.DefaultDeviceEndpointID
			} else {
				// Make sure that it's a valid tracker in that it's not one of our known endpoints
				if req.TrackerID == note.DefaultDeviceEndpointID || req.TrackerID == note.DefaultHubEndpointID {
					rsp.Err = fmt.Sprintf("cannot use this reserved tracker name: %s", req.TrackerID)
					break
				}
			}

			// Get the changed notefiles for that tracker
			notefiles = box.GetChangedNotefiles(ctx, req.TrackerID)
		}

		// Prune the list of notefiles based on whether or not the user's perception is
		// that they should ever even exist on the notehub. The obvious things we're trying to skim off
		// are .qo and .qos.  A notefile that declares neither sync direction is pruned only when
		// we are acting on behalf of the device, which is the case for the LoRa/NTN packet paths
		// that reach here by looping back through the V0 API; an API caller such as noteboard must
		// be able to see these files because the API is the only way to manage them.
		actingAsDevice := endpointID == note.DefaultDeviceEndpointID
		newNotefiles := []string{}
		for _, file := range notefiles {
			isQueue, syncToHub, syncFromHub, _, _, _ := NotefileAttributesFromID(file)
			if isQueue && syncToHub {
				continue
			}
			if !syncToHub && !syncFromHub && actingAsDevice {
				continue
			}
			newNotefiles = append(newNotefiles, file)
		}
		notefiles = newNotefiles

		// Prune the list of notefiles based on the input
		if req.Files != nil && len(*req.Files) != 0 {
			newNotefiles := []string{}
			filterFiles := *req.Files
			for i := range filterFiles {
				for j := range notefiles {
					if notefiles[j] == filterFiles[i] {
						newNotefiles = append(newNotefiles, notefiles[j])
						break
					}
				}
			}
			notefiles = newNotefiles
		}

		// Generate the list of notefiles in the response data structure
		fileinfoArray := map[string]note.NotefileInfo{}
		for i := range notefiles {
			notefileID := notefiles[i]

			// If this is the notebox's notefile, omit it
			if notefileID == box.EndpointID() {
				continue
			}

			// If we're getting a list of all notefiles, skip the reserved notefiles
			// entirely.  Otherwise, if we have a tracker, allow the exceptions.
			if tracker == ReservedIDDelimiter {
				if NotefileIDIsReserved(notefileID) && !req.Allow {
					continue
				}
			} else {
				if NotefileIDIsReservedWithExceptions(notefileID) && !req.Allow {
					continue
				}
			}

			// Get the number of pending changes.  Note that if we didn't supply a tracker,
			// we supply all the results even if there are 0 changes (0 notes) in the notefile.
			openfile, file, err := box.OpenNotefile(ctx, notefileID)
			if note.ErrorContains(err, note.ErrNotefileNoExist) {
				// Listed in the notebox but not openable here, so one bad entry doesn't fail the whole listing
				logDebug(ctx, "file.changes: skipping notefile %s: %s", notefileID, err)
				continue
			}
			if err != nil {
				rsp.Err = fmt.Sprintf("error opening notefile %s to get changes: %s", notefileID, err)
				break
			}

			// Use a special internal "count only" mode of this call, for efficiency
			_, _, totalChanges, totalNotes, _, _, err := file.GetChanges(tracker, false, -1)
			if err != nil {
				rsp.Err = fmt.Sprintf("cannot get changes for %s: %s", notefileID, err)
				break
			} else {
				// Append it
				fileinfo := note.NotefileInfo{}
				if !showOnlyTotal {
					fileinfo.Changes = totalChanges
				}
				fileinfo.Total = totalNotes
				changes += totalChanges
				total += totalNotes
				fileinfoArray[notefileID] = fileinfo
			}
			openfile.Close(ctx)

		}

		// Done
		if len(fileinfoArray) != 0 {
			rsp.FileInfo = &fileinfoArray
			if !showOnlyTotal {
				rsp.Changes = int32(changes)
			}
			rsp.Total = int32(total)
		}

		// Return whether or not pending
		rsp.Pending = req.Pending && rsp.Changes > 0

	case notecard.ReqNotesGetL:
		fallthrough
	case notecard.ReqNoteChanges:
		showTotalOnly := false
		updateTracker := true

		if req.NotefileID == "" {
			rsp.Err = "no notefile specified"
			break
		}

		// Make sure that a tracker name is specified
		if req.TrackerID == "" {
			showTotalOnly = true
			updateTracker = false
			req.TrackerID = ReservedIDDelimiter
		}

		// If we're debugging and we want to see the changes for the specified tracker
		// BUT we don't want to update the tracker just yet.
		if req.Pending {
			updateTracker = false
		}

		// Make sure that it's a valid tracker in that it's not one of our known endpoints
		if req.TrackerID == note.DefaultDeviceEndpointID || req.TrackerID == note.DefaultHubEndpointID {
			rsp.Err = fmt.Sprintf("cannot use this reserved tracker name: %s", req.TrackerID)
			break
		}
		if NotefileIDIsReservedWithExceptions(req.NotefileID) && !req.Allow {
			rsp.Err = "reserved notefile name"
			break
		}

		// Open the notefile
		openfile, file, err := box.OpenNotefile(ctx, req.NotefileID)
		if err != nil {
			rsp.Err = fmt.Sprintf("error opening notefile: %s", err)
			break
		}

		// If the flag was set, clear the tracker to make sure we get all notes
		if (req.Start || req.Reset) && updateTracker {
			file.ClearTracker(req.TrackerID)
		}

		// Get the changed notes for that tracker, up to the specified (or default) max
		chgfile, _, totalChanges, totalNotes, since, until, err := file.GetChanges(req.TrackerID, req.Deleted, int(req.Max))
		if err != nil {
			if req.Stop && updateTracker {
				_ = file.DeleteTracker(req.TrackerID)
			}
			openfile.Close(ctx)
			rsp.Err = fmt.Sprintf("cannot get list of changed notes: %s", err)
			break
		}

		// Update the change tracker because we're confident that we'll return successfully
		if updateTracker {
			_ = file.UpdateChangeTracker(req.TrackerID, since, until)
		}

		// If the flag was set, delete the tracker because it is no longer needed
		if req.Stop && updateTracker {
			_ = file.DeleteTracker(req.TrackerID)
		}

		// We'll need to know if it's a queue, and whether the notefile can sync to the
		// device at all.  A note is only ever pending to the device in a notefile that
		// synchronizes from the hub; in any other notefile the device has no tracker,
		// and ChangePendingForEndpoint would report every note as pending.
		isQueue, _, syncFromHub, _, _, _ := NotefileAttributesFromID(req.NotefileID)

		// Generate the list of notes in the response data structure
		noteIDs := chgfile.NoteIDs(true)
		infolist := map[string]note.Info{}
		for i := range noteIDs {
			noteID := noteIDs[i]
			xnote, err := chgfile.GetNote(noteID)
			if err != nil {
				if rsp.Err == "" {
					rsp.Err = fmt.Sprintf("error opening note: %s", err)
				}
				break
			}

			// Get the info from the note
			info := note.Info{}
			if !isQueue && xnote.Deleted {
				info.Deleted = true
			}
			if xnote.Body != nil {
				info.Body = &xnote.Body
			}
			if len(xnote.Payload) != 0 {
				payload := xnote.GetPayload()
				info.Payload = &payload
			}
			info.When = xnote.When()
			info.WhereLat, info.WhereLon, info.WhereWhen = Where(xnote)
			if req.Edge || req.Pending {
				// Need to use the original note, not the chgfile copy, to get this info accurately
				onote, err := file.GetNote(noteID)
				if err == nil {
					if req.Pending {
						info.Pending = syncFromHub && file.ChangePendingForEndpoint(note.DefaultDeviceEndpointID, onote)
					}
					if req.Edge {
						info.Edge = onote.GetEndpointID() == note.DefaultDeviceEndpointID
					}
				}
			}

			// Add it to the list to be returned
			infolist[noteID] = info

			// Delete it as a side-effect, if desired
			if req.Delete {
				_ = file.DeleteNote(ctx, ei, endpointID, noteIDs[i])
			}

		}
		if len(infolist) > 0 {
			rsp.Notes = &infolist
			if !showTotalOnly {
				rsp.Changes = int32(totalChanges)
			}
		}
		rsp.Total = int32(totalNotes)

		// Close the notefile
		openfile.Close(ctx)

	}

	// Var.get post-processing: hoist body fields to top-level rsp fields and clear
	// the note-shaped scaffolding (Body/NoteID/Time/Deleted), mirroring the
	// notecard-side var.get epilogue in reqnote.c so the response shape matches
	// what callers expect from a var.get rather than a note.get.
	if isVarGet && rsp.Err == "" && rsp.Body != nil {
		body := *rsp.Body
		if v, ok := body["value"].(float64); ok {
			rsp.Value = v
		}
		if v, ok := body["flag"].(bool); ok {
			rsp.Flag = v
		}
		if v, ok := body["text"].(string); ok {
			rsp.Text = v
		}
		// values may arrive either as []interface{} (after a JSON round-trip
		// through note storage) or as []float64 (when the body never left
		// in-memory form), so handle both shapes.
		switch vals := body["values"].(type) {
		case []float64:
			if len(vals) > 0 {
				cp := append([]float64(nil), vals...)
				rsp.Values = &cp
			}
		case []interface{}:
			floats := make([]float64, 0, len(vals))
			for _, x := range vals {
				if f, ok := x.(float64); ok {
					floats = append(floats, f)
				}
			}
			if len(floats) > 0 {
				rsp.Values = &floats
			}
		}
		rsp.Body = nil
		rsp.NoteID = ""
		rsp.Time = 0
		rsp.Deleted = false
	}

	// Debug
	if debugRequest {
		rspJSON, _ := note.JSONMarshal(rsp)
		logDebug(ctx, "<< %s", string(rspJSON))
	}

	// Done
	return
}

// Define an invalid time so that we force a history to be added with a 0 time.
// (see packet.go and elsewhere)
const InvalidTime = int64(0xffffffff)

// Annotate a note structure with optional location and tower info from a request
func annotateNoteWithWhereWhen(xnote *note.Note, req notecard.Request) {
	if req.Time != 0 || req.Latitude != 0 || req.Longitude != 0 {
		if req.Time == InvalidTime {
			req.Time = 0
		}
		newHistory := note.History{}
		newHistory.When = req.Time
		if req.LocationOLC != "" {
			newHistory.Where = req.LocationOLC
		} else if req.Latitude != 0 || req.Longitude != 0 {
			newHistory.Where = golc.Encode(req.Latitude, req.Longitude, 12)
		}
		newHistory.WhereWhen = req.LocationTime
		newHistories := []note.History{}
		newHistories = append(newHistories, newHistory)
		xnote.Histories = &newHistories
	}
	if req.Tower != nil {
		xnote.Tower = req.Tower
	}
}

// ErrorResponse creates a simple JSON response given an error
func ErrorResponse(err error) (response []byte) {
	rsp := notecard.Request{}
	rsp.Err = fmt.Sprintf("%s", err)
	response, _ = note.JSONMarshal(&rsp)
	return
}
