package platform

import (
	"bufio"
	"context"
	"fmt"
	"os/exec"
)

func ReadJournal(ctx context.Context, userUnit string) (<-chan string, error) {
	//nolint:gosec // unit name is controlled by the application, not user input
	cmd := exec.CommandContext(ctx, "journalctl", "--user", "-u", userUnit, "-n", "500", "-f", "-o", "cat")

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("cannot create stdout pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("cannot start journalctl: %w", err)
	}

	lines := make(chan string)

	go func() {
		defer close(lines)
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			lines <- scanner.Text()
		}
		_ = cmd.Wait() //nolint:errcheck
	}()

	return lines, nil
}
