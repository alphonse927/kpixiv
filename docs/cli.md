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

### `kpixivctl version`

Show version and build information:

```text
kpixivctl 0.9.0
Commit:        7ded4bb
Build date:    2026-07-31T08:00:00Z
Go version:    go1.26.4
```

Build metadata (commit, date) is injected at build time by the Makefile.
Fyne version is shown when the build links it (GUI daemon builds).

### `kpixivctl wallpaper fetch`

Download wallpapers from the configured Pixiv ranking feed. With `--dry-run`,
fetches the feed without downloading anything:

```text
Fetched from Daily ranking.
Total: 36, Downloaded: 6, Filtered: 12, Skipped: 18, Failed: 0
```

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

| Subcommand | Description                                        |
|------------|----------------------------------------------------|
| `stats`    | Show cache statistics (count, disk usage, oldest/newest, queue size) |
| `clear`    | Remove all cached images and report freed space    |

Example `cache stats` output:

```text
Downloaded images: 124
Disk usage:        482 MB
Oldest image:      2026-07-20 09:12:03
Newest image:      2026-07-30 21:40:11
In queue:          18
```

### `kpixivctl config`

| Subcommand          | Description                                 |
|---------------------|---------------------------------------------|
| `show`              | Print the current configuration             |
| `set <key> <value>` | Set a configuration value                   |
| `reset`             | Restore the configuration to default values |

`config set` validates the value, clamps below-minimum settings, and prints a
note for every adjustment:

```text
Set wallpaper.set_interval = 3
Note: Wallpaper change interval is below 5 minutes; using 5.
```

Supported config keys for `set`:

| Key                               | Type | Description                               |
|-----------------------------------|------|-------------------------------------------|
| `log_level`                       | enum | `debug`, `info`, `warn`, or `error`       |
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
| `notifications.enabled`           | bool | Enable desktop notifications              |

### `kpixivctl monitors`

List active KDE Plasma screens with connector names, model names, and rotation status.

```bash
[0] DP-1 (LG ULTRAGEAR/DP-1)	rotation=enabled	orientation=any
[1] DP-2 (DELL S2721QS/DP-2)	rotation=disabled	orientation=any
```

### `kpixivctl status`

Show a dashboard of the current state: configuration, wallpaper settings,
daemon status, cache statistics, and per-monitor info.

```text
KPixiv Status
─────────────

Configuration
──────────────────────
  Version:               0.9.0
  Config file:           ~/.config/kpixiv/config.yaml
  Download dir:          ~/Pictures/KPixiv
  ...

Current State
─────────────
  Daemon:                running
  Pixiv:                 connected (expires in 12 hours)
  Current wallpaper:     ~/Pictures/KPixiv/rank/1234.jpg
  ...
```

### `kpixivctl doctor`

Run a series of diagnostic checks over the KPixiv installation: configuration,
directory permissions, cache health, Pixiv authentication and API reachability,
DBus session, Plasma presence, wallpaper backend, and systemd autostart.

```text
kPixiv Doctor
────────────────────────────────────────
✓ Configuration
    /home/user/.config/kpixiv/config.yaml
✓ Directories
    data: /home/user/.local/share/kpixiv
      state: /home/user/.local/state/kpixiv
✓ Cache
    124 cached image(s), 482 MB
! Authentication
    not logged in to Pixiv
    fix: Run 'kpixivctl account login' to enable bookmarks and sync.
...
7 passed, 2 warnings, 0 failed
```

Each failing check ends with an actionable `fix:` hint. The command exits with
a nonzero status when any check fails, which makes it suitable for scripts.

## Exit codes

| Code | Meaning                                                         |
|------|-----------------------------------------------------------------|
| `0`  | Success, or `doctor` ran with no failures                       |
| `1`  | Command failed, or `doctor` found failing checks                |
