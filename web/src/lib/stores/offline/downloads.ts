import { writable, get } from 'svelte/store';
import { authStore } from '$lib/stores/auth';
import { getApiBase } from '$lib/api/base';
import { nativePlatform } from '$lib/utils/platform';
import type { Track } from '$lib/types';
import type { LyricLine } from '$lib/stores/player/lyrics';

export type DownloadStatus = 'downloading' | 'done' | 'error';

export interface DownloadEntry {
  trackId:       string;
  title:         string;
  artistName:    string;
  albumName:     string;
  albumId?:      string;
  status:        DownloadStatus;
  progress:      number;   // 0–100
  sizeBytes:     number;
  downloadedAt?: number;
  error?:        string;
}

const AUDIO_CACHE      = 'orb-offline-audio';
const META_KEY         = 'orb-downloads-v1';
const IDB_NAME         = 'orb-offline-audio';
const IDB_STORE        = 'blobs';
const IDB_LYRICS_STORE = 'lyrics';
const IDB_WAVE_STORE   = 'waveform';
const IDB_VERSION      = 2;

export const downloads = writable<Map<string, DownloadEntry>>(new Map());

/** True when the Cache API is available (secure context only). */
const cacheAvailable = typeof caches !== 'undefined';

// ── IndexedDB helpers (works on ALL origins, secure or insecure) ─────────────

function openIDB(): Promise<IDBDatabase> {
  return new Promise((resolve, reject) => {
    const req = indexedDB.open(IDB_NAME, IDB_VERSION);
    req.onupgradeneeded = () => {
      const db = req.result;
      for (const store of [IDB_STORE, IDB_LYRICS_STORE, IDB_WAVE_STORE]) {
        if (!db.objectStoreNames.contains(store)) {
          db.createObjectStore(store);
        }
      }
    };
    req.onsuccess = () => resolve(req.result);
    req.onerror   = () => reject(req.error);
  });
}

async function idbPutTo(store: string, key: string, value: unknown): Promise<void> {
  const db = await openIDB();
  return new Promise((resolve, reject) => {
    const tx = db.transaction(store, 'readwrite');
    tx.objectStore(store).put(JSON.stringify(value), key);
    tx.oncomplete = () => { db.close(); resolve(); };
    tx.onerror    = () => { db.close(); reject(tx.error); };
  });
}

async function idbGetFrom<T>(store: string, key: string): Promise<T | undefined> {
  const db = await openIDB();
  return new Promise((resolve, reject) => {
    const tx  = db.transaction(store, 'readonly');
    const req = tx.objectStore(store).get(key);
    req.onsuccess = () => {
      db.close();
      resolve(req.result != null ? JSON.parse(req.result as string) as T : undefined);
    };
    req.onerror = () => { db.close(); reject(req.error); };
  });
}

async function idbDeleteFrom(store: string, key: string): Promise<void> {
  const db = await openIDB();
  return new Promise((resolve, reject) => {
    const tx = db.transaction(store, 'readwrite');
    tx.objectStore(store).delete(key);
    tx.oncomplete = () => { db.close(); resolve(); };
    tx.onerror    = () => { db.close(); reject(tx.error); };
  });
}

async function idbClearStore(store: string): Promise<void> {
  const db = await openIDB();
  return new Promise((resolve, reject) => {
    const tx = db.transaction(store, 'readwrite');
    tx.objectStore(store).clear();
    tx.oncomplete = () => { db.close(); resolve(); };
    tx.onerror    = () => { db.close(); reject(tx.error); };
  });
}

async function idbPut(trackId: string, blob: Blob): Promise<void> {
  const db = await openIDB();
  return new Promise((resolve, reject) => {
    const tx = db.transaction(IDB_STORE, 'readwrite');
    tx.objectStore(IDB_STORE).put(blob, trackId);
    tx.oncomplete = () => { db.close(); resolve(); };
    tx.onerror    = () => { db.close(); reject(tx.error); };
  });
}

async function idbGet(trackId: string): Promise<Blob | undefined> {
  const db = await openIDB();
  return new Promise((resolve, reject) => {
    const tx  = db.transaction(IDB_STORE, 'readonly');
    const req = tx.objectStore(IDB_STORE).get(trackId);
    req.onsuccess = () => { db.close(); resolve(req.result ?? undefined); };
    req.onerror   = () => { db.close(); reject(req.error); };
  });
}

async function idbDelete(trackId: string): Promise<void> {
  const db = await openIDB();
  return new Promise((resolve, reject) => {
    const tx = db.transaction(IDB_STORE, 'readwrite');
    tx.objectStore(IDB_STORE).delete(trackId);
    tx.oncomplete = () => { db.close(); resolve(); };
    tx.onerror    = () => { db.close(); reject(tx.error); };
  });
}

async function idbClear(): Promise<void> {
  const db = await openIDB();
  return new Promise((resolve, reject) => {
    const tx = db.transaction(IDB_STORE, 'readwrite');
    tx.objectStore(IDB_STORE).clear();
    tx.oncomplete = () => { db.close(); resolve(); };
    tx.onerror    = () => { db.close(); reject(tx.error); };
  });
}

// ── Persistence (metadata only — lightweight localStorage) ───────────────────

function persist(map: Map<string, DownloadEntry>): void {
  try {
    localStorage.setItem(META_KEY, JSON.stringify([...map]));
  } catch { /* storage full — ignore */ }
  syncMetadataToAndroid(map);
}

/** Push download metadata to Android MediaService for Android Auto offline browsing. */
async function syncMetadataToAndroid(map: Map<string, DownloadEntry>): Promise<void> {
  if (nativePlatform() !== 'android') return;
  try {
    const { invoke } = await import('@tauri-apps/api/core');
    const completed = [...map.values()]
      .filter(e => e.status === 'done')
      .map(e => ({
        trackId: e.trackId,
        title: e.title,
        artistName: e.artistName,
        albumName: e.albumName,
        albumId: e.albumId ?? null,
      }));
    await invoke('sync_downloads', { metadataJson: JSON.stringify(completed) });
  } catch { /* best-effort */ }
}

/** Save audio bytes to Android native storage for offline Android Auto playback. */
async function saveToNativeStorage(trackId: string, blob: Blob): Promise<void> {
  if (nativePlatform() !== 'android') return;
  try {
    const { invoke } = await import('@tauri-apps/api/core');
    const buffer = await blob.arrayBuffer();
    // Pass Uint8Array directly — Tauri v2 transfers binary without JSON serialization,
    // avoiding the main-thread freeze that Array.from() caused on large audio files.
    await invoke('save_offline_file', { trackId, data: new Uint8Array(buffer) });
  } catch { /* best-effort */ }
}

/** Download and save cover art to Android native storage for offline browsing. */
async function saveCoverToNativeStorage(albumId: string): Promise<void> {
  if (nativePlatform() !== 'android' || !albumId) return;
  try {
    const { invoke } = await import('@tauri-apps/api/core');
    const coverUrl = `${getApiBase()}/covers/${albumId}`;
    const res = await fetch(coverUrl);
    if (!res.ok) return;
    const blob = await res.blob();
    const buffer = await blob.arrayBuffer();
    await invoke('save_cover_art', { albumId, data: new Uint8Array(buffer) });
  } catch { /* best-effort */ }
}

/** Delete audio file from Android native storage. */
async function deleteFromNativeStorage(trackId: string): Promise<void> {
  if (nativePlatform() !== 'android') return;
  try {
    const { invoke } = await import('@tauri-apps/api/core');
    await invoke('delete_offline_file', { trackId });
  } catch { /* best-effort */ }
}

export function restoreDownloads(): void {
  try {
    const raw = localStorage.getItem(META_KEY);
    if (raw) downloads.set(new Map(JSON.parse(raw)));
  } catch { /* corrupt data — start fresh */ }
}

// ── Storage estimate ─────────────────────────────────────────────────────────

export async function getStorageEstimate(): Promise<StorageEstimate | null> {
  return navigator.storage?.estimate?.() ?? null;
}

// ── Download a single track ──────────────────────────────────────────────────

export async function downloadTrack(track: Track): Promise<void> {
  // Request durable storage — critical on iOS where caches are evicted
  // after ~7 days of inactivity without this permission.
  if (navigator.storage?.persist) {
    await navigator.storage.persist().catch(() => {});
  }

  downloads.update(m => {
    m.set(track.id, {
      trackId:    track.id,
      title:      track.title,
      artistName: track.artist_name ?? '',
      albumName:  track.album_name ?? '',
      albumId:    track.album_id ?? undefined,
      status:     'downloading',
      progress:   0,
      sizeBytes:  0,
    });
    persist(m);
    return m;
  });

  try {
    const auth = get(authStore);
    const url  = `${getApiBase()}/stream/${track.id}`;

    const res = await fetch(url, {
      headers: { Authorization: auth.token ? `Bearer ${auth.token}` : '' },
    });
    if (!res.ok) throw new Error(`Server returned ${res.status}`);

    let blob: Blob;

    if (res.body) {
      // Stream with progress tracking
      const contentLength = Number(res.headers.get('Content-Length') ?? 0);
      const contentType   = res.headers.get('Content-Type') ?? 'audio/flac';
      const reader        = res.body.getReader();
      const chunks: Uint8Array[] = [];
      let received = 0;
      let lastReportedProgress = -1;

      for (;;) {
        const { done, value } = await reader.read();
        if (done) break;
        chunks.push(value);
        received += value.byteLength;
        const progress = contentLength ? Math.round((received / contentLength) * 100) : 0;
        // Throttle store updates: only fire when progress changes by ≥1% to avoid
        // thousands of re-renders per download on slow mobile hardware.
        if (progress !== lastReportedProgress) {
          lastReportedProgress = progress;
          downloads.update(m => {
            const entry = m.get(track.id);
            if (entry) {
              m.set(track.id, { ...entry, progress, sizeBytes: received });
            }
            return m;
          });
        }
      }

      blob = new Blob(chunks, { type: contentType });
    } else {
      // Fallback for browsers without ReadableStream body (some mobile WebViews)
      blob = await res.blob();
    }

    // Primary storage: IndexedDB (works on ALL origins, secure or insecure)
    await idbPut(track.id, blob);

    // Secondary storage: Cache API (when available — enables SW offline serving)
    if (cacheAvailable) {
      try {
        const cacheResp = new Response(blob.slice(), {
          headers: {
            'Content-Type':   blob.type || 'audio/flac',
            'Content-Length':  String(blob.size),
            'Accept-Ranges':  'bytes',
          },
        });
        const cache = await caches.open(AUDIO_CACHE);
        await cache.put(new URL(url, location.href).pathname, cacheResp);
      } catch { /* Cache API unavailable — IDB is the source of truth */ }
    }

    // Android: save audio + cover art to native filesystem for Android Auto offline playback
    await saveToNativeStorage(track.id, blob);

    // Cache lyrics and waveform for offline playback (best-effort).
    const headers = { Authorization: auth.token ? `Bearer ${auth.token}` : '' };
    const base = getApiBase();
    await Promise.allSettled([
      fetch(`${base}/library/tracks/${track.id}/lyrics`, { headers })
        .then(r => r.ok ? r.json() : null)
        .then(data => data?.length ? idbPutTo(IDB_LYRICS_STORE, track.id, data) : null),
      fetch(`${base}/library/tracks/${track.id}/waveform`, { headers })
        .then(r => r.ok ? r.json() : null)
        .then(data => Array.isArray(data?.peaks) && data.peaks.length ? idbPutTo(IDB_WAVE_STORE, track.id, data.peaks) : null),
    ]);

    downloads.update(m => {
      m.set(track.id, {
        trackId:      track.id,
        title:        track.title,
        artistName:   track.artist_name ?? '',
        albumName:    track.album_name ?? '',
        albumId:      track.album_id ?? undefined,
        status:       'done',
        progress:     100,
        sizeBytes:    blob.size,
        downloadedAt: Date.now(),
      });
      persist(m);
      return m;
    });
  } catch (err: unknown) {
    const msg = err instanceof Error ? err.message : 'Unknown error';
    downloads.update(m => {
      const existing = m.get(track.id);
      m.set(track.id, {
        trackId:    track.id,
        title:      track.title,
        artistName: track.artist_name ?? '',
        albumName:  track.album_name ?? '',
        status:     'error',
        progress:   0,
        sizeBytes:  0,
        error:      msg,
        ...(existing?.downloadedAt ? { downloadedAt: existing.downloadedAt } : {}),
      });
      persist(m);
      return m;
    });
  }
}

// ── Batch download (sequential to avoid hammering the server) ─────────────────

export async function downloadAlbum(tracks: Track[]): Promise<void> {
  const savedCovers = new Set<string>();
  for (const track of tracks) {
    const entry = get(downloads).get(track.id);
    if (entry?.status === 'done') continue;
    await downloadTrack(track);
    // Save cover art once per album, not once per track.
    if (track.album_id && !savedCovers.has(track.album_id)) {
      savedCovers.add(track.album_id);
      await saveCoverToNativeStorage(track.album_id);
    }
  }
}

/** Download all tracks in a playlist. */
export const downloadPlaylist = downloadAlbum;

/** Download all favorites. */
export const downloadFavorites = downloadAlbum;

// ── Get offline blob for playback (used by AudioEngine on insecure origins) ──

/**
 * Returns a blob URL for an offline-downloaded track, or null if not available.
 * The caller MUST call URL.revokeObjectURL() when done.
 */
export async function getOfflineBlobUrl(trackId: string): Promise<string | null> {
  try {
    const blob = await idbGet(trackId);
    if (blob) return URL.createObjectURL(blob);
  } catch { /* IDB unavailable */ }
  return null;
}

/**
 * Returns the raw Blob for an offline-downloaded track, or null.
 */
export async function getOfflineBlob(trackId: string): Promise<Blob | null> {
  try {
    return (await idbGet(trackId)) ?? null;
  } catch {
    return null;
  }
}

// ── Background Fetch download (Android Phase 2) ──────────────────────────────

export async function downloadTrackBackground(track: Track, coverUrl: string): Promise<void> {
  if (!('serviceWorker' in navigator)) return downloadTrack(track);

  const swReg = await navigator.serviceWorker.ready;
  if (!('backgroundFetch' in swReg)) return downloadTrack(track);

  const bgFetch = (swReg as any).backgroundFetch;
  await bgFetch.fetch(
    `orb-dl-${track.id}`,
    [`${getApiBase()}/stream/${track.id}`],
    {
      title:         `Downloading ${track.title}`,
      downloadTotal: track.file_size ?? 0,
      icons:         coverUrl ? [{ src: coverUrl, sizes: '128x128', type: 'image/jpeg' }] : [],
    }
  );

  downloads.update(m => {
    m.set(track.id, {
      trackId:    track.id,
      title:      track.title,
      artistName: track.artist_name ?? '',
      albumName:  track.album_name ?? '',
      status:     'downloading',
      progress:   0,
      sizeBytes:  0,
    });
    persist(m);
    return m;
  });
}

// Register a listener for DOWNLOAD_COMPLETE messages from the SW
if (typeof navigator !== 'undefined' && 'serviceWorker' in navigator) {
  navigator.serviceWorker.addEventListener('message', (event) => {
    if (event.data?.type !== 'DOWNLOAD_COMPLETE') return;
    const trackId = event.data.trackId as string;
    downloads.update(m => {
      const entry = m.get(trackId);
      if (entry) {
        m.set(trackId, {
          ...entry,
          status:       'done',
          progress:     100,
          downloadedAt: Date.now(),
        });
        persist(m);
      }
      return m;
    });
  });
}

// ── Delete a downloaded track ────────────────────────────────────────────────

export async function deleteDownload(trackId: string): Promise<void> {
  // Remove from IDB (primary + lyrics + waveform)
  try { await idbDelete(trackId); } catch { /* ignore */ }
  try { await idbDeleteFrom(IDB_LYRICS_STORE, trackId); } catch { /* ignore */ }
  try { await idbDeleteFrom(IDB_WAVE_STORE, trackId); } catch { /* ignore */ }

  // Remove from Cache API (secondary)
  if (cacheAvailable) {
    try {
      const cache = await caches.open(AUDIO_CACHE);
      await cache.delete(`/api/stream/${trackId}`);
    } catch { /* ignore */ }
  }

  // Remove from Android native storage
  await deleteFromNativeStorage(trackId);

  downloads.update(m => {
    m.delete(trackId);
    persist(m);
    return m;
  });
}

// ── Delete ALL downloaded tracks ─────────────────────────────────────────────

export async function deleteAllDownloads(): Promise<void> {
  const map = get(downloads);

  // Clear IDB (audio + lyrics + waveform)
  try { await idbClear(); } catch { /* ignore */ }
  try { await idbClearStore(IDB_LYRICS_STORE); } catch { /* ignore */ }
  try { await idbClearStore(IDB_WAVE_STORE); } catch { /* ignore */ }

  // Clear Cache API entries
  if (cacheAvailable) {
    try {
      const cache = await caches.open(AUDIO_CACHE);
      for (const [id] of map) {
        await cache.delete(`/api/stream/${id}`).catch(() => {});
      }
    } catch { /* ignore */ }
  }

  downloads.set(new Map());
  persist(new Map());
}

// ── Check if a track is available offline ────────────────────────────────────

export async function isDownloaded(trackId: string): Promise<boolean> {
  // Check IDB first (works everywhere)
  try {
    const blob = await idbGet(trackId);
    if (blob) return true;
  } catch { /* fall through */ }

  // Fall back to Cache API (secure contexts)
  if (cacheAvailable) {
    try {
      const cache = await caches.open(AUDIO_CACHE);
      return !!(await cache.match(`/api/stream/${trackId}`));
    } catch { /* ignore */ }
  }

  return false;
}

// ── Offline lyrics / waveform getters ────────────────────────────────────────

/** Returns cached lyrics for an offline track, or null if not available. */
export async function getOfflineLyrics(trackId: string): Promise<LyricLine[] | null> {
  try {
    return (await idbGetFrom<LyricLine[]>(IDB_LYRICS_STORE, trackId)) ?? null;
  } catch {
    return null;
  }
}

/** Returns cached waveform peaks for an offline track, or null if not available. */
export async function getOfflinePeaks(trackId: string): Promise<number[] | null> {
  try {
    return (await idbGetFrom<number[]>(IDB_WAVE_STORE, trackId)) ?? null;
  } catch {
    return null;
  }
}
