/**
 * Sleep timer for the music player.
 *
 * Features:
 *  - Presets: 15 / 30 / 60 minutes, or end-of-track
 *  - Countdown display (minutes remaining)
 *  - Fade-out over the last 30 seconds before pause
 *  - Restores volume after pause or cancel
 */

import { writable, get } from 'svelte/store';
import { audioEngine } from '$lib/audio/engine';
import { volume } from './musicPlayer';
import { pauseLocal } from './musicPlayer';

export type MusicSleepPreset = 15 | 30 | 60 | 'end_of_track';
export const MUSIC_SLEEP_PRESETS: MusicSleepPreset[] = [15, 30, 60, 'end_of_track'];

/** Current active preset, or null when the timer is off. */
export const musicSleepPreset = writable<MusicSleepPreset | null>(null);

/** Milliseconds remaining until the timer fires (0 when off). Updated every second. */
export const musicSleepMsRemaining = writable(0);

/** True while the fade-out is in progress. */
export const musicSleepFading = writable(false);

const FADE_DURATION_MS = 30_000;
const TICK_MS = 250;

let _expiresAt = 0;
let _tickInterval: ReturnType<typeof setInterval> | null = null;
let _savedVolume = 1;
let _endOfTrackArmed = false;

function _clearTimerState() {
	if (_tickInterval !== null) {
		clearInterval(_tickInterval);
		_tickInterval = null;
	}
	_expiresAt = 0;
	_endOfTrackArmed = false;
	musicSleepMsRemaining.set(0);
	musicSleepFading.set(false);
}

function _expire() {
	_clearTimerState();
	musicSleepPreset.set(null);
	// Restore volume before pausing so next play starts at full volume.
	audioEngine.setVolume(_savedVolume);
	pauseLocal();
}

function _tick() {
	if (_expiresAt === 0) return;

	const remaining = _expiresAt - Date.now();

	if (remaining <= 0) {
		musicSleepMsRemaining.set(0);
		_expire();
		return;
	}

	musicSleepMsRemaining.set(remaining);

	// Fade out in the last FADE_DURATION_MS
	if (remaining <= FADE_DURATION_MS) {
		musicSleepFading.set(true);
		const ratio = remaining / FADE_DURATION_MS;
		audioEngine.setVolume(_savedVolume * ratio);
	}
}

/**
 * Set the sleep timer. Pass null to clear.
 * 'end_of_track' arms a one-shot that fires at the next track end.
 */
export function setMusicSleepTimer(preset: MusicSleepPreset): void {
	// Cancel any existing timer first.
	_clearTimerState();
	// Restore volume in case a previous fade was in progress.
	audioEngine.setVolume(get(volume));

	_savedVolume = get(volume);
	musicSleepPreset.set(preset);
	musicSleepFading.set(false);

	if (preset === 'end_of_track') {
		_endOfTrackArmed = true;
		// Display a large remaining time so the indicator shows "EOT".
		musicSleepMsRemaining.set(-1);
		return;
	}

	_expiresAt = Date.now() + preset * 60_000;
	musicSleepMsRemaining.set(preset * 60_000);

	_tickInterval = setInterval(_tick, TICK_MS);
}

/** Cancel the sleep timer and restore volume. */
export function clearMusicSleepTimer(): void {
	_clearTimerState();
	musicSleepPreset.set(null);
	// Restore volume in case a fade was in progress.
	audioEngine.setVolume(get(volume));
}

/**
 * Call this from musicPlayer.ts when a track ends (onTrackEnd).
 * If the timer is in end_of_track mode this will pause; otherwise no-op.
 */
export function notifyTrackEnd(): void {
	if (!_endOfTrackArmed) return;
	_endOfTrackArmed = false;
	_clearTimerState();
	musicSleepPreset.set(null);
	audioEngine.setVolume(_savedVolume);
	pauseLocal();
}
