/**
 * casting.ts
 *
 * Manages two types of cast targets:
 *
 *  1. Audio Output Devices — route browser audio to a different system audio
 *     output (Bluetooth speakers, HDMI, USB DAC, etc.) via the browser's
 *     HTMLMediaElement.setSinkId() API.  Requires the user to grant the
 *     "speaker-selection" permission (or the browser to prompt for it).
 *
 *  2. Chromecast — stream audio to a Google Cast-enabled device using the
 *     Cast Web Sender SDK.  The Default Media Receiver is used so no custom
 *     Cast receiver app or developer registration is required.
 *
 * Both features are optional / progressive:
 *  - `sinkIdSupported` is false on Firefox and older browsers.
 *  - `castAvailable` is false unless the Cast SDK loads (Chrome + Cast
 *    extension / built-in Chrome cast support required).
 */

import { writable, get } from 'svelte/store';
import { browser } from '$app/environment';

// ── Remote Playback API (mobile casting) ──────────────────────────────────────

/**
 * True when the browser supports the Remote Playback API on the underlying
 * <audio> element.  This is available on mobile Chrome / Edge and enables
 * casting to Chromecast, AirPlay, etc. via the native OS device picker.
 */
export let remotePlaybackSupported = false;

if (browser) {
	import('$lib/audio/engine').then(({ audioEngine }) => {
		remotePlaybackSupported = audioEngine.remotePlaybackSupported;
	});
}

/**
 * Show the browser's native remote-playback device picker (Chromecast,
 * AirPlay, etc.).  Uses the Remote Playback API on the underlying
 * HTMLAudioElement.  Throws if the API is unavailable or the user cancels.
 */
export async function promptRemotePlayback(): Promise<void> {
	const { audioEngine } = await import('$lib/audio/engine');
	await audioEngine.promptRemotePlayback();
}

// ── Audio Output Devices ──────────────────────────────────────────────────────

const AUDIO_OUTPUT_STORAGE_KEY = 'orb_audio_output_id';

export interface AudioOutputDevice {
	deviceId: string;
	label: string;
}

/** List of available audio outputs (populated after calling refreshAudioOutputDevices). */
export const audioOutputDevices = writable<AudioOutputDevice[]>([]);

/** Currently selected audio output device ID ('default' = system default). */
export const selectedAudioOutputId = writable<string>(
	browser ? (localStorage.getItem(AUDIO_OUTPUT_STORAGE_KEY) ?? 'default') : 'default'
);

/**
 * True when the browser supports routing audio to a specific output via
 * HTMLMediaElement.setSinkId().  Currently Chrome / Edge only.
 */
export const sinkIdSupported: boolean =
	browser && typeof (HTMLAudioElement.prototype as unknown as { setSinkId?: unknown }).setSinkId === 'function';

/**
 * Enumerate all available audio output devices.
 * On first call the browser may show a permission prompt.
 * Silently fails if the API is unavailable.
 */
export async function refreshAudioOutputDevices(): Promise<void> {
	if (!browser) return;
	if (!navigator.mediaDevices?.enumerateDevices) return;

	try {
		const raw = await navigator.mediaDevices.enumerateDevices();
		const outputs: AudioOutputDevice[] = raw
			.filter((d) => d.kind === 'audiooutput')
			.map((d) => ({
				deviceId: d.deviceId,
				label:
					d.label ||
					(d.deviceId === 'default'
						? 'System Default'
						: d.deviceId === 'communications'
							? 'Communications Device'
							: `Output ${d.deviceId.slice(0, 8)}`)
			}));

		// Always put 'default' first.
		outputs.sort((a, b) => {
			if (a.deviceId === 'default') return -1;
			if (b.deviceId === 'default') return 1;
			return a.label.localeCompare(b.label);
		});

		audioOutputDevices.set(outputs);
	} catch {
		// Permission denied or API unavailable — leave the store empty.
	}
}

/**
 * Select an audio output device.  Persists the choice and applies it
 * immediately to the active audio engine.
 */
export async function setAudioOutput(deviceId: string): Promise<void> {
	selectedAudioOutputId.set(deviceId);
	try {
		localStorage.setItem(AUDIO_OUTPUT_STORAGE_KEY, deviceId);
	} catch {
		/* storage blocked */
	}
	// Delegate to the audio engine (lazy import to avoid circular deps).
	const { audioEngine } = await import('$lib/audio/engine');
	await audioEngine.setAudioOutput(deviceId);
}

// Initialise on page load and keep the list fresh when devices change.
if (browser) {
	refreshAudioOutputDevices();
	navigator.mediaDevices?.addEventListener?.('devicechange', refreshAudioOutputDevices);
}

// ── Chromecast ────────────────────────────────────────────────────────────────
// Auto-sync the current track to the Cast receiver whenever it changes while
// a Cast session is active.  Uses a dynamic import of the player store to
// avoid a circular dependency (player → casting → player).
if (browser) {
	import('$lib/stores/player').then(({ currentTrack }) => {
		let prevTrackId: string | null = null;
		currentTrack.subscribe((track) => {
			if (!track || track.id === prevTrackId) return;
			prevTrackId = track.id;
			// Only sync if we already have an active cast session.
			if (get(castState) === 'connected') {
				syncCastTrack(track.id, 0).catch(() => {});
			}
		});
	});
}

/** Default Media Receiver App ID — works without registering a custom receiver. */
const CAST_APP_ID = 'CC1AD845';

export type CastState = 'unavailable' | 'idle' | 'connecting' | 'connected';

/** Current state of the Chromecast session. */
export const castState = writable<CastState>('unavailable');

/** Friendly name of the currently connected Cast receiver. */
export const castDeviceName = writable<string>('');

// Internal: Cast API session handle.
// eslint-disable-next-line @typescript-eslint/no-explicit-any
let _castSession: any = null;
let _castReady = false;

// Minimal ambient declarations so we don't need the full Cast type package.
declare global {
	interface Window {
		__onGCastApiAvailable?: (isAvailable: boolean) => void;
		chrome?: {
			cast?: unknown;
		};
	}
}

// eslint-disable-next-line @typescript-eslint/no-explicit-any
type CastAPI = any;

/**
 * Return the legacy chrome.cast API namespace.
 * SessionRequest, ApiConfig, initialize, requestSession, and the media
 * helpers all live under chrome.cast — NOT under the window.cast (CAF)
 * namespace that `?loadCastFramework=1` also exposes.
 */
function getCast(): CastAPI {
	return (window as unknown as { chrome: { cast: CastAPI } }).chrome?.cast;
}

/**
 * Load and initialise the Cast Web Sender SDK.
 * Safe to call multiple times — idempotent after the first call.
 * Call this when the user opens the device picker or the settings page so
 * that the SDK is ready before they click "Cast".
 */
export function initCastSdk(): void {
	if (!browser || _castReady) return;
	if (document.getElementById('cast-sender-sdk')) {
		// Script already injected (e.g. HMR reload) — try setup directly.
		// Check for SessionRequest to confirm the legacy chrome.cast API is ready.
		if (getCast()?.SessionRequest) _onCastAvailable(true);
		return;
	}

	window.__onGCastApiAvailable = _onCastAvailable;

	const script = document.createElement('script');
	script.id = 'cast-sender-sdk';
	script.src =
		'https://www.gstatic.com/cv/js/sender/v1/cast_sender.js?loadCastFramework=1';
	document.head.appendChild(script);
}

function _onCastAvailable(isAvailable: boolean): void {
	if (!isAvailable || _castReady) return;
	_castReady = true;

	const cast = getCast();
	const sessionRequest = new cast.SessionRequest(CAST_APP_ID);
	const apiConfig = new cast.ApiConfig(
		sessionRequest,
		(session: CastAPI) => {
			// Session was resumed from a previous navigation.
			_castSession = session;
			castDeviceName.set(session.receiver.friendlyName);
			castState.set('connected');
		},
		(availability: string) => {
			const AVAILABLE = cast.ReceiverAvailability
				? cast.ReceiverAvailability.AVAILABLE
				: 'available';
			castState.set(availability === AVAILABLE ? 'idle' : 'unavailable');
		}
	);

	cast.initialize(
		apiConfig,
		() => { /* success — receiverListener fires asynchronously */ },
		(err: unknown) => console.warn('[cast] init error', err)
	);
}

/**
 * Request a new Cast session (shows the Cast picker to the user).
 */
export async function startCast(): Promise<void> {
	if (!_castReady) {
		initCastSdk();
		// Give the SDK a moment to initialise, then retry once.
		await new Promise((r) => setTimeout(r, 2000));
		if (!_castReady) throw new Error('Cast SDK not ready');
	}
	castState.set('connecting');
	return new Promise((resolve, reject) => {
		const cast = getCast();
		cast.requestSession(
			(session: CastAPI) => {
				_castSession = session;
				castDeviceName.set(session.receiver.friendlyName);
				castState.set('connected');
				// Immediately sync the current track to the Cast receiver.
				syncCastTrack().catch(() => {});
				resolve();
			},
			(err: unknown) => {
				castState.set('idle');
				reject(err);
			}
		);
	});
}

/**
 * Stop the current Cast session and return audio to local output.
 */
export function stopCast(): void {
	if (_castSession) {
		_castSession.stop(() => {}, () => {});
		_castSession = null;
	}
	castState.set('idle');
	castDeviceName.set('');
}

/**
 * Send the specified track (or the currently playing track) to the Cast
 * receiver.  A no-op when no Cast session is active.
 *
 * @param trackId   - Track UUID; uses currentTrack from player store if omitted.
 * @param positionMs - Playback start offset in milliseconds.
 */
export async function syncCastTrack(trackId?: string, positionMs = 0): Promise<void> {
	if (get(castState) !== 'connected' || !_castSession) return;

	let id = trackId;
	let pos = positionMs;

	if (!id) {
		const { currentTrack, positionMs: posMsStore } = await import('$lib/stores/player');
		const track = get(currentTrack);
		if (!track) return;
		id = track.id;
		pos = get(posMsStore);
	}

	const { getApiBase } = await import('$lib/api/base');
	const { authStore } = await import('$lib/stores/auth');
	const token = (get(authStore) as { token?: string }).token ?? '';
	const streamUrl = `${getApiBase()}/stream/${id}?token=${encodeURIComponent(token)}`;

	const cast = getCast();
	// Use the cast.media namespace for load requests.
	const mediaInfo = new cast.media.MediaInfo(streamUrl, 'audio/flac');
	mediaInfo.streamType = cast.media.StreamType.BUFFERED;

	const request = new cast.media.LoadRequest(mediaInfo);
	request.currentTime = pos / 1000;
	request.autoplay = true;

	_castSession.loadMedia(
		request,
		() => { /* success */ },
		(err: unknown) => console.warn('[cast] loadMedia error', err)
	);
}
