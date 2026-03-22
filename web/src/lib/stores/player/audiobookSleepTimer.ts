/**
 * Audiobook sleep timer logic.
 *
 * Extracted from audiobookPlayer.ts for modularity.
 */

import { writable } from 'svelte/store';

// Sleep timer: minutes remaining (0 = off)
export const sleepTimerMins = writable(0);

let _sleepTimeout: ReturnType<typeof setTimeout> | null = null;

/**
 * Set or clear the sleep timer. When the timer fires it calls the
 * provided `onExpire` callback (which should pause audiobook playback).
 */
export function setSleepTimer(minutes: number, onExpire: () => void) {
	if (_sleepTimeout !== null) {
		clearTimeout(_sleepTimeout);
		_sleepTimeout = null;
	}
	sleepTimerMins.set(minutes);
	if (minutes > 0) {
		_sleepTimeout = setTimeout(() => {
			onExpire();
			sleepTimerMins.set(0);
		}, minutes * 60_000);
	}
}

export const SLEEP_PRESETS = [5, 10, 15, 20, 30, 45, 60];
