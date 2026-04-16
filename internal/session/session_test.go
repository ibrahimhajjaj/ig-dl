package session

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ibrhajjaj/ig-dl/internal/types"
)

// TestSaveImportRoundTrip writes a session to disk, reads it back through
// the importer, and asserts every field survives.
func TestSaveImportRoundTrip(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	// Use a nested path so Save has to create the parent dir.
	path := filepath.Join(dir, "nested", "session.json")

	orig := &types.Session{
		Cookies: []http.Cookie{
			{Name: "sessionid", Value: "abc", Domain: ".instagram.com", Path: "/", Secure: true, HttpOnly: true},
			{Name: "csrftoken", Value: "tok", Domain: ".instagram.com", Path: "/"},
		},
		Headers:     map[string]string{"x-ig-app-id": "936619743392459"},
		QueryHashes: map[string]string{"qh1": "/graphql"},
		DocIDs:      map[string]string{"did1": "/api"},
		CapturedAt:  time.Now().UTC().Truncate(time.Second),
	}

	if err := Save(orig, path); err != nil {
		t.Fatalf("Save: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("file mode = %v, want 0600", info.Mode().Perm())
	}

	// Parent directory must be 0700.
	parentInfo, err := os.Stat(filepath.Dir(path))
	if err != nil {
		t.Fatalf("stat parent: %v", err)
	}
	if perm := parentInfo.Mode().Perm(); perm != 0o700 {
		t.Errorf("parent dir mode = %v, want 0700", perm)
	}

	got, err := Import(path)
	if err != nil {
		t.Fatalf("Import: %v", err)
	}

	if len(got.Cookies) != len(orig.Cookies) {
		t.Fatalf("cookies count = %d, want %d", len(got.Cookies), len(orig.Cookies))
	}
	for i := range got.Cookies {
		if got.Cookies[i].Name != orig.Cookies[i].Name ||
			got.Cookies[i].Value != orig.Cookies[i].Value ||
			got.Cookies[i].Domain != orig.Cookies[i].Domain {
			t.Errorf("cookie[%d] = %+v, want %+v", i, got.Cookies[i], orig.Cookies[i])
		}
	}
	if got.Headers["x-ig-app-id"] != orig.Headers["x-ig-app-id"] {
		t.Errorf("headers mismatch: %v vs %v", got.Headers, orig.Headers)
	}
	if got.QueryHashes["qh1"] != orig.QueryHashes["qh1"] {
		t.Errorf("query hashes mismatch: %v vs %v", got.QueryHashes, orig.QueryHashes)
	}
	if got.DocIDs["did1"] != orig.DocIDs["did1"] {
		t.Errorf("doc ids mismatch: %v vs %v", got.DocIDs, orig.DocIDs)
	}
	if !got.CapturedAt.Equal(orig.CapturedAt) {
		t.Errorf("captured_at = %v, want %v", got.CapturedAt, orig.CapturedAt)
	}
}

// TestSave_NilSession rejects a nil input with a non-nil error rather
// than writing an empty file.
func TestSave_NilSession(t *testing.T) {
	t.Parallel()
	err := Save(nil, filepath.Join(t.TempDir(), "x.json"))
	if err == nil {
		t.Fatal("expected error for nil session")
	}
}

// TestAge_ZeroAndPast covers the two interesting branches of Age: an
// uninitialised CapturedAt (returns 0) and a CapturedAt in the past
// (returns a positive, bounded duration).
func TestAge(t *testing.T) {
	t.Parallel()

	var zero types.Session
	if age := Age(&zero); age != 0 {
		t.Errorf("zero CapturedAt: age = %v, want 0", age)
	}
	if age := Age(nil); age != 0 {
		t.Errorf("nil session: age = %v, want 0", age)
	}

	past := &types.Session{CapturedAt: time.Now().Add(-5 * time.Minute)}
	age := Age(past)
	if age <= 0 {
		t.Errorf("past CapturedAt: age = %v, want > 0", age)
	}
	if age < 4*time.Minute || age > 10*time.Minute {
		t.Errorf("past CapturedAt: age = %v, want ~5m", age)
	}
}
