package mcp

import (
	"context"
	"strings"
	"testing"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestDownloadURLPrompt_BuildsBody(t *testing.T) {
	ctx := context.Background()
	req := &mcpsdk.GetPromptRequest{
		Params: &mcpsdk.GetPromptParams{
			Arguments: map[string]string{
				"url":     "https://www.instagram.com/p/ABCDEF/",
				"out_dir": "/tmp/out",
			},
		},
	}
	res, err := downloadURLPrompt(ctx, req)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(res.Messages) != 1 {
		t.Fatalf("want 1 message, got %d", len(res.Messages))
	}
	text := res.Messages[0].Content.(*mcpsdk.TextContent).Text
	for _, want := range []string{
		"https://www.instagram.com/p/ABCDEF/",
		"ig_session_status",
		"ig_download_url",
		`"/tmp/out"`,
	} {
		if !strings.Contains(text, want) {
			t.Errorf("prompt body missing %q\n---\n%s", want, text)
		}
	}
}

func TestDownloadURLPrompt_RequiresURL(t *testing.T) {
	req := &mcpsdk.GetPromptRequest{Params: &mcpsdk.GetPromptParams{Arguments: map[string]string{}}}
	if _, err := downloadURLPrompt(context.Background(), req); err == nil {
		t.Fatal("want error for missing url, got nil")
	}
}

func TestArchiveProfilePrompt_IncludeFilter(t *testing.T) {
	req := &mcpsdk.GetPromptRequest{
		Params: &mcpsdk.GetPromptParams{
			Arguments: map[string]string{
				"handle":  "test_user",
				"include": "posts, stories",
			},
		},
	}
	res, err := archiveProfilePrompt(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	text := res.Messages[0].Content.(*mcpsdk.TextContent).Text
	if !strings.Contains(text, `include=["posts","stories"]`) {
		t.Errorf("include arg not JSON-quoted correctly:\n%s", text)
	}
	if !strings.Contains(text, "@test_user") {
		t.Error("handle missing from prompt body")
	}
}

func TestArchiveProfilePrompt_RequiresHandle(t *testing.T) {
	req := &mcpsdk.GetPromptRequest{Params: &mcpsdk.GetPromptParams{Arguments: map[string]string{}}}
	if _, err := archiveProfilePrompt(context.Background(), req); err == nil {
		t.Fatal("want error for missing handle, got nil")
	}
}

func TestSessionHealthPrompt_NoArgs(t *testing.T) {
	res, err := sessionHealthPrompt(context.Background(), &mcpsdk.GetPromptRequest{})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	text := res.Messages[0].Content.(*mcpsdk.TextContent).Text
	for _, want := range []string{
		"ig_session_status",
		"86400",    // 24h threshold
		"604800",   // 7d threshold
		"chrome://inspect/#remote-debugging",
		"companion extension",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("session-health prompt missing %q", want)
		}
	}
}

func TestToJSONArray(t *testing.T) {
	cases := []struct{ in, want string }{
		{"posts,stories", `["posts","stories"]`},
		{"  posts , stories , highlights  ", `["posts","stories","highlights"]`},
		{"posts", `["posts"]`},
		{"", `[]`},
		{",,,", `[]`},
	}
	for _, tc := range cases {
		if got := toJSONArray(tc.in); got != tc.want {
			t.Errorf("toJSONArray(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
