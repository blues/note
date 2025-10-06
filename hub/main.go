// Copyright 2017 Blues Inc.  All rights reserved.
// Use of this source code is governed by licenses granted by the
// copyright holder including that found in the LICENSE file.

package main

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"os"

	notelib "github.com/blues/note/lib"
)

// Address/ports of our server, used to spawn listers and to assign handlers to devices
var (
	serverAddress      string
	serverPortTCP      string
	serverPortTCPS     string
	serverPortHTTP     string
	serverHTTPReqTopic string
)

// Main service entry point
func main() {
	// If not specified on the command line, get our server address and ports
	if len(os.Args) < 2 {
		rsp, err := http.Get("http://checkip.amazonaws.com")
		if err != nil {
			fmt.Printf("can't get our own IP address: %s", err)
			return
		}
		defer rsp.Body.Close()
		var buf []byte
		buf, err = io.ReadAll(rsp.Body)
		if err != nil {
			fmt.Printf("error fetching IP addr: %s", err)
			return
		}
		serverAddress = string(bytes.TrimSpace(buf))
	} else {
		serverAddress = os.Args[1]
	}
	serverHTTPReqTopic = "/req"
	serverPortHTTP = ":80"
	serverPortTCP = ":8081"
	serverPortTCPS = ":8086"

	// Initialize file system folders
	eventDir := os.Getenv("HOME") + "/note/events"
	eventLogInit(eventDir)
	notefileDir := os.Getenv("HOME") + "/note/notefiles"
	notelib.FileSetStorageLocation(notefileDir)

	// Set discovery callback
	notelib.HubSetDiscover(NotehubDiscover)

	// Spawn the TCP listeners
	go tcpHandler()
	go tcpsHandler()
	fmt.Printf("\nON DEVICE, SET HOST USING:\n'{\"req\":\"hub.set\",\"host\":\"tcp:%s%s|tcps:%s%s\",\"product\":\"<your-product-uid>\"}'\n\n",
		serverAddress, serverPortTCP, serverAddress, serverPortTCPS)
	fmt.Printf("TO RESTORE DEVICE'S HUB CONFIGURATION, USE:\n'{\"req\":\"hub.set\",\"host\":\"-\"}'\n\n")
	fmt.Printf("Your hub's   data will be stored in: %s\n", notefileDir)
	fmt.Printf("Your hub's events will be stored in: %s\n", eventDir)
	fmt.Printf("\n")

	// Spawn HTTP for inbound web requests
	http.HandleFunc(serverHTTPReqTopic, httpReqHandler)
	http.HandleFunc(serverHTTPReqTopic+"/", httpReqHandler)
	err := http.ListenAndServe(serverPortHTTP, nil)
	if err != nil {
		fmt.Printf("Error running HTTP server on %s: %v\n", serverPortHTTP, err)
		os.Exit(1)
	}
}

// Get the directory containing keys
func keyDirectory() string {
	return os.Getenv("HOME") + "/note/keys/"
}
