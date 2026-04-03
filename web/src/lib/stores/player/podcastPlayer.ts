/**
 * Podcast episode player store — separate from music and audiobook players.
 *
 * Features:
 *  - Play podcast episodes via HTMLAudioElement (browser/Tauri desktop)
 *  - Per-episode position saved to backend every 10s and on pause/end
 *  - Skip ±15/30 s controls
 *  - Mark as played
 */

import { writable, get } from 'svelte/store';
import { getApiBase } from '$lib/api/base';
import { podcasts as podcastsApi } from '$lib/api/podcasts';
import { authStore } from '$lib/stores/auth';
import type { Podcast, PodcastEpisode } from '$lib/types';
import { nativePlatform } from '$lib/utils/platform';
import * as engine from './engine';
import type { ContentProvider } from './engine';

export type PodcastPlaybackState = 'idle' | 'loading' | 'playing' | 'paused';

export const currentEpisode = writable<PodcastEpisode | null>(null);
export const currentPodcast = writable<Podcast | null>(null);
export const podcastPlaybackState = writable<PodcastPlaybackState>('idle');
export const podcastPositionMs = writable(0);
export const podcastDurationMs = writable(0);
export const podcastBufferedPct = writable(0);

// ── Internal ──────────────────────────────────────────────────────────────────

let _audio: HTMLAudioElement | null = null;
let _saveInterval: ReturnType<typeof setInterval> | null = null;

function _streamToken(): string {
	const token = get(authStore).token;
	return token ? `?token=${encodeURIComponent(token)}` : '';
}

function _episodeStreamUrl(episodeId: string): string {
	return `${getApiBase()}/stream/podcast/${episodeId}${_streamToken()}`;
}

function _getAudio(): HTMLAudioElement {
	if (!_audio) {
		_audio = new Audio();
		_audio.preload = 'metadata';
		_audio.volume = get(engine.engineVolume);

		_audio.addEventListener('loadedmetadata', () => {
			if (_audio) {
				podcastDurationMs.set(Math.round(_audio.duration * 1000));
			}
		});

		_audio.addEventListener('timeupdate', () => {
			if (_audio) {
				podcastPositionMs.set(Math.round(_audio.currentTime * 1000));
				if (_audio.buffered.length > 0) {
					const end = _audio.buffered.end(_audio.buffered.length - 1);
					podcastBufferedPct.set(_audio.duration > 0 ? (end / _audio.duration) * 100 : 0);
				}
			}
		});

		_audio.addEventListener('ended', () => {
			podcastPlaybackState.set('paused');
			_stopSaveInterval();
			_saveProgress(true);
		});

		_audio.addEventListener('playing', () => {
			podcastPlaybackState.set('playing');
		});

		_audio.addEventListener('pause', () => {
			if (get(podcastPlaybackState) === 'playing') {
				podcastPlaybackState.set('paused');
				_saveProgress(false);
			}
		});

		_audio.addEventListener('error', () => {
			podcastPlaybackState.set('idle');
		});
	}
	return _audio;
}

function _startSaveInterval() {
	_stopSaveInterval();
	_saveInterval = setInterval(() => _saveProgress(false), 10_000);
}

function _stopSaveInterval() {
	if (_saveInterval !== null) {
		clearInterval(_saveInterval);
		_saveInterval = null;
	}
}

function _saveProgress(completed: boolean) {
	const ep = get(currentEpisode);
	if (!ep) return;
	podcastsApi.updateProgress(ep.id, get(podcastPositionMs), completed).catch(() => { });
}

// ── Public API ────────────────────────────────────────────────────────────────

export async function playEpisode(
	episode: PodcastEpisode,
	podcast: Podcast,
	startMs?: number,
) {
	// If same episode and already loaded, just toggle
	const current = get(currentEpisode);
	if (current?.id === episode.id) {
		togglePodcastPlayPause();
		return;
	}

	_stopSaveInterval();

	// Pause music / audiobook before taking over audio focus.
	engine.switchMode('podcast');

	currentEpisode.set(episode);
	currentPodcast.set(podcast);
	podcastPlaybackState.set('loading');
	podcastDurationMs.set(episode.duration_ms ?? 0);

	engine._writableCurrentContent.set({
		id: episode.id,
		title: episode.title,
		artist: podcast.author ?? undefined,
		coverUrl: `${getApiBase()}/covers/podcast/${episode.podcast_id}`,
		durationMs: episode.duration_ms ?? 0,
	});

	// Resolve start position: explicit > saved backend progress > 0
	let resolvedStartMs = startMs;
	if (resolvedStartMs === undefined) {
		try {
			const { progress } = await podcastsApi.getProgress(episode.id);
			resolvedStartMs = progress.completed ? 0 : (progress.position_ms ?? 0);
		} catch {
			resolvedStartMs = 0;
		}
	}
	resolvedStartMs = resolvedStartMs ?? 0;
	podcastPositionMs.set(resolvedStartMs);

	const isAndroid = nativePlatform() === 'android';

	if (isAndroid) {
		// On Android use the unified engine (ExoPlayer under the hood)
		const url = _episodeStreamUrl(episode.id);
		try {
			await engine.play(url, {
				id: episode.id,
				title: episode.title,
				artist: get(currentPodcast)?.author ?? undefined,
				coverUrl: `${getApiBase()}/covers/podcast/${episode.podcast_id}`,
				durationMs: episode.duration_ms ?? 0,
			}, {
				isAudiobook: false,
				nativeUrl: url,
				startMs: resolvedStartMs,
			});
			podcastPlaybackState.set('playing');
			_startSaveInterval();
		} catch (e) {
			console.error('Failed to play podcast episode (native)', e);
			podcastPlaybackState.set('idle');
		}
	} else {
		// Browser + Tauri desktop: HTMLAudioElement
		const audio = _getAudio();
		audio.src = _episodeStreamUrl(episode.id);
		audio.currentTime = resolvedStartMs / 1000;
		try {
			await audio.play();
			podcastPlaybackState.set('playing');
			_startSaveInterval();
		} catch (e) {
			console.error('Failed to play podcast episode', e);
			podcastPlaybackState.set('idle');
		}
	}
}

export async function togglePodcastPlayPause() {
	const state = get(podcastPlaybackState);

	if (state === 'playing') {
		if (_audio) {
			_audio.pause();
		} else {
			engine.pause();
		}
		podcastPlaybackState.set('paused');
		_stopSaveInterval();
		_saveProgress(false);
	} else if (state === 'paused' || state === 'loading') {
		if (_audio) {
			try {
				await _audio.play();
				podcastPlaybackState.set('playing');
				_startSaveInterval();
			} catch (e) {
				console.error('Podcast resume failed', e);
			}
		} else {
			engine.resume();
			podcastPlaybackState.set('playing');
			_startSaveInterval();
		}
	}
}

export function seekPodcastMs(ms: number) {
	if (_audio) {
		_audio.currentTime = ms / 1000;
	} else {
		engine.seek(ms); // Android: routes to ExoPlayer
	}
	podcastPositionMs.set(ms);
}

export function skipForwardPodcast(seconds = 30) {
	if (_audio) {
		const t = Math.min(_audio.currentTime + seconds, _audio.duration || 0);
		_audio.currentTime = t;
		podcastPositionMs.set(Math.round(t * 1000));
	} else {
		const t = Math.min(get(podcastPositionMs) + seconds * 1000, get(podcastDurationMs));
		engine.seek(t);
		podcastPositionMs.set(t);
	}
}

export function skipBackwardPodcast(seconds = 15) {
	if (_audio) {
		const t = Math.max(_audio.currentTime - seconds, 0);
		_audio.currentTime = t;
		podcastPositionMs.set(Math.round(t * 1000));
	} else {
		const t = Math.max(get(podcastPositionMs) - seconds * 1000, 0);
		engine.seek(t);
		podcastPositionMs.set(t);
	}
}

export async function markEpisodePlayed(completed: boolean) {
	const ep = get(currentEpisode);
	if (!ep) return;
	await podcastsApi.updateProgress(ep.id, get(podcastPositionMs), completed);
}

export function closePodcast() {
	_stopSaveInterval();
	_saveProgress(false);
	if (_audio) {
		_audio.pause();
		_audio.src = '';
	} else {
		engine.pause();
	}
	currentEpisode.set(null);
	currentPodcast.set(null);
	podcastPlaybackState.set('idle');
	podcastPositionMs.set(0);
	podcastDurationMs.set(0);
	podcastBufferedPct.set(0);
	// Return engine mode to music so the music bar is responsive again.
	engine._writableMode.set('music');
	engine._writableCurrentContent.set(null);
}

// ── Sleep timer ───────────────────────────────────────────────────────────────

export const podcastSleepTimerMins = writable(0);
let _sleepTimeout: ReturnType<typeof setTimeout> | null = null;

export const PODCAST_SLEEP_PRESETS = [5, 10, 15, 20, 30, 45, 60];

export function setPodcastSleepTimer(minutes: number) {
	if (_sleepTimeout !== null) {
		clearTimeout(_sleepTimeout);
		_sleepTimeout = null;
	}
	podcastSleepTimerMins.set(minutes);
	if (minutes > 0) {
		_sleepTimeout = setTimeout(() => {
			if (_audio) {
				_audio.pause();
				podcastPlaybackState.set('paused');
				_stopSaveInterval();
				_saveProgress(false);
			}
			podcastSleepTimerMins.set(0);
		}, minutes * 60_000);
	}
}

// ── Engine content provider ───────────────────────────────────────────────────

const podcastContentProvider: ContentProvider = {
	onTrackEnd() {
		// Handled by the HTMLAudioElement 'ended' event above.
	},
	onPositionUpdate(ms: number) {
		podcastPositionMs.set(ms);
	},
	onModeActivated() {
		// Podcast starts via playEpisode() — nothing to do here.
	},
	onModeDeactivated() {
		// Another mode (music / audiobook) is taking over — pause the podcast.
		_stopSaveInterval();
		if (_audio) {
			_audio.pause();
		} else {
			engine.pause();
		}
		const current = get(podcastPlaybackState);
		if (current === 'playing' || current === 'loading') {
			podcastPlaybackState.set('paused');
			_saveProgress(false);
		}
	},
	onControlCommand(action: string, payload) {
		switch (action) {
			case 'toggle': togglePodcastPlayPause(); break;
			case 'seek':
				if (payload?.position_ms !== undefined) seekPodcastMs(payload.position_ms as number);
				break;
			case 'skip_forward': skipForwardPodcast(30); break;
			case 'skip_backward': skipBackwardPodcast(15); break;
			case 'volume':
				if (payload?.volume !== undefined) engine.setVolume(payload.volume as number);
				break;
		}
	},
};

engine.registerProvider('podcast', podcastContentProvider);

// Keep the private HTMLAudioElement volume in sync with the engine volume store.
engine.engineVolume.subscribe((gain) => {
	if (_audio) _audio.volume = gain;
});

// Bridge podcast stores → engine stores so that heartbeats and any engine
// consumers reflect podcast playback state while it is the active mode.
podcastPlaybackState.subscribe((state) => {
	if (get(engine.mode) === 'podcast') {
		engine._writablePlaybackState.set(state as engine.EnginePlaybackState);
	}
});
podcastPositionMs.subscribe((ms) => {
	if (get(engine.mode) === 'podcast') {
		engine._writablePositionMs.set(ms);
	}
});
podcastDurationMs.subscribe((dur) => {
	if (get(engine.mode) === 'podcast') {
		engine._writableDurationMs.set(dur);
	}
});
podcastBufferedPct.subscribe((pct) => {
	if (get(engine.mode) === 'podcast') {
		engine._writableBufferedPct.set(pct);
	}
});

export function formatPodcastMs(ms: number): string {
	const totalSecs = Math.floor(ms / 1000);
	const h = Math.floor(totalSecs / 3600);
	const m = Math.floor((totalSecs % 3600) / 60);
	const s = totalSecs % 60;
	if (h > 0) return `${h}:${m.toString().padStart(2, '0')}:${s.toString().padStart(2, '0')}`;
	return `${m}:${s.toString().padStart(2, '0')}`;
}
