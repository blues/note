// Copyright 2017 Blues Inc.  All rights reserved.
// Use of this source code is governed by licenses granted by the
// copyright holder including that found in the LICENSE file.

// Package notelib fileio.go is the lowest level I/O driver underlying the file transport
package notelib

import (
	"context"
	"os"
	"path/filepath"
	"strings"
)

type FileIO interface {
	Exists(ctx context.Context, path string) (exists bool, err error)
	Create(ctx context.Context, path string) (err error)
	Delete(ctx context.Context, path string) (err error)
	ReadJSON(ctx context.Context, path string) (data []byte, err error)
	WriteJSON(ctx context.Context, path string, data []byte) (err error)
	CleanName(name string) string
}

type filesystem struct{}

func (fio *filesystem) CleanName(name string) string {
	return fsCleanName(name)
}

func (fio *filesystem) Exists(ctx context.Context, path string) (exists bool, err error) {
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

func (fio *filesystem) Create(ctx context.Context, path string) (err error) {
	str := strings.Split(path, "/")
	if len(str) > 1 {
		folder := strings.Join(str[0:len(str)-1], "/")
		_ = os.MkdirAll(folder, 0o777)
	}

	return fio.WriteJSON(ctx, path, []byte("{}"))
}

func (fio *filesystem) Delete(ctx context.Context, path string) (err error) {
	return os.Remove(path)
}

func (fio *filesystem) ReadJSON(ctx context.Context, path string) (data []byte, err error) {
	data, err = os.ReadFile(path)
	return
}

func (fio *filesystem) WriteJSON(ctx context.Context, path string, data []byte) (err error) {
	_ = os.MkdirAll(filepath.Dir(path), 0777)
	fd, err2 := os.OpenFile(path, os.O_WRONLY|os.O_TRUNC|os.O_CREATE, 0o666)
	if err2 != nil {
		err = err2
		logError(ctx, "fileioWrite: error creating %s: %s", path, err)
		return
	}

	// Write it
	_, err = fd.Write(data)
	if err != nil {
		logError(ctx, "fileioWrite: error writing %s: %s", path, err)
		fd.Close()
		return
	}

	// Done
	fd.Close()
	return
}
