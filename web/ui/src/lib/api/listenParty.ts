import { apiFetch } from './client';

export interface SessionInfo {
	session_id: string;
	host_name: string;
	participant_count: number;
	created_at: string;
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
};
