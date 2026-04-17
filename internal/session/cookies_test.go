package session

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ibrahimhajjaj/ig-dl/internal/types"
)

// TestWriteNetscape_Fixture exercises every branch of formatNetscapeLine
// via WriteNetscape: httpOnly prefix, secure flag, dotted-domain flag,
// session-cookie zero-expiry, and explicit-expiry unix epoch.
func TestWriteNetscape_Fixture(t *testing.T) {
	t.Parallel()

	expires := time.Date(2030, 6, 1, 12, 0, 0, 0, time.UTC)
	s := &types.Session{
		CapturedAt: time.Now().UTC(),
		Cookies: []http.Cookie{
			{
				Name:     "sessionid",
				Value:    "abc123",
				Domain:   ".instagram.com",
				Path:     "/",
				Secure:   true,
				HttpOnly: true,
				Expires:  expires,
			},
			{
				Name:   "csrftoken",
				Value:  "tok",
				Domain: ".instagram.com",
				Path:   "/",
				Secure: true,
				// no HttpOnly, no expiry → session cookie on a
				// dotted domain with Secure=TRUE.
			},
			{
				Name:    "ds_user_id",
				Value:   "42",
				Domain:  "www.instagram.com",
				Path:    "/accounts",
				Secure:  false,
				Expires: expires,
			},
			{
				Name:     "ig_did",
				Value:    "did",
				Domain:   ".instagram.com",
				Path:     "/",
				Secure:   true,
				HttpOnly: true,
				MaxAge:   -1, // force session marker even if Expires set
				Expires:  expires,
			},
		},
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "cookies.txt")
	if err := WriteNetscape(s, path); err != nil {
		t.Fatalf("WriteNetscape: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("file mode = %v, want 0600", info.Mode().Perm())
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	got := string(data)
	if !strings.HasPrefix(got, "# Netscape HTTP Cookie File\n") {
		t.Errorf("missing Netscape header, got prefix=%q", got[:min(len(got), 40)])
	}

	lines := strings.Split(strings.TrimRight(got, "\n"), "\n")

	// Strip comment/blank header lines.
	var cookieLines []string
	for _, l := range lines {
		if l == "" || strings.HasPrefix(l, "# ") {
			continue
		}
		cookieLines = append(cookieLines, l)
	}
	if len(cookieLines) != 4 {
		t.Fatalf("want 4 cookie lines, got %d: %v", len(cookieLines), cookieLines)
	}

	wantExp := expires.Unix()

	checks := []struct {
		name string
		line string
		want string
	}{
		{
			name: "httpOnly+secure+explicit expiry on dotted domain",
			line: cookieLines[0],
			want: joinTabs("#HttpOnly_.instagram.com", "TRUE", "/", "TRUE", itoa(wantExp), "sessionid", "abc123"),
		},
		{
			name: "secure session cookie (no expiry) on dotted domain",
			line: cookieLines[1],
			want: joinTabs(".instagram.com", "TRUE", "/", "TRUE", "0", "csrftoken", "tok"),
		},
		{
			name: "non-secure, non-dotted domain with explicit expiry and custom path",
			line: cookieLines[2],
			want: joinTabs("www.instagram.com", "FALSE", "/accounts", "FALSE", itoa(wantExp), "ds_user_id", "42"),
		},
		{
			name: "httpOnly with MaxAge<0 forces session zero even with Expires set",
			line: cookieLines[3],
			want: joinTabs("#HttpOnly_.instagram.com", "TRUE", "/", "TRUE", "0", "ig_did", "did"),
		},
	}

	for _, c := range checks {
		if c.line != c.want {
			t.Errorf("%s:\n got  %q\n want %q", c.name, c.line, c.want)
		}
		// Every cookie line must be tab-separated exactly (6 tabs → 7 fields).
		if strings.Count(c.line, "\t") != 6 {
			t.Errorf("%s: expected exactly 6 tab separators, got %d in %q",
				c.name, strings.Count(c.line, "\t"), c.line)
		}
	}
}

// TestFormatNetscapeLine pokes the helper directly for edge cases that
// matter but don't need to go through a file (empty path default, flag
// toggling).
func TestFormatNetscapeLine(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name              string
		domain, path      string
		secure, httpOnly  bool
		expires           int64
		want              string
	}{
		{
			name:    "empty path defaults to /",
			domain:  ".instagram.com", path: "", secure: true, httpOnly: false, expires: 0,
			want: joinTabs(".instagram.com", "TRUE", "/", "TRUE", "0", "n", "v") + "",
		},
		{
			name:    "non-dotted domain uses FALSE flag",
			domain:  "instagram.com", path: "/p", secure: false, httpOnly: false, expires: 1700000000,
			want: joinTabs("instagram.com", "FALSE", "/p", "FALSE", "1700000000", "n", "v") + "",
		},
	}

	for _, tc := range tests {
		got := formatNetscapeLine(tc.domain, tc.path, "n", "v", tc.secure, tc.httpOnly, tc.expires)
		// formatNetscapeLine always appends \n; compare trimmed.
		if strings.TrimRight(got, "\n") != tc.want {
			t.Errorf("%s:\n got  %q\n want %q", tc.name, got, tc.want)
		}
		if !strings.HasSuffix(got, "\n") {
			t.Errorf("%s: missing trailing newline", tc.name)
		}
	}
}

func joinTabs(fields ...string) string {
	return strings.Join(fields, "\t")
}

func itoa(i int64) string {
	// local to avoid importing strconv just for this
	if i == 0 {
		return "0"
	}
	neg := i < 0
	if neg {
		i = -i
	}
	var buf [20]byte
	pos := len(buf)
	for i > 0 {
		pos--
		buf[pos] = byte('0' + i%10)
		i /= 10
	}
	if neg {
		pos--
		buf[pos] = '-'
	}
	return string(buf[pos:])
}

