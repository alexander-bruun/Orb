import { writable, get } from 'svelte/store';
import { STORAGE_KEYS } from '$lib/constants';
import { audiobooks as audiobooksApi } from '$lib/api/audiobooks';

export interface LocalAudiobookProgress {
  audiobookId: string;
  positionMs: number;
  completed: boolean;
  updatedAt: number;
}

const META_KEY = STORAGE_KEYS.AUDIOBOOK_PROGRESS;

// Map of audiobookId -> LocalAudiobookProgress
export const localProgress = writable<Map<string, LocalAudiobookProgress>>(new Map());

export function restoreLocalProgress(): void {
  try {
    const raw = localStorage.getItem(META_KEY);
    if (raw) {
      localProgress.set(new Map(JSON.parse(raw)));
    }
  } catch {
    // start fresh if corrupt
  }
}

function persist(map: Map<string, LocalAudiobookProgress>): void {
  try {
    localStorage.setItem(META_KEY, JSON.stringify([...map]));
  } catch {
    // storage full
  }
}

export function saveLocalProgress(audiobookId: string, positionMs: number, completed: boolean): void {
  localProgress.update(m => {
    m.set(audiobookId, {
      audiobookId,
      positionMs,
      completed,
      updatedAt: Date.now()
    });
    persist(m);
    return m;
  });
}

export function getLocalProgress(audiobookId: string): LocalAudiobookProgress | undefined {
  return get(localProgress).get(audiobookId);
}

/**
 * Sync all local progress to the server.
 * Called when the device comes back online.
 */
export async function syncProgressToServer(): Promise<void> {
  const all = get(localProgress);
  if (all.size === 0) return;

  // Clone entries to avoid issues during iteration if we remove them
  const entries = [...all.values()];

  for (const entry of entries) {
    try {
      // We don't have a batch API for progress yet, so we sync one by one.
      // In a real scenario, we might want to check if the server's progress
      // is newer, but usually local progress is more recent if we've been offline.
      await audiobooksApi.saveProgress(entry.audiobookId, entry.positionMs, entry.completed);

      // Success - remove from local storage
      localProgress.update(m => {
        m.delete(entry.audiobookId);
        persist(m);
        return m;
      });
    } catch (e) {
      // If it fails, keep it for the next sync attempt
      console.error(`Failed to sync progress for ${entry.audiobookId}:`, e);
    }
  }
}
