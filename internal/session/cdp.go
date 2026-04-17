package session

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/cdproto/storage"
	cdptarget "github.com/chromedp/cdproto/target"
	"github.com/chromedp/chromedp"

	"github.com/ibrhajjaj/ig-dl/internal/types"
)

// igAPIHeaders is the set of request headers we harvest from live IG API
// calls. They're keyed lowercase because Chrome delivers header names in
// their original case and we compare case-insensitively.
var igAPIHeaders = map[string]struct{}{
	"x-ig-www-claim":    {},
	"x-ig-app-id":       {},
	"x-asbd-id":         {},
	"x-instagram-ajax":  {},
}

// igHosts are the exact Instagram hostnames we consider "an IG tab" for
// tab-picking and "an IG API request" for header capture.
var igHosts = []string{
	"www.instagram.com",
	"instagram.com",
	"i.instagram.com",
	"api.instagram.com",
}

// captureWindow is the default duration we listen to network events after
// attaching before returning the session. Callers that need a different
// window use AttachRunningChromeWithOptions.
const captureWindow = 2 * time.Second

// AttachOptions tunes how AttachRunningChrome behaves. The zero value is
// safe and matches the defaults used by AttachRunningChrome.
type AttachOptions struct {
	// CaptureWindow bounds how long we sit on the IG tab listening for
	// network events to harvest headers and query hashes. Defaults to 2s
	// when zero.
	CaptureWindow time.Duration
	// HTTPClient is used to probe /json/version. Mainly useful for tests
	// that want to inject a transport; production code uses a default
	// client with a 2-second timeout.
	HTTPClient *http.Client
}

// AttachRunningChrome connects to a Chrome instance already running with
// --remote-debugging-port=<debugPort>, looks for a tab on instagram.com,
// pulls the cookie jar for the instagram.com domain, and listens briefly
// for outgoing API requests so it can scrape the header set Instagram
// requires for GraphQL/API calls. On success it returns a populated
// types.Session; on any failure it returns one of the sentinel errors
// (ErrCDPUnreachable, ErrNoIGTab) or a wrapped chromedp error.
//
// The passed context bounds the whole operation — if it is cancelled or
// hits its deadline, AttachRunningChrome returns promptly. A two-second
// capture window runs on top of the context for the network-event
// listener; see AttachRunningChromeWithOptions to tune it.
func AttachRunningChrome(ctx context.Context, debugPort int) (*types.Session, error) {
	return AttachRunningChromeWithOptions(ctx, debugPort, AttachOptions{})
}

// AttachDiscovered tries to find a running Chromium-based browser that
// exposes CDP via its DevToolsActivePort file and attaches to it.
//
// Two on-disk shapes produce a valid ActivePort:
//
//  1. Classic `--remote-debugging-port` launch: the file carries just a
//     port, and CDP is discovered through `/json/version`.
//  2. Chrome M144+ `chrome://inspect/#remote-debugging` toggle: the file
//     carries the port AND a specific WebSocket path (e.g.
//     `/devtools/browser/<uuid>`). The /json/version endpoint 404s in
//     this mode — we connect the WebSocket directly.
//
// Shape 2 is the only known way to get CDP against the user's real
// default profile without a relaunch; shape 1 requires --user-data-dir.
func AttachDiscovered(ctx context.Context, preferred Browser) (*types.Session, *ActivePort, error) {
	all := DiscoverAllActivePorts()
	if len(all) == 0 {
		return nil, nil, ErrNoActivePort
	}
	// Re-order so `preferred` is tried first when set.
	if preferred != "" {
		ordered := make([]*ActivePort, 0, len(all))
		for _, ap := range all {
			if ap.Browser == preferred {
				ordered = append(ordered, ap)
			}
		}
		for _, ap := range all {
			if ap.Browser != preferred {
				ordered = append(ordered, ap)
			}
		}
		all = ordered
	}

	var lastErr error
	for _, ap := range all {
		if !ap.IsLive(ctx) {
			continue
		}
		s, err := attachOneActive(ctx, ap)
		if err == nil {
			return s, ap, nil
		}
		lastErr = fmt.Errorf("%s (port %d): %w", ap.Browser, ap.Port, err)
		// Keep iterating — ErrNoIGTab in one browser doesn't preclude
		// another browser having a logged-in IG tab.
	}
	if lastErr == nil {
		return nil, nil, ErrNoActivePort
	}
	return nil, nil, lastErr
}

func attachOneActive(ctx context.Context, ap *ActivePort) (*types.Session, error) {
	if ap.WSPath != "" {
		return AttachViaWSEndpoint(ctx, fmt.Sprintf("ws://127.0.0.1:%d%s", ap.Port, ap.WSPath), AttachOptions{})
	}
	return AttachRunningChromeWithOptions(ctx, ap.Port, AttachOptions{})
}

// AttachViaWSEndpoint attaches to an already-running browser via a
// direct WebSocket URL (e.g. ws://127.0.0.1:9222/devtools/browser/<id>),
// bypassing the /json/version HTTP discovery step. This is the path
// taken for Chrome M144+ toggle-style live sessions.
//
// The passed context bounds the whole operation. If Chrome prompts the
// user for permission (M144 behaviour), the dialog must be answered
// within that deadline.
func AttachViaWSEndpoint(ctx context.Context, wsURL string, opts AttachOptions) (*types.Session, error) {
	if opts.CaptureWindow <= 0 {
		opts.CaptureWindow = captureWindow
	}

	allocCtx, allocCancel := chromedp.NewRemoteAllocator(ctx, wsURL, chromedp.NoModifyURL)
	defer allocCancel()

	browserCtx, browserCancel := chromedp.NewContext(allocCtx)
	defer browserCancel()

	if err := chromedp.Run(browserCtx); err != nil {
		return nil, fmt.Errorf("session: connect via %s: %w", wsURL, err)
	}

	return captureFromBrowserContext(browserCtx, opts.CaptureWindow)
}

// AttachRunningChromeWithOptions is the configurable variant of
// AttachRunningChrome. See AttachOptions for the knobs.
func AttachRunningChromeWithOptions(ctx context.Context, debugPort int, opts AttachOptions) (*types.Session, error) {
	if opts.CaptureWindow <= 0 {
		opts.CaptureWindow = captureWindow
	}
	if opts.HTTPClient == nil {
		opts.HTTPClient = &http.Client{Timeout: 2 * time.Second}
	}

	debugURL := fmt.Sprintf("http://127.0.0.1:%d", debugPort)

	// Probe /json/version with a small retry loop so transient network
	// blips (e.g. Chrome is mid-restart) don't fail us instantly.
	var probeErr error
	for attempt := 0; attempt < 3; attempt++ {
		probeErr = probeDebugEndpoint(ctx, opts.HTTPClient, debugURL)
		if probeErr == nil {
			break
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
	if probeErr != nil {
		return nil, fmt.Errorf("%w: %v", ErrCDPUnreachable, probeErr)
	}

	allocCtx, allocCancel := chromedp.NewRemoteAllocator(ctx, debugURL)
	defer allocCancel()

	// The browser-level context lets us enumerate targets and issue
	// browser-scoped commands (storage.GetCookies).
	browserCtx, browserCancel := chromedp.NewContext(allocCtx)
	defer browserCancel()

	// Touch Run once to ensure the websocket is attached before asking
	// for targets — chromedp lazily connects.
	if err := chromedp.Run(browserCtx); err != nil {
		return nil, fmt.Errorf("session: connect to chrome: %w", err)
	}

	return captureFromBrowserContext(browserCtx, opts.CaptureWindow)
}

// captureFromBrowserContext takes an already-connected chromedp browser
// context and performs the IG-tab attach + header/cookie capture that
// both AttachRunningChromeWithOptions and AttachViaWSEndpoint share.
func captureFromBrowserContext(browserCtx context.Context, captureFor time.Duration) (*types.Session, error) {
	targets, err := chromedp.Targets(browserCtx)
	if err != nil {
		return nil, fmt.Errorf("session: list targets: %w", err)
	}

	targetID, targetURL := pickIGTab(targets)
	if targetID == "" {
		return nil, ErrNoIGTab
	}

	// Attach to the IG tab. We capture tab-scoped network events here.
	tabCtx, tabCancel := chromedp.NewContext(browserCtx, chromedp.WithTargetID(targetID))
	defer tabCancel()

	// Bring the tab session up (equivalent to Target.attachToTarget).
	if err := chromedp.Run(tabCtx); err != nil {
		return nil, fmt.Errorf("session: attach to IG tab %s: %w", targetURL, err)
	}

	// Collector shared between the header/query-hash goroutine and the
	// main thread.
	cap := newCapture()

	// Start listening *before* enabling network so we don't race the
	// first few events.
	listenCtx, listenCancel := context.WithCancel(tabCtx)
	defer listenCancel()
	chromedp.ListenTarget(listenCtx, func(ev any) {
		if e, ok := ev.(*network.EventRequestWillBeSent); ok {
			cap.onRequest(e)
		}
	})

	// Enable the Network domain for the tab so events flow.
	if err := chromedp.Run(tabCtx, network.Enable()); err != nil {
		return nil, fmt.Errorf("session: enable network domain: %w", err)
	}

	// Pull cookies from the browser-wide cookie store. Storage.getCookies
	// returns the entire jar; we filter to *.instagram.com below.
	var rawCookies []*network.Cookie
	if err := chromedp.Run(browserCtx, chromedp.ActionFunc(func(actx context.Context) error {
		cks, err := storage.GetCookies().Do(actx)
		if err != nil {
			return err
		}
		rawCookies = cks
		return nil
	})); err != nil {
		return nil, fmt.Errorf("session: read cookies: %w", err)
	}

	cookies := filterIGCookies(rawCookies)

	// Listen for network events for up to captureFor, but cut out early
	// if the caller cancels the outer context.
	select {
	case <-time.After(captureFor):
	case <-browserCtx.Done():
	}
	listenCancel()

	headers, queryHashes, docIDs := cap.snapshot()

	return &types.Session{
		Cookies:     cookies,
		Headers:     headers,
		QueryHashes: queryHashes,
		DocIDs:      docIDs,
		CapturedAt:  time.Now().UTC(),
	}, nil
}

// probeDebugEndpoint hits /json/version to confirm the Chrome DevTools
// HTTP endpoint is up before chromedp's websocket path tries to connect.
// A failure here is the classic "Chrome isn't running with
// --remote-debugging-port" case, and we want a targeted error for it.
func probeDebugEndpoint(ctx context.Context, client *http.Client, baseURL string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/json/version", nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		// Distinguish "connection refused" from other failures so
		// callers printing this error get a crisper message.
		var netErr *net.OpError
		if errors.As(err, &netErr) {
			return fmt.Errorf("cannot reach %s: %w", baseURL, err)
		}
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		// Since Chrome 136 / Edge 136, the debug port binds but the CDP
		// endpoint is disabled when the browser uses the default
		// user-data-dir. Surfacing this hint here saves users a long
		// debugging session.
		return fmt.Errorf("%s returned status %d (if the port is bound but CDP isn't answering, relaunch the browser with --user-data-dir=<fresh-path> alongside --remote-debugging-port; see README)", baseURL+"/json/version", resp.StatusCode)
	}
	return nil
}

// pickIGTab scans a list of CDP targets and returns the ID and URL of the
// first "page" target whose URL host matches one of the known IG hosts.
// Returns "" for both if no match.
func pickIGTab(targets []*cdptarget.Info) (cdptarget.ID, string) {
	for _, t := range targets {
		if t == nil {
			continue
		}
		if t.Type != "page" {
			continue
		}
		if isIGTabURL(t.URL) {
			return t.TargetID, t.URL
		}
	}
	return "", ""
}

// isIGTabURL returns true if rawURL is a page hosted on an Instagram
// hostname. It parses rather than string-matches so "instagram.com" in a
// path or query does not create a false positive.
func isIGTabURL(rawURL string) bool {
	if rawURL == "" {
		return false
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	host := strings.ToLower(u.Hostname())
	if host == "" {
		return false
	}
	// Accept exact host matches and any subdomain of instagram.com.
	if host == "instagram.com" || strings.HasSuffix(host, ".instagram.com") {
		return true
	}
	return false
}

// isIGCookieDomain returns true if the cookie's domain should be carried
// along in the exported Session. We accept the root domain variants IG
// actually serves cookies on.
func isIGCookieDomain(domain string) bool {
	d := strings.ToLower(strings.TrimSpace(domain))
	switch d {
	case ".instagram.com", "instagram.com", "www.instagram.com":
		return true
	}
	// Any *.instagram.com subdomain counts too (e.g. i.instagram.com).
	return strings.HasSuffix(d, ".instagram.com")
}

// filterIGCookies converts CDP network.Cookie entries into net/http
// cookies, keeping only those on an instagram.com domain.
func filterIGCookies(cks []*network.Cookie) []http.Cookie {
	out := make([]http.Cookie, 0, len(cks))
	for _, c := range cks {
		if c == nil {
			continue
		}
		if !isIGCookieDomain(c.Domain) {
			continue
		}
		hc := http.Cookie{
			Name:     c.Name,
			Value:    c.Value,
			Path:     c.Path,
			Domain:   c.Domain,
			Secure:   c.Secure,
			HttpOnly: c.HTTPOnly,
		}
		if c.Session {
			// Session cookie — leave Expires zero and MaxAge 0.
		} else if c.Expires > 0 {
			hc.Expires = time.Unix(int64(c.Expires), 0).UTC()
		}
		out = append(out, hc)
	}
	return out
}

// isIGAPIURL returns true for the set of URLs whose request headers we
// want to harvest (anything on an IG hostname that isn't obviously a
// static asset). Keeping it simple: hostname match, don't bother filtering
// image/css/js here because Chrome delivers those without the IG auth
// headers anyway.
func isIGAPIURL(rawURL string) bool {
	if rawURL == "" {
		return false
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	host := strings.ToLower(u.Hostname())
	if host == "" {
		return false
	}
	return host == "instagram.com" || strings.HasSuffix(host, ".instagram.com")
}

// captureHeaderValue returns (lowered-name, value, true) if name is one
// of the IG API headers we care about, otherwise ("", "", false).
func captureHeaderValue(name string, value any) (string, string, bool) {
	lname := strings.ToLower(name)
	if _, ok := igAPIHeaders[lname]; !ok {
		return "", "", false
	}
	sv, ok := value.(string)
	if !ok {
		// Chrome sends header values as strings, but network.Headers is
		// map[string]any, so be defensive.
		return "", "", false
	}
	return lname, sv, true
}

// capture accumulates the artefacts we read off
// EventRequestWillBeSent. It is goroutine-safe because
// chromedp.ListenTarget fires callbacks from an event-dispatch goroutine.
type capture struct {
	mu          sync.Mutex
	headers     map[string]string
	queryHashes map[string]string
	docIDs      map[string]string
}

func newCapture() *capture {
	return &capture{
		headers:     make(map[string]string),
		queryHashes: make(map[string]string),
		docIDs:      make(map[string]string),
	}
}

// onRequest inspects a single request event, pulling interesting headers
// and query params into the collector. Non-IG requests are ignored.
func (c *capture) onRequest(e *network.EventRequestWillBeSent) {
	if e == nil || e.Request == nil {
		return
	}
	req := e.Request
	if !isIGAPIURL(req.URL) {
		return
	}
	// Headers.
	for name, v := range req.Headers {
		if lname, sv, ok := captureHeaderValue(name, v); ok {
			c.mu.Lock()
			c.headers[lname] = sv
			c.mu.Unlock()
		}
	}
	// Query hashes and doc IDs.
	u, err := url.Parse(req.URL)
	if err == nil {
		q := u.Query()
		if qh := q.Get("query_hash"); qh != "" {
			c.mu.Lock()
			c.queryHashes[qh] = u.Path
			c.mu.Unlock()
		}
		if id := q.Get("doc_id"); id != "" {
			c.mu.Lock()
			c.docIDs[id] = u.Path
			c.mu.Unlock()
		}
	}
}

// snapshot returns a copy of the collected maps safe for the caller to
// retain without racing against further events.
func (c *capture) snapshot() (map[string]string, map[string]string, map[string]string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	h := make(map[string]string, len(c.headers))
	for k, v := range c.headers {
		h[k] = v
	}
	qh := make(map[string]string, len(c.queryHashes))
	for k, v := range c.queryHashes {
		qh[k] = v
	}
	d := make(map[string]string, len(c.docIDs))
	for k, v := range c.docIDs {
		d[k] = v
	}
	return h, qh, d
}

// parseDebugEndpointJSON is a convenience helper for tests that want to
// verify we'd accept the kind of JSON Chrome serves on /json/version. It
// is exported only to the package.
func parseDebugEndpointJSON(data []byte) (map[string]any, error) {
	var out map[string]any
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, err
	}
	return out, nil
}
