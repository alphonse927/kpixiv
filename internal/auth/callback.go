package auth

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/alphonse927/kpixiv/internal/logger"
)

const (
	defaultTimeout = 5 * time.Minute
	socketDir      = "kpixiv"
	socketFile     = "auth.sock"
	socketFallback = ".local/state/kpixiv"
)

type CallbackReceiver interface {
	Wait(ctx context.Context) (string, error)
}

type socketReceiver struct {
	path   string
	ln     net.Listener
	result chan string
}

func socketPath() (string, error) {
	runtimeDir := os.Getenv("XDG_RUNTIME_DIR")
	if runtimeDir != "" {
		return filepath.Join(runtimeDir, socketDir, socketFile), nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, socketFallback, socketFile), nil
}

func newSocketReceiver() (*socketReceiver, error) {
	sockPath, err := socketPath()
	if err != nil {
		return nil, err
	}

	dir := filepath.Dir(sockPath)
	if mErr := os.MkdirAll(dir, 0750); mErr != nil {
		return nil, fmt.Errorf("cannot create state dir: %w", mErr)
	}

	os.Remove(sockPath) //nolint:errcheck,gosec // best-effort cleanup of stale socket

	ln, err := net.Listen("unix", sockPath)
	if err != nil {
		return nil, fmt.Errorf("cannot listen on auth socket: %w", err)
	}

	sr := &socketReceiver{
		path:   sockPath,
		ln:     ln,
		result: make(chan string, 1),
	}

	go sr.acceptOnce()

	logger.WithComponent(authComponent).Debug("auth socket listening", "path", sockPath)

	return sr, nil
}

func (sr *socketReceiver) acceptOnce() {
	conn, err := sr.ln.Accept()
	if err != nil {
		select {
		case sr.result <- "":
		default:
		}
		return
	}
	defer conn.Close() //nolint:errcheck // best-effort close

	line, err := bufio.NewReader(conn).ReadString('\n')
	if err != nil {
		return
	}

	url := strings.TrimSpace(line)
	if url == "" {
		return
	}

	logger.WithComponent(authComponent).Info("callback received via socket")

	select {
	case sr.result <- url:
	default:
	}

	fmt.Fprintf(conn, "ok\n") //nolint:errcheck // failure to ack is not actionable
}

func (sr *socketReceiver) Wait(ctx context.Context) (string, error) {
	select {
	case url := <-sr.result:
		if url == "" {
			return "", errors.New("failed to receive callback")
		}
		return url, nil
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

func (sr *socketReceiver) Close() {
	sr.ln.Close()      //nolint:errcheck,gosec // best-effort close
	os.Remove(sr.path) //nolint:errcheck,gosec // best-effort cleanup
}

// SendCallback connects to the running Login flow and delivers a URL.
// Used when kpixiv is invoked with a pixiv:// URI.
func SendCallback(url string) error {
	path, err := socketPath()
	if err != nil {
		return err
	}

	conn, err := net.Dial("unix", path)
	if err != nil {
		return fmt.Errorf("no authentication in progress")
	}
	defer conn.Close() //nolint:errcheck // best-effort close

	fmt.Fprintf(conn, "%s\n", url) //nolint:errcheck // write failure caught by subsequent read

	response, err := bufio.NewReader(conn).ReadString('\n')
	if err != nil {
		return fmt.Errorf("authentication flow closed before completing")
	}
	if strings.TrimSpace(response) != "ok" {
		return fmt.Errorf("unexpected response from auth socket: %s", strings.TrimSpace(response))
	}

	return nil
}
