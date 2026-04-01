/**
 * Spotify playlist import — server-side OAuth flow.
 *
 * The server admin sets SPOTIFY_CLIENT_ID + SPOTIFY_CLIENT_SECRET once.
 * Users just click "Connect Spotify" and log in through Spotify's own UI.
 *
 * Flow:
 *   startSpotifyLogin()        → redirects to GET /auth/spotify
 *   [Spotify login UI]
 *   GET /auth/spotify/callback → server exchanges code, redirects browser to
 *   /playlists#spotify_token=TOKEN
 *   handleSpotifyReturn()      → reads token from fragment, stores in sessionStorage
 */

import type { Track } from '$lib/types';
import { library } from '$lib/api/library';
import { apiFetch } from '$lib/api/client';
import { getApiBase } from '$lib/api/base';

const TOKEN_KEY = 'spotify_access_token';

// ── Backend capability check ──────────────────────────────────────────────────

export async function isSpotifyConfigured(): Promise<boolean> {
	try {
		const res = await apiFetch<{ enabled: boolean }>('/auth/spotify/config');
		return res.enabled;
	} catch {
		return false;
	}
}

// ── Session helpers ───────────────────────────────────────────────────────────

export function getSpotifyToken(): string | null {
	return sessionStorage.getItem(TOKEN_KEY);
}

export function isSpotifyAuthorized(): boolean {
	return !!getSpotifyToken();
}

export function clearSpotifySession(): void {
	sessionStorage.removeItem(TOKEN_KEY);
}

// ── OAuth flow ────────────────────────────────────────────────────────────────

/** Redirect the browser to the server's Spotify auth endpoint. */
export function startSpotifyLogin(): void {
	window.location.href = `${getApiBase()}/auth/spotify`;
}

/**
 * Call this on page load. Reads the token from the URL fragment
 * (#spotify_token=...) left by the server callback redirect.
 * Returns true if a token was found and stored.
 * Throws if the server returned an error (?spotify_error=...).
 */
export function handleSpotifyReturn(): boolean {
	// Check for error query param first
	const params = new URLSearchParams(window.location.search);
	const err = params.get('spotify_error');
	if (err) {
		// Clean the URL
		params.delete('spotify_error');
		const clean = window.location.pathname + (params.toString() ? '?' + params : '');
		window.history.replaceState({}, '', clean);
		throw new Error(err);
	}

	// Check fragment for token
	const hash = window.location.hash.slice(1);
	const hashParams = new URLSearchParams(hash);
	const token = hashParams.get('spotify_token');
	if (!token) return false;

	sessionStorage.setItem(TOKEN_KEY, token);
	// Remove the fragment so it doesn't leak into logs/history
	window.history.replaceState({}, '', window.location.pathname + window.location.search);
	return true;
}

// ── Spotify API calls ─────────────────────────────────────────────────────────

export interface SpotifyPlaylistSummary {
	id: string;
	name: string;
	description: string;
	trackCount: number;
	imageUrl?: string;
}

export interface SpotifyTrack {
	title: string;
	artist: string;
	album: string;
	durationMs: number;
}

async function spotifyGet<T>(path: string): Promise<T> {
	const token = getSpotifyToken();
	if (!token) throw new Error('Not connected to Spotify.');
	const res = await fetch(`https://api.spotify.com/v1${path}`, {
		headers: { Authorization: `Bearer ${token}` },
	});
	if (res.status === 401) {
		clearSpotifySession();
		throw new Error('Spotify session expired. Please reconnect.');
	}
	if (!res.ok) throw new Error(`Spotify API error: ${res.status}`);
	return res.json();
}

export async function fetchUserPlaylists(): Promise<SpotifyPlaylistSummary[]> {
	const data = await spotifyGet<{ items: SpotifyApiPlaylist[] }>('/me/playlists?limit=50');
	return (data.items ?? []).map((p) => ({
		id: p.id,
		name: p.name,
		description: p.description ?? '',
		trackCount: p.tracks?.total ?? 0,
		imageUrl: p.images?.[0]?.url,
	}));
}

export async function fetchPlaylistTracks(playlistId: string): Promise<SpotifyTrack[]> {
	const tracks: SpotifyTrack[] = [];
	let path = `/playlists/${playlistId}/tracks?limit=100&fields=next,items(track(name,duration_ms,artists,album(name),is_local))`;
	while (path) {
		const data = await spotifyGet<SpotifyApiTracksPage>(path);
		for (const item of data.items ?? []) {
			const t = item.track;
			if (!t || t.is_local) continue;
			tracks.push({
				title: t.name,
				artist: t.artists?.[0]?.name ?? '',
				album: t.album?.name ?? '',
				durationMs: t.duration_ms ?? 0,
			});
		}
		path = data.next ? data.next.replace('https://api.spotify.com/v1', '') : '';
	}
	return tracks;
}

// ── Matching ──────────────────────────────────────────────────────────────────

function normalize(s: string): string {
	return s.toLowerCase().replace(/[^a-z0-9]/g, '');
}

function fuzzyMatch(a: string, b: string): boolean {
	const na = normalize(a);
	const nb = normalize(b);
	if (!na || !nb) return false;
	if (nb.includes(na) || na.includes(nb)) return true;
	let ai = 0;
	for (let bi = 0; bi < nb.length && ai < na.length; bi++) {
		if (nb[bi] === na[ai]) ai++;
	}
	return ai / na.length >= 0.8;
}

export interface SpotifyMatchResult {
	spotify: SpotifyTrack;
	matched: Track | null;
}

export async function matchSpotifyTracks(
	spotifyTracks: SpotifyTrack[],
	concurrency = 5
): Promise<SpotifyMatchResult[]> {
	const results: SpotifyMatchResult[] = new Array(spotifyTracks.length);

	async function matchOne(i: number): Promise<void> {
		const s = spotifyTracks[i];
		const q = [s.artist, s.title].filter(Boolean).join(' ');
		try {
			const res = await library.search(q);
			const match = (res.tracks ?? []).find((t) => fuzzyMatch(s.title, t.title)) ?? null;
			results[i] = { spotify: s, matched: match };
		} catch {
			results[i] = { spotify: s, matched: null };
		}
	}

	for (let i = 0; i < spotifyTracks.length; i += concurrency) {
		await Promise.all(
			spotifyTracks.slice(i, i + concurrency).map((_, j) => matchOne(i + j))
		);
	}
	return results;
}

// ── Spotify API types (minimal) ───────────────────────────────────────────────

interface SpotifyApiPlaylist {
	id: string;
	name: string;
	description?: string;
	images?: { url: string }[];
	tracks?: { total: number };
}

interface SpotifyApiTracksPage {
	next: string | null;
	items: Array<{
		track: {
			name: string;
			duration_ms: number;
			is_local?: boolean;
			artists: Array<{ name: string }>;
			album: { name: string };
		} | null;
	}>;
}
