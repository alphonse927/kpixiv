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

func readLine(t *testing.T, lines <-chan string, label string) string {
	t.Helper()
	select {
	case got := <-lines:
		return got
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for %s", label)
		return ""
	}
}

func expectChannelClosed(t *testing.T, lines <-chan string, label string) {
	t.Helper()
	select {
	case _, ok := <-lines:
		if ok {
			t.Fatalf("expected channel to close %s, got a value", label)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for channel to close %s", label)
	}
}

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
		if got := readLine(t, lines, fmt.Sprintf("seeded line %q", w)); got != w {
			t.Fatalf("got line %q, want %q", got, w)
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

	if got := readLine(t, lines, "appended line"); got != "line3" {
		t.Fatalf("got appended line %q, want %q", got, "line3")
	}

	cancel()
	expectChannelClosed(t, lines, "after context cancellation")
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
	const totalLines = 30000 // comfortably larger than defaultTailSeedBytes
	for i := range totalLines {
		if _, wErr := fmt.Fprintf(f, "line-%06d\n", i); wErr != nil {
			t.Fatalf("Fprintf() returned error: %v", wErr)
		}
	}
	if closeErr := f.Close(); closeErr != nil {
		t.Fatalf("Close() returned error: %v", closeErr)
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
