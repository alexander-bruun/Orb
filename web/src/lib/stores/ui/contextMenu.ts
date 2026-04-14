import { writable } from 'svelte/store';
import type { Track } from '$lib/types';

export const contextMenu = writable<{
	visible: boolean;
	x: number;
	y: number;
	track: Track | null;
	tracks: Track[];
}>({ visible: false, x: 0, y: 0, track: null, tracks: [] });

export function openContextMenu(e: MouseEvent, track: Track) {
	e.preventDefault();
	contextMenu.set({ visible: true, x: e.clientX, y: e.clientY, track, tracks: [track] });
}

export function openMultiContextMenu(e: MouseEvent, tracks: Track[]) {
	e.preventDefault();
	contextMenu.set({
		visible: true,
		x: e.clientX,
		y: e.clientY,
		track: tracks[0] ?? null,
		tracks
	});
}

export function closeContextMenu() {
	contextMenu.update((m) => ({ ...m, visible: false, track: null, tracks: [] }));
}
