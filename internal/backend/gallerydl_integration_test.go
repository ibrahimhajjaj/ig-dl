//go:build integration

package backend

import (
	"bytes"
	"context"
	"os/exec"
	"testing"
	"time"

	"github.com/ibrahimhajjaj/ig-dl/internal/types"
)

// TestGalleryDL_VersionProbe is the minimum real-subprocess integration
// test: it verifies gallery-dl is on PATH and responds to --version.
// Full extractor tests against a fixture server are a follow-up —
// this skeleton proves the exec plumbing works.
//
// Run with: go test -tags=integration ./internal/backend/...
func TestGalleryDL_VersionProbe(t *testing.T) {
	if _, err := exec.LookPath("gallery-dl"); err != nil {
		t.Skip("gallery-dl not on PATH")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var out bytes.Buffer
	cmd := exec.CommandContext(ctx, "gallery-dl", "--version")
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		t.Fatalf("gallery-dl --version failed: %v", err)
	}
	if out.Len() == 0 {
		t.Fatal("gallery-dl --version produced no output")
	}
}

// TestGalleryDL_Fetch_BadURL exercises the real subprocess path with a
// URL that will reliably 404, asserting we surface a classified error
// rather than panicking.
func TestGalleryDL_Fetch_BadURL(t *testing.T) {
	if _, err := exec.LookPath("gallery-dl"); err != nil {
		t.Skip("gallery-dl not on PATH")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	gd := &GalleryDL{
		OutDir: t.TempDir(),
	}
	target := types.Target{
		Kind: types.TargetURLPost,
		URL:  "https://www.instagram.com/p/nonexistent_XXXXXX_definitely_not_a_post/",
	}
	err := gd.Fetch(ctx, target, nil, gd.OutDir)
	// Don't care which error — just that we get one and the process
	// terminated cleanly.
	if err == nil {
		t.Fatal("expected an error for a bogus URL; got nil")
	}
}
