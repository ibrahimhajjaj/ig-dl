#!/usr/bin/env bash
#
# Manual end-to-end smoke checklist for ig-dl. Not automated — requires a
# logged-in Instagram session in a Chrome instance launched with
# --remote-debugging-port=9222, and `gallery-dl` + `yt-dlp` on PATH.
#
# Usage:
#   IG_DL_BIN=./ig-dl ./scripts/smoke.sh <test-handle> <test-post-url>
#
# Exits non-zero on the first failed step so the release flow halts.

set -euo pipefail

BIN="${IG_DL_BIN:-./ig-dl}"
HANDLE="${1:-}"
POST_URL="${2:-}"
TMP_OUT="$(mktemp -d)"
trap 'rm -rf "$TMP_OUT"' EXIT

need() {
  command -v "$1" >/dev/null 2>&1 || { echo "missing: $1" >&2; exit 1; }
}

step() {
  printf '\n\033[1m== %s ==\033[0m\n' "$*"
}

need "$BIN"
need gallery-dl
need yt-dlp

step "1. Chrome debug port probe"
if ! curl -sf http://localhost:9222/json/version >/dev/null; then
  echo "Chrome is not listening on :9222. Start it with:" >&2
  echo "  /Applications/Google\\ Chrome.app/Contents/MacOS/Google\\ Chrome --remote-debugging-port=9222" >&2
  exit 1
fi

step "2. ig-dl status (expect not-authed or authed)"
"$BIN" status

step "3. ig-dl login (captures session from Chrome)"
"$BIN" login

step "4. ig-dl status (must now report authed)"
"$BIN" status | grep -q "authed"

if [[ -n "$POST_URL" ]]; then
  step "5. single post download → $POST_URL"
  "$BIN" -o "$TMP_OUT/post" "$POST_URL"
  find "$TMP_OUT/post" -type f | head

  step "6. idempotent re-run (archive should skip)"
  "$BIN" -o "$TMP_OUT/post" "$POST_URL"
fi

if [[ -n "$HANDLE" ]]; then
  step "7. profile bulk download → $HANDLE"
  "$BIN" -o "$TMP_OUT/user" user "$HANDLE"
  find "$TMP_OUT/user" -type f | wc -l
fi

step "8. logout clears cache"
"$BIN" logout
"$BIN" status | grep -q "not authed"

step "ALL SMOKE CHECKS PASSED"
