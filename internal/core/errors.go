package core

import "strings"

// ErrorCategory classifies a failure into one of the stable categories
// the CLI and MCP both surface. The zero value means "generic failure".
type ErrorCategory string

const (
	ErrCategoryGeneric        ErrorCategory = "generic_failure"
	ErrCategoryNoSession      ErrorCategory = "no_session"
	ErrCategoryBackendMissing ErrorCategory = "backend_missing"
	ErrCategoryAuthFailed     ErrorCategory = "auth_failed"
	ErrCategoryRateLimited    ErrorCategory = "rate_limited"
)

// Classify inspects an error and returns the structured category the
// front-ends surface. Both the CLI (for exit codes) and the MCP server
// (for structured error payloads) use the same table.
func Classify(err error) ErrorCategory {
	if err == nil {
		return ""
	}
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "no session") ||
		strings.Contains(msg, "no usable session") ||
		strings.Contains(msg, "errnosession") ||
		strings.Contains(msg, "session.json"):
		return ErrCategoryNoSession
	case strings.Contains(msg, "executable file not found") ||
		strings.Contains(msg, "backend missing") ||
		strings.Contains(msg, "not on path"):
		return ErrCategoryBackendMissing
	case strings.Contains(msg, "auth failed") ||
		strings.Contains(msg, "login required") ||
		strings.Contains(msg, "401") ||
		strings.Contains(msg, "403"):
		return ErrCategoryAuthFailed
	case strings.Contains(msg, "rate limit") || strings.Contains(msg, "429"):
		return ErrCategoryRateLimited
	}
	return ErrCategoryGeneric
}

// ExitCode maps an ErrorCategory to the spec's CLI exit-code table.
func ExitCode(cat ErrorCategory) int {
	switch cat {
	case "":
		return 0
	case ErrCategoryNoSession:
		return 2
	case ErrCategoryBackendMissing:
		return 3
	case ErrCategoryAuthFailed:
		return 4
	case ErrCategoryRateLimited:
		return 5
	}
	return 1
}
