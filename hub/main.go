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
	"time"

	notelib "github.com/blues/note/lib"
)

// Address/ports of our server, used to spawn listers and to assign handlers to devices
var serverAddress string
var serverPortTCP string
var serverPortTCPS string
var serverPortHTTP string
var serverHTTPReqTopic string

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
	eventLogInit(os.Getenv("HOME") + "/note/events")
	notelib.FileSetStorageLocation(os.Getenv("HOME") + "/note/notefiles")

	// Set discovery callback
	notelib.HubSetDiscover(NotehubDiscover)

	// Spawn the TCP listeners
	go tcpHandler()
	go tcpsHandler()
	fmt.Printf("\nON DEVICE, SET HOST USING:\n'{\"req\":\"service.set\",\"host\":\"tcp:%s%s|tcps:%s%s\"}'\n\n",
		serverAddress, serverPortTCP, serverAddress, serverPortTCPS)
	fmt.Printf("TO RESTORE DEVICE'S HUB CONFIGURATION, USE:\n'{\"req\":\"service.set\",\"host\":\"-\"}'\n\n")

	// Spawn HTTP for inbound web requests
	http.HandleFunc(serverHTTPReqTopic, httpReqHandler)
	http.HandleFunc(serverHTTPReqTopic+"/", httpReqHandler)
	go http.ListenAndServe(serverPortHTTP, nil)

	// Wait forever
	for {
		time.Sleep(5 * time.Minute)
	}

}

// Get the directory containing keys
func keyDirectory() string {
	return os.Getenv("HOME") + "/note/keys/"
}
