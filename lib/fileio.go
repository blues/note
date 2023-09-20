// Copyright 2017 Blues Inc.  All rights reserved.
// Use of this source code is governed by licenses granted by the
// copyright holder including that found in the LICENSE file.

// Package notelib fileio.go is the lowest level I/O driver underlying the file transport
package notelib

import (
	"context"
	"os"
	"strings"
)

// FileioExistsFunc checks for file existence
type FileioExistsFunc func(ctx context.Context, path string) (exists bool, err error)

// FileioDeleteFunc deletes a file
type FileioDeleteFunc func(ctx context.Context, path string) (err error)

// FileioCreateFunc creates a file
type FileioCreateFunc func(ctx context.Context, path string) (err error)

// FileioWriteJSONFunc writes a JSON file
type FileioWriteJSONFunc func(ctx context.Context, path string, data []byte) (err error)

// FileioReadJSONFunc reads a JSON file
type FileioReadJSONFunc func(ctx context.Context, path string) (data []byte, err error)

// Fileio defines a set of functions for alternative file I/O
type Fileio struct {
	Exists    FileioExistsFunc
	Create    FileioCreateFunc
	Delete    FileioDeleteFunc
	ReadJSON  FileioReadJSONFunc
	WriteJSON FileioWriteJSONFunc
}

var fileioDefault = Fileio{
	Exists:    fileioExists,
	Create:    fileioCreate,
	Delete:    fileioDelete,
	ReadJSON:  fileioReadJSON,
	WriteJSON: fileioWriteJSON,
}

// See if a file exists
func fileioExists(ctx context.Context, path string) (exists bool, err error) {
	exists = true
	_, err = os.Stat(path)
	if err != nil {
		exists = false
		if os.IsNotExist(err) {
			err = nil
		}
	}
	return
}

// Delete a file
func fileioDelete(ctx context.Context, path string) (err error) {
	return os.Remove(path)
}

// Create a file and write it
func fileioCreate(ctx context.Context, path string) (err error) {
	str := strings.Split(path, "/")
	if len(str) > 1 {
		folder := strings.Join(str[0:len(str)-1], "/")
		_ = os.MkdirAll(folder, 0o777)
	}

	return fileioWriteJSON(ctx, path, []byte("{}"))
}

// Write an existing JSON file
func fileioWriteJSON(ctx context.Context, path string, data []byte) (err error) {
	fd, err2 := os.OpenFile(path, os.O_WRONLY|os.O_TRUNC|os.O_CREATE, 0o666)
	if err2 != nil {
		logError("fileioWrite: error creating %s: %s", path, err)
		err = err2
		return
	}

	// Write it
	_, err = fd.Write(data)
	if err != nil {
		logError("fileioWrite: error writing %s: %s", path, err)
		fd.Close()
		return
	}

	// Done
	fd.Close()
	return
}

// Read an existing file
func fileioReadJSON(ctx context.Context, path string) (data []byte, err error) {
	data, err = os.ReadFile(path)
	return
}
