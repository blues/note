// Copyright 2017 Blues Inc.  All rights reserved.
// Use of this source code is governed by licenses granted by the
// copyright holder including that found in the LICENSE file.

// Package notelib debug.go contains things that assist in debugging
package notelib

import (
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"
)

// Debugging details
var synchronous = true
var debugEvent = true
var debugBox = false
var debugSync = false
var debugSyncMax = false
var debugCompress = false
var debugHubRequest = true
var debugRequest = true
var debugFile = false
var text = [...]string{
	"event", "box", "sync", "syncmax", "compress", "hubrequest", "request", "file",
}
var vars = [...]*bool{
	&debugEvent, &debugBox, &debugSync, &debugSyncMax, &debugCompress, &debugHubRequest, &debugRequest, &debugFile,
}

// Buffering
var debugInitialized bool
var debugChannel chan string

// Initialize for debugging
func debugInit() {

	// The first time through, start our handler
	if debugInitialized {
		return
	}

	// Initialzie from environment variables
	for i := range text {
		avail, value := debugEnv(text[i])
		if avail {
			*(vars[i]) = value
		}
	}

	debugChannel = make(chan string, 500)

	debugInitialized = true

	go debugHandler()

}

// DebugSet sets the debug variable
func DebugSet(which string, value string) {

	// Convert the value as appropriate
	boolValue := false
	lcValue := strings.ToLower(value)
	if lcValue == "t" || lcValue == "true" || lcValue == "on" {
		boolValue = true
	} else if lcValue == "f" || lcValue == "false" || lcValue == "off" {
		boolValue = false
	} else {
		i, err := strconv.Atoi(value)
		if err == nil {
			if i != 0 {
				boolValue = true
			}
		}
	}

	// Find the string
	for i := range text {
		if text[i] == which {
			*(vars[i]) = boolValue
			debugf("%s is now %t\n", which, boolValue)
			return
		}
	}

	// Failure
	debugf("%s not found\n", which)

}

// Get the value of a boolean environment variable
func debugEnv(which string) (isAvail bool, value bool) {
	envvar := "DEBUG_" + strings.ToUpper(which)
	env := os.Getenv(envvar)
	if env == "" {
		return false, false
	}
	i, err := strconv.Atoi(env)
	if err != nil {
		return true, false
	}
	if i == 0 {
		return true, false
	}
	debugf("%s is ENABLED\n", envvar)
	return true, true
}

// Printf is an externally-callable version of same, for server use so it doesn't get held up writing to console
func Printf(format string, args ...interface{}) {
	debugf(format, args...)
}

// Output with printf-style args
func debugf(format string, args ...interface{}) {
	debug(fmt.Sprintf(format, args...))
}

// Output a debug string to local buffering, in a synchronized manner, and enqueue it for background
// output on the console.  We do this because many of our debug routines are done at interrupt level,
// and fmt.Printf is notoriously slow.
func debug(message string) {

	// Exit if not yet initialized
	if !debugInitialized {
		debugInit()
	}

	// Append to what's pending, unless the channel is full, in which case we drop it and move on
	if synchronous {
		fmt.Printf("%s", message)
	} else {
		select {
		case debugChannel <- message:
		default:
			// channel full
		}
	}

}

// Background handler
func debugHandler() {
	for {
		output := <-debugChannel
		fmt.Printf("%s", output)
		runtime.Gosched()
	}
}
