# ig-dl session exporter

Companion Chrome extension for the [`ig-dl`](../) CLI. It is the **fallback
auth path** the CLI uses when it cannot attach to a running Chrome instance
over CDP.

It does exactly one thing: capture your current Instagram session (cookies +
rotating API headers) and let you export it as a JSON file that the CLI can
import.

## Install

1. Open `chrome://extensions` in Chrome (or any Chromium-based browser).
2. Enable **Developer mode** (toggle in the top-right).
3. Click **Load unpacked** and pick this `extension-companion/` directory.
4. The extension "ig-dl session exporter" appears in the list. No pinned
   toolbar button — everything happens on the options page.

## Use

1. Open [instagram.com](https://www.instagram.com/) and confirm you are
   logged in.
2. Browse normally for a few seconds — visit your feed, a profile, a reel.
   The extension's background service worker silently observes the requests
   your browser is already making and records the rotating headers
   (`x-ig-www-claim`, `x-ig-app-id`, `x-asbd-id`, `x-instagram-ajax`) plus
   any `query_hash` / `doc_id` values.
3. Open the extension's options page:
   `chrome://extensions` → find "ig-dl session exporter" → **Details** →
   **Extension options**. (Or right-click the extension icon if pinned →
   **Options**.)
4. Click **Export session for CLI**. A file named `ig-dl-session.json` lands
   in your Downloads folder.
5. Feed it to the CLI:

   ```bash
   ig-dl login --import ~/Downloads/ig-dl-session.json
   ```

That's it — the CLI now has everything it needs to authenticate downloads.

## What's in the file

```jsonc
{
  "cookies": [
    { "Name": "sessionid", "Value": "...", "Domain": ".instagram.com",
      "Path": "/", "Expires": "2026-10-01T12:34:56.000Z",
      "Secure": true, "HttpOnly": true }
  ],
  "headers":       { "x-ig-app-id": "...", "x-ig-www-claim": "...", ... },
  "query_hashes":  { "<hash>": "<hash>", ... },
  "doc_ids":       { "<id>":   "<id>",   ... },
  "captured_at":   "2026-04-16T08:00:00.000Z"
}
```

Session cookies (cookies with no expiration) serialize as
`"0001-01-01T00:00:00Z"` — Go's `time.Time` zero value — so the CLI can
detect them with `t.IsZero()`.

## Required permissions

| Permission            | Why                                                           |
|-----------------------|---------------------------------------------------------------|
| `cookies`             | Read Instagram cookies for export.                            |
| `storage`             | Persist captured headers / query hashes across worker restarts. |
| `webRequest`          | Observe outgoing `/api/*` and `/graphql/*` requests (headers only; no modification). |
| `downloads`           | Save the exported JSON into your Downloads folder.            |
| Host: `*.instagram.com` | Scope the above to Instagram only.                          |

The extension does **not** inject code into any page, does not modify any
request, and makes no network calls of its own.

## Privacy

`ig-dl-session.json` contains your Instagram session cookies. **Treat it
like a password.** Anyone who gets the file can act as you on Instagram
until the session expires or you log out of the browser. Don't commit it,
don't share it, delete it after the CLI has imported it if you're being
careful.

## Relationship to the original extension

This is a standalone, minimal extension separate from the full
[`extension/`](../extension/) in the repo. It shares no code with it and
does not modify it — it's intentionally tiny so there's less to audit. If
you have the original "Exta Pro" extension installed as well, this
extension will also pick up its `instaHeaders` / `instaQueryHash` /
`instaDocIds` storage keys and merge them into the export (values from
this extension win on conflict).
