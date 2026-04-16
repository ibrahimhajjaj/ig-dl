// ig-dl session exporter — options page logic.
//
// Builds the session JSON from:
//   * chrome.cookies.getAll({domain: "instagram.com"}) — every IG cookie
//     the browser holds.
//   * chrome.storage.local — rotating headers + query_hash / doc_id maps
//     captured by bg.js, merged with whatever the legacy "Exta Pro" IG
//     extension left around (instaHeaders / instaQueryHash / instaDocIds)
//     so users who already had that installed get a richer payload.
//
// The result is serialized, stuffed into a Blob, and handed to
// chrome.downloads.download as "ig-dl-session.json". The CLI picks it up
// via `ig-dl login --import <path>`.

'use strict';

const STATUS_EL_ID = 'status';
const BUTTON_ID = 'export-btn';
const DOWNLOAD_FILENAME = 'ig-dl-session.json';

// Go's time.Time zero value in RFC3339. Session cookies (no expirationDate)
// map to this so the Go side can reliably detect "no expiry" with IsZero().
const GO_ZERO_TIME = '0001-01-01T00:00:00Z';

function setStatus(msg, cls) {
  const el = document.getElementById(STATUS_EL_ID);
  if (!el) return;
  el.textContent = msg;
  el.className = cls || '';
}

function getAllCookies() {
  return new Promise((resolve, reject) => {
    try {
      chrome.cookies.getAll({ domain: 'instagram.com' }, (cookies) => {
        const err = chrome.runtime.lastError;
        if (err) {
          reject(new Error(err.message || String(err)));
          return;
        }
        resolve(cookies || []);
      });
    } catch (e) {
      reject(e);
    }
  });
}

function getStorage(keys) {
  return new Promise((resolve, reject) => {
    try {
      chrome.storage.local.get(keys, (items) => {
        const err = chrome.runtime.lastError;
        if (err) {
          reject(new Error(err.message || String(err)));
          return;
        }
        resolve(items || {});
      });
    } catch (e) {
      reject(e);
    }
  });
}

// Convert a Chrome cookie `expirationDate` (unix seconds, may be float,
// may be undefined for session cookies) into an RFC3339 string that
// Go's time.Parse(time.RFC3339) can consume. Session cookies collapse to
// Go's zero time so the Go side can treat missing-expiry uniformly.
function expirationToRFC3339(expirationDate) {
  if (typeof expirationDate !== 'number' || !isFinite(expirationDate) || expirationDate <= 0) {
    return GO_ZERO_TIME;
  }
  const ms = Math.round(expirationDate * 1000);
  const d = new Date(ms);
  if (isNaN(d.getTime())) return GO_ZERO_TIME;
  return d.toISOString(); // toISOString yields RFC3339 with "Z" zone.
}

// Shape cookies to the struct the Go session/cookies.go loader expects.
// Field names are Capitalized so the CLI can unmarshal straight into
// the same type it already uses elsewhere (http.Cookie-ish).
function shapeCookies(raw) {
  return raw.map((c) => ({
    Name: c.name || '',
    Value: c.value || '',
    Domain: c.domain || '',
    Path: c.path || '/',
    Expires: expirationToRFC3339(c.expirationDate),
    Secure: !!c.secure,
    HttpOnly: !!c.httpOnly,
  }));
}

// Merge legacy instaHeaders into our igdlHeaders, with our values winning.
// Same for query hashes / doc ids. Null-safe on both sides.
function mergeMaps(ours, theirs) {
  const base = theirs && typeof theirs === 'object' ? theirs : {};
  const top = ours && typeof ours === 'object' ? ours : {};
  return Object.assign({}, base, top);
}

async function buildPayload() {
  const [cookies, storage] = await Promise.all([
    getAllCookies(),
    getStorage([
      'igdlHeaders',
      'igdlQueryHashes',
      'igdlDocIds',
      'instaHeaders',
      'instaQueryHash',
      'instaDocIds',
    ]),
  ]);

  const headers = mergeMaps(storage.igdlHeaders, storage.instaHeaders);
  const queryHashes = mergeMaps(storage.igdlQueryHashes, storage.instaQueryHash);
  const docIds = mergeMaps(storage.igdlDocIds, storage.instaDocIds);

  return {
    cookies: shapeCookies(cookies),
    headers: headers,
    query_hashes: queryHashes,
    doc_ids: docIds,
    captured_at: new Date().toISOString(),
  };
}

function downloadBlob(url, filename) {
  return new Promise((resolve, reject) => {
    try {
      chrome.downloads.download({ url: url, filename: filename, saveAs: false }, (id) => {
        const err = chrome.runtime.lastError;
        if (err) {
          reject(new Error(err.message || String(err)));
          return;
        }
        if (typeof id !== 'number') {
          reject(new Error('downloads.download returned no id'));
          return;
        }
        resolve(id);
      });
    } catch (e) {
      reject(e);
    }
  });
}

async function handleExport() {
  const btn = document.getElementById(BUTTON_ID);
  if (btn) btn.disabled = true;
  setStatus('Collecting session…', '');

  let objectUrl = null;
  try {
    const payload = await buildPayload();
    if (!payload.cookies || payload.cookies.length === 0) {
      throw new Error('No Instagram cookies found. Log in to instagram.com in this browser first.');
    }
    const json = JSON.stringify(payload, null, 2);
    const blob = new Blob([json], { type: 'application/json' });
    objectUrl = URL.createObjectURL(blob);
    await downloadBlob(objectUrl, DOWNLOAD_FILENAME);
    setStatus(
      'Exported ' + DOWNLOAD_FILENAME + ' to your Downloads folder (' +
        payload.cookies.length + ' cookies, ' +
        Object.keys(payload.headers).length + ' headers).',
      'ok'
    );
  } catch (e) {
    console.error('[ig-dl exporter] export failed:', e);
    setStatus('Export failed: ' + (e && e.message ? e.message : String(e)), 'err');
  } finally {
    // Revoke the object URL shortly after the download has kicked off.
    // chrome.downloads.download captures the blob synchronously but give
    // it a moment to avoid racing on slower systems.
    if (objectUrl) {
      setTimeout(() => {
        try { URL.revokeObjectURL(objectUrl); } catch (_e) { /* noop */ }
      }, 2000);
    }
    if (btn) btn.disabled = false;
  }
}

document.addEventListener('DOMContentLoaded', () => {
  const btn = document.getElementById(BUTTON_ID);
  if (btn) btn.addEventListener('click', handleExport);
});
