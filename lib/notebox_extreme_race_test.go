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

// TestNoteboxExtremeRace runs with extreme concurrency and duration to force race conditions
func TestNoteboxExtremeRace(t *testing.T) {
	longRunningTest(t)
	ctx := context.Background()

	// Set up storage
	tmpDir := t.TempDir()

	FileSetStorageLocation(tmpDir)

	// Enable debug output
	oldDebugBox := debugBox
	debugBox = false
	defer func() {
		debugBox = oldDebugBox
	}()

	// Internal parameterized duration - default to 30s
	duration := 30 * time.Second
	if envDuration := os.Getenv("NOTEBOX_EXTREME_RACE_DURATION"); envDuration != "" {
		if parsed, err := time.ParseDuration(envDuration); err == nil {
			duration = parsed
		}
	}

	const (
		endpoint       = "extreme-race-endpoint"
		numWorkers     = 500 // 5x more workers
		reportInterval = 10 * time.Second
	)

	storageObject := FileStorageObject("extreme_race_box")

	// Create the notebox
	err := CreateNotebox(ctx, endpoint, storageObject)
	if err != nil {
		t.Fatalf("Failed to create notebox: %v", err)
	}

	// Counters
	var (
		operations     int64
		opens          int64
		closes         int64
		errors         int64
		panics         int64
		nilOpenfile    int64
		deviceNotFound int64
	)

	// Track all panic messages
	panicMessages := sync.Map{}
	errorMessages := sync.Map{}

	// Channel to stop workers
	stop := make(chan struct{})
	var wg sync.WaitGroup

	t.Logf("Starting %d workers for %v...", numWorkers, duration)
	t.Logf("This test will stress the system heavily")

	startTime := time.Now()

	// Periodic reporter
	go func() {
		ticker := time.NewTicker(reportInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				t.Logf("\n--- Progress Report ---")
				t.Logf("Operations: %d (%.0f ops/sec)",
					atomic.LoadInt64(&operations),
					float64(atomic.LoadInt64(&operations))/time.Since(startTime).Seconds())
				t.Logf("Opens: %d, Closes: %d",
					atomic.LoadInt64(&opens),
					atomic.LoadInt64(&closes))
				t.Logf("Errors: %d, Panics: %d, Nil Openfile: %d",
					atomic.LoadInt64(&errors),
					atomic.LoadInt64(&panics),
					atomic.LoadInt64(&nilOpenfile))
				t.Logf("Device not found errors: %d", atomic.LoadInt64(&deviceNotFound))
				t.Logf("Goroutines: %d", runtime.NumGoroutine())
			case <-stop:
				return
			}
		}
	}()

	// Start workers with different behaviors
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			for {
				select {
				case <-stop:
					return
				default:
					// Different strategies for different workers
					strategy := id % 10

					func() {
						defer func() {
							if r := recover(); r != nil {
								atomic.AddInt64(&panics, 1)
								panicStr := fmt.Sprintf("%v", r)
								panicMessages.Store(panicStr, atomic.AddInt64(&panics, 0))
								t.Logf("Worker %d PANIC (strategy %d): %v", id, strategy, r)
							}
						}()

						atomic.AddInt64(&operations, 1)

						switch strategy {
						case 0, 1, 2:
							// Rapid open/close - most common
							box, err := OpenNotebox(ctx, endpoint, storageObject)
							if err != nil {
								atomic.AddInt64(&errors, 1)
								if err.Error() == "device not found {device-noexist}" {
									atomic.AddInt64(&deviceNotFound, 1)
								}
								errorMessages.Store(err.Error(), true)
								return
							}
							atomic.AddInt64(&opens, 1)

							// Check for nil openfile
							of := box.Openfile()
							if of == nil {
								atomic.AddInt64(&nilOpenfile, 1)
								t.Logf("Worker %d: CRITICAL - Openfile() returned nil!", id)
							}

							err = box.Close(ctx)
							if err != nil {
								atomic.AddInt64(&errors, 1)
								errorMessages.Store(err.Error(), true)
							} else {
								atomic.AddInt64(&closes, 1)
							}

						case 3, 4:
							// Open multiple times without closing (stress open count)
							boxes := make([]*Notebox, 5)
							for j := 0; j < 5; j++ {
								box, err := OpenNotebox(ctx, endpoint, storageObject)
								if err != nil {
									atomic.AddInt64(&errors, 1)
									errorMessages.Store(err.Error(), true)
									// Close any we opened
									for k := 0; k < j; k++ {
										if boxes[k] != nil {
											boxes[k].Close(ctx)
										}
									}
									return
								}
								boxes[j] = box
								atomic.AddInt64(&opens, 1)
							}

							// Close them all
							for _, box := range boxes {
								if box != nil {
									box.Close(ctx)
									atomic.AddInt64(&closes, 1)
								}
							}

						case 5:
							// Aggressive purger
							_ = checkpointAllNoteboxes(ctx, true)

						case 6:
							// Open, add notefiles, close
							box, err := OpenNotebox(ctx, endpoint, storageObject)
							if err != nil {
								atomic.AddInt64(&errors, 1)
								errorMessages.Store(err.Error(), true)
								return
							}
							atomic.AddInt64(&opens, 1)

							// Add multiple notefiles
							for j := 0; j < 10; j++ {
								notefileID := fmt.Sprintf("stress-%d-%d.db", id, j)
								_ = box.AddNotefile(ctx, notefileID, nil)
							}

							box.Close(ctx)
							atomic.AddInt64(&closes, 1)

						case 7:
							// Try to break the cache
							boxLock.Lock()
							if v, ok := openboxes.Load(storageObject); ok {
								if nbi, ok := v.(*NoteboxInstance); ok {
									// Mess with close times
									nbi.openfiles.Range(func(k, v interface{}) bool {
										if of, ok := v.(*OpenNotefile); ok {
											of.closeTime = time.Now().Add(-time.Hour)
										}
										return true
									})
								}
							}
							boxLock.Unlock()

						case 8:
							// Direct deletion from cache (simulate extreme race)
							if id%100 == 0 { // Only occasionally to not break everything
								boxLock.Lock()
								openboxes.Delete(storageObject)
								boxLock.Unlock()
							}

						case 9:
							// Multiple simultaneous operations
							var innerWg sync.WaitGroup
							for j := 0; j < 5; j++ {
								innerWg.Add(1)
								go func() {
									defer innerWg.Done()
									box, err := OpenNotebox(ctx, endpoint, storageObject)
									if err == nil {
										atomic.AddInt64(&opens, 1)
										box.Close(ctx)
										atomic.AddInt64(&closes, 1)
									} else {
										atomic.AddInt64(&errors, 1)
									}
								}()
							}
							innerWg.Wait()
						}
					}()
				}
			}
		}(i)
	}

	// Let it run
	time.Sleep(duration)
	close(stop)

	t.Log("\nStopping workers...")
	wg.Wait()

	elapsed := time.Since(startTime)

	// Final report
	t.Logf("\n=== EXTREME RACE TEST FINAL RESULTS ===")
	t.Logf("Duration:              %v", elapsed)
	t.Logf("Total operations:      %d (%.0f ops/sec)", operations, float64(operations)/elapsed.Seconds())
	t.Logf("Opens:                 %d", opens)
	t.Logf("Closes:                %d", closes)
	t.Logf("Errors:                %d", errors)
	t.Logf("Panics:                %d", panics)
	t.Logf("Nil Openfile:          %d", nilOpenfile)
	t.Logf("Device not found:      %d", deviceNotFound)

	t.Logf("\nUnique panic types:")
	panicMessages.Range(func(key, value interface{}) bool {
		t.Logf("  - %s (last at panic #%d)", key, value)
		return true
	})

	t.Logf("\nUnique error types:")
	errorMessages.Range(func(key, value interface{}) bool {
		t.Logf("  - %s", key)
		return true
	})

	if panics > 0 || nilOpenfile > 0 {
		t.Error("RACE CONDITION DETECTED! The test found evidence of concurrent access bugs.")
	} else if errors > 100 {
		t.Logf("High error rate detected (%d errors), which may indicate race conditions", errors)
	} else {
		t.Log("No obvious race conditions detected in this run, but they may still exist")
	}
}

// TestNoteboxUntilPanic runs until it catches a panic or nil openfile
func TestNoteboxUntilPanic(t *testing.T) {
	longRunningTest(t)
	ctx := context.Background()

	// Set up storage
	tmpDir := t.TempDir()

	FileSetStorageLocation(tmpDir)

	// Enable debug
	oldDebugBox := debugBox
	debugBox = false
	defer func() {
		debugBox = oldDebugBox
	}()

	// Maximum duration - default to 30s, can be overridden by env var
	maxDuration := 30 * time.Second
	if envDuration := os.Getenv("NOTEBOX_UNTIL_PANIC_DURATION"); envDuration != "" {
		if parsed, err := time.ParseDuration(envDuration); err == nil {
			maxDuration = parsed
		}
	}

	const (
		endpoint   = "until-panic-endpoint"
		numWorkers = 1000 // Maximum workers
	)

	storageObject := FileStorageObject("until_panic_box")

	// Create the notebox
	err := CreateNotebox(ctx, endpoint, storageObject)
	if err != nil {
		t.Fatalf("Failed to create notebox: %v", err)
	}

	var (
		operations       int64
		foundPanic       atomic.Value
		foundNilOpenfile atomic.Value
	)

	foundPanic.Store(false)
	foundNilOpenfile.Store(false)

	stop := make(chan struct{})
	var wg sync.WaitGroup

	t.Logf("Running %d workers until we find a panic or nil openfile (max %v)...", numWorkers, maxDuration)

	startTime := time.Now()

	// Progress reporter
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				elapsed := time.Since(startTime)
				ops := atomic.LoadInt64(&operations)
				t.Logf("Progress: %v elapsed, %d operations (%.0f ops/sec)",
					elapsed.Round(time.Second), ops, float64(ops)/elapsed.Seconds())
			case <-stop:
				return
			}
		}
	}()

	// Start aggressive workers
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			for {
				select {
				case <-stop:
					return
				default:
					if foundPanic.Load().(bool) || foundNilOpenfile.Load().(bool) {
						return
					}

					func() {
						defer func() {
							if r := recover(); r != nil {
								t.Logf("\n🎯 FOUND PANIC in worker %d after %d operations: %v",
									id, atomic.LoadInt64(&operations), r)
								foundPanic.Store(true)
								close(stop)
							}
						}()

						atomic.AddInt64(&operations, 1)

						// Aggressive operations
						switch id % 5 {
						case 0:
							// Standard open/close
							box, err := OpenNotebox(ctx, endpoint, storageObject)
							if err == nil {
								if box.Openfile() == nil {
									t.Logf("\n🎯 FOUND NIL OPENFILE in worker %d after %d operations!",
										id, atomic.LoadInt64(&operations))
									foundNilOpenfile.Store(true)
									close(stop)
									return
								}
								box.Close(ctx)
							}

						case 1:
							// Multiple rapid operations
							for j := 0; j < 10; j++ {
								box, _ := OpenNotebox(ctx, endpoint, storageObject)
								if box != nil {
									if box.Openfile() == nil {
										t.Logf("\n🎯 FOUND NIL OPENFILE in worker %d after %d operations!",
											id, atomic.LoadInt64(&operations))
										foundNilOpenfile.Store(true)
										close(stop)
										return
									}
									box.Close(ctx)
								}
							}

						case 2:
							// Force purge
							_ = checkpointAllNoteboxes(ctx, true)

						case 3:
							// Corrupt cache
							boxLock.Lock()
							openboxes.Delete(storageObject)
							boxLock.Unlock()

						case 4:
							// Simultaneous opens
							var boxes []*Notebox
							for j := 0; j < 20; j++ {
								if box, err := OpenNotebox(ctx, endpoint, storageObject); err == nil {
									boxes = append(boxes, box)
								}
							}
							for _, box := range boxes {
								if box != nil {
									box.Close(ctx)
								}
							}
						}
					}()
				}
			}
		}(i)
	}

	// Wait for either condition or timeout
	select {
	case <-stop:
		// Found something!
	case <-time.After(maxDuration):
		t.Logf("\nReached maximum duration without finding issue")
		close(stop)
	}

	wg.Wait()

	elapsed := time.Since(startTime)
	finalOps := atomic.LoadInt64(&operations)

	t.Logf("\n=== UNTIL PANIC TEST RESULTS ===")
	t.Logf("Duration:         %v", elapsed)
	t.Logf("Total operations: %d (%.0f ops/sec)", finalOps, float64(finalOps)/elapsed.Seconds())
	t.Logf("Found panic:      %v", foundPanic.Load().(bool))
	t.Logf("Found nil openfile: %v", foundNilOpenfile.Load().(bool))

	if foundPanic.Load().(bool) || foundNilOpenfile.Load().(bool) {
		t.Error("Successfully reproduced the race condition!")
	} else {
		t.Log("Did not reproduce the race condition within the time limit")
		t.Log("The race may require very specific timing or conditions")
	}
}
