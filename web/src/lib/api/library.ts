import { apiFetch } from './client';
import type { Track, Album, Artist, Genre, RelatedArtist, SearchFilters } from '$lib/types';

export const library = {
	tracks: (limit = 50, offset = 0) =>
		apiFetch<Track[]>(`/library/tracks?limit=${limit}&offset=${offset}`),
	albums: (limit = 500, offset = 0, sortBy: 'title' | 'artist' | 'year' = 'title') =>
		apiFetch<{ items: Album[]; total: number }>(`/library/albums?limit=${limit}&offset=${offset}&sort_by=${sortBy}`),
	artists: (limit = 50, offset = 0) =>
		apiFetch<Artist[]>(`/library/artists?limit=${limit}&offset=${offset}`),
	album: (id: string) => apiFetch<{ album: Album; tracks: Track[]; artist?: Artist; genres?: Genre[]; variants?: Album[] }>(`/library/albums/${id}`),
	artist: (id: string) => apiFetch<{ artist: Artist; albums: Album[]; genres?: Genre[]; related_artists?: RelatedArtist[]; appears_on?: Album[] }>(`/library/artists/${id}`),
	artistBio: (id: string) => apiFetch<{ bio: string; bio_url: string }>(`/library/artists/${id}/bio`),
	track: (id: string) => apiFetch<{ track: Track; genres?: Genre[] }>(`/library/tracks/${id}`),
	trackWaveform: (id: string) => apiFetch<{ peaks: number[] }>(`/library/tracks/${id}/waveform`),
	genres: () => apiFetch<Genre[]>('/library/genres'),
	genreArtists: (id: string, limit = 50, offset = 0) =>
		apiFetch<Artist[]>(`/library/genres/${id}/artists?limit=${limit}&offset=${offset}`),
	genreDetail: (id: string) => apiFetch<Genre>(`/library/genres/${id}`),
	genreAlbums: (id: string, limit = 50, offset = 0) =>
		apiFetch<Album[]>(`/library/genres/${id}/albums?limit=${limit}&offset=${offset}`),
	addTrack: (id: string) => apiFetch(`/library/tracks/${id}`, { method: 'POST' }),
	removeTrack: (id: string) => apiFetch(`/library/tracks/${id}`, { method: 'DELETE' }),
	search: (q: string, filters?: SearchFilters) => {
		const p = new URLSearchParams({ q });
		if (filters) {
			if (filters.genre) p.set('genre', filters.genre);
			if (filters.year_from) p.set('year_from', String(filters.year_from));
			if (filters.year_to) p.set('year_to', String(filters.year_to));
			if (filters.format) p.set('format', filters.format);
			if (filters.bitrate_min) p.set('bitrate_min', String(filters.bitrate_min));
			if (filters.bitrate_max) p.set('bitrate_max', String(filters.bitrate_max));
			if (filters.bpm_min) p.set('bpm_min', String(filters.bpm_min));
			if (filters.bpm_max) p.set('bpm_max', String(filters.bpm_max));
			if (filters.types?.length) p.set('types', filters.types.join(','));
			if (filters.sort_tracks) p.set('sort_tracks', filters.sort_tracks);
			if (filters.sort_albums) p.set('sort_albums', filters.sort_albums);
		}
		return apiFetch<{ tracks: Track[]; albums: Album[]; artists: Artist[] }>(`/library/search?${p}`);
	},
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
	ratings: () => apiFetch<Record<string, number>>('/library/ratings'),
	setRating: (id: string, rating: number) =>
		apiFetch(`/library/ratings/${id}`, {
			method: 'PUT',
			headers: { 'Content-Type': 'application/json' },
			body: JSON.stringify({ rating }),
		}),
	deleteRating: (id: string) => apiFetch(`/library/ratings/${id}`, { method: 'DELETE' }),
	// Batch-fetch tracks by IDs in a single request (used for queue restore).
	tracksBatch: (ids: string[]) =>
		apiFetch<Track[]>(`/library/tracks/batch?ids=${ids.join(',')}`),
	lyrics: (trackId: string) =>
		apiFetch<{ time_ms: number; text: string }[]>(`/library/tracks/${trackId}/lyrics`),
	setLyrics: (trackId: string, lyrics: string) =>
		apiFetch(`/library/tracks/${trackId}/lyrics`, {
			method: 'PUT',
			headers: { 'Content-Type': 'application/json' },
			body: JSON.stringify({ lyrics })
		})
};
