/**
 * Player barrel — re-exports music-specific stores/functions from musicPlayer.ts
 * and wires up cross-cutting concerns:
 *  - Persistence (localStorage save/restore)
 *  - Media Session API (hardware media keys, OS overlays)
 *  - Visibility change handler (AudioContext resume, device refresh)
 *  - Discord Rich Presence (desktop only)
 *  - System tray integration (desktop only)
 *  - Android native event bridge
 */

// ── Re-exports ───────────────────────────────────────────────────────────────
// Every public API from the music player is re-exported so consumers can keep
// importing from '$lib/stores/player'.

export {
	// Stores
	currentTrack, playbackState, positionMs, durationMs, volume,
	queue, queueIndex, bufferedPct, repeatMode, shuffle, shuffleOrder,
	userQueue, smartShuffleEnabled, queueModalOpen, autoplayEnabled,
	radioMode, discordEnabled, replayGainEnabled,
	// Derived
	formattedFormat, formattedPosition, formattedDuration,
	// Constants
	isAndroidNative,
	// Functions
	playTrack, pauseLocal, togglePlayPause, seek, setVolume,
	playNext, addToQueue, removeFromUserQueue,
	next, previous, toggleRepeat, toggleShuffle, loadQueue,
	transferPlayback, receivePlayCommand,
	stopShadowTick, syncVisibleState,
	startRadio, stopRadio,
} from './musicPlayer';

export {
	MUSIC_SLEEP_PRESETS,
	musicSleepPreset, musicSleepMsRemaining, musicSleepFading,
	setMusicSleepTimer, clearMusicSleepTimer,
} from './musicSleepTimer';

// ── Imports for orchestration ────────────────────────────────────────────────

import { get } from 'svelte/store';
import type { Track, PlaybackState } from '$lib/types';
import { audioEngine } from '$lib/audio/engine';
import { getApiBase } from '$lib/api/base';
import { library as libraryApi } from '$lib/api/library';
import { isTauri, nativePlatform } from '$lib/utils/platform';
import { invoke } from '@tauri-apps/api/core';
import { listen } from '@tauri-apps/api/event';
import { favorites } from '$lib/stores/library/favorites';
import { TIMINGS, STORAGE_KEYS } from '$lib/constants';
import { refreshDevices as refreshDevicesFromSession } from './deviceSession';
import { activePlayer } from './engine';
import {
	toggleABPlayPause, pauseAudiobook,
	skipForward, skipBackward, seekAudiobookMs,
	currentAudiobook, abPlaybackState, abPositionMs, abDurationMs,
	restoreAudiobookState,
} from './audiobookPlayer';
import {
	currentEpisode,
	currentPodcast,
	podcastPlaybackState,
	podcastPositionMs,
	podcastDurationMs,
	restorePodcastState,
	togglePodcastPlayPause,
	skipForwardPodcast,
	skipBackwardPodcast,
	seekPodcastMs,
} from './podcastPlayer';
import { authStore } from '$lib/stores/auth';
import { audiobooks as audiobooksApi } from '$lib/api/audiobooks';
import { podcasts as podcastsApi } from '$lib/api/podcasts';
import * as engine from './engine';
import {
	openNativePictureInPicture,
	closeNativePictureInPicture,
	syncNativePictureInPictureBridge,
	teardownNativePictureInPictureBridge,
} from './nativePictureInPicture';

import {
	currentTrack, playbackState, positionMs, durationMs, volume,
	queue, queueIndex, repeatMode, shuffle, shuffleOrder,
	autoplayEnabled, discordEnabled, replayGainEnabled, smartShuffleEnabled,
	isAndroidNative,
	togglePlayPause, seek, next, previous, toggleShuffle,
} from './musicPlayer';

// ── Persistence ──────────────────────────────────────────────────────────────

const STORAGE_KEY = STORAGE_KEYS.PLAYER_STATE;
const POSITION_SAVE_INTERVAL_MS = TIMINGS.POSITION_SAVE_INTERVAL;
let lastWriteTime = 0;
let saveTimeout: ReturnType<typeof setTimeout> | null = null;
let saveEnabled = false;

function writeState() {
	try {
		const st = {
			trackId: get(currentTrack)?.id ?? null,
			pos: Math.floor(get(positionMs) / 1000),
			podcastEpisodeId: get(currentEpisode)?.id ?? null,
			podcastId: get(currentPodcast)?.id ?? null,
			podcastPos: Math.floor(get(podcastPositionMs) / 1000),
			queue: get(queue),
			queueIndex: get(queueIndex),
			volume: get(volume),
			repeat: get(repeatMode),
			shuffle: get(shuffle),
			shuffleOrder: get(shuffleOrder),
			autoplay: get(autoplayEnabled),
			discord: get(discordEnabled),
			replayGain: get(replayGainEnabled),
			smartShuffle: get(smartShuffleEnabled),
			activePlayer: get(activePlayer),
			audiobookId: get(currentAudiobook)?.id ?? null,
		};
		localStorage.setItem(STORAGE_KEY, JSON.stringify(st));
	} catch {
		// ignore storage errors
	}
}

function schedulePositionSave() {
	if (!saveEnabled) return;
	const now = Date.now();
	const remaining = POSITION_SAVE_INTERVAL_MS - (now - lastWriteTime);
	if (remaining <= 0) {
		if (saveTimeout) { clearTimeout(saveTimeout); saveTimeout = null; }
		writeState();
		lastWriteTime = now;
	} else if (!saveTimeout) {
		saveTimeout = setTimeout(() => {
			writeState();
			lastWriteTime = Date.now();
			saveTimeout = null;
		}, remaining);
	}
}

function scheduleStateSave() {
	if (!saveEnabled) return;
	if (saveTimeout) clearTimeout(saveTimeout);
	saveTimeout = setTimeout(() => {
		writeState();
		lastWriteTime = Date.now();
		saveTimeout = null;
	}, TIMINGS.STATE_SAVE_DEBOUNCE);
}

positionMs.subscribe(() => schedulePositionSave());
playbackState.subscribe(() => scheduleStateSave());
currentTrack.subscribe(() => scheduleStateSave());
currentAudiobook.subscribe(() => scheduleStateSave());
currentEpisode.subscribe(() => scheduleStateSave());
currentPodcast.subscribe(() => scheduleStateSave());
activePlayer.subscribe(() => scheduleStateSave());
queueIndex.subscribe(() => scheduleStateSave());
volume.subscribe(() => scheduleStateSave());
repeatMode.subscribe(() => scheduleStateSave());
shuffle.subscribe(() => scheduleStateSave());
smartShuffleEnabled.subscribe(() => scheduleStateSave());
autoplayEnabled.subscribe(() => scheduleStateSave());
discordEnabled.subscribe(() => scheduleStateSave());
replayGainEnabled.subscribe(() => {
	scheduleStateSave();
	const track = get(currentTrack);
	if (track) {
		const rgDb = get(replayGainEnabled) ? (track.replay_gain_track ?? 0) : 0;
		audioEngine.setReplayGainDb(rgDb);
	}
});
podcastPositionMs.subscribe(() => schedulePositionSave());

// ── Media Session API ────────────────────────────────────────────────────────

function mediaSessionSupported(): boolean {
	if (isAndroidNative) return false;
	return typeof navigator !== 'undefined' && 'mediaSession' in navigator;
}

function syncMediaMetadata(track: Track | null) {
	if (!mediaSessionSupported() || !track) return;
	const artwork: MediaImage[] = [];
	if (track.album_id) {
		const base = typeof location !== 'undefined' ? location.origin : '';
		artwork.push({ src: `${base}${getApiBase()}/covers/${track.album_id}`, sizes: '512x512', type: 'image/jpeg' });
	}
	navigator.mediaSession.metadata = new MediaMetadata({
		title: track.title,
		artist: track.artist_name ?? '',
		album: track.album_name ?? '',
		artwork,
	});
}

function syncMediaSessionPlaybackState(state: PlaybackState) {
	if (!mediaSessionSupported()) return;
	if (state === 'loading') return;
	navigator.mediaSession.playbackState = state === 'playing' ? 'playing' : 'paused';
}

function syncPositionState(posMs: number, durMs: number) {
	if (!mediaSessionSupported()) return;
	const duration = durMs / 1000;
	const position = Math.min(posMs / 1000, Math.max(0, duration));
	if (duration > 0) {
		try {
			navigator.mediaSession.setPositionState({ duration, position, playbackRate: 1 });
		} catch {
			// Some browsers throw if values are out of range.
		}
	}
}

function syncAudiobookMediaMetadata() {
	if (!mediaSessionSupported()) return;
	const book = get(currentAudiobook);
	if (!book) return;
	const base = typeof location !== 'undefined' ? location.origin : '';
	navigator.mediaSession.metadata = new MediaMetadata({
		title: book.title,
		artist: book.author_name ?? '',
		album: book.series ?? '',
		artwork: [{ src: `${base}${getApiBase()}/covers/audiobook/${book.id}`, sizes: '512x512', type: 'image/jpeg' }],
	});
}

function syncPodcastMediaMetadata() {
	if (!mediaSessionSupported()) return;
	const episode = get(currentEpisode);
	if (!episode) return;
	const podcast = get(currentPodcast);
	const base = typeof location !== 'undefined' ? location.origin : '';
	navigator.mediaSession.metadata = new MediaMetadata({
		title: episode.title,
		artist: podcast?.author ?? '',
		album: podcast?.title ?? '',
		artwork: [{ src: `${base}${getApiBase()}/covers/podcast/${episode.podcast_id}`, sizes: '512x512', type: 'image/jpeg' }],
	});
}

if (mediaSessionSupported()) {
	navigator.mediaSession.setActionHandler('play', () => {
		const mode = get(activePlayer);
		if (mode === 'audiobook') toggleABPlayPause();
		else if (mode === 'podcast') togglePodcastPlayPause();
		else togglePlayPause();
	});
	navigator.mediaSession.setActionHandler('pause', () => {
		const mode = get(activePlayer);
		if (mode === 'audiobook') toggleABPlayPause();
		else if (mode === 'podcast') togglePodcastPlayPause();
		else togglePlayPause();
	});
	navigator.mediaSession.setActionHandler('stop', () => {
		const mode = get(activePlayer);
		if (mode === 'audiobook') {
			pauseAudiobook();
		} else if (mode === 'podcast') {
			if (get(podcastPlaybackState) === 'playing') {
				togglePodcastPlayPause();
			}
		} else {
			audioEngine.stop();
			positionMs.set(0);
			playbackState.set('paused');
		}
	});
	navigator.mediaSession.setActionHandler('previoustrack', () => {
		const mode = get(activePlayer);
		if (mode === 'audiobook') skipBackward(10);
		else if (mode === 'podcast') skipBackwardPodcast(15);
		else previous();
	});
	navigator.mediaSession.setActionHandler('nexttrack', () => {
		const mode = get(activePlayer);
		if (mode === 'audiobook') skipForward(30);
		else if (mode === 'podcast') skipForwardPodcast(30);
		else next();
	});
	navigator.mediaSession.setActionHandler('seekto', (details) => {
		if (details.seekTime === undefined) return;
		const mode = get(activePlayer);
		if (mode === 'audiobook') seekAudiobookMs(details.seekTime * 1000);
		else if (mode === 'podcast') seekPodcastMs(details.seekTime * 1000);
		else seek(details.seekTime);
	});
	navigator.mediaSession.setActionHandler('seekbackward', (details) => {
		const mode = get(activePlayer);
		if (mode === 'audiobook') skipBackward(details.seekOffset ?? 10);
		else if (mode === 'podcast') skipBackwardPodcast(details.seekOffset ?? 10);
		else seek(Math.max(0, get(positionMs) / 1000 - (details.seekOffset ?? 10)));
	});
	navigator.mediaSession.setActionHandler('seekforward', (details) => {
		const mode = get(activePlayer);
		if (mode === 'audiobook') skipForward(details.seekOffset ?? 30);
		else if (mode === 'podcast') skipForwardPodcast(details.seekOffset ?? 30);
		else seek(Math.min(get(durationMs) / 1000, get(positionMs) / 1000 + (details.seekOffset ?? 10)));
	});
	try {
		(
			navigator.mediaSession.setActionHandler as unknown as
			(action: string, handler: MediaSessionActionHandler | null) => void
		)('enterpictureinpicture', () => {
			openNativePictureInPicture().catch((err) => {
				console.warn('[PiP] Failed to open native Picture-in-Picture:', err);
			});
		});
		(
			navigator.mediaSession.setActionHandler as unknown as
			(action: string, handler: MediaSessionActionHandler | null) => void
		)('leavepictureinpicture', () => {
			closeNativePictureInPicture().catch((err) => {
				console.warn('[PiP] Failed to close native Picture-in-Picture:', err);
			});
		});
	} catch {
		// Browser/media session impl does not support PiP actions.
	}
}

function syncNativePiPBridgeFromState() {
	const mode = get(activePlayer);
	const st = mode === 'audiobook'
		? get(abPlaybackState)
		: mode === 'podcast'
			? get(podcastPlaybackState)
			: get(playbackState);
	if (st === 'idle') {
		teardownNativePictureInPictureBridge().catch(() => { });
		return;
	}
	syncNativePictureInPictureBridge(st === 'playing').catch(() => { });
}

currentTrack.subscribe((track) => {
	if (get(activePlayer) === 'music') syncMediaMetadata(track);
});
playbackState.subscribe((state) => {
	if (get(activePlayer) === 'music') syncMediaSessionPlaybackState(state);
	syncNativePiPBridgeFromState();
});
currentAudiobook.subscribe(() => {
	if (get(activePlayer) === 'audiobook') syncAudiobookMediaMetadata();
	syncNativePiPBridgeFromState();
});
currentEpisode.subscribe(() => {
	if (get(activePlayer) === 'podcast') syncPodcastMediaMetadata();
	syncNativePiPBridgeFromState();
});
currentPodcast.subscribe(() => {
	if (get(activePlayer) === 'podcast') syncPodcastMediaMetadata();
});
abPlaybackState.subscribe((state) => {
	if (get(activePlayer) === 'audiobook') {
		syncMediaSessionPlaybackState(state === 'playing' ? 'playing' : state === 'paused' ? 'paused' : 'idle');
	}
	syncNativePiPBridgeFromState();
});
podcastPlaybackState.subscribe((state) => {
	if (get(activePlayer) === 'podcast') {
		syncMediaSessionPlaybackState(state === 'playing' ? 'playing' : state === 'paused' ? 'paused' : 'idle');
	}
	syncNativePiPBridgeFromState();
});
activePlayer.subscribe((mode) => {
	if (!mediaSessionSupported()) return;
	if (mode === 'audiobook') {
		syncAudiobookMediaMetadata();
		const abState = get(abPlaybackState);
		syncMediaSessionPlaybackState(abState === 'playing' ? 'playing' : 'paused');
	} else if (mode === 'podcast') {
		syncPodcastMediaMetadata();
		const podState = get(podcastPlaybackState);
		syncMediaSessionPlaybackState(podState === 'playing' ? 'playing' : 'paused');
	} else {
		syncMediaMetadata(get(currentTrack));
		syncMediaSessionPlaybackState(get(playbackState));
	}
	syncNativePiPBridgeFromState();
});
currentTrack.subscribe(() => {
	syncNativePiPBridgeFromState();
});

// ── Visibility change handler ────────────────────────────────────────────────

if (typeof document !== 'undefined') {
	document.addEventListener('visibilitychange', () => {
		if (document.visibilityState !== 'visible') return;

		refreshDevicesFromSession().catch(() => { });

		if (isAndroidNative) {
			// Full snapshot reconciliation — recovers from WebView freeze.
			engine.reconcileFromNativeSnapshot().catch(() => { });
			return;
		}

		if (
			get(playbackState) === 'playing' &&
			audioEngine.isLoaded &&
			!engine.isCurrentlyRemoteMirror()
		) {
			audioEngine.resumeAllContexts();
		}
	});
}

// ── Discord Rich Presence ────────────────────────────────────────────────────

if (isTauri() && !isAndroidNative) {
	type DiscordPresencePayload = {
		title: string;
		artist: string;
		playing: boolean;
		coverUrl: string | null;
	};

	function getDiscordPresencePayload(): DiscordPresencePayload | null {
		if (!get(discordEnabled)) return null;
		const apiBase = getApiBase();
		const mode = get(activePlayer);

		if (mode === 'audiobook') {
			const audiobook = get(currentAudiobook);
			if (!audiobook) return null;
			return {
				title: audiobook.title,
				artist: audiobook.author_name ?? '',
				playing: get(abPlaybackState) === 'playing',
				coverUrl: apiBase.startsWith('https://') ? `${apiBase}/covers/audiobook/${audiobook.id}` : null,
			};
		}

		if (mode === 'podcast') {
			const episode = get(currentEpisode);
			if (!episode) return null;
			const podcast = get(currentPodcast);
			return {
				title: episode.title,
				artist: podcast?.title ?? podcast?.author ?? '',
				playing: get(podcastPlaybackState) === 'playing',
				coverUrl: podcast?.id && apiBase.startsWith('https://')
					? `${apiBase}/covers/podcast/${podcast.id}`
					: null,
			};
		}

		const track = get(currentTrack);
		if (!track) return null;
		return {
			title: track.title,
			artist: track.artist_name ?? '',
			playing: get(playbackState) === 'playing',
			coverUrl: track.album_id && apiBase.startsWith('https://')
				? `${apiBase}/covers/${track.album_id}`
				: null,
		};
	}

	function pushDiscordPresence() {
		const payload = getDiscordPresencePayload();
		if (!payload) return;
		invoke('discord_update', payload).catch((err) => console.error('[Discord] update failed:', err));
	}

	function clearDiscordPresence() {
		invoke('discord_clear').catch(() => { });
	}

	function syncDiscordPresence() {
		if (!get(discordEnabled)) return;
		if (getDiscordPresencePayload()) pushDiscordPresence();
		else clearDiscordPresence();
	}

	currentTrack.subscribe(() => syncDiscordPresence());
	currentAudiobook.subscribe(() => syncDiscordPresence());
	currentEpisode.subscribe(() => syncDiscordPresence());
	currentPodcast.subscribe(() => syncDiscordPresence());
	activePlayer.subscribe(() => syncDiscordPresence());

	playbackState.subscribe((state) => {
		if (get(discordEnabled) && state !== 'loading') {
			syncDiscordPresence();
		}
	});
	abPlaybackState.subscribe((state) => {
		if (get(discordEnabled) && state !== 'loading') {
			syncDiscordPresence();
		}
	});
	podcastPlaybackState.subscribe((state) => {
		if (get(discordEnabled) && state !== 'loading') {
			syncDiscordPresence();
		}
	});

	discordEnabled.subscribe(async (enabled) => {
		if (!enabled) {
			invoke('discord_disconnect').catch(() => { });
			clearDiscordPresence();
		} else {
			await invoke('discord_connect').then(() => {
				syncDiscordPresence();
			}).catch((err) => {
				console.error('[Discord] connect failed:', err);
			});
		}
	});
}

// ── System Tray Integration ──────────────────────────────────────────────────

if (isTauri() && !isAndroidNative) {
	playbackState.subscribe((state) => {
		invoke('set_tray_playback_state', { playing: state === 'playing' }).catch(() => { });
	});

	listen('tray-play-pause', () => togglePlayPause()).catch(() => { });
	listen('tray-previous', () => previous()).catch(() => { });
	listen('tray-next', () => next()).catch(() => { });
}

// ── Position state sync (throttled) ──────────────────────────────────────────

let _posStateTimer: ReturnType<typeof setTimeout> | null = null;
function currentPositionForMode(): number {
	const mode = get(activePlayer);
	if (mode === 'audiobook') return get(abPositionMs);
	if (mode === 'podcast') return get(podcastPositionMs);
	return get(positionMs);
}
function currentDurationForMode(): number {
	const mode = get(activePlayer);
	if (mode === 'audiobook') return get(abDurationMs);
	if (mode === 'podcast') return get(podcastDurationMs);
	return get(durationMs);
}
function schedulePositionStateSync() {
	if (_posStateTimer) return;
	_posStateTimer = setTimeout(() => {
		_posStateTimer = null;
		syncPositionState(currentPositionForMode(), currentDurationForMode());
	}, TIMINGS.POSITION_STATE_SYNC);
}
positionMs.subscribe(schedulePositionStateSync);
abPositionMs.subscribe(schedulePositionStateSync);
podcastPositionMs.subscribe(schedulePositionStateSync);
durationMs.subscribe(() => syncPositionState(currentPositionForMode(), currentDurationForMode()));
abDurationMs.subscribe(() => syncPositionState(currentPositionForMode(), currentDurationForMode()));
podcastDurationMs.subscribe(() => syncPositionState(currentPositionForMode(), currentDurationForMode()));

// ── Restore state ────────────────────────────────────────────────────────────

(async function restoreState() {
	try {
		const raw = localStorage.getItem(STORAGE_KEY);
		if (!raw) return;
		const st = JSON.parse(raw);

		// Restore common settings
		let vol = typeof st.volume === 'number' ? st.volume : 1;
		if (nativePlatform() === 'android') {
			try { vol = await invoke<number>('get_volume'); } catch { /* use saved */ }
		}
		volume.set(vol);
		audioEngine.setVolume(vol);
		engine.setVolume(vol);
		if (st.repeat === 'one' || st.repeat === 'all') repeatMode.set(st.repeat);
		if (st.shuffle === true) {
			shuffle.set(true);
			if (Array.isArray(st.shuffleOrder)) shuffleOrder.set(st.shuffleOrder);
		}
		if (st.autoplay === false) autoplayEnabled.set(false);
		if (st.discord === true) discordEnabled.set(true);
		if (st.replayGain === true) replayGainEnabled.set(true);
		if (st.smartShuffle === true) smartShuffleEnabled.set(true);

		// Restore podcast player
		if (st.activePlayer === 'podcast' && st.podcastEpisodeId) {
			const [episodeRes, podcastRes, progressRes] = await Promise.all([
				podcastsApi.getEpisode(st.podcastEpisodeId).catch(() => null),
				st.podcastId ? podcastsApi.get(st.podcastId).catch(() => null) : Promise.resolve(null),
				podcastsApi.getProgress(st.podcastEpisodeId).catch(() => null),
			]);
			const episode = episodeRes?.episode ?? null;
			if (episode) {
				let podcast = podcastRes?.podcast ?? null;
				if (!podcast) {
					const fallbackPodcastRes = await podcastsApi.get(episode.podcast_id).catch(() => null);
					podcast = fallbackPodcastRes?.podcast ?? null;
				}
				if (podcast) {
					const posMs = progressRes?.progress?.position_ms ?? (typeof st.podcastPos === 'number' ? st.podcastPos * 1000 : 0);
					restorePodcastState(episode, podcast, posMs);
				}
			}
			// Also restore music queue in the background
			if (st.trackId) {
				libraryApi.track(st.trackId).then((res) => {
					if (!res?.track) return;
					currentTrack.set(res.track);
					durationMs.set(res.track.duration_ms);
					positionMs.set((st.pos || 0) * 1000);
					queueIndex.set(typeof st.queueIndex === 'number' ? st.queueIndex : 0);
					if (Array.isArray(st.queue) && st.queue.length) {
						queue.set(st.queue as Track[]);
					}
				}).catch(() => { });
			}
			return;
		}

		// Restore audiobook player
		if (st.activePlayer === 'audiobook' && st.audiobookId) {
			const [abRes, progressRes] = await Promise.all([
				audiobooksApi.get(st.audiobookId).catch(() => null),
				audiobooksApi.getProgress(st.audiobookId).catch(() => null),
			]);
			if (abRes?.audiobook) {
				const book = abRes.audiobook;
				const posMs = progressRes?.progress?.position_ms ?? 0;
				restoreAudiobookState(book, posMs);
			}
			// Also restore music queue in the background
			if (st.trackId) {
				libraryApi.track(st.trackId).then((res) => {
					if (!res?.track) return;
					currentTrack.set(res.track);
					durationMs.set(res.track.duration_ms);
					positionMs.set((st.pos || 0) * 1000);
					queueIndex.set(typeof st.queueIndex === 'number' ? st.queueIndex : 0);
					if (Array.isArray(st.queue) && st.queue.length) {
						queue.set(st.queue as Track[]);
					}
				}).catch(() => { });
			}
			return;
		}

		// Restore music player
		if (!st?.trackId) return;

		const res = await libraryApi.track(st.trackId).catch(() => null);
		if (!res?.track) return;
		const track = res.track;

		currentTrack.set(track);
		durationMs.set(track.duration_ms);
		positionMs.set((st.pos || 0) * 1000);
		queueIndex.set(typeof st.queueIndex === 'number' ? st.queueIndex : 0);

		if (Array.isArray(st.queue) && st.queue.length) {
			queue.set(st.queue as Track[]);
		} else if (Array.isArray(st.queueIds) && st.queueIds.length) {
			const res = await libraryApi.tracksBatch(st.queueIds).catch(() => null);
			const qTracks = (Array.isArray(res) ? res : []) as Track[];
			if (qTracks.length) queue.set(qTracks);
		}

		if (isAndroidNative) {
			// Use snapshot reconciliation for a complete state sync on startup.
			try {
				await engine.reconcileFromNativeSnapshot();
			} catch {
				playbackState.set('paused');
			}
		} else {
			playbackState.set('paused');
		}
	} catch {
		// ignore parse / storage errors
	} finally {
		saveEnabled = true;
		engine.signalRestoreComplete();
	}
})().catch(() => { engine.signalRestoreComplete(); });

// ── Stop playback on logout ───────────────────────────────────────────────────

let _prevToken: string | null = get(authStore).token;
authStore.subscribe((auth) => {
	if (_prevToken !== null && auth.token === null) {
		// User just logged out — halt all playback immediately
		if (get(activePlayer) === 'audiobook') {
			pauseAudiobook();
		} else {
			audioEngine.stop();
			positionMs.set(0);
			playbackState.set('paused');
		}
	}
	_prevToken = auth.token;
});

// ── Android Native Event Bridge ──────────────────────────────────────────────

if (isAndroidNative) {
	listen('native-next', () => { next(); }).catch(() => { });
	listen('native-previous', () => { previous(); }).catch(() => { });
	listen('native-shuffle-toggle', () => { toggleShuffle(); }).catch(() => { });
	listen('native-favorite-toggle', () => {
		const track = get(currentTrack);
		if (track) favorites.toggle(track.id, track).catch(() => { });
	}).catch(() => { });

	shuffle.subscribe((sh) => {
		invoke('set_shuffle_state', { shuffled: sh }).catch(() => { });
	});

	listen<number>('native-volume-change', (event) => {
		volume.set(event.payload);
		audioEngine.setVolume(event.payload);
	}).catch(() => { });

	const syncNativeFavorite = () => {
		const track = get(currentTrack);
		if (!track) return;
		const isFav = get(favorites).has(track.id);
		invoke('set_favorite_state', { favorited: isFav }).catch(() => { });
	};
	currentTrack.subscribe(syncNativeFavorite);
	favorites.subscribe(syncNativeFavorite);
}
