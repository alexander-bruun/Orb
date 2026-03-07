import { writable, get } from 'svelte/store';
import { library } from '$lib/api/library';

const store = writable<Set<string>>(new Set());

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

	add: async (trackId: string) => {
		await library.addFavorite(trackId);
		store.update(s => { const n = new Set(s); n.add(trackId); return n; });
	},

	remove: async (trackId: string) => {
		await library.removeFavorite(trackId);
		store.update(s => { const n = new Set(s); n.delete(trackId); return n; });
	},

	toggle: async (trackId: string) => {
		if (get(store).has(trackId)) {
			await library.removeFavorite(trackId);
			store.update(s => { const n = new Set(s); n.delete(trackId); return n; });
		} else {
			await library.addFavorite(trackId);
			store.update(s => { const n = new Set(s); n.add(trackId); return n; });
		}
	}
};
