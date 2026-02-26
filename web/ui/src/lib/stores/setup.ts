import { writable } from 'svelte/store';

// null = not yet checked, true = no users exist, false = setup done
export const setupRequired = writable<boolean | null>(null);
