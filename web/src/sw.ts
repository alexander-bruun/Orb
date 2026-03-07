/// <reference lib="webworker" />
import { precacheAndRoute, cleanupOutdatedCaches } from 'workbox-precaching';
import { registerRoute } from 'workbox-routing';
import { CacheFirst } from 'workbox-strategies';
import { ExpirationPlugin } from 'workbox-expiration';

declare const self: ServiceWorkerGlobalScope;

cleanupOutdatedCaches();
precacheAndRoute(self.__WB_MANIFEST);

// ── Ensure the offline page itself is cached so refreshing `/offline` works ──
const OFFLINE_CACHE = 'orb-offline-page';
const OFFLINE_PATHS = ['/offline', '/offline/', '/offline/index.html'];

self.addEventListener('install', (event: ExtendableEvent) => {
  event.waitUntil(
    (async () => {
      try {
        const cache = await caches.open(OFFLINE_CACHE);
        await Promise.all(
          OFFLINE_PATHS.map(async (p) => {
            try {
              const resp = await fetch(p, { cache: 'no-store' });
              if (resp && resp.ok) await cache.put(p, resp.clone());
            } catch {
              // ignore individual failures — best-effort cache
            }
          })
        );
      } catch {
        // best-effort; don't fail the install if offline page can't be cached
      }
    })()
  );
});

// ── Cover art (unchanged from generateSW config) ─────────────────────────────
registerRoute(
  ({ url }) => /\/api\/(covers|artists)\//.test(url.pathname),
  new CacheFirst({
    cacheName: 'orb-cover-art',
    plugins: [
      new ExpirationPlugin({
        maxEntries: 500,
        maxAgeSeconds: 60 * 60 * 24 * 30, // 30 days
      }),
    ],
  })
);

// ── Offline audio range-request synthesiser ───────────────────────────────────
//
// Downloaded tracks are stored in 'orb-offline-audio' keyed by pathname only
// (no query params) so that Streamer's ?net=wifi suffix does not cause a miss.
//
// When the streamer issues a Range request against a cached file, we slice the
// ArrayBuffer and return a synthesised 206 Partial Content response — fully
// transparent to AudioEngine and Streamer.

const AUDIO_CACHE = 'orb-offline-audio';

self.addEventListener('fetch', (event: FetchEvent) => {
  const url = new URL(event.request.url);

  // Only intercept audio stream requests
  if (!url.pathname.startsWith('/api/stream/')) return;

  event.respondWith(
    (async () => {
      // Cache key is pathname-only — strips ?net=wifi, ?net=mobile, etc.
      const cacheKey = url.origin + url.pathname;
      const cache = await caches.open(AUDIO_CACHE);
      const cached = await cache.match(cacheKey);

      // No offline copy → fall through to the network as normal
      if (!cached) return fetch(event.request);

      const rangeHeader = event.request.headers.get('range');

      // No Range header (e.g. waveform peak loader) → return full response
      if (!rangeHeader) return cached.clone();

      // Synthesise a proper 206 Partial Content response from the stored file
      const arrayBuffer = await cached.arrayBuffer();
      const total = arrayBuffer.byteLength;
      const match = rangeHeader.match(/bytes=(\d+)-(\d*)/);

      if (!match) return cached.clone();

      const start = parseInt(match[1], 10);
      const end = match[2] ? parseInt(match[2], 10) : total - 1;
      const slice = arrayBuffer.slice(start, end + 1);

      return new Response(slice, {
        status: 206,
        statusText: 'Partial Content',
        headers: {
          'Content-Type': cached.headers.get('Content-Type') ?? 'audio/flac',
          'Content-Range': `bytes ${start}-${end}/${total}`,
          'Content-Length': String(slice.byteLength),
          'Accept-Ranges': 'bytes',
        },
      });
    })()
  );
});

// ── Background Fetch (Android Phase 2) ───────────────────────────────────────

self.addEventListener('backgroundfetchsuccess', (event: Event) => {
  const bgEvent = event as any;
  bgEvent.waitUntil(
    (async () => {
      const cache = await caches.open(AUDIO_CACHE);
      const records: any[] = await bgEvent.registration.matchAll();

      for (const record of records) {
        const resp = await record.responseReady;
        // Store under the pathname-only key (no query params)
        await cache.put(new URL(record.request.url).pathname, resp);
      }

      await bgEvent.updateUI({ title: 'Download complete' });

      // Notify the page so the downloads store can update its state
      const clients = await self.clients.matchAll({ includeUncontrolled: true });
      const trackId = (bgEvent.registration.id as string).replace('orb-dl-', '');
      for (const client of clients) {
        client.postMessage({ type: 'DOWNLOAD_COMPLETE', trackId });
      }
    })()
  );
});

self.addEventListener('backgroundfetchfail', (event: Event) => {
  const bgEvent = event as any;
  bgEvent.waitUntil(bgEvent.updateUI({ title: 'Download failed' }));
});

// Serve cached offline page for navigation requests to /offline
registerRoute(
  ({ request, url }) => request.mode === 'navigate' && url.pathname === '/offline',
  async ({ event }) => {
    const cache = await caches.open(OFFLINE_CACHE);
    const match = (await cache.match('/offline')) || (await cache.match('/offline/index.html'));
    if (match) return match.clone();
    // Fallback to network if we don't have it cached
    return fetch((event as FetchEvent).request).catch(() => new Response('Offline', { status: 503 }));
  }
);
