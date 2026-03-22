/**
 * Native (Tauri/Android) bridge code for audiobook playback.
 *
 * Extracted from audiobookPlayer.ts for modularity.
 */

import { get } from 'svelte/store';
import { invoke } from '@tauri-apps/api/core';
import { listen } from '@tauri-apps/api/event';
import { getApiBase } from '$lib/api/base';
import { authStore } from '$lib/stores/auth';
import { isNative } from '$lib/utils/platform';
import type { Audiobook, AudiobookChapter } from '$lib/types';
import { currentAudiobook, abPositionMs, abSpeed } from './audiobookPlayer';
import * as engine from './engine';
import { TIMINGS } from '$lib/constants';
import { isMultiFileBook, findChapterIndex } from './audiobookChapters';

// ── Internal state ────────────────────────────────────────────────────────────

let _nativePositionInterval: ReturnType<typeof setInterval> | null = null;
let _nativeInitialized = false;

/** Tracks when we last seeked to avoid polling overwriting seek position */
export let _lastSeekMs = 0;
export function setLastSeekMs(v: number) { _lastSeekMs = v; }

/**
 * Shadow epoch for calculating position without relying on polling alone.
 * When playback starts or seeks, we record when the position would be 0
 * Then calculate current position as Date.now() - _abShadowEpochMs
 */
export let _abShadowEpochMs = 0;
export function setAbShadowEpochMs(v: number) { _abShadowEpochMs = v; }

/** Multi-file chapter tracking */
export let _isMultiFile = false;
export function setIsMultiFile(v: boolean) { _isMultiFile = v; }
export let _currentChapterIndex = 0;
export function setCurrentChapterIndex(v: number) { _currentChapterIndex = v; }

// ── Helpers ──────────────────────────────────────────────────────────────────

export function streamToken(): string {
	const token = get(authStore).token;
	return token ? `?token=${encodeURIComponent(token)}` : '';
}

export function chapterUrl(chapter: AudiobookChapter): string {
	return `${getApiBase()}/stream/audiobook/chapter/${chapter.id}${streamToken()}`;
}

// ── Native listeners ─────────────────────────────────────────────────────────

export async function initNativeListeners(
	skipBackwardFn: (s: number) => void,
	skipForwardFn: (s: number) => void,
	cycleSpeedFn: () => void,
	jumpToChapterStartFn: () => void,
) {
	if (_nativeInitialized || !isNative()) return;
	_nativeInitialized = true;

	listen<void>('native-ab-skip-back-15', () => skipBackwardFn(15));
	listen<void>('native-ab-skip-forward-15', () => skipForwardFn(15));
	listen<void>('native-ab-speed-cycle', () => cycleSpeedFn());
	listen<void>('native-ab-chapter-start', () => jumpToChapterStartFn());
}

// ── Native playback helpers ──────────────────────────────────────────────────

async function _resolveNativeChapterUrl(chapter: AudiobookChapter): Promise<string> {
	try {
		const path = await invoke<string | null>('get_offline_file_path', { trackId: chapter.id });
		if (path) return `file://${path}`;
	} catch { /* fall through */ }
	if (_isMultiFile) {
		return chapterUrl(chapter);
	}
	return `${getApiBase()}/stream/audiobook/${chapter.id}${streamToken()}`;
}

export async function playNativeAudiobook(chapter: AudiobookChapter, autoPlay: boolean) {
	if (!isNative()) return;
	const url = await _resolveNativeChapterUrl(chapter);
	const book = get(currentAudiobook);
	const title = book ? `${book.title}` : 'Audiobook';
	const artist = book?.author_name || 'Unknown';

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

export function pauseNative() {
	if (!isNative()) return;
	engine.pause();
}

export function resumeNative() {
	if (!isNative()) return;
	engine.resume();
}

export async function seekNative(positionMs: number) {
	if (!isNative()) return;
	engine.seek(positionMs);
}

export function stopNativePlayback() {
	if (!isNative()) return;
	engine.pause();
}

export function startNativePositionPolling(
	seekAudiobookMsFn: (ms: number) => void,
) {
	if (!isNative()) return;
	stopNativePositionPolling();
	_nativePositionInterval = setInterval(async () => {
		try {
			const now = Date.now();

			let pos: number;
			if (_abShadowEpochMs > 0 && now - _lastSeekMs >= 500) {
				pos = now - _abShadowEpochMs;
			} else {
				let rawPos = await invoke<number>('get_position_music');
				if (_isMultiFile) {
					const book = get(currentAudiobook);
					const chapterStart = book?.chapters?.[_currentChapterIndex]?.start_ms ?? 0;
					rawPos += chapterStart;
				}
				pos = rawPos;
			}

			abPositionMs.set(pos);

			const book = get(currentAudiobook);
			if (book && _isMultiFile) {
				const newIdx = findChapterIndex(book, pos);
				if (newIdx !== _currentChapterIndex) {
					const chapters = book.chapters ?? [];
					const chapter = chapters[newIdx];
					if (chapter) {
						_currentChapterIndex = newIdx;
						const chapterOffsetMs = pos - (chapter?.start_ms ?? 0);
						await playNativeAudiobook(chapter, true);
						await seekNative(chapterOffsetMs);
					}
				}
			}
		} catch {
			// Ignore errors during polling
		}
	}, TIMINGS.POSITION_TICK);
}

export function stopNativePositionPolling() {
	if (_nativePositionInterval !== null) {
		clearInterval(_nativePositionInterval);
		_nativePositionInterval = null;
	}
}
