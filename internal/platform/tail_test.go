package platform

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestTailFileStreamsExistingAndAppendedLines(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.log")

	if err := os.WriteFile(path, []byte("line1\nline2\n"), 0600); err != nil {
		t.Fatalf("WriteFile() returned error: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	lines, err := TailFile(ctx, path)
	if err != nil {
		t.Fatalf("TailFile() returned error: %v", err)
	}

	want := []string{"line1", "line2"}
	for _, w := range want {
		select {
		case got := <-lines:
			if got != w {
				t.Fatalf("got line %q, want %q", got, w)
			}
		case <-time.After(2 * time.Second):
			t.Fatalf("timed out waiting for seeded line %q", w)
		}
	}

	// Append a new line after TailFile has already caught up to EOF, and
	// confirm it gets picked up without restarting.
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatalf("OpenFile() returned error: %v", err)
	}
	if _, err := f.WriteString("line3\n"); err != nil {
		t.Fatalf("WriteString() returned error: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("Close() returned error: %v", err)
	}

	select {
	case got := <-lines:
		if got != "line3" {
			t.Fatalf("got appended line %q, want %q", got, "line3")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for appended line")
	}

	cancel()

	select {
	case _, ok := <-lines:
		if ok {
			t.Fatal("expected channel to close after context cancellation")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for channel to close after cancellation")
	}
}

func TestTailFileHandlesPartialLineAtEOF(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.log")

	// Write a line without a trailing newline, simulating a write that is
	// still in progress.
	if err := os.WriteFile(path, []byte("partial"), 0600); err != nil {
		t.Fatalf("WriteFile() returned error: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	lines, err := TailFile(ctx, path)
	if err != nil {
		t.Fatalf("TailFile() returned error: %v", err)
	}

	// Nothing should be emitted yet: the line has no terminating newline.
	select {
	case got := <-lines:
		t.Fatalf("expected no line yet, got %q", got)
	case <-time.After(200 * time.Millisecond):
	}

	// Complete the line; the accumulated content should now be emitted as
	// a single, correct line rather than a garbled fragment.
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatalf("OpenFile() returned error: %v", err)
	}
	if _, err := f.WriteString(" line\n"); err != nil {
		t.Fatalf("WriteString() returned error: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("Close() returned error: %v", err)
	}

	select {
	case got := <-lines:
		if got != "partial line" {
			t.Fatalf("got %q, want %q", got, "partial line")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for the completed line")
	}
}

func TestTailFileSeedsFromTailOfLargeFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.log")

	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("Create() returned error: %v", err)
	}
	const totalLines = 20000 // comfortably larger than defaultTailSeedBytes
	for i := 0; i < totalLines; i++ {
		if _, err := fmt.Fprintf(f, "line-%06d\n", i); err != nil {
			t.Fatalf("Fprintf() returned error: %v", err)
		}
	}
	if err := f.Close(); err != nil {
		t.Fatalf("Close() returned error: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	lines, err := TailFile(ctx, path)
	if err != nil {
		t.Fatalf("TailFile() returned error: %v", err)
	}

	var got []string
	timeout := time.After(700 * time.Millisecond)
collect:
	for {
		select {
		case l := <-lines:
			got = append(got, l)
		case <-timeout:
			break collect
		}
	}

	if len(got) == 0 {
		t.Fatal("expected some seeded lines from the tail of the file, got none")
	}
	if len(got) >= totalLines {
		t.Fatalf("expected TailFile to seed only the tail of a large file, got all %d lines", len(got))
	}
	last := got[len(got)-1]
	if !strings.HasPrefix(last, "line-") || !strings.HasSuffix(last, fmt.Sprintf("%06d", totalLines-1)) {
		t.Fatalf("expected the last seeded line to be the file's last line, got %q", last)
	}
}
