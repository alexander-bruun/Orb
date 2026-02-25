import { get } from 'svelte/store';
import { authStore } from '$lib/stores/auth';

const BASE = import.meta.env.VITE_API_BASE ?? '/api';

export class ApiError extends Error {
	constructor(
		public status: number,
		message: string
	) {
		super(message);
	}
}

export async function apiFetch<T>(
	path: string,
	options: RequestInit = {}
): Promise<T> {
	const auth = get(authStore);
	const headers: Record<string, string> = {
		'Content-Type': 'application/json',
		...(options.headers as Record<string, string>)
	};
	if (auth.token) {
		headers['Authorization'] = `Bearer ${auth.token}`;
	}

	const res = await fetch(`${BASE}${path}`, { ...options, headers });
	if (!res.ok) {
		let msg = res.statusText;
		try {
			const body = await res.json();
			msg = body.error ?? msg;
		} catch {
			// ignore
		}
		throw new ApiError(res.status, msg);
	}
	if (res.status === 204) return undefined as T;
	return res.json();
}

export async function apiStream(
	path: string,
	rangeStart: number,
	rangeEnd?: number
): Promise<Response> {
	const auth = get(authStore);
	const range = rangeEnd !== undefined ? `bytes=${rangeStart}-${rangeEnd}` : `bytes=${rangeStart}-`;
	const res = await fetch(`${BASE}${path}`, {
		headers: {
			Authorization: auth.token ? `Bearer ${auth.token}` : '',
			Range: range
		}
	});
	if (!res.ok && res.status !== 206) {
		throw new ApiError(res.status, 'stream error');
	}
	return res;
}
