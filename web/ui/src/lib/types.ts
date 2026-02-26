export interface User {
	id: string;
	username: string;
	email: string;
}

export interface Artist {
	id: string;
	name: string;
	sort_name: string;
	mbid?: string;
}

export interface Album {
	id: string;
    artist_id?: string;
	title: string;
	release_year?: number;
	label?: string;
	cover_art_key?: string;
    artist_name?: string;
    artist?: Artist;
    track_count?: number;
}

export interface Track {
	id: string;
	album_id?: string;
    artist_id?: string;
	title: string;
	track_number?: number;
	disc_number: number;
	duration_ms: number;
	file_key: string;
	file_size: number;
	format: 'flac' | 'wav' | 'mp3';
	bit_depth?: number;
	sample_rate: number;
	channels: number;
	bitrate_kbps?: number;
    artist_name?: string;
    artist?: Artist;
    featured_artist_ids?: string[];
    featured_artists?: Artist[];
}

export interface Playlist {
	id: string;
	user_id: string;
	name: string;
	description?: string;
	cover_art_key?: string;
	created_at: string;
	updated_at: string;
}

export type PlaybackState = 'idle' | 'loading' | 'playing' | 'paused';
