package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/alphonse927/kpixiv/internal/config"
	"github.com/alphonse927/kpixiv/internal/pixiv"
	"github.com/alphonse927/kpixiv/internal/platform"
	"github.com/alphonse927/kpixiv/internal/storage"
	"github.com/alphonse927/kpixiv/internal/wallpaper"
)

type checkStatus int

const (
	statusOK checkStatus = iota
	statusWarn
	statusFail
)

func (s checkStatus) Mark() string {
	switch s {
	case statusOK:
		return "✓"
	case statusWarn:
		return "!"
	case statusFail:
		return "✗"
	default:
		return "?"
	}
}

type check struct {
	name   string
	result checkStatus
	detail string
	hint   string
}

// doctorCmd runs diagnostic checks against the installation without failing
// hard on configuration problems — each check is independent and reports its
// own result.
var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Run diagnostic checks on the KPixiv installation",
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		return nil // configuration is checked inside the checks
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		runDoctor()
		return nil
	},
}

func runDoctor() {
	fmt.Println("kPixiv Doctor")
	fmt.Println(strings.Repeat("─", 40))

	checks := []check{
		checkConfiguration(),
		checkDirectories(),
		checkCache(),
		checkAuthentication(),
		checkPixivAPI(),
		checkDBus(),
		checkPlasma(),
		checkWallpaperBackend(),
		checkAutostart(),
	}

	for _, c := range checks {
		fmt.Printf("%s %s\n", c.result.Mark(), c.name)
		if c.detail != "" {
			fmt.Printf("    %s\n", c.detail)
		}
		if c.hint != "" {
			fmt.Printf("    fix: %s\n", c.hint)
		}
	}

	ok := 0
	warn := 0
	fail := 0
	for _, c := range checks {
		switch c.result {
		case statusOK:
			ok++
		case statusWarn:
			warn++
		case statusFail:
			fail++
		}
	}

	fmt.Println()
	fmt.Printf("%d passed, %d warnings, %d failed\n", ok, warn, fail)
	if fail > 0 {
		os.Exit(1)
	}
}

func checkConfiguration() check {
	cfg, err := config.Load(cfgPath)
	if err != nil {
		return check{
			name:   "Configuration",
			result: statusFail,
			detail: fmt.Sprintf("could not load configuration: %v", err),
			hint:   "Run 'kpixivctl config show' for details, or delete the file to regenerate defaults.",
		}
	}

	issues := cfg.Validate()
	if len(issues) == 0 {
		return check{
			name:   "Configuration",
			result: statusOK,
			detail: cfg.ConfigPath,
		}
	}

	return check{
		name:   "Configuration",
		result: statusWarn,
		detail: fmt.Sprintf("%d value(s) were outside the supported range and adjusted:\n      • %s",
			len(issues), strings.Join(issues, "\n      • ")),
		hint: "Review the values with 'kpixivctl config show'.",
	}
}

func checkDirectories() check {
	home, err := os.UserHomeDir()
	if err != nil {
		return check{name: "Directories", result: statusFail, detail: "cannot determine home directory"}
	}

	// Use a minimal config so directory checks work even with a broken file.
	cfg := config.Default()
	cfg.ConfigPath = config.DefaultPath()

	st, err := storage.New(home, cfg.DownloadPath)
	if err != nil {
		return check{
			name:   "Directories",
			result: statusFail,
			detail: fmt.Sprintf("cannot initialize storage directories: %v", err),
		}
	}

	if err := ensureWritable(st.DataDir()); err != nil {
		return check{name: "Directories", result: statusFail, detail: fmt.Sprintf("data directory not writable: %v", err)}
	}
	if err := ensureWritable(st.StateDir()); err != nil {
		return check{name: "Directories", result: statusFail, detail: fmt.Sprintf("state directory not writable: %v", err)}
	}

	return check{
		name:   "Directories",
		result: statusOK,
		detail: fmt.Sprintf("data: %s\n      state: %s", st.DataDir(), st.StateDir()),
	}
}

func checkCache() check {
	home, _ := os.UserHomeDir() //nolint:errcheck // fallback below
	st, err := storage.New(home, "")
	if err != nil {
		return check{name: "Cache", result: statusFail, detail: fmt.Sprintf("cannot initialize storage: %v", err)}
	}

	stats, err := st.CacheStats()
	if err != nil {
		return check{name: "Cache", result: statusFail, detail: fmt.Sprintf("cannot read cache metadata: %v", err)}
	}

	return check{
		name:   "Cache",
		result: statusOK,
		detail: fmt.Sprintf("%d cached image(s), %s", stats.Count, formatBytes(stats.Size)),
	}
}

func checkAuthentication() check {
	home, _ := os.UserHomeDir() //nolint:errcheck // storage.New falls back
	st, err := storage.New(home, "")
	if err != nil {
		return check{name: "Authentication", result: statusFail, detail: "cannot initialize storage"}
	}

	client, err := pixiv.NewClient(st.StateDir())
	if err != nil {
		return check{
			name:   "Authentication",
			result: statusFail,
			detail: fmt.Sprintf("cannot load Pixiv session: %v", err),
			hint:   "Run 'kpixivctl account login' to re-authenticate.",
		}
	}

	if !client.LoggedIn() {
		return check{
			name:   "Authentication",
			result: statusWarn,
			detail: "not logged in to Pixiv",
			hint:   "Run 'kpixivctl account login' to enable bookmarks and sync.",
		}
	}

	user := client.AuthUserName()
	if user == "" {
		user = "(unknown user)"
	}

	expiry := client.AuthExpiry()
	if !expiry.IsZero() && expiry.Before(time.Now()) {
		return check{
			name:   "Authentication",
			result: statusWarn,
			detail: fmt.Sprintf("session for %s has expired; it will refresh automatically", user),
		}
	}

	detail := "logged in as " + user
	if !expiry.IsZero() {
		detail += fmt.Sprintf(" (token valid until %s)", expiry.Format(time.DateTime))
	}
	return check{name: "Authentication", result: statusOK, detail: detail}
}

func checkPixivAPI() check {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://www.pixiv.net/ranking.php?format=json&mode=daily&content=illust&p=1", nil)
	if err != nil {
		return check{name: "Pixiv API", result: statusFail, detail: fmt.Sprintf("cannot build request: %v", err)}
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return check{
			name:   "Pixiv API",
			result: statusFail,
			detail: fmt.Sprintf("cannot reach pixiv.net: %v", err),
			hint:   "Check your internet connection and firewall.",
		}
	}
	//nolint:errcheck // best-effort close
	_ = resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
		return check{name: "Pixiv API", result: statusOK, detail: "ranking endpoint reachable"}
	case http.StatusForbidden:
		return check{
			name:   "Pixiv API",
			result: statusWarn,
			detail: "pixiv.net returned 403 (possible Cloudflare challenge)",
		}
	default:
		return check{
			name:   "Pixiv API",
			result: statusWarn,
			detail: fmt.Sprintf("pixiv.net responded with status %d", resp.StatusCode),
		}
	}
}

func checkDBus() check {
	if os.Getenv("DBUS_SESSION_BUS_ADDRESS") == "" {
		return check{
			name:   "DBus session bus",
			result: statusWarn,
			detail: "DBUS_SESSION_BUS_ADDRESS is not set",
			hint:   "Launch kPixiv from your desktop session, not over SSH.",
		}
	}

	bin, err := exec.LookPath("dbus-send")
	if err != nil {
		return check{
			name:   "DBus session bus",
			result: statusWarn,
			detail: "dbus-send not found",
			hint:   "Install dbus (usually present on any desktop install).",
		}
	}

	// #nosec G204 -- fixed arguments, no user input.
	out, err := exec.Command(bin, "--session", "--dest=org.freedesktop.DBus",
		"--type=method_call", "--print-reply", "/", "org.freedesktop.DBus.Peer.Ping").CombinedOutput()
	if err != nil {
		return check{
			name:   "DBus session bus",
			result: statusWarn,
			detail: fmt.Sprintf("session bus ping failed: %s", strings.TrimSpace(string(out))),
		}
	}

	return check{name: "DBus session bus", result: statusOK, detail: "session bus reachable"}
}

func checkPlasma() check {
	desktop := os.Getenv("XDG_CURRENT_DESKTOP")
	if !strings.Contains(strings.ToLower(desktop), "kde") {
		return check{
			name:   "KDE Plasma",
			result: statusWarn,
			detail: fmt.Sprintf("XDG_CURRENT_DESKTOP is %q; kPixiv is optimized for KDE Plasma", desktop),
		}
	}

	if platform.IsInstanceRunning() {
		return check{name: "KDE Plasma", result: statusOK, detail: "detected " + desktop + "; daemon is running"}
	}
	return check{name: "KDE Plasma", result: statusOK, detail: "detected " + desktop}
}

func checkWallpaperBackend() check {
	setter := wallpaper.NewKDESetter(false)
	if _, err := setter.Screens(); err != nil {
		return check{
			name:   "Wallpaper backend",
			result: statusWarn,
			detail: fmt.Sprintf("cannot query Plasma screens: %v", err),
			hint:   "Ensure qdbus is installed and Plasma is running.",
		}
	}

	return check{name: "Wallpaper backend", result: statusOK, detail: "Plasma screens queryable"}
}

func checkAutostart() check {
	enabled, err := platform.IsServiceEnabled("kpixiv.service")
	if err != nil {
		return check{
			name:   "Autostart",
			result: statusWarn,
			detail: fmt.Sprintf("cannot query service: %v", err),
		}
	}

	if !enabled {
		return check{
			name:   "Autostart",
			result: statusWarn,
			detail: "KPixiv will not start automatically on login",
			hint:   "Run 'kpixivctl autostart enable' to enable it.",
		}
	}

	detail := "systemd user service enabled"
	if platform.IsServiceActive("kpixiv.service") {
		detail += " and active"
	} else {
		detail += " (not currently running)"
	}
	return check{name: "Autostart", result: statusOK, detail: detail}
}

func ensureWritable(dir string) error {
	tmp, err := os.CreateTemp(dir, ".doctor-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	//nolint:errcheck // best-effort close
	_ = tmp.Close()
	return os.Remove(name)
}
