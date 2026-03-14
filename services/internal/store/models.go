package store

import (
	"encoding/json"
	"time"
)

// Playlist represents a playlist in the database.
type Playlist struct {
	ID          string  `json:"id"`
	UserID      string  `json:"user_id"`
	Name        string  `json:"name"`
	Description string  `json:"description,omitempty"`
	CoverArtKey *string `json:"cover_art_key,omitempty"`
	CreatedAt   string  `json:"created_at"`
}

// User represents a user in the database.
type User struct {
	ID                 string     `json:"id"`
	Username           string     `json:"username"`
	Email              string     `json:"email"`
	PasswordHash       string     `json:"-"`
	CreatedAt          time.Time  `json:"created_at"`
	LastLoginAt        *time.Time `json:"last_login_at,omitempty"`
	TOTPSecret         *string    `json:"-"`
	TOTPEnabled        bool       `json:"totp_enabled"`
	TOTPBackupCodes    *string    `json:"-"`
	IsAdmin            bool       `json:"is_admin"`
	IsActive           bool       `json:"is_active"`
	StorageQuotaBytes  *int64     `json:"storage_quota_bytes,omitempty"`
}

// Artist represents an artist in the database.
type Artist struct {
	ID             string     `json:"id"`
	Name           string     `json:"name"`
	SortName       string     `json:"sort_name"`
	Mbid           *string    `json:"mbid,omitempty"`
	ArtistType     *string    `json:"artist_type,omitempty"`
	Country        *string    `json:"country,omitempty"`
	BeginDate      *string    `json:"begin_date,omitempty"`
	EndDate        *string    `json:"end_date,omitempty"`
	Disambiguation *string    `json:"disambiguation,omitempty"`
	ImageKey       *string    `json:"image_key,omitempty"`
	EnrichedAt     *time.Time `json:"enriched_at,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
}

// Album represents an album in the database.
type Album struct {
	ID               string     `json:"id"`
	ArtistID         *string    `json:"artist_id,omitempty"`
	ArtistName       *string    `json:"artist_name,omitempty"`
	Title            string     `json:"title"`
	ReleaseYear      *int       `json:"release_year,omitempty"`
	Label            *string    `json:"label,omitempty"`
	CoverArtKey      *string    `json:"cover_art_key,omitempty"`
	Mbid             *string    `json:"mbid,omitempty"`
	AlbumType        *string    `json:"album_type,omitempty"`
	ReleaseDate      *string    `json:"release_date,omitempty"`
	ReleaseGroupMbid *string    `json:"release_group_mbid,omitempty"`
	EnrichedAt       *time.Time `json:"enriched_at,omitempty"`
	AlbumGroupID     *string    `json:"album_group_id,omitempty"`
	Edition          *string    `json:"edition,omitempty"`
	TrackCount       int        `json:"track_count"`
	CreatedAt        time.Time  `json:"created_at"`
}

// Track represents a track in the database.
type Track struct {
	ID          string     `json:"id"`
	AlbumID     *string    `json:"album_id,omitempty"`
	ArtistID    *string    `json:"artist_id,omitempty"`
	Title       string     `json:"title"`
	TrackNumber *int       `json:"track_number,omitempty"`
	DiscNumber  int        `json:"disc_number"`
	DurationMs  int        `json:"duration_ms"`
	FileKey     string     `json:"file_key"`
	FileSize    int64      `json:"file_size"`
	Format      string     `json:"format"`
	BitDepth    *int       `json:"bit_depth,omitempty"`
	SampleRate  int        `json:"sample_rate"`
	Channels    int        `json:"channels"`
	BitrateKbps *int       `json:"bitrate_kbps,omitempty"`
	SeekTable   []byte     `json:"seek_table,omitempty"`
	Fingerprint string     `json:"fingerprint,omitempty"`
	Isrc        *string    `json:"isrc,omitempty"`
	Mbid        *string    `json:"mbid,omitempty"`
	EnrichedAt  *time.Time `json:"enriched_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	// ReplayGainTrack is the track-level ReplayGain value in dB (from track_features).
	// Nil when no ReplayGain data is available.
	ReplayGainTrack *float64 `json:"replay_gain_track,omitempty"`
	// BPM is the track tempo in beats per minute (from track_features). Nil when unknown.
	BPM *float64 `json:"bpm,omitempty"`
}

// UpsertArtistParams for upserting an artist.
type UpsertArtistParams struct {
	ID       string
	Name     string
	SortName string
	Mbid     *string
}

// UpsertAlbumParams for upserting an album.
type UpsertAlbumParams struct {
	ID           string
	ArtistID     *string
	Title        string
	ReleaseYear  *int
	Label        *string
	CoverArtKey  *string
	Mbid         *string
	AlbumGroupID *string
	Edition      *string
}

// UpsertTrackParams for upserting a track.
type UpsertTrackParams struct {
	ID          string
	AlbumID     *string
	ArtistID    *string
	Title       string
	TrackNumber *int
	DiscNumber  int
	DurationMs  int
	FileKey     string
	FileSize    int64
	Format      string
	BitDepth    *int
	SampleRate  int
	Channels    int
	BitrateKbps *int
	SeekTable   []byte
	Fingerprint string
}

// AddTrackToLibraryParams for adding a track to a user's library.
type AddTrackToLibraryParams struct {
	UserID  string
	TrackID string
}

// CreateUserParams for creating a user.
type CreateUserParams struct {
	ID           string
	Username     string
	Email        string
	PasswordHash string
}

// ListTracksByUserParams for listing tracks by user.
type ListTracksByUserParams struct {
	UserID string
	Limit  int32
	Offset int32
}

// ListAlbumsParams for listing albums.
type ListAlbumsParams struct {
	Limit  int32
	Offset int32
	SortBy string // "title" | "artist" | "year"; defaults to "title"
}

// ListArtistsParams for listing artists.
type ListArtistsParams struct {
	Limit  int32
	Offset int32
}

// CreatePlaylistParams for creating a playlist.
type CreatePlaylistParams struct {
	ID          string
	UserID      string
	Name        string
	Description string
	CoverArtKey string
}

// UpdatePlaylistParams for updating a playlist.
type UpdatePlaylistParams struct {
	ID          string
	Name        string
	Description string
	CoverArtKey string
}

// DeletePlaylistParams for deleting a playlist.
type DeletePlaylistParams struct {
	ID string
}

// InsertQueueEntryParams for inserting a queue entry.
type InsertQueueEntryParams struct {
	UserID   string
	TrackID  string
	Position int
	// Add Source field for queue operations
	Source string
}

// RemoveTrackFromLibraryParams for removing a track from a user's library.
type RemoveTrackFromLibraryParams struct {
	UserID  string
	TrackID string
}

// SearchTracksParams for searching tracks.
type SearchTracksParams struct {
	ToTsquery  string
	Limit      int
	Genre      string // filter by genre name (case-insensitive, empty = no filter)
	YearFrom   *int   // filter by album release_year >= YearFrom
	YearTo     *int   // filter by album release_year <= YearTo
	Format     string // filter by format (flac/mp3/wav etc., empty = no filter)
	BitrateMin *int     // filter by bitrate_kbps >= BitrateMin
	BitrateMax *int     // filter by bitrate_kbps <= BitrateMax
	BPMMin     *float64 // filter by bpm >= BPMMin
	BPMMax     *float64 // filter by bpm <= BPMMax
	SortBy     string   // "relevance" | "title" | "year" | "bitrate" | "duration" | "bpm"
}

// SearchAlbumsParams for searching albums.
type SearchAlbumsParams struct {
	ToTsquery string
	Limit     int
	Genre     string // filter by genre name (case-insensitive, empty = no filter)
	YearFrom  *int   // filter by release_year >= YearFrom
	YearTo    *int   // filter by release_year <= YearTo
	SortBy    string // "relevance" | "title" | "year"
}

// SearchArtistsParams for searching artists.
type SearchArtistsParams struct {
	ToTsquery string
	Limit     int
}

// AddTrackToPlaylistParams for adding a track to a playlist.
type AddTrackToPlaylistParams struct {
	PlaylistID string
	TrackID    string
	Position   int
}

// RemoveTrackFromPlaylistParams for removing a track from a playlist.
type RemoveTrackFromPlaylistParams struct {
	PlaylistID string
	TrackID    string
}

// UpdatePlaylistTrackOrderParams for reordering tracks in a playlist.
type UpdatePlaylistTrackOrderParams struct {
	PlaylistID string
	TrackID    string
	Position   int32
}

// ListRecentlyPlayedParams for recently played tracks.
type ListRecentlyPlayedParams struct {
	UserID string
	Limit  int
	From   *time.Time
	To     *time.Time
}

// ListMostPlayedParams for most-played tracks.
type ListMostPlayedParams struct {
	UserID string
	Limit  int
	From   *time.Time
	To     *time.Time
}

// FavoriteParams for adding or removing a favorite.
type FavoriteParams struct {
	UserID  string
	TrackID string
}

// RecordPlayParams for recording a track play.
type RecordPlayParams struct {
	UserID           string
	TrackID          string
	DurationPlayedMs int
}

// ListRecentAlbumsParams for listing recently added albums.
type ListRecentAlbumsParams struct {
	Limit int
}

// IngestStateRow is a row from the ingest_state table.
// The ingest tool bulk-loads these at startup to avoid per-file DB queries.
type IngestStateRow struct {
	Path      string
	MtimeUnix int64
	FileSize  int64
	TrackID   string
}

// Genre represents a genre tag.
type Genre struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// RelatedArtist represents a relationship between two artists.
type RelatedArtist struct {
	ArtistID   string `json:"artist_id"`
	RelatedID  string `json:"related_id"`
	RelType    string `json:"rel_type"`
	ArtistName string `json:"artist_name,omitempty"`
}

// UpdateArtistEnrichmentParams for enriching artist metadata from MusicBrainz.
type UpdateArtistEnrichmentParams struct {
	ID             string
	Mbid           *string
	ArtistType     *string
	Country        *string
	BeginDate      *string
	EndDate        *string
	Disambiguation *string
	ImageKey       *string
}

// UpdateAlbumEnrichmentParams for enriching album metadata from MusicBrainz.
type UpdateAlbumEnrichmentParams struct {
	ID               string
	Mbid             *string
	Label            *string
	AlbumType        *string
	ReleaseDate      *string
	ReleaseGroupMbid *string
}

// UpdateTrackEnrichmentParams for enriching track metadata from MusicBrainz.
type UpdateTrackEnrichmentParams struct {
	ID   string
	Mbid *string
	Isrc *string
}

// TrackFeatures holds in-house audio features extracted during ingest.
type TrackFeatures struct {
	TrackID     string  `json:"track_id"`
	BPM         float64 `json:"bpm"`          // 0 = unknown
	KeyEstimate string  `json:"key_estimate"` // e.g. "Cm", "F#"; "" = unknown
	ReplayGain  float64 `json:"replay_gain"`  // track gain in dB; 0 = not set
}

// TrackSimilarityRow is a row in the track_similarity table.
type TrackSimilarityRow struct {
	TrackA string  `json:"track_a"`
	TrackB string  `json:"track_b"`
	Score  float64 `json:"score"`
}

// TrackWithScore is a Track with an attached similarity score.
type TrackWithScore struct {
	Track
	Score      float64 `json:"similarity_score"`
	ArtistName *string `json:"artist_name,omitempty"`
}

// TrackBasic holds minimal track info needed for bulk similarity computation.
// Deprecated: use TrackInfoFull instead.
type TrackBasic struct {
	ID         string
	ArtistID   string
	AlbumID    string
	DurationMs int
}

// TrackInfoFull holds all track data needed for the multi-signal similarity
// algorithm, loaded in a single query joining tracks, albums, and artists.
type TrackInfoFull struct {
	ID           string
	ArtistID     string
	AlbumID      string
	Title        string
	DurationMs   int
	Format       string
	BitDepth     int // 0 when not set (lossy formats)
	SampleRate   int
	Channels     int
	BitrateKbps  int    // 0 when not set (lossless formats)
	ReleaseYear  int    // 0 when unknown; from the track's album
	AlbumType    string // "Album" | "EP" | "Single" | "Live" | etc.
	AlbumGroupID string // shared across editions of the same record
	Country      string // ISO 3166-1 alpha-2; from the track's artist
	ArtistType   string // "Person" | "Group" | etc.
}

// CoPlayPair holds a pair of tracks that were played in the same listening
// session, together with the number of distinct users that co-played them.
type CoPlayPair struct {
	TrackA string
	TrackB string
	Count  int
}

// RelatedArtistPair is a minimal related-artist edge.
type RelatedArtistPair struct {
	ArtistID  string
	RelatedID string
}

// UserStreamingPrefs holds per-user streaming quality limits.
// NULL fields mean "no limit". The top-level Max* fields are the "any network" defaults;
// Wifi* and Mobile* fields override them when the client is on a specific network type.
type UserStreamingPrefs struct {
	UserID               string    `json:"user_id"`
	MaxBitrateKbps       *int      `json:"max_bitrate_kbps"`        // NULL = unlimited (any network default)
	MaxSampleRate        *int      `json:"max_sample_rate"`         // NULL = unlimited (advisory)
	MaxBitDepth          *int      `json:"max_bit_depth"`           // NULL = unlimited (advisory)
	WifiMaxBitrateKbps   *int      `json:"wifi_max_bitrate_kbps"`   // NULL = inherit default
	WifiMaxSampleRate    *int      `json:"wifi_max_sample_rate"`    // NULL = inherit default
	WifiMaxBitDepth      *int      `json:"wifi_max_bit_depth"`      // NULL = inherit default
	MobileMaxBitrateKbps *int      `json:"mobile_max_bitrate_kbps"` // NULL = inherit default
	MobileMaxSampleRate  *int      `json:"mobile_max_sample_rate"`  // NULL = inherit default
	MobileMaxBitDepth    *int      `json:"mobile_max_bit_depth"`    // NULL = inherit default
	UpdatedAt            time.Time `json:"updated_at"`
}

// UpsertUserStreamingPrefsParams holds the parameters for upserting streaming prefs.

type UpsertUserStreamingPrefsParams struct {
	UserID               string
	MaxBitrateKbps       *int
	MaxSampleRate        *int
	MaxBitDepth          *int
	WifiMaxBitrateKbps   *int
	WifiMaxSampleRate    *int
	WifiMaxBitDepth      *int
	MobileMaxBitrateKbps *int
	MobileMaxSampleRate  *int
	MobileMaxBitDepth    *int
}

// EQBand represents a single band in a parametric equalizer.
type EQBand struct {
	Frequency float64 `json:"frequency"` // center frequency in Hz
	Gain      float64 `json:"gain"`      // dB, range [-12, +12]
	Type      string  `json:"type"`      // "lowshelf" | "peaking" | "highshelf"
}

// EQProfile represents a named equalizer preset owned by a user.
type EQProfile struct {
	ID        string    `json:"id"`
	UserID    string    `json:"user_id"`
	Name      string    `json:"name"`
	Bands     []EQBand  `json:"bands"`
	IsDefault bool      `json:"is_default"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// GenreEQMapping associates a genre with an EQ profile for a specific user.
type GenreEQMapping struct {
	UserID    string `json:"user_id"`
	GenreID   string `json:"genre_id"`
	GenreName string `json:"genre_name,omitempty"`
	ProfileID string `json:"profile_id"`
}

// CreateEQProfileParams holds the parameters for creating an EQ profile.
type CreateEQProfileParams struct {
	ID        string
	UserID    string
	Name      string
	Bands     []EQBand
	IsDefault bool
}

// UpdateEQProfileParams holds the parameters for updating an EQ profile.
type UpdateEQProfileParams struct {
	ID     string
	UserID string
	Name   string
	Bands  []EQBand
}

// --- Admin analytics ---

// AdminSummary holds high-level server statistics.
type AdminSummary struct {
	TotalUsers        int   `json:"total_users"`
	ActiveUsers       int   `json:"active_users"`
	TotalTracks       int   `json:"total_tracks"`
	TotalAlbums       int   `json:"total_albums"`
	TotalArtists      int   `json:"total_artists"`
	TotalPlays        int   `json:"total_plays"`
	TotalPlayedMs     int   `json:"total_played_ms"`
	TotalSizeBytes    int64 `json:"total_size_bytes"`
	AlbumsNoCoverArt  int   `json:"albums_no_cover_art"`
}

// InviteToken represents an admin-generated invite for a new user.
type InviteToken struct {
	Token     string     `json:"token"`
	Email     string     `json:"email"`
	CreatedBy string     `json:"created_by"`
	CreatedAt time.Time  `json:"created_at"`
	ExpiresAt time.Time  `json:"expires_at"`
	UsedAt    *time.Time `json:"used_at,omitempty"`
	UsedBy    *string    `json:"used_by,omitempty"`
}

// AuditLog records an admin or system action.
type AuditLog struct {
	ID         int64           `json:"id"`
	ActorID    *string         `json:"actor_id,omitempty"`
	ActorName  *string         `json:"actor_name,omitempty"`
	Action     string          `json:"action"`
	TargetType string          `json:"target_type,omitempty"`
	TargetID   string          `json:"target_id,omitempty"`
	Detail     json.RawMessage `json:"detail,omitempty"`
	CreatedAt  time.Time       `json:"created_at"`
}

// FormatStat holds track count and total size for a given audio format.
type FormatStat struct {
	Format     string `json:"format"`
	Count      int    `json:"count"`
	SizeBytes  int64  `json:"size_bytes"`
}

// StorageStats holds disk usage statistics for the library.
type StorageStats struct {
	TotalSizeBytes int64        `json:"total_size_bytes"`
	TrackCount     int          `json:"track_count"`
	ByFormat       []FormatStat `json:"by_format"`
}

// UserPlayStat holds a user's listening statistics.
type UserPlayStat struct {
	UserID            string     `json:"user_id"`
	Username          string     `json:"username"`
	Email             string     `json:"email"`
	IsAdmin           bool       `json:"is_admin"`
	IsActive          bool       `json:"is_active"`
	StorageQuotaBytes *int64     `json:"storage_quota_bytes,omitempty"`
	PlayCount         int        `json:"play_count"`
	LastLoginAt       *time.Time `json:"last_login_at,omitempty"`
	CreatedAt         time.Time  `json:"created_at"`
}

// TrackPlayCount holds a track's total play count.
type TrackPlayCount struct {
	TrackID    string  `json:"track_id"`
	Title      string  `json:"title"`
	ArtistName *string `json:"artist_name,omitempty"`
	AlbumTitle *string `json:"album_title,omitempty"`
	Plays      int     `json:"plays"`
}

// ArtistPlayCount holds an artist's total play count.
type ArtistPlayCount struct {
	ArtistID string `json:"artist_id"`
	Name     string `json:"name"`
	Plays    int    `json:"plays"`
}

// DailyPlayCount holds the number of plays on a given day.
type DailyPlayCount struct {
	Date  string `json:"date"` // YYYY-MM-DD
	Plays int    `json:"plays"`
}
