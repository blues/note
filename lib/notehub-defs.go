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
	Notefile  *Notefile               `json:"A"`
	Notefiles *map[string]Notefile    `json:"B"`
	Body      *map[string]interface{} `json:"C"`
	Payload   *[]byte                 `json:"D"`
}

// cf is the compressed format of these structures
type cf struct {
	Notefile  []byte `json:"E"`
	Notefiles []byte `json:"F"`
	Body      []byte `json:"G"`
	Payload   []byte `json:"H"`
}

// NotehubMessage is the data structure used both for requests and responses.
// Note that this must be kept in perfect sync with the notehub protobuf definitions.
type notehubMessage struct {
	Version                        uint32
	MessageType                    string
	Error                          string
	DeviceUID                      string
	DeviceEndpointID               string
	HubTimeNs                      int64
	HubEndpointID                  string
	HubSessionHandler              string
	HubSessionFactoryResetID       string
	HubSessionTicket               string
	HubSessionTicketExpiresTimeSec int64
	NotefileID                     string
	NotefileIDs                    string
	MaxChanges                     int32
	Since                          int64
	Until                          int64
	NoteID                         string
	SessionIDPrev                  int64
	SessionIDNext                  int64
	SessionIDMismatch              bool
	ProductUID                     string
	*nf                            `json:""`
	*cf                            `json:""`
	UsageProvisioned               int64
	UsageRcvdBytes                 uint32
	UsageSentBytes                 uint32
	UsageTCPSessions               uint32
	UsageTLSSessions               uint32
	UsageRcvdNotes                 uint32
	UsageSentNotes                 uint32
	HighPowerSecsTotal             uint32
	HighPowerSecsData              uint32
	HighPowerSecsGPS               uint32
	HighPowerCyclesTotal           uint32
	HighPowerCyclesData            uint32
	HighPowerCyclesGPS             uint32
	DeviceSN                       string
	CellID                         string
	NotificationSession            bool
	Voltage100                     int32
	Temp100                        int32
	Voltage1000                    int32
	Temp1000                       int32
	ContinuousSession              bool
	MotionSecs                     int64
	MotionOrientation              string
	SessionTrigger                 string
}

// HubSessionContext are the fields that are coordinated between the client and
// the server for the duration of an open session.  The "Active" flag is true
// if the client believes that the service is actively maintaining it in parallel,
// and vice-versa on the service side.
type HubSessionContext struct {
	Active           bool
	Secure           bool
	Terminated       bool
	Discovery        bool
	Notification     bool
	BeganSec         int
	DeviceUID        string
	DeviceSN         string
	ProductUID       string
	DeviceEndpointID string
	HubEndpointID    string
	HubSessionTicket string
	FactoryResetID   string
	Transactions     int
	EventsRouted     int
	EventQ           *chan HubSessionEvent
	LatestUpdated    bool
	Latest           *map[string]note.Event
	Notefiles        []string
	NotefilesUpdated bool
	RouteInfo        map[string]interface{}
	// Fields that are useful when logged in a session log
	Session note.DeviceSession
	// Cached Where info, used by event processing for efficiency
	CachedWhereLat      float64
	CachedWhereLon      float64
	CachedWhereLocation string
	CachedWhereCountry  string
	CachedWhereTimeZone string
	// URL for routing of events to a device-specified service
	DeviceRouteURL string
	// For monitoring notefile changes
	DeviceMonitorID int64
}

// HubSessionEvent is an event queue entry, containing everything necessary to process an event
type HubSessionEvent struct {
	Session *HubSessionContext
	Local   bool
	File    Notefile
	Event   note.Event
	Exit    bool
}

// HTTPUserAgent is the HTTP user agent for all our uses of HTTP
const HTTPUserAgent = "notes"
