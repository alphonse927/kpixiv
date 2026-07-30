package auth

import (
	"context"
	"fmt"
	"html"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/alphonse927/kpixiv/internal/logger"
)

const (
	defaultTimeout = 5 * time.Minute
	callbackPath   = "/callback"
	healthPath     = "/"
)

type callbackServer struct {
	srv      *http.Server
	ln       net.Listener
	port     int
	resultCh chan string
	errCh    chan error
	done     chan struct{}
	mu       sync.Mutex
	started  bool
	received bool
}

func newCallbackServer() *callbackServer {
	return &callbackServer{
		resultCh: make(chan string, 1),
		errCh:    make(chan error, 1),
		done:     make(chan struct{}),
	}
}

func (cs *callbackServer) Start(ctx context.Context) (int, error) {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	if cs.started {
		return 0, fmt.Errorf("callback server already running")
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, fmt.Errorf("failed to start callback listener: %w", err)
	}

	cs.ln = ln
	addr := ln.Addr().(*net.TCPAddr) //nolint:errcheck // listener addr is always TCP on 127.0.0.1
	cs.port = addr.Port

	mux := http.NewServeMux()
	mux.HandleFunc(callbackPath, cs.handleCallback)
	mux.HandleFunc(healthPath, cs.handleHealth)

	cs.srv = &http.Server{
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  30 * time.Second,
	}

	go func() {
		err := cs.srv.Serve(ln)
		if err != nil && err != http.ErrServerClosed {
			select {
			case cs.errCh <- err:
			default:
			}
		}
	}()

	cs.started = true

	logger.WithComponent("auth").Info("callback server started", "port", cs.port)

	go cs.watchContext(ctx)

	return cs.port, nil
}

func (cs *callbackServer) watchContext(ctx context.Context) {
	select {
	case <-ctx.Done():
		cs.mu.Lock()
		if !cs.received {
			select {
			case cs.errCh <- ctx.Err():
			default:
			}
		}
		cs.mu.Unlock()
		cs.shutdown() //nolint:contextcheck // fresh context created inside shutdown for graceful timeout
	case <-cs.done:
	}
}

func (cs *callbackServer) handleCallback(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	redirectURL := r.URL.Query().Get("url")
	if redirectURL == "" {
		cs.writeErrorPage(w, "Missing callback URL parameter.")
		return
	}

	cs.mu.Lock()
	if cs.received {
		cs.mu.Unlock()
		cs.writeErrorPage(w, "Callback already received.")
		return
	}
	cs.received = true
	cs.mu.Unlock()

	logger.WithComponent("auth").Info("callback received via scheme handler")

	select {
	case cs.resultCh <- redirectURL:
	default:
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, successPage) //nolint:errcheck // response write failure is not actionable
}

func (cs *callbackServer) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)

	cs.mu.Lock()
	received := cs.received
	cs.mu.Unlock()

	if received {
		fmt.Fprint(w, successPage) //nolint:errcheck // response write failure is not actionable
	} else {
		fmt.Fprint(w, waitingPage) //nolint:errcheck // response write failure is not actionable
	}
}

func (cs *callbackServer) writeErrorPage(w http.ResponseWriter, msg string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusBadRequest)
	fmt.Fprintf(w, errorPageHTML, html.EscapeString(msg)) //nolint:errcheck // response write failure is not actionable
}

func (cs *callbackServer) WaitForResult(ctx context.Context) (string, error) {
	select {
	case url := <-cs.resultCh:
		return url, nil
	case err := <-cs.errCh:
		return "", err
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

func (cs *callbackServer) Port() int {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	return cs.port
}

func (cs *callbackServer) shutdown() {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	if !cs.started {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := cs.srv.Shutdown(ctx); err != nil {
		logger.WithComponent("auth").Warn("callback server shutdown error", "error", err)
	}

	if cs.ln != nil {
		cs.ln.Close() //nolint:errcheck,gosec // best-effort close on listener
	}

	cs.started = false
	logger.WithComponent("auth").Info("callback server shut down")
}

const waitingPage = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<title>kPixiv – Waiting for Authentication</title>
<style>
body { font-family: sans-serif; display: flex; justify-content: center; align-items: center; height: 100vh; margin: 0; background: #2d3444; color: #dce4f5; }
.box { text-align: center; padding: 2rem; }
.spinner { border: 4px solid #4a5568; border-top: 4px solid #7c9bff; border-radius: 50%; width: 40px; height: 40px; animation: spin 1s linear infinite; margin: 1rem auto; }
@keyframes spin { 0% { transform: rotate(0deg); } 100% { transform: rotate(360deg); } }
h2 { margin-bottom: 0.5rem; }
p { color: #a0aec0; }
</style>
</head>
<body>
<div class="box">
<div class="spinner"></div>
<h2>Waiting for Pixiv Authentication</h2>
<p>Complete the login in your browser to continue.</p>
</div>
</body>
</html>`

const successPage = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<title>kPixiv – Authentication Successful</title>
<style>
body { font-family: sans-serif; display: flex; justify-content: center; align-items: center; height: 100vh; margin: 0; background: #2d3444; color: #dce4f5; }
.box { text-align: center; padding: 2rem; }
.icon { font-size: 3rem; margin-bottom: 0.5rem; color: #68d391; }
h2 { margin-bottom: 0.5rem; }
p { color: #a0aec0; }
</style>
</head>
<body>
<div class="box">
<div class="icon">&#10003;</div>
<h2>Authentication Successful</h2>
<p>You may now close this window.</p>
</div>
</body>
</html>`

const errorPageHTML = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<title>kPixiv – Authentication Failed</title>
<style>
body { font-family: sans-serif; display: flex; justify-content: center; align-items: center; height: 100vh; margin: 0; background: #2d3444; color: #dce4f5; }
.box { text-align: center; padding: 2rem; }
.icon { font-size: 3rem; margin-bottom: 0.5rem; color: #fc8181; }
h2 { margin-bottom: 0.5rem; }
p { color: #a0aec0; }
</style>
</head>
<body>
<div class="box">
<div class="icon">&#10007;</div>
<h2>Authentication Failed</h2>
<p>%s</p>
</div>
</body>
</html>`
