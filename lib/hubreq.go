// Copyright 2017 Blues Inc.  All rights reserved.
// Use of this source code is governed by licenses granted by the
// copyright holder including that found in the LICENSE file.

// Package notelib hubreq.go is the service-side complement to the Notehub client-side package
package notelib

import (
	"encoding/json"
	"fmt"
	"github.com/blues/note-go/note"
	"hash/crc32"
	"strings"
	"time"
)

// Flag indicating TLS support on this server
var serverSupportsTLS bool

// NoteboxInitFunc is the func to initialize the notebox at the start of a session
type NoteboxInitFunc func(box *Notebox) (err error)
var fnNoteboxUpdateEnv NoteboxInitFunc

// HubSetNoteboxInit sets the global notebox function to update env vars
func HubSetNoteboxInit(fn NoteboxInitFunc) {
	fnNoteboxUpdateEnv = fn
}

// Update the environment variables for an open notebox
func hubUpdateEnvVars(box *Notebox) {
	if fnNoteboxUpdateEnv != nil {
		fnNoteboxUpdateEnv(box)
	}
}

// GetNotificationFunc retrieves hub notifications to be sent to client
type GetNotificationFunc func(deviceUID string, productUID string) (notifications []string, err error)

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
type WebRequestFunc func(deviceUID string, productUID string, alias string, reqtype string, target string, bodyJSON []byte, payloadJSON []byte, session *HubSessionContext) (rspstatuscode int, rspBodyJSON []byte, rspPayloadJSON []byte, err error)

var fnHubWebRequest WebRequestFunc

// HubSetWebRequest sets the web request function
func HubSetWebRequest(fn WebRequestFunc) {
	fnHubWebRequest = fn
}

// SignalFunc performs a web request on behalf of a device
type SignalFunc func(deviceUID string, bodyJSON []byte, payloadJSON []byte, session *HubSessionContext) (err error)

var fnHubSignal SignalFunc

// HubSetSignal sets the web request function
func HubSetSignal(fn SignalFunc) {
	fnHubSignal = fn
}

// HubRequest creates a request message from a map of arguments, given an optional
// context parameter for fields that are shared across requests that happen within
// a single session.  The return arguments should be interpreted as follows:
// - if err is returned, do not return a reply to the remote requestor
// - if err is nil, always send result back to request
func HubRequest(session *HubSessionContext, content []byte, event EventFunc, context interface{}) (result []byte, err error) {

	// Preset in case of error return
	var rsp notehubMessage
	rsp.Version = currentProtocolVersion

	// Convert from on-wire text to a message
	req, wirelen, err2 := msgFromWire(content)
	if err2 != nil {
		err = err2
		return
	}

	// Display the request being processed
	if debugHubRequest {
		JSON, _ := json.Marshal(req)
		debugf("Request (%db json, %db wire) %s\n%s\n", len(JSON), wirelen, msgTypeName(req.MessageType), JSON)
	}

	// Authenticate the session
	if !session.Active {

		// Ensure that the device is valid.  (If it had to be provisioned it would've been before
		// we ever arrived here.)
		sessionTicket, _, _, _, err2 := HubDiscover(req.DeviceUID, req.DeviceSN, req.ProductUID)
		if err2 != nil {
			err = fmt.Errorf("discover error "+ErrAuth+": %s", err2)
		} else {

			if req.MessageType == msgDiscover {

				if TLSSupport() && !session.Secure {
					err = fmt.Errorf("secure session required " + ErrAuth)
					debugf("%s: attempt to discover with unsecure session\n", req.DeviceUID)
				}

			} else {

				// If the ticket isn't an exact match, then it's not a valid connect attemp
				if sessionTicket == "" || sessionTicket != req.HubSessionTicket {
					err = fmt.Errorf("session not authorized " + ErrAuth + ErrTicket)
					debugf("TICKET REJECTED for %s (server may have been restarted)\n    Assigned: %s\n   Requested: %s\n", req.DeviceUID, sessionTicket, req.HubSessionTicket)
				}
			}
		}
	}

	// Only proceed if no auth error
	if err == nil {

		// Set or supply session context as appropriate.  The client will send the data on EITHER
		// a formal "set session context" message, OR on the very first message (as an optimization)
		if !session.Active {
			WireExtractSessionContext(content, session)
		} else {
			req.DeviceUID = session.DeviceUID
			req.DeviceSN = session.DeviceSN
			req.ProductUID = session.ProductUID
			req.DeviceEndpointID = session.DeviceEndpointID
			req.HubEndpointID = session.HubEndpointID
			req.HubSessionTicket = session.HubSessionTicket
			req.UsageProvisioned = session.Session.This.Since
			req.UsageRcvdBytes = session.Session.This.RcvdBytes
			req.UsageSentBytes = session.Session.This.SentBytes
			req.UsageTCPSessions = session.Session.This.TCPSessions
			req.UsageTLSSessions = session.Session.This.TLSSessions
			req.UsageRcvdNotes = session.Session.This.RcvdNotes
			req.UsageSentNotes = session.Session.This.SentNotes
			req.CellID = session.Session.CellID
			req.NotificationSession = session.Notification
			req.Development = session.Session.Development
		}

		// If there is a null session ticket, the only request that's permitted is a Discover request.
		if req.HubSessionTicket == "" && req.MessageType != msgDiscover {
			err = fmt.Errorf("transaction is not allowed "+ErrAuth+" without a session ticket: %s", req.MessageType)
		}

		// Process it and generate the return results
		if err == nil {
			rsp = processRequest(session, req, event, context)
		}

	}

	// After the first request has been successfully executed, mark it as active, and piggyback the session info
	if err != nil {
		rsp.Error = fmt.Sprintf("%s", err)
		err = nil
	} else if !session.Active {

		session.Active = true

		// Return info to the client about itself
		if session.Session.Tower.OLC != "" {

			// Load the location to compute cell offset, if possible
			shortZone := ""
			offsetSecondsEastOfUTC := 0
			location, err := time.LoadLocation(session.Session.Tower.TimeZone)
			if err != nil {
				fmt.Printf("*** Can't load location for: %s\n", session.Session.Tower.TimeZone)
			} else {
				localTime := time.Now().In(location)
				shortZone, offsetSecondsEastOfUTC = localTime.Zone()
			}

			// Return everything packed into the CellID field
			rsp.CellID = session.Session.Tower.OLC
			rsp.CellID += "|" + session.Session.Tower.CountryCode
			rsp.CellID += "|" + session.Session.Tower.TimeZone
			rsp.CellID += "|" + shortZone
			if shortZone != "" {
				rsp.CellID += "|" + fmt.Sprintf("%d", offsetSecondsEastOfUTC/60)
			} else {
				rsp.CellID += "|"
			}
			rsp.CellID += "|" + session.Session.Tower.Name

		}

	}

	// Convert back to on-wire format
	result, wirelen, err = msgToWire(rsp)
	if err != nil {
		return
	}

	// Display the result
	if debugHubRequest {
		JSON, _ := json.Marshal(rsp)
		debugf("Response (%db json, %db wire)\n%s\n", len(JSON), wirelen, JSON)
	}

	// Done
	return
}

// processRequest processes a request message
func processRequest(session *HubSessionContext, req notehubMessage, event EventFunc, context interface{}) (response notehubMessage) {
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
		// This is a nil transaction

	case msgPingLegacy:
		// This is a nil transaction

	case msgGetNotification:
		err = hubGetNotification(session, req, &rsp, event, context)

	case msgDiscover:
		err = hubDiscovery(session, req, &rsp, event, context)

	case msgNoteboxSummary:
		err = hubNoteboxSummary(session, req, &rsp, event, context)

	case msgNoteboxChanges:
		err = hubNoteboxChanges(session, req, &rsp, event, context)

	case msgNoteboxMerge:
		err = hubNoteboxMerge(session, req, &rsp, event, context)

	case msgNoteboxUpdateChangeTracker:
		err = hubNoteboxUpdateChangeTracker(session, req, &rsp, event, context)

	case msgNotefileChanges:
		err = hubNotefileChanges(session, req, &rsp, event, context)

	case msgNotefileMerge:
		err = hubNotefileMerge(session, req, &rsp, event, context)

	case msgNotefilesMerge:
		err = hubNotefilesMerge(session, req, &rsp, event, context)

	case msgNotefileUpdateChangeTracker:
		err = hubNotefileUpdateChangeTracker(session, req, &rsp, event, context)

	case msgNotefileAddNote:
		err = hubNotefileAddNote(session, req, &rsp, event, context)

	case msgNotefileDeleteNote:
		err = hubNotefileDeleteNote(session, req, &rsp, event, context)

	case msgNotefileUpdateNote:
		err = hubNotefileUpdateNote(session, req, &rsp, event, context)

	case msgReadFile:
		err = hubReadFile(session, req, &rsp, event, context)

	case msgWebRequest:
		err = hubWebRequest(session, req, &rsp, event, context)

	case msgSignal:
		err = hubSignal(session, req, &rsp, event, context)

	}

	// If an error, return it this way
	if err != nil {
		rsp.Error = fmt.Sprintf("%s", err)
	}

	return rsp

}

// Open the endpoint's notebox
func openHubNoteboxForDevice(session *HubSessionContext, deviceUID string, deviceSN string, productUID string, endpointID string, event EventFunc, context interface{}) (box *Notebox, appUID string, err error) {

	// Ensure that hub endpoint is available
	var hubEndpointID, deviceStorageObject string
	_, hubEndpointID, appUID, deviceStorageObject, err = HubDiscover(deviceUID, deviceSN, productUID)
	if err != nil {
		return
	}

	// Open the endpoint's box on behalf of the Notehub, because we can't open it directly under the device endpoint IDs
	box, err = OpenEndpointNotebox(hubEndpointID, deviceStorageObject, true)
	if err != nil {
		return
	}

	// Set the default notification context for files opened within this box instance
	box.SetEventInfo(deviceUID, deviceSN, productUID, appUID, event, context)

	// Done
	return

}

// Perform Web Request
func hubWebRequest(session *HubSessionContext, req notehubMessage, rsp *notehubMessage, event EventFunc, context interface{}) (err error) {

	if fnHubWebRequest == nil {
		return fmt.Errorf("no web request handler has been set " + ErrHubNoHandler)
	}

	// Unpack parameters for the request
	alias := req.NoteID
	if alias == "" {
		return fmt.Errorf("web request requires the alias for the notehub route to be used")
	}
	reqtype := req.NotefileID
	if reqtype == "" {
		return fmt.Errorf("web request requires POST, PUT, or GET")
	}
	target := req.NotefileIDs

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

	// Perform the web request
	statuscode, rspBodyJSON, rspPayload, err := fnHubWebRequest(req.DeviceUID, req.ProductUID, alias, reqtype, target, snote.GetBody(), snote.Payload, session)

	if err != nil {
		rspBodyJSON = []byte(fmt.Sprintf("{\"err\":\"%s\"}", err))
	}

	// Place the web response back into the special notefile
	notefile = CreateNotefile(false)
	snote, err = note.CreateNote(rspBodyJSON, rspPayload)
	if err != nil {
		return err
	}

	// Add the note to the notefile
	err = notefile.AddNote(req.DeviceEndpointID, specialNoteID, snote)
	if err != nil {
		return err
	}
	rsp.MaxChanges = int32(statuscode)
	err = rsp.SetNotefile(notefile)
	if err != nil {
		return err
	}

	// Done
	return

}

// Perform Signal
func hubSignal(session *HubSessionContext, req notehubMessage, rsp *notehubMessage, event EventFunc, context interface{}) (err error) {

	if fnHubSignal == nil {
		return fmt.Errorf("no signal request handler has been set " + ErrHubNoHandler)
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
func hubGetNotification(session *HubSessionContext, msg notehubMessage, rsp *notehubMessage, event EventFunc, context interface{}) (err error) {
	if fnHubNotifications == nil {
		return fmt.Errorf("no notification handler has been set " + ErrHubNoHandler)
	}
	if !session.Notification {
		return fmt.Errorf("this transaction is not allowed on normal sessions used for request I/O")
	}
	var changes []string
	changes, err = fnHubNotifications(msg.DeviceUID, msg.ProductUID)
	if err != nil {
		return
	}

	// Turn the changes into a payload
	payload := []byte(strings.Join(changes, "\n"))
	if len(payload) > 0 {

		// Add the payload to a special notefile
		var snote note.Note
		snote, err = note.CreateNote(nil, payload)
		if err == nil {
			notefile := CreateNotefile(false)
			err = notefile.AddNote(msg.DeviceEndpointID, "signal", snote)
			if err == nil {
				err = rsp.SetNotefile(notefile)
			}
		}

	}

	return
}

// Discover request processing
func hubDiscovery(session *HubSessionContext, msg notehubMessage, rsp *notehubMessage, event EventFunc, context interface{}) (err error) {

	// Get discovery info from the server via callback, including the appropriate certificate.  Note that
	// this is the SECOND discovery request made during processing of the discovery message, and if the
	// handlers had to be assigned they would've been assigned in the previous call above.
	discinfo, err2 := hubProcessDiscoveryRequest(msg.DeviceUID, msg.DeviceSN, msg.ProductUID, msg.HubSessionHandler)
	if err2 != nil {
		return err2
	}

	// Return info about hub
	rsp.HubTimeNs = discinfo.HubTimeNs
	rsp.HubEndpointID = discinfo.HubEndpointID
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

			debugf("PERFORMING CERTIFICATE ROTATION\n")

			newNotefile := CreateNotefile(false)
			xnote, err := note.CreateNote(nil, discinfo.HubCert)
			if err == nil {
				noteID := "result"
				err = newNotefile.AddNote(discinfo.HubEndpointID, noteID, xnote)
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
func hubNoteboxChanges(session *HubSessionContext, req notehubMessage, rsp *notehubMessage, event EventFunc, context interface{}) (err error) {

	// Open the box
	box, _, err2 := openHubNoteboxForDevice(session, req.DeviceUID, req.DeviceSN, req.ProductUID, req.DeviceEndpointID, event, context)
	if err2 != nil {
		err = err2
		return
	}

	// Get the tracked changes
	chgfile, _, totalChanges, since, until, err4 := box.GetChanges(req.DeviceEndpointID, defaultMaxGetNoteboxChangesBatchSize)
	if err4 != nil {
		err = err4
		box.Close()
		return
	}

	// Return the results
	rsp.Since = since
	rsp.Until = until
	rsp.MaxChanges = int32(totalChanges)
	err = rsp.SetNotefile(chgfile)

	// Done
	box.Close()

	return
}

// Notebox Merge
func hubNoteboxMerge(session *HubSessionContext, req notehubMessage, rsp *notehubMessage, event EventFunc, context interface{}) (err error) {

	// Open the box
	box, _, err2 := openHubNoteboxForDevice(session, req.DeviceUID, req.DeviceSN, req.ProductUID, req.DeviceEndpointID, event, context)
	if err2 != nil {
		err = err2
		return
	}

	// Unmarshal the notefile
	notefile, err4 := req.GetNotefile()
	if err4 != nil {
		err = err4
		box.Close()
		return
	}

	// Merge the tracked changes
	err = box.MergeNotebox(notefile)
	if err != nil {
		box.Close()
		return err
	}

	// Done
	box.Close()

	return nil
}

// Update the tracker
func hubNoteboxUpdateChangeTracker(session *HubSessionContext, req notehubMessage, rsp *notehubMessage, event EventFunc, context interface{}) (err error) {

	// Open the box
	box, _, err2 := openHubNoteboxForDevice(session, req.DeviceUID, req.DeviceSN, req.ProductUID, req.DeviceEndpointID, event, context)
	if err2 != nil {
		err = err2
		return
	}

	// Merge the tracked changes
	err = box.UpdateChangeTracker(req.DeviceEndpointID, req.Since, req.Until)
	if err != nil {
		box.Close()
		return err
	}

	// Done
	box.Close()

	return nil
}

// Notefile Changes
func hubNotefileChanges(session *HubSessionContext, req notehubMessage, rsp *notehubMessage, event EventFunc, context interface{}) (err error) {

	// Open the box
	box, _, err2 := openHubNoteboxForDevice(session, req.DeviceUID, req.DeviceSN, req.ProductUID, req.DeviceEndpointID, event, context)
	if err2 != nil {
		err = err2
		return
	}

	// Open the specified notefile
	openfile, file, err4 := box.OpenNotefile(req.NotefileID)
	if err4 != nil {
		err = err4
		box.Close()
		return
	}

	// Get the tracked changes
	chgfile, _, totalChanges, since, until, err5 := file.GetChanges(req.DeviceEndpointID, int(req.MaxChanges))
	if err5 != nil {
		err = err5
		openfile.Close()
		box.Close()
		return
	}

	// Return the results
	rsp.Since = since
	rsp.Until = until
	rsp.MaxChanges = int32(totalChanges)
	err = rsp.SetNotefile(chgfile)

	// Done
	openfile.Close()
	box.Close()

	return err
}

// Notefile Merge
func hubNotefileMerge(session *HubSessionContext, req notehubMessage, rsp *notehubMessage, event EventFunc, context interface{}) (err error) {

	// Open the box
	box, appUID, err2 := openHubNoteboxForDevice(session, req.DeviceUID, req.DeviceSN, req.ProductUID, req.DeviceEndpointID, event, context)
	if err2 != nil {
		err = err2
		return
	}

	// Open the specified notefile
	openfile, file, err4 := box.OpenNotefile(req.NotefileID)
	if err4 != nil {
		err = err4
		box.Close()
		return
	}

	// Set the notification context
	file.SetEventInfo(req.DeviceUID, req.DeviceSN, req.ProductUID, appUID, event, context)

	// Unmarshal the notefile
	notefile, err5 := req.GetNotefile()
	if err5 != nil {
		err = err5
		openfile.Close()
		box.Close()
		return
	}

	// Merge the tracked changes
	err = file.MergeNotefile(notefile)
	if err != nil {
		openfile.Close()
		box.Close()
		return err
	}

	// Purge tombstones from this file, since much of what was replicated inward likely
	// had been tombstones, some of which may no longer be needed.
	file.PurgeTombstones(box.endpointID)

	// Done
	openfile.Close()
	box.Close()

	return nil
}

// Multi-notefile Merge
func hubNotefilesMerge(session *HubSessionContext, req notehubMessage, rsp *notehubMessage, event EventFunc, context interface{}) (err error) {

	// Open the box
	box, appUID, err2 := openHubNoteboxForDevice(session, req.DeviceUID, req.DeviceSN, req.ProductUID, req.DeviceEndpointID, event, context)
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
		openfile, file, err5 := box.OpenNotefile(NotefileID)
		if err5 != nil {
			err = err5
			box.Close()
			return
		}

		// Set the notification context
		file.SetEventInfo(req.DeviceUID, req.DeviceSN, req.ProductUID, appUID, event, context)

		// Merge the tracked changes
		err = file.MergeNotefile(chgfile)
		if err != nil {
			openfile.Close()
			box.Close()
			return err
		}

		// Purge tombstones from this file, since much of what was replicated inward likely
		// had been tombstones, some of which may no longer be needed.
		file.PurgeTombstones(box.endpointID)

		// Done
		openfile.Close()

	}

	box.Close()

	return nil
}

// Update the tracker
func hubNotefileUpdateChangeTracker(session *HubSessionContext, req notehubMessage, rsp *notehubMessage, event EventFunc, context interface{}) (err error) {

	// Open the box
	box, _, err2 := openHubNoteboxForDevice(session, req.DeviceUID, req.DeviceSN, req.ProductUID, req.DeviceEndpointID, event, context)
	if err2 != nil {
		err = err2
		return
	}

	// Open the specified notefile
	openfile, file, err4 := box.OpenNotefile(req.NotefileID)
	if err4 != nil {
		err = err4
		box.Close()
		return
	}

	// Merge the tracked changes, also deleting tombstones
	err = file.UpdateChangeTracker(req.DeviceEndpointID, req.Since, req.Until)
	if err != nil {
		openfile.Close()
		box.Close()
		return err
	}

	// Done
	openfile.Close()
	box.Close()

	return nil

}

// Notebox Summary
func hubNoteboxSummary(session *HubSessionContext, req notehubMessage, rsp *notehubMessage, event EventFunc, context interface{}) (err error) {

	// Open the box
	box, _, err2 := openHubNoteboxForDevice(session, req.DeviceUID, req.DeviceSN, req.ProductUID, req.DeviceEndpointID, event, context)
	if err2 != nil {
		err = err2
		return
	}

	// Validate the session ID, and delete all local trackers to force resync if we've gotten out of sync
	sessionIDPrev := box.Notefile().swapTrackerSessionID(req.DeviceEndpointID, req.SessionIDNext)
	if sessionIDPrev != req.SessionIDPrev {
		if debugSync {
			debugf("Reset trackers because of ID mismatch (was %d, expecting %d now %d)\n", sessionIDPrev, req.SessionIDPrev, req.SessionIDNext)
		}
		box.ClearAllTrackers(req.DeviceEndpointID)
		rsp.SessionIDMismatch = true
	}

	// Update the environment vars for the notebox, which may result in a changed _env.dbs
	hubUpdateEnvVars(box)

	// Get the info
	fileChanges, err4 := box.GetChangedNotefiles(req.DeviceEndpointID)
	if err4 != nil {
		err = err4
		return
	}

	// Return the results so long as they fit within the protocol buffer (see notehub.options).
	// This is safe because it will simply take multiple passes to sync all of these files.
	for {
		rsp.NotefileIDs = strings.Join(fileChanges, ReservedIDDelimiter)
		if len(rsp.NotefileIDs) < (250-10) {
			break
		}
		if len(fileChanges) == 0 {
			break
		}
		fileChanges = fileChanges[:len(fileChanges)-1]
	}

	// Done
	box.Close()

	return err
}

// Notefile AddNote
func hubNotefileAddNote(session *HubSessionContext, req notehubMessage, rsp *notehubMessage, event EventFunc, context interface{}) (err error) {

	// Open the box
	box, appUID, err2 := openHubNoteboxForDevice(session, req.DeviceUID, req.DeviceSN, req.ProductUID, req.DeviceEndpointID, event, context)
	if err2 != nil {
		err = err2
		return
	}

	// Open the specified notefile
	openfile, file, err4 := box.OpenNotefile(req.NotefileID)
	if err4 != nil {
		err = err4
		box.Close()
		return
	}

	// Set the notification context
	file.SetEventInfo(req.DeviceUID, req.DeviceSN, req.ProductUID, appUID, event, context)

	// Perform the operation
	var body, payload []byte
	ibody, err5 := req.GetBody()
	if err5 != nil {
		err = err5
	} else {
		body, err = json.Marshal(ibody)
	}
	if err == nil {
		payload, err = req.GetPayload()
	}
	if err == nil {
		var newNote note.Note
		newNote, err = note.CreateNote(body, payload)
		if err == nil {
			err = file.AddNote(req.DeviceEndpointID, req.NoteID, newNote)
		}
	}
	if err != nil {
		openfile.Close()
		box.Close()
		return err
	}

	// Done
	openfile.Close()
	box.Close()

	return nil
}

// Notefile DeleteNote
func hubNotefileDeleteNote(session *HubSessionContext, req notehubMessage, rsp *notehubMessage, event EventFunc, context interface{}) (err error) {

	// Open the box
	box, appUID, err2 := openHubNoteboxForDevice(session, req.DeviceUID, req.DeviceSN, req.ProductUID, req.DeviceEndpointID, event, context)
	if err2 != nil {
		err = err2
		return
	}

	// Open the specified notefile
	openfile, file, err4 := box.OpenNotefile(req.NotefileID)
	if err4 != nil {
		err = err4
		box.Close()
		return
	}

	// Set the notification context
	file.SetEventInfo(req.DeviceUID, req.DeviceSN, req.ProductUID, appUID, event, context)

	// Perform the operation
	err = file.DeleteNote(req.DeviceEndpointID, req.NoteID)
	if err != nil {
		openfile.Close()
		box.Close()
		return err
	}

	// Done
	openfile.Close()
	box.Close()

	return nil
}

// Notefile UpdateNote
func hubNotefileUpdateNote(session *HubSessionContext, req notehubMessage, rsp *notehubMessage, event EventFunc, context interface{}) (err error) {

	// Open the box
	box, appUID, err2 := openHubNoteboxForDevice(session, req.DeviceUID, req.DeviceSN, req.ProductUID, req.DeviceEndpointID, event, context)
	if err2 != nil {
		err = err2
		return
	}

	// Open the specified notefile
	openfile, file, err4 := box.OpenNotefile(req.NotefileID)
	if err4 != nil {
		err = err4
		box.Close()
		return
	}

	// Set the notification context
	file.SetEventInfo(req.DeviceUID, req.DeviceSN, req.ProductUID, appUID, event, context)

	// Perform the operation
	note, err5 := file.GetNote(req.NoteID)
	if err5 != nil {
		err = err5
		openfile.Close()
		box.Close()
		return
	}
	var body, payload []byte
	ibody, err6 := req.GetBody()
	if err6 != nil {
		err = err6
	} else {
		body, err = json.Marshal(ibody)
	}
	if err == nil {
		payload, err = req.GetPayload()
	}
	if err == nil {
		note.SetBody(body)
		note.SetPayload(payload)
	}
	if err == nil {
		err = file.UpdateNote(req.DeviceEndpointID, req.NoteID, note)
	}
	if err != nil {
		openfile.Close()
		box.Close()
		return err
	}

	// Done
	openfile.Close()
	box.Close()

	return nil
}

// Read a file range
func hubReadFile(session *HubSessionContext, req notehubMessage, rsp *notehubMessage, event EventFunc, context interface{}) (err error) {

	// If callback not set, this function can't function
	if fnHubReadFile == nil {
		err = fmt.Errorf("hub is lacking the capability to read an uploaded file")
		return
	}

	// Open the box
	box, appUID, err2 := openHubNoteboxForDevice(session, req.DeviceUID, req.DeviceSN, req.ProductUID, req.DeviceEndpointID, event, context)
	if err2 != nil {
		err = err2
		return
	}

	// Get the request parameters, which we overload onto other fields in the protobuf
	filename := req.NotefileIDs
	offset := int32(req.Since)
	length := int32(req.Until)
	filetype := req.NoteID

	// Perform the read, and always do it compressed because the device firmware does a decompress.
	getInfo := offset == 0
	body, payload, err2 := fnHubReadFile(appUID, filetype, filename, offset, length, true, getInfo)
	if err2 != nil {
		err = err2
		box.Close()
		return
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
		box.Close()
		return
	}
	err = newNotefile.AddNote(req.DeviceEndpointID, "result", xnote)
	if err != nil {
		box.Close()
		return
	}

	// Return the results
	err = rsp.SetNotefile(newNotefile)

	// Done
	box.Close()

	return
}

// RegisterTLSSupport tells the discover module that we do support TLS
func RegisterTLSSupport() {
	serverSupportsTLS = true
}

// TLSSupport tells the discover module that we do support TLS
func TLSSupport() bool {
	return serverSupportsTLS
}

// Debugging function to display message name in a friendly way
func msgTypeName(msgType string) string {
	switch msgType {
	case msgSetSessionContext:
		return "SetSessionContext"
	case msgPing:
		return "Ping"
	case msgPingLegacy:
		return "Ping (Legacy)"
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
		return "NotefilesMerge"
	case msgNotefileUpdateChangeTracker:
		return "NotefileUpdateChangeTracker"
	case msgNotefileAddNote:
		return "NotefileAddNote"
	case msgNotefileDeleteNote:
		return "NotefileDeleteNote"
	case msgNotefileUpdateNote:
		return "NotefileUpdateNote"
	case msgGetNotification:
		return "GetNotification"
	case msgReadFile:
		return "ReadFile"
	case msgWebRequest:
		return "WebRequest"
	}
	return msgType
}
