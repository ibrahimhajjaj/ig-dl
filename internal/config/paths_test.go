package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExpandHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	// Confirm the env actually took effect on this platform.
	got, err := os.UserHomeDir()
	if err != nil || got != home {
		t.Skipf("UserHomeDir does not honor $HOME on this platform: got %q err %v", got, err)
	}

	cases := []struct {
		in   string
		want string
	}{
		{"~/foo", filepath.Join(home, "foo")},
		{"~", home},
		{"/absolute/path", "/absolute/path"},
		{"relative/path", "relative/path"},
		{"", ""},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.in, func(t *testing.T) {
			got, err := ExpandHome(tc.in)
			if err != nil {
				t.Fatalf("ExpandHome(%q) error: %v", tc.in, err)
			}
			if got != tc.want {
				t.Errorf("ExpandHome(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestExpandHome_UnsupportedForm(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	// "~otheruser" is explicitly not supported; should error.
	if _, err := ExpandHome("~otheruser/foo"); err == nil {
		t.Error("expected error for ~otheruser form, got nil")
	}
}

func TestArchiveFor_Happy(t *testing.T) {
	cfg := Config{ArchiveDir: "/tmp/archive"}
	got := cfg.ArchiveFor("ibra")
	if !strings.Contains(got, "ibra.sqlite") {
		t.Errorf("ArchiveFor(ibra) = %q, want contains ibra.sqlite", got)
	}
	if filepath.Dir(got) != "/tmp/archive" {
		t.Errorf("ArchiveFor(ibra) dir = %q, want /tmp/archive", filepath.Dir(got))
	}
}

func TestArchiveFor_Sanitization(t *testing.T) {
	cfg := Config{ArchiveDir: "/tmp/archive"}

	// "../evil" — traversal sequences stripped and slash flattened.
	got := cfg.ArchiveFor("../evil")
	if strings.Contains(got, "..") {
		t.Errorf("ArchiveFor(../evil) = %q, should not contain ..", got)
	}
	if strings.Contains(got, "/evil") && filepath.Dir(got) != "/tmp/archive" {
		t.Errorf("ArchiveFor(../evil) = %q escaped ArchiveDir", got)
	}
	if !strings.HasSuffix(got, "evil.sqlite") {
		t.Errorf("ArchiveFor(../evil) = %q, want suffix evil.sqlite", got)
	}

	// "a/b" — path separators flattened.
	got = cfg.ArchiveFor("a/b")
	if strings.Contains(got, "a/b") {
		t.Errorf("ArchiveFor(a/b) = %q, should not contain a/b", got)
	}
	if !strings.HasSuffix(got, "ab.sqlite") {
		t.Errorf("ArchiveFor(a/b) = %q, want suffix ab.sqlite", got)
	}

	// "" — empty handle rejected.
	if got := cfg.ArchiveFor(""); got != "" {
		t.Errorf("ArchiveFor(\"\") = %q, want empty", got)
	}

	// ".." — fully-stripped handle rejected.
	if got := cfg.ArchiveFor(".."); got != "" {
		t.Errorf("ArchiveFor(..) = %q, want empty", got)
	}

	// leading dot stripped
	got = cfg.ArchiveFor(".hidden")
	if strings.HasPrefix(filepath.Base(got), ".") {
		t.Errorf("ArchiveFor(.hidden) = %q, base should not start with .", got)
	}
}
