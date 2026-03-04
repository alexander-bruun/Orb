import { apiFetch } from './client';
import type { Track } from '$lib/types';

export interface ScoredTrack extends Track {
	similarity_score?: number;
}

export const recommend = {
	similar: (trackId: string, limit = 20, excludeAlbumId?: string) => {
		const params = new URLSearchParams({ limit: String(limit) });
		if (excludeAlbumId) params.set('exclude_album', excludeAlbumId);
		return apiFetch<ScoredTrack[]>(`/recommend/similar/${trackId}?${params}`);
	},

	radio: (limit = 50) => apiFetch<ScoredTrack[]>(`/recommend/radio?limit=${limit}`),

	autoplay: (afterTrackId: string, exclude: string[] = [], limit = 5) => {
		const params = new URLSearchParams({ after: afterTrackId, limit: String(limit) });
		if (exclude.length) params.set('exclude', exclude.join(','));
		return apiFetch<ScoredTrack[]>(`/recommend/autoplay?${params}`);
	}
};
