// Copyright 2017 Blues Inc.  All rights reserved.
// Use of this source code is governed by licenses granted by the
// copyright holder including that found in the LICENSE file.

package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/blues/note-go/note"
	notelib "github.com/blues/note/lib"
	golc "github.com/google/open-location-code/go"
)

// Event log directory
var eventLogDirectory string

// Initialize the event log
func eventLogInit(dir string) {
	eventLogDirectory = dir
	_ = os.MkdirAll(eventLogDirectory, 0o777)
}

// Event handling procedure
func notehubEvent(ctx context.Context, session *notelib.HubSession, local bool, event *note.Event) (err error) {
	// Retrieve the session context

	// If this is a queue and this is a template note, recursively expand it to multiple notifications
	if event.Bulk {
		return eventBulk(session, local, *event)
	}

	// Don't record events for environment variable updates
	if event.NotefileID == "_env.dbs" {
		return
	}

	// Add info about session and when routed
	event.TowerID = session.Session.CellID

	// Marshal the event in a tightly-compressed manner, preparing to output it as Newline-Delimited JSON (NDJSON)
	eventJSON, err := note.JSONMarshal(event)
	if err != nil {
		return err
	}
	eventNDJSON := string(eventJSON) + "\r\n"

	// Generate a valid log file name
	unPrefixedDeviceUID := strings.TrimPrefix(event.DeviceUID, "dev:")
	filename := fmt.Sprintf("%s-%s", time.Now().UTC().Format("2006-01-02"), unPrefixedDeviceUID)
	filename = filename + ".json"

	// Append the JSON to the file
	f, err := os.OpenFile(eventLogDirectory+"/"+filename, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err == nil {
		f.WriteString(eventNDJSON)
		f.Close()
	}

	// Done
	fmt.Printf("event: appended to %s\n", filename)
	return
}

// For bulk data, process the template and payload, generating recursive notifications
func eventBulk(session *notelib.HubSession, local bool, event note.Event) (err error) {
	// Get the template from the note
	bodyJSON, err := note.JSONMarshal(event.Body)
	if err != nil {
		return err
	}

	// Begin decode of payload using this template
	bdc, err := notelib.BulkDecodeTemplate(context.Background(), bodyJSON, event.Payload)
	if err != nil {
		return err
	}

	// Parse each entry within the payload
	for {

		// Get the next entry
		recordStart := bdc.BinLen
		body, payload, when, wherewhen, olc, _, success := bdc.BulkDecodeNextEntry(context.Background())
		if !success {
			break
		}

		// Generate a new notification request with a unique EventUID.  The note's length
		// is the number of bytes its record occupied in the bulk data, including when/where.
		// Note that when on the wire, this is both an overstatement and an understatement
		// of how much it took to transmit the data: it understates because it does not
		// take into account its 'share' of the fact that the note in the notefile containing
		// this bulk data also transmits the content of the template, and it overstates
		// because the entire bulk data block of N notes is snappy-compressed as a single
		// content-dependent block of which we don't know this entry's contribution.
		// But this is the best we can do, and it is a useful metric for estimation.
		nn := event
		nn.WireLen = uint32(recordStart - bdc.BinLen)
		nn.Req = note.EventAdd
		nn.When = when
		nn.Where = olc
		if nn.Where != "" {
			area, err := golc.Decode(nn.Where)
			if err == nil {
				nn.WhereLat, nn.WhereLon = area.Center()
				nn.WhereWhen = wherewhen
			}
		}
		nn.Updates = 1
		nn.Bulk = false
		nn.Body = &body
		nn.Payload = payload
		nn.EventUID = notelib.GenerateEventUid(&nn)
		_ = notehubEvent(context.Background(), session, local, &nn)

	}

	// Done
	return
}
