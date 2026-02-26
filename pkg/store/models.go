package store

import "time"

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
	ID           string     `json:"id"`
	Username     string     `json:"username"`
	Email        string     `json:"email"`
	PasswordHash string     `json:"-"`
	CreatedAt    time.Time  `json:"created_at"`
	LastLoginAt  *time.Time `json:"last_login_at,omitempty"`
}

// Artist represents an artist in the database.
type Artist struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	SortName  string    `json:"sort_name"`
	Mbid      *string   `json:"mbid,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

// Album represents an album in the database.
type Album struct {
	ID          string    `json:"id"`
	ArtistID    *string   `json:"artist_id,omitempty"`
	ArtistName  *string   `json:"artist_name,omitempty"`
	Title       string    `json:"title"`
	ReleaseYear *int      `json:"release_year,omitempty"`
	Label       *string   `json:"label,omitempty"`
	CoverArtKey *string   `json:"cover_art_key,omitempty"`
	Mbid        *string   `json:"mbid,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
}

// Track represents a track in the database.
type Track struct {
	ID          string    `json:"id"`
	AlbumID     *string   `json:"album_id,omitempty"`
	ArtistID    *string   `json:"artist_id,omitempty"`
	Title       string    `json:"title"`
	TrackNumber *int      `json:"track_number,omitempty"`
	DiscNumber  int       `json:"disc_number"`
	DurationMs  int       `json:"duration_ms"`
	FileKey     string    `json:"file_key"`
	FileSize    int64     `json:"file_size"`
	Format      string    `json:"format"`
	BitDepth    *int      `json:"bit_depth,omitempty"`
	SampleRate  int       `json:"sample_rate"`
	Channels    int       `json:"channels"`
	BitrateKbps *int      `json:"bitrate_kbps,omitempty"`
	SeekTable   []byte    `json:"seek_table,omitempty"`
	Fingerprint string    `json:"fingerprint,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
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
	ID          string
	ArtistID    *string
	Title       string
	ReleaseYear *int
	Label       *string
	CoverArtKey *string
	Mbid        *string
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
	ToTsquery string
	Limit     int
}

// SearchAlbumsParams for searching albums.
type SearchAlbumsParams struct {
	ToTsquery string
	Limit     int
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
}

// IngestStateRow is a row from the ingest_state table.
// The ingest tool bulk-loads these at startup to avoid per-file DB queries.
type IngestStateRow struct {
	Path      string
	MtimeUnix int64
	FileSize  int64
	TrackID   string
}
