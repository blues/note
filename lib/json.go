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
	"encoding/json"
	"github.com/blues/note-go/note"
)

// jsonConvertJSONToNotefile deserializes/unmarshals a JSON buffer to an in-memory Notefile.
func jsonConvertJSONToNotefile(jsonNotefile []byte) (Notefile, error) {

	// Unmarshal the JSON
	newNotefile := Notefile{}
	err := json.Unmarshal(jsonNotefile, &newNotefile)
	if err != nil {
		return Notefile{}, err
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

// ConvertToJSON serializes/marshals the in-memory Notefile into a JSON buffer
func (file *Notefile) uConvertToJSON(indent bool) (output []byte, err error) {

	// Do the marshal
	if indent {
		output, err = json.MarshalIndent(file, "", "    ")
	} else {
		output, err = json.Marshal(file)
	}

	return
}

// convertToJSON serializes/marshals the in-memory Notefile into a JSON buffer
func (file *Notefile) convertToJSON(indent bool) (output []byte, err error) {

	// Lock for reading
	nfLock.RLock()

	// Convert it
	output, err = file.uConvertToJSON(indent)

	// Unlock
	nfLock.RUnlock()
	return
}

// Body functions for Notebox body
func noteboxBodyFromJSON(data []byte) (body noteboxBody) { json.Unmarshal(data, &body); return }
func noteboxBodyToJSON(body noteboxBody) (data []byte)   { data, _ = json.Marshal(body); return }
