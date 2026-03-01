import { browser } from '$app/environment';

const STORAGE_KEY = 'orb_server_url';

/**
 * Returns the current API base URL.
 *
 * Resolution order:
 * 1. localStorage `orb_server_url` (runtime, set in Settings for Tauri apps)
 * 2. VITE_API_BASE env var (build-time, for self-hosted web deployments)
 * 3. '/api' (default, works with reverse proxy)
 */
export function getApiBase(): string {
	if (browser) {
		const stored = localStorage.getItem(STORAGE_KEY);
		if (stored) return stored;
	}
	return (import.meta.env.VITE_API_BASE as string | undefined) ?? '/api';
}

/** Derives a WebSocket base URL from the current API base. */
export function getWsBase(): string {
	const base = getApiBase();
	if (typeof location === 'undefined') return 'ws://localhost:8080/api';
	const proto = location.protocol === 'https:' ? 'wss:' : 'ws:';
	if (base.startsWith('http')) return base.replace(/^https?:/, proto);
	return `${proto}//${location.host}${base}`;
}

export function setServerUrl(url: string): void {
	if (browser) {
		if (url) {
			localStorage.setItem(STORAGE_KEY, url);
		} else {
			localStorage.removeItem(STORAGE_KEY);
		}
	}
}

export function getServerUrl(): string {
	if (browser) return localStorage.getItem(STORAGE_KEY) ?? '';
	return '';
}
