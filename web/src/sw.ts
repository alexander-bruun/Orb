/// <reference lib="webworker" />
import { precacheAndRoute, cleanupOutdatedCaches } from 'workbox-precaching';
import { registerRoute } from 'workbox-routing';
import { CacheFirst } from 'workbox-strategies';
import { ExpirationPlugin } from 'workbox-expiration';

declare const self: ServiceWorkerGlobalScope;

/** Minimal interface for a Background Fetch record (non-standard, Chromium only). */
interface BackgroundFetchRecord {
	request: Request;
	responseReady: Promise<Response>;
}

/** Minimal interface for a Background Fetch registration. */
interface BackgroundFetchRegistration {
	id: string;
	matchAll(): Promise<BackgroundFetchRecord[]>;
}

/** Minimal interface for Background Fetch events dispatched to the service worker. */
interface BackgroundFetchEvent extends ExtendableEvent {
	registration: BackgroundFetchRegistration;
	updateUI(options: { title: string }): Promise<void>;
}

cleanupOutdatedCaches();
precacheAndRoute(self.__WB_MANIFEST);

// ── Pre-cache key app pages so they load instantly and work offline ───────────
// /favorites is the offline landing pivot — cache it explicitly so a hard
// refresh works even with no network.  Other shell pages benefit as well.
const SHELL_CACHE = 'orb-shell-pages';
const SHELL_PATHS = ['/', '/favorites', '/login'];

self.addEventListener('install', (event: ExtendableEvent) => {
  event.waitUntil(
    (async () => {
      try {
        const cache = await caches.open(SHELL_CACHE);
        await Promise.all(
          SHELL_PATHS.map(async (p) => {
            try {
              const resp = await fetch(p, { cache: 'no-store' });
              if (resp && resp.ok) await cache.put(p, resp.clone());
            } catch {
              // ignore individual failures — best-effort cache
            }
          })
        );
      } catch {
        // best-effort; don't fail the install
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
  const bgEvent = event as BackgroundFetchEvent;
  bgEvent.waitUntil(
    (async () => {
      const cache = await caches.open(AUDIO_CACHE);
      const records = await bgEvent.registration.matchAll();

      for (const record of records) {
        const resp = await record.responseReady;
        // Store under the pathname-only key (no query params)
        await cache.put(new URL(record.request.url).pathname, resp);
      }

      await bgEvent.updateUI({ title: 'Download complete' });

      // Notify the page so the downloads store can update its state
      const clients = await self.clients.matchAll({ includeUncontrolled: true });
      const trackId = bgEvent.registration.id.replace('orb-dl-', '');
      for (const client of clients) {
        client.postMessage({ type: 'DOWNLOAD_COMPLETE', trackId });
      }
    })()
  );
});

self.addEventListener('backgroundfetchfail', (event: Event) => {
  const bgEvent = event as BackgroundFetchEvent;
  bgEvent.waitUntil(bgEvent.updateUI({ title: 'Download failed' }));
});

// Serve the cached /favorites shell for navigation requests when offline,
// and keep the /offline path alive as a redirect shim.
registerRoute(
  ({ request, url }) =>
    request.mode === 'navigate' &&
    (url.pathname === '/' || url.pathname === '/favorites' || url.pathname === '/offline'),
  async ({ event }) => {
    const req = event as FetchEvent;
    // Try network first so the page is always fresh when online.
    try {
      const fresh = await fetch(req.request);
      if (fresh.ok) return fresh;
    } catch {
      // Network unavailable — fall through to shell cache.
    }
    const cache = await caches.open(SHELL_CACHE);
    // Prefer the cached home shell; fall back to favorites, then error.
    const match = (await cache.match('/')) ?? (await cache.match('/favorites'));
    if (match) return match.clone();
    return new Response('Offline', { status: 503 });
  }
);
