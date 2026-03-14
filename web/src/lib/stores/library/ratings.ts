import { writable, get } from 'svelte/store';
import { library } from '$lib/api/library';

const store = writable<Map<string, number>>(new Map());

export const ratings = {
	subscribe: store.subscribe,

	load: async () => {
		try {
			const map = await library.ratings();
			store.set(new Map(Object.entries(map ?? {})));
		} catch {
			store.set(new Map());
		}
	},

	set: async (trackId: string, rating: number) => {
		await library.setRating(trackId, rating);
		store.update(m => { const n = new Map(m); n.set(trackId, rating); return n; });
	},

	remove: async (trackId: string) => {
		await library.deleteRating(trackId);
		store.update(m => { const n = new Map(m); n.delete(trackId); return n; });
	},

	toggle: async (trackId: string, rating: number) => {
		if (get(store).get(trackId) === rating) {
			await library.deleteRating(trackId);
			store.update(m => { const n = new Map(m); n.delete(trackId); return n; });
		} else {
			await library.setRating(trackId, rating);
			store.update(m => { const n = new Map(m); n.set(trackId, rating); return n; });
		}
	},
};
