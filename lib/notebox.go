// Copyright 2017 Blues Inc.  All rights reserved.
// Use of this source code is governed by licenses granted by the
// copyright holder including that found in the LICENSE file.

// A Notebox is defined to be a Notefile describing a set of
// Notefiles among endpoints that communicate and share info
// with one another.  Generally, an "edge device" endpoint
// has a single Notebox whose ID is unique to that device,
// while a "central service" endpoint may manage a vast
// number of Noteboxes, one per "edge device" it shares
// info and commmunicates with.  In theory, if a group of
// edge devices wanted to communicate peer-to-peer as well
// as with the service, or if a group of services wanted
// to communicate with each other, all these device and
// service endpoints would would share a common notebox.

// A NOTE ABOUT MUTEX ORDERING
// The following mutex ordering should be obeyed:
// 1. boxLock (OUTERMOST)
// 2. box.Openfile().lock (the openfile data structure)
// 3. box.Notefile().lock (the notefile pointed to by the openfile data structure)

// Package notelib notebox.go deals with the handling of the sync'ed container of a set of related notefiles
package notelib

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/blues/note-go/note"
)

// Address of the Noteboxes, indexed by storage object
var openboxes sync.Map

// This ONLY protects the list of open noteboxes
var boxLock sync.RWMutex

// Indicates whether or not we created a goroutine for checkpointing
var autoCheckpointStarted bool
var autoCheckpointStartedLock sync.Mutex

var (
	AutoCheckpointSeconds = 5 * 60
	AutoPurgeSeconds      = 5 * 60
	CheckpointSilently    = false
)

// See if a string is in a string array
func isNoteInNoteArray(val string, array []string) bool {
	for i := range array {
		if array[i] == val {
			return true
		}
	}
	return false
}

// compositeNoteID creates a valid Note ID from the required components
func compositeNoteID(endpoint string, notefile string) string {
	return endpoint + ReservedIDDelimiter + notefile
}

// parseCompositeNoteID extracts componets from a Notebox Note ID
func parseCompositeNoteID(noteid string) (isInstance bool, endpoint string, notefile string) {
	s := strings.Split(noteid, ReservedIDDelimiter)
	// If this isn't a local instance NoteID, return that fact (with an invalid endpoint ID in that field)
	if len(s) != 2 {
		return false, ReservedIDDelimiter, noteid
	}
	return true, s[0], s[1]
}

// Initialize notebox storage
func initNotebox(ctx context.Context, endpointID string, boxStorage string, storage storageClass) (err error) {
	// Create the notefile for the notebox
	boxfile := CreateNotefile(false)

	// Create the note for managing the notebox's notefile, whose ID is the endpoint itself
	body := noteboxBody{}
	// Removed 2017-11-27 because not only is this not needed, but it is also potentially
	// a security issue if it were present and used, because the client could cause the
	// service to fetch the notebox from elsewhere.
	if false {
		body.Notefile.Storage = boxStorage
	}
	note, err := note.CreateNote(noteboxBodyToJSON(body), nil)
	if err == nil {
		_, err = boxfile.AddNote(endpointID, compositeNoteID(endpointID, endpointID), note)
	}
	if err != nil {
		return err
	}

	// Write the storage object.  No lock isneeded because we just created it
	err = storage.writeNotefile(ctx, boxfile, boxStorage, boxStorage)
	if err != nil {
		return err
	}

	if debugBox {
		logDebug("%s created", boxStorage)
	}

	// Done
	return nil
}

// CreateNotebox creates a notebox on this endpoint
func CreateNotebox(ctx context.Context, endpointID string, boxStorage string) (err error) {
	// Get storage from the provided storage object
	storage, err := storageProvider(boxStorage)
	if err != nil {
		return err
	}

	// Create the object at an explicit storage object address
	err = storage.createObject(ctx, boxStorage)
	if err != nil {
		return err
	}

	// Initialize the notebox
	err = initNotebox(ctx, endpointID, boxStorage, storage)
	if err != nil {
		_ = storage.delete(ctx, boxStorage, boxStorage)
		return err
	}

	// Done
	return nil
}

// OpenEndpointNotebox opens - creating if necessary - a notebox in File storage for an endpoint
func OpenEndpointNotebox(ctx context.Context, localEndpointID string, boxLocalStorage string, create bool) (box *Notebox, err error) {
	// Open the notebox
	box, err = OpenNotebox(ctx, localEndpointID, boxLocalStorage)

	// If error, assume that it doesn't exist, so create it and try again
	if err != nil && create {

		err = CreateNotebox(ctx, localEndpointID, boxLocalStorage)
		if err != nil {
			return &Notebox{}, err
		}

		box, err = OpenNotebox(ctx, localEndpointID, boxLocalStorage)
		if err != nil {
			return &Notebox{}, err
		}

	}

	// Done
	return box, err
}

// Delete deletes a closed Notebox, and releases its storage.
func Delete(ctx context.Context, boxStorage string) (err error) {

	// First, checkpoint this notebox, with purge
	box := &Notebox{}
	boxi, present := openboxes.Load(boxStorage)
	if !present {
		return fmt.Errorf("notebox isn't open")
	}
	box.instance = boxi.(*NoteboxInstance)
	box.checkpoint(ctx, true, true, false, false)

	// See if it's still around
	_, present = openboxes.Load(boxStorage)
	if present {
		return fmt.Errorf(note.ErrNotefileInUse+" cannot delete an open notebox: %s", boxStorage)
	}

	// Get the storage class
	storage, err := storageProvider(boxStorage)
	if err != nil {
		return err
	}

	// Delete the storage object
	return storage.delete(ctx, boxStorage, boxStorage)
}

// reopenNotebox reopens it if it's already open
func uReopenNotebox(ctx context.Context, localEndpointID string, boxLocalStorage string) (notebox *Notebox, err error) {

	// Create a new reference to the instance
	boxi, present := openboxes.Load(boxLocalStorage)
	if !present {
		err = fmt.Errorf("notebox isn't open")
		return
	}
	instance := boxi.(*NoteboxInstance)

	// Validate that we're opening with the same endpoint ID, because
	// a notebox can only be open "on behalf of" a single endpoint ID at a time
	// because we use the endpoint on subsequent notebox calls.
	if localEndpointID != instance.endpointID {
		return nil, fmt.Errorf("error opening notebox: notebox can only be opened on behalf of one Endpoint at a time: %s", boxLocalStorage)
	}

	// Increment the refcnt
	iv, present := instance.openfiles.Load(boxLocalStorage)
	if !present {
		return nil, fmt.Errorf("error opening notebox: can't find underlying boxfile")
	}
	boxOpenfile := iv.(*OpenNotefile)
	atomic.AddInt32(&boxOpenfile.openCount, 1)
	if debugBox {
		logDebug("After openbox, %s open refcnt now %d", boxLocalStorage, boxOpenfile.openCount)
	}

	// Done
	box := &Notebox{}
	box.instance = instance
	return box, nil

}

// OpenNotebox opens a local notebox into memory.
func OpenNotebox(ctx context.Context, localEndpointID string, boxLocalStorage string) (notebox *Notebox, err error) {
	// Initialize debugging if we've not done so before
	debugEnvInit()

	// See if it's already open, and bump the refcount if so
	boxLock.Lock()
	_, present := openboxes.Load(boxLocalStorage)
	if present {
		notebox, err = uReopenNotebox(ctx, localEndpointID, boxLocalStorage)
		boxLock.Unlock()
		return
	}
	boxLock.Unlock()

	// Load the notefile for this storage object
	storage, err := storageProvider(boxLocalStorage)
	if err != nil {
		return nil, err
	}
	notefile, err := storage.readNotefile(ctx, boxLocalStorage, boxLocalStorage)
	if err != nil {
		return nil, fmt.Errorf("device not found " + note.ErrDeviceNotFound)
	}

	// For the notebox itself, create a new notefile data structure with a single refcnt
	boxOpenfile := &OpenNotefile{}
	boxOpenfile.notefile = notefile
	boxOpenfile.storage = boxLocalStorage
	boxOpenfile.openCount = 1
	if debugBox {
		logDebug("After openbox, %s open refcnt now %d", boxLocalStorage, boxOpenfile.openCount)
	}

	// Create a new open notebox and add it to the global list
	box := &Notebox{}
	box.instance = &NoteboxInstance{}
	box.instance.endpointID = localEndpointID
	box.instance.storage = boxLocalStorage
	box.instance.openfiles = sync.Map{}
	boxOpenfile.box = box

	// Now that we're ready to store it, see if someone else opened it in the interim.  If
	// so, bump the refcnt and discard what we've done.  If not, we're the ones who opened it.
	boxLock.Lock()
	_, present = openboxes.Load(boxLocalStorage)
	if present {
		notebox, err = uReopenNotebox(ctx, localEndpointID, boxLocalStorage)
		boxLock.Unlock()
		return
	}
	box.instance.openfiles.Store(boxLocalStorage, boxOpenfile)
	openboxes.Store(boxLocalStorage, box.instance)
	boxLock.Unlock()

	// Make sure that we've created a checkpointing task
	if !autoCheckpointStarted {
		autoCheckpointStartedLock.Lock()
		if !autoCheckpointStarted {
			autoCheckpointStarted = true
			go autoCheckpoint(ctx)
			go autoPurge(ctx)
		}
		autoCheckpointStartedLock.Unlock()
	}
	return box, nil
}

// Automatically checkpoint modified noteboxes
func autoCheckpoint(ctx context.Context) {
	for {
		time.Sleep(time.Duration(AutoCheckpointSeconds) * time.Second)
		err := checkpointAllNoteboxes(ctx, false)
		if err != nil {
			logError("autoCheckpoint error: %s", err)
		}
	}
}

// Automatically purge noteboxes/notefiles with zero refcnt that haven't been recently used
func autoPurge(ctx context.Context) {
	for {
		time.Sleep(time.Duration(AutoPurgeSeconds) * time.Second)
		err := checkpointAllNoteboxes(ctx, true)
		if err != nil {
			logError("autoPurge error: %s", err)
		}
	}
}

// Checkpoint all noteboxes and purge noteboxes that have been sitting unused for the autopurge interval
func Checkpoint(ctx context.Context) (err error) {
	return checkpointAllNoteboxes(ctx, true)
}

// CheckpointNoteboxIfNeeded checkpoints a notebox if it's open
func CheckpointNoteboxIfNeeded(ctx context.Context, localEndpointID string, boxLocalStorage string) error {
	// Initialize debugging if we've not done so before
	debugEnvInit()

	var err error
	var notebox *Notebox

	// Reopen the notebox (and bump the refcount) if it's already open
	boxLock.Lock()
	boxi, present := openboxes.Load(boxLocalStorage)
	if present && boxi.(*NoteboxInstance).endpointID == localEndpointID {
		notebox, err = uReopenNotebox(ctx, localEndpointID, boxLocalStorage)
	}
	boxLock.Unlock()

	if notebox == nil || err != nil {
		return err
	}

	err = notebox.Checkpoint(ctx)
	_ = notebox.Close(ctx)
	return err
}

// Purge checkpoints all noteboxes and force a purge of notefiles
func Purge(ctx context.Context) error {
	var firstError error

	// Iterate, checkpointing each open notebox.
	openboxes.Range(func(boxStorage, boxi interface{}) bool {

		box := &Notebox{}
		box.instance = boxi.(*NoteboxInstance)

		emptyBox, err := box.checkpoint(ctx, true, true, true, true)
		if firstError == nil {
			firstError = err
		}

		// If we're purging, get rid of the notefile if it wasn't in use
		if emptyBox {
			openboxes.Delete(boxStorage)
		}

		return true

	})

	// Done
	return firstError
}

// checkpointAllNoteboxes ensures that what's on-disk is up to date
func checkpointAllNoteboxes(ctx context.Context, purge bool) error {
	var firstError error

	// Iterate, checkpointing each open notebox.
	openboxes.Range(func(boxStorage, boxi interface{}) bool {

		box := &Notebox{}
		box.instance = boxi.(*NoteboxInstance)

		emptyBox, err := box.checkpoint(ctx, true, purge, false, false)
		if firstError == nil {
			firstError = err
		}

		// If we're purging, get rid of the notefile if it wasn't in use
		if purge && emptyBox {
			openboxes.Delete(boxStorage)
			if debugBox {
				logDebug("Notebox %s purged", boxStorage)
			}
		}

		return true

	})

	// Done
	return firstError
}

// Checkpoint a notebox
func (box *Notebox) Checkpoint(ctx context.Context) (err error) {
	_, err = box.checkpoint(ctx, true, true, false, false)
	return
}

// uCheckpoint checkpoints an open notebox
func (box *Notebox) checkpoint(ctx context.Context, write bool, purgeClosed bool, purgeClosedForce bool, purgeClosedDeleted bool) (emptyBox bool, err error) {
	var firstError error

	// Count the number of notefiles "in use", including the box itself
	openfiles := 0

	// Iterate, checkpointing each open notefile.
	box.instance.openfiles.Range(func(fileStorage, openfilei interface{}) bool {
		openfile := openfilei.(*OpenNotefile)

		// Checkpoint the notefile, even if it has a 0 refcnt
		var err error
		if write {
			err = box.checkpointNotefile(ctx, openfile)
			if firstError == nil {
				firstError = err
			}
		}

		openfile.sanityCheckOpenCount()

		// If it's closed, purge it in a race-free way
		if openfile.openCount == 0 {
			openfile.lock.Lock()
			if openfile.openCount == 0 {

				// If we're being asked to purge deleted files, do it
				if purgeClosedDeleted && openfile.deleted {
					box.instance.openfiles.Delete(fileStorage)
					if debugBox {
						logDebug("%s purged (had been deleted)", fileStorage)
					}
					openfile.lock.Unlock()
					return true
				}

				// Purge files that haven't been closed recently, just as an optimization that
				// will keep things around in memory that are very frequently used so that we don't
				// thrash flash I/O unnecessarily.
				if purgeClosedForce || (purgeClosed && (time.Since(openfile.closeTime) >= time.Duration(AutoPurgeSeconds)*time.Second)) {
					box.instance.openfiles.Delete(fileStorage)
					if debugBox {
						if openfile.deleted {
							logDebug("%s purged (had been deleted)", fileStorage)
						} else {
							logDebug("%s purged", fileStorage)
						}
					}
					openfile.lock.Unlock()
					return true
				}
			}
			openfile.lock.Unlock()

		}

		// Keep track of the number of notefiles that remain open
		if openfile.openCount != 0 {
			openfiles++
		}

		return true

	})

	// Done
	return openfiles == 0, firstError
}

// Checkpoint an open notefile
func (box *Notebox) checkpointNotefile(ctx context.Context, openfile *OpenNotefile) error {
	storage, err := storageProvider(openfile.storage)
	if err != nil {
		return err
	}

	// Lock this file
	openfile.lock.Lock()
	defer openfile.lock.Unlock()

	// Only checkpoint if modified
	modCount := openfile.notefile.Modified()
	if modCount != openfile.modCountAfterCheckpoint {
		if !CheckpointSilently {
			logDebug("CHECKPOINTED %s (%d mods, %d since first opened)", openfile.storage, modCount-openfile.modCountAfterCheckpoint, modCount)
		}
		// Write storage unless the underlying storage has been deleted during a Notebox merge
		if !openfile.deleted {
			openfile.notefile.lock.Lock()
			err = storage.writeNotefile(ctx, openfile.notefile, box.instance.storage, openfile.storage)
			openfile.notefile.lock.Unlock()
		} else {
			err = nil
		}

		// Set the modification count
		if err == nil {
			openfile.modCountAfterCheckpoint = modCount
		} else {
			logError("Error checkpointing %s: %s", openfile.storage, err)
		}

	}

	return err
}

// Notefiles gets a list of all openable notefiles in the current boxfile
func (box *Notebox) Notefiles(includeTombstones bool) (notefiles []string) {

	// Enum all notes in the boxfile, looking at the global files only
	allNotefiles := []string{}
	boxfile := box.Notefile()
	boxfile.lock.RLock()
	for noteID, note := range boxfile.Notes {
		isInstance, _, notefileID := parseCompositeNoteID(noteID)
		if !isInstance {
			if includeTombstones {
				allNotefiles = append(allNotefiles, notefileID)
			} else {
				if !note.Deleted {
					allNotefiles = append(allNotefiles, notefileID)
				}
			}
		}
	}
	boxfile.lock.RUnlock()

	if debugBox {
		logDebug("All Notefiles:\n%v", allNotefiles)
	}

	return allNotefiles
}

// ClearAllTrackers deletes all trackers for this endpoint in order to force a resync of all files
func (box *Notebox) ClearAllTrackers(ctx context.Context, endpointID string) {
	// Clear the tracker on the notebox itself
	box.Notefile().ClearTracker(endpointID)

	// Get a list of all notefiles including the
	allNotefileIDs := box.Notefiles(false)

	// Iterate over all except the notebox itself
	for i := range allNotefileIDs {
		notefileID := allNotefileIDs[i]
		openfile, file, err := box.OpenNotefile(ctx, notefileID)
		if err == nil {
			file.ClearTracker(endpointID)
			openfile.Close(ctx)
		}
	}
}

// EndpointID gets the notebox's endpoint ID
func (box *Notebox) EndpointID() string {
	return box.instance.endpointID
}

// GetChangedNotefiles determines, for a given tracker, if there are changes in any notebox and,
// if so, for which ones.
func (box *Notebox) GetChangedNotefiles(ctx context.Context, endpointID string) (changedNotefiles []string) {
	// Get the names of all possible openable Notefiles
	allNotefiles := box.Notefiles(true)

	// Add the local endpoint, because that's explicitly not included in the list
	allNotefiles = append(allNotefiles, box.instance.endpointID)

	// Get the notefile info of the "source" and "destination" for the changes
	sourceEndpointID := box.instance.endpointID
	destinationEndpointID := endpointID

	// Enum the notefiles, looking for changes
	changedNotefiles = []string{}
	for i := range allNotefiles {
		notefileID := allNotefiles[i]
		openfile, file, err := box.OpenNotefile(ctx, notefileID)
		if err == nil {
			info, err := box.GetNotefileInfo(notefileID)
			if err != nil {
				info = note.NotefileInfo{}
			}
			suppress := false
			hubEndpointID := info.SyncHubEndpointID
			if info.SyncHubEndpointID == "" {
				hubEndpointID = note.DefaultHubEndpointID
			}
			isQueue, syncToHub, syncFromHub, _, _, _ := NotefileAttributesFromID(notefileID)
			if isQueue && syncToHub {
				suppress = true
			}
			if !syncToHub && hubEndpointID == destinationEndpointID {
				suppress = true
			}
			if !syncFromHub && hubEndpointID == sourceEndpointID {
				suppress = true
			}
			// If the file has never been tracked, we need to mark it as changed regardless
			// of whether or not it is "push" or "pull".
			if !file.IsTracker(endpointID) {
				suppress = false
			}
			// If not suppressing, return true only if the file has been changed
			if !suppress {
				areFileChanges, err := file.AreChanges(endpointID)
				if err == nil && areFileChanges {
					changedNotefiles = append(changedNotefiles, notefileID)
				}
			}
			openfile.Close(ctx)
		}
	}

	// Done
	return changedNotefiles
}

// Close releases a notebox, but leaves it in-memory with a 0 refcount for low-overhead re-open
func (box *Notebox) Close(ctx context.Context) (err error) {

	// Exit if we're about to do something really bad
	boxOpenfile := box.Openfile()
	if boxOpenfile.openCount == 0 {
		return fmt.Errorf("closing notebox: notebox already closed: %s", box.instance.endpointID)
	}

	// If we're on last refcnt, checkpoint to make sure everything is up-to-date.  Otherwise,
	// rely upon the periodic timer-based checkpoint to checkpoint the box.
	if boxOpenfile.openCount == 1 && !autoCheckpointStarted {
		_, err = box.checkpoint(ctx, true, false, false, false)
	}

	// Drop the refcnt on our own storage, re-reading boxOpenFile after the checkpoint.
	atomic.AddInt32(&boxOpenfile.openCount, -1)
	boxOpenfile.sanityCheckOpenCount()
	boxOpenfile.closeTime = time.Now()
	if debugBox {
		logDebug("After closebox, %s close refcnt=%d modcnt=%d", box.instance.storage, boxOpenfile.openCount, boxOpenfile.modCountAfterCheckpoint)
	}

	// Done
	return err
}

// NotefileAttributesFromID extracts attributes implied by the notefileID
func NotefileAttributesFromID(notefileID string) (isQueue bool, syncToHub bool, syncFromHub bool, secure bool, reserved bool, err error) {
	// Preset the error returns for the odd case where we are getting notefile attributes
	// on a notebox, where the notefileid is an endpoint id - which obviously doesn't have an extension
	isQueue = false
	syncToHub = false
	syncFromHub = false
	secure = false
	reserved = false

	// See if it's a reserved filename
	reserved = strings.HasPrefix(notefileID, "_")

	// Look at the extension
	components := strings.Split(notefileID, ".")
	numComponents := len(components)
	if numComponents < 2 {
		err = fmt.Errorf("notefileID must end in a recognized type: %s", notefileID)
		return
	}
	notefileType := components[numComponents-1]
	extension := notefileType

	// First, look for the base type
	validType := false
	if strings.HasPrefix(notefileType, "q") {
		notefileType = strings.TrimPrefix(notefileType, "q")
		isQueue = true
		syncToHub = true
		syncFromHub = true
		validType = true
	} else if strings.HasPrefix(notefileType, "db") {
		notefileType = strings.TrimPrefix(notefileType, "db")
		syncToHub = true
		syncFromHub = true
		validType = true
	}

	// Now, look for I/O attributes
	if strings.HasPrefix(notefileType, "i") {
		notefileType = strings.TrimPrefix(notefileType, "i")
		syncToHub = false
	} else if strings.HasPrefix(notefileType, "o") {
		notefileType = strings.TrimPrefix(notefileType, "o")
		syncFromHub = false
	} else if strings.HasPrefix(notefileType, "x") {
		notefileType = strings.TrimPrefix(notefileType, "x")
		syncFromHub = false
		syncToHub = false
	}

	// Now, look for the security attribute
	if strings.HasPrefix(notefileType, "s") {
		notefileType = strings.TrimPrefix(notefileType, "s")
		secure = true
	}

	// If we haven't gotten a type or exhausted the option flags, it's unrecognized
	if !validType || notefileType != "" {
		err = fmt.Errorf("notefile type not recognized: %s", extension)
	}

	// Done
	return
}

// NotefileExists returns true if notefile exists and is not deleted
func (box *Notebox) NotefileExists(notefileID string) (present bool) {
	boxfile := box.Notefile()
	boxfile.lock.RLock()
	desc, descFound := boxfile.Notes[notefileID]
	xnote, found := boxfile.Notes[compositeNoteID(box.instance.endpointID, notefileID)]
	boxfile.lock.RUnlock()
	if descFound && desc.Deleted {
		descFound = false
	}
	if found && xnote.Deleted {
		found = false
	}
	if notefileID == box.instance.endpointID {
		descFound = true
	}
	present = descFound && found
	return
}

// AddNotefile adds a new notefile to the notebox, and return "nil" if it already exists
func (box *Notebox) AddNotefile(ctx context.Context, notefileID string, notefileInfo *note.NotefileInfo) error {
	// First, do an immediate check to see if it already exists.  If so, short circuit everything
	// and return a clean "no error", which is relied upon by callers.
	if box.NotefileExists(notefileID) {

		// Refresh the notefile info, which may likely have changed
		if notefileInfo == nil {
			notefileInfo = &note.NotefileInfo{}
		}

		return box.SetNotefileInfo(notefileID, *notefileInfo)

	}

	// Find out if it's a queue
	isQueue, _, _, _, _, err := NotefileAttributesFromID(notefileID)
	if err != nil {
		return err
	}

	// Add the notefile
	box.Openfile().lock.Lock()
	boxfile := box.Notefile()
	boxfile.lock.RLock()
	err = box.uAddNotefile(ctx, notefileID, notefileInfo, CreateNotefile(isQueue))
	boxfile.lock.RUnlock()
	box.Openfile().lock.Unlock()

	// Done
	return err
}

// GetNotefileInfo retrieves info about a specific notefile
func (box *Notebox) GetNotefileInfo(notefileID string) (notefileInfo note.NotefileInfo, err error) {
	// If this is for a boxfile, just return null info because there is no notefile info for a boxfile
	if box.instance.endpointID == notefileID {
		return note.NotefileInfo{}, nil
	}

	// Lock open files
	boxfile := box.Notefile()

	// Find the specified Notefile's global descriptor
	xnote, err := boxfile.GetNote(notefileID)
	if err != nil {
		return note.NotefileInfo{}, fmt.Errorf(note.ErrNotefileNoExist+" cannot get info for notefile %s: %s", notefileID, err)
	}

	// Get the info from the notefile body
	body := noteboxBodyFromJSON(xnote.GetBody())
	if body.Notefile.Info == nil {
		notefileInfo = note.NotefileInfo{}
	} else {
		notefileInfo = *body.Notefile.Info
	}

	// Done
	return
}

// SetNotefileInfo sets the info about a notefile that is allowed to be changed after notefile creation
func (box *Notebox) SetNotefileInfo(notefileID string, notefileInfo note.NotefileInfo) (err error) {
	// Now we're going to add things to the boxfile
	boxfile := box.Notefile()

	// Find the specified Notefile's global descriptor
	xnote, err := boxfile.GetNote(notefileID)
	if err != nil {
		return fmt.Errorf(note.ErrNotefileNoExist+" cannot set info for notefile %s: %s", notefileID, err)
	}

	// Get the info from the notefile body
	body := noteboxBodyFromJSON(xnote.GetBody())
	if body.Notefile.Info == nil {
		newInfo := note.NotefileInfo{}
		body.Notefile.Info = &newInfo
	}

	// For now, all NotefileInfo fields can be changed dynamically.  If anything in here is only
	// allowed to be set at Notefile creation, here is where we should enforce that.
	{
	}

	// Update the info
	body.Notefile.Info = &notefileInfo
	_ = xnote.SetBody(noteboxBodyToJSON(body))

	// Update the note
	err = boxfile.UpdateNote(box.instance.endpointID, notefileID, xnote)
	if err != nil {
		return fmt.Errorf(note.ErrNotefileNoExist+" cannot set info for notefile %s: %s", notefileID, err)
	}

	// Update it in-memory if it's open
	boxLock.Lock()
	iv, present := box.instance.openfiles.Load(body.Notefile.Storage)
	if present {
		openfile := iv.(*OpenNotefile)
		openfile.notefile.notefileInfo = notefileInfo
		box.instance.openfiles.Store(body.Notefile.Storage, openfile)
	}
	boxLock.Unlock()

	// Done
	return
}

// CreateNotefile creates a new Notefile in-memory, which will ultimately be added to this Notebox
func (box *Notebox) CreateNotefile(isQueue bool) *Notefile {
	return CreateNotefile(isQueue)
}

// AddNotefile adds a new notefile to the notebox.
// Note, importantly, that this does not open the notefile, and furthermore that because the supplied
// notefile is copied, it should no longer be used.  If one wishes to use the new storage-associated notefile,
// one should open it via the OpenNotefile method on the Notebox.
// NOTE: the boxfile must be locked at least for READ by the caller, and the notefile must be locked for WRITE
func (box *Notebox) uAddNotefile(ctx context.Context, notefileID string, notefileInfo *note.NotefileInfo, notefile *Notefile) error {
	// Reject any attempt to add a notefile whose name contains our special delimiter
	if strings.Contains(notefileID, ReservedIDDelimiter) {
		return fmt.Errorf(note.ErrNotefileName+" name cannot contain reserved character '%s'", ReservedIDDelimiter)
	}

	// Reject notefiles with invalid names
	_, _, _, _, _, err := NotefileAttributesFromID(notefileID)
	if err != nil {
		return err
	}

	// Debug
	if debugBox {
		logDebug("Adding: %s", notefileID)
	}

	// Find the specified notefile descriptor and instance
	boxfile := box.Notefile()
	descnote, descfound := boxfile.Notes[notefileID]
	xnote, found := boxfile.Notes[compositeNoteID(box.instance.endpointID, notefileID)]
	if found && !xnote.Deleted && descfound && !descnote.Deleted {
		return fmt.Errorf(note.ErrNotefileExists+" adding notefile: notefile already exists: %s", notefileID)
	}

	// First, create the global Notefile descriptor or undelete it
	if !descfound {
		body := noteboxBody{}
		body.Notefile.Info = notefileInfo
		descnote, err = note.CreateNote(noteboxBodyToJSON(body), nil)
		if err == nil {
			err = boxfile.uAddNote(box.instance.endpointID, notefileID, &descnote, false)
		}
		if err != nil {
			return err
		}
	} else if descfound && descnote.Deleted {
		descnote.Deleted = false
		body := noteboxBodyFromJSON(descnote.GetBody())
		body.Notefile.Info = notefileInfo
		_ = descnote.SetBody(noteboxBodyToJSON(body))
		err = boxfile.uUpdateNote(box.instance.endpointID, notefileID, &descnote)
		if err != nil {
			return err
		}
		if debugBox {
			logDebug("%s info updated: %v", notefileID, notefileInfo)
		}
	}

	// Inherit storage from the box
	storage, err := storageProvider(box.instance.storage)
	if err != nil {
		return err
	}

	// If not already assigned, create a new storage object
	fileStorage, err := storage.create(ctx, box.instance.storage, notefileID)
	if err != nil {
		return err
	}

	// Create the note for managing the instance.
	body := noteboxBody{}
	body.Notefile.Storage = fileStorage
	if !found {
		var inote note.Note
		inote, err = note.CreateNote(noteboxBodyToJSON(body), nil)
		if err == nil {
			err = boxfile.uAddNote(box.instance.endpointID, compositeNoteID(box.instance.endpointID, notefileID), &inote, false)
		}
	} else {
		_ = xnote.SetBody(noteboxBodyToJSON(body))
		err = boxfile.uUpdateNote(box.instance.endpointID, compositeNoteID(box.instance.endpointID, notefileID), &xnote)
	}
	if err != nil {
		_ = storage.delete(ctx, box.instance.storage, fileStorage)
		return err
	}

	// Write the storage object for the new notefile.  Note that if an error occurs
	// from here on, we need to leave storage around because it's already described
	// by the instance note above.
	err = storage.writeNotefile(ctx, notefile, box.instance.storage, fileStorage)
	if err != nil {
		return err
	}

	// Write the storage object locked, for consistency
	err = storage.writeNotefile(ctx, boxfile, box.instance.storage, box.instance.storage)
	if err != nil {
		return err
	}

	if debugBox {
		logDebug("%s created", fileStorage)
	}

	// Done
	return err
}

// OpenNotefile gets the pointer to the openfile associated with the notebox
func (box *Notebox) Openfile() *OpenNotefile {
	iv, present := box.instance.openfiles.Load(box.instance.storage)
	if !present {
		return &OpenNotefile{}
	}
	return iv.(*OpenNotefile)
}

// Notefile gets the pointer to the notefile associated with the notebox
func (box *Notebox) Notefile() *Notefile {
	return box.Openfile().notefile
}

// ConvertToJSON serializes/marshals the in-memory Notebox into a JSON buffer
func (box *Notebox) ConvertToJSON(fIndent bool) (output []byte, err error) {
	return box.Notefile().ConvertToJSON(fIndent)
}

// OpenNotefile opens a notefile, reading it from storage and bumping its refcount. As such,
// you MUST pair this with a call to CloseNotefile.  Note that this function must work
// for the "" NotefileID (when opening the notebox "|" instance), so don't add a check
// that would prohibit this.
func (box *Notebox) OpenNotefile(ctx context.Context, notefileID string) (iOpenfile *OpenNotefile, notefile *Notefile, err error) {

	// Purge all deleted notefiles from the notebox, because we may be opening one that was deleted
	_, err = box.checkpoint(ctx, false, false, false, true)
	if err != nil {
		logError("checkpoint error: %s", err)
	}

	// Lock the notebox
	box.Openfile().lock.Lock()
	defer box.Openfile().lock.Unlock()

	// Find the specified Notefile and its descriptor, and exit if it doesn't exist
	boxfile := box.Notefile()
	boxfile.lock.RLock()
	desc, descFound := boxfile.Notes[notefileID]
	xnote, found := boxfile.Notes[compositeNoteID(box.instance.endpointID, notefileID)]
	boxfile.lock.RUnlock()
	if descFound && desc.Deleted {
		descFound = false
	}
	if found && xnote.Deleted {
		found = false
	}
	if notefileID == box.instance.endpointID {
		descFound = true
	}
	if !found || !descFound {
		return nil, nil, fmt.Errorf(note.ErrNotefileNoExist+" opening notefile: notefile does not exist: %s", notefileID)
	}

	// See if the notefile is currently open, by using its storage object ID
	notefileBody := noteboxBodyFromJSON(xnote.GetBody())
	filestorage := notefileBody.Notefile.Storage
	iv, present := box.instance.openfiles.Load(filestorage)
	if present {
		openfile := iv.(*OpenNotefile)

		// Increment the refcnt
		atomic.AddInt32(&openfile.openCount, 1)
		if debugBox {
			logDebug("After openfile, %s open refcnt now %d", filestorage, openfile.openCount)
		}

		// Done
		return openfile, openfile.notefile, nil

	}

	// Fetch the storage provider
	storage, err := storageProvider(filestorage)
	if err != nil {
		return nil, nil, err
	}

	// Load the notefile for this storage object.  If for some reason there's an error,
	// log it to the console (because it's corruption or a bug) and attempt recovery by
	// creating a blank notefile.  This may not always be the best thing, but it's better
	// than getting stuck in a mode where we can't open or otherwise manipulate the contents.
	notefile, err = storage.readNotefile(ctx, box.instance.storage, filestorage)
	if err != nil {
		logError("recovering from error reading notefile from %s: %s", filestorage, err)
		notefile = CreateNotefile(false)
	}

	// Set the notefile ID and queue attribute
	notefile.notefileID = notefileID
	var isQueue bool
	isQueue, _, _, _, _, err = NotefileAttributesFromID(notefile.notefileID)
	if err == nil && isQueue {
		notefile.Queue = true
	}

	// Insert other info into needed for the Notifier.  Note that we cannot set the
	// deviceUID or productUID on the client side because they are not always known
	// at the time we do the open.  As such, client-side notifiers will need
	// to function without these values.
	notefile.SetEventInfo(box.defaultEventDeviceUID, box.defaultEventDeviceSN, box.defaultEventProductUID, box.defaultEventAppUID, box.defaultEventFn, box.defaultEventCtx)

	// Copy the notefile info for all but box notefiles
	if notefileID != box.instance.endpointID {
		descBody := noteboxBodyFromJSON(desc.GetBody())
		if descBody.Notefile.Info != nil {
			notefile.notefileInfo = *descBody.Notefile.Info
		}
	}

	// For the notefile itself, create a new notefile data structure with a single refcnt
	openfile := &OpenNotefile{}
	openfile.notefile = notefile
	openfile.modCountAfterCheckpoint = notefile.modCount
	openfile.storage = filestorage
	openfile.box = box
	openfile.openCount = 1
	if debugBox {
		logDebug("After openfile, %s open refcnt now %d", filestorage, openfile.openCount)
	}

	// Add this to the global list of open notefiles for this box
	box.instance.openfiles.Store(filestorage, openfile)

	// Done
	return openfile, notefile, nil
}

// SetClientInfo sets information that is necessary for HTTP client access checking
func (box *Notebox) SetClientInfo(httpReq *http.Request, httpRsp http.ResponseWriter) {
	box.clientHTTPReq = httpReq
	box.clientHTTPRsp = httpRsp
}

// SetEventInfo establishes default information used for change notification on notefiles opened in the box
func (box *Notebox) SetEventInfo(deviceUID string, deviceSN string, productUID string, appUID string, iFn EventFunc, iCtx interface{}) {
	box.defaultEventDeviceUID = deviceUID
	box.defaultEventDeviceSN = deviceSN
	box.defaultEventProductUID = productUID
	box.defaultEventAppUID = appUID
	box.defaultEventFn = iFn
	box.defaultEventCtx = iCtx
}

// GetEventInfo retrieves the info
func (box *Notebox) GetEventInfo() (deviceUID string, deviceSN string, productUID string) {
	return box.defaultEventDeviceUID, box.defaultEventDeviceSN, box.defaultEventAppUID
}

// CheckpointNotefile checkpoints an open Notefile to storage
func (box *Notebox) CheckpointNotefile(ctx context.Context, notefileID string) (err error) {

	// Find the specified Notefile, and exit if it doesn't exist
	boxfile := box.Notefile()
	boxfile.lock.RLock()
	xnote, found := boxfile.Notes[compositeNoteID(box.instance.endpointID, notefileID)]
	boxfile.lock.RUnlock()
	if !found {
		return fmt.Errorf(note.ErrNotefileNoExist+" checkpointing notefile: notefile does not exist: %s", notefileID)
	}

	// See if the notefile is currently open, by using its storage object ID
	notefileBody := noteboxBodyFromJSON(xnote.GetBody())
	iv, present := box.instance.openfiles.Load(notefileBody.Notefile.Storage)
	if !present {
		return fmt.Errorf("checkpointing notefile: notefile isn't currently open: %s", notefileID)
	}
	openfile := iv.(*OpenNotefile)

	// Checkpoint to make sure everything is up-to-date
	err = box.checkpointNotefile(ctx, openfile)

	// Done
	return err
}

// Close closes an open Notefile, decrementing its refcount and making it available
// for purging from memory if this is the last reference.
func (openfile *OpenNotefile) Close(ctx context.Context) (err error) {

	// Defensive coding only
	openfile.sanityCheckOpenCount()
	if openfile.openCount == 0 {
		logError("*** MISMATCHED OpenNotefile/CloseNotefile ***")
		return fmt.Errorf("notefile being closed without a matching open")
	}

	// Drop the refcnt on our own storage
	atomic.AddInt32(&openfile.openCount, -1)
	openfile.closeTime = time.Now()
	if debugBox {
		logDebug("After closefile of %s, close refcnt now %d", openfile.storage, openfile.openCount)
	}

	// Checkpoint to make sure everything is up-to-date
	if !autoCheckpointStarted {
		err = openfile.box.checkpointNotefile(ctx, openfile)
	}

	// Done
	return err
}

func (openfile *OpenNotefile) sanityCheckOpenCount() {
	if openfile.openCount < 0 {
		logError("openCount for %s is negative: %d", openfile.notefile.notefileID, openfile.openCount)
	} else if openfile.openCount > 100 {
		logWarn("openCount for %s is unusually high: %d", openfile.notefile.notefileID, openfile.openCount)
	}
}

// DeleteNotefile deletes a closed Notefile, and releases its storage.
func (box *Notebox) DeleteNotefile(ctx context.Context, notefileID string) (err error) {
	if debugBox {
		logDebug("Deleting %s", notefileID)
	}

	// Lock the notebox
	box.Openfile().lock.Lock()

	// Get the address of the open notebox
	boxfile := box.Notefile()

	// Find the specified Notefile, and exit if it doesn't exist.  It's important that we
	// exit with 'no error' because we may have been here simply because the global instance
	// had existed.
	notefileNoteID := compositeNoteID(box.instance.endpointID, notefileID)
	boxfile.lock.RLock()
	xnote, found := boxfile.Notes[compositeNoteID(box.instance.endpointID, notefileID)]
	boxfile.lock.RUnlock()
	if !found {
		box.Openfile().lock.Unlock()
		_ = boxfile.DeleteNote(box.instance.endpointID, notefileID)
		return nil
	}

	// Don't do storage-related operations if the storage has never yet been allocated
	notefileBody := noteboxBodyFromJSON(xnote.GetBody())
	if notefileBody.Notefile.Storage != "" {

		// See if the notefile is currently open, by using its storage object ID
		iv, present := box.instance.openfiles.Load(notefileBody.Notefile.Storage)
		if present {
			openfile := iv.(*OpenNotefile)
			openfile.lock.Lock()
			currentlyOpen := openfile.openCount != 0
			if !currentlyOpen {
				box.instance.openfiles.Delete(notefileBody.Notefile.Storage)
			}
			openfile.lock.Unlock()
			if currentlyOpen {
				box.Openfile().lock.Unlock()
				if debugBox {
					logDebug("Can't delete: %s is in use", notefileID)
				}
				return fmt.Errorf(note.ErrNotefileInUse+" deleting notefile: notefile is currently open: %s", notefileID)
			}
		}

		// Get the storage class
		storage, err := storageProvider(notefileBody.Notefile.Storage)
		if err != nil {
			box.Openfile().lock.Unlock()
			return err
		}

		// Delete the note used for managing notefiles across all instances,
		// ignoring errors because it may have been deleted by another instance
		_ = boxfile.DeleteNote(box.instance.endpointID, notefileID)

		// Delete the storage object
		_ = storage.delete(ctx, box.instance.storage, notefileBody.Notefile.Storage)

	}

	// It's not open, so we don't need anything further locked
	box.Openfile().lock.Unlock()

	// Delete the note for managing the instance
	_ = boxfile.DeleteNote(box.instance.endpointID, notefileNoteID)

	// Done
	return nil
}

// GetNote gets a note from a notefile
func (box *Notebox) GetNote(ctx context.Context, notefileID string, noteID string) (note note.Note, err error) {
	openfile, file, err := box.OpenNotefile(ctx, notefileID)
	if err != nil {
		return
	}

	note, err = file.GetNote(noteID)

	openfile.Close(ctx)

	return
}

// addNote adds a new note to a notefile but rejects attempts to add more than 100 notes
func (box *Notebox) addNoteLimited(ctx context.Context, endpointID string, notefileID string, noteID string, note note.Note) (err error) {
	payloadMaxBytes := 250
	if note.Payload != nil && len(note.Payload) > payloadMaxBytes {
		return fmt.Errorf("%d-byte payload (maximum of %d bytes allowed)", len(note.Payload), payloadMaxBytes)
	}

	openfile, file, err := box.OpenNotefile(ctx, notefileID)
	if err != nil {
		return err
	}

	notes := file.CountNotes(false)
	notesMax := 100
	if notes >= notesMax {
		openfile.Close(ctx)
		return fmt.Errorf("a maximum of %d notes may be pending for device (currently %d)", notesMax, notes)
	}

	_, err = file.AddNote(endpointID, noteID, note)

	openfile.Close(ctx)

	return err
}

// AddNote adds a new note to a notefile, which is a VERY common operation
func (box *Notebox) AddNote(ctx context.Context, endpointID string, notefileID string, noteID string, note note.Note) (err error) {
	openfile, file, err := box.OpenNotefile(ctx, notefileID)
	if err != nil {
		return err
	}

	_, err = file.AddNote(endpointID, noteID, note)

	openfile.Close(ctx)

	return err
}

// UpdateNote updates an existing note from notefile
func (box *Notebox) UpdateNote(ctx context.Context, endpointID string, notefileID string, noteID string, note note.Note) (err error) {
	openfile, file, err := box.OpenNotefile(ctx, notefileID)
	if err != nil {
		return err
	}

	err = file.UpdateNote(endpointID, noteID, note)

	openfile.Close(ctx)

	return err
}

// DeleteNote deletes an existing note from notefile
func (box *Notebox) DeleteNote(ctx context.Context, endpointID string, notefileID string, noteID string) (err error) {
	openfile, file, err := box.OpenNotefile(ctx, notefileID)
	if err != nil {
		return err
	}

	err = file.DeleteNote(endpointID, noteID)

	openfile.Close(ctx)

	return err
}

// GetChanges retrieves the next batch of changes being tracked
func (box *Notebox) GetChanges(endpointID string, maxBatchSize int) (file *Notefile, numChanges int, totalChanges int, totalNotes int, since int64, until int64, err error) {
	// Get the notefile for this notebox
	boxfile := box.Notefile()

	// Get the tracked changes for that notefile
	notefile, numChanges, totalChanges, totalNotes, since, until, err := boxfile.GetChanges(endpointID, true, maxBatchSize)
	if err != nil {
		return
	}

	// Done
	return notefile, numChanges, totalChanges, totalNotes, since, until, nil
}

// UpdateChangeTracker updates the tracker once changes have been processed
func (box *Notebox) UpdateChangeTracker(endpointID string, since int64, until int64) error {

	// Get the notefile for this notebox
	boxfile := box.Notefile()

	// Get the tracked changes for the notebox's notefile, NEVER purging tombstones.  This
	// behavior is essential so that we can delete and completely re-add noteboxes and be assured
	// that the changes will propagate.
	err := boxfile.updateChangeTrackerEx(endpointID, since, until, false)
	if err != nil {
		return err
	}

	// Done
	return nil
}

// MergeNotebox takes a "changes" Notefile from box.GetChanges, and integrates those
// changes into the current notebox.  NOTE that the "fromBoxFile" should be assumed
// to be modified and then freed by this method, so the caller must not retain any references.
func (box *Notebox) MergeNotebox(ctx context.Context, fromBoxfile *Notefile) (err error) {

	// Get pointers to our notefile
	boxfile := box.Notefile()

	// First, purge anything pertaining to the local instances, so that we don't inadvertently
	// delete notes that are our only pointers to local storage.  We simply don't allow remote
	// endpoints to modify our local instance notes.
	for noteID := range fromBoxfile.Notes {
		isInstance, endpointID, _ := parseCompositeNoteID(noteID)
		if isInstance && endpointID == box.instance.endpointID {
			delete(fromBoxfile.Notes, noteID)
		}
	}

	// Next, merge what's remaining.  This will add and remove remote all remote notefile changes,
	// as well as updating the global notefile notes that contain NotefileInfo
	err = boxfile.MergeNotefile(fromBoxfile)
	if err != nil {
		return err
	}

	// Purge tombstones from this file, since much of what was replicated inward likely
	// had been tombstones, some of which may no longer be needed.
	boxfile.PurgeTombstones(note.DefaultHubEndpointID)

	// If any of the notefile info docs are updated, flush the cached versions of same
	// if it's possible to do so.  We do this by forcibly removing refcnt==0 from memory.
	_, _ = box.checkpoint(ctx, true, false, true, false)

	// Now that this is done, lock the world, both the boxfile and the box's notefile,
	// because in essence this is a custom merge procedure.
	box.Openfile().lock.Lock()
	boxfile.lock.Lock()

	// Now, iterate over all notes in the notefile looking for work to do.  The rules we need to
	// pay attention to are (in this order),
	// 1. If ANY global notefile is deleted, then ALL instances must eventually go away
	// 2. If a global notefile note exists ANYWHERE, it must eventually appear on ALL instances
	// First, classify the entire contents of the notebox
	didSomething := false
	existsLocally := []string{}
	deletedLocally := []string{}
	existsGloballyNoteID := []string{}
	deletedGloballyNoteID := []string{}
	for noteID, note := range boxfile.Notes {
		isInstance, endpointID, notefileID := parseCompositeNoteID(noteID)
		// Skip notebox descriptors
		if endpointID == notefileID {
			continue
		}
		// Skip other endpoints' instances
		if isInstance && endpointID != box.instance.endpointID {
			continue
		}
		// Classify it
		if !isInstance && note.Deleted {
			deletedGloballyNoteID = append(deletedGloballyNoteID, noteID)
		} else if !isInstance && !note.Deleted {
			existsGloballyNoteID = append(existsGloballyNoteID, noteID)
		} else if isInstance && endpointID == box.instance.endpointID && note.Deleted {
			deletedLocally = append(deletedLocally, notefileID)
		} else if isInstance && endpointID == box.instance.endpointID && !note.Deleted {
			existsLocally = append(existsLocally, notefileID)
		}
	}

	if debugBox {
		logDebug("MergeNotebox:\ndeletedGloballyNoteID: %v\nexistsGloballyNoteID: %v\ndeletedLocally: %v\nexistsLocally: %v", deletedGloballyNoteID, existsGloballyNoteID, deletedLocally, existsLocally)
	}

	// 1. For all that are globally deleted, make sure they are deleted locally
	for i := range deletedGloballyNoteID {
		notefileID := deletedGloballyNoteID[i]
		if !isNoteInNoteArray(notefileID, deletedLocally) && isNoteInNoteArray(notefileID, existsLocally) {
			didSomething = true
			deletedLocally = append(deletedLocally, notefileID)

			// Delete the instance note, clearing out the storage for good measure
			noteid := compositeNoteID(box.instance.endpointID, notefileID)
			note := boxfile.Notes[noteid]
			body := noteboxBodyFromJSON(note.GetBody())
			fileStorage := body.Notefile.Storage
			body.Notefile.Storage = ""
			_ = note.SetBody(noteboxBodyToJSON(body))
			_ = boxfile.uDeleteNote(box.instance.endpointID, noteid, &note)

			// Also delete the global descriptor, ignoring errors because it may already be gone
			note, present := boxfile.Notes[notefileID]
			if present {
				_ = boxfile.uDeleteNote(box.instance.endpointID, notefileID, &note)
			}

			// If this is the storage ID of any open notefile, prevent future writes
			iv, present := box.instance.openfiles.Load(fileStorage)
			if present {
				openfile := iv.(*OpenNotefile)
				openfile.deleted = true
			}

			// Delete the local storage
			storage, err := storageProvider(fileStorage)
			if err == nil {
				_ = storage.delete(ctx, box.instance.storage, fileStorage)
			}

		}
	}

	// 2. For all nondeleted that are globally present, make sure they are replicated locally
	for i := range existsGloballyNoteID {
		notefileID := existsGloballyNoteID[i]
		if !isNoteInNoteArray(notefileID, existsLocally) || isNoteInNoteArray(notefileID, deletedLocally) {
			didSomething = true
			existsLocally = append(existsLocally, notefileID)

			// Add a new notefile
			err = box.uAddNotefile(ctx, notefileID, nil, CreateNotefile(false))
			if err != nil {
				logError("NoteboxMerge: can't add notefile %s: %s", notefileID, err)
			}

		}
	}

	// Done with box notefile
	boxfile.lock.Unlock()
	box.Openfile().lock.Unlock()

	// If anything was done, immediately flush the notebox to increase the probability
	// that we don't leave things in a corrupt state if the server unexpectedly terminates
	// Note that this must be called without any locks held
	if didSomething {
		_, err = box.checkpoint(ctx, true, false, false, false)
		if err != nil {
			logError("NoteboxMerge: error checkpointing: %s", err)
		}
	}

	// Done with box descriptor
	return nil
}

// NoteboxAccessFunc is the func to check an access assertion
type NoteboxAccessFunc func(httpReq *http.Request, httpRsp http.ResponseWriter, resource string, actions string) (err error)

var fnNoteboxAccess NoteboxAccessFunc

// HubSetAccessControl sets the global notebox function to check access assertions
func HubSetAccessControl(fn NoteboxAccessFunc) {
	fnNoteboxAccess = fn
}

// VerifyAccess checks an assertion of access
func (box *Notebox) VerifyAccess(resource string, actions string) (err error) {
	if fnNoteboxAccess != nil {
		resource = box.defaultEventAppUID + note.ACResourceSep + box.defaultEventDeviceUID + note.ACResourceSep + resource
		return fnNoteboxAccess(box.clientHTTPReq, box.clientHTTPRsp, resource, actions)
	}
	return nil
}
