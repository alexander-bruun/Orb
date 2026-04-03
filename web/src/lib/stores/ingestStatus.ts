import { writable, get } from 'svelte/store';
import { browser } from '$app/environment';
import { getApiBase } from '$lib/api/base';
import { apiFetch } from '$lib/api/client';
import { authStore } from '$lib/stores/auth';
import { TIMINGS } from '$lib/constants';

export interface LastScan {
	started_at: string;
	finished_at: string;
	ingested: number;
	skipped: number;
	errors: number;
	enqueued: number;
}

export interface IngestState {
	phase: 'idle' | 'running' | 'complete' | 'error';
	running: boolean;
	done: number;
	total: number;
	skipped: number;
	errors: number;
	startedAt?: number;
	currentFile?: string;
	lastScan?: LastScan;
}

const initial: IngestState = {
	phase: 'idle',
	running: false,
	done: 0,
	total: 0,
	skipped: 0,
	errors: 0,
};

function createIngestStatusStore() {
	const { subscribe, update, set } = writable<IngestState>(initial);

	let es: EventSource | null = null;
	let pollTimer: number | null = null;
	// Ref-count so multiple callers (TopBar + admin page) can safely call init/destroy
	let initCount = 0;

	function connectSSE() {
		if (!browser || es) return;
		const token = get(authStore).token;
		if (!token) return;

		const url = `${getApiBase()}/admin/ingest/stream?token=${encodeURIComponent(token)}`;
		es = new EventSource(url);

		es.onmessage = (e) => {
			try {
				const data = JSON.parse(e.data);
				update((s) => {
					const now = Date.now();
					const isStarting = data.type === 'progress' && !s.running;
					return {
						...s,
						running: data.type === 'progress',
						phase:
							data.type === 'complete' ? 'complete' : data.type === 'error' ? 'error' : 'running',
						done: data.done ?? s.done,
						total: data.total ?? s.total,
						skipped: data.skipped ?? s.skipped,
						errors: data.errors ?? s.errors,
						currentFile: data.file_path || s.currentFile,
						startedAt: isStarting ? now : s.startedAt,
					};
				});
				if (data.type === 'complete' || data.type === 'error') {
					disconnectSSE();
					// Refresh full status to pick up last_scan
					setTimeout(fetchStatus, TIMINGS.INGEST_COMPLETE_FETCH_DELAY);
				}
			} catch {
				// ignore parse errors
			}
		};

		es.onerror = () => {
			disconnectSSE();
		};
	}

	function disconnectSSE() {
		if (es) {
			es.close();
			es = null;
		}
	}

	async function fetchStatus() {
		try {
			const data = await apiFetch<{ running?: boolean; last_scan?: LastScan }>('/admin/ingest/status');
			update((s) => {
				const wasRunning = s.running;
				const nowRunning = Boolean(data.running);
				if (nowRunning && !wasRunning) {
					// New scan detected (e.g. automatic/periodic) — reset counters and connect SSE
					connectSSE();
					return {
						...s,
						running: true,
						phase: 'running',
						done: 0,
						total: 0,
						skipped: 0,
						errors: 0,
						startedAt: Date.now(),
						currentFile: undefined,
						lastScan: data.last_scan ?? s.lastScan,
					};
				} else if (!nowRunning && wasRunning) {
					disconnectSSE();
				}
				return {
					...s,
					running: nowRunning,
					phase: nowRunning ? 'running' : data.last_scan ? 'complete' : 'idle',
					lastScan: data.last_scan ?? s.lastScan,
				};
			});
		} catch {
			// ignore — user may not be admin or server is down
		}
	}

	function init() {
		if (!browser) return;
		initCount++;
		if (initCount === 1) {
			fetchStatus();
			pollTimer = window.setInterval(fetchStatus, TIMINGS.INGEST_POLL_INTERVAL);
		}
	}

	function destroy() {
		initCount = Math.max(0, initCount - 1);
		if (initCount === 0) {
			disconnectSSE();
			if (pollTimer !== null) {
				clearInterval(pollTimer);
				pollTimer = null;
			}
		}
	}

	async function triggerScan(force = false) {
		const qs = force ? '?force=true' : '';
		await apiFetch(`/admin/ingest/scan${qs}`, { method: 'POST' });
		update((s) => ({
			...s,
			running: true,
			phase: 'running',
			done: 0,
			total: 0,
			skipped: 0,
			errors: 0,
			startedAt: Date.now(),
			currentFile: undefined,
		}));
		// Small delay so the server starts before we subscribe
		setTimeout(connectSSE, TIMINGS.INGEST_SSE_CONNECT_DELAY);
	}

	return { subscribe, init, destroy, triggerScan, fetchStatus };
}

export const ingestStatus = createIngestStatusStore();
