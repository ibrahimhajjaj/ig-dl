package session

import (
	"os"
	"path/filepath"
	"testing"
)

// TestImport is table-driven over the validation branches:
// happy path, malformed JSON, empty cookies, missing/zero CapturedAt.
func TestImport(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		body    string
		wantErr bool
		errHint string // substring the error message must contain
	}{
		{
			name: "valid",
			body: `{
				"cookies": [
					{"Name":"sessionid","Value":"abc","Domain":".instagram.com","Path":"/"}
				],
				"headers": {"x-ig-app-id":"123"},
				"query_hashes": {"qh":"/graphql"},
				"doc_ids": {},
				"captured_at": "2026-04-16T10:00:00Z"
			}`,
			wantErr: false,
		},
		{
			name:    "malformed JSON",
			body:    `{"cookies": [`,
			wantErr: true,
			errHint: "decode",
		},
		{
			name: "missing cookies array",
			body: `{
				"headers": {},
				"captured_at": "2026-04-16T10:00:00Z"
			}`,
			wantErr: true,
			errHint: "cookies",
		},
		{
			name: "empty cookies array",
			body: `{
				"cookies": [],
				"captured_at": "2026-04-16T10:00:00Z"
			}`,
			wantErr: true,
			errHint: "cookies",
		},
		{
			name: "zero CapturedAt",
			body: `{
				"cookies": [{"Name":"sid","Value":"v","Domain":"instagram.com"}],
				"captured_at": "0001-01-01T00:00:00Z"
			}`,
			wantErr: true,
			errHint: "captured_at",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()
			path := filepath.Join(dir, "session.json")
			if err := os.WriteFile(path, []byte(tc.body), 0o600); err != nil {
				t.Fatalf("write fixture: %v", err)
			}

			s, err := Import(path)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil (session=%+v)", s)
				}
				if tc.errHint != "" && !contains(err.Error(), tc.errHint) {
					t.Errorf("error %q does not contain %q", err.Error(), tc.errHint)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if s == nil {
				t.Fatal("session is nil on success")
			}
			if len(s.Cookies) == 0 {
				t.Error("cookies empty on valid session")
			}
			if s.CapturedAt.IsZero() {
				t.Error("CapturedAt zero on valid session")
			}
		})
	}
}

// TestImport_MissingFile confirms a file-not-found surfaces as an error
// rather than a nil session with no indication.
func TestImport_MissingFile(t *testing.T) {
	t.Parallel()
	s, err := Import(filepath.Join(t.TempDir(), "does-not-exist.json"))
	if err == nil {
		t.Fatalf("expected error for missing file, got session=%+v", s)
	}
}

func contains(s, sub string) bool {
	return len(sub) == 0 || indexOf(s, sub) >= 0
}

func indexOf(s, sub string) int {
	// small local to avoid a strings import in a single helper
	if len(sub) == 0 {
		return 0
	}
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
