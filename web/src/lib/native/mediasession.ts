/**
 * Native Android media playback via tauri-plugin-mediasession.
 *
 * Uses Media3 ExoPlayer for native audio playback with automatic
 * foreground service, system media notification, and lock screen controls.
 *
 * On non-mobile platforms this module is a no-op — the web audio engine
 * handles playback and the browser's MediaSession API handles controls.
 */
import { invoke } from '@tauri-apps/api/core';
import { addPluginListener } from '@tauri-apps/api/core';

// ── Types ────────────────────────────────────────────────────────────────────

export interface LoadTrackOptions {
	url: string;
	title: string;
	artist: string;
	album?: string;
	artwork?: string;
}

export interface PlaybackStateEvent {
	state: 'idle' | 'loading' | 'playing' | 'paused' | 'ended';
	position_ms: number;
}

export interface PositionUpdateEvent {
	position_ms: number;
	duration_ms: number;
}

export interface MediaActionEvent {
	action:
		| 'media-play'
		| 'media-pause'
		| 'media-next'
		| 'media-previous'
		| 'media-stop'
		| 'media-seekto';
	seekPos?: number;
}

// ── Commands ─────────────────────────────────────────────────────────────────

/** Start the native playback service. Called once on app startup. */
export async function initializePlayer(): Promise<void> {
	await invoke('plugin:mediasession|initialize_player');
}

/** Load a track into ExoPlayer and begin playback. */
export async function loadTrack(options: LoadTrackOptions): Promise<void> {
	await invoke('plugin:mediasession|load_track', {
		url: options.url,
		title: options.title,
		artist: options.artist,
		album: options.album ?? null,
		artwork: options.artwork ?? null,
	});
}

export async function play(): Promise<void> {
	await invoke('plugin:mediasession|play');
}

export async function pause(): Promise<void> {
	await invoke('plugin:mediasession|pause');
}

export async function nextTrack(): Promise<void> {
	await invoke('plugin:mediasession|next_track');
}

export async function previousTrack(): Promise<void> {
	await invoke('plugin:mediasession|previous_track');
}

export async function seek(positionMs: number): Promise<void> {
	await invoke('plugin:mediasession|seek', { position_ms: positionMs });
}

/** Stop playback and tear down the foreground service. */
export async function stop(): Promise<void> {
	await invoke('plugin:mediasession|stop');
}

/** Read the native debug log file contents. */
export async function getLog(): Promise<string> {
	const result = await invoke<{ value: string }>('plugin:mediasession|get_log');
	return result.value;
}

/** Write a message to the native log file from JS. */
export async function writeLog(message: string): Promise<void> {
	await invoke('plugin:mediasession|write_log', { message });
}

// ── Events ───────────────────────────────────────────────────────────────────

/** Fired when ExoPlayer's playback state changes (idle/loading/playing/paused/ended). */
export async function onPlaybackState(
	handler: (event: PlaybackStateEvent) => void
): Promise<() => void> {
	const listener = await addPluginListener('mediasession', 'playback_state', handler);
	return () => listener.unregister();
}

/** Periodic position updates (~500 ms) while playing. */
export async function onPositionUpdate(
	handler: (event: PositionUpdateEvent) => void
): Promise<() => void> {
	const listener = await addPluginListener('mediasession', 'position_update', handler);
	return () => listener.unregister();
}

/** Media button presses from notification, lock screen, Bluetooth, etc. */
export async function onMediaAction(
	handler: (event: MediaActionEvent) => void
): Promise<() => void> {
	const listener = await addPluginListener('mediasession', 'mediaAction', handler);
	return () => listener.unregister();
}

/** Fired when the current track finishes playing. */
export async function onTrackEnded(handler: () => void): Promise<() => void> {
	const listener = await addPluginListener('mediasession', 'track_ended', handler);
	return () => listener.unregister();
}
