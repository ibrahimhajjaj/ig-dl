package mcp

import (
	"context"
	"fmt"
	"strings"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// registerPrompts adds user-selectable prompt templates to the MCP
// server. Each prompt describes a common ig-dl workflow; the client
// picks one, supplies its arguments, and the server returns a
// ready-to-use message the LLM then executes (typically by calling one
// or more ig_* tools).
//
// Prompts sit alongside tools in the MCP model — tools are the
// imperative primitives, prompts are curated usage recipes.
func registerPrompts(srv *mcpsdk.Server) {
	srv.AddPrompt(&mcpsdk.Prompt{
		Name:        "download_url",
		Description: "Download a single Instagram URL (post, reel, story, or highlight) and briefly describe what landed on disk.",
		Arguments: []*mcpsdk.PromptArgument{
			{Name: "url", Description: "Instagram URL to download", Required: true},
			{Name: "out_dir", Description: "Optional output directory override"},
		},
	}, downloadURLPrompt)

	srv.AddPrompt(&mcpsdk.Prompt{
		Name:        "archive_profile",
		Description: "Back up an entire Instagram profile (posts, stories, highlights) and report per-stage counts.",
		Arguments: []*mcpsdk.PromptArgument{
			{Name: "handle", Description: "Instagram username (no @)", Required: true},
			{Name: "include", Description: "Comma-separated stages to limit to (posts, stories, highlights). Empty = all."},
			{Name: "out_dir", Description: "Optional output directory override"},
		},
	}, archiveProfilePrompt)

	srv.AddPrompt(&mcpsdk.Prompt{
		Name:        "session_health",
		Description: "Diagnose the current ig-dl session: auth state, browser discovery, and remediation if it's stale or missing.",
	}, sessionHealthPrompt)
}

func downloadURLPrompt(_ context.Context, req *mcpsdk.GetPromptRequest) (*mcpsdk.GetPromptResult, error) {
	url := req.Params.Arguments["url"]
	if url == "" {
		return nil, fmt.Errorf("missing required argument: url")
	}
	outDir := req.Params.Arguments["out_dir"]
	outClause := ""
	if outDir != "" {
		outClause = fmt.Sprintf(` (with out_dir=%q)`, outDir)
	}

	body := fmt.Sprintf(`Use the ig-dl MCP tools to download this Instagram URL and summarize the result.

URL: %s

Steps:
1. Call  ig_session_status  to confirm a valid session exists. If not, call  ig_login  (or prompt the user to run `+"`ig-dl login`"+` first).
2. Call  ig_download_url  with url=%q%s.
3. Inspect the structured result (out_dir, backend used, any failures) and summarize it in one or two sentences. Mention which backend ran (gallery-dl or yt-dlp) and where the file landed.

If the tool returns an error with category "auth_failed" or "no_session", tell the user to refresh their session and don't retry automatically.`,
		url, url, outClause)

	return &mcpsdk.GetPromptResult{
		Description: "Download a single Instagram URL via ig-dl",
		Messages: []*mcpsdk.PromptMessage{
			{Role: "user", Content: &mcpsdk.TextContent{Text: body}},
		},
	}, nil
}

func archiveProfilePrompt(_ context.Context, req *mcpsdk.GetPromptRequest) (*mcpsdk.GetPromptResult, error) {
	handle := req.Params.Arguments["handle"]
	if handle == "" {
		return nil, fmt.Errorf("missing required argument: handle")
	}
	include := req.Params.Arguments["include"]
	outDir := req.Params.Arguments["out_dir"]

	args := []string{fmt.Sprintf("handle=%q", handle)}
	if include != "" {
		args = append(args, fmt.Sprintf("include=%s", toJSONArray(include)))
	}
	if outDir != "" {
		args = append(args, fmt.Sprintf("out_dir=%q", outDir))
	}
	argStr := strings.Join(args, ", ")

	body := fmt.Sprintf(`Archive the Instagram profile @%s using the ig-dl MCP tools.

Steps:
1. Call  ig_session_status . If not authed or the session is very old, recommend the user re-authenticates via  ig-dl login  (don't call ig_login unless the user explicitly asks — it triggers a browser permission dialog).
2. Call  ig_download_user  with %s.
   The tool fans out into three parallel stages (posts, stories, highlights) that each write into their own subdirectory (<out_dir>/<handle>/<stage>/). A bounded worker pool handles concurrency.
3. When the tool returns, report:
     - out_dir — the top-level directory for this profile
     - per-stage success counts (from the  counts  map)
     - any failures (list them by stage and brief error)
     - the archive file path from  meta.archive  — re-running the prompt is cheap because gallery-dl skips already-downloaded items via that SQLite archive.

If auth_failed bubbles up, stop and tell the user to refresh. If rate_limited appears persistently, report it and suggest waiting a few minutes before retrying.`,
		handle, argStr)

	return &mcpsdk.GetPromptResult{
		Description: fmt.Sprintf("Archive Instagram profile @%s", handle),
		Messages: []*mcpsdk.PromptMessage{
			{Role: "user", Content: &mcpsdk.TextContent{Text: body}},
		},
	}, nil
}

func sessionHealthPrompt(_ context.Context, _ *mcpsdk.GetPromptRequest) (*mcpsdk.GetPromptResult, error) {
	body := `Diagnose the ig-dl authentication state and produce a short health report.

Steps:
1. Call  ig_session_status . Note whether  authed  is true, the  source  (imported / edge / chrome / fixed-port:<n>), and the  age_seconds .
2. Reason about the state:
     - Not authed → the user has no session. Recommend either (a) enabling chrome://inspect/#remote-debugging in their real browser and running ` + "`ig-dl login`" + `, or (b) using the companion extension's "Export for CLI" button and ` + "`ig-dl login --import <path>`" + `.
     - Authed but age_seconds > 86400 (24h) → session is stale; it'll be refreshed silently on the next command if a live browser is reachable. Otherwise suggest ` + "`ig-dl login`" + ` proactively.
     - Authed but age_seconds > 604800 (7d) → surface a warning; recommend ` + "`ig-dl login`" + ` before the next download.
     - Authed and fresh → all good; tell the user what they can run next (e.g. ` + "`ig-dl <url>`" + `, ` + "`ig-dl user <handle>`" + `).
3. Produce a 3-4 line human-readable summary. Do NOT call ig_login without explicit user consent — it triggers a permission dialog in the user's browser.`

	return &mcpsdk.GetPromptResult{
		Description: "Diagnose ig-dl session health",
		Messages: []*mcpsdk.PromptMessage{
			{Role: "user", Content: &mcpsdk.TextContent{Text: body}},
		},
	}, nil
}

// toJSONArray converts a comma-separated stage list like
// "posts,stories" into the JSON literal `["posts","stories"]` so it
// drops cleanly into the prompt body as a valid tool argument.
func toJSONArray(csv string) string {
	parts := strings.Split(csv, ",")
	quoted := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		quoted = append(quoted, fmt.Sprintf("%q", p))
	}
	return "[" + strings.Join(quoted, ",") + "]"
}
