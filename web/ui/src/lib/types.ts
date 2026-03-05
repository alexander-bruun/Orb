export interface User {
	id: string;
	username: string;
	email: string;
	is_admin?: boolean;
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
	album_group_id?: string;
	edition?: string;
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
    album_name?: string;
    artist?: Artist;
    featured_artist_ids?: string[];
    featured_artists?: Artist[];
	/** Track-level ReplayGain offset in dB. Undefined when no ReplayGain data is available. */
	replay_gain_track?: number;
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

/** Filters for the advanced search. All fields are optional. */
export interface SearchFilters {
	genre?: string;
	year_from?: number;
	year_to?: number;
	format?: string;
	bitrate_min?: number;
	bitrate_max?: number;
	/** Which result sections to show. Defaults to all three. */
	types?: ('tracks' | 'albums' | 'artists')[];
	/** Sort for track results: relevance | title | year | bitrate | duration */
	sort_tracks?: string;
	/** Sort for album results: relevance | title | year */
	sort_albums?: string;
}

/** A saved search filter preset stored in localStorage. */
export interface SavedFilter {
	name: string;
	filters: SearchFilters;
	createdAt: string;
}

// ──────────────────────────────────────────────────────────────
// Equalizer
// ──────────────────────────────────────────────────────────────

export type EQBandType = 'lowshelf' | 'peaking' | 'highshelf';

export interface EQBand {
	frequency: number; // Hz
	gain: number;      // dB, range [-12, +12]
	type: EQBandType;
}

export interface EQProfile {
	id: string;
	user_id: string;
	name: string;
	bands: EQBand[];
	is_default: boolean;
	created_at: string;
	updated_at: string;
}

export interface GenreEQMapping {
	user_id: string;
	genre_id: string;
	genre_name?: string;
	profile_id: string;
}

/** Default flat 10-band EQ configuration. */
export const DEFAULT_EQ_BANDS: EQBand[] = [
	{ frequency: 31,    gain: 0, type: 'lowshelf'  },
	{ frequency: 62,    gain: 0, type: 'peaking'   },
	{ frequency: 125,   gain: 0, type: 'peaking'   },
	{ frequency: 250,   gain: 0, type: 'peaking'   },
	{ frequency: 500,   gain: 0, type: 'peaking'   },
	{ frequency: 1000,  gain: 0, type: 'peaking'   },
	{ frequency: 2000,  gain: 0, type: 'peaking'   },
	{ frequency: 4000,  gain: 0, type: 'peaking'   },
	{ frequency: 8000,  gain: 0, type: 'peaking'   },
	{ frequency: 16000, gain: 0, type: 'highshelf' },
];
