// Package session owns the authenticated-browser state that every
// downstream stage depends on. It produces a types.Session either by
// attaching to a running Chrome instance over the Chrome DevTools Protocol
// (primary path) or by importing a JSON file exported from the companion
// browser extension (fallback path), and knows how to persist that state
// to disk and render it into the Netscape cookies.txt format the
// gallery-dl / yt-dlp backends consume.
package session

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/ibrahimhajjaj/ig-dl/internal/types"
)

// Sentinel errors exposed by the session package. Callers use errors.Is to
// branch on them (e.g. the CLI maps ErrNoSession to exit code 2).
var (
	// ErrNoSession indicates that neither the CDP attach path nor the
	// JSON importer produced a usable session. This is the "no auth
	// available at all" condition the CLI surfaces as exit code 2.
	ErrNoSession = errors.New("session: no usable session (chrome attach and import both failed)")

	// ErrNoIGTab indicates the browser was reachable on the debug port
	// but no open tab matched an instagram.com host.
	ErrNoIGTab = errors.New("session: no instagram.com tab found in attached browser")

	// ErrCDPUnreachable indicates the debug endpoint at
	// http://localhost:<port>/json/version could not be reached. Any
	// Chromium-based browser (Chrome, Edge, Brave, etc.) launched with
	// --remote-debugging-port serves it.
	ErrCDPUnreachable = errors.New("session: CDP debug endpoint unreachable")
)

// Load resolves an authenticated session.
//
// Order of attempts (cheapest + dialog-free first):
//
//  1. Read sessionJSONPath. If it parses and contains cookies, use it.
//     This is free and never triggers the browser's M144 permission
//     dialog — the whole point of persisting the session after login.
//  2. DevToolsActivePort discovery (CDP attach). Pops the M144
//     permission dialog in the user's browser.
//  3. Fixed-port CDP attach (legacy --remote-debugging-port flow).
//
// Callers wanting a forced CDP capture should call AttachDiscovered /
// AttachRunningChrome directly (that is what `ig-dl login` does) rather
// than Load — Load is the hot path used by every download command and
// must not spam the user with permission dialogs.
func Load(ctx context.Context, sessionJSONPath string, debugPort int) (*types.Session, error) {
	// 1. Cached session on disk — always try first so repeated commands
	//    within the session's lifetime don't re-trigger the M144 dialog.
	if s, err := Import(sessionJSONPath); err == nil {
		return s, nil
	}
	// 2. Discovery path. Real browser, real profile, but triggers the
	//    M144 permission dialog — only reached when we have no cached
	//    session to fall back on.
	if s, _, err := AttachDiscovered(ctx, ""); err == nil {
		return s, nil
	}
	// 3. Fixed-port path: --remote-debugging-port + --user-data-dir.
	if s, err := AttachRunningChrome(ctx, debugPort); err == nil {
		return s, nil
	}
	return nil, fmt.Errorf("%w: import from %q failed and no live browser was reachable", ErrNoSession, sessionJSONPath)
}

// Save serialises s as pretty-printed JSON at path. The parent directory
// is created with mode 0700 if missing and the file is written with mode
// 0600 because the cookie set inside counts as a credential.
func Save(s *types.Session, path string) error {
	if s == nil {
		return errors.New("session: cannot save nil session")
	}
	dir := filepath.Dir(path)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return fmt.Errorf("session: create parent dir: %w", err)
		}
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("session: marshal: %w", err)
	}
	// Write with 0600 so the credential material isn't world-readable.
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("session: write %s: %w", path, err)
	}
	return nil
}

// Age reports how long ago the session was captured. A zero CapturedAt
// (e.g. an uninitialised Session value) returns 0 so callers can treat it
// distinctly from "a fraction of a second old".
func Age(s *types.Session) time.Duration {
	if s == nil || s.CapturedAt.IsZero() {
		return 0
	}
	return time.Since(s.CapturedAt)
}
