/**
 * Audiobook player store — completely separate from the music player.
 *
 * Features:
 *  - Variable playback speed (0.5× – 2×)
 *  - Sleep timer (auto-pause after N minutes)
 *  - Per-book resume position (saved to backend every 10 s and on pause/unload)
 *  - Chapter-aware: current chapter derived from positionMs
 *  - Bookmarks (create / delete)
 *  - Multi-file mode: per-chapter MP3 files streamed sequentially
 */

import { writable, derived, get } from 'svelte/store';
import { browser } from '$app/environment';
import { getApiBase } from '$lib/api/base';
import { audiobooks as audiobooksApi } from '$lib/api/audiobooks';
import { authStore } from '$lib/stores/auth';
import type { Audiobook, AudiobookChapter, AudiobookBookmark } from '$lib/types';
import { activePlayer } from '$lib/stores/activePlayer';

// ── Playback state ────────────────────────────────────────────────────────────

export type ABPlaybackState = 'idle' | 'loading' | 'playing' | 'paused';

export const currentAudiobook   = writable<Audiobook | null>(null);
export const abPlaybackState    = writable<ABPlaybackState>('idle');
export const abPositionMs       = writable(0);
export const abDurationMs       = writable(0);
export const abBufferedPct      = writable(0);
export const abSpeed            = writable(1.0);
export const abVolume           = writable(1.0);
export const abBookmarks        = writable<AudiobookBookmark[]>([]);

// Sleep timer: minutes remaining (0 = off)
export const sleepTimerMins     = writable(0);

// ── Derived ───────────────────────────────────────────────────────────────────

export const abCurrentChapter = derived(
	[currentAudiobook, abPositionMs],
	([$book, $pos]) => {
		if (!$book?.chapters?.length) return null;
		let current: AudiobookChapter | null = null;
		for (const ch of $book.chapters) {
			if ($pos >= ch.start_ms) current = ch;
			else break;
		}
		return current;
	}
);

export const abProgress = derived(
	[abPositionMs, abDurationMs],
	([$pos, $dur]) => ($dur > 0 ? ($pos / $dur) * 100 : 0)
);

export const abFormattedPosition = derived(abPositionMs, ($ms) => formatMs($ms));
export const abFormattedDuration = derived(abDurationMs, ($ms) => formatMs($ms));

export const abFormattedFormat = derived(currentAudiobook, ($book) => {
	if (!$book?.format) return '';
	return $book.format.toUpperCase();
});

// ── Internal audio element ────────────────────────────────────────────────────

let _audio: HTMLAudioElement | null = null;
let _saveInterval: ReturnType<typeof setInterval> | null = null;
let _sleepTimeout: ReturnType<typeof setTimeout> | null = null;

// Multi-file chapter tracking
let _isMultiFile = false;
let _currentChapterIndex = 0;

function _isMultiFileBook(book: Audiobook): boolean {
	return (book.chapters?.length ?? 0) > 0 && book.chapters![0].file_key != null;
}

function _streamToken(): string {
	const token = get(authStore).token;
	return token ? `?token=${encodeURIComponent(token)}` : '';
}

function _chapterUrl(chapter: AudiobookChapter): string {
	return `${getApiBase()}/stream/audiobook/chapter/${chapter.id}${_streamToken()}`;
}

function _findChapterIndex(book: Audiobook, posMs: number): number {
	const chapters = book.chapters ?? [];
	let idx = 0;
	for (let i = 0; i < chapters.length; i++) {
		if (posMs >= chapters[i].start_ms) idx = i;
		else break;
	}
	return idx;
}

function getAudio(): HTMLAudioElement {
	if (!_audio) {
		_audio = new Audio();
		_audio.preload = 'metadata';

		_audio.addEventListener('loadedmetadata', () => {
			// For multi-file mode, total duration comes from book.duration_ms (set in playAudiobook).
			// For single-file mode, derive from the audio element.
			if (!_isMultiFile) {
				abDurationMs.set(Math.round(_audio!.duration * 1000));
			}
		});

		_audio.addEventListener('timeupdate', () => {
			let pos: number;
			if (_isMultiFile) {
				const book = get(currentAudiobook);
				const chapters = book?.chapters ?? [];
				const chOffset = chapters[_currentChapterIndex]?.start_ms ?? 0;
				pos = chOffset + Math.round(_audio!.currentTime * 1000);
			} else {
				pos = Math.round(_audio!.currentTime * 1000);
			}
			abPositionMs.set(pos);
			if (_audio!.buffered.length > 0 && _audio!.duration > 0) {
				abBufferedPct.set(
					(_audio!.buffered.end(_audio!.buffered.length - 1) / _audio!.duration) * 100
				);
			}
		});

		// 'play' fires when audio.play() is called (but may still buffer).
		// 'playing' fires when buffering resolves and audio is actually running.
		_audio.addEventListener('play', () => abPlaybackState.set('playing'));
		_audio.addEventListener('playing', () => abPlaybackState.set('playing'));
		_audio.addEventListener('pause', () => {
			abPlaybackState.set('paused');
			_persistProgress();
		});
		_audio.addEventListener('ended', () => {
			if (_isMultiFile) {
				const book = get(currentAudiobook);
				const chapters = book?.chapters ?? [];
				if (_currentChapterIndex < chapters.length - 1) {
					_currentChapterIndex++;
					_loadChapter(chapters[_currentChapterIndex], 0, true);
				} else {
					abPlaybackState.set('paused');
					_persistProgress(true);
				}
			} else {
				abPlaybackState.set('paused');
				_persistProgress(true);
			}
		});

		_audio.addEventListener('waiting', () => abPlaybackState.set('loading'));
		_audio.addEventListener('canplay', () => {
			if (get(abPlaybackState) === 'loading') {
				abPlaybackState.set(_audio?.paused ? 'paused' : 'playing');
			}
		});
		_audio.addEventListener('error', () => {
			if (get(abPlaybackState) === 'loading') {
				abPlaybackState.set('paused');
			}
		});
	}
	return _audio;
}

// ── Progress persistence ──────────────────────────────────────────────────────

let _lastSavedMs = -1;

function _persistProgress(completed = false) {
	const book = get(currentAudiobook);
	if (!book) return;
	const pos = get(abPositionMs);
	if (pos === _lastSavedMs && !completed) return;
	_lastSavedMs = pos;
	audiobooksApi.saveProgress(book.id, pos, completed).catch(() => {});
}

function _startSaveInterval() {
	_stopSaveInterval();
	_saveInterval = setInterval(_persistProgress, 10_000);
}

function _stopSaveInterval() {
	if (_saveInterval !== null) {
		clearInterval(_saveInterval);
		_saveInterval = null;
	}
}

// ── Chapter loading helper ────────────────────────────────────────────────────

function _loadChapter(chapter: AudiobookChapter, seekMs: number, autoPlay: boolean) {
	const audio = getAudio();
	const url = _chapterUrl(chapter);
	audio.src = url;
	audio.playbackRate = get(abSpeed);
	audio.volume = get(abVolume);
	abPlaybackState.set('loading');

	const seekAndMaybePlay = () => {
		if (seekMs > 0) {
			audio.currentTime = seekMs / 1000;
		}
		if (autoPlay) {
			audio.play().catch(() => abPlaybackState.set('paused'));
			_startSaveInterval();
		}
	};

	if (audio.readyState >= 1) {
		seekAndMaybePlay();
	} else {
		audio.addEventListener('loadedmetadata', seekAndMaybePlay, { once: true });
	}
}

// ── Sleep timer ───────────────────────────────────────────────────────────────

export function setSleepTimer(minutes: number) {
	if (_sleepTimeout !== null) {
		clearTimeout(_sleepTimeout);
		_sleepTimeout = null;
	}
	sleepTimerMins.set(minutes);
	if (minutes > 0) {
		_sleepTimeout = setTimeout(() => {
			pauseAudiobook();
			sleepTimerMins.set(0);
		}, minutes * 60_000);
	}
}

// ── Public API ────────────────────────────────────────────────────────────────

export function restoreAudiobookState(book: Audiobook, posMs: number) {
	currentAudiobook.set(book);
	abDurationMs.set(book.duration_ms ?? 0);
	abPositionMs.set(posMs);
	abPlaybackState.set('paused');
	activePlayer.set('audiobook');
	_isMultiFile = _isMultiFileBook(book);
	_currentChapterIndex = _findChapterIndex(book, posMs);
}

export async function playAudiobook(book: Audiobook, startMs?: number) {
	const audio = getAudio();

	activePlayer.set('audiobook');
	currentAudiobook.set(book);
	abPlaybackState.set('loading');
	abBookmarks.set([]);

	_isMultiFile = _isMultiFileBook(book);

	// For multi-file mode, set total duration from book metadata immediately
	if (_isMultiFile) {
		abDurationMs.set(book.duration_ms ?? 0);
	}

	// Determine resume position
	let resumeMs = startMs;
	if (resumeMs === undefined) {
		try {
			const { progress } = await audiobooksApi.getProgress(book.id);
			resumeMs = progress.position_ms;
		} catch {
			resumeMs = 0;
		}
	}
	resumeMs = resumeMs ?? 0;

	// Load bookmarks in parallel
	audiobooksApi.listBookmarks(book.id)
		.then(({ bookmarks }) => abBookmarks.set(bookmarks))
		.catch(() => {});

	if (_isMultiFile) {
		const chapters = book.chapters ?? [];
		_currentChapterIndex = _findChapterIndex(book, resumeMs);
		const chapter = chapters[_currentChapterIndex];
		const chapterOffsetMs = resumeMs - (chapter?.start_ms ?? 0);
		_loadChapter(chapter, chapterOffsetMs, true);
	} else {
		const src = `${getApiBase()}/stream/audiobook/${book.id}${_streamToken()}`;
		if (audio.src !== src) {
			audio.src = src;
		}
		audio.playbackRate = get(abSpeed);
		audio.volume = get(abVolume);

		const seekAndPlay = () => {
			if (resumeMs && resumeMs > 0) {
				audio.currentTime = resumeMs / 1000;
			}
			audio.play().catch(() => abPlaybackState.set('paused'));
			_startSaveInterval();
		};

		if (audio.readyState >= 1) {
			seekAndPlay();
		} else {
			audio.addEventListener('loadedmetadata', seekAndPlay, { once: true });
		}
	}
}

export function toggleABPlayPause() {
	const audio = getAudio();
	const book = get(currentAudiobook);
	if (audio.paused) {
		if (book && !audio.src) {
			playAudiobook(book, get(abPositionMs));
			return;
		}
		activePlayer.set('audiobook');
		audio.play().catch(() => {});
		_startSaveInterval();
	} else {
		audio.pause();
		_stopSaveInterval();
	}
}

export function seekAudiobook(seconds: number) {
	if (_isMultiFile) {
		seekAudiobookMs(Math.round(seconds * 1000));
	} else {
		const audio = getAudio();
		audio.currentTime = seconds;
		abPositionMs.set(Math.round(seconds * 1000));
	}
}

export function seekAudiobookMs(ms: number) {
	if (_isMultiFile) {
		const book = get(currentAudiobook);
		if (!book) return;
		const chapters = book.chapters ?? [];
		const newIdx = _findChapterIndex(book, ms);
		const chapter = chapters[newIdx];
		const chapterOffsetMs = ms - (chapter?.start_ms ?? 0);
		const wasPlaying = _audio != null && !_audio.paused;

		if (newIdx !== _currentChapterIndex) {
			_currentChapterIndex = newIdx;
			_loadChapter(chapter, chapterOffsetMs, wasPlaying);
			abPositionMs.set(ms);
		} else {
			getAudio().currentTime = chapterOffsetMs / 1000;
			abPositionMs.set(ms);
		}
	} else {
		seekAudiobook(ms / 1000);
	}
}

export function skipForward(seconds = 30) {
	if (_isMultiFile) {
		const newMs = get(abPositionMs) + seconds * 1000;
		seekAudiobookMs(Math.min(newMs, get(abDurationMs)));
	} else {
		const audio = getAudio();
		audio.currentTime = Math.min(audio.currentTime + seconds, audio.duration || 0);
	}
}

export function skipBackward(seconds = 10) {
	if (_isMultiFile) {
		const newMs = get(abPositionMs) - seconds * 1000;
		seekAudiobookMs(Math.max(newMs, 0));
	} else {
		const audio = getAudio();
		audio.currentTime = Math.max(audio.currentTime - seconds, 0);
	}
}

export function setABSpeed(rate: number) {
	abSpeed.set(rate);
	if (_audio) _audio.playbackRate = rate;
}

export function setABVolume(v: number) {
	abVolume.set(v);
	if (_audio) _audio.volume = v;
}

export function jumpToChapter(chapter: AudiobookChapter) {
	if (_isMultiFile) {
		const book = get(currentAudiobook);
		const chapters = book?.chapters ?? [];
		const idx = chapters.findIndex((ch) => ch.id === chapter.id);
		if (idx >= 0) {
			const wasPlaying = _audio != null && !_audio.paused;
			_currentChapterIndex = idx;
			_loadChapter(chapter, 0, wasPlaying);
			abPositionMs.set(chapter.start_ms);
		}
	} else {
		seekAudiobookMs(chapter.start_ms);
	}
}

export function pauseAudiobook() {
	_audio?.pause();
	_stopSaveInterval();
}

export function closeAudiobook() {
	pauseAudiobook();
	_persistProgress();
	_audio?.removeAttribute('src');
	_audio?.load();
	currentAudiobook.set(null);
	abPlaybackState.set('idle');
	abPositionMs.set(0);
	abDurationMs.set(0);
	abBookmarks.set([]);
	_isMultiFile = false;
	_currentChapterIndex = 0;
}

// ── Bookmark helpers ──────────────────────────────────────────────────────────

export async function createBookmark(note?: string) {
	const book = get(currentAudiobook);
	if (!book) return;
	const pos = get(abPositionMs);
	const { bookmark } = await audiobooksApi.createBookmark(book.id, pos, note);
	abBookmarks.update((bms) => [...bms, bookmark].sort((a, b) => a.position_ms - b.position_ms));
}

export async function deleteBookmark(bookmarkId: string) {
	const book = get(currentAudiobook);
	if (!book) return;
	await audiobooksApi.deleteBookmark(book.id, bookmarkId);
	abBookmarks.update((bms) => bms.filter((b) => b.id !== bookmarkId));
}

// ── Persist on page unload ────────────────────────────────────────────────────

if (browser) {
	window.addEventListener('beforeunload', () => {
		_persistProgress();
	});
}

// ── Utilities ─────────────────────────────────────────────────────────────────

function formatMs(ms: number): string {
	const totalSecs = Math.floor(ms / 1000);
	const h = Math.floor(totalSecs / 3600);
	const m = Math.floor((totalSecs % 3600) / 60);
	const s = totalSecs % 60;
	if (h > 0) {
		return `${h}:${String(m).padStart(2, '0')}:${String(s).padStart(2, '0')}`;
	}
	return `${m}:${String(s).padStart(2, '0')}`;
}

export const AB_SPEEDS = [0.5, 0.75, 1.0, 1.25, 1.5, 1.75, 2.0];
export const SLEEP_PRESETS = [5, 10, 15, 20, 30, 45, 60];
