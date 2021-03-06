// Copyright 2017 Blues Inc.  All rights reserved.
// Use of this source code is governed by licenses granted by the
// copyright holder including that found in the LICENSE file.

// Package notelib fileio.go is the lowest level I/O driver underlying the file transport
package notelib

import (
	"io/ioutil"
	"os"
	"strings"
)

// FileioExistsFunc checks for file existence
type FileioExistsFunc func(path string) (exists bool, err error)

// FileioDeleteFunc deletes a file
type FileioDeleteFunc func(path string) (err error)

// FileioCreateFunc creates a file
type FileioCreateFunc func(path string) (err error)

// FileioWriteJSONFunc writes a JSON file
type FileioWriteJSONFunc func(path string, data []byte) (err error)

// FileioReadJSONFunc reads a JSON file
type FileioReadJSONFunc func(path string) (data []byte, err error)

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
func fileioExists(path string) (exists bool, err error) {
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
func fileioDelete(path string) (err error) {
	return os.Remove(path)
}

// Create a file and write it
func fileioCreate(path string) (err error) {

	str := strings.Split(path, "/")
	if len(str) > 1 {
		folder := strings.Join(str[0:len(str)-1], "/")
		os.MkdirAll(folder, 0777)
	}

	return fileioWriteJSON(path, []byte("{}"))

}

// Write an existing JSON file
func fileioWriteJSON(path string, data []byte) (err error) {

	fd, err2 := os.OpenFile(path, os.O_WRONLY|os.O_TRUNC|os.O_CREATE, 0666)
	if err2 != nil {
		debugf("fileioWrite: error creating %s: %s\n", path, err)
		err = err2
		return
	}

	// Write it
	_, err = fd.Write(data)
	if err != nil {
		debugf("fileioWrite: error writing %s: %s\n", path, err)
		fd.Close()
		return
	}

	// Done
	fd.Close()
	return

}

// Read an existing file
func fileioReadJSON(path string) (data []byte, err error) {
	data, err = ioutil.ReadFile(path)
	return
}
