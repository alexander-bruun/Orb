import { apiFetch } from './client';
import type { Audiobook, AudiobookProgress, AudiobookBookmark } from '$lib/types';

export const audiobooks = {
	list(limit = 50, offset = 0, sortBy = 'title'): Promise<{ audiobooks: Audiobook[] }> {
		return apiFetch(`/audiobooks?limit=${limit}&offset=${offset}&sort_by=${sortBy}`);
	},

	inProgress(limit = 10): Promise<{ audiobooks: (Audiobook & { position_ms: number; progress_updated_at: string })[] }> {
		return apiFetch(`/audiobooks/in-progress?limit=${limit}`);
	},

	get(id: string): Promise<{ audiobook: Audiobook }> {
		return apiFetch(`/audiobooks/${id}`);
	},

	getProgress(id: string): Promise<{ progress: AudiobookProgress }> {
		return apiFetch(`/audiobooks/${id}/progress`);
	},

	saveProgress(id: string, positionMs: number, completed = false): Promise<void> {
		return apiFetch(`/audiobooks/${id}/progress`, {
			method: 'PUT',
			headers: { 'Content-Type': 'application/json' },
			body: JSON.stringify({ position_ms: positionMs, completed }),
		});
	},

	listBookmarks(id: string): Promise<{ bookmarks: AudiobookBookmark[] }> {
		return apiFetch(`/audiobooks/${id}/bookmarks`);
	},

	createBookmark(id: string, positionMs: number, note?: string): Promise<{ bookmark: AudiobookBookmark }> {
		return apiFetch(`/audiobooks/${id}/bookmarks`, {
			method: 'POST',
			headers: { 'Content-Type': 'application/json' },
			body: JSON.stringify({ position_ms: positionMs, note }),
		});
	},

	deleteBookmark(audiobookId: string, bookmarkId: string): Promise<void> {
		return apiFetch(`/audiobooks/${audiobookId}/bookmarks/${bookmarkId}`, { method: 'DELETE' });
	},

	listBySeries(series: string): Promise<{ audiobooks: Audiobook[]; series: string }> {
		return apiFetch(`/audiobooks/series/${encodeURIComponent(series)}`);
	},

	triggerScan(): Promise<{ status: string }> {
		return apiFetch('/audiobooks/admin/scan', { method: 'POST' });
	},
};
