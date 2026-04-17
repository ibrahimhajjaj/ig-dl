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

Three supported paths. Pick the first that fits your setup.

### Path 1 (recommended) — `chrome://inspect/#remote-debugging` toggle

**Requires Chrome 144+ / Edge 144+. No relaunch, works against your real
default profile, keeps all your logins.**

```
1. In your normal browser (Chrome, Edge, Brave, etc.):
   visit:  chrome://inspect/#remote-debugging
           (or edge://inspect/#remote-debugging)
2. Toggle "Allow remote debugging for this browser instance" ON
3. Make sure instagram.com is open in a tab and you're signed in
4. ./ig-dl login
   (the browser pops a permission dialog — click "Allow")
5. → "session captured from edge"  (or chrome, brave, ...)
```

`ig-dl` reads the browser's `DevToolsActivePort` file, connects via the
exact WebSocket path the toggle registered, and captures cookies +
rotating IG headers from your live tab. To reset the authorization,
toggle the switch off and on again.

Verify with `ig-dl browsers` — the browser should appear as `[live]`.

### Path 2 — companion extension (works on any Chromium, any version)

Drop-in fallback if your browser is pre-M144 (older Chrome/Edge), or
you'd rather not flip the inspect toggle.

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

### Path 3 — `--remote-debugging-port` + `--user-data-dir` (automation / CI)

Use when you want CDP without depending on a human flipping the Path 1
toggle — e.g. scripted runs or headless setups. Chrome 136+ requires
`--user-data-dir` with this flag; it will silently ignore
`--remote-debugging-port` on the default profile.

```sh
# Fresh profile — you sign into IG once, profile persists between runs
/Applications/Microsoft\ Edge.app/Contents/MacOS/Microsoft\ Edge \
  --remote-debugging-port=9222 \
  --user-data-dir="$HOME/.ig-dl/browser-profile" &

# Optional — clone your real profile once so you're pre-logged-in:
#   osascript -e 'quit app "Microsoft Edge"' && sleep 2
#   rm -rf  "$HOME/.ig-dl/browser-profile"
#   cp -R   "$HOME/Library/Application Support/Microsoft Edge" \
#           "$HOME/.ig-dl/browser-profile"
#   (then launch as above)

# Verify the debug endpoint answered (should return JSON, not 404)
sleep 2 && curl -s http://127.0.0.1:9222/json/version | head -3

./ig-dl login
```

Chrome/Brave launch commands are identical — swap the app bundle path.

### `ig-dl browsers`

Lists every browser whose user-data-dir contains a `DevToolsActivePort`
file. `[live]` = a debug-capable browser is currently answering; `[stale]`
= leftover file from a past debug session (ig-dl won't connect to it).
Handles both classic `--remote-debugging-port` shapes (line 1 port only)
and the M144 toggle shape (line 1 port + line 2 WebSocket path).

## Claude Code integration (MCP)

```sh
claude mcp add ig-dl -- $(go env GOPATH)/bin/ig-dl mcp
```

**Tools** (callable primitives): `ig_download_url`, `ig_download_user`,
`ig_download_saved`, `ig_session_status`, `ig_login`.

**Prompts** (user-selectable workflow templates in MCP clients that
surface them, e.g. `/ig-dl:download_url`):

- `download_url` — pick a URL, have the LLM check auth + download + summarize.
- `archive_profile` — back up a full profile, report per-stage counts.
- `session_health` — diagnose current auth state and propose remediation.

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
