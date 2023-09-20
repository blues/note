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
var (
	synchronous     = true
	debugEvent      = true
	debugBox        = false
	debugSync       = false
	debugSyncMax    = false
	debugCompress   = false
	debugHubRequest = true
	debugRequest    = true
	debugFile       = false
	text            = [...]string{
		"event", "box", "sync", "syncmax", "compress", "hubrequest", "request", "file",
	}
)

var vars = [...]*bool{
	&debugEvent, &debugBox, &debugSync, &debugSyncMax, &debugCompress, &debugHubRequest, &debugRequest, &debugFile,
}

var debugEnvInitialized = false

func debugEnvInit() {
	if debugEnvInitialized {
		return
	}
	// Initialzie from environment variables
	for i := range text {
		avail, value := debugEnv(text[i])
		if avail {
			*(vars[i]) = value
		}
	}
	debugEnvInitialized = true
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
			logInfo("%s is now %t", which, boolValue)
			return
		}
	}

	// Failure
	logError("notelib debug setting %s not found", which)
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
	logInfo("%s is ENABLED", envvar)
	return true, true
}

// Log functions with printf-style args
var debugLoggerFunc func(string)

func logDebug(format string, args ...interface{}) {
	message := fmt.Sprintf(format, args...)
	if debugLoggerFunc != nil {
		debugLoggerFunc(message)
	} else {
		defaultLogger(message)
	}
}

var infoLoggerFunc func(string)

func logInfo(format string, args ...interface{}) {
	message := fmt.Sprintf(format, args...)
	if infoLoggerFunc != nil {
		infoLoggerFunc(message)
	} else {
		defaultLogger(message)
	}
}

var warnLoggerFunc func(string)

func logWarn(format string, args ...interface{}) {
	message := fmt.Sprintf(format, args...)
	if warnLoggerFunc != nil {
		warnLoggerFunc(message)
	} else {
		defaultLogger(message)
	}
}

var errorLoggerFunc func(string)

func logError(format string, args ...interface{}) {
	message := fmt.Sprintf(format, args...)
	if errorLoggerFunc != nil {
		errorLoggerFunc(message)
	} else {
		defaultLogger(message)
	}
}
func InitLogging(debugFunc func(string), infoFunc func(string), warnFunc func(string), errorFunc func(string)) {
	debugEnvInit()
	debugLoggerFunc = debugFunc
	infoLoggerFunc = infoFunc
	warnLoggerFunc = warnFunc
	errorLoggerFunc = errorFunc
}

// Default logger for notelib
var (
	loggerInitialized bool
	loggerChannel     chan string
)

// Output a debug string to local buffering, in a synchronized manner, and enqueue it for background
// output on the console.  We do this because many of our debug routines are done at interrupt level,
// and fmt.Printf is notoriously slow.
func defaultLogger(message string) {
	// Exit if not yet initialized
	if !loggerInitialized {
		defaultLoggerInit()
	}

	// Append to what's pending, unless the channel is full, in which case we drop it and move on
	if synchronous {
		fmt.Println(message)
	} else {
		select {
		case loggerChannel <- message:
		default:
			// channel full
		}
	}
}

// Initialize for debugging
func defaultLoggerInit() {
	// The first time through, start our handler
	if loggerInitialized {
		return
	}

	debugEnvInit()

	loggerChannel = make(chan string, 500)

	loggerInitialized = true

	go loggerHandler()
}

// Background handler
func loggerHandler() {
	for {
		output := <-loggerChannel
		fmt.Println(output)
		runtime.Gosched()
	}
}
