// 20-May-2023 Concurrency Rework
//
//   2021 M1 Macbook Pro
//     BEFORE
//       concurrency_test: notebox achieved 70 iterations
//       concurrency_test: notefile achieved 41189 iterations
//     AFTER
//       concurrency_test: notebox achieved 2060 iterations
//       concurrency_test: notefile achieved 167884 iterations
//
//   2022 M1 Macbook Air:
//     BEFORE
//       concurrency_test: notebox achieved 70 iterations
//       concurrency_test: notefile achieved 33241 iterations
//     AFTER
//       concurrency_test: notebox achieved 1801 iterations
//       concurrency_test: notefile achieved 148776 iterations

package notelib

import (
	"context"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/blues/note-go/note"
)

// Temporarily change this to something large, such as 60 or 120 seconds, if you're testing performance.
var testDurationSeconds = 5

var counterLock sync.Mutex
var startTests = false
var stopTests = false
var startedTests = 0
var activeTests = 0

func TestConcurrency(t *testing.T) {
	ctx := context.Background()

	// Set file storage directory
	FileSetStorageLocation(os.Getenv("HOME") + "/note/notefiles")
	AutoCheckpointSeconds = testDurationSeconds / 2
	AutoPurgeSeconds = testDurationSeconds / 2

	// Set checkpoint so it happens in the middle of the test

	// Start all the tests
	go parallelTest(t, "notebox", false, 10, testNotebox)
	go parallelTest(t, "notefile", false, 100, testNotefile)

	// Wait for startedTests to stabilize
	for {
		time.Sleep(1 * time.Second)
		counterLock.Lock()
		wait := activeTests < startedTests
		counterLock.Unlock()
		if !wait {
			break
		}
	}

	// Start the tests
	logDebug(ctx, "concurrency_test: %d goroutines started", activeTests)
	startTests = true

	// Let them run for a while
	time.Sleep(time.Duration(testDurationSeconds) * time.Second)

	// Stop the tests and wait for them to run down
	stopTests = true
	for {
		counterLock.Lock()
		wait := activeTests > 0
		counterLock.Unlock()
		if !wait {
			break
		}
		time.Sleep(250 * time.Millisecond)
	}
	logDebug(ctx, "concurrency_test: completed")

}

func parallelTest(t *testing.T, testName string, summarizeWork bool, workInstances int, workFn func(ctx context.Context, testid string) bool) {
	ctx := context.Background()

	// Start the tests and notify the caller that we've started them
	counterLock.Lock()
	startedTests += workInstances + 1
	activeTests++
	counterLock.Unlock()
	time.Sleep(250 * time.Millisecond)

	// Activate all the work instancse
	active := 0
	iterations := uint64(0)
	errors := uint64(0)
	for i := 0; i < workInstances; i++ {
		go parallelWork(workFn, summarizeWork, &active, &iterations, &errors)
		for active != i+1 {
			time.Sleep(10 * time.Millisecond)
		}
	}

	// Wait for as long as our tasks are still active
	for {
		counterLock.Lock()
		wait := active > 0
		counterLock.Unlock()
		if !wait {
			break
		}
		time.Sleep(1 * time.Second)
	}

	// Validate the results of our tasks
	logDebug(ctx, "concurrency_test: %s achieved %d iterations", testName, iterations)

	// Done
	counterLock.Lock()
	activeTests--
	counterLock.Unlock()

}

// Perform work in parallel
func parallelWork(workFn func(ctx context.Context, testid string) bool, summarizeWork bool, pActive *int, iterations *uint64, errors *uint64) {
	ctx := context.Background()

	// Notify the callers that we've started, and wait for 'go'
	counterLock.Lock()
	id := *pActive
	(*pActive)++
	activeTests++
	counterLock.Unlock()
	for !startTests {
		time.Sleep(10 * time.Millisecond)
	}

	// Perform the work
	localIterations := 0
	testid := fmt.Sprintf("concurrency_test_%04d", id)
	for !stopTests {
		if !workFn(ctx, testid) {
			atomic.AddUint64(errors, 1)
		} else {
			atomic.AddUint64(iterations, 1)
			localIterations++
		}
		time.Sleep(1 * time.Millisecond)
	}

	// Run down
	counterLock.Lock()
	if summarizeWork {
		logDebug(ctx, "concurrency: %s achieved %d iterations", testid, localIterations)
	}
	activeTests--
	(*pActive)--
	counterLock.Unlock()

}

// Perform a single notebox test
func testNotebox(ctx context.Context, testid string) bool {
	box, err := OpenEndpointNotebox(ctx, "", FileStorageObject(testid), true)
	if err != nil {
		logDebug(ctx, "boxOpen: %s", err)
		return false
	}

	notefiles := 10
	for nid := 0; nid < notefiles; nid++ {
		notefileID := fmt.Sprintf("notefile%d.db", nid)

		_ = box.DeleteNotefile(ctx, notefileID)
		err = box.AddNotefile(ctx, notefileID, nil)
		if err != nil {
			logDebug(ctx, "boxAddNotefile: %s", err)
			return false
		}

		for i := 0; i < 100; i++ {
			n, err := note.CreateNote([]byte("{\"hi\":\"there\"}"), nil)
			if err != nil {
				return false
			}
			err = box.AddNote(ctx, "", notefileID, fmt.Sprintf("note%d", i), n)
			if err != nil {
				fmt.Printf("boxAddNote: %s", err)
				return false
			}
		}

		openfile, nf, err := box.OpenNotefile(ctx, notefileID)
		if err != nil {
			logDebug(ctx, "boxOpenNotefile: %s", err)
			return false
		}

		for i := 0; i < 100; i++ {
			err := nf.DeleteNote(ctx, "", fmt.Sprintf("note%d", i))
			if err != nil {
				return false
			}
			n, err := note.CreateNote([]byte("{\"hi\":\"there\"}"), nil)
			if err != nil {
				return false
			}
			err = box.AddNote(ctx, "", notefileID, "", n)
			if err != nil {
				return false
			}
		}

		openfile.Close(ctx)

	}

	if false {
		for nid := 0; nid < notefiles; nid++ {
			err = box.DeleteNotefile(ctx, fmt.Sprintf("notefile%d", nid))
			if err != nil {
				logDebug(ctx, "boxAddNotefile: %s", err)
				return false
			}
		}
	}

	err = box.Close(ctx)
	if err != nil {
		logDebug(ctx, "boxClose: %s", err)
		return false
	}

	return true
}

// Perform a single notefile test
func testNotefile(ctx context.Context, testid string) bool {

	nf := CreateNotefile(false)
	for i := 0; i < 100; i++ {
		n, err := note.CreateNote([]byte("{\"hi\":\"there\"}"), nil)
		if err != nil {
			return false
		}
		_, err = nf.AddNote(ctx, "", fmt.Sprintf("note%d", i), n)
		if err != nil {
			return false
		}
	}
	for i := 0; i < 100; i++ {
		err := nf.DeleteNote(ctx, "", fmt.Sprintf("note%d", i))
		if err != nil {
			return false
		}
		n, err := note.CreateNote([]byte("{\"hi\":\"there\"}"), nil)
		if err != nil {
			return false
		}
		_, err = nf.AddNote(ctx, "", "", n)
		if err != nil {
			return false
		}
	}

	return true

}
