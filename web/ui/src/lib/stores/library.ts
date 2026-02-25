import { writable } from 'svelte/store';
import type { Track, Album, Artist } from '$lib/types';

export const libraryTracks = writable<Track[]>([]);
export const libraryAlbums = writable<Album[]>([]);
export const libraryArtists = writable<Artist[]>([]);
export const searchResults = writable<{
	tracks: Track[];
	albums: Album[];
	artists: Artist[];
}>({ tracks: [], albums: [], artists: [] });
export const searchQuery = writable('');
