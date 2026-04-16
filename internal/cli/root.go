// Package cli wires cobra commands on top of internal/core.
package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/ibrhajjaj/ig-dl/internal/config"
	"github.com/ibrhajjaj/ig-dl/internal/core"
	"github.com/spf13/cobra"
)

var (
	flagOutDir   string
	flagJSON     bool
	flagInclude  []string
	flagImport   string
)

// NewRoot builds the root `ig-dl` command tree.
func NewRoot() *cobra.Command {
	root := &cobra.Command{
		Use:   "ig-dl [flags] <url>",
		Short: "Download Instagram content from the terminal",
		Long: `ig-dl downloads Instagram posts, reels, stories, highlights, full profiles, and saved collections.

It piggybacks on your already-logged-in Chrome session (CDP attach on the debug port)
and shells out to gallery-dl or yt-dlp for actual media fetching.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return cmd.Help()
			}
			return runURL(cmd.Context(), args[0])
		},
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	root.PersistentFlags().StringVarP(&flagOutDir, "out", "o", "", "output directory (overrides config OutDir)")
	root.PersistentFlags().BoolVar(&flagJSON, "json", false, "emit structured JSON result to stdout (same schema as MCP tools)")

	root.AddCommand(newUserCmd())
	root.AddCommand(newSavedCmd())
	root.AddCommand(newLoginCmd())
	root.AddCommand(newLogoutCmd())
	root.AddCommand(newMCPCmd())
	root.AddCommand(newStatusCmd())
	root.AddCommand(newBrowsersCmd())
	return root
}

// Execute runs the root command. Returns an exit code per the
// spec's exit-code table.
func Execute() int {
	ctx, stop := signalCtx()
	defer stop()

	root := NewRoot()
	if err := root.ExecuteContext(ctx); err != nil {
		return exitCodeFor(err)
	}
	return 0
}

func loadOpts() (core.Options, error) {
	cfg, err := config.Load()
	if err != nil {
		return core.Options{}, err
	}
	return core.Options{
		Config: cfg,
		OutDir: flagOutDir,
		Stdout: os.Stdout,
		Stderr: os.Stderr,
	}, nil
}

func emitJSON(payload any) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(payload)
}

func emit(result *core.Result) error {
	if !flagJSON {
		if result == nil {
			return nil
		}
		fmt.Fprintf(os.Stdout, "\n→ output: %s\n", result.OutDir)
		if len(result.Failures) > 0 {
			fmt.Fprintf(os.Stderr, "failures: %d\n", len(result.Failures))
			for _, f := range result.Failures {
				fmt.Fprintf(os.Stderr, "  - %s\n", f)
			}
		}
		return nil
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(result)
}

func runURL(ctx context.Context, url string) error {
	opts, err := loadOpts()
	if err != nil {
		return err
	}
	res, err := core.DownloadURL(ctx, url, opts)
	if err != nil {
		return err
	}
	return emit(res)
}

func signalCtx() (context.Context, context.CancelFunc) {
	return context.WithCancel(context.Background())
}

// exitCodeFor maps a bubbled error to the spec's exit-code table via
// core.Classify, so the CLI and the MCP server share one taxonomy.
func exitCodeFor(err error) int {
	if err == nil {
		return 0
	}
	fmt.Fprintln(os.Stderr, err)
	return core.ExitCode(core.Classify(err))
}
