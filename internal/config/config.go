// Package config loads and validates ig-dl's on-disk configuration.
//
// Configuration lives at <ConfigDir>/config.toml (ConfigDir defaults to
// ~/.ig-dl). The file is optional: when absent, Default() values are used.
// When present, its fields are overlaid onto the defaults, so users only
// need to specify keys they want to change.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/BurntSushi/toml"
)

// Config is the fully-resolved runtime configuration for ig-dl.
//
// All path fields are absolute once Load has returned. Durations are
// parsed from Go duration strings in TOML (e.g. "24h", "168h").
type Config struct {
	// ConfigDir is the root directory that holds config.toml, cookies.txt,
	// session.json, and the archive subdirectory.
	ConfigDir string `toml:"config_dir"`
	// OutDir is the default download destination when the user does not
	// override it on the command line.
	OutDir string `toml:"out_dir"`
	// CookiesPath is the Netscape cookies.txt file fed to backends.
	CookiesPath string `toml:"cookies_path"`
	// SessionPath is the JSON session blob written by CDP attach or the
	// companion extension's export.
	SessionPath string `toml:"session_path"`
	// ArchiveDir holds per-handle gallery-dl archive databases so re-runs
	// can skip already-downloaded media.
	ArchiveDir string `toml:"archive_dir"`
	// Concurrency is the default size of the profile-bulk worker pool.
	Concurrency int `toml:"concurrency"`
	// ChromeDebugPort is the port the CDP attacher connects to.
	ChromeDebugPort int `toml:"chrome_debug_port"`
	// Backend holds overridable paths to the external downloader binaries.
	Backend BackendPaths `toml:"backend"`
	// StaleAfter is the age past which a session is silently refreshed when
	// Chrome is reachable.
	StaleAfter time.Duration `toml:"stale_after"`
	// WarnAfter is the age past which ig-dl prints a "session is old"
	// warning regardless of refresh success.
	WarnAfter time.Duration `toml:"warn_after"`
}

// BackendPaths holds the paths to the downloader binaries.
//
// Values are passed to os/exec.LookPath so bare names like "gallery-dl"
// resolve against $PATH.
type BackendPaths struct {
	// GalleryDLPath is the path (or $PATH name) of the gallery-dl binary.
	GalleryDLPath string `toml:"gallery_dl_path"`
	// YTDLPPath is the path (or $PATH name) of the yt-dlp binary.
	YTDLPPath string `toml:"yt_dlp_path"`
}

// Default returns a Config populated with ig-dl's built-in defaults.
//
// ConfigDir is rooted at the current user's home directory; if that
// cannot be determined, ConfigDir falls back to ".ig-dl" in the current
// working directory. All derived paths (CookiesPath, SessionPath,
// ArchiveDir) are computed relative to ConfigDir.
func Default() Config {
	configDir := defaultConfigDir()
	return Config{
		ConfigDir:       configDir,
		OutDir:          "./downloads",
		CookiesPath:     filepath.Join(configDir, "cookies.txt"),
		SessionPath:     filepath.Join(configDir, "session.json"),
		ArchiveDir:      filepath.Join(configDir, "archive"),
		Concurrency:     3,
		ChromeDebugPort: 9222,
		Backend: BackendPaths{
			GalleryDLPath: "gallery-dl",
			YTDLPPath:     "yt-dlp",
		},
		StaleAfter: 24 * time.Hour,
		WarnAfter:  7 * 24 * time.Hour,
	}
}

// defaultConfigDir resolves ~/.ig-dl, or falls back to ./.ig-dl when the
// home directory cannot be determined.
func defaultConfigDir() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ".ig-dl"
	}
	return filepath.Join(home, ".ig-dl")
}

// partialConfig mirrors Config but uses pointers for every field so the
// TOML decoder can tell "absent" from "zero value". Only the fields that
// were actually present in the file are non-nil after decoding.
type partialConfig struct {
	ConfigDir       *string           `toml:"config_dir"`
	OutDir          *string           `toml:"out_dir"`
	CookiesPath     *string           `toml:"cookies_path"`
	SessionPath     *string           `toml:"session_path"`
	ArchiveDir      *string           `toml:"archive_dir"`
	Concurrency     *int              `toml:"concurrency"`
	ChromeDebugPort *int              `toml:"chrome_debug_port"`
	Backend         *partialBackend   `toml:"backend"`
	StaleAfter      *duration         `toml:"stale_after"`
	WarnAfter       *duration         `toml:"warn_after"`
}

// partialBackend is the pointer-field twin of BackendPaths.
type partialBackend struct {
	GalleryDLPath *string `toml:"gallery_dl_path"`
	YTDLPPath     *string `toml:"yt_dlp_path"`
}

// duration is a time.Duration with a TOML unmarshaller that accepts Go
// duration strings like "24h" or "30m".
type duration struct {
	D time.Duration
}

// UnmarshalText parses text as a Go duration string.
func (d *duration) UnmarshalText(text []byte) error {
	parsed, err := time.ParseDuration(string(text))
	if err != nil {
		return fmt.Errorf("invalid duration %q: %w", string(text), err)
	}
	d.D = parsed
	return nil
}

// Load returns the effective configuration: defaults, overlaid by any
// values found in <ConfigDir>/config.toml, with ConfigDir and ArchiveDir
// created on disk with 0700 permissions before returning.
//
// If config.toml is absent, Load returns the defaults with a nil error.
// If it exists but cannot be parsed, Load returns a descriptive error.
func Load() (Config, error) {
	cfg := Default()

	// If the user overrides ConfigDir via a top-level TOML key, the derived
	// paths need to be recomputed. Read the file once into a partial
	// struct, apply ConfigDir first if present, then overlay the rest.
	path := filepath.Join(cfg.ConfigDir, "config.toml")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			if dirErr := cfg.EnsureDirs(); dirErr != nil {
				return cfg, dirErr
			}
			return cfg, nil
		}
		return cfg, fmt.Errorf("read %s: %w", path, err)
	}

	var partial partialConfig
	if _, err := toml.Decode(string(data), &partial); err != nil {
		return cfg, fmt.Errorf("parse %s: %w", path, err)
	}

	// Apply ConfigDir first so that if the user moves it, the still-default
	// derived paths move with it before any explicit overrides land.
	if partial.ConfigDir != nil {
		cfg.ConfigDir = *partial.ConfigDir
		cfg.CookiesPath = filepath.Join(cfg.ConfigDir, "cookies.txt")
		cfg.SessionPath = filepath.Join(cfg.ConfigDir, "session.json")
		cfg.ArchiveDir = filepath.Join(cfg.ConfigDir, "archive")
	}
	if partial.OutDir != nil {
		cfg.OutDir = *partial.OutDir
	}
	if partial.CookiesPath != nil {
		cfg.CookiesPath = *partial.CookiesPath
	}
	if partial.SessionPath != nil {
		cfg.SessionPath = *partial.SessionPath
	}
	if partial.ArchiveDir != nil {
		cfg.ArchiveDir = *partial.ArchiveDir
	}
	if partial.Concurrency != nil {
		cfg.Concurrency = *partial.Concurrency
	}
	if partial.ChromeDebugPort != nil {
		cfg.ChromeDebugPort = *partial.ChromeDebugPort
	}
	if partial.Backend != nil {
		if partial.Backend.GalleryDLPath != nil {
			cfg.Backend.GalleryDLPath = *partial.Backend.GalleryDLPath
		}
		if partial.Backend.YTDLPPath != nil {
			cfg.Backend.YTDLPPath = *partial.Backend.YTDLPPath
		}
	}
	if partial.StaleAfter != nil {
		cfg.StaleAfter = partial.StaleAfter.D
	}
	if partial.WarnAfter != nil {
		cfg.WarnAfter = partial.WarnAfter.D
	}

	if err := cfg.EnsureDirs(); err != nil {
		return cfg, err
	}
	return cfg, nil
}

// EnsureDirs creates ConfigDir and ArchiveDir (including parents) with
// mode 0700. Existing directories are left alone.
func (c Config) EnsureDirs() error {
	if c.ConfigDir == "" {
		return fmt.Errorf("config: ConfigDir is empty")
	}
	if err := os.MkdirAll(c.ConfigDir, 0o700); err != nil {
		return fmt.Errorf("create config dir %s: %w", c.ConfigDir, err)
	}
	if c.ArchiveDir != "" {
		if err := os.MkdirAll(c.ArchiveDir, 0o700); err != nil {
			return fmt.Errorf("create archive dir %s: %w", c.ArchiveDir, err)
		}
	}
	return nil
}

// Validate checks that every field holds a sensible value. It returns
// the first problem it finds, or nil if all invariants hold.
func (c Config) Validate() error {
	if c.ConfigDir == "" {
		return fmt.Errorf("config: ConfigDir must not be empty")
	}
	if c.OutDir == "" {
		return fmt.Errorf("config: OutDir must not be empty")
	}
	if c.CookiesPath == "" {
		return fmt.Errorf("config: CookiesPath must not be empty")
	}
	if c.SessionPath == "" {
		return fmt.Errorf("config: SessionPath must not be empty")
	}
	if c.ArchiveDir == "" {
		return fmt.Errorf("config: ArchiveDir must not be empty")
	}
	if c.Concurrency < 1 {
		return fmt.Errorf("config: Concurrency must be >= 1, got %d", c.Concurrency)
	}
	if c.ChromeDebugPort < 1 || c.ChromeDebugPort > 65535 {
		return fmt.Errorf("config: ChromeDebugPort must be in 1..65535, got %d", c.ChromeDebugPort)
	}
	if c.StaleAfter <= 0 {
		return fmt.Errorf("config: StaleAfter must be > 0, got %s", c.StaleAfter)
	}
	if c.WarnAfter < c.StaleAfter {
		return fmt.Errorf("config: WarnAfter (%s) must be >= StaleAfter (%s)", c.WarnAfter, c.StaleAfter)
	}
	return nil
}
