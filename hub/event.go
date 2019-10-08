// Copyright 2017 Blues Inc.  All rights reserved.
// Use of this source code is governed by licenses granted by the
// copyright holder including that found in the LICENSE file.

package main

import (
	"encoding/json"
	"fmt"
	"github.com/blues/note-go/note"
	"github.com/blues/note/lib"
	"os"
	"strings"
	"time"
)

// Event log directory
var eventLogDirectory string

// Initialize the event log
func eventLogInit(dir string) {
	eventLogDirectory = dir
	os.MkdirAll(eventLogDirectory, 0777)
}

// Event handling procedure
func notehubEvent(context interface{}, local bool, file *notelib.Notefile, event *note.Event) (err error) {

	// Retrieve the session context
	var session *notelib.HubSessionContext
	session = context.(*notelib.HubSessionContext)

	// If this is a queue and this is a template note, recursively expand it to multiple notifications
	if event.Bulk {
		var session *notelib.HubSessionContext
		session = context.(*notelib.HubSessionContext)
		eventBulk(session, local, *file, *event)
		return
	}

	// Clean the event before sending it out
	if event.Sent {
		event.NoteID = ""
		event.Sent = false
		event.Deleted = false
	}
	event.Routed = time.Now().UTC().Unix()
	event.TowerID = session.Session.CellID

	// Marshal the event in a tightly-compressed manner, preparing to output it as Newline-Delimited JSON (NDJSON)
	eventJSON, err := json.Marshal(event)
	if err != nil {
		return err
	}
	eventNDJSON := string(eventJSON) + "\r\n"

	// Generate a valid log file name
	filename := fmt.Sprintf("%s-%s", time.Now().UTC().Format("2006-01-02"), event.DeviceUID)
	filename = strings.Replace(filename, "imei:", "", 1) + ".json"

	// Append the JSON to the file
	f, err := os.OpenFile(eventLogDirectory+"/"+filename, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err == nil {
		f.WriteString(eventNDJSON)
		f.Close()
	}

	// Done
	fmt.Printf("event: appended to %s\n", filename)
	return

}

// For bulk data, process the template and payload, generating recursive notifications
func eventBulk(session *notelib.HubSessionContext, local bool, file notelib.Notefile, event note.Event) (err error) {

	// Get the template from the note
	bodyJSON, err := json.Marshal(event.Body)
	if err != nil {
		return err
	}

	// Decode the template
	context, entries, err := notelib.BulkDecodeTemplate(bodyJSON, event.Payload)
	if err != nil {
		return err
	}

	// Parse each entry of the payload
	for i := 0; i < entries; i++ {
		body, payload, when, where := notelib.BulkDecodeEntry(&context, i)

		// Genereate a new notification request
		nn := event
		nn.Req = note.EventAdd
		nn.When = when / 1000000000
		nn.Where = notelib.OLCFromINT64(where)
		nn.Updates = 1
		nn.Bulk = false
		nn.Body = &body
		nn.Payload = payload
		notehubEvent(session, local, &file, &nn)

	}

	// Done
	return
}
