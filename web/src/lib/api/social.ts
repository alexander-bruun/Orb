import { apiFetch } from './client';
import type { Playlist } from '$lib/types';

export interface PublicProfile {
	id: string;
	username: string;
	display_name: string;
	bio: string;
	avatar_key: string | null;
	follower_count: number;
	following_count: number;
	playlist_count: number;
	joined_at: string;
}

export interface ActivityRow {
	id: string;
	user_id: string;
	username: string;
	display_name?: string;
	avatar_key?: string;
	type: string;
	entity_type: string;
	entity_id: string;
	metadata?: Record<string, unknown>;
	created_at: string;
}

export interface UserStats {
	total_plays: number;
	total_played_ms: number;
	top_artists: Array<{ artist_id: string; artist_name: string; plays: number }>;
}

export interface UserProfileFields {
	display_name: string;
	bio: string;
	profile_public: boolean;
	avatar_key: string | null;
}

export const social = {
	// Feed
	getFeed: (limit = 20, offset = 0) =>
		apiFetch<ActivityRow[]>(`/social/feed?limit=${limit}&offset=${offset}`),

	hasActivity: () =>
		apiFetch<{ has_activity: boolean }>('/social/has-activity'),

	// Follow / unfollow
	follow: (username: string) =>
		apiFetch<void>(`/social/follow/${encodeURIComponent(username)}`, { method: 'POST' }),

	unfollow: (username: string) =>
		apiFetch<void>(`/social/follow/${encodeURIComponent(username)}`, { method: 'DELETE' }),

	getFollowers: () => apiFetch<unknown[]>('/social/followers'),
	getFollowing: () => apiFetch<unknown[]>('/social/following'),

	// Public profiles
	getProfile: (username: string) =>
		apiFetch<{ profile: PublicProfile; is_following?: boolean }>(
			`/profile/${encodeURIComponent(username)}`
		),

	getProfilePlaylists: (username: string) =>
		apiFetch<Playlist[]>(`/profile/${encodeURIComponent(username)}/playlists`),

	getProfileActivity: (username: string, limit = 20, offset = 0) =>
		apiFetch<ActivityRow[]>(
			`/profile/${encodeURIComponent(username)}/activity?limit=${limit}&offset=${offset}`
		),

	getProfileStats: (username: string) =>
		apiFetch<UserStats>(`/profile/${encodeURIComponent(username)}/stats`),

	// Self profile edit
	getMyProfile: () => apiFetch<UserProfileFields>('/user/profile'),

	updateMyProfile: (fields: { display_name: string; bio: string; profile_public: boolean }) =>
		apiFetch<UserProfileFields>('/user/profile', {
			method: 'PATCH',
			body: JSON.stringify(fields)
		}),

	uploadAvatar: (file: File) => {
		const form = new FormData();
		form.append('avatar', file);
		return apiFetch<{ avatar_key: string }>('/user/avatar', {
			method: 'POST',
			headers: {},      // let fetch set multipart boundary
			body: form
		});
	}
};
