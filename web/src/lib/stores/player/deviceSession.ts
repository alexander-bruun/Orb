/**
 * deviceSession.ts
 *
 * Manages the lifecycle of the local device session:
 *  - Generates / persists a stable device ID.
 *  - Registers the device with the API once the user is authenticated.
 *  - Keeps the registration alive with a periodic heartbeat (every 30 s).
 *  - Opens an SSE channel for cross-device events.
 *  - Exposes `exclusiveMode` so other stores / the settings page can react.
 */

import { writable, get } from 'svelte/store';
import { browser } from '$app/environment';
import { devices as devicesApi, type Device, type DeviceEvent } from '$lib/api/devices';
import { ApiError } from '$lib/api/client';
import { authStore } from '$lib/stores/auth';
import { isTauri, nativePlatform } from '$lib/utils/platform';
import { isOffline } from '$lib/stores/offline/connectivity';
import { TIMINGS, STORAGE_KEYS } from '$lib/constants';
import * as engine from './engine';

// ── Local device identity ────────────────────────────────────────────────────

const DEVICE_KEY = STORAGE_KEYS.DEVICE_ID;
// Stable native device ID cached in localStorage so it's available synchronously
// on subsequent cold starts (populated from ANDROID_ID on first run).
const NATIVE_DEVICE_KEY = STORAGE_KEYS.NATIVE_DEVICE_ID;
const DEVICE_NAME_KEY = STORAGE_KEYS.DEVICE_NAME;

function generateId(): string {
	if (typeof crypto !== 'undefined' && typeof crypto.randomUUID === 'function') {
		return crypto.randomUUID();
	}
	// Fallback for older mobile browsers that lack crypto.randomUUID
	return 'xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx'.replace(/[xy]/g, (c) => {
		const r = (typeof crypto !== 'undefined' && crypto.getRandomValues)
			? (crypto.getRandomValues(new Uint8Array(1))[0] & 15)
			: Math.floor(Math.random() * 16);
		return (c === 'x' ? r : (r & 0x3 | 0x8)).toString(16);
	});
}

function getOrCreateDeviceId(): string {
	if (!browser) return '';
	try {
		// Use sessionStorage so each tab/window gets its own device ID.
		// localStorage is shared across all tabs, which causes duplicate SSE
		// event handling and double-processing of control commands when
		// multiple windows are open.
		let id = sessionStorage.getItem(DEVICE_KEY);
		if (!id) {
			id = generateId();
			try { sessionStorage.setItem(DEVICE_KEY, id); } catch { /* storage blocked */ }
		}
		return id;
	} catch {
		// sessionStorage unavailable (e.g. Safari/Firefox private mode) – use a
		// session-scoped ID so the device still registers for this tab.
		return generateId();
	}
}

/** Resolve the stable Android hardware ID. Returns cached value instantly if available. */
async function resolveNativeDeviceId(): Promise<string> {
	try {
		// Return cached value from a previous run without an IPC round-trip.
		const cached = localStorage.getItem(NATIVE_DEVICE_KEY);
		if (cached) return cached;

		const { invoke } = await import('@tauri-apps/api/core');
		const id = await invoke<string>('get_device_id');
		if (id) {
			try { localStorage.setItem(NATIVE_DEVICE_KEY, id); } catch { /* storage blocked */ }
			return id;
		}
	} catch { /* IPC unavailable — fall through */ }
	return generateId();
}

function getDeviceName(): string {
	if (!browser) return 'Unknown';
	try {
		const stored = localStorage.getItem(DEVICE_NAME_KEY);
		if (stored) return stored;
	} catch { /* storage blocked in private mode */ }

	const ua = navigator.userAgent;

	// Detect browser
	let browserName = 'Browser';
	if (/OPR|Opera/i.test(ua)) browserName = 'Opera';
	else if (/Edg\//i.test(ua)) browserName = 'Edge';
	else if (/Firefox/i.test(ua)) browserName = 'Firefox';
	else if (/SamsungBrowser/i.test(ua)) browserName = 'Samsung Internet';
	else if (/CriOS/i.test(ua)) browserName = 'Chrome';
	else if (/Chrome/i.test(ua)) browserName = 'Chrome';
	else if (/Safari/i.test(ua)) browserName = 'Safari';

	// Detect OS / platform
	let platform = '';
	if (/Android/i.test(ua)) platform = 'Android';
	else if (/iPhone/i.test(ua)) platform = 'iPhone';
	else if (/iPad/i.test(ua)) platform = 'iPad';
	else if (/Macintosh/i.test(ua)) platform = 'macOS';
	else if (/Windows/i.test(ua)) platform = 'Windows';
	else if (/CrOS/i.test(ua)) platform = 'ChromeOS';
	else if (/Linux/i.test(ua)) platform = 'Linux';

	// For native desktop (Tauri) app, use a distinct prefix.
	if (typeof window !== 'undefined' && isTauri()) {
		return platform ? `Orb Desktop · ${platform}` : 'Orb Desktop';
	}

	if (platform) return `${browserName} · ${platform}`;
	return browserName;
}

// ── Exported stores ──────────────────────────────────────────────────────────

/**
 * Stable ID for this browser tab / native window.
 * On Android native: populated from ANDROID_ID (survives reinstalls) before
 * the first session starts. Uses ES module live-binding so all importers see
 * the updated value after startSession resolves it.
 * On browser: a per-tab UUID from sessionStorage.
 */
// Pre-populate from localStorage cache so the value is available synchronously
// on subsequent runs (populated async on first run via resolveNativeDeviceId).
export let deviceId: string = browser
	? (isTauri() && nativePlatform() === 'android'
		? (localStorage.getItem(NATIVE_DEVICE_KEY) ?? '')
		: getOrCreateDeviceId())
	: '';

/** Human-readable name for this device (editable). */
export const deviceName = writable<string>(browser ? getDeviceName() : 'Browser');

/** Whether exclusive mode is enabled for the signed-in user. */
export const exclusiveMode = writable<boolean>(false);

/** Live list of all active devices for the signed-in user. */
export const activeDevices = writable<Device[]>([]);

/** True while the device is registered and heartbeating. */
export const deviceRegistered = writable<boolean>(false);

/** ID of the currently active (exclusive) device, or empty string. */
export const activeDeviceId = writable<string>('');

// ── Internal state ───────────────────────────────────────────────────────────

let heartbeatTimer: ReturnType<typeof setInterval> | null = null;
let sseSource: EventSource | null = null;

// ── Lifecycle ────────────────────────────────────────────────────────────────

/** Start the device session. Called when the user is authenticated. */
export async function startSession() {
	if (!browser) return;

	// On Android native, resolve the hardware-stable ANDROID_ID before registering.
	// This updates the exported live binding so heartbeat / SSE handlers use it too.
	if (isTauri() && nativePlatform() === 'android' && !deviceId) {
		deviceId = await resolveNativeDeviceId();
	}

	if (!deviceId) return;
	const name = get(deviceName);

	try {
		// Load the user's exclusive-mode setting.
		const settings = await devicesApi.getPlaybackSettings();
		exclusiveMode.set(settings.exclusive_mode);

		// Register this device.
		await devicesApi.register(deviceId, name);
		deviceRegistered.set(true);

		// Load current device list.
		await refreshDevices();

		// Send an immediate heartbeat so the server has this device's state
		// before other devices poll. Without this, a browser refresh causes
		// the server to return position_ms: 0 to secondary devices until
		// the first periodic heartbeat (30s later).
		sendHeartbeat().catch(() => {});

		// Start heartbeat (every 30 s).
		if (heartbeatTimer) clearInterval(heartbeatTimer);
		heartbeatTimer = setInterval(sendHeartbeat, TIMINGS.HEARTBEAT_INTERVAL);

		// Open SSE channel.
		openSSE();
	} catch (err) {
		console.warn('[deviceSession] startSession failed:', err);
	}
}

/**  Stop heartbeat and close SSE. Called on logout. */
export function stopSession() {
	if (heartbeatTimer) { clearInterval(heartbeatTimer); heartbeatTimer = null; }
	if (sseSource) { sseSource.close(); sseSource = null; }
	deviceRegistered.set(false);
	activeDevices.set([]);
	if (browser && deviceId) {
		devicesApi.unregister(deviceId).catch(() => {});
	}
}

/** Send current playback state to the server immediately. */
export async function sendHeartbeat() {
	if (!browser || !deviceId || !get(deviceRegistered)) return;
	try {
		const state = buildState();
		await devicesApi.heartbeat(deviceId, state);
	} catch (err: unknown) {
		// 404 means our session TTL expired (e.g. mobile screen off). Re-register.
		if (err instanceof ApiError && err.status === 404) {
			deviceRegistered.set(false);
			startSession();
		}
		// Other errors: silently ignore.
	}
}

function buildState() {
	return engine.buildHeartbeatState();
}

/** Refresh the device list from the server. */
export async function refreshDevices() {
	try {
		const list = await devicesApi.list();
		activeDevices.set(list);
		const prevActiveId = get(activeDeviceId);
		const active = list.find((d) => d.is_active);
		if (active) {
			activeDeviceId.set(active.id);
			// If another device is actively playing, mirror its state immediately
			// so the progress bar is correct from the first render — no need to
			// wait for the next heartbeat SSE event.
			// Only mirror when exclusive mode is enabled.
			// Wait for restoreState() to finish first so the remote sync always
			// overwrites stale localStorage data (not the other way around).
			// Skip sync if the state is empty (no track or audiobook loaded) —
			// this avoids resetting position to 0 when a device just registered.
			if (get(exclusiveMode) && active.id !== deviceId && active.state &&
				(active.state.track_id || active.state.audiobook_id)) {
				await engine.restoreReady;
				engine.syncRemoteState(active.state);
			}
		} else {
			activeDeviceId.set('');
		}

		// If we were shadowing a remote device that is no longer the active
		// device, stop the shadow tick and reset to paused. This prevents
		// phantom "playing" state when the remote device goes offline.
		const wasRemoteShadow = prevActiveId && prevActiveId !== deviceId;
		const nowRemoteShadow = active && active.id !== deviceId;
		if (wasRemoteShadow && !nowRemoteShadow) {
			engine.stopAll();
		}
	} catch {
		// ignore
	}
}

// ── SSE ──────────────────────────────────────────────────────────────────────

function openSSE() {
	if (!browser) return;
	if (sseSource) { sseSource.close(); }

	sseSource = devicesApi.openEvents(handleDeviceEvent, () => {
		// Reconnect after 5 s on error, but only if we're still online.
		// When offline, the isOffline subscription handles reconnection once
		// the connection is restored, so we don't need to hammer the server.
		setTimeout(() => {
			if (get(deviceRegistered) && !get(isOffline)) {
				openSSE();
				refreshDevices();
			}
		}, TIMINGS.SSE_RECONNECT_DELAY);
	});
}

function handleDeviceEvent(evt: DeviceEvent) {
	switch (evt.type) {
		case 'state':
			// Mirror the active device's state on idle devices via the engine.
			// Only mirror when exclusive mode is enabled; otherwise each device
			// plays independently.
			if (
				get(exclusiveMode) &&
				evt.device_id &&
				evt.device_id !== deviceId &&
				evt.device_id === get(activeDeviceId) &&
				evt.state
			) {
				engine.syncRemoteState(evt.state);
			}
			refreshDevices();
			break;

		case 'registered':
		case 'unregistered':
			refreshDevices();
			break;

		case 'exclusive_mode':
			exclusiveMode.set(evt.enabled ?? false);
			break;

		case 'pause_others':
			// In non-exclusive mode, ignore pause_others — each device is independent.
			if (!get(exclusiveMode)) {
				refreshDevices();
				break;
			}
			// Stop local audio BEFORE switching activeDeviceId.  If we updated
			// the pointer first, togglePlayPause/pauseLocal would delegate the
			// pause to the newly-active remote device instead of stopping this
			// device's own engine — causing the remote to flash-pause.
			if (evt.device_id !== deviceId) {
				engine.stopAll();
			}
			// Now update the active device pointer.
			if (evt.device_id) activeDeviceId.set(evt.device_id);
			refreshDevices();
			break;

		case 'play_command':
			// In non-exclusive mode, ignore play commands from other devices.
			if (!get(exclusiveMode)) {
				refreshDevices();
				break;
			}
			// A play command implicitly makes the target the active device.
			// Update the pointer for ALL devices so playTrack() on the target
			// device does not mis-delegate back to the originator.
			if (evt.device_id) activeDeviceId.set(evt.device_id);

			// Non-target devices must stop their audio so only one device plays.
			if (evt.device_id !== deviceId) {
				engine.stopAll();
			}

			// Target device: route to the music content provider via engine.
			if (evt.device_id === deviceId && evt.track_id) {
				engine.handlePlayCommand(evt.track_id, evt.position_ms ?? 0, evt.queue);
			}
			refreshDevices();
			break;

		case 'control_command':
			// This device is the target of a remote control action.
			// Route through the engine — it dispatches to the active provider.
			if (evt.device_id === deviceId && evt.action) {
				engine.receiveControlCommand(evt.action, {
					position_ms: evt.position_ms,
					volume: evt.volume,
					speed: evt.speed,
				});
			}
			break;
	}
}

// ── Auth integration ─────────────────────────────────────────────────────────

// Auto-start / stop session when authentication state changes.
if (browser) {
	authStore.subscribe((state) => {
		if (state.token && !get(deviceRegistered)) {
			startSession();
		} else if (!state.token && get(deviceRegistered)) {
			stopSession();
		}
	});
}

// ── Offline / online recovery ────────────────────────────────────────────────

// When the connection is restored, re-establish the device session and
// reclaim active status if this device was playing locally while offline.
if (browser) {
	let _prevOffline = false;
	isOffline.subscribe((offline) => {
		if (_prevOffline && !offline) {
			// Came back online — re-register, restart heartbeat and SSE.
			const auth = get(authStore);
			if (auth.token) {
				startSession().then(() => {
					// If this device was playing locally while we were offline,
					// claim the active slot so the server reflects reality.
					if (get(engine.enginePlaybackState) === 'playing' && deviceId) {
						devicesApi.activate(deviceId).catch(() => {});
					}
				});
			}
		}
		_prevOffline = offline;
	});
}
