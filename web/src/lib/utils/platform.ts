import { Capacitor } from '@capacitor/core';

export function isTauri(): boolean {
	return typeof window !== 'undefined' && '__TAURI_INTERNALS__' in window;
}

export function isCapacitor(): boolean {
	return Capacitor.isNativePlatform();
}

/**
 * Returns the current native platform, or 'web' when running in a browser.
 * Values: 'ios' | 'android' | 'web'
 */
export function nativePlatform(): 'ios' | 'android' | 'web' {
	if (isTauri()) return 'web';
	return Capacitor.getPlatform() as 'ios' | 'android' | 'web';
}

/** True when running inside any native shell (Tauri desktop or Capacitor mobile). */
export function isNative(): boolean {
	return isTauri() || isCapacitor();
}
