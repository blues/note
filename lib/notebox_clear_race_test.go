package notelib

import (
	"context"
	"fmt"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestNoteboxRaceConditionClearDemo demonstrates the concurrency issues clearly
func TestNoteboxRaceConditionClearDemo(t *testing.T) {
	longRunningTest(t)
	ctx := context.Background()

	// Set up a temporary directory for storage
	tmpDir := t.TempDir()

	// Set file storage directory
	FileSetStorageLocation(tmpDir)

	// Enable debug logging to see what's happening
	oldDebugBox := debugBox
	oldDebugSync := debugSync
	debugBox = false
	debugSync = false
	defer func() {
		debugBox = oldDebugBox
		debugSync = oldDebugSync
	}()

	// Use a custom logger to capture errors
	var loggedErrors []string
	var logMutex sync.Mutex
	errorLogger := func(ctx context.Context, format string, args ...interface{}) {
		msg := fmt.Sprintf(format, args...)
		logMutex.Lock()
		loggedErrors = append(loggedErrors, msg)
		logMutex.Unlock()
		t.Logf("ERROR: %s", msg)
	}
	// Initialize logging to capture errors
	InitLogging(nil, nil, nil, errorLogger, nil, nil)

	const (
		endpoint      = "test-endpoint-1"
		numGoroutines = 10
	)

	// Create a unique storage for this test
	storageObject := FileStorageObject("race_demo_notebox")

	t.Logf("Creating notebox with endpoint='%s', storage='%s'", endpoint, storageObject)

	// Create the notebox first
	err := CreateNotebox(ctx, endpoint, storageObject)
	if err != nil {
		t.Fatalf("Failed to create notebox: %v", err)
	}

	// Pre-open and close to ensure it's in the cache
	box, err := OpenNotebox(ctx, endpoint, storageObject)
	if err != nil {
		t.Fatalf("Failed to pre-open notebox: %v", err)
	}
	err = box.Close(ctx)
	if err != nil {
		t.Fatalf("Failed to pre-close notebox: %v", err)
	}

	t.Log("\n=== Starting concurrent operations ===")

	// Track what happens
	var (
		openAttempts      int32
		openSuccesses     int32
		openFailures      int32
		closeAttempts     int32
		closeSuccesses    int32
		closeFailures     int32
		sanityCheckPanics int32
		otherPanics       int32
	)

	// Use channels to coordinate timing
	start := make(chan struct{})
	var wg sync.WaitGroup

	// Start goroutines that will all try to open/close simultaneously
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			// Wait for signal to start
			<-start

			// Try to open
			func() {
				defer func() {
					if r := recover(); r != nil {
						atomic.AddInt32(&otherPanics, 1)
						t.Logf("Goroutine %d: PANIC during open: %v", id, r)
					}
				}()

				atomic.AddInt32(&openAttempts, 1)
				box, err := OpenNotebox(ctx, endpoint, storageObject)
				if err != nil {
					atomic.AddInt32(&openFailures, 1)
					t.Logf("Goroutine %d: Failed to open: %v", id, err)
					return
				}
				atomic.AddInt32(&openSuccesses, 1)

				// Try to close
				func() {
					defer func() {
						if r := recover(); r != nil {
							if fmt.Sprintf("%v", r) == "closing notebox: notebox already closed: test-endpoint-1" {
								atomic.AddInt32(&sanityCheckPanics, 1)
								t.Logf("Goroutine %d: PANIC - sanity check failed: %v", id, r)
							} else {
								atomic.AddInt32(&otherPanics, 1)
								t.Logf("Goroutine %d: PANIC during close: %v", id, r)
							}
						}
					}()

					atomic.AddInt32(&closeAttempts, 1)
					err := box.Close(ctx)
					if err != nil {
						atomic.AddInt32(&closeFailures, 1)
						t.Logf("Goroutine %d: Failed to close: %v", id, err)
					} else {
						atomic.AddInt32(&closeSuccesses, 1)
					}
				}()
			}()
		}(i)
	}

	// Start all goroutines at once
	close(start)
	wg.Wait()

	// Give a moment for any async operations
	time.Sleep(100 * time.Millisecond)

	// Report results
	t.Log("\n=== Results ===")
	t.Logf("Open attempts:    %d", openAttempts)
	t.Logf("Open successes:   %d", openSuccesses)
	t.Logf("Open failures:    %d", openFailures)
	t.Logf("Close attempts:   %d", closeAttempts)
	t.Logf("Close successes:  %d", closeSuccesses)
	t.Logf("Close failures:   %d", closeFailures)
	t.Logf("Sanity check panics: %d", sanityCheckPanics)
	t.Logf("Other panics:     %d", otherPanics)

	logMutex.Lock()
	t.Logf("\nLogged errors: %d", len(loggedErrors))
	for i, err := range loggedErrors {
		t.Logf("  Error %d: %s", i+1, err)
	}
	logMutex.Unlock()

	// Check for issues
	if openFailures > 0 || closeFailures > 0 || sanityCheckPanics > 0 || otherPanics > 0 {
		t.Error("Concurrency issues detected!")
	}
}

// TestNoteboxOpenCountRace specifically tests the openCount race condition
func TestNoteboxOpenCountRace(t *testing.T) {
	longRunningTest(t)
	ctx := context.Background()

	// Set up storage
	tmpDir := t.TempDir()

	FileSetStorageLocation(tmpDir)

	const endpoint = "opencount-test"
	storageObject := FileStorageObject("opencount_test_box")

	// Create notebox
	err := CreateNotebox(ctx, endpoint, storageObject)
	if err != nil {
		t.Fatalf("Failed to create notebox: %v", err)
	}

	// This test tries to trigger races in concurrent open/close operations
	// It properly tracks all opened boxes and ensures clean shutdown

	t.Log("Testing concurrent open/close operations...")

	// Track all opened boxes for proper cleanup
	var openedBoxes []*Notebox
	var boxesMutex sync.Mutex

	// Counters for operations
	var successfulOpens int32
	var successfulCloses int32
	var alreadyClosedErrors int32

	// Channel for errors
	const numWorkers = 50
	errors := make(chan error, numWorkers*10)

	var wg sync.WaitGroup

	// Create workers that repeatedly open and close boxes
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()

			// Each worker does multiple operations
			for j := 0; j < 10; j++ {
				runtime.Gosched() // Increase chance of race

				// Open a notebox
				box, err := OpenNotebox(ctx, endpoint, storageObject)
				if err != nil {
					errors <- fmt.Errorf("worker %d op %d: open failed: %v", workerID, j, err)
					continue
				}

				atomic.AddInt32(&successfulOpens, 1)

				// Check the count
				of := box.Openfile()
				count := atomic.LoadInt32(&of.openCount)
				if count <= 0 {
					errors <- fmt.Errorf("worker %d op %d: openCount is %d after open!", workerID, j, count)
				}

				// Store for cleanup
				boxesMutex.Lock()
				openedBoxes = append(openedBoxes, box)
				boxesMutex.Unlock()

				// Sometimes close immediately, sometimes leave open for contention
				if j%3 == 0 {
					err = box.Close(ctx)
					if err != nil {
						if strings.Contains(err.Error(), "already closed") {
							atomic.AddInt32(&alreadyClosedErrors, 1)
						} else {
							errors <- fmt.Errorf("worker %d op %d: close failed: %v", workerID, j, err)
						}
					} else {
						atomic.AddInt32(&successfulCloses, 1)
					}
				}
			}
		}(i)
	}

	// Wait for all workers to complete
	wg.Wait()
	close(errors)

	// Collect errors
	var errorList []error
	for err := range errors {
		errorList = append(errorList, err)
	}

	// Clean up all remaining open boxes
	t.Log("Cleaning up opened boxes...")
	cleanupErrors := 0
	for _, box := range openedBoxes {
		if box != nil {
			err := box.Close(ctx)
			// It's OK if it's already closed
			if err != nil && !strings.Contains(err.Error(), "already closed") {
				cleanupErrors++
				t.Logf("Cleanup error: %v", err)
			}
		}
	}

	// Report results
	t.Logf("Operation summary:")
	t.Logf("  Successful opens: %d", successfulOpens)
	t.Logf("  Successful closes: %d", successfulCloses)
	t.Logf("  Already closed errors: %d (expected some)", alreadyClosedErrors)
	t.Logf("  Other errors: %d", len(errorList))
	t.Logf("  Cleanup errors: %d", cleanupErrors)

	// Show first few errors if any
	if len(errorList) > 0 {
		t.Errorf("Found %d errors during test", len(errorList))
		for i := 0; i < len(errorList) && i < 5; i++ {
			t.Logf("  Error %d: %v", i+1, errorList[i])
		}
	}

	// Verify no goroutines are stuck by waiting a bit
	time.Sleep(100 * time.Millisecond)

	// Force a checkpoint to ensure everything is cleaned up
	err = checkpointAllNoteboxes(ctx, true)
	if err != nil {
		t.Logf("Final checkpoint error: %v", err)
	}
}

// TestNoteboxPurgeWhileOpenRace tests the race between purging and opening
func TestNoteboxPurgeWhileOpenRace(t *testing.T) {
	longRunningTest(t)
	ctx := context.Background()

	// Set up storage
	tmpDir := t.TempDir()

	FileSetStorageLocation(tmpDir)

	// Temporarily make purging more aggressive
	oldAutoPurgeSeconds := AutoPurgeSeconds
	AutoPurgeSeconds = 0
	defer func() {
		AutoPurgeSeconds = oldAutoPurgeSeconds
	}()

	const (
		endpoint   = "purge-test"
		iterations = 20
	)

	t.Log("Testing race between purge and open operations...")

	panicCount := 0
	errorCount := 0

	for i := 0; i < iterations; i++ {
		storageObject := FileStorageObject(fmt.Sprintf("purge_box_%d", i))

		// Create and open
		err := CreateNotebox(ctx, endpoint, storageObject)
		if err != nil {
			t.Fatalf("Iteration %d: Failed to create: %v", i, err)
		}

		box, err := OpenNotebox(ctx, endpoint, storageObject)
		if err != nil {
			t.Fatalf("Iteration %d: Failed to open: %v", i, err)
		}

		// Close to make it purgeable
		box.Close(ctx)

		// Make it eligible for immediate purge
		boxLock.Lock()
		if v, ok := openboxes.Load(storageObject); ok {
			if nbi, ok := v.(*NoteboxInstance); ok {
				nbi.openfiles.Range(func(k, v interface{}) bool {
					if of, ok := v.(*OpenNotefile); ok {
						of.closeTime = time.Now().Add(-time.Hour)
					}
					return true
				})
			}
		}
		boxLock.Unlock()

		// Now race: one goroutine tries to open while another purges
		var wg sync.WaitGroup
		wg.Add(2)

		// Goroutine 1: Try to open
		go func() {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					panicCount++
					t.Logf("Iteration %d: PANIC in open: %v", i, r)
				}
			}()

			_, err := OpenNotebox(ctx, endpoint, storageObject)
			if err != nil {
				errorCount++
				t.Logf("Iteration %d: Error in open: %v", i, err)
			}
		}()

		// Goroutine 2: Try to purge
		go func() {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					panicCount++
					t.Logf("Iteration %d: PANIC in purge: %v", i, r)
				}
			}()

			_ = checkpointAllNoteboxes(ctx, true)
		}()

		wg.Wait()
	}

	t.Logf("\nPurge race results:")
	t.Logf("- Iterations: %d", iterations)
	t.Logf("- Panics: %d", panicCount)
	t.Logf("- Errors: %d", errorCount)

	if panicCount > 0 {
		t.Error("Race condition caused panics!")
	}

	if errorCount > 0 {
		t.Errorf("Race condition caused %d errors - noteboxes were purged while being opened!", errorCount)
	}
}
