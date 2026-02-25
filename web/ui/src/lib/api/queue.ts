import { apiFetch } from './client';
import type { Track } from '$lib/types';

export const queue = {
	get: () => apiFetch<Track[]>('/queue'),
	replace: (trackIds: string[], source?: string) =>
		apiFetch('/queue', { method: 'PUT', body: JSON.stringify({ track_ids: trackIds, source }) }),
	clear: () => apiFetch('/queue', { method: 'DELETE' }),
	addNext: (trackId: string, source?: string) =>
		apiFetch('/queue/next', { method: 'POST', body: JSON.stringify({ track_id: trackId, source }) }),
	addLast: (trackId: string, source?: string) =>
		apiFetch('/queue/last', { method: 'POST', body: JSON.stringify({ track_id: trackId, source }) })
};
