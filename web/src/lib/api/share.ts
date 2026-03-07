import { apiFetch } from './client';
import { getApiBase } from './base';
import type { Track, Album } from '$lib/types';

export type ShareType = 'track' | 'album';

export interface CreateShareResp {
	token: string;
}

export interface RedeemShareResp {
	type: ShareType;
	track?: Track;
	album?: Album;
	tracks?: Track[];
	stream_session: string;
	session_ttl_seconds: number;
}

export const share = {
	/** Create a one-time share token for a track or album. Requires auth. */
	create(type: ShareType, id: string): Promise<CreateShareResp> {
		return apiFetch<CreateShareResp>('/share/', {
			method: 'POST',
			body: JSON.stringify({ type, id })
		});
	},

	/** Redeem a one-time share token. Public endpoint — no auth needed. */
	redeem(token: string): Promise<RedeemShareResp> {
		return apiFetch<RedeemShareResp>(`/share/${token}`);
	},

	/** Build a streaming URL for a track within a share streaming session. */
	streamUrl(sessionToken: string, trackId: string): string {
		return `${getApiBase()}/share/stream/${sessionToken}/${trackId}`;
	}
};

