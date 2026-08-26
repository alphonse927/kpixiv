# Changelog

All notable changes to KPixiv are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Fixed

- Fix the formatting of the next rotation status message when paused.

## [v0.10.1] – Steady Sync

Reliability fixes for fetching, syncing, and daemon management.

### Changed

- `kpixivctl autostart enable` now also starts kPixiv immediately, not just
  on the next login (`systemctl --user enable --now`). Likewise, `...
  autostart disable` now stops it immediately rather than only preventing
  future autostart. The Settings "Run KPixiv in the background" checkbox
  follows the same behavior, and now asks for confirmation before turning
  it off, since doing so stops kPixiv right away.
- kPixiv now runs exclusively as a systemd user service. Running the
  `kpixiv` binary directly hands off to the systemd-managed instance
  (starting it if needed) and exits. A `--foreground` debug mode is
  available for manual execution without systemd integration.

### Fixed

- "Next fetch"/"Next sync" could get stuck showing "Any moment now"
  indefinitely once fetch or bookmark sync attempts started failing. The
  countdown is now computed from the last *attempt*, success or failure, and
  shows "Fetching…" / "Syncing…" live while running, with error details
  appended when the most recent attempt failed.
- Bookmark sync no longer gets stuck showing "Next sync: Any moment now"
  indefinitely. The scheduler's bookmark-sync ticker is now always running,
  and whether a tick triggers a sync is decided from the live config,
  matching how wallpaper rotation and ranking fetch already behaved.
- Logs are now centralized to `~/.local/state/kpixiv/kpixiv.log` with
  automatic rotation, and the Settings "View Logs..." viewer reads from that
  file instead of the systemd journal.
- Launching kPixiv while another instance is already running now logs a
  warning and shows a desktop notification instead of silently exiting.

## [v0.10.0] – Per-Monitor Harmony

Per-monitor control from the system tray, plus richer activity reporting.

### Added

- kPixiv now runs an initial ranking fetch and bookmark sync immediately on
  startup (and right after logging into Pixiv), instead of waiting a full
  fetch/sync interval — which could be hours — for the first attempt.
- Fetch and bookmark sync attempts are now logged at info level ("Starting
  ranking fetch", "Starting bookmark sync", and their outcomes), visible in
  `~/.local/state/kpixiv/kpixiv.log` without needing `--verbose`.
- Multi-monitor support in the system tray: when multi-monitor rotation is
  enabled, the "current wallpaper" actions are replaced with one submenu per
  monitor, letting you rotate, copy, open, bookmark, or exclude the wallpaper
  on each screen independently. The top-level Next Wallpaper action rotates
  every monitor at once. A per-artwork bookmark lookup underpins the per-screen
  bookmark state.
- `kpixivctl status` now reports the last and next ranking fetch, and (when
  bookmarks are enabled) the last and next bookmark sync.
- `kpixiv --foreground`: an advanced/debug flag that runs the app directly in
  the current terminal instead of handing off to the systemd user service.
  No auto-restart, no autostart integration -- intended for local debugging
  only.
- GUI Home page redesign: a modernized layout with live status indicators for
  the daemon and Pixiv, cached/history counts, fetch and bookmark-sync
  timings, and per-monitor wallpaper previews.

### Changed

- Persisted fetch pagination (`pagination.json`) was replaced by a unified
  `Activity` state (`activity.json`) that also records the last ranking fetch
  and last bookmark sync times, so status survives daemon restarts.
- The scheduler now delegates ranking-page tracking to the fetcher instead of
  holding it as in-memory state.

### Removed

- The tolerant "another instance is already running, just exit quietly"
  behavior as a normal, expected code path. It's replaced by the systemd
  hand-off described above; the single-instance lock is now a safety net for
  the advanced `--foreground` debug flag, not something users are expected
  to hit in normal use.

### Fixed

- "Next change" showed the misleading "Any moment now" when wallpaper
  rotation was paused and the previous countdown target had already elapsed.
  It now shows "Paused" whenever rotation is disabled, regardless of how
  much time has passed since the last rotation.
- The ranking page now resets to 1 when the daily ranking rolls over to a new
  day (JST), so the fresh ranking is crawled from the top instead of
  continuing from stale pages of the previous day.
- KScreen primary-monitor detection accepts both `priority:` and
  `priority ` output formats.

## [v0.9.1] – True Orientation

### What's Changed

- Config: replace `pixiv.landscape_only` with `wallpaper.orientation`
  (`any`, `landscape`, `portrait`).
- Status: display the active wallpaper orientation in `kpixivctl` output.
- Build: add build metadata and improve CLI functionality.
- CI: enable Go module caching, run `go test ./...`, and add explicit
  permissions to workflows.

## [v0.9.0] – Orienting the Doctor

Polish sprint focused on diagnostics, CLI ergonomics, notifications, and GUI
clarity.

### Added

- `kpixivctl doctor` — a diagnostic command that checks configuration,
  directories, cache health, Pixiv authentication and API reachability, DBus
  session, Plasma presence, wallpaper backend, and systemd autostart. Failing
  checks carry actionable `fix:` hints and produce a nonzero exit code.
- `kpixivctl version` — prints version, git commit, build date, and Go/Fyne
  versions. Commit and date are injected by the Makefile at build time.
- `kpixivctl cache stats` — reports disk usage, oldest/newest image, and queue
  size in addition to the image count. `cache clear` now also removes
  thumbnails and reports the freed space.
- `kpixivctl config reset` — restores the configuration file to default
  values.
- Desktop notifications on fetch completion, bookmark sync, bookmarking an
  artwork, and excluding a wallpaper. Notifications are now delivered with the
  correct application name and icon, and degrade to log-only output when
  `notify-send` is unavailable.
- New `notifications.enabled` configuration key (and a GUI toggle) to turn
  desktop notifications on or off.
- GUI Overview dashboard showing daemon, Pixiv, cache, history, next-change,
  and next-wallpaper state, plus first-run guidance and a warnings panel.
- GUI first-run actions ("Fetch Wallpapers" / "Set up Pixiv Login") and an
  About page with the application icon.
- `internal/notify` and `internal/human` helper packages.
- Build info collection in `internal/build` with commit, date, and dependency
  versions.

### Fixed

- A monitor orientation of `any` is no longer reported as an invalid value;
  the dashboard, doctor, and settings save all accept it.
- Wallpaper rotation no longer fires a desktop notification on every change.
- The settings preview now regenerates missing thumbnails on demand and the
  "Next wallpaper" overview row reflects per-monitor queues when multi-monitor
  rotation is enabled.
- The hidden `completion` command was removed from `kpixivctl`.
- Per-request Pixiv fetch log lines were dropped from `info` to `debug`;
  meaningful summaries remain at `info`.
- Expired Pixiv sessions are detected during token refresh, cleared, and
  surfaced with a clear re-login message.

### Changed

- `config.Validate()` returns the list of applied adjustments instead of a
  bare boolean, so the GUI can show a "Configuration adjusted" dialog and the
  CLI can print notes for clamped values.
- `config.Set(key, value)` validates values before saving; unknown keys and
  invalid values produce actionable errors.
- Cache statistics are reused for 30 seconds to avoid expensive filesystem
  rescans during status refreshes.
- A fresh installation reports "never" for the last rotation instead of a
  misleading current timestamp.

## [v0.8.0] – Smooth Operator

### What's Changed

- fix: use heredoc for GITHUB_OUTPUT in release title/notes
- refactor: enhance Makefile with modularized install targets and improved
  output
- fix: refresh GUI account page when logout originates from tray
- fix: suppress funlen lint warning in createWidgets

## [v0.7.0] – Clean Slate

This release introduces a new **kpixivctl** CLI companion for service
management, full multi-monitor wallpaper support, Pixiv bookmark
synchronization, and a modular storage backend.

### New Features

- **kpixivctl** — new CLI binary for service/autostart management, wallpaper
  control, and Pixiv account tasks
- **Multi-monitor** — independent wallpapers per display with orientation-based
  filtering
- **Bookmark sync** — synchronize your Pixiv bookmarks and auto-cleanup
  unbookmarked images
- **Pagination** — persistent scroll state across Pixiv ranking fetches
- **Desktop integration** — autostart via systemd, single-instance enforcement,
  desktop entry installation
- **Configurable log levels** — switch between info and debug output

### Improvements

- **Queue-based rotation** — wallpapers rotate from a configurable ranked queue
  of sources (daily, weekly, monthly, bookmarks, or all)
- **Storage refactor** — package split into focused files (history, metadata,
  cleanup, thumbnails, etc.)
- **Set/Filter utilities** — generic set and slice helpers for cleaner code
- **Comprehensive tests** — storage package has thorough unit test coverage

### CI & Tooling

- GitHub Actions for testing and automated releases
- golangci-lint integration
- Release workflow produces stripped binaries

### Notes

- This release rewrites history with correct author attribution. Tags have been
  re-mapped accordingly.

## [v0.6.0] – Every Monitor

This release adds full multi-monitor support, per-screen wallpaper rotation,
and deeper KDE integration. KPixiv now manages independent wallpaper queues
for each connected display.

- Multi-Monitor Support — each screen gets its own wallpaper queue and
  independent rotation
- Per-Screen Wallpaper Setting — set next/previous wallpapers on individual
  monitors via CLI and tray
- Primary Screen Detection — lock screen wallpapers now target the correct
  display
- Orientation Filtering — restrict wallpapers to landscape or portrait per
  monitor
- Robust Monitor Detection — improved screen output parsing with fallback for
  headless or unusual setups
- Queue-Based Rotation — migrated from single global queue to per-monitor
  queues with rebuild and cleanup

## [v0.5.0] – Bookmark This

This release introduces Pixiv bookmark management, a fully redesigned settings
window, and deeper system integration. KPixiv now acts as a full Pixiv
companion — syncing your bookmarks into a dedicated favorites collection,
showing live status at a glance, and letting you control everything from a
modern tabbed GUI.

- Browser-based Pixiv Login — OAuth PKCE flow opens your browser; paste the
  callback URL to authenticate directly from the settings window
- Pixiv Bookmark Sync — periodic background sync of your Pixiv bookmarks;
  kpixiv bookmarks sync/list/add/add-current CLI commands; auto-cleanup
  removes unbookmarked images
- Redesigned Settings Window — tabbed navigation: Home (live status with
  thumbnail preview, cached count, rotation timers), Settings, Account, About
- In-App Log Viewer — tail the kpixiv.service systemd journal directly from
  the GUI
- Systemd Autostart Toggle — enable/disable the user service from the Settings
  page
- Thumbnail Generation — auto-generated 140px JPEG thumbnails for the Home
  page preview
- New config section — bookmarks.enabled, bookmarks.sync_interval,
  bookmarks.auto_cleanup
- Favorites directory moved — now stored under ~/.local/share/kpixiv/Favorites/
  separated from ranking downloads

## [v0.4.0] – Connected Canvas

This release brings Pixiv account integration, a GUI settings window, and a
fully redesigned tray experience. KPixiv is no longer just a wallpaper
rotator — it is now a proper Pixiv client living in your system tray.

- Pixiv OAuth Login — browser-based PKCE authentication with automatic token
  refresh
- Redesigned Tray Menu — bookmark artwork, copy to favorites, exclude
  wallpapers, open in Pixiv, and more
- GUI Settings Window — edit ranking, intervals, dimensions, lock screen, and
  storage from the tray
- Enhanced kpixiv status — now shows R-18, landscape only, previous wallpaper,
  and queue count
- New config options — cleanup_days, set_lock_screen, history_limit

## [v0.3.0] – Plasma in Motion

kPixiv has now several options and even now has his own tray icon with some
options like:

- Next Wallpaper
- Rotate Wallpaper (Enable/Disable)
- Open Current Artwork
- Exclude Current Wallpaper

## [v0.2.0] – Running in Background

This release improves the daemon, adds persistent state handling, introduces a
cleaner filesystem layout, and continues polishing the KDE Plasma experience.

KPixiv keeps getting closer to becoming a proper native Pixiv wallpaper daemon
for KDE.

## [v0.1.0]  – Foundation

It's alive! 🎉

First release. It fetches wallpapers, resolves images, downloads them, and sets
them on your KDE desktop. Works on my machine. Probably works on yours too.

Thanks for testing. Let me know what breaks.
