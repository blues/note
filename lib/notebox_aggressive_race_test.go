package notelib

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestNoteboxAggressiveRace aggressively tests the race condition between
// OpenNotebox checking existence and uReopenNotebox assuming it exists
func TestNoteboxAggressiveRace(t *testing.T) {
	longRunningTest(t)
	ctx := context.Background()

	// Set up storage
	tmpDir := t.TempDir()

	FileSetStorageLocation(tmpDir)

	const (
		endpoint   = "race-endpoint"
		numWorkers = 50
		duration   = 2 * time.Second
	)

	storageObject := FileStorageObject("aggressive_race_box")

	// Create the notebox
	err := CreateNotebox(ctx, endpoint, storageObject)
	if err != nil {
		t.Fatalf("Failed to create notebox: %v", err)
	}

	// Counters
	var (
		opens       int64
		closes      int64
		openErrors  int64
		closeErrors int64
		panics      int64
	)

	// Channel to stop workers
	stop := make(chan struct{})
	var wg sync.WaitGroup

	// Start workers that continuously open/close
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			for {
				select {
				case <-stop:
					return
				default:
					// Catch panics
					func() {
						defer func() {
							if r := recover(); r != nil {
								atomic.AddInt64(&panics, 1)
								t.Logf("Worker %d PANIC: %v", id, r)
							}
						}()

						// Open
						box, err := OpenNotebox(ctx, endpoint, storageObject)
						if err != nil {
							atomic.AddInt64(&openErrors, 1)
							return
						}
						atomic.AddInt64(&opens, 1)

						// Close immediately
						err = box.Close(ctx)
						if err != nil {
							atomic.AddInt64(&closeErrors, 1)
						} else {
							atomic.AddInt64(&closes, 1)
						}
					}()
				}
			}
		}(i)
	}

	// Let it run
	time.Sleep(duration)
	close(stop)
	wg.Wait()

	// Report
	t.Logf("Operations in %v:", duration)
	t.Logf("  Opens:        %d", opens)
	t.Logf("  Closes:       %d", closes)
	t.Logf("  Open errors:  %d", openErrors)
	t.Logf("  Close errors: %d", closeErrors)
	t.Logf("  Panics:       %d", panics)

	if panics > 0 {
		t.Error("Race condition caused panics!")
	}
}

// TestNoteboxSpecificRaceScenario tests the specific race scenario
func TestNoteboxSpecificRaceScenario(t *testing.T) {
	longRunningTest(t)
	ctx := context.Background()

	// Clean up any leftover noteboxes from previous tests
	_ = Purge(ctx)

	// Set up storage
	tmpDir := t.TempDir()

	FileSetStorageLocation(tmpDir)

	const endpoint = "specific-race"

	// This test attempts to hit the exact race condition:
	// 1. Thread A: OpenNotebox sees notebox exists in openboxes.Load()
	// 2. Thread A: Releases boxLock
	// 3. Thread B: Purges the notebox from openboxes
	// 4. Thread A: Calls uReopenNotebox which expects it to exist

	for iteration := 0; iteration < 100; iteration++ {
		storageObject := FileStorageObject(fmt.Sprintf("specific_%d", iteration))

		// Create notebox
		err := CreateNotebox(ctx, endpoint, storageObject)
		if err != nil {
			t.Fatalf("Failed to create: %v", err)
		}

		// Open and close to get it cached with openCount=0
		box, err := OpenNotebox(ctx, endpoint, storageObject)
		if err != nil {
			t.Fatalf("Failed to open: %v", err)
		}
		box.Close(ctx)

		// Make it immediately purgeable
		boxLock.Lock()
		if v, ok := openboxes.Load(storageObject); ok {
			if nbi, ok := v.(*NoteboxInstance); ok {
				nbi.openfiles.Range(func(k, v interface{}) bool {
					if of, ok := v.(*OpenNotefile); ok {
						of.closeTime = time.Now().Add(-time.Hour)
						of.modCountAfterCheckpoint = 0 // No modifications
					}
					return true
				})
			}
		}
		boxLock.Unlock()

		// Now we'll try to trigger the race
		var wg sync.WaitGroup
		var openErr error
		var openPanic, purgePanic interface{}

		wg.Add(2)

		// Thread A: Try to open (will check existence then call uReopenNotebox)
		go func() {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					openPanic = r
				}
			}()

			// Add a tiny delay to let the purge start
			time.Sleep(time.Microsecond)

			_, openErr = OpenNotebox(ctx, endpoint, storageObject)
		}()

		// Thread B: Force a purge
		go func() {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					purgePanic = r
				}
			}()

			// Directly purge this specific notebox
			boxLock.Lock()
			if v, ok := openboxes.Load(storageObject); ok {
				if nbi, ok := v.(*NoteboxInstance); ok {
					// Check if it can be purged
					canPurge := true
					nbi.openfiles.Range(func(k, v interface{}) bool {
						if of, ok := v.(*OpenNotefile); ok {
							if of.openCount > 0 {
								canPurge = false
								return false
							}
						}
						return true
					})

					if canPurge {
						// Remove from map - this is what causes the race!
						openboxes.Delete(storageObject)
					}
				}
			}
			boxLock.Unlock()
		}()

		wg.Wait()

		// Check results
		if openPanic != nil {
			t.Errorf("Iteration %d: PANIC in open: %v", iteration, openPanic)
		}
		if purgePanic != nil {
			t.Errorf("Iteration %d: PANIC in purge: %v", iteration, purgePanic)
		}
		if openErr != nil {
			// This might be expected if the box was purged
			if openErr.Error() != "device not found {device-noexist}" {
				t.Logf("Iteration %d: Unexpected open error: %v", iteration, openErr)
			}
		}
	}

	// Clean up all noteboxes created during the test
	err := Purge(ctx)
	if err != nil {
		t.Logf("Final purge error: %v", err)
	}

	// Force cleanup of any remaining noteboxes to ensure test isolation
	openboxes.Range(func(key, value interface{}) bool {
		if nbi, ok := value.(*NoteboxInstance); ok {
			// Only delete noteboxes created by this test
			if nbi.endpointID == endpoint {
				openboxes.Delete(key)
			}
		}
		return true
	})
}

// TestNoteboxMultipleCloseRace tests what happens when multiple goroutines
// try to close the same notebox simultaneously
func TestNoteboxMultipleCloseRace(t *testing.T) {
	longRunningTest(t)
	ctx := context.Background()

	// Set up storage
	tmpDir := t.TempDir()

	FileSetStorageLocation(tmpDir)

	const (
		endpoint   = "multiclose-test"
		numClosers = 10
	)

	storageObject := FileStorageObject("multiclose_box")

	// Create notebox
	err := CreateNotebox(ctx, endpoint, storageObject)
	if err != nil {
		t.Fatalf("Failed to create notebox: %v", err)
	}

	// Open it
	box, err := OpenNotebox(ctx, endpoint, storageObject)
	if err != nil {
		t.Fatalf("Failed to open notebox: %v", err)
	}

	// Now have multiple goroutines try to close it simultaneously
	var wg sync.WaitGroup
	errors := make([]error, numClosers)
	panics := make([]interface{}, numClosers)

	for i := 0; i < numClosers; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					panics[id] = r
				}
			}()

			err := box.Close(ctx)
			if err != nil {
				errors[id] = err
			}
		}(i)
	}

	wg.Wait()

	// Report results
	errorCount := 0
	panicCount := 0

	for i, err := range errors {
		if err != nil {
			errorCount++
			// Only log if we want verbose output
			if testing.Verbose() {
				t.Logf("Closer %d error: %v", i, err)
			}
		}
	}

	for i, p := range panics {
		if p != nil {
			panicCount++
			t.Errorf("Closer %d PANIC: %v", i, p)
		}
	}

	t.Logf("Multiple close results: %d errors, %d panics", errorCount, panicCount)

	// Close should be idempotent - we expect 0 errors
	if errorCount != 0 {
		t.Errorf("Expected 0 close errors (Close should be idempotent), got %d", errorCount)
	}

	if panicCount > 0 {
		t.Error("Multiple close caused panics!")
	}
}
