package notelib

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestNoteboxExtendedRace runs for an extended time to catch the panic
func TestNoteboxExtendedRace(t *testing.T) {
	longRunningTest(t)
	ctx := context.Background()

	// Set up storage
	tmpDir := t.TempDir()

	FileSetStorageLocation(tmpDir)

	// Enable debug output to see the error message
	oldDebugBox := debugBox
	debugBox = false
	defer func() {
		debugBox = oldDebugBox
	}()

	const (
		endpoint   = "extended-race-endpoint"
		numWorkers = 100              // More workers
		duration   = 30 * time.Second // Run much longer
	)

	storageObject := FileStorageObject("extended_race_box")

	// Create the notebox
	err := CreateNotebox(ctx, endpoint, storageObject)
	if err != nil {
		t.Fatalf("Failed to create notebox: %v", err)
	}

	// Counters
	var (
		operations  int64
		opens       int64
		closes      int64
		errors      int64
		panics      int64
		nilOpenfile int64
	)

	// Track specific errors
	errorTypes := sync.Map{}

	// Channel to stop workers
	stop := make(chan struct{})
	var wg sync.WaitGroup

	t.Logf("Starting %d workers for %v...", numWorkers, duration)

	// Start workers that do various operations
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			for {
				select {
				case <-stop:
					return
				default:
					// Different workers do different things to increase race likelihood
					operation := id % 3

					func() {
						defer func() {
							if r := recover(); r != nil {
								atomic.AddInt64(&panics, 1)
								errStr := fmt.Sprintf("%v", r)
								errorTypes.Store(errStr, true)
								t.Logf("Worker %d PANIC: %v", id, r)
							}
						}()

						atomic.AddInt64(&operations, 1)

						switch operation {
						case 0:
							// Rapid open/close
							box, err := OpenNotebox(ctx, endpoint, storageObject)
							if err != nil {
								atomic.AddInt64(&errors, 1)
								return
							}
							atomic.AddInt64(&opens, 1)

							// Check if Openfile returns nil
							of := box.Openfile()
							if of == nil {
								atomic.AddInt64(&nilOpenfile, 1)
								t.Logf("Worker %d: Openfile() returned nil!", id)
							}

							err = box.Close(ctx)
							if err != nil {
								atomic.AddInt64(&errors, 1)
							} else {
								atomic.AddInt64(&closes, 1)
							}

						case 1:
							// Open, do work, then close
							box, err := OpenNotebox(ctx, endpoint, storageObject)
							if err != nil {
								atomic.AddInt64(&errors, 1)
								return
							}
							atomic.AddInt64(&opens, 1)

							// Do some work
							notefileID := fmt.Sprintf("worker-%d.db", id)
							_ = box.AddNotefile(ctx, notefileID, nil)

							// Small delay
							time.Sleep(time.Microsecond * time.Duration(id%10))

							err = box.Close(ctx)
							if err != nil {
								atomic.AddInt64(&errors, 1)
							} else {
								atomic.AddInt64(&closes, 1)
							}

						case 2:
							// Try to trigger purge race
							box, err := OpenNotebox(ctx, endpoint, storageObject)
							if err != nil {
								atomic.AddInt64(&errors, 1)
								return
							}
							atomic.AddInt64(&opens, 1)

							// Close immediately
							box.Close(ctx)
							atomic.AddInt64(&closes, 1)

							// Try to force a purge
							if id%10 == 0 {
								_ = checkpointAllNoteboxes(ctx, true)
							}
						}
					}()
				}
			}
		}(i)
	}

	// Let it run
	startTime := time.Now()
	time.Sleep(duration)
	close(stop)

	t.Log("Stopping workers...")
	wg.Wait()

	elapsed := time.Since(startTime)

	// Report
	t.Logf("\n=== Extended Race Test Results ===")
	t.Logf("Duration:       %v", elapsed)
	t.Logf("Total ops:      %d (%.0f ops/sec)", operations, float64(operations)/elapsed.Seconds())
	t.Logf("Opens:          %d", opens)
	t.Logf("Closes:         %d", closes)
	t.Logf("Errors:         %d", errors)
	t.Logf("Panics:         %d", panics)
	t.Logf("Nil Openfile:   %d", nilOpenfile)

	t.Logf("\nUnique panic types:")
	errorTypes.Range(func(key, value interface{}) bool {
		t.Logf("  - %s", key)
		return true
	})

	if panics > 0 || nilOpenfile > 0 {
		t.Error("Race condition detected!")
	}
}

// TestNoteboxReopenRaceExtended specifically tests the reopen race with proper setup
func TestNoteboxReopenRaceExtended(t *testing.T) {
	longRunningTest(t)
	ctx := context.Background()

	// Set up storage
	tmpDir := t.TempDir()

	FileSetStorageLocation(tmpDir)

	const (
		endpoint      = "reopen-race-endpoint"
		iterations    = 1000 // 10x more iterations
		numGoroutines = 50
	)

	t.Logf("Running %d iterations of reopen race test...", iterations)

	var (
		totalErrors          int64
		totalPanics          int64
		deviceNotFoundErrors int64
	)

	for i := 0; i < iterations; i++ {
		// Create a unique storage for each iteration
		storageObject := FileStorageObject(fmt.Sprintf("reopen_test_%d", i))

		// Create the notebox - this is important!
		err := CreateNotebox(ctx, endpoint, storageObject)
		if err != nil {
			t.Fatalf("Iteration %d: Failed to create notebox: %v", i, err)
		}

		// Open and close to get it in cache
		box, err := OpenNotebox(ctx, endpoint, storageObject)
		if err != nil {
			t.Fatalf("Iteration %d: Failed to initial open: %v", i, err)
		}
		box.Close(ctx)

		// Now race multiple reopens
		var wg sync.WaitGroup
		wg.Add(numGoroutines)

		start := make(chan struct{})

		for g := 0; g < numGoroutines; g++ {
			go func(id int) {
				defer wg.Done()

				// Wait for start signal
				<-start

				// Catch panics
				defer func() {
					if r := recover(); r != nil {
						atomic.AddInt64(&totalPanics, 1)
						t.Logf("Iteration %d, goroutine %d: PANIC: %v", i, id, r)
					}
				}()

				// Try to open
				box, err := OpenNotebox(ctx, endpoint, storageObject)
				if err != nil {
					atomic.AddInt64(&totalErrors, 1)
					if err.Error() == "device not found {device-noexist}" {
						atomic.AddInt64(&deviceNotFoundErrors, 1)
					}
					return
				}

				// If successful, close it
				box.Close(ctx)
			}(g)
		}

		// Start all goroutines at once
		close(start)
		wg.Wait()

		// Progress report every 100 iterations
		if i > 0 && i%100 == 0 {
			t.Logf("Progress: %d/%d iterations, %d errors, %d panics",
				i, iterations, totalErrors, totalPanics)
		}
	}

	t.Logf("\n=== Reopen Race Extended Results ===")
	t.Logf("Iterations:            %d", iterations)
	t.Logf("Total errors:          %d", totalErrors)
	t.Logf("Device not found:      %d", deviceNotFoundErrors)
	t.Logf("Total panics:          %d", totalPanics)

	if totalPanics > 0 {
		t.Error("Race condition caused panics!")
	}
}

// TestNoteboxMissingOpenfileRace specifically tries to trigger the missing openfile issue
func TestNoteboxMissingOpenfileRace(t *testing.T) {
	longRunningTest(t)
	ctx := context.Background()

	// Set up storage
	tmpDir := t.TempDir()

	FileSetStorageLocation(tmpDir)

	// Track when we see the error log
	var errorLogged int64
	oldErrorLogger := errorLoggerFunc
	errorLoggerFunc = func(ctx context.Context, format string, args ...interface{}) {
		msg := fmt.Sprintf(format, args...)
		if msg == "somehow notebox storage instance is missing (notebox open/closed race condition??)" {
			atomic.AddInt64(&errorLogged, 1)
		}
		// Call the default logger too
		if oldErrorLogger != nil {
			oldErrorLogger(ctx, format, args...)
		} else {
			defaultLogger(ctx, format, args...)
		}
	}
	defer func() {
		errorLoggerFunc = oldErrorLogger
	}()

	const (
		endpoint   = "missing-openfile-endpoint"
		numWorkers = 100
		duration   = 10 * time.Second
	)

	storageObject := FileStorageObject("missing_openfile_box")

	// Create the notebox
	err := CreateNotebox(ctx, endpoint, storageObject)
	if err != nil {
		t.Fatalf("Failed to create notebox: %v", err)
	}

	var (
		operations       int64
		nilOpenfileCount int64
	)

	stop := make(chan struct{})
	var wg sync.WaitGroup

	t.Logf("Trying to trigger missing openfile race condition...")

	// Start workers
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			for {
				select {
				case <-stop:
					return
				default:
					func() {
						defer func() {
							if r := recover(); r != nil {
								t.Logf("Worker %d PANIC: %v", id, r)
							}
						}()

						atomic.AddInt64(&operations, 1)

						// Open notebox
						box, err := OpenNotebox(ctx, endpoint, storageObject)
						if err != nil {
							return
						}

						// This is the critical check - does Openfile() return nil?
						of := box.Openfile()
						if of == nil {
							atomic.AddInt64(&nilOpenfileCount, 1)
							t.Logf("Worker %d: FOUND IT! Openfile() returned nil", id)
							// Don't close - this might help us understand the state
							return
						}

						// Do some work to stress the system
						if id%2 == 0 {
							notefileID := fmt.Sprintf("stress-%d.db", id)
							_ = box.AddNotefile(ctx, notefileID, nil)
						}

						// Close
						box.Close(ctx)

						// Occasionally try to purge
						if id%20 == 0 {
							_ = checkpointAllNoteboxes(ctx, true)
						}
					}()
				}
			}
		}(i)
	}

	// Run for duration
	time.Sleep(duration)
	close(stop)
	wg.Wait()

	t.Logf("\n=== Missing Openfile Race Results ===")
	t.Logf("Operations:         %d", operations)
	t.Logf("Nil Openfile count: %d", nilOpenfileCount)
	t.Logf("Error logged count: %d", errorLogged)

	if nilOpenfileCount > 0 || errorLogged > 0 {
		t.Error("Successfully triggered the missing openfile race condition!")
		t.Logf("This confirms the bug exists - Openfile() can return nil due to race conditions")
	} else {
		t.Log("Did not trigger the specific race condition in this run")
		t.Log("The race condition may require specific timing that's hard to reproduce")
	}
}
