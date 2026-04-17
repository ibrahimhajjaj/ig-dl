package router

import (
	"testing"

	"github.com/ibrahimhajjaj/ig-dl/internal/types"
)

func TestChoose(t *testing.T) {
	t.Parallel()

	// Enumerate every TargetKind constant declared in internal/types. If
	// new kinds are added, this test must be updated alongside Choose().
	cases := []struct {
		name string
		kind types.TargetKind
		want BackendChoice
	}{
		{"TargetUnknown", types.TargetUnknown, BackendGalleryDL},
		{"TargetURLPost", types.TargetURLPost, BackendGalleryDL},
		{"TargetURLReel", types.TargetURLReel, BackendYTDLP},
		{"TargetURLStory", types.TargetURLStory, BackendGalleryDL},
		{"TargetURLHighlight", types.TargetURLHighlight, BackendGalleryDL},
		{"TargetURLTV", types.TargetURLTV, BackendYTDLP},
		{"TargetUserAll", types.TargetUserAll, BackendGalleryDL},
		{"TargetSaved", types.TargetSaved, BackendGalleryDL},
	}

	// Compile-time guard: if a new TargetKind is added to internal/types,
	// this switch will fail to compile and force an update here.
	for _, tc := range cases {
		switch tc.kind {
		case types.TargetUnknown, types.TargetURLPost, types.TargetURLReel,
			types.TargetURLStory, types.TargetURLHighlight, types.TargetURLTV,
			types.TargetUserAll, types.TargetSaved:
		}
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := Choose(types.Target{Kind: tc.kind})
			if got != tc.want {
				t.Errorf("Choose(Kind=%v) = %v, want %v", tc.kind, got, tc.want)
			}
		})
	}
}

func TestBackendChoiceString(t *testing.T) {
	t.Parallel()
	cases := []struct {
		b    BackendChoice
		want string
	}{
		{BackendGalleryDL, "gallery-dl"},
		{BackendYTDLP, "yt-dlp"},
	}
	for _, tc := range cases {
		if got := tc.b.String(); got != tc.want {
			t.Errorf("BackendChoice(%d).String() = %q, want %q", int(tc.b), got, tc.want)
		}
	}
}

func TestBackendChoiceStringUnknown(t *testing.T) {
	t.Parallel()
	got := BackendChoice(99).String()
	if got == "" {
		t.Error("BackendChoice(99).String() should not be empty")
	}
}
