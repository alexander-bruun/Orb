/**
 * Playlist export helpers — M3U and XSPF generation.
 *
 * Both functions build the file in-memory and trigger a browser download.
 * Track stream URLs are authenticated via the token query param so the
 * exported file works in any player that can pass a URL.
 */

import type { Track } from '$lib/types';

function streamUrl(trackId: string, apiBase: string, token: string): string {
	return `${apiBase}/stream/${trackId}?token=${encodeURIComponent(token)}`;
}

function triggerDownload(content: string, filename: string, mimeType: string): void {
	const blob = new Blob([content], { type: mimeType });
	const url = URL.createObjectURL(blob);
	const a = document.createElement('a');
	a.href = url;
	a.download = filename;
	a.click();
	setTimeout(() => URL.revokeObjectURL(url), 10_000);
}

function safeFilename(name: string): string {
	return name.replace(/[/\\?%*:|"<>]/g, '_');
}

/** Export playlist as an extended M3U file. */
export function exportM3U(
	playlistName: string,
	tracks: Track[],
	apiBase: string,
	token: string
): void {
	const lines: string[] = ['#EXTM3U', ''];
	for (const t of tracks) {
		const durationSecs = Math.round((t.duration_ms ?? 0) / 1000);
		const artist = t.artist_name ?? '';
		const title = t.title ?? '';
		const label = artist ? `${artist} - ${title}` : title;
		lines.push(`#EXTINF:${durationSecs},${label}`);
		lines.push(streamUrl(t.id, apiBase, token));
	}
	triggerDownload(lines.join('\n'), `${safeFilename(playlistName)}.m3u`, 'audio/x-mpegurl');
}

/** Export playlist as an XSPF (XML Shareable Playlist Format) file. */
export function exportXSPF(
	playlistName: string,
	tracks: Track[],
	apiBase: string,
	token: string
): void {
	function esc(s: string): string {
		return s
			.replace(/&/g, '&amp;')
			.replace(/</g, '&lt;')
			.replace(/>/g, '&gt;')
			.replace(/"/g, '&quot;');
	}

	const trackXml = tracks
		.map((t) => {
			const url = streamUrl(t.id, apiBase, token);
			const dur = t.duration_ms ?? 0;
			const artist = t.artist_name ?? '';
			const album = t.album_name ?? '';
			return [
				'\t\t<track>',
				`\t\t\t<location>${esc(url)}</location>`,
				`\t\t\t<title>${esc(t.title)}</title>`,
				artist ? `\t\t\t<creator>${esc(artist)}</creator>` : '',
				album ? `\t\t\t<album>${esc(album)}</album>` : '',
				dur ? `\t\t\t<duration>${dur}</duration>` : '',
				'\t\t</track>',
			]
				.filter(Boolean)
				.join('\n');
		})
		.join('\n');

	const xml = [
		'<?xml version="1.0" encoding="UTF-8"?>',
		'<playlist version="1" xmlns="http://xspf.org/ns/0/">',
		`\t<title>${esc(playlistName)}</title>`,
		'\t<trackList>',
		trackXml,
		'\t</trackList>',
		'</playlist>',
	].join('\n');

	triggerDownload(xml, `${safeFilename(playlistName)}.xspf`, 'application/xspf+xml');
}
