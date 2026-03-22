/**
 * Listen-party store — manages the WebSocket connection for both host and
 * guest roles and provides reactive state for the UI.
 */
import { writable, get, derived } from 'svelte/store';
import { listenPartyApi } from '$lib/api/listenParty';
import { authStore } from '$lib/stores/auth';
import {
	currentTrack,
	playbackState,
	positionMs as playerPositionMs,
} from '$lib/stores/player';
import {
	currentAudiobook,
	abPlaybackState,
	abPositionMs,
	abCurrentChapter,
} from '$lib/stores/player/audiobookPlayer';
import { activePlayer } from '$lib/stores/player/engine';

import { getApiBase, getWsBase } from '$lib/api/base';
import { TIMINGS } from '$lib/constants';

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

export interface AudiobookChapter {
	id: string;
	audiobook_id: string;
	title: string;
	start_ms: number;
	end_ms: number;
	chapter_num: number;
	file_key?: string;
}

export interface AudiobookInfo {
	id: string;
	title: string;
	author_name: string;
	duration_ms: number;
	chapters?: AudiobookChapter[];
}

export interface SyncState {
	item_type: 'track' | 'audiobook';
	track_id?: string; // compat
	item_id: string;
	chapter_id?: string;
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

/** Host-only: whether access code protection is enabled */
export const lpCodeEnabled = writable(false);
/** Host-only: the current 4-digit access code (null when disabled) */
export const lpAccessCode  = writable<string | null>(null);

/** Guest-only: auth token for stream URLs */
export const lpGuestToken     = writable<string | null>(null);
/** Guest-only: currently playing item type */
export const lpGuestItemType  = writable<'track' | 'audiobook' | null>(null);
/** Guest-only: currently playing track metadata */
export const lpGuestTrack     = writable<TrackInfo | null>(null);
/** Guest-only: currently playing audiobook metadata */
export const lpGuestAudiobook = writable<AudiobookInfo | null>(null);
/** Guest-only: latest position in ms (updated from audio + ticks) */
export const lpGuestPositionMs = writable(0);
/** Guest-only: whether the host is playing */
export const lpGuestPlaying   = writable(false);
/** Guest-only: duration ms of the current track/book */
export const lpGuestDurationMs = writable(0);
/** True when the guest has been kicked */
export const lpKicked         = writable(false);
/** True when the host ended the session */
export const lpSessionEnded   = writable(false);
/** Guest-only: the participant ID assigned by the server (used to filter self from participants list) */
export const lpGuestParticipantId = writable<string | null>(null);

// Guest-only derived stores for chapter-aware progress
export const lpGuestCurrentChapter = derived(
	[lpGuestAudiobook, lpGuestPositionMs],
	([$book, $pos]) => {
		if (!$book?.chapters?.length) return null;
		let current: AudiobookChapter | null = null;
		for (const ch of $book.chapters) {
			if ($pos >= ch.start_ms) current = ch;
			else break;
		}
		return current;
	}
);

export const lpGuestChapterProgress = derived(
	[lpGuestAudiobook, lpGuestPositionMs, lpGuestCurrentChapter],
	([$book, $pos, $chapter]) => {
		if (!$chapter) return 0;
		const nextChapter = $book?.chapters?.find(ch => ch.start_ms > $chapter.start_ms);
		const chapterDurationMs = nextChapter ? nextChapter.start_ms - $chapter.start_ms : ($book?.duration_ms ?? 0) - $chapter.start_ms;
		const posInChapter = $pos - $chapter.start_ms;
		return chapterDurationMs > 0 ? Math.max(0, Math.min(100, (posInChapter / chapterDurationMs) * 100)) : 0;
	}
);

export const lpGuestPreviousChapter = derived(
	[lpGuestAudiobook, lpGuestCurrentChapter],
	([$book, $current]) => {
		if (!$book?.chapters || !$current) return null;
		const idx = $book.chapters.findIndex(ch => ch.id === $current.id);
		return idx > 0 ? $book.chapters[idx - 1] : null;
	}
);

export const lpGuestNextChapter = derived(
	[lpGuestAudiobook, lpGuestCurrentChapter],
	([$book, $current]) => {
		if (!$book?.chapters || !$current) return null;
		const idx = $book.chapters.findIndex(ch => ch.id === $current.id);
		return idx >= 0 && idx < $book.chapters.length - 1 ? $book.chapters[idx + 1] : null;
	}
);

// ---------------------------------------------------------------------------
// Internal state
// ---------------------------------------------------------------------------

let ws: WebSocket | null = null;
let guestAudio: HTMLAudioElement | null = null;
let positionTick: ReturnType<typeof setInterval> | null = null;
let hostSyncTimer: ReturnType<typeof setTimeout> | null = null;
let playerUnsubscribers: Array<() => void> = [];
let lastSentItemId = '';
let lastSentChapterId = '';
let lastGuestItemId = '';
let lastGuestChapterId = '';
let guestChapterStartMs = 0;
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
	ws = new WebSocket(`${getWsBase()}/listen/${sessionId}/ws?token=${encodeURIComponent(token)}`);

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
		case 'joined':
			// Restore code state when host reconnects (server echoes current session state)
			lpCodeEnabled.set(!!(msg.code_enabled));
			lpAccessCode.set(msg.code_enabled && msg.access_code ? (msg.access_code as string) : null);
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
	}
}

function _watchPlayerForHost() {
	// Track / Audiobook changes
	const unsubActive = activePlayer.subscribe(() => _hostSendSync());

	let lastHostItemId = '';
	const unsubTrack = currentTrack.subscribe((track) => {
		if (get(activePlayer) !== 'music') return;
		const tid = track?.id ?? '';
		if (tid && tid !== lastHostItemId) {
			lastHostItemId = tid;
			lastSentItemId = tid;
			lastSentPositionMs = 0;
			if (hostSyncTimer) { clearTimeout(hostSyncTimer); hostSyncTimer = null; }
			_wsSend({ type: 'sync_state', state: { item_type: 'track', item_id: tid, position_ms: 0, playing: true } });
		}
	});

	const unsubBook = currentAudiobook.subscribe((book) => {
		if (get(activePlayer) !== 'audiobook') return;
		const bid = book?.id ?? '';
		if (bid && bid !== lastHostItemId) {
			lastHostItemId = bid;
			lastSentItemId = bid;
			lastSentPositionMs = 0;
			if (hostSyncTimer) { clearTimeout(hostSyncTimer); hostSyncTimer = null; }
			_wsSend({ type: 'sync_state', state: { item_type: 'audiobook', item_id: bid, position_ms: 0, playing: true } });
		}
	});
	
	const unsubChapter = abCurrentChapter.subscribe((ch) => {
		if (get(activePlayer) !== 'audiobook') return;
		const cid = ch?.id ?? '';
		if (cid && cid !== lastSentChapterId) {
			_hostSendSync();
		}
	});

	// Playback state changes (play/pause) — sync immediately.
	const unsubMusicState = playbackState.subscribe((st) => {
		if (get(activePlayer) !== 'music') return;
		if (st !== 'loading') _hostSendSync();
	});
	const unsubABState = abPlaybackState.subscribe((st) => {
		if (get(activePlayer) !== 'audiobook') return;
		if (st !== 'loading') _hostSendSync();
	});

	// Position updates
	const unsubMusicPos = playerPositionMs.subscribe((posMs) => {
		if (get(activePlayer) !== 'music') return;
		_handlePosDrift(posMs);
	});
	const unsubABPos = abPositionMs.subscribe((posMs) => {
		if (get(activePlayer) !== 'audiobook') return;
		_handlePosDrift(posMs);
	});

	playerUnsubscribers = [unsubActive, unsubTrack, unsubBook, unsubChapter, unsubMusicState, unsubABState, unsubMusicPos, unsubABPos];
}

function _handlePosDrift(posMs: number) {
	if (get(lpRole) !== 'host') return;
	const drift = Math.abs(posMs - lastSentPositionMs);

	if (drift > TIMINGS.LISTEN_PARTY_DRIFT_THRESHOLD) {
		if (hostSyncTimer) { clearTimeout(hostSyncTimer); hostSyncTimer = null; }
		_hostSendSync();
		return;
	}

	if (hostSyncTimer) return;
	hostSyncTimer = setTimeout(() => {
		hostSyncTimer = null;
		_hostSendSync();
	}, TIMINGS.LISTEN_PARTY_HOST_SYNC_DEBOUNCE);
}

function _stopPlayerWatch() {
	for (const unsub of playerUnsubscribers) unsub();
	playerUnsubscribers = [];
	if (hostSyncTimer) { clearTimeout(hostSyncTimer); hostSyncTimer = null; }
	lastSentPositionMs = 0;
	lastSentItemId = '';
	lastSentChapterId = '';
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
	const mode = get(activePlayer);
	
	if (mode === 'music') {
		const track  = get(currentTrack);
		const state  = get(playbackState);
		if (state === 'loading') return;
		const posMs  = get(playerPositionMs);
		const trackId = track?.id ?? '';
		if (trackId === lastSentItemId && state === 'idle') return;
		lastSentItemId = trackId;
		lastSentPositionMs = posMs;
		_wsSend({
			type: 'sync_state',
			state: { item_type: 'track', item_id: trackId, position_ms: Math.round(posMs), playing: state === 'playing' },
		});
	} else {
		const book = get(currentAudiobook);
		const chapter = get(abCurrentChapter);
		const state = get(abPlaybackState);
		if (state === 'loading') return;
		const posMs = get(abPositionMs);
		const bookId = book?.id ?? '';
		const chapterId = chapter?.id ?? '';
		if (bookId === lastSentItemId && chapterId === lastSentChapterId && state === 'idle') return;
		lastSentItemId = bookId;
		lastSentChapterId = chapterId;
		lastSentPositionMs = posMs;
		_wsSend({
			type: 'sync_state',
			state: {
				item_type: 'audiobook',
				item_id: bookId,
				chapter_id: chapterId,
				position_ms: Math.round(posMs),
				playing: state === 'playing'
			},
		});
	}
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

/**
 * Enable or regenerate the 4-digit access code for the current session.
 * Updates lpCodeEnabled and lpAccessCode stores with the new code.
 */
export async function hostEnableCode(): Promise<void> {
	const sessionId = get(lpSessionId);
	if (!sessionId) return;
	const { code } = await listenPartyApi.enableCode(sessionId);
	lpCodeEnabled.set(true);
	lpAccessCode.set(code);
}

/**
 * Disable access code protection for the current session.
 */
export async function hostDisableCode(): Promise<void> {
	const sessionId = get(lpSessionId);
	if (!sessionId) return;
	await listenPartyApi.disableCode(sessionId);
	lpCodeEnabled.set(false);
	lpAccessCode.set(null);
}

// ---------------------------------------------------------------------------
// Guest: connect to an existing session
// ---------------------------------------------------------------------------

export async function connectAsGuest(sessionId: string, nickname: string, code?: string): Promise<void> {
	return new Promise((resolve, reject) => {
		ws = new WebSocket(`${getWsBase()}/listen/${sessionId}/ws`);
		lpRole.set('guest');
		lpSessionId.set(sessionId);

		let joined = false;

		ws.onopen = () => ws!.send(JSON.stringify({ type: 'join', nickname, ...(code ? { code } : {}) }));

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
					if (msg.audiobook_info) lpGuestAudiobook.set(msg.audiobook_info as AudiobookInfo);
					
					if (msg.current_state) {
						_applyGuestSync(
							msg.current_state as SyncState,
							(msg.track_info as TrackInfo) ?? null,
							(msg.audiobook_info as AudiobookInfo) ?? null,
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
			if (!joined) {
				// Code 1008 (Policy Violation) is used by the server for invalid access code.
				if (ev.code === 1008 && ev.reason === 'invalid access code') {
					reject(new Error('invalid_code'));
				} else {
					reject(new Error('WebSocket closed before join'));
				}
			} else if (ev.code === 1008) {
				// Only mark as kicked after the guest was already in the session.
				lpKicked.set(true);
			}
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
				if (msg.audiobook_info) lpGuestAudiobook.set(msg.audiobook_info as AudiobookInfo);
				
				_applyGuestSync(
					msg.state as SyncState,
					(msg.track_info as TrackInfo) ?? null,
					(msg.audiobook_info as AudiobookInfo) ?? null,
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
	audiobookInfo: AudiobookInfo | null,
	sessionId: string,
	guestToken: string,
	isJoin: boolean,
) {
	// Compat
	if (!state.item_id && state.track_id) {
		state.item_id = state.track_id;
		state.item_type = 'track';
	}

	// Update desired play state before any async audio operations so that the
	// loadedmetadata callback reads the latest value rather than a stale closure.
	guestWantsPlaying = state.playing;

	const latencyMs = state.playing ? Math.max(0, Date.now() - state.server_time_ms) : 0;
	const adjustedMs = state.position_ms + latencyMs;

	lpGuestPositionMs.set(adjustedMs);
	lpGuestPlaying.set(state.playing);
	lpGuestItemType.set(state.item_type);

	if (state.item_type === 'track' && trackInfo) {
		lpGuestDurationMs.set(trackInfo.duration_ms);
		guestChapterStartMs = 0;
	} else if (state.item_type === 'audiobook' && audiobookInfo) {
		lpGuestDurationMs.set(audiobookInfo.duration_ms);
		guestChapterStartMs = 0;
	}

	if (!state.item_id) return;

	const itemChanged = state.item_id !== lastGuestItemId || state.chapter_id !== lastGuestChapterId;
	lastGuestItemId = state.item_id;
	lastGuestChapterId = state.chapter_id ?? '';

	// If streaming a chapter, calculate the relative seek position and offset.
	let targetSeconds = adjustedMs / 1000;
	if (state.item_type === 'audiobook' && state.chapter_id) {
		const book = audiobookInfo || get(lpGuestAudiobook);
		if (book?.chapters) {
			const ch = book.chapters.find((c) => c.id === state.chapter_id);
			if (ch) {
				guestChapterStartMs = ch.start_ms;
				targetSeconds = Math.max(0, (adjustedMs - ch.start_ms) / 1000);
			}
		}
	}

	let streamUrl = '';
	if (state.item_type === 'track') {
		streamUrl = `${getApiBase()}/listen/${sessionId}/stream/${state.item_id}?guest_token=${encodeURIComponent(guestToken)}`;
	} else {
		if (state.chapter_id) {
			streamUrl = `${getApiBase()}/listen/${sessionId}/stream/audiobook/chapter/${state.chapter_id}?guest_token=${encodeURIComponent(guestToken)}`;
		} else {
			streamUrl = `${getApiBase()}/listen/${sessionId}/stream/audiobook/${state.item_id}?guest_token=${encodeURIComponent(guestToken)}`;
		}
	}

	if (itemChanged || isJoin) {
		_guestPlay(streamUrl, targetSeconds, state.playing);
	} else if (guestAudio) {
		// Seek if position drifted — covers both playing and paused states.
		const drift = Math.abs(guestAudio.currentTime * 1000 - targetSeconds * 1000);
		if (drift > 500) {
			guestAudio.currentTime = targetSeconds;
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
			lpGuestPositionMs.set(guestChapterStartMs + guestAudio!.currentTime * 1000);
		});
		guestAudio.addEventListener('loadedmetadata', () => {
			if (guestAudio!.duration && isFinite(guestAudio!.duration)) {
				// For tracks/single-file books, we can use the element duration.
				// For multi-file audiobooks, lpGuestDurationMs is set from the book metadata.
				if (get(lpGuestItemType) === 'track') {
					lpGuestDurationMs.set(guestAudio!.duration * 1000);
				}
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
			lpGuestPositionMs.set(guestChapterStartMs + guestAudio.currentTime * 1000);
		}
	}, TIMINGS.POSITION_TICK);
}

// ---------------------------------------------------------------------------
// Shared
// ---------------------------------------------------------------------------

export function disconnect() {
	_stopPlayerWatch();
	_clearPositionTick();
	if (ws) { ws.onclose = null; ws.close(); ws = null; }
	if (guestAudio) { guestAudio.pause(); guestAudio.src = ''; guestAudio = null; }
	lastGuestItemId = '';
	lastSentItemId = '';
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
	lpCodeEnabled.set(false);
	lpAccessCode.set(null);
	lpGuestToken.set(null);
	lpGuestItemType.set(null);
	lpGuestTrack.set(null);
	lpGuestAudiobook.set(null);
	lpGuestPositionMs.set(0);
	lpGuestDurationMs.set(0);
	lpGuestPlaying.set(false);
	lpKicked.set(false);
	lpSessionEnded.set(false);
	lpGuestParticipantId.set(null);
	guestWantsPlaying = false;
}
