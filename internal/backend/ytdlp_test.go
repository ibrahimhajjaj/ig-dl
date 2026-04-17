package backend

import (
	"reflect"
	"testing"

	"github.com/ibrahimhajjaj/ig-dl/internal/types"
)

func TestYTDLP_buildArgs(t *testing.T) {
	const (
		cookies = "/tmp/cookies.txt"
		outDir  = "/tmp/out"
	)

	tests := []struct {
		name    string
		y       *YTDLP
		target  types.Target
		want    []string
		wantErr bool
	}{
		{
			name: "Reel",
			y:    &YTDLP{CookiesFile: cookies},
			target: types.Target{
				Kind: types.TargetURLReel,
				URL:  "https://www.instagram.com/reel/XYZ/",
			},
			want: []string{
				"yt-dlp",
				"--cookies", cookies,
				"-o", "/tmp/out/%(id)s.%(ext)s",
				"https://www.instagram.com/reel/XYZ/",
			},
		},
		{
			name: "TV",
			y:    &YTDLP{CookiesFile: cookies},
			target: types.Target{
				Kind: types.TargetURLTV,
				URL:  "https://www.instagram.com/tv/ABC/",
			},
			want: []string{
				"yt-dlp",
				"--cookies", cookies,
				"-o", "/tmp/out/%(id)s.%(ext)s",
				"https://www.instagram.com/tv/ABC/",
			},
		},
		{
			name:   "Post_rejected",
			y:      &YTDLP{CookiesFile: cookies},
			target: types.Target{Kind: types.TargetURLPost, URL: "https://www.instagram.com/p/ABC/"},
			wantErr: true,
		},
		{
			name:    "Reel_empty_url_errors",
			y:       &YTDLP{CookiesFile: cookies},
			target:  types.Target{Kind: types.TargetURLReel},
			wantErr: true,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			got, err := tc.y.buildArgs(tc.target, outDir)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got argv %v", got)
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

func TestYTDLP_buildArgs_DefaultBinary(t *testing.T) {
	y := &YTDLP{CookiesFile: "/c.txt"}
	got, err := y.buildArgs(types.Target{Kind: types.TargetURLReel, URL: "u"}, "/o")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got[0] != "yt-dlp" {
		t.Fatalf("default binary = %q, want yt-dlp", got[0])
	}
}

func TestYTDLP_buildArgs_CustomBinary(t *testing.T) {
	y := &YTDLP{BinPath: "/opt/yt-dlp", CookiesFile: "/c.txt"}
	got, err := y.buildArgs(types.Target{Kind: types.TargetURLReel, URL: "u"}, "/o")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got[0] != "/opt/yt-dlp" {
		t.Fatalf("binary = %q, want /opt/yt-dlp", got[0])
	}
}

func TestYTDLP_buildArgs_DefaultOutDirFallback(t *testing.T) {
	y := &YTDLP{CookiesFile: "/c.txt", OutDir: "/configured"}
	got, err := y.buildArgs(types.Target{Kind: types.TargetURLReel, URL: "u"}, "")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	// 4th positional arg is the -o template; check it starts with configured outDir.
	var outTmpl string
	for i, a := range got {
		if a == "-o" && i+1 < len(got) {
			outTmpl = got[i+1]
			break
		}
	}
	if outTmpl != "/configured/%(id)s.%(ext)s" {
		t.Fatalf("outDir fallback: -o = %q, want /configured/%%(id)s.%%(ext)s", outTmpl)
	}
}
