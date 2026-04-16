package cli

import (
	"context"
	"fmt"

	"github.com/ibrhajjaj/ig-dl/internal/session"
	"github.com/spf13/cobra"
)

func newBrowsersCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "browsers",
		Short: "Show which Chromium-based browsers have CDP enabled",
		Long: `Scans every known browser's user-data-dir for a DevToolsActivePort
file and reports which (if any) are currently serving CDP.

To enable CDP on your normal browser without relaunch:
  1. Open the browser
  2. Visit chrome://inspect/#remote-debugging (or edge://inspect/...)
  3. Toggle "Enable Remote Debugging" on
  4. Re-run this command — the browser should appear here`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runBrowsers(cmd.Context())
		},
	}
}

func runBrowsers(ctx context.Context) error {
	all := session.DiscoverAllActivePorts()
	if len(all) == 0 && !flagJSON {
		fmt.Println("no DevToolsActivePort files found.")
		fmt.Println("launch a Chromium browser with --remote-debugging-port + --user-data-dir; see README paths B/C.")
		return nil
	}
	for _, ap := range all {
		live := ap.IsLive(ctx)
		status := "stale"
		if live {
			status = "live"
		}
		if flagJSON {
			fmt.Printf(`{"browser":%q,"port":%d,"source":%q,"live":%t}`+"\n", ap.Browser, ap.Port, ap.Source, live)
		} else {
			fmt.Printf("  %-10s port=%d  [%s]  %s\n", ap.Browser, ap.Port, status, ap.Source)
		}
	}
	return nil
}
