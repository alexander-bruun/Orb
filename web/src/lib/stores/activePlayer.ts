import { writable } from 'svelte/store';

/**
 * Tracks which player is currently "in front".
 * Setting this does not pause the other — the layout watches it and triggers
 * the appropriate pause. This lives in its own module to avoid circular deps
 * between audiobookPlayer.ts and player/index.ts.
 */
export type ActivePlayerMode = 'music' | 'audiobook';
export const activePlayer = writable<ActivePlayerMode>('music');
