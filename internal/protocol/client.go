package protocol

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"time"

	"github.com/alphonse927/kpixiv/internal/storage"
	"github.com/alphonse927/kpixiv/internal/wallpaper"
)

type Client struct {
	socketPath string
}

func NewClient(socketPath string) *Client {
	if socketPath == "" {
		socketPath = DefaultSocketPath()
	}
	return &Client{socketPath: socketPath}
}

func (c *Client) Start() error {
	return c.refresh()
}

func (c *Client) NextWallpaper() error {
	return c.do(MethodNextWallpaper)
}

func (c *Client) NextWallpaperForMonitor(monitorID string) error {
	return c.doWith(MethodNextWallpaperForMonitor, map[string]string{"monitor_id": monitorID})
}

func (c *Client) NextWallpaperForAllMonitors() error {
	return c.do(MethodNextWallpaperAll)
}

func (c *Client) BookmarkCurrentArtwork() error {
	return c.do(MethodBookmarkCurrent)
}

func (c *Client) ExcludeCurrentWallpaper() error {
	return c.do(MethodExcludeCurrent)
}

func (c *Client) OpenCurrentArtwork() error {
	return c.do(MethodOpenCurrent)
}

func (c *Client) OpenCurrentArtworkInPixiv() error {
	return c.do(MethodOpenCurrentPixiv)
}

func (c *Client) CopyCurrentArtwork() error {
	return c.do(MethodCopyCurrent)
}

func (c *Client) BookmarkWallpaper(artworkID string) error {
	return c.doWith(MethodBookmark, map[string]string{"artwork_id": artworkID})
}

func (c *Client) ExcludeWallpaper(artworkID string) error {
	return c.doWith(MethodExclude, map[string]string{"artwork_id": artworkID})
}

func (c *Client) OpenWallpaperFile(artworkID string) error {
	return c.doWith(MethodOpenFile, map[string]string{"artwork_id": artworkID})
}

func (c *Client) OpenWallpaperInPixiv(artworkID string) error {
	return c.doWith(MethodOpenPixiv, map[string]string{"artwork_id": artworkID})
}

func (c *Client) CopyWallpaperToFavorites(artworkID string) error {
	return c.doWith(MethodCopyFavorites, map[string]string{"artwork_id": artworkID})
}

func (c *Client) PauseRotation() {
	if err := c.do(MethodPauseRotation); err != nil {
		return
	}
}

func (c *Client) ResumeRotation() {
	if err := c.do(MethodResumeRotation); err != nil {
		return
	}
}
func (c *Client) PixivLoggedIn() bool {
	state, err := c.state()
	return err == nil && state.PixivLoggedIn
}

func (c *Client) PixivUserName() string {
	state, err := c.state()
	if err != nil {
		return ""
	}
	return state.PixivUsername
}

func (c *Client) IsArtworkBookmarked() bool {
	var bookmarked bool
	if err := c.request(MethodIsBookmarked, nil, &bookmarked); err != nil {
		return false
	}
	return bookmarked
}

func (c *Client) MultiMonitorEnabled() bool {
	state, err := c.state()
	return err == nil && state.MultiMonitor
}

func (c *Client) Monitors() ([]wallpaper.Screen, error) {
	state, err := c.state()
	if err != nil {
		return nil, err
	}

	monitors := make([]wallpaper.Screen, 0, len(state.Monitors))
	for _, monitor := range state.Monitors {
		monitors = append(monitors, wallpaper.Screen{ID: monitor.ID, Name: monitor.Name, Model: monitor.Model})
	}

	return monitors, nil
}

func (c *Client) MonitorWallpapers() (map[string]*storage.ImageMeta, error) {
	state, err := c.state()
	if err != nil {
		return nil, err
	}

	result := make(map[string]*storage.ImageMeta, len(state.Wallpapers))
	for id, wallpaperInfo := range state.Wallpapers {
		result[id] = &storage.ImageMeta{ID: wallpaperInfo.ID, Path: wallpaperInfo.Path, Title: wallpaperInfo.Title, Artist: wallpaperInfo.Artist}
	}

	return result, nil
}

func (c *Client) LoginToPixiv() error {
	return c.do(MethodLoginPixiv)
}

func (c *Client) LogoutFromPixiv() error {
	return c.do(MethodLogoutPixiv)
}

func (c *Client) ShowSettingsWindow() error {
	return c.do(MethodShowSettings)
}

func (c *Client) ShowAccountSettings() error {
	return c.do(MethodShowAccountSettings)
}
func (c *Client) Shutdown() {
	if err := c.do(MethodShutdown); err != nil {
		return
	}
}

func (c *Client) state() (StateSnapshot, error) {
	var snapshot StateSnapshot
	if err := c.request(MethodGetState, nil, &snapshot); err != nil {
		return StateSnapshot{}, err
	}

	return snapshot, nil
}

func (c *Client) refresh() error {
	_, err := c.state()
	return err
}

func (c *Client) do(method string) error {
	return c.request(method, nil, nil)
}

func (c *Client) doWith(method string, params any) error {
	return c.request(method, params, nil)
}

func (c *Client) request(method string, params any, out any) error {
	var rawParams json.RawMessage
	if params != nil {
		b, err := json.Marshal(params)
		if err != nil {
			return err
		}
		rawParams = b
	}

	conn, err := net.DialTimeout("unix", c.socketPath, 2*time.Second)
	if err != nil {
		return fmt.Errorf("failed to connect to daemon: %w", err)
	}
	defer func() {
		if err = conn.Close(); err != nil {
			return
		}
	}()

	enc := json.NewEncoder(conn)
	dec := json.NewDecoder(conn)
	msg := Message{Type: MsgRequest, ID: fmt.Sprintf("%d", time.Now().UnixNano()), Method: method, Params: rawParams}
	if err = enc.Encode(&msg); err != nil {
		return err
	}

	var resp Message
	if err = dec.Decode(&resp); err != nil {
		return err
	}

	if resp.Error != "" {
		return errors.New(resp.Error)
	}

	if out != nil && len(resp.Result) > 0 && string(resp.Result) != "null" {
		return json.Unmarshal(resp.Result, out)
	}

	return nil
}
