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
	email_verified: boolean;
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

export interface Webhook {
	id: string;
	url: string;
	secret: string;
	events: string[];
	enabled: boolean;
	description: string;
	created_at: string;
	updated_at: string;
}

export interface WebhookDelivery {
	id: number;
	webhook_id: string;
	event: string;
	payload: Record<string, unknown>;
	status_code?: number;
	error?: string;
	delivered_at: string;
}

export interface CreateWebhookBody {
	url: string;
	secret: string;
	events: string[];
	description: string;
}

export interface UpdateWebhookBody {
	url: string;
	secret: string;
	events: string[];
	enabled: boolean;
	description: string;
}

export interface ServerLogTail {
	path: string;
	exists: boolean;
	lines: string[];
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
	logs: (lines = 300): Promise<ServerLogTail> =>
		apiFetch(`/admin/logs?lines=${lines}`),

	// Artwork
	albumsNoCover: (
		limit = 50,
		offset = 0
	): Promise<{ albums: Album[]; total: number }> =>
		apiFetch(`/admin/albums/no-cover?limit=${limit}&offset=${offset}`),
	refetchAlbumCover: (albumId: string): Promise<{ cover_art_key: string }> =>
		apiFetch(`/admin/albums/${albumId}/refetch-cover`, { method: 'POST' }),
	uploadAlbumCover: async (albumId: string, file: File): Promise<{ cover_art_key: string }> => {
		const token = get(authStore).token ?? '';
		const body = new FormData();
		body.append('cover', file);
		const res = await fetch(`${getApiBase()}/admin/albums/${albumId}/cover`, {
			method: 'PUT',
			headers: token ? { Authorization: `Bearer ${token}` } : {},
			body
		});
		if (!res.ok) {
			let msg = res.statusText;
			try { const d = await res.json(); msg = d.error ?? msg; } catch { /* ignore */ }
			throw new Error(msg);
		}
		return res.json();
	},
	refetchAudiobookCover: (id: string): Promise<{ cover_art_key: string }> =>
		apiFetch(`/admin/audiobooks/${id}/refetch-cover`, { method: 'POST' }),
	uploadAudiobookCover: async (id: string, file: File): Promise<{ cover_art_key: string }> => {
		const token = get(authStore).token ?? '';
		const body = new FormData();
		body.append('cover', file);
		const res = await fetch(`${getApiBase()}/admin/audiobooks/${id}/cover`, {
			method: 'PUT',
			headers: token ? { Authorization: `Bearer ${token}` } : {},
			body
		});
		if (!res.ok) {
			let msg = res.statusText;
			try { const d = await res.json(); msg = d.error ?? msg; } catch { /* ignore */ }
			throw new Error(msg);
		}
		return res.json();
	},

	// Metadata editing
	updateAlbumMeta: (id: string, body: { title: string; release_year?: number | null; label?: string | null }): Promise<void> =>
		apiFetch(`/admin/albums/${id}`, { method: 'PATCH', body: JSON.stringify(body) }),
	updateTrackMeta: (id: string, body: { title: string; track_number?: number | null; disc_number?: number }): Promise<void> =>
		apiFetch(`/admin/tracks/${id}`, { method: 'PATCH', body: JSON.stringify(body) }),
	updateAudiobookMeta: (id: string, body: {
		title: string;
		author_name?: string | null;
		description?: string | null;
		series?: string | null;
		series_index?: number | null;
		published_year?: number | null;
	}): Promise<void> =>
		apiFetch(`/admin/audiobooks/${id}`, { method: 'PATCH', body: JSON.stringify(body) }),
	refreshAudiobookMeta: (id: string): Promise<{ status: string }> =>
		apiFetch(`/admin/audiobooks/${id}/refresh`, { method: 'POST' }),

	// Ingest
	triggerScan: (): Promise<void> => apiFetch('/admin/ingest/scan', { method: 'POST' }),
	triggerForceScan: (): Promise<void> => apiFetch('/admin/ingest/scan?force=true', { method: 'POST' }),
	reingestAlbum: (albumID: string): Promise<void> => apiFetch(`/admin/ingest/album/${albumID}`, { method: 'POST' }),
	ingestStatus: (): Promise<Record<string, unknown>> => apiFetch('/admin/ingest/status'),

	// Site settings
	getSettings: (): Promise<SiteSettings> => apiFetch('/admin/settings'),
	updateSmtpSettings: (cfg: SiteSettings): Promise<void> =>
		apiFetch('/admin/settings/smtp', { method: 'PUT', body: JSON.stringify(cfg) }),
	testSmtp: (to: string): Promise<void> =>
		apiFetch('/admin/settings/smtp/test', { method: 'POST', body: JSON.stringify({ to }) }),
	updateIntegrationSettings: (cfg: {
		ticketmaster_api_key?: string;
		spotify_client_id?: string;
		spotify_client_secret?: string;
		spotify_frontend_url?: string;
	}): Promise<void> =>
		apiFetch('/admin/settings/integrations', { method: 'PUT', body: JSON.stringify(cfg) }),

	// Webhooks
	listWebhookEvents: (): Promise<string[]> => apiFetch('/admin/webhooks/events'),
	listWebhooks: (): Promise<Webhook[]> => apiFetch('/admin/webhooks'),
	createWebhook: (body: CreateWebhookBody): Promise<Webhook> =>
		apiFetch('/admin/webhooks', { method: 'POST', body: JSON.stringify(body) }),
	getWebhook: (id: string): Promise<Webhook> => apiFetch(`/admin/webhooks/${id}`),
	updateWebhook: (id: string, body: UpdateWebhookBody): Promise<Webhook> =>
		apiFetch(`/admin/webhooks/${id}`, { method: 'PUT', body: JSON.stringify(body) }),
	deleteWebhook: (id: string): Promise<void> =>
		apiFetch(`/admin/webhooks/${id}`, { method: 'DELETE' }),
	listWebhookDeliveries: (id: string, limit = 50): Promise<WebhookDelivery[]> =>
		apiFetch(`/admin/webhooks/${id}/deliveries?limit=${limit}`),
	testWebhook: (id: string): Promise<void> =>
		apiFetch(`/admin/webhooks/${id}/test`, { method: 'POST' }),

	// System backup / restore
	backupDownloadURL: (): string => {
		const token = get(authStore).token ?? '';
		return `${getApiBase()}/admin/backup?token=${encodeURIComponent(token)}`;
	},
	restoreBackup: async (file: File): Promise<{ status: string }> => {
		const token = get(authStore).token ?? '';
		const body = new FormData();
		body.append('backup', file);
		const res = await fetch(`${getApiBase()}/admin/restore`, {
			method: 'POST',
			headers: token ? { Authorization: `Bearer ${token}` } : {},
			body
		});
		if (!res.ok) {
			let msg = res.statusText;
			try {
				const data = await res.json();
				msg = data.error ?? msg;
			} catch {
				// ignore
			}
			throw new Error(msg);
		}
		return res.json();
	},

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
