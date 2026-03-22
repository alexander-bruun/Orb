/**
 * Shared helper for building native stream URLs.
 *
 * Both the music player and the unified engine need to resolve a track ID
 * to either an offline file:// path (if downloaded) or an authenticated
 * streaming URL.  This module provides the single canonical implementation.
 */

import { get } from 'svelte/store';
import { getApiBase } from '$lib/api/base';
import { authStore } from '$lib/stores/auth';

/**
 * Build a stream URL for native ExoPlayer.
 * Uses a local file:// path when the track has been downloaded for offline use,
 * otherwise falls back to an authenticated streaming URL.
 */
export async function buildNativeStreamUrl(trackId: string): Promise<string> {
	const base = getApiBase();
	const token = get(authStore).token ?? '';
	try {
		const { invoke } = await import('@tauri-apps/api/core');
		const path = await invoke<string | null>('get_offline_file_path', { trackId });
		if (path) return `file://${path}`;
	} catch { /* fall through to streaming */ }
	return `${base}/stream/${trackId}?token=${encodeURIComponent(token)}`;
}
