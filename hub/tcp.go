// Copyright 2017 Blues Inc.  All rights reserved.
// Use of this source code is governed by licenses granted by the
// copyright holder including that found in the LICENSE file.

// Inbound TCP support
package main

import (
	"fmt"
	"net"
)

// tcpHandler kicks off TCP request server
func tcpHandler() {

	fmt.Printf("Serving requests on tcp:%s%s\n", serverAddress, serverPortTCP)

	serverAddr, err := net.ResolveTCPAddr("tcp", serverPortTCP)
	if err != nil {
		fmt.Printf("tcp: error resolving TCP port: %v\n", err)
		return
	}

	connServer, err := net.ListenTCP("tcp", serverAddr)
	if err != nil {
		fmt.Printf("tcp: error listening on TCP port: %v\n", err)
		return
	}
	defer connServer.Close()

	for {

		// Accept the TCP connection
		connSession, err := connServer.AcceptTCP()
		if err != nil {
			fmt.Printf("tcp: error accepting TCP session: %v\n", err)
			continue
		}

		// The scope of a TCP connection may be many requests, so dispatch
		// this to a goroutine which will deal with the session until it is closed.
		go sessionHandler(connSession, false)

	}

}
