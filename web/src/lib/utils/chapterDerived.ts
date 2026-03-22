/**
 * Chapter-aware derived store factories.
 *
 * Both the audiobook player and the listen-party guest need identical logic
 * for deriving the current chapter, chapter progress, and adjacent chapters
 * from a book/position pair.  This module extracts that shared logic.
 */

import { derived, type Readable } from 'svelte/store';

/**
 * Minimal chapter shape required by the derived helpers.
 * Compatible with both `AudiobookChapter` from `$lib/types` and the
 * local `AudiobookChapter` defined in the listen-party store.
 */
export interface ChapterLike {
	id: string;
	start_ms: number;
}

/**
 * Minimal book shape: must expose `duration_ms` and an optional `chapters` array.
 */
export interface BookLike {
	duration_ms: number;
	chapters?: ChapterLike[];
}

/**
 * Given a readable book store and a readable position-ms store, returns a
 * derived store that resolves the chapter containing the current position.
 */
export function deriveCurrentChapter<C extends ChapterLike, B extends { chapters?: C[]; duration_ms: number }>(
	bookStore: Readable<B | null>,
	positionStore: Readable<number>,
): Readable<C | null> {
	return derived([bookStore, positionStore], ([$book, $pos]) => {
		if (!$book?.chapters?.length) return null;
		let current: C | null = null;
		for (const ch of $book.chapters) {
			if ($pos >= ch.start_ms) current = ch;
			else break;
		}
		return current;
	});
}

/**
 * Given a book store, position store, and the derived current-chapter store,
 * returns a derived store with the percentage progress within the current chapter (0-100).
 */
export function deriveChapterProgress<C extends ChapterLike, B extends { chapters?: C[]; duration_ms: number }>(
	bookStore: Readable<B | null>,
	positionStore: Readable<number>,
	currentChapterStore: Readable<C | null>,
): Readable<number> {
	return derived([bookStore, positionStore, currentChapterStore], ([$book, $pos, $chapter]) => {
		if (!$chapter) return 0;
		const nextChapter = $book?.chapters?.find((ch) => ch.start_ms > $chapter.start_ms);
		const chapterDurationMs = nextChapter
			? nextChapter.start_ms - $chapter.start_ms
			: ($book?.duration_ms ?? 0) - $chapter.start_ms;
		const posInChapter = $pos - $chapter.start_ms;
		return chapterDurationMs > 0
			? Math.max(0, Math.min(100, (posInChapter / chapterDurationMs) * 100))
			: 0;
	});
}

/**
 * Returns a derived store resolving the chapter immediately before the current one.
 */
export function derivePreviousChapter<C extends ChapterLike, B extends { chapters?: C[]; duration_ms: number }>(
	bookStore: Readable<B | null>,
	currentChapterStore: Readable<C | null>,
): Readable<C | null> {
	return derived([bookStore, currentChapterStore], ([$book, $current]) => {
		if (!$book?.chapters || !$current) return null;
		const idx = $book.chapters.findIndex((ch) => ch.id === $current.id);
		return idx > 0 ? $book.chapters[idx - 1] : null;
	});
}

/**
 * Returns a derived store resolving the chapter immediately after the current one.
 */
export function deriveNextChapter<C extends ChapterLike, B extends { chapters?: C[]; duration_ms: number }>(
	bookStore: Readable<B | null>,
	currentChapterStore: Readable<C | null>,
): Readable<C | null> {
	return derived([bookStore, currentChapterStore], ([$book, $current]) => {
		if (!$book?.chapters || !$current) return null;
		const idx = $book.chapters.findIndex((ch) => ch.id === $current.id);
		return idx >= 0 && idx < $book.chapters.length - 1 ? $book.chapters[idx + 1] : null;
	});
}
