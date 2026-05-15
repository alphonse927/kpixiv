# KPixiv

A lightweight CLI wallpaper daemon for fetching and rotating Pixiv wallpapers on KDE Plasma.

## Features

- Fetch wallpapers from Daily, Weekly, and Monthly rankings
- Resolve original resolution images from Pixiv thumbnails
- Download and store wallpapers locally with deduplication
- Apply wallpapers directly to KDE Plasma desktop
- Daemon mode for automatic wallpaper rotation
- Dry-run mode for testing

## Requirements

- Go 1.26+
- KDE Plasma desktop environment
- qdbus (for KDE integration)

## Install

```bash
# Build and link to ~/.local/bin/kpixiv
make install
sudo systemctl enable --now kpixiv
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

## Usage

```bash
kpixiv fetch       # Download wallpapers
kpixiv next        # Apply next wallpaper
kpixiv daemon      # Run with automatic rotation
kpixiv status      # Show cache and storage info
```
