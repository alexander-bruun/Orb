import { writable } from 'svelte/store';

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
