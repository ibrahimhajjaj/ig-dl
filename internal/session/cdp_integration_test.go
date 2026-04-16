//go:build integration

package session

import "testing"

// TestAttachRunningChrome_Integration is a placeholder for the real CDP
// integration test that will launch a headless Chromium with a fixture
// IG-like tab and verify AttachRunningChrome captures the expected
// cookies, headers and query hashes.
//
// Build with:
//
//	go test -tags integration ./internal/session/...
//
// The real implementation will land in a follow-up commit; it is gated
// off the default build because it spawns a browser subprocess and
// therefore is slow and flaky on CI machines without Chrome installed.
func TestAttachRunningChrome_Integration(t *testing.T) {
	t.Skip("CDP integration test not yet implemented — launch headless Chromium, open an instagram.com fixture page, run AttachRunningChrome, and assert Session.Cookies / Session.Headers are populated")
}
