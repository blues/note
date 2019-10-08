// Copyright 2017 Blues Inc.  All rights reserved.
// Use of this source code is governed by licenses granted by the
// copyright holder including that found in the LICENSE file.

// Inbound TCP support
package main

import (
	"fmt"
	"net"
	"github.com/blues/note/lib"
	"github.com/google/uuid"
)

// Process requests for the duration of a session being open
func sessionHandler(connSession net.Conn, secure bool) {

	// Keep track of this, from a resource consumption perspective
	fmt.Printf("Opened session\n")

	// Always start with a blank, inactive Session Context
	sessionContext := notelib.HubSessionContext{}
	sessionContext.Secure = secure
	sessionContext.Session.SessionUID = uuid.New().String()
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
			err = notelib.WireExtractSessionContext(request, &sessionContext)
			if err != nil {
				fmt.Printf("session: error extracting session context from request: %s", err)
				break
			}

			// Exit if no DeviceUID, at a minimum because this is needed for authentication
			if sessionContext.DeviceUID == "" {
				fmt.Printf("session: device UID is missing from request\n")
				break
			}

			// Make sure that this device is provisioned
			device, err2 := deviceGetOrProvision(sessionContext.DeviceUID, sessionContext.ProductUID)
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
		fmt.Printf("\nReceived %d-byte message from %s\n", len(request), sessionContext.DeviceUID)
		response, err = notelib.HubRequest(&sessionContext, request, notehubEvent, &sessionContext)
		if err != nil {
			fmt.Printf("session: error processing request: %s\n", err)
			break
		}

		// Write the response
		connSession.Write(response)
		sessionContext.Transactions++

	}

	// Close the connection
	connSession.Close()
	fmt.Printf("\nClosed session\n\n")

}
