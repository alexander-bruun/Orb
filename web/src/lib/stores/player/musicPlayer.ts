/**
 * Music player store — all music-specific playback logic.
 *
 * Features:
 *  - Queue management (shuffle, smart shuffle, repeat, user queue)
 *  - Crossfade / gapless transitions (WASM hi-res path)
 *  - Radio mode (auto-queue similar tracks)
 *  - Device transfer (push/pull playback between devices)
 *  - Remote state mirroring (shadow tick for smooth progress bars)
 *  - Exclusive mode delegation (control-device mode)
 */

import { writable, derived, get } from 'svelte/store';
import type { Track, PlaybackState } from '$lib/types';
import { audioEngine } from '$lib/audio/engine';
import { getApiBase } from '$lib/api/base';
import { queue as queueApi } from '$lib/api/queue';
import { library as libraryApi } from '$lib/api/library';
import { recommend } from '$lib/api/recommend';
import { addToast } from '$lib/stores/ui/toast';
import { isTauri, nativePlatform } from '$lib/utils/platform';
import { buildNativeStreamUrl } from '$lib/utils/nativeStream';
import { authStore } from '$lib/stores/auth';
import { exclusiveMode, deviceId, activeDeviceId, activeDevices, sendHeartbeat } from './deviceSession';
import { devices as devicesApi } from '$lib/api/devices';
import { isCurrentlyOffline } from '$lib/stores/offline/connectivity';
import { selectedAudioOutputId, sinkIdSupported } from './casting';
import { crossfadeEnabled, crossfadeSecs, gaplessEnabled } from '$lib/stores/settings/crossfade';
import * as engine from './engine';
import type { MusicContentProvider, ControlPayload, RemoteState } from './engine';
import { notifyTrackEnd as sleepTimerNotifyTrackEnd } from './musicSleepTimer';

// ── Playback state ────────────────────────────────────────────────────────────

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
/** When true, shuffle spreads tracks by artist and de-prioritises recently played. */
export const smartShuffleEnabled = writable(false);
/** Controls visibility of the Up Next queue panel. */
export const queueModalOpen = writable(false);
/** When true, similar tracks auto-queue when the queue runs out. */
export const autoplayEnabled = writable(true);
/**
 * When set, the player is in radio mode: the queue is continuously topped up
 * with similar tracks as playback advances. Set by startRadio(), cleared by
 * stopRadio() or explicit user play actions.
 */
export const radioMode = writable<{ seedTrackId?: string; seedArtistId?: string } | null>(null);
/** When true, display current track in Discord Rich Presence (desktop only). */
export const discordEnabled = writable(false);
/** When true, normalize track loudness using ReplayGain metadata. */
export const replayGainEnabled = writable(false);

/** Tracks played in this session; used by smart shuffle to de-prioritise repeats. */
const recentlyPlayedIds = new Set<string>();

// ── Scrobble threshold timer ──────────────────────────────────────────────────
// Fires when >50% of a track (or >4 min) has elapsed and the same track is
// still playing, submitting a scrobble to Last.fm / ListenBrainz via the API.

let _scrobbleTimer: ReturnType<typeof setTimeout> | null = null;

function scheduleScrobble(track: Track) {
	if (_scrobbleTimer) clearTimeout(_scrobbleTimer);
	const startedAt = Date.now();
	const delayMs = Math.min((track.duration_ms ?? 300_000) / 2, 4 * 60_000);
	_scrobbleTimer = setTimeout(() => {
		_scrobbleTimer = null;
		if (get(currentTrack)?.id === track.id && get(playbackState) === 'playing') {
			libraryApi.scrobble(track.id, startedAt).catch(() => {});
		}
	}, delayMs);
}

function cancelScrobble() {
	if (_scrobbleTimer) {
		clearTimeout(_scrobbleTimer);
		_scrobbleTimer = null;
	}
}

// ── Shuffle helpers ──────────────────────────────────────────────────────────

/** Fisher-Yates in-place shuffle of an array. */
function fisherYates<T>(arr: T[]): T[] {
	for (let i = arr.length - 1; i > 0; i--) {
		const j = Math.floor(Math.random() * (i + 1));
		[arr[i], arr[j]] = [arr[j], arr[i]];
	}
	return arr;
}

/** Fisher-Yates shuffle, optionally pinning one index to position 0. */
function generateShuffle(length: number, pinIndex = -1): number[] {
	const order = Array.from({ length }, (_, i) => i);
	fisherYates(order);
	if (pinIndex >= 0) {
		const pos = order.indexOf(pinIndex);
		if (pos !== 0) [order[0], order[pos]] = [order[pos], order[0]];
	}
	return order;
}

/**
 * Smart shuffle: groups tracks by artist and interleaves the groups so the
 * same artist never plays back-to-back. Recently played tracks (this session)
 * are moved toward the end of the order. The pinned track (if any) is always
 * placed at position 0.
 */
function generateSmartShuffle(tracks: Track[], pinIndex = -1): number[] {
	const groupMap = new Map<string, number[]>();
	tracks.forEach((t, i) => {
		const key = t.artist_id ?? t.artist_name ?? '__unknown__';
		if (!groupMap.has(key)) groupMap.set(key, []);
		groupMap.get(key)!.push(i);
	});

	const groups = fisherYates([...groupMap.values()].map((idxs) => fisherYates([...idxs])));
	groups.sort((a, b) => b.length - a.length);

	const result: number[] = [];
	const maxLen = Math.max(...groups.map((g) => g.length), 0);
	for (let round = 0; round < maxLen; round++) {
		for (const group of groups) {
			if (round < group.length) result.push(group[round]);
		}
	}

	if (recentlyPlayedIds.size > 0) {
		const notRecent = result.filter((i) => !recentlyPlayedIds.has(tracks[i].id));
		const recent    = result.filter((i) =>  recentlyPlayedIds.has(tracks[i].id));
		result.length = 0;
		result.push(...notRecent, ...recent);
	}

	if (pinIndex >= 0) {
		const pos = result.indexOf(pinIndex);
		if (pos > 0) [result[0], result[pos]] = [result[pos], result[0]];
	}

	return result;
}

/** Returns either a smart or plain shuffle depending on the current setting. */
function buildShuffleOrder(tracks: Track[], pinIndex = -1): number[] {
	return get(smartShuffleEnabled)
		? generateSmartShuffle(tracks, pinIndex)
		: generateShuffle(tracks.length, pinIndex);
}

/**
 * Map a logical queue position to the actual queue array index.
 * When shuffle is off this is a no-op.
 */
function actualIndex(logicalPos: number): number {
	const order = get(shuffleOrder);
	return get(shuffle) && order.length > logicalPos ? order[logicalPos] : logicalPos;
}

// ── Native Android playback helpers ──────────────────────────────────────────

export const isAndroidNative = typeof window !== 'undefined' && nativePlatform() === 'android';

// ── Derived stores ───────────────────────────────────────────────────────────

/** Returns the maximum output channel count the browser's audio hardware supports. */
function browserMaxChannels(): number {
	try {
		const ctx = new AudioContext();
		const n = ctx.destination.maxChannelCount;
		ctx.close();
		return n;
	} catch {
		return 2;
	}
}

/** Converts a raw channel count to a layout label: "5.1", "7.1", or e.g. "4ch". */
function channelLayoutLabel(channels: number): string {
	if (channels <= 2) return '';
	if (channels === 6) return '5.1';
	if (channels === 8) return '7.1';
	return `${channels}ch`;
}

export const formattedFormat = derived(currentTrack, ($t) => {
	if (!$t) return '';
	const bd = $t.bit_depth ? `${$t.bit_depth}bit` : '';
	const sr = `${($t.sample_rate / 1000).toFixed(1)}kHz`;

	let ch = '';
	const srcChannels = $t.channels ?? 2;
	if (srcChannels > 2) {
		const srcLabel = channelLayoutLabel(srcChannels);
		const outChannels = browserMaxChannels();
		if (outChannels >= srcChannels) {
			ch = srcLabel;
		} else {
			// Browser hardware can't render all channels — it'll be downmixed.
			const outLabel = outChannels === 2 ? 'Stereo' : channelLayoutLabel(outChannels);
			ch = `${srcLabel} → ${outLabel}`;
		}
	}

	return [bd, sr, ch].filter(Boolean).join(' · ');
});

export const formattedPosition = derived(positionMs, ($ms) => formatTime($ms));
export const formattedDuration = derived(durationMs, ($ms) => formatTime($ms));

function formatTime(ms: number): string {
	const s = Math.floor(ms / 1000);
	const m = Math.floor(s / 60);
	const sec = s % 60;
	return `${m}:${sec.toString().padStart(2, '0')}`;
}

// ── Crossfade / gapless helpers ──────────────────────────────────────────────

function peekNext(): Track | null {
	const uq = get(userQueue);
	if (uq.length > 0) return uq[0];
	const q = get(queue);
	const idx = get(queueIndex);
	const repeat = get(repeatMode);
	if (repeat === 'one') return get(currentTrack);
	if (idx < q.length - 1) return q[actualIndex(idx + 1)];
	if (repeat === 'all') return q[actualIndex(0)];
	return null;
}

function advanceQueueState() {
	const uq = get(userQueue);
	if (uq.length > 0) {
		userQueue.update((q) => q.slice(1));
		return;
	}
	const q = get(queue);
	const idx = get(queueIndex);
	const repeat = get(repeatMode);
	if (repeat === 'one') return;
	if (idx < q.length - 1) {
		queueIndex.set(idx + 1);
	} else if (repeat === 'all') {
		if (get(shuffle)) shuffleOrder.set(buildShuffleOrder(get(queue)));
		queueIndex.set(0);
	}
}

async function setupCrossfade(track: Track): Promise<void> {
	if (!get(crossfadeEnabled) && !get(gaplessEnabled)) return;
	if (!audioEngine.isWasmActive) return;
	if ((track.bit_depth ?? 16) <= 16) return;

	const nextTrack = peekNext();
	if (!nextTrack) return;
	if ((nextTrack.bit_depth ?? 16) <= 16) return;

	const fadeSecs = get(crossfadeEnabled) ? get(crossfadeSecs) : 0;

	await audioEngine.preloadNext(nextTrack.id, nextTrack.sample_rate);

	audioEngine.onFullBufferForCrossfade(() => {
		audioEngine.scheduleCrossfade(fadeSecs, () => {
			currentTrack.set(nextTrack);
			durationMs.set(nextTrack.duration_ms);
			positionMs.set(0);
			playbackState.set('playing');

			const rgDb = get(replayGainEnabled) ? (nextTrack.replay_gain_track ?? 0) : 0;
			audioEngine.setReplayGainDb(rgDb);

			advanceQueueState();

			libraryApi.recordPlay(nextTrack.id, nextTrack.duration_ms ?? 0).catch(() => {});
			scheduleScrobble(nextTrack);
			sendHeartbeat().catch(() => {});

			setupCrossfade(nextTrack).catch(() => {});
		});
	});
}

// ── Core playback ────────────────────────────────────────────────────────────

export async function playTrack(track: Track, trackList?: Track[], startSeconds = 0) {
	const activeDev = get(activeDeviceId);
	if (get(exclusiveMode) && activeDev && activeDev !== deviceId && !isCurrentlyOffline()) {
		engine.switchMode('music');
		currentTrack.set(track);
		playbackState.set('loading');
		durationMs.set(track.duration_ms);
		positionMs.set(startSeconds * 1000);
		if (trackList) {
			queue.set(trackList);
			const idx = trackList.findIndex((t) => t.id === track.id);
			queueIndex.set(idx >= 0 ? idx : 0);
		}
		try {
			const tracksToSend = trackList ?? get(queue);
			await devicesApi.playCommand(
				activeDev,
				track.id,
				Math.round(startSeconds * 1000),
				tracksToSend && tracksToSend.length > 0 ? tracksToSend : undefined
			);
		} catch (err) {
			console.error('playTrack delegation error', err);
		}
		return;
	}
	if (trackList) {
		queue.set(trackList);
		const idx = trackList.findIndex((t) => t.id === track.id);
		const actualIdx = idx >= 0 ? idx : 0;
		if (get(shuffle)) {
			const order = buildShuffleOrder(trackList, actualIdx);
			shuffleOrder.set(order);
			queueIndex.set(0);
		} else {
			queueIndex.set(actualIdx);
		}
	}
	engine.switchMode('music');
	engine.stopShadowTick();
	currentTrack.set(track);
	playbackState.set('loading');

	// Set the engine's current content so heartbeats contain the correct track ID.
	engine._writableCurrentContent.set({
		id: track.id,
		title: track.title,
		artist: track.artist_name ?? undefined,
		album: track.album_id ?? undefined,
		coverUrl: track.album_id ? `${getApiBase()}/covers/${track.album_id}` : undefined,
		durationMs: track.duration_ms,
	});
	positionMs.set(startSeconds * 1000);
	durationMs.set(track.duration_ms);
	try {
		if (isAndroidNative) {
			const streamUrl = await buildNativeStreamUrl(track.id);
			const coverUrl = track.album_id ? `${getApiBase()}/covers/${track.album_id}?token=${encodeURIComponent(get(authStore).token ?? '')}` : undefined;
			await engine.play(streamUrl, {
				id: track.id,
				title: track.title,
				artist: track.artist_name ?? undefined,
				album: track.album_id ?? undefined,
				coverUrl,
				durationMs: track.duration_ms,
			}, {
				startMs: Math.round(startSeconds * 1000),
				isAudiobook: false,
				nativeUrl: streamUrl,
			});
			engine.setNativePlayerReady(true);
		} else {
			const rgDb = get(replayGainEnabled) ? (track.replay_gain_track ?? 0) : 0;
			audioEngine.setReplayGainDb(rgDb);
			if (sinkIdSupported) {
				const sinkId = get(selectedAudioOutputId);
				if (sinkId && sinkId !== 'default') {
					audioEngine.setAudioOutput(sinkId).catch(() => {});
				}
			}
			await audioEngine.play(track.id, track.bit_depth ?? 16, track.sample_rate, startSeconds);
			setupCrossfade(track).catch(() => {});
		}
		engine.setRemoteMirror(false);
		playbackState.set('playing');
		if (get(exclusiveMode) && deviceId) {
			devicesApi.activate(deviceId).catch(() => {});
		}
		sendHeartbeat().catch(() => {});
		recentlyPlayedIds.add(track.id);
		libraryApi.recordPlay(track.id, track.duration_ms ?? 0).catch(() => {});
		scheduleScrobble(track);
	} catch (err) {
		console.error('playTrack error', err);
		playbackState.set('idle');
	}
}

// ── Playback controls ────────────────────────────────────────────────────────

/**
 * Pause local audio without delegating to remote devices.
 * Used by the device session `pause_others` handler.
 */
export function pauseLocal() {
	engine.pause();
	cancelScrobble();
	const state = get(playbackState);
	if (state === 'playing' || state === 'loading') {
		playbackState.set('paused');
	}
}

export function togglePlayPause() {
	const activeDev = get(activeDeviceId);
	if (get(exclusiveMode) && activeDev && activeDev !== deviceId && !isCurrentlyOffline()) {
		devicesApi.controlCommand(activeDev, 'toggle').catch(() => {});
		return;
	}
	const state = get(playbackState);
	if (state === 'playing') {
		engine.pause();
		cancelScrobble();
		playbackState.set('paused');
		sendHeartbeat().catch(() => {});
	} else if (state === 'paused') {
		if (isAndroidNative) {
			if (!engine.isNativePlayerReady()) {
				const track = get(currentTrack);
				if (track) playTrack(track, undefined, get(positionMs) / 1000);
				return;
			}
			engine.resume();
			playbackState.set('playing');
			sendHeartbeat().catch(() => {});
		} else if (!audioEngine.isLoaded) {
			const track = get(currentTrack);
			if (track) {
				playTrack(track, undefined, get(positionMs) / 1000);
			}
		} else {
			engine.setRemoteMirror(false);
			audioEngine.resume();
			playbackState.set('playing');
			sendHeartbeat().catch(() => {});
		}
	}
}

export function seek(posSeconds: number) {
	const activeDev = get(activeDeviceId);
	if (get(exclusiveMode) && activeDev && activeDev !== deviceId && !isCurrentlyOffline()) {
		const posMs = Math.round(posSeconds * 1000);
		devicesApi.controlCommand(activeDev, 'seek', { position_ms: posMs }).catch(() => {});
		positionMs.set(posMs);
		engine.setShadowEpochMs(Date.now() - posMs);
		return;
	}
	const dur = get(durationMs) || 0;
	const maxSec = Math.max(0, dur / 1000);
	const clamped = Math.max(0, Math.min(posSeconds, Math.max(0, maxSec - 0.01)));
	engine.seek(Math.round(clamped * 1000));
	positionMs.set(clamped * 1000);
	sendHeartbeat().catch(() => {});
	const seekTrack = get(currentTrack);
	if (seekTrack) setupCrossfade(seekTrack).catch(() => {});
}

export function setVolume(gain: number) {
	const activeDev = get(activeDeviceId);
	if (get(exclusiveMode) && activeDev && activeDev !== deviceId && !isCurrentlyOffline()) {
		devicesApi.controlCommand(activeDev, 'volume', { volume: gain }).catch(() => {});
		return;
	}
	volume.set(gain);
	engine.setVolume(gain);
	sendHeartbeat().catch(() => {});
}

// ── Queue management ─────────────────────────────────────────────────────────

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

/** Fetch more similar tracks and append them to the queue (radio mode). */
async function _replenishRadio(currentQueue: Track[]) {
	const current = get(currentTrack);
	if (!current) return;
	const exclude = currentQueue.map((t: Track) => t.id);
	const more = await recommend.autoplay(current.id, exclude, 20);
	if (more.length > 0) {
		queue.update((q) => [...q, ...more]);
	}
}

export async function next() {
	const activeDev = get(activeDeviceId);
	if (get(exclusiveMode) && activeDev && activeDev !== deviceId && !isCurrentlyOffline()) {
		devicesApi.controlCommand(activeDev, 'next').catch(() => {});
		return;
	}
	const uq = get(userQueue);
	if (uq.length > 0) {
		const [nextTrack, ...rest] = uq;
		userQueue.set(rest);
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
		const rm = get(radioMode);
		if (rm && q.length - nextIdx <= 10) {
			_replenishRadio(q).catch(() => {});
		}
	} else if (repeat === 'all') {
		if (get(shuffle)) {
			shuffleOrder.set(buildShuffleOrder(q));
		}
		queueIndex.set(0);
		await playTrack(q[actualIndex(0)]);
	} else if (get(radioMode)) {
		try {
			await _replenishRadio(q);
			const newQ = get(queue);
			if (newQ.length > q.length) {
				const nextIdx = idx + 1;
				queueIndex.set(nextIdx);
				await playTrack(newQ[actualIndex(nextIdx)]);
				return;
			}
		} catch {
			// Fall through to stop.
		}
		audioEngine.stop();
		positionMs.set(0);
		playbackState.set('paused');
	} else if (get(autoplayEnabled)) {
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
					return;
				}
			} catch {
				// Fall through to stop.
			}
		}
		addToast('No more tracks available for autoplay.', 'info');
		audioEngine.stop();
		positionMs.set(0);
		playbackState.set('paused');
	} else {
		audioEngine.stop();
		positionMs.set(0);
		playbackState.set('paused');
	}
}

export async function previous() {
	const activeDev = get(activeDeviceId);
	if (get(exclusiveMode) && activeDev && activeDev !== deviceId && !isCurrentlyOffline()) {
		devicesApi.controlCommand(activeDev, 'previous').catch(() => {});
		return;
	}
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
			const order = buildShuffleOrder(q, idx);
			shuffleOrder.set(order);
			queueIndex.set(0);
		} else {
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

// ── Device transfer ──────────────────────────────────────────────────────────

export async function transferPlayback(targetId: string) {
	if (targetId === deviceId) {
		const devices = get(activeDevices);
		const active = devices.find((d) => d.is_active && d.id !== deviceId);

		activeDeviceId.set(deviceId);
		await devicesApi.activate(deviceId).catch(() => {});

		if (active?.state?.track_id) {
			await receivePlayCommand(active.state.track_id, active.state.position_ms ?? 0);
		}
		return;
	}

	const track = get(currentTrack);
	const pos   = Math.round(get(positionMs));
	const q     = get(queue);
	const wasPlaying = get(playbackState) === 'playing';

	if (wasPlaying && audioEngine.isLoaded) {
		audioEngine.pause();
		playbackState.set('paused');
	}

	if (track) {
		await devicesApi.playCommand(
			targetId,
			track.id,
			pos,
			q.length > 0 ? q : undefined
		).catch((err) => console.error('transferPlayback error', err));
	}
}

export async function receivePlayCommand(trackId: string, posMs: number, embeddedQueue?: Track[]) {
	let trackList: Track[];
	if (embeddedQueue && embeddedQueue.length > 0) {
		trackList = embeddedQueue;
	} else {
		await loadQueue();
		trackList = get(queue);
	}
	let track = trackList.find((t) => t.id === trackId);
	if (!track) {
		const res = await libraryApi.track(trackId).catch(() => null);
		if (!res?.track) return;
		track = res.track;
	}
	await playTrack(track, trackList.length > 0 ? trackList : undefined, posMs / 1000);
}

// ── Shadow tick / remote state mirroring ─────────────────────────────────────

export function stopShadowTick() {
	engine.stopShadowTick();
}

function startShadowTick() {
	engine.startShadowTick(undefined, 1.0);
}

export async function syncVisibleState(
	trackId: string,
	posMs: number,
	playing = true,
	epochMs?: number,
	remoteVolume?: number
) {
	if (get(playbackState) === 'playing' && audioEngine.isLoaded) return;

	if (remoteVolume !== undefined) {
		volume.set(remoteVolume);
	}

	if (playing) {
		engine.setShadowEpochMs(Date.now() - posMs);
	}

	const existing = get(currentTrack);
	if (existing?.id === trackId) {
		positionMs.set(posMs);
		engine.setRemoteMirror(playing);
		playbackState.set(playing ? 'playing' : 'paused');
		if (playing) {
			startShadowTick();
		} else {
			stopShadowTick();
		}
		return;
	}

	try {
		const res = await libraryApi.track(trackId);
		if (!res?.track) return;
		currentTrack.set(res.track);
		durationMs.set(res.track.duration_ms);
		positionMs.set(posMs);
		engine.setRemoteMirror(playing);
		playbackState.set(playing ? 'playing' : 'paused');
		if (playing) {
			startShadowTick();
		} else {
			stopShadowTick();
		}
	} catch {
		// ignore
	}
}

// ── Radio mode ───────────────────────────────────────────────────────────────

export async function startRadio(seedTrackId?: string, seedArtistId?: string) {
	let tracks: Track[];
	try {
		if (seedTrackId) {
			tracks = await recommend.similar(seedTrackId, 50);
		} else if (seedArtistId) {
			tracks = await recommend.radioByArtist(seedArtistId, 50);
		} else {
			tracks = await recommend.radio(50);
		}
	} catch (err) {
		console.error('startRadio error', err);
		return;
	}
	if (!tracks || tracks.length === 0) return;
	radioMode.set({ seedTrackId, seedArtistId });
	shuffle.set(false);
	shuffleOrder.set([]);
	queue.set(tracks);
	queueIndex.set(0);
	await playTrack(tracks[0], tracks);
}

/** Stop radio mode. The current queue remains but will not be topped up. */
export function stopRadio() {
	radioMode.set(null);
}

// ── Register music content provider with the unified engine ──────────────────

const musicContentProvider: MusicContentProvider = {
	onTrackEnd() {
		sleepTimerNotifyTrackEnd();
		next();
	},
	onPositionUpdate(ms: number) {
		positionMs.set(ms);
	},
	onModeActivated() {
		// Nothing extra needed — music playback is started via playTrack().
	},
	onModeDeactivated() {
		pauseLocal();
	},

	onRemoteSync(state: RemoteState) {
		if (state.track_id) {
			syncVisibleState(
				state.track_id,
				state.position_ms,
				state.playing,
				state.playback_epoch_ms,
				state.volume
			);
		}
	},
	onPlayCommand(trackId: string, posMs: number, queue?: unknown[]) {
		receivePlayCommand(trackId, posMs, queue as Track[] | undefined);
	},
	onPrevious() {
		previous();
	},
	onShuffleToggle() {
		shuffle.update((v: boolean) => !v);
	},
	onControlCommand(action: string, payload: ControlPayload) {
		switch (action) {
			case 'toggle': togglePlayPause(); break;
			case 'seek':
				if (payload?.position_ms !== undefined) seek(payload.position_ms / 1000);
				break;
			case 'volume':
				if (payload?.volume !== undefined) setVolume(payload.volume);
				break;
			case 'next': next(); break;
			case 'previous': previous(); break;
		}
	},
};
engine.registerProvider('music', musicContentProvider);

// ── Store bridges ────────────────────────────────────────────────────────────
// Bidirectional sync between engine stores and music player stores.
// Forward: engine → player (for engine-driven updates like shadow tick, native events).
// Reverse: player → engine (for browser path which bypasses engine.play()).
// Svelte's safe_not_equal prevents infinite loops.

engine.enginePlaybackState.subscribe((state) => {
	if (get(engine.mode) === 'music') {
		const current = get(playbackState);
		if (current !== state) playbackState.set(state as PlaybackState);
	}
});
engine.enginePositionMs.subscribe((ms) => {
	if (get(engine.mode) === 'music') {
		positionMs.set(ms);
	}
});
engine.engineDurationMs.subscribe((dur) => {
	if (get(engine.mode) === 'music') {
		durationMs.set(dur);
	}
});
engine.engineVolume.subscribe((vol) => {
	volume.set(vol);
});

playbackState.subscribe((state) => {
	if (get(engine.mode) === 'music') {
		engine._writablePlaybackState.set(state as engine.EnginePlaybackState);
	}
});
positionMs.subscribe((ms) => {
	if (get(engine.mode) === 'music') {
		engine._writablePositionMs.set(ms);
	}
});
durationMs.subscribe((dur) => {
	if (get(engine.mode) === 'music') {
		engine._writableDurationMs.set(dur);
	}
});
