# ig-dl

Instagram content downloader — CLI + MCP server in a single Go binary.

`ig-dl` piggybacks on your already-logged-in Chrome session (no login flow
in the CLI) and shells out to `gallery-dl` / `yt-dlp` for the actual media
fetching. The same binary also speaks the Model Context Protocol over
stdio, so Claude Code can drive it as tools.

## Install

```sh
# From source (requires Go 1.26+)
go install github.com/ibrhajjaj/ig-dl/cmd/ig-dl@latest

# Or build locally
make install
```

External binaries needed on `PATH`:

```sh
brew install gallery-dl yt-dlp
```

## Usage

```sh
ig-dl <url>                 # single post, reel, story, or highlight
ig-dl user <handle>         # all content for a profile
ig-dl saved                 # your saved collection
ig-dl login                 # capture session from running Chrome
ig-dl login --import <path> # import session.json from the companion extension
ig-dl logout                # clear cached session + cookies
ig-dl status                # show session state
ig-dl mcp                   # start MCP server on stdio
```

Global flags: `--out <dir>`, `--json`.

## Auth: getting a session

### Primary path — attach to a running Chromium-based browser

Any browser that speaks the Chrome DevTools Protocol works: **Chrome**,
**Edge**, **Brave**, **Arc**, **Vivaldi**, **Chromium**. Launch it with the
remote debugging port open, log into Instagram, then run `ig-dl login`:

```sh
# macOS — Chrome
/Applications/Google\ Chrome.app/Contents/MacOS/Google\ Chrome \
  --remote-debugging-port=9222 &

# macOS — Edge
/Applications/Microsoft\ Edge.app/Contents/MacOS/Microsoft\ Edge \
  --remote-debugging-port=9222 &

# macOS — Brave
/Applications/Brave\ Browser.app/Contents/MacOS/Brave\ Browser \
  --remote-debugging-port=9222 &

# Open instagram.com, log in, then:
ig-dl login
```

Only one browser can bind port 9222 at a time — close other debug sessions
first, or override via `chrome_debug_port` in `~/.ig-dl/config.toml`.

### Fallback path — companion extension

If you can't run Chrome with `--remote-debugging-port`, install the
companion extension in `extension-companion/` (`chrome://extensions` → Load
unpacked). After using Instagram normally, open the extension's options
page and click **Export session for CLI**. Then:

```sh
ig-dl login --import ~/Downloads/ig-dl-session.json
```

## Claude Code integration (MCP)

```sh
claude mcp add ig-dl -- $(go env GOPATH)/bin/ig-dl mcp
```

Five tools are exposed: `ig_download_url`, `ig_download_user`,
`ig_download_saved`, `ig_session_status`, `ig_login`.

## Config

`~/.ig-dl/config.toml` (all fields optional; defaults shown):

```toml
out_dir = "./downloads"
concurrency = 3
chrome_debug_port = 9222
stale_after = "24h"
warn_after = "168h"

[backend]
gallery_dl_path = "gallery-dl"
yt_dlp_path = "yt-dlp"
```

## Development

```sh
make build           # build ./ig-dl
make test            # go test ./...
make test-integration  # go test -tags=integration (needs chrome + backends)
make lint            # vet + staticcheck
make smoke HANDLE=test.account POST_URL=https://...  # manual E2E
```

## Status

v0.1 — all packages wired, all unit tests green, integration tests behind
`-tags=integration`. Smoke checklist in `scripts/smoke.sh`.
