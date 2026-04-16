// Package backend implements the concrete download runners (gallery-dl,
// yt-dlp) behind the types.Backend interface. Each runner builds an argv,
// execs the backend binary, streams its output to the caller-provided
// writers, and classifies non-zero exit codes into a
// types.AuthErrorCategory so the session manager can decide whether to
// refresh+retry.
package backend

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"

	"github.com/ibrhajjaj/ig-dl/internal/types"
)

// ExecError wraps a backend invocation failure with enough context for the
// session manager to decide whether to refresh credentials and retry, and
// for the CLI layer to print something useful.
type ExecError struct {
	// Category classifies the failure for retry/auth-refresh decisions.
	Category types.AuthErrorCategory
	// ExitCode is the subprocess exit code, or -1 if the process was
	// never started (e.g. binary missing).
	ExitCode int
	// Stderr is the captured standard-error output of the subprocess.
	Stderr string
	// Inner is the underlying error returned by exec.Cmd.Run (or
	// context cancellation, etc).
	Inner error
}

// Error renders a single-line description suitable for CLI error output.
func (e *ExecError) Error() string {
	if e == nil {
		return "<nil>"
	}
	if e.Inner != nil {
		return fmt.Sprintf("backend exec failed (category=%d, exit=%d): %v: %s",
			e.Category, e.ExitCode, e.Inner, strings.TrimSpace(e.Stderr))
	}
	return fmt.Sprintf("backend exec failed (category=%d, exit=%d): %s",
		e.Category, e.ExitCode, strings.TrimSpace(e.Stderr))
}

// Unwrap exposes the inner error for errors.Is / errors.As.
func (e *ExecError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Inner
}

// runCmd runs cmd, streaming stdout and stderr to the supplied writers while
// also tee-ing stderr into an in-memory buffer so callers can classify
// failures. On error, the returned *ExecError carries the captured stderr
// and a best-effort AuthErrorCategory classification.
//
// If stdout or stderr is nil, output is still captured but not forwarded.
func runCmd(ctx context.Context, cmd *exec.Cmd, stdout, stderr io.Writer) error {
	var errBuf bytes.Buffer

	// Tee stderr so we both stream it to the caller (if provided) and keep
	// a copy for classification.
	if stderr != nil {
		cmd.Stderr = io.MultiWriter(stderr, &errBuf)
	} else {
		cmd.Stderr = &errBuf
	}
	if stdout != nil {
		cmd.Stdout = stdout
	}

	err := cmd.Run()
	if err == nil {
		return nil
	}

	exitCode := -1
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		exitCode = exitErr.ExitCode()
	}

	stderrStr := errBuf.String()
	return &ExecError{
		Category: Classify(err, stderrStr, exitCode),
		ExitCode: exitCode,
		Stderr:   stderrStr,
		Inner:    err,
	}
}

// Classify maps an exec failure (err + captured stderr + exit code) onto an
// AuthErrorCategory. The rules are conservative: only signals we are
// confident about produce a non-None category.
//
//   - A missing binary (exec.ErrNotFound, possibly wrapped) →
//     AuthErrBackendMissing.
//   - gallery-dl's documented exit code 4 (authentication error), or any
//     stderr mentioning "login required", "HTTP Error 401", "HTTP Error
//     403", or "Authentication" (case-insensitive) → AuthErrAuthFailed.
//   - Stderr containing "429" or "rate limit" (case-insensitive) →
//     AuthErrRateLimited.
//   - Anything else → AuthErrNone (generic failure; caller decides).
func Classify(err error, stderr string, exitCode int) types.AuthErrorCategory {
	if err != nil && errors.Is(err, exec.ErrNotFound) {
		return types.AuthErrBackendMissing
	}

	lower := strings.ToLower(stderr)

	if exitCode == 4 ||
		strings.Contains(lower, "login required") ||
		strings.Contains(lower, "http error 401") ||
		strings.Contains(lower, "http error 403") ||
		strings.Contains(lower, "authentication") {
		return types.AuthErrAuthFailed
	}

	if strings.Contains(lower, "429") || strings.Contains(lower, "rate limit") {
		return types.AuthErrRateLimited
	}

	return types.AuthErrNone
}
