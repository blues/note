// Copyright 2017 Blues Inc.  All rights reserved.
// Use of this source code is governed by licenses granted by the
// copyright holder including that found in the LICENSE file.

// Package notelib notehub-defs contains device-hub RPC protocol definitions
package notelib

import (
	"github.com/blues/note-go/note"
)

// ReservedIDDelimiter is used to separate lists of noteIDs, notefileIDs, endpointIDs, thus is invalid in names
const ReservedIDDelimiter = ","

// The current version of the edge/notehub message protocol
const currentProtocolVersion = 0

// Message types
const msgSetSessionContext = "A"
const msgPing = ""
const msgPingLegacy = "P"
const msgDiscover = "D"
const msgNoteboxSummary = "S"
const msgNoteboxChanges = "C"
const msgNoteboxMerge = "n"
const msgNoteboxUpdateChangeTracker = "T"
const msgNotefileChanges = "c"
const msgNotefileMerge = "m"
const msgNotefilesMerge = "M"
const msgNotefileUpdateChangeTracker = "t"
const msgNotefileAddNote = "a"
const msgNotefileDeleteNote = "d"
const msgNotefileUpdateNote = "u"
const msgGetNotification = "N"
const msgReadFile = "R"
const msgWebRequest = "W"
const msgSignal = "s"

// nf is the native format of these structures
type nf struct {
	Notefile  *Notefile               `json:"A,omitempty"`
	Notefiles *map[string]Notefile    `json:"B,omitempty"`
	Body      *map[string]interface{} `json:"C,omitempty"`
	Payload   *[]byte                 `json:"D,omitempty"`
}

// cf is the compressed format of these structures
type cf struct {
	Notefile  []byte `json:"E,omitempty"`
	Notefiles []byte `json:"F,omitempty"`
	Body      []byte `json:"G,omitempty"`
	Payload   []byte `json:"H,omitempty"`
}

// NotehubMessage is the data structure used both for requests and responses.
// Note that this must be kept in perfect sync with the notehub protobuf definitions.
type notehubMessage struct {
	Version                        uint32 `json:"a,omitempty"`
	MessageType                    string `json:"b,omitempty"`
	Error                          string `json:"c,omitempty"`
	DeviceUID                      string `json:"d,omitempty"`
	DeviceEndpointID               string `json:"e,omitempty"`
	HubTimeNs                      int64  `json:"f,omitempty"`
	HubEndpointID                  string `json:"g,omitempty"`
	HubSessionHandler              string `json:"h,omitempty"`
	HubSessionTicket               string `json:"i,omitempty"`
	HubSessionTicketExpiresTimeSec int64  `json:"j,omitempty"`
	NotefileID                     string `json:"k,omitempty"`
	NotefileIDs                    string `json:"l,omitempty"`
	MaxChanges                     int32  `json:"m,omitempty"`
	Since                          int64  `json:"n,omitempty"`
	Until                          int64  `json:"o,omitempty"`
	NoteID                         string `json:"q,omitempty"`
	SessionIDPrev                  int64  `json:"r,omitempty"`
	SessionIDNext                  int64  `json:"s,omitempty"`
	SessionIDMismatch              bool   `json:"t,omitempty"`
	ProductUID                     string `json:"u,omitempty"`
	*nf                            `json:",omitempty"`
	*cf                            `json:",omitempty"`
	UsageProvisioned               int64  `json:"A,omitempty"`
	UsageRcvdBytes                 uint32 `json:"B,omitempty"`
	UsageSentBytes                 uint32 `json:"C,omitempty"`
	UsageTCPSessions               uint32 `json:"D,omitempty"`
	UsageTLSSessions               uint32 `json:"E,omitempty"`
	UsageRcvdNotes                 uint32 `json:"F,omitempty"`
	UsageSentNotes                 uint32 `json:"G,omitempty"`
	DeviceSN                       string `json:"S,omitempty"`
	CellID                         string `json:"I,omitempty"`
	NotificationSession            bool   `json:"N,omitempty"`
	Voltage100                     int32  `json:"V,omitempty"`
	Temp100                        int32  `json:"T,omitempty"`
}

// HubSessionContext are the fields that are coordinated between the client and
// the server for the duration of an open session.  The "Active" flag is true
// if the client believes that the service is actively maintaining it in parallel,
// and vice-versa on the service side.
type HubSessionContext struct {
	Active           bool
	Secure           bool
	Discovery        bool
	Notification     bool
	DeviceUID        string
	DeviceSN         string
	ProductUID       string
	DeviceEndpointID string
	HubEndpointID    string
	HubSessionTicket string
	Transactions     int
	EventQ           *chan HubSessionEvent
	LatestUpdated    bool
	Latest           *map[string]note.Event
	RouteInfo        map[string]interface{}
	// Fields that are useful when logged in a session log
	Session note.DeviceSession
	// Cached Where info, used by event processing for efficiency
	CachedWhereLat      float64
	CachedWhereLon      float64
	CachedWhereLocation string
	CachedWhereCountry  string
	CachedWhereTimeZone string
}

// HubSessionEvent is an event queue entry, containing everything necessary to process an event
type HubSessionEvent struct {
	Session HubSessionContext
	Local   bool
	File    Notefile
	Event   note.Event
	Exit    bool
}

// HTTPUserAgent is the HTTP user agent for all our uses of HTTP
const HTTPUserAgent = "notes"
