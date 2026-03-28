/**
 * Centralised magic numbers, localStorage keys, and IndexedDB names.
 *
 * Import from '$lib/constants' instead of hard-coding values so that
 * every occurrence can be found (and changed) in one place.
 */

// ── Timing constants (milliseconds) ──────────────────────────────────────────

export const TIMINGS = {
	/** Position-tracking / shadow-tick interval for smooth progress bars. */
	POSITION_TICK: 250,
	/** Debounce before persisting player state after a position change. */
	POSITION_SAVE_INTERVAL: 1_000,
	/** Debounce before persisting non-position player state changes. */
	STATE_SAVE_DEBOUNCE: 200,
	/** MediaSession position-state sync throttle. */
	POSITION_STATE_SYNC: 500,
	/** Device heartbeat interval (keep-alive with the server). */
	HEARTBEAT_INTERVAL: 30_000,
	/** SSE reconnect delay after an error. */
	SSE_RECONNECT_DELAY: 5_000,
	/** Connectivity health-check timeout. */
	HEALTH_CHECK_TIMEOUT: 3_000,
	/** Periodic connectivity check interval. */
	CONNECTIVITY_CHECK_INTERVAL: 15_000,
	/** Ingest status polling interval. */
	INGEST_POLL_INTERVAL: 20_000,
	/** Audiobook progress auto-save interval. */
	AUDIOBOOK_SAVE_INTERVAL: 10_000,
	/** Guard window after a seek to ignore stale position polls. */
	SEEK_GUARD: 500,
	/** Delay before fetching ingest status after completion. */
	INGEST_COMPLETE_FETCH_DELAY: 600,
	/** Delay before connecting SSE after triggering a scan. */
	INGEST_SSE_CONNECT_DELAY: 150,
	/** Delay for Chromecast retry. */
	CAST_RETRY_DELAY: 2_000,
	/** Listen-party host sync debounce. */
	LISTEN_PARTY_HOST_SYNC_DEBOUNCE: 2_000,
	/** Listen-party host sync drift threshold. */
	LISTEN_PARTY_DRIFT_THRESHOLD: 3_000,
	/** Native crossfade sync debounce (see crossfade.ts). */
	NATIVE_CROSSFADE_SYNC_DELAY: 200,
} as const;

// ── localStorage key constants ───────────────────────────────────────────────

export const STORAGE_KEYS = {
	AUTH: 'orb_auth',
	SERVER_URL: 'orb_server_url',
	THEME: 'orb_theme',
	AVATAR: 'orb_avatar',
	WAVEFORM_ENABLED: 'orb_waveform_enabled',
	PLAYER_STATE: 'orb-player-state-v1',
	DEVICE_ID: 'orb_device_id',
	NATIVE_DEVICE_ID: 'orb_native_device_id',
	DEVICE_NAME: 'orb_device_name',
	AUDIO_OUTPUT_ID: 'orb_audio_output_id',
	CROSSFADE_SECS: 'orb-crossfade-secs',
	CROSSFADE_ENABLED: 'orb-crossfade-enabled',
	GAPLESS_ENABLED: 'orb-gapless-enabled',
	VISUALIZER_PREFS: 'orb-visualizer-prefs',
	VISUALIZER_BUTTON_ENABLED: 'orb_visualizer_button_enabled',
	COVER_EXPANDED: 'orb:cover-expanded',
	SAVED_SEARCH_FILTERS: 'orb:savedSearchFilters',
	DOWNLOADS_META: 'orb-downloads-v1',
	AUDIOBOOK_PROGRESS: 'orb-ab-progress-v1',
	AUDIOBOOK_META: 'orb-ab-meta-v1',
	BOTTOM_BAR_SECONDARY: 'orb_bottom_bar_secondary',
	LISTEN_ALONG_ENABLED: 'orb_listen_along_enabled',
} as const;

// ── IndexedDB constants ──────────────────────────────────────────────────────

export const IDB = {
	NAME: 'orb-offline-audio',
	VERSION: 2,
	STORE_BLOBS: 'blobs',
	STORE_LYRICS: 'lyrics',
	STORE_WAVEFORM: 'waveform',
} as const;
