# KPixiv

A Linux wallpaper application for fetching and rotating Pixiv wallpapers on KDE Plasma, supervised by systemd user services.

## Features

- Fetch wallpapers from Daily, Weekly, and Monthly rankings
- Resolve original resolution images from Pixiv thumbnails
- Download and store wallpapers locally with deduplication
- Apply wallpapers directly to the KDE Plasma desktop
- **Per-monitor wallpaper assignment** on multi-monitor setups — each screen gets its own wallpaper
- **Orientation-aware rotation** — per-monitor filter (landscape/portrait/any) with queue fallback
- **EDID-based monitor model detection** — displays connector name and model (e.g. `DP-1 (LG ULTRAGEAR)`)
- **Automatic queue recreation** — per-monitor wallpaper queues are rebuilt when monitor settings change
- Tray-centric runtime with automatic wallpaper rotation
- Pixiv OAuth login with automatic token refresh (browser-based PKCE flow)
- Bookmark and exclude wallpapers from the tray
- GUI settings window with tabbed navigation (Home, Monitors, Settings, Account, About)
- Thumbnail generation for the settings Home page
- In-app log viewer (systemd journal)
- Systemd service autostart toggle from settings
- Pixiv bookmark sync — periodic background sync of bookmarked images
- CLI management commands for scripting and automation
- Dry-run mode for testing

## Requirements

- Go 1.26+
- KDE Plasma desktop environment
- qdbus (for KDE integration)

## Install

```bash
make install
systemctl --user enable --now kpixiv.service
```

This builds both binaries, installs the systemd user service, and reloads systemd.

## Binaries

| Binary        | Description                        | Links Fyne |
|---------------|------------------------------------|-----------:|
| `kpixiv`      | Desktop application (tray + GUI)   |        Yes |
| `kpixivctl`   | Headless CLI for scripting         |         No |

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
  multi_monitor_enabled: true
  monitors:
    "DP-1":
      rotation_enabled: true
      orientation: "landscape"
    "DP-2":
      rotation_enabled: true
      orientation: "portrait"

bookmarks:
  enabled: false
  sync_interval: 60
  auto_cleanup: true

kde:
  set_lock_screen: false
```

### Options

| Option                                       | Default             | Description                                        |
|----------------------------------------------|---------------------|----------------------------------------------------|
| `download_path`                              | `~/Pictures/KPixiv` | Where to store wallpapers                          |
| `pixiv.ranking`                              | `daily`             | Ranking type (`daily`/`weekly`/`monthly`)          |
| `pixiv.r18`                                  | `false`             | Include R-18 content                               |
| `pixiv.min_width`                            | `1280`              | Minimum image width                                |
| `pixiv.min_height`                           | `720`               | Minimum image height                               |
| `pixiv.landscape_only`                       | `true`              | Only download landscape images                     |
| `wallpaper.mode`                             | `fill`              | Scaling mode (`fill`/`cover`/`fit`)                |
| `wallpaper.multi_monitor_enabled`            | `false`             | Enable per-monitor wallpaper assignment            |
| `wallpaper.monitors.<conn>.rotation_enabled` | `true`             | Enable rotation for this monitor                   |
| `wallpaper.monitors.<conn>.orientation`      | `any`              | Orientation filter: `any`, `landscape`, `portrait` |
| `wallpaper.history_limit`                    | `10`                | Max wallpapers to keep in rotation history         |
| `wallpaper.set_interval`                     | `5`                 | Minutes between wallpaper changes                  |
| `wallpaper.fetch_interval`                   | `30`                | Minutes between Pixiv fetch cycles                 |
| `wallpaper.cleanup_days`                     | `7`                 | Remove cached wallpapers older than N days         |
| `bookmarks.enabled`                          | `false`             | Enable periodic bookmark sync                      |
| `bookmarks.sync_interval`                    | `60`                | Minutes between bookmark sync cycles               |
| `bookmarks.auto_cleanup`                     | `true`              | Remove unbookmarked images from favorites          |
| `kde.set_lock_screen`                        | `false`             | Also apply wallpaper to the KDE lock screen        |

## Pixiv Login

KPixiv uses Pixiv's OAuth PKCE flow for authenticated API access.

**Desktop:** From the tray menu, select **Login to Pixiv** (or use the Account page in Settings).

**CLI:**
```bash
kpixivctl account login
```

This prints an authorization URL, opens your browser, and prompts for the callback.

Once logged in, tokens are persisted and automatically refreshed. Use `kpixivctl account status` to check login state and `kpixivctl account logout` to clear the session.

## Runtime Architecture

KPixiv runs as a single desktop process:

```
systemd user service -> kpixiv -> tray + scheduler + GUI
```

A separate CLI binary (`kpixivctl`) provides headless access to the same data for scripting without requiring Fyne or a display.

See `docs/cli-desktop-split.md` and `docs/architecture.md` for details.

## Usage

### Desktop application

```bash
kpixiv                          # Launch tray + scheduler + GUI
  --reset                       # Clear all cached images before starting
```

The system tray provides:

```
Next Wallpaper              Immediately switch to the next queued wallpaper
Rotate Wallpaper            Toggle automatic rotation on/off
---
Login to Pixiv / <user>    Authenticate or show connected user
Bookmark Current Artwork    Bookmark on Pixiv
Logout from Pixiv           Clear saved session
---
Copy to Favorites           Copy wallpaper to favorites directory
Open Current Artwork        Open the image file
Open Artwork in Pixiv       Open artwork page in browser
Exclude Current Wallpaper   Blacklist and switch away
---
Settings                    Open configuration window
Quit                        Stop KPixiv
```

### CLI

```bash
kpixivctl wallpaper fetch               # Download wallpapers from Pixiv
kpixivctl wallpaper next                 # Apply the next wallpaper in queue
  --monitor DP-1                         # Apply on a specific monitor
  --all                                  # Apply on all monitors
kpixivctl wallpaper queue ranking        # Rebuild queue from ranking images
kpixivctl wallpaper queue bookmarks      # Rebuild queue from bookmarks
kpixivctl wallpaper queue all            # Rebuild queue from both sources

kpixivctl bookmarks sync                 # Sync bookmarked images from Pixiv
kpixivctl bookmarks list                 # List local bookmarked images
kpixivctl bookmarks add <illust_id>      # Bookmark an artwork on Pixiv
kpixivctl bookmarks add-current          # Bookmark the current wallpaper
  --monitor DP-1
  --all

kpixivctl account login                  # Log in to Pixiv
kpixivctl account logout                 # Log out from Pixiv
kpixivctl account status                 # Show login status

kpixivctl cache stats                    # Show downloaded image count
kpixivctl cache clear                    # Remove all cached images

kpixivctl config show                    # Print configuration
kpixivctl config set <key> <value>       # Set a config value

kpixivctl monitors                       # List active KDE Plasma screens
kpixivctl status                         # Show full application status
```

### Global flags

| Flag             | Description                                                    |
|------------------|----------------------------------------------------------------|
| `-c, --config`   | Path to config file (default: `~/.config/kpixiv/config.yaml`)  |
| `-v, --verbose`  | Enable verbose logging                                         |
| `--dry-run`      | Show actions without downloading or applying                   |

### Settings Window

The tray **Settings** option opens a GUI dialog with tabbed navigation:

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

The **View Logs** option (accessible via the Settings window) opens a scrollable window that tails the `kpixiv.service` systemd journal in real time.

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
kpixivctl bookmarks sync
```
