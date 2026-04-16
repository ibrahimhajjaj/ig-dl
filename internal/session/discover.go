package session

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// Browser identifies a Chromium-based browser whose DevToolsActivePort
// file ig-dl knows how to locate.
type Browser string

const (
	BrowserChrome   Browser = "chrome"
	BrowserEdge     Browser = "edge"
	BrowserBrave    Browser = "brave"
	BrowserArc      Browser = "arc"
	BrowserVivaldi  Browser = "vivaldi"
	BrowserChromium Browser = "chromium"
)

// KnownBrowsers returns every browser ig-dl can discover a
// DevToolsActivePort for, in stable order.
func KnownBrowsers() []Browser {
	return []Browser{
		BrowserChrome, BrowserEdge, BrowserBrave,
		BrowserArc, BrowserVivaldi, BrowserChromium,
	}
}

// ErrNoActivePort indicates no live DevToolsActivePort was found for any
// known Chromium-based browser — meaning no browser is currently
// serving CDP. The fix is to launch one with
// --remote-debugging-port=<port> and --user-data-dir=<fresh-path>.
var ErrNoActivePort = errors.New("session: no live CDP endpoint found; launch a Chromium browser with --remote-debugging-port and --user-data-dir (see README)")

// ActivePort describes a browser that is currently exposing CDP.
type ActivePort struct {
	Browser Browser
	Port    int
	// WSPath is the optional WebSocket path prefix from line 2 of the
	// DevToolsActivePort file. Empty if the browser didn't write one.
	WSPath string
	// Source is the absolute path of the DevToolsActivePort file we read.
	Source string
}

// DiscoverActivePort scans the known browser user-data-dirs for a
// DevToolsActivePort file and returns the first one it can parse. It
// prefers the browser the caller names (via `preferred`, may be empty),
// then falls back to the default KnownBrowsers order.
//
// This is the live-attach path Chromium introduced in 2025: the user
// opens `chrome://inspect/#remote-debugging` in their normal browser,
// toggles on remote debugging, and Chromium writes the dynamic port it
// picked to `<user-data-dir>/DevToolsActivePort`. Reading the file is
// the zero-config way to find the port — no `--remote-debugging-port`
// flag required on launch.
func DiscoverActivePort(preferred Browser) (*ActivePort, error) {
	return discoverActivePort(preferred, true)
}

// DiscoverAllActivePorts returns every DevToolsActivePort file it
// finds without filtering stale entries. Used by `ig-dl browsers` to
// show the full picture.
func DiscoverAllActivePorts() []*ActivePort {
	var out []*ActivePort
	for _, b := range KnownBrowsers() {
		paths, err := userDataDirs(b)
		if err != nil {
			continue
		}
		for _, dir := range paths {
			port, wspath, err := readActivePort(filepath.Join(dir, "DevToolsActivePort"))
			if err != nil {
				continue
			}
			out = append(out, &ActivePort{
				Browser: b,
				Port:    port,
				WSPath:  wspath,
				Source:  filepath.Join(dir, "DevToolsActivePort"),
			})
		}
	}
	return out
}

// IsLive probes the discovered port to check that CDP is actually
// answering — DevToolsActivePort files often linger after a debug
// browser is closed.
//
// Two protocol shapes are considered live:
//   - Classic `--remote-debugging-port`: /json/version returns 200.
//   - Chrome M144 toggle: /json/version returns 404 but a TCP
//     connection succeeds AND a specific WebSocket path was recorded.
func (a *ActivePort) IsLive(ctx context.Context) bool {
	if a == nil {
		return false
	}
	// Fast path: can we even open a TCP connection?
	d := net.Dialer{Timeout: 500 * time.Millisecond}
	conn, err := d.DialContext(ctx, "tcp", fmt.Sprintf("127.0.0.1:%d", a.Port))
	if err != nil {
		return false
	}
	_ = conn.Close()
	// If the DevToolsActivePort file recorded a WSPath (M144 toggle
	// shape), TCP connect is enough — CDP will handle the ws upgrade
	// directly and /json/version isn't exposed.
	if a.WSPath != "" {
		return true
	}
	// Classic shape: require /json/version to return 200.
	client := &http.Client{Timeout: 500 * time.Millisecond}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		fmt.Sprintf("http://127.0.0.1:%d/json/version", a.Port), nil)
	if err != nil {
		return false
	}
	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

func discoverActivePort(preferred Browser, requireLive bool) (*ActivePort, error) {
	order := orderedBrowsers(preferred)
	var lastErr error
	for _, b := range order {
		paths, err := userDataDirs(b)
		if err != nil {
			lastErr = err
			continue
		}
		for _, dir := range paths {
			port, wspath, err := readActivePort(filepath.Join(dir, "DevToolsActivePort"))
			if err != nil {
				if !errors.Is(err, os.ErrNotExist) {
					lastErr = err
				}
				continue
			}
			ap := &ActivePort{
				Browser: b,
				Port:    port,
				WSPath:  wspath,
				Source:  filepath.Join(dir, "DevToolsActivePort"),
			}
			if requireLive && !ap.IsLive(context.Background()) {
				continue
			}
			return ap, nil
		}
	}
	if lastErr != nil {
		return nil, fmt.Errorf("%w (last error: %v)", ErrNoActivePort, lastErr)
	}
	return nil, ErrNoActivePort
}

// readActivePort reads and parses a DevToolsActivePort file. Line 1 is
// the port; line 2, if present, is the WebSocket path prefix.
func readActivePort(path string) (int, string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return 0, "", err
	}
	lines := strings.Split(strings.TrimRight(string(raw), "\n"), "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) == "" {
		return 0, "", fmt.Errorf("%s: empty file", path)
	}
	port, err := strconv.Atoi(strings.TrimSpace(lines[0]))
	if err != nil {
		return 0, "", fmt.Errorf("%s: bad port %q: %w", path, lines[0], err)
	}
	if port < 1 || port > 65535 {
		return 0, "", fmt.Errorf("%s: port %d out of range", path, port)
	}
	wspath := ""
	if len(lines) > 1 {
		wspath = strings.TrimSpace(lines[1])
	}
	return port, wspath, nil
}

// orderedBrowsers puts `preferred` at the front of KnownBrowsers (if it
// names a known browser), so callers control which browser we probe
// first without losing the fallback order.
func orderedBrowsers(preferred Browser) []Browser {
	all := KnownBrowsers()
	if preferred == "" {
		return all
	}
	out := make([]Browser, 0, len(all))
	out = append(out, preferred)
	for _, b := range all {
		if b != preferred {
			out = append(out, b)
		}
	}
	return out
}

// userDataDirs returns the candidate user-data-dirs to probe for a
// given browser. Multiple paths are returned when a browser ships
// variants (Canary, Beta) whose data dirs we want to try.
func userDataDirs(b Browser) ([]string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	switch runtime.GOOS {
	case "darwin":
		return macUserDataDirs(home, b), nil
	case "linux":
		return linuxUserDataDirs(home, b), nil
	case "windows":
		return windowsUserDataDirs(b), nil
	}
	return nil, fmt.Errorf("session: DevToolsActivePort discovery not implemented on %s", runtime.GOOS)
}

func macUserDataDirs(home string, b Browser) []string {
	appSupport := filepath.Join(home, "Library", "Application Support")
	switch b {
	case BrowserChrome:
		return []string{
			filepath.Join(appSupport, "Google", "Chrome"),
			filepath.Join(appSupport, "Google", "Chrome Canary"),
			filepath.Join(appSupport, "Google", "Chrome Beta"),
		}
	case BrowserEdge:
		return []string{
			filepath.Join(appSupport, "Microsoft Edge"),
			filepath.Join(appSupport, "Microsoft Edge Beta"),
			filepath.Join(appSupport, "Microsoft Edge Dev"),
		}
	case BrowserBrave:
		return []string{filepath.Join(appSupport, "BraveSoftware", "Brave-Browser")}
	case BrowserArc:
		return []string{filepath.Join(appSupport, "Arc", "User Data")}
	case BrowserVivaldi:
		return []string{filepath.Join(appSupport, "Vivaldi")}
	case BrowserChromium:
		return []string{filepath.Join(appSupport, "Chromium")}
	}
	return nil
}

func linuxUserDataDirs(home string, b Browser) []string {
	configDir := filepath.Join(home, ".config")
	switch b {
	case BrowserChrome:
		return []string{
			filepath.Join(configDir, "google-chrome"),
			filepath.Join(configDir, "google-chrome-beta"),
			filepath.Join(configDir, "google-chrome-unstable"),
		}
	case BrowserEdge:
		return []string{
			filepath.Join(configDir, "microsoft-edge"),
			filepath.Join(configDir, "microsoft-edge-beta"),
			filepath.Join(configDir, "microsoft-edge-dev"),
		}
	case BrowserBrave:
		return []string{filepath.Join(configDir, "BraveSoftware", "Brave-Browser")}
	case BrowserVivaldi:
		return []string{filepath.Join(configDir, "vivaldi")}
	case BrowserChromium:
		return []string{filepath.Join(configDir, "chromium")}
	}
	return nil
}

func windowsUserDataDirs(b Browser) []string {
	local := os.Getenv("LOCALAPPDATA")
	if local == "" {
		return nil
	}
	switch b {
	case BrowserChrome:
		return []string{filepath.Join(local, "Google", "Chrome", "User Data")}
	case BrowserEdge:
		return []string{filepath.Join(local, "Microsoft", "Edge", "User Data")}
	case BrowserBrave:
		return []string{filepath.Join(local, "BraveSoftware", "Brave-Browser", "User Data")}
	case BrowserVivaldi:
		return []string{filepath.Join(local, "Vivaldi", "User Data")}
	case BrowserChromium:
		return []string{filepath.Join(local, "Chromium", "User Data")}
	}
	return nil
}
