// Copyright 2017 Blues Inc.  All rights reserved.
// Use of this source code is governed by licenses granted by the
// copyright holder including that found in the LICENSE file.

// Package notelib notelib.go has certain internal definitions, placed here so that
// they parallel the clang version.
package notelib

import (
	"net/http"
	"sync"
	"time"

	"github.com/blues/note-go/note"
)

// noteboxBody is the is the Body field within each note
// within the notebox. Each note's unique ID is constructed
// as a structured name of the form <endpointID>|<notefileID>.
// EndpointID is a unique string assigned to each endpoint.
// NotefileID is a unique string assigned to each notefile.
// All NoteID's without the | separator are reserved for
// future use, and at that time we will augment the Body
// data structure to accomodate the new type of data.

const TemplateFlagClearAfterSync = 0x00000001

type notefileDesc struct {
	// Optional metadata about the notefile
	Info *note.NotefileInfo `json:"i,omitempty"`
	// Storage method and physical location
	Storage string `json:"s,omitempty"`
	// Template info
	BodyTemplate    string `json:"B,omitempty"`
	PayloadTemplate uint32 `json:"P,omitempty"`
	TemplateFormat  uint32 `json:"f,omitempty"`
	TemplatePort    uint16 `json:"X,omitempty"`
	TemplateFlags   uint32 `json:"d,omitempty"`
}

type noteboxBody struct {
	// Used when Note ID is of the form "endpointID|notefileID"
	Notefile notefileDesc `json:"n,omitempty"`
}

// OpenNotefile is the in-memory data structure for an open notefile
type OpenNotefile struct {
	// Locks access to just this data structure
	lock sync.RWMutex
	// This notefile's containing box
	box *Notebox
	// Number of current users of the notefile who are
	// counting on the notefile address to be stable
	openCount int32
	// The time of last close where refcnt wne to 0
	closeTime time.Time
	// Modification count at point of last checkpoint
	modCountAfterCheckpoint int
	// The address of the open notefile
	notefile *Notefile
	// This notefile's storage object
	storage string
	// Whether or not this notefile has been deleted
	deleted bool
}

// NoteboxInstance is the in-memory data structure for an open notebox
type NoteboxInstance struct {
	// Map of POINTERS to OpenNotefiles, indexed by storage object.
	// These must be pointers so that we can look it up and bump refcnt
	// atomically without a lock.
	openfiles sync.Map
	// This notebox's storage object
	storage string
	// The endpoint ID that is to be used for all operations on the notebox
	endpointID string
}

// Notebox is the in-memory data structure for an open notebox
type Notebox struct {
	// Map of the Notefiles, indexed by storage object
	instance *NoteboxInstance
	// Default parameters to be passed to notefiles being opened in notebox
	defaultEventFn         EventFunc   // The function to call when notifying of a change
	defaultEventSession    *HubSession // And a parameter TODO rename
	defaultEventDeviceUID  string
	defaultEventDeviceSN   string
	defaultEventProductUID string
	defaultEventAppUID     string
	// For HTTP access control
	clientHTTPReq *http.Request
	clientHTTPRsp http.ResponseWriter
}
