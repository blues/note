// Copyright 2017 Blues Inc.  All rights reserved.
// Use of this source code is governed by licenses granted by the
// copyright holder including that found in the LICENSE file.

// Package notelib notehub-defs contains device-hub RPC protocol definitions
package notelib

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"time"

	"github.com/blues/note-go/note"
	"github.com/google/uuid"
)

// HTTPUserAgent is the HTTP user agent for all our uses of HTTP
const HTTPUserAgent = "BluesNotehub/1.0"

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

// Msg type prefix; if present, we suppress response
const msgSuppressResponsePrefix = "-"

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
	Version                        uint32  `json:"a,omitempty"`
	DeviceUID                      string  `json:"d,omitempty"`
	DeviceEndpointID               string  `json:"e,omitempty"`
	HubTimeNs                      int64   `json:"f,omitempty"`
	HubEndpointID                  string  `json:"g,omitempty"`
	HubSessionHandler              string  `json:"h,omitempty"`
	HubSessionFactoryResetID       string  `json:"i,omitempty"`
	HubSessionTicket               string  `json:"j,omitempty"`
	HubSessionTicketExpiresTimeSec int64   `json:"k,omitempty"`
	SessionIDPrev                  int64   `json:"r,omitempty"`
	SessionIDNext                  int64   `json:"s,omitempty"`
	SessionIDMismatch              bool    `json:"t,omitempty"`
	ProductUID                     string  `json:"u,omitempty"`
	UsageProvisioned               int64   `json:"v,omitempty"`
	UsageRcvdBytes                 uint32  `json:"w,omitempty"`
	UsageSentBytes                 uint32  `json:"x,omitempty"`
	UsageTCPSessions               uint32  `json:"y,omitempty"`
	UsageTLSSessions               uint32  `json:"z,omitempty"`
	UsageRcvdNotes                 uint32  `json:"A,omitempty"`
	UsageSentNotes                 uint32  `json:"B,omitempty"`
	HighPowerSecsTotal             uint32  `json:"C,omitempty"`
	HighPowerSecsData              uint32  `json:"D,omitempty"`
	HighPowerSecsGPS               uint32  `json:"E,omitempty"`
	HighPowerCyclesTotal           uint32  `json:"F,omitempty"`
	HighPowerCyclesData            uint32  `json:"G,omitempty"`
	HighPowerCyclesGPS             uint32  `json:"H,omitempty"`
	DeviceSN                       string  `json:"I,omitempty"`
	CellID                         string  `json:"J,omitempty"`
	NotificationSession            bool    `json:"K,omitempty"`
	Voltage100                     int32   `json:"L,omitempty"`
	Temp100                        int32   `json:"M,omitempty"`
	Voltage1000                    int32   `json:"N,omitempty"`
	Temp1000                       int32   `json:"O,omitempty"`
	ContinuousSession              bool    `json:"P,omitempty"`
	MotionSecs                     int64   `json:"Q,omitempty"`
	MotionOrientation              string  `json:"R,omitempty"`
	SessionTrigger                 string  `json:"S,omitempty"`
	DeviceSKU                      string  `json:"T,omitempty"`
	DeviceFirmware                 int64   `json:"U,omitempty"`
	DevicePIN                      string  `json:"V,omitempty"`
	DeviceOrderingCode             string  `json:"W,omitempty"`
	UsageRcvdBytesSecondary        uint32  `json:"X,omitempty"`
	UsageSentBytesSecondary        uint32  `json:"Y,omitempty"`
	Where                          string  `json:"wh,omitempty"`
	WhereWhen                      int64   `json:"ww,omitempty"`
	HubPacketHandler               string  `json:"ph,omitempty"`
	PowerSource                    uint32  `json:"ps,omitempty"`
	PowerMahUsed                   float64 `json:"pm,omitempty"`

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

const NotecardPowerCharging = uint32(0x00000001)
const NotecardPowerUsb = uint32(0x00000002)
const NotecardPowerPrimary = uint32(0x00000004)

type HubSession struct {
	// These is all the fields sent from the notecard and decoded in wire.go
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

	// Information about the current session
	SessionStart time.Time
	AppUID       string
	Active       bool
	Terminated   bool
	Transactions int

	// Used for hubreq.go to retain inter-transaction state
	Notefiles         []string
	NotefilesUpdated  bool
	PendingWebPayload []byte
	DeviceMonitorID   int64

	// This field is used for any hub implementation to attach
	// data specific to that implementation
	Hub interface{}
}

// NewUUID returns a new UUID
func NewUUID() (randstr string) {
	return uuid.New().String()
}

// NewUNameFromUID returns a name from a UUID
func NameFromUUID(uid string) (name string) {
	hash := sha256.Sum256([]byte(uid))
	uint32Hash := binary.BigEndian.Uint32(hash[:4])
	return note.WordsFromNumber(uint32Hash)
}

func InitDeviceSession(sess *note.DeviceSession, handlerAddr string, secure bool) time.Time {
	now := time.Now()
	sess.SessionUID = NewUUID()
	sess.SessionBegan = now.UTC().Unix()
	sess.Period().Since = sess.SessionBegan
	sess.TLSSession = secure
	sess.Handler = handlerAddr
	if secure {
		sess.Period().TLSSessions++
	} else {
		sess.Period().TCPSessions++
	}
	return now
}

func NewHubSession(handlerAddr string, secure bool) HubSession {
	deviceSession := note.DeviceSession{}
	sessionBegan := InitDeviceSession(&deviceSession, handlerAddr, secure)
	return HubSession{
		Session:      deviceSession,
		SessionStart: sessionBegan,
	}
}

func (session HubSession) LogInfo(ctx context.Context, format string, args ...interface{}) {
	if debugHubRequest {
		logInfo(ctx, session.logTag()+" "+format, args...)
	}
}

func (session HubSession) LogWarn(ctx context.Context, format string, args ...interface{}) {
	if debugHubRequest {
		logWarn(ctx, session.logTag()+" "+format, args...)
	}
}

func (session HubSession) LogError(ctx context.Context, format string, args ...interface{}) {
	if debugHubRequest {
		logError(ctx, session.logTag()+" "+format, args...)
	}
}

func (session HubSession) LogDebug(ctx context.Context, format string, args ...interface{}) {
	if debugHubRequest {
		logDebug(ctx, session.logTag()+" "+format, args...)
	}
}

func (session HubSession) logTag() string {
	sessionUID := "[NO-SESSION]"
	deviceUID := "[NO-DEVICE]"
	sessionTypes := ""

	if len(session.Session.SessionUID) > 8 {
		sessionUID = session.Session.SessionUID[:8]
	} else if len(session.Session.SessionUID) > 0 {
		sessionUID = session.Session.SessionUID
	}

	if len(session.Session.DeviceUID) > 0 {
		deviceUID = session.Session.DeviceUID
	}

	if session.Notification {
		sessionTypes += "N"
	}
	if session.Session.ContinuousSession {
		sessionTypes += "C"
	}
	if session.Discovery {
		sessionTypes += "D"
	}
	if sessionTypes == "" {
		if len(session.Session.DeviceUID) > 0 {
			sessionTypes = "P"
		} else {
			sessionTypes = "?"
		}
	}

	return fmt.Sprintf("%s/%s (%s)", deviceUID, sessionUID, sessionTypes)
}

// PV1 wire format (see wire.go)
//
//	PacketType (high nibble, including a 'packet encrypted' flag)
//	PacketCid (low nibble)
//			0 None - the server knows which device is connected
//			1 Server-assigned ConnectionID
//	If PacketCid == CidRandom
//			[CidRandomLen]uint8
//	If sent to an unencrypted port:
//		MessagePort uint8
//		Data [0..N]uint8 (possibly Snappy-compressed)
//	If sent to an encrypted port
//		ChaCha20Poly1305Iv [ChaCha20Poly1305IvLen]uint8
//		Ciphertext:
//			MessagePort uint8
//			Data uint8[0..N] (possibly Snappy-compressed)
//		MAC derived from ChaCha20Poly1305AuthTagPlaintext:
//			ChaCha20Poly1305AuthTag uint8[ChaCha20Poly1305AuthTagLen]

// Packet type is split - a nibble of flags and a nibble of Connection ID type
const PacketTypeMask = 0x0f

const PacketFlagEncrypted = 0x80
const PacketFlagCompressed = 0x40
const PacketFlagDownlinksPending = 0x20
const PacketFlagSpare1 = 0x10

// Type of connection ID present in the packet
const CidNone = byte(0)   // Zero bytes
const CidRandom = byte(1) // CidRandomLen bytes

const CidRandomLen = 6

// Default MTU
const UdpMinMtu = 256 // Below this, we really wouldn't have enough room for any user data to speak of
const UdpMaxMtu = 548 // Minimum internet datagram is 576, IPv4 header is 20, UDP header is 8

// Supported packet service types.  Except for UDP, where there is no ID, The appropriate ID for that service type
// is appended.  For example, it might be the IMEI, IMSI, ICCID, or vendor-proprietary ID of the unit
const UdpPs = "udp:"
const UdpMtu = UdpMinMtu
const UdpCidType = CidRandom

// After trying AES GCM and struggling with block size and tag size constraints, and after then consulting
// with the folks at WolfSSL, we arrived at using the ChaCha20 stream cipher (thus eliminating block constraints)
// along with the Poly1305 MAC for integrity and authentication.
const PV1 = "pv1"
const PV1EncrAlg = "chacha20-poly1305"
const ChaCha20Poly1305KeyLen = 32
const ChaCha20Poly1305IvLen = 12
const ChaCha20Poly1305AuthTagLen = 16
const ChaCha20Poly1305AuthTagPlaintext = "notecard <3 notehub"

// Message ports for packet handling
const MportInvalid = 0
const MportUserNotefileFirst = 1
const MportUserNotefileLast = 100
const MportSysNotefileFirst = 101
const MportSysNotefileLast = 125
const MportUserSysNotefileFirst = 126
const MportUserSysNotefileLast = 150
const MportChangesDownlink = 250
const MportTimeDownlink = 251
const MportMoreDownlink = 252
const MportEchoDownlink = 253
const MportMultiportUplink = 254         // Enum of <port><datalen><data>
const MportMultiportUplinkDownlink = 255 // Same but with a guaranteed response formatted the same way

const MportSysNotefileEnv = (MportSysNotefileFirst + 0)

// Decoded packet
type Packet struct {
	ConnectionID []byte
	MessagePort  uint8
	Data         []byte
}

type PacketHandlerNotecard struct {
	PacketService string `json:"service,omitempty"`
	PacketCidType byte   `json:"cid_type,omitempty"`
	PacketMtu     uint   `json:"packet_mtu,omitempty"`
	EncrAlg       string `json:"alg,omitempty"`
	EncrKey       []byte `json:"key,omitempty"`
}

type PacketHandlerNotehub struct {
	// The Notehub generates a unique connection ID for every NTN device.  It
	// is always assigned, and for certain NTN transports (such as UDP) it is
	// carried 'in-band' in the packet contents, while for other transports
	// (such as skylo) it is never carrierd on-the-wire.
	CidType byte   `json:"cid_type,omitempty"` // CID_RANDOM
	Cid     []byte `json:"cid,omitempty"`      // Random connection ID
	// The Notehub can optionally tell the notecard to override the standard
	// encryption for .dbs/.qos/.qis files, either forcing it to be ON or OFF
	MustEncrypt bool `json:"encrall,omitempty"`
	MayEncrypt  bool `json:"encr,omitempty"`
	// Specifically used by the notecard/notehub when the transport is UDP
	UdpMtu      uint   `json:"mtu,omitempty"`
	UdpIpV4     string `json:"ipv4,omitempty"`
	UdpIpV4Port string `json:"ipv4port,omitempty"`
	UdpPolicy   string `json:"comms,omitempty"`
}

// Packet handler information
type PacketHandler struct {
	// Uploaded by Notecard when packet service is requested, and these are the
	// parameters used when decoding packets from the notecard AND used when
	// encoding packets to be sent back to the notecard.
	Notecard PacketHandlerNotecard `json:"notecard,omitempty"`

	// Set by Notehub by policy when handler is issued
	Notehub PacketHandlerNotehub `json:"notehub,omitempty"`
}
