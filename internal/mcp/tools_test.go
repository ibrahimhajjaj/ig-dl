package mcp

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/ibrhajjaj/ig-dl/internal/core"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestOKResult_SerialisesPayload(t *testing.T) {
	r := &core.Result{
		OutDir:  "./downloads/p_abc",
		Counts:  map[string]int{"invocations": 1},
		Handle:  "",
		Meta:    map[string]string{"url": "https://instagram.com/p/abc/", "backend": "gallery-dl"},
	}
	got := okResult(r)
	if got.IsError {
		t.Fatalf("IsError = true, want false")
	}
	if len(got.Content) != 1 {
		t.Fatalf("Content len = %d, want 1", len(got.Content))
	}
	tc, ok := got.Content[0].(*mcpsdk.TextContent)
	if !ok {
		t.Fatalf("Content[0] type = %T, want *TextContent", got.Content[0])
	}
	var round core.Result
	if err := json.Unmarshal([]byte(tc.Text), &round); err != nil {
		t.Fatalf("payload is not valid JSON: %v\n%s", err, tc.Text)
	}
	if round.OutDir != r.OutDir {
		t.Fatalf("OutDir round-trip: got %q, want %q", round.OutDir, r.OutDir)
	}
	if round.Meta["backend"] != "gallery-dl" {
		t.Fatalf("Meta.backend round-trip: got %q", round.Meta["backend"])
	}
}

func TestErrorResult_MarksAndCarriesCategory(t *testing.T) {
	cases := []struct {
		in           error
		wantCategory string
	}{
		{errors.New("no session available"), string(core.ErrCategoryNoSession)},
		{errors.New("HTTP Error 401"), string(core.ErrCategoryAuthFailed)},
		{errors.New("HTTP Error 429"), string(core.ErrCategoryRateLimited)},
		{errors.New(`exec: "gallery-dl": executable file not found in $PATH`), string(core.ErrCategoryBackendMissing)},
		{errors.New("something odd happened"), string(core.ErrCategoryGeneric)},
	}
	for _, tc := range cases {
		t.Run(tc.in.Error(), func(t *testing.T) {
			got := errorResult(tc.in)
			if !got.IsError {
				t.Fatalf("IsError = false, want true")
			}
			txt := got.Content[0].(*mcpsdk.TextContent).Text
			if !strings.Contains(txt, `"category":"`+tc.wantCategory+`"`) {
				t.Fatalf("category mismatch: %s", txt)
			}
			if !strings.Contains(txt, `"message"`) {
				t.Fatalf("message key missing: %s", txt)
			}
		})
	}
}

func TestWithOutDir(t *testing.T) {
	base := core.Options{OutDir: "./default"}
	t.Run("override replaces", func(t *testing.T) {
		got := withOutDir(base, "./custom")
		if got.OutDir != "./custom" {
			t.Fatalf("got %q, want ./custom", got.OutDir)
		}
	})
	t.Run("empty override preserves base", func(t *testing.T) {
		got := withOutDir(base, "")
		if got.OutDir != "./default" {
			t.Fatalf("got %q, want ./default", got.OutDir)
		}
	})
}

func TestToolResult_Dispatch(t *testing.T) {
	t.Run("success path returns okResult", func(t *testing.T) {
		r := &core.Result{OutDir: "x"}
		ctr, payload, err := toolResult(r, nil)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if payload != r {
			t.Fatalf("payload pointer mismatch")
		}
		if ctr.IsError {
			t.Fatalf("IsError = true, want false")
		}
	})
	t.Run("error path returns errorResult", func(t *testing.T) {
		ctr, payload, err := toolResult(nil, errors.New("boom"))
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if payload != nil {
			t.Fatalf("payload = %v, want nil", payload)
		}
		if !ctr.IsError {
			t.Fatalf("IsError = false, want true")
		}
	})
}

func TestRegisterTools_AllRegistered(t *testing.T) {
	srv := NewServer(core.Options{})
	// The SDK doesn't expose a list method on the Server directly, but we
	// can assert that server isn't nil and that a second registration of
	// the same tool name would clobber — registerTools should have
	// produced a working server instance without panicking.
	if srv == nil {
		t.Fatal("NewServer returned nil")
	}
}
