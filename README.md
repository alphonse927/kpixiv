# KPixiv

A tray-centric Linux wallpaper application for fetching and rotating Pixiv wallpapers on KDE Plasma, supervised by systemd user services.

## Features

- Fetch wallpapers from Daily, Weekly, and Monthly rankings
- Resolve original resolution images from Pixiv thumbnails
- Download and store wallpapers locally with deduplication
- Apply wallpapers directly to KDE Plasma desktop
- Tray-centric runtime with automatic wallpaper rotation
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

This builds the binary, copies it to `/usr/local/bin/`, installs the systemd user service, and reloads systemd.

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
  keep_history: 5
  set_interval: 5
  fetch_interval: 30
```

### Options

| Option | Default | Description |
|---|---|---|
| `download_path` | `~/Pictures/KPixiv` | Where to store wallpapers |
| `pixiv.ranking` | `daily` | Ranking type (daily/weekly/monthly) |
| `pixiv.r18` | `false` | Include R18 content |
| `pixiv.min_width` | `1280` | Minimum image width |
| `pixiv.min_height` | `720` | Minimum image height |
| `pixiv.landscape_only` | `true` | Only download landscape images |
| `wallpaper.mode` | `fill` | Wallpaper scaling mode |
| `wallpaper.keep_history` | `5` | Wallpapers to keep in history |
| `wallpaper.set_interval` | `5` | Minutes between wallpaper changes |
| `wallpaper.fetch_interval` | `30` | Minutes between fetching new images |

## Runtime Architecture

KPixiv runs as one process:

`systemd user service -> kpixiv process -> tray + scheduler + fetch + wallpaper management`

- No split daemon/tray design
- No IPC or local socket layer
- Tray lifecycle is app lifecycle (`Quit` stops the whole process)
- systemd still supervises startup and restart behavior

## Usage

```bash
kpixiv fetch       # Download wallpapers
kpixiv next        # Apply next wallpaper
kpixiv daemon      # Launch tray-enabled runtime (used by systemd)
kpixiv status      # Show cache and storage info
```

### Tray Menu

When running `kpixiv daemon`, the tray menu provides:

- Next Wallpaper
- Pause Rotation
- Resume Rotation
- Open Current Artwork
- Open Folder
- Restart Rotation
- Quit
