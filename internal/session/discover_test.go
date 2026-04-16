package session

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestReadActivePort(t *testing.T) {
	dir := t.TempDir()
	cases := []struct {
		name     string
		body     string
		wantPort int
		wantWS   string
		wantErr  bool
	}{
		{"port only", "9222\n", 9222, "", false},
		{"port + ws path", "1976\n/devtools/browser/abc\n", 1976, "/devtools/browser/abc", false},
		{"trailing whitespace", "  4242  \n\n", 4242, "", false},
		{"empty file", "", 0, "", true},
		{"non-numeric", "not-a-port\n", 0, "", true},
		{"port out of range low", "0\n", 0, "", true},
		{"port out of range high", "70000\n", 0, "", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := filepath.Join(dir, strings.ReplaceAll(tc.name, " ", "_")+"-DevToolsActivePort")
			if err := os.WriteFile(p, []byte(tc.body), 0600); err != nil {
				t.Fatal(err)
			}
			port, ws, err := readActivePort(p)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("want err, got port=%d ws=%q", port, ws)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
			if port != tc.wantPort {
				t.Errorf("port = %d, want %d", port, tc.wantPort)
			}
			if ws != tc.wantWS {
				t.Errorf("ws = %q, want %q", ws, tc.wantWS)
			}
		})
	}
}

func TestReadActivePort_MissingFile(t *testing.T) {
	_, _, err := readActivePort(filepath.Join(t.TempDir(), "does-not-exist"))
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("want os.ErrNotExist, got %v", err)
	}
}

func TestOrderedBrowsers(t *testing.T) {
	t.Run("no preferred", func(t *testing.T) {
		got := orderedBrowsers("")
		if len(got) != len(KnownBrowsers()) {
			t.Fatalf("len mismatch: got %d, want %d", len(got), len(KnownBrowsers()))
		}
	})
	t.Run("edge preferred", func(t *testing.T) {
		got := orderedBrowsers(BrowserEdge)
		if got[0] != BrowserEdge {
			t.Fatalf("edge not first: %v", got)
		}
		// edge should appear exactly once
		count := 0
		for _, b := range got {
			if b == BrowserEdge {
				count++
			}
		}
		if count != 1 {
			t.Fatalf("edge appears %d times, want 1", count)
		}
	})
	t.Run("unknown preferred falls back", func(t *testing.T) {
		got := orderedBrowsers(Browser("weird"))
		if got[0] != Browser("weird") {
			t.Fatalf("unknown browser should be first anyway, got %v", got[0])
		}
	})
}

func TestUserDataDirs_MacShapes(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("darwin-only path check")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	cases := map[Browser]string{
		BrowserChrome: filepath.Join(home, "Library", "Application Support", "Google", "Chrome"),
		BrowserEdge:   filepath.Join(home, "Library", "Application Support", "Microsoft Edge"),
		BrowserBrave:  filepath.Join(home, "Library", "Application Support", "BraveSoftware", "Brave-Browser"),
		BrowserArc:    filepath.Join(home, "Library", "Application Support", "Arc", "User Data"),
	}
	for b, want := range cases {
		dirs, err := userDataDirs(b)
		if err != nil {
			t.Errorf("%s: %v", b, err)
			continue
		}
		if len(dirs) == 0 || dirs[0] != want {
			t.Errorf("%s primary dir = %v, want %q", b, dirs, want)
		}
	}
}

func TestDiscoverActivePort_NotFound(t *testing.T) {
	// Point HOME at an empty temp dir so no browser can be discovered.
	tmp := t.TempDir()
	t.Setenv("HOME", tmp)
	if runtime.GOOS == "windows" {
		t.Setenv("LOCALAPPDATA", tmp)
	}
	_, err := DiscoverActivePort("")
	if !errors.Is(err, ErrNoActivePort) {
		t.Fatalf("want ErrNoActivePort, got %v", err)
	}
}
