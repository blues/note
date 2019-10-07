// Copyright 2017 Blues Inc.  All rights reserved.
// Use of this source code is governed by licenses granted by the
// copyright holder including that found in the LICENSE file.

package main

import (
	"encoding/json"
	"fmt"
	"github.com/blues/hub/notelib"
	"github.com/blues/note-go/note"
)

// Event handling procedure
func notehubEvent(context interface{}, local bool, file *notelib.Notefile, event *note.Event) (err error) {

	// If this is a queue and this is a template note, recursively expand it to multiple notifications
	if event.Bulk {
		var session *notelib.HubSessionContext
		session = context.(*notelib.HubSessionContext)
		eventBulk(session, local, *file, *event)
		return
	}

	// Process the request by just printing out the request
	eventJSON, err := json.Marshal(event)
	if err != nil {
		return err
	}
	fmt.Printf("\nEVENT TO BE PROCESSED OR ROUTED:\n%s\n", string(eventJSON))

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
