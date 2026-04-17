package backend

import (
	"errors"
	"fmt"
	"os/exec"
	"testing"

	"github.com/ibrahimhajjaj/ig-dl/internal/types"
)

func TestClassify(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		stderr   string
		exitCode int
		want     types.AuthErrorCategory
	}{
		{
			name:     "nil error -> none",
			err:      nil,
			stderr:   "",
			exitCode: 0,
			want:     types.AuthErrNone,
		},
		{
			name:     "exec.ErrNotFound direct -> backend missing",
			err:      exec.ErrNotFound,
			stderr:   "",
			exitCode: -1,
			want:     types.AuthErrBackendMissing,
		},
		{
			name:     "exec.ErrNotFound wrapped -> backend missing",
			err:      fmt.Errorf("starting process: %w", exec.ErrNotFound),
			stderr:   "",
			exitCode: -1,
			want:     types.AuthErrBackendMissing,
		},
		{
			name:     "gallery-dl exit code 4 -> auth failed",
			err:      errors.New("exit status 4"),
			stderr:   "something",
			exitCode: 4,
			want:     types.AuthErrAuthFailed,
		},
		{
			name:     "stderr 401 -> auth failed",
			err:      errors.New("exit status 1"),
			stderr:   "urllib.error.HTTPError: HTTP Error 401: Unauthorized\n",
			exitCode: 1,
			want:     types.AuthErrAuthFailed,
		},
		{
			name:     "stderr 403 case-insensitive -> auth failed",
			err:      errors.New("exit status 1"),
			stderr:   "http error 403: forbidden",
			exitCode: 1,
			want:     types.AuthErrAuthFailed,
		},
		{
			name:     "stderr login required -> auth failed",
			err:      errors.New("exit status 1"),
			stderr:   "ERROR: Login required to access this resource\n",
			exitCode: 1,
			want:     types.AuthErrAuthFailed,
		},
		{
			name:     "stderr Authentication mixed case -> auth failed",
			err:      errors.New("exit status 1"),
			stderr:   "Authentication failed\n",
			exitCode: 1,
			want:     types.AuthErrAuthFailed,
		},
		{
			name:     "stderr 429 -> rate limited",
			err:      errors.New("exit status 1"),
			stderr:   "HTTP Error 429: Too Many Requests\n",
			exitCode: 1,
			want:     types.AuthErrRateLimited,
		},
		{
			name:     "stderr rate limit phrase -> rate limited",
			err:      errors.New("exit status 1"),
			stderr:   "Rate Limit exceeded, try again later\n",
			exitCode: 1,
			want:     types.AuthErrRateLimited,
		},
		{
			name:     "generic failure -> none",
			err:      errors.New("exit status 1"),
			stderr:   "something went wrong unrelated\n",
			exitCode: 1,
			want:     types.AuthErrNone,
		},
		{
			name:     "auth takes precedence over rate limit when both appear",
			err:      errors.New("exit status 1"),
			stderr:   "HTTP Error 401 and rate limit also\n",
			exitCode: 1,
			want:     types.AuthErrAuthFailed,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			got := Classify(tc.err, tc.stderr, tc.exitCode)
			if got != tc.want {
				t.Fatalf("Classify() = %d, want %d", got, tc.want)
			}
		})
	}
}

func TestExecError_Error(t *testing.T) {
	e := &ExecError{
		Category: types.AuthErrAuthFailed,
		ExitCode: 4,
		Stderr:   "login required\n",
		Inner:    errors.New("exit status 4"),
	}
	s := e.Error()
	if s == "" {
		t.Fatal("ExecError.Error() returned empty string")
	}
}

func TestExecError_Unwrap(t *testing.T) {
	inner := errors.New("boom")
	e := &ExecError{Inner: inner}
	if !errors.Is(e, inner) {
		t.Fatalf("errors.Is did not unwrap ExecError -> inner")
	}
}

func TestExecError_NilError(t *testing.T) {
	var e *ExecError
	if got := e.Error(); got != "<nil>" {
		t.Fatalf("nil ExecError.Error() = %q, want <nil>", got)
	}
}
