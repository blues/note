// Copyright 2017 Blues Inc.  All rights reserved.
// Use of this source code is governed by licenses granted by the
// copyright holder including that found in the LICENSE file.
// Derived from the "FeedSync" sample code which was covered
// by the Microsoft Public License (Ms-Pl) of December 3, 2007.
// FeedSync itself was derived from the algorithms underlying
// Lotus Notes replication, developed by Ray Ozzie et al c.1985.

// Package notelib notefile.go handles management and sync of collections of individual notes
package notelib

import (
	"github.com/blues/note-go/note"
)

// jsonConvertJSONToNotefile deserializes/unmarshals a JSON buffer to an in-memory Notefile.
func jsonConvertJSONToNotefile(jsonNotefile []byte) (*Notefile, error) {
	// Unmarshal the JSON
	newNotefile := &Notefile{}
	err := note.JSONUnmarshal(jsonNotefile, &newNotefile)
	if err != nil {
		return newNotefile, err
	}

	// If any of the core map data structures are empty, make sure they
	// have valid maps so that the caller can blindly do range enums, etc.
	if newNotefile.Notes == nil {
		newNotefile.Notes = map[string]note.Note{}
	}
	if newNotefile.Trackers == nil {
		newNotefile.Trackers = map[string]Tracker{}
	}

	return newNotefile, nil
}

// Body functions for Notebox body
// Todo: Should these return errors?
func noteboxBodyFromJSON(data []byte) (body noteboxBody) { _ = note.JSONUnmarshal(data, &body); return }
func noteboxBodyToJSON(body noteboxBody) (data []byte)   { data, _ = note.JSONMarshal(body); return }
