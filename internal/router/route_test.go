package router

import (
	"errors"
	"testing"

	"github.com/ibrahimhajjaj/ig-dl/internal/types"
)

func TestParse(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name       string
		input      string
		wantKind   types.TargetKind
		wantHandle string
		wantErr    bool
	}{
		// Posts
		{"post https www", "https://www.instagram.com/p/ABC123_-/", types.TargetURLPost, "", false},
		{"post https bare", "https://instagram.com/p/ABC123/", types.TargetURLPost, "", false},
		{"post http www", "http://www.instagram.com/p/ABC123/", types.TargetURLPost, "", false},
		{"post m subdomain", "https://m.instagram.com/p/ABC123/", types.TargetURLPost, "", false},
		{"post no trailing slash", "https://www.instagram.com/p/ABC123", types.TargetURLPost, "", false},
		{"post with query", "https://www.instagram.com/p/ABC123/?igsh=xyz", types.TargetURLPost, "", false},
		{"post with fragment", "https://www.instagram.com/p/ABC123/#foo", types.TargetURLPost, "", false},
		{"post with query and fragment", "https://www.instagram.com/p/ABC123/?a=b#frag", types.TargetURLPost, "", false},

		// Reels
		{"reel singular", "https://www.instagram.com/reel/XYZ_999/", types.TargetURLReel, "", false},
		{"reel plural", "https://www.instagram.com/reels/XYZ_999/", types.TargetURLReel, "", false},
		{"reel http m no slash", "http://m.instagram.com/reel/XYZ_999", types.TargetURLReel, "", false},
		{"reel with query", "https://instagram.com/reel/XYZ/?utm_source=foo", types.TargetURLReel, "", false},

		// TV
		{"tv https www", "https://www.instagram.com/tv/TVID01/", types.TargetURLTV, "", false},
		{"tv m no slash", "https://m.instagram.com/tv/TVID01", types.TargetURLTV, "", false},

		// Stories
		{"story handle and id", "https://www.instagram.com/stories/someuser/123456789/", types.TargetURLStory, "someuser", false},
		{"story handle only", "https://www.instagram.com/stories/someuser/", types.TargetURLStory, "someuser", false},
		{"story uppercase handle lowercased", "https://www.instagram.com/stories/SomeUser/123/", types.TargetURLStory, "someuser", false},

		// Highlights
		{"highlight", "https://www.instagram.com/stories/highlights/17890/", types.TargetURLHighlight, "", false},
		{"highlight m no slash", "https://m.instagram.com/stories/highlights/17890", types.TargetURLHighlight, "", false},

		// Profiles
		{"profile simple", "https://www.instagram.com/natgeo/", types.TargetUserAll, "natgeo", false},
		{"profile no trailing slash", "https://www.instagram.com/natgeo", types.TargetUserAll, "natgeo", false},
		{"profile with dots", "https://www.instagram.com/nat.geo/", types.TargetUserAll, "nat.geo", false},
		{"profile with underscores", "https://www.instagram.com/nat_geo_official/", types.TargetUserAll, "nat_geo_official", false},
		{"profile with digits", "https://www.instagram.com/user123/", types.TargetUserAll, "user123", false},
		{"profile uppercase lowercased", "https://www.instagram.com/NatGeo/", types.TargetUserAll, "natgeo", false},
		{"profile bare host", "https://instagram.com/natgeo/", types.TargetUserAll, "natgeo", false},
		{"profile m subdomain", "https://m.instagram.com/natgeo/", types.TargetUserAll, "natgeo", false},
		{"profile with query", "https://www.instagram.com/natgeo/?hl=en", types.TargetUserAll, "natgeo", false},

		// Saved
		{"saved", "https://www.instagram.com/natgeo/saved/", types.TargetSaved, "natgeo", false},
		{"saved no trailing slash", "https://www.instagram.com/natgeo/saved", types.TargetSaved, "natgeo", false},

		// Bare handles
		{"bare handle", "natgeo", types.TargetUserAll, "natgeo", false},
		{"bare handle at prefix", "@natgeo", types.TargetUserAll, "natgeo", false},
		{"bare handle uppercase", "NatGeo", types.TargetUserAll, "natgeo", false},
		{"bare handle with dots and underscores", "nat.geo_official", types.TargetUserAll, "nat.geo_official", false},
		{"bare handle with digits", "user123", types.TargetUserAll, "user123", false},
		{"bare handle with whitespace trimmed", "  natgeo  ", types.TargetUserAll, "natgeo", false},

		// Errors
		{"empty", "", 0, "", true},
		{"whitespace only", "   ", 0, "", true},
		{"bare handle invalid chars", "nat geo", 0, "", true},
		{"bare handle with hyphen", "nat-geo", 0, "", true},
		{"at sign alone", "@", 0, "", true},
		{"unknown host", "https://example.com/p/ABC/", 0, "", true},
		{"ftp scheme", "ftp://www.instagram.com/p/ABC/", 0, "", true},
		{"missing scheme with slashes", "www.instagram.com/p/ABC/", 0, "", true},
		{"unknown path explore", "https://www.instagram.com/explore/tags/nature/", 0, "", true},
		{"malformed reel no id", "https://www.instagram.com/reel/", 0, "", true},
		{"malformed post no id", "https://www.instagram.com/p/", 0, "", true},
		{"malformed tv no id", "https://www.instagram.com/tv/", 0, "", true},
		{"malformed stories empty", "https://www.instagram.com/stories/", 0, "", true},
		{"malformed highlight no id", "https://www.instagram.com/stories/highlights/", 0, "", true},
		{"profile deep path", "https://www.instagram.com/natgeo/tagged/", 0, "", true},
		{"garbage url", "https://:::bad::url::", 0, "", true},
		{"only slashes", "///", 0, "", true},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := Parse(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("Parse(%q) expected error, got %+v", tc.input, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("Parse(%q) unexpected error: %v", tc.input, err)
			}
			if got.Kind != tc.wantKind {
				t.Errorf("Parse(%q) Kind = %v, want %v", tc.input, got.Kind, tc.wantKind)
			}
			if got.Handle != tc.wantHandle {
				t.Errorf("Parse(%q) Handle = %q, want %q", tc.input, got.Handle, tc.wantHandle)
			}
		})
	}
}

func TestParse_EmptyInputErrSentinel(t *testing.T) {
	t.Parallel()
	_, err := Parse("")
	if !errors.Is(err, ErrEmptyInput) {
		t.Fatalf("Parse(\"\") err = %v, want ErrEmptyInput", err)
	}
}

func TestParse_UnknownTargetErrSentinel(t *testing.T) {
	t.Parallel()
	_, err := Parse("https://example.com/foo/")
	if !errors.Is(err, ErrUnknownTarget) {
		t.Fatalf("Parse unknown host err = %v, want ErrUnknownTarget", err)
	}
}

func TestParse_StripsQueryAndFragmentFromCanonicalURL(t *testing.T) {
	t.Parallel()
	got, err := Parse("https://www.instagram.com/p/ABC123/?igsh=abc#section")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "https://www.instagram.com/p/ABC123/"
	if got.URL != want {
		t.Errorf("URL = %q, want %q", got.URL, want)
	}
}

func TestSavedTarget(t *testing.T) {
	t.Parallel()
	got := SavedTarget()
	if got.Kind != types.TargetSaved {
		t.Errorf("SavedTarget().Kind = %v, want TargetSaved", got.Kind)
	}
	if got.URL != "" || got.Handle != "" {
		t.Errorf("SavedTarget() should have empty URL and Handle, got %+v", got)
	}
}
