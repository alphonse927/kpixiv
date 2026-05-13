# KPixiv

A lightweight wallpaper daemon for fetching and rotating Pixiv wallpapers on KDE Plasma.

## Features

- Fetch wallpapers from Pixiv Daily Rankings
- Automatic wallpaper rotation on a configurable interval
- Filter wallpapers by resolution and orientation
- Manual "next wallpaper" command
- Clean CLI interface

## Requirements

- Go 1.26+
- KDE Plasma desktop environment
- qdbus (for KDE Plasma integration)

## Installation

```bash
make build
make install
```

## Configuration

KPixiv uses a TOML configuration file at `~/.config/kpixiv/config.toml`.

Example configuration:

```toml
interval_minutes = 30
download_path = "~/Pictures/KPixiv"

[pixiv]
ranking = "daily"
r18 = false
min_width = 1920
min_height = 1080
landscape_only = true

[wallpaper]
mode = "fill"
keep_history = 20
```

### Configuration Options

- `interval_minutes`: How often to rotate wallpaper (default: 30)
- `download_path`: Where to store downloaded wallpapers (default: ~/Pictures/KPixiv)
- `pixiv.ranking`: Ranking type - "daily", "weekly", or "monthly"
- `pixiv.r18`: Include R18 content (default: false)
- `pixiv.min_width`: Minimum image width (default: 1920)
- `pixiv.min_height`: Minimum image height (default: 1080)
- `pixiv.landscape_only`: Only download landscape images (default: true)
- `wallpaper.mode`: Wallpaper scaling mode
- `wallpaper.keep_history`: Number of wallpapers to keep in history (default: 20)

## Usage

### Fetch wallpapers
```bash
kpixiv fetch
```

### Set next wallpaper
```bash
kpixiv next
```

### Run daemon
```bash
kpixiv daemon
```

### Check status
```bash
kpixiv status
```

### Verbose logging
```bash
kpixiv daemon --verbose
```

## Project Structure

```
kpixiv/
├── cmd/kpixiv/         # CLI entry point
├── internal/
│   ├── cache/          # Image cache management
│   ├── config/         # Configuration loading
│   ├── logger/         # Logging setup
│   ├── pixiv/          # Pixiv API client
│   ├── scheduler/      # Daemon scheduler
│   ├── storage/        # Data persistence
│   └── wallpaper/     # Wallpaper backend
├── config.example.toml # Example config
├── Makefile
└── README.md
```

## Architecture

KPixiv is designed with clean separation of concerns:

- **Config**: Loads and validates TOML configuration
- **Pixiv Client**: Fetches rankings from Pixiv API
- **Storage**: Manages downloaded images and history
- **Cache**: In-memory cache of fetched images
- **Scheduler**: Handles automatic rotation
- **Wallpaper Backend**: Abstraction for setting wallpapers (KDE, future: GNOME, etc.)

## Development

```bash
# Build
make build

# Run tests
make test

# Clean
make clean
```

## License

MIT
