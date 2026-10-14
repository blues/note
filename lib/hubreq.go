// Copyright 2017 Blues Inc.  All rights reserved.
// Use of this source code is governed by licenses granted by the
// copyright holder including that found in the LICENSE file.

// Package notelib hubreq.go is the service-side complement to the Notehub client-side package
package notelib

import (
	"context"
	"crypto/md5"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"hash/crc32"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/blues/note-go/note"
	"github.com/blues/note-go/notehub"
	golc "github.com/google/open-location-code/go"
)

// Flag indicating TLS support on this server. Read/written from
// different goroutines, so use atomic access.
var serverSupportsTLS atomic.Bool

// Maximum field length in our protobufs if null were included in the cstring (see notehub.options)
const maxProtobufStringLength = 253

// Update the environment variables for an open notebox
func hubUpdateEnvVars(ctx context.Context, ei *NotefileEventInfo, box *Notebox, deviceUID string, appUID string, updateFeeds bool) {
	if fnNoteboxUpdateEnv != nil {
		err := fnNoteboxUpdateEnv(ctx, ei, box, deviceUID, appUID, updateFeeds)
		if err != nil {
			logDebug(ctx, "env: can't update env vars: %s", err)
		}
	}
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
	contentCopyDebug := make([]byte, len(content))
	copy(contentCopyDebug, content)
	req, wirelen, err2 := msgFromWire(ctx, content)
	if err2 != nil {
		reqtype = "indeterminate"
		err = err2
		// Important note: If you see large hex output in the log, it means that you should explore
		// why it is failing to parse. To do this, copy the hex output from the log entry and add it
		// as a new test case in wire_test.go
		session.LogWarn(ctx, "msgFromWire failed (%s) to decode %s", err, hex.EncodeToString(contentCopyDebug))
		return
	}

	// Determine if the message type indicates that we want to suppress response
	if strings.HasPrefix(req.MessageType, msgSuppressResponsePrefix) {
		req.SuppressResponse = true
		req.MessageType = strings.TrimPrefix(req.MessageType, msgSuppressResponsePrefix)
	}

	suppress = req.SuppressResponse
	reqtype = msgTypeName(req.MessageType)

	// Some transactions are so frequent that they can impact performance if constantly logged
	silentLog := false
	switch req.MessageType {
	case msgSignal, msgSocketSend, msgGetNotification:
		silentLog = true
	}

	// Start logging
	timerName := timerBegin(ctx, "hubRequest("+reqtype+")")
	defer timerEnd(ctx, timerName)

	// Warn if transactions take too long
	reqStart := time.Now()
	defer func() {
		if time.Since(reqStart) > transactionErrorDuration && req.MessageType != msgGetNotification {
			session.LogWarn(ctx, "%s %s [timewarn] excessively long request: %s", req.NotefileID, reqtype, time.Since(reqStart))
		}
	}()

	// Extract key info from the request
	filteredMsg := filterForLog(req)
	filteredJSON, _ := note.JSONMarshal(filteredMsg)
	if !silentLog {
		if reqtype == "Ping" {
			// These are extremely common for continuous sessions and never fail. They account for about 4% of the logs.
			// We only LogInfo the response, not the request.
			session.LogDebug(ctx, "Req %d (%db) %s %s", session.Transactions, wirelen, reqtype, filteredJSON)
		} else {
			session.LogInfo(ctx, "Req %d (%db) %s %s", session.Transactions, wirelen, reqtype, filteredJSON)
		}
	}

	// Authenticate the session
	if !session.Active {

		// Ensure that the device is valid.  (If it had to be provisioned it would've been before
		// we ever arrived here.)
		sessionTicket, err2 := HubDiscoverSessionTicket(req.DeviceUID, req.DeviceSN, req.ProductUID, session.AppUID)
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
			_, err = WireExtractSessionContext(ctx, content, session)
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
		rsp.Error = TruncateErrorString(err.Error(), maxProtobufStringLength)
		err = nil
	} else if !session.Active {

		session.Active = true

		// Return whether or not the socket was successfully opened.  This field will either
		// be "" if successfully opened, or the error string if it failed
		rsp.SocketAlias = session.Session.SocketConnectError

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
		logError(ctx, "msgToWire: %s", err)
		return
	}

	// Display the result
	filteredRsp := filterForLog(rsp)
	filteredJSON, _ = note.JSONMarshal(filteredRsp)
	if !silentLog {
		session.LogInfo(ctx, "Rsp %d (%db) %s in %s %s", session.Transactions, wirelen, reqtype, time.Since(reqStart), filteredJSON)
	}

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
	rsp.Error = TruncateErrorString(errorMessage, maxProtobufStringLength)

	// Convert to on-wire format
	response, wirelen, err := msgToWire(rsp)
	if err != nil {
		return
	}
	result = response

	// Display the result
	session.LogInfo(ctx, "Rsp %d (%db) Error %s", session.Transactions, wirelen, errorMessage)

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
		if notecardTimeNeedsCalibration(session) {
			rsp.HubTimeNs = time.Now().UnixNano()
		}

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

	case msgSocketSend:
		err = hubSocketSend(ctx, session, req, &rsp, event)

	}

	// If an error, return it this way
	if err != nil {
		rsp.Error = TruncateErrorString(err.Error(), maxProtobufStringLength)
	}

	return rsp
}

// Open the endpoint's notebox
func openHubNoteboxForDevice(ctx context.Context, session *HubSession, deviceUID string, deviceSN string, productUID string, endpointID string, event EventFunc) (box *Notebox, ei *NotefileEventInfo, appUID string, err error) {
	timerName := timerBegin(ctx, "openNoteboxForDevice")
	defer timerEnd(ctx, timerName)

	// Warn if this take too long; it should be a few milliseconds
	start := time.Now()
	defer func() {
		if time.Since(start) > time.Second {
			session.LogWarn(ctx, "[timewarn] excessively long time to open the notebox: %s", time.Since(start))
		}
	}()

	// Ensure that hub endpoint is available
	var hubEndpointID, deviceStorageObject string
	timerName = timerBegin(ctx, "hubDiscover")
	hubEndpointID, appUID, deviceStorageObject, err = HubDiscover(deviceUID, deviceSN, productUID, appUID)
	timerEnd(ctx, timerName)
	if err != nil {
		return
	}

	// Open the endpoint's box on behalf of the Notehub, because we can't open it directly under the device endpoint IDs
	timerName = timerBegin(ctx, "openNotebox`")
	box, err = OpenEndpointNotebox(ctx, hubEndpointID, deviceStorageObject, true)
	timerEnd(ctx, timerName)
	if err != nil {
		return
	}

	// Set the default notification context for files opened within this box instance
	ei = EventInfo(deviceUID, deviceSN, productUID, appUID, session.Session.Transport, event, session)

	// If we haven't yet enum'ed the notefiles, do so
	sawNotefile(session, box, "")

	// Done
	return
}

// Ensure that the specified notefile is in the list
func sawNotefile(session *HubSession, box *Notebox, notefileID string) {
	// First time through
	if !session.NotefilesUpdated {
		session.Notefiles = nil
		for _, file := range box.Notefiles(false) {
			if !NotefileIsLocalOnly(file) {
				session.Notefiles = append(session.Notefiles, file)
			}
		}
		session.NotefilesUpdated = true
	}

	// Exit if just loading the notefiles
	if notefileID == "" {
		return
	}
	if NotefileIsLocalOnly(notefileID) {
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
// Since has the time of the request (if not present because old firmware, the time of the special "web" note is used)
// Until is the crc16 of the binary template encoded in this transaction
// DeviceOrderingCode is the label to be used
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
		// Added 2024-12-13 to optimize on-wire traffic for HAAS
		reqtype = "POST"
	}
	reqcontent := req.MotionOrientation
	reqmaxbytes := int(req.MotionSecs)
	target := req.NotefileIDs
	label := req.DeviceOrderingCode
	totalPayloadLen := int(req.HighPowerSecsTotal)
	payloadOffset := int(req.HighPowerSecsData)

	// Unpack the notefile that contains the special note.  Note that
	// if there is no notefile in the protobuf, this still returns a
	// valid but empty notefile.
	notefile, err := req.GetNotefile()
	if err != nil {
		return err
	}

	// For newer firmware, extract the 'when' from the protobuf
	reqwhen := req.Since

	// If the binary in the payload is template-encoded, its CRC16 is in the Until field
	payloadTemplateCrc16 := uint16(uint32(req.Until))

	// Handle the case where there are no notes in the notefile,
	// which is the super-optimized case (for HAAS) where all
	// that exists on-wire is the protobuf
	specialNoteID := "web"
	specialNoteIDTemplate := "template"
	var snote, tnote note.Note
	if notefile.CountNotes(true) != 0 {
		snote, err = notefile.GetNote(specialNoteID)
		if err != nil {
			return err
		}
		if reqwhen == 0 {
			reqwhen = snote.When()
		}
		tnote, err = notefile.GetNote(specialNoteIDTemplate)
		if err == nil {
			session.BinTemplateJSON = tnote.GetBody()
			session.BinTemplateCrc16 = payloadTemplateCrc16
		}
	}
	notefile.Close()

	// If the payload isn't in the note, try the binary payload area in the wire protocol,
	// as this is where "large IO" objects are optionally uploaded
	if len(snote.Payload) == 0 {
		snote.Payload = req.GetPayload()
	}

	// The note's length is that of its body plus its payload as transmitted on the wire, or
	// for a segmented upload (whose length is only reported when complete) its total payload.
	// Unfortunately, this is not completely accurate because jcWireLen uses JSONMarshal,
	// with its randomly-ordered fields, and so it isn't going to reproduce precisely what
	// a different marshaler might have produced.  But it's close enough for our purposes.
	payloadLen := len(snote.Payload)
	if totalPayloadLen > 0 {
		payloadLen = totalPayloadLen
	}
	ctx = ContextWithWireLen(ctx, jcWireLen(snote.Body, nil)+uint32(payloadLen))

	// The payloadOffset field serves dual purpose:
	// - For GET requests without payload: byte offset for HTTP Range header
	// - For POST requests with segmented payload: offset where this segment starts
	// The callback can distinguish these cases using totalPayloadLen (0 = not segmented).
	reqoffset := payloadOffset

	// Decompress the payload if requested to do so.  Note that we tunnel this indication through
	// the SessionIDMismatch flag which was a convenient bool in the protobuf.
	if req.SessionIDMismatch {
		timerName := timerBegin(ctx, "snappyDecode")
		snote.Payload, err = snappyDecodeLimited(snote.Payload)
		timerEnd(ctx, timerName)
		if err != nil {
			return err
		}
	}

	// If an MD5 field is present, verify this segment's integrity
	if len(snote.Payload) > 0 && req.SessionTrigger != "" {
		// nosemgrep: go.lang.security.audit.crypto.use_of_weak_crypto.use-of-md5 -- no MD5 collision risk here: nothing trusts the hash, it only detects transit corruption of the payload
		if !strings.EqualFold(req.SessionTrigger, fmt.Sprintf("%x", md5.Sum(snote.Payload))) {
			return fmt.Errorf("payload was corrupted on the network in transit from notecard to notehub " + note.ErrWebPayload)
		}
	}

	// If there's a template, transfer the payload to a special "decoded" body subfield
	if payloadTemplateCrc16 != 0 {
		bulkContext, err := BulkDecodeTemplate(ctx, session.BinTemplateJSON, snote.Payload)
		if err != nil {
			return err
		}
		events := []note.Event{}
		timerName := timerBegin(ctx, "bulkDecode")
		for {
			body, payload, when, wherewhen, olc, _, success := bulkContext.BulkDecodeNextEntry(ctx)
			if !success {
				break
			}
			event := note.Event{}
			event.When = when
			if olc != "" {
				area, err := golc.Decode(olc)
				if err == nil {
					event.WhereLat, event.WhereLon = area.Center()
					event.WhereWhen = wherewhen
				}
			}
			event.Body = &body
			event.Payload = payload
			events = append(events, event)
		}
		timerEnd(ctx, timerName)
		if len(events) > 0 {
			if snote.Body == nil {
				snote.Body = map[string]interface{}{}
			}
			snote.Body["decoded"] = events
			session.LogInfo(ctx, "[webreq] %d events (packed into %d bytes) tunneled through %s", len(events), len(snote.Payload), alias)
		}
		snote.Payload = []byte{}
	}

	// Process web requests asynchronously if desired
	if req.SuppressResponse {
		go func() {
			_, _, _, _, err := fnHubWebRequest(ctx, session, req.DeviceUID, req.ProductUID, alias, reqtype, reqcontent, reqoffset, totalPayloadLen, reqmaxbytes, target, label, reqwhen, snote.GetBody(), snote.Payload)
			if err != nil {
				logInfo(ctx, "error during async web request: %s", err)
			}
		}()
		return nil
	}

	// Perform the web request.  If an error occurs, place the result in rsp.Error but
	// continue processing the transaction.
	timerName := timerBegin(ctx, "webRequest")
	statuscode, rspHeader, rspBodyJSON, rspPayload, err := fnHubWebRequest(ctx, session, req.DeviceUID, req.ProductUID, alias, reqtype, reqcontent, reqoffset, totalPayloadLen, reqmaxbytes, target, label, reqwhen, snote.GetBody(), snote.Payload)
	timerEnd(ctx, timerName)
	if err != nil {
		rsp.Error = TruncateErrorString(err.Error(), maxProtobufStringLength)
	}

	// If the callback returns StatusContinue, it means more segments are needed for a
	// segmented upload. Return an empty response with the continue status code so the
	// device knows to send the next segment.
	if statuscode == http.StatusContinue {
		rsp.MaxChanges = http.StatusContinue
		notefile = CreateNotefile(false)
		snote, _ = note.CreateNote(nil, nil)
		_, err = notefile.AddNote(ctx, nil, req.DeviceEndpointID, specialNoteID, snote)
		if err != nil {
			return fmt.Errorf("addNote error when pending payload: %w", err)
		}
		err = rsp.SetNotefile(notefile)
		if err != nil {
			return fmt.Errorf("setNotefile error when pending payload: %w", err)
		}
		return nil
	}

	// If there's a payload, return its MD5 to the caller so the device can check for corruption
	if len(rspPayload) > 0 {
		// nosemgrep: go.lang.security.audit.crypto.use_of_weak_crypto.use-of-md5 -- no MD5 collision risk here: nothing trusts the hash, it only detects transit corruption of the payload
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
	_, err = notefile.AddNote(ctx, nil, req.DeviceEndpointID, specialNoteID, snote)
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

// Perform Websock Send
func hubSocketSend(ctx context.Context, session *HubSession, req notehubMessage, rsp *notehubMessage, event EventFunc) (err error) {

	// Get the payload
	payload := req.GetPayload()

	// If there's a socket open, transmit it
	if session.SocketTransmit != nil {
		err = session.SocketTransmit(ctx, session, payload)
	}

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

	// Get the notifications pending
	notifications := fnHubNotifications(ctx, msg.DeviceEndpointID, session, event)

	// Sort the notifications into notifications and websocket binary segments to be returned as a payload
	payload := []byte{}
	signals := []string{}
	for _, notification := range notifications {
		payloadBytes, isPayload := payloadFromString(notification)
		if isPayload {
			payload = append(payload, payloadBytes...)
		} else {
			signals = append(signals, notification)
		}
	}

	// If we've got a payload, return it
	if len(payload) > 0 {
		rsp.SetPayload(payload)
	}

	// If we've got notifications, return them within a notefile
	if len(signals) > 0 {
		notePayload := []byte(strings.Join(signals, "\n"))
		if len(notePayload) > 0 {
			var snote note.Note
			snote, err = note.CreateNote(nil, notePayload)
			if err == nil {
				notefile := CreateNotefile(false)
				_, err = notefile.AddNote(ctx, nil, msg.DeviceEndpointID, "signal", snote)
				if err == nil {
					err = rsp.SetNotefile(notefile)
				}
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
				_, err = newNotefile.AddNote(ctx, nil, discinfo.HubEndpointID, noteID, xnote)
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
	box, _, _, err2 := openHubNoteboxForDevice(ctx, session, req.DeviceUID, req.DeviceSN, req.ProductUID, req.DeviceEndpointID, event)
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
	box, _, _, err2 := openHubNoteboxForDevice(ctx, session, req.DeviceUID, req.DeviceSN, req.ProductUID, req.DeviceEndpointID, event)
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
	box, _, _, err2 := openHubNoteboxForDevice(ctx, session, req.DeviceUID, req.DeviceSN, req.ProductUID, req.DeviceEndpointID, event)
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
	if NotefileIsLocalOnly(req.NotefileID) {
		return nil
	}

	// Open the box
	box, _, _, err2 := openHubNoteboxForDevice(ctx, session, req.DeviceUID, req.DeviceSN, req.ProductUID, req.DeviceEndpointID, event)
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
	if NotefileIsLocalOnly(req.NotefileID) {
		return nil
	}

	// Open the box
	box, ei, _, err2 := openHubNoteboxForDevice(ctx, session, req.DeviceUID, req.DeviceSN, req.ProductUID, req.DeviceEndpointID, event)
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

	// Unmarshal the notefile
	notefile, err5 := req.GetNotefile()
	if err5 != nil {
		err = err5
		openfile.Close(ctx)
		box.Close(ctx)
		return
	}

	// Merge the tracked changes
	err = file.MergeNotefile(ctx, ei, notefile)
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
	box, ei, _, err2 := openHubNoteboxForDevice(ctx, session, req.DeviceUID, req.DeviceSN, req.ProductUID, req.DeviceEndpointID, event)
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
		if NotefileIsLocalOnly(NotefileID) {
			continue
		}

		// Open the specified notefile
		openfile, file, err5 := box.OpenNotefile(ctx, NotefileID)
		if err5 != nil {
			err = err5
			box.Close(ctx)
			return
		}
		sawNotefile(session, box, NotefileID)

		// Merge the tracked changes
		err = file.MergeNotefile(ctx, ei, chgfile)
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
	if NotefileIsLocalOnly(req.NotefileID) {
		return nil
	}

	// Open the box
	box, _, _, err2 := openHubNoteboxForDevice(ctx, session, req.DeviceUID, req.DeviceSN, req.ProductUID, req.DeviceEndpointID, event)
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
	box, ei, appUID, err2 := openHubNoteboxForDevice(ctx, session, req.DeviceUID, req.DeviceSN, req.ProductUID, req.DeviceEndpointID, event)
	if err2 != nil {
		err = err2
		return
	}

	// Validate the session ID, and delete all local trackers to force resync if we've gotten out of sync
	// Note that this session ID will be flushed by our 5 minute checkpoint task and by normal session close.
	timerName := timerBegin(ctx, "swapTracker")
	sessionIDPrev := box.Notefile().swapTrackerSessionID(req.DeviceEndpointID, req.SessionIDNext)
	timerEnd(ctx, timerName)
	if sessionIDPrev != req.SessionIDPrev {
		if debugSync {
			logDebug(ctx, "Reset trackers because of ID mismatch (was %d, expecting %d now %d)", sessionIDPrev, req.SessionIDPrev, req.SessionIDNext)
		}
		box.ClearAllTrackers(ctx, req.DeviceEndpointID)
		rsp.SessionIDMismatch = true
	}

	// Update the environment vars for the notebox, which may result in a changed _env.dbs
	timerName = timerBegin(ctx, "updateEnvVars")
	hubUpdateEnvVars(ctx, ei, box, req.DeviceUID, appUID, true)
	timerEnd(ctx, timerName)

	// Get the info
	timerName = timerBegin(ctx, "getChangedNotefiles")
	fileChanges := box.GetChangedNotefiles(ctx, req.DeviceEndpointID)
	timerEnd(ctx, timerName)

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

	// Return the results truncated to fit within the protocol buffer (see notehub.options).
	// This is safe because it will simply take multiple passes to sync all of these files.
	rsp.NotefileIDs = fmt.Sprintf("%.*s", maxProtobufStringLength, strings.Join(fileChanges, ReservedIDDelimiter))

	// Done
	box.Close(ctx)

	// Every 12 hours, return the hub time to the device for its own clock calibration
	if notecardTimeNeedsCalibration(session) {
		rsp.HubTimeNs = time.Now().UnixNano()
	}

	return err
}

// Notefile AddNote, which is used only by the (undocumented) "live" note add
// NotefileIDs is overloaded with the OLC 'where' of the note, if any.
func hubNotefileAddNote(ctx context.Context, session *HubSession, req notehubMessage, rsp *notehubMessage, event EventFunc) (err error) {
	if NotefileIsLocalOnly(req.NotefileID) {
		return nil
	}

	// Open the box
	box, ei, _, err2 := openHubNoteboxForDevice(ctx, session, req.DeviceUID, req.DeviceSN, req.ProductUID, req.DeviceEndpointID, event)
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

	// First, extract the body and payload
	var body []byte
	bodyObject, err5 := req.GetBody()
	if err5 != nil {
		err = err5
	} else {
		body, err = note.JSONMarshal(bodyObject)
	}
	payload := req.GetPayload()

	// The note's length is that of its body and payload as they were transmitted on the wire.  Note
	// however that this isn't the entire on-wire size because it doesn't take into account the
	// actual request message itself, which is not part of the body/payload but rather which were
	// in this "req" structure and encoded on the wire as part of the notecard-notehub RPC.
	wireLen := uint32(req.GetBodyWireLen() + len(payload))

	// See if this is a special note that has a payload that was encoded with a template specified in the body
	decodePayloadWithTemplateFlag := uint32(0x00000001)
	if err == nil && len(payload) > 0 && (req.PowerSource&decodePayloadWithTemplateFlag) != 0 {

		// The template in the body isn't user data, so only the template-encoded payload is counted
		// in this type of encoding.
		wireLen = uint32(len(payload))

		// The template is in the body
		binTemplateJSON := body
		body = []byte{}
		var bulkContext BulkTemplateContext
		bulkContext, err = BulkDecodeTemplate(ctx, binTemplateJSON, payload)
		if err == nil {
			events := []note.Event{}
			for {
				noteBody, notePayload, when, wherewhen, olc, _, success := bulkContext.BulkDecodeNextEntry(ctx)
				if !success {
					break
				}
				event := note.Event{}
				event.When = when
				if olc != "" {
					area, err := golc.Decode(olc)
					if err == nil {
						event.WhereLat, event.WhereLon = area.Center()
						event.WhereWhen = wherewhen
					}
				}
				event.Body = &noteBody
				event.Payload = notePayload
				events = append(events, event)
			}
			if len(events) > 0 {
				bodyObject := map[string]interface{}{}
				bodyObject["decoded"] = events
				body, err = note.JSONMarshal(bodyObject)
			}
			payload = []byte{}
		}

	}

	// Add the note to the notefile
	if err == nil {
		var newNote note.Note
		newNote, err = note.CreateNote(body, payload)
		if err == nil {

			// Set the note's time and location
			history := newHistory(req.DeviceEndpointID, req.Until, req.NotefileIDs, req.Since, 0)
			histories := append([]note.History{}, history)
			newNote.Histories = &histories

			// Add the note to the notefile with history set up here
			ei.WireLen = wireLen
			_, err = file.AddNoteWithHistory(ctx, ei, req.DeviceEndpointID, req.NoteID, newNote)
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
	if NotefileIsLocalOnly(req.NotefileID) {
		return nil
	}

	// Open the box
	box, ei, _, err2 := openHubNoteboxForDevice(ctx, session, req.DeviceUID, req.DeviceSN, req.ProductUID, req.DeviceEndpointID, event)
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

	// Perform the operation
	err = file.DeleteNote(ctx, ei, req.DeviceEndpointID, req.NoteID)
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
	if NotefileIsLocalOnly(req.NotefileID) {
		return nil
	}

	// Open the box
	box, ei, _, err2 := openHubNoteboxForDevice(ctx, session, req.DeviceUID, req.DeviceSN, req.ProductUID, req.DeviceEndpointID, event)
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
		// The note's length is that of its body and payload as they were transmitted on the wire
		ei.WireLen = uint32(req.GetBodyWireLen() + len(req.GetPayload()))
		err = file.UpdateNote(ctx, ei, req.DeviceEndpointID, req.NoteID, xnote)
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
	box, _, appUID, err2 := openHubNoteboxForDevice(ctx, session, req.DeviceUID, req.DeviceSN, req.ProductUID, req.DeviceEndpointID, event)
	if err2 != nil {
		err = err2
		return
	}

	// Get the request parameters, which we overload onto other fields in the protobuf
	filename := req.NotefileIDs
	offset := int32(req.Since)
	length := int32(req.Until)
	filetype := notehub.ParseUploadType(req.NoteID)
	returnUncompressedBinary := req.MaxChanges != 0

	// Perform the read, and always do it compressed because the device firmware does a decompress.
	getInfo := offset == 0
	metadata, payload, err2 := fnHubReadFile(ctx, appUID, filetype, filename, offset, length, !returnUncompressedBinary)
	if err2 != nil {
		err = err2
		box.Close(ctx)
		return
	}

	// locally we have bogus firmware that's created for testing the UI.
	// if any device tries to update to this firmware, always return an error
	if metadata.Firmware != nil && metadata.Firmware.Description == notehub.TestFirmwareString {
		return fmt.Errorf("this firmware is invalid and created for testing purposes only")
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
		if bodyEncoded, err2 := note.JSONMarshal(metadata); err2 != nil {
			return err2
		} else {
			xnote, err = note.CreateNote(bodyEncoded, payload)
		}
	} else {
		xnote, err = note.CreateNote(nil, payload)
	}
	if err != nil {
		box.Close(ctx)
		return
	}
	_, err = newNotefile.AddNote(ctx, nil, req.DeviceEndpointID, "result", xnote)
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
	box, _, _, err2 := openHubNoteboxForDevice(ctx, session, req.DeviceUID, req.DeviceSN, req.ProductUID, req.DeviceEndpointID, event)
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
	serverSupportsTLS.Store(enabled)
}

// TLSSupport tells the discover module that we do support TLS
func TLSSupport() bool {
	return serverSupportsTLS.Load()
}

// filterForLog removes fields from a notehub request which contain customer data and should not be logged and returns a filteredNotehubMessage for logging purposes.
func filterForLog(req notehubMessage) notehubMessage {
	if req.MessageType == msgWebRequest {
		if req.NotefileIDs != "" {
			// Web requests store the URL path in this field. Replace them with "..." to avoid logging customer data.
			req.NotefileIDs = "..."
		}
	}
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
	case msgSocketSend:
		return "HubSocketSend"
	}
	return "(" + msgType + ")"
}

// notecardTimeNeedsCalibration checks if it's time to send the hub time to the
// notecarrd for its clock calibration. It returns true periodically and updates
// the session's LastCalibratedNotecardTime field when it does so.
func notecardTimeNeedsCalibration(session *HubSession) bool {
	const calibrationIntervalSecs = 12 * 60 * 60 // 12 hours
	now := time.Now().Unix()
	if now-session.LastCalibratedNotecardTime >= calibrationIntervalSecs {
		session.LastCalibratedNotecardTime = now
		return true
	}
	return false
}

// Truncate an error string, moving errors to the front if it's too long
func TruncateErrorString(errstr string, maxchars int) string {
	if len(errstr) <= maxchars {
		return errstr
	}
	re := regexp.MustCompile(`\{[^}]*\}`)
	matches := re.FindAllString(errstr, -1)
	if len(matches) > 0 {
		errstr = strings.TrimSpace(re.ReplaceAllString(errstr, ""))
		errstr = fmt.Sprintf("%s %s", strings.Join(matches, ""), errstr)
	}
	return fmt.Sprintf("%.*s", maxchars, errstr)
}

// Used to send []byte messages as strings in devmon
const payloadPrefix = "bytes:"

// Convert payload to a string
func PayloadToString(payload []byte) string {
	return payloadPrefix + base64.StdEncoding.EncodeToString(payload)
}

// Convert string to a payload
func payloadFromString(in string) ([]byte, bool) {
	if !strings.HasPrefix(in, payloadPrefix) {
		return []byte{}, false
	}
	payload, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(in, payloadPrefix))
	if err != nil {
		return []byte{}, false
	}
	return payload, true
}
