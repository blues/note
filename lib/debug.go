// Copyright 2017 Blues Inc.  All rights reserved.
// Use of this source code is governed by licenses granted by the
// copyright holder including that found in the LICENSE file.

// Package notelib debug.go contains things that assist in debugging
package notelib

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// Debugging details
var (
	synchronous     = true
	DebugEvent      = true
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
	&DebugEvent, &debugBox, &debugSync, &debugSyncMax, &debugCompress, &debugHubRequest, &debugRequest, &debugFile,
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
			logInfo(context.Background(), "%s is now %t", which, boolValue)
			return
		}
	}

	// Failure
	logError(context.Background(), "notelib debug setting %s not found", which)
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
	logInfo(context.Background(), "%s is ENABLED", envvar)
	return true, true
}

// Log functions with printf-style args
var debugLoggerFunc LogFunc

func logDebug(ctx context.Context, format string, args ...interface{}) {
	if debugLoggerFunc != nil {
		debugLoggerFunc(ctx, format, args...)
	} else {
		defaultLogger(ctx, format, args...)
	}
}

var infoLoggerFunc LogFunc

func logInfo(ctx context.Context, format string, args ...interface{}) {
	if infoLoggerFunc != nil {
		infoLoggerFunc(ctx, format, args...)
	} else {
		defaultLogger(ctx, format, args...)
	}
}

var warnLoggerFunc LogFunc

func logWarn(ctx context.Context, format string, args ...interface{}) {
	if warnLoggerFunc != nil {
		warnLoggerFunc(ctx, format, args...)
	} else {
		defaultLogger(ctx, format, args...)
	}
}

var errorLoggerFunc LogFunc

func logError(ctx context.Context, format string, args ...interface{}) {
	if errorLoggerFunc != nil {
		errorLoggerFunc(ctx, format, args...)
	} else {
		defaultLogger(ctx, format, args...)
	}
}

type LogFunc func(ctx context.Context, msg string, v ...interface{})

func InitLogging(
	debugFunc LogFunc,
	infoFunc LogFunc,
	warnFunc LogFunc,
	errorFunc LogFunc) {
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
func defaultLogger(ctx context.Context, format string, args ...interface{}) {
	// Exit if not yet initialized
	if !loggerInitialized {
		defaultLoggerInit()
	}

	message := fmt.Sprintf(format, args...)

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

// Default timewarn period used for suppressing LogInfo entirely
const LogPeriodSuppressSecs = 2

// PeriodicInfo is a struct that enables information to be displayed only periodically,
// for 'sampled' tracing purposes, rather than on a continuous basis
type LogPeriod struct {
	LastLog    time.Time
	PeriodSecs int
}

// LogInfoPeriodReset ensures that the very next period log does a trace
func LogInfoPeriodReset(period *LogPeriod) {
	period.LastLog = time.Time{}
}

// LogInfoPeriodically writes an unstructured log message with the INFO level but only periodically
func LogInfoPeriodically(ctx context.Context, period *LogPeriod, msg string, v ...interface{}) {
	if !period.LastLog.IsZero() && time.Since(period.LastLog) < time.Duration(period.PeriodSecs)*time.Second {
		return
	}
	logInfo(ctx, msg, v...)
	period.LastLog = time.Now()
}

// LogInfoBumpTrace bumps a trace interval
func LogInfoBumpTrace(prev *time.Time) (duration time.Duration) {
	duration = time.Since(*prev)
	*prev = time.Now()
	return
}
