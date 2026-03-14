import { writable, get } from 'svelte/store';

const CF_KEY = 'orb-crossfade-secs';
const GAPLESS_KEY = 'orb-gapless-enabled';

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

/**
 * Crossfade duration in seconds (1–12).
 * Only active when crossfadeEnabled is true.
 */
export const crossfadeSecs = persistedFloat(CF_KEY, 3, 1, 12);

/**
 * Whether crossfade is enabled.
 * When false, gaplessEnabled is checked instead.
 */
export const crossfadeEnabled = persistedBool('orb-crossfade-enabled', false);

/**
 * When true and crossfade is off, the next track starts sample-accurately
 * as the current one ends (WASM/24-bit path only).
 */
export const gaplessEnabled = persistedBool(GAPLESS_KEY, false);

// ── Native Android crossfade/gapless sync ────────────────────────────────────

function isAndroidNative(): boolean {
	if (typeof window === 'undefined') return false;
	return (window as any).__TAURI_METADATA__?.currentPlatform === 'android';
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
			const cf      = get(crossfadeEnabled);
			const secs    = get(crossfadeSecs);
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
	gaplessEnabled.subscribe(() => scheduleCrossfadeNativeSync());
}

/**
 * Push the current crossfade + gapless settings to the Android media layer.
 * Call this once on app startup to sync the persisted values into ExoPlayer.
 */
export async function syncNativeCrossfade(): Promise<void> {
	scheduleCrossfadeNativeSync();
}
