// Package router translates user-supplied Instagram URLs and handles into
// normalised Target values and selects the backend (gallery-dl or yt-dlp)
// that should fetch each target.
package router

import (
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/ibrhajjaj/ig-dl/internal/types"
)

// ErrEmptyInput is returned when Parse is given an empty or whitespace-only
// input string.
var ErrEmptyInput = errors.New("router: empty input")

// ErrUnknownTarget is returned when Parse cannot map the input to any
// supported Instagram target shape.
var ErrUnknownTarget = errors.New("router: unrecognised Instagram target")

// handleRegexp matches an Instagram handle: alphanumerics, dots, and
// underscores. Handles must be 1-30 characters; Instagram caps at 30 but we
// accept anything non-empty up to a generous bound to keep the parser
// forgiving.
var handleRegexp = regexp.MustCompile(`^[A-Za-z0-9._]{1,30}$`)

// shortcodeRegexp matches the shortcode segment used by posts, reels, tv,
// and stories highlight IDs. Instagram shortcodes are Base64-ish
// (alphanumerics + `-` + `_`).
var shortcodeRegexp = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

// acceptedHosts is the set of Instagram hostnames Parse recognises.
var acceptedHosts = map[string]struct{}{
	"instagram.com":   {},
	"www.instagram.com": {},
	"m.instagram.com":   {},
}

// reservedHandles are URL path segments under instagram.com/ that are not
// user profiles. They route to concrete sub-pages instead.
var reservedHandles = map[string]struct{}{
	"p":          {},
	"reel":       {},
	"reels":      {},
	"tv":         {},
	"stories":    {},
	"explore":    {},
	"accounts":   {},
	"direct":     {},
	"about":      {},
	"developer":  {},
	"legal":      {},
	"privacy":    {},
	"terms":      {},
	"help":       {},
	"press":      {},
	"api":        {},
	"web":        {},
	"session":    {},
	"challenge":  {},
	"emails":     {},
	"graphql":    {},
	"ajax":       {},
}

// Parse accepts either a full Instagram URL or a bare handle and returns the
// corresponding Target. Query strings and fragments are stripped before the
// path is matched. Accepted hosts are instagram.com, www.instagram.com, and
// m.instagram.com over http or https.
func Parse(input string) (types.Target, error) {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return types.Target{}, ErrEmptyInput
	}

	// Bare handle: either "@name" or a plain handle with no URL scheme and
	// no slashes.
	if strings.HasPrefix(trimmed, "@") {
		handle := strings.TrimPrefix(trimmed, "@")
		if !handleRegexp.MatchString(handle) {
			return types.Target{}, fmt.Errorf("%w: invalid handle %q", ErrUnknownTarget, input)
		}
		return types.Target{Kind: types.TargetUserAll, Handle: strings.ToLower(handle)}, nil
	}

	if !strings.Contains(trimmed, "/") && !strings.Contains(trimmed, ":") {
		if !handleRegexp.MatchString(trimmed) {
			return types.Target{}, fmt.Errorf("%w: invalid handle %q", ErrUnknownTarget, input)
		}
		return types.Target{Kind: types.TargetUserAll, Handle: strings.ToLower(trimmed)}, nil
	}

	// Otherwise treat as URL. url.Parse is tolerant, so also require a
	// scheme and host up front.
	if !strings.HasPrefix(trimmed, "http://") && !strings.HasPrefix(trimmed, "https://") {
		return types.Target{}, fmt.Errorf("%w: missing http(s) scheme in %q", ErrUnknownTarget, input)
	}

	u, err := url.Parse(trimmed)
	if err != nil {
		return types.Target{}, fmt.Errorf("%w: %v", ErrUnknownTarget, err)
	}

	host := strings.ToLower(u.Host)
	if _, ok := acceptedHosts[host]; !ok {
		return types.Target{}, fmt.Errorf("%w: unsupported host %q", ErrUnknownTarget, host)
	}

	// Strip query and fragment by rebuilding the canonical URL from scheme,
	// host, and path only. This is the URL we attach to the Target so
	// downstream backends get a deterministic value.
	path := strings.Trim(u.Path, "/")
	segments := []string{}
	if path != "" {
		segments = strings.Split(path, "/")
	}

	canonicalURL := (&url.URL{
		Scheme: u.Scheme,
		Host:   u.Host,
		Path:   u.Path,
	}).String()

	if len(segments) == 0 {
		return types.Target{}, fmt.Errorf("%w: empty path in %q", ErrUnknownTarget, input)
	}

	head := segments[0]

	switch head {
	case "p":
		// /p/<shortcode>/
		if len(segments) < 2 || !shortcodeRegexp.MatchString(segments[1]) {
			return types.Target{}, fmt.Errorf("%w: malformed post URL %q", ErrUnknownTarget, input)
		}
		return types.Target{Kind: types.TargetURLPost, URL: canonicalURL}, nil

	case "reel", "reels":
		// /reel/<shortcode>/ or /reels/<shortcode>/
		if len(segments) < 2 || !shortcodeRegexp.MatchString(segments[1]) {
			return types.Target{}, fmt.Errorf("%w: malformed reel URL %q", ErrUnknownTarget, input)
		}
		return types.Target{Kind: types.TargetURLReel, URL: canonicalURL}, nil

	case "tv":
		// /tv/<shortcode>/
		if len(segments) < 2 || !shortcodeRegexp.MatchString(segments[1]) {
			return types.Target{}, fmt.Errorf("%w: malformed tv URL %q", ErrUnknownTarget, input)
		}
		return types.Target{Kind: types.TargetURLTV, URL: canonicalURL}, nil

	case "stories":
		// /stories/highlights/<id>/ OR /stories/<handle>/<id>/
		if len(segments) < 2 {
			return types.Target{}, fmt.Errorf("%w: malformed stories URL %q", ErrUnknownTarget, input)
		}
		if segments[1] == "highlights" {
			if len(segments) < 3 || !shortcodeRegexp.MatchString(segments[2]) {
				return types.Target{}, fmt.Errorf("%w: malformed highlight URL %q", ErrUnknownTarget, input)
			}
			return types.Target{Kind: types.TargetURLHighlight, URL: canonicalURL}, nil
		}
		// /stories/<handle>/<id>?/
		if !handleRegexp.MatchString(segments[1]) {
			return types.Target{}, fmt.Errorf("%w: invalid handle in stories URL %q", ErrUnknownTarget, input)
		}
		handle := strings.ToLower(segments[1])
		return types.Target{Kind: types.TargetURLStory, URL: canonicalURL, Handle: handle}, nil

	default:
		// Reserved, non-profile first segment.
		if _, reserved := reservedHandles[head]; reserved {
			return types.Target{}, fmt.Errorf("%w: unsupported path %q", ErrUnknownTarget, u.Path)
		}
		// Profile or /<handle>/saved/.
		if !handleRegexp.MatchString(head) {
			return types.Target{}, fmt.Errorf("%w: invalid handle in %q", ErrUnknownTarget, input)
		}
		handle := strings.ToLower(head)
		if len(segments) >= 2 && segments[1] == "saved" {
			return types.Target{Kind: types.TargetSaved, URL: canonicalURL, Handle: handle}, nil
		}
		// Any remaining extra segments under a profile are unsupported.
		if len(segments) > 1 {
			return types.Target{}, fmt.Errorf("%w: unsupported profile sub-path %q", ErrUnknownTarget, u.Path)
		}
		return types.Target{Kind: types.TargetUserAll, URL: canonicalURL, Handle: handle}, nil
	}
}
