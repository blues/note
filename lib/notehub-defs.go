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
const msgCheckpoint = "Z"

// nf is the native format of these structures
// Note that the field names in this structure must be unique within
// the notehubMessage structure because they are "flattened" to
// be at the root level of the JSON.
type nf struct {
	Notefile  *Notefile               `json:"NN,omitempty"`
	Notefiles *map[string]Notefile    `json:"NM,omitempty"`
	Body      *map[string]interface{} `json:"NB,omitempty"`
	Payload   *[]byte                 `json:"NP,omitempty"`
}

// cf is the compressed format of these structures
// Note that the field names in this structure must be unique within
// the notehubMessage structure because they are "flattened" to
// be at the root level of the JSON.
type cf struct {
	Notefile  []byte `json:"CN,omitempty"`
	Notefiles []byte `json:"CM,omitempty"`
	Body      []byte `json:"CB,omitempty"`
	Payload   []byte `json:"CP,omitempty"`
}

// NotehubMessage is the data structure used both for requests and responses.
// Note that this must be kept in perfect sync with the notehub protobuf definitions.
//
// IMPORTANT HISTORICAL NOTE:
// In our "current" era, the on-wire protocol for requests and responses is
// "protocol buffer"-based (see notehub.proto / notehub.options), and if you
// look at those files you'll see the consistency between that and this structure.
// However, you might be wondering "why does this structure have json field
// names, and why are those field names so short?"  The answer is partially
// historical, partially aesthetic, and partially forward-thinking.
// In the origin story of notecard/notehub, the card/hub protocol was implemented
// in HTTP, and I was using REST-over-cellular. In that original implementation,
// which was completely stateless, I wanted to see just how efficient I could make
// the protocol in terms of bytes-used because it was so nice to have an RPC
// using simple/scaleable LB's, etc. As a part of that, the most important aspects
// were that this message structure (and sub-objects hung off of it like Notefile)
// were very tightly packed, hence the use of single-letter field names and
// the use of "omitempty". However, no matter what I did, the protocol was HORRIBLY
// inefficient, as you'd expect. HTTP was not written to be efficient, and this
// is what caused the industry to develop things like WAP and COAP. In any case,
// once I moved to essentially marshaling/unmarshaling this structure manually
// to/from protobufs in wire.go, the json field names here are important only
// for aesthetics: when showing messages in the log (or, critically, on the
// minhub console where we are trying to show how tight our protocol is),
// these field names help convey info in a very efficient manner.
type notehubMessage struct {
	Version                        uint32              `json:"a,omitempty"`
	MessageType                    string              `json:"b,omitempty"`
	Error                          string              `json:"c,omitempty"`
	DeviceUID                      string              `json:"d,omitempty"`
	DeviceEndpointID               string              `json:"e,omitempty"`
	HubTimeNs                      int64               `json:"f,omitempty"`
	HubEndpointID                  string              `json:"g,omitempty"`
	HubSessionHandler              string              `json:"h,omitempty"`
	HubSessionFactoryResetID       string              `json:"i,omitempty"`
	HubSessionTicket               string              `json:"j,omitempty"`
	HubSessionTicketExpiresTimeSec int64               `json:"k,omitempty"`
	NotefileID                     string              `json:"l,omitempty"` // customer private data, should not log
	NotefileIDs                    string              `json:"m,omitempty"` // customer private data, should not log
	MaxChanges                     int32               `json:"n,omitempty"`
	Since                          int64               `json:"o,omitempty"`
	Until                          int64               `json:"p,omitempty"`
	NoteID                         string              `json:"q,omitempty"`
	SessionIDPrev                  int64               `json:"r,omitempty"`
	SessionIDNext                  int64               `json:"s,omitempty"`
	SessionIDMismatch              bool                `json:"t,omitempty"`
	ProductUID                     string              `json:"u,omitempty"`
	UsageProvisioned               int64               `json:"v,omitempty"`
	UsageRcvdBytes                 uint32              `json:"w,omitempty"`
	UsageSentBytes                 uint32              `json:"x,omitempty"`
	UsageTCPSessions               uint32              `json:"y,omitempty"`
	UsageTLSSessions               uint32              `json:"z,omitempty"`
	UsageRcvdNotes                 uint32              `json:"A,omitempty"`
	UsageSentNotes                 uint32              `json:"B,omitempty"`
	HighPowerSecsTotal             uint32              `json:"C,omitempty"`
	HighPowerSecsData              uint32              `json:"D,omitempty"`
	HighPowerSecsGPS               uint32              `json:"E,omitempty"`
	HighPowerCyclesTotal           uint32              `json:"F,omitempty"`
	HighPowerCyclesData            uint32              `json:"G,omitempty"`
	HighPowerCyclesGPS             uint32              `json:"H,omitempty"`
	DeviceSN                       string              `json:"I,omitempty"`
	CellID                         string              `json:"J,omitempty"`
	NotificationSession            bool                `json:"K,omitempty"`
	Voltage100                     int32               `json:"L,omitempty"`
	Temp100                        int32               `json:"M,omitempty"`
	Voltage1000                    int32               `json:"N,omitempty"`
	Temp1000                       int32               `json:"O,omitempty"`
	ContinuousSession              bool                `json:"P,omitempty"`
	MotionSecs                     int64               `json:"Q,omitempty"`
	MotionOrientation              string              `json:"R,omitempty"`
	SessionTrigger                 string              `json:"S,omitempty"`
	DeviceSKU                      string              `json:"T,omitempty"`
	DeviceFirmware                 int64               `json:"U,omitempty"`
	DevicePIN                      string              `json:"V,omitempty"`
	DeviceOrderingCode             string              `json:"W,omitempty"`
	UsageRcvdBytesSecondary        uint32              `json:"X,omitempty"`
	UsageSentBytesSecondary        uint32              `json:"Y,omitempty"`
	*nf                            `json:",omitempty"` // customer private data, should not log
	*cf                            `json:",omitempty"` // customer private data, should not log
}

// HubSessionContext are the fields that are coordinated between the client and
// the server for the duration of an open session.  The "Active" flag is true
// if the client believes that the service is actively maintaining it in parallel,
// and vice-versa on the service side.
type HubSessionContext struct {
	Active             bool
	Secure             bool
	Terminated         bool
	Discovery          bool
	Notification       bool
	BeganSec           int
	HandlerUID         string
	DeviceUID          string
	DeviceSN           string
	DeviceSKU          string
	DeviceOrderingCode string
	DeviceFirmware     int64
	DevicePIN          string
	ProductUID         string
	AppUID             string
	DeviceEndpointID   string
	HubEndpointID      string
	HubSessionTicket   string
	FactoryResetID     string
	IdForLogging       string
	Transactions       int
	EventsRouted       int
	EventQ             *chan *HubSessionEvent
	LatestUpdated      bool
	Latest             *map[string]note.Event
	Notefiles          []string
	NotefilesUpdated   bool
	RouteInfo          map[string]interface{}
	// Fields that are useful when logged in a session log
	WhereWhen int64 // Captured date (event.When) of when we saved DeviceSession.Where
	Session   note.DeviceSession
	// Cached Where info, used by event processing for efficiency
	CachedWhereLat      float64
	CachedWhereLon      float64
	CachedWhereLocation string
	CachedWhereCountry  string
	CachedWhereTimeZone string
	// For monitoring notefile changes
	DeviceMonitorID int64
	// For segmented web payload uploads.  This is a holding area
	// for a multi-segment payload being built up by web transactions.
	PendingWebPayload []byte
}

// HubSessionEvent is an event queue entry, containing everything necessary to process an event
type HubSessionEvent struct {
	Local bool
	File  Notefile
	Event note.Event
	Exit  bool
}

// HTTPUserAgent is the HTTP user agent for all our uses of HTTP
const HTTPUserAgent = "notes"

// LogFilter removes fields from a notehub request which contain customer data and should not be logged and returns a filteredNotehubMessage for logging purposes.
func (req notehubMessage) FilterForLog() notehubMessage {
	req.nf = nil
	req.cf = nil
	return req
}
