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
	olc "github.com/google/open-location-code/go"
	"github.com/google/uuid"
)

// Event log directory
var eventLogDirectory string

// Initialize the event log
func eventLogInit(dir string) {
	eventLogDirectory = dir
	os.MkdirAll(eventLogDirectory, 0o777)
}

// Event handling procedure
func notehubEvent(ctx context.Context, context interface{}, local bool, file *notelib.Notefile, event *note.Event) (err error) {
	// Retrieve the session context
	session := context.(*notelib.HubSessionContext)

	// If this is a queue and this is a template note, recursively expand it to multiple notifications
	if event.Bulk {
		session := context.(*notelib.HubSessionContext)
		eventBulk(session, local, file, *event)
		return
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
func eventBulk(session *notelib.HubSessionContext, local bool, file *notelib.Notefile, event note.Event) (err error) {
	// Get the template from the note
	bodyJSON, err := note.JSONMarshal(event.Body)
	if err != nil {
		return err
	}

	// Begin decode of payload using this template
	bdc, err := notelib.BulkDecodeTemplate(bodyJSON, event.Payload)
	if err != nil {
		return err
	}

	// Parse each entry within the payload
	for {

		// Get the next entry (ignoring OLC until we are ready to support it)
		body, payload, when, where, wherewhen, _, _, success := bdc.BulkDecodeNextEntry()
		if !success {
			break
		}

		// Generate a new notification request with a unique EventUID
		nn := event
		nn.Req = note.EventAdd
		nn.When = when
		nn.WhereWhen = wherewhen
		nn.Where = notelib.OLCFromINT64(where)
		if nn.Where != "" {
			area, err := olc.Decode(nn.Where)
			if err == nil {
				nn.WhereLat, nn.WhereLon = area.Center()
			}
		}
		nn.Updates = 1
		nn.Bulk = false
		nn.Body = &body
		nn.Payload = payload
		nn.EventUID = uuid.New().String()
		notehubEvent(context.Background(), session, local, file, &nn)

	}

	// Done
	return nil
}
