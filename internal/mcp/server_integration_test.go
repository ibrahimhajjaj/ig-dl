//go:build integration

package mcp

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/ibrhajjaj/ig-dl/internal/config"
	"github.com/ibrhajjaj/ig-dl/internal/core"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// TestMCP_InMemoryStatus spins up the ig-dl MCP server against an
// in-memory transport, calls ig_session_status (which never touches
// the network when no session file exists), and asserts the schema.
//
// Run with: go test -tags=integration ./internal/mcp/...
func TestMCP_InMemoryStatus(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Use an isolated config dir so we don't touch ~/.ig-dl.
	tmp := t.TempDir()
	cfg := config.Default()
	cfg.ConfigDir = tmp
	cfg.SessionPath = tmp + "/session.json"
	cfg.CookiesPath = tmp + "/cookies.txt"
	cfg.ArchiveDir = tmp + "/archive"

	srv := NewServer(core.Options{Config: cfg})
	client := mcpsdk.NewClient(&mcpsdk.Implementation{Name: "test-client", Version: "0.0.1"}, nil)

	t1, t2 := mcpsdk.NewInMemoryTransports()
	if _, err := srv.Connect(ctx, t1, nil); err != nil {
		t.Fatalf("server connect: %v", err)
	}
	cs, err := client.Connect(ctx, t2, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	defer cs.Close()

	res, err := cs.CallTool(ctx, &mcpsdk.CallToolParams{
		Name:      "ig_session_status",
		Arguments: map[string]any{},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if res.IsError {
		t.Fatalf("status tool returned IsError; content: %+v", res.Content)
	}
	tc, ok := res.Content[0].(*mcpsdk.TextContent)
	if !ok {
		t.Fatalf("content type = %T", res.Content[0])
	}
	if !strings.Contains(tc.Text, `"authed"`) {
		t.Fatalf("payload missing authed field: %s", tc.Text)
	}
}
