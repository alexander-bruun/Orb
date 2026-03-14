import { apiFetch } from './client';
import type { SmartPlaylist, SmartPlaylistRule, Track } from '$lib/types';

export interface SmartPlaylistUpsert {
	name?: string;
	description?: string;
	rules?: SmartPlaylistRule[];
	rule_match?: 'all' | 'any';
	sort_by?: string;
	sort_dir?: 'asc' | 'desc';
	limit_count?: number | null;
}

export const smartPlaylists = {
	list: () => apiFetch<SmartPlaylist[]>('/smart-playlists'),

	get: (id: string) => apiFetch<SmartPlaylist>(`/smart-playlists/${id}`),

	create: (data: SmartPlaylistUpsert) =>
		apiFetch<SmartPlaylist>('/smart-playlists', {
			method: 'POST',
			headers: { 'Content-Type': 'application/json' },
			body: JSON.stringify(data)
		}),

	update: (id: string, data: SmartPlaylistUpsert) =>
		apiFetch<SmartPlaylist>(`/smart-playlists/${id}`, {
			method: 'PATCH',
			headers: { 'Content-Type': 'application/json' },
			body: JSON.stringify(data)
		}),

	delete: (id: string) =>
		apiFetch<void>(`/smart-playlists/${id}`, { method: 'DELETE' }),

	tracks: (id: string) => apiFetch<Track[]>(`/smart-playlists/${id}/tracks`)
};
