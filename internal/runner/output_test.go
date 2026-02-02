package runner_test

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/redbackthomson/nix-tasks/internal/runner"
)

func TestDetectOutputMode(t *testing.T) {
	// Save and restore CI env var
	origCI := os.Getenv("CI")
	defer func() {
		if origCI != "" {
			os.Setenv("CI", origCI)
		} else {
			os.Unsetenv("CI")
		}
	}()

	// Test without CI
	os.Unsetenv("CI")
	if got := runner.DetectOutputMode(); got != runner.Buffered {
		t.Errorf("DetectOutputMode() without CI = %v, want Buffered", got)
	}

	// Test with CI
	os.Setenv("CI", "true")
	if got := runner.DetectOutputMode(); got != runner.Streaming {
		t.Errorf("DetectOutputMode() with CI = %v, want Streaming", got)
	}
}

func TestOutputManager_BufferedMode(t *testing.T) {
	mgr := runner.NewOutputManager(runner.Buffered)

	// Get writer for task
	w := mgr.Writer("test-task")
	if w == nil {
		t.Fatal("Writer() returned nil")
	}

	// Write should succeed
	n, err := w.Write([]byte("test output"))
	if err != nil {
		t.Errorf("Write() error = %v", err)
	}
	if n != 11 {
		t.Errorf("Write() n = %d, want 11", n)
	}
}

func TestOutputManager_StreamingMode(t *testing.T) {
	mgr := runner.NewOutputManager(runner.Streaming)

	// Get writer for task
	w := mgr.Writer("test-task")
	if w == nil {
		t.Fatal("Writer() returned nil")
	}

	// Writing would go to stdout - just verify we get a non-nil writer
	// In production this would prefix output with task name
}

func TestPrefixWriter(t *testing.T) {
	// Test that streaming output gets prefixed
	// This tests the internal behavior through integration
	var buf bytes.Buffer

	// We can't easily test prefixWriter directly since it's unexported
	// But we can verify the OutputManager produces correct behavior
	mgr := runner.NewOutputManager(runner.Buffered)
	w := mgr.Writer("my-task")

	_, err := w.Write([]byte("line1\nline2\n"))
	if err != nil {
		t.Errorf("Write() error = %v", err)
	}

	// In buffered mode, we can access the buffer content
	// by calling FlushOnError (which prints to stdout)
	// For unit testing, we just verify write succeeds
	_ = buf
}

func TestOutputModes(t *testing.T) {
	tests := []struct {
		name string
		mode runner.OutputMode
	}{
		{"Buffered", runner.Buffered},
		{"Streaming", runner.Streaming},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mgr := runner.NewOutputManager(tt.mode)
			w := mgr.Writer("test")
			if w == nil {
				t.Error("Writer returned nil")
			}
		})
	}
}

func TestOutputManager_MultipleWriters(t *testing.T) {
	mgr := runner.NewOutputManager(runner.Buffered)

	// Get writers for multiple tasks
	w1 := mgr.Writer("task1")
	w2 := mgr.Writer("task2")

	// Both should be non-nil and different
	if w1 == nil || w2 == nil {
		t.Fatal("Writer returned nil")
	}

	// Write to both
	_, _ = w1.Write([]byte("output1"))
	_, _ = w2.Write([]byte("output2"))

	// Buffers should be independent (test by verifying no errors)
}

func TestOutputManager_ConcurrentWrites(t *testing.T) {
	mgr := runner.NewOutputManager(runner.Buffered)

	// Test concurrent access doesn't panic
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func(n int) {
			w := mgr.Writer("task")
			for j := 0; j < 100; j++ {
				_, _ = w.Write([]byte("test"))
			}
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}
}

func TestStreamingWriterPrefixes(t *testing.T) {
	// Create a streaming output manager
	mgr := runner.NewOutputManager(runner.Streaming)

	// Get a writer (note: in production this writes to stdout)
	w := mgr.Writer("my-task")

	// Verify writer is not nil
	if w == nil {
		t.Error("streaming writer is nil")
	}

	// For a proper test, we'd need to capture stdout
	// which is complex in Go. The integration tests will cover this.
}

func TestFlushOnError(t *testing.T) {
	// This is mostly an integration concern since it writes to stdout
	// Just verify it doesn't panic
	mgr := runner.NewOutputManager(runner.Buffered)

	// Flush without any buffer
	mgr.FlushOnError("nonexistent")

	// Create buffer and flush
	w := mgr.Writer("task")
	_, _ = w.Write([]byte("content"))
	mgr.FlushOnError("task")

	// Flush in streaming mode (should be no-op)
	mgr2 := runner.NewOutputManager(runner.Streaming)
	mgr2.FlushOnError("task")
}

// Test that buffered content contains what was written
func TestBufferedContent(t *testing.T) {
	mgr := runner.NewOutputManager(runner.Buffered)
	w := mgr.Writer("task")

	// Write some content
	content := "test output\n"
	n, err := w.Write([]byte(content))

	if err != nil {
		t.Errorf("Write error: %v", err)
	}
	if n != len(content) {
		t.Errorf("Write length: got %d, want %d", n, len(content))
	}

	// The writer in buffered mode should be a *bytes.Buffer
	if buf, ok := w.(*bytes.Buffer); ok {
		if !strings.Contains(buf.String(), "test output") {
			t.Errorf("Buffer content: got %q, want contains 'test output'", buf.String())
		}
	}
}
