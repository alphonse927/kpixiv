package protocol

import (
	"encoding/json"
)

type HandlerFunc func(json.RawMessage, *Message)

type MsgType string

const (
	MsgRequest  MsgType = "request"
	MsgResponse MsgType = "response"
	MsgEvent    MsgType = "event"
)

const (
	MethodGetState                = "get_state"
	MethodNextWallpaper           = "next_wallpaper"
	MethodNextWallpaperForMonitor = "next_wallpaper_for_monitor"
	MethodNextWallpaperAll        = "next_wallpaper_all"
	MethodBookmarkCurrent         = "bookmark_current"
	MethodExcludeCurrent          = "exclude_current"
	MethodOpenCurrent             = "open_current"
	MethodOpenCurrentPixiv        = "open_current_pixiv"
	MethodCopyCurrent             = "copy_current"
	MethodBookmark                = "bookmark"
	MethodExclude                 = "exclude"
	MethodOpenFile                = "open_file"
	MethodOpenPixiv               = "open_pixiv"
	MethodCopyFavorites           = "copy_favorites"
	MethodPauseRotation           = "pause_rotation"
	MethodResumeRotation          = "resume_rotation"
	MethodRotateToggle            = "rotate_toggle"
	MethodLoginPixiv              = "login_pixiv"
	MethodLogoutPixiv             = "logout_pixiv"
	MethodShowSettings            = "show_settings"
	MethodShowAccountSettings     = "show_account_settings"
	MethodIsBookmarked            = "is_bookmarked"
	MethodShutdown                = "shutdown"
)

type Message struct {
	Type   MsgType         `json:"type"`
	ID     string          `json:"id,omitempty"`
	Method string          `json:"method,omitempty"`
	Params json.RawMessage `json:"params,omitempty"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  string          `json:"error,omitempty"`
}

type MonitorInfo struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Model string `json:"model"`
}

type WallpaperInfo struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Artist string `json:"artist"`
	Path   string `json:"path"`
}

type StateSnapshot struct {
	Monitors        []MonitorInfo            `json:"monitors"`
	Wallpapers      map[string]WallpaperInfo `json:"wallpapers"`
	PixivLoggedIn   bool                     `json:"pixiv_logged_in"`
	PixivUsername   string                   `json:"pixiv_username"`
	RotationEnabled bool                     `json:"rotation_enabled"`
	MultiMonitor    bool                     `json:"multi_monitor"`
}
