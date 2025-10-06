package notelib

import (
	"context"
	"fmt"
	"os"
	"testing"
)

// TestMain runs before all tests in this package
func TestMain(m *testing.M) {
	// Silent checkpoints
	CheckpointSilently = true

	// Set errorLoggerFunc to panic immediately for all tests
	// This will cause the test to fail properly with a stack trace
	errorLoggerFunc = func(ctx context.Context, msg string, v ...interface{}) {
		errMsg := fmt.Sprintf(msg, v...)
		fmt.Printf("ERROR: %s\n", errMsg)
		panic(errMsg) // Panic immediately - tests will fail with proper reporting
	}

	// Run tests
	code := m.Run()

	// Exit with test result code
	os.Exit(code)
}

func longRunningTest(t *testing.T) {
	t.Skip()
}
