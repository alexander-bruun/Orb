/**
 * Audiobook state persistence to localStorage and backend.
 *
 * Extracted from audiobookPlayer.ts for modularity.
 */

import { get } from 'svelte/store';
import { browser } from '$app/environment';
import { audiobooks as audiobooksApi } from '$lib/api/audiobooks';
import { currentAudiobook, abPositionMs } from './audiobookPlayer';
import { TIMINGS } from '$lib/constants';

// ── Progress persistence ──────────────────────────────────────────────────────

let _lastSavedMs = -1;
let _saveInterval: ReturnType<typeof setInterval> | null = null;

export function persistProgress(completed = false) {
	const book = get(currentAudiobook);
	if (!book) return;
	const pos = get(abPositionMs);
	if (pos === _lastSavedMs && !completed) return;
	_lastSavedMs = pos;
	audiobooksApi.saveProgress(book.id, pos, completed).catch(() => { });
}

export function startSaveInterval() {
	stopSaveInterval();
	_saveInterval = setInterval(persistProgress, TIMINGS.AUDIOBOOK_SAVE_INTERVAL);
}

export function stopSaveInterval() {
	if (_saveInterval !== null) {
		clearInterval(_saveInterval);
		_saveInterval = null;
	}
}

// ── Persist on page unload ────────────────────────────────────────────────────

if (browser) {
	window.addEventListener('beforeunload', () => {
		persistProgress();
	});
}
