import { writable } from 'svelte/store';
import type { Track } from '$lib/types';

export const contextMenu = writable<{
	visible: boolean;
	x: number;
	y: number;
	track: Track | null;
}>({ visible: false, x: 0, y: 0, track: null });

export function openContextMenu(e: MouseEvent, track: Track) {
	e.preventDefault();
	contextMenu.set({ visible: true, x: e.clientX, y: e.clientY, track });
}

export function closeContextMenu() {
	contextMenu.update((m) => ({ ...m, visible: false, track: null }));
}
