import { writable } from 'svelte/store';
import type { Track, Album, Artist, SearchFilters, SavedFilter } from '$lib/types';

export const libraryTracks = writable<Track[]>([]);
export const libraryAlbums = writable<Album[]>([]);
export const libraryArtists = writable<Artist[]>([]);
export const searchResults = writable<{
	tracks: Track[];
	albums: Album[];
	artists: Artist[];
}>({ tracks: [], albums: [], artists: [] });
export const searchQuery = writable('');
export const searchFilters = writable<SearchFilters>({});

const SAVED_FILTERS_KEY = 'orb:savedSearchFilters';

function loadSavedFilters(): SavedFilter[] {
	try {
		const raw = localStorage.getItem(SAVED_FILTERS_KEY);
		return raw ? JSON.parse(raw) : [];
	} catch {
		return [];
	}
}

function persistSavedFilters(filters: SavedFilter[]) {
	localStorage.setItem(SAVED_FILTERS_KEY, JSON.stringify(filters));
}

export const savedFilters = writable<SavedFilter[]>(
	typeof localStorage !== 'undefined' ? loadSavedFilters() : []
);

savedFilters.subscribe((val) => {
	if (typeof localStorage !== 'undefined') {
		persistSavedFilters(val);
	}
});

export function saveFilter(name: string, filters: SearchFilters) {
	savedFilters.update((list) => {
		const next = list.filter((f) => f.name !== name);
		next.unshift({ name, filters, createdAt: new Date().toISOString() });
		return next;
	});
}

export function deleteSavedFilter(name: string) {
	savedFilters.update((list) => list.filter((f) => f.name !== name));
}
