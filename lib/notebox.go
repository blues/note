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

// Package notelib notebox.go deals with the handling of the sync'ed container of a set of related notefiles
package notelib

import (
	"fmt"
	"github.com/blues/note-go/note"
	"strings"
	"sync"
	"time"
)

// Address of the Noteboxes, indexed by storage object
var openboxes = map[string]*Notebox{}

// For now, we use just a single mutex to protect all operations
// even across multiple noteboxes. If we were to protect each notebox
// with its own mutex, and were to use atomic operations for refcnts,
// we would be unable to use Maps because that data structure is
// free to move, and range operators do not allow access to the
// address of a specific map entry - only its value.
// The list of open noteboxes
var boxLock sync.RWMutex

// Indicates whether or not we created a goroutine for checkpointing
var autoCheckpointStarted bool

const autoCheckpointMinutes = 1
const autoPurgeMinutes = 5

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
func initNotebox(endpointID string, boxStorage string, storage storageClass) (err error) {

	// Create the notefile for the notebox
	boxfile := CreateNotefile(false)

	// Create the note for managing the notebox's notefile, whose ID is the endpoint itself
	body := noteboxBody{}
	//Removed 2017-11-27 because not only is this not needed, but it is also potentially
	// a security issue if it were present and used, because the client could cause the
	// service to fetch the notebox from elsewhere.
	if false {
		body.Notefile.Storage = boxStorage
	}
	note, err := note.CreateNote(noteboxBodyToJSON(body), nil)
	if err == nil {
		err = boxfile.AddNote(endpointID, compositeNoteID(endpointID, endpointID), note)
	}
	if err != nil {
		return err
	}

	// Write the storage object.  No lock isneeded because we just created it
	err = storage.writeNotefile(&boxfile, boxStorage, boxStorage)
	if err != nil {
		return err
	}

	if debugBox {
		debugf("%s created\n", boxStorage)
	}

	// Done
	return nil

}

// CreateNotebox creates a notebox on this endpoint
func CreateNotebox(endpointID string, boxStorage string) (err error) {

	// Get storage from the provided storage object
	storage, err := storageProvider(boxStorage)
	if err != nil {
		return err
	}

	// Create the object at an explicit storage object address
	err = storage.createObject(boxStorage)
	if err != nil {
		return err
	}

	// Initialize the notebox
	err = initNotebox(endpointID, boxStorage, storage)
	if err != nil {
		storage.delete(boxStorage, boxStorage)
		return err
	}

	// Done
	return nil

}

// OpenEndpointNotebox opens - creating if necessary - a notebox in File storage for an endpoint
func OpenEndpointNotebox(localEndpointID string, boxLocalStorage string, create bool) (box *Notebox, err error) {

	// Open the notebox
	box, err = OpenNotebox(localEndpointID, boxLocalStorage)

	// If error, assume that it doesn't exist, so create it and try again
	if err != nil && create {

		err = CreateNotebox(localEndpointID, boxLocalStorage)
		if err != nil {
			return &Notebox{}, err
		}

		box, err = OpenNotebox(localEndpointID, boxLocalStorage)
		if err != nil {
			return &Notebox{}, err
		}

	}

	// Done
	return box, err

}

// Delete deletes a closed Notebox, and releases its storage.
func Delete(boxStorage string) (err error) {

	// Lock the world
	boxLock.Lock()

	// First, checkpoint all noteboxes, with purge
	err = uCheckpointAllNoteboxes(true)
	if err != nil {
		debugf("Checkpoint error on delete: %s\n", err)
	}

	// See if it's still around
	_, present := openboxes[boxStorage]
	if present {
		boxLock.Unlock()
		return fmt.Errorf(ErrNotefileInUse+" cannot delete an open notebox: %s", boxStorage)
	}

	// Get the storage class
	storage, err := storageProvider(boxStorage)
	if err != nil {
		boxLock.Unlock()
		return err
	}

	// Delete the storage object
	storage.delete(boxStorage, boxStorage)

	// Done
	boxLock.Unlock()
	return nil

}

// OpenNotebox opens a local notebox into memory.
func OpenNotebox(localEndpointID string, boxLocalStorage string) (notebox *Notebox, err error) {

	// Initialize debugging if we've not done so before
	debugInit()

	// Lock the world
	boxLock.Lock()

	// See if it's present in the list
	box, present := openboxes[boxLocalStorage]

	// If so, increment the open count on the notebox's notefile and exit
	if present {

		// Validate that we're opening with the same endpoint ID, because
		// a notebox can only be open "on behalf of" a single endpoint ID at a time
		// because we use the endpoint on subsequent notebox calls.
		if localEndpointID != box.endpointID {
			boxLock.Unlock()
			return nil, fmt.Errorf("error opening notebox: notebox can only be opened on behalf of one Endpoint at a time: %s", boxLocalStorage)
		}

		// Increment the refcnt
		boxOpenfile := box.openfiles[boxLocalStorage]
		boxOpenfile.openCount++
		box.openfiles[boxLocalStorage] = boxOpenfile
		if debugBox {
			debugf("After openbox, %s open refcnt now %d\n", boxLocalStorage, boxOpenfile.openCount)
		}

		boxLock.Unlock()
		return box, nil

	}

	// Fetch the storage provider
	storage, err := storageProvider(boxLocalStorage)
	if err != nil {
		boxLock.Unlock()
		return nil, err
	}

	// Load the notefile for this storage object
	notefile, err := storage.readNotefile(boxLocalStorage, boxLocalStorage)
	if err != nil {
		boxLock.Unlock()
		return nil, fmt.Errorf("device not found " + ErrDeviceNotFound)
	}

	// For the notebox itself, create a new notefile data structure with a single refcnt
	boxOpenfile := OpenNotefile{}
	boxOpenfile.notefile = notefile
	boxOpenfile.storage = boxLocalStorage
	boxOpenfile.openCount = 1
	if debugBox {
		debugf("After openbox, %s open refcnt now %d\n", boxLocalStorage, boxOpenfile.openCount)
	}

	// Create a new open notebox and add it to the global list
	box = &Notebox{}
	box.endpointID = localEndpointID
	box.storage = boxLocalStorage
	box.openfiles = map[string]OpenNotefile{}
	boxOpenfile.box = box
	box.openfiles[boxLocalStorage] = boxOpenfile
	openboxes[boxLocalStorage] = box

	// Unlock
	boxLock.Unlock()

	// Make sure that we've created a checkpointing task
	if !autoCheckpointStarted {
		autoCheckpointStarted = true
		go autoCheckpoint()
		go autoPurge()
	}

	return box, nil

}

// Automatically checkpoint modified noteboxes
func autoCheckpoint() {
	for {
		time.Sleep(autoCheckpointMinutes * time.Minute)
		err := checkpointAllNoteboxes(false)
		if err != nil {
			debugf("autoCheckpoint error: %s\n", err)
		}
	}
}

// Automatically purge noteboxes/notefiles with zero refcnt that haven't been recently used
func autoPurge() {
	for {
		time.Sleep(autoPurgeMinutes * time.Minute)
		err := checkpointAllNoteboxes(true)
		if err != nil {
			debugf("autoPurge error: %s\n", err)
		}
	}
}

// Checkpoint all noteboxes and purge closed noteboxes
func Checkpoint() (err error) {
	return checkpointAllNoteboxes(true)
}

// checkpointAllNoteboxes ensures that what's on-disk is up to date
func checkpointAllNoteboxes(purge bool) error {

	// Lock the world
	boxLock.Lock()

	// Do the checkpoint
	err := uCheckpointAllNoteboxes(purge)

	// Done
	boxLock.Unlock()
	return err
}

// checkpointAllNoteboxes ensures that what's on-disk is up to date
func uCheckpointAllNoteboxes(purge bool) error {
	var firstError error

	// Iterate, checkpointing each open notebox.
	for boxStorage, box := range openboxes {

		emptyBox, err := box.uCheckpoint(purge, false, false)
		if firstError == nil {
			firstError = err
		}

		// If we're purging, get rid of the notefile if it wasn't in use
		if purge && emptyBox {
			delete(openboxes, boxStorage)
			if debugBox {
				debugf("Notebox %s purged\n", boxStorage)
			}
		}

	}

	// Done
	return firstError

}

// uCheckpoint checkpoints an open notebox
func (box *Notebox) uCheckpoint(purgeClosed bool, purgeClosedForce bool, purgeClosedDeleted bool) (emptyBox bool, err error) {
	var firstError error

	// Count the number of notefiles "in use", including the box itself
	openfiles := 0

	// Iterate, checkpointing each open notefile.
	for fileStorage, openfile := range box.openfiles {

		// Checkpoint the notefile, even if it has a 0 refcnt
		err := box.uCheckpointNotefile(&openfile)
		if firstError == nil {
			firstError = err
		}

		// If it's closed, potentially purge it
		if openfile.openCount == 0 {

			// If we're being asked to purge deleted files, do it
			if purgeClosedDeleted && openfile.deleted {
				delete(box.openfiles, fileStorage)
				if debugBox {
					debugf("%s purged (had been deleted)\n", fileStorage)
				}
				continue
			}

			// Purge files that haven't been closed recently, just as an optimization that
			// will keep things around in memory that are very frequently used so that we don't
			// thrash flash I/O unnecessarily.
			if purgeClosedForce || (purgeClosed && (time.Since(openfile.closeTime) >= time.Duration(autoPurgeMinutes)*time.Minute)) {
				delete(box.openfiles, fileStorage)
				if debugBox {
					if openfile.deleted {
						debugf("%s purged (had been deleted)\n", fileStorage)
					} else {
						debugf("%s purged\n", fileStorage)
					}
				}
				continue
			}

		}

		// Keep track of the number of notefiles that remain open
		if openfile.openCount != 0 {
			openfiles++
		}

	}

	// Done
	return openfiles == 0, firstError

}

// Checkpoint an open notefile
func (box *Notebox) uCheckpointNotefile(openfile *OpenNotefile) error {

	storage, err := storageProvider(openfile.storage)
	if err != nil {
		return err
	}

	// Only checkpoint if modified
	modCount := openfile.notefile.Modified()
	if modCount != openfile.modCountAfterCheckpoint {
		if debugBox || debugHubRequest {
			debugf("CHECKPOINT %s (%d mods, %d since first opened)\n", openfile.storage, modCount-openfile.modCountAfterCheckpoint, modCount)
		}

		// Write storage unless the underlying storage has been deleted during a Notebox merge
		if !openfile.deleted {
			nfLock.RLock()
			err = storage.writeNotefile(openfile.notefile, box.storage, openfile.storage)
			nfLock.RUnlock()
		} else {
			err = nil
		}

		// Bump the modification count
		if err == nil {
			openfile.modCountAfterCheckpoint = modCount
			box.openfiles[openfile.storage] = *openfile
		} else {
			debugf("Error checkpointing %s: %s\n", openfile.storage, err)
		}

	}

	return err

}

// Notefiles gets a list of all openable notefiles in the current boxfile
func (box *Notebox) Notefiles(includeTombstones bool) (notefiles []string, err error) {

	// Get pointers to our notefile
	boxopenfile, _ := box.openfiles[box.storage]
	boxfile := boxopenfile.notefile

	// Lock the notefile list
	nfLock.RLock()

	// Enum all notes in the boxfile, looking at the global files only
	allNotefiles := []string{}
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

	// Done
	nfLock.RUnlock()

	if debugBox {
		debugf("All Notefiles:\n%v\n", allNotefiles)
	}

	return allNotefiles, nil

}

// ClearAllTrackers deletes all trackers for this endpoint in order to force a resync of all files
func (box *Notebox) ClearAllTrackers(endpointID string) (err error) {

	// Clear the tracker on the notebox itself
	box.Notefile().ClearTracker(endpointID)

	// Get a list of all notefiles including the
	allNotefileIDs, err := box.Notefiles(false)
	if err != nil {
		return
	}

	// Iterate over all except the notebox itself
	for i := range allNotefileIDs {
		notefileID := allNotefileIDs[i]
		openfile, file, err := box.OpenNotefile(notefileID)
		if err == nil {
			file.ClearTracker(endpointID)
			openfile.Close()
		}
	}

	// Done
	return

}

// EndpointID gets the notebox's endpoint ID
func (box *Notebox) EndpointID() string {
	return box.endpointID
}

// GetChangedNotefiles determines, for a given tracker, if there are changes in any notebox and,
// if so, for which ones.
func (box *Notebox) GetChangedNotefiles(endpointID string) (changedNotefiles []string, err error) {

	// Get the names of all possible openable Notefiles
	allNotefiles, err := box.Notefiles(true)
	if err != nil {
		return []string{}, err
	}

	// Add the local endpoint, because that's explicitly not included in the list
	allNotefiles = append(allNotefiles, box.endpointID)

	// Get the notefile info of the "source" and "destination" for the changes
	sourceEndpointID := box.endpointID
	destinationEndpointID := endpointID

	// Enum the notefiles, looking for changes
	changedNotefiles = []string{}
	for i := range allNotefiles {
		notefileID := allNotefiles[i]
		openfile, file, err := box.OpenNotefile(notefileID)
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
			_, syncToHub, syncFromHub, _, _, _ := NotefileAttributesFromID(notefileID)
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
			openfile.Close()
		}
	}

	// Done
	return changedNotefiles, nil

}

// Close releases a notebox, but leaves it in-memory with a 0 refcount for low-overhead re-open
func (box *Notebox) Close() (err error) {

	// Lock the world
	boxLock.Lock()

	// Exit if we're about to do something really bad
	boxOpenfile, _ := box.openfiles[box.storage]
	if boxOpenfile.openCount == 0 {
		boxLock.Unlock()
		return fmt.Errorf("closing notebox: notebox already closed: %s", box.endpointID)
	}

	// If we're on last refcnt, checkpoint to make sure everything is up-to-date.  Otherwise,
	// rely upon the periodic timer-based checkpoint to checkpoint the box.  Also, close the
	// sync notehub and disconnect if this is the final refcnt
	if boxOpenfile.openCount == 1 {

		// Checkpoint
		_, err = box.uCheckpoint(false, false, false)

	}

	// Drop the refcnt on our own storage
	boxOpenfile.openCount--
	boxOpenfile.closeTime = time.Now()
	box.openfiles[box.storage] = boxOpenfile
	if debugBox {
		debugf("After closebox, %s close refcnt=%d modcnt=%d\n", box.storage, boxOpenfile.openCount, boxOpenfile.modCountAfterCheckpoint)
	}

	// Unlock
	boxLock.Unlock()

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
		err = fmt.Errorf("notefile name must end in a recognized type")
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

// NotefileExists returns true if notefile exists
func (box *Notebox) NotefileExists(notefileID string) (present bool) {
	boxLock.Lock()
	nfLock.Lock()
	boxopenfile, _ := box.openfiles[box.storage]
	_, present = boxopenfile.notefile.Notes[compositeNoteID(box.endpointID, notefileID)]
	nfLock.Unlock()
	boxLock.Unlock()
	return
}

// AddNotefile adds a new notefile to the notebox, and return "nil" if it already exists
func (box *Notebox) AddNotefile(notefileID string, notefileInfo *note.NotefileInfo) error {

	// Get the creation attributes
	isQueue, _, _, _, _, err := NotefileAttributesFromID(notefileID)
	if err != nil {
		return err
	}

	// Lock the world.  Note that we need the boxfile also, because of the AddNote
	boxLock.Lock()

	// First, do an immediate check to see if it already exists.  If so, short circuit everything
	// and return a clean "no error", which is relied upon by callers.
	nfLock.Lock()
	boxopenfile, _ := box.openfiles[box.storage]
	xnote, present := boxopenfile.notefile.Notes[compositeNoteID(box.endpointID, notefileID)]
	if present && !xnote.Deleted {
		nfLock.Unlock()
		boxLock.Unlock()

		// Refresh the notefile info, which may likely have changed
		if notefileInfo == nil {
			notefileInfo = &note.NotefileInfo{}
		}
		return box.SetNotefileInfo(notefileID, *notefileInfo)

	}
	nfLock.Unlock()

	// Add it
	err = box.uAddNotefile(notefileID, notefileInfo, CreateNotefile(isQueue))

	// Done
	boxLock.Unlock()
	return err
}

// GetNotefileInfo retrieves info about a specific notefile
func (box *Notebox) GetNotefileInfo(notefileID string) (notefileInfo note.NotefileInfo, err error) {

	// If this is for a boxfile, just return null info because there is no notefile info for a boxfile
	if box.endpointID == notefileID {
		return note.NotefileInfo{}, nil
	}

	// Now we're going to add things to the boxfile
	boxopenfile, _ := box.openfiles[box.storage]
	boxfile := boxopenfile.notefile

	// Find the specified Notefile's global descriptor
	xnote, err := boxfile.GetNote(notefileID)
	if err != nil {
		return note.NotefileInfo{}, fmt.Errorf(ErrNotefileNoExist+" cannot get info for notefile %s: %s", notefileID, err)
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
	boxopenfile, _ := box.openfiles[box.storage]
	boxfile := boxopenfile.notefile

	// Find the specified Notefile's global descriptor
	xnote, err := boxfile.GetNote(notefileID)
	if err != nil {
		return fmt.Errorf(ErrNotefileNoExist+" cannot set info for notefile %s: %s", notefileID, err)
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
	xnote.SetBody(noteboxBodyToJSON(body))

	// Update the note
	err = boxfile.UpdateNote(box.endpointID, notefileID, xnote)
	if err != nil {
		return fmt.Errorf(ErrNotefileNoExist+" cannot set info for notefile %s: %s", notefileID, err)
	}

	// Update it in-memory if it's open
	boxLock.Lock()
	nfLock.Lock()
	openfile, present := box.openfiles[body.Notefile.Storage]
	if present {
		openfile.notefile.notefileInfo = notefileInfo
	}
	nfLock.Unlock()
	boxLock.Unlock()

	// Done
	return

}

// CreateNotefile creates a new Notefile in-memory, which will ultimately be added to this Notebox
func (box *Notebox) CreateNotefile(isQueue bool) Notefile {
	return CreateNotefile(isQueue)
}

// AddNotefile adds a new notefile to the notebox.
// Note, importantly, that this does not open the notefile, and furthermore that because the supplied
// notefile is copied, it should no longer be used.  If one wishes to use the new storage-associated notefile,
// one should open it via the OpenNotefile method on the Notebox.
func (box *Notebox) uAddNotefile(notefileID string, notefileInfo *note.NotefileInfo, notefile Notefile) error {

	// Reject any attempt to add a notefile whose name contains our special delimiter
	if strings.Contains(notefileID, ReservedIDDelimiter) {
		return fmt.Errorf(ErrNotefileName+" name cannot contain reserved character '%s'", ReservedIDDelimiter)
	}

	// Reject notefiles with invalid names
	_, _, _, _, _, err := NotefileAttributesFromID(notefileID)
	if err != nil {
		return err
	}

	// Debug
	if debugBox {
		debugf("Adding: %s\n", notefileID)
	}

	// Now we're going to add things to the boxfile
	boxopenfile, _ := box.openfiles[box.storage]
	boxfile := boxopenfile.notefile

	// Find the specified notefile descriptor
	descnote, descfound := boxfile.Notes[notefileID]

	// Find the specified Notefile instance
	xnote, found := boxfile.Notes[compositeNoteID(box.endpointID, notefileID)]
	if found && !xnote.Deleted {
		return fmt.Errorf(ErrNotefileExists+" adding notefile: notefile already exists in notebox: %s", notefileID)
	}

	// First, create the global Notefile descriptor or undelete it
	if !descfound {
		body := noteboxBody{}
		body.Notefile.Info = notefileInfo
		var newdescnote note.Note
		newdescnote, err = note.CreateNote(noteboxBodyToJSON(body), nil)
		if err == nil {
			err = boxfile.uAddNote(box.endpointID, notefileID, &newdescnote, false)
		}
		if err != nil {
			return err
		}
	} else if descfound && descnote.Deleted {
		descnote.Deleted = false
		body := noteboxBodyFromJSON(descnote.GetBody())
		body.Notefile.Info = notefileInfo
		descnote.SetBody(noteboxBodyToJSON(body))
		err = boxfile.uUpdateNote(box.endpointID, notefileID, &descnote)
		if err != nil {
			return err
		}
		if debugBox {
			debugf("%s info updated: %v\n", notefileID, notefileInfo)
		}
	}

	// Inherit storage from the box
	storage, err := storageProvider(box.storage)
	if err != nil {
		return err
	}

	// Ask the new storage instance to create a new unique object
	fileStorage, err := storage.create(box.storage, notefileID)
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
			err = boxfile.uAddNote(box.endpointID, compositeNoteID(box.endpointID, notefileID), &inote, false)
		}
	} else {
		xnote.SetBody(noteboxBodyToJSON(body))
		err = boxfile.uUpdateNote(box.endpointID, compositeNoteID(box.endpointID, notefileID), &xnote)
	}
	if err != nil {
		storage.delete(box.storage, fileStorage)
		return err
	}

	// Write the storage object for the new notefile.  Note that if an error occurs
	// from here on, we need to leave storage around because it's already described
	// by the instance note above.
	err = storage.writeNotefile(&notefile, box.storage, fileStorage)
	if err != nil {
		return err
	}

	// Write the storage object locked, for consistency
	err = storage.writeNotefile(boxfile, box.storage, box.storage)
	if err != nil {
		return err
	}

	if debugBox {
		debugf("%s created\n", fileStorage)
	}

	// Done
	return err

}

// Notefile get the pointer to the notefile associated with the notebox, so that we can use
// standard Notefile methods to synchronize the Notebox.
func (box *Notebox) Notefile() (notefile *Notefile) {
	boxopenfile, _ := box.openfiles[box.storage]
	return boxopenfile.notefile
}

// convertToJSON serializes/marshals the in-memory Notebox into a JSON buffer
func (box *Notebox) convertToJSON(fIndent bool) (output []byte, err error) {
	boxopenfile, _ := box.openfiles[box.storage]
	boxfile := boxopenfile.notefile
	return boxfile.convertToJSON(fIndent)
}

// OpenNotefile opens a notefile, reading it from storage and bumping its refcount. As such,
// you MUST pair this with a call to CloseNotefile.  Note that this function must work
// for the "" NotefileID (when opening the notebox "|" instance), so don't add a check
// that would prohibit this.
func (box *Notebox) OpenNotefile(notefileID string) (iOpenfile *OpenNotefile, notefile *Notefile, err error) {

	// Lock the world
	boxLock.Lock()

	// Purge all deleted notefiles from the notebox, because we may be opening one that was deleted
	_, err = box.uCheckpoint(false, false, true)
	if err != nil {
		debugf("checkpoint error: %s\n", err)
	}

	// Lock the box's notefile so we can even see if it exists
	boxopenfile, _ := box.openfiles[box.storage]
	boxfile := boxopenfile.notefile

	// Find the specified Notefile and its descriptor, and exit if it doesn't exist
	nfLock.RLock()
	desc, descFound := boxfile.Notes[notefileID]
	note, found := boxfile.Notes[compositeNoteID(box.endpointID, notefileID)]
	nfLock.RUnlock()
	if descFound && desc.Deleted {
		descFound = false
	}
	if found && note.Deleted {
		found = false
	}
	if notefileID == box.endpointID {
		descFound = true
	}
	if !found || !descFound {
		boxLock.Unlock()
		return nil, nil, fmt.Errorf(ErrNotefileNoExist+" opening notefile: notefile does not exist: %s", notefileID)
	}

	// See if the notefile is currently open, by using its storage object ID
	notefileBody := noteboxBodyFromJSON(note.GetBody())
	filestorage := notefileBody.Notefile.Storage
	openfile, present := box.openfiles[filestorage]
	if present {

		// Increment the refcnt
		openfile.openCount++
		box.openfiles[filestorage] = openfile
		if debugBox {
			debugf("After openfile, %s open refcnt now %d\n", filestorage, openfile.openCount)
		}

		// Done
		boxLock.Unlock()
		return &openfile, openfile.notefile, nil

	}

	// Fetch the storage provider
	storage, err := storageProvider(filestorage)
	if err != nil {
		boxLock.Unlock()
		return nil, nil, err
	}

	// Load the notefile for this storage object.  If for some reason there's an error,
	// log it to the console (because it's corruption or a bug) and attempt recovery by
	// creating a blank notefile.  This may not always be the best thing, but it's better
	// than getting stuck in a mode where we can't open or otherwise manipulate the contents.
	notefile, err = storage.readNotefile(box.storage, filestorage)
	if err != nil {
		debugf("recovering from error reading notefile from %s: %s\n", filestorage, err)
		newNotefile := CreateNotefile(false)
		notefile = &newNotefile
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
	if notefileID != box.endpointID {
		descBody := noteboxBodyFromJSON(desc.GetBody())
		if descBody.Notefile.Info != nil {
			notefile.notefileInfo = *descBody.Notefile.Info
		}
	}

	// For the notefile itself, create a new notefile data structure with a single refcnt
	openfile = OpenNotefile{}
	openfile.notefile = notefile
	openfile.modCountAfterCheckpoint = notefile.modCount
	openfile.storage = filestorage
	openfile.box = box
	openfile.openCount = 1
	if debugBox {
		debugf("After openfile, %s open refcnt now %d\n", filestorage, openfile.openCount)
	}

	// Add this to the global list of open notefiles for this box
	box.openfiles[filestorage] = openfile

	// Done
	boxLock.Unlock()
	return &openfile, notefile, nil

}

// SetDiscoveryService establishes hub context so that we may auto-connect during sync
func (box *Notebox) SetDiscoveryService(hubLocationService string, hubAppUID string) error {
	box.hubLocationService = hubLocationService
	box.hubAppUID = hubAppUID
	return nil
}

// SetEventInfo establishes default information used for change notification on notefiles opened in the box
func (box *Notebox) SetEventInfo(deviceUID string, deviceSN string, productUID string, appUID string, iFn EventFunc, iCtx interface{}) error {
	box.defaultEventDeviceUID = deviceUID
	box.defaultEventDeviceSN = deviceSN
	box.defaultEventProductUID = productUID
	box.defaultEventAppUID = appUID
	box.defaultEventFn = iFn
	box.defaultEventCtx = iCtx
	return nil
}

// GetEventInfo retrieves the info
func (box *Notebox) GetEventInfo() (deviceUID string, deviceSN string, productUID string) {
	return box.defaultEventDeviceUID, box.defaultEventDeviceSN, box.defaultEventAppUID
}

// CheckpointNotefile checkpoints an open Notefile to storage
func (box *Notebox) CheckpointNotefile(notefileID string) (err error) {

	// Lock the workd
	boxLock.Lock()

	// Lock the box's notefile so we can even see if it exists
	boxopenfile, _ := box.openfiles[box.storage]
	boxfile := boxopenfile.notefile

	// Find the specified Notefile, and exit if it doesn't exist
	nfLock.RLock()
	note, found := boxfile.Notes[compositeNoteID(box.endpointID, notefileID)]
	nfLock.RUnlock()
	if !found {
		boxLock.Unlock()
		return fmt.Errorf(ErrNotefileNoExist+" checkpointing notefile: notefile does not exist: %s", notefileID)
	}

	// See if the notefile is currently open, by using its storage object ID
	notefileBody := noteboxBodyFromJSON(note.GetBody())
	openfile, present := box.openfiles[notefileBody.Notefile.Storage]
	if !present {
		boxLock.Unlock()
		return fmt.Errorf("checkpointing notefile: notefile isn't currently open: %s", notefileID)
	}

	// Checkpoint to make sure everything is up-to-date
	err = box.uCheckpointNotefile(&openfile)

	// Done
	boxLock.Unlock()
	return err

}

// Close closes an open Notefile, decrementing its refcount and making it available
// for purging from memory if this is the last reference.
func (openfile *OpenNotefile) Close() (err error) {

	// Lock the notefile
	boxLock.Lock()

	// Get the containing box
	box := openfile.box

	// Drop the refcnt on our own storage.  Note that we need to get an up-to-date copy
	// of openfile, because we've got a pointer to a stale copy that was pulled from the map
	// when we opened it up.
	nfLock.Lock()
	file := box.openfiles[openfile.storage]
	file.openCount--
	file.closeTime = time.Now()
	box.openfiles[openfile.storage] = file
	nfLock.Unlock()
	if debugBox {
		debugf("After closefile of %s, close refcnt now %d\n", file.storage, file.openCount)
	}

	// Checkpoint to make sure everything is up-to-date
	err = box.uCheckpointNotefile(&file)

	// Done
	boxLock.Unlock()
	return err

}

// DeleteNotefile deletes a closed Notefile, and releases its storage.
func (box *Notebox) DeleteNotefile(notefileID string) (err error) {

	if debugBox {
		debugf("Deleting %s\n", notefileID)
	}

	// Lock the world
	boxLock.Lock()

	// Make sure that all modified files are written and that we remove unused ones from memory
	_, err = box.uCheckpoint(false, true, false)
	if err != nil {
		debugf("Checkpoint error on delete: %s\n", err)
	}

	// Get the address of the open notebox
	boxopenfile, _ := box.openfiles[box.storage]
	boxfile := boxopenfile.notefile

	// Find the specified Notefile, and exit if it doesn't exist.  It's important that we
	// exit with 'no error' because we may have been here simply because the global instance
	// had existed.
	notefileNoteID := compositeNoteID(box.endpointID, notefileID)
	nfLock.RLock()
	note, found := boxfile.Notes[compositeNoteID(box.endpointID, notefileID)]
	nfLock.RUnlock()
	if !found {
		boxLock.Unlock()
		boxfile.DeleteNote(box.endpointID, notefileID)
		return nil
	}

	// See if the notefile is currently open, by using its storage object ID
	notefileBody := noteboxBodyFromJSON(note.GetBody())
	_, present := box.openfiles[notefileBody.Notefile.Storage]
	if present {
		boxLock.Unlock()
		if debugBox {
			debugf("Can't delete: %s is in use\n", notefileID)
		}
		return fmt.Errorf(ErrNotefileInUse+" deleting notefile: notefile is currently open: %s", notefileID)
	}

	// Get the storage class
	storage, err := storageProvider(notefileBody.Notefile.Storage)
	if err != nil {
		boxLock.Unlock()
		return err
	}

	// Delete the note used for managing notefiles across all instances,
	// ignoring errors because it may have been deleted by another instance
	boxfile.DeleteNote(box.endpointID, notefileID)

	// It's not open, so we don't need anything further locked
	boxLock.Unlock()

	// Delete the storage object
	storage.delete(box.storage, notefileBody.Notefile.Storage)

	// Delete the note for managing the instance
	boxfile.DeleteNote(box.endpointID, notefileNoteID)

	// Done
	return nil

}

// GetNote gets a note from a notefile
func (box *Notebox) GetNote(notefileID string, noteID string) (note note.Note, err error) {

	openfile, file, err := box.OpenNotefile(notefileID)
	if err != nil {
		return
	}

	note, err = file.GetNote(noteID)

	openfile.Close()

	return

}

// AddNote adds a new note to a notefile, which is a VERY common operation
func (box *Notebox) AddNote(endpointID string, notefileID string, noteID string, note note.Note) (err error) {

	openfile, file, err := box.OpenNotefile(notefileID)
	if err != nil {
		return err
	}

	err = file.AddNote(endpointID, noteID, note)

	openfile.Close()

	return err

}

// UpdateNote updates an existing note from notefile
func (box *Notebox) UpdateNote(endpointID string, notefileID string, noteID string, note note.Note) (err error) {

	openfile, file, err := box.OpenNotefile(notefileID)
	if err != nil {
		return err
	}

	err = file.UpdateNote(endpointID, noteID, note)

	openfile.Close()

	return err

}

// DeleteNote deletes an existing note from notefile
func (box *Notebox) DeleteNote(endpointID string, notefileID string, noteID string) (err error) {

	openfile, file, err := box.OpenNotefile(notefileID)
	if err != nil {
		return err
	}

	err = file.DeleteNote(endpointID, noteID)

	openfile.Close()

	return err

}

// GetChanges retrieves the next batch of changes being tracked
func (box *Notebox) GetChanges(endpointID string, maxBatchSize int) (file Notefile, numChanges int, totalChanges int, since int64, until int64, err error) {

	// Get the notefile for this notebox
	boxopenfile, _ := box.openfiles[box.storage]
	boxfile := boxopenfile.notefile

	// Get the tracked changes for that notefile
	notefile, numChanges, totalChanges, since, until, err := boxfile.GetChanges(endpointID, maxBatchSize)
	if err != nil {
		return Notefile{}, 0, 0, 0, 0, err
	}

	// Done
	return notefile, numChanges, totalChanges, since, until, nil

}

// UpdateChangeTracker updates the tracker once changes have been processed
func (box *Notebox) UpdateChangeTracker(endpointID string, since int64, until int64) error {

	// Get the notefile for this notebox
	boxopenfile, _ := box.openfiles[box.storage]
	boxfile := boxopenfile.notefile

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
// changes into the current notebox.
func (box *Notebox) MergeNotebox(fromBoxfile Notefile) (err error) {

	// Get pointers to our notefile
	boxopenfile, _ := box.openfiles[box.storage]
	boxfile := boxopenfile.notefile

	// First, purge anything pertaining to the local instances, so that we don't inadvertently
	// delete notes that are our only pointers to local storage.  We simply don't allow remote
	// endpoints to modify our local instance notes.
	for noteID := range fromBoxfile.Notes {
		isInstance, endpointID, _ := parseCompositeNoteID(noteID)
		if isInstance && endpointID == box.endpointID {
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
	boxLock.Lock()
	box.uCheckpoint(false, true, false)

	// Now that this is done, lock the world, both the boxfile and the box's notefile,
	// because in essence this is a custom merge procedure.
	nfLock.Lock()

	// Now, iterate over all notes in the notefile looking for work to do.  The rules we need to
	// pay attention to are (in this order),
	// 1. If ANY global notefile is deleted, then ALL instances must eventually go away
	// 2. If a global notefile note exists ANYWHERE, it must eventually appear on ALL instances
	// First, classify the entire contents of the notebox
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
		if isInstance && endpointID != box.endpointID {
			continue
		}
		// Classify it
		if !isInstance && note.Deleted {
			deletedGloballyNoteID = append(deletedGloballyNoteID, noteID)
		} else if !isInstance && !note.Deleted {
			existsGloballyNoteID = append(existsGloballyNoteID, noteID)
		} else if isInstance && endpointID == box.endpointID && note.Deleted {
			deletedLocally = append(deletedLocally, notefileID)
		} else if isInstance && endpointID == box.endpointID && !note.Deleted {
			existsLocally = append(existsLocally, notefileID)
		}
	}

	if debugBox {
		debugf("MergeNotebox:\ndeletedGloballyNoteID: %v\nexistsGloballyNoteID: %v\ndeletedLocally: %v\nexistsLocally: %v\n", deletedGloballyNoteID, existsGloballyNoteID, deletedLocally, existsLocally)
	}

	// 1. For all that are globally deleted, make sure they are deleted locally
	for i := range deletedGloballyNoteID {
		notefileID := deletedGloballyNoteID[i]
		if !isNoteInNoteArray(notefileID, deletedLocally) && isNoteInNoteArray(notefileID, existsLocally) {
			deletedLocally = append(deletedLocally, notefileID)

			// Delete the instance note, clearing out the storage for good measure
			noteid := compositeNoteID(box.endpointID, notefileID)
			note, _ := boxfile.Notes[noteid]
			body := noteboxBodyFromJSON(note.GetBody())
			fileStorage := body.Notefile.Storage
			body.Notefile.Storage = ""
			note.SetBody(noteboxBodyToJSON(body))
			boxfile.uDeleteNote(box.endpointID, noteid, &note)

			// Also delete the global descriptor, ignoring errors because it may already be gone
			note, present := boxfile.Notes[notefileID]
			if present {
				boxfile.uDeleteNote(box.endpointID, notefileID, &note)
			}

			// If this is the storage ID of any open notefile, prevent future writes
			openfile, present := box.openfiles[fileStorage]
			if present {
				openfile.deleted = true
				box.openfiles[fileStorage] = openfile
			}

			// Delete the local storage
			storage, err := storageProvider(fileStorage)
			if err == nil {
				storage.delete(box.storage, fileStorage)
			}

		}
	}

	// 2. For all nondeleted that are globally present, make sure they are replicated locally
	for i := range existsGloballyNoteID {
		notefileID := existsGloballyNoteID[i]
		if !isNoteInNoteArray(notefileID, existsLocally) || isNoteInNoteArray(notefileID, deletedLocally) {
			existsLocally = append(existsLocally, notefileID)

			// Add a new notefile
			box.uAddNotefile(notefileID, nil, CreateNotefile(false))

		}
	}

	// Done
	nfLock.Unlock()
	boxLock.Unlock()
	return nil

}
