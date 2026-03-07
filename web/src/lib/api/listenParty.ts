import { apiFetch } from './client';

export interface SessionInfo {
	session_id: string;
	host_name: string;
	participant_count: number;
	created_at: string;
	code_enabled: boolean;
}

export const listenPartyApi = {
	createSession(): Promise<{ session_id: string }> {
		return apiFetch('/listen', { method: 'POST' });
	},

	getSession(id: string): Promise<SessionInfo> {
		return apiFetch(`/listen/${id}`);
	},

	endSession(id: string): Promise<void> {
		return apiFetch(`/listen/${id}`, { method: 'DELETE' }) as unknown as Promise<void>;
	},

	/** Enable code protection (or regenerate the code) for a session. Returns the new 4-digit code. */
	enableCode(id: string): Promise<{ code: string }> {
		return apiFetch(`/listen/${id}/code`, { method: 'POST' });
	},

	/** Disable code protection for a session. */
	disableCode(id: string): Promise<void> {
		return apiFetch(`/listen/${id}/code`, { method: 'DELETE' }) as unknown as Promise<void>;
	},
};
