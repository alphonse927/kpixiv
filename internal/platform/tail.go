package platform

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"
	"time"
)

// pollInterval controls how often TailFile checks for newly appended data
// once it has caught up to the end of the file.
const pollInterval = 300 * time.Millisecond

func pollDelay() <-chan time.Time {
	return time.After(pollInterval)
}

// defaultTailSeedBytes bounds how much of the existing file is read on open,
// so viewing a large log doesn't require loading it all into memory. It's
// roughly enough for several hundred typical log lines.
const defaultTailSeedBytes = 256 * 1024

// TailFile streams the recent tail of a file and then follows it for new
// lines, similar in spirit to `tail -f`. It seeds the returned channel with
// up to defaultTailSeedBytes of existing content (dropping a possibly
// truncated first line), then polls for appended content until ctx is
// canceled, at which point the channel is closed.
//
// Unlike ReadJournal, this works no matter how the process was started
// (the systemd user service, or a manual --foreground run for debugging),
// because it reads the file the
// application itself writes to rather than relying on the systemd journal.
func TailFile(ctx context.Context, path string) (<-chan string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("cannot open log file: %w", err)
	}

	info, err := f.Stat()
	if err != nil {
		f.Close() //nolint:errcheck,gosec // best-effort cleanup on error path
		return nil, fmt.Errorf("cannot stat log file: %w", err)
	}

	offset := info.Size()
	if seekBack := offset - defaultTailSeedBytes; seekBack > 0 {
		offset = seekBack
	} else {
		offset = 0
	}
	if _, err := f.Seek(offset, 0); err != nil {
		f.Close() //nolint:errcheck,gosec // best-effort cleanup on error path
		return nil, fmt.Errorf("cannot seek log file: %w", err)
	}

	lines := make(chan string)

	go func() {
		defer close(lines)
		defer f.Close() //nolint:errcheck // best-effort cleanup

		reader := bufio.NewReader(f)
		skipPartial := offset > 0 // first complete line after a seek may be truncated
		var pending strings.Builder

		for {
			select {
			case <-ctx.Done():
				return
			default:
			}

			chunk, err := reader.ReadString('\n')
			if len(chunk) > 0 {
				pending.WriteString(chunk)
			}

			if err == nil {
				// pending now ends in '\n': a complete line was read.
				full := pending.String()
				pending.Reset()
				if skipPartial {
					skipPartial = false
				} else {
					select {
					case lines <- trimNewline(full):
					case <-ctx.Done():
						return
					}
				}
				continue
			}

			// EOF (or a transient read error): whatever is in pending is an
			// incomplete line still being written. Leave it buffered and
			// wait for more data to be appended rather than emitting or
			// discarding it.
			select {
			case <-ctx.Done():
				return
			case <-pollDelay():
			}
		}
	}()

	return lines, nil
}

func trimNewline(s string) string {
	if n := len(s); n > 0 && s[n-1] == '\n' {
		s = s[:n-1]
	}
	if n := len(s); n > 0 && s[n-1] == '\r' {
		s = s[:n-1]
	}
	return s
}
