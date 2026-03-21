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
import { invoke } from "@tauri-apps/api/core";
import { listen } from "@tauri-apps/api/event";
import { getApiBase } from '$lib/api/base';
import { audiobooks as audiobooksApi } from '$lib/api/audiobooks';
import { authStore } from '$lib/stores/auth';
import type { Audiobook, AudiobookChapter, AudiobookBookmark } from '$lib/types';
import { isNative } from '$lib/utils/platform';
import { exclusiveMode, deviceId, activeDeviceId, sendHeartbeat } from './deviceSession';
import { devices as devicesApi } from '$lib/api/devices';
import { isCurrentlyOffline } from '$lib/stores/offline/connectivity';
import * as engine from './engine';
import type { ContentProvider } from './engine';

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

// Chapter-aware progress: position within the current chapter
export const abChapterProgress = derived(
	[currentAudiobook, abPositionMs, abCurrentChapter],
	([$book, $pos, $chapter]) => {
		if (!$chapter) return 0;
		const nextChapter = $book?.chapters?.find(ch => ch.start_ms > $chapter.start_ms);
		const chapterDurationMs = nextChapter ? nextChapter.start_ms - $chapter.start_ms : ($book?.duration_ms ?? 0) - $chapter.start_ms;
		const posInChapter = $pos - $chapter.start_ms;
		return chapterDurationMs > 0 ? Math.max(0, Math.min(100, (posInChapter / chapterDurationMs) * 100)) : 0;
	}
);

// Previous chapter
export const abPreviousChapter = derived(
	[currentAudiobook, abCurrentChapter],
	([$book, $current]) => {
		if (!$book?.chapters || !$current) return null;
		const idx = $book.chapters.findIndex(ch => ch.id === $current.id);
		return idx > 0 ? $book.chapters[idx - 1] : null;
	}
);

// Next chapter
export const abNextChapter = derived(
	[currentAudiobook, abCurrentChapter],
	([$book, $current]) => {
		if (!$book?.chapters || !$current) return null;
		const idx = $book.chapters.findIndex(ch => ch.id === $current.id);
		return idx >= 0 && idx < $book.chapters.length - 1 ? $book.chapters[idx + 1] : null;
	}
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

// Native Android playback
let _nativePositionInterval: ReturnType<typeof setInterval> | null = null;
let _nativeInitialized = false;
let _lastSeekMs = 0; // Track when we last seeked to avoid polling overwriting seek position

// Shadow epoch for calculating position without relying on polling alone
// When playback starts or seeks, we record when the position would be 0
// Then calculate current position as Date.now() - _abShadowEpochMs
let _abShadowEpochMs = 0;

// Shadow tick for idle-device mirroring: advances abPositionMs between
// heartbeats (30s gap) so the progress bar moves smoothly.
let _abShadowTickTimer: ReturnType<typeof setInterval> | null = null;
let _abShadowTickSpeed = 1.0;
let _abShadowBasePos = 0;
let _abShadowStartTime = 0;

function _startABShadowTick(posMs: number, speed: number) {
	_stopABShadowTick();
	// Delegate to the unified engine's shadow tick (speed-aware).
	engine.startShadowTick(posMs, speed);
	// Also keep local state in sync for the legacy store.
	_abShadowBasePos = posMs;
	_abShadowStartTime = Date.now();
	_abShadowTickSpeed = speed;
	_abShadowTickTimer = setInterval(() => {
		const elapsed = Date.now() - _abShadowStartTime;
		const pos = Math.round(_abShadowBasePos + elapsed * _abShadowTickSpeed);
		abPositionMs.set(pos);
	}, 250);
}

function _stopABShadowTick() {
	engine.stopShadowTick();
	if (_abShadowTickTimer !== null) {
		clearInterval(_abShadowTickTimer);
		_abShadowTickTimer = null;
	}
}

function _isMultiFileBook(book: Audiobook): boolean {
	return (book.chapters?.length ?? 0) > 0 && book.chapters![0].file_key != null;
}

async function _initNativeListeners() {
	if (_nativeInitialized || !isNative()) return;
	_nativeInitialized = true;

	// Audiobook-specific notification actions are registered here but will
	// also be handled by the engine's initNativeListeners. During the
	// migration both registrations coexist safely (duplicate listeners are
	// harmless — only the first to call the action wins).
	listen<void>('native-ab-skip-back-15', () => skipBackward(15));
	listen<void>('native-ab-skip-forward-15', () => skipForward(15));
	listen<void>('native-ab-speed-cycle', () => _cycleSpeed());
	listen<void>('native-ab-chapter-start', () => _jumpToChapterStart());
	// native-pause / native-play are now handled by the unified engine.
	// The engine updates enginePlaybackState centrally for all modes.
}

function _cycleSpeed() {
	const current = get(abSpeed);
	const nextSpeed = AB_SPEEDS[(AB_SPEEDS.indexOf(current) + 1) % AB_SPEEDS.length];
	setABSpeed(nextSpeed);
}

function _jumpToChapterStart() {
	const chapter = get(abCurrentChapter);
	if (chapter) {
		seekAudiobookMs(chapter.start_ms);
	}
}

async function _resolveNativeChapterUrl(chapter: AudiobookChapter): Promise<string> {
	try {
		const path = await invoke<string | null>('get_offline_file_path', { trackId: chapter.id });
		if (path) return `file://${path}`;
	} catch { /* fall through */ }
	// Single-file books use /stream/audiobook/{id}, multi-file use /stream/audiobook/chapter/{id}
	if (_isMultiFile) {
		return _chapterUrl(chapter);
	}
	return `${getApiBase()}/stream/audiobook/${chapter.id}${_streamToken()}`;
}

async function _playNativeAudiobook(chapter: AudiobookChapter, autoPlay: boolean) {
	if (!isNative()) return;
	const url = await _resolveNativeChapterUrl(chapter);
	const book = get(currentAudiobook);
	const title = book ? `${book.title}` : 'Audiobook';
	const artist = book?.author_name || 'Unknown';

	// Build a full cover URL for the Android notification (not just the storage key).
	const token = get(authStore).token ?? '';
	const coverUrl = book?.id
		? `${getApiBase()}/covers/audiobook/${book.id}?token=${encodeURIComponent(token)}`
		: undefined;

	try {
		await engine.play(url, {
			id: book?.id ?? chapter.id,
			title,
			artist,
			coverUrl,
			durationMs: book?.duration_ms ?? 0,
		}, {
			isAudiobook: true,
			speed: get(abSpeed),
			nativeUrl: url,
		});
		if (!autoPlay) {
			engine.pause();
		}
	} catch (e) {
		console.error('Failed to play via native:', e);
	}
}

function _pauseNative() {
	if (!isNative()) return;
	engine.pause();
}

function _resumeNative() {
	if (!isNative()) return;
	engine.resume();
}

async function _seekNative(positionMs: number) {
	if (!isNative()) return;
	engine.seek(positionMs);
}

function _stopNativePlayback() {
	if (!isNative()) return;
	engine.pause();
}

function _startNativePositionPolling() {
	if (!isNative()) return;
	_stopNativePositionPolling();
	_nativePositionInterval = setInterval(async () => {
		try {
			const now = Date.now();

			// Use shadow epoch to calculate expected position (like music player does)
			// This provides smooth position updates without snapping back
			let pos: number;
			if (_abShadowEpochMs > 0 && now - _lastSeekMs >= 500) {
				// Calculate expected position based on when playback started
				pos = now - _abShadowEpochMs;
			} else {
				// Within 500ms of a seek, poll the actual position to verify seek completed.
				// ExoPlayer reports position within the current chapter file, so for
				// multi-file books we add the chapter's start_ms to get the absolute
				// book position.
				let rawPos = await invoke<number>('get_position_music');
				if (_isMultiFile) {
					const book = get(currentAudiobook);
					const chapterStart = book?.chapters?.[_currentChapterIndex]?.start_ms ?? 0;
					rawPos += chapterStart;
				}
				pos = rawPos;
			}

			abPositionMs.set(pos);

			// Check for chapter transition
			const book = get(currentAudiobook);
			if (book && _isMultiFile) {
				const newIdx = _findChapterIndex(book, pos);
				if (newIdx !== _currentChapterIndex) {
					const chapters = book.chapters ?? [];
					const chapter = chapters[newIdx];
					if (chapter) {
						_currentChapterIndex = newIdx;
						const chapterOffsetMs = pos - (chapter?.start_ms ?? 0);
						await _playNativeAudiobook(chapter, true);
						await _seekNative(chapterOffsetMs);
					}
				}
			}
		} catch (e) {
			// Ignore errors during polling
		}
	}, 250);
}

function _stopNativePositionPolling() {
	if (_nativePositionInterval !== null) {
		clearInterval(_nativePositionInterval);
		_nativePositionInterval = null;
	}
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
	engine.switchMode('audiobook');
	_isMultiFile = _isMultiFileBook(book);
	_currentChapterIndex = _findChapterIndex(book, posMs);
}

export async function playAudiobook(book: Audiobook, startMs?: number) {
	_stopABShadowTick(); // Stop remote mirror tick — we're playing locally now
	const audio = getAudio();

	engine.switchMode('audiobook');
	currentAudiobook.set(book); // Set immediately so UI shows up
	abPlaybackState.set('loading');
	abBookmarks.set([]);

	let fullBook = book;
	if (!book.chapters || book.chapters.length === 0) {
		try {
			const res = await audiobooksApi.get(book.id);
			fullBook = res.audiobook;
			currentAudiobook.set(fullBook); // Update with full details (chapters, etc.)
		} catch (e) {
			console.error('Failed to fetch full audiobook details', e);
		}
	}

	_isMultiFile = _isMultiFileBook(fullBook);

	// Set engine's current content with the BOOK ID (not chapter ID) so that
	// heartbeats contain the correct audiobook_id for all paths (web, Android,
	// single-file, multi-file). Fixes Bug 1.
	engine._writableCurrentContent.set({
		id: fullBook.id,
		title: fullBook.title,
		artist: fullBook.author_name ?? undefined,
		coverUrl: fullBook.id ? `${getApiBase()}/covers/audiobook/${fullBook.id}` : undefined,
		durationMs: fullBook.duration_ms ?? 0,
	});

	// For multi-file mode, set total duration from book metadata immediately
	if (_isMultiFile) {
		abDurationMs.set(fullBook.duration_ms ?? 0);
	}

	// Determine resume position
	let resumeMs = startMs;
	if (resumeMs === undefined) {
		try {
			const { progress } = await audiobooksApi.getProgress(fullBook.id);
			resumeMs = progress.position_ms;
		} catch {
			resumeMs = 0;
		}
	}
	resumeMs = resumeMs ?? 0;

	// Load bookmarks in parallel
	audiobooksApi.listBookmarks(fullBook.id)
		.then(({ bookmarks }) => abBookmarks.set(bookmarks))
		.catch(() => {});

	if (_isMultiFile) {
		const chapters = fullBook.chapters ?? [];
		_currentChapterIndex = _findChapterIndex(fullBook, resumeMs);
		const chapter = chapters[_currentChapterIndex];
		const chapterOffsetMs = resumeMs - (chapter?.start_ms ?? 0);

		if (isNative()) {
			// On Android, use ExoPlayer
			await _initNativeListeners();
			await _playNativeAudiobook(chapter, true);
			if (chapterOffsetMs > 0) {
				await _seekNative(chapterOffsetMs);
			}
			abPositionMs.set(resumeMs);
			_abShadowEpochMs = Date.now() - resumeMs; // Set shadow epoch for position calculation
			abPlaybackState.set('playing');
			_startNativePositionPolling();
			_startSaveInterval();
		} else {
			// On web/desktop, use HTML5 audio
			_loadChapter(chapter, chapterOffsetMs, true);
		}
	} else {
		const src = `${getApiBase()}/stream/audiobook/${fullBook.id}${_streamToken()}`;

		if (isNative()) {
			// On Android, use ExoPlayer for single-file too
			await _initNativeListeners();
			abDurationMs.set(fullBook.duration_ms ?? 0);
			const dummyChapter: AudiobookChapter = {
				id: fullBook.id,
				audiobook_id: fullBook.id,
				chapter_num: 1,
				title: '',
				start_ms: 0,
				end_ms: fullBook.duration_ms ?? 0,
			};
			await _playNativeAudiobook(dummyChapter, true);
			if (resumeMs > 0) {
				await _seekNative(resumeMs);
			}
			abPositionMs.set(resumeMs);
			_abShadowEpochMs = Date.now() - resumeMs; // Set shadow epoch for position calculation
			abPlaybackState.set('playing');
			_startNativePositionPolling();
			_startSaveInterval();
		} else {
			// On web/desktop, use HTML5 audio
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
			abPlaybackState.set('playing');
		}
	}

	// Claim exclusive slot and notify peers (Bug 2 fix).
	if (get(exclusiveMode) && deviceId) {
		devicesApi.activate(deviceId).catch(() => {});
	}
	sendHeartbeat().catch(() => {});
}

export async function toggleABPlayPause() {
	// Exclusive mode: delegate to active device.
	const activeDev = get(activeDeviceId);
	if (get(exclusiveMode) && activeDev && activeDev !== deviceId && !isCurrentlyOffline()) {
		devicesApi.controlCommand(activeDev, 'toggle').catch(() => {});
		return;
	}
	_stopABShadowTick(); // Stop remote mirror tick — we're acting locally now
	if (isNative()) {
		const state = get(abPlaybackState);
		if (state === 'playing') {
			await _pauseNative();
			abPlaybackState.set('paused');
			_stopSaveInterval();
		} else {
			const book = get(currentAudiobook);
			if (!book) {
				abPlaybackState.set('paused');
				return;
			}
			// Re-assert audiobook mode in case the MediaService was restarted
			try { await invoke('set_audiobook_mode', { isAudiobook: true }); } catch {}
			await _resumeNative();
			abPlaybackState.set('playing');
			_abShadowEpochMs = Date.now() - get(abPositionMs); // Set shadow epoch when resuming
			_startSaveInterval();
			_startNativePositionPolling();
		}
	} else {
		const audio = getAudio();
		const book = get(currentAudiobook);
		if (audio.paused) {
			if (book && !audio.src) {
				playAudiobook(book, get(abPositionMs));
				return;
			}
			engine.switchMode('audiobook');
			audio.play().catch(() => {});
			_startSaveInterval();
		} else {
			audio.pause();
			_stopSaveInterval();
		}
	}
	sendHeartbeat().catch(() => {});
}

export function seekAudiobook(seconds: number) {
	// Exclusive mode: delegate to active device.
	const activeDev = get(activeDeviceId);
	if (get(exclusiveMode) && activeDev && activeDev !== deviceId && !isCurrentlyOffline()) {
		const posMs = Math.round(seconds * 1000);
		devicesApi.controlCommand(activeDev, 'seek', { position_ms: posMs }).catch(() => {});
		abPositionMs.set(posMs); // optimistic update
		return;
	}
	const ms = Math.round(seconds * 1000);
	_lastSeekMs = Date.now();
	_abShadowEpochMs = _lastSeekMs - ms; // Set shadow epoch for smooth position calculation
	if (_isMultiFile || isNative()) {
		seekAudiobookMs(ms);
	} else {
		const audio = getAudio();
		audio.currentTime = seconds;
		abPositionMs.set(ms);
	}
}

export async function seekAudiobookMs(ms: number) {
	// Exclusive mode: delegate to active device.
	const activeDev = get(activeDeviceId);
	if (get(exclusiveMode) && activeDev && activeDev !== deviceId && !isCurrentlyOffline()) {
		devicesApi.controlCommand(activeDev, 'seek', { position_ms: ms }).catch(() => {});
		abPositionMs.set(ms); // optimistic update
		return;
	}
	_lastSeekMs = Date.now(); // Mark that we just seeked
	_abShadowEpochMs = _lastSeekMs - ms; // Set shadow epoch so position is calculated smoothly
	if (isNative()) {
		const book = get(currentAudiobook);
		if (_isMultiFile && book) {
			const chapters = book.chapters ?? [];
			const newIdx = _findChapterIndex(book, ms);
			const chapter = chapters[newIdx];
			const chapterOffsetMs = ms - (chapter?.start_ms ?? 0);
			if (newIdx !== _currentChapterIndex) {
				_currentChapterIndex = newIdx;
				await _playNativeAudiobook(chapter, true);
			}
			await _seekNative(chapterOffsetMs);
		} else {
			await _seekNative(ms);
		}
		abPositionMs.set(ms);
	} else if (_isMultiFile) {
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
	// Exclusive mode: delegate using the mirrored position as the seek target.
	const activeDev = get(activeDeviceId);
	if (get(exclusiveMode) && activeDev && activeDev !== deviceId && !isCurrentlyOffline()) {
		const targetMs = Math.min(get(abPositionMs) + seconds * 1000, get(abDurationMs));
		devicesApi.controlCommand(activeDev, 'seek', { position_ms: targetMs }).catch(() => {});
		abPositionMs.set(targetMs); // optimistic update
		return;
	}
	if (_isMultiFile || isNative()) {
		const newMs = get(abPositionMs) + seconds * 1000;
		seekAudiobookMs(Math.min(newMs, get(abDurationMs)));
	} else {
		const audio = getAudio();
		audio.currentTime = Math.min(audio.currentTime + seconds, audio.duration || 0);
	}
}

export function skipBackward(seconds = 10) {
	// Exclusive mode: delegate using the mirrored position as the seek target.
	const activeDev = get(activeDeviceId);
	if (get(exclusiveMode) && activeDev && activeDev !== deviceId && !isCurrentlyOffline()) {
		const targetMs = Math.max(get(abPositionMs) - seconds * 1000, 0);
		devicesApi.controlCommand(activeDev, 'seek', { position_ms: targetMs }).catch(() => {});
		abPositionMs.set(targetMs); // optimistic update
		return;
	}
	if (_isMultiFile || isNative()) {
		const newMs = get(abPositionMs) - seconds * 1000;
		seekAudiobookMs(Math.max(newMs, 0));
	} else {
		const audio = getAudio();
		audio.currentTime = Math.max(audio.currentTime - seconds, 0);
	}
}

export async function setABSpeed(rate: number) {
	// Exclusive mode: delegate to active device.
	const activeDev = get(activeDeviceId);
	if (get(exclusiveMode) && activeDev && activeDev !== deviceId && !isCurrentlyOffline()) {
		abSpeed.set(rate); // optimistic local update
		devicesApi.controlCommand(activeDev, 'speed', { speed: rate }).catch(() => {});
		return;
	}
	abSpeed.set(rate);
	engine.setSpeed(rate);
	if (!isNative() && _audio) {
		_audio.playbackRate = rate;
	}
}

export function setABVolume(v: number) {
	// Exclusive mode: delegate to active device.
	const activeDev = get(activeDeviceId);
	if (get(exclusiveMode) && activeDev && activeDev !== deviceId && !isCurrentlyOffline()) {
		devicesApi.controlCommand(activeDev, 'volume', { volume: v }).catch(() => {});
		return;
	}
	abVolume.set(v);
	engine.setVolume(v);
	if (!isNative() && _audio) {
		_audio.volume = v;
	}
	sendHeartbeat().catch(() => {});
}

export async function jumpToChapter(chapter: AudiobookChapter) {
	// Exclusive mode: delegate as a seek to the chapter's start position.
	const activeDev = get(activeDeviceId);
	if (get(exclusiveMode) && activeDev && activeDev !== deviceId && !isCurrentlyOffline()) {
		devicesApi.controlCommand(activeDev, 'seek', { position_ms: chapter.start_ms }).catch(() => {});
		abPositionMs.set(chapter.start_ms); // optimistic update
		return;
	}
	if (_isMultiFile && isNative()) {
		const book = get(currentAudiobook);
		const chapters = book?.chapters ?? [];
		const idx = chapters.findIndex((ch) => ch.id === chapter.id);
		if (idx >= 0) {
			_currentChapterIndex = idx;
			_lastSeekMs = Date.now();
			_abShadowEpochMs = _lastSeekMs - chapter.start_ms;
			await _playNativeAudiobook(chapter, true);
			abPositionMs.set(chapter.start_ms);
		}
	} else if (_isMultiFile) {
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
	// Guard: skip native calls and persist when no audiobook was ever loaded.
	// The layout $effect calls this on mount (activePlayer defaults to 'music'),
	// and the JNI/persist calls are wasteful when there's nothing to pause.
	if (!get(currentAudiobook)) return;

	_audio?.pause();
	_stopSaveInterval();
	_stopNativePositionPolling();
	_stopABShadowTick();
	_persistProgress();
	if (isNative()) {
		engine.pause();
	}
	abPlaybackState.set('paused');
}

export async function closeAudiobook() {
	_stopNativePositionPolling();
	await _stopNativePlayback();
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

/**
 * Mirror the active device's audiobook state on this idle device without
 * starting audio. Called by deviceSession when a 'state' SSE event arrives
 * from the active device while it is playing an audiobook.
 */
export function syncABVisibleState(audiobookId: string, posMs: number, playing: boolean, vol?: number) {
	if (vol !== undefined) abVolume.set(vol);
	abPositionMs.set(posMs);
	abPlaybackState.set(playing ? 'playing' : 'paused');
	engine.switchMode('audiobook');

	// Shadow tick: advance position smoothly between heartbeats.
	if (playing) {
		_startABShadowTick(posMs, get(abSpeed));
	} else {
		_stopABShadowTick();
	}

	const existing = get(currentAudiobook);
	if (!existing || existing.id !== audiobookId) {
		audiobooksApi.get(audiobookId)
			.then(({ audiobook }) => {
				currentAudiobook.set(audiobook);
				abDurationMs.set(audiobook.duration_ms ?? 0);
			})
			.catch(() => {});
	}
}

// ── Persist on page unload ────────────────────────────────────────────────────

if (browser) {
	window.addEventListener('beforeunload', () => {
		_persistProgress();
	});
}

// ── Device transfer ───────────────────────────────────────────────────────────

/**
 * Transfer audiobook playback to another device, or pull it to this device.
 *
 * Pull (targetId === this device):
 *   Reads the remote device's audiobook state and starts local playback.
 *
 * Push (targetId !== this device):
 *   Pauses local audio, activates the target, and sends a heartbeat so the
 *   target can pick up the audiobook state. The target will need to press play
 *   (full auto-play push requires backend audiobook play_command support).
 */
export async function transferAudiobookPlayback(targetId: string) {
	if (targetId === deviceId) {
		// Pull playback to this device from whoever is currently active.
		const { activeDevices: devStore } = await import('$lib/stores/player/deviceSession');
		const devices = get(devStore);
		const active = devices.find((d) => d.is_active && d.id !== deviceId);

		activeDeviceId.set(deviceId);
		await devicesApi.activate(deviceId).catch(() => {});

		if (active?.state?.is_audiobook && active.state.audiobook_id) {
			try {
				const { audiobook } = await audiobooksApi.get(active.state.audiobook_id);
				await playAudiobook(audiobook, active.state.position_ms ?? 0);
			} catch (e) {
				console.error('[audiobook] transferAudiobookPlayback pull failed:', e);
			}
		}
		return;
	}

	// Push to another device: pause local, activate target, send heartbeat.
	if (get(abPlaybackState) === 'playing') {
		pauseAudiobook();
	}

	await devicesApi.activate(targetId).catch(() => {});
	// Immediately send a heartbeat so the server has the audiobook state
	// before the target device refreshes.
	await sendHeartbeat();
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

// Device session wiring removed — SSE dispatch now routes through the engine's
// content provider pattern. See engine.registerProvider('audiobook', ...) below.

// ─── Register audiobook content provider with the unified engine ─────────────

const abContentProvider: ContentProvider & {
	onSkipForward?(secs: number): void;
	onSkipBackward?(secs: number): void;
	onSpeedCycle?(): void;
	onJumpToChapterStart?(): void;
	onControlCommand?(action: string, payload: any): void;
	onRemoteSync?(state: any): void;
} = {
	onTrackEnd() {
		// End of chapter / book — handled by HTMLAudioElement 'ended' event
		// and native position polling chapter detection for now.
	},
	onPositionUpdate(ms: number) {
		abPositionMs.set(ms);
	},
	onModeActivated() {
		// Nothing extra — audiobook playback is started via playAudiobook().
	},
	onModeDeactivated() {
		pauseAudiobook();
	},

	// Extended callbacks for engine's native event routing
	onSkipForward(secs: number) { skipForward(secs); },
	onSkipBackward(secs: number) { skipBackward(secs); },
	onSpeedCycle() { _cycleSpeed(); },
	onJumpToChapterStart() { _jumpToChapterStart(); },
	onControlCommand(action: string, payload: any) {
		switch (action) {
			case 'toggle': toggleABPlayPause(); break;
			case 'seek':
				if (payload?.position_ms !== undefined) seekAudiobookMs(payload.position_ms);
				break;
			case 'volume':
				if (payload?.volume !== undefined) setABVolume(payload.volume);
				break;
			case 'speed':
				if (payload?.speed !== undefined) setABSpeed(payload.speed);
				break;
			case 'next': case 'skip_forward': skipForward(30); break;
			case 'previous': case 'skip_backward': skipBackward(10); break;
		}
	},
	onRemoteSync(state: any) {
		// Mirror audiobook metadata when receiving remote state.
		if (state.audiobook_id) {
			syncABVisibleState(
				state.audiobook_id,
				state.position_ms,
				state.playing,
				state.volume
			);
		}
	},
};

engine.registerProvider('audiobook', abContentProvider);

// Reverse bridges: audiobook stores → engine stores. The browser/web audiobook
// path manages a raw HTMLAudioElement and only updates abPlaybackState /
// abPositionMs / abDurationMs. Without these bridges, the engine's stores stay
// stale and heartbeats report playing: false / position_ms: 0 for audiobooks.
abPlaybackState.subscribe((state) => {
	if (get(engine.mode) === 'audiobook') {
		engine._writablePlaybackState.set(state as engine.EnginePlaybackState);
	}
});
abPositionMs.subscribe((ms) => {
	if (get(engine.mode) === 'audiobook') {
		engine._writablePositionMs.set(ms);
	}
});
abDurationMs.subscribe((dur) => {
	if (get(engine.mode) === 'audiobook') {
		engine._writableDurationMs.set(dur);
	}
});
