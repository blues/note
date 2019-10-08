// Copyright 2017 Blues Inc.  All rights reserved.
// Use of this source code is governed by licenses granted by the
// copyright holder including that found in the LICENSE file.
// Derived from the "FeedSync" sample code which was covered
// by the Microsoft Public License (Ms-Pl) of December 3, 2007.
// FeedSync itself was derived from the algorithms underlying
// Lotus Notes replication, developed by Ray Ozzie et al c.1985.

// Package notelib notefile.go handles management and sync of collections of individual notes
package notelib

import (
	"encoding/json"
	"fmt"
	"github.com/blues/note-go/note"
	"github.com/google/uuid"
	"math/rand"
	"sync"
	"time"
)

// Notefile is The outermost data structure of a Notefile JSON
// object, containing a set of notes that may be synchronized.
type Notefile struct {
	modCount        int                  // Incremented every modification, for checkpointing purposes
	eventFn         EventFunc            // The function to call when eventing of a change
	eventCtx        interface{}          // An argument to be passed to the event call
	eventDeviceUID  string               // The deviceUID being dealt with at the time of the event setup
	eventDeviceSN   string               // The deviceSN being dealt with at the time of the event setup
	eventProductUID string               // The productUID being dealt with at the time of the event setup
	eventAppUID     string               // The appUID being dealt with at the time of the event setup
	notefileID      string               // The NotefileID for the open notefile
	notefileInfo    note.NotefileInfo    // The NotefileInfo for the open notefile
	Queue           bool                 `json:"Q,omitempty"`
	Notes           map[string]note.Note `json:"N,omitempty"`
	Trackers        map[string]Tracker   `json:"T,omitempty"`
	Change          int64                `json:"C,omitempty"`
}

// Tracker is the structure maintained on a per-endpoint basis
type Tracker struct {
	Change    int64 `json:"c,omitempty"`
	SessionID int64 `json:"i,omitempty"`
}

// defaultMaxGetChangesBatchSize is the default for the max of changes that we'll return in a batch of GetChanges.
const defaultMaxGetChangesBatchSize = 10

// inboundQueuesProcessedByNotifier defines whether or not the local behavior
// is that queues are processed by the notifier, or if they
// are processed externally.  On devices, inbound queues are processed by the
// app which looks for them.  On the service, inbound queues are processed by the notifier.
const inboundQueuesProcessedByNotifier = true

// For now, we use just a single mutex to protect all operations
// for notefiles. If we were to protect each notebox with its own mutex,
// as I used to do, we would need to find a stable place to put them
// in static memory because the notefile address changes whenever
// the notefile is reallocated.
var nfLock sync.RWMutex

// Tests to see if a note is deleted, but NOT a "sent pre-deleted" note, knowing
// that we pre-delete notes that are sent.
func isFullTombstone(note note.Note) bool {
	return (note.Deleted && !note.Sent)
}

// CreateNotefile instantiates and initializes a new Notefile, and prepares it so that
// notes may be added on behalf of the specified endpoint.  The ID of that endpoint
// may be changed at any time and is merely an affordance so that we don't need to
// put the endpointID onto every Note call.
func CreateNotefile(isQueue bool) Notefile {
	newNotefile := Notefile{}
	newNotefile.Notes = map[string]note.Note{}
	newNotefile.Trackers = map[string]Tracker{}
	newNotefile.Queue = isQueue
	// Change must start at 1 because trackers with Change==0 are "inactive" trackers
	newNotefile.Change = 1
	return newNotefile
}

// Close closes and frees the Notefile
func (nf *Notefile) Close() {
}

// Modified returns the number of times this has been modified in-memory since being
// opened.  Note that this is not persistent, and is intended to be used for checkpointing.
func (nf *Notefile) Modified() int {
	return nf.modCount
}

// Notefile returns the notefileID for this notefile
func (nf *Notefile) Notefile() (notefileID string) {
	return nf.notefileID
}

// NoteIDs retrieves the list of all Note IDs in the notefile
func (nf *Notefile) NoteIDs(includeTombstones bool) (noteIDs []string) {
	noteIDs = []string{}

	nfLock.RLock()
	for noteID, note := range nf.Notes {
		if includeTombstones {
			noteIDs = append(noteIDs, noteID)
		} else {
			if !isFullTombstone(note) {
				noteIDs = append(noteIDs, noteID)
			}
		}
	}
	nfLock.RUnlock()

	return

}

// CountNotes returns the count of notes within this notefile
func (nf *Notefile) CountNotes(includeTombstones bool) (count int) {

	nfLock.RLock()
	for _, note := range nf.Notes {
		if includeTombstones {
			count++
		} else {
			if !isFullTombstone(note) {
				count++
			}
		}
	}
	nfLock.RUnlock()

	return

}

// Info returns the notefileinfo as it was when this notefile was opened
func (nf *Notefile) Info() (info note.NotefileInfo) {
	return nf.notefileInfo
}

// SetEventInfo supplies information used for change notification
func (nf *Notefile) SetEventInfo(deviceUID string, deviceSN string, productUID string, appUID string, fn EventFunc, fnctx interface{}) error {

	// Lock for writing
	nfLock.Lock()

	// Lay out the data needed for the call
	nf.eventFn = fn
	nf.eventCtx = fnctx
	nf.eventDeviceUID = deviceUID
	nf.eventDeviceSN = deviceSN
	nf.eventProductUID = productUID
	nf.eventAppUID = appUID

	// Unlock & exit
	nfLock.Unlock()
	return nil

}

// NewNoteID Generates a new unique Note ID.  We must choose this in a way such that
// it is unique EVEN IF we don't have all the notes in this file because of a prior
// purge.  That is, it must be unique even as it relates to notes that are no longer
// in the database, because those same notes may still be in a replica DB.
func (nf *Notefile) uNewNoteID(endpointID string) string {

	// We will need a single random byte below
	buf := make([]byte, 1)
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	r.Read(buf)
	randomByte := int64(buf[0])

	// Compute the prefix for the new Note ID, to give assurance that we won't step on each other
	// even though operating fully distributed.
	prefix := endpointID + ":"
	if endpointID == "" {
		prefix = ""
	}

	// Use the "change number", which is guaranteed to be correct unless for some reason
	// the local database has been deleted and started anew.  In order to cover
	// that case, even though it's rare, just shift it over and add a random byte.
	randomID := (nf.Change * 256) + randomByte

	// Done
	return fmt.Sprintf("%s%d", prefix, randomID)

}

// NewNoteID Generates a new unique Note ID
func (nf *Notefile) NewNoteID(endpointID string) string {

	// Lock this, because we'll be testing for the presence of different note IDs
	nfLock.RLock()

	// Generate one
	noteID := nf.uNewNoteID(endpointID)

	// Unlock and exit
	nfLock.RUnlock()

	return noteID

}

// GetNote retrieves a copy of a note from a Notefile, by ID
func (nf *Notefile) GetNote(noteID string) (xnote note.Note, err error) {

	// Lock for reading
	nfLock.RLock()

	// Read the note
	_, present := nf.Notes[noteID]
	if !present {
		nfLock.RUnlock()
		return note.Note{}, fmt.Errorf(ErrNoteNoExist+" note not found: %s", noteID)
	}
	xnote = nf.Notes[noteID]

	// Unlock
	nfLock.RUnlock()
	return xnote, nil

}

// processEvent handles a notification command
// NOTE that for Update/Add/Delete, it is up to the caller to replace
// event.EndpointID as appropriate as the source for the operation!
func (nf *Notefile) processEvent(event note.Event) (err error) {

	// Perform the desired action
	switch event.Req {

	case note.EventNoAction:
		// Do nothing

	case note.EventAdd:
		xnote := note.Note{}
		xnote.Body = *event.Body
		xnote.Payload = event.Payload
		err = nf.AddNote(event.EndpointID, event.NoteID, xnote)
		if err != nil {
			return fmt.Errorf("cannot add note %s to %s: %s", event.NoteID, nf.notefileID, err)
		}

	case note.EventUpdate:
		var xnote note.Note
		xnote, err = nf.GetNote(event.NoteID)
		if err != nil {
			return err
		}
		xnote.Body = *event.Body
		xnote.Payload = event.Payload
		err = nf.UpdateNote(event.EndpointID, event.NoteID, xnote)
		if err != nil {
			return fmt.Errorf("cannot update note %s in %s: %s", event.NoteID, nf.notefileID, err)
		}

	case note.EventDelete:
		err = nf.DeleteNote(event.EndpointID, event.NoteID)
		if err != nil {
			return fmt.Errorf("error deleting note %s in %s: %s", event.NoteID, nf.notefileID, err)
		}

		// Purge tombstones, because unlike Merge there is no client-initiated call that is going to
		// eventually get rid of all the dead wood within the file.
		nf.PurgeTombstones(event.EndpointID)

	default:
		return fmt.Errorf("unrecognized request: %s", event.Req)

	}

	return err

}

// event dispatches to the event proc based on the last change made to NoteID
func (nf *Notefile) event(local bool, NoteID string) {

	if nf.eventFn == nil {
		return
	}

	// Fetch the note
	xnote, err := nf.GetNote(NoteID)
	if err != nil {
		// This error is expected for notes that have been deleted from queues
		if ErrorContains(err, ErrNoteNoExist) {
			return
		}
		debugf("event: GetNote(%s) error: %s", NoteID, err)
		return
	}

	// Set up the notification structure
	event := note.Event{}
	event.EventUID = uuid.New().String()
	if xnote.Deleted && !nf.Queue {
		event.Req = note.EventDelete
	} else if xnote.Updates != 0 {
		event.Req = note.EventUpdate
	} else {
		event.Req = note.EventAdd
	}
	event.NoteID = NoteID
	event.EndpointID = xnote.EndpointID()
	event.Deleted = xnote.Deleted
	event.Sent = xnote.Sent
	event.Bulk = xnote.Bulk
	event.Updates = xnote.Updates
	event.NotefileID = nf.notefileID
	event.DeviceUID = nf.eventDeviceUID
	event.DeviceSN = nf.eventDeviceSN
	event.ProductUID = nf.eventProductUID
	if nf.eventAppUID != "" {
		app := note.EventApp{}
		event.App = &app
		event.App.AppUID = nf.eventAppUID
	}
	event.Payload = xnote.Payload
	if xnote.Body != nil {
		event.Body = &xnote.Body
	}
	if xnote.Histories != nil && len(*xnote.Histories) > 0 {
		histories := *xnote.Histories
		event.When = histories[0].When
		event.Where = histories[0].Where
	}

	// Clean the event in case it's a queue event
	if event.Sent {
		event.NoteID = ""
		event.Sent = false
		event.Deleted = false
	}

	// Debug
	if debugEvent {
		debugf("event: %s %s %s %s %s", event.Req, event.NotefileID, nf.eventAppUID, event.DeviceUID, event.ProductUID)
		if len(event.Payload) > 0 {
			if event.Bulk {
				debugf(" (%d-byte BULK payload)", len(event.Payload))
			} else {
				debugf(" (%d-byte payload)", len(event.Payload))
			}
		}
		if event.Body != nil {
			bodyJSON, err := json.Marshal(event.Body)
			if err != nil {
				debugf(" (CANNOT MARSHAL TEMPLATE - BULK DATA DISCARDED)\n%v\n", event.Body)
				return
			}
			debugf(" %s", string(bodyJSON))
		}
		debugf("\n")
	}

	// Disable the event function so as to avoid recursion
	savedFn := nf.eventFn
	nf.eventFn = nil

	// Perform the notification (checking savedFn in case of race condition, because everything is unlocked
	if savedFn != nil {
		err = savedFn(nf.eventCtx, local, nf, &event)
		if err != nil {
			debugf("event error: %s\n", err)
		} else {
			event.Req = event.Rsp
			err = nf.processEvent(event)
			if err != nil {
				debugf("error processing event response: %s\n", err)
			}
		}
	}

	// Restore the function
	nf.eventFn = savedFn

}

// unlockedAddNoteEx adds a newly-created Note to a Notefile, without modifying any
// fields in the note other than the "Change" field.  The caller is responsible
// for updating modCount.
func (nf *Notefile) uAddNoteEx(noteID string, note *note.Note) error {

	// Find it
	_, present := nf.Notes[noteID]
	if present {
		return fmt.Errorf(ErrNoteExists+" note already exists: %s", noteID)
	}

	// Make the change
	note.Change = nf.Change
	nf.Change++
	nf.Notes[noteID] = *note

	// Done
	return nil
}

// Add a note, unlocked
func (nf *Notefile) uAddNote(endpointID string, noteID string, xnote *note.Note, deleted bool) error {

	// Set the required fields
	xnote.Updates = 0
	xnote.Deleted = deleted
	xnote.Histories = nil
	xnote.Conflicts = nil

	// Append the first history
	noteHistories := []note.History{}
	noteHistories = append(noteHistories, newHistory(endpointID, 0, "", 0))
	xnote.Histories = &noteHistories

	// Do it
	err := nf.uAddNoteEx(noteID, xnote)

	// Mark modified and exit
	nf.modCount++
	return err

}

// AddNote adds a newly-created Note to a Notefile.  After the note is added to the Notefile,
// it should no longer be used because its contents is copied into the Notefile.  If a unique
// ID for this note isn't supplied, one is generated.  Note that if this is a queue,
// AddNote adds the newly-created note in a pre-deleted state.  This is quite useful
// for notefiles that are being "tracked", because after all trackers receive it, they will
// undelete it and thus it will be present everywhere except the source.  This is an intentional
// asymmetry in synchronization that is primarily useful when sending "to hub" or "from hub".
// By convention, sent notes should be deleted after they have been processed by the recipient(s).
func (nf *Notefile) AddNote(endpointID string, noteID string, xnote note.Note) (err error) {

	// Create a note ID if not specified
	if nf.Queue || noteID == "" {
		noteID = nf.NewNoteID(endpointID)
	}

	// Lock for writing
	nfLock.Lock()

	// Add it
	xnote.Sent = nf.Queue
	err = nf.uAddNote(endpointID, noteID, &xnote, nf.Queue)

	// Unlock
	nfLock.Unlock()

	// Exit if err
	if err != nil {
		return err
	}

	// Event listeners
	nf.event(endpointID != note.DefaultDeviceEndpointID, noteID)

	// Done
	return err

}

// unlockedReplaceNote the contents of a Note in the Notefile with a completely different one
func (nf *Notefile) uReplaceNote(noteID string, xnote *note.Note) error {

	existingNote, present := nf.Notes[noteID]
	if !present {
		return fmt.Errorf("during merge cannot replace note: "+ErrNoteNoExist+" note not found: %s", noteID)
	}

	// If we're replacing a non-deleted note with a deletion of a Sent note, we can opportunistically purge
	// it because it can and will never be replicated outbound.
	if existingNote.Sent && xnote.Deleted {
		delete(nf.Notes, noteID)
	} else {
		xnote.Change = nf.Change
		nf.Change++
		nf.Notes[noteID] = *xnote
	}

	nf.modCount++
	return nil

}

// uUpdateNote updates unlocked
func (nf *Notefile) uUpdateNote(endpointID string, noteID string, xnote *note.Note) error {

	_, present := nf.Notes[noteID]
	if !present {
		return fmt.Errorf(ErrNoteNoExist+" cannot update: note not found: %s", noteID)
	}

	// Update the history, etc
	updateNote(xnote, endpointID, false, false)

	// Replace it
	return nf.uReplaceNote(noteID, xnote)

}

// UpdateNote updates an existing note within a Notefile, as well
// as updating all its history and conflict metadata as appropriate.
func (nf *Notefile) UpdateNote(endpointID string, noteID string, xnote note.Note) error {

	// Exit if trying to update within a queue
	if nf.Queue {
		return fmt.Errorf(ErrNotefileQueueDisallowed + " operation not allowed on queue notefiles")
	}

	// Lock for writing
	nfLock.Lock()

	// Update
	err := nf.uUpdateNote(endpointID, noteID, &xnote)

	// Unlock & exit
	nfLock.Unlock()

	// Event listeners
	if err == nil {
		nf.event(endpointID != note.DefaultDeviceEndpointID, noteID)
	}

	return err
}

// DeleteNote sets the deleted flag on an existing note from in a Notefile,
// marking it so that it will be purged at a later time when safe to do so
// from a synchronization perspective.
func (nf *Notefile) uDeleteNote(endpointID string, noteID string, xnote *note.Note) (err error) {

	// If this is a queue, do an automatic purge
	if nf.Queue {
		delete(nf.Notes, noteID)
		return
	}

	// Update the history, etc
	updateNote(xnote, endpointID, false, true)

	// Replace it
	return nf.uReplaceNote(noteID, xnote)

}

// DeleteNote sets the deleted flag on an existing note from in a Notefile,
// marking it so that it will be purged at a later time when safe to do so
// from a synchronization perspective.
func (nf *Notefile) DeleteNote(endpointID string, noteID string) error {

	// Lock for writing
	nfLock.Lock()

	// Read the existing note
	xnote, present := nf.Notes[noteID]
	if !present {
		nfLock.Unlock()
		return fmt.Errorf(ErrNoteNoExist+" cannot delete note: note not found: %s", noteID)
	}

	// Delete it
	err := nf.uDeleteNote(endpointID, noteID, &xnote)

	// Unlock
	nfLock.Unlock()

	// Event listeners
	if err == nil {
		nf.event(endpointID != note.DefaultDeviceEndpointID, noteID)
	}

	// Done
	return err

}

// resolveNoteConflicts resolves all conflicts that are pending for a Note in a Notefile
func (nf *Notefile) resolveNoteConflicts(endpointID string, noteID string, xnote note.Note) error {

	// Lock for writing
	nfLock.Lock()

	_, present := nf.Notes[noteID]
	if !present {
		nfLock.Unlock()
		return fmt.Errorf(ErrNoteNoExist+" cannot resolve conflicts: note not found: %s", noteID)
	}

	updateNote(&xnote, endpointID, true, false)
	xnote.Change = nf.Change
	nf.Change++
	nf.Notes[noteID] = xnote

	// Done
	nf.modCount++
	nfLock.Unlock()
	return nil

}

// Get the list of Notes that must be merged
func (nf *Notefile) uGetMergeNoteChangeList(fromNotefile *Notefile) (notelist []note.Note, noteidlist []string) {

	mergeNoteChangeList := []note.Note{}
	mergeNoteIDChangeList := []string{}

	fromNotes := []note.Note{}
	fromNoteIDs := []string{}
	for fromNoteID, Note := range fromNotefile.Notes {
		fromNotes = append(fromNotes, Note)
		fromNoteIDs = append(fromNoteIDs, fromNoteID)
	}

	for i, fromNote := range fromNotes {
		fromNoteID := fromNoteIDs[i]
		_, present := nf.Notes[fromNoteID]

		mergedNote := fromNote

		if present {
			LocalNote := nf.Notes[fromNoteID]
			ConflictDataDiffers, CompareResult := compareModified(LocalNote, fromNote)
			NeedToMerge :=
				(ConflictDataDiffers) ||
					(CompareResult == -1) ||
					((CompareResult == 1) && (!isSubsumedBy(fromNote, LocalNote)))

			if !NeedToMerge {
				continue
			}

			mergedNote = Merge(&LocalNote, &fromNote)
		}

		mergeNoteChangeList = append(mergeNoteChangeList, mergedNote)
		mergeNoteIDChangeList = append(mergeNoteIDChangeList, fromNoteID)

	}

	return mergeNoteChangeList, mergeNoteIDChangeList
}

// MergeNotefile combines/merges the entire contents of one Notefile into another
func (nf *Notefile) MergeNotefile(fromNotefile Notefile) error {

	// Lock for writing
	nfLock.Lock()

	// Perform the merge
	var firstError error
	modifiedNoteIDs := []string{}
	mergeNoteChangeList, mergeNoteIDChangeList := nf.uGetMergeNoteChangeList(&fromNotefile)
	for i, mergeNote := range mergeNoteChangeList {
		idMergeNote := mergeNoteIDChangeList[i]

		// Now we do special-case checking for notes that are "sent" into the hub.  A "sent" note is one that
		// is created as a deleted tombstone by the source, and which is then marked as "not deleted" by any
		// endpoint that receives it via Merge.  If we detect this special case, we "undelete" it so that
		// it appears as a new note to those being notified.  It will be deleted below after notification.
		if mergeNote.Sent {
			mergeNote.Deleted = false
		}

		// See if it's present
		_, present := nf.Notes[idMergeNote]
		var thisError error

		// Add or replace the note, as appropriate
		if !present {
			// Add the note
			thisError = nf.uAddNoteEx(idMergeNote, &mergeNote)
			nf.modCount++
		} else {
			thisError = nf.uReplaceNote(idMergeNote, &mergeNote)
			nf.modCount++
		}

		// If no error, add this to the list of notes to later trigger the notifier
		if thisError == nil {
			modifiedNoteIDs = append(modifiedNoteIDs, idMergeNote)
		}

		// Only return the first error that occurs
		if firstError == nil {
			firstError = thisError
		}

	}

	// Done with lock
	nfLock.Unlock()

	// We've now got a list of successfully added or merged notes.  Now that everything is unlocked,
	// go through this list and event the server of what has transpired.  Also, if the destination is
	// an inbound queue, delete the contents after modification because this is the core essential
	// behavior of queues
	for i := range modifiedNoteIDs {
		nf.event(false, modifiedNoteIDs[i])
		if inboundQueuesProcessedByNotifier {
			if nf.Queue {
				nf.DeleteNote(note.DefaultHubEndpointID, modifiedNoteIDs[i])
			}
		}
	}
	if inboundQueuesProcessedByNotifier {
		if nf.Queue {
			nf.PurgeTombstones(note.DefaultHubEndpointID)
		}
	}

	// Done
	return firstError

}

// AddTracker creates an object that will be used for performing incremental queries
// against the notefile.  Do NOT create trackers that won't actively be used, because
// deletion tombstones will remain in the Notefile for as long as necessary by the
// oldest pending tracker.  In other words, delete trackers when not in use.
func (nf *Notefile) AddTracker(trackerID string) error {

	// Lock for writing
	nfLock.Lock()

	_, present := nf.Trackers[trackerID]
	if present {
		nfLock.Unlock()
		return fmt.Errorf(ErrTrackerExists+" cannot add tracker: tracker already exists: %s", trackerID)
	}

	// Add it as an active tracker.  (Change == 0 means inactive)
	tracker := Tracker{}
	tracker.Change = 1
	nf.Trackers[trackerID] = tracker

	// Done
	nf.modCount++
	nfLock.Unlock()
	return nil

}

// GetTrackers gets a list of the trackers for a given notefile
func (nf *Notefile) GetTrackers() []string {
	trackers := []string{}

	// Lock for reading
	nfLock.RLock()

	// Enum the list
	for k := range nf.Trackers {
		trackers = append(trackers, k)
	}

	// Done
	nfLock.RUnlock()
	return trackers

}

// DeleteTracker deletes an existing tracker
func (nf *Notefile) DeleteTracker(trackerID string) error {

	// Lock for writing
	nfLock.Lock()

	_, present := nf.Trackers[trackerID]
	if !present {
		nfLock.Unlock()
		return fmt.Errorf(ErrTrackerNoExist+" cannot delete tracker: Tracker not found: %s", trackerID)
	}

	delete(nf.Trackers, trackerID)

	// Done
	nf.modCount++
	nfLock.Unlock()
	return nil

}

// ClearTracker clears the change count in a tracker
func (nf *Notefile) ClearTracker(trackerID string) error {

	// Lock for writing
	nfLock.Lock()

	// Clear it if it's present, leaving it active but set so that all notes are returned
	tracker, present := nf.Trackers[trackerID]
	if !present {
		tracker = Tracker{}
	}
	tracker.Change = 1
	nf.Trackers[trackerID] = tracker

	// Done
	nf.modCount++
	nfLock.Unlock()
	return nil

}

// IsTracker returns TRUE if there's a tracker present, else false
func (nf *Notefile) IsTracker(trackerID string) (isTracker bool) {

	// Get the tracker info
	nfLock.RLock()
	_, present := nf.Trackers[trackerID]
	nfLock.RUnlock()

	return present

}

// Swap the session ID so that we may validate it in a test-and-set manner
func (nf *Notefile) swapTrackerSessionID(trackerID string, newSessionID int64) (prevSessionID int64) {

	// Lock for writing, because we'll modify tracker info
	nfLock.Lock()

	// Return 0 for the prev sessionID if it doesn't exist
	tracker, present := nf.Trackers[trackerID]
	if present {
		prevSessionID = tracker.SessionID
	} else {
		tracker = Tracker{}
	}

	// Save the new session ID if it's nonzero
	if newSessionID != 0 {
		tracker.SessionID = newSessionID
		nf.Trackers[trackerID] = tracker
	}
	nf.modCount++

	// Unlock
	nfLock.Unlock()
	return
}

// AreChanges determines whether or not there are any changes pending for this endpoint
func (nf *Notefile) AreChanges(trackerID string) (areChanges bool, err error) {

	// Get the tracker info
	nfLock.RLock()
	tracker, present := nf.Trackers[trackerID]
	nfLock.RUnlock()
	if !present {
		// If there is no tracker, we really need to sync for the first time even if the
		// file is empty and thus nf.Change is 0.
		return true, nil
	}

	// If we're completely up-to-date, we're done
	upToDate := nf.Change == tracker.Change

	// Done
	return !upToDate, nil

}

// Simple insertion sort
func insertionSort(data []int64, a, b int) {
	for i := a + 1; i < b; i++ {
		for j := i; j > a && data[j] < data[j-1]; j-- {
			x := data[j]
			data[j] = data[j-1]
			data[j-1] = x
		}
	}
}

// If a change should be ignored because returning it is a material waste of bandwidth
func changeShouldBeIgnored(note *note.Note, endpointID string) bool {

	// If the histories are blank, assume the endpoint is ommitted because of the way JSON does defaults
	lastUpdaterEndpointID := ""
	if note.Histories != nil && len(*note.Histories) != 0 {
		lastUpdateNote := (*note.Histories)[0]
		lastUpdaterEndpointID = lastUpdateNote.EndpointID
	}
	if lastUpdaterEndpointID != endpointID {
		return false
	}

	// This is a change that should be ignored because it originated at that endpoint
	return true

}

// GetChanges retrieves the next batch of changes being tracked, initializing a new tracker if necessary.
func (nf *Notefile) GetChanges(endpointID string, maxBatchSize int) (chgfile Notefile, numChanges int, totalChanges int, since int64, until int64, err error) {
	fullSearch := false
	debugGetChanges := false
	newNotefile := CreateNotefile(false)

	// There is a special form, only used internally and not exposed at the API, wherein if maxBatchSize
	// is -1 it is an instruction NOT to generate any output but instead just to count.
	countOnly := false
	includeTombstones := true
	if maxBatchSize == -1 {
		maxBatchSize = 0
		countOnly = true
		includeTombstones = false
	}

	// Lock for writing because we will be removing tombstones
	nfLock.Lock()

	// Init return values to include the entire range of notes in the notefile
	since = 1
	until = nf.Change
	fullSearch = true

	// The special endpointID of "ReservedIDDelimiter" means that we do not want to use a tracker.  This
	// feature is internally-used only, and is implemented so that we can get all changed notes without
	// generating a modification to the file that would result if we updated and deleted a temp tracker.
	if endpointID != ReservedIDDelimiter {

		// Bring 'since' up to the minimum change required for this tracker
		tracker, present := nf.Trackers[endpointID]
		if present {

			if tracker.Change > 1 {
				fullSearch = false
			}

		} else {

			// Set the sequence number to 0, so we pick up everything from the beginning
			tracker = Tracker{}
			tracker.Change = since
			nf.Trackers[endpointID] = tracker
			nf.modCount++

		}
		since = tracker.Change

	}

	// If a maximum batch size hasn't been specified, artificially constrain it
	if maxBatchSize == 0 {
		maxBatchSize = defaultMaxGetChangesBatchSize
	}

	// Debug
	if debugGetChanges {
		debugf("GetChanges %s ep:'%s' nf.Change:%d since:%d batch:%d\n", nf.notefileID, endpointID, nf.Change, since, maxBatchSize)
	}

	// Do a pass to compute the since/until that will give us the correct range for the batch.
	// We do this by creating an array that is one larger than we need, and by doing an insertion
	// as the first entry each and every time.  By the time we're done, we will know the best batch
	// to return - even allowing for "holes" in the change numbering because of changes that
	// should be ignored, or changes that are no longer here because of purged tombstones.
	array := make([]int64, maxBatchSize+1)
	for noteID, note := range nf.Notes {
		if note.Change >= since {
			if !fullSearch && changeShouldBeIgnored(&note, endpointID) {
				if debugGetChanges {
					debugf("GetChanges ignoring noteID:%s note.Change:%d\n%v\n", noteID, note.Change, note)
				}
				continue
			}
			if includeTombstones || !isFullTombstone(note) {
				totalChanges++
				if debugGetChanges {
					debugf("GetChanges sorting noteID:%s note.Change:%d totalChanges:%d\n", noteID, note.Change, totalChanges)
				}
				if array[0] == 0 || note.Change < array[maxBatchSize] {
					if array[0] == 0 {
						array[0] = note.Change
					} else {
						array[maxBatchSize] = note.Change
					}
					insertionSort(array, 0, len(array))
				}
			}
		}
	}
	if array[0] == 0 {
		until = nf.Change
	} else {
		until = array[maxBatchSize]
	}
	if debugGetChanges {
		debugf("GetChanges until computed to be %d\n", until)
	}

	// If we're just counting, exit
	if countOnly {
		nfLock.Unlock()
		return newNotefile, 0, totalChanges, since, until, nil
	}

	// Enumerate to see what to return
	for noteID, note := range nf.Notes {
		// If before the base we're tracking, skip it
		if note.Change < since {
			continue
		}
		// If after the max we're allowing for results, skip it
		if note.Change >= until {
			continue
		}
		if !fullSearch && changeShouldBeIgnored(&note, endpointID) {
			if debugGetChanges {
				debugf("GetChanges again skipping noteID:%s note.Change:%d\n%v\n", noteID, note.Change, note)
			}
			continue
		}
		// Add it
		if debugGetChanges {
			debugf("GetChanges adding %s note.Change:%d\n", noteID, note.Change)
		}
		err = newNotefile.uAddNoteEx(noteID, &note)
		if err != nil {
			nfLock.Unlock()
			return newNotefile, 0, 0, 0, 0, err
		}
		numChanges++
		newNotefile.modCount++
	}

	if debugGetChanges {
		debugf("GetChanges done numChanges:%d\n", numChanges)
	}

	// Done
	nfLock.Unlock()
	return newNotefile, numChanges, totalChanges, since, until, nil

}

// PurgeTombstones purges tombstones that are no longer needed by trackers
func (nf *Notefile) PurgeTombstones(localEndpointID string) (err error) {

	// Lock for writing because we will be removing tombstones
	nfLock.Lock()

	// Compute the minimum change number of all tombstones
	minDeletion := nf.Change
	for _, note := range nf.Notes {
		// Track the minimum deletion actually found, for later purge
		if note.Deleted && note.Change < minDeletion {
			minDeletion = note.Change
		}
	}

	// If we don't have any deleted notes, we're done
	if minDeletion >= nf.Change {
		nfLock.Unlock()
		return nil
	}

	// Find the minimum change number of all existing trackers.  Note that we
	// have a special case of tracker.Change == 0 meaning "inactive tracker"
	minTracked := nf.Change
	for trackerID, tracker := range nf.Trackers {
		// Don't hold tombstones for local endpoint; we're only interested in
		// keeping tombstones around for endpoints that are tracking local changes
		if trackerID == localEndpointID {
			continue
		}
		if tracker.Change != 0 && tracker.Change < minTracked {
			minTracked = tracker.Change
		}
	}

	// If there's nothing to purge, we're done
	if minDeletion >= minTracked {
		nfLock.Unlock()
		return nil
	}

	// Purge the tombstones
	for noteID, note := range nf.Notes {
		if note.Deleted && note.Change < minTracked {
			delete(nf.Notes, noteID)
			nf.modCount++
		}
	}

	// Done
	nfLock.Unlock()
	return nil

}

// UpdateChangeTracker updates the tracker and purges tombstones after changes have been committed
func (nf *Notefile) updateChangeTrackerEx(endpointID string, since int64, until int64, purgeTombstones bool) (err error) {

	// Lock for writing because we will be changing trackers
	nfLock.Lock()

	// Get the current base
	baseChangeForTracker := int64(1)
	tracker, present := nf.Trackers[endpointID]
	if present {

		if tracker.Change != 0 {
			baseChangeForTracker = tracker.Change
		}

	} else {

		// Initialize the tracker if it doesn't exist
		tracker = Tracker{}
		tracker.Change = 1

	}

	// Check the tracker.  If the Since has changed, it means that someone has been messing
	// with the tracker.  This is unexpected, and if so we reset the sequence just to force
	// us to review all changes from the beginning.  Otherwise, set it as requested.
	if baseChangeForTracker != since {

		baseChangeForTracker = 1
		tracker.Change = baseChangeForTracker

	} else {

		tracker.Change = until

	}

	// Write the tracker
	nf.Trackers[endpointID] = tracker
	nf.modCount++

	// We're done updating the tracker
	nfLock.Unlock()

	// Now, purge the tombstones
	if purgeTombstones {
		nf.PurgeTombstones(endpointID)
	}

	// Done
	return nil

}

// UpdateChangeTracker updates the tracker and purges tombstones after changes have been committed
func (nf *Notefile) UpdateChangeTracker(endpointID string, since int64, until int64) (err error) {
	return nf.updateChangeTrackerEx(endpointID, since, until, true)
}

// InternalizePayload populates the Payload field inside a set of notes in a notefile from an external source
func (nf *Notefile) InternalizePayload(xp []byte) (err error) {
	xpLen := uint32(len(xp))
	nfLock.Lock()
	for noteID, note := range nf.Notes {
		if note.XPLen == 0 {
			continue
		}
		if note.XPOff+note.XPLen > xpLen {
			err = fmt.Errorf("payload outside %d xp bounds: offset %d len %d", xpLen, note.XPOff, note.XPLen)
			break
		}
		note.Payload = xp[note.XPOff : note.XPOff+note.XPLen]
		note.XPOff = 0
		note.XPLen = 0
		nf.Notes[noteID] = note
	}
	nfLock.Unlock()
	return
}
