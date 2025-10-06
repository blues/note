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
	"context"
	"fmt"
	"math/rand"
	"strings"
	"sync"
	"time"

	"github.com/blues/note-go/note"
	olc "github.com/google/open-location-code/go"
)

// Debug
var (
	debugGetChanges = false
	debugModified   = false
)

// Notefile is The outermost data structure of a Notefile JSON
// object, containing a set of notes that may be synchronized.
type Notefile struct {
	lock            sync.RWMutex         // Relies upon the notefile never being reallocated
	modCount        int                  // Incremented every modification, for checkpointing purposes
	eventFn         EventFunc            // The function to call when eventing of a change
	eventSession    *HubSession          // An argument to be passed to the event call
	eventDeviceUID  string               // The deviceUID being dealt with at the time of the event setup
	eventDeviceSN   string               // The deviceSN being dealt with at the time of the event setup
	eventProductUID string               // The productUID being dealt with at the time of the event setup
	eventAppUID     string               // The appUID being dealt with at the time of the event setup
	eventTransport  string               // The transport being dealt with at the time of the event setup
	notefileID      string               // The NotefileID for the open notefile
	notefileInfo    note.NotefileInfo    // The NotefileInfo for the open notefile
	Queue           bool                 `json:"Q,omitempty"`
	Notes           map[string]note.Note `json:"N,omitempty"`
	Trackers        map[string]Tracker   `json:"T,omitempty"`
	Change          int64                `json:"C,omitempty"`
}

// Tracker is the structure maintained on a per-endpoint basis. When created, the Active flag being false
// indicates that any GetChanges will return ALL notes in the file. Only after the tracking entity
// has received all notes at least once can it then switch into "Optimize" mode. When Optimizing, a tracking
// entity will not receive its own uploaded changes - thus preventing "loopback". (Loopback is not
// harmful per se, but it is suboptimal from a bandwidth perspective.)
type Tracker struct {
	Change    int64 `json:"c,omitempty"`
	SessionID int64 `json:"i,omitempty"`
	Optimize  bool  `json:"o,omitempty"`
}

// defaultMaxGetChangesBatchSize is the default for the max of changes that we'll return in a batch of GetChanges.
const defaultMaxGetChangesBatchSize = 25

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

// Tests to see if a note is deleted, but NOT a "sent pre-deleted" note, knowing
// that we pre-delete notes that are sent.
func isFullTombstone(note note.Note) bool {
	return note.Deleted && !note.Sent
}

// CreateNotefile instantiates and initializes a new Notefile, and prepares it so that
// notes may be added on behalf of the specified endpoint.  The ID of that endpoint
// may be changed at any time and is merely an affordance so that we don't need to
// put the endpointID onto every Note call.
func CreateNotefile(isQueue bool) *Notefile {
	newNotefile := Notefile{}
	newNotefile.Notes = map[string]note.Note{}
	newNotefile.Trackers = map[string]Tracker{}
	newNotefile.Queue = isQueue
	// Change must start at 1 because trackers with Change==0 are "inactive" trackers
	newNotefile.Change = 1
	return &newNotefile
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

// Notes retrieves a duplicate list of all Notes in the notefile
func (nf *Notefile) AllNotes(includeTombstones bool) (notes map[string]note.Note) {
	notes = map[string]note.Note{}
	nf.lock.RLock()
	for noteID, note := range nf.Notes {
		if includeTombstones {
			notes[noteID] = note
		} else {
			if !isFullTombstone(note) {
				notes[noteID] = note
			}
		}
	}
	nf.lock.RUnlock()
	return
}

// NoteIDs retrieves the list of all Note IDs in the notefile
func (nf *Notefile) NoteIDs(includeTombstones bool) (noteIDs []string) {
	noteIDs = []string{}

	nf.lock.RLock()
	for noteID, note := range nf.Notes {
		if includeTombstones {
			noteIDs = append(noteIDs, noteID)
		} else {
			if !isFullTombstone(note) {
				noteIDs = append(noteIDs, noteID)
			}
		}
	}
	nf.lock.RUnlock()

	return
}

// CountNotes returns the count of notes within this notefile
func (nf *Notefile) CountNotes(includeTombstones bool) (count int) {
	nf.lock.RLock()
	if includeTombstones {
		count = len(nf.Notes)
	} else {
		for _, note := range nf.Notes {
			if !isFullTombstone(note) {
				count++
			}
		}
	}
	nf.lock.RUnlock()

	return
}

// Info returns the notefileinfo as it was when this notefile was opened
func (nf *Notefile) Info() (info note.NotefileInfo) {
	return nf.notefileInfo
}

// SetEventInfo supplies information used for change notification
func (nf *Notefile) SetEventInfo(deviceUID string, deviceSN string, productUID string, appUID string, transport string, eventFn EventFunc, eventSession *HubSession) {
	// Lock for writing
	nf.lock.Lock()

	// Lay out the data needed for the call
	nf.eventFn = eventFn
	nf.eventSession = eventSession
	nf.eventDeviceUID = deviceUID
	nf.eventDeviceSN = deviceSN
	nf.eventProductUID = productUID
	nf.eventAppUID = appUID
	nf.eventTransport = transport

	// Unlock & exit
	nf.lock.Unlock()
}

// GetEventInfo Gets information used for change notification
func (nf *Notefile) GetEventInfo() (deviceUID string, deviceSN string, productUID string, appUID string, transport string, eventFn EventFunc, eventSession *HubSession) {

	// Lock for reading
	nf.lock.RLock()

	// Lay out the data needed for the call
	eventFn = nf.eventFn
	eventSession = nf.eventSession
	deviceUID = nf.eventDeviceUID
	deviceSN = nf.eventDeviceSN
	productUID = nf.eventProductUID
	appUID = nf.eventAppUID
	transport = nf.eventTransport

	// Unlock & exit
	nf.lock.RUnlock()
	return
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
	nf.lock.RLock()

	// Generate one
	noteID := nf.uNewNoteID(endpointID)

	// Unlock and exit
	nf.lock.RUnlock()

	return noteID
}

// GetNote retrieves a copy of a note from a Notefile, by ID
func (nf *Notefile) GetNote(noteID string) (xnote note.Note, err error) {
	// Lock for reading
	nf.lock.RLock()

	// Read the note
	_, present := nf.Notes[noteID]
	if !present {
		nf.lock.RUnlock()
		return note.Note{}, fmt.Errorf(note.ErrNoteNoExist+" note not found: %s", noteID)
	}
	xnote = nf.Notes[noteID]

	// Unlock
	nf.lock.RUnlock()
	return xnote, nil
}

// event dispatches to the event proc based on the last change made to NoteID
func (nf *Notefile) event(ctx context.Context, local bool, NoteID string) {
	if nf.eventFn == nil {
		return
	}

	// Fetch the note
	xnote, err := nf.GetNote(NoteID)
	if err != nil {
		// This error is expected for notes that have been deleted from queues
		if ErrorContains(err, note.ErrNoteNoExist) {
			return
		}
		logError(ctx, "event: GetNote(%s) error: %s", NoteID, err)
		return
	}

	// Set up the notification structure.  Note that prior to Oct 2024 we used to
	// add the EndpointID, however the fact that Notehub-generated events failed
	// to add EndpointID (combined with the fact that we don't document what
	// EndpointID even means) means that it's best to not include it in the first place.
	event := note.Event{}
	if xnote.Deleted && !nf.Queue {
		event.Req = note.EventDelete
	} else if xnote.Updates != 0 {
		event.Req = note.EventUpdate
	} else {
		event.Req = note.EventAdd
	}
	event.NoteID = NoteID
	event.Deleted = xnote.Deleted
	event.Sent = xnote.Sent
	event.Bulk = xnote.Bulk
	event.Updates = xnote.Updates
	event.NotefileID = nf.notefileID
	event.DeviceUID = nf.eventDeviceUID
	event.DeviceSN = nf.eventDeviceSN
	event.ProductUID = nf.eventProductUID
	if nf.eventAppUID != "" {
		event.AppUID = nf.eventAppUID
	}
	event.Transport = nf.eventTransport
	event.Payload = xnote.Payload
	if xnote.Body != nil {
		event.Body = &xnote.Body
	}
	if xnote.Histories != nil && len(*xnote.Histories) > 0 {
		histories := *xnote.Histories
		event.When = histories[0].When
		event.Where = histories[0].Where
		event.WhereWhen = histories[0].WhereWhen
		if event.Where != "" {
			area, err := olc.Decode(event.Where)
			if err == nil {
				event.WhereLat, event.WhereLon = area.Center()
			}
		}
	}
	if xnote.Tower != nil {
		if xnote.Tower.OLC != "" {
			area, err := olc.Decode(event.Where)
			if err == nil {
				xnote.Tower.Lat, xnote.Tower.Lon = area.Center()
			}
		}
		event.TowerLat = xnote.Tower.Lat
		event.TowerLon = xnote.Tower.Lon
		event.TowerWhen = xnote.Tower.When
		event.TowerCountry = xnote.Tower.CountryCode
		event.TowerLocation = xnote.Tower.Name
		event.TowerTimeZone = xnote.Tower.TimeZone
	}

	// Clean the event in case it's a queue event
	if event.Sent {
		event.NoteID = ""
		event.Sent = false
		event.Deleted = false
	}

	// Now that the event has been initialized, generate a UID.  Note that if we are
	// generating it locally, generate a totally unique event UID because we know for a
	// fact that the even isn't going to be a duplicate of one generated anywhere else,
	// and otherwise generate a UID that is content-dependent so that if the notecard
	// uploads duplicates they won't be saved.
	if local {
		event.EventUID = GenerateEventUid(nil)
	} else {
		event.EventUID = GenerateEventUid(&event)
	}

	// Debug
	if DebugEvent {
		logString := fmt.Sprintf("event: %s %s %s %s %s", event.Req, event.NotefileID, nf.eventAppUID, event.DeviceUID, event.ProductUID)
		if len(event.Payload) > 0 {
			if event.Bulk {
				logString += fmt.Sprintf(" (%d-byte BULK payload)", len(event.Payload))
			} else {
				logString += fmt.Sprintf(" (%d-byte payload)", len(event.Payload))
			}
		}
		// No need to log customer data body
		logDebug(ctx, "%s", logString)
	}

	// Disable the event function so as to avoid recursion
	savedFn := nf.eventFn
	nf.eventFn = nil

	// Perform the notification (checking savedFn in case of race condition, because everything is unlocked
	if savedFn != nil {
		start := time.Now()
		err = savedFn(ctx, nf.eventSession, local, &event)
		duration := time.Since(start)
		// Since most note operations take a few milliseconds, logging the event should
		// really only take a few milliseconds else the logging will dominate the operation itself,
		// so issue a warning so that we can fix the logging.
		if duration > 5*time.Second {
			typeMsg := "event"
			if local {
				typeMsg = "local " + typeMsg
			}
			if event.Bulk {
				typeMsg = "bulk " + typeMsg
			}
			if nf.eventSession == nil {
				typeMsg = typeMsg + " (no session)"
			}
			message := fmt.Sprintf("%s %s %s [timewarn] warning: %s enqueue took %s", nf.eventAppUID, nf.eventDeviceUID, nf.notefileID, typeMsg, duration)
			if duration > transactionErrorDuration {
				logWarn(ctx, "%s", message)
			} else {
				logInfo(ctx, "%s", message)
			}
		}
		if err != nil {
			logError(ctx, "%s %s %s event notification error: %s", nf.eventAppUID, event.DeviceUID, event.NotefileID, err)
		}
	}

	// Restore the function
	nf.eventFn = savedFn
}

// unlockedAddNoteEx adds a newly-created Note to a Notefile, without modifying any
// fields in the note other than the "Change" field.  The caller is responsible
// for updating modCount.
func (nf *Notefile) uAddNoteEx(noteID string, xnote *note.Note) error {
	// Find it
	_, present := nf.Notes[noteID]
	if present {
		return fmt.Errorf(note.ErrNoteExists+" note already exists: %s", noteID)
	}

	// Make the change
	xnote.Change = nf.Change
	nf.Change++
	nf.Notes[noteID] = *xnote

	// Done
	return nil
}

// Add a note, unlocked
func (nf *Notefile) uAddNote(endpointID string, noteID string, xnote *note.Note, deleted bool) error {
	// Set the required fields
	xnote.Updates = 0
	xnote.Deleted = deleted
	xnote.Conflicts = nil

	// Append the first history
	if xnote.Histories == nil {
		noteHistories := []note.History{}
		noteHistories = append(noteHistories, newHistory(endpointID, 0, "", 0, 0))
		xnote.Histories = &noteHistories
	}

	// Do it
	err := nf.uAddNoteEx(noteID, xnote)

	// Mark modified and exit
	nf.MarkAsModified("uAddNote")
	return err
}

// Mark the notefile as modified
func (nf *Notefile) MarkAsModified(reason string) {
	if debugModified {
		notefileID := nf.notefileID
		if notefileID == "" {
			notefileID = "notefile"
		}
		logDebug(context.Background(), "%s modified: %s", notefileID, reason)
	}
	nf.modCount++
}

// AddNote adds a newly-created Note to a Notefile.  After the note is added to the Notefile,
// it should no longer be used because its contents is copied into the Notefile.  If a unique
// ID for this note isn't supplied, one is generated.  Note that if this is a queue,
// AddNote adds the newly-created note in a pre-deleted state.  This is quite useful
// for notefiles that are being "tracked", because after all trackers receive it, they will
// undelete it and thus it will be present everywhere except the source.  This is an intentional
// asymmetry in synchronization that is primarily useful when sending "to hub" or "from hub".
// By convention, sent notes should be deleted after they have been processed by the recipient(s).
func (nf *Notefile) AddNote(ctx context.Context, endpointID string, noteID string, xnote note.Note) (newNoteID string, err error) {
	xnote.Histories = nil
	return nf.AddNoteWithHistory(ctx, endpointID, noteID, xnote)
}

// Set the 'when' date on a note immediately AFTER it has been added/updated
func (nf *Notefile) SetNoteWhen(noteID string, when int64) (success bool) {

	// Lock for writing
	nf.lock.Lock()

	// Set it
	onote, present := nf.Notes[noteID]
	if present {
		if onote.Histories != nil && len(*onote.Histories) > 0 {
			if (*onote.Histories)[0].When != when {
				(*onote.Histories)[0].When = when
				nf.MarkAsModified("setWhen")
			}
			success = true
		}
	}

	// Unlock
	nf.lock.Unlock()
	return

}

// AddNoteWithHistory is same as AddNote, but it retains history
func (nf *Notefile) AddNoteWithHistory(ctx context.Context, endpointID string, noteID string, xnote note.Note) (newNoteID string, err error) {
	// Create a note ID if not specified
	if nf.Queue || noteID == "" {
		noteID = nf.NewNoteID(endpointID)
	}

	// If the note exists as a deleted note, turn it into an update
	onote, present := nf.Notes[noteID]
	if !nf.Queue && present && onote.Deleted {
		err = nf.UpdateNote(ctx, endpointID, noteID, xnote)
		return noteID, err
	}

	// Lock for writing
	nf.lock.Lock()

	// Add it
	xnote.Sent = nf.Queue
	err = nf.uAddNote(endpointID, noteID, &xnote, nf.Queue)

	// Unlock
	nf.lock.Unlock()

	// Exit if err
	if err != nil {
		return noteID, err
	}

	// Event listeners
	nf.event(ctx, endpointID != note.DefaultDeviceEndpointID, noteID)

	// Done
	return noteID, err
}

// unlockedReplaceNote the contents of a Note in the Notefile with a completely different one
func (nf *Notefile) uReplaceNote(noteID string, xnote *note.Note) error {
	existingNote, present := nf.Notes[noteID]
	if !present {
		return fmt.Errorf("during merge cannot replace note: "+note.ErrNoteNoExist+" note not found: %s", noteID)
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

	nf.MarkAsModified("uReplaceNote")
	return nil
}

// uUpdateNote updates unlocked
func (nf *Notefile) uUpdateNote(endpointID string, noteID string, xnote *note.Note) error {
	_, present := nf.Notes[noteID]
	if !present {
		return fmt.Errorf(note.ErrNoteNoExist+" cannot update: note not found: %s", noteID)
	}

	// Update the history, etc
	updateNote(xnote, endpointID, false, false)

	// Replace it
	return nf.uReplaceNote(noteID, xnote)
}

// UpdateNote updates an existing note within a Notefile, as well
// as updating all its history and conflict metadata as appropriate.
func (nf *Notefile) UpdateNote(ctx context.Context, endpointID string, noteID string, xnote note.Note) error {
	// Exit if trying to update within a queue
	if nf.Queue {
		return fmt.Errorf(note.ErrNotefileQueueDisallowed + " operation not allowed on queue notefiles")
	}

	// Lock for writing
	nf.lock.Lock()

	// Update
	err := nf.uUpdateNote(endpointID, noteID, &xnote)

	// Unlock & exit
	nf.lock.Unlock()

	// Event listeners
	if err == nil {
		nf.event(ctx, endpointID != note.DefaultDeviceEndpointID, noteID)
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
		nf.MarkAsModified("uDeleteNote")
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
func (nf *Notefile) DeleteNote(ctx context.Context, endpointID string, noteID string) error {
	// Lock for writing
	nf.lock.Lock()

	// Read the existing note
	xnote, present := nf.Notes[noteID]
	if !present {
		nf.lock.Unlock()
		return fmt.Errorf(note.ErrNoteNoExist+" cannot delete note: note not found: %s", noteID)
	}

	// Delete it
	err := nf.uDeleteNote(endpointID, noteID, &xnote)

	// Unlock
	nf.lock.Unlock()

	// Event listeners
	if err == nil {
		nf.event(ctx, endpointID != note.DefaultDeviceEndpointID, noteID)
	}

	// Done
	return err
}

// ResolveNoteConflicts resolves all conflicts that are pending for a Note in a Notefile
func (nf *Notefile) ResolveNoteConflicts(endpointID string, noteID string, xnote note.Note) error {
	// Lock for writing
	nf.lock.Lock()

	_, present := nf.Notes[noteID]
	if !present {
		nf.lock.Unlock()
		return fmt.Errorf(note.ErrNoteNoExist+" cannot resolve conflicts: note not found: %s", noteID)
	}

	updateNote(&xnote, endpointID, true, false)
	xnote.Change = nf.Change
	nf.Change++
	nf.Notes[noteID] = xnote

	// Done
	nf.MarkAsModified("ResolveNoteConflicts")
	nf.lock.Unlock()
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
			NeedToMerge := (ConflictDataDiffers) ||
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
func (nf *Notefile) MergeNotefile(ctx context.Context, fromNotefile *Notefile) error {
	// Lock for writing
	nf.lock.Lock()

	// Perform the merge
	var firstError error
	modifiedNoteIDs := []string{}
	mergeNoteChangeList, mergeNoteIDChangeList := nf.uGetMergeNoteChangeList(fromNotefile)
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
			nf.MarkAsModified("MergeNotefile:uAddNoteEx")
		} else {
			thisError = nf.uReplaceNote(idMergeNote, &mergeNote)
			nf.MarkAsModified("MergeNotefile:uReplaceNote")
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
	nf.lock.Unlock()

	// We've now got a list of successfully added or merged notes.  Now that everything is unlocked,
	// go through this list and event the server of what has transpired.  Also, if the destination is
	// an inbound queue, delete the contents after modification because this is the core essential
	// behavior of queues
	for i := range modifiedNoteIDs {
		nf.event(ctx, false, modifiedNoteIDs[i])
		if inboundQueuesProcessedByNotifier {
			if nf.Queue {
				_ = nf.DeleteNote(ctx, note.DefaultHubEndpointID, modifiedNoteIDs[i])
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
	nf.lock.Lock()

	_, present := nf.Trackers[trackerID]
	if present {
		nf.lock.Unlock()
		return fmt.Errorf(note.ErrTrackerExists+" cannot add tracker: tracker already exists: %s", trackerID)
	}

	// Add it as an active tracker.  (Change == 0 means inactive)
	tracker := Tracker{}
	tracker.Change = 1
	nf.Trackers[trackerID] = tracker

	// Done
	nf.MarkAsModified("AddTracker")
	nf.lock.Unlock()
	return nil
}

// GetTrackers gets a list of the trackers for a given notefile
func (nf *Notefile) GetTrackers() []string {
	trackers := []string{}

	// Lock for reading
	nf.lock.RLock()

	// Enum the list
	for k := range nf.Trackers {
		trackers = append(trackers, k)
	}

	// Done
	nf.lock.RUnlock()
	return trackers
}

// DeleteTracker deletes an existing tracker
func (nf *Notefile) DeleteTracker(trackerID string) error {
	// Lock for writing
	nf.lock.Lock()

	_, present := nf.Trackers[trackerID]
	if !present {
		nf.lock.Unlock()
		return fmt.Errorf(note.ErrTrackerNoExist+" cannot delete tracker: Tracker not found: %s", trackerID)
	}

	delete(nf.Trackers, trackerID)

	// Done
	nf.MarkAsModified("DeleteTracker")
	nf.lock.Unlock()
	return nil
}

// ClearTracker clears the change count in a tracker
func (nf *Notefile) ClearTracker(trackerID string) {
	// Lock for writing
	nf.lock.Lock()

	// Clear it if it's present, leaving it active but set so that all notes are returned
	tracker, present := nf.Trackers[trackerID]
	if !present {
		tracker = Tracker{}
	}
	tracker.Change = 1
	nf.Trackers[trackerID] = tracker

	// Done
	nf.MarkAsModified("ClearTracker")
	nf.lock.Unlock()
}

// IsTracker returns TRUE if there's a tracker present, else false
func (nf *Notefile) IsTracker(trackerID string) (isTracker bool) {
	// Get the tracker info
	nf.lock.RLock()
	_, present := nf.Trackers[trackerID]
	nf.lock.RUnlock()

	return present
}

// Swap the session ID so that we may validate it in a test-and-set manner
func (nf *Notefile) swapTrackerSessionID(trackerID string, newSessionID int64) (prevSessionID int64) {
	// Lock for writing, because we'll modify tracker info
	nf.lock.Lock()

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
	nf.MarkAsModified("swapTrackerSessionID")

	// Unlock
	nf.lock.Unlock()
	return
}

// AreChanges determines whether or not there are any changes pending for this endpoint
func (nf *Notefile) AreChanges(trackerID string) (areChanges bool, err error) {
	// Get the tracker info
	nf.lock.RLock()
	tracker, present := nf.Trackers[trackerID]
	nf.lock.RUnlock()
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

// NotefileIDIsReserved returns true if the notefileID is reserved
func NotefileIDIsReserved(notefileID string) bool {
	// This character is blocked from being in a notefile name because we use it as a separator
	// within the notebox
	if strings.Contains(notefileID, ReservedIDDelimiter) {
		return true
	}

	// Underscore is our primary reserved character
	return strings.HasPrefix(notefileID, "_")
}

// NotefileIDIsReservedWithExceptions returns true if the notefileID is reserved
func NotefileIDIsReservedWithExceptions(notefileID string) bool {
	// This character is blocked from being in a notefile name because we use it as a separator
	// within the notebox
	if strings.Contains(notefileID, ReservedIDDelimiter) {
		return true
	}

	// Certain notefiles are explicitly allowed because we expect users to be messing inside them
	if notefileID == note.TrackNotefile ||
		notefileID == note.LogNotefile ||
		notefileID == note.HealthNotefile ||
		notefileID == note.HealthHostNotefile ||
		notefileID == note.NotecardRequestNotefile ||
		notefileID == note.NotecardResponseNotefile {
		return false
	}

	// Underscore is our primary reserved character
	return strings.HasPrefix(notefileID, "_")
}

// GetChanges retrieves the next batch of changes being tracked, initializing a new tracker if necessary.
func (nf *Notefile) GetChanges(endpointID string, includeTombstones bool, maxBatchSize int) (chgfile *Notefile, numChanges int, totalChanges int, totalNotes int, since int64, until int64, err error) {
	newNotefile := CreateNotefile(false)

	// The special endpointID of "ReservedIDDelimiter" means that we do not want to use a tracker.  This
	// feature is internally-used only, and is implemented so that we can get all changed notes without
	// generating a modification to the file that would result if we updated and deleted a temp tracker.
	usingTracker := endpointID != ReservedIDDelimiter

	// There is a special form, only used internally and not exposed at the API, wherein if maxBatchSize
	// is -1 it is an instruction NOT to generate any output but instead just to count.
	countOnly := false
	if maxBatchSize == -1 {
		maxBatchSize = 0
		countOnly = true
		includeTombstones = false
	}

	// Lock for writing because we will be removing tombstones
	nf.lock.Lock()

	// Init return values to include the entire range of notes in the notefile
	since = 1

	// The "optimize" flag indicates whether we have gone through at least one full pass of returning
	// changes to a caller.  Until doing one full pass, we cannot be optimal.
	optimize := false

	// Initialize a new tracker on first use
	if usingTracker {

		// Bring 'since' up to the minimum change required for this tracker
		tracker, present := nf.Trackers[endpointID]
		if present {

			// Anytime  we are searching from the beginning, start without optimization
			if tracker.Change <= 1 && tracker.Optimize {
				tracker.Optimize = false
				nf.Trackers[endpointID] = tracker
				nf.MarkAsModified("GetChanges:ALL-TRACKER")
			}

			// Remember whether or not we are optimizing
			optimize = tracker.Optimize

		} else {

			// Set the sequence number to 1, so we pick up everything from the beginning
			tracker = Tracker{}
			tracker.Change = 1
			tracker.Optimize = false
			nf.Trackers[endpointID] = tracker
			nf.MarkAsModified("GetChanges:ALL")

		}

		// The first change sequence number to be returned, inclusive of this number
		since = tracker.Change

	}

	// If a maximum batch size hasn't been specified, artificially constrain it
	if maxBatchSize == 0 {
		maxBatchSize = defaultMaxGetChangesBatchSize
	}

	// Debug
	if debugGetChanges {
		logDebug(context.Background(), "GetChanges %s ep:'%s' nf.Change:%d since:%d batch:%d optimize:%t", nf.notefileID, endpointID, nf.Change, since, maxBatchSize, optimize)
	}

	// Do a pass to compute the since/until that will give us the correct range for the batch.
	// We do this by creating an array that is one larger than we need, and by doing an insertion
	// as the first entry each and every time.  By the time we're done, we will know the best batch
	// to return - even allowing for "holes" in the change numbering because of changes that
	// should be ignored, or changes that are no longer here because of purged tombstones.
	array := make([]int64, maxBatchSize+1)
	for noteID, note := range nf.Notes {
		if debugGetChanges {
			logDebug(context.Background(), "note noteID:%s Change:%d Deleted:%t Sent:%t shouldIgnore:%t, isFullTombstone:%t",
				noteID, note.Change, note.Deleted, note.Sent, changeShouldBeIgnored(&note, endpointID), isFullTombstone(note))
		}
		if !isFullTombstone(note) {
			totalNotes++
		}
		if note.Change >= since {
			if optimize && changeShouldBeIgnored(&note, endpointID) {
				if debugGetChanges {
					logDebug(context.Background(), "GetChanges ignoring noteID:%s note.Change:%d\n%v", noteID, note.Change, note)
				}
				continue
			}
			if includeTombstones || !isFullTombstone(note) {
				totalChanges++
				if debugGetChanges {
					logDebug(context.Background(), "GetChanges sorting noteID:%s note.Change:%d totalChanges:%d", noteID, note.Change, totalChanges)
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
		logDebug(context.Background(), "GetChanges until computed to be %d", until)
	}

	// If we're just counting, exit
	if countOnly {
		nf.lock.Unlock()
		return newNotefile, 0, totalChanges, totalNotes, since, until, nil
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
		if optimize && changeShouldBeIgnored(&note, endpointID) {
			if debugGetChanges {
				logDebug(context.Background(), "GetChanges again skipping noteID:%s note.Change:%d\n%v", noteID, note.Change, note)
			}
			continue
		}
		// If not including tombstones, skip it
		if !(includeTombstones || !isFullTombstone(note)) {
			continue
		}
		// We're now committed to the change.  Add it, and bump the change count
		if debugGetChanges {
			logDebug(context.Background(), "GetChanges adding %s note.Change:%d", noteID, note.Change)
		}
		err = newNotefile.uAddNoteEx(noteID, &note)
		if err != nil {
			nf.lock.Unlock()
			return newNotefile, 0, 0, 0, 0, 0, err
		}
		numChanges++
		newNotefile.modCount++
	}

	if debugGetChanges {
		logDebug(context.Background(), "GetChanges done numChanges:%d", numChanges)
	}

	// If the number of changes returned is 0, it means that future searches for this endpoint
	// can be optimized so that we don't "loopback" changes from that endpoint.  (Note that the
	// tracker "present" check below is just defensive coding.)
	if usingTracker && numChanges == 0 {
		tracker, present := nf.Trackers[endpointID]
		if present && !tracker.Optimize {
			tracker.Optimize = true
			nf.Trackers[endpointID] = tracker
			nf.MarkAsModified("GetChanges:None")
			if debugGetChanges {
				logDebug(context.Background(), "GetChanges optimization enabled for future calls")
			}
		}
	}

	// Done
	nf.lock.Unlock()
	return newNotefile, numChanges, totalChanges, totalNotes, since, until, nil
}

// PurgeTombstones purges tombstones that are no longer needed by trackers
func (nf *Notefile) PurgeTombstones(ignoreEndpointID string) {
	// Lock for writing because we will be removing tombstones
	nf.lock.Lock()

	if debugGetChanges {
		logDebug(context.Background(), "purgeTombstones: %s", nf.notefileID)
	}

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
		if debugGetChanges {
			logDebug(context.Background(), "purgeTombstones: no deleted notes")
		}
		nf.lock.Unlock()
		return
	}

	// Find the minimum change number of all existing trackers.  Note that we
	// have a special case of tracker.Change == 0 meaning "inactive tracker"
	minTracked := nf.Change
	for trackerID, tracker := range nf.Trackers {
		// Don't hold tombstones for the specified (local) endpoint; we're only interested in
		// keeping tombstones around for remote endpoints that are tracking local changes
		if trackerID == ignoreEndpointID || ignoreEndpointID == "*" {
			continue
		}
		if tracker.Change != 0 && tracker.Change < minTracked {
			minTracked = tracker.Change
		}
	}

	// If there's nothing to purge, we're done
	if minDeletion >= minTracked {
		if debugGetChanges {
			logDebug(context.Background(), "purgeTombstones: nothing to purge")
		}
		nf.lock.Unlock()
		return
	}

	if debugGetChanges {
		logDebug(context.Background(), "purgeTombstones: purging with minTracked:%d", minTracked)
	}

	// Purge the tombstones
	for noteID, note := range nf.Notes {
		if note.Deleted && note.Change < minTracked {
			if debugGetChanges {
				logDebug(context.Background(), "purgeTombstones: deleting noteID:%s Sent:%t Deleted:%t Change:%d", noteID, note.Sent, note.Deleted, note.Change)
			}
			delete(nf.Notes, noteID)
			nf.MarkAsModified("GetChanges:PurgeTombstones")
		}
	}

	// Done
	nf.lock.Unlock()
}

// UpdateChangeTracker updates the tracker and purges tombstones after changes have been committed
func (nf *Notefile) updateChangeTrackerEx(endpointID string, since int64, until int64, purgeTombstones bool) (err error) {
	// Lock for writing because we will be changing trackers
	nf.lock.Lock()

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
	nf.MarkAsModified("updateChangeTrackerEx")

	// We're done updating the tracker
	nf.lock.Unlock()

	// Now, purge the tombstones, given that this endpoint ID no longer cares about them
	if purgeTombstones {
		nf.PurgeTombstones(note.DefaultHubEndpointID)
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
	nf.lock.Lock()
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
	nf.lock.Unlock()
	return
}

// GetLeastRecentNoteID enumerates and returns the first logical note to dequeue
func (nf *Notefile) GetLeastRecentNoteID() (noteID string, err error) {
	// Compute the minimum change number of all notes
	var minChangeNoteID, minModifiedNoteID string
	var minModified int64
	minChange := nf.Change
	nf.lock.RLock()
	for noteID, xnote := range nf.Notes {
		if xnote.Deleted {
			continue
		}
		// Track the minimum deletion actually found, for later purge
		if xnote.Change < minChange {
			minChange = xnote.Change
			minChangeNoteID = noteID
		}
		// Track the minimum modified date, only believing "real" unix dates
		var modified int64
		if xnote.Histories != nil && len(*xnote.Histories) > 0 {
			histories := *xnote.Histories
			modified = histories[0].When
		}
		if modified > 1483228800 && (modified == 0 || modified < minModified) {
			minModified = modified
			minModifiedNoteID = noteID
		}
	}
	nf.lock.RUnlock()

	// Always prefer to order things based on unix modified dates
	if minModified != 0 {
		noteID = minModifiedNoteID
		return
	}

	// Otherwise, use the minimum by change
	if minChange < nf.Change {
		noteID = minChangeNoteID
		return
	}

	// No notes found
	err = fmt.Errorf("no notes available in queue " + note.ErrNoteNoExist)
	return
}

// GetMostRecentNote enumerates and returns the most recent note in the notefile.  If
// there are notes in the notefile but all their when's are zero, an arbitrary note
// will be returned.
func (nf *Notefile) GetMostRecentNoteID() (mostRecentNoteID string, mostRecentWhen int64, err error) {
	nf.lock.RLock()
	for noteID, xnote := range nf.Notes {
		if xnote.Deleted {
			continue
		}
		when := xnote.When()
		if mostRecentNoteID == "" || when > mostRecentWhen {
			mostRecentWhen = when
			mostRecentNoteID = noteID
		}
	}
	nf.lock.RUnlock()
	if mostRecentNoteID == "" {
		err = fmt.Errorf("no notes available in queue " + note.ErrNoteNoExist)
	}
	return
}

// ConvertToJSON serializes/marshals the in-memory Notefile into a JSON buffer
func (nf *Notefile) uConvertToJSON(indent bool) (output []byte, err error) {
	if indent {
		output, err = note.JSONMarshalIndent(nf, "", "    ")
	} else {
		output, err = note.JSONMarshal(nf)
	}
	return
}

// ConvertToJSON serializes/marshals the in-memory Notefile into a JSON buffer
func (nf *Notefile) ConvertToJSON(indent bool) (output []byte, err error) {
	nf.lock.RLock()
	output, err = nf.uConvertToJSON(indent)
	nf.lock.RUnlock()
	return
}
