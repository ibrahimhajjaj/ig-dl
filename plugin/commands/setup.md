---
description: First-time ig-dl setup — checks binaries, browser CDP access, and captures a session
---

Walk the user through getting ig-dl ready to download Instagram content. Each step runs a real command via the Bash tool and reports its exit status before moving on.

**Step 1 — Check the `ig-dl` binary is on PATH.**
Run: `which ig-dl && ig-dl --help | head -5`
If not found, tell the user to install it: `go install github.com/ibrahimhajjaj/ig-dl/cmd/ig-dl@latest`. Stop.

**Step 2 — Check `gallery-dl` and `yt-dlp` are on PATH.**
Run: `which gallery-dl yt-dlp`
If either missing, suggest: `brew install gallery-dl yt-dlp`. Both are needed — gallery-dl for posts/stories/highlights/profiles, yt-dlp for reels.

**Step 3 — Show current session status.**
Run: `ig-dl status`
If "authed" already, skip to step 6. If "not authed", continue.

**Step 4 — Ask the user to enable CDP in their browser.**
Tell them exactly: "Open `chrome://inspect/#remote-debugging` (or `edge://inspect/#remote-debugging`) in your normal browser and toggle 'Allow remote debugging for this browser instance' ON. Make sure instagram.com is open in a tab and you're signed in there."

Wait for the user to confirm they did it.

**Step 5 — Verify ig-dl sees the live browser.**
Run: `ig-dl browsers`
If any entry shows `[live]`, proceed. If every entry is `[stale]`, tell the user the toggle didn't stick — they should try flipping it off and on again, or fall back to the companion extension path (step 7).

**Step 6 — Capture the session.**
Run: `ig-dl login`
Tell the user the browser will pop a permission dialog asking them to allow the remote debugging session — they must click **Allow**. On success, report "session captured from <browser>".

**Step 7 — If CDP didn't work, fall back to the companion extension.**
Point the user at the repo's `extension-companion/` directory, tell them to load it via `chrome://extensions → Developer mode → Load unpacked`, then click "Export session for CLI" in the extension's options page. Finally:
`ig-dl login --import ~/Downloads/ig-dl-session.json`

**Final step — Confirm.**
Run `ig-dl status` one more time and verify it reports `authed (source=<browser or imported>, age=...s)`. If so, the setup is complete and the user can now run `ig-dl <instagram-url>` or use any of the `ig_*` MCP tools.
