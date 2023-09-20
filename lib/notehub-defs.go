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
const (
	msgSetSessionContext           = "A"
	msgPing                        = ""
	msgPingLegacy                  = "P"
	msgDiscover                    = "D"
	msgNoteboxSummary              = "S"
	msgNoteboxChanges              = "C"
	msgNoteboxMerge                = "n"
	msgNoteboxUpdateChangeTracker  = "T"
	msgNotefileChanges             = "c"
	msgNotefileMerge               = "m"
	msgNotefilesMerge              = "M"
	msgNotefileUpdateChangeTracker = "t"
	msgNotefileAddNote             = "a"
	msgNotefileDeleteNote          = "d"
	msgNotefileUpdateNote          = "u"
	msgGetNotification             = "N"
	msgReadFile                    = "R"
	msgWebRequest                  = "W"
	msgSignal                      = "s"
	msgCheckpoint                  = "Z"
)

// nf is the native format of these structures
// Note that the field names in this structure must be unique within
// the notehubMessage structure because they are "flattened" to
// be at the root level of the JSON.
type nf struct {
	Notefile  *Notefile               `json:"NN,omitempty"`
	Notefiles *map[string]*Notefile   `json:"NM,omitempty"`
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

	// These fields are part of the SESSION CONTEXT, and are transmitted to
	// the service as a part of the very first transaction on a session,
	// no matter what MessageType the transaction happens to be.
	// THESE FIELDS SHOULD NEVER BE OVERLOADED BY RPC TRANSACTIONS FOR
	// THEIR OWN PER-TRANSACTION DATA TRANSFER REQUIREMENTS!!!
	Version                        uint32 `json:"a,omitempty"`
	DeviceUID                      string `json:"d,omitempty"`
	DeviceEndpointID               string `json:"e,omitempty"`
	HubTimeNs                      int64  `json:"f,omitempty"`
	HubEndpointID                  string `json:"g,omitempty"`
	HubSessionHandler              string `json:"h,omitempty"`
	HubSessionFactoryResetID       string `json:"i,omitempty"`
	HubSessionTicket               string `json:"j,omitempty"`
	HubSessionTicketExpiresTimeSec int64  `json:"k,omitempty"`
	SessionIDPrev                  int64  `json:"r,omitempty"`
	SessionIDNext                  int64  `json:"s,omitempty"`
	SessionIDMismatch              bool   `json:"t,omitempty"`
	ProductUID                     string `json:"u,omitempty"`
	UsageProvisioned               int64  `json:"v,omitempty"`
	UsageRcvdBytes                 uint32 `json:"w,omitempty"`
	UsageSentBytes                 uint32 `json:"x,omitempty"`
	UsageTCPSessions               uint32 `json:"y,omitempty"`
	UsageTLSSessions               uint32 `json:"z,omitempty"`
	UsageRcvdNotes                 uint32 `json:"A,omitempty"`
	UsageSentNotes                 uint32 `json:"B,omitempty"`
	HighPowerSecsTotal             uint32 `json:"C,omitempty"`
	HighPowerSecsData              uint32 `json:"D,omitempty"`
	HighPowerSecsGPS               uint32 `json:"E,omitempty"`
	HighPowerCyclesTotal           uint32 `json:"F,omitempty"`
	HighPowerCyclesData            uint32 `json:"G,omitempty"`
	HighPowerCyclesGPS             uint32 `json:"H,omitempty"`
	DeviceSN                       string `json:"I,omitempty"`
	CellID                         string `json:"J,omitempty"`
	NotificationSession            bool   `json:"K,omitempty"`
	Voltage100                     int32  `json:"L,omitempty"`
	Temp100                        int32  `json:"M,omitempty"`
	Voltage1000                    int32  `json:"N,omitempty"`
	Temp1000                       int32  `json:"O,omitempty"`
	ContinuousSession              bool   `json:"P,omitempty"`
	MotionSecs                     int64  `json:"Q,omitempty"`
	MotionOrientation              string `json:"R,omitempty"`
	SessionTrigger                 string `json:"S,omitempty"`
	DeviceSKU                      string `json:"T,omitempty"`
	DeviceFirmware                 int64  `json:"U,omitempty"`
	DevicePIN                      string `json:"V,omitempty"`
	DeviceOrderingCode             string `json:"W,omitempty"`
	UsageRcvdBytesSecondary        uint32 `json:"X,omitempty"`
	UsageSentBytesSecondary        uint32 `json:"Y,omitempty"`
	Where                          string `json:"wh,omitempty"`
	WhereWhen                      int64  `json:"ww,omitempty"`

	// These fields are used within the handling of MessageType to mean things
	// that are specific to the transaction type specified in MessageType.  Note
	// that we use protobuf 'field overloading' because of a strong desire
	// to keep the protobuf 'message ID' to be 127 or less.  We do this because
	// the protobuf message ID is a 'varint' that occupies only a single byte
	// if it is less than 128, and occupies up to 10 bytes depending upon
	// the magnitude and sign of the number higher than that.  Because these
	// fields are overloaded with various user data, anything other than
	// MessageType, Error, and SuppressResponse should not be logged.
	MessageType      string `json:"b,omitempty"`
	Error            string `json:"c,omitempty"`
	SuppressResponse bool   `json:"Z,omitempty"`
	NotefileID       string `json:"l,omitempty"`
	NotefileIDs      string `json:"m,omitempty"`
	MaxChanges       int32  `json:"n,omitempty"`
	Since            int64  `json:"o,omitempty"`
	Until            int64  `json:"p,omitempty"`
	NoteID           string `json:"q,omitempty"`
	*nf              `json:",omitempty"`
	*cf              `json:",omitempty"`
}

// Key for HubSessionContext when stored in a golang context
type HubContextKey string

const HubSessionContextKey HubContextKey = "HubSessionContext"

// HubSessionContext are the fields that are coordinated between the client and
// the server for the duration of an open session.  The Device substructure consists
// of the fields written to the persistent Session record, while all else is ephemeral.
type HubSessionContext struct {
	Active             bool
	Terminated         bool
	IdForLogging       string
	Transactions       int
	Discovery          bool
	Notification       bool
	DeviceSKU          string
	DeviceOrderingCode string
	DeviceFirmware     int64
	DevicePIN          string
	DeviceEndpointID   string
	HubEndpointID      string
	HubSessionTicket   string
	FactoryResetID     string
	Session            note.DeviceSession
	// Used for hubreq.go to retain inter-transaction state
	Notefiles         []string
	NotefilesUpdated  bool
	PendingWebPayload []byte
	DeviceMonitorID   int64
}

// HTTPUserAgent is the HTTP user agent for all our uses of HTTP
const HTTPUserAgent = "BluesNotehub/1.0"
