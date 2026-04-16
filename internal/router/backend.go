package router

import (
	"fmt"

	"github.com/ibrhajjaj/ig-dl/internal/types"
)

// BackendChoice names one of the supported backend runners.
type BackendChoice int

const (
	// BackendGalleryDL selects the `gallery-dl` runner. It handles posts,
	// stories, highlights, profiles, and saved bulk downloads.
	BackendGalleryDL BackendChoice = iota

	// BackendYTDLP selects the `yt-dlp` runner. It handles single video
	// URLs (reels and IGTV).
	BackendYTDLP
)

// String returns the binary name for the backend, suitable for logging and
// exec lookups.
func (b BackendChoice) String() string {
	switch b {
	case BackendGalleryDL:
		return "gallery-dl"
	case BackendYTDLP:
		return "yt-dlp"
	default:
		return fmt.Sprintf("BackendChoice(%d)", int(b))
	}
}

// Choose returns the backend that should fetch the given target. The policy
// is fixed by the design spec: reels and IGTV go to yt-dlp; everything else
// (posts, stories, highlights, profiles, saved) goes to gallery-dl.
func Choose(t types.Target) BackendChoice {
	switch t.Kind {
	case types.TargetURLReel, types.TargetURLTV:
		return BackendYTDLP
	default:
		return BackendGalleryDL
	}
}
