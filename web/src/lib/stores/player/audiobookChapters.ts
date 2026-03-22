/**
 * Audiobook chapter navigation and chapter-derived stores.
 *
 * Extracted from audiobookPlayer.ts for modularity.
 * Shared state (writable stores) is imported from audiobookPlayer.ts.
 */

import { derived } from 'svelte/store';
import type { Audiobook, AudiobookChapter } from '$lib/types';
import { currentAudiobook, abPositionMs, abDurationMs } from './audiobookPlayer';

// ── Chapter-derived stores ───────────────────────────────────────────────────

export const abCurrentChapter = derived(
	[currentAudiobook, abPositionMs],
	([$book, $pos]) => {
		if (!$book?.chapters?.length) return null;
		let current: AudiobookChapter | null = null;
		for (const ch of $book.chapters) {
			if ($pos >= ch.start_ms) current = ch;
			else break;
		}
		return current;
	}
);

export const abProgress = derived(
	[abPositionMs, abDurationMs],
	([$pos, $dur]) => ($dur > 0 ? ($pos / $dur) * 100 : 0)
);

// Chapter-aware progress: position within the current chapter
export const abChapterProgress = derived(
	[currentAudiobook, abPositionMs, abCurrentChapter],
	([$book, $pos, $chapter]) => {
		if (!$chapter) return 0;
		const nextChapter = $book?.chapters?.find(ch => ch.start_ms > $chapter.start_ms);
		const chapterDurationMs = nextChapter ? nextChapter.start_ms - $chapter.start_ms : ($book?.duration_ms ?? 0) - $chapter.start_ms;
		const posInChapter = $pos - $chapter.start_ms;
		return chapterDurationMs > 0 ? Math.max(0, Math.min(100, (posInChapter / chapterDurationMs) * 100)) : 0;
	}
);

// Previous chapter
export const abPreviousChapter = derived(
	[currentAudiobook, abCurrentChapter],
	([$book, $current]) => {
		if (!$book?.chapters || !$current) return null;
		const idx = $book.chapters.findIndex(ch => ch.id === $current.id);
		return idx > 0 ? $book.chapters[idx - 1] : null;
	}
);

// Next chapter
export const abNextChapter = derived(
	[currentAudiobook, abCurrentChapter],
	([$book, $current]) => {
		if (!$book?.chapters || !$current) return null;
		const idx = $book.chapters.findIndex(ch => ch.id === $current.id);
		return idx >= 0 && idx < $book.chapters.length - 1 ? $book.chapters[idx + 1] : null;
	}
);

// ── Chapter helpers ──────────────────────────────────────────────────────────

export function isMultiFileBook(book: Audiobook): boolean {
	return (book.chapters?.length ?? 0) > 0 && book.chapters![0].file_key != null;
}

export function findChapterIndex(book: Audiobook, posMs: number): number {
	const chapters = book.chapters ?? [];
	let idx = 0;
	for (let i = 0; i < chapters.length; i++) {
		if (posMs >= chapters[i].start_ms) idx = i;
		else break;
	}
	return idx;
}
