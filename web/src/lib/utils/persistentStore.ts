/**
 * Generic localStorage-backed Svelte store factory.
 *
 * Replaces 15+ ad-hoc implementations scattered across the codebase with a
 * single, well-tested helper that handles:
 *  - SSR safety (`browser` guard)
 *  - Consistent try/catch around getItem / setItem
 *  - JSON, string, number, and boolean serialisation
 *  - Optional `update` support
 */

import { writable, type Writable } from 'svelte/store';
import { browser } from '$app/environment';

/** Options for customising serialisation behaviour. */
export interface PersistentStoreOptions<T> {
	/** Custom serialiser (defaults to JSON.stringify). */
	serialize?: (value: T) => string;
	/** Custom deserialiser (defaults to JSON.parse). */
	deserialize?: (raw: string) => T;
}

/**
 * Create a Svelte writable store whose value is automatically persisted to
 * localStorage under the given key.
 *
 * ```ts
 * const volume = createPersistentStore<number>('orb_volume', 1);
 * volume.set(0.5); // also writes to localStorage
 * ```
 */
export function createPersistentStore<T>(
	key: string,
	defaultValue: T,
	options?: PersistentStoreOptions<T>,
): Writable<T> {
	const serialize = options?.serialize ?? JSON.stringify;
	const deserialize = options?.deserialize ?? JSON.parse;

	function load(): T {
		if (!browser) return defaultValue;
		try {
			const raw = localStorage.getItem(key);
			if (raw === null) return defaultValue;
			return deserialize(raw);
		} catch {
			return defaultValue;
		}
	}

	function save(value: T): void {
		if (!browser) return;
		try {
			localStorage.setItem(key, serialize(value));
		} catch {
			/* storage full or blocked */
		}
	}

	const initial = load();
	const { subscribe, set: rawSet, update: rawUpdate } = writable<T>(initial);

	return {
		subscribe,
		set(value: T) {
			rawSet(value);
			save(value);
		},
		update(fn: (current: T) => T) {
			rawUpdate((current) => {
				const next = fn(current);
				save(next);
				return next;
			});
		},
	};
}

/**
 * Convenience: a persistent store for a boolean value.
 * Serialises as 'true'/'false' strings for readability in dev tools.
 */
export function createPersistentBoolStore(key: string, defaultValue: boolean): Writable<boolean> {
	return createPersistentStore<boolean>(key, defaultValue, {
		serialize: (v) => String(v),
		deserialize: (raw) => raw === 'true',
	});
}

/**
 * Convenience: a persistent store for a number with optional min/max clamping.
 */
export function createPersistentNumberStore(
	key: string,
	defaultValue: number,
	min = -Infinity,
	max = Infinity,
): Writable<number> {
	const clamp = (v: number) => Math.max(min, Math.min(max, v));
	return createPersistentStore<number>(key, clamp(defaultValue), {
		serialize: (v) => String(clamp(v)),
		deserialize: (raw) => {
			const v = parseFloat(raw);
			return isNaN(v) ? defaultValue : clamp(v);
		},
	});
}
