// Package core is the single orchestration layer shared by the CLI and the
// MCP server. Both front-ends call Download* through this package so there
// is exactly one implementation of each user-facing operation.
package core

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"

	"github.com/ibrhajjaj/ig-dl/internal/backend"
	"github.com/ibrhajjaj/ig-dl/internal/config"
	"github.com/ibrhajjaj/ig-dl/internal/router"
	"github.com/ibrhajjaj/ig-dl/internal/session"
	"github.com/ibrhajjaj/ig-dl/internal/types"
)

// Progress is the optional callback front-ends use to surface structured
// status events (MCP progress notifications, CLI spinner updates).
type Progress func(stage string, current, total float64, msg string)

// Options bundles the knobs an orchestration call needs. It is the common
// argument passed by every CLI command and every MCP tool.
type Options struct {
	Config   config.Config
	OutDir   string
	Stdout   io.Writer
	Stderr   io.Writer
	Progress Progress
}

// Result is the structured summary returned to the caller after a
// download. It intentionally maps 1:1 to the MCP tool output schema and
// the CLI --json schema so every channel sees the same contract.
type Result struct {
	OutDir   string            `json:"out_dir"`
	Counts   map[string]int    `json:"counts,omitempty"`
	Files    []string          `json:"files,omitempty"`
	Failures []string          `json:"failures,omitempty"`
	Handle   string            `json:"handle,omitempty"`
	Meta     map[string]string `json:"meta,omitempty"`
}

// DownloadURL handles the single-URL path (posts, reels, stories,
// highlights). The router picks the backend, the session manager fronts
// auth, and the selected backend shells out.
func DownloadURL(ctx context.Context, url string, opt Options) (*Result, error) {
	if err := opt.Config.Validate(); err != nil {
		return nil, fmt.Errorf("config invalid: %w", err)
	}
	target, err := router.Parse(url)
	if err != nil {
		return nil, err
	}

	outDir, err := resolveOutDir(opt, target)
	if err != nil {
		return nil, err
	}

	b, err := pickBackend(opt, target)
	if err != nil {
		return nil, err
	}

	sess, err := ensureSession(ctx, opt)
	if err != nil {
		return nil, err
	}

	notify(opt.Progress, "download", 0, 1, fmt.Sprintf("downloading %s", url))
	if err := b.Fetch(ctx, target, sess, outDir); err != nil {
		return nil, fmt.Errorf("%s: %w", router.Choose(target), err)
	}
	notify(opt.Progress, "download", 1, 1, "done")

	return &Result{
		OutDir: outDir,
		Counts: map[string]int{"invocations": 1},
		Meta:   map[string]string{"url": url, "backend": router.Choose(target).String()},
	}, nil
}

// DownloadUser runs the profile-bulk flow: profile posts/reels,
// stories, and highlights. Each stage maps to one backend invocation
// and emits one progress event.
func DownloadUser(ctx context.Context, handle string, include []string, opt Options) (*Result, error) {
	if err := opt.Config.Validate(); err != nil {
		return nil, fmt.Errorf("config invalid: %w", err)
	}
	target, err := router.Parse(handle)
	if err != nil {
		return nil, err
	}
	if target.Kind != types.TargetUserAll {
		return nil, fmt.Errorf("not a profile handle: %s", handle)
	}

	outDir, err := resolveOutDir(opt, target)
	if err != nil {
		return nil, err
	}

	sess, err := ensureSession(ctx, opt)
	if err != nil {
		return nil, err
	}

	b := &backend.GalleryDL{
		BinPath:     opt.Config.Backend.GalleryDLPath,
		CookiesFile: opt.Config.CookiesPath,
		OutDir:      outDir,
		ArchiveDir:  opt.Config.ArchiveDir,
		Stdout:      opt.Stdout,
		Stderr:      opt.Stderr,
	}

	counts := map[string]int{}
	var failures []string
	notify(opt.Progress, "profile", 0, 1, fmt.Sprintf("gallery-dl for %s", target.Handle))
	if err := b.Fetch(ctx, target, sess, outDir); err != nil {
		failures = append(failures, err.Error())
	} else {
		counts["profile"] = 1
	}
	notify(opt.Progress, "profile", 1, 1, "done")

	res := &Result{
		OutDir:   outDir,
		Handle:   target.Handle,
		Counts:   counts,
		Failures: failures,
		Meta:     map[string]string{"backend": "gallery-dl", "archive": opt.Config.ArchiveFor(target.Handle)},
	}
	if len(failures) > 0 {
		return res, errors.New("one or more stages failed")
	}
	return res, nil
}

// DownloadSaved pulls the authenticated user's saved collection.
func DownloadSaved(ctx context.Context, opt Options) (*Result, error) {
	if err := opt.Config.Validate(); err != nil {
		return nil, fmt.Errorf("config invalid: %w", err)
	}

	target := router.SavedTarget()

	outDir, err := resolveOutDir(opt, target)
	if err != nil {
		return nil, err
	}

	sess, err := ensureSession(ctx, opt)
	if err != nil {
		return nil, err
	}

	b := &backend.GalleryDL{
		BinPath:     opt.Config.Backend.GalleryDLPath,
		CookiesFile: opt.Config.CookiesPath,
		OutDir:      outDir,
		Stdout:      opt.Stdout,
		Stderr:      opt.Stderr,
	}
	notify(opt.Progress, "saved", 0, 1, "downloading saved collection")
	if err := b.Fetch(ctx, target, sess, outDir); err != nil {
		return nil, err
	}
	notify(opt.Progress, "saved", 1, 1, "done")

	return &Result{
		OutDir: outDir,
		Meta:   map[string]string{"backend": "gallery-dl"},
	}, nil
}

// SessionStatus reports whether a usable session exists and how old it
// is. It tries importer first (cheapest), then DevToolsActivePort
// discovery (fast, no launch flag), and finally a fixed-port probe.
func SessionStatus(ctx context.Context, opt Options) (authed bool, ageSeconds float64, source string, err error) {
	s, ierr := session.Import(opt.Config.SessionPath)
	if ierr == nil {
		return true, session.Age(s).Seconds(), "imported", nil
	}
	// Discovery probe (checks if any known browser has CDP enabled
	// without needing a launch flag).
	if s, ap, derr := session.AttachDiscovered(ctx, ""); derr == nil && s != nil {
		return true, session.Age(s).Seconds(), "cdp:" + string(ap.Browser), nil
	}
	// Fixed-port probe (fallback).
	if s, cerr := session.AttachRunningChrome(ctx, opt.Config.ChromeDebugPort); cerr == nil {
		return true, session.Age(s).Seconds(), "cdp:fixed-port", nil
	}
	return false, 0, "", nil
}

// Login forces a fresh session capture via CDP and persists it to
// SessionPath + writes the Netscape cookies file. It tries the
// DevToolsActivePort discovery path first (chrome://inspect toggle,
// works against the real profile), then falls back to the fixed debug
// port configured in config.toml.
func Login(ctx context.Context, opt Options) error {
	s, ap, derr := session.AttachDiscovered(ctx, "")
	if derr != nil {
		s, err := session.AttachRunningChrome(ctx, opt.Config.ChromeDebugPort)
		if err != nil {
			return fmt.Errorf("attach browser on :%d (also tried DevToolsActivePort discovery: %v): %w",
				opt.Config.ChromeDebugPort, derr, err)
		}
		return persist(opt, s)
	}
	_ = ap // callers that want to log which browser was used can call AttachDiscovered directly
	return persist(opt, s)
}

func persist(opt Options, s *types.Session) error {
	if err := session.Save(s, opt.Config.SessionPath); err != nil {
		return fmt.Errorf("save session: %w", err)
	}
	if err := session.WriteNetscape(s, opt.Config.CookiesPath); err != nil {
		return fmt.Errorf("write cookies: %w", err)
	}
	return nil
}

// ImportSession loads a session.json produced by the companion
// extension (or any compatible exporter) and persists it to the
// configured session path + cookies file.
func ImportSession(opt Options, path string) error {
	s, err := session.Import(path)
	if err != nil {
		return err
	}
	if err := session.Save(s, opt.Config.SessionPath); err != nil {
		return err
	}
	return session.WriteNetscape(s, opt.Config.CookiesPath)
}

// Logout deletes the cached session and cookies files. It is
// intentionally lenient: missing files do not error.
func Logout(opt Options) error {
	for _, p := range []string{opt.Config.SessionPath, opt.Config.CookiesPath} {
		if err := removeIfExists(p); err != nil {
			return err
		}
	}
	return nil
}

// --- internals ---

func ensureSession(ctx context.Context, opt Options) (*types.Session, error) {
	s, err := session.Load(ctx, opt.Config.SessionPath, opt.Config.ChromeDebugPort)
	if err != nil {
		return nil, err
	}
	if err := session.WriteNetscape(s, opt.Config.CookiesPath); err != nil {
		return nil, err
	}
	return s, nil
}

func pickBackend(opt Options, t types.Target) (types.Backend, error) {
	switch router.Choose(t) {
	case router.BackendGalleryDL:
		return &backend.GalleryDL{
			BinPath:     opt.Config.Backend.GalleryDLPath,
			CookiesFile: opt.Config.CookiesPath,
			OutDir:      opt.OutDir,
			ArchiveDir:  opt.Config.ArchiveDir,
			Stdout:      opt.Stdout,
			Stderr:      opt.Stderr,
		}, nil
	case router.BackendYTDLP:
		return &backend.YTDLP{
			BinPath:     opt.Config.Backend.YTDLPPath,
			CookiesFile: opt.Config.CookiesPath,
			OutDir:      opt.OutDir,
			Stdout:      opt.Stdout,
			Stderr:      opt.Stderr,
		}, nil
	}
	return nil, fmt.Errorf("no backend for %v", t.Kind)
}

func resolveOutDir(opt Options, t types.Target) (string, error) {
	base := opt.OutDir
	if base == "" {
		base = opt.Config.OutDir
	}
	if t.Kind == types.TargetUserAll && t.Handle != "" {
		return filepath.Join(base, t.Handle), nil
	}
	if t.Kind == types.TargetSaved {
		return filepath.Join(base, "saved"), nil
	}
	return base, nil
}

func notify(p Progress, stage string, cur, total float64, msg string) {
	if p != nil {
		p(stage, cur, total, msg)
	}
}
