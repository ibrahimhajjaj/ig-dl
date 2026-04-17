---
name: using-ig-dl
description: Use when the user asks to download something from Instagram, archive an Instagram profile, extract an IG reel/story/post, or mentions Instagram URLs. Prefer ig-dl's MCP tools and prompts over ad-hoc curl/yt-dlp invocations.
---

# Using ig-dl

The `ig-dl` plugin ships an MCP server exposing five tools and three curated prompts. When the user wants to grab Instagram content, use these primitives — don't hand-roll `curl` or raw `gallery-dl` calls, and don't ask the user for cookies.

## Decision order

1. **Does the user want a single Instagram URL downloaded?** → Either call the `ig_download_url` tool directly (if you're mid-task) or suggest the `/ig-dl:download_url` prompt (if the user is just starting).
2. **Does the user want to archive a whole profile?** → `ig_download_user` tool, or the `/ig-dl:archive_profile` prompt.
3. **Does the user want their saved collection?** → `ig_download_saved` tool.
4. **Is the user reporting "it's not working"?** → First call `ig_session_status`, then use the `/ig-dl:session_health` prompt or the `/ig-dl:diagnose` command to investigate before trying downloads again.

## Tool contract (stable across CLI `--json` and MCP)

Every successful download returns:

```json
{
  "out_dir": "…",
  "counts": { "stage_name": 1, … },
  "failures": [],
  "handle": "…",   // profile/saved operations only
  "meta": { "backend": "gallery-dl"|"yt-dlp", … }
}
```

On failure the MCP server returns `IsError: true` with a structured payload:

```json
{ "category": "no_session"|"auth_failed"|"backend_missing"|"rate_limited"|"generic_failure", "message": "…" }
```

## Do NOT

- **Do not call `ig_login` without explicit user consent** — it triggers a permission dialog in the user's running browser and should only fire when the user directly asks you to authenticate.
- **Do not auto-retry on `auth_failed` or `no_session`.** ig-dl already does one refresh-and-retry internally for auth failures; if it still bubbles up, the user needs to act (login or check browser CDP toggle). Tell them.
- **Do not suggest manually exporting cookies or editing `~/.ig-dl/session.json`.** The companion extension's "Export for CLI" button or `ig-dl login` are the only supported session sources.
- **Do not parse backend stdout.** The tools already classify results; use the structured JSON.

## Setup nudge

If the user hasn't set up ig-dl yet (e.g. `which ig-dl` fails or `ig-dl status` says "not authed"), point them at the `/ig-dl:setup` slash command rather than walking them through it ad-hoc.
