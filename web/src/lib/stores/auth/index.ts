import { writable, derived, get } from 'svelte/store';
import type { User } from '$lib/types';
import { apiFetch } from '$lib/api/client';
import { nativePlatform } from '$lib/utils/platform';

interface AuthState {
	token: string | null;
	refreshToken: string | null;
	user: User | null;
}

const STORAGE_KEY = 'orb_auth';
import { getApiBase } from '$lib/api/base';

/** Push server URL + JWT to the Android MediaService for Android Auto browsing. */
async function syncCredentialsToAndroid(token: string | null) {
	if (nativePlatform() !== 'android' || !token) return;
	try {
		const { invoke } = await import('@tauri-apps/api/core');
		await invoke('set_api_credentials', { baseUrl: getApiBase(), token });
	} catch {
		// best-effort — service may not be running yet
	}
}

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
	const initial = loadFromStorage();
	const { subscribe, set, update } = writable<AuthState>(initial);

	// Sync existing credentials to Android on app start
	syncCredentialsToAndroid(initial.token);

	function doLogout() {
		set({ token: null, refreshToken: null, user: null });
		clearStorage();
	}

	return {
		subscribe,
		// Returns { totpRequired: false } on success, or { totpRequired: true, tempToken } when 2FA is needed.
		async login(email: string, password: string): Promise<{ totpRequired: boolean; tempToken?: string }> {
			const res = await apiFetch<{
				access_token?: string;
				refresh_token?: string;
				user_id?: string;
				username?: string;
				is_admin?: boolean;
				email_verified?: boolean;
				totp_required: boolean;
				temp_token?: string;
			}>(
				'/auth/login',
				{ method: 'POST', body: JSON.stringify({ email, password }) }
			);
			if (res.totp_required) {
				return { totpRequired: true, tempToken: res.temp_token };
			}
			const state: AuthState = {
				token: res.access_token!,
				refreshToken: res.refresh_token!,
				user: { id: res.user_id!, username: res.username ?? '', email, is_admin: res.is_admin ?? false, email_verified: res.email_verified ?? false }
			};
			set(state);
			saveToStorage(state);
			syncCredentialsToAndroid(state.token);
			return { totpRequired: false };
		},
		async verifyTOTP(tempToken: string, code: string, email: string) {
			const res = await apiFetch<{ access_token: string; refresh_token: string; user_id: string; username: string; is_admin?: boolean; email_verified?: boolean }>(
				'/auth/totp/verify',
				{ method: 'POST', body: JSON.stringify({ temp_token: tempToken, code }) }
			);
			const state: AuthState = {
				token: res.access_token,
				refreshToken: res.refresh_token,
				user: { id: res.user_id, username: res.username ?? '', email, is_admin: res.is_admin ?? false, email_verified: res.email_verified ?? false }
			};
			set(state);
			saveToStorage(state);
			syncCredentialsToAndroid(state.token);
		},
		async register(username: string, email: string, password: string, inviteToken?: string) {
			await apiFetch('/auth/register', {
				method: 'POST',
				body: JSON.stringify({ username, email, password, ...(inviteToken ? { invite_token: inviteToken } : {}) })
			});
		},
		logout() {
			apiFetch('/auth/logout', { method: 'POST' }).catch(() => {});
			doLogout();
		},
		updateEmail(email: string) {
			update((s) => {
				const next = { ...s, user: s.user ? { ...s.user, email, email_verified: false } : s.user };
				saveToStorage(next);
				return next;
			});
		},
		updateEmailVerified(verified: boolean) {
			update((s) => {
				const next = { ...s, user: s.user ? { ...s.user, email_verified: verified } : s.user };
				saveToStorage(next);
				return next;
			});
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
					const res = await fetch(`${getApiBase()}/auth/refresh`, {
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
						syncCredentialsToAndroid(next.token);
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
