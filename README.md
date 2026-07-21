# KPixiv

A tray-centric Linux wallpaper application for fetching and rotating Pixiv wallpapers on KDE Plasma, supervised by systemd user services.

## Features

- Fetch wallpapers from Daily, Weekly, and Monthly rankings
- Resolve original resolution images from Pixiv thumbnails
- Download and store wallpapers locally with deduplication
- Apply wallpapers directly to the KDE Plasma desktop
- **Per-monitor wallpaper assignment** on multi-monitor setups — each screen gets its own wallpaper
- **Orientation-aware rotation** — per-monitor filter (landscape/portrait/any) with queue fallback
- **EDID-based monitor model detection** — displays connector name and model (e.g. `DP-3 (DELL S2721DS/0)`)
- Tray-centric runtime with automatic wallpaper rotation
- Pixiv OAuth login with automatic token refresh (browser-based PKCE flow)
- Bookmark and exclude wallpapers from the tray
- GUI settings window with tabbed navigation (Home, Monitors, Settings, Account, About)
- Thumbnail generation for the settings Home page
- In-app log viewer (systemd journal)
- Systemd service autostart toggle from settings
- Pixiv bookmark sync — periodic background sync of bookmarked images
- CLI bookmark management commands
- Dry-run mode for testing

## Requirements

- Go 1.26+
- KDE Plasma desktop environment
- qdbus (for KDE integration)

## Install

```bash
# Build and link to ~/.local/bin/kpixiv
make install
systemctl --user enable --now kpixiv.service
```

This builds the binary, installs the systemd user service, and reloads systemd.

## Configuration

Config file: `~/.config/kpixiv/config.yaml`

```yaml
download_path: "~/Pictures/KPixiv"

pixiv:
  ranking: "daily"
  r18: false
  min_width: 1280
  min_height: 720
  landscape_only: true

wallpaper:
  mode: "fill"
  history_limit: 10
  set_interval: 5
  fetch_interval: 30
  cleanup_days: 7
  multi_monitor_enabled: false
  monitors:
    "0":
      rotation_enabled: true
      orientation: "any"

bookmarks:
  enabled: false
  sync_interval: 60
  auto_cleanup: true

kde:
  set_lock_screen: false
```

### Options

| Option                                    | Default             | Description                                        |
|-------------------------------------------|---------------------|----------------------------------------------------|
| `download_path`                           | `~/Pictures/KPixiv` | Where to store wallpapers                          |
| `pixiv.ranking`                           | `daily`             | Ranking type (`daily`/`weekly`/`monthly`)          |
| `pixiv.r18`                               | `false`             | Include R-18 content                               |
| `pixiv.min_width`                         | `1280`              | Minimum image width                                |
| `pixiv.min_height`                        | `720`               | Minimum image height                               |
| `pixiv.landscape_only`                    | `true`              | Only download landscape images                     |
| `wallpaper.mode`                          | `fill`              | Scaling mode (`fill`/`cover`/`fit`)                |
| `wallpaper.multi_monitor_enabled`         | `false`             | Enable per-monitor wallpaper assignment            |
| `wallpaper.monitors.<id>.rotation_enabled`| `true`              | Enable rotation for this monitor                   |
| `wallpaper.monitors.<id>.orientation`     | `any`               | Orientation filter: `any`, `landscape`, `portrait` |
| `wallpaper.history_limit`                 | `10`                | Max wallpapers to keep in rotation history         |
| `wallpaper.set_interval`                  | `5`                 | Minutes between wallpaper changes                  |
| `wallpaper.fetch_interval`                | `30`                | Minutes between Pixiv fetch cycles                 |
| `wallpaper.cleanup_days`                  | `7`                 | Remove cached wallpapers older than N days         |
| `bookmarks.enabled`                       | `false`             | Enable periodic bookmark sync                      |
| `bookmarks.sync_interval`                 | `60`                | Minutes between bookmark sync cycles               |
| `bookmarks.auto_cleanup`                  | `true`              | Remove unbookmarked images from favorites          |
| `kde.set_lock_screen`                     | `false`             | Also apply wallpaper to the KDE lock screen        |

## Pixiv Login

KPixiv uses Pixiv's OAuth PKCE flow for authenticated API access.

1. From the tray menu, select **Login to Pixiv** (or use the Account page in Settings)
2. A browser opens to Pixiv's login page — sign in with your account
3. After signing in, the browser redirects to a callback URL or an error page
4. Copy the full URL from the address bar and paste it into the dialog

Once logged in, tokens are persisted and automatically refreshed. The tray menu shows your Pixiv username when connected. Use **Logout from Pixiv** to clear the session.

## Runtime Architecture

KPixiv currently runs as one process:

`systemd user service -> kpixiv process -> tray + scheduler + fetch + wallpaper management`

- No split daemon/tray design (socket separation is planned — see `docs/tray-socket-design.md`)
- No IPC or local socket layer
- Tray lifecycle is app lifecycle (`Quit` stops the whole process)
- systemd supervises startup and restart behavior

## Usage

### Commands

```bash
kpixiv fetch                        # Download wallpapers from Pixiv rankings
kpixiv next                         # Apply the next wallpaper in queue
  --monitor 0                       # Apply next wallpaper on monitor 0 only
  --all                             # Apply next wallpaper on all monitors
kpixiv daemon                       # Launch tray-enabled runtime (used by systemd)
  --reset                           # Clear all cached images before starting
kpixiv monitors                     # List active KDE Plasma screens
kpixiv status                       # Show config, history, and storage info
kpixiv queue                        # Manage the wallpaper queue

kpixiv bookmarks sync               # Sync bookmarked images from Pixiv
kpixiv bookmarks list               # List locally bookmarked images
kpixiv bookmarks add <illust_id>    # Bookmark an artwork on Pixiv
kpixiv bookmarks add-current        # Bookmark the current wallpaper on Pixiv
  --monitor 1                       # Bookmark monitor 1's current wallpaper
  --all                             # Bookmark current wallpapers on all monitors
```

### Global flags

| Flag             | Description                                                   |
|------------------|---------------------------------------------------------------|
| `-c, --config`   | Path to config file (default: `~/.config/kpixiv/config.yaml`) |
| `-v, --verbose`  | Enable verbose logging                                        |
| `--dry-run`      | Show actions without downloading or applying                  |

### Tray Menu

When running `kpixiv daemon`, the tray menu provides:

```
Next Wallpaper              Immediately switch to the next queued wallpaper
Rotate Wallpaper            Toggle automatic rotation on/off
---
Login to Pixiv / <user>    Authenticate with Pixiv or show connected user
Bookmark Current Artwork    Bookmark the current artwork on Pixiv
Logout from Pixiv           Clear the saved Pixiv session
---
Copy to Favorites           Copy the current wallpaper to the favorites directory
Open Current Artwork        Open the current wallpaper file
Open Artwork in Pixiv       Open the current artwork's Pixiv page in browser
Exclude Current Wallpaper   Blacklist this wallpaper and immediately switch
---
Settings                    Open the configuration settings window
Quit                        Stop KPixiv
```

### Settings Window

The **Settings** tray option opens a GUI dialog with tabbed navigation:

- **Home** — live status: current wallpaper info with thumbnail preview, cached count, next/last rotation time. When multi-monitor is enabled, shows per-monitor wallpapers instead.
- **Monitors** — enable/disable per-monitor wallpaper mode, list detected monitors with EDID model names, toggle rotation per monitor, set orientation filter (Any / Landscape / Portrait). At least one monitor must be enabled.
- **Settings** — editing config live:
  - **Intervals** — wallpaper change and download intervals
  - **Wallpaper Source** — daily, weekly, or monthly feed
  - **Dimensions** — minimum width/height filters
  - **Storage** — download directory, history limit, cleanup age
  - **Lock screen** — toggle KDE lock screen wallpaper
  - **Bookmarks** — enable bookmark sync, sync interval, auto-cleanup
- **Account** — Pixiv login/logout with URL entry for the callback
- **About** — application info

Changes are saved to the config file on applying. The **Autostart** toggle on the Settings page enables/disables the systemd user service for an automatic startup on login.

### Log Viewer

The **View Logs** option (accessible from the system tray via the Settings window) opens a scrollable window that tails the `kpixiv.service` systemd journal in real time.

## Favorites

The **Copy to Favorites** tray option copies the current wallpaper to:

```
~/.local/share/kpixiv/Favorites/
```

This provides a simple way to keep wallpapers you like without them being cleaned up by the automatic rotation.

## Bookmark Sync

When enabled in config (`bookmarks.enabled: true`), KPixiv periodically syncs your Pixiv bookmarks into the Favorites directory. Synced images are tagged as `favorites` source in metadata and are excluded from automatic cleanup.

On the first sync, all bookmarked pages are fetched. Subsequent syncs only check the first page for new bookmarks. When `auto_cleanup` is enabled, images that are no longer bookmarked are removed.

Manual sync can be triggered at any time via:

```bash
kpixiv bookmarks sync
```
