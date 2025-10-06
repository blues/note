package notelib

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestNoteboxTorture performs extreme torture testing with many concurrent operations
func TestNoteboxTorture(t *testing.T) {
	longRunningTest(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Set up storage
	tmpDir := t.TempDir()

	FileSetStorageLocation(tmpDir)

	// Enable all debugging
	oldDebugBox := debugBox
	debugBox = false
	defer func() {
		debugBox = oldDebugBox
	}()

	// Make purging extremely aggressive
	oldAutoPurgeSeconds := AutoPurgeSeconds
	AutoPurgeSeconds = 0 // Purge immediately
	defer func() {
		AutoPurgeSeconds = oldAutoPurgeSeconds
	}()

	// Duration - default to 30s, can be overridden by env var
	duration := 30 * time.Second
	if envDuration := os.Getenv("NOTEBOX_TORTURE_DURATION"); envDuration != "" {
		if parsed, err := time.ParseDuration(envDuration); err == nil {
			duration = parsed
		}
	}

	const (
		numBoxes   = 10   // Multiple noteboxes to increase complexity
		numWorkers = 2000 // Extreme concurrency
	)

	// Create multiple noteboxes
	boxes := make([]string, numBoxes)
	for i := 0; i < numBoxes; i++ {
		endpoint := fmt.Sprintf("torture-endpoint-%d", i)
		storage := FileStorageObject(fmt.Sprintf("torture_box_%d", i))
		boxes[i] = storage

		err := CreateNotebox(ctx, endpoint, storage)
		if err != nil {
			t.Fatalf("Failed to create notebox %d: %v", i, err)
		}
	}

	// Shared counters
	type Stats struct {
		operations     int64
		opens          int64
		closes         int64
		errors         int64
		panics         int64
		nilOpenfile    int64
		cacheDeletes   int64
		purges         int64
		concurrentOpen int64
	}

	var stats Stats

	// Track detailed errors
	errorDetails := sync.Map{}
	panicDetails := sync.Map{}

	// Control channels
	stop := make(chan struct{})
	panicFound := make(chan string, 1)

	var wg sync.WaitGroup
	activeWorkers := make([]int32, numWorkers) // Track which workers are still active

	t.Logf("Starting TORTURE TEST: %d workers on %d noteboxes for %v", numWorkers, numBoxes, duration)
	t.Logf("This will stress your system heavily!")

	startTime := time.Now()

	// Monitor goroutine
	go func() {
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()

		lastOps := int64(0)

		for {
			select {
			case <-ticker.C:
				ops := atomic.LoadInt64(&stats.operations)
				opsPerSec := float64(ops-lastOps) / 10.0
				lastOps = ops

				t.Logf("\n=== Status Update ===")
				t.Logf("Elapsed: %v", time.Since(startTime).Round(time.Second))
				t.Logf("Ops/sec: %.0f (total: %d)", opsPerSec, ops)
				t.Logf("Opens: %d, Closes: %d, Errors: %d",
					atomic.LoadInt64(&stats.opens),
					atomic.LoadInt64(&stats.closes),
					atomic.LoadInt64(&stats.errors))
				t.Logf("Panics: %d, Nil Openfile: %d",
					atomic.LoadInt64(&stats.panics),
					atomic.LoadInt64(&stats.nilOpenfile))
				t.Logf("Cache deletes: %d, Purges: %d",
					atomic.LoadInt64(&stats.cacheDeletes),
					atomic.LoadInt64(&stats.purges))
				t.Logf("Goroutines: %d", runtime.NumGoroutine())

			case <-stop:
				return
			}
		}
	}()

	// Chaos workers - different strategies
	for i := 0; i < numWorkers; i++ {
		strategy := i % 20 // 20 different strategies

		wg.Add(1)
		go func(id int, strategy int) {
			defer wg.Done()
			defer func() {
				atomic.StoreInt32(&activeWorkers[id], 0)
			}()
			atomic.StoreInt32(&activeWorkers[id], 1)

			for {
				select {
				case <-stop:
					return
				case <-ctx.Done():
					return
				default:
					// Pick a random notebox
					boxIdx := id % numBoxes
					storage := boxes[boxIdx]
					endpoint := fmt.Sprintf("torture-endpoint-%d", boxIdx)

					func() {
						defer func() {
							if r := recover(); r != nil {
								atomic.AddInt64(&stats.panics, 1)
								panicStr := fmt.Sprintf("Worker %d (strategy %d): %v", id, strategy, r)
								panicDetails.Store(panicStr, time.Now())

								select {
								case panicFound <- panicStr:
								default:
								}

								t.Logf("💥 PANIC: %s", panicStr)
							}
						}()

						atomic.AddInt64(&stats.operations, 1)

						switch strategy {
						case 0, 1, 2, 3, 4: // 25% - Normal open/close
							box, err := OpenNotebox(ctx, endpoint, storage)
							if err != nil {
								atomic.AddInt64(&stats.errors, 1)
								errorDetails.Store(err.Error(), atomic.AddInt64(&stats.errors, 0))
								return
							}
							atomic.AddInt64(&stats.opens, 1)

							// Critical check
							if box.Openfile() == nil {
								atomic.AddInt64(&stats.nilOpenfile, 1)
								t.Logf("🚨 NIL OPENFILE DETECTED by worker %d!", id)
								panic("nil openfile returned from OpenNotebox")
							}

							box.Close(ctx)
							atomic.AddInt64(&stats.closes, 1)

						case 5, 6: // 10% - Multiple concurrent opens
							const numOpens = 50
							atomic.AddInt64(&stats.concurrentOpen, 1)

							var openWg sync.WaitGroup
							openWg.Add(numOpens)

							for j := 0; j < numOpens; j++ {
								go func() {
									defer openWg.Done()
									if box, err := OpenNotebox(ctx, endpoint, storage); err == nil {
										atomic.AddInt64(&stats.opens, 1)
										if box.Openfile() == nil {
											panic("nil openfile in concurrent open")
										}
										box.Close(ctx)
										atomic.AddInt64(&stats.closes, 1)
									}
								}()
							}
							openWg.Wait()

						case 7: // 5% - Aggressive purging
							atomic.AddInt64(&stats.purges, 1)
							_ = checkpointAllNoteboxes(ctx, true)

						case 8: // 5% - Direct cache manipulation
							boxLock.Lock()
							openboxes.Delete(storage)
							boxLock.Unlock()
							atomic.AddInt64(&stats.cacheDeletes, 1)

						case 9: // 5% - Corrupt notebox state
							boxLock.Lock()
							if v, ok := openboxes.Load(storage); ok {
								if nbi, ok := v.(*NoteboxInstance); ok {
									// Set all files to closed and old
									nbi.openfiles.Range(func(k, v interface{}) bool {
										if of, ok := v.(*OpenNotefile); ok {
											of.openCount = 0
											of.closeTime = time.Now().Add(-24 * time.Hour)
										}
										return true
									})
								}
							}
							boxLock.Unlock()

						case 10: // 5% - Open without closing (leak handles)
							for j := 0; j < 10; j++ {
								_, _ = OpenNotebox(ctx, endpoint, storage)
								atomic.AddInt64(&stats.opens, 1)
							}

						case 11: // 5% - Rapid create/destroy notefiles
							if box, err := OpenNotebox(ctx, endpoint, storage); err == nil {
								for j := 0; j < 100; j++ {
									notefileID := fmt.Sprintf("torture-%d-%d.db", id, j)
									_ = box.AddNotefile(ctx, notefileID, nil)
								}
								box.Close(ctx)
							}

						case 12: // 5% - Simultaneous operations on same box
							var opWg sync.WaitGroup
							for j := 0; j < 10; j++ {
								opWg.Add(1)
								go func(opId int) {
									defer opWg.Done()

									box, err := OpenNotebox(ctx, endpoint, storage)
									if err == nil {
										// Do various operations
										switch opId % 3 {
										case 0:
											_ = box.AddNotefile(ctx, fmt.Sprintf("op-%d", opId), nil)
										case 1:
											// Skip Merge - method doesn't exist
										case 2:
											// Just close
										}
										box.Close(ctx)
									}
								}(j)
							}
							opWg.Wait()

						case 13: // 5% - Race between open and delete
							go func() {
								_, _ = OpenNotebox(ctx, endpoint, storage)
							}()
							go func() {
								boxLock.Lock()
								openboxes.Delete(storage)
								boxLock.Unlock()
							}()

						case 14: // 5% - Stress test checkpoint operations
							if box, err := OpenNotebox(ctx, endpoint, storage); err == nil {
								// Test concurrent checkpoints
								for j := 0; j < 5; j++ {
									go func() {
										_ = box.Checkpoint(ctx)
									}()
								}
								box.Close(ctx)
							}

						case 15: // 5% - Test checkpoint during operations
							if box, err := OpenNotebox(ctx, endpoint, storage); err == nil {
								go func() {
									_ = box.Checkpoint(ctx)
								}()
								go func() {
									box.Close(ctx)
								}()
							}

						case 16: // 5% - Nil out internal structures
							boxLock.Lock()
							if v, ok := openboxes.Load(storage); ok {
								if nbi, ok := v.(*NoteboxInstance); ok {
									// This is extremely destructive
									nbi.openfiles = sync.Map{}
								}
							}
							boxLock.Unlock()

						case 17: // 5% - Multiple endpoints same storage
							for j := 0; j < 5; j++ {
								altEndpoint := fmt.Sprintf("alt-endpoint-%d", j)
								_, _ = OpenNotebox(ctx, altEndpoint, storage)
							}

						case 18: // 5% - Recursive operations
							var recursive func(depth int)
							recursive = func(depth int) {
								if depth > 5 {
									return
								}
								if box, err := OpenNotebox(ctx, endpoint, storage); err == nil {
									recursive(depth + 1)
									box.Close(ctx)
								}
							}
							recursive(0)

						case 19: // 5% - Maximum stress everything
							// Do everything at once
							for j := 0; j < 20; j++ {
								go func() {
									_, _ = OpenNotebox(ctx, endpoint, storage)
								}()
							}
							go func() {
								_ = checkpointAllNoteboxes(ctx, true)
							}()
							go func() {
								boxLock.Lock()
								openboxes.Delete(storage)
								boxLock.Unlock()
							}()
						}
					}()
				}
			}
		}(i, strategy)
	}

	// Wait for duration or panic
	select {
	case panicMsg := <-panicFound:
		t.Logf("\n🎯 FOUND PANIC: %s", panicMsg)
		t.Log("Letting test run a bit more to collect additional data...")
		time.Sleep(10 * time.Second)
	case <-time.After(duration):
		t.Log("\nDuration reached, stopping test...")
	}

	t.Log("Signaling workers to stop...")
	close(stop)
	cancel() // Also cancel the context to help stop any blocking operations

	// Give workers time to finish with a timeout
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		t.Log("All workers stopped cleanly")
	case <-time.After(10 * time.Second):
		t.Log("WARNING: Workers did not stop within 10 seconds")

		// Show which workers are still active
		stuckCount := 0
		for i := 0; i < numWorkers; i++ {
			if atomic.LoadInt32(&activeWorkers[i]) == 1 {
				stuckCount++
				if stuckCount <= 10 { // Only show first 10
					t.Logf("  Worker %d (strategy %d) is still active", i, i%20)
				}
			}
		}
		if stuckCount > 10 {
			t.Logf("  ... and %d more workers are still active", stuckCount-10)
		}

		t.Log("Forcing test completion...")
		// Force completion by not waiting anymore
	}

	elapsed := time.Since(startTime)

	// Final report
	t.Logf("\n\n=== 🔥 TORTURE TEST FINAL REPORT 🔥 ===")
	t.Logf("Duration:           %v", elapsed)
	t.Logf("Total operations:   %d (%.0f ops/sec)", stats.operations, float64(stats.operations)/elapsed.Seconds())
	t.Logf("Opens:              %d", stats.opens)
	t.Logf("Closes:             %d", stats.closes)
	t.Logf("Errors:             %d", stats.errors)
	t.Logf("Panics:             %d", stats.panics)
	t.Logf("Nil Openfile:       %d", stats.nilOpenfile)
	t.Logf("Cache deletes:      %d", stats.cacheDeletes)
	t.Logf("Purges:             %d", stats.purges)
	t.Logf("Concurrent opens:   %d", stats.concurrentOpen)

	t.Logf("\nError distribution:")
	errorCounts := make(map[string]int)
	errorDetails.Range(func(key, value interface{}) bool {
		errorCounts[key.(string)]++
		return true
	})
	for err, count := range errorCounts {
		t.Logf("  %s: %d times", err, count)
	}

	t.Logf("\nPanic details:")
	panicDetails.Range(func(key, value interface{}) bool {
		t.Logf("  %s at %v", key, value)
		return true
	})

	// Verdict
	if stats.panics > 0 || stats.nilOpenfile > 0 {
		t.Error("\n❌ CRITICAL RACE CONDITIONS DETECTED!")
		t.Errorf("Found %d panics and %d nil openfile instances", stats.panics, stats.nilOpenfile)
		t.Error("The notebox implementation has serious concurrency bugs that need immediate attention")
	} else if stats.errors > stats.operations/100 {
		t.Errorf("\n⚠️  High error rate detected: %d errors (%.2f%%)",
			stats.errors, float64(stats.errors)/float64(stats.operations)*100)
		t.Error("This suggests potential race conditions even without panics")
	} else {
		t.Log("\n✓ No critical race conditions detected in this run")
		t.Log("However, race conditions may still exist under different timing conditions")
	}
}

// TestNoteboxRaceWithGoRaceDetector should be run with: go test -race
func TestNoteboxRaceWithGoRaceDetector(t *testing.T) {
	longRunningTest(t)
	if !testing.Short() {
		t.Skip("Run with -short to execute this race detector test")
	}

	ctx := context.Background()

	// Set up storage
	tmpDir := t.TempDir()

	FileSetStorageLocation(tmpDir)

	const (
		endpoint   = "gorace-endpoint"
		numWorkers = 100
		numOps     = 1000
	)

	storage := FileStorageObject("gorace_box")

	// Create notebox
	err := CreateNotebox(ctx, endpoint, storage)
	if err != nil {
		t.Fatalf("Failed to create notebox: %v", err)
	}

	t.Logf("Running with Go race detector enabled (-race flag)")
	t.Logf("This test specifically looks for data races")

	var wg sync.WaitGroup

	// Start workers
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			for j := 0; j < numOps; j++ {
				// Various operations that might race
				switch j % 5 {
				case 0:
					// Normal open/close
					if box, err := OpenNotebox(ctx, endpoint, storage); err == nil {
						box.Close(ctx)
					}
				case 1:
					// Concurrent checkpoint
					_ = checkpointAllNoteboxes(ctx, false)
				case 2:
					// Add notefiles
					if box, err := OpenNotebox(ctx, endpoint, storage); err == nil {
						_ = box.AddNotefile(ctx, fmt.Sprintf("race-%d-%d", id, j), nil)
						box.Close(ctx)
					}
				case 3:
					// Checkpoint operations
					if box, err := OpenNotebox(ctx, endpoint, storage); err == nil {
						_ = box.Checkpoint(ctx)
						box.Close(ctx)
					}
				case 4:
					// Direct cache access
					boxLock.Lock()
					openboxes.Range(func(k, v interface{}) bool {
						// Just reading
						return true
					})
					boxLock.Unlock()
				}
			}
		}(i)
	}

	wg.Wait()

	t.Log("Go race detector test completed")
	t.Log("If no races were reported by 'go test -race', that's good")
	t.Log("But the absence of races here doesn't mean there are no logic races")
}
