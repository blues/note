// Copyright 2018 Blues Inc.  All rights reserved.
// Use of this source code is governed by licenses granted by the
// copyright holder including that found in the LICENSE file.

// Package notelib discover.go is the notehub discovery handling support
package notelib

import (
	"fmt"
)

// DiscoverInfo is the information returned by DiscoverFunc
type DiscoverInfo struct {
	HubEndpointID                 string
	HubSessionHandler             string
	HubSessionTicket              string
	HubDeviceStorageObject        string
	HubDeviceAppUID               string
	HubTimeNs                     int64
	HubSessionTicketExpiresTimeNs int64
	HubCert                       []byte
}

// DiscoverFunc is the func to retrieve discovery info for this server
type DiscoverFunc func(edgeUID string, deviceSN string, productUID string, hostname string) (info DiscoverInfo, err error)

var fnDiscover DiscoverFunc

// HubSetDiscover sets the global discovery function
func HubSetDiscover(fn DiscoverFunc) {
	// Remember the discovery function
	fnDiscover = fn

	// Initialize debugging if we've not done so before
	debugEnvInit()
}

// HubDiscover ensures that we've read the local server's discover info, and return the Hub's Endpoint ID
func HubDiscover(deviceUID string, deviceSN string, productUID string) (hubSessionTicket string, hubEndpointID string, appUID string, deviceStorageObject string, err error) {
	if fnDiscover == nil {
		err = fmt.Errorf("no discovery function is available")
		return
	}

	// Call the discover func with the null edge UID just to get basic server info
	discinfo, err := fnDiscover(deviceUID, deviceSN, productUID, "*")
	if err != nil {
		err = fmt.Errorf("error from discovery handler for %s: %s", deviceUID, err)
		return
	}

	return discinfo.HubSessionTicket, discinfo.HubEndpointID, discinfo.HubDeviceAppUID, discinfo.HubDeviceStorageObject, nil
}

// HubDiscover calls the discover function, and return discovery info
func hubProcessDiscoveryRequest(deviceUID string, deviceSN string, productUID string, hostname string) (info DiscoverInfo, err error) {
	if fnDiscover == nil {
		err = fmt.Errorf("no discovery function is available")
		return
	}

	// Call the discover func
	info, err = fnDiscover(deviceUID, deviceSN, productUID, hostname)
	if err != nil {
		err = fmt.Errorf("error from discovery handler for %s: %s", deviceUID, err)
		return
	}

	// Done
	return
}
