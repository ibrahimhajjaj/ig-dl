package mcp

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/ibrahimhajjaj/ig-dl/internal/core"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// --- Tool input/output schemas ---

type downloadURLArgs struct {
	URL    string `json:"url" jsonschema:"Instagram URL: post, reel, story, or highlight"`
	OutDir string `json:"out_dir,omitempty" jsonschema:"override config OutDir"`
}

type downloadURLsArgs struct {
	URLs   []string `json:"urls" jsonschema:"Array of Instagram URLs to download in one call. Use this instead of looping ig_download_url so the server can batch them through a worker pool."`
	OutDir string   `json:"out_dir,omitempty" jsonschema:"override config OutDir for the whole batch"`
}

type downloadUserArgs struct {
	Handle  string   `json:"handle" jsonschema:"Instagram username without the @"`
	OutDir  string   `json:"out_dir,omitempty"`
	Include []string `json:"include,omitempty" jsonschema:"stages to include: posts reels stories highlights"`
}

type downloadSavedArgs struct {
	OutDir string `json:"out_dir,omitempty"`
}

type sessionStatusArgs struct{}

type loginArgs struct {
	Import string `json:"import,omitempty" jsonschema:"path to session.json exported by the companion extension"`
}

type loginResult struct {
	Authed  bool   `json:"authed"`
	Source  string `json:"source,omitempty"`
	Message string `json:"message,omitempty"`
}

type statusResult struct {
	Authed     bool    `json:"authed"`
	Source     string  `json:"source,omitempty"`
	AgeSeconds float64 `json:"age_seconds,omitempty"`
}

// registerTools wires every ig-dl operation as an MCP tool on srv.
func registerTools(srv *mcpsdk.Server, baseOpts core.Options) {
	mcpsdk.AddTool(srv, &mcpsdk.Tool{
		Name:        "ig_download_url",
		Description: "Download a single Instagram post, reel, story, or highlight by URL.",
	}, func(ctx context.Context, req *mcpsdk.CallToolRequest, args downloadURLArgs) (*mcpsdk.CallToolResult, *core.Result, error) {
		opts := withOutDir(baseOpts, args.OutDir)
		attachProgress(&opts, req)
		res, err := core.DownloadURL(ctx, args.URL, opts)
		return toolResult(res, err)
	})

	mcpsdk.AddTool(srv, &mcpsdk.Tool{
		Name:        "ig_download_urls",
		Description: "Download multiple Instagram URLs in one call. Strongly preferred over looping ig_download_url — the server runs them through a bounded worker pool and only attaches to the session once.",
	}, func(ctx context.Context, req *mcpsdk.CallToolRequest, args downloadURLsArgs) (*mcpsdk.CallToolResult, *core.Result, error) {
		opts := withOutDir(baseOpts, args.OutDir)
		attachProgress(&opts, req)
		res, err := core.DownloadURLs(ctx, args.URLs, opts)
		return toolResult(res, err)
	})

	mcpsdk.AddTool(srv, &mcpsdk.Tool{
		Name:        "ig_download_user",
		Description: "Download a full Instagram profile (posts, reels, stories, highlights).",
	}, func(ctx context.Context, req *mcpsdk.CallToolRequest, args downloadUserArgs) (*mcpsdk.CallToolResult, *core.Result, error) {
		opts := withOutDir(baseOpts, args.OutDir)
		attachProgress(&opts, req)
		res, err := core.DownloadUser(ctx, args.Handle, args.Include, opts)
		return toolResult(res, err)
	})

	mcpsdk.AddTool(srv, &mcpsdk.Tool{
		Name:        "ig_download_saved",
		Description: "Download the authenticated user's saved collection.",
	}, func(ctx context.Context, req *mcpsdk.CallToolRequest, args downloadSavedArgs) (*mcpsdk.CallToolResult, *core.Result, error) {
		opts := withOutDir(baseOpts, args.OutDir)
		attachProgress(&opts, req)
		res, err := core.DownloadSaved(ctx, opts)
		return toolResult(res, err)
	})

	mcpsdk.AddTool(srv, &mcpsdk.Tool{
		Name:        "ig_session_status",
		Description: "Report whether a valid Instagram session is cached, its age and source.",
	}, func(ctx context.Context, req *mcpsdk.CallToolRequest, _ sessionStatusArgs) (*mcpsdk.CallToolResult, *statusResult, error) {
		authed, age, source, err := core.SessionStatus(ctx, baseOpts)
		if err != nil {
			return errorResult(err), nil, nil
		}
		out := &statusResult{Authed: authed, Source: source, AgeSeconds: age}
		return okResult(out), out, nil
	})

	mcpsdk.AddTool(srv, &mcpsdk.Tool{
		Name:        "ig_login",
		Description: "Capture a fresh session from a running Chrome (debug port), or import session.json from the companion extension.",
	}, func(ctx context.Context, req *mcpsdk.CallToolRequest, args loginArgs) (*mcpsdk.CallToolResult, *loginResult, error) {
		if args.Import != "" {
			if err := core.ImportSession(baseOpts, args.Import); err != nil {
				return errorResult(err), nil, nil
			}
			out := &loginResult{Authed: true, Source: "imported", Message: "session imported"}
			return okResult(out), out, nil
		}
		source, err := core.Login(ctx, baseOpts)
		if err != nil {
			return errorResult(err), nil, nil
		}
		out := &loginResult{Authed: true, Source: source, Message: fmt.Sprintf("session captured from %s", source)}
		return okResult(out), out, nil
	})
}

func withOutDir(base core.Options, override string) core.Options {
	if override != "" {
		base.OutDir = override
	}
	return base
}

func attachProgress(opts *core.Options, req *mcpsdk.CallToolRequest) {
	token := req.Params.GetProgressToken()
	if token == nil {
		return
	}
	opts.Progress = func(stage string, cur, total float64, msg string) {
		_ = req.Session.NotifyProgress(context.Background(), &mcpsdk.ProgressNotificationParams{
			ProgressToken: token,
			Message:       fmt.Sprintf("[%s] %s", stage, msg),
			Progress:      cur,
			Total:         total,
		})
	}
}

// toolResult shapes a core.Result + error into the MCP dual return.
func toolResult(res *core.Result, err error) (*mcpsdk.CallToolResult, *core.Result, error) {
	if err != nil {
		return errorResult(err), nil, nil
	}
	return okResult(res), res, nil
}

func okResult(payload any) *mcpsdk.CallToolResult {
	blob, _ := json.MarshalIndent(payload, "", "  ")
	return &mcpsdk.CallToolResult{
		Content: []mcpsdk.Content{&mcpsdk.TextContent{Text: string(blob)}},
	}
}

func errorResult(err error) *mcpsdk.CallToolResult {
	cat := core.Classify(err)
	payload := map[string]string{
		"category": string(cat),
		"message":  err.Error(),
	}
	blob, _ := json.Marshal(payload)
	return &mcpsdk.CallToolResult{
		IsError: true,
		Content: []mcpsdk.Content{&mcpsdk.TextContent{Text: string(blob)}},
	}
}
