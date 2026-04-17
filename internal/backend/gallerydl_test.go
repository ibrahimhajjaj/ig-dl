package backend

import (
	"reflect"
	"strings"
	"testing"

	"github.com/ibrahimhajjaj/ig-dl/internal/types"
)

func TestGalleryDL_buildArgs(t *testing.T) {
	const (
		cookies = "/tmp/cookies.txt"
		outDir  = "/tmp/out"
	)

	tests := []struct {
		name    string
		g       *GalleryDL
		target  types.Target
		want    [][]string
		wantErr bool
	}{
		{
			name: "URLPost",
			g:    &GalleryDL{CookiesFile: cookies},
			target: types.Target{
				Kind: types.TargetURLPost,
				URL:  "https://www.instagram.com/p/ABC/",
			},
			want: [][]string{
				{"gallery-dl", "--cookies", cookies, "-d", outDir, "https://www.instagram.com/p/ABC/"},
			},
		},
		{
			name: "URLStory",
			g:    &GalleryDL{CookiesFile: cookies},
			target: types.Target{
				Kind: types.TargetURLStory,
				URL:  "https://www.instagram.com/stories/alice/123/",
			},
			want: [][]string{
				{"gallery-dl", "--cookies", cookies, "-d", outDir, "https://www.instagram.com/stories/alice/123/"},
			},
		},
		{
			name: "URLHighlight",
			g:    &GalleryDL{CookiesFile: cookies},
			target: types.Target{
				Kind: types.TargetURLHighlight,
				URL:  "https://www.instagram.com/stories/highlights/987/",
			},
			want: [][]string{
				{"gallery-dl", "--cookies", cookies, "-d", outDir, "https://www.instagram.com/stories/highlights/987/"},
			},
		},
		{
			name:   "UserAll_no_archive_three_invocations",
			g:      &GalleryDL{CookiesFile: cookies},
			target: types.Target{Kind: types.TargetUserAll, Handle: "alice"},
			want: [][]string{
				{"gallery-dl", "--cookies", cookies, "-d", outDir, "https://www.instagram.com/alice/"},
				{"gallery-dl", "--cookies", cookies, "-d", outDir, "https://www.instagram.com/stories/alice/"},
				{"gallery-dl", "--cookies", cookies, "-d", outDir, "-o", "instagram.highlights=true", "https://www.instagram.com/alice/"},
			},
		},
		{
			name:   "UserAll_with_archive",
			g:      &GalleryDL{CookiesFile: cookies, ArchiveDir: "/var/archive"},
			target: types.Target{Kind: types.TargetUserAll, Handle: "alice"},
			want: [][]string{
				{"gallery-dl", "--cookies", cookies, "-d", outDir, "--download-archive", "/var/archive/alice.sqlite", "https://www.instagram.com/alice/"},
				{"gallery-dl", "--cookies", cookies, "-d", outDir, "--download-archive", "/var/archive/alice.sqlite", "https://www.instagram.com/stories/alice/"},
				{"gallery-dl", "--cookies", cookies, "-d", outDir, "--download-archive", "/var/archive/alice.sqlite", "-o", "instagram.highlights=true", "https://www.instagram.com/alice/"},
			},
		},
		{
			name:   "Saved_with_handle",
			g:      &GalleryDL{CookiesFile: cookies},
			target: types.Target{Kind: types.TargetSaved, Handle: "alice"},
			want: [][]string{
				{"gallery-dl", "--cookies", cookies, "-d", outDir, "https://www.instagram.com/alice/saved/"},
			},
		},
		{
			name:    "Saved_without_handle_or_url_errors",
			g:       &GalleryDL{CookiesFile: cookies},
			target:  types.Target{Kind: types.TargetSaved},
			wantErr: true,
		},
		{
			name:    "UserAll_without_handle_errors",
			g:       &GalleryDL{CookiesFile: cookies},
			target:  types.Target{Kind: types.TargetUserAll},
			wantErr: true,
		},
		{
			name:    "Reel_routed_elsewhere_errors",
			g:       &GalleryDL{CookiesFile: cookies},
			target:  types.Target{Kind: types.TargetURLReel, URL: "https://www.instagram.com/reel/XYZ/"},
			wantErr: true,
		},
		{
			name:    "TV_routed_elsewhere_errors",
			g:       &GalleryDL{CookiesFile: cookies},
			target:  types.Target{Kind: types.TargetURLTV, URL: "https://www.instagram.com/tv/XYZ/"},
			wantErr: true,
		},
		{
			name:    "Post_empty_url_errors",
			g:       &GalleryDL{CookiesFile: cookies},
			target:  types.Target{Kind: types.TargetURLPost},
			wantErr: true,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			got, err := tc.g.buildArgs(tc.target, outDir)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got argvs %v", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("argv mismatch\nwant: %v\ngot:  %v", tc.want, got)
			}
		})
	}
}

func TestGalleryDL_buildArgs_DefaultBinary(t *testing.T) {
	g := &GalleryDL{CookiesFile: "/c.txt"}
	got, err := g.buildArgs(types.Target{Kind: types.TargetURLPost, URL: "u"}, "/out")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got[0][0] != "gallery-dl" {
		t.Fatalf("default binary = %q, want gallery-dl", got[0][0])
	}
}

func TestGalleryDL_buildArgs_CustomBinary(t *testing.T) {
	g := &GalleryDL{BinPath: "/opt/bin/gallery-dl", CookiesFile: "/c.txt"}
	got, err := g.buildArgs(types.Target{Kind: types.TargetURLPost, URL: "u"}, "/out")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got[0][0] != "/opt/bin/gallery-dl" {
		t.Fatalf("binary = %q, want /opt/bin/gallery-dl", got[0][0])
	}
}

func TestGalleryDL_buildArgs_DefaultOutDirFallback(t *testing.T) {
	g := &GalleryDL{CookiesFile: "/c.txt", OutDir: "/configured"}
	got, err := g.buildArgs(types.Target{Kind: types.TargetURLPost, URL: "u"}, "")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	joined := strings.Join(got[0], " ")
	if !strings.Contains(joined, "/configured") {
		t.Fatalf("expected fallback outDir in argv; got %q", joined)
	}
}
