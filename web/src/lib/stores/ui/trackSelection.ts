import { writable, get } from 'svelte/store';
import type { Track } from '$lib/types';

export const selectedTrackIds = writable<Set<string>>(new Set());
export const lastClickedIndex = writable<number>(-1);

export function clearSelection() {
	selectedTrackIds.set(new Set());
	lastClickedIndex.set(-1);
}

export function toggleTrack(id: string, index: number) {
	selectedTrackIds.update((s) => {
		const next = new Set(s);
		if (next.has(id)) next.delete(id);
		else next.add(id);
		return next;
	});
	lastClickedIndex.set(index);
}

export function selectRange(tracks: Track[], fromIndex: number, toIndex: number) {
	const min = Math.min(fromIndex, toIndex);
	const max = Math.max(fromIndex, toIndex);
	selectedTrackIds.update((s) => {
		const next = new Set(s);
		for (let i = min; i <= max; i++) {
			if (tracks[i]) next.add(tracks[i].id);
		}
		return next;
	});
	lastClickedIndex.set(toIndex);
}

export function selectAll(tracks: Track[]) {
	selectedTrackIds.set(new Set(tracks.map((t) => t.id)));
	lastClickedIndex.set(tracks.length - 1);
}

export function invertSelection(tracks: Track[]) {
	const current = get(selectedTrackIds);
	const next = new Set<string>();
	for (const t of tracks) {
		if (!current.has(t.id)) next.add(t.id);
	}
	selectedTrackIds.set(next);
}
