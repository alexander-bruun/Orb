/**
 * Listen-party store — manages the WebSocket connection for both host and
 * guest roles and provides reactive state for the UI.
 */
import { writable, get } from 'svelte/store';
import { listenPartyApi } from '$lib/api/listenParty';
import { authStore } from '$lib/stores/auth';
import {
	currentTrack,
	playbackState,
	positionMs as playerPositionMs,
} from '$lib/stores/player';

const API_BASE = (import.meta.env.VITE_API_BASE as string | undefined) ?? '/api';

const BASE_WS = (() => {
	if (typeof location === 'undefined') return 'ws://localhost:8080/api';
	const proto = location.protocol === 'https:' ? 'wss:' : 'ws:';
	if (API_BASE.startsWith('http')) return API_BASE.replace(/^https?:/, proto);
	return `${proto}//${location.host}${API_BASE}`;
})();

// ---------------------------------------------------------------------------
// Public types
// ---------------------------------------------------------------------------

export interface Participant {
	id: string;
	nickname: string;
	joined_at?: string;
}

export interface TrackInfo {
	id: string;
	title: string;
	artist_name: string;
	album_id: string;
	bit_depth: number;
	sample_rate: number;
	duration_ms: number;
}

export interface SyncState {
	track_id: string;
	position_ms: number;
	playing: boolean;
	server_time_ms: number;
}

// ---------------------------------------------------------------------------
// Stores
// ---------------------------------------------------------------------------

export const lpRole        = writable<'host' | 'guest' | null>(null);
export const lpSessionId   = writable<string | null>(null);
export const lpParticipants = writable<Participant[]>([]);
export const lpPanelOpen   = writable(false);
export const lpConnected   = writable(false);

/** Guest-only: auth token for stream URLs */
export const lpGuestToken     = writable<string | null>(null);
/** Guest-only: currently playing track metadata */
export const lpGuestTrack     = writable<TrackInfo | null>(null);
/** Guest-only: latest position in ms (updated from audio + ticks) */
export const lpGuestPositionMs = writable(0);
/** Guest-only: whether the host is playing */
export const lpGuestPlaying   = writable(false);
/** Guest-only: duration ms of the current track */
export const lpGuestDurationMs = writable(0);
/** True when the guest has been kicked */
export const lpKicked         = writable(false);
/** True when the host ended the session */
export const lpSessionEnded   = writable(false);
/** Guest-only: the participant ID assigned by the server (used to filter self from participants list) */
export const lpGuestParticipantId = writable<string | null>(null);

// ---------------------------------------------------------------------------
// Internal state
// ---------------------------------------------------------------------------

let ws: WebSocket | null = null;
let guestAudio: HTMLAudioElement | null = null;
let positionTick: ReturnType<typeof setInterval> | null = null;
let hostSyncTimer: ReturnType<typeof setTimeout> | null = null;
let playerUnsubscribers: Array<() => void> = [];
let lastSentTrackId = '';
let lastGuestTrackId = '';
/** Desired play/pause state for the guest — read by onMeta to avoid race conditions */
let guestWantsPlaying = false;
/** Desired volume for the guest audio element */
let guestVolume = 1;
/** Position (ms) that was last actually broadcast to guests — used to detect meaningful
 *  position changes (seeks) regardless of how many intermediate ticks occurred. */
let lastSentPositionMs = 0;

// ---------------------------------------------------------------------------
// Host: create session then connect
// ---------------------------------------------------------------------------

export async function createAndConnect(): Promise<string> {
	const { session_id } = await listenPartyApi.createSession();
	lpSessionId.set(session_id);
	lpRole.set('host');
	_connectHost(session_id);
	return session_id;
}

function _connectHost(sessionId: string) {
	const token = get(authStore).token ?? '';
	ws = new WebSocket(`${BASE_WS}/listen/${sessionId}/ws?token=${encodeURIComponent(token)}`);

	ws.onopen = () => {
		lpConnected.set(true);
		lpPanelOpen.set(true);
		_watchPlayerForHost();
		_hostSendSync();
	};

	ws.onmessage = (ev) => {
		try { _handleHostMessage(JSON.parse(ev.data as string)); }
		catch { /* ignore */ }
	};

	ws.onclose = () => {
		lpConnected.set(false);
		_stopPlayerWatch();
	};

	ws.onerror = () => {};
}

function _handleHostMessage(msg: Record<string, unknown>) {
	switch (msg.type) {
		case 'participants':
			lpParticipants.set((msg.participants as Participant[]) ?? []);
			break;
		case 'participant_joined': {
			const p = msg.participant as Participant;
			if (p) lpParticipants.update((list) =>
				list.find((x) => x.id === p.id) ? list : [...list, p]
			);
			break;
		}
		case 'participant_left': {
			const p = msg.participant as Participant;
			if (p) lpParticipants.update((list) => list.filter((x) => x.id !== p.id));
			break;
		}
	}
}

function _watchPlayerForHost() {
	// Track changes: subscribe directly so guests are notified immediately when
	// the host switches tracks. We send position 0 because the audio engine
	// hasn't started producing position updates for the new track yet.
	let lastHostTrackId = get(currentTrack)?.id ?? '';
	const unsubTrack = currentTrack.subscribe((track) => {
		const tid = track?.id ?? '';
		if (tid && tid !== lastHostTrackId) {
			lastHostTrackId = tid;
			lastSentTrackId = tid;
			lastSentPositionMs = 0;
			// Clear any pending periodic sync so it doesn't fire with stale data.
			if (hostSyncTimer) { clearTimeout(hostSyncTimer); hostSyncTimer = null; }
			// Immediately notify guests of the new track.
			_wsSend({ type: 'sync_state', state: { track_id: tid, position_ms: 0, playing: true } });
		} else {
			lastHostTrackId = tid;
		}
	});

	// Playback state changes (play/pause) — sync immediately.
	const unsubState = playbackState.subscribe((st) => {
		// Skip 'loading' — it's a transient state between tracks. The track
		// subscriber above handles the new-track notification.
		if (st !== 'loading') _hostSendSync();
	});

	// Position updates — compare against last SENT position (not last tick) so
	// that even incremental slider drags are detected as seeks once the
	// cumulative distance from the last broadcast exceeds the threshold.
	// Normal playback advances ~1000 ms/s, so a 3 s threshold avoids false
	// positives while still catching any meaningful seek.
	const unsubPos = playerPositionMs.subscribe((posMs) => {
		if (get(lpRole) !== 'host') return;
		const drift = Math.abs(posMs - lastSentPositionMs);

		// Seek detected: position jumped >3 s from what guests last heard.
		if (drift > 3000) {
			if (hostSyncTimer) { clearTimeout(hostSyncTimer); hostSyncTimer = null; }
			_hostSendSync();
			return;
		}

		// Periodic position sync — keep guests within ~2 s of the host.
		if (hostSyncTimer) return;
		hostSyncTimer = setTimeout(() => {
			hostSyncTimer = null;
			_hostSendSync();
		}, 2000);
	});

	playerUnsubscribers = [unsubTrack, unsubState, unsubPos];
}

function _stopPlayerWatch() {
	for (const unsub of playerUnsubscribers) unsub();
	playerUnsubscribers = [];
	if (hostSyncTimer) { clearTimeout(hostSyncTimer); hostSyncTimer = null; }
	lastSentPositionMs = 0;
}

/** Safe WebSocket send — catches errors so a failed write never breaks
 *  store subscription chains or interval callbacks. */
function _wsSend(payload: Record<string, unknown>): boolean {
	if (!ws || ws.readyState !== WebSocket.OPEN) return false;
	try {
		ws.send(JSON.stringify(payload));
		return true;
	} catch {
		return false;
	}
}

function _hostSendSync() {
	if (!ws || ws.readyState !== WebSocket.OPEN) return;
	const track  = get(currentTrack);
	const state  = get(playbackState);
	if (state === 'loading') return;
	const posMs  = get(playerPositionMs);
	const trackId = track?.id ?? '';
	if (trackId === lastSentTrackId && state === 'idle') return;
	lastSentTrackId = trackId;
	lastSentPositionMs = posMs;
	_wsSend({
		type: 'sync_state',
		state: { track_id: trackId, position_ms: Math.round(posMs), playing: state === 'playing' },
	});
}

export function hostKick(participantId: string) {
	_wsSend({ type: 'kick', participant_id: participantId });
}

export async function hostEndSession() {
	const sessionId = get(lpSessionId);
	disconnect();
	if (sessionId) await listenPartyApi.endSession(sessionId).catch(() => {});
	_resetState();
}

// ---------------------------------------------------------------------------
// Guest: connect to an existing session
// ---------------------------------------------------------------------------

export async function connectAsGuest(sessionId: string, nickname: string): Promise<void> {
	return new Promise((resolve, reject) => {
		ws = new WebSocket(`${BASE_WS}/listen/${sessionId}/ws`);
		lpRole.set('guest');
		lpSessionId.set(sessionId);

		let joined = false;

		ws.onopen = () => ws!.send(JSON.stringify({ type: 'join', nickname }));

		ws.onmessage = (ev) => {
			try {
				const msg: Record<string, unknown> = JSON.parse(ev.data as string);

				if (!joined && msg.type === 'joined' && msg.role === 'guest') {
					joined = true;
					lpConnected.set(true);
					const token = (msg.guest_token as string) ?? '';
					lpGuestToken.set(token);
					lpGuestParticipantId.set((msg.participant_id as string) ?? null);
					if (msg.track_info) lpGuestTrack.set(msg.track_info as TrackInfo);
					if (msg.current_state) {
						_applyGuestSync(
							msg.current_state as SyncState,
							(msg.track_info as TrackInfo) ?? null,
							sessionId,
							token,
							true,
						);
					}
					// Switch to normal message handler.
					ws!.onmessage = (e) => {
						try { _handleGuestMessage(JSON.parse(e.data as string), sessionId); }
						catch { /* ignore */ }
					};
					resolve();
				}
			} catch { /* ignore */ }
		};

		ws.onclose = (ev) => {
			lpConnected.set(false);
			_clearPositionTick();
			if (!joined) reject(new Error('WebSocket closed before join'));
			if (ev.code === 1008) lpKicked.set(true);
		};

		ws.onerror = () => {
			if (!joined) reject(new Error('WebSocket error'));
		};
	});
}

function _handleGuestMessage(msg: Record<string, unknown>, sessionId: string) {
	const guestToken = get(lpGuestToken) ?? '';
	switch (msg.type) {
		case 'sync':
			if (msg.state) {
				if (msg.track_info) lpGuestTrack.set(msg.track_info as TrackInfo);
				_applyGuestSync(
					msg.state as SyncState,
					(msg.track_info as TrackInfo) ?? null,
					sessionId,
					guestToken,
					false,
				);
			}
			break;
		case 'participants':
			lpParticipants.set((msg.participants as Participant[]) ?? []);
			break;
		case 'participant_joined': {
			const p = msg.participant as Participant;
			if (p) lpParticipants.update((list) =>
				list.find((x) => x.id === p.id) ? list : [...list, p]
			);
			break;
		}
		case 'participant_left': {
			const p = msg.participant as Participant;
			if (p) lpParticipants.update((list) => list.filter((x) => x.id !== p.id));
			break;
		}
		case 'kicked':
			lpKicked.set(true);
			disconnect();
			break;
		case 'session_ended':
			lpSessionEnded.set(true);
			disconnect();
			break;
	}
}

function _applyGuestSync(
	state: SyncState,
	trackInfo: TrackInfo | null,
	sessionId: string,
	guestToken: string,
	isJoin: boolean,
) {
	// Update desired play state before any async audio operations so that the
	// loadedmetadata callback reads the latest value rather than a stale closure.
	guestWantsPlaying = state.playing;

	const latencyMs = state.playing ? Math.max(0, Date.now() - state.server_time_ms) : 0;
	const adjustedMs = state.position_ms + latencyMs;

	lpGuestPositionMs.set(adjustedMs);
	lpGuestPlaying.set(state.playing);
	if (trackInfo) {
		lpGuestDurationMs.set(trackInfo.duration_ms);
	}

	if (!state.track_id) return;

	const trackChanged = state.track_id !== lastGuestTrackId;
	lastGuestTrackId = state.track_id;

	const streamUrl = `${API_BASE}/listen/${sessionId}/stream/${state.track_id}?guest_token=${encodeURIComponent(guestToken)}`;

	if (trackChanged || isJoin) {
		_guestPlay(streamUrl, adjustedMs / 1000, state.playing);
	} else if (guestAudio) {
		// Seek if position drifted — covers both playing and paused states.
		const drift = Math.abs(guestAudio.currentTime * 1000 - adjustedMs);
		if (drift > 500) {
			guestAudio.currentTime = adjustedMs / 1000;
		}
		// Sync play/pause state.
		if (state.playing && guestAudio.paused) {
			guestAudio.play().catch(() => {});
		} else if (!state.playing && !guestAudio.paused) {
			guestAudio.pause();
		}
	}

	_startPositionTick(state.playing);
}

export function setGuestVolume(v: number) {
	guestVolume = Math.max(0, Math.min(1, v));
	if (guestAudio) guestAudio.volume = guestVolume;
}

function _ensureGuestAudio(): HTMLAudioElement {
	if (!guestAudio) {
		guestAudio = new Audio();
		guestAudio.volume = guestVolume;
		guestAudio.addEventListener('timeupdate', () => {
			lpGuestPositionMs.set(guestAudio!.currentTime * 1000);
		});
		guestAudio.addEventListener('loadedmetadata', () => {
			if (guestAudio!.duration && isFinite(guestAudio!.duration)) {
				lpGuestDurationMs.set(guestAudio!.duration * 1000);
			}
		});
		guestAudio.addEventListener('ended', () => {
			lpGuestPlaying.set(false);
			_clearPositionTick();
		});
	}
	return guestAudio;
}

function _guestPlay(url: string, startSeconds: number, shouldPlay: boolean) {
	// Store desired state in module variable so later syncs arriving before
	// loadedmetadata fires can override it.
	guestWantsPlaying = shouldPlay;
	const audio = _ensureGuestAudio();
	audio.pause();
	audio.src = url;
	audio.load();
	const onMeta = () => {
		audio.removeEventListener('loadedmetadata', onMeta);
		if (startSeconds > 0) audio.currentTime = startSeconds;
		// Read the module-level flag, not the closure-captured shouldPlay, so
		// that a pause sync received while the audio was loading is respected.
		if (guestWantsPlaying) audio.play().catch(() => {});
		lpGuestPlaying.set(guestWantsPlaying);
	};
	audio.addEventListener('loadedmetadata', onMeta);
}

function _clearPositionTick() {
	if (positionTick) { clearInterval(positionTick); positionTick = null; }
}

function _startPositionTick(playing: boolean) {
	_clearPositionTick();
	if (!playing) return;
	positionTick = setInterval(() => {
		// Keep store in sync between timeupdate events.
		if (guestAudio && !guestAudio.paused) {
			lpGuestPositionMs.set(guestAudio.currentTime * 1000);
		}
	}, 250);
}

// ---------------------------------------------------------------------------
// Shared
// ---------------------------------------------------------------------------

export function disconnect() {
	_stopPlayerWatch();
	_clearPositionTick();
	if (ws) { ws.onclose = null; ws.close(); ws = null; }
	if (guestAudio) { guestAudio.pause(); guestAudio.src = ''; guestAudio = null; }
	lastGuestTrackId = '';
	lastSentTrackId = '';
	lpConnected.set(false);
}

export function leaveSession() {
	disconnect();
	_resetState();
}

function _resetState() {
	lpRole.set(null);
	lpSessionId.set(null);
	lpParticipants.set([]);
	lpPanelOpen.set(false);
	lpConnected.set(false);
	lpGuestToken.set(null);
	lpGuestTrack.set(null);
	lpGuestPositionMs.set(0);
	lpGuestDurationMs.set(0);
	lpGuestPlaying.set(false);
	lpKicked.set(false);
	lpSessionEnded.set(false);
	lpGuestParticipantId.set(null);
	guestWantsPlaying = false;
}
