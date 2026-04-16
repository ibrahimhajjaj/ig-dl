package core

import (
	"errors"
	"testing"
)

func TestClassify(t *testing.T) {
	cases := []struct {
		in   error
		want ErrorCategory
	}{
		{nil, ""},
		{errors.New("no session available"), ErrCategoryNoSession},
		{errors.New("ErrNoSession"), ErrCategoryNoSession},
		{errors.New(`exec: "gallery-dl": executable file not found in $PATH`), ErrCategoryBackendMissing},
		{errors.New("backend missing"), ErrCategoryBackendMissing},
		{errors.New("HTTP Error 401"), ErrCategoryAuthFailed},
		{errors.New("HTTP Error 403"), ErrCategoryAuthFailed},
		{errors.New("login required"), ErrCategoryAuthFailed},
		{errors.New("auth failed after refresh"), ErrCategoryAuthFailed},
		{errors.New("HTTP Error 429"), ErrCategoryRateLimited},
		{errors.New("rate limit exceeded"), ErrCategoryRateLimited},
		{errors.New("something totally else"), ErrCategoryGeneric},
	}
	for _, tc := range cases {
		name := "nil"
		if tc.in != nil {
			name = tc.in.Error()
		}
		t.Run(name, func(t *testing.T) {
			got := Classify(tc.in)
			if got != tc.want {
				t.Fatalf("Classify(%v) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestExitCode(t *testing.T) {
	cases := []struct {
		cat  ErrorCategory
		code int
	}{
		{"", 0},
		{ErrCategoryNoSession, 2},
		{ErrCategoryBackendMissing, 3},
		{ErrCategoryAuthFailed, 4},
		{ErrCategoryRateLimited, 5},
		{ErrCategoryGeneric, 1},
		{ErrorCategory("weird_unmapped"), 1},
	}
	for _, tc := range cases {
		t.Run(string(tc.cat), func(t *testing.T) {
			if got := ExitCode(tc.cat); got != tc.code {
				t.Fatalf("ExitCode(%q) = %d, want %d", tc.cat, got, tc.code)
			}
		})
	}
}
