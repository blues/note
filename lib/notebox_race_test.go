package notelib

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"
)

// TestNoteboxOpenCloseRaceCondition specifically tests the race condition between
// OpenNotebox checking for existence and uReopenNotebox assuming it exists
func TestNoteboxOpenCloseRaceCondition(t *testing.T) {
	longRunningTest(t)
	ctx := context.Background()

	// Create temp directory for test
	tmpDir := t.TempDir()

	// Set file storage directory
	FileSetStorageLocation(tmpDir)

	const endpoint = "race-test-endpoint"

	// This test attempts to trigger the specific race where:
	// 1. Thread A calls OpenNotebox, finds the notebox exists in openboxes.Load()
	// 2. Thread A releases boxLock
	// 3. Thread B closes the notebox and it gets purged from openboxes
	// 4. Thread A calls uReopenNotebox which expects the notebox to still exist

	for iteration := 0; iteration < 100; iteration++ {
		// Create a unique storage for each iteration
		testStorage := FileStorageObject(fmt.Sprintf("race_test_%d_%d", time.Now().UnixNano(), iteration))

		// Create the notebox
		err := CreateNotebox(ctx, endpoint, testStorage)
		if err != nil {
			t.Fatalf("Failed to create notebox: %v", err)
		}

		// Open it once to put it in the cache
		box, err := OpenNotebox(ctx, endpoint, testStorage)
		if err != nil {
			t.Fatalf("Failed to open notebox: %v", err)
		}

		// Close it but it should remain in cache with openCount=0
		err = box.Close(ctx)
		if err != nil {
			t.Fatalf("Failed to close notebox: %v", err)
		}

		// Now try to trigger the race
		var wg sync.WaitGroup
		const numGoroutines = 10

		// Channel to coordinate timing
		ready := make(chan struct{})

		// One goroutine will try to purge the notebox
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-ready
			// Try to force a purge by calling checkpoint
			_ = checkpointAllNoteboxes(ctx, true)
		}()

		// Multiple goroutines will try to open the notebox
		errors := make(chan error, numGoroutines)
		for i := 0; i < numGoroutines; i++ {
			wg.Add(1)
			go func(id int) {
				defer wg.Done()
				<-ready

				// Small random delay to increase chance of hitting the race
				time.Sleep(time.Duration(id) * time.Microsecond)

				// Try to open - this might hit the race condition
				box, err := OpenNotebox(ctx, endpoint, testStorage)
				if err != nil {
					errors <- fmt.Errorf("goroutine %d: %v", id, err)
					return
				}

				// If we got here, close it
				err = box.Close(ctx)
				if err != nil {
					errors <- fmt.Errorf("goroutine %d close: %v", id, err)
				}
			}(i)
		}

		// Start all goroutines
		close(ready)
		wg.Wait()
		close(errors)

		// Check for errors
		for err := range errors {
			t.Logf("Iteration %d error: %v", iteration, err)
		}
	}
}

// TestNoteboxConcurrentReopenWithPurge tests the specific scenario where
// uReopenNotebox is called on a notebox that's being purged
func TestNoteboxConcurrentReopenWithPurge(t *testing.T) {
	longRunningTest(t)
	ctx := context.Background()

	// Set file storage directory
	FileSetStorageLocation("/tmp")

	// Temporarily reduce auto-purge time to make the race more likely
	oldAutoPurgeSeconds := AutoPurgeSeconds
	AutoPurgeSeconds = 0 // Purge immediately
	defer func() {
		AutoPurgeSeconds = oldAutoPurgeSeconds
	}()

	const (
		endpoint   = "purge-race-endpoint"
		iterations = 50
	)

	for i := 0; i < iterations; i++ {
		testStorage := FileStorageObject(fmt.Sprintf("purge_test_%d", i))

		// Create and open a notebox
		err := CreateNotebox(ctx, endpoint, testStorage)
		if err != nil {
			t.Fatalf("Failed to create notebox: %v", err)
		}

		box, err := OpenNotebox(ctx, endpoint, testStorage)
		if err != nil {
			t.Fatalf("Failed to open notebox: %v", err)
		}

		// Close it - it should have openCount=0 but still be in cache
		err = box.Close(ctx)
		if err != nil {
			t.Fatalf("Failed to close notebox: %v", err)
		}

		// Set close time to past to make it eligible for purge
		boxLock.Lock()
		if v, ok := openboxes.Load(testStorage); ok {
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

		// Now race between purge and reopen
		var wg sync.WaitGroup
		wg.Add(2)

		var openErr, purgeErr error

		go func() {
			defer wg.Done()
			// Try to reopen
			_, openErr = OpenNotebox(ctx, endpoint, testStorage)
		}()

		go func() {
			defer wg.Done()
			// Try to purge
			purgeErr = checkpointAllNoteboxes(ctx, true)
		}()

		wg.Wait()

		if openErr != nil {
			t.Logf("Iteration %d: Open error: %v", i, openErr)
		}
		if purgeErr != nil {
			t.Logf("Iteration %d: Purge error: %v", i, purgeErr)
		}

		// Clean up any remaining notebox
		if openErr == nil {
			if v, ok := openboxes.Load(testStorage); ok {
				if _, ok := v.(*NoteboxInstance); ok {
					if box, err := OpenNotebox(ctx, endpoint, testStorage); err == nil {
						box.Close(ctx)
					}
				}
			}
		}
	}
}
