package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDefault(t *testing.T) {
	cfg := Default()

	if cfg.ConfigDir == "" {
		t.Error("Default().ConfigDir is empty")
	}
	if cfg.OutDir != "./downloads" {
		t.Errorf("OutDir = %q, want ./downloads", cfg.OutDir)
	}
	if !strings.HasSuffix(cfg.CookiesPath, "cookies.txt") {
		t.Errorf("CookiesPath = %q, want suffix cookies.txt", cfg.CookiesPath)
	}
	if !strings.HasSuffix(cfg.SessionPath, "session.json") {
		t.Errorf("SessionPath = %q, want suffix session.json", cfg.SessionPath)
	}
	if filepath.Base(cfg.ArchiveDir) != "archive" {
		t.Errorf("ArchiveDir = %q, want base 'archive'", cfg.ArchiveDir)
	}
	if cfg.Concurrency != 3 {
		t.Errorf("Concurrency = %d, want 3", cfg.Concurrency)
	}
	if cfg.ChromeDebugPort != 9222 {
		t.Errorf("ChromeDebugPort = %d, want 9222", cfg.ChromeDebugPort)
	}
	if cfg.Backend.GalleryDLPath != "gallery-dl" {
		t.Errorf("GalleryDLPath = %q, want gallery-dl", cfg.Backend.GalleryDLPath)
	}
	if cfg.Backend.YTDLPPath != "yt-dlp" {
		t.Errorf("YTDLPPath = %q, want yt-dlp", cfg.Backend.YTDLPPath)
	}
	if cfg.StaleAfter != 24*time.Hour {
		t.Errorf("StaleAfter = %s, want 24h", cfg.StaleAfter)
	}
	if cfg.WarnAfter != 7*24*time.Hour {
		t.Errorf("WarnAfter = %s, want 168h", cfg.WarnAfter)
	}
}

// setHome points HOME at a temp dir so Default()/Load() don't touch the
// real user home. On macOS, os.UserHomeDir also consults $HOME first.
func setHome(t *testing.T) string {
	t.Helper()
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	// Also set the Windows-ish env just in case the test binary runs
	// cross-platform; harmless on Unix.
	t.Setenv("USERPROFILE", tmp)
	return tmp
}

func TestLoad_NoFileReturnsDefaults(t *testing.T) {
	home := setHome(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() returned error: %v", err)
	}

	wantDir := filepath.Join(home, ".ig-dl")
	if cfg.ConfigDir != wantDir {
		t.Errorf("ConfigDir = %q, want %q", cfg.ConfigDir, wantDir)
	}
	if cfg.Concurrency != 3 {
		t.Errorf("Concurrency = %d, want default 3", cfg.Concurrency)
	}
	if cfg.ChromeDebugPort != 9222 {
		t.Errorf("ChromeDebugPort = %d, want default 9222", cfg.ChromeDebugPort)
	}

	// EnsureDirs side-effect: config dir was created.
	if info, err := os.Stat(wantDir); err != nil {
		t.Errorf("expected ConfigDir to exist: %v", err)
	} else if !info.IsDir() {
		t.Errorf("ConfigDir is not a directory")
	}
}

func TestLoad_PartialOverrideMerges(t *testing.T) {
	home := setHome(t)
	configDir := filepath.Join(home, ".ig-dl")
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	toml := `concurrency = 10
chrome_debug_port = 9333
stale_after = "12h"
`
	if err := os.WriteFile(filepath.Join(configDir, "config.toml"), []byte(toml), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() returned error: %v", err)
	}

	// Overridden fields
	if cfg.Concurrency != 10 {
		t.Errorf("Concurrency = %d, want 10", cfg.Concurrency)
	}
	if cfg.ChromeDebugPort != 9333 {
		t.Errorf("ChromeDebugPort = %d, want 9333", cfg.ChromeDebugPort)
	}
	if cfg.StaleAfter != 12*time.Hour {
		t.Errorf("StaleAfter = %s, want 12h", cfg.StaleAfter)
	}

	// Other fields kept defaults
	if cfg.OutDir != "./downloads" {
		t.Errorf("OutDir = %q, want default ./downloads", cfg.OutDir)
	}
	if cfg.Backend.GalleryDLPath != "gallery-dl" {
		t.Errorf("GalleryDLPath = %q, want default gallery-dl", cfg.Backend.GalleryDLPath)
	}
	if cfg.Backend.YTDLPPath != "yt-dlp" {
		t.Errorf("YTDLPPath = %q, want default yt-dlp", cfg.Backend.YTDLPPath)
	}
	if cfg.WarnAfter != 7*24*time.Hour {
		t.Errorf("WarnAfter = %s, want default 168h", cfg.WarnAfter)
	}
}

func TestLoad_MalformedTOMLReturnsError(t *testing.T) {
	home := setHome(t)
	configDir := filepath.Join(home, ".ig-dl")
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	// "concurrency" unterminated string => TOML parse error.
	bad := "concurrency = \"oops\nchrome_debug_port = 1\n"
	if err := os.WriteFile(filepath.Join(configDir, "config.toml"), []byte(bad), 0o600); err != nil {
		t.Fatalf("write bad config: %v", err)
	}

	_, err := Load()
	if err == nil {
		t.Fatal("expected error from malformed TOML, got nil")
	}
	if !strings.Contains(err.Error(), "parse") && !strings.Contains(err.Error(), "config.toml") {
		t.Errorf("error %q missing descriptive context", err)
	}
}

func TestValidate(t *testing.T) {
	base := Default()
	// Default() should be valid on its own.
	if err := base.Validate(); err != nil {
		t.Fatalf("Default() failed Validate: %v", err)
	}

	cases := []struct {
		name    string
		mutate  func(*Config)
		wantSub string
	}{
		{"concurrency zero", func(c *Config) { c.Concurrency = 0 }, "Concurrency"},
		{"port zero", func(c *Config) { c.ChromeDebugPort = 0 }, "ChromeDebugPort"},
		{"port too high", func(c *Config) { c.ChromeDebugPort = 70000 }, "ChromeDebugPort"},
		{"stale zero", func(c *Config) { c.StaleAfter = 0 }, "StaleAfter"},
		{"warn less than stale", func(c *Config) {
			c.StaleAfter = 10 * time.Hour
			c.WarnAfter = 5 * time.Hour
		}, "WarnAfter"},
		{"blank config dir", func(c *Config) { c.ConfigDir = "" }, "ConfigDir"},
		{"blank out dir", func(c *Config) { c.OutDir = "" }, "OutDir"},
		{"blank cookies path", func(c *Config) { c.CookiesPath = "" }, "CookiesPath"},
		{"blank session path", func(c *Config) { c.SessionPath = "" }, "SessionPath"},
		{"blank archive dir", func(c *Config) { c.ArchiveDir = "" }, "ArchiveDir"},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			cfg := Default()
			tc.mutate(&cfg)
			err := cfg.Validate()
			if err == nil {
				t.Fatalf("Validate passed but should have failed for %s", tc.name)
			}
			if !strings.Contains(err.Error(), tc.wantSub) {
				t.Errorf("error %q missing substring %q", err, tc.wantSub)
			}
		})
	}
}
