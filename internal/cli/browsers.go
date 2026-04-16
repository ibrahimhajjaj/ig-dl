package cli

import (
	"context"
	"fmt"
	"net/http"
	"time"

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
	client := &http.Client{Timeout: 1 * time.Second}
	found := 0
	for _, b := range session.KnownBrowsers() {
		ap, err := session.DiscoverActivePort(b)
		if err != nil || ap == nil || ap.Browser != b {
			// DiscoverActivePort with a preferred browser still scans all;
			// skip entries that resolved to a different browser than we asked.
			continue
		}
		found++
		live := probeLive(ctx, client, ap.Port)
		status := "stale"
		if live {
			status = "live"
		}
		if flagJSON {
			// one JSON object per line
			fmt.Printf(`{"browser":%q,"port":%d,"source":%q,"live":%t}`+"\n", ap.Browser, ap.Port, ap.Source, live)
		} else {
			fmt.Printf("  %-10s port=%d  [%s]  %s\n", ap.Browser, ap.Port, status, ap.Source)
		}
	}
	if found == 0 && !flagJSON {
		fmt.Println("no browser has CDP enabled — toggle chrome://inspect/#remote-debugging in your browser")
	}
	return nil
}

func probeLive(ctx context.Context, client *http.Client, port int) bool {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		fmt.Sprintf("http://127.0.0.1:%d/json/version", port), nil)
	if err != nil {
		return false
	}
	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}
