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
	"sync"
	"time"

	"github.com/ibrahimhajjaj/ig-dl/internal/backend"
	"github.com/ibrahimhajjaj/ig-dl/internal/config"
	"github.com/ibrahimhajjaj/ig-dl/internal/router"
	"github.com/ibrahimhajjaj/ig-dl/internal/session"
	"github.com/ibrahimhajjaj/ig-dl/internal/types"
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
	if err := fetchWithResilience(ctx, b, target, sess, outDir, opt); err != nil {
		return nil, fmt.Errorf("%s: %w", router.Choose(target), err)
	}
	notify(opt.Progress, "download", 1, 1, "done")

	return &Result{
		OutDir: outDir,
		Counts: map[string]int{"invocations": 1},
		Meta:   map[string]string{"url": url, "backend": router.Choose(target).String()},
	}, nil
}

// DownloadUser runs the profile-bulk flow. Per the spec it splits a
// profile handle into per-kind stages (posts, stories, highlights) and
// executes them through a bounded worker pool (Concurrency from config)
// so the three gallery-dl invocations can overlap. Each stage writes
// into its own output subdir (<outDir>/posts, /stories, /highlights).
//
// `include` optionally restricts which stages run. Empty = run all.
// Valid stage names: posts, stories, highlights.
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
	_ = sess // backend auth comes via the cookies file we just wrote

	gd := &backend.GalleryDL{
		BinPath:     opt.Config.Backend.GalleryDLPath,
		CookiesFile: opt.Config.CookiesPath,
		OutDir:      outDir,
		ArchiveDir:  opt.Config.ArchiveDir,
		Stdout:      opt.Stdout,
		Stderr:      opt.Stderr,
	}

	stages := profileStages(target.Handle, outDir, include)
	counts := map[string]int{}
	var failures []string
	var mu sync.Mutex

	concurrency := opt.Config.Concurrency
	if concurrency < 1 {
		concurrency = 1
	}
	if concurrency > len(stages) {
		concurrency = len(stages)
	}

	notify(opt.Progress, "profile", 0, float64(len(stages)),
		fmt.Sprintf("running %d stage(s) for %s", len(stages), target.Handle))

	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup
	for _, st := range stages {
		st := st
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			if err := runStage(ctx, gd, st, opt, target.Handle); err != nil {
				mu.Lock()
				failures = append(failures, fmt.Sprintf("%s: %v", st.Kind, err))
				mu.Unlock()
			} else {
				mu.Lock()
				counts[st.Kind]++
				mu.Unlock()
			}
			mu.Lock()
			done := 0
			for _, n := range counts {
				done += n
			}
			done += len(failures)
			mu.Unlock()
			notify(opt.Progress, "profile", float64(done), float64(len(stages)), st.Kind+" finished")
		}()
	}
	wg.Wait()

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

type userStage struct {
	Kind   string
	URL    string
	OutDir string
	Extra  []string
}

// profileStages expands a handle into per-kind download stages that can
// run in parallel. Each stage has its own output subdirectory so the
// spec-specified layout is preserved.
func profileStages(handle, baseOut string, include []string) []userStage {
	all := []userStage{
		{
			Kind:   "posts",
			URL:    fmt.Sprintf("https://www.instagram.com/%s/", handle),
			OutDir: filepath.Join(baseOut, "posts"),
		},
		{
			Kind:   "stories",
			URL:    fmt.Sprintf("https://www.instagram.com/stories/%s/", handle),
			OutDir: filepath.Join(baseOut, "stories"),
		},
		{
			Kind:   "highlights",
			URL:    fmt.Sprintf("https://www.instagram.com/%s/", handle),
			OutDir: filepath.Join(baseOut, "highlights"),
			Extra:  []string{"-o", "instagram.highlights=true"},
		},
	}
	if len(include) == 0 {
		return all
	}
	want := make(map[string]struct{}, len(include))
	for _, k := range include {
		want[k] = struct{}{}
	}
	out := make([]userStage, 0, len(all))
	for _, s := range all {
		if _, ok := want[s.Kind]; ok {
			out = append(out, s)
		}
	}
	return out
}

// runStage wraps a single gallery-dl invocation with the full resilience
// policy (auth refresh-retry, rate-limit backoff). It mirrors
// fetchWithResilience but works directly with gallery-dl.RunURL rather
// than the types.Backend interface, because per-stage invocations need
// their own per-stage output dirs and --download-archive args.
func runStage(ctx context.Context, gd *backend.GalleryDL, st userStage, opt Options, handle string) error {
	extra := append([]string{}, gd.ArchiveArg(handle)...)
	extra = append(extra, st.Extra...)

	const maxRateAttempts = 3
	backoff := time.Second
	for attempt := 1; attempt <= maxRateAttempts; attempt++ {
		err := gd.RunURL(ctx, st.URL, st.OutDir, extra...)
		if err == nil {
			return nil
		}
		switch Classify(err) {
		case ErrCategoryAuthFailed:
			if attempt == 1 {
				if fresh, _, derr := session.AttachDiscovered(ctx, ""); derr == nil && fresh != nil {
					_ = session.Save(fresh, opt.Config.SessionPath)
					_ = session.WriteNetscape(fresh, opt.Config.CookiesPath)
					continue
				}
			}
			return err
		case ErrCategoryRateLimited:
			if attempt >= maxRateAttempts {
				return err
			}
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(backoff):
			}
			backoff *= 2
			continue
		default:
			return err
		}
	}
	return nil
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
	if err := fetchWithResilience(ctx, b, target, sess, outDir, opt); err != nil {
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
// port configured in config.toml. Returns a human-readable source
// label naming which browser actually produced the session.
func Login(ctx context.Context, opt Options) (source string, err error) {
	s, ap, derr := session.AttachDiscovered(ctx, "")
	if derr != nil {
		s2, ferr := session.AttachRunningChrome(ctx, opt.Config.ChromeDebugPort)
		if ferr != nil {
			return "", fmt.Errorf("attach browser on :%d (also tried DevToolsActivePort discovery: %v): %w",
				opt.Config.ChromeDebugPort, derr, ferr)
		}
		if perr := persist(opt, s2); perr != nil {
			return "", perr
		}
		return fmt.Sprintf("fixed-port:%d", opt.Config.ChromeDebugPort), nil
	}
	if err := persist(opt, s); err != nil {
		return "", err
	}
	return string(ap.Browser), nil
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
	// Age-based refresh per the spec: if the session is older than
	// StaleAfter AND a live browser with CDP is reachable, silently
	// capture a fresh one before dispatch. If no browser is reachable
	// we proceed with the existing session; downstream auth errors are
	// handled by the retry path.
	age := session.Age(s)
	if opt.Config.StaleAfter > 0 && age > opt.Config.StaleAfter {
		if fresh, _, derr := session.AttachDiscovered(ctx, ""); derr == nil && fresh != nil {
			if err := session.Save(fresh, opt.Config.SessionPath); err == nil {
				s = fresh
			}
		}
	}
	// Warn (to stderr) on very old sessions regardless of refresh outcome.
	if opt.Config.WarnAfter > 0 && session.Age(s) > opt.Config.WarnAfter && opt.Stderr != nil {
		fmt.Fprintf(opt.Stderr, "warning: session is %s old; consider running `ig-dl login`\n", session.Age(s).Truncate(1e9))
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

// fetchWithResilience wraps a Backend.Fetch with the retry policy the
// spec mandates: one auth refresh-and-retry on auth_failed, and
// exponential backoff on rate_limit up to 3 attempts.
func fetchWithResilience(ctx context.Context, b types.Backend, t types.Target, sess *types.Session, outDir string, opt Options) error {
	const maxRateAttempts = 3
	var lastErr error
	backoff := time.Second

	for attempt := 1; attempt <= maxRateAttempts; attempt++ {
		err := b.Fetch(ctx, t, sess, outDir)
		if err == nil {
			return nil
		}
		lastErr = err

		cat := Classify(err)
		switch cat {
		case ErrCategoryAuthFailed:
			// One refresh-and-retry per the spec.
			if attempt == 1 {
				notify(opt.Progress, "session", 0, 1, "auth failed; refreshing session")
				if fresh, _, derr := session.AttachDiscovered(ctx, ""); derr == nil && fresh != nil {
					_ = session.Save(fresh, opt.Config.SessionPath)
					_ = session.WriteNetscape(fresh, opt.Config.CookiesPath)
					sess = fresh
					notify(opt.Progress, "session", 1, 1, "session refreshed")
					continue
				}
			}
			return err
		case ErrCategoryRateLimited:
			if attempt >= maxRateAttempts {
				return err
			}
			notify(opt.Progress, "backoff", float64(attempt), float64(maxRateAttempts),
				fmt.Sprintf("rate limited; sleeping %s", backoff))
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(backoff):
			}
			backoff *= 2
			continue
		default:
			return err
		}
	}
	return lastErr
}
