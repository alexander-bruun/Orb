import { writable, get } from 'svelte/store';
import { getApiBase } from '$lib/api/base';

/**
 * Reactive store: true when the backend API is unreachable.
 *
 * Detection uses the lightweight /healthz endpoint which always returns 200.
 * Falls back to navigator.onLine when available but always verifies with
 * a real request since onLine only detects local network presence, not
 * actual server reachability.
 */
export const isOffline = writable(false);

let intervalId: ReturnType<typeof setInterval> | null = null;

/**
 * Ping the API and update the offline store.
 * Returns the current offline state.
 */
export async function checkConnectivity(): Promise<boolean> {
  // If browser explicitly says offline, skip the fetch entirely.
  if (typeof navigator !== 'undefined' && !navigator.onLine) {
    isOffline.set(true);
    return true;
  }
  try {
    // Use AbortController with a manual timeout for broad compatibility.
    const timeoutMs = 3000;
    let controller: AbortController | null = null;
    let timeoutId: ReturnType<typeof setTimeout> | null = null;
    if (typeof AbortController !== 'undefined') {
      controller = new AbortController();
      timeoutId = setTimeout(() => controller?.abort(), timeoutMs);
    }

    const res = await fetch(`${getApiBase()}/healthz`, {
      method: 'GET',
      cache: 'no-store',
      signal: controller ? controller.signal : undefined,
    });
    if (timeoutId) clearTimeout(timeoutId);
    // healthz always returns 200 when the server is up.
    isOffline.set(!res.ok);
    return !res.ok;
  } catch {
    isOffline.set(true);
    return true;
  }
}

/**
 * Returns true if we're currently in offline state (synchronous read).
 */
export function isCurrentlyOffline(): boolean {
  return get(isOffline);
}

/**
 * Start periodic connectivity checks (every 15 s).
 * Safe to call multiple times — only one interval runs.
 */
export function startConnectivityMonitor(): void {
  if (intervalId !== null) return;

  // Initial check
  checkConnectivity();

  // Listen to browser online/offline events for instant transitions
  if (typeof window !== 'undefined') {
    window.addEventListener('online', () => checkConnectivity());
    window.addEventListener('offline', () => {
      isOffline.set(true);
    });
  }

  intervalId = setInterval(checkConnectivity, 15_000);
}

/**
 * Stop periodic connectivity checks.
 */
export function stopConnectivityMonitor(): void {
  if (intervalId !== null) {
    clearInterval(intervalId);
    intervalId = null;
  }
}
