package backend

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"

	"github.com/ibrhajjaj/ig-dl/internal/types"
)

// YTDLP is a types.Backend that shells out to the `yt-dlp` command-line
// tool. It is used for single-video targets (reels, IGTV) where yt-dlp's
// format-selection is typically stronger than gallery-dl's.
type YTDLP struct {
	// BinPath is the path to the yt-dlp binary. If empty, the runner
	// uses the literal name "yt-dlp" and relies on PATH lookup.
	BinPath string
	// CookiesFile is the path to a Netscape-format cookies.txt used to
	// authenticate yt-dlp against Instagram.
	CookiesFile string
	// OutDir is the default output directory. buildArgs prefers the
	// explicit outDir argument passed to Fetch when it is non-empty.
	OutDir string
	// Stdout and Stderr receive the subprocess output streams. Either
	// may be nil to discard that stream.
	Stdout, Stderr io.Writer
}

// binary returns the executable name/path to invoke, defaulting to
// "yt-dlp" so PATH lookup happens when BinPath is unset.
func (y *YTDLP) binary() string {
	if y.BinPath == "" {
		return "yt-dlp"
	}
	return y.BinPath
}

// Fetch satisfies types.Backend. It builds a single yt-dlp argv and runs
// it, returning any *ExecError from the runner.
func (y *YTDLP) Fetch(ctx context.Context, t types.Target, s *types.Session, outDir string) error {
	argv, err := y.buildArgs(t, outDir)
	if err != nil {
		return err
	}
	if len(argv) == 0 {
		return fmt.Errorf("yt-dlp: empty argv for target kind %d", t.Kind)
	}
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	return runCmd(ctx, cmd, y.Stdout, y.Stderr)
}

// buildArgs returns the argv for a single yt-dlp invocation:
//
//	yt-dlp --cookies <cookies> -o <outDir>/%(id)s.%(ext)s <url>
//
// It supports the video-shaped targets (Reel, TV). Other kinds are the
// gallery-dl router's job and return an error here so a misroute
// surfaces loudly.
func (y *YTDLP) buildArgs(t types.Target, outDir string) ([]string, error) {
	if outDir == "" {
		outDir = y.OutDir
	}

	switch t.Kind {
	case types.TargetURLReel, types.TargetURLTV:
		if t.URL == "" {
			return nil, errors.New("yt-dlp: empty URL")
		}
		return []string{
			y.binary(),
			"--cookies", y.CookiesFile,
			"-o", outDir + "/%(id)s.%(ext)s",
			t.URL,
		}, nil
	default:
		return nil, fmt.Errorf("yt-dlp: unsupported target kind %d", t.Kind)
	}
}
