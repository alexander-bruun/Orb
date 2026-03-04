import { writable, derived, get } from 'svelte/store';
import type { Track, PlaybackState } from '$lib/types';
import { audioEngine } from '$lib/audio/engine';
import { getApiBase } from '$lib/api/base';
import { queue as queueApi } from '$lib/api/queue';
import { library as libraryApi } from '$lib/api/library';
import { recommend } from '$lib/api/recommend';
import { addToast } from '$lib/stores/toast';
import { isTauri } from '$lib/utils/platform';

export const currentTrack = writable<Track | null>(null);
export const playbackState = writable<PlaybackState>('idle');
export const positionMs = writable(0);
export const durationMs = writable(0);
export const volume = writable(1);
export const queue = writable<Track[]>([]);
export const queueIndex = writable(0);
/** Percentage (0–100) of the track that has been downloaded/buffered. */
export const bufferedPct = writable(0);
export const repeatMode = writable<'off' | 'one' | 'all'>('off');
export const shuffle = writable(false);
/** Permutation of queue indices used when shuffle is on. */
export const shuffleOrder = writable<number[]>([]);
/** Tracks explicitly queued by the user via Play Next / Add to Queue. */
export const userQueue = writable<Track[]>([]);
/** Controls visibility of the Up Next queue panel. */
export const queueModalOpen = writable(false);
/** When true, similar tracks auto-queue when the queue runs out. */
export const autoplayEnabled = writable(true);
/** When true, display current track in Discord Rich Presence (desktop only). */
export const discordEnabled = writable(false);
/** When true, normalize track loudness using ReplayGain metadata. */
export const replayGainEnabled = writable(false);

/** Fisher-Yates shuffle, optionally pinning one index to position 0. */
function generateShuffle(length: number, pinIndex = -1): number[] {
	const order = Array.from({ length }, (_, i) => i);
	for (let i = length - 1; i > 0; i--) {
		const j = Math.floor(Math.random() * (i + 1));
		[order[i], order[j]] = [order[j], order[i]];
	}
	if (pinIndex >= 0) {
		const pos = order.indexOf(pinIndex);
		if (pos !== 0) [order[0], order[pos]] = [order[pos], order[0]];
	}
	return order;
}

/**
 * Map a logical queue position to the actual queue array index.
 * When shuffle is off this is a no-op.
 */
function actualIndex(logicalPos: number): number {
	const order = get(shuffleOrder);
	return get(shuffle) && order.length > logicalPos ? order[logicalPos] : logicalPos;
}

export const formattedFormat = derived(currentTrack, ($t) => {
	if (!$t) return '';
	const bd = $t.bit_depth ? `${$t.bit_depth}bit` : '';
	const sr = `${($t.sample_rate / 1000).toFixed(1)}kHz`;
	return [bd, sr].filter(Boolean).join(' · ');
});

export const formattedPosition = derived(positionMs, ($ms) => formatTime($ms));
export const formattedDuration = derived(durationMs, ($ms) => formatTime($ms));

function formatTime(ms: number): string {
	const s = Math.floor(ms / 1000);
	const m = Math.floor(s / 60);
	const sec = s % 60;
	return `${m}:${sec.toString().padStart(2, '0')}`;
}

export async function playTrack(track: Track, trackList?: Track[], startSeconds = 0) {
	if (trackList) {
		queue.set(trackList);
		const idx = trackList.findIndex((t) => t.id === track.id);
		const actualIdx = idx >= 0 ? idx : 0;
		if (get(shuffle)) {
			const order = generateShuffle(trackList.length, actualIdx);
			shuffleOrder.set(order);
			queueIndex.set(0);
		} else {
			queueIndex.set(actualIdx);
		}
	}
	currentTrack.set(track);
	playbackState.set('loading');
	try {
		// Apply replay gain before starting playback so the engine applies it
		// from the very first decoded sample.
		const rgDb = get(replayGainEnabled) ? (track.replay_gain_track ?? 0) : 0;
		audioEngine.setReplayGainDb(rgDb);
		await audioEngine.play(track.id, track.bit_depth ?? 16, track.sample_rate, startSeconds);
		playbackState.set('playing');
		// Record the play fire-and-forget; ignore errors so playback is never blocked.
		libraryApi.recordPlay(track.id, 0).catch(() => {});
	} catch (err) {
		console.error('playTrack error', err);
		playbackState.set('idle');
	}
}

export function togglePlayPause() {
	const state = get(playbackState);
	if (state === 'playing') {
		audioEngine.pause();
		playbackState.set('paused');
	} else if (state === 'paused') {
		if (!audioEngine.isLoaded) {
			// Restore scenario: nothing is loaded in the engine yet (e.g. after page
			// refresh). Load the track starting from the saved position.
			const track = get(currentTrack);
			if (track) {
				playTrack(track, undefined, get(positionMs) / 1000);
			}
		} else {
			audioEngine.resume();
			playbackState.set('playing');
		}
	}
}

export function seek(posSeconds: number) {
	const dur = get(durationMs) || 0;
	const maxSec = Math.max(0, dur / 1000);
	const clamped = Math.max(0, Math.min(posSeconds, Math.max(0, maxSec - 0.01)));
	audioEngine.seek(clamped);
	positionMs.set(clamped * 1000);
}

export function setVolume(gain: number) {
	volume.set(gain);
	audioEngine.setVolume(gain);
}

/** Insert a track at the front of the user queue (plays after current track). */
export function playNext(track: Track) {
	userQueue.update((q) => [track, ...q]);
}

/** Append a track to the end of the user queue. */
export function addToQueue(track: Track) {
	userQueue.update((q) => [...q, track]);
}

/** Remove a track at the given index from the user queue. */
export function removeFromUserQueue(index: number) {
	userQueue.update((q) => q.filter((_, i) => i !== index));
}

export async function next() {
	// User-queued tracks take priority over the playback context.
	const uq = get(userQueue);
	if (uq.length > 0) {
		const [nextTrack, ...rest] = uq;
		userQueue.set(rest);
		// Close modal if queue drops to 1 or fewer
		if (rest.length <= 1) queueModalOpen.set(false);
		await playTrack(nextTrack);
		return;
	}

	const q = get(queue);
	const idx = get(queueIndex);
	const repeat = get(repeatMode);

	if (repeat === 'one') {
		const track = get(currentTrack);
		if (track) await playTrack(track);
		return;
	}

	if (idx < q.length - 1) {
		const nextIdx = idx + 1;
		queueIndex.set(nextIdx);
		await playTrack(q[actualIndex(nextIdx)]);
	} else if (repeat === 'all') {
		if (get(shuffle)) {
			const order = generateShuffle(q.length);
			shuffleOrder.set(order);
		}
		queueIndex.set(0);
		await playTrack(q[actualIndex(0)]);
	} else if (get(autoplayEnabled)) {
		// End of queue — try auto-playing similar tracks.
		const current = get(currentTrack);
		if (current) {
			try {
				const qIds = q.map((t: Track) => t.id);
				const recs = await recommend.autoplay(current.id, qIds, 10);
				if (recs.length > 0) {
					const newQueue = [...q, ...recs];
					queue.set(newQueue);
					const nextIdx = idx + 1;
					queueIndex.set(nextIdx);
					await playTrack(newQueue[actualIndex(nextIdx)]);
					return;					} else {
						addToast('No similar tracks found — run the ingest to compute similarities.', 'info');				}
			} catch {
				// Fall through to stop.
			}
		}
		audioEngine.stop();
		positionMs.set(0);
		playbackState.set('paused');
	} else {
		// Autoplay disabled: stop the engine, reset position.
		audioEngine.stop();
		positionMs.set(0);
		playbackState.set('paused');
	}
}

export async function previous() {
	const idx = get(queueIndex);
	if (get(positionMs) > 3000) {
		seek(0);
		return;
	}
	if (idx > 0) {
		const prevIdx = idx - 1;
		queueIndex.set(prevIdx);
		const q = get(queue);
		await playTrack(q[actualIndex(prevIdx)]);
	}
}

export function toggleRepeat() {
	repeatMode.update((m) => (m === 'off' ? 'one' : m === 'one' ? 'all' : 'off'));
}

export function toggleShuffle() {
	shuffle.update((sh) => {
		const q = get(queue);
		const idx = get(queueIndex);
		if (!sh) {
			// Turning on: pin the current track to position 0 in the shuffle order.
			const order = generateShuffle(q.length, idx);
			shuffleOrder.set(order);
			queueIndex.set(0);
		} else {
			// Turning off: restore queueIndex to the actual queue position.
			const order = get(shuffleOrder);
			queueIndex.set(order.length > 0 ? order[idx] : idx);
			shuffleOrder.set([]);
		}
		return !sh;
	});
}

export async function loadQueue() {
	try {
		const tracks = await queueApi.get();
		queue.set(tracks);
	} catch {
		// ignore
	}
}

/**
 * Start a radio queue.
 * If a seed track ID is provided, loads tracks similar to that track.
 * Otherwise loads a personalised station based on the user's listening history.
 */
export async function startRadio(seedTrackId?: string) {
	let tracks: Track[];
	try {
		tracks = seedTrackId
			? await recommend.similar(seedTrackId, 50)
			: await recommend.radio(50);
	} catch (err) {
		console.error('startRadio error', err);
		return;
	}
	if (!tracks || tracks.length === 0) return;
	shuffle.set(false);
	shuffleOrder.set([]);
	queue.set(tracks);
	queueIndex.set(0);
	await playTrack(tracks[0], tracks);
}

// Persistence: save minimal player state so we can resume after a refresh.
const STORAGE_KEY = 'orb-player-state-v1';
const POSITION_SAVE_INTERVAL_MS = 1000;
let lastWriteTime = 0;
let saveTimeout: ReturnType<typeof setTimeout> | null = null;
// Saves are disabled until restoreState() completes so that store
// initialization (writable defaults) and the async restore don't wipe
// localStorage before we've had a chance to read it.
let saveEnabled = false;

function writeState() {
	try {
		const st = {
			trackId: get(currentTrack)?.id ?? null,
			pos: Math.floor(get(positionMs) / 1000),
			// Store full track objects so restore doesn't need individual API calls.
			// The legacy "queueIds" key is intentionally omitted from new saves.
			queue: get(queue),
			queueIndex: get(queueIndex),
			volume: get(volume),
			repeat: get(repeatMode),
			shuffle: get(shuffle),
			shuffleOrder: get(shuffleOrder),
			autoplay: get(autoplayEnabled),
			discord: get(discordEnabled),
			replayGain: get(replayGainEnabled)
		};
		localStorage.setItem(STORAGE_KEY, JSON.stringify(st));
	} catch {
		// ignore storage errors
	}
}

// Position updates fire every ~250ms, so debouncing doesn't work — the timer
// keeps getting reset. Instead, throttle position saves to at most once every
// POSITION_SAVE_INTERVAL_MS, with a trailing write so the final position is
// always captured.
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

// Non-position state changes (track, volume, queue) are infrequent — save
// promptly after a short debounce.
function scheduleStateSave() {
	if (!saveEnabled) return;
	if (saveTimeout) clearTimeout(saveTimeout);
	saveTimeout = setTimeout(() => {
		writeState();
		lastWriteTime = Date.now();
		saveTimeout = null;
	}, 200);
}

positionMs.subscribe(() => schedulePositionSave());
playbackState.subscribe(() => scheduleStateSave());
currentTrack.subscribe(() => scheduleStateSave());
queueIndex.subscribe(() => scheduleStateSave());
volume.subscribe(() => scheduleStateSave());
repeatMode.subscribe(() => scheduleStateSave());
shuffle.subscribe(() => scheduleStateSave());
autoplayEnabled.subscribe(() => scheduleStateSave());
discordEnabled.subscribe(() => scheduleStateSave());
replayGainEnabled.subscribe(() => {
	scheduleStateSave();
	// Re-apply (or clear) replay gain for the currently playing track when the
	// user toggles the feature, so the change takes effect immediately.
	const track = get(currentTrack);
	if (track) {
		const rgDb = get(replayGainEnabled) ? (track.replay_gain_track ?? 0) : 0;
		audioEngine.setReplayGainDb(rgDb);
	}
});

// ─── Media Session API ───────────────────────────────────────────────────────
// Wires hardware media keys (Play/Pause, Next, Previous on keyboard/headphones)
// and OS-level media overlays to the player store.

function mediaSessionSupported(): boolean {
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
		artwork,
	});
}

function syncMediaSessionPlaybackState(state: PlaybackState) {
	if (!mediaSessionSupported()) return;
	navigator.mediaSession.playbackState =
		state === 'playing' ? 'playing' :
		state === 'paused' || state === 'idle' ? 'paused' : 'none';
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

if (mediaSessionSupported()) {
	navigator.mediaSession.setActionHandler('play', () => togglePlayPause());
	navigator.mediaSession.setActionHandler('pause', () => togglePlayPause());
	navigator.mediaSession.setActionHandler('previoustrack', () => { previous(); });
	navigator.mediaSession.setActionHandler('nexttrack', () => { next(); });
	navigator.mediaSession.setActionHandler('seekto', (details) => {
		if (details.seekTime !== undefined) seek(details.seekTime);
	});
}

currentTrack.subscribe(syncMediaMetadata);
playbackState.subscribe(syncMediaSessionPlaybackState);

// ─── Discord Rich Presence ───────────────────────────────────────────────────
// Only active in the Tauri desktop shell. Updates presence when the track
// changes or Discord is toggled on; clears it when toggled off.
if (isTauri()) {
	async function pushDiscordPresence(track: Track | null) {
		if (!get(discordEnabled) || !track) return;
		try {
			const { invoke } = await import(/* @vite-ignore */ '@tauri-apps/api/core');
			await invoke('discord_update', {
				title: track.title,
				artist: track.artist_name ?? '',
				album: ''
			});
		} catch {
			// Discord may not be running — ignore silently.
		}
	}

	async function clearDiscordPresence() {
		try {
			const { invoke } = await import(/* @vite-ignore */ '@tauri-apps/api/core');
			await invoke('discord_clear');
		} catch {
			// ignore
		}
	}

	currentTrack.subscribe((track) => {
		if (get(discordEnabled)) {
			if (track) pushDiscordPresence(track);
			else clearDiscordPresence();
		}
	});

	discordEnabled.subscribe(async (enabled) => {
		if (!enabled) {
			await clearDiscordPresence();
		} else {
			const track = get(currentTrack);
			if (track) await pushDiscordPresence(track);
		}
	});
}

// ─── System Tray Integration ─────────────────────────────────────────────────
// Syncs playback state to the tray Play/Pause label and handles tray menu
// button clicks (Previous / Play-Pause / Next) via Tauri events.
if (isTauri()) {
	playbackState.subscribe(async (state) => {
		try {
			const { invoke } = await import(/* @vite-ignore */ '@tauri-apps/api/core');
			await invoke('set_tray_playback_state', { playing: state === 'playing' });
		} catch {
			// ignore — tray may not be ready yet on first load
		}
	});

	// Listen for tray menu button events emitted from Rust.
	(async () => {
		try {
			const { listen } = await import(/* @vite-ignore */ '@tauri-apps/api/event');
			await listen('tray-play-pause', () => togglePlayPause());
			await listen('tray-previous', () => previous());
			await listen('tray-next', () => next());
		} catch {
			// ignore
		}
	})();
}

// Throttle position state updates — positionMs fires every ~250 ms.
let _posStateTimer: ReturnType<typeof setTimeout> | null = null;
function schedulePositionStateSync() {
	if (_posStateTimer) return;
	_posStateTimer = setTimeout(() => {
		_posStateTimer = null;
		syncPositionState(get(positionMs), get(durationMs));
	}, 500);
}
positionMs.subscribe(schedulePositionStateSync);
durationMs.subscribe(() => syncPositionState(get(positionMs), get(durationMs)));

// Restore previous state on load. Always restores as paused — the user must
// press play to resume. This avoids autoplay policy violations and the
// play-from-0-then-seek glitch from the prior approach of calling playTrack
// then seeking afterward.
(async function restoreState() {
	try {
		const raw = localStorage.getItem(STORAGE_KEY);
		if (!raw) return;
		const st = JSON.parse(raw);
		if (!st?.trackId) return;

		const res = await libraryApi.track(st.trackId).catch(() => null);
		if (!res?.track) return;
		const track = res.track;

		currentTrack.set(track);
		durationMs.set(track.duration_ms);
		positionMs.set((st.pos || 0) * 1000);
		queueIndex.set(typeof st.queueIndex === 'number' ? st.queueIndex : 0);
		const vol = typeof st.volume === 'number' ? st.volume : 1;
		volume.set(vol);
		audioEngine.setVolume(vol);
		if (st.repeat === 'one' || st.repeat === 'all') repeatMode.set(st.repeat);
		if (st.shuffle === true) {
			shuffle.set(true);
			if (Array.isArray(st.shuffleOrder)) shuffleOrder.set(st.shuffleOrder);
		}
		if (st.autoplay === false) autoplayEnabled.set(false);
		if (st.discord === true) discordEnabled.set(true);
		if (st.replayGain === true) replayGainEnabled.set(true);

		if (Array.isArray(st.queue) && st.queue.length) {
			// New format: full track objects stored directly — no API calls needed.
			queue.set(st.queue as Track[]);
		} else if (Array.isArray(st.queueIds) && st.queueIds.length) {
			// Legacy format: only IDs were saved — batch-fetch in one request.
			const res = await libraryApi.tracksBatch(st.queueIds).catch(() => null);
			const qTracks = (Array.isArray(res) ? res : []) as Track[];
			if (qTracks.length) queue.set(qTracks);
		}

		// Leave playback state as paused. Pressing play will call togglePlayPause,
		// which detects the unloaded engine and starts from the saved position.
		playbackState.set('paused');
	} catch {
		// ignore parse / storage errors
	} finally {
		saveEnabled = true;
	}
})();
