import { apiFetch } from './client';
import type { Playlist, Track } from '$lib/types';

export const playlists = {
	list: () => apiFetch<Playlist[]>('/playlists'),
	create: (name: string, description?: string) =>
		apiFetch<Playlist>('/playlists', {
			method: 'POST',
			body: JSON.stringify({ name, description })
		}),
	get: (id: string) =>
		apiFetch<{ playlist: Playlist; tracks: Track[] }>(`/playlists/${id}`),
	update: (id: string, name: string, description?: string) =>
		apiFetch<Playlist>(`/playlists/${id}`, {
			method: 'PATCH',
			body: JSON.stringify({ name, description })
		}),
	delete: (id: string) => apiFetch(`/playlists/${id}`, { method: 'DELETE' }),
	addTrack: (id: string, trackId: string) =>
		apiFetch(`/playlists/${id}/tracks`, {
			method: 'POST',
			body: JSON.stringify({ track_id: trackId })
		}),
	removeTrack: (id: string, trackId: string) =>
		apiFetch(`/playlists/${id}/tracks/${trackId}`, { method: 'DELETE' }),
	reorder: (id: string, order: Array<{ track_id: string; position: number }>) =>
		apiFetch(`/playlists/${id}/tracks/order`, {
			method: 'PUT',
			body: JSON.stringify({ order })
		})
};
