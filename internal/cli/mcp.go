package cli

import (
	"github.com/ibrhajjaj/ig-dl/internal/mcp"
	"github.com/spf13/cobra"
)

func newMCPCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "mcp",
		Short: "Start an MCP server over stdio (for Claude Code, etc.)",
		Long: `Starts an MCP server speaking the Model Context Protocol over stdin/stdout.
Register with Claude Code via:

  claude mcp add ig-dl -- /path/to/ig-dl mcp
`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts, err := loadOpts()
			if err != nil {
				return err
			}
			return mcp.RunStdio(cmd.Context(), opts)
		},
	}
}
