// Copyright 2017 Blues Inc.  All rights reserved.
// Use of this source code is governed by licenses granted by the
// copyright holder including that found in the LICENSE file.

package main

import (
	"os"
	"time"

	"github.com/blues/note-go/note"
	notelib "github.com/blues/note/lib"
)

// NotehubDiscover is responsible for discovery of information about the services and apps
func NotehubDiscover(deviceUID string, deviceSN string, productUID string, appUID string, needHandlerInfo bool, hostname string, packetHandlerVersion string) (info notelib.DiscoverInfo, err error) {
	// Return basic info about the server
	info.HubEndpointID = note.DefaultHubEndpointID
	info.HubTimeNs = time.Now().UnixNano()

	// Return info about a specific device if requested
	if deviceUID != "" {
		device, err2 := deviceGetOrProvision(deviceUID, deviceSN, productUID)
		if err2 != nil {
			err = err2
			return
		}
		info.HubDeviceStorageObject = notelib.FileStorageObject(deviceUID)
		info.HubDeviceAppUID = device.AppUID
		if needHandlerInfo {
			info.HubSessionHandler = device.Handler
			info.HubSessionTicket = device.Ticket
			info.HubSessionTicketExpiresTimeNs = device.TicketExpiresTimeSec * int64(1000000000)
		}
	}

	// Return the tcps issuer rootca cert, used for device-side certificate rotation
	serviceCertFile, err2 := os.ReadFile(keyDirectory() + "root.crt")
	if err2 == nil {
		info.HubCert = serviceCertFile
	}

	// Done
	return
}
