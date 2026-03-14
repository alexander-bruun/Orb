import { writable, derived, get } from 'svelte/store';
import type { Track, PlaybackState } from '$lib/types';
import { audioEngine } from '$lib/audio/engine';
import { getApiBase } from '$lib/api/base';
import { queue as queueApi } from '$lib/api/queue';
import { library as libraryApi } from '$lib/api/library';
import { recommend } from '$lib/api/recommend';
import { addToast } from '$lib/stores/ui/toast';
import { isTauri, nativePlatform } from '$lib/utils/platform';
import { invoke } from '@tauri-apps/api/core';
import { listen } from '@tauri-apps/api/event';
import { authStore } from '$lib/stores/auth';
import { favorites } from '$lib/stores/library/favorites';
import { exclusiveMode, deviceId, activeDeviceId, activeDevices, setPlayerRef, sendHeartbeat, refreshDevices as refreshDevicesFromSession } from './deviceSession';
import { devices as devicesApi } from '$lib/api/devices';
import { selectedAudioOutputId, sinkIdSupported } from './casting';
import { crossfadeEnabled, crossfadeSecs, gaplessEnabled } from '$lib/stores/settings/crossfade';

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
/** When true, display current track in Discord Rich Presence (desktop only). */
export const discordEnabled = writable(false);
/** When true, normalize track loudness using ReplayGain metadata. */
export const replayGainEnabled = writable(false);

/** Tracks played in this session; used by smart shuffle to de-prioritise repeats. */
const recentlyPlayedIds = new Set<string>();

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
	// Group indices by artist (fall back to artist_name or a shared sentinel).
	const groupMap = new Map<string, number[]>();
	tracks.forEach((t, i) => {
		const key = t.artist_id ?? t.artist_name ?? '__unknown__';
		if (!groupMap.has(key)) groupMap.set(key, []);
		groupMap.get(key)!.push(i);
	});

	// Shuffle within each group and randomise the group order.
	const groups = fisherYates([...groupMap.values()].map((idxs) => fisherYates([...idxs])));

	// Sort groups by size descending so large artists spread through the whole list.
	groups.sort((a, b) => b.length - a.length);

	// Round-robin interleave: one track per group per round.
	const result: number[] = [];
	const maxLen = Math.max(...groups.map((g) => g.length), 0);
	for (let round = 0; round < maxLen; round++) {
		for (const group of groups) {
			if (round < group.length) result.push(group[round]);
		}
	}

	// De-prioritise recently played tracks by moving them toward the end.
	if (recentlyPlayedIds.size > 0) {
		const notRecent = result.filter((i) => !recentlyPlayedIds.has(tracks[i].id));
		const recent    = result.filter((i) =>  recentlyPlayedIds.has(tracks[i].id));
		result.length = 0;
		result.push(...notRecent, ...recent);
	}

	// Pin the selected track to position 0.
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
// When running on Android inside Tauri, audio is handled by ExoPlayer via JNI.
// The browser audio engine is bypassed entirely.

const isAndroidNative = typeof window !== 'undefined' && nativePlatform() === 'android';

/** Position polling timer for native Android playback. */
let nativePositionTimer: ReturnType<typeof setInterval> | null = null;

function startNativePositionPolling() {
	stopNativePositionPolling();
	nativePositionTimer = setInterval(async () => {
		try {
			const pos = await invoke<number>('get_position_music');
			const dur = await invoke<number>('get_duration_music');
			positionMs.set(pos);
			if (dur > 0) durationMs.set(dur);
		} catch { /* ignore */ }
	}, 250);
}

function stopNativePositionPolling() {
	if (nativePositionTimer) {
		clearInterval(nativePositionTimer);
		nativePositionTimer = null;
	}
}

/** Build a stream URL for native ExoPlayer with auth token as query param. */
function buildNativeStreamUrl(trackId: string): string {
	const base = getApiBase();
	const token = get(authStore).token ?? '';
	return `${base}/stream/${trackId}?token=${encodeURIComponent(token)}`;
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

// ── Crossfade / gapless helpers ──────────────────────────────────────────────

/**
 * Peek at the track that would play next without modifying any state.
 * Returns null when the next track can't be determined (e.g. end of queue
 * with autoplay needing an async API call).
 */
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

/**
 * Advance the queue state (userQueue / queueIndex) by one slot without
 * starting audio. Mirrors the logic in next() but skips playback.
 */
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

/**
 * Preload the next track and schedule a crossfade or gapless transition.
 * Only applies to the WASM (24-bit+) path. Best-effort: if the preload
 * doesn't finish in time, normal sequential playback happens instead.
 */
async function setupCrossfade(track: Track): Promise<void> {
	if (!get(crossfadeEnabled) && !get(gaplessEnabled)) return;
	if (!audioEngine.isWasmActive) return;
	// Both the current and next track must be hi-res for sample-accurate transition.
	if ((track.bit_depth ?? 16) <= 16) return;

	const nextTrack = peekNext();
	if (!nextTrack) return;
	if ((nextTrack.bit_depth ?? 16) <= 16) return;

	const fadeSecs = get(crossfadeEnabled) ? get(crossfadeSecs) : 0;

	// Preload next track's audio in the background.
	await audioEngine.preloadNext(nextTrack.id, nextTrack.sample_rate);

	// Wait for the current track's full buffer to be ready (WASM quick-start
	// may still be decoding), then schedule the transition.
	audioEngine.onFullBufferForCrossfade(() => {
		audioEngine.scheduleCrossfade(fadeSecs, () => {
			// ── Transition: audio is now playing the next track ──
			currentTrack.set(nextTrack);
			durationMs.set(nextTrack.duration_ms);
			positionMs.set(0);
			playbackState.set('playing');

			// Apply replay gain for the incoming track.
			const rgDb = get(replayGainEnabled) ? (nextTrack.replay_gain_track ?? 0) : 0;
			audioEngine.setReplayGainDb(rgDb);

			// Advance queue pointers to match.
			advanceQueueState();

			// Record play and sync state to other devices/services.
			libraryApi.recordPlay(nextTrack.id, 0).catch(() => {});
			sendHeartbeat().catch(() => {});

			// Chain: set up the next-next crossfade.
			setupCrossfade(nextTrack).catch(() => {});
		});
	});
}

export async function playTrack(track: Track, trackList?: Track[], startSeconds = 0) {
	// Control-device mode: delegate playback to the active device.
	// The queue is embedded directly in the play command (atomic queue + play)
	// so there is no race condition with a separate queue write.
	const activeDev = get(activeDeviceId);
	if (get(exclusiveMode) && activeDev && activeDev !== deviceId) {
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
	// Stop any in-progress shadow tick so it doesn't compete with real audio.
	stopShadowTick();
	currentTrack.set(track);
	playbackState.set('loading');
	// Immediately reset the seek bar so it doesn't linger at the previous
	// track's position while the new track loads.
	positionMs.set(startSeconds * 1000);
	durationMs.set(track.duration_ms);
	try {
		if (isAndroidNative) {
			// Native Android path: ExoPlayer handles playback via JNI bridge.
			const streamUrl = buildNativeStreamUrl(track.id);
			const coverUrl = track.album_id ? `${getApiBase()}/covers/${track.album_id}?token=${encodeURIComponent(get(authStore).token ?? '')}` : undefined;
			await invoke('play_music', {
				url: streamUrl,
				title: track.title,
				artist: track.artist_name ?? undefined,
				coverUrl
			});
			if (startSeconds > 0) {
				await invoke('seek_music', { positionMs: Math.round(startSeconds * 1000) });
			}
			startNativePositionPolling();
		} else {
			// Browser audio engine path (desktop + web).
			const rgDb = get(replayGainEnabled) ? (track.replay_gain_track ?? 0) : 0;
			audioEngine.setReplayGainDb(rgDb);
			if (sinkIdSupported) {
				const sinkId = get(selectedAudioOutputId);
				if (sinkId && sinkId !== 'default') {
					audioEngine.setAudioOutput(sinkId).catch(() => {});
				}
			}
			await audioEngine.play(track.id, track.bit_depth ?? 16, track.sample_rate, startSeconds);
			// Preload the next track and schedule crossfade / gapless if enabled.
			setupCrossfade(track).catch(() => {});
		}
		_isRemoteMirror = false;
		playbackState.set('playing');
		if (get(exclusiveMode) && deviceId) {
			devicesApi.activate(deviceId).catch(() => {});
		}
		sendHeartbeat().catch(() => {});
		recentlyPlayedIds.add(track.id);
		libraryApi.recordPlay(track.id, 0).catch(() => {});
	} catch (err) {
		console.error('playTrack error', err);
		playbackState.set('idle');
	}
}

/**
 * Pause local audio without delegating to remote devices.
 * Used by the device session `pause_others` handler to stop this device
 * before the active-device pointer has been updated, preventing the
 * toggle from being misdirected to the newly-active remote device.
 */
export function pauseLocal() {
	_isRemoteMirror = false;
	if (isAndroidNative) {
		invoke('pause_music').catch(() => {});
		stopNativePositionPolling();
	} else if (audioEngine.isLoaded) {
		audioEngine.stop();
	}
	const state = get(playbackState);
	if (state === 'playing' || state === 'loading') {
		playbackState.set('paused');
	}
}

export function togglePlayPause() {
	// Control-device mode: delegate to active device (only in exclusive mode).
	const activeDev = get(activeDeviceId);
	if (get(exclusiveMode) && activeDev && activeDev !== deviceId) {
		devicesApi.controlCommand(activeDev, 'toggle').catch(() => {});
		return;
	}
	const state = get(playbackState);
	if (state === 'playing') {
		if (isAndroidNative) {
			invoke('pause_music').catch(() => {});
			stopNativePositionPolling();
		} else {
			audioEngine.pause();
		}
		playbackState.set('paused');
		sendHeartbeat().catch(() => {});
	} else if (state === 'paused') {
		if (isAndroidNative) {
			// On Android, resume the paused ExoPlayer instance.
			invoke('resume_music').catch(() => {});
			startNativePositionPolling();
			playbackState.set('playing');
			sendHeartbeat().catch(() => {});
		} else if (!audioEngine.isLoaded) {
			// Restore scenario: nothing is loaded in the engine yet (e.g. after page
			// refresh). Load the track starting from the saved position.
			const track = get(currentTrack);
			if (track) {
				playTrack(track, undefined, get(positionMs) / 1000);
			}
		} else {
			_isRemoteMirror = false;
			audioEngine.resume();
			playbackState.set('playing');
			sendHeartbeat().catch(() => {});
		}
	}
}

export function seek(posSeconds: number) {
	// Control-device mode: delegate seek to the active device (only in exclusive mode).
	const activeDev = get(activeDeviceId);
	if (get(exclusiveMode) && activeDev && activeDev !== deviceId) {
		const posMs = Math.round(posSeconds * 1000);
		devicesApi.controlCommand(activeDev, 'seek', { position_ms: posMs }).catch(() => {});
		// Optimistically update local state so the seek bar reflects the new
		// position immediately, rather than snapping back until the next heartbeat.
		positionMs.set(posMs);
		_shadowEpochMs = Date.now() - posMs;
		return;
	}
	const dur = get(durationMs) || 0;
	const maxSec = Math.max(0, dur / 1000);
	const clamped = Math.max(0, Math.min(posSeconds, Math.max(0, maxSec - 0.01)));
	if (isAndroidNative) {
		invoke('seek_music', { positionMs: Math.round(clamped * 1000) }).catch(() => {});
	} else {
		audioEngine.seek(clamped);
	}
	positionMs.set(clamped * 1000);
	sendHeartbeat().catch(() => {});
	// Re-schedule crossfade with updated timing after seek.
	const seekTrack = get(currentTrack);
	if (seekTrack) setupCrossfade(seekTrack).catch(() => {});
}

export function setVolume(gain: number) {
	// Control-device mode: delegate volume to the active device (only in exclusive mode).
	const activeDev = get(activeDeviceId);
	if (get(exclusiveMode) && activeDev && activeDev !== deviceId) {
		devicesApi.controlCommand(activeDev, 'volume', { volume: gain }).catch(() => {});
		return;
	}
	volume.set(gain);
	audioEngine.setVolume(gain);

	// On Android, sync to system music volume
	if (nativePlatform() === 'android') {
		invoke('set_volume', { volume: gain }).catch(() => {});
	}

	sendHeartbeat().catch(() => {});
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
	// Control-device mode: delegate to active device (only in exclusive mode).
	const activeDev = get(activeDeviceId);
	if (get(exclusiveMode) && activeDev && activeDev !== deviceId) {
		devicesApi.controlCommand(activeDev, 'next').catch(() => {});
		return;
	}
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
			shuffleOrder.set(buildShuffleOrder(q));
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
	// Control-device mode: delegate to active device (only in exclusive mode).
	const activeDev = get(activeDeviceId);
	if (get(exclusiveMode) && activeDev && activeDev !== deviceId) {
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
			// Turning on: pin the current track to position 0 in the shuffle order.
			const order = buildShuffleOrder(q, idx);
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
 * Transfer playback from this device to another (or pull from remote to self).
 * Shared by BottomBar and MobilePlayer to avoid duplicated transfer logic.
 */
export async function transferPlayback(targetId: string) {
	if (targetId === deviceId) {
		// Pull playback to this device from whoever is currently active.
		const devices = get(activeDevices);
		const active = devices.find((d) => d.is_active && d.id !== deviceId);

		// Claim this device as active both locally AND on the server BEFORE
		// starting playback.  receivePlayCommand → playTrack → sendHeartbeat
		// triggers refreshDevices(); if the server still reports the old device
		// as active, refreshDevices would reset activeDeviceId and all future
		// play commands would delegate back to the old device.
		activeDeviceId.set(deviceId);
		await devicesApi.activate(deviceId).catch(() => {});

		if (active?.state?.track_id) {
			await receivePlayCommand(active.state.track_id, active.state.position_ms ?? 0);
		}
		return;
	}

	// Snapshot state BEFORE any async work — prevents races with SSE updates.
	const track = get(currentTrack);
	const pos   = Math.round(get(positionMs));
	const q     = get(queue);
	const wasPlaying = get(playbackState) === 'playing';

	// Stop local audio immediately — bypass togglePlayPause() which would
	// delegate to the (now-changing) active device and cause a race.
	if (wasPlaying && audioEngine.isLoaded) {
		audioEngine.pause();
		playbackState.set('paused');
	}

	// Send an atomic play command with the full queue embedded.
	// The backend writes the queue to Redis AND sets the active device pointer
	// atomically, so no separate queueApi.replace() or activate() is needed.
	if (track) {
		await devicesApi.playCommand(
			targetId,
			track.id,
			pos,
			q.length > 0 ? q : undefined
		).catch((err) => console.error('transferPlayback error', err));
	}
}

/**
 * Receive a play_command from another device.
 * Uses the embedded queue from the SSE payload when available;
 * falls back to loading from the server otherwise.
 */
export async function receivePlayCommand(trackId: string, posMs: number, embeddedQueue?: Track[]) {
	let trackList: Track[];
	if (embeddedQueue && embeddedQueue.length > 0) {
		trackList = embeddedQueue;
	} else {
		await loadQueue();
		trackList = get(queue);
	}
	// Find the track in the embedded queue to avoid a separate API call.
	let track = trackList.find((t) => t.id === trackId);
	if (!track) {
		const res = await libraryApi.track(trackId).catch(() => null);
		if (!res?.track) return;
		track = res.track;
	}
	await playTrack(track, trackList.length > 0 ? trackList : undefined, posMs / 1000);
}

// ── Shadow tick ──────────────────────────────────────────────────────────────
// When this device is not the active player, we mirror the active device's
// progress locally. The server only sends state snapshots every ~30 s (via
// heartbeat), so we advance positionMs locally at 250 ms intervals between
// those snapshots.

const SHADOW_TICK_MS = 250;
let shadowTickTimer: ReturnType<typeof setInterval> | null = null;
// Unix-ms timestamp at which the remote track's position was 0.
// Set whenever syncVisibleState is called; used by the shadow tick to compute
// the exact current position as Date.now() - _shadowEpochMs (no timer drift).
let _shadowEpochMs = 0;

/**
 * True when the current playbackState='playing' was set by syncVisibleState
 * (mirroring a remote device) rather than by actual local audio playback.
 * Used by the visibilitychange handler to avoid auto-resuming a stale
 * AudioContext when returning to the tab.
 */
let _isRemoteMirror = false;

export function stopShadowTick() {
	if (shadowTickTimer) {
		clearInterval(shadowTickTimer);
		shadowTickTimer = null;
	}
}

function startShadowTick() {
	stopShadowTick();
	shadowTickTimer = setInterval(() => {
		// Bail if this device has taken over real playback (audio engine loaded
		// locally). On control-only devices playbackState can be 'playing' to
		// mirror the remote device — the shadow tick must keep running.
		if (get(playbackState) === 'playing' && audioEngine.isLoaded) {
			stopShadowTick();
			return;
		}
		// Self-terminate when playback is no longer 'playing' (e.g. pauseLocal
		// was called because the remote active device disappeared).
		if (get(playbackState) !== 'playing') {
			stopShadowTick();
			return;
		}
		const dur = get(durationMs);
		// Compute position from epoch so the bar advances without any drift,
		// regardless of how often the active device sends heartbeats.
		const computed = _shadowEpochMs > 0
			? Date.now() - _shadowEpochMs
			: get(positionMs) + SHADOW_TICK_MS;
		positionMs.set(dur > 0 && computed >= dur ? dur : computed);
	}, SHADOW_TICK_MS);
}

/**
 * Mirror the active device's visible track / position / volume on this device
 * without starting audio. Called every time a 'state' SSE event arrives from
 * the active device. When playing=true, a local ticker advances positionMs
 * between server snapshots so the progress bar updates smoothly.
 */
export async function syncVisibleState(
	trackId: string,
	posMs: number,
	playing = true,
	/** Server-computed epoch (unix ms). When provided, the shadow tick derives
	 *  position as Date.now()-epochMs instead of incrementing, eliminating drift.
	 *  The client re-anchors locally anyway to cancel server/client clock skew. */
	epochMs?: number,
	/** Remote device's current volume level (0.0–1.0). */
	remoteVolume?: number
) {
	// Never interrupt a device that is actively streaming audio locally.
	// On control-only devices the audio engine is not loaded, so this guard
	// passes through even when playbackState is 'playing' (mirroring remote).
	if (get(playbackState) === 'playing' && audioEngine.isLoaded) return;

	// Mirror the active device's volume so the slider reflects the remote level.
	if (remoteVolume !== undefined) {
		volume.set(remoteVolume);
	}

	// Re-anchor epoch locally — always use client clock to cancel server↔client
	// clock skew. Any epoch the server provided is only used for documentation;
	// the client derives it the same way for perfect per-tick accuracy.
	if (playing) {
		_shadowEpochMs = Date.now() - posMs;
	}

	const existing = get(currentTrack);
	if (existing?.id === trackId) {
		// Same track — re-anchor position from server snapshot, then tick.
		positionMs.set(posMs);
		_isRemoteMirror = playing;
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
		_isRemoteMirror = playing;
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
			replayGain: get(replayGainEnabled),
			smartShuffle: get(smartShuffleEnabled)
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
smartShuffleEnabled.subscribe(() => scheduleStateSave());
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
	// On Android native, Media3 manages its own MediaSession — skip the browser one.
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
	// During loading, leave the previous playbackState unchanged — resetting
	// it to 'none' causes Chrome to tear down the media notification and
	// rebuild it, which makes the notification flicker or disappear entirely.
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

if (mediaSessionSupported()) {
	navigator.mediaSession.setActionHandler('play', () => togglePlayPause());
	navigator.mediaSession.setActionHandler('pause', () => togglePlayPause());
	navigator.mediaSession.setActionHandler('stop', () => {
		audioEngine.stop();
		positionMs.set(0);
		playbackState.set('paused');
	});
	navigator.mediaSession.setActionHandler('previoustrack', () => { previous(); });
	navigator.mediaSession.setActionHandler('nexttrack', () => { next(); });
	navigator.mediaSession.setActionHandler('seekto', (details) => {
		if (details.seekTime !== undefined) seek(details.seekTime);
	});
	navigator.mediaSession.setActionHandler('seekbackward', (details) => {
		seek(Math.max(0, get(positionMs) / 1000 - (details.seekOffset ?? 10)));
	});
	navigator.mediaSession.setActionHandler('seekforward', (details) => {
		seek(Math.min(get(durationMs) / 1000, get(positionMs) / 1000 + (details.seekOffset ?? 10)));
	});
}

currentTrack.subscribe(syncMediaMetadata);
playbackState.subscribe(syncMediaSessionPlaybackState);

// When the user returns to Chrome from another app, Android may have
// auto-suspended AudioContexts (including the one capturing the native
// <audio> element via createMediaElementSource), silencing playback and
// removing the media notification. Resuming on visibility restores both.
// IMPORTANT: Only resume when the player is supposed to be playing AND has
// local audio loaded. The WASM path pauses by suspending its AudioContext,
// so resuming unconditionally would un-pause audio while the UI still shows
// "paused" — causing an audio/UI desync.
if (typeof document !== 'undefined') {
	document.addEventListener('visibilitychange', () => {
		if (document.visibilityState !== 'visible') return;

		// Eagerly refresh the device list to catch any events we missed while
		// backgrounded (e.g. pause_others, device un-registration).
		refreshDevicesFromSession().catch(() => {});

		// Only resume audio contexts if this device was actually playing
		// locally — NOT when we're just mirroring a remote device's state.
		// Without this guard, returning to a tab that was shadow-mirroring a
		// remote device could resume a stale AudioContext, causing phantom
		// audio with the UI stuck on "paused".
		if (
			get(playbackState) === 'playing' &&
			audioEngine.isLoaded &&
			!_isRemoteMirror
		) {
			audioEngine.resumeAllContexts();
		}
	});
}

// ─── Discord Rich Presence ───────────────────────────────────────────────────
// Only active in the Tauri desktop shell. Updates presence when the track
// changes or Discord is toggled on; clears it when toggled off.
if (isTauri() && !isAndroidNative) {
	function pushDiscordPresence(track: Track | null) {
		if (!get(discordEnabled) || !track) return;
		invoke('discord_update', { title: track.title, artist: track.artist_name ?? '', album: '' }).catch(() => {});
	}

	function clearDiscordPresence() {
		invoke('discord_clear').catch(() => {});
	}

	currentTrack.subscribe((track) => {
		if (get(discordEnabled)) {
			if (track) pushDiscordPresence(track);
			else clearDiscordPresence();
		}
	});

	discordEnabled.subscribe(async (enabled) => {
		if (!enabled) {
			clearDiscordPresence();
		} else {
			const track = get(currentTrack);
			if (track) pushDiscordPresence(track);
		}
	});
}

// ─── System Tray Integration ─────────────────────────────────────────────────
// Syncs playback state to the tray Play/Pause label and handles tray menu
// button clicks (Previous / Play-Pause / Next) via Tauri events.
// Desktop only — tray and Discord commands don't exist on Android.
if (isTauri() && !isAndroidNative) {
	playbackState.subscribe((state) => {
		invoke('set_tray_playback_state', { playing: state === 'playing' }).catch(() => {});
	});

	// Listen for tray menu button events from the Tauri main process.
	listen('tray-play-pause', () => togglePlayPause()).catch(() => {});
	listen('tray-previous', () => previous()).catch(() => {});
	listen('tray-next', () => next()).catch(() => {});
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
		let vol = typeof st.volume === 'number' ? st.volume : 1;
		// On Android, use the system music volume instead of the saved value
		if (nativePlatform() === 'android') {
			try {
				vol = await invoke<number>('get_volume');
			} catch { /* use saved value */ }
		}
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
		if (st.smartShuffle === true) smartShuffleEnabled.set(true);

		if (Array.isArray(st.queue) && st.queue.length) {
			// New format: full track objects stored directly — no API calls needed.
			queue.set(st.queue as Track[]);
		} else if (Array.isArray(st.queueIds) && st.queueIds.length) {
			// Legacy format: only IDs were saved — batch-fetch in one request.
			const res = await libraryApi.tracksBatch(st.queueIds).catch(() => null);
			const qTracks = (Array.isArray(res) ? res : []) as Track[];
			if (qTracks.length) queue.set(qTracks);
		}

		// On Android, ExoPlayer may still be playing in the foreground service
		// after the WebView was destroyed and recreated. Query the actual state
		// so the play/pause button matches reality.
		if (isAndroidNative) {
			try {
				const playing = await invoke<boolean>('get_is_playing');
				if (playing) {
					playbackState.set('playing');
					startNativePositionPolling();
				} else {
					playbackState.set('paused');
				}
			} catch {
				playbackState.set('paused');
			}
		} else {
			// Non-Android: leave as paused. Pressing play will reload the track.
			playbackState.set('paused');
		}
	} catch {
		// ignore parse / storage errors
	} finally {
		saveEnabled = true;
	}
})();

// ─── Android Native Event Bridge ──────────────────────────────────────────────
// Listen for events emitted by the Kotlin MediaService via JNI → Rust → Tauri.
// These handle notification button presses (next, previous, shuffle, favorite).

if (isAndroidNative) {
	listen('native-next', () => { next(); }).catch(() => {});
	listen('native-previous', () => { previous(); }).catch(() => {});
	listen('native-shuffle-toggle', () => { toggleShuffle(); }).catch(() => {});
	listen('native-favorite-toggle', () => {
		const track = get(currentTrack);
		if (track) favorites.toggle(track.id, track).catch(() => {});
	}).catch(() => {});

	// Sync shuffle state to the native notification icon whenever it changes.
	shuffle.subscribe((sh) => {
		invoke('set_shuffle_state', { shuffled: sh }).catch(() => {});
	});

	// Sync volume slider when hardware volume buttons change the system volume.
	listen<number>('native-volume-change', (event) => {
		volume.set(event.payload);
		audioEngine.setVolume(event.payload);
	}).catch(() => {});

	// Sync favorite state to the native notification icon when the track or
	// favorites set changes.
	const syncNativeFavorite = () => {
		const track = get(currentTrack);
		if (!track) return;
		const isFav = get(favorites).has(track.id);
		invoke('set_favorite_state', { favorited: isFav }).catch(() => {});
	};
	currentTrack.subscribe(syncNativeFavorite);
	favorites.subscribe(syncNativeFavorite);
}

// ─── Device Session wiring ────────────────────────────────────────────────────
// Inject player references into the device session so it can react to
// cross-device events (e.g. pause this device when another takes over).
if (typeof document !== 'undefined') {
	setPlayerRef({
		playbackState,
		currentTrack,
		positionMs,
		volume,
		togglePlayPause,
		pauseLocal,
		next,
		previous,
		seek,
		setVolume,
		playTrack,
		receivePlayCommand,
		syncVisibleState,
		stopShadowTick,
	});
}
