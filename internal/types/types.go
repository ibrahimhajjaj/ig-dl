// Package types holds the cross-cutting types used by every layer of ig-dl.
//
// Every package imports these definitions so the CLI, MCP server, session
// manager, router, and backend runners speak the same Session/Target/Backend
// vocabulary.
package types

import (
	"context"
	"net/http"
	"time"
)

// Session is the authenticated Instagram state captured from a running
// browser. Either the CDP attach path or the importer path produces one.
type Session struct {
	Cookies     []http.Cookie     `json:"cookies"`
	Headers     map[string]string `json:"headers"`
	QueryHashes map[string]string `json:"query_hashes"`
	DocIDs      map[string]string `json:"doc_ids"`
	CapturedAt  time.Time         `json:"captured_at"`
}

// TargetKind categorises a download target so the router can pick a backend.
type TargetKind int

const (
	TargetUnknown TargetKind = iota
	TargetURLPost
	TargetURLReel
	TargetURLStory
	TargetURLHighlight
	TargetURLTV
	TargetUserAll
	TargetSaved
)

// Target is a normalised download request.
type Target struct {
	Kind   TargetKind
	URL    string
	Handle string
}

// Backend is implemented by gallery-dl and yt-dlp runners.
type Backend interface {
	Fetch(ctx context.Context, t Target, s *Session, outDir string) error
}

// AuthErrorCategory classifies backend failures that the session manager
// can act on (refresh + retry).
type AuthErrorCategory int

const (
	AuthErrNone AuthErrorCategory = iota
	AuthErrNoSession
	AuthErrBackendMissing
	AuthErrAuthFailed
	AuthErrRateLimited
)
