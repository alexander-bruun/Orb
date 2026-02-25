import { writable, derived } from 'svelte/store';
import type { User } from '$lib/types';
import { apiFetch } from '$lib/api/client';

interface AuthState {
	token: string | null;
	refreshToken: string | null;
	user: User | null;
}

const STORAGE_KEY = 'orb_auth';

function loadFromStorage(): AuthState {
	if (typeof localStorage === 'undefined') return { token: null, refreshToken: null, user: null };
	try {
		const raw = localStorage.getItem(STORAGE_KEY);
		return raw ? JSON.parse(raw) : { token: null, refreshToken: null, user: null };
	} catch {
		return { token: null, refreshToken: null, user: null };
	}
}

function createAuthStore() {
	const { subscribe, set, update } = writable<AuthState>(loadFromStorage());

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
			if (typeof localStorage !== 'undefined') {
				localStorage.setItem(STORAGE_KEY, JSON.stringify(state));
			}
		},
		async register(username: string, email: string, password: string) {
			await apiFetch('/auth/register', {
				method: 'POST',
				body: JSON.stringify({ username, email, password })
			});
		},
		logout() {
			apiFetch('/auth/logout', { method: 'POST' }).catch(() => {});
			set({ token: null, refreshToken: null, user: null });
			if (typeof localStorage !== 'undefined') {
				localStorage.removeItem(STORAGE_KEY);
			}
		}
	};
}

export const authStore = createAuthStore();
export const isAuthenticated = derived(authStore, ($a) => !!$a.token);
