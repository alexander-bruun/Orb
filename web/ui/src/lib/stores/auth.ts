import { writable, derived, get } from 'svelte/store';
import type { User } from '$lib/types';
import { apiFetch } from '$lib/api/client';

interface AuthState {
	token: string | null;
	refreshToken: string | null;
	user: User | null;
}

const STORAGE_KEY = 'orb_auth';
const BASE = import.meta.env.VITE_API_BASE ?? '/api';

function loadFromStorage(): AuthState {
	if (typeof localStorage === 'undefined') return { token: null, refreshToken: null, user: null };
	try {
		const raw = localStorage.getItem(STORAGE_KEY);
		return raw ? JSON.parse(raw) : { token: null, refreshToken: null, user: null };
	} catch {
		return { token: null, refreshToken: null, user: null };
	}
}

function saveToStorage(state: AuthState) {
	if (typeof localStorage !== 'undefined') {
		localStorage.setItem(STORAGE_KEY, JSON.stringify(state));
	}
}

function clearStorage() {
	if (typeof localStorage !== 'undefined') {
		localStorage.removeItem(STORAGE_KEY);
	}
}

// Deduplicates concurrent refresh calls so only one request is in-flight at a time.
let refreshPromise: Promise<boolean> | null = null;

function createAuthStore() {
	const { subscribe, set, update } = writable<AuthState>(loadFromStorage());

	function doLogout() {
		set({ token: null, refreshToken: null, user: null });
		clearStorage();
	}

	return {
		subscribe,
		async login(email: string, password: string) {
			const res = await apiFetch<{ access_token: string; refresh_token: string; user_id: string }>(
				'/auth/login',
				{ method: 'POST', body: JSON.stringify({ email, password }) }
			);
			const state: AuthState = {
				token: res.access_token,
				refreshToken: res.refresh_token,
				user: { id: res.user_id, username: '', email }
			};
			set(state);
			saveToStorage(state);
		},
		async register(username: string, email: string, password: string) {
			await apiFetch('/auth/register', {
				method: 'POST',
				body: JSON.stringify({ username, email, password })
			});
		},
		logout() {
			apiFetch('/auth/logout', { method: 'POST' }).catch(() => {});
			doLogout();
		},
		// Exchanges the stored refresh token for a new access + refresh token pair.
		// Returns true on success, false if the session is unrecoverable (forces logout).
		// Concurrent calls share the same in-flight request.
		async refreshTokens(): Promise<boolean> {
			if (refreshPromise) return refreshPromise;

			const state = get({ subscribe });
			if (!state.refreshToken) return false;

			refreshPromise = (async () => {
				try {
					const res = await fetch(`${BASE}/auth/refresh`, {
						method: 'POST',
						headers: { 'Content-Type': 'application/json' },
						body: JSON.stringify({ refresh_token: state.refreshToken })
					});
					if (!res.ok) {
						doLogout();
						return false;
					}
					const data: { access_token: string; refresh_token: string } = await res.json();
					update((s) => {
						const next = { ...s, token: data.access_token, refreshToken: data.refresh_token };
						saveToStorage(next);
						return next;
					});
					return true;
				} catch {
					return false;
				} finally {
					refreshPromise = null;
				}
			})();

			return refreshPromise;
		}
	};
}

export const authStore = createAuthStore();
export const isAuthenticated = derived(authStore, ($a) => !!$a.token);
