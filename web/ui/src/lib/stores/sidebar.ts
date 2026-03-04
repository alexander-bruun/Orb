import { writable } from 'svelte/store';

/** Controls the mobile slide-in sidebar. Ignored on desktop (≥641 px). */
export const sidebarOpen = writable(false);
