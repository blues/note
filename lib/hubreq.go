// Copyright 2017 Blues Inc.  All rights reserved.
// Use of this source code is governed by licenses granted by the
// copyright holder including that found in the LICENSE file.

// Package notelib hubreq.go is the service-side complement to the Notehub client-side package
package notelib

import (
	"context"
	"crypto/md5"
	"fmt"
	"hash/crc32"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/blues/note-go/note"
	"github.com/golang/snappy"
)

// Flag indicating TLS support on this server
var serverSupportsTLS bool

// NoteboxUpdateEnvFunc is the func to update environment variables for a device
type NoteboxUpdateEnvFunc func(ctx context.Context, box *Notebox, deviceUID string, appUID string, updateFeeds bool) (err error)

var fnNoteboxUpdateEnv NoteboxUpdateEnvFunc

// HubSetUpdateEnvironment sets the global notebox function to update env vars
func HubSetUpdateEnvironment(ctx context.Context, fn NoteboxUpdateEnvFunc) {
	fnNoteboxUpdateEnv = fn
}

// Update the environment variables for an open notebox
func hubUpdateEnvVars(ctx context.Context, box *Notebox, deviceUID string, appUID string, updateFeeds bool) {
	if fnNoteboxUpdateEnv != nil {
		err := fnNoteboxUpdateEnv(ctx, box, deviceUID, appUID, updateFeeds)
		if err != nil {
			logDebug(ctx, "env: can't update env vars: %s", err)
		}
	}
}

// GetNotificationFunc retrieves hub notifications to be sent to client
type GetNotificationFunc func(ctx context.Context, deviceEndpointID string, session *HubSession) (notifications []string)

var fnHubNotifications GetNotificationFunc

// HubSetDeviceNotifications sets the hub notification
func HubSetDeviceNotifications(fn GetNotificationFunc) {
	fnHubNotifications = fn
}

// ReadFileFunc is the func to read a byte range from the named file
type ReadFileFunc func(appUID string, filetype string, key string, offset int32, length int32, compress bool, getInfo bool) (body []byte, payload []byte, err error)

var fnHubReadFile ReadFileFunc

// HubSetReadFile sets the read file func
func HubSetReadFile(fn ReadFileFunc) {
	fnHubReadFile = fn
}

// WebRequestFunc performs a web request on behalf of a device
type WebRequestFunc func(ctx context.Context, sess *HubSession, deviceUID string, productUID string, alias string, reqtype string, reqcontent string, reqoffset int, reqmaxbytes int, target string, bodyJSON []byte, payload []byte) (rspstatuscode int, rspheader map[string][]string, rspBodyJSON []byte, rspPayloadJSON []byte, err error)

var fnHubWebRequest WebRequestFunc

// HubSetWebRequest sets the web request function
func HubSetWebRequest(fn WebRequestFunc) {
	fnHubWebRequest = fn
}

// SignalFunc performs a web request on behalf of a device
type SignalFunc func(deviceUID string, bodyJSON []byte, payload []byte, session *HubSession) (err error)

var fnHubSignal SignalFunc

// HubSetSignal sets the web request function
func HubSetSignal(fn SignalFunc) {
	fnHubSignal = fn
}

// Make a checkpoint request
func HubCheckpointRequest() (message []byte, err error) {
	var req notehubMessage
	req.Version = currentProtocolVersion
	req.MessageType = msgCheckpoint
	message, _, err = msgToWire(req)
	return
}

// HubRequest creates a request message from a map of arguments, given an optional
// context parameter for fields that are shared across requests that happen within
// a single session.  The return arguments should be interpreted as follows:
// - if err is returned, do not return a reply to the remote requestor
// - if err is nil, always send result back to request
func HubRequest(ctx context.Context, content []byte, event EventFunc, session *HubSession) (reqtype string, result []byte, suppress bool, err error) {
	// Preset in case of error return
	var rsp notehubMessage
	rsp.Version = currentProtocolVersion

	// Retrieve the session context
	if session == nil {
		err = fmt.Errorf("no hub session context")
		return
	}

	// Convert from on-wire text to a message, and extract its type
	req, wirelen, err2 := msgFromWire(content)
	if err2 != nil {
		reqtype = "indeterminate"
		err = err2
		return
	}

	suppress = req.SuppressResponse
	reqtype = msgTypeName(req.MessageType)
	reqStart := time.Now()

	filteredMsg := filterForLog(req)
	filteredJSON, _ := note.JSONMarshal(filteredMsg)
	session.LogInfo(ctx, "Request #%d (%db wire) %s %s", session.Transactions, wirelen, reqtype, filteredJSON)

	// Authenticate the session
	if !session.Active {

		// Ensure that the device is valid.  (If it had to be provisioned it would've been before
		// we ever arrived here.)
		sessionTicket, _, _, _, err2 := HubDiscover(req.DeviceUID, req.DeviceSN, req.ProductUID)
		if err2 != nil {
			err = fmt.Errorf("discover error "+note.ErrAuth+": %s", err2)
		} else {
			if req.MessageType == msgDiscover {
				if TLSSupport() && !session.Session.TLSSession {
					err = fmt.Errorf("secure session required " + note.ErrAuth)
					session.LogWarn(ctx, "%s: attempt to discover with unsecure session", req.DeviceUID)
				}
			} else {
				// If the ticket isn't an exact match, then it's not a valid connect attemp
				if sessionTicket == "" || sessionTicket != req.HubSessionTicket {
					err = fmt.Errorf("session not authorized " + note.ErrAuth + note.ErrTicket)
					session.LogWarn(ctx, "TICKET REJECTED for %s (server may have been restarted)\n    Assigned: %s\n   Requested: %s", req.DeviceUID, sessionTicket, req.HubSessionTicket)
				}
			}
		}
	}

	// Only proceed if no auth error
	if err == nil {

		// Set or supply session context as appropriate.  The client will send the data on EITHER
		// a formal "set session context" message, OR on the very first message (as an optimization)
		if !session.Active {
			_, err = WireExtractSessionContext(content, session)
			if err != nil {
				err = fmt.Errorf("couldn't extract session context: %w", err)
				return
			}
		} else {
			req.DeviceUID = session.Session.DeviceUID
			req.DeviceSN = session.Session.DeviceSN
			req.DeviceEndpointID = session.DeviceEndpointID
			req.HubEndpointID = session.HubEndpointID
			req.HubSessionTicket = session.HubSessionTicket
			req.UsageProvisioned = session.Session.This().Since
			req.UsageRcvdBytes = session.Session.This().RcvdBytes
			req.UsageSentBytes = session.Session.This().SentBytes
			req.UsageRcvdBytesSecondary = session.Session.This().RcvdBytesSecondary
			req.UsageSentBytesSecondary = session.Session.This().SentBytesSecondary
			req.UsageTCPSessions = session.Session.This().TCPSessions
			req.UsageTLSSessions = session.Session.This().TLSSessions
			req.UsageRcvdNotes = session.Session.This().RcvdNotes
			req.UsageSentNotes = session.Session.This().SentNotes
			req.NotificationSession = session.Notification
			req.ContinuousSession = session.Session.ContinuousSession
			req.CellID = session.Session.CellID
		}

		// Make sure that we refresh the ProductUID from the session regardless of whether this
		// is the first transaction or a subsequent transaction because the session may have
		// redirected us to a different productUID
		req.ProductUID = session.Session.ProductUID

		// If there is a null session ticket, the only request that's permitted is a Discover request.
		if req.HubSessionTicket == "" && req.MessageType != msgDiscover {
			err = fmt.Errorf("transaction is not allowed "+note.ErrAuth+" without a session ticket: %s", reqtype)
		}

		// Process it and generate the return results
		if err == nil {
			rsp = processRequest(ctx, session, req, event)
		}

	}

	// After the first request has been successfully executed, mark it as active, and piggyback the session info
	if err != nil {
		rsp.Error = fmt.Sprintf("%s", err)
		err = nil
	} else if !session.Active {

		session.Active = true

		// Return info to the client about its position and timezone.  Prefer to use Tower because
		// it may be more up-to-date than the triangulated position, but if it's not available (such
		// as is the case on WiFi) then use the last known triangulated position.
		lastKnownLocation := note.TowerLocation{}
		if session.Session.Tower.OLC != "" {
			lastKnownLocation = session.Session.Tower
		} else if session.Session.Tri.OLC != "" {
			lastKnownLocation = session.Session.Tri
		}
		if lastKnownLocation.OLC != "" {

			// Load the location to compute cell offset, if possible
			shortZone := ""
			offsetSecondsEastOfUTC := 0
			location, err := time.LoadLocation(lastKnownLocation.TimeZone)
			if err != nil {
				session.LogWarn(ctx, "*** Can't load location for: %s", lastKnownLocation.TimeZone)
			} else {
				localTime := time.Now().In(location)
				shortZone, offsetSecondsEastOfUTC = localTime.Zone()
			}

			// Return everything packed into the CellID field
			rsp.CellID = lastKnownLocation.OLC
			rsp.CellID += "|" + lastKnownLocation.CountryCode
			rsp.CellID += "|" + lastKnownLocation.TimeZone
			rsp.CellID += "|" + shortZone
			if shortZone != "" {
				rsp.CellID += "|" + fmt.Sprintf("%d", offsetSecondsEastOfUTC/60)
			} else {
				rsp.CellID += "|"
			}
			rsp.CellID += "|" + lastKnownLocation.Name

		}

	}

	// Convert back to on-wire format
	result, wirelen, err = msgToWire(rsp)
	if err != nil {
		return
	}

	// Display the result
	filteredRsp := filterForLog(rsp)
	filteredJSON, _ = note.JSONMarshal(filteredRsp)
	suppressedMsg := ""
	if suppress {
		suppressedMsg = " SUPPRESSED"
	}
	session.LogInfo(ctx, "Response #%d (%db wire%s) %s in %s %s", session.Transactions, wirelen, suppressedMsg, reqtype, time.Since(reqStart), filteredJSON)

	// Done
	return
}

// HubErrorResponse creates an error response message to be sent to the client device, and
// terminates the session so that no further requests can be processed.
func HubErrorResponse(ctx context.Context, session *HubSession, errorMessage string) (result []byte) {
	var rsp notehubMessage

	// Put the session into a terminated state because of the error
	session.Terminated = true

	// Create a response message
	rsp.Version = currentProtocolVersion
	rsp.Error = errorMessage

	// Convert to on-wire format
	response, wirelen, err := msgToWire(rsp)
	if err != nil {
		return
	}
	result = response

	// Display the result
	session.LogInfo(ctx, "Response #%d (%db wire) Error %s", session.Transactions, wirelen, errorMessage)

	// Done
	return
}

// processRequest processes a request message
func processRequest(ctx context.Context, session *HubSession, req notehubMessage, event EventFunc) (response notehubMessage) {
	var err error

	// Default fields in the response data structure
	rsp := notehubMessage{}
	rsp.Version = currentProtocolVersion

	// Dispatch based on message type
	switch req.MessageType {

	default:
		rsp.Error = "unrecognized message type"

	case msgSetSessionContext:
		// This is a nil transaction

	case msgPing:
		rsp.HubTimeNs = time.Now().UnixNano()

	case msgPingLegacy:
		// This is a nil transaction

	case msgGetNotification:
		err = hubGetNotification(ctx, session, req, &rsp, event)

	case msgDiscover:
		err = hubDiscovery(ctx, session, req, &rsp, event)

	case msgNoteboxSummary:
		err = hubNoteboxSummary(ctx, session, req, &rsp, event)

	case msgNoteboxChanges:
		err = hubNoteboxChanges(ctx, session, req, &rsp, event)

	case msgNoteboxMerge:
		err = hubNoteboxMerge(ctx, session, req, &rsp, event)

	case msgNoteboxUpdateChangeTracker:
		err = hubNoteboxUpdateChangeTracker(ctx, session, req, &rsp, event)

	case msgNotefileChanges:
		err = hubNotefileChanges(ctx, session, req, &rsp, event)

	case msgNotefileMerge:
		err = hubNotefileMerge(ctx, session, req, &rsp, event)

	case msgNotefilesMerge:
		err = hubNotefilesMerge(ctx, session, req, &rsp, event)

	case msgNotefileUpdateChangeTracker:
		err = hubNotefileUpdateChangeTracker(ctx, session, req, &rsp, event)

	case msgNotefileAddNote:
		err = hubNotefileAddNote(ctx, session, req, &rsp, event)

	case msgNotefileDeleteNote:
		err = hubNotefileDeleteNote(ctx, session, req, &rsp, event)

	case msgNotefileUpdateNote:
		err = hubNotefileUpdateNote(ctx, session, req, &rsp, event)

	case msgReadFile:
		err = hubReadFile(ctx, session, req, &rsp, event)

	case msgWebRequest:
		err = hubWebRequest(ctx, session, req, &rsp, event)

	case msgSignal:
		err = hubSignal(ctx, session, req, &rsp, event)

	case msgCheckpoint:
		err = hubCheckpoint(ctx, session, req, &rsp, event)

	}

	// If an error, return it this way
	if err != nil {
		rsp.Error = fmt.Sprintf("%s", err)
	}

	return rsp
}

// Open the endpoint's notebox
func openHubNoteboxForDevice(ctx context.Context, session *HubSession, deviceUID string, deviceSN string, productUID string, endpointID string, event EventFunc) (box *Notebox, appUID string, err error) {
	// Ensure that hub endpoint is available
	var hubEndpointID, deviceStorageObject string
	_, hubEndpointID, appUID, deviceStorageObject, err = HubDiscover(deviceUID, deviceSN, productUID)
	if err != nil {
		return
	}

	// Open the endpoint's box on behalf of the Notehub, because we can't open it directly under the device endpoint IDs
	box, err = OpenEndpointNotebox(ctx, hubEndpointID, deviceStorageObject, true)
	if err != nil {
		return
	}

	// Set the default notification context for files opened within this box instance
	box.SetEventInfo(deviceUID, deviceSN, productUID, appUID, event, session)

	// If we haven't yet enum'ed the notefiles, do so
	sawNotefile(session, box, "")

	// Done
	return
}

// Ensure that the specified notefile is in the list
func sawNotefile(session *HubSession, box *Notebox, notefileID string) {
	// First time through
	if !session.NotefilesUpdated {
		session.Notefiles = box.Notefiles(false)
		session.NotefilesUpdated = true
	}

	// Exit if just loading the notefiles
	if notefileID == "" {
		return
	}

	// Ensure it's there
	found := false
	for _, n := range session.Notefiles {
		if n == notefileID {
			found = true
			break
		}
	}
	if !found {
		session.Notefiles = append(session.Notefiles, notefileID)
	}
}

// Perform Web Request, with message fields being used/overloaded as follows:
// NoteID is Route
// NotefileID is Method such as "GET"
// NotefileIDs is HTTP URL target such as "/foo=bar&xxx=1"
// MotionOrientation is HTTP content type
// MotionSecs is maximum result size
// SessionIDMismatch is set to true if the payload is compressed
// HighPowerSecsTotal is totalPayloadLen
// HighPowerSecsData is payloadOffset
// HighPowerSecsGPS is an indication (a maximum size on device) to return the binary as a binary payload
// SessionTrigger is MD5 (in both directions)
// MaxChanges on the response is the HTTP status code
func hubWebRequest(ctx context.Context, session *HubSession, req notehubMessage, rsp *notehubMessage, event EventFunc) (err error) {
	if fnHubWebRequest == nil {
		return fmt.Errorf("no web request handler has been set " + note.ErrHubNoHandler)
	}

	// Unpack parameters for the request
	alias := req.NoteID
	if alias == "" {
		return fmt.Errorf("web request requires the alias for the notehub route to be used")
	}
	reqtype := req.NotefileID
	if reqtype == "" {
		return fmt.Errorf("web request requires a method")
	}
	reqcontent := req.MotionOrientation
	reqmaxbytes := int(req.MotionSecs)
	target := req.NotefileIDs
	totalPayloadLen := int(req.HighPowerSecsTotal)
	payloadOffset := int(req.HighPowerSecsData)

	// Unpack the notefile that contains the special note
	notefile, err := req.GetNotefile()
	if err != nil {
		return err
	}
	specialNoteID := "web"
	snote, err := notefile.GetNote(specialNoteID)
	if err != nil {
		return err
	}
	notefile.Close()

	// If the payload isn't in the note, try the binary payload area in the wire protocol,
	// as this is where "large IO" objects are optionally uploaded
	if len(snote.Payload) == 0 {
		snote.Payload = req.GetPayload()
	}

	// If there is no payload, use "offset" for byte range operations (along with "max" for length)
	reqoffset := int(0)
	if len(snote.Payload) == 0 {
		reqoffset = int(payloadOffset)
		payloadOffset = 0
	}

	// Decompress the payload if requested to do so.  Note that we tunnel this indication through
	// the SessionIDMismatch flag which was a convenient bool in the protobuf.
	if req.SessionIDMismatch {
		snote.Payload, err = snappy.Decode(nil, snote.Payload)
		if err != nil {
			return err
		}
	}

	// If an MD5 field is present, verify the payload's consistency
	if len(snote.Payload) > 0 && req.SessionTrigger != "" {
		if !strings.EqualFold(req.SessionTrigger, fmt.Sprintf("%x", md5.Sum(snote.Payload))) {
			return fmt.Errorf("payload was corrupted on the network in transit from notecard to notehub " + note.ErrWebPayload)
		}
	}

	// If a segmented payload upload has been requested (as indicated by HighPowerSecsTotal in wire protocol)
	if totalPayloadLen == 0 {
		// If not requesting a segmented upload, clear any pending retained payload
		session.PendingWebPayload = []byte{}
	} else if len(snote.Payload) > 0 {

		// It would be absurd for a developer to be sending this much data, but in the
		// spirit of defensive coding let's make sure they don't do crazy things.
		maxPayloadLen := 10000000
		if totalPayloadLen > maxPayloadLen {
			session.PendingWebPayload = []byte{}
			return fmt.Errorf("segmented payloads are limited to %d bytes (%d requested) ", maxPayloadLen, totalPayloadLen)
		}

		// If this is the first segment, clear out the pending payload
		if payloadOffset == 0 {
			session.PendingWebPayload = []byte{}
		}

		// Validate that the payload segments are arriving in-order as indicated by offset
		if payloadOffset != len(session.PendingWebPayload) {
			actual := len(session.PendingWebPayload)
			session.PendingWebPayload = []byte{}
			return fmt.Errorf("segmented payloads must be uploaded in exact order (%d already uploaded but offset is %d) "+note.ErrWebPayload, actual, payloadOffset)
		}

		// Append this chunk to the pending web payload
		session.PendingWebPayload = append(session.PendingWebPayload, snote.Payload...)

		// If we still haven't received the entire payload, just return with no error and no response
		if len(session.PendingWebPayload) < totalPayloadLen {
			rsp.MaxChanges = http.StatusContinue
			notefile = CreateNotefile(false)
			snote, _ = note.CreateNote(nil, nil)
			_, err = notefile.AddNote(req.DeviceEndpointID, specialNoteID, snote)
			if err != nil {
				err = fmt.Errorf("addNote error when pending payload: %w", err)
				return
			}
			err = rsp.SetNotefile(notefile)
			if err != nil {
				err = fmt.Errorf("setNotefile error when pending payload: %w", err)
				return
			}
			return
		}

		// If we've received too much, indicate so
		if len(session.PendingWebPayload) > totalPayloadLen {
			actual := len(session.PendingWebPayload)
			session.PendingWebPayload = []byte{}
			return fmt.Errorf("too much total segmented data received (%d actual, %d expected) "+note.ErrWebPayload, actual, totalPayloadLen)
		}

		// We've completed assembling the payload, so proceed with the web request
		snote.Payload = session.PendingWebPayload
		session.PendingWebPayload = []byte{}

	}

	// Process web requests asynchronously if desired
	if req.SuppressResponse {
		go func() {
			_, _, _, _, err := fnHubWebRequest(ctx, session, req.DeviceUID, req.ProductUID, alias, reqtype, reqcontent, reqoffset, reqmaxbytes, target, snote.GetBody(), snote.Payload)
			if err != nil {
				logInfo(ctx, "error during async web request: %s", err)
			}
		}()
		return nil
	}

	// Perform the web request.  If an error occurs, place the result in rsp.Error but
	// continue processing the transaction.
	statuscode, rspHeader, rspBodyJSON, rspPayload, err := fnHubWebRequest(ctx, session, req.DeviceUID, req.ProductUID, alias, reqtype, reqcontent, reqoffset, reqmaxbytes, target, snote.GetBody(), snote.Payload)
	if err != nil {
		rsp.Error = fmt.Sprintf("%s", err)
	}

	// If there's a payload, return its MD5 to the caller so the device can check for corruption
	if len(rspPayload) > 0 {
		rsp.SessionTrigger = fmt.Sprintf("%x", md5.Sum(rspPayload))
	}

	// For byte range requests, return the total bytes in the file.  The
	// returned header field looks like Content-Range: bytes 0-1023/21450915
	totalBytes := 0
	contentRange, present := rspHeader["Content-Range"]
	if present {
		firstContentRangeComponents := strings.Split(contentRange[0], "/")
		if len(firstContentRangeComponents) >= 2 {
			totalBytes, _ = strconv.Atoi(firstContentRangeComponents[1])
		}
	}
	rsp.MaxChanges = int32(statuscode)
	rsp.HighPowerSecsTotal = uint32(totalBytes)

	// Place the web response back into the special notefile.  If the device specifies that it would
	// prefer to receive it as binary in the payload buffer, move it there.
	notefile = CreateNotefile(false)
	if req.HighPowerSecsGPS == 0 {
		// Create the note with both the body and payload
		snote, err = note.CreateNote(rspBodyJSON, rspPayload)
		if err != nil {
			return err
		}
	} else {
		// Add the payload to the response message
		rsp.SetPayload(rspPayload)
		// Create the note with only the body
		snote, err = note.CreateNote(rspBodyJSON, nil)
		if err != nil {
			return err
		}
	}

	// Add the note to the notefile and add it to the response message
	_, err = notefile.AddNote(req.DeviceEndpointID, specialNoteID, snote)
	if err != nil {
		return err
	}
	err = rsp.SetNotefile(notefile)
	if err != nil {
		return err
	}

	// Done
	return
}

// Perform Signal
func hubSignal(ctx context.Context, session *HubSession, req notehubMessage, rsp *notehubMessage, event EventFunc) (err error) {
	if fnHubSignal == nil {
		return fmt.Errorf("no signal request handler has been set " + note.ErrHubNoHandler)
	}

	// Unpack the notefile that contains the special note
	notefile, err := req.GetNotefile()
	if err != nil {
		return err
	}
	specialNoteID := "signal"
	snote, err := notefile.GetNote(specialNoteID)
	if err != nil {
		return err
	}
	notefile.Close()

	// Enqueue the signal
	err = fnHubSignal(req.DeviceUID, snote.GetBody(), snote.Payload, session)

	// Done
	return
}

// Get Notification message
func hubGetNotification(ctx context.Context, session *HubSession, msg notehubMessage, rsp *notehubMessage, event EventFunc) (err error) {
	// These conditions should never happen
	if fnHubNotifications == nil {
		return fmt.Errorf("no notification handler has been set " + note.ErrHubNoHandler)
	}
	if !session.Notification {
		return fmt.Errorf("this transaction is not allowed on normal sessions used for request I/O")
	}
	if session.DeviceMonitorID == 0 {
		return fmt.Errorf("device monitoring is not active")
	}

	// Get the changes pending
	changes := fnHubNotifications(ctx, msg.DeviceEndpointID, session)

	// Turn the changes into a payload
	payload := []byte(strings.Join(changes, "\n"))
	if len(payload) > 0 {

		// Add the payload to a special notefile
		var snote note.Note
		snote, err = note.CreateNote(nil, payload)
		if err == nil {
			notefile := CreateNotefile(false)
			_, err = notefile.AddNote(msg.DeviceEndpointID, "signal", snote)
			if err == nil {
				err = rsp.SetNotefile(notefile)
			}
		}

	}

	return
}

// Discover request processing
func hubDiscovery(ctx context.Context, session *HubSession, msg notehubMessage, rsp *notehubMessage, event EventFunc) (err error) {
	// Get discovery info from the server via callback, including the appropriate certificate.  Note that
	// this is the SECOND discovery request made during processing of the discovery message, and if the
	// handlers had to be assigned they would've been assigned in the previous call above.
	discinfo, err2 := hubProcessDiscoveryRequest(msg.DeviceUID, msg.DeviceSN, msg.ProductUID, msg.HubSessionHandler, msg.HubPacketHandler)
	if err2 != nil {
		return err2
	}

	// Return info about hub
	rsp.HubTimeNs = discinfo.HubTimeNs
	rsp.HubEndpointID = discinfo.HubEndpointID
	rsp.HubPacketHandler = discinfo.HubPacketHandler
	rsp.HubSessionHandler = discinfo.HubSessionHandler
	rsp.HubSessionTicket = discinfo.HubSessionTicket
	rsp.HubSessionTicketExpiresTimeSec = discinfo.HubSessionTicketExpiresTimeNs / 1000000000

	// Optionally perform server certificate rotation for the client.  The client requests this by
	// setting Since to the CRC32B (IEEE CRC32) of the certificate.
	if msg.Since != 0 && len(discinfo.HubCert) != 0 {

		// Rotate the certificate only if it has changed
		clientCertCRC32 := uint32(msg.Since)
		serviceCertCRC32 := crc32.ChecksumIEEE(discinfo.HubCert)
		if clientCertCRC32 != serviceCertCRC32 {

			logDebug(ctx, "PERFORMING CERTIFICATE ROTATION")

			newNotefile := CreateNotefile(false)
			xnote, err := note.CreateNote(nil, discinfo.HubCert)
			if err == nil {
				noteID := "result"
				_, err = newNotefile.AddNote(discinfo.HubEndpointID, noteID, xnote)
				if err == nil {
					if rsp.SetNotefile(newNotefile) == nil {
						rsp.NoteID = noteID
					}
				}
			}

		}

	}

	// Done
	return nil
}

// Notebox Changes
func hubNoteboxChanges(ctx context.Context, session *HubSession, req notehubMessage, rsp *notehubMessage, event EventFunc) (err error) {
	// Open the box
	box, _, err2 := openHubNoteboxForDevice(ctx, session, req.DeviceUID, req.DeviceSN, req.ProductUID, req.DeviceEndpointID, event)
	if err2 != nil {
		err = err2
		return
	}

	// Get the tracked changes
	chgfile, _, totalChanges, _, since, until, err4 := box.GetChanges(req.DeviceEndpointID, defaultMaxGetNoteboxChangesBatchSize)
	if err4 != nil {
		err = err4
		box.Close(ctx)
		return
	}

	// Return the results
	rsp.Since = since
	rsp.Until = until
	rsp.MaxChanges = int32(totalChanges)
	err = rsp.SetNotefile(chgfile)

	// Done
	box.Close(ctx)

	return
}

// Notebox Merge
func hubNoteboxMerge(ctx context.Context, session *HubSession, req notehubMessage, rsp *notehubMessage, event EventFunc) (err error) {
	// Open the box
	box, _, err2 := openHubNoteboxForDevice(ctx, session, req.DeviceUID, req.DeviceSN, req.ProductUID, req.DeviceEndpointID, event)
	if err2 != nil {
		err = err2
		return
	}

	// Unmarshal the notefile
	notefile, err4 := req.GetNotefile()
	if err4 != nil {
		err = err4
		box.Close(ctx)
		return
	}

	// Merge the tracked changes
	err = box.MergeNotebox(ctx, notefile)
	if err != nil {
		box.Close(ctx)
		return err
	}

	// Update the list of notefiles
	notefiles := box.Notefiles(false)
	for _, notefileID := range notefiles {
		sawNotefile(session, box, notefileID)
	}

	// Done
	box.Close(ctx)

	return nil
}

// Update the tracker
func hubNoteboxUpdateChangeTracker(ctx context.Context, session *HubSession, req notehubMessage, rsp *notehubMessage, event EventFunc) (err error) {
	// Open the box
	box, _, err2 := openHubNoteboxForDevice(ctx, session, req.DeviceUID, req.DeviceSN, req.ProductUID, req.DeviceEndpointID, event)
	if err2 != nil {
		err = err2
		return
	}

	// Merge the tracked changes
	err = box.UpdateChangeTracker(req.DeviceEndpointID, req.Since, req.Until)
	if err != nil {
		box.Close(ctx)
		return err
	}

	// Done
	box.Close(ctx)

	return nil
}

// Notefile Changes
func hubNotefileChanges(ctx context.Context, session *HubSession, req notehubMessage, rsp *notehubMessage, event EventFunc) (err error) {
	// Open the box
	box, _, err2 := openHubNoteboxForDevice(ctx, session, req.DeviceUID, req.DeviceSN, req.ProductUID, req.DeviceEndpointID, event)
	if err2 != nil {
		err = err2
		return
	}

	// Open the specified notefile
	openfile, file, err4 := box.OpenNotefile(ctx, req.NotefileID)
	if err4 != nil {
		err = err4
		box.Close(ctx)
		return
	}
	sawNotefile(session, box, req.NotefileID)

	// Get the tracked changes
	chgfile, _, totalChanges, _, since, until, err5 := file.GetChanges(req.DeviceEndpointID, true, int(req.MaxChanges))
	if err5 != nil {
		err = err5
		openfile.Close(ctx)
		box.Close(ctx)
		return
	}

	// Return the results
	rsp.Since = since
	rsp.Until = until
	rsp.MaxChanges = int32(totalChanges)
	err = rsp.SetNotefile(chgfile)

	// Done
	openfile.Close(ctx)
	box.Close(ctx)

	return err
}

// Notefile Merge
func hubNotefileMerge(ctx context.Context, session *HubSession, req notehubMessage, rsp *notehubMessage, event EventFunc) (err error) {
	// Open the box
	box, appUID, err2 := openHubNoteboxForDevice(ctx, session, req.DeviceUID, req.DeviceSN, req.ProductUID, req.DeviceEndpointID, event)
	if err2 != nil {
		err = err2
		return
	}

	// Open the specified notefile
	openfile, file, err4 := box.OpenNotefile(ctx, req.NotefileID)
	if err4 != nil {
		err = err4
		box.Close(ctx)
		return
	}
	sawNotefile(session, box, req.NotefileID)

	// Set the notification context
	file.SetEventInfo(req.DeviceUID, req.DeviceSN, req.ProductUID, appUID, event, session)

	// Unmarshal the notefile
	notefile, err5 := req.GetNotefile()
	if err5 != nil {
		err = err5
		openfile.Close(ctx)
		box.Close(ctx)
		return
	}

	// Merge the tracked changes
	err = file.MergeNotefile(notefile)
	if err != nil {
		openfile.Close(ctx)
		box.Close(ctx)
		return err
	}

	// Purge tombstones from this file, since much of what was replicated inward likely
	// had been tombstones, some of which may no longer be needed.
	file.PurgeTombstones(note.DefaultHubEndpointID)

	// Done
	openfile.Close(ctx)
	box.Close(ctx)

	return nil
}

// Multi-notefile Merge
func hubNotefilesMerge(ctx context.Context, session *HubSession, req notehubMessage, rsp *notehubMessage, event EventFunc) (err error) {
	// Open the box
	box, appUID, err2 := openHubNoteboxForDevice(ctx, session, req.DeviceUID, req.DeviceSN, req.ProductUID, req.DeviceEndpointID, event)
	if err2 != nil {
		err = err2
		return
	}

	// Unmarshal the set of notefiles
	fileset, err4 := req.GetNotefiles()
	if err4 != nil {
		err = err4
		return
	}

	// Loop over the incoming notefiles
	for NotefileID, chgfile := range fileset {

		// Open the specified notefile
		openfile, file, err5 := box.OpenNotefile(ctx, NotefileID)
		if err5 != nil {
			err = err5
			box.Close(ctx)
			return
		}
		sawNotefile(session, box, NotefileID)

		// Set the notification context
		file.SetEventInfo(req.DeviceUID, req.DeviceSN, req.ProductUID, appUID, event, session)

		// Merge the tracked changes
		err = file.MergeNotefile(chgfile)
		if err != nil {
			openfile.Close(ctx)
			box.Close(ctx)
			return err
		}

		// Purge tombstones from this file, since much of what was replicated inward likely
		// had been tombstones, some of which may no longer be needed.
		file.PurgeTombstones(note.DefaultHubEndpointID)

		// Done
		openfile.Close(ctx)

	}

	box.Close(ctx)

	return nil
}

// Update the tracker
func hubNotefileUpdateChangeTracker(ctx context.Context, session *HubSession, req notehubMessage, rsp *notehubMessage, event EventFunc) (err error) {
	// Open the box
	box, _, err2 := openHubNoteboxForDevice(ctx, session, req.DeviceUID, req.DeviceSN, req.ProductUID, req.DeviceEndpointID, event)
	if err2 != nil {
		err = err2
		return
	}

	// Open the specified notefile
	openfile, file, err4 := box.OpenNotefile(ctx, req.NotefileID)
	if err4 != nil {
		err = err4
		box.Close(ctx)
		return
	}
	sawNotefile(session, box, req.NotefileID)

	// Merge the tracked changes, also deleting tombstones
	err = file.UpdateChangeTracker(req.DeviceEndpointID, req.Since, req.Until)
	if err != nil {
		openfile.Close(ctx)
		box.Close(ctx)
		return err
	}

	// Done
	openfile.Close(ctx)
	box.Close(ctx)

	return nil
}

// Notebox Summary
func hubNoteboxSummary(ctx context.Context, session *HubSession, req notehubMessage, rsp *notehubMessage, event EventFunc) (err error) {
	// Open the box
	box, appUID, err2 := openHubNoteboxForDevice(ctx, session, req.DeviceUID, req.DeviceSN, req.ProductUID, req.DeviceEndpointID, event)
	if err2 != nil {
		err = err2
		return
	}

	// Validate the session ID, and delete all local trackers to force resync if we've gotten out of sync
	// Then, flush the notebox to ensure that if this session for this device drops, that a follow-on session
	// starting in another handler will pick up the proper sessionID so that it doesn't need to do a full sync.
	sessionIDPrev := box.Notefile().swapTrackerSessionID(req.DeviceEndpointID, req.SessionIDNext)
	if sessionIDPrev != req.SessionIDPrev {
		if debugSync {
			logDebug(ctx, "Reset trackers because of ID mismatch (was %d, expecting %d now %d)", sessionIDPrev, req.SessionIDPrev, req.SessionIDNext)
		}
		box.ClearAllTrackers(ctx, req.DeviceEndpointID)
		rsp.SessionIDMismatch = true
	}
	err2 = box.Checkpoint(ctx)
	if err2 != nil {
		logError(ctx, "Error checkpointing notebox: %v", err2)
	}

	// Set the notification context
	box.SetEventInfo(req.DeviceUID, req.DeviceSN, req.ProductUID, appUID, event, session)

	// Update the environment vars for the notebox, which may result in a changed _env.dbs
	hubUpdateEnvVars(ctx, box, req.DeviceUID, appUID, true)

	// Get the info
	fileChanges := box.GetChangedNotefiles(ctx, req.DeviceEndpointID)

	// In an attempt to reduce the use of the buffer below, skim files that are obviously
	// never going to be "pulled" by the device, regardless of whether or not they were
	// technically modified on the service.  The obvious things we're trying to skim off
	// are .qo, .qos, and anything ending in 'x' which is a local-only file
	f := fileChanges
	fileChanges = []string{}
	for _, file := range f {
		isQueue, syncToHub, syncFromHub, _, _, _ := NotefileAttributesFromID(file)
		if !(isQueue && syncToHub) && (syncToHub || syncFromHub) {
			fileChanges = append(fileChanges, file)
		}
	}

	// Return the results so long as they fit within the protocol buffer (see notehub.options).
	// This is safe because it will simply take multiple passes to sync all of these files.
	for {
		rsp.NotefileIDs = strings.Join(fileChanges, ReservedIDDelimiter)
		if len(rsp.NotefileIDs) < (250 - 10) {
			break
		}
		if len(fileChanges) == 0 {
			break
		}
		fileChanges = fileChanges[:len(fileChanges)-1]
	}

	// Done
	box.Close(ctx)

	return err
}

// Notefile AddNote, which is used only by the (undocumented) "live" note add
// NotefileIDs is overloaded with the OLC 'where' of the note, if any.
func hubNotefileAddNote(ctx context.Context, session *HubSession, req notehubMessage, rsp *notehubMessage, event EventFunc) (err error) {
	// Open the box
	box, appUID, err2 := openHubNoteboxForDevice(ctx, session, req.DeviceUID, req.DeviceSN, req.ProductUID, req.DeviceEndpointID, event)
	if err2 != nil {
		err = err2
		return
	}

	// If the notefile doesn't exist, create it
	if !box.NotefileExists(req.NotefileID) {
		err = box.AddNotefile(ctx, req.NotefileID, nil)
		if err != nil {
			box.Close(ctx)
			return
		}
	}

	// Open the specified notefile
	openfile, file, err4 := box.OpenNotefile(ctx, req.NotefileID)
	if err4 != nil {
		err = err4
		box.Close(ctx)
		return
	}
	sawNotefile(session, box, req.NotefileID)

	// Set the notification context
	file.SetEventInfo(req.DeviceUID, req.DeviceSN, req.ProductUID, appUID, event, session)

	// Perform the operation
	var body []byte
	ibody, err5 := req.GetBody()
	if err5 != nil {
		err = err5
	} else {
		body, err = note.JSONMarshal(ibody)
	}
	if err == nil {
		var newNote note.Note
		newNote, err = note.CreateNote(body, req.GetPayload())
		if err == nil {

			// Set the note's time and location
			history := newHistory(req.DeviceEndpointID, req.Until, req.NotefileIDs, req.Since, 0)
			histories := append([]note.History{}, history)
			newNote.Histories = &histories

			// Add the note to the notefile with history set up here
			_, err = file.AddNoteWithHistory(req.DeviceEndpointID, req.NoteID, newNote)
			if err == nil {

				// If this is an outbound queue, purge ALL tombstones from the
				// file that would normally have been purged during sync/merge
				isQ, isQO, _, _, _, _ := NotefileAttributesFromID(req.NotefileID)
				if isQ && isQO {
					file.PurgeTombstones("*")
				}

			}
		}
	}
	if err != nil {
		openfile.Close(ctx)
		box.Close(ctx)
		return err
	}

	// Done
	openfile.Close(ctx)
	box.Close(ctx)

	return nil
}

// Notefile DeleteNote
func hubNotefileDeleteNote(ctx context.Context, session *HubSession, req notehubMessage, rsp *notehubMessage, event EventFunc) (err error) {
	// Open the box
	box, appUID, err2 := openHubNoteboxForDevice(ctx, session, req.DeviceUID, req.DeviceSN, req.ProductUID, req.DeviceEndpointID, event)
	if err2 != nil {
		err = err2
		return
	}

	// Open the specified notefile
	openfile, file, err4 := box.OpenNotefile(ctx, req.NotefileID)
	if err4 != nil {
		err = err4
		box.Close(ctx)
		return
	}
	sawNotefile(session, box, req.NotefileID)

	// Set the notification context
	file.SetEventInfo(req.DeviceUID, req.DeviceSN, req.ProductUID, appUID, event, session)

	// Perform the operation
	err = file.DeleteNote(req.DeviceEndpointID, req.NoteID)
	if err != nil {
		openfile.Close(ctx)
		box.Close(ctx)
		return err
	}

	// Done
	openfile.Close(ctx)
	box.Close(ctx)

	return nil
}

// Notefile UpdateNote
func hubNotefileUpdateNote(ctx context.Context, session *HubSession, req notehubMessage, rsp *notehubMessage, event EventFunc) (err error) {
	// Open the box
	box, appUID, err2 := openHubNoteboxForDevice(ctx, session, req.DeviceUID, req.DeviceSN, req.ProductUID, req.DeviceEndpointID, event)
	if err2 != nil {
		err = err2
		return
	}

	// Open the specified notefile
	openfile, file, err4 := box.OpenNotefile(ctx, req.NotefileID)
	if err4 != nil {
		err = err4
		box.Close(ctx)
		return
	}
	sawNotefile(session, box, req.NotefileID)

	// Set the notification context
	file.SetEventInfo(req.DeviceUID, req.DeviceSN, req.ProductUID, appUID, event, session)

	// Perform the operation
	xnote, err5 := file.GetNote(req.NoteID)
	if err5 != nil {
		err = err5
		openfile.Close(ctx)
		box.Close(ctx)
		return
	}
	var body []byte
	ibody, err6 := req.GetBody()
	if err6 != nil {
		err = err6
	} else {
		body, err = note.JSONMarshal(ibody)
	}
	if err == nil {
		xnote.SetPayload(req.GetPayload())
		err = xnote.SetBody(body)
		if err != nil {
			err = fmt.Errorf("hubNotefileUpdateNote unable to set note body: %s", err)
		}
	}
	if err == nil {
		err = file.UpdateNote(req.DeviceEndpointID, req.NoteID, xnote)
	}

	// Done
	openfile.Close(ctx)
	box.Close(ctx)

	return err
}

// Read a file range, with message fields being used/overloaded as follows:
// NotefileIDs is the filename
// Since is the offset
// Until is the length
// NoteID is the file type
// MaxChanges is an indication (a maximum size on device) to return the binary as a binary payload
func hubReadFile(ctx context.Context, session *HubSession, req notehubMessage, rsp *notehubMessage, event EventFunc) (err error) {
	// If callback not set, this function can't function
	if fnHubReadFile == nil {
		err = fmt.Errorf("hub is lacking the capability to read an uploaded file")
		return
	}

	// Open the box
	box, appUID, err2 := openHubNoteboxForDevice(ctx, session, req.DeviceUID, req.DeviceSN, req.ProductUID, req.DeviceEndpointID, event)
	if err2 != nil {
		err = err2
		return
	}

	// Get the request parameters, which we overload onto other fields in the protobuf
	filename := req.NotefileIDs
	offset := int32(req.Since)
	length := int32(req.Until)
	filetype := req.NoteID
	returnUncompressedBinary := req.MaxChanges != 0

	// Perform the read, and always do it compressed because the device firmware does a decompress.
	getInfo := offset == 0
	body, payload, err2 := fnHubReadFile(appUID, filetype, filename, offset, length, !returnUncompressedBinary, getInfo)
	if err2 != nil {
		err = err2
		box.Close(ctx)
		return
	}

	// If we'd like the binary returned uncompressed as the response payload, do so
	if returnUncompressedBinary {
		rsp.SetPayload(payload)
		payload = nil
	}

	// Create a note within a new notefile in order to return the result
	newNotefile := CreateNotefile(false)
	var xnote note.Note
	if getInfo {
		xnote, err = note.CreateNote(body, payload)
	} else {
		xnote, err = note.CreateNote(nil, payload)
	}
	if err != nil {
		box.Close(ctx)
		return
	}
	_, err = newNotefile.AddNote(req.DeviceEndpointID, "result", xnote)
	if err != nil {
		box.Close(ctx)
		return
	}

	// Return the results
	err = rsp.SetNotefile(newNotefile)

	// Done
	box.Close(ctx)

	return
}

// Notebox Checkpoint
func hubCheckpoint(ctx context.Context, session *HubSession, req notehubMessage, rsp *notehubMessage, event EventFunc) (err error) {
	// Open the box
	box, _, err2 := openHubNoteboxForDevice(ctx, session, req.DeviceUID, req.DeviceSN, req.ProductUID, req.DeviceEndpointID, event)
	if err2 != nil {
		err = err2
		return
	}

	// Do the checkpoint
	err = box.Checkpoint(ctx)

	// Done
	box.Close(ctx)

	return err
}

// SetTLSSupport tells the discover module that we do support TLS
func SetTLSSupport(enabled bool) {
	serverSupportsTLS = enabled
}

// TLSSupport tells the discover module that we do support TLS
func TLSSupport() bool {
	return serverSupportsTLS
}

// filterForLog removes fields from a notehub request which contain customer data and should not be logged and returns a filteredNotehubMessage for logging purposes.
func filterForLog(req notehubMessage) notehubMessage {
	req.nf = nil
	req.cf = nil
	return req
}

// Debugging function to display message name in a friendly way
func msgTypeName(msgType string) string {
	switch msgType {
	case msgSetSessionContext:
		return "SetSessionContext"
	case msgPing:
		return "Ping"
	case msgPingLegacy:
		return "PingLegacy"
	case msgGetNotification:
		return "GetNotification"
	case msgDiscover:
		return "Discover"
	case msgNoteboxSummary:
		return "NoteboxSummary"
	case msgNoteboxChanges:
		return "NoteboxChanges"
	case msgNoteboxMerge:
		return "NoteboxMerge"
	case msgNoteboxUpdateChangeTracker:
		return "NoteboxUpdateChangeTracker"
	case msgNotefileChanges:
		return "NotefileChanges"
	case msgNotefileMerge:
		return "NotefileMerge"
	case msgNotefilesMerge:
		return "MultiNotefileMerge"
	case msgNotefileUpdateChangeTracker:
		return "NotefileUpdateChangeTracker"
	case msgNotefileAddNote:
		return "NotefileAddNote"
	case msgNotefileUpdateNote:
		return "NotefileUpdateNote"
	case msgNotefileDeleteNote:
		return "NotefileDeleteNote"
	case msgReadFile:
		return "FirmwareGetByteRange"
	case msgWebRequest:
		return "WebRequest"
	case msgSignal:
		return "HubSignal"
	case msgCheckpoint:
		return "Checkpoint"
	}
	return "(" + msgType + ")"
}
