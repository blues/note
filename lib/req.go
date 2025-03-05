// Copyright 2017 Blues Inc.  All rights reserved.
// Use of this source code is governed by licenses granted by the
// copyright holder including that found in the LICENSE file.

package notelib

import (
	"context"
	"fmt"
	"math"
	"strings"

	"github.com/blues/note-go/note"
	"github.com/blues/note-go/notecard"
)

// Request performs a local operation using the JSON API
func (box *Notebox) Request(ctx context.Context, endpointID string, reqJSON []byte) (rspJSON []byte) {
	var err error
	req := notecard.Request{}
	rsp := notecard.Request{}

	// Debug
	if debugRequest {
		logDebug(ctx, ">> %s", string(reqJSON))
	}

	// Unmarshal the incoming request
	err = note.JSONUnmarshal(reqJSON, &req)
	if err != nil {
		rsp.Err = fmt.Sprintf("unknown request: %s", err)
		rspJSON, _ = note.JSONMarshal(rsp)
		if debugRequest {
			logDebug(ctx, "<< %s", string(rspJSON))
		}
		return
	}

	// Extract the request ID, which will be used to correlate requests with responses
	rsp.RequestID = req.RequestID

	// Handle legacy which used "files." instead of "file.".  This was changed 2019-11-18 and
	// can be removed after we ship because the compat was just until we distributed new notecard fw.
	req.Req = strings.Replace(req.Req, "files.", "file.", -1)

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
		err = box.VerifyAccess(note.ACResourceNotefiles, note.ACActionCreate)
		if err != nil {
			rsp.Err = fmt.Sprintf("%s", err)
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
		err = box.VerifyAccess(note.ACResourceNotefiles, note.ACActionDelete)
		if err != nil {
			rsp.Err = fmt.Sprintf("%s", err)
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
		// Check for access to add a note
		err = box.VerifyAccess(note.ACResourceNotefile+req.NotefileID, note.ACActionCreate)
		if err != nil {

			// Preset assuming access failure
			rsp.Err = fmt.Sprintf("%s", err)

			// Special-case access check for notefiles with "add only" access control
			var info note.NotefileInfo
			info, err = box.GetNotefileInfo(req.NotefileID)
			if err != nil || !info.AnonAddAllowed {
				break
			}

		}
		// Make sure that the file exists
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
		// Check the noteID
		isQueue, toHub, _, _, _, _ := NotefileAttributesFromID(req.NotefileID)
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
			err = box.AddNoteWithHistory(ctx, endpointID, req.NotefileID, req.NoteID, xnote)
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
		err = box.VerifyAccess(note.ACResourceNotefile+req.NotefileID, note.ACActionUpdate)
		if err != nil {
			rsp.Err = fmt.Sprintf("%s", err)
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
				err = box.AddNoteWithHistory(ctx, endpointID, req.NotefileID, req.NoteID, xnote)
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
		err = box.UpdateNote(ctx, endpointID, req.NotefileID, req.NoteID, xnote)
		if err != nil {
			rsp.Err = fmt.Sprintf("error updating note: %s", err)
			break
		}

	case notecard.ReqNoteDelete:
		err = box.VerifyAccess(note.ACResourceNotefile+req.NotefileID, note.ACActionDelete)
		if err != nil {
			rsp.Err = fmt.Sprintf("%s", err)
			break
		}
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
		err = box.DeleteNote(ctx, endpointID, req.NotefileID, req.NoteID)
		if err != nil {
			rsp.Err = fmt.Sprintf("error deleting note: %s", err)
			break
		}

	case notecard.ReqNoteGet:
		if req.NotefileID == "" {
			rsp.Err = "no notefile specified"
			break
		}
		isQueue, _, _, _, _, _ := NotefileAttributesFromID(req.NotefileID)
		if req.NoteID == "" {
			if !isQueue {
				rsp.Err = "no note ID specified"
				break
			}
		}
		if isQueue || req.Delete {
			err = box.VerifyAccess(note.ACResourceNotefile+req.NotefileID, note.ACActionRead+note.ACActionAnd+note.ACActionDelete)
			if err != nil {
				rsp.Err = fmt.Sprintf("%s", err)
				break
			}
		} else {
			err = box.VerifyAccess(note.ACResourceNotefile+req.NotefileID, note.ACActionRead)
			if err != nil {
				rsp.Err = fmt.Sprintf("%s", err)
				break
			}
		}
		// Handle queues by getting the LRU note.  Note that we allow either inbound
		// or outbound queues to be accessed this way via API, explicitly to allow
		// the API to be used to manage queues in either direction.
		if isQueue && req.NoteID == "" {
			openfile, file, err2 := box.OpenNotefile(ctx, req.NotefileID)
			if err2 != nil {
				rsp.Err = fmt.Sprintf("%s", err2)
				break
			}
			req.NoteID, err = file.GetLeastRecentNoteID()
			if err != nil {
				openfile.Close(ctx)
				rsp.Err = fmt.Sprintf("%s", err)
				break
			}
			openfile.Close(ctx)
		}
		// Get the note
		var xnote note.Note
		xnote, err = box.GetNote(ctx, req.NotefileID, req.NoteID)
		if err != nil {
			rsp.Err = fmt.Sprintf("error getting note: %s", err)
			break
		}
		rsp.NoteID = req.NoteID
		if !isQueue && xnote.Deleted {
			if !req.Deleted {
				rsp.Err = fmt.Sprintf("note has been deleted: %s "+note.ErrNoteNoExist, req.NoteID)
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
		if xnote.Histories != nil && len(*xnote.Histories) > 0 {
			histories := *xnote.Histories

			if histories[0].When < 1483228800 || histories[0].When > math.MaxUint32 { // before 1/1/2017 or can't fit into a uint32
				rsp.Time = 0
			} else {
				rsp.Time = histories[0].When
			}
		}
		if req.Delete {
			_ = box.DeleteNote(ctx, endpointID, req.NotefileID, req.NoteID)
		}

	case notecard.ReqFileGetL:
		fallthrough
	case notecard.ReqFileChanges:
		fallthrough
	case notecard.ReqFileChangesPending:
		var notefiles []string
		changes := 0
		total := 0
		showOnlyTotal := false

		// Check access
		err = box.VerifyAccess(note.ACResourceNotefiles, note.ACActionRead)
		if err != nil {
			rsp.Err = fmt.Sprintf("%s", err)
			break
		}

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
		// are .qo, .qos, and anything ending in 'x' which is a local-only file
		newNotefiles := []string{}
		for _, file := range notefiles {
			isQueue, syncToHub, syncFromHub, _, _, _ := NotefileAttributesFromID(file)
			if !(isQueue && syncToHub) && (syncToHub || syncFromHub) {
				newNotefiles = append(newNotefiles, file)
			}
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

		// Check access
		if req.Delete {
			err = box.VerifyAccess(note.ACResourceNotefile+req.NotefileID, note.ACActionRead+note.ACActionAnd+note.ACActionDelete)
			if err != nil {
				rsp.Err = fmt.Sprintf("%s", err)
				break
			}
		} else {
			err = box.VerifyAccess(note.ACResourceNotefile+req.NotefileID, note.ACActionRead)
			if err != nil {
				rsp.Err = fmt.Sprintf("%s", err)
				break
			}
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

		// We'll need to know if it's a queue
		isQueue, _, _, _, _, _ := NotefileAttributesFromID(req.NotefileID)

		// Generate the list of notes in the response data structure
		noteIDs := chgfile.NoteIDs(true)
		infolist := map[string]note.Info{}
		for i := range noteIDs {
			xnote, err := chgfile.GetNote(noteIDs[i])
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

			// Add it to the list to be returned
			infolist[noteIDs[i]] = info

			// Delete it as a side-effect, if desired
			if req.Delete {
				_ = file.DeleteNote(ctx, endpointID, noteIDs[i])
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

	// Marshal a response
	rspJSON, _ = note.JSONMarshal(rsp)

	// Append a \n so that the requester can recognize end-of-response
	rspJSON = []byte(string(rspJSON) + "\n")

	// Debug
	if debugRequest {
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
		newHistory.Where = req.LocationOLC
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
