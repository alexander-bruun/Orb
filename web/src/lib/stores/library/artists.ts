import { library as libApi } from '$lib/api/library';
import type { Artist } from '$lib/types';

const nameCache = new Map<string, string>();
// In-flight promise cache: prevents a thundering herd where many components
// concurrently request the same artist before the first response arrives.
const pendingCache = new Map<string, Promise<Artist | null>>();

export async function getArtist(id: string): Promise<Artist | null> {
  if (!id) return null;
  if (nameCache.has(id)) return { id, name: nameCache.get(id)! } as Artist;
  // Share any already-inflight request for this ID instead of spawning a new one.
  if (pendingCache.has(id)) return pendingCache.get(id)!;
  const promise = libApi.artist(id)
    .then((res) => {
      const artist = res?.artist ?? null;
      if (artist) nameCache.set(id, artist.name);
      return artist;
    })
    .catch(() => null)
    .finally(() => pendingCache.delete(id));
  pendingCache.set(id, promise);
  return promise;
}

export async function getArtistName(id: string): Promise<string> {
  if (!id) return '';
  if (nameCache.has(id)) return nameCache.get(id) as string;
  const a = await getArtist(id);
  return a?.name ?? '';
}

export function preloadArtists(ids: string[]) {
  for (const id of ids ?? []) {
    if (!id || nameCache.has(id) || pendingCache.has(id)) continue;
    // fire and forget
    getArtist(id);
  }
}

export function clearArtistCache() {
  nameCache.clear();
  pendingCache.clear();
}
