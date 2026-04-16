# ig-dl

Instagram content downloader — CLI + MCP server in a single Go binary.

`ig-dl` piggybacks on your already-logged-in Chrome session (no login flow
in the CLI) and delegates actual media fetching to `gallery-dl` and
`yt-dlp`.

- `ig-dl <url>` — download a single post / reel / story / highlight
- `ig-dl user <handle>` — download everything for a profile
- `ig-dl saved` — download your Saved collection
- `ig-dl login` — refresh the session from your running Chrome
- `ig-dl mcp` — start an MCP server over stdio (for Claude Code et al.)

Status: scaffolding. See `docs/superpowers/specs/` (local-only) for the
design doc driving implementation.
