// Copyright 2019 Blues Inc.  All rights reserved.
// Use of this source code is governed by licenses granted by the
// copyright holder including that found in the LICENSE file.

package notelib

import (
	"github.com/blues/note-go/note"
)

// EventFunc is the func to get called whenever there is a note add/update/delete
type EventFunc func(context interface{}, local bool, file *Notefile, data *note.Event) (err error)
