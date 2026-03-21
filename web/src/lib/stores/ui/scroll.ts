import { writable } from 'svelte/store';

/**
 * Persists scroll positions of the main content area by URL.
 * This is useful when using a custom scroll container instead of the window.
 */
export const scrollPositions = writable<Record<string, number>>({});
