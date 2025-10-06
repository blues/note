package notelib

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestNoteboxVisualRaceEvidence provides clear visual evidence of race conditions
func TestNoteboxVisualRaceEvidence(t *testing.T) {
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

	const (
		endpoint   = "visual-race-endpoint"
		numWorkers = 1000
		duration   = 30 * time.Second
	)

	storage := FileStorageObject("visual_race_box")

	// Create notebox
	err := CreateNotebox(ctx, endpoint, storage)
	if err != nil {
		t.Fatalf("Failed to create notebox: %v", err)
	}

	// Visual indicators
	type RaceEvidence struct {
		timestamp time.Time
		worker    int
		event     string
		details   string
	}

	var (
		evidenceMu sync.Mutex
		evidence   []RaceEvidence
	)

	addEvidence := func(worker int, event, details string) {
		evidenceMu.Lock()
		evidence = append(evidence, RaceEvidence{
			timestamp: time.Now(),
			worker:    worker,
			event:     event,
			details:   details,
		})
		evidenceMu.Unlock()
	}

	// Counters for visual display
	var (
		totalOps      int64
		successfulOps int64
		nilOpenfile   int64
		panics        int64
		errors        int64
		suspiciousOps int64
	)

	// Different colored output for different events
	const (
		colorRed    = "\033[31m"
		colorGreen  = "\033[32m"
		colorYellow = "\033[33m"
		colorBlue   = "\033[34m"
		colorReset  = "\033[0m"
		colorBold   = "\033[1m"
	)

	stop := make(chan struct{})
	var wg sync.WaitGroup

	fmt.Printf("\n%s%s=== VISUAL RACE CONDITION TEST ===%s\n", colorBold, colorBlue, colorReset)
	fmt.Printf("Running %d workers for %v to find race conditions...\n\n", numWorkers, duration)

	startTime := time.Now()

	// Real-time display
	go func() {
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()

		lastOps := int64(0)

		for {
			select {
			case <-ticker.C:
				ops := atomic.LoadInt64(&totalOps)
				opsThisSec := ops - lastOps
				lastOps = ops

				// Clear line and print status
				fmt.Printf("\r[%s] Ops/sec: %s%d%s | Success: %s%d%s | Errors: %s%d%s | Suspicious: %s%d%s | NIL: %s%d%s | PANIC: %s%d%s   ",
					time.Since(startTime).Round(time.Second),
					colorGreen, opsThisSec, colorReset,
					colorGreen, atomic.LoadInt64(&successfulOps), colorReset,
					colorYellow, atomic.LoadInt64(&errors), colorReset,
					colorYellow, atomic.LoadInt64(&suspiciousOps), colorReset,
					colorRed, atomic.LoadInt64(&nilOpenfile), colorReset,
					colorRed+colorBold, atomic.LoadInt64(&panics), colorReset,
				)

			case <-stop:
				fmt.Println()
				return
			}
		}
	}()

	// Workers
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			consecutiveErrors := 0

			for {
				select {
				case <-stop:
					return
				default:
					func() {
						defer func() {
							if r := recover(); r != nil {
								atomic.AddInt64(&panics, 1)

								fmt.Printf("\n%s%s💥 PANIC in worker %d: %v%s\n",
									colorRed, colorBold, id, r, colorReset)

								addEvidence(id, "PANIC", fmt.Sprintf("%v", r))

								// Don't stop - keep going to find more
							}
						}()

						atomic.AddInt64(&totalOps, 1)

						// Try to open
						box, err := OpenNotebox(ctx, endpoint, storage)
						if err != nil {
							atomic.AddInt64(&errors, 1)
							consecutiveErrors++

							// Suspicious if we get many errors in a row
							if consecutiveErrors > 10 {
								atomic.AddInt64(&suspiciousOps, 1)
								fmt.Printf("\n%s⚠️  Worker %d: %d consecutive errors - possible race condition%s\n",
									colorYellow, id, consecutiveErrors, colorReset)
								addEvidence(id, "CONSECUTIVE_ERRORS", fmt.Sprintf("%d errors, last: %v", consecutiveErrors, err))
							}

							return
						}

						consecutiveErrors = 0

						// CRITICAL CHECK - this is what we're looking for
						of := box.Openfile()
						if of == nil {
							atomic.AddInt64(&nilOpenfile, 1)

							fmt.Printf("\n%s%s🚨 CRITICAL: Worker %d got NIL OPENFILE! This is the race condition!%s\n",
								colorRed, colorBold, id, colorReset)

							addEvidence(id, "NIL_OPENFILE", "Openfile() returned nil after successful OpenNotebox")

							// Try to gather more info
							fmt.Printf("%s   Stack info would show OpenNotebox succeeded but internal state is corrupted%s\n",
								colorRed, colorReset)

							panic("nil openfile - race condition detected!")
						}

						atomic.AddInt64(&successfulOps, 1)

						// Do some work to stress the system
						if id%10 == 0 {
							// Some workers add notefiles
							for j := 0; j < 5; j++ {
								notefileID := fmt.Sprintf("stress-%d-%d", id, j)
								_ = box.AddNotefile(ctx, notefileID, nil)
							}
						}

						// Close
						err = box.Close(ctx)
						if err != nil {
							atomic.AddInt64(&errors, 1)
						}

						// Some workers force purges
						if id%50 == 0 {
							_ = checkpointAllNoteboxes(ctx, true)
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

	// Final report with visual evidence
	fmt.Printf("\n\n%s%s========== RACE CONDITION TEST RESULTS ==========%s\n",
		colorBold, colorBlue, colorReset)

	elapsed := time.Since(startTime)
	fmt.Printf("Test duration: %v\n", elapsed)
	fmt.Printf("Total operations: %d (%.0f ops/sec)\n", totalOps, float64(totalOps)/elapsed.Seconds())

	fmt.Printf("\n%sResults:%s\n", colorBold, colorReset)
	fmt.Printf("  %s✓%s Successful operations: %d\n", colorGreen, colorReset, successfulOps)
	fmt.Printf("  %s⚠%s  Errors encountered: %d\n", colorYellow, colorReset, errors)
	fmt.Printf("  %s⚠%s  Suspicious patterns: %d\n", colorYellow, colorReset, suspiciousOps)
	fmt.Printf("  %s✗%s NIL Openfile (RACE!): %d\n", colorRed, colorReset, nilOpenfile)
	fmt.Printf("  %s✗%s Panics: %d\n", colorRed, colorReset, panics)

	if len(evidence) > 0 {
		fmt.Printf("\n%s%sRACE CONDITION EVIDENCE:%s\n", colorBold, colorRed, colorReset)
		fmt.Println(strings.Repeat("=", 80))

		// Show up to 10 most interesting pieces of evidence
		shown := 0
		for _, e := range evidence {
			if e.event == "NIL_OPENFILE" || e.event == "PANIC" {
				fmt.Printf("%s[%s] Worker %d: %s%s%s\n",
					colorYellow,
					e.timestamp.Format("15:04:05.000"),
					e.worker,
					colorRed+colorBold,
					e.event,
					colorReset)
				fmt.Printf("  %sDetails: %s%s\n", colorRed, e.details, colorReset)
				shown++
				if shown >= 10 {
					break
				}
			}
		}

		fmt.Println(strings.Repeat("=", 80))
	}

	// Verdict
	if nilOpenfile > 0 || panics > 0 {
		t.Errorf("\n%s%s❌ RACE CONDITION CONFIRMED!%s\n", colorRed, colorBold, colorReset)
		t.Errorf("Found %d instances of nil Openfile and %d panics", nilOpenfile, panics)
		t.Error("\nThe notebox has a race condition where:")
		t.Error("1. OpenNotebox checks if a notebox exists in the cache")
		t.Error("2. Between that check and calling uReopenNotebox, another goroutine purges it")
		t.Error("3. uReopenNotebox assumes it exists and crashes or returns invalid state")
		t.Error("\nThis is a critical bug that needs immediate fixing!")
	} else if suspiciousOps > 0 {
		t.Logf("\n%s⚠️  Suspicious patterns detected%s", colorYellow, colorReset)
		t.Log("While no definitive race was caught, the error patterns suggest potential issues")
	} else {
		t.Log("\n✓ No race conditions detected in this run")
		t.Log("However, races may still exist - they're timing-dependent")
	}
}

// TestNoteboxRaceReproducer tries to reproduce the exact race condition scenario
func TestNoteboxRaceReproducer(t *testing.T) {
	longRunningTest(t)
	ctx := context.Background()

	tmpDir := t.TempDir()

	FileSetStorageLocation(tmpDir)

	// Make purging very aggressive
	oldAutoPurgeSeconds := AutoPurgeSeconds
	AutoPurgeSeconds = 0
	defer func() {
		AutoPurgeSeconds = oldAutoPurgeSeconds
	}()

	fmt.Println("\n=== RACE CONDITION REPRODUCER ===")
	fmt.Println("This test tries to reproduce the exact scenario where:")
	fmt.Println("1. Thread A: OpenNotebox finds box in cache")
	fmt.Println("2. Thread B: Purges the box from cache")
	fmt.Println("3. Thread A: Calls uReopenNotebox on non-existent box")
	fmt.Println()

	const attempts = 10000

	var (
		reproduced int64
		panics     int64
	)

	for attempt := 0; attempt < attempts; attempt++ {
		storage := FileStorageObject(fmt.Sprintf("reproducer_%d", attempt))
		endpoint := "reproducer"

		// Create and open to get in cache
		err := CreateNotebox(ctx, endpoint, storage)
		if err != nil {
			continue
		}

		box, err := OpenNotebox(ctx, endpoint, storage)
		if err != nil {
			continue
		}
		box.Close(ctx)

		// Now set up the race
		var wg sync.WaitGroup
		wg.Add(2)

		raceDetected := false

		// Thread A: Try to open (will check existence then reopen)
		go func() {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					atomic.AddInt64(&panics, 1)
					raceDetected = true
					fmt.Printf("✓ Attempt %d: Reproduced panic: %v\n", attempt, r)
				}
			}()

			// This is the critical operation
			box, err := OpenNotebox(ctx, endpoint, storage)
			if err != nil {
				return
			}

			// Check if we got nil
			if box.Openfile() == nil {
				atomic.AddInt64(&reproduced, 1)
				raceDetected = true
				fmt.Printf("✓ Attempt %d: Reproduced nil openfile!\n", attempt)
			}

			box.Close(ctx)
		}()

		// Thread B: Purge at just the right moment
		go func() {
			defer wg.Done()

			// Try to purge the cache entry
			boxLock.Lock()
			openboxes.Delete(storage)
			boxLock.Unlock()
		}()

		wg.Wait()

		if raceDetected {
			fmt.Printf("🎯 Successfully reproduced the race condition!\n")
		}

		// Progress
		if attempt > 0 && attempt%1000 == 0 {
			fmt.Printf("Progress: %d/%d attempts, %d reproductions\n",
				attempt, attempts, atomic.LoadInt64(&reproduced)+atomic.LoadInt64(&panics))
		}
	}

	fmt.Printf("\n=== REPRODUCER RESULTS ===\n")
	fmt.Printf("Attempts:      %d\n", attempts)
	fmt.Printf("Nil openfile:  %d\n", reproduced)
	fmt.Printf("Panics:        %d\n", panics)
	fmt.Printf("Success rate:  %.2f%%\n", float64(reproduced+panics)/float64(attempts)*100)

	if reproduced > 0 || panics > 0 {
		t.Errorf("Successfully reproduced the race condition %d times!", reproduced+panics)
		t.Error("This confirms the bug exists in the current implementation")
	} else {
		t.Log("Could not reproduce in this run, but the race condition likely still exists")
	}
}
