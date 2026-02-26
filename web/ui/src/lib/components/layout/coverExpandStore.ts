import { writable } from 'svelte/store';

const STORAGE_KEY = 'orb:cover-expanded';

function createExpandedStore() {
  const initial = typeof localStorage !== 'undefined'
    ? localStorage.getItem(STORAGE_KEY) === 'true'
    : false;

  const { subscribe, update, set } = writable(initial);

  return {
    subscribe,
    set(value: boolean) {
      if (typeof localStorage !== 'undefined') {
        localStorage.setItem(STORAGE_KEY, String(value));
      }
      set(value);
    },
    update(fn: (v: boolean) => boolean) {
      update(v => {
        const next = fn(v);
        if (typeof localStorage !== 'undefined') {
          localStorage.setItem(STORAGE_KEY, String(next));
        }
        return next;
      });
    }
  };
}

export const expanded = createExpandedStore();
