import { writable, derived } from 'svelte/store';
import { currentTrack, positionMs } from '.';
import { apiFetch } from '$lib/api/client';
import { getOfflineLyrics } from '$lib/stores/offline/downloads';

export interface LyricLine {
	time_ms: number;
	text: string;
}

export type LyricsMode = 'modal' | 'overlay' | 'teleprompter';

function makeLyricsModeStore() {
	const stored = typeof localStorage !== 'undefined'
		? (localStorage.getItem('orb_lyrics_mode') as LyricsMode | null)
		: null;
	const { subscribe, set, update } = writable<LyricsMode>(stored ?? 'modal');
	return {
		subscribe,
		update,
		set(v: LyricsMode) {
			if (typeof localStorage !== 'undefined') localStorage.setItem('orb_lyrics_mode', v);
			set(v);
		},
	};
}

export const lyricsOpen = writable(false);
export const lyricsMode = makeLyricsModeStore();
export const lyricsLines = writable<LyricLine[]>([]);
export const lyricsLoading = writable(false);

/** Index of the lyric line that should be highlighted at the current playback position. */
export const activeLyricIndex = derived(
	[lyricsLines, positionMs],
	([$lines, $pos]) => {
		if ($lines.length === 0) return -1;
		let idx = -1;
		for (let i = 0; i < $lines.length; i++) {
			if ($lines[i].time_ms <= $pos) idx = i;
			else break;
		}
		return idx;
	}
);

let loadedTrackId: string | null = null;

currentTrack.subscribe(async (track) => {
	if (!track) {
		lyricsLines.set([]);
		loadedTrackId = null;
		return;
	}
	if (track.id === loadedTrackId) return;
	loadedTrackId = track.id;
	lyricsLoading.set(true);
	lyricsLines.set([]);
	try {
		const lines = await apiFetch<LyricLine[]>(`/library/tracks/${track.id}/lyrics`);
		if (loadedTrackId === track.id) {
			lyricsLines.set(lines ?? []);
		}
	} catch {
		// Try offline cache before giving up.
		const cached = await getOfflineLyrics(track.id);
		if (loadedTrackId === track.id) {
			lyricsLines.set(cached ?? []);
		}
	} finally {
		if (loadedTrackId === track.id) {
			lyricsLoading.set(false);
		}
	}
});
