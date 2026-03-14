import { writable, get } from 'svelte/store';
import { library } from '$lib/api/library';
import type { Track } from '$lib/types';

const store = writable<Set<string>>(new Set());
const trackStore = writable<Track[] | null>(null);

export const favorites = {
	subscribe: store.subscribe,

	load: async () => {
		try {
			const ids = await library.favoriteIDs();
			store.set(new Set(ids ?? []));
		} catch {
			store.set(new Set());
		}
	},

	loadTracks: async () => {
		try {
			const tracks = await library.favorites();
			trackStore.set(tracks);
			store.set(new Set(tracks.map(t => t.id)));
		} catch {
			trackStore.set([]);
		}
	},

	add: async (trackId: string, track?: Track) => {
		await library.addFavorite(trackId);
		store.update(s => { const n = new Set(s); n.add(trackId); return n; });
		if (track) {
			trackStore.update(ts => ts !== null ? [track, ...ts] : null);
		}
	},

	remove: async (trackId: string) => {
		await library.removeFavorite(trackId);
		store.update(s => { const n = new Set(s); n.delete(trackId); return n; });
		trackStore.update(ts => ts !== null ? ts.filter(t => t.id !== trackId) : null);
	},

	toggle: async (trackId: string, track?: Track) => {
		if (get(store).has(trackId)) {
			await library.removeFavorite(trackId);
			store.update(s => { const n = new Set(s); n.delete(trackId); return n; });
			trackStore.update(ts => ts !== null ? ts.filter(t => t.id !== trackId) : null);
		} else {
			await library.addFavorite(trackId);
			store.update(s => { const n = new Set(s); n.add(trackId); return n; });
			if (track) {
				trackStore.update(ts => ts !== null ? [track, ...ts] : null);
			}
		}
	}
};

export const favoriteTracks = {
	subscribe: trackStore.subscribe,
};
