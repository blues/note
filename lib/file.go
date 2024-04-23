// Copyright 2017 Blues Inc.  All rights reserved.
// Use of this source code is governed by licenses granted by the
// copyright holder including that found in the LICENSE file.

// Package notelib file.go is a 'storage driver' for a standard hierarchical file system
package notelib

import (
	"context"
	"fmt"
	"strings"
)

// This is an important policy that defines how we handle filenames.  When true, "." is replaced with "-" and .json is appended,
// but when false the filename is left as-is except for a bit of cleaning.
const cleanWithJSONExtension = false

// Default filename
const (
	defaultFileStorageName = "notefiles"
	defaultFileStorageExt  = ".db"
)

// Root storage location
var fsRootStorageLocation = "."

// FileSetStorageLocation sets the root location for storage
func FileSetStorageLocation(root string) {
	fsRootStorageLocation = root
}

// Default file I/O package
var fio = &fileioDefault

// fileStorage creates a new storage object
func fileStorage() storageClass {
	fs := storageClass{}
	fs.class = "file"
	fs.create = fsCreate
	fs.createObject = fsCreateObject
	fs.delete = fsDelete
	fs.writeNotefile = fsWriteNotefile
	fs.readNotefile = fsReadNotefile
	return fs
}

// isFileStorage sees whether or not a storage object is a file storage object
func isFileStorage(storageObject string) bool {
	components := strings.Split(storageObject, ":")
	if len(components) >= 1 {
		if components[0] == fileStorage().class {
			return true
		}
	}
	return false
}

// FileSetHandler sets an alternate file I/O package
func FileSetHandler(newfio *Fileio) {
	fio = newfio
}

// FileStorageObject gives a deterministic storage object name from two components
func FileStorageObject(containerName string) string {
	// Generate a filename using the default storage file name
	name := fsFilename(containerName, defaultFileStorageName)

	// Return it as a storage object name
	return fileStorage().class + ":" + name
}

// FileDefaultStorageObject gives a deterministic storage object name from two components
func FileDefaultStorageObject() string {
	return FileStorageObject("")
}

// Remove the scheme if it's present
func fsRemoveScheme(storageObject string) string {
	scheme := fileStorage().class + ":"
	if !strings.HasPrefix(storageObject, scheme) {
		return storageObject
	}
	return strings.TrimPrefix(storageObject, scheme)
}

// Conservatively clean up an input hint and create a valid filename out of it,
// so that it can exist in any reasonable file system.
func fsCleanName(name string) string {
	clean := ""
	for _, r := range name {
		c := string(r)
		if (c >= "a" && c <= "z") || (c >= "A" && c <= "Z") || (c >= "0" && c <= "9") || (c == "_" || c == "-" || c == "/") {
			clean = clean + c
		} else {
			if !cleanWithJSONExtension && c == "." {
				clean = clean + c
			} else {
				clean = clean + "-"
			}
		}
	}
	if len(clean) == 0 {
		clean = "x"
	}
	return clean
}

// fsNamesFromStorageObject extracts the names from the storage object
func fsNamesFromFileStorageObject(storageObject string) (containerName string, fileName string) {
	// Remove the scheme
	name := fsRemoveScheme(storageObject)

	// Split the container and filename
	str := strings.Split(name, "/")

	// No container? Return the filename
	if len(str) < 2 {
		return "", str[0]
	}

	// Return the container and filename, associating all subdir components with the container
	return strings.Join(str[0:len(str)-1], "/"), str[len(str)-1]
}

// FileCleanName generates a clean filename with correct extension from a filename input
func FileCleanName(filename string) string {
	if cleanWithJSONExtension {
		return fsCleanName(filename) + ".json"
	}
	if filename == defaultFileStorageName {
		return fsCleanName(filename + defaultFileStorageExt)
	}
	return fsCleanName(filename)
}

// Generate a filename given some hints that are used to 'weigh in' on the aesthetics
func fsFilename(storageHint string, filenameHint string) string {
	// If no filename is specified, default it
	if filenameHint == "" {
		filenameHint = defaultFileStorageName
	}

	// Generate the filename
	cleanFilename := FileCleanName(filenameHint)

	// If storage is specified (such as a Notebox), get the folder hint from storage.  Note
	// that if it's the file system, we know that we need to clean it because of the colons, but
	// otherwise, let it pass through.
	if storageHint != "" {
		if fio == &fileioDefault {
			return fsCleanName(storageHint) + "/" + cleanFilename
		}
		return storageHint + "/" + cleanFilename
	}

	// Return just the clean filename
	return cleanFilename
}

// Combine two components into a filename
func fsConstruct(storage string, filename string) string {
	// If storage is specified (such as a Notebox), get the folder hint from storage
	if storage != "" {
		return storage + "/" + filename
	}

	// Return just the filename
	return filename
}

// Get the directory that's at the root of all notebox storage
func fsRoot() string {
	return fsRootStorageLocation
}

// Make a path from the specified name
func fsPath(filename string) string {
	return fsRoot() + "/" + filename
}

// Create a file system object within the folder suggested by storageHint, but without that folder
// specified in the storageObject path returned.
func fsCreate(ctx context.Context, storageHint string, filenameHint string) (storageObject string, err error) {
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
		_ = fio.Delete(ctx, path)
		err = fio.Create(ctx, path)
		if err != nil {
			return
		}
	}

	// Regenerate the name by removing the storage container
	_, name = fsNamesFromFileStorageObject(name)

	// Create an object name from this name WITHOUT the container
	return fileStorage().class + ":" + name, nil
}

// Create a file system object given an explicit storage object name, assuming that this already has the
// container within the storageObject specifier
func fsCreateObject(ctx context.Context, storageObject string) (err error) {
	// Get the filename from the storage object name
	name := fsRemoveScheme(storageObject)

	// Create it (and overwrite it if it already exists), thus reserving it.  In normal cases
	// we don't try to create an object that already exists, and so the fact that we overwrite
	// is simply defensive coding.
	path := fsPath(name)
	err = fio.Create(ctx, path)
	if err != nil {
		_ = fio.Delete(ctx, path)
		err = fio.Create(ctx, path)
		if err != nil {
			return
		}
	}

	// Done
	return nil
}

// Convert two storage objects to a file path
func fsObjectsToPath(iContainer string, iObject string) string {
	// Remove schemes
	container := fsRemoveScheme(iContainer)
	object := fsRemoveScheme(iObject)

	// Extract the container name
	container, _ = fsNamesFromFileStorageObject(container)

	// Extract just the object name
	_, object = fsNamesFromFileStorageObject(object)

	// Construct them into a composite name
	name := fsConstruct(container, object)

	// Expand to path
	path := fsPath(name)

	// Done
	return path
}

// Delete a file system object
func fsDelete(ctx context.Context, container string, object string) (err error) {
	return fio.Delete(ctx, fsObjectsToPath(container, object))
}

// Writes a notefile to the specified file
func fsWriteNotefile(ctx context.Context, file *Notefile, container string, object string) (err error) {
	// Convert to JSON
	jsonNotefile, err := file.uConvertToJSON(true)
	if err != nil {
		logWarn(ctx, "file: error converting %d-note notefile %s to json: %s", len(file.Notes), object, err)
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
		logDebug(ctx, "file: wrote %s %db containing %d notes", object, len(jsonNotefile), len(file.Notes))
	}

	// Done
	return nil
}

// Reads a notefile from the specified file
func fsReadNotefile(ctx context.Context, container string, object string) (notefile *Notefile, err error) {
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
