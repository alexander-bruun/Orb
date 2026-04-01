/**
 * Playlist import — parse M3U / XSPF files, then fuzzy-match tracks
 * against the local Orb library via the search API.
 *
 * The matcher searches by "artist title" and accepts the top result when
 * the normalized title overlaps with the search term.
 */

import type { Track } from '$lib/types';
import { library } from '$lib/api/library';

export interface ParsedTrack {
	title: string;
	artist?: string;
	durationSecs?: number;
	/** Original URL from the playlist file (ignored for matching). */
	url?: string;
}

// ── Parsers ──────────────────────────────────────────────────────────────────

/** Parse an extended M3U / M3U8 file into a list of entries. */
export function parseM3U(text: string): ParsedTrack[] {
	const lines = text.split(/\r?\n/);
	const results: ParsedTrack[] = [];
	let pending: Partial<ParsedTrack> = {};

	for (const raw of lines) {
		const line = raw.trim();
		if (!line || line === '#EXTM3U') continue;

		if (line.startsWith('#EXTINF:')) {
			// #EXTINF:duration,Artist - Title
			const rest = line.slice('#EXTINF:'.length);
			const commaIdx = rest.indexOf(',');
			if (commaIdx === -1) continue;
			const durStr = rest.slice(0, commaIdx).trim();
			const label = rest.slice(commaIdx + 1).trim();
			const durationSecs = parseInt(durStr, 10) || undefined;
			// Split "Artist - Title" on first " - "
			const dashIdx = label.indexOf(' - ');
			if (dashIdx !== -1) {
				pending = {
					artist: label.slice(0, dashIdx).trim(),
					title: label.slice(dashIdx + 3).trim(),
					durationSecs,
				};
			} else {
				pending = { title: label, durationSecs };
			}
		} else if (!line.startsWith('#')) {
			// URL / file path line
			if (pending.title) {
				results.push({ ...pending, url: line } as ParsedTrack);
			} else {
				// Plain M3U without #EXTINF — use last path segment as title
				const seg = line.split(/[/\\]/).pop() ?? line;
				const title = seg.replace(/\.[^.]+$/, ''); // strip extension
				results.push({ title, url: line });
			}
			pending = {};
		}
	}
	return results;
}

/** Parse an XSPF XML playlist. */
export function parseXSPF(text: string): ParsedTrack[] {
	let doc: Document;
	try {
		doc = new DOMParser().parseFromString(text, 'application/xml');
	} catch {
		return [];
	}
	const ns = 'http://xspf.org/ns/0/';
	const trackEls = doc.getElementsByTagNameNS(ns, 'track');
	// Fallback: some XSPF files omit the namespace
	const items = trackEls.length > 0
		? Array.from(trackEls)
		: Array.from(doc.getElementsByTagName('track'));

	return items.map((el) => {
		const get = (tag: string) =>
			el.getElementsByTagNameNS(ns, tag)[0]?.textContent?.trim() ||
			el.getElementsByTagName(tag)[0]?.textContent?.trim() ||
			'';
		const title = get('title');
		const artist = get('creator');
		const durMs = parseInt(get('duration'), 10) || undefined;
		const url = get('location');
		return {
			title: title || url || '',
			artist: artist || undefined,
			durationSecs: durMs ? Math.round(durMs / 1000) : undefined,
			url: url || undefined,
		};
	}).filter((t) => !!t.title);
}

// ── Matcher ──────────────────────────────────────────────────────────────────

function normalize(s: string): string {
	return s.toLowerCase().replace(/[^a-z0-9]/g, '');
}

/** Returns true if b contains at least 60 % of a's characters in sequence. */
function fuzzyMatch(a: string, b: string): boolean {
	const na = normalize(a);
	const nb = normalize(b);
	if (!na || !nb) return false;
	if (nb.includes(na) || na.includes(nb)) return true;
	// Subsequence check for short titles
	let ai = 0;
	for (let bi = 0; bi < nb.length && ai < na.length; bi++) {
		if (nb[bi] === na[ai]) ai++;
	}
	return ai / na.length >= 0.8;
}

export interface MatchResult {
	parsed: ParsedTrack;
	matched: Track | null;
}

/**
 * Fuzzy-match an array of parsed tracks against the local library.
 * Runs up to `concurrency` searches in parallel.
 */
export async function matchTracks(
	parsed: ParsedTrack[],
	concurrency = 5
): Promise<MatchResult[]> {
	const results: MatchResult[] = new Array(parsed.length);

	async function matchOne(i: number): Promise<void> {
		const p = parsed[i];
		const q = [p.artist, p.title].filter(Boolean).join(' ');
		if (!q) {
			results[i] = { parsed: p, matched: null };
			return;
		}
		try {
			const res = await library.search(q);
			const candidates = res.tracks ?? [];
			const match = candidates.find((t) => fuzzyMatch(p.title, t.title)) ?? null;
			results[i] = { parsed: p, matched: match };
		} catch {
			results[i] = { parsed: p, matched: null };
		}
	}

	// Process in batches of `concurrency`
	for (let i = 0; i < parsed.length; i += concurrency) {
		const batch = parsed
			.slice(i, i + concurrency)
			.map((_, j) => matchOne(i + j));
		await Promise.all(batch);
	}
	return results;
}

/** Detect format from filename extension or raw content. */
export function detectFormat(filename: string, content: string): 'm3u' | 'xspf' | null {
	const lower = filename.toLowerCase();
	if (lower.endsWith('.m3u') || lower.endsWith('.m3u8')) return 'm3u';
	if (lower.endsWith('.xspf')) return 'xspf';
	if (content.trimStart().startsWith('<?xml') || content.includes('<playlist')) return 'xspf';
	if (content.trimStart().startsWith('#EXTM3U') || content.includes('#EXTINF')) return 'm3u';
	return null;
}
