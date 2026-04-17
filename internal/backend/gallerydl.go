package backend

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"

	"github.com/ibrahimhajjaj/ig-dl/internal/types"
)

// GalleryDL is a types.Backend that shells out to the `gallery-dl`
// command-line tool. It handles single-URL targets (posts, stories,
// highlights, saved) and profile-wide bulk operations (TargetUserAll,
// which fans out into multiple gallery-dl invocations).
type GalleryDL struct {
	// BinPath is the path to the gallery-dl binary. If empty, the
	// runner uses the literal name "gallery-dl" and relies on PATH
	// lookup.
	BinPath string
	// CookiesFile is the path to a Netscape-format cookies.txt that
	// authenticates gallery-dl as the logged-in user.
	CookiesFile string
	// OutDir is the default output directory. buildArgs prefers the
	// explicit outDir argument passed to Fetch when it is non-empty.
	OutDir string
	// ArchiveDir, if non-empty, enables per-handle
	// --download-archive usage for TargetUserAll. The archive file is
	// ArchiveDir/<handle>.sqlite so re-runs skip already-downloaded
	// items natively.
	ArchiveDir string
	// Stdout and Stderr receive the subprocess output streams. Either
	// may be nil to discard that stream.
	Stdout, Stderr io.Writer
}

// binary returns the executable name/path to invoke, defaulting to
// "gallery-dl" so PATH lookup happens when BinPath is unset.
func (g *GalleryDL) binary() string {
	if g.BinPath == "" {
		return "gallery-dl"
	}
	return g.BinPath
}

// RunURL is the primitive single-invocation runner used by callers that
// want to schedule per-stage invocations themselves (e.g. the core
// orchestrator's worker pool for profile bulk). It skips buildArgs and
// runs exactly one gallery-dl subprocess for the given URL.
//
// The argv is: gallery-dl --cookies <c> -d <outDir> [archive] [extra...] <url>
func (g *GalleryDL) RunURL(ctx context.Context, url, outDir string, extra ...string) error {
	if url == "" {
		return fmt.Errorf("gallery-dl: empty URL")
	}
	argv := []string{g.binary(), "--cookies", g.CookiesFile, "-d", outDir}
	argv = append(argv, extra...)
	argv = append(argv, url)
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	return runCmd(ctx, cmd, g.Stdout, g.Stderr)
}

// ArchiveArg returns the --download-archive argument pair for a
// profile handle, or nil if ArchiveDir is unset. Used by the
// orchestrator to opt each stage into resume behaviour.
func (g *GalleryDL) ArchiveArg(handle string) []string {
	if g.ArchiveDir == "" || handle == "" {
		return nil
	}
	return []string{"--download-archive", filepath.Join(g.ArchiveDir, handle+".sqlite")}
}

// Fetch satisfies types.Backend. It expands the Target into one or more
// gallery-dl invocations via buildArgs and runs them sequentially,
// stopping at the first failure and returning its *ExecError.
func (g *GalleryDL) Fetch(ctx context.Context, t types.Target, s *types.Session, outDir string) error {
	argvs, err := g.buildArgs(t, outDir)
	if err != nil {
		return err
	}
	if len(argvs) == 0 {
		return fmt.Errorf("gallery-dl: no invocations for target kind %d", t.Kind)
	}
	for _, argv := range argvs {
		if len(argv) == 0 {
			continue
		}
		cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
		if err := runCmd(ctx, cmd, g.Stdout, g.Stderr); err != nil {
			return err
		}
	}
	return nil
}

// buildArgs expands a Target into one or more argv slices.
//
// For single-URL targets (Post, Story, Highlight, Saved) the result is
// exactly one argv of the shape:
//
//	gallery-dl --cookies <cookies> -d <outDir> <url>
//
// For TargetUserAll it returns three argvs so gallery-dl runs once per
// content category. Rationale: gallery-dl's instagram extractor covers
// posts+reels via the profile URL, stories via the /stories/<handle>/
// URL, and highlights via the profile page with the
// "instagram.highlights=true" extractor option. At this layer the tests
// only assert argv shape; the exact highlights URL/option is expected
// to be tuned during the E2E smoke pass against a real account, so we
// keep the three slots explicit and well-labelled.
//
// If ArchiveDir is set and the target is TargetUserAll, each expanded
// invocation gets `--download-archive <ArchiveDir>/<handle>.sqlite`.
//
// Reel and TV targets are not gallery-dl's job (router sends them to
// yt-dlp); buildArgs returns an error for them so a misrouted call
// surfaces loudly instead of silently producing a wrong argv.
func (g *GalleryDL) buildArgs(t types.Target, outDir string) ([][]string, error) {
	if outDir == "" {
		outDir = g.OutDir
	}

	base := []string{g.binary(), "--cookies", g.CookiesFile, "-d", outDir}

	switch t.Kind {
	case types.TargetURLPost, types.TargetURLStory, types.TargetURLHighlight:
		if t.URL == "" {
			return nil, fmt.Errorf("gallery-dl: empty URL for target kind %d", t.Kind)
		}
		argv := append([]string{}, base...)
		argv = append(argv, t.URL)
		return [][]string{argv}, nil

	case types.TargetSaved:
		var url string
		switch {
		case t.URL != "":
			url = t.URL
		case t.Handle != "":
			url = "https://www.instagram.com/" + t.Handle + "/saved/"
		default:
			return nil, errors.New("gallery-dl: TargetSaved requires Handle or URL")
		}
		argv := append([]string{}, base...)
		argv = append(argv, url)
		return [][]string{argv}, nil

	case types.TargetUserAll:
		if t.Handle == "" {
			return nil, errors.New("gallery-dl: TargetUserAll requires Handle")
		}
		profileURL := "https://www.instagram.com/" + t.Handle + "/"
		storiesURL := "https://www.instagram.com/stories/" + t.Handle + "/"
		// Highlights: gallery-dl's instagram extractor picks up
		// highlights via the profile URL plus the
		// `instagram.highlights=true` extractor option. We pass the
		// profile URL a second time with `-o instagram.highlights=true`
		// so this invocation is clearly the highlights slot. Exact
		// option name may be tuned during E2E smoke.
		highlightsArgs := []string{
			"-o", "instagram.highlights=true",
			profileURL,
		}

		archiveFor := func() []string {
			if g.ArchiveDir == "" {
				return nil
			}
			path := filepath.Join(g.ArchiveDir, t.Handle+".sqlite")
			return []string{"--download-archive", path}
		}

		build := func(tail ...string) []string {
			argv := append([]string{}, base...)
			if extra := archiveFor(); extra != nil {
				argv = append(argv, extra...)
			}
			argv = append(argv, tail...)
			return argv
		}

		return [][]string{
			build(profileURL),
			build(storiesURL),
			build(highlightsArgs...),
		}, nil

	case types.TargetURLReel, types.TargetURLTV:
		return nil, fmt.Errorf("gallery-dl: target kind %d is routed to yt-dlp", t.Kind)

	default:
		return nil, fmt.Errorf("gallery-dl: unsupported target kind %d", t.Kind)
	}
}
