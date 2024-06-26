// Copyright 2017 Blues Inc.  All rights reserved.
// Use of this source code is governed by licenses granted by the
// copyright holder including that found in the LICENSE file.

// Inbound TCP support
package main

import (
	"context"
	"fmt"
	"net"

	notelib "github.com/blues/note/lib"
	"github.com/google/uuid"
)

// Process requests for the duration of a session being open
func sessionHandler(connSession net.Conn, secure bool) {

	// Keep track of this, from a resource consumption perspective
	fmt.Printf("Opened session\n")

	var sessionSource string
	if secure {
		sessionSource = "tcps:" + connSession.RemoteAddr().String()
	} else {
		sessionSource = "tcp:" + connSession.RemoteAddr().String()
	}

	// Always start with a blank, inactive Session Context
	sessionContext := notelib.NewHubSession(uuid.New().String(), sessionSource, secure)

	// Set up golang context with session context stored within
	ctx := context.Background()

	for {
		var request, response []byte
		var err error
		firstTransaction := sessionContext.Transactions == 0

		// Extract a request from the wire, and exit if error
		_, request, err = notelib.WireReadRequest(connSession, true)
		if err != nil {
			if !notelib.ErrorContains(err, "{closed}") {
				fmt.Printf("session: error reading request: %s\n", err)
			}
			break
		}

		// Do special processing on the first transaction
		if firstTransaction {

			// Extract session context info from the wire format even before processing the transaction
			_, err = notelib.WireExtractSessionContext(request, &sessionContext)
			if err != nil {
				fmt.Printf("session: error extracting session context from request: %s", err)
				break
			}

			// Exit if no DeviceUID, at a minimum because this is needed for authentication
			if sessionContext.Session.DeviceUID == "" {
				fmt.Printf("session: device UID is missing from request\n")
				break
			}

			// Make sure that this device is provisioned
			device, err2 := deviceGetOrProvision(sessionContext.Session.DeviceUID, sessionContext.Session.DeviceSN, sessionContext.Session.ProductUID)
			if err2 != nil {
				fmt.Printf("session: can't get or provision device: %s\n", err2)
				break
			}

			// If TLS, validate that the client device hasn't changed.
			if secure {
				err2 = tlsAuthenticate(connSession, device)
				if err2 != nil {
					fmt.Printf("session: %s\n", err2)
					break
				}
			}

		}

		// Process the request
		fmt.Printf("\nReceived %d-byte message from %s\n", len(request), sessionContext.Session.DeviceUID)
		var reqtype string
		var suppressResponse bool
		reqtype, response, suppressResponse, err = notelib.HubRequest(ctx, request, notehubEvent, &sessionContext)
		if err != nil {
			fmt.Printf("session: error processing '%s' request: %s\n", reqtype, err)
			break
		}

		// Write the response
		if !suppressResponse {
			connSession.Write(response)
		}
		sessionContext.Transactions++

	}

	// Close the connection
	connSession.Close()
	fmt.Printf("\nClosed session\n\n")
}
