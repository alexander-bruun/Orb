/**
 * Unified Transport Engine
 *
 * Single owner of playback state, native bridge communication, position
 * polling, shadow tick, and mode switching. Music and audiobook become
 * "content providers" that configure the engine rather than independent
 * state machines.
 *
 * See docs/unified-playback-engine.md for the full design.
 */

import { writable, derived, get, type Readable } from 'svelte/store';
import { browser } from '$app/environment';
import { isTauri, isNative, nativePlatform } from '$lib/utils/platform';
import { audioEngine } from '$lib/audio/engine';
import { getApiBase } from '$lib/api/base';
import { authStore } from '$lib/stores/auth';
import type { DeviceState } from '$lib/api/devices';
import { TIMINGS } from '$lib/constants';

// ── Types ────────────────────────────────────────────────────────────────────

export type PlaybackMode = 'music' | 'audiobook' | 'podcast';
export type EnginePlaybackState = 'idle' | 'loading' | 'playing' | 'paused';

export interface ContentMetadata {
	id: string;
	title: string;
	artist?: string;
	album?: string;
	coverUrl?: string;
	durationMs: number;
}

/** Payload for remote control commands routed through the engine. */
export interface ControlPayload {
	position_ms?: number;
	volume?: number;
	speed?: number;
	[key: string]: unknown;
}

/**
 * Content providers implement this interface and register with the engine.
 * The engine calls these callbacks to delegate content-specific decisions.
 */
export interface ContentProvider {
	/** Called when the current audio reaches the end. */
	onTrackEnd(): void;
	/** Called every ~250ms with the current position. */
	onPositionUpdate(ms: number): void;
	/** Called when this provider's mode becomes active. */
	onModeActivated(): void;
	/** Called when the engine switches away from this provider's mode. */
	onModeDeactivated(): void;
	/** Handle a remote control command. */
	onControlCommand?(action: string, payload: ControlPayload): void;
	/** Mirror state from a remote device. */
	onRemoteSync?(state: RemoteState): void;
}

/** Extended provider for music mode — includes music-specific native callbacks. */
export interface MusicContentProvider extends ContentProvider {
	onPrevious?(): void;
	onShuffleToggle?(): void;
	onFavoriteToggle?(): void;
	onPlayCommand?(trackId: string, posMs: number, queue?: unknown[]): void;
}

/** Extended provider for audiobook mode — includes audiobook-specific native callbacks. */
export interface AudiobookContentProvider extends ContentProvider {
	onSkipForward?(secs: number): void;
	onSkipBackward?(secs: number): void;
	onSpeedCycle?(): void;
	onJumpToChapterStart?(): void;
}

/** Extended provider for podcast mode — includes podcast-specific native callbacks. */
export interface PodcastContentProvider extends ContentProvider {
	onSkipForward?(secs: number): void;
	onSkipBackward?(secs: number): void;
	onSpeedCycle?(): void;
}

// ── Engine-owned stores (read-only for consumers) ────────────────────────────

const _mode = writable<PlaybackMode>('music');
const _playbackState = writable<EnginePlaybackState>('idle');
const _positionMs = writable(0);
const _durationMs = writable(0);
const _volume = writable(1);
const _speed = writable(1.0);
const _currentContent = writable<ContentMetadata | null>(null);
/** Percentage (0–100) of buffered content. */
const _bufferedPct = writable(0);

// Public read-only views
export const mode: Readable<PlaybackMode> = { subscribe: _mode.subscribe };
export const enginePlaybackState: Readable<EnginePlaybackState> = { subscribe: _playbackState.subscribe };
export const enginePositionMs: Readable<number> = { subscribe: _positionMs.subscribe };
export const engineDurationMs: Readable<number> = { subscribe: _durationMs.subscribe };
export const engineVolume: Readable<number> = { subscribe: _volume.subscribe };
export const engineSpeed: Readable<number> = { subscribe: _speed.subscribe };
export const engineCurrentContent: Readable<ContentMetadata | null> = { subscribe: _currentContent.subscribe };
export const engineBufferedPct: Readable<number> = { subscribe: _bufferedPct.subscribe };

// ── Writable accessors (for migration — providers use these during transition) ──
// These will be removed once providers fully delegate to engine transport commands.
export const _writableMode = _mode;
/** Convenience alias so UI components can import { activePlayer } from engine. */
export { _mode as activePlayer };
export const _writablePlaybackState = _playbackState;
export const _writablePositionMs = _positionMs;
export const _writableDurationMs = _durationMs;
export const _writableVolume = _volume;
export const _writableSpeed = _speed;
export const _writableCurrentContent = _currentContent;
export const _writableBufferedPct = _bufferedPct;

// ── Content provider registry ────────────────────────────────────────────────

type AnyContentProvider = ContentProvider | MusicContentProvider | AudiobookContentProvider | PodcastContentProvider;
const providers = new Map<PlaybackMode, AnyContentProvider>();

export function registerProvider(forMode: 'music', provider: MusicContentProvider): void;
export function registerProvider(forMode: 'audiobook', provider: AudiobookContentProvider): void;
export function registerProvider(forMode: 'podcast', provider: PodcastContentProvider): void;
export function registerProvider(forMode: PlaybackMode, provider: AnyContentProvider): void;
export function registerProvider(forMode: PlaybackMode, provider: AnyContentProvider) {
	providers.set(forMode, provider);
}

function getMusicProvider(): MusicContentProvider | undefined {
	return providers.get('music') as MusicContentProvider | undefined;
}

function getAudiobookProvider(): AudiobookContentProvider | undefined {
	return providers.get('audiobook') as AudiobookContentProvider | undefined;
}

function getPodcastProvider(): PodcastContentProvider | undefined {
	return providers.get('podcast') as PodcastContentProvider | undefined;
}

function activeProvider(): ContentProvider | undefined {
	return providers.get(get(_mode));
}

// ── Native bridge state ──────────────────────────────────────────────────────

const isAndroidNative = browser && nativePlatform() === 'android';

let nativePositionTimer: ReturnType<typeof setInterval> | null = null;
let nativePlayerReady = false;
let nativeListenersInit = false;

// Seek guard: prevent position polling from overwriting a recent seek.
let lastSeekTime = 0;
const SEEK_GUARD_MS = TIMINGS.SEEK_GUARD;

// ── Shadow tick (idle-device mirroring) ──────────────────────────────────────
// When this device mirrors a remote active device, the shadow tick advances
// positionMs between server heartbeats (30s gap) for smooth progress bars.

const SHADOW_TICK_MS = TIMINGS.POSITION_TICK;
let shadowTickTimer: ReturnType<typeof setInterval> | null = null;
let shadowEpochMs = 0;
let shadowSpeed = 1.0;

/**
 * True when the current 'playing' state was set by syncRemoteState (mirroring
 * a remote device) rather than by actual local audio. Used by visibility
 * handlers to avoid resuming stale AudioContexts.
 */
let isRemoteMirror = false;

// ── Position polling (native Android) ────────────────────────────────────────

function startNativePositionPolling() {
	stopNativePositionPolling();
	nativePositionTimer = setInterval(async () => {
		try {
			const { invoke } = await import('@tauri-apps/api/core');
			const now = Date.now();

			// Within SEEK_GUARD_MS of a seek, trust the seek position.
			if (now - lastSeekTime < SEEK_GUARD_MS) return;

			const pos = await invoke<number>('get_position_music');
			const dur = await invoke<number>('get_duration_music');
			_positionMs.set(pos);
			if (dur > 0) _durationMs.set(dur);

			// Notify the active content provider.
			activeProvider()?.onPositionUpdate(pos);
		} catch { /* ignore */ }
	}, TIMINGS.POSITION_TICK);
}

function stopNativePositionPolling() {
	if (nativePositionTimer) {
		clearInterval(nativePositionTimer);
		nativePositionTimer = null;
	}
}

// ── Shadow tick implementation ───────────────────────────────────────────────

export function startShadowTick(posMs?: number, speed?: number) {
	stopShadowTick();
	if (posMs !== undefined) {
		shadowEpochMs = Date.now() - posMs;
	}
	if (speed !== undefined) {
		shadowSpeed = speed;
	}
	shadowTickTimer = setInterval(() => {
		// Self-terminate if real local audio is loaded (no longer mirroring).
		if (get(_playbackState) === 'playing' && audioEngine.isLoaded && !isRemoteMirror) {
			stopShadowTick();
			return;
		}
		// Self-terminate when no longer playing.
		if (get(_playbackState) !== 'playing') {
			stopShadowTick();
			return;
		}
		const dur = get(_durationMs);
		const computed = shadowEpochMs > 0
			? Math.round((Date.now() - shadowEpochMs) * shadowSpeed)
			: get(_positionMs) + Math.round(SHADOW_TICK_MS * shadowSpeed);
		_positionMs.set(dur > 0 && computed >= dur ? dur : computed);

		// Notify the active content provider.
		activeProvider()?.onPositionUpdate(get(_positionMs));
	}, SHADOW_TICK_MS);
}

export function stopShadowTick() {
	if (shadowTickTimer) {
		clearInterval(shadowTickTimer);
		shadowTickTimer = null;
	}
}

// ── Native event listeners (one-time init) ───────────────────────────────────

async function initNativeListeners() {
	if (nativeListenersInit || !isNative()) return;
	nativeListenersInit = true;

	const { listen } = await import('@tauri-apps/api/event');

	// Shared play/pause events — route to whichever mode is active.
	listen<void>('native-pause', () => {
		_playbackState.set('paused');
		stopNativePositionPolling();
	});
	listen<void>('native-play', () => {
		_playbackState.set('playing');
		if (get(_mode) !== 'audiobook') startNativePositionPolling();
	});

	// Music-mode notification actions
	listen<void>('native-next', () => {
		if (get(_mode) === 'music') activeProvider()?.onTrackEnd();
	});
	listen<void>('native-previous', () => {
		// Handled by music provider via registered callback
		getMusicProvider()?.onPrevious?.();
	});
	listen<void>('native-shuffle-toggle', () => {
		getMusicProvider()?.onShuffleToggle?.();
	});
	listen<void>('native-favorite-toggle', () => {
		getMusicProvider()?.onFavoriteToggle?.();
	});

	// Audiobook-mode notification actions
	listen<void>('native-ab-skip-back-15', () => {
		getAudiobookProvider()?.onSkipBackward?.(15);
	});
	listen<void>('native-ab-skip-forward-15', () => {
		getAudiobookProvider()?.onSkipForward?.(15);
	});
	listen<void>('native-ab-speed-cycle', () => {
		getAudiobookProvider()?.onSpeedCycle?.();
	});
	listen<void>('native-ab-chapter-start', () => {
		getAudiobookProvider()?.onJumpToChapterStart?.();
	});

	// Podcast-mode notification actions
	listen<void>('native-pod-skip-back-15', () => {
		getPodcastProvider()?.onSkipBackward?.(15);
	});
	listen<void>('native-pod-skip-forward-30', () => {
		getPodcastProvider()?.onSkipForward?.(30);
	});
	listen<void>('native-pod-speed-cycle', () => {
		getPodcastProvider()?.onSpeedCycle?.();
	});

	// Volume changes from hardware buttons
	listen<number>('native-volume-change', (event) => {
		_volume.set(event.payload);
	});
}

// ── Transport commands ───────────────────────────────────────────────────────

/**
 * Build a stream URL for native ExoPlayer. Uses offline file:// when available.
 */
async function buildNativeStreamUrl(trackId: string): Promise<string> {
	const base = getApiBase();
	const token = get(authStore).token ?? '';
	try {
		const { invoke } = await import('@tauri-apps/api/core');
		const path = await invoke<string | null>('get_offline_file_path', { trackId });
		if (path) return `file://${path}`;
	} catch { /* fall through */ }
	return `${base}/stream/${trackId}?token=${encodeURIComponent(token)}`;
}

export interface PlayOptions {
	startMs?: number;
	speed?: number;
	isAudiobook?: boolean;
	/** For native: pre-built stream URL (audiobook chapters use custom URLs). */
	nativeUrl?: string;
}

/**
 * Play audio through the engine. This is the single entry point for starting
 * audio — both music and audiobook providers call this.
 */
export async function play(url: string, meta: ContentMetadata, opts: PlayOptions = {}): Promise<void> {
	stopShadowTick();
	isRemoteMirror = false;

	_currentContent.set(meta);
	_durationMs.set(meta.durationMs);
	_positionMs.set(opts.startMs ?? 0);
	_playbackState.set('loading');

	const isAudiobook = opts.isAudiobook ?? (get(_mode) === 'audiobook');

	if (isAndroidNative) {
		await initNativeListeners();
		const { invoke } = await import('@tauri-apps/api/core');

		// Sequence: pause → set mode → play. Prevents ExoPlayer contamination.
		try {
			await invoke('set_audiobook_mode', { isAudiobook });
		} catch { /* ignore */ }

		if (opts.speed && opts.speed !== 1.0) {
			try { await invoke('set_playback_speed', { speed: opts.speed }); } catch { /* ignore */ }
		}

		const streamUrl = opts.nativeUrl ?? url;
		const coverUrl = meta.coverUrl;
		try {
			await invoke('play_music', {
				url: streamUrl,
				title: meta.title,
				artist: meta.artist ?? undefined,
				coverUrl,
			});
		} catch (e) {
			console.error('[engine] native play failed:', e);
			_playbackState.set('idle');
			return;
		}

		if (opts.startMs && opts.startMs > 0) {
			try { await invoke('seek_music', { positionMs: opts.startMs }); } catch { /* ignore */ }
		}

		nativePlayerReady = true;
		_playbackState.set('playing');
		if (opts.speed) _speed.set(opts.speed);
		if (get(_mode) !== 'audiobook') startNativePositionPolling();
	} else {
		// Browser audio engine (desktop + web) — only for music.
		// Audiobook provider manages its own HTMLAudioElement for now.
		// This will be unified in a later phase.
		_playbackState.set('playing');
		if (opts.speed) _speed.set(opts.speed);
	}
}

export function pause(): void {
	isRemoteMirror = false;
	stopShadowTick();

	if (isAndroidNative) {
		import('@tauri-apps/api/core').then(({ invoke }) => {
			invoke('pause_music').catch(() => { });
		});
		stopNativePositionPolling();
	} else if (audioEngine.isLoaded) {
		audioEngine.pause();
	}

	const state = get(_playbackState);
	if (state === 'playing' || state === 'loading') {
		_playbackState.set('paused');
	}
}

export function resume(): void {
	isRemoteMirror = false;

	if (isAndroidNative) {
		if (!nativePlayerReady) {
			// Force-close restore scenario: need to re-play.
			// Content provider should handle this.
			return;
		}
		import('@tauri-apps/api/core').then(({ invoke }) => {
			invoke('resume_music').catch(() => { });
		});
		if (get(_mode) !== 'audiobook') startNativePositionPolling();
	} else if (audioEngine.isLoaded) {
		audioEngine.resume();
	}

	_playbackState.set('playing');
}

export function seek(ms: number): void {
	lastSeekTime = Date.now();
	shadowEpochMs = lastSeekTime - ms;
	_positionMs.set(ms);

	if (isAndroidNative) {
		import('@tauri-apps/api/core').then(({ invoke }) => {
			invoke('seek_music', { positionMs: ms }).catch(() => { });
		});
	} else if (audioEngine.isLoaded) {
		audioEngine.seek(ms / 1000);
	}
}

export function setSpeed(rate: number): void {
	_speed.set(rate);
	if (isAndroidNative) {
		import('@tauri-apps/api/core').then(({ invoke }) => {
			invoke('set_playback_speed', { speed: rate }).catch(() => { });
		});
	}
	// HTMLAudioElement speed is managed by the audiobook provider for now.
}

export function setVolume(gain: number): void {
	_volume.set(gain);

	if (!isAndroidNative) {
		audioEngine.setVolume(gain);
	} else {
		import('@tauri-apps/api/core').then(({ invoke }) => {
			invoke('set_volume', { volume: gain }).catch(() => { });
		});
	}
}

// ── Mode management ──────────────────────────────────────────────────────────

/**
 * Atomically switch the active playback mode. Pauses current audio,
 * reconfigures the native notification surface, and notifies providers.
 *
 * This replaces the reactive $effect in +layout.svelte and the
 * activePlayer writable store.
 */
export function switchMode(newMode: PlaybackMode): void {
	const currentMode = get(_mode);
	if (currentMode === newMode) return;

	// 1. Notify the outgoing provider.
	const outgoing = providers.get(currentMode);
	outgoing?.onModeDeactivated();

	// 2. Pause current audio (synchronous from the engine's perspective).
	pause();

	// 3. Update mode.
	_mode.set(newMode);

	// 4. Reconfigure native notification surface.
	if (isAndroidNative) {
		import('@tauri-apps/api/core').then(({ invoke }) => {
			invoke('set_audiobook_mode', { isAudiobook: newMode === 'audiobook' }).catch(() => { });
		});
	}

	// 5. Notify the incoming provider.
	const incoming = providers.get(newMode);
	incoming?.onModeActivated();
}

// ── Remote state mirroring ───────────────────────────────────────────────────

export interface RemoteState {
	is_audiobook?: boolean;
	track_id?: string;
	audiobook_id?: string;
	audiobook_title?: string;
	position_ms: number;
	playing: boolean;
	volume?: number;
	speed?: number;
	playback_epoch_ms?: number;
}

/**
 * Mirror a remote device's state on this idle device. Called by the device
 * session when an SSE `state` event arrives from the active device.
 */
export function syncRemoteState(state: RemoteState): void {
	// Don't interrupt a device that is actively streaming audio locally.
	// On Android, audioEngine is never loaded — check native player instead.
	const hasLocalAudio = audioEngine.isLoaded || (isAndroidNative && nativePlayerReady);
	if (get(_playbackState) === 'playing' && hasLocalAudio && !isRemoteMirror) {
		return;
	}

	// Switch mode if needed.
	const targetMode: PlaybackMode = state.is_audiobook ? 'audiobook' : 'music';
	if (get(_mode) !== targetMode) {
		_mode.set(targetMode);
	}

	// Update shared state.
	if (state.volume !== undefined) _volume.set(state.volume);
	if (state.speed !== undefined) _speed.set(state.speed);
	_positionMs.set(state.position_ms);
	isRemoteMirror = state.playing;
	_playbackState.set(state.playing ? 'playing' : 'paused');

	// Re-anchor shadow epoch.
	if (state.playing) {
		shadowEpochMs = Date.now() - state.position_ms;
		shadowSpeed = state.speed ?? 1.0;
		startShadowTick();
	} else {
		stopShadowTick();
	}

	// Let the content provider handle content-specific mirroring (load metadata, etc.).
	const provider = providers.get(targetMode);
	provider?.onRemoteSync?.(state);
}

/**
 * Receive a control command from a remote device (via SSE).
 * Routes to the appropriate transport action or content provider.
 */
export function receiveControlCommand(action: string, payload: ControlPayload): void {
	const currentMode = get(_mode);
	const provider = providers.get(currentMode);

	// Delegate all control commands to the active provider. The provider
	// handles transport-level actions (toggle, seek, volume) as well as
	// content-specific navigation (next, previous, skip). This ensures
	// that during migration, audiobook's HTMLAudioElement and music's
	// AudioEngine are controlled correctly by their respective providers.
	switch (action) {
		case 'seek':
			// Engine handles seek directly (updates position, shadow epoch,
			// native bridge). Provider can optionally react too.
			if (payload?.position_ms !== undefined) seek(payload.position_ms);
			break;
		case 'volume':
			if (payload?.volume !== undefined) setVolume(payload.volume);
			break;
		case 'speed':
			if (payload?.speed !== undefined) setSpeed(payload.speed);
			break;
		default:
			// toggle, next, previous, skip_forward, skip_backward —
			// delegate to provider for content-aware handling.
			provider?.onControlCommand?.(action, payload);
			break;
	}
}

// ── Heartbeat state builder ──────────────────────────────────────────────────

/**
 * Build the heartbeat state object from unified engine state.
 * Replaces the mode-checking buildState() in deviceSession.ts.
 */
export function buildHeartbeatState(): DeviceState {
	const currentMode = get(_mode);
	const content = get(_currentContent);
	const pos = get(_positionMs);
	const playing = get(_playbackState) === 'playing';
	const vol = get(_volume);

	if (currentMode === 'audiobook') {
		return {
			is_audiobook: true,
			audiobook_id: content?.id ?? '',
			audiobook_title: content?.title ?? '',
			position_ms: pos,
			playing,
			volume: vol,
		};
	}

	// Podcast has no multi-device sync backend — report as idle so other
	// devices don't attempt to mirror it.
	if (currentMode === 'podcast') {
		return { track_id: '', track_title: '', album_id: '', position_ms: 0, playing: false, volume: vol };
	}

	return {
		track_id: content?.id ?? '',
		track_title: content?.title ?? '',
		album_id: content?.album ?? '',
		position_ms: pos,
		playing,
		volume: vol,
	};
}

// ── Stop all providers ───────────────────────────────────────────────────────

/**
 * Stop all audio and deactivate every provider. Used when another device
 * takes over (pause_others) — we need to silence both music's AudioEngine
 * AND audiobook's HTMLAudioElement, regardless of which mode is active.
 */
export function stopAll(): void {
	pause();
	for (const provider of providers.values()) {
		provider.onModeDeactivated();
	}
}

/**
 * Route a play_command from SSE to the music content provider.
 * The provider loads the queue and starts playback.
 */
export function handlePlayCommand(trackId: string, posMs: number, queue?: unknown[]): void {
	// Play commands are always music — switch mode if needed.
	if (get(_mode) !== 'music') {
		switchMode('music');
	}
	getMusicProvider()?.onPlayCommand?.(trackId, posMs, queue);
}

// ── Lifecycle ────────────────────────────────────────────────────────────────

export function destroy(): void {
	stopNativePositionPolling();
	stopShadowTick();
	nativeListenersInit = false;
	nativePlayerReady = false;
}

// ── Utility accessors (for migration) ────────────────────────────────────────

export function isNativePlayerReady(): boolean {
	return nativePlayerReady;
}

export function setNativePlayerReady(ready: boolean): void {
	nativePlayerReady = ready;
}

export function isCurrentlyRemoteMirror(): boolean {
	return isRemoteMirror;
}

export function setRemoteMirror(mirror: boolean): void {
	isRemoteMirror = mirror;
}

export function getShadowEpochMs(): number {
	return shadowEpochMs;
}

export function setShadowEpochMs(epoch: number): void {
	shadowEpochMs = epoch;
}

// ── Restore coordination ─────────────────────────────────────────────────────
// The player store's restoreState() and deviceSession's refreshDevices() both
// run concurrently on startup. Without coordination, restoreState() can
// overwrite the remote device's current state with stale localStorage data.
// This deferred promise lets refreshDevices() wait for restore to finish
// before calling syncRemoteState(), guaranteeing remote sync always wins.

let _resolveRestore: () => void;
export const restoreReady: Promise<void> = new Promise((r) => { _resolveRestore = r; });
export function signalRestoreComplete(): void { _resolveRestore(); }
