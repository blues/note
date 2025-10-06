// Copyright 2019 Blues Inc.  All rights reserved.
// Use of this source code is governed by licenses granted by the
// copyright holder including that found in the LICENSE file.

package notelib

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/blues/note-go/note"
	"github.com/google/uuid"
)

// EventFunc is the func to get called whenever there is a note add/update/delete
type EventFunc func(ctx context.Context, sess *HubSession, local bool, data *note.Event) (err error)

// Generate a UUID for an event in a way that will allow detection of duplicates of "user data"
func GenerateEventUid(event *note.Event) string {

	// If the captured date is unknown, we can't make any assumptions about duplication
	if event == nil || event.When == 0 {
		return uuid.New().String()
	}

	// Create a new MD5 hash instance
	var h = md5.New()

	// Hash deviceUID so we don't conflict with another device in the app
	h.Write([]byte(event.DeviceUID))

	// Convert the 'when' to a byte slice and hash it
	whenBytes := make([]byte, 8)
	binary.LittleEndian.PutUint64(whenBytes, uint64(event.When))
	h.Write(whenBytes)

	// Hash notefileID and noteID
	h.Write([]byte(event.NotefileID))
	h.Write([]byte(event.NoteID))

	// If body is available, hash it
	if event.Body != nil {
		h.Write(marshalSortedJSON(*event.Body))
	}

	// Hash the payload
	h.Write(event.Payload)

	// If details is available, hash it
	if event.Details != nil {
		h.Write(marshalSortedJSON(*event.Details))
	}

	// Copy the MD5 checksum intoa 16-byte array for UUID conversion
	var array16 [16]byte
	copy(array16[:], h.Sum(nil))

	// Convert to a compliant UUID
	return makeCustomUuid(array16)

}

// Generate a cusstom (type 8) UUID from a 16-byte array and return it as a formatted string
// https://www.ietf.org/archive/id/draft-peabody-dispatch-new-uuid-format-04.html#name-uuid-version-8
func makeCustomUuid(input [16]byte) string {
	uuid := input // Copy the entire input into the uuid array

	// Set the version (type 8 UUID, which is 0b1000)
	uuid[6] = (uuid[6] & 0x0F) | 0x80

	// Set the variant (the two most significant bits: 0b10 for RFC 4122)
	uuid[8] = (uuid[8] & 0x3F) | 0x80

	// Format the UUID as 8-4-4-4-12 directly with %x
	return fmt.Sprintf("%x-%x-%x-%x-%x",
		uuid[0:4],   // First 4 bytes (8 hex characters)
		uuid[4:6],   // Next 2 bytes (4 hex characters)
		uuid[6:8],   // Next 2 bytes (4 hex characters)
		uuid[8:10],  // Next 2 bytes (4 hex characters)
		uuid[10:16]) // Final 6 bytes (12 hex characters)
}

// This method recursively sorts JSON fields and marshals them.  Thie purpose of
// this method is to account for the fact that golang maps always come back in an
// intentionally-arbitrary order, and for hashing we need them in a deterministic order.
func marshalSortedJSON(jsonObject interface{}) []byte {

	switch v := jsonObject.(type) {

	case map[string]interface{}:

		// Sort map keys
		keys := make([]string, 0, len(v))
		for key := range v {
			keys = append(keys, key)
		}
		sort.Strings(keys)

		// Create a buffer to hold the marshaled JSON
		var buffer bytes.Buffer
		buffer.WriteString("{")

		// Recursively marshal the value
		for i, key := range keys {
			valueBytes := marshalSortedJSON(v[key])
			if i > 0 {
				buffer.WriteString(",")
			}
			buffer.WriteString(fmt.Sprintf("\"%s\":%s", key, valueBytes))
		}

		buffer.WriteString("}")
		return buffer.Bytes()

	case []interface{}:

		// Handle JSON arrays
		var buffer bytes.Buffer
		buffer.WriteString("[")

		// Recursively marshal the elements
		for i, elem := range v {
			elemBytes := marshalSortedJSON(elem)
			if i > 0 {
				buffer.WriteString(",")
			}
			buffer.Write(elemBytes)
		}

		buffer.WriteString("]")
		return buffer.Bytes()

	default:
		j, err := json.Marshal(v)
		if err != nil {
			return []byte{}
		}
		return j
	}

}
