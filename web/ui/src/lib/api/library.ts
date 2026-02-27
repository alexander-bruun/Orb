import { apiFetch } from './client';
import type { Track, Album, Artist } from '$lib/types';

export const library = {
	tracks: (limit = 50, offset = 0) =>
		apiFetch<Track[]>(`/library/tracks?limit=${limit}&offset=${offset}`),
	albums: (limit = 500, offset = 0, sortBy: 'title' | 'artist' | 'year' = 'title') =>
		apiFetch<{ items: Album[]; total: number }>(`/library/albums?limit=${limit}&offset=${offset}&sort_by=${sortBy}`),
	artists: (limit = 50, offset = 0) =>
		apiFetch<Artist[]>(`/library/artists?limit=${limit}&offset=${offset}`),
	album: (id: string) => apiFetch<{ album: Album; tracks: Track[]; artist?: Artist }>(`/library/albums/${id}`),
	artist: (id: string) => apiFetch<{ artist: Artist; albums: Album[] }>(`/library/artists/${id}`),
	track: (id: string) => apiFetch<Track>(`/library/tracks/${id}`),
	addTrack: (id: string) => apiFetch(`/library/tracks/${id}`, { method: 'POST' }),
	removeTrack: (id: string) => apiFetch(`/library/tracks/${id}`, { method: 'DELETE' }),
	search: (q: string) =>
		apiFetch<{ tracks: Track[]; albums: Album[]; artists: Artist[] }>(`/library/search?q=${encodeURIComponent(q)}`),
	recentlyPlayed: (limit = 100, from?: string, to?: string) => {
		const p = new URLSearchParams({ limit: String(limit) });
		if (from) p.set('from', from);
		if (to) p.set('to', to);
		return apiFetch<Track[]>(`/library/recently-played?${p}`);
	},
	mostPlayed: (limit = 100, from?: string, to?: string) => {
		const p = new URLSearchParams({ limit: String(limit) });
		if (from) p.set('from', from);
		if (to) p.set('to', to);
		return apiFetch<Track[]>(`/library/most-played?${p}`);
	},
	recentlyPlayedAlbums: () => apiFetch<Album[]>('/library/recently-played/albums'),
	recentlyAddedAlbums: (limit = 20) => apiFetch<Album[]>(`/library/recently-added/albums?limit=${limit}`),
	recordPlay: (trackId: string, durationPlayedMs = 0) =>
		apiFetch('/library/history', {
			method: 'POST',
			headers: { 'Content-Type': 'application/json' },
			body: JSON.stringify({ track_id: trackId, duration_played_ms: durationPlayedMs })
		}),
	favorites: () => apiFetch<Track[]>('/library/favorites'),
	favoriteIDs: () => apiFetch<string[]>('/library/favorites/ids'),
	addFavorite: (id: string) => apiFetch(`/library/favorites/${id}`, { method: 'POST' }),
	removeFavorite: (id: string) => apiFetch(`/library/favorites/${id}`, { method: 'DELETE' }),
	lyrics: (trackId: string) =>
		apiFetch<{ time_ms: number; text: string }[]>(`/library/tracks/${trackId}/lyrics`),
	setLyrics: (trackId: string, lyrics: string) =>
		apiFetch(`/library/tracks/${trackId}/lyrics`, {
			method: 'PUT',
			headers: { 'Content-Type': 'application/json' },
			body: JSON.stringify({ lyrics })
		})
};
