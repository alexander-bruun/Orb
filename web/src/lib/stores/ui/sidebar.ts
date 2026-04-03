import { writable } from 'svelte/store';
import { createPersistentNumberStore } from '$lib/utils/persistentStore';

/** Controls the mobile slide-in sidebar. Ignored on desktop (≥641 px). */
export const sidebarOpen = writable(false);

export const SIDEBAR_MIN_WIDTH = 184;
export const SIDEBAR_MAX_WIDTH = 420;
export const SIDEBAR_DEFAULT_WIDTH = 232;

/** Desktop sidebar width in px, persisted across sessions. */
export const sidebarWidth = createPersistentNumberStore(
  'orb_sidebar_width',
  SIDEBAR_DEFAULT_WIDTH,
  SIDEBAR_MIN_WIDTH,
  SIDEBAR_MAX_WIDTH,
);
