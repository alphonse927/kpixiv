# CLI Reference

`kpixivctl` provides headless access to all KPixiv functionality.

## Global flags

| Flag            | Description                                                   |
|-----------------|---------------------------------------------------------------|
| `-c, --config`  | Path to config file (default: `~/.config/kpixiv/config.yaml`) |
| `-v, --verbose` | Enable verbose logging                                        |
| `--dry-run`     | Show actions without downloading or applying                  |
| `--version`     | Print version information                                     |

## Commands

### `kpixivctl wallpaper fetch`

Download wallpapers from the configured Pixiv ranking feed.

### `kpixivctl wallpaper next`

Apply the next wallpaper in the rotation queue.

| Flag             | Description                 |
|------------------|-----------------------------|
| `--monitor DP-1` | Apply on a specific monitor |
| `--all`          | Apply on all monitors       |

### `kpixivctl wallpaper queue`

Rebuild the rotation queue from local images.

| Subcommand  | Description                                        |
|-------------|----------------------------------------------------|
| `ranking`   | Clear queue, load images from the Ranking folder   |
| `bookmarks` | Clear queue, load images from the Bookmarks folder |
| `all`       | Clear queue, load images from both folders         |

### `kpixivctl bookmarks`

| Subcommand        | Description                             |
|-------------------|-----------------------------------------|
| `sync`            | Sync bookmarked images from Pixiv       |
| `list`            | List locally bookmarked images          |
| `add <illust_id>` | Bookmark an artwork on Pixiv            |
| `add-current`     | Bookmark the current wallpaper on Pixiv |

`add-current` flags:

| Flag             | Description                                       |
|------------------|---------------------------------------------------|
| `--monitor DP-1` | Bookmark current wallpaper for a specific screen  |
| `--all`          | Bookmark current wallpaper on every active screen |

### `kpixivctl account`

| Subcommand | Description                                        |
|------------|----------------------------------------------------|
| `login`    | Log in to Pixiv (prints URL, prompts for callback) |
| `logout`   | Log out from Pixiv                                 |
| `status`   | Show login status                                  |

### `kpixivctl cache`

| Subcommand | Description                 |
|------------|-----------------------------|
| `stats`    | Show downloaded image count |
| `clear`    | Remove all cached images    |

### `kpixivctl config`

| Subcommand          | Description                     |
|---------------------|---------------------------------|
| `show`              | Print the current configuration |
| `set <key> <value>` | Set a configuration value       |

Supported config keys for `set`:

| Key                               | Type | Description                               |
|-----------------------------------|------|-------------------------------------------|
| `pixiv.r18`                       | bool | Include R-18 content                      |
| `pixiv.ranking`                   | int  | 0=daily, 1=weekly, 2=monthly              |
| `pixiv.min_width`                 | int  | Minimum image width (min 1280)            |
| `pixiv.min_height`                | int  | Minimum image height (min 720)            |
| `pixiv.landscape_only`            | bool | Only download landscape images            |
| `wallpaper.set_interval`          | int  | Minutes between wallpaper changes (min 5) |
| `wallpaper.fetch_interval`        | int  | Minutes between fetch cycles (min 30)     |
| `wallpaper.history_limit`         | int  | Max wallpapers in history (min 1)         |
| `wallpaper.cleanup_days`          | int  | Remove cache older than N days (min 1)    |
| `wallpaper.rotation_enabled`      | bool | Enable automatic rotation                 |
| `wallpaper.fetch_enabled`         | bool | Enable automatic fetching                 |
| `wallpaper.multi_monitor_enabled` | bool | Enable per-monitor wallpaper assignment   |
| `kde.set_lock_screen`             | bool | Also apply wallpaper to the lock screen   |
| `bookmarks.enabled`               | bool | Enable periodic bookmark sync             |
| `bookmarks.sync_interval`         | int  | Minutes between sync cycles (min 60)      |
| `bookmarks.auto_cleanup`          | bool | Remove unbookmarked images                |

### `kpixivctl monitors`

List active KDE Plasma screens with connector names, model names, and rotation status.

```bash
[0] DP-1 (LG ULTRAGEAR/DP-1)	enabled
[1] DP-2 (DELL S2721QS/DP-2)	disabled
```

### `kpixivctl status`

Show full application status: config, storage, current wallpaper, monitor info, and queue size.
