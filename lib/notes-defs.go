// Copyright 2017 Blues Inc.  All rights reserved.
// Use of this source code is governed by licenses granted by the
// copyright holder including that found in the LICENSE file.

// Package notelib notes-defs.go contains Structures and definitions needed outside the package
package notelib

// ReservedIDDelimiter is used to separate lists of noteIDs, notefileIDs, endpointIDs, thus is invalid in names
const ReservedIDDelimiter = ","

// HTTPUserAgent is the HTTP user agent for all our uses of HTTP
const HTTPUserAgent = "notes"

// DefaultMaxGetChangesBatchSize is the default for the max of changes that we'll return in a batch of GetChanges.
const DefaultMaxGetChangesBatchSize = 10
