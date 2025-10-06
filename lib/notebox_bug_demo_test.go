package notelib

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"
)

// TestNoteboxConcurrencyBugDemo demonstrates the specific bug:
// Multiple goroutines calling OpenNotebox/Close on the same endpoint:localStorage
// can cause race conditions due to insufficient concurrency protection
func TestNoteboxConcurrencyBugDemo(t *testing.T) {
	longRunningTest(t)
	ctx := context.Background()

	// Clean up any leftover noteboxes from previous tests
	_ = Purge(ctx)
	forceCleanupAllNoteboxes() // Force cleanup to ensure test isolation

	// Create temp directory for test
	tmpDir := t.TempDir()

	// Set file storage directory
	FileSetStorageLocation(tmpDir)

	const (
		endpoint      = "bug-demo-endpoint"
		numGoroutines = 20
		numOperations = 50
	)

	// Create a single storage location that all goroutines will share
	testStorage := FileStorageObject("shared_notebox_storage")

	// Create the notebox once
	err := CreateNotebox(ctx, endpoint, testStorage)
	if err != nil {
		t.Fatalf("Failed to create notebox: %v", err)
	}

	// Track errors and panics
	var (
		errors sync.Map
		panics sync.Map
		wg     sync.WaitGroup
	)

	wg.Add(numGoroutines)

	// Launch N goroutines that all try to open/work/close the same notebox
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()

			for op := 0; op < numOperations; op++ {
				// Catch panics
				func() {
					defer func() {
						if r := recover(); r != nil {
							panics.Store(fmt.Sprintf("goroutine-%d-op-%d", id, op), r)
						}
					}()

					// Open the notebox
					box, err := OpenNotebox(ctx, endpoint, testStorage)
					if err != nil {
						errors.Store(fmt.Sprintf("open-%d-%d", id, op), err)
						return
					}

					// Do some work
					notefileID := fmt.Sprintf("test-%d.db", id)
					err = box.AddNotefile(ctx, notefileID, nil)
					if err != nil {
						errors.Store(fmt.Sprintf("addnotefile-%d-%d", id, op), err)
					}

					// Close the notebox
					err = box.Close(ctx)
					if err != nil {
						errors.Store(fmt.Sprintf("close-%d-%d", id, op), err)
					}
				}()

				// Small delay to increase race likelihood
				if op%10 == 0 {
					time.Sleep(time.Microsecond)
				}
			}
		}(i)
	}

	wg.Wait()

	// Report results
	var errorCount, panicCount int
	errors.Range(func(key, value interface{}) bool {
		errorCount++
		if errorCount <= 5 {
			t.Logf("Error %s: %v", key, value)
		}
		return true
	})

	panics.Range(func(key, value interface{}) bool {
		panicCount++
		t.Errorf("PANIC in %s: %v", key, value)
		return true
	})

	t.Logf("\nSummary:")
	t.Logf("- Total operations: %d", numGoroutines*numOperations)
	t.Logf("- Total errors: %d", errorCount)
	t.Logf("- Total panics: %d", panicCount)

	if errorCount > 0 || panicCount > 0 {
		t.Fail()
	}

	// Force cleanup of closed noteboxes
	time.Sleep(100 * time.Millisecond)
	err = Purge(ctx)
	if err != nil {
		t.Logf("Purge error: %v", err)
	}

	// Check for leaked noteboxes
	time.Sleep(100 * time.Millisecond)
	checkForLeakedNoteboxes(t)
}

// checkForLeakedNoteboxes verifies no noteboxes remain open
func checkForLeakedNoteboxes(t *testing.T) {
	count := 0
	openboxes.Range(func(key, value interface{}) bool {
		count++
		if nbi, ok := value.(*NoteboxInstance); ok {
			openfiles := 0
			nbi.openfiles.Range(func(k, v interface{}) bool {
				if of, ok := v.(*OpenNotefile); ok {
					if of.openCount > 0 {
						openfiles++
					}
				}
				return true
			})
			if openfiles > 0 {
				t.Errorf("Leaked notebox: storage=%v, endpoint=%s, openfiles=%d",
					key, nbi.endpointID, openfiles)
			}
		}
		return true
	})

	if count > 0 {
		t.Logf("Total noteboxes in cache: %d", count)
	}
}

// forceCleanupAllNoteboxes forcefully removes all noteboxes from the openboxes map
// This is used for test cleanup to ensure test isolation
func forceCleanupAllNoteboxes() {
	openboxes.Range(func(key, value interface{}) bool {
		openboxes.Delete(key)
		return true
	})
}
