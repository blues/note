// Copyright 2017 Blues Inc.  All rights reserved.
// Use of this source code is governed by licenses granted by the
// copyright holder including that found in the LICENSE file.

// Package notelib file.go is a 'storage driver' for a standard hierarchical file system
package notelib

import (
	"context"
	"runtime/debug"
	"strings"
)

const (
	// Default filename
	defaultFileStorageName = "notefiles"
	defaultFileStorageExt  = ".db"

	// This is an important policy that defines how we handle filenames.  When true, "." is replaced with "-" and .json is appended,
	// but when false the filename is left as-is except for a bit of cleaning.
	cleanWithJSONExtension = false

	// Storage class name
	fileStorageClass = "file"
)

var (
	// Root storage location
	fsRootStorageLocation = "."

	// Default file I/O package
	fio FileIO = &filesystem{}
)

// FileSetStorageLocation sets the root location for storage
func FileSetStorageLocation(root string) {
	fsRootStorageLocation = root
}

// FileSetHandler sets an alternate file I/O package
func FileSetHandler(newfio FileIO) {
	fio = newfio
}

// FileStorageObject gives a deterministic storage object name from two components
func FileStorageObject(containerName string) string {
	// Generate a filename using the default storage file name
	name := fsFilename(containerName, defaultFileStorageName)

	// Return it as a storage object name
	return fileStorageClass + ":" + name
}

// Remove the scheme if it's present
func fsRemoveScheme(storageObject string) string {
	scheme := fileStorageClass + ":"
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
		return fio.CleanName(storageHint) + "/" + cleanFilename
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

// It is quite catastrophic if we attempt to delete the entire file storage root, because for NTN
// recovery is simply impossible.  As such, we treat this like a 'bug check' and forbid the operation.
func deletionForbidden(ctx context.Context, path string) bool {

	// If the path ends in our default storage extension, this must be a file storage object
	if !strings.HasSuffix(path, defaultFileStorageName+defaultFileStorageExt) {
		return false
	}

	// This should be so incredibly rare that if it does happen we want to see exactly why we are here
	logError(ctx, "[dbwarn] attempt to delete file storage root via:\n%s", debug.Stack())
	return true
}
