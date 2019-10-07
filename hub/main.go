// Copyright 2017 Blues Inc.  All rights reserved.
// Use of this source code is governed by licenses granted by the
// copyright holder including that found in the LICENSE file.

package main

import (
	"bytes"
	"fmt"
	"github.com/blues/note/lib"
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

	// Initialize file system folders
	eventLogInit(os.Getenv("HOME") + "/note/events")
	notelib.FileSetStorageLocation(os.Getenv("HOME") + "/note/notefiles")

	// Set discovery callback
	notelib.HubSetDiscover(NotehubDiscover)

	// Spawn the TCP listeners
	go tcpHandler()
	go tcpsHandler()
	fmt.Printf("\nON DEVICE, CONNECT TO THIS SERVER USING:\n{\"req\":\"service.set\",\"host\":\"tcp:%s%s|tcps:%s%s\"}\n",
		serverAddress, serverPortTCP, serverAddress, serverPortTCPS)
	fmt.Printf("TO RESTORE DEVICE'S HUB CONFIGURATION, USE:\n{\"req\":\"service.set\",\"host\":\"-\"}\n\n")

	// Wait forever
	for {
		time.Sleep(5 * time.Minute)
	}

}
