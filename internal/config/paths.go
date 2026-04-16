package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ExpandHome expands a leading "~" in path to the current user's home
// directory. A bare "~" becomes the home directory itself; "~/foo"
// becomes "<home>/foo". Paths that do not start with "~" are returned
// unchanged. A leading "~user" form is not supported.
func ExpandHome(path string) (string, error) {
	if path == "" || path[0] != '~' {
		return path, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return path, fmt.Errorf("expand home: %w", err)
	}
	if path == "~" {
		return home, nil
	}
	// Only expand "~/..." — reject "~user/..." as unsupported.
	if path[1] != '/' && path[1] != filepath.Separator {
		return path, fmt.Errorf("expand home: unsupported form %q", path)
	}
	return filepath.Join(home, path[2:]), nil
}

// ArchiveFor returns the archive database path for a given Instagram
// handle: <ArchiveDir>/<sanitized-handle>.sqlite.
//
// Sanitisation strips path separators, removes ".." segments, and trims
// leading dots so a hostile handle cannot escape ArchiveDir. An empty
// or fully-stripped handle returns an empty string; callers should
// treat that as an error.
func (c Config) ArchiveFor(handle string) string {
	sanitised := sanitiseHandle(handle)
	if sanitised == "" {
		return ""
	}
	return filepath.Join(c.ArchiveDir, sanitised+".sqlite")
}

// sanitiseHandle strips characters that could let a handle escape the
// archive directory. It removes path separators, ".." sequences, and
// any leading dots.
func sanitiseHandle(handle string) string {
	h := strings.TrimSpace(handle)
	if h == "" {
		return ""
	}
	// Remove traversal sequences before touching slashes so "../" and
	// "..\\" both vanish entirely.
	h = strings.ReplaceAll(h, "..", "")
	// Flatten any path separators — handles are single names, not paths.
	h = strings.ReplaceAll(h, "/", "")
	h = strings.ReplaceAll(h, "\\", "")
	// Strip leading dots so results never start with '.' (hidden file,
	// current-dir, parent-dir shorthands).
	h = strings.TrimLeft(h, ".")
	return h
}
