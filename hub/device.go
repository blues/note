// Copyright 2017 Blues Inc.  All rights reserved.
// Use of this source code is governed by licenses granted by the
// copyright holder including that found in the LICENSE file.

package main

import (
	"fmt"
	"github.com/google/uuid"
	"time"
)

// Lifetime of a session handler assignment, before device comes back and asks for a new handler
const ticketExpirationMinutes = 60 * 24 * 3

// DeviceState is a session state for a device, and when the state was last updated
type DeviceState struct {

	// Provisioning Info
	DeviceUID  string `json:"device,omitempty"`
	ProductUID string `json:"product,omitempty"`
	AppUID     string `json:"app,omitempty"`
	DeviceSN   string `json:"sn,omitempty"`
	DeviceDN   string `json:"dn,omitempty"`

	// Session State
	Handler              string `json:"handler,omitempty"`
	Ticket               string `json:"ticket,omitempty"`
	TicketExpiresTimeSec int64  `json:"ticket_expires,omitempty"`
}

// Simulation of persistent device storage
var deviceCacheInitialized = false
var deviceCache map[string]DeviceState

// Simulate loading a specific device into the device cache
func deviceCacheLoad(deviceUID string) (device DeviceState, present bool, err error) {

	// Initialize the device cache if not yet initialized
	if !deviceCacheInitialized {
		deviceCacheInitialized = true
		deviceCache = map[string]DeviceState{}
	}

	// Since we don't have persistence in this hub implementation, just see if it's in the cache
	device, present = deviceCache[deviceUID]
	return

}

// Look up a device, provisioning it if necessary
func deviceGetOrProvision(deviceUID string, deviceSN string, productUID string) (device DeviceState, err error) {

	// Load the device
	device, present, err2 := deviceCacheLoad(deviceUID)
	if err2 != nil {
		err = err2
		return
	}

	// Provision the device if not present in the cache
	if !present || productUID != device.ProductUID {

		// Provision the device
		device.DeviceUID = deviceUID
		device.ProductUID = productUID
		device.DeviceSN = deviceSN

	}

	// Look up which app this product is currently assigned to
	device.AppUID, err = appOwningProduct(productUID)
	if err != nil {
		return
	}

	// If the ticket expired, assign a new handler.
	if device.TicketExpiresTimeSec == 0 || time.Now().Unix() >= device.TicketExpiresTimeSec {

		device.Handler = "tcp:" + serverAddress + serverPortTCP
		device.Handler += "|"
		device.Handler += "tcps:" + serverAddress + serverPortTCPS

		// Assign a new ticket whose duration will be constrained
		device.Ticket = uuid.New().String()
		device.TicketExpiresTimeSec =
			time.Now().Add(time.Duration(ticketExpirationMinutes) * time.Minute).Unix()

	}

	// Update the cached data structure
	deviceCache[deviceUID] = device

	// We've successfully found or provisioned the device
	return

}

// Look up, in persistent storage, which app currently 'owns' the reservation for this product UID
func appOwningProduct(productUID string) (appUID string, err error) {
	return
}

// Look up device in the cache
func deviceGet(deviceUID string) (device DeviceState, err error) {
	device, present, err2 := deviceCacheLoad(deviceUID)
	if err2 != nil {
		err = err2
		return
	}
	if !present {
		err = fmt.Errorf("deviceGet: not found")
		return
	}
	return
}

// Look up a device, provisioning it if necessary
func deviceSet(device DeviceState) (err error) {

	// Set in cache
	deviceCache[device.DeviceUID] = device

	// This is where we'd write-through to persistent storage
	return

}
