package protocol

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"sync"

	"github.com/alphonse927/kpixiv/internal/logger"
)

type Server struct {
	handler    Controller
	onShutdown func()
	socketPath string

	mu       sync.Mutex
	listener net.Listener
	conns    map[net.Conn]struct{}
}

func NewServer(handler Controller) *Server {
	return &Server{handler: handler, conns: make(map[net.Conn]struct{})}
}

func NewServerWithShutdown(handler Controller, onShutdown func()) *Server {
	return &Server{handler: handler, onShutdown: onShutdown, conns: make(map[net.Conn]struct{})}
}

func (s *Server) ListenAndServe(socketPath string) error {
	s.socketPath = socketPath
	if err := os.MkdirAll(filepath.Dir(socketPath), 0o700); err != nil {
		return fmt.Errorf("failed to create socket directory: %w", err)
	}

	if err := os.Remove(socketPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("failed to remove stale socket: %w", err)
	}

	ln, err := net.Listen("unix", socketPath)
	if err != nil {
		return fmt.Errorf("failed to listen on unix socket: %w", err)
	}

	return s.Serve(ln)
}

func (s *Server) Serve(listener net.Listener) error {
	s.mu.Lock()
	s.listener = listener
	s.mu.Unlock()

	for {
		conn, err := listener.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				return nil
			}
			return err
		}

		s.mu.Lock()
		s.conns[conn] = struct{}{}
		s.mu.Unlock()

		go s.handleConnection(conn)
	}
}

func (s *Server) Close() error {
	s.mu.Lock()
	listener := s.listener
	conns := make([]net.Conn, 0, len(s.conns))
	for conn := range s.conns {
		conns = append(conns, conn)
	}
	s.listener = nil
	s.conns = make(map[net.Conn]struct{})
	s.mu.Unlock()

	for _, conn := range conns {
		if err := conn.Close(); err != nil {
			logger.WithComponent("protocol").Debug("failed to close connection", "error", err)
		}
	}

	if listener != nil {
		if err := listener.Close(); err != nil {
			return err
		}
	}

	if s.socketPath != "" {
		if err := os.Remove(s.socketPath); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}

	return nil
}

func (s *Server) handleConnection(conn net.Conn) {
	defer func() {
		if err := conn.Close(); err != nil {
			logger.WithComponent("protocol").Debug("failed to close connection", "error", err)
		}
		s.mu.Lock()
		delete(s.conns, conn)
		s.mu.Unlock()
	}()

	log := logger.WithComponent("protocol")
	dec := json.NewDecoder(bufio.NewReader(conn))
	enc := json.NewEncoder(conn)

	for {
		var msg Message
		if err := dec.Decode(&msg); err != nil {
			if errors.Is(err, io.EOF) {
				return
			}
			log.Debug("closing protocol connection", "error", err)
			return
		}

		resp := s.dispatch(msg.Method, msg.Params, msg.ID)
		if err := enc.Encode(resp); err != nil {
			log.Debug("failed to send protocol response", "error", err)
			return
		}
	}
}

func (s *Server) dispatch(method string, params json.RawMessage, id string) Message {
	resp := Message{Type: MsgResponse, ID: id}
	if handler, ok := s.handlers()[method]; ok {
		handler(params, &resp)
		if resp.Error == "" && resp.Result == nil {
			resp.Result = json.RawMessage("null")
		}

		return resp
	}

	resp.Error = fmt.Sprintf("unsupported method %q", method)
	return resp
}

func (s *Server) applyAction(resp *Message, fn func() error) {
	if err := fn(); err != nil {
		resp.Error = err.Error()
	}
}

func (s *Server) applyArtworkAction(resp *Message, params json.RawMessage, fn func(string) error) {
	var payload struct {
		ArtworkID string `json:"artwork_id"`
	}

	if err := json.Unmarshal(params, &payload); err != nil {
		resp.Error = err.Error()
		return
	}

	if err := fn(payload.ArtworkID); err != nil {
		resp.Error = err.Error()
	}
}

func (s *Server) buildStateSnapshot() StateSnapshot {
	snapshot := StateSnapshot{
		Monitors:        make([]MonitorInfo, 0),
		Wallpapers:      make(map[string]WallpaperInfo),
		PixivLoggedIn:   s.handler.PixivLoggedIn(),
		PixivUsername:   s.handler.PixivUserName(),
		RotationEnabled: false,
		MultiMonitor:    false,
	}

	if cfg := s.handler.Config(); cfg != nil {
		snapshot.RotationEnabled = cfg.Wallpaper.RotationEnabled
		snapshot.MultiMonitor = cfg.Wallpaper.MultiMonitorEnabled
	}

	if monitors, err := s.handler.Monitors(); err == nil {
		for _, screen := range monitors {
			snapshot.Monitors = append(snapshot.Monitors, MonitorInfo{
				ID:    screen.ID,
				Name:  screen.Name,
				Model: screen.Model,
			})
		}
	}

	if wallpapers, err := s.handler.MonitorWallpapers(); err == nil {
		for screenID, meta := range wallpapers {
			if meta == nil {
				continue
			}
			snapshot.Wallpapers[screenID] = WallpaperInfo{
				ID:     meta.ID,
				Title:  meta.Title,
				Artist: meta.Artist,
				Path:   meta.Path,
			}
		}
	}

	return snapshot
}

func (s *Server) handlers() map[string]HandlerFunc {
	return map[string]HandlerFunc{
		MethodGetState:                s.handleGetState,
		MethodNextWallpaper:           s.handleNextWallpaper,
		MethodNextWallpaperForMonitor: s.handleNextWallpaperForMonitor,
		MethodNextWallpaperAll:        s.handleNextWallpaperAll,
		MethodBookmarkCurrent:         s.handleBookmarkCurrent,
		MethodExcludeCurrent:          s.handleExcludeCurrent,
		MethodOpenCurrent:             s.handleOpenCurrent,
		MethodOpenCurrentPixiv:        s.handleOpenCurrentPixiv,
		MethodCopyCurrent:             s.handleCopyCurrent,
		MethodBookmark:                s.handleBookmarkArtwork,
		MethodExclude:                 s.handleExcludeArtwork,
		MethodOpenFile:                s.handleOpenArtworkFile,
		MethodOpenPixiv:               s.handleOpenArtworkPixiv,
		MethodCopyFavorites:           s.handleCopyArtworkToFavorites,
		MethodPauseRotation:           s.handlePauseRotation,
		MethodResumeRotation:          s.handleResumeRotation,
		MethodRotateToggle:            s.handleRotateToggle,
		MethodLoginPixiv:              s.handleLoginPixiv,
		MethodLogoutPixiv:             s.handleLogoutPixiv,
		MethodShowSettings:            s.handleShowSettings,
		MethodShowAccountSettings:     s.handleShowAccountSettings,
		MethodIsBookmarked:            s.handleIsBookmarked,
		MethodShutdown:                s.handleShutdown,
	}
}

func (s *Server) handleGetState(_ json.RawMessage, resp *Message) {
	state := s.buildStateSnapshot()
	data, err := json.Marshal(state)
	if err != nil {
		resp.Error = err.Error()
		return
	}

	resp.Result = data
}

func (s *Server) handleNextWallpaper(_ json.RawMessage, resp *Message) {
	s.applyAction(resp, s.handler.NextWallpaper)
}

func (s *Server) handleNextWallpaperForMonitor(params json.RawMessage, resp *Message) {
	var payload struct {
		MonitorID string `json:"monitor_id"`
	}

	if err := json.Unmarshal(params, &payload); err != nil {
		resp.Error = err.Error()
		return
	}

	s.applyAction(resp, func() error { return s.handler.NextWallpaperForMonitor(payload.MonitorID) })
}

func (s *Server) handleNextWallpaperAll(_ json.RawMessage, resp *Message) {
	s.applyAction(resp, s.handler.NextWallpaperForAllMonitors)
}

func (s *Server) handleBookmarkCurrent(_ json.RawMessage, resp *Message) {
	s.applyAction(resp, s.handler.BookmarkCurrentArtwork)
}

func (s *Server) handleExcludeCurrent(_ json.RawMessage, resp *Message) {
	s.applyAction(resp, s.handler.ExcludeCurrentWallpaper)
}

func (s *Server) handleOpenCurrent(_ json.RawMessage, resp *Message) {
	s.applyAction(resp, s.handler.OpenCurrentArtwork)
}

func (s *Server) handleOpenCurrentPixiv(_ json.RawMessage, resp *Message) {
	s.applyAction(resp, s.handler.OpenCurrentArtworkInPixiv)
}

func (s *Server) handleCopyCurrent(_ json.RawMessage, resp *Message) {
	s.applyAction(resp, s.handler.CopyCurrentArtwork)
}

func (s *Server) handleBookmarkArtwork(params json.RawMessage, resp *Message) {
	s.applyArtworkAction(resp, params, s.handler.BookmarkWallpaper)
}

func (s *Server) handleExcludeArtwork(params json.RawMessage, resp *Message) {
	s.applyArtworkAction(resp, params, s.handler.ExcludeWallpaper)
}

func (s *Server) handleOpenArtworkFile(params json.RawMessage, resp *Message) {
	s.applyArtworkAction(resp, params, s.handler.OpenWallpaperFile)
}

func (s *Server) handleOpenArtworkPixiv(params json.RawMessage, resp *Message) {
	s.applyArtworkAction(resp, params, s.handler.OpenWallpaperInPixiv)
}

func (s *Server) handleCopyArtworkToFavorites(params json.RawMessage, resp *Message) {
	s.applyArtworkAction(resp, params, s.handler.CopyWallpaperToFavorites)
}

func (s *Server) handlePauseRotation(_ json.RawMessage, _ *Message) {
	s.handler.PauseRotation()
}

func (s *Server) handleResumeRotation(_ json.RawMessage, _ *Message) {
	s.handler.ResumeRotation()
}

func (s *Server) handleRotateToggle(_ json.RawMessage, _ *Message) {
	if cfg := s.handler.Config(); cfg != nil && cfg.Wallpaper.RotationEnabled {
		s.handler.PauseRotation()
		return
	}

	s.handler.ResumeRotation()
}

func (s *Server) handleLoginPixiv(_ json.RawMessage, resp *Message) {
	s.applyAction(resp, s.handler.LoginToPixiv)
}

func (s *Server) handleLogoutPixiv(_ json.RawMessage, resp *Message) {
	s.applyAction(resp, s.handler.LogoutFromPixiv)
}

func (s *Server) handleShowSettings(_ json.RawMessage, resp *Message) {
	s.applyAction(resp, s.handler.ShowSettingsWindow)
}

func (s *Server) handleShowAccountSettings(_ json.RawMessage, resp *Message) {
	s.applyAction(resp, s.handler.ShowAccountSettings)
}

func (s *Server) handleIsBookmarked(_ json.RawMessage, resp *Message) {
	resp.Result = mustJSON(s.handler.IsArtworkBookmarked())
}

func (s *Server) handleShutdown(_ json.RawMessage, _ *Message) {
	s.handler.Shutdown()
	if s.onShutdown != nil {
		go s.onShutdown()
	}
}

func mustJSON(v any) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		return json.RawMessage("null")
	}
	return b
}
