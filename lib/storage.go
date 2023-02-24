// Copyright 2017 Blues Inc.  All rights reserved.
// Use of this source code is governed by licenses granted by the
// copyright holder including that found in the LICENSE file.

// Package notelib storage.go is the class definition for storage drivers
package notelib

import (
	"context"
	"fmt"
)

// StorageCreateFunc Creates a new storage object instance, with the supplied string as a hint.
// The hint makes it possible to have reasonably meaningful names within
// certain storage mechanisms that can support it (i.e. file systems),
// however it may be completely ignored by other storage mechanisms
// that only deal in terms of addresses, object IDs, or handles.
type storageCreateFunc func(ctx context.Context, iStorageObjectHint string, iFileNameHint string) (storageObject string, err error)

// StorageCreateObjectFunc Creates a new storage object instance,
// with the supplied storage object name being exact.
type storageCreateObjectFunc func(ctx context.Context, storageObject string) (err error)

// StorageDeleteFunc deletes an existing storage object instance
type storageDeleteFunc func(ctx context.Context, iStorageObjectHint string, iObject string) (err error)

// StorageWriteNotefileFunc writes a notefile to the specified storage instance
type storageWriteNotefileFunc func(ctx context.Context, iNotefile *Notefile, iStorageObjectHint string, iObject string) (err error)

// StorageReadNotefileFunc reads a notefile from the specified storage instance
type storageReadNotefileFunc func(ctx context.Context, iStorageObjectHint string, iObject string) (oNotefile *Notefile, err error)

// storageClass is the access method by which we do physical I/O
type storageClass struct {
	class         string
	create        storageCreateFunc
	createObject  storageCreateObjectFunc
	delete        storageDeleteFunc
	writeNotefile storageWriteNotefileFunc
	readNotefile  storageReadNotefileFunc
}

// Storage creates a storage object from the class in an object string
func storageProvider(iObject string) (storage storageClass, err error) {

	// Enumerate known storage providers
	if isFileStorage(iObject) {
		return fileStorage(), nil
	}

	// Not found
	return storageClass{}, fmt.Errorf("storage provider not found: %s", iObject)

}
