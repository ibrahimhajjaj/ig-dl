---
description: Diagnose why ig-dl isn't working — checks auth, browsers, and backend availability
---

Produce a concise health report for the user's ig-dl setup. Run each check, collect the output, and at the end synthesize a 4-6 line summary with the exact remediation step if anything's off.

**Checks:**

1. `ig-dl status` — authentication state.
2. `ig-dl browsers` — which Chromium browsers are exposing CDP, which are live vs stale.
3. `which ig-dl gallery-dl yt-dlp` — are all three binaries on PATH?
4. `ls -la ~/.ig-dl/ 2>/dev/null` — does the config directory exist and what's in it?
5. If `~/.ig-dl/session.json` exists, run `stat -f "%Sm" ~/.ig-dl/session.json` (macOS) or `stat -c %y ~/.ig-dl/session.json` (linux) to get its last-modified timestamp.

**Synthesis:**

- If `status` reports "not authed" and no `[live]` browser in step 2 → user needs to enable `chrome://inspect/#remote-debugging` in a real browser, then run `ig-dl login`.
- If `status` is authed but age > 24h → it'll auto-refresh on the next command if a live browser is around; otherwise suggest `ig-dl login` proactively.
- If `status` is authed but age > 7d → warn that the session is very old; recommend `ig-dl login` before attempting a download.
- If any backend binary is missing → `brew install gallery-dl yt-dlp` (or platform equivalent).
- If `ig-dl` itself is missing → `go install github.com/ibrahimhajjaj/ig-dl/cmd/ig-dl@latest`.

Do NOT run `ig-dl login` on the user's behalf — it triggers a permission dialog in their browser and should be user-initiated.
