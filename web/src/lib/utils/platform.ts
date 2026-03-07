export function isTauri(): boolean {
	return typeof window !== 'undefined' && '__TAURI_INTERNALS__' in window;
}

/**
 * Returns the current native platform, or 'web' when running in a browser.
 * On Tauri mobile builds the OS name is exposed via __TAURI_INTERNALS__.
 */
export function nativePlatform(): 'ios' | 'android' | 'web' {
	if (!isTauri()) return 'web';
	// Tauri v2 sets __TAURI_INTERNALS__.metadata.currentWindow.platform on mobile
	try {
		const ua = navigator.userAgent.toLowerCase();
		if (ua.includes('android')) return 'android';
		if (ua.includes('iphone') || ua.includes('ipad')) return 'ios';
	} catch {
		// ignore
	}
	return 'web';
}

/** True when running inside a Tauri shell (desktop or mobile). */
export function isNative(): boolean {
	return isTauri();
}
