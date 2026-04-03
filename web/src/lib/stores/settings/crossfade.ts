import { writable, get } from 'svelte/store';
import { STORAGE_KEYS } from '$lib/constants';

const CF_KEY = STORAGE_KEYS.CROSSFADE_SECS;
const CF_START_KEY = STORAGE_KEYS.CROSSFADE_START_SECS;
const GAPLESS_KEY = STORAGE_KEYS.GAPLESS_ENABLED;
const CF_OUT_CURVE_KEY = STORAGE_KEYS.CROSSFADE_OUT_CURVE;
const CF_IN_CURVE_KEY = STORAGE_KEYS.CROSSFADE_IN_CURVE;

const CURVE_POINT_COUNT = 5;
const CURVE_SAMPLES = 64;

export const DEFAULT_CROSSFADE_OUT_CURVE = [1, 0.82, 0.58, 0.26, 0];
export const DEFAULT_CROSSFADE_IN_CURVE = [0, 0.26, 0.58, 0.82, 1];

function readFloat(key: string, fallback: number): number {
	try {
		const v = parseFloat(localStorage.getItem(key) ?? '');
		return isNaN(v) ? fallback : v;
	} catch {
		return fallback;
	}
}

function readBool(key: string, fallback: boolean): boolean {
	try {
		const v = localStorage.getItem(key);
		return v === null ? fallback : v === 'true';
	} catch {
		return fallback;
	}
}

function clamp01(v: number): number {
	return Math.min(1, Math.max(0, v));
}

function normaliseCurve(val: unknown, fallback: number[]): number[] {
	if (!Array.isArray(val) || val.length !== CURVE_POINT_COUNT) return [...fallback];
	const next = val.map((n, idx) => {
		const parsed = Number(n);
		if (!Number.isFinite(parsed)) return fallback[idx];
		return clamp01(parsed);
	});
	// Keep endpoints anchored for predictable fade boundaries.
	next[0] = fallback[0];
	next[next.length - 1] = fallback[fallback.length - 1];
	return next;
}

function readCurve(key: string, fallback: number[]): number[] {
	try {
		const raw = localStorage.getItem(key);
		if (!raw) return [...fallback];
		return normaliseCurve(JSON.parse(raw), fallback);
	} catch {
		return [...fallback];
	}
}

function persistedFloat(key: string, fallback: number, min: number, max: number) {
	const initial = Math.max(min, Math.min(max, readFloat(key, fallback)));
	const { subscribe, set } = writable<number>(initial);
	return {
		subscribe,
		set(val: number) {
			const clamped = Math.max(min, Math.min(max, val));
			set(clamped);
			try { localStorage.setItem(key, String(clamped)); } catch { /* ignore */ }
		}
	};
}

function persistedBool(key: string, fallback: boolean) {
	const initial = readBool(key, fallback);
	const { subscribe, set } = writable<boolean>(initial);
	return {
		subscribe,
		set(val: boolean) {
			set(val);
			try { localStorage.setItem(key, String(val)); } catch { /* ignore */ }
		}
	};
}

function persistedCurve(key: string, fallback: number[]) {
	const initial = readCurve(key, fallback);
	const { subscribe, set } = writable<number[]>(initial);
	return {
		subscribe,
		set(val: number[]) {
			const next = normaliseCurve(val, fallback);
			set(next);
			try { localStorage.setItem(key, JSON.stringify(next)); } catch { /* ignore */ }
		}
	};
}

/**
 * Crossfade duration in seconds (1–12).
 * Only active when crossfadeEnabled is true.
 */
export const crossfadeSecs = persistedFloat(CF_KEY, 3, 1, 12);

/**
 * Seconds before the end of the outgoing track when crossfade starts.
 * This lets users move the overlap earlier/later independently of duration.
 */
export const crossfadeStartSecs = persistedFloat(CF_START_KEY, 3, 0.5, 20);

/**
 * Whether crossfade is enabled.
 * Can be enabled together with gapless playback.
 */
export const crossfadeEnabled = persistedBool(STORAGE_KEYS.CROSSFADE_ENABLED, false);

/**
 * When true, the next track is scheduled sample-accurately at track end.
 * If crossfade is also on, this acts as a fallback when overlap cannot be
 * scheduled in time (WASM/24-bit path only).
 */
export const gaplessEnabled = persistedBool(GAPLESS_KEY, false);

/**
 * Outgoing-track fade curve. 5 points from t=0..1 where 1=full gain, 0=silent.
 * Endpoints are fixed at [1, 0].
 */
export const crossfadeOutCurve = persistedCurve(CF_OUT_CURVE_KEY, DEFAULT_CROSSFADE_OUT_CURVE);

/**
 * Incoming-track fade curve. 5 points from t=0..1 where 1=full gain, 0=silent.
 * Endpoints are fixed at [0, 1].
 */
export const crossfadeInCurve = persistedCurve(CF_IN_CURVE_KEY, DEFAULT_CROSSFADE_IN_CURVE);

export function resetCrossfadeCurves(): void {
	crossfadeOutCurve.set(DEFAULT_CROSSFADE_OUT_CURVE);
	crossfadeInCurve.set(DEFAULT_CROSSFADE_IN_CURVE);
}

/**
 * Build a dense WebAudio gain curve from the 5 editable control points.
 */
export function buildCrossfadeGainCurve(points: number[], fallback: number[]): Float32Array {
	const safe = normaliseCurve(points, fallback);
	const out = new Float32Array(CURVE_SAMPLES);
	const segments = safe.length - 1;
	for (let i = 0; i < CURVE_SAMPLES; i++) {
		const t = i / (CURVE_SAMPLES - 1);
		const pos = t * segments;
		const idx = Math.min(segments - 1, Math.floor(pos));
		const localT = pos - idx;
		const a = safe[idx];
		const b = safe[idx + 1];
		out[i] = a + (b - a) * localT;
	}
	return out;
}

// ── Native Android crossfade/gapless sync ────────────────────────────────────

function isAndroidNative(): boolean {
	if (typeof window === 'undefined') return false;
	return (window as unknown as { __TAURI_METADATA__?: { currentPlatform?: string } }).__TAURI_METADATA__?.currentPlatform === 'android';
}

/** Debounce handle so rapid slider drags don't flood the JNI bridge. */
let nativeSyncTimer: ReturnType<typeof setTimeout> | null = null;

function scheduleCrossfadeNativeSync() {
	if (!isAndroidNative()) return;
	if (nativeSyncTimer) clearTimeout(nativeSyncTimer);
	nativeSyncTimer = setTimeout(async () => {
		nativeSyncTimer = null;
		try {
			const { invoke } = await import('@tauri-apps/api/core');
			const cf = get(crossfadeEnabled);
			const secs = get(crossfadeSecs);
			const gapless = get(gaplessEnabled);
			await invoke('set_crossfade_settings', { enabled: cf, secs });
			await invoke('set_gapless_enabled', { enabled: gapless });
		} catch { /* best-effort */ }
	}, 100);
}

// Subscribe to each store so changes made in the settings UI propagate to
// ExoPlayer automatically. The subscriptions also fire on first subscribe
// (Svelte store guarantee), but we guard with isAndroidNative() so the
// imports are never attempted on non-Android targets.
	if (typeof window !== 'undefined') {
	crossfadeEnabled.subscribe(() => scheduleCrossfadeNativeSync());
	crossfadeSecs.subscribe(() => scheduleCrossfadeNativeSync());
	crossfadeStartSecs.subscribe(() => scheduleCrossfadeNativeSync());
	gaplessEnabled.subscribe(() => scheduleCrossfadeNativeSync());
}

/**
 * Push the current crossfade + gapless settings to the Android media layer.
 * Call this once on app startup to sync the persisted values into ExoPlayer.
 */
export async function syncNativeCrossfade(): Promise<void> {
	scheduleCrossfadeNativeSync();
}
