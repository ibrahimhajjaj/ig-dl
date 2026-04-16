// ig-dl session exporter — service worker.
//
// Passively observes Instagram API/GraphQL requests made by the user's
// regular browsing and records:
//   * The four rotating request headers the private endpoints require
//     (x-ig-www-claim, x-ig-app-id, x-asbd-id, x-instagram-ajax).
//   * Any `query_hash` / `doc_id` values seen on the URL query string.
//
// Nothing is injected into pages and no request is modified. The captured
// data is stashed in chrome.storage.local and later bundled with cookies
// by options.js when the user clicks "Export session for CLI".

'use strict';

const TRACKED_HEADERS = [
  'x-ig-www-claim',
  'x-ig-app-id',
  'x-asbd-id',
  'x-instagram-ajax',
];

const HEADERS_KEY = 'igdlHeaders';
const QUERY_HASHES_KEY = 'igdlQueryHashes';
const DOC_IDS_KEY = 'igdlDocIds';

// Extract the tracked headers from a requestHeaders array (Chrome webRequest shape).
// Returns a plain object with lower-cased header names; omits missing ones.
function pickHeaders(requestHeaders) {
  const out = {};
  if (!Array.isArray(requestHeaders)) return out;
  for (const h of requestHeaders) {
    if (!h || !h.name) continue;
    const lower = h.name.toLowerCase();
    if (TRACKED_HEADERS.indexOf(lower) !== -1 && typeof h.value === 'string' && h.value.length > 0) {
      out[lower] = h.value;
    }
  }
  return out;
}

// Merge newly-seen headers on top of whatever is stored. Latest wins per key.
function mergeHeaders(picked) {
  if (Object.keys(picked).length === 0) return;
  chrome.storage.local.get(HEADERS_KEY, (existing) => {
    const prev = (existing && existing[HEADERS_KEY]) || {};
    const next = Object.assign({}, prev, picked);
    chrome.storage.local.set({ [HEADERS_KEY]: next });
  });
}

// Accumulate query_hash / doc_id maps. Value → value (identity map) so the
// Go side can trivially serialize as map[string]string and the shape
// matches the pre-existing instaQueryHash / instaDocIds storage.
function accumulateIdentity(storageKey, value) {
  if (!value) return;
  chrome.storage.local.get(storageKey, (existing) => {
    const prev = (existing && existing[storageKey]) || {};
    if (prev[value] === value) return;
    const next = Object.assign({}, prev, { [value]: value });
    chrome.storage.local.set({ [storageKey]: next });
  });
}

function captureUrlIdentifiers(rawUrl) {
  if (!rawUrl) return;
  let parsed;
  try {
    parsed = new URL(rawUrl);
  } catch (_e) {
    return;
  }
  const qh = parsed.searchParams.get('query_hash');
  const did = parsed.searchParams.get('doc_id');
  if (qh) accumulateIdentity(QUERY_HASHES_KEY, qh);
  if (did) accumulateIdentity(DOC_IDS_KEY, did);
}

function onBeforeSendHeaders(details) {
  try {
    const picked = pickHeaders(details.requestHeaders);
    mergeHeaders(picked);
    captureUrlIdentifiers(details.url);
  } catch (e) {
    // Never throw from the listener; just log.
    console.error('[ig-dl exporter] capture error:', e);
  }
}

chrome.webRequest.onBeforeSendHeaders.addListener(
  onBeforeSendHeaders,
  {
    urls: [
      '*://*.instagram.com/api/*',
      '*://*.instagram.com/graphql/*',
    ],
  },
  ['requestHeaders', 'extraHeaders']
);
