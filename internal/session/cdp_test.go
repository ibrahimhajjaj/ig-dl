package session

import (
	"net/http"
	"testing"

	"github.com/chromedp/cdproto/network"
)

// TestIsIGTabURL drives the pure URL-classification helper across the
// hostnames we need to accept / reject, including look-alikes and weirdly
// cased URLs.
func TestIsIGTabURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		url  string
		want bool
	}{
		{"https://www.instagram.com/", true},
		{"https://www.instagram.com/some/path?q=1", true},
		{"https://instagram.com/p/XYZ/", true},
		{"https://i.instagram.com/api/v1/", true},
		{"https://WWW.INSTAGRAM.COM/", true}, // case-insensitive host
		{"https://instagram.com.evil.com/", false},
		{"https://example.com/instagram.com/", false},
		{"about:blank", false},
		{"", false},
		{"not a url", false},
		{"https://news.ycombinator.com/", false},
	}
	for _, tc := range tests {
		if got := isIGTabURL(tc.url); got != tc.want {
			t.Errorf("isIGTabURL(%q) = %v, want %v", tc.url, got, tc.want)
		}
	}
}

// TestIsIGCookieDomain covers the cookie-domain filter used to drop
// non-Instagram cookies from the browser jar before export.
func TestIsIGCookieDomain(t *testing.T) {
	t.Parallel()

	tests := []struct {
		domain string
		want   bool
	}{
		{".instagram.com", true},
		{"instagram.com", true},
		{"www.instagram.com", true},
		{"i.instagram.com", true},
		{"  .instagram.com  ", true},
		{".INSTAGRAM.COM", true},
		{".facebook.com", false},
		{"", false},
		{"instagram.com.evil.com", false},
	}
	for _, tc := range tests {
		if got := isIGCookieDomain(tc.domain); got != tc.want {
			t.Errorf("isIGCookieDomain(%q) = %v, want %v", tc.domain, got, tc.want)
		}
	}
}

// TestCaptureHeaderValue asserts the header filter only surfaces the
// four IG API headers we care about, and rejects non-string values
// gracefully.
func TestCaptureHeaderValue(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		header  string
		value   any
		wantKey string
		wantVal string
		wantOK  bool
	}{
		{"www-claim", "x-ig-www-claim", "hmac.xyz", "x-ig-www-claim", "hmac.xyz", true},
		{"upper case", "X-IG-APP-ID", "936619743392459", "x-ig-app-id", "936619743392459", true},
		{"asbd", "x-asbd-id", "198387", "x-asbd-id", "198387", true},
		{"ajax", "x-instagram-ajax", "abc123", "x-instagram-ajax", "abc123", true},
		{"unrelated header", "user-agent", "Mozilla", "", "", false},
		{"non-string value", "x-ig-app-id", 42, "", "", false},
	}
	for _, tc := range tests {
		k, v, ok := captureHeaderValue(tc.header, tc.value)
		if k != tc.wantKey || v != tc.wantVal || ok != tc.wantOK {
			t.Errorf("%s: got (%q,%q,%v), want (%q,%q,%v)",
				tc.name, k, v, ok, tc.wantKey, tc.wantVal, tc.wantOK)
		}
	}
}

// TestCapture_OnRequest exercises the event collector directly: it must
// record IG-API headers, query_hash/doc_id query params, and ignore
// non-IG requests.
func TestCapture_OnRequest(t *testing.T) {
	t.Parallel()

	c := newCapture()

	// An IG request carrying the full header set and a query_hash param.
	c.onRequest(&network.EventRequestWillBeSent{
		Request: &network.Request{
			URL: "https://www.instagram.com/graphql/query/?query_hash=abc123&variables=%7B%7D",
			Headers: network.Headers{
				"x-ig-www-claim":   "hmac.xyz",
				"x-ig-app-id":      "936619743392459",
				"x-asbd-id":        "198387",
				"x-instagram-ajax": "ajaxv",
				"user-agent":       "Mozilla",
			},
		},
	})

	// A separate IG request with doc_id instead of query_hash.
	c.onRequest(&network.EventRequestWillBeSent{
		Request: &network.Request{
			URL:     "https://i.instagram.com/api/graphql/?doc_id=7788990011",
			Headers: network.Headers{},
		},
	})

	// A non-IG request: must not pollute the capture.
	c.onRequest(&network.EventRequestWillBeSent{
		Request: &network.Request{
			URL: "https://news.ycombinator.com/foo?query_hash=nope&doc_id=nope",
			Headers: network.Headers{
				"x-ig-app-id": "should-be-ignored",
			},
		},
	})

	headers, qhs, dids := c.snapshot()

	if got, want := headers["x-ig-www-claim"], "hmac.xyz"; got != want {
		t.Errorf("www-claim = %q, want %q", got, want)
	}
	if got, want := headers["x-ig-app-id"], "936619743392459"; got != want {
		t.Errorf("app-id = %q, want %q", got, want)
	}
	if got, want := headers["x-asbd-id"], "198387"; got != want {
		t.Errorf("asbd-id = %q, want %q", got, want)
	}
	if got, want := headers["x-instagram-ajax"], "ajaxv"; got != want {
		t.Errorf("ajax = %q, want %q", got, want)
	}
	if _, ok := headers["user-agent"]; ok {
		t.Error("user-agent should not be captured")
	}
	if _, ok := qhs["abc123"]; !ok {
		t.Errorf("query_hash abc123 not recorded: %v", qhs)
	}
	if _, ok := dids["7788990011"]; !ok {
		t.Errorf("doc_id 7788990011 not recorded: %v", dids)
	}
	// Verify non-IG request was filtered out.
	if _, ok := qhs["nope"]; ok {
		t.Error("non-IG query_hash leaked into capture")
	}
	if _, ok := dids["nope"]; ok {
		t.Error("non-IG doc_id leaked into capture")
	}
}

// TestFilterIGCookies checks that only instagram.com cookies survive,
// session cookies keep zero Expires, and HTTPOnly/Secure bits transfer.
func TestFilterIGCookies(t *testing.T) {
	t.Parallel()

	cks := []*network.Cookie{
		{Name: "a", Value: "1", Domain: ".instagram.com", Secure: true, HTTPOnly: true, Session: true},
		{Name: "b", Value: "2", Domain: "www.instagram.com", Expires: 1700000000},
		{Name: "c", Value: "3", Domain: ".facebook.com"},
		nil, // defensive: nil entries must be skipped
	}
	got := filterIGCookies(cks)
	if len(got) != 2 {
		t.Fatalf("want 2 cookies, got %d: %+v", len(got), got)
	}
	if got[0].Name != "a" || !got[0].Secure || !got[0].HttpOnly {
		t.Errorf("cookie[0] = %+v", got[0])
	}
	if !got[0].Expires.IsZero() {
		t.Errorf("session cookie should have zero Expires, got %v", got[0].Expires)
	}
	if got[1].Name != "b" {
		t.Errorf("cookie[1] = %+v", got[1])
	}
	if got[1].Expires.IsZero() {
		t.Error("cookie[1] should have non-zero Expires")
	}
}

// TestProbeDebugEndpoint_BadPort confirms a closed port produces a
// non-nil error promptly (we don't care about the exact message).
func TestProbeDebugEndpoint_BadPort(t *testing.T) {
	t.Parallel()
	client := &http.Client{}
	// Port 1 is reliably refused on all platforms we care about.
	if err := probeDebugEndpoint(t.Context(), client, "http://127.0.0.1:1"); err == nil {
		t.Fatal("expected error, got nil")
	}
}

// TestParseDebugEndpointJSON is a trivial round-trip that mainly exists
// to lock the helper in place — if Chrome changes the /json/version
// schema, this test still passes (since it just parses generic JSON),
// but the symbol stays reachable for integration tests that want to
// poke at the response.
func TestParseDebugEndpointJSON(t *testing.T) {
	t.Parallel()
	m, err := parseDebugEndpointJSON([]byte(`{"Browser":"Chrome/124.0.0.0"}`))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if m["Browser"] != "Chrome/124.0.0.0" {
		t.Errorf("unexpected content: %+v", m)
	}
}
