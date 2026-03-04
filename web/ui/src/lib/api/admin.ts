import { apiFetch } from '$lib/api/client';

export interface AdminSummary {
	total_users: number;
	total_tracks: number;
	total_albums: number;
	total_artists: number;
	total_plays: number;
	total_played_ms: number;
}

export interface UserPlayStat {
	user_id: string;
	username: string;
	email: string;
	is_admin: boolean;
	play_count: number;
	created_at: string;
}

export interface TrackPlayCount {
	track_id: string;
	title: string;
	artist_name?: string;
	album_title?: string;
	plays: number;
}

export interface ArtistPlayCount {
	artist_id: string;
	name: string;
	plays: number;
}

export interface DailyPlayCount {
	date: string;
	plays: number;
}

export const admin = {
	summary: (): Promise<AdminSummary> => apiFetch('/admin/summary'),

	users: (): Promise<UserPlayStat[]> => apiFetch('/admin/users'),

	topTracks: (limit = 10): Promise<TrackPlayCount[]> =>
		apiFetch(`/admin/top-tracks?limit=${limit}`),

	topArtists: (limit = 10): Promise<ArtistPlayCount[]> =>
		apiFetch(`/admin/top-artists?limit=${limit}`),

	playsByDay: (days = 30): Promise<DailyPlayCount[]> =>
		apiFetch(`/admin/plays-by-day?days=${days}`),

	setUserAdmin: (userId: string, isAdmin: boolean): Promise<void> =>
		apiFetch(`/admin/users/${userId}/admin`, {
			method: 'PUT',
			body: JSON.stringify({ is_admin: isAdmin })
		})
};
