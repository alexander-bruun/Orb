import { apiFetch } from './client';
import { getApiBase } from './base';
import type { Track } from '$lib/types';
import { STORAGE_KEYS } from '$lib/constants';

export interface DeviceState {
	track_id?: string;
	track_title?: string;
	album_id?: string;
	position_ms: number;
	playing: boolean;
	/** Volume level 0.0–1.0; owned by the active device. */
	volume?: number;
	/** Unix-ms timestamp at which the track position would be 0.
	 *  Computed server-side as (now_ms - position_ms) when playing=true.
	 *  Clients can derive current position as Date.now() - playback_epoch_ms. */
	playback_epoch_ms?: number;
	/** Set when the active device is playing an audiobook instead of a music track. */
	is_audiobook?: boolean;
	audiobook_id?: string;
	audiobook_title?: string;
}

export interface Device {
	id: string;
	name: string;
	state: DeviceState;
	last_seen: string;
	is_active: boolean;
}

export interface PlaybackSettings {
	exclusive_mode: boolean;
}

export interface DeviceEvent {
	type: 'state' | 'pause_others' | 'registered' | 'unregistered' | 'play_command' | 'control_command' | 'exclusive_mode';
	device_id?: string;
	state?: DeviceState;
	enabled?: boolean;
	// play_command fields
	track_id?: string;
	position_ms?: number;
	queue?: Track[];           // embedded queue for play_command
	// control_command fields
	action?: 'toggle' | 'next' | 'previous' | 'seek' | 'volume' | 'skip_forward' | 'skip_backward' | 'speed';
	volume?: number;           // 0.0–1.0; for 'volume' action
	speed?: number;            // playback speed; for 'speed' action
}

export const devices = {
	list: () => apiFetch<Device[]>('/user/devices'),

	register: (device_id: string, name: string, audio_caps?: { max_channels: number }) =>
		apiFetch<Device>('/user/devices', {
			method: 'POST',
			body: JSON.stringify({ device_id, name, ...(audio_caps ? { audio_caps } : {}) })
		}),

	heartbeat: (id: string, state: DeviceState) =>
		apiFetch<void>(`/user/devices/${id}/heartbeat`, {
			method: 'POST',
			body: JSON.stringify(state)
		}),

	unregister: (id: string) =>
		apiFetch<void>(`/user/devices/${id}`, { method: 'DELETE' }),

	activate: (id: string) =>
		apiFetch<void>(`/user/devices/${id}/activate`, { method: 'POST' }),

	playCommand: (id: string, track_id: string, position_ms = 0, queue?: Track[]) =>
		apiFetch<void>(`/user/devices/${id}/play`, {
			method: 'POST',
			body: JSON.stringify({ track_id, position_ms, queue })
		}),

	controlCommand: (id: string, action: 'toggle' | 'next' | 'previous' | 'seek' | 'volume' | 'skip_forward' | 'skip_backward' | 'speed', opts?: { position_ms?: number; volume?: number; speed?: number }) =>
		apiFetch<void>(`/user/devices/${id}/control`, {
			method: 'POST',
			body: JSON.stringify({ action, ...opts })
		}),

	getPlaybackSettings: () => apiFetch<PlaybackSettings>('/user/playback-settings'),

	patchPlaybackSettings: (settings: Partial<PlaybackSettings>) =>
		apiFetch<PlaybackSettings>('/user/playback-settings', {
			method: 'PATCH',
			body: JSON.stringify(settings)
		}),

	/** Open an SSE connection for device events. Returns an EventSource. */
	openEvents(onMessage: (e: DeviceEvent) => void, onError?: () => void): EventSource {
		const url = `${getApiBase()}/user/devices/events`;
		// SSE cannot set custom headers; pass the JWT via query param (server supports ?token=).
		let token: string | null = null;
		try {
			const raw = typeof localStorage !== 'undefined' ? localStorage.getItem(STORAGE_KEYS.AUTH) : null;
			if (raw) token = JSON.parse(raw).token ?? null;
		} catch { /* ignore */ }
		const src = new EventSource(token ? `${url}?token=${encodeURIComponent(token)}` : url);
		src.onmessage = (e) => {
			try {
				onMessage(JSON.parse(e.data) as DeviceEvent);
			} catch {
				// ignore malformed messages
			}
		};
		if (onError) src.onerror = onError;
		return src;
	}
};
