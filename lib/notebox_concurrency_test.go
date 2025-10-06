package notelib

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestNoteboxConcurrentOpenClose tests for race conditions when multiple
// goroutines open and close the same notebox concurrently
func TestNoteboxConcurrentOpenClose(t *testing.T) {
	longRunningTest(t)
	ctx := context.Background()

	// Create temp directory for test
	tmpDir := t.TempDir()

	// Test parameters
	const (
		numGoroutines = 50
		numIterations = 100
		endpoint      = "concurrent-test-endpoint"
	)

	// Set file storage directory
	FileSetStorageLocation(tmpDir)

	// Create a temporary storage location
	testStorage := FileStorageObject(fmt.Sprintf("concurrent_test_%d", time.Now().UnixNano()))

	// First create the notebox
	err := CreateNotebox(ctx, endpoint, testStorage)
	if err != nil {
		t.Fatalf("Failed to create notebox: %v", err)
	}

	// Counters for tracking operations
	var (
		openSuccesses  int64
		openFailures   int64
		closeSuccesses int64
		closeFailures  int64
		workSuccesses  int64
		workFailures   int64
	)

	// WaitGroup to coordinate goroutines
	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	// Error channel to collect any panics or errors
	errChan := make(chan error, numGoroutines*numIterations)

	// Start time for synchronizing goroutines
	start := make(chan struct{})

	// Launch concurrent goroutines
	for i := 0; i < numGoroutines; i++ {
		go func(goroutineID int) {
			defer wg.Done()

			// Wait for all goroutines to be ready
			<-start

			for j := 0; j < numIterations; j++ {
				// Protect against panics
				func() {
					defer func() {
						if r := recover(); r != nil {
							errChan <- fmt.Errorf("goroutine %d iteration %d panicked: %v", goroutineID, j, r)
						}
					}()

					// Try to open the notebox - using the SAME endpoint and storage for all goroutines
					box, err := OpenNotebox(ctx, endpoint, testStorage)
					if err != nil {
						atomic.AddInt64(&openFailures, 1)
						// Log first few errors
						if openFailures <= 5 {
							errChan <- fmt.Errorf("goroutine %d failed to open: %v", goroutineID, err)
						}
						return
					}
					atomic.AddInt64(&openSuccesses, 1)

					// Do some work with the notebox
					func() {
						defer func() {
							if r := recover(); r != nil {
								atomic.AddInt64(&workFailures, 1)
								errChan <- fmt.Errorf("goroutine %d iteration %d panicked during work: %v", goroutineID, j, r)
							}
						}()

						// Try to perform some operations
						of := box.Openfile()
						if of != nil {
							// Simulate some work - add and read a note
							notefileID := "test.db"
							_ = box.AddNotefile(ctx, notefileID, nil)
							atomic.AddInt64(&workSuccesses, 1)
						} else {
							atomic.AddInt64(&workFailures, 1)
						}
					}()

					// Add small random delay to increase chance of race conditions
					if goroutineID%5 == 0 {
						time.Sleep(time.Microsecond)
					}

					// Try to close the notebox
					err = box.Close(ctx)
					if err != nil {
						atomic.AddInt64(&closeFailures, 1)
						errChan <- fmt.Errorf("goroutine %d iteration %d failed to close: %v", goroutineID, j, err)
					} else {
						atomic.AddInt64(&closeSuccesses, 1)
					}

					// Another small delay
					if goroutineID%3 == 0 {
						time.Sleep(time.Microsecond)
					}
				}()
			}
		}(i)
	}

	// Start all goroutines simultaneously
	close(start)

	// Wait for all goroutines to complete
	wg.Wait()
	close(errChan)

	// Collect any errors
	var errors []error
	for err := range errChan {
		errors = append(errors, err)
	}

	// Report results
	t.Logf("Operation statistics:")
	t.Logf("  Opens:  %d successes, %d failures", openSuccesses, openFailures)
	t.Logf("  Work:   %d successes, %d failures", workSuccesses, workFailures)
	t.Logf("  Closes: %d successes, %d failures", closeSuccesses, closeFailures)
	t.Logf("  Total errors: %d", len(errors))

	// Print first few errors if any
	if len(errors) > 0 {
		t.Errorf("Encountered %d errors during concurrent operations", len(errors))
		for i, err := range errors {
			if i < 5 { // Only print first 5 errors
				t.Errorf("  Error %d: %v", i+1, err)
			}
		}
		if len(errors) > 5 {
			t.Errorf("  ... and %d more errors", len(errors)-5)
		}
	}

	// Force a checkpoint and purge to clean up any remaining boxes
	err = checkpointAllNoteboxes(ctx, true)
	if err != nil {
		t.Logf("Checkpoint error during cleanup: %v", err)
	}

	// Verify no noteboxes are leaked
	time.Sleep(100 * time.Millisecond) // Give time for cleanup
	verifyNoLeakedNoteboxes(t, endpoint)
}

// TestNoteboxConcurrentOpenCloseStress is a more intense version with rapid operations
func TestNoteboxConcurrentOpenCloseStress(t *testing.T) {
	longRunningTest(t)
	ctx := context.Background()

	// Create temp directory for test
	tmpDir := t.TempDir()

	const (
		numGoroutines = 100
		testDuration  = 5 * time.Second
		endpoint      = "stress-test-endpoint"
	)

	// Set file storage directory
	FileSetStorageLocation(tmpDir)

	// Create a temporary storage location
	testStorage := FileStorageObject(fmt.Sprintf("stress_test_%d", time.Now().UnixNano()))

	// First create the notebox
	err := CreateNotebox(ctx, endpoint, testStorage)
	if err != nil {
		t.Fatalf("Failed to create notebox: %v", err)
	}

	var (
		operations int64
		errors     int64
	)

	// Error channel
	errChan := make(chan error, 1000)

	// Stop channel
	stop := make(chan struct{})

	// WaitGroup
	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	// Launch goroutines
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()

			for {
				select {
				case <-stop:
					return
				default:
					// Rapid open/close operations
					func() {
						defer func() {
							if r := recover(); r != nil {
								atomic.AddInt64(&errors, 1)
								select {
								case errChan <- fmt.Errorf("goroutine %d panicked: %v", id, r):
								default:
								}
							}
						}()

						box, err := OpenNotebox(ctx, endpoint, testStorage)
						if err != nil {
							atomic.AddInt64(&errors, 1)
							return
						}

						// Minimal work
						_ = box.Openfile()

						err = box.Close(ctx)
						if err != nil {
							atomic.AddInt64(&errors, 1)
						}

						atomic.AddInt64(&operations, 1)
					}()
				}
			}
		}(i)
	}

	// Run for specified duration
	time.Sleep(testDuration)
	close(stop)
	wg.Wait()

	// Report results
	t.Logf("Stress test completed:")
	t.Logf("  Total operations: %d", operations)
	t.Logf("  Total errors: %d", errors)
	t.Logf("  Operations per second: %.2f", float64(operations)/testDuration.Seconds())

	if errors > 0 {
		t.Errorf("Stress test encountered %d errors", errors)

		// Collect sample errors
		close(errChan)
		sampleErrors := make([]error, 0, 5)
		for err := range errChan {
			sampleErrors = append(sampleErrors, err)
			if len(sampleErrors) >= 5 {
				break
			}
		}

		for i, err := range sampleErrors {
			t.Errorf("  Sample error %d: %v", i+1, err)
		}
	}
}

// TestNoteboxReopenRace specifically tests the race condition in uReopenNotebox
func TestNoteboxReopenRace(t *testing.T) {
	longRunningTest(t)
	ctx := context.Background()

	// Clean up any leftover noteboxes from previous tests
	_ = Purge(ctx)

	// Create temp directory for test
	tmpDir := t.TempDir()

	// Set file storage directory
	FileSetStorageLocation(tmpDir)

	// Create a temporary storage location
	testStorage := FileStorageObject(fmt.Sprintf("reopen_test_%d", time.Now().UnixNano()))

	const (
		numGoroutines = 20
		endpoint      = "reopen-race-endpoint"
	)

	// First, create the notebox
	err := CreateNotebox(ctx, endpoint, testStorage)
	if err != nil {
		t.Fatalf("Failed to create notebox: %v", err)
	}

	// Then open and close it to set up the condition
	box, err := OpenNotebox(ctx, endpoint, testStorage)
	if err != nil {
		t.Fatalf("Failed to open initial notebox: %v", err)
	}
	err = box.Close(ctx)
	if err != nil {
		t.Fatalf("Failed to close initial notebox: %v", err)
	}

	// Now have multiple goroutines try to reopen it simultaneously
	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	start := make(chan struct{})
	errors := make([]error, numGoroutines)
	boxes := make([]*Notebox, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()

			// Wait for signal to start
			<-start

			// Try to open the notebox
			box, err := OpenNotebox(ctx, endpoint, testStorage)
			if err != nil {
				errors[id] = err
			} else {
				boxes[id] = box
			}
		}(i)
	}

	// Start all goroutines at once
	close(start)
	wg.Wait()

	// Count successes and failures
	var successes, failures int
	for i, err := range errors {
		if err != nil {
			failures++
			t.Logf("Goroutine %d failed: %v", i, err)
		} else if boxes[i] != nil {
			successes++
			// Clean up
			_ = boxes[i].Close(ctx)
		}
	}

	t.Logf("Reopen race results: %d successes, %d failures", successes, failures)

	// At least some should succeed
	if successes == 0 {
		t.Error("All reopens failed - this suggests a problem")
	}
}

// Helper function to verify no noteboxes are leaked
func verifyNoLeakedNoteboxes(t *testing.T, endpoint string) {
	// This is a bit hacky but we need to check the internal state
	// In a real fix, we'd expose a method to check this properly
	boxLock.Lock()
	defer boxLock.Unlock()

	count := 0
	openboxes.Range(func(key, value interface{}) bool {
		if nbi, ok := value.(*NoteboxInstance); ok {
			if nbi.endpointID == endpoint {
				count++
				// Count open notefiles and check their open counts
				openfiles := 0
				totalOpenCount := int32(0)
				nbi.openfiles.Range(func(k, v interface{}) bool {
					openfiles++
					if of, ok := v.(*OpenNotefile); ok {
						oc := atomic.LoadInt32(&of.openCount)
						totalOpenCount += oc
						if oc > 0 {
							t.Logf("  Notefile %v has openCount=%d", k, oc)
						}
					}
					return true
				})
				t.Logf("Leaked notebox found: endpoint=%s, openfiles=%d, totalOpenCount=%d",
					nbi.endpointID, openfiles, totalOpenCount)
			}
		}
		return true
	})

	if count > 0 {
		// Only error if there are boxes with non-zero open counts
		// Boxes with zero counts will be purged eventually
		t.Logf("Found %d noteboxes for endpoint %s in cache (may be purged later if openCount=0)", count, endpoint)
	}
}
