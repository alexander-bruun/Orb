export interface User {
	id: string;
	username: string;
	email: string;
}

export interface Genre {
	id: string;
	name: string;
}

export interface RelatedArtist {
	artist_id: string;
	related_id: string;
	rel_type: string;
	artist_name?: string;
}

export interface Artist {
	id: string;
	name: string;
	sort_name: string;
	mbid?: string;
	artist_type?: string;
	country?: string;
	begin_date?: string;
	end_date?: string;
	disambiguation?: string;
	image_key?: string;
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
	album_type?: string;
	release_date?: string;
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
	isrc?: string;
	mbid?: string;
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
