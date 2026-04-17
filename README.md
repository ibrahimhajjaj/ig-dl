# ig-dl

Instagram content downloader — CLI + MCP server in a single Go binary.

`ig-dl` piggybacks on your already-logged-in Chrome session (no login flow
in the CLI) and shells out to `gallery-dl` / `yt-dlp` for the actual media
fetching. The same binary also speaks the Model Context Protocol over
stdio, so Claude Code can drive it as tools.

## Why not just use `gallery-dl` or `yt-dlp` directly?

You *can*, and both are great. `ig-dl` wraps them to remove the
annoying parts and add a few things you'd otherwise build yourself:

- **Automatic session capture.** No hand-exporting cookies via a browser
  extension and `curl`-friendly cookie files. `ig-dl login` attaches to
  your running Chromium browser over CDP (including Chrome 144+'s
  `chrome://inspect/#remote-debugging` toggle, which works against your
  *real* default profile) and writes a fresh `cookies.txt` for the
  backends.
- **Smart routing.** Reels → `yt-dlp` (better for single videos).
  Posts, stories, highlights, saved, profile bulk → `gallery-dl`
  (better for images + multi-item extraction). One URL in, correct tool
  out.
- **Resilience.** Auth-error-driven session refresh + retry,
  exponential backoff on rate limits, CDP connect retry, age-based
  silent refresh — none of which `gallery-dl`/`yt-dlp` do for you.
- **Structured output.** `--json` gives the same shape as the MCP tool
  responses, so shell pipelines, CI jobs, and AI agents consume a
  single stable contract.
- **MCP server in the same binary.** `ig-dl mcp` exposes five tools
  (`ig_download_url`, `ig_download_user`, etc.) over stdio for Claude
  Code or any MCP client — no Node shim, no Python bridge.
- **Profile-bulk parallelism + per-stage layout.** Splits a profile
  into posts / stories / highlights, runs them through a bounded
  worker pool, writes into `<handle>/posts`, `<handle>/stories`,
  `<handle>/highlights`. gallery-dl's default layout doesn't give you
  that split.
- **Typed error categories.** Backend auth failure vs rate-limit vs
  missing binary are distinct exit codes and distinct MCP error
  categories, not one opaque error string.

If you only ever download single URLs occasionally, a direct
`gallery-dl --cookies ~/cookies.txt <url>` is fine. If you want all of
the above without writing the glue yourself, that's what `ig-dl` is.

## Install

```sh
# From source (requires Go 1.26+)
go install github.com/ibrahimhajjaj/ig-dl/cmd/ig-dl@latest

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
ig-dl login                 # capture session from running browser
ig-dl login --import <path> # import session.json from the companion extension
ig-dl logout                # clear cached session + cookies
ig-dl status                # show session state
ig-dl browsers              # list Chromium browsers with CDP enabled
ig-dl mcp                   # start MCP server on stdio
```

Global flags: `--out <dir>`, `--json`.

## Auth: getting a session

Chrome 136+ / Edge 136+ refuse `--remote-debugging-port` when pointed
at your default profile as a security hardening. There's no toggle or
policy that re-enables it against the default profile — Chrome's
official guidance is to use a custom `--user-data-dir`. That leaves
three workable paths:

### Path A — companion extension (recommended: real profile, no launch flags)

Install the sidecar in your normal browser once, click a button
whenever you need to refresh the session:

```
1. In your normal browser:
     edge://extensions (or chrome://extensions)
     enable "Developer mode"
     "Load unpacked" → select this repo's  extension-companion/  directory
2. Use Instagram normally. When you want to download, open the
   extension's options page and click "Export session for CLI".
3. ./ig-dl login --import ~/Downloads/ig-dl-session.json
```

Your real logins stay in your real browser. Only cost: one click per
session refresh.

### Path B — profile copy + `--remote-debugging-port`

Clones your real profile to an isolated dir so CDP works AND your IG
login comes along as a snapshot:

```sh
# Quit the target browser first (profile files are locked while open)
osascript -e 'quit app "Microsoft Edge"' ; sleep 2

# Copy real profile → isolated dir (~1-2 GB)
rm -rf "$HOME/.ig-dl/edge-profile"
cp -R "$HOME/Library/Application Support/Microsoft Edge" \
      "$HOME/.ig-dl/edge-profile"

# Launch with CDP against the copy
/Applications/Microsoft\ Edge.app/Contents/MacOS/Microsoft\ Edge \
  --remote-debugging-port=9222 \
  --user-data-dir="$HOME/.ig-dl/edge-profile" &

# Verify — should return JSON, not 404
sleep 2 && curl -s http://127.0.0.1:9222/json/version | head -3

./ig-dl login
```

To refresh, re-run the copy step. Changes inside the copy (new
bookmarks, saved passwords) don't sync back to your main browser.

### Path C — fresh profile + `--remote-debugging-port`

No copy. You log into Instagram once in the isolated profile; the
profile persists, so future launches stay logged in.

```sh
/Applications/Microsoft\ Edge.app/Contents/MacOS/Microsoft\ Edge \
  --remote-debugging-port=9222 \
  --user-data-dir="$HOME/.ig-dl/browser-profile" &
# In the new Edge window, open instagram.com and log in once, then:
./ig-dl login
```

### Discovery (all paths)

`ig-dl browsers` lists every browser whose user-data-dir contains a
`DevToolsActivePort` file. Files marked `[live]` mean a debug-capable
browser is currently answering on that port; `[stale]` means the file
is a leftover from a past debug session and can be ignored (ig-dl
won't connect to stale ports). This auto-detection works for Path B
and Path C regardless of which port the browser picked.

**Important:** Chromium-based browsers (since Chrome 136 / Edge 136, late
2025) silently ignore `--remote-debugging-port` when pointed at your
default user-data-dir, as a security hardening. Always pair the debug
port flag with a fresh `--user-data-dir` — you'll sign into Instagram
once inside that isolated profile and ig-dl attaches to it.

```sh
# macOS — Chrome
/Applications/Google\ Chrome.app/Contents/MacOS/Google\ Chrome \
  --remote-debugging-port=9222 \
  --user-data-dir="$HOME/.ig-dl/browser-profile" &

# macOS — Edge
/Applications/Microsoft\ Edge.app/Contents/MacOS/Microsoft\ Edge \
  --remote-debugging-port=9222 \
  --user-data-dir="$HOME/.ig-dl/browser-profile" &

# macOS — Brave
/Applications/Brave\ Browser.app/Contents/MacOS/Brave\ Browser \
  --remote-debugging-port=9222 \
  --user-data-dir="$HOME/.ig-dl/browser-profile" &
```

Inside the new browser window, open `instagram.com` and log in (it's a
fresh profile, so you won't carry over your main session — that's the
point). Then, in a terminal:

```sh
ig-dl login
```

Verify the debug port is actually listening — if this returns 404 or
nothing, the `--user-data-dir` flag is missing:

```sh
curl -s http://localhost:9222/json/version | head -3
```

Only one browser at a time can bind port 9222 — close other debug
sessions first, or override via `chrome_debug_port` in
`~/.ig-dl/config.toml`.

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
