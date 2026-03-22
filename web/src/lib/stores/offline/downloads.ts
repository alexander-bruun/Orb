import { writable, get } from 'svelte/store';
import { authStore } from '$lib/stores/auth';
import { getApiBase } from '$lib/api/base';
import { nativePlatform } from '$lib/utils/platform';
import type { Track, Audiobook, AudiobookChapter } from '$lib/types';
import type { LyricLine } from '$lib/stores/player/lyrics';
import { STORAGE_KEYS, IDB } from '$lib/constants';

export type DownloadStatus = 'downloading' | 'done' | 'error';

export interface DownloadEntry {
  trackId:       string;
  title:         string;
  artistName:    string;
  albumName:     string;
  albumId?:      string;
  isAudiobook?:  boolean;
  status:        DownloadStatus;
  progress:      number;   // 0–100
  sizeBytes:     number;
  downloadedAt?: number;
  error?:        string;
}

const AUDIO_CACHE      = IDB.NAME;
const META_KEY         = STORAGE_KEYS.DOWNLOADS_META;
const IDB_NAME         = IDB.NAME;
const IDB_STORE        = IDB.STORE_BLOBS;
const IDB_LYRICS_STORE = IDB.STORE_LYRICS;
const IDB_WAVE_STORE   = IDB.STORE_WAVEFORM;
const IDB_VERSION      = IDB.VERSION;

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

    // On Android, use the native downloader to avoid OutOfMemoryError for large files.
    // This streams directly to disk on the Kotlin side.
    if (nativePlatform() === 'android') {
      const { invoke } = await import('@tauri-apps/api/core');
      await invoke('download_track_native', {
        trackId: track.id,
        url,
        token: auth.token
      });
      // Native download handles saving to filesystem.
    } else {
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
        blob = await res.blob();
      }

      await idbPut(track.id, blob);

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
        } catch { /* ignore */ }
      }
    }

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
      const existing = m.get(track.id);
      m.set(track.id, {
        trackId:      track.id,
        title:        track.title,
        artistName:   track.artist_name ?? '',
        albumName:    track.album_name ?? '',
        albumId:      track.album_id ?? undefined,
        status:       'done',
        progress:     100,
        sizeBytes:    existing?.sizeBytes ?? 0,
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

/** Listen for native download progress events on Android. */
if (typeof window !== 'undefined' && nativePlatform() === 'android') {
  import('@tauri-apps/api/event').then(({ listen }) => {
    listen<[string, number, number]>('download-progress', (event) => {
      const [trackId, progress, totalBytes] = event.payload;
      downloads.update(m => {
        const entry = m.get(trackId);
        if (entry) {
          m.set(trackId, { ...entry, progress, sizeBytes: totalBytes });
        }
        return m;
      });
    });
  });
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

/** Minimal interface for the Background Fetch API (non-standard, Chromium only). */
interface BackgroundFetchManager {
  fetch(
    id: string,
    requests: string[],
    options: { title: string; downloadTotal: number; icons: { src: string; sizes: string; type: string }[] },
  ): Promise<unknown>;
}

export async function downloadTrackBackground(track: Track, coverUrl: string): Promise<void> {
  if (!('serviceWorker' in navigator)) return downloadTrack(track);

  const swReg = await navigator.serviceWorker.ready;
  if (!('backgroundFetch' in swReg)) return downloadTrack(track);

  const bgFetch = (swReg as unknown as { backgroundFetch: BackgroundFetchManager }).backgroundFetch;
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

// ── Audiobook downloads ──────────────────────────────────────────────────────

/** Download a single audiobook chapter. */
export async function downloadAudiobookChapter(
  chapter: AudiobookChapter,
  book: { id: string; title: string; author_name?: string },
  token: string
): Promise<void> {
  const url = `${getApiBase()}/stream/audiobook/chapter/${chapter.id}?token=${encodeURIComponent(token)}`;
  const audiobookId = book.id;
  const albumName = book.title;
  const artistName = book.author_name ?? '';

  if (nativePlatform() === 'android') {
    // Use native download on Android
    try {
      const { invoke: tauriInvoke } = await import('@tauri-apps/api/core');

      // Mark as downloading first so the UI shows progress
      downloads.update((map) => {
        map.set(chapter.id, {
          trackId: chapter.id,
          title: chapter.title,
          artistName,
          albumName,
          albumId: audiobookId,
          isAudiobook: true,
          status: 'downloading',
          progress: 0,
          sizeBytes: 0,
        });
        return map;
      });

      await tauriInvoke('download_track_native', {
        trackId: chapter.id,
        url,
        token
      });

      // Mark as done
      downloads.update((map) => {
        map.set(chapter.id, {
          trackId: chapter.id,
          title: chapter.title,
          artistName,
          albumName,
          albumId: audiobookId,
          isAudiobook: true,
          status: 'done',
          progress: 100,
          sizeBytes: 0,
          downloadedAt: Date.now()
        });
        persist(map);
        return map;
      });

      // Sync metadata to Android MediaService
      const meta = get(downloads);
      const metaArray = Array.from(meta.values())
        .filter(e => e.status === 'done' && e.albumId)
        .map(e => ({
          trackId: e.trackId,
          title: e.title,
          artistName: e.artistName,
          albumName: e.albumName,
          albumId: e.albumId
        }));
      await tauriInvoke('sync_downloads', {
        metadataJson: JSON.stringify(metaArray)
      });
    } catch (e) {
      console.error(`Failed to download chapter ${chapter.id}:`, e);
      downloads.update((map) => {
        map.set(chapter.id, {
          trackId: chapter.id,
          title: chapter.title,
          artistName,
          albumName,
          albumId: audiobookId,
          isAudiobook: true,
          status: 'error',
          progress: 0,
          sizeBytes: 0,
          error: String(e)
        });
        return map;
      });
    }
  } else {
    // Use fetch + IndexedDB on web
    try {
      downloads.update((map) => {
        map.set(chapter.id, {
          trackId: chapter.id,
          title: chapter.title,
          artistName,
          albumName,
          albumId: audiobookId,
          isAudiobook: true,
          status: 'downloading',
          progress: 0,
          sizeBytes: 0
        });
        return map;
      });

      const response = await fetch(url);
      if (!response.ok) throw new Error(`HTTP ${response.status}`);

      const contentLength = response.headers.get('content-length');
      const totalBytes = contentLength ? parseInt(contentLength, 10) : 0;

      if (!response.body) throw new Error('No response body');

      const reader = response.body.getReader();
      const chunks: Uint8Array[] = [];
      let downloadedBytes = 0;

      while (true) {
        const { done, value } = await reader.read();
        if (done) break;
        chunks.push(value);
        downloadedBytes += value.length;

        const progress = totalBytes > 0 ? Math.round((downloadedBytes / totalBytes) * 100) : 0;
        downloads.update((map) => {
          const entry = map.get(chapter.id);
          if (entry) {
            entry.progress = progress;
            entry.sizeBytes = downloadedBytes;
          }
          return map;
        });
      }

      const blob = new Blob(chunks);
      await idbPut(chapter.id, blob);

      downloads.update((map) => {
        map.set(chapter.id, {
          trackId: chapter.id,
          title: chapter.title,
          artistName,
          albumName,
          albumId: audiobookId,
          isAudiobook: true,
          status: 'done',
          progress: 100,
          sizeBytes: downloadedBytes,
          downloadedAt: Date.now()
        });
        return map;
      });
    } catch (e) {
      console.error(`Failed to download chapter ${chapter.id}:`, e);
      downloads.update((map) => {
        map.set(chapter.id, {
          trackId: chapter.id,
          title: chapter.title,
          artistName,
          albumName,
          albumId: audiobookId,
          isAudiobook: true,
          status: 'error',
          progress: 0,
          sizeBytes: 0,
          error: String(e)
        });
        return map;
      });
    }
  }
}

/** Download all chapters of an audiobook sequentially. */
export async function downloadAudiobook(
  book: Audiobook,
  token: string
): Promise<void> {
  const chapters = book.chapters ?? [];
  for (const chapter of chapters) {
    await downloadAudiobookChapter(chapter, book, token);
  }
}

/** Check if all chapters of a book are downloaded. */
export function isAudiobookDownloaded(
  audiobookId: string,
  chapters: AudiobookChapter[]
): boolean {
  const meta = get(downloads);
  return chapters.every(ch => {
    const entry = meta.get(ch.id);
    return entry?.status === 'done';
  });
}

/** Get offline chapter URL for playback (blob URL on web, file:// path on Android). */
export async function getOfflineChapterUrl(chapterId: string): Promise<string | null> {
  if (nativePlatform() === 'android') {
    try {
      const { invoke: tauriInvoke } = await import('@tauri-apps/api/core');
      const path = await tauriInvoke<string | null>('get_offline_file_path', { trackId: chapterId });
      if (path) return `file://${path}`;
    } catch { /* fall through */ }
    return null;
  }

  const blob = await getOfflineBlob(chapterId);
  if (!blob) return null;

  return URL.createObjectURL(blob);
}

/** Delete all downloaded chapters for a book. */
export async function deleteAudiobookDownload(
  audiobookId: string,
  chapters: AudiobookChapter[]
): Promise<void> {
  for (const chapter of chapters) {
    await deleteDownload(chapter.id);
  }
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
