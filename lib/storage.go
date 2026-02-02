// Copyright 2017 Blues Inc.  All rights reserved.
// Use of this source code is governed by licenses granted by the
// copyright holder including that found in the LICENSE file.

// Package notelib storage.go is the class definition for storage drivers
package notelib

import (
	"context"
	"fmt"
	"strings"
)

type storage struct{}

func (s storage) create(ctx context.Context, storageHint string, filenameHint string) (storageObject string, err error) {
	// Find a path that doesn't yet exist
	container, _ := fsNamesFromFileStorageObject(storageHint)
	name := fsFilename(container, filenameHint)

	// If and only if we're cleaning, select a path that doesn't exist - else overwrite the file
	if cleanWithJSONExtension {
		for i := 1; ; i++ {

			exists, err2 := fio.Exists(ctx, fsPath(name))
			if err2 != nil {
				err = err2
				return
			}
			if !exists {
				break
			}

			name = fsFilename(container, fmt.Sprintf("%s%d", filenameHint, i))

		}
	}

	// Create it, thus reserving its name.  Note that if it exists we must overwrite it (as the
	// comment above specifies).  This has the effect of self-healing rather than blocking
	// operations where the high level file descriptor was lost and the file is now being recreated.
	// (Note that this is particularly important when !cleanWithJSONExtension, else a new unique
	// file object would have been created in that case.)
	path := fsPath(name)
	err = fio.Create(ctx, path)
	if err != nil {
		if deletionForbidden(ctx, path) {
			err = fmt.Errorf("file: can't re-create %s: %s", path, err)
			return
		}
		_ = fio.Delete(ctx, path)
		err = fio.Create(ctx, path)
		if err != nil {
			return
		}
	}

	// Regenerate the name by removing the storage container
	_, name = fsNamesFromFileStorageObject(name)

	// Create an object name from this name WITHOUT the container
	return fileStorageClass + ":" + name, nil
}

func (s storage) createObject(ctx context.Context, storageObject string) (err error) {
	// Get the filename from the storage object name
	name := fsRemoveScheme(storageObject)

	// Create it (and overwrite it if it already exists), thus reserving it.  In normal cases
	// we don't try to create an object that already exists, and so the fact that we overwrite
	// is simply defensive coding.
	path := fsPath(name)
	err = fio.Create(ctx, path)
	if err != nil {
		if deletionForbidden(ctx, path) {
			return fmt.Errorf("file: can't re-create %s: %s", path, err)
		}
		_ = fio.Delete(ctx, path)
		err = fio.Create(ctx, path)
		if err != nil {
			return
		}
	}

	// Done
	return nil
}

func (s storage) delete(ctx context.Context, container string, object string) (err error) {
	path := fsObjectsToPath(container, object)
	if deletionForbidden(ctx, path) {
		return nil
	}
	return fio.Delete(ctx, path)
}

func (s storage) writeNotefile(ctx context.Context, notefile *Notefile, container string, object string) (err error) {
	// Convert to JSON
	jsonNotefile, err := notefile.uConvertToJSON(true)
	if err != nil {
		logWarn(ctx, "file: error converting %d-note notefile %s to json: %s", len(notefile.Notes), object, err)
		return err
	}

	// Open the file, truncating it
	path := fsObjectsToPath(container, object)
	err = fio.WriteJSON(ctx, path, jsonNotefile)
	if err != nil {
		return
	}

	// Debug
	if debugFile {
		logDebug(ctx, "file: wrote %s %db containing %d notes", object, len(jsonNotefile), len(notefile.Notes))
	}

	// Done
	return nil
}

func (s storage) readNotefile(ctx context.Context, container string, object string) (oNotefile *Notefile, err error) {
	// Read the file
	path := fsObjectsToPath(container, object)
	contents, err := fio.ReadJSON(ctx, path)
	if err != nil {
		return &Notefile{}, err
	}

	// Unmarshal the contents
	file, err := jsonConvertJSONToNotefile(contents)
	if err != nil {
		logWarn(ctx, "file: error converting notefile %s to json: %s", object, err)
		return &Notefile{}, err
	}

	// Debug
	if debugFile {
		logDebug(ctx, "file: read %s %db containing %d notes", object, len(contents), len(file.Notes))
	}

	// Done
	return file, nil
}

// Storage creates a storage object from the class in an object string
func storageProvider(iObject string) (storage, error) {
	components := strings.Split(iObject, ":")
	if len(components) < 1 || components[0] != fileStorageClass {
		return storage{}, fmt.Errorf("storage provider not found: %s", iObject)
	}

	return storage{}, nil
}
