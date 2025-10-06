package notelib

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestNoteboxShortRaceDetection is a 30-second version suitable for CI/automation
func TestNoteboxShortRaceDetection(t *testing.T) {
	longRunningTest(t)
	// This test always runs, even in short mode
	ctx := context.Background()

	// Set up storage
	tmpDir := t.TempDir()

	FileSetStorageLocation(tmpDir)

	// Aggressive settings for race detection
	oldDebugBox := debugBox
	oldAutoPurgeSeconds := AutoPurgeSeconds
	debugBox = false     // Prevent file descriptor exhaustion
	AutoPurgeSeconds = 0 // Immediate purge
	defer func() {
		debugBox = oldDebugBox
		AutoPurgeSeconds = oldAutoPurgeSeconds
	}()

	const (
		duration   = 30 * time.Second
		numWorkers = 100 // Fewer workers for shorter test
	)

	fmt.Printf("\n🔍 SHORT RACE DETECTION TEST (30s)\n")
	fmt.Printf("Workers: %d\n\n", numWorkers)

	// Create test noteboxes
	const numBoxes = 5
	boxes := make([]string, numBoxes)
	for i := 0; i < numBoxes; i++ {
		boxes[i] = fmt.Sprintf("box%d", i)
		_ = CreateNotebox(ctx, fmt.Sprintf("endpoint%d", i), boxes[i])
	}

	var (
		totalOps         int64
		totalPanics      int64
		totalNilOpenfile int64
	)

	stop := make(chan struct{})
	startTime := time.Now()

	// Simple status reporter
	go func() {
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				ops := atomic.LoadInt64(&totalOps)
				panics := atomic.LoadInt64(&totalPanics)
				nils := atomic.LoadInt64(&totalNilOpenfile)

				fmt.Printf("[%v] Ops: %d | Panics: %d | Nil Openfile: %d\n",
					time.Since(startTime).Round(time.Second),
					ops, panics, nils)

			case <-stop:
				return
			}
		}
	}()

	var wg sync.WaitGroup

	// Concurrent workers performing various operations
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()

			for {
				select {
				case <-stop:
					return
				default:
					atomic.AddInt64(&totalOps, 1)

					// Perform random operations
					boxIdx := workerID % numBoxes
					storage := boxes[boxIdx]
					endpoint := fmt.Sprintf("endpoint%d", boxIdx)

					// Catch panics
					func() {
						defer func() {
							if r := recover(); r != nil {
								atomic.AddInt64(&totalPanics, 1)
							}
						}()

						switch workerID % 5 {
						case 0:
							box, err := OpenNotebox(ctx, endpoint, storage)
							if err == nil && box != nil && box.Openfile() == nil {
								atomic.AddInt64(&totalNilOpenfile, 1)
							}
						case 1:
							_ = checkpointAllNoteboxes(ctx, true)
						case 2:
							_, _ = OpenEndpointNotebox(ctx, endpoint, storage, true)
						case 3:
							// Concurrent opens
							for j := 0; j < 3; j++ {
								go func() {
									_, _ = OpenNotebox(ctx, endpoint, storage)
								}()
							}
						case 4:
							// Force purge
							AutoPurgeSeconds = 0
							_ = checkpointAllNoteboxes(ctx, true)
						}
					}()
				}
			}
		}(i)
	}

	// Run for 30 seconds using a timer
	timer := time.NewTimer(duration)
	<-timer.C
	close(stop)

	// Wait for workers to finish (with timeout to prevent hanging)
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// Workers finished cleanly
	case <-time.After(5 * time.Second):
		// Workers took too long to stop
		t.Log("Warning: Workers took more than 5 seconds to stop")
	}

	// Final report
	elapsed := time.Since(startTime)
	ops := atomic.LoadInt64(&totalOps)
	panics := atomic.LoadInt64(&totalPanics)
	nils := atomic.LoadInt64(&totalNilOpenfile)

	fmt.Printf("\n📊 FINAL RESULTS:\n")
	fmt.Printf("Duration: %v\n", elapsed.Round(time.Second))
	fmt.Printf("Total operations: %d\n", ops)
	fmt.Printf("Operations/sec: %.0f\n", float64(ops)/elapsed.Seconds())
	fmt.Printf("Panics detected: %d\n", panics)
	fmt.Printf("Nil openfile detected: %d\n", nils)

	// Fail if we detected race conditions
	if panics > 0 || nils > 0 {
		t.Errorf("Race conditions detected: %d panics, %d nil openfile returns", panics, nils)
	}
}

// TestNoteboxContinuousRaceDetection runs continuously until it finds clear evidence
func TestNoteboxContinuousRaceDetection(t *testing.T) {
	longRunningTest(t)

	// Skip this test unless explicitly enabled via environment variable
	if os.Getenv("RUN_CONTINUOUS_RACE_TEST") != "true" {
		t.Skip("Skipping continuous race detection test. Set RUN_CONTINUOUS_RACE_TEST=true to run.")
	}

	ctx := context.Background()

	// Set up storage
	tmpDir := t.TempDir()

	FileSetStorageLocation(tmpDir)

	// Maximum aggression settings
	oldDebugBox := debugBox
	oldAutoPurgeSeconds := AutoPurgeSeconds
	// IMPORTANT: Don't enable debugBox with 5000 workers - it will exhaust file descriptors!
	// Each log message does a synchronous write to stdout, and with thousands of concurrent
	// operations, we hit the OS limit of 1048575 concurrent file operations.
	debugBox = false     // Disabled to prevent "too many concurrent operations" panic
	AutoPurgeSeconds = 0 // Immediate purge
	defer func() {
		debugBox = oldDebugBox
		AutoPurgeSeconds = oldAutoPurgeSeconds
	}()

	// Check for environment variable override
	duration := 1 * time.Hour // Default for manual runs
	if envDuration := os.Getenv("NOTEBOX_RACE_TEST_DURATION"); envDuration != "" {
		if parsed, err := time.ParseDuration(envDuration); err == nil {
			duration = parsed
		}
	}

	const (
		numWorkers   = 5000 // Maximum workers
		targetPanics = 10   // Stop after finding this many panics
	)

	fmt.Printf("\n🔥 CONTINUOUS RACE DETECTION TEST 🔥\n")
	fmt.Printf("This test will run until it finds %d race condition instances\n", targetPanics)
	fmt.Printf("Maximum duration: %v\n", duration)
	fmt.Printf("Workers: %d\n\n", numWorkers)

	// Create multiple noteboxes for chaos
	const numBoxes = 20
	boxes := make([]string, numBoxes)
	for i := 0; i < numBoxes; i++ {
		storage := FileStorageObject(fmt.Sprintf("continuous_box_%d", i))
		endpoint := fmt.Sprintf("continuous_endpoint_%d", i)
		boxes[i] = storage

		err := CreateNotebox(ctx, endpoint, storage)
		if err != nil {
			t.Fatalf("Failed to create notebox %d: %v", i, err)
		}
	}

	// Tracking
	var (
		totalOps         int64
		totalPanics      int64
		totalNilOpenfile int64
		foundEvidence    atomic.Value
	)
	foundEvidence.Store(false)

	// Evidence collection
	type Evidence struct {
		Time      time.Time
		Worker    int
		Type      string
		Message   string
		Operation string
	}

	var (
		evidenceMu sync.Mutex
		evidence   []Evidence
	)

	stop := make(chan struct{})
	startTime := time.Now()

	// Status reporter
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()

		lastOps := int64(0)

		for {
			select {
			case <-ticker.C:
				ops := atomic.LoadInt64(&totalOps)
				opsPerSec := float64(ops-lastOps) / 5.0
				lastOps = ops

				panics := atomic.LoadInt64(&totalPanics)
				nils := atomic.LoadInt64(&totalNilOpenfile)

				fmt.Printf("\r[%v] Ops/sec: %.0f | Total: %d | PANICS: %d | NIL OPENFILE: %d | Goroutines: %d    ",
					time.Since(startTime).Round(time.Second),
					opsPerSec,
					ops,
					panics,
					nils,
					runtime.NumGoroutine())

				// Check if we found enough evidence
				if panics+nils >= int64(targetPanics) {
					foundEvidence.Store(true)
					close(stop)
					return
				}

			case <-stop:
				return
			}
		}
	}()

	var wg sync.WaitGroup

	// Chaos workers - each implements a different attack strategy
	for i := 0; i < numWorkers; i++ {
		strategy := i % 30 // 30 different strategies

		wg.Add(1)
		go func(workerID int, strategy int) {
			defer wg.Done()

			for {
				select {
				case <-stop:
					return
				default:
					// Pick random notebox
					boxIdx := workerID % numBoxes
					storage := boxes[boxIdx]
					endpoint := fmt.Sprintf("continuous_endpoint_%d", boxIdx)

					func() {
						operation := ""
						defer func() {
							if r := recover(); r != nil {
								atomic.AddInt64(&totalPanics, 1)

								evidenceMu.Lock()
								evidence = append(evidence, Evidence{
									Time:      time.Now(),
									Worker:    workerID,
									Type:      "PANIC",
									Message:   fmt.Sprintf("%v", r),
									Operation: operation,
								})
								evidenceMu.Unlock()

								fmt.Printf("\n💥 PANIC #%d in worker %d: %v\n",
									atomic.LoadInt64(&totalPanics), workerID, r)
							}
						}()

						atomic.AddInt64(&totalOps, 1)

						switch strategy {
						case 0, 1, 2, 3, 4, 5: // 20% - Standard open/close
							operation = "standard open/close"
							box, err := OpenNotebox(ctx, endpoint, storage)
							if err != nil {
								return
							}

							if box.Openfile() == nil {
								atomic.AddInt64(&totalNilOpenfile, 1)

								evidenceMu.Lock()
								evidence = append(evidence, Evidence{
									Time:      time.Now(),
									Worker:    workerID,
									Type:      "NIL_OPENFILE",
									Message:   "Openfile() returned nil",
									Operation: operation,
								})
								evidenceMu.Unlock()

								fmt.Printf("\n🚨 NIL OPENFILE #%d in worker %d\n",
									atomic.LoadInt64(&totalNilOpenfile), workerID)
								panic("nil openfile detected")
							}

							box.Close(ctx)

						case 6, 7, 8: // 10% - Rapid fire open/close
							operation = "rapid fire"
							for j := 0; j < 100; j++ {
								if box, err := OpenNotebox(ctx, endpoint, storage); err == nil {
									if box.Openfile() == nil {
										panic("nil openfile in rapid fire")
									}
									box.Close(ctx)
								}
							}

						case 9, 10: // 7% - Concurrent opens on same box
							operation = "concurrent opens"
							var innerWg sync.WaitGroup
							for j := 0; j < 50; j++ {
								innerWg.Add(1)
								go func() {
									defer innerWg.Done()
									if box, err := OpenNotebox(ctx, endpoint, storage); err == nil {
										if box.Openfile() == nil {
											panic("nil openfile in concurrent open")
										}
										box.Close(ctx)
									}
								}()
							}
							innerWg.Wait()

						case 11: // 3% - Force aggressive purge
							operation = "aggressive purge"
							_ = checkpointAllNoteboxes(ctx, true)

						case 12: // 3% - Delete from cache during operations
							operation = "cache delete"
							go func() {
								for k := 0; k < 10; k++ {
									boxLock.Lock()
									openboxes.Delete(storage)
									boxLock.Unlock()
									time.Sleep(time.Microsecond)
								}
							}()

							// Try to use it while being deleted
							for k := 0; k < 10; k++ {
								if box, err := OpenNotebox(ctx, endpoint, storage); err == nil {
									if box.Openfile() == nil {
										panic("nil openfile after cache delete")
									}
									box.Close(ctx)
								}
							}

						case 13: // 3% - Corrupt internal state
							operation = "corrupt state"
							boxLock.Lock()
							if v, ok := openboxes.Load(storage); ok {
								if nbi, ok := v.(*NoteboxInstance); ok {
									// Clear the openfiles map
									nbi.openfiles = sync.Map{}
								}
							}
							boxLock.Unlock()

						case 14: // 3% - Race between check and reopen
							operation = "check/reopen race"
							// This specifically targets the bug

							// First, ensure it's in cache but closed
							box, _ := OpenNotebox(ctx, endpoint, storage)
							if box != nil {
								box.Close(ctx)
							}

							// Now race
							var raceWg sync.WaitGroup
							raceWg.Add(2)

							go func() {
								defer raceWg.Done()
								// Try to open - this checks existence then reopens
								if box, err := OpenNotebox(ctx, endpoint, storage); err == nil {
									if box.Openfile() == nil {
										panic("nil openfile in reopen race")
									}
									box.Close(ctx)
								}
							}()

							go func() {
								defer raceWg.Done()
								// Delete from cache at the critical moment
								time.Sleep(time.Nanosecond * 100)
								boxLock.Lock()
								openboxes.Delete(storage)
								boxLock.Unlock()
							}()

							raceWg.Wait()

						case 15: // 3% - Multiple endpoints same storage
							operation = "multiple endpoints"
							for j := 0; j < 5; j++ {
								altEndpoint := fmt.Sprintf("alt-%d-%d", workerID, j)
								if box, err := OpenNotebox(ctx, altEndpoint, storage); err == nil {
									if box.Openfile() == nil {
										panic("nil openfile with alt endpoint")
									}
									box.Close(ctx)
								}
							}

						case 16: // 3% - Leak handles then force purge
							operation = "leak and purge"
							// Open without closing
							for j := 0; j < 20; j++ {
								_, _ = OpenNotebox(ctx, endpoint, storage)
							}
							// Force purge
							_ = checkpointAllNoteboxes(ctx, true)

						case 17: // 3% - Stress notefiles
							operation = "stress notefiles"
							if box, err := OpenNotebox(ctx, endpoint, storage); err == nil {
								// Add many notefiles
								for j := 0; j < 100; j++ {
									notefileID := fmt.Sprintf("stress-%d-%d", workerID, j)
									_ = box.AddNotefile(ctx, notefileID, nil)
								}
								box.Close(ctx)
							}

						default: // Remaining % - Mixed chaos
							operation = "chaos"
							// Do random destructive things
							switch workerID % 5 {
							case 0:
								_, _ = OpenNotebox(ctx, endpoint, storage)
							case 1:
								_ = checkpointAllNoteboxes(ctx, true)
							case 2:
								boxLock.Lock()
								openboxes.Range(func(k, v interface{}) bool {
									if workerID%10 == 0 {
										openboxes.Delete(k)
									}
									return true
								})
								boxLock.Unlock()
							case 3:
								for j := 0; j < 10; j++ {
									go func() {
										_, _ = OpenNotebox(ctx, endpoint, storage)
									}()
								}
							case 4:
								// Just cause general mayhem
								go func() {
									_ = checkpointAllNoteboxes(ctx, true)
								}()
								go func() {
									_, _ = OpenNotebox(ctx, endpoint, storage)
								}()
								go func() {
									boxLock.Lock()
									openboxes.Delete(storage)
									boxLock.Unlock()
								}()
							}
						}
					}()
				}
			}
		}(i, strategy)
	}

	// Wait for evidence or timeout
	select {
	case <-stop:
		fmt.Printf("\n\n✅ Found sufficient evidence of race conditions!\n")
	case <-time.After(duration):
		fmt.Printf("\n\n⏱️  Reached maximum duration\n")
		close(stop)
	}

	// Let workers stop
	wg.Wait()

	// Final report
	elapsed := time.Since(startTime)

	fmt.Printf("\n\n========== CONTINUOUS RACE DETECTION RESULTS ==========\n")
	fmt.Printf("Duration:          %v\n", elapsed)
	fmt.Printf("Total operations:  %d (%.0f ops/sec)\n",
		totalOps, float64(totalOps)/elapsed.Seconds())
	fmt.Printf("Panics found:      %d\n", totalPanics)
	fmt.Printf("Nil Openfile:      %d\n", totalNilOpenfile)
	fmt.Printf("Total evidence:    %d race conditions detected\n", totalPanics+totalNilOpenfile)

	if len(evidence) > 0 {
		fmt.Printf("\n📋 DETAILED EVIDENCE:\n")
		fmt.Println(strings.Repeat("-", 100))

		for i, e := range evidence {
			fmt.Printf("%d. [%s] Worker %d - %s during '%s'\n",
				i+1,
				e.Time.Format("15:04:05.000"),
				e.Worker,
				e.Type,
				e.Operation)
			fmt.Printf("   Message: %s\n", e.Message)

			if i >= 19 { // Show first 20
				remaining := len(evidence) - 20
				if remaining > 0 {
					fmt.Printf("\n   ... and %d more instances\n", remaining)
				}
				break
			}
		}
		fmt.Println(strings.Repeat("-", 100))
	}

	// Conclusion
	if totalPanics > 0 || totalNilOpenfile > 0 {
		t.Errorf("\n❌ RACE CONDITIONS CONFIRMED!")
		t.Errorf("Found %d panics and %d nil openfile returns", totalPanics, totalNilOpenfile)
		t.Error("\nThe notebox implementation has serious concurrency bugs:")
		t.Error("- OpenNotebox can return a box with nil Openfile() due to race conditions")
		t.Error("- The cache can be corrupted by concurrent operations")
		t.Error("- Purging can happen while boxes are being opened")
		t.Error("\nThese bugs can cause crashes in production!")
	} else {
		t.Log("\n⚠️  No definitive race conditions found in this run")
		t.Log("However, the implementation still appears to have potential race conditions")
		t.Log("Longer runs or different timing may reveal them")
	}
}

// TestNoteboxInfiniteRaceHunt runs forever until stopped, logging all findings
func TestNoteboxInfiniteRaceHunt(t *testing.T) {
	longRunningTest(t)

	if !testing.Short() {
		t.Skip("Run with -short flag to start the infinite race hunt")
	}

	ctx := context.Background()

	// Set up storage
	tmpDir := t.TempDir()

	FileSetStorageLocation(tmpDir)

	// Maximum settings
	debugBox = false // Disabled to prevent "too many concurrent operations" panic
	AutoPurgeSeconds = 0

	const numWorkers = 10000 // Maximum chaos

	fmt.Printf("\n🔥🔥🔥 INFINITE RACE HUNT STARTED 🔥🔥🔥\n")
	fmt.Printf("This will run forever until you stop it with Ctrl+C\n")
	fmt.Printf("All race conditions found will be logged\n")
	fmt.Printf("Workers: %d\n\n", numWorkers)

	// Create noteboxes
	storage := FileStorageObject("infinite_box")
	endpoint := "infinite_endpoint"

	err := CreateNotebox(ctx, endpoint, storage)
	if err != nil {
		t.Fatalf("Failed to create notebox: %v", err)
	}

	var (
		totalOps    int64
		totalPanics int64
		totalNils   int64
	)

	startTime := time.Now()

	// Logger
	go func() {
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()

		for range ticker.C {
			elapsed := time.Since(startTime)
			ops := atomic.LoadInt64(&totalOps)

			fmt.Printf("\r[%v] Ops: %d (%.0f/s) | FOUND: %d panics, %d nils    ",
				elapsed.Round(time.Second),
				ops,
				float64(ops)/elapsed.Seconds(),
				atomic.LoadInt64(&totalPanics),
				atomic.LoadInt64(&totalNils))
		}
	}()

	// Infinite workers
	for i := 0; i < numWorkers; i++ {
		go func(id int) {
			for {
				func() {
					defer func() {
						if r := recover(); r != nil {
							atomic.AddInt64(&totalPanics, 1)
							fmt.Printf("\n💥 [%v] PANIC #%d: %v\n",
								time.Now().Format("15:04:05.000"),
								atomic.LoadInt64(&totalPanics),
								r)
						}
					}()

					atomic.AddInt64(&totalOps, 1)

					// Random operations
					switch id % 10 {
					case 0, 1, 2, 3, 4:
						// Open/close
						if box, err := OpenNotebox(ctx, endpoint, storage); err == nil {
							if box.Openfile() == nil {
								atomic.AddInt64(&totalNils, 1)
								fmt.Printf("\n🚨 [%v] NIL #%d detected!\n",
									time.Now().Format("15:04:05.000"),
									atomic.LoadInt64(&totalNils))
								panic("nil openfile")
							}
							box.Close(ctx)
						}
					case 5:
						_ = checkpointAllNoteboxes(ctx, true)
					case 6:
						boxLock.Lock()
						openboxes.Delete(storage)
						boxLock.Unlock()
					default:
						// Chaos
						go func() {
							_, _ = OpenNotebox(ctx, endpoint, storage)
						}()
						go func() {
							_ = checkpointAllNoteboxes(ctx, true)
						}()
					}
				}()
			}
		}(i)
	}

	// Run forever
	select {}
}
