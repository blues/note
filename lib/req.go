// Copyright 2017 Blues Inc.  All rights reserved.
// Use of this source code is governed by licenses granted by the
// copyright holder including that found in the LICENSE file.

package notelib

import (
	"fmt"
	"strings"

	"github.com/blues/note-go/note"
	"github.com/blues/note-go/notecard"
)

// Request performs a local operation using the JSON API
func (box *Notebox) Request(endpointID string, reqJSON []byte) (rspJSON []byte) {
	var err error
	req := notecard.Request{}
	rsp := notecard.Request{}

	// Debug
	if debugRequest {
		debugf(">> %s\n", string(reqJSON))
	}

	// Unmarshal the incoming request
	err = note.JSONUnmarshal(reqJSON, &req)
	if err != nil {
		rsp.Err = fmt.Sprintf("unknown request: %s", err)
		rspJSON, _ = note.JSONMarshal(rsp)
		if debugRequest {
			debugf("<< %s\n", string(rspJSON))
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
			if !NotefileIDIsReserved(notefileID) {
				err = box.AddNotefile(notefileID, &notefileInfo)
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
			if !NotefileIDIsReserved(deleteFiles[i]) {
				err = box.DeleteNotefile(deleteFiles[i])
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
		if NotefileIDIsReserved(req.NotefileID) {
			rsp.Err = "reserved notefile name"
			break
		}
		if !box.NotefileExists(req.NotefileID) {
			err = box.AddNotefile(req.NotefileID, nil)
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
		if isQueue && toHub && !fromHub {
			rsp.Err = "can't add note to an outbound queue"
			break
		}
		// Add the note
		xnote := note.Note{}
		if req.Payload != nil {
			xnote.Payload = *req.Payload
		}
		if req.Body != nil {
			xnote.Body = *req.Body
		}
		err = box.AddNote(endpointID, req.NotefileID, req.NoteID, xnote)
		if err != nil {
			rsp.Err = fmt.Sprintf("%s", err)
			break
		}

	case notecard.ReqNoteUpdate:
		if req.NotefileID == "" {
			rsp.Err = "no notefile specified"
			break
		}
		if NotefileIDIsReserved(req.NotefileID) {
			rsp.Err = "reserved notefile name"
			break
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
		xnote, err = box.GetNote(req.NotefileID, req.NoteID)
		if err != nil {
			if req.Payload != nil {
				xnote.Payload = *req.Payload
			}
			if req.Body != nil {
				xnote.Body = *req.Body
			}
			err = box.AddNote(endpointID, req.NotefileID, req.NoteID, xnote)
			if err != nil {
				rsp.Err = fmt.Sprintf("error adding note: %s", err)
				break
			}
			break
		}
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
		err = box.UpdateNote(endpointID, req.NotefileID, req.NoteID, xnote)
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
		if NotefileIDIsReserved(req.NotefileID) {
			rsp.Err = "reserved notefile name"
			break
		}
		if req.NoteID == "" {
			rsp.Err = "no note ID specified"
			break
		}
		err = box.DeleteNote(endpointID, req.NotefileID, req.NoteID)
		if err != nil {
			rsp.Err = fmt.Sprintf("error deleting note: %s", err)
			break
		}

	case notecard.ReqNoteGet:
		if req.NotefileID == "" {
			rsp.Err = "no notefile specified"
			break
		}
		if NotefileIDIsReserved(req.NotefileID) {
			rsp.Err = "reserved notefile name"
			break
		}
		isQueue, toHub, _, _, _, _ := NotefileAttributesFromID(req.NotefileID)
		if req.NoteID == "" {
			if !isQueue || !toHub {
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
		// Handle queues by getting the LRU note
		if isQueue && toHub && req.NoteID == "" {
			openfile, file, err2 := box.OpenNotefile(req.NotefileID)
			if err2 != nil {
				rsp.Err = fmt.Sprintf("%s", err2)
				break
			}
			req.NoteID, err = file.GetLeastRecentNoteID()
			if err != nil {
				openfile.Close()
				rsp.Err = fmt.Sprintf("%s", err)
				break
			}
			openfile.Close()
		}
		// Get the note
		var xnote note.Note
		xnote, err = box.GetNote(req.NotefileID, req.NoteID)
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
			rsp.Time = histories[0].When
			if rsp.Time < 1483228800 { // 1/1/2017
				rsp.Time = 0
			}
		}
		if req.Delete {
			box.DeleteNote(endpointID, req.NotefileID, req.NoteID)
		}

	case notecard.ReqFileGetL:
		fallthrough
	case notecard.ReqFileChanges:
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

		// If no tracker, generate the entire list of notefiles
		tracker := req.TrackerID
		if tracker == "" {

			// Use the special reserved name, which (for internal use only) means "no tracker"
			showOnlyTotal = true
			tracker = ReservedIDDelimiter
			notefiles, err = box.Notefiles(false)
			if err != nil {
				rsp.Err = fmt.Sprintf("cannot get list of all notefiles: %s", err)
				break
			}

		} else {

			// Make sure that it's a valid tracker in that it's not one of our known endpoints
			if req.TrackerID == note.DefaultDeviceEndpointID || req.TrackerID == note.DefaultHubEndpointID {
				rsp.Err = fmt.Sprintf("cannot use this reserved tracker name: %s", req.TrackerID)
				break
			}

			// Update the environment vars for the notebox, which may result in a changed _env.dbs
			hubUpdateEnvVars(box)

			// Get the changed notefiles for that tracker
			notefiles, err = box.GetChangedNotefiles(req.TrackerID)
			if err != nil {
				rsp.Err = fmt.Sprintf("cannot get list of changed notefiles: %s", err)
				break
			}

		}

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
			if NotefileIDIsReserved(notefileID) {
				continue
			}

			// Get the number of pending changes.  Note that if we didn't supply a tracker,
			// we supply all the results even if there are 0 changes (0 notes) in the notefile.
			openfile, file, err := box.OpenNotefile(notefileID)
			if err != nil {
				rsp.Err = fmt.Sprintf("error opening notefile %s to get changes: %s", notefileID, err)
				break
			}

			// Use a special internal "count only" mode of this call, for efficiency
			_, _, totalChanges, totalNotes, _, _, err := file.GetChanges(tracker, -1)
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
			openfile.Close()

		}

		// Done
		if len(fileinfoArray) != 0 {
			rsp.FileInfo = &fileinfoArray
			if !showOnlyTotal {
				rsp.Changes = int32(changes)
			}
			rsp.Total = int32(total)
		}

	case notecard.ReqNotesGetL:
		fallthrough
	case notecard.ReqNoteChanges:
		showTotalOnly := false
		updateTracker := true

		if req.NotefileID == "" {
			rsp.Err = "no notefile specified"
			break
		}
		if NotefileIDIsReserved(req.NotefileID) {
			rsp.Err = "reserved notefile name"
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
			break
		}

		// Make sure that it's a valid tracker in that it's not one of our known endpoints
		if req.TrackerID == note.DefaultDeviceEndpointID || req.TrackerID == note.DefaultHubEndpointID {
			rsp.Err = fmt.Sprintf("cannot use this reserved tracker name: %s", req.TrackerID)
			break
		}

		// Open the notefile
		openfile, file, err := box.OpenNotefile(req.NotefileID)
		if err != nil {
			rsp.Err = fmt.Sprintf("error opening notefile: %s", err)
			break
		}

		// If the flag was set, clear the tracker to make sure we get all notes
		if req.Start && updateTracker {
			file.ClearTracker(req.TrackerID)
		}

		// Get the changed notes for that tracker, up to the specified (or default) max
		chgfile, _, totalChanges, totalNotes, since, until, err := file.GetChanges(req.TrackerID, int(req.Max))
		if err != nil {
			if req.Stop && updateTracker {
				file.DeleteTracker(req.TrackerID)
			}
			openfile.Close()
			rsp.Err = fmt.Sprintf("cannot get list of changed notes: %s", err)
			break
		}

		// Update the change tracker because we're confident that we'll return successfully
		if updateTracker {
			file.UpdateChangeTracker(req.TrackerID, since, until)
		}

		// If the flag was set, delete the tracker because it is no longer needed
		if req.Stop && updateTracker {
			file.DeleteTracker(req.TrackerID)
		}

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

			// Skip it if we don't want deleted
			if xnote.Deleted && !req.Deleted {
				continue
			}

			// Get the info from the note
			info := note.Info{}
			if xnote.Deleted {
				info.Deleted = true
			}
			if xnote.Body != nil {
				info.Body = &xnote.Body
			}
			if len(xnote.Payload) != 0 {
				payload := xnote.GetPayload()
				info.Payload = &payload
			}

			// Add it to the list to be returned
			infolist[noteIDs[i]] = info

			// Delete it as a side-effect, if desired
			if req.Delete && !xnote.Deleted {
				file.DeleteNote(endpointID, noteIDs[i])
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
		openfile.Close()

	}

	// Marshal a response
	rspJSON, _ = note.JSONMarshal(rsp)

	// Append a \n so that the requestor can recognize end-of-response
	rspJSON = []byte(string(rspJSON) + "\n")

	// Debug
	if debugRequest {
		debugf("<< %s\n", string(rspJSON))
	}

	// Done
	return

}

// ErrorResponse creates a simple JSON response given an error
func ErrorResponse(err error) (response []byte) {
	rsp := notecard.Request{}
	rsp.Err = fmt.Sprintf("%s", err)
	response, _ = note.JSONMarshal(&rsp)
	return
}
