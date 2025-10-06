// Test file that demonstrates the concurrency bug in OpenNotebox/Close
//
// The bug: Multiple goroutines calling OpenNotebox() and Close() on the same
// endpoint:localStorage can cause race conditions due to insufficient
// concurrency protection in the open and close methods.

package notelib

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestNoteboxConcurrencyBugSummary demonstrates all the race conditions found
func TestNoteboxConcurrencyBugSummary(t *testing.T) {
	longRunningTest(t)
	// Set up storage
	tmpDir := t.TempDir()

	FileSetStorageLocation(tmpDir)

	t.Log("=== NOTEBOX CONCURRENCY BUG DEMONSTRATION ===")
	t.Log("")
	t.Log("This test demonstrates race conditions when multiple goroutines")
	t.Log("concurrently call OpenNotebox() and Close() on the same endpoint:localStorage")
	t.Log("")

	// Test 1: Basic concurrent open/close
	t.Run("ConcurrentOpenClose", func(t *testing.T) {
		testConcurrentOpenClose(t)
	})

	// Test 2: Race between checking existence and reopening
	t.Run("ReopenRace", func(t *testing.T) {
		testReopenRace(t)
	})

	// Test 3: OpenCount field race
	t.Run("OpenCountRace", func(t *testing.T) {
		testOpenCountRace(t)
	})

	t.Log("")
	t.Log("=== SUMMARY ===")
	t.Log("The tests above demonstrate several race conditions:")
	t.Log("1. Race on openCount field between atomic operations and non-atomic reads")
	t.Log("2. Race between OpenNotebox checking existence and uReopenNotebox assuming it exists")
	t.Log("3. Race on closeTime field with concurrent writes")
	t.Log("4. Missing synchronization in sanityCheckOpenCount()")
	t.Log("")
	t.Log("Run with -race flag to see detailed race reports")
}

func testConcurrentOpenClose(t *testing.T) {
	ctx := context.Background()

	const (
		endpoint      = "concurrent-endpoint"
		numGoroutines = 20
		numOperations = 50
	)

	storageObject := FileStorageObject("concurrent_test")

	// Create notebox
	err := CreateNotebox(ctx, endpoint, storageObject)
	if err != nil {
		t.Fatalf("Failed to create notebox: %v", err)
	}

	var (
		opens  int64
		closes int64
		errors int64
		panics int64
	)

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()

			for j := 0; j < numOperations; j++ {
				func() {
					defer func() {
						if r := recover(); r != nil {
							atomic.AddInt64(&panics, 1)
							t.Logf("Goroutine %d: PANIC: %v", id, r)
						}
					}()

					// Open
					box, err := OpenNotebox(ctx, endpoint, storageObject)
					if err != nil {
						atomic.AddInt64(&errors, 1)
						return
					}
					atomic.AddInt64(&opens, 1)

					// Close
					err = box.Close(ctx)
					if err != nil {
						atomic.AddInt64(&errors, 1)
					} else {
						atomic.AddInt64(&closes, 1)
					}
				}()
			}
		}(i)
	}

	wg.Wait()

	t.Logf("Results: %d opens, %d closes, %d errors, %d panics",
		opens, closes, errors, panics)

	if panics > 0 {
		t.Error("Concurrent operations caused panics!")
	}
}

func testReopenRace(t *testing.T) {
	ctx := context.Background()

	const endpoint = "reopen-endpoint"
	storageObject := FileStorageObject("reopen_test")

	// Create and setup notebox
	err := CreateNotebox(ctx, endpoint, storageObject)
	if err != nil {
		t.Fatalf("Failed to create notebox: %v", err)
	}

	// Open and close to get it cached
	box, err := OpenNotebox(ctx, endpoint, storageObject)
	if err != nil {
		t.Fatalf("Failed to open: %v", err)
	}
	box.Close(ctx)

	// The race: OpenNotebox checks existence, releases lock,
	// then calls uReopenNotebox which assumes it still exists

	t.Log("Testing race between existence check and reopen...")

	var reopenErrors int64
	var reopenPanics int64

	// Make it purgeable
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

	var wg sync.WaitGroup
	wg.Add(2)

	// Thread 1: Try to open (will check then reopen)
	go func() {
		defer wg.Done()
		defer func() {
			if r := recover(); r != nil {
				atomic.AddInt64(&reopenPanics, 1)
				t.Logf("Reopen PANIC: %v", r)
			}
		}()

		_, err := OpenNotebox(ctx, endpoint, storageObject)
		if err != nil {
			atomic.AddInt64(&reopenErrors, 1)
		}
	}()

	// Thread 2: Purge it
	go func() {
		defer wg.Done()

		boxLock.Lock()
		openboxes.Delete(storageObject)
		boxLock.Unlock()
	}()

	wg.Wait()

	t.Logf("Reopen race: %d errors, %d panics", reopenErrors, reopenPanics)
}

func testOpenCountRace(t *testing.T) {
	ctx := context.Background()

	const endpoint = "opencount-endpoint"
	storageObject := FileStorageObject("opencount_test")

	// Create notebox
	err := CreateNotebox(ctx, endpoint, storageObject)
	if err != nil {
		t.Fatalf("Failed to create notebox: %v", err)
	}

	// Open to get reference
	box, err := OpenNotebox(ctx, endpoint, storageObject)
	if err != nil {
		t.Fatalf("Failed to open: %v", err)
	}

	of := box.Openfile()
	t.Logf("Testing openCount race condition...")
	t.Logf("Initial openCount: %d", atomic.LoadInt32(&of.openCount))

	// The race: atomic operations on openCount vs non-atomic reads in sanityCheckOpenCount

	var wg sync.WaitGroup
	const numOps = 50

	// Half open, half close
	for i := 0; i < numOps; i++ {
		wg.Add(1)
		if i%2 == 0 {
			// Open
			go func() {
				defer wg.Done()
				box, err := OpenNotebox(ctx, endpoint, storageObject)
				if err == nil {
					// Trigger sanityCheckOpenCount by closing
					box.Close(ctx)
				}
			}()
		} else {
			// Close
			go func() {
				defer wg.Done()
				box.Close(ctx)
			}()
		}
	}

	wg.Wait()

	finalCount := atomic.LoadInt32(&of.openCount)
	t.Logf("Final openCount: %d", finalCount)

	// The race condition is in sanityCheckOpenCount() which does:
	// if openCount < 0 { ... }
	// This reads openCount without atomic load while other threads
	// are doing atomic.AddInt32(&openCount, -1)
}
