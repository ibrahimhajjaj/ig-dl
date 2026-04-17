// Package mcp exposes ig-dl's core operations as MCP tools over stdio.
package mcp

import (
	"context"

	"github.com/ibrahimhajjaj/ig-dl/internal/core"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// RunStdio boots the MCP server on stdin/stdout and blocks until the
// client disconnects or the context is cancelled.
func RunStdio(ctx context.Context, opts core.Options) error {
	srv := NewServer(opts)
	return srv.Run(ctx, &mcpsdk.StdioTransport{})
}

// NewServer constructs an MCP server with every ig-dl tool registered.
// Exported so integration tests can connect via in-memory transports.
func NewServer(opts core.Options) *mcpsdk.Server {
	srv := mcpsdk.NewServer(&mcpsdk.Implementation{
		Name:    "ig-dl",
		Version: "0.1.4",
	}, nil)
	registerTools(srv, opts)
	registerPrompts(srv)
	return srv
}
