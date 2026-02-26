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
	recentlyPlayed: () => apiFetch<Track[]>('/library/recently-played')
};
