/**
 * Native Android Auto / CarPlay integration via tauri-plugin-car.
 * Provides media browsing, now-playing, and playback control for car displays.
 *
 * On non-mobile platforms, this module is a no-op.
 */
import { invoke } from '@tauri-apps/api/core';
import { addPluginListener } from '@tauri-apps/api/core';

export interface Track {
	id: string;
	title: string;
	artist?: string;
	album?: string;
	duration_ms?: number;
	artwork_url?: string;
}

export interface MediaItem {
	id: string;
	title: string;
	subtitle?: string;
	playable: boolean;
	artwork_url?: string;
	children?: MediaItem[];
}

export interface CarActionEvent {
	action:
		| 'car-play'
		| 'car-pause'
		| 'car-next'
		| 'car-previous'
		| 'car-stop'
		| 'car-play-item'
		| 'car-seekto';
	payload?: string;
}

export interface CarConnectionEvent {
	connected: boolean;
}

export async function setNowPlaying(track: Track): Promise<void> {
	await invoke('plugin:car|set_now_playing', {
		track,
	});
}

export async function setMediaRoot(items: MediaItem[]): Promise<void> {
	await invoke('plugin:car|set_media_root', {
		items: JSON.stringify(items),
	});
}

export async function setPlaybackState(playing: boolean, positionMs: number): Promise<void> {
	await invoke('plugin:car|set_playback_state', {
		playing,
		position_ms: positionMs,
	});
}

export async function onCarAction(handler: (event: CarActionEvent) => void): Promise<() => void> {
	const listener = await addPluginListener('car', 'carAction', handler);
	return () => listener.unregister();
}

export async function onCarConnection(
	handler: (event: CarConnectionEvent) => void
): Promise<() => void> {
	const listener = await addPluginListener('car', 'carConnection', handler);
	return () => listener.unregister();
}
