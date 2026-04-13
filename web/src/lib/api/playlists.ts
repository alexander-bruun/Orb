// Returns up to 4 individual album cover URLs for a playlist's tracks.
// The frontend renders these as a 2x2 CSS grid — no server-side compositing needed.
export async function getPlaylistCoverGrid(id: string): Promise<string[]> {
	const { getApiBase } = await import('./base');
	const base = getApiBase();
	try {
		const res = await apiFetch<{ tracks: Array<{ album_id?: string | null }> }>(`/playlists/${id}`);
		const seen = new Set<string>();
		const urls: string[] = [];
		for (const t of res.tracks ?? []) {
			if (t.album_id && !seen.has(t.album_id)) {
				seen.add(t.album_id);
				urls.push(`${base}/covers/${t.album_id}`);
				if (urls.length >= 4) break;
			}
		}
		return urls;
	} catch {
		return [];
	}
}
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
	update: (id: string, fields: { name?: string; description?: string; is_public?: boolean }) =>
		apiFetch<Playlist>(`/playlists/${id}`, {
			method: 'PATCH',
			body: JSON.stringify(fields)
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
