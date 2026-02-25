import { library as libApi } from '$lib/api/library';

const nameCache = new Map<string, string>();

export async function getArtist(id: string) {
  if (!id) return null;
  if (nameCache.has(id)) return { id, name: nameCache.get(id) };
  try {
    const res = await libApi.artist(id);
    const artist = res?.artist;
    if (artist) nameCache.set(id, artist.name);
    return artist;
  } catch (e) {
    return null;
  }
}

export async function getArtistName(id: string): Promise<string> {
  if (!id) return '';
  if (nameCache.has(id)) return nameCache.get(id) as string;
  const a = await getArtist(id);
  return a?.name ?? '';
}

export function preloadArtists(ids: string[]) {
  for (const id of ids ?? []) {
    if (!id || nameCache.has(id)) continue;
    // fire and forget
    getArtist(id);
  }
}

export function clearArtistCache() {
  nameCache.clear();
}
