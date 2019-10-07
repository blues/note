// Copyright 2017 Blues Inc.  All rights reserved.
// Use of this source code is governed by licenses granted by the
// copyright holder including that found in the LICENSE file.

// Package notelib err.go contains things that assist in error handling
package notelib

import (
	"fmt"
	"strings"
)

// ErrorContains tests to see if an error contains an error keyword that we might expect
func ErrorContains(err error, errKeyword string) bool {
	if err == nil {
		return false
	}
	return strings.Contains(fmt.Sprintf("%s", err), errKeyword)
}

// ErrorString safely returns a string from any error, returning "" for nil
func ErrorString(err error) string {
	if err == nil {
		return ""
	}
	return fmt.Sprintf("%s", err)
}
