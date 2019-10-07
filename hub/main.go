// Copyright 2017 Blues Inc.  All rights reserved.
// Use of this source code is governed by licenses granted by the
// copyright holder including that found in the LICENSE file.

package main

import (
	"bytes"
	"fmt"
	"github.com/blues/hub/notelib"
	"io/ioutil"
	"net/http"
	"os"
	"time"
)

// Address/ports of our server, used to spawn listers and to assign handlers to devices
var serverAddress string
var serverPortTCP string
var serverPortTCPS string

// Main service entry point
func main() {

	// Get our server address and ports
	rsp, err := http.Get("http://checkip.amazonaws.com")
	if err != nil {
		fmt.Printf("can't get our own IP address: %s", err)
		return
	}
	defer rsp.Body.Close()
	var buf []byte
	buf, err = ioutil.ReadAll(rsp.Body)
	if err != nil {
		fmt.Printf("error fetching IP addr: %s", err)
		return
	}
	serverAddress = string(bytes.TrimSpace(buf))
	serverPortTCP = ":8001"
	serverPortTCPS = ":8002"

	// Initialize callbacks
	notelib.FileSetStorageLocation(os.Getenv("HOME") + "/notefiles")
	notelib.HubSetDiscover(NotehubDiscover)

	// Spawn the TCP listeners
	go tcpHandler()
	go tcpsHandler()
	fmt.Printf("\nON DEVICE, SET HOST USING:\n{\"req\":\"service.set\",\"host\":\"tcp:%s%s|tcps:%s%s\"}\n\n",
		serverAddress, serverPortTCP, serverAddress, serverPortTCPS)

	// Wait forever
	for {
		time.Sleep(5 * time.Minute)
	}

}
