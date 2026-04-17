// Command ig-dl is a CLI + MCP server for downloading Instagram content.
package main

import (
	"os"

	"github.com/ibrahimhajjaj/ig-dl/internal/cli"
)

func main() {
	os.Exit(cli.Execute())
}
