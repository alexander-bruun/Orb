import { apiFetch } from '$lib/api/client';
import { getApiBase } from '$lib/api/base';
import { authStore } from '$lib/stores/auth';
import { get } from 'svelte/store';

// ── Types ──────────────────────────────────────────────────────────────────

export interface AdminSummary {
	total_users: number;
	active_users: number;
	total_tracks: number;
	total_albums: number;
	total_artists: number;
	total_plays: number;
	total_played_ms: number;
	total_size_bytes: number;
	albums_no_cover_art: number;
}

export interface UserPlayStat {
	user_id: string;
	username: string;
	email: string;
	is_admin: boolean;
	is_active: boolean;
	storage_quota_bytes?: number | null;
	play_count: number;
	last_login_at?: string;
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

export interface FormatStat {
	format: string;
	count: number;
	size_bytes: number;
}

export interface StorageStats {
	total_size_bytes: number;
	track_count: number;
	by_format: FormatStat[];
}

export interface InviteToken {
	token: string;
	email: string;
	created_by: string;
	created_at: string;
	expires_at: string;
	used_at?: string;
	used_by?: string;
}

export interface AuditLog {
	id: number;
	actor_id?: string;
	actor_name?: string;
	action: string;
	target_type?: string;
	target_id?: string;
	detail?: Record<string, unknown>;
	created_at: string;
}

export interface Album {
	id: string;
	artist_id?: string;
	artist_name?: string;
	title: string;
	release_year?: number;
	mbid?: string;
}

export interface SiteSettings {
	smtp_host?: string;
	smtp_port?: string;
	smtp_username?: string;
	smtp_password?: string;
	smtp_from_address?: string;
	smtp_from_name?: string;
	smtp_tls?: string;
	site_base_url?: string;
}

export interface IngestProgressEvent {
	type: 'progress' | 'complete' | 'error';
	total: number;
	done: number;
	skipped: number;
	errors: number;
	file_path?: string;
	message?: string;
}

// ── API ─────────────────────────────────────────────────────────────────────

export const admin = {
	// Analytics
	summary: (): Promise<AdminSummary> => apiFetch('/admin/summary'),
	users: (): Promise<UserPlayStat[]> => apiFetch('/admin/users'),
	topTracks: (limit = 10): Promise<TrackPlayCount[]> =>
		apiFetch(`/admin/top-tracks?limit=${limit}`),
	topArtists: (limit = 10): Promise<ArtistPlayCount[]> =>
		apiFetch(`/admin/top-artists?limit=${limit}`),
	playsByDay: (days = 30): Promise<DailyPlayCount[]> =>
		apiFetch(`/admin/plays-by-day?days=${days}`),
	storageStats: (): Promise<StorageStats> => apiFetch('/admin/storage'),

	// User management
	setUserAdmin: (userId: string, isAdmin: boolean): Promise<void> =>
		apiFetch(`/admin/users/${userId}/admin`, {
			method: 'PUT',
			body: JSON.stringify({ is_admin: isAdmin })
		}),
	setUserActive: (userId: string, active: boolean): Promise<void> =>
		apiFetch(`/admin/users/${userId}/active`, {
			method: 'PUT',
			body: JSON.stringify({ active })
		}),
	setUserQuota: (userId: string, quotaBytes: number | null): Promise<void> =>
		apiFetch(`/admin/users/${userId}/quota`, {
			method: 'PUT',
			body: JSON.stringify({ quota_bytes: quotaBytes })
		}),
	deleteUser: (userId: string): Promise<void> =>
		apiFetch(`/admin/users/${userId}`, { method: 'DELETE' }),

	// Invites
	createInvite: (
		email: string
	): Promise<{ token: string; invite_url: string; expires_at: string }> =>
		apiFetch('/admin/invites', { method: 'POST', body: JSON.stringify({ email }) }),
	listInvites: (): Promise<InviteToken[]> => apiFetch('/admin/invites'),
	revokeInvite: (token: string): Promise<void> =>
		apiFetch(`/admin/invites/${token}`, { method: 'DELETE' }),

	// Audit log
	auditLogs: (
		limit = 50,
		offset = 0
	): Promise<{ logs: AuditLog[]; total: number }> =>
		apiFetch(`/admin/audit-logs?limit=${limit}&offset=${offset}`),

	// Artwork
	albumsNoCover: (
		limit = 50,
		offset = 0
	): Promise<{ albums: Album[]; total: number }> =>
		apiFetch(`/admin/albums/no-cover?limit=${limit}&offset=${offset}`),
	refetchAlbumCover: (albumId: string): Promise<{ cover_art_key: string }> =>
		apiFetch(`/admin/albums/${albumId}/refetch-cover`, { method: 'POST' }),

	// Ingest
	triggerScan: (): Promise<void> => apiFetch('/admin/ingest/scan', { method: 'POST' }),
	ingestStatus: (): Promise<Record<string, unknown>> => apiFetch('/admin/ingest/status'),

	// Site settings
	getSettings: (): Promise<SiteSettings> => apiFetch('/admin/settings'),
	updateSmtpSettings: (cfg: SiteSettings): Promise<void> =>
		apiFetch('/admin/settings/smtp', { method: 'PUT', body: JSON.stringify(cfg) }),
	testSmtp: (to: string): Promise<void> =>
		apiFetch('/admin/settings/smtp/test', { method: 'POST', body: JSON.stringify({ to }) }),

	// SSE: real-time ingest progress
	openIngestStream(onEvent: (e: IngestProgressEvent) => void): EventSource {
		const token = get(authStore).token ?? '';
		const base = getApiBase().replace(/\/$/, '');
		const es = new EventSource(
			`${base}/admin/ingest/stream?token=${encodeURIComponent(token)}`
		);
		es.onmessage = (e) => {
			try {
				onEvent(JSON.parse(e.data) as IngestProgressEvent);
			} catch {
				// ignore malformed frames
			}
		};
		return es;
	}
};
