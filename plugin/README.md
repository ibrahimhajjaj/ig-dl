# ig-dl (Claude Code plugin)

Wraps the `ig-dl` CLI as a Claude Code plugin. Installing this plugin
registers the `ig-dl` MCP server (5 tools + 3 prompt templates) plus a
couple of setup/diagnose slash commands.

## Prerequisites

`ig-dl` is not bundled with the plugin — Claude Code plugins don't
auto-download Go binaries. Install it separately, first:

```sh
go install github.com/ibrahimhajjaj/ig-dl/cmd/ig-dl@latest
# plus the external downloaders:
brew install gallery-dl yt-dlp
```

Verify `which ig-dl gallery-dl yt-dlp` all resolve before installing
the plugin.

## Install

Once the repo is public on GitHub:

```
/plugin marketplace add ibrahimhajjaj/ig-dl
/plugin install ig-dl@ig-dl-marketplace
```

Local (from this repo, before pushing):

```sh
cd /path/to/ig-downloader
claude plugin install ./plugin
```

The marketplace manifest lives at the repo root
(`.claude-plugin/marketplace.json`); the plugin itself lives at
`./plugin/`. Both are referenced by the commands above automatically.

## What you get

**MCP tools** (callable by the LLM directly):

| Tool | Purpose |
|---|---|
| `ig_download_url` | Single post / reel / story / highlight |
| `ig_download_user` | Full profile with per-stage counts |
| `ig_download_saved` | Your own saved collection |
| `ig_session_status` | Is the session authed? How old? |
| `ig_login` | Capture a fresh session (triggers browser permission dialog) |

**MCP prompts** (user-selectable templates):

- `/ig-dl:download_url` — pick a URL, get a guided download + summary
- `/ig-dl:archive_profile` — back up a whole profile with per-stage reporting
- `/ig-dl:session_health` — diagnose auth state, recommend remediation

**Slash commands**:

- `/ig-dl:setup` — first-time guided setup (binaries → CDP toggle → login)
- `/ig-dl:diagnose` — health check across auth, browsers, backends

**Skill**:

- `using-ig-dl` — activates whenever the user mentions Instagram
  URLs/downloads, biases Claude toward the MCP tools over ad-hoc
  shell-outs.

## File layout

```
plugin/
├── .claude-plugin/plugin.json   # plugin metadata
├── .mcp.json                    # MCP server config (spawns `ig-dl mcp`)
├── commands/
│   ├── setup.md
│   └── diagnose.md
├── skills/
│   └── using-ig-dl/SKILL.md
└── README.md                    # this file
```

## Notes

- The plugin version tracks the `ig-dl` binary version (currently 0.1.5).
- The MCP command entry uses `${HOME}/go/bin/ig-dl` (the default
  `go install` location) rather than bare `ig-dl` because Claude Code's
  MCP runner inherits the PATH from when the app launched — if you
  added `~/go/bin` to your shell after launching Claude Code, a bare
  `ig-dl` lookup will fail. The absolute form sidesteps that. If you
  installed the binary somewhere else (e.g. `/usr/local/bin/ig-dl`
  from a downloaded release), edit `.mcp.json` after install.
- gallery-dl and yt-dlp lookups still rely on inherited PATH —
  `/opt/homebrew/bin` is typically already there from Claude Code's
  launch env, so no extra config needed for Homebrew users.
- Uninstalling the plugin removes the MCP registration + slash commands
  but leaves `ig-dl` itself and `~/.ig-dl/` alone.
