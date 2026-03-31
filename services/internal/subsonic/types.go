// Package subsonic implements the Subsonic / OpenSubsonic REST API.
// Spec: http://www.subsonic.org/pages/api.jsp
// OpenSubsonic extensions: https://opensubsonic.netlify.app/
package subsonic

import "encoding/xml"

// apiVersion is the Subsonic REST API version we claim compatibility with.
const apiVersion = "1.16.1"

// ── Envelope ──────────────────────────────────────────────────────────────────

// Response is the top-level XML/JSON envelope.
type Response struct {
	XMLName        xml.Name `xml:"subsonic-response" json:"-"`
	XMLNS          string   `xml:"xmlns,attr"                   json:"-"`
	Status         string   `xml:"status,attr"                  json:"status"`
	Version        string   `xml:"version,attr"                 json:"version"`
	Type           string   `xml:"type,attr"                    json:"type"`
	ServerVersion  string   `xml:"serverVersion,attr"           json:"serverVersion"`
	OpenSubsonic   bool     `xml:"openSubsonic,attr"            json:"openSubsonic"`

	// Only one of these should be populated per response.
	Error                  *SubsonicError          `xml:"error,omitempty"                    json:"error,omitempty"`
	License                *License                `xml:"license,omitempty"                  json:"license,omitempty"`
	MusicFolders           *MusicFolders           `xml:"musicFolders,omitempty"             json:"musicFolders,omitempty"`
	Indexes                *Indexes                `xml:"indexes,omitempty"                  json:"indexes,omitempty"`
	Artists                *ArtistsID3             `xml:"artists,omitempty"                  json:"artists,omitempty"`
	Directory              *Directory              `xml:"directory,omitempty"                json:"directory,omitempty"`
	Artist                 *ArtistWithAlbumsID3    `xml:"artist,omitempty"                   json:"artist,omitempty"`
	Album                  *AlbumWithSongsID3      `xml:"album,omitempty"                    json:"album,omitempty"`
	Song                   *Child                  `xml:"song,omitempty"                     json:"song,omitempty"`
	AlbumList              *AlbumList              `xml:"albumList,omitempty"                json:"albumList,omitempty"`
	AlbumList2             *AlbumList2             `xml:"albumList2,omitempty"               json:"albumList2,omitempty"`
	RandomSongs            *Songs                  `xml:"randomSongs,omitempty"              json:"randomSongs,omitempty"`
	SongsByGenre           *Songs                  `xml:"songsByGenre,omitempty"             json:"songsByGenre,omitempty"`
	NowPlaying             *NowPlaying             `xml:"nowPlaying,omitempty"               json:"nowPlaying,omitempty"`
	SearchResult2          *SearchResult2          `xml:"searchResult2,omitempty"            json:"searchResult2,omitempty"`
	SearchResult3          *SearchResult3          `xml:"searchResult3,omitempty"            json:"searchResult3,omitempty"`
	Playlists              *Playlists              `xml:"playlists,omitempty"                json:"playlists,omitempty"`
	Playlist               *PlaylistWithSongs      `xml:"playlist,omitempty"                 json:"playlist,omitempty"`
	Starred                *Starred                `xml:"starred,omitempty"                  json:"starred,omitempty"`
	Starred2               *Starred2               `xml:"starred2,omitempty"                 json:"starred2,omitempty"`
	Genres                 *Genres                 `xml:"genres,omitempty"                   json:"genres,omitempty"`
	User                   *SubsonicUser           `xml:"user,omitempty"                     json:"user,omitempty"`
	OpenSubsonicExtensions []OpenSubsonicExtension `xml:"openSubsonicExtensions,omitempty"   json:"openSubsonicExtensions,omitempty"`
}

// SubsonicError is returned in the response when an error occurs.
type SubsonicError struct {
	Code    int    `xml:"code,attr"    json:"code"`
	Message string `xml:"message,attr" json:"message"`
}

// ── System ────────────────────────────────────────────────────────────────────

// License indicates that this server is always licensed (self-hosted).
type License struct {
	Valid          bool   `xml:"valid,attr"          json:"valid"`
	Email          string `xml:"email,attr"          json:"email"`
	LicenseExpires string `xml:"licenseExpires,attr" json:"licenseExpires"`
}

// OpenSubsonicExtension describes a supported OpenSubsonic extension.
type OpenSubsonicExtension struct {
	Name     string `xml:"name,attr"    json:"name"`
	Versions []int  `xml:"versions,attr" json:"versions"`
}

// ── Music folders ─────────────────────────────────────────────────────────────

// MusicFolders contains the list of configured music folders.
type MusicFolders struct {
	MusicFolder []MusicFolder `xml:"musicFolder" json:"musicFolder"`
}

// MusicFolder is a single configured music folder.
type MusicFolder struct {
	ID   int    `xml:"id,attr"   json:"id"`
	Name string `xml:"name,attr" json:"name"`
}

// ── Indexes (legacy artist list) ──────────────────────────────────────────────

// Indexes is the legacy artist index (grouped by first letter).
type Indexes struct {
	LastModified    int64   `xml:"lastModified,attr"    json:"lastModified"`
	IgnoredArticles string  `xml:"ignoredArticles,attr" json:"ignoredArticles"`
	Index           []Index `xml:"index"                json:"index,omitempty"`
}

// Index is one letter group in the artist index.
type Index struct {
	Name   string   `xml:"name,attr" json:"name"`
	Artist []Artist `xml:"artist"    json:"artist"`
}

// Artist (legacy) — used in Indexes.
type Artist struct {
	ID             string `xml:"id,attr"             json:"id"`
	Name           string `xml:"name,attr"           json:"name"`
	AlbumCount     int    `xml:"albumCount,attr"     json:"albumCount,omitempty"`
	ArtistImageURL string `xml:"artistImageUrl,attr" json:"artistImageUrl,omitempty"`
}

// ── ID3 artist/album/song ─────────────────────────────────────────────────────

// ArtistsID3 is the top-level wrapper for the ID3 artist list (getArtists).
type ArtistsID3 struct {
	LastModified    int64       `xml:"lastModified,attr"    json:"lastModified"`
	IgnoredArticles string      `xml:"ignoredArticles,attr" json:"ignoredArticles"`
	Index           []IndexID3  `xml:"index"                json:"index,omitempty"`
}

// IndexID3 is one letter group in the ID3 artist index.
type IndexID3 struct {
	Name   string      `xml:"name,attr" json:"name"`
	Artist []ArtistID3 `xml:"artist"    json:"artist"`
}

// ArtistID3 is an artist in the ID3 format.
type ArtistID3 struct {
	ID             string `xml:"id,attr"             json:"id"`
	Name           string `xml:"name,attr"           json:"name"`
	AlbumCount     int    `xml:"albumCount,attr"     json:"albumCount,omitempty"`
	Starred        string `xml:"starred,attr"        json:"starred,omitempty"`
	ArtistImageURL string `xml:"artistImageUrl,attr" json:"artistImageUrl,omitempty"`
}

// ArtistWithAlbumsID3 is an artist with its albums (getArtist response).
type ArtistWithAlbumsID3 struct {
	ID             string      `xml:"id,attr"             json:"id"`
	Name           string      `xml:"name,attr"           json:"name"`
	AlbumCount     int         `xml:"albumCount,attr"     json:"albumCount"`
	Starred        string      `xml:"starred,attr"        json:"starred,omitempty"`
	ArtistImageURL string      `xml:"artistImageUrl,attr" json:"artistImageUrl,omitempty"`
	Album          []AlbumID3  `xml:"album"               json:"album,omitempty"`
}

// AlbumID3 is an album in the ID3 format.
type AlbumID3 struct {
	ID         string `xml:"id,attr"         json:"id"`
	Name       string `xml:"name,attr"       json:"name"`
	Artist     string `xml:"artist,attr"     json:"artist,omitempty"`
	ArtistID   string `xml:"artistId,attr"   json:"artistId,omitempty"`
	CoverArt   string `xml:"coverArt,attr"   json:"coverArt,omitempty"`
	SongCount  int    `xml:"songCount,attr"  json:"songCount"`
	Duration   int    `xml:"duration,attr"   json:"duration"`
	Year       int    `xml:"year,attr"       json:"year,omitempty"`
	Genre      string `xml:"genre,attr"      json:"genre,omitempty"`
	Starred    string `xml:"starred,attr"    json:"starred,omitempty"`
	Created    string `xml:"created,attr"    json:"created,omitempty"`
}

// AlbumWithSongsID3 is an album with its songs (getAlbum response).
type AlbumWithSongsID3 struct {
	AlbumID3
	Song []Child `xml:"song" json:"song,omitempty"`
}

// Child represents a song in Subsonic responses (used in many places).
type Child struct {
	ID          string `xml:"id,attr"          json:"id"`
	Parent      string `xml:"parent,attr"      json:"parent,omitempty"`
	IsDir       bool   `xml:"isDir,attr"       json:"isDir"`
	Title       string `xml:"title,attr"       json:"title"`
	Album       string `xml:"album,attr"       json:"album,omitempty"`
	Artist      string `xml:"artist,attr"      json:"artist,omitempty"`
	Track       int    `xml:"track,attr"       json:"track,omitempty"`
	Year        int    `xml:"year,attr"        json:"year,omitempty"`
	Genre       string `xml:"genre,attr"       json:"genre,omitempty"`
	CoverArt    string `xml:"coverArt,attr"    json:"coverArt,omitempty"`
	Size        int64  `xml:"size,attr"        json:"size,omitempty"`
	ContentType string `xml:"contentType,attr" json:"contentType,omitempty"`
	Suffix      string `xml:"suffix,attr"      json:"suffix,omitempty"`
	Duration    int    `xml:"duration,attr"    json:"duration,omitempty"`
	BitRate     int    `xml:"bitRate,attr"     json:"bitRate,omitempty"`
	Path        string `xml:"path,attr"        json:"path,omitempty"`
	IsVideo     bool   `xml:"isVideo,attr"     json:"isVideo,omitempty"`
	DiscNumber  int    `xml:"discNumber,attr"  json:"discNumber,omitempty"`
	Created     string `xml:"created,attr"     json:"created,omitempty"`
	AlbumID     string `xml:"albumId,attr"     json:"albumId,omitempty"`
	ArtistID    string `xml:"artistId,attr"    json:"artistId,omitempty"`
	Type        string `xml:"type,attr"        json:"type,omitempty"`
	Starred     string `xml:"starred,attr"     json:"starred,omitempty"`
	UserRating  int    `xml:"userRating,attr"  json:"userRating,omitempty"`
}

// ── Directory (legacy browsing) ───────────────────────────────────────────────

// Directory is a legacy music directory node.
type Directory struct {
	ID       string  `xml:"id,attr"       json:"id"`
	Parent   string  `xml:"parent,attr"   json:"parent,omitempty"`
	Name     string  `xml:"name,attr"     json:"name"`
	Child    []Child `xml:"child"         json:"child,omitempty"`
}

// ── Album/Song lists ──────────────────────────────────────────────────────────

// AlbumList wraps legacy album list.
type AlbumList struct {
	Album []Child `xml:"album" json:"album,omitempty"`
}

// AlbumList2 wraps ID3 album list.
type AlbumList2 struct {
	Album []AlbumID3 `xml:"album" json:"album,omitempty"`
}

// Songs wraps a list of songs (used in randomSongs, songsByGenre, etc.).
type Songs struct {
	Song []Child `xml:"song" json:"song,omitempty"`
}

// NowPlaying wraps the list of currently-playing tracks.
type NowPlaying struct {
	Entry []NowPlayingEntry `xml:"entry" json:"entry,omitempty"`
}

// NowPlayingEntry extends Child with playback metadata.
type NowPlayingEntry struct {
	Child
	Username        string `xml:"username,attr"        json:"username"`
	MinutesAgo      int    `xml:"minutesAgo,attr"      json:"minutesAgo"`
	PlayerID        int    `xml:"playerId,attr"        json:"playerId"`
}

// ── Search ────────────────────────────────────────────────────────────────────

// SearchResult2 wraps legacy search results.
type SearchResult2 struct {
	Artist []Artist `xml:"artist" json:"artist,omitempty"`
	Album  []Child  `xml:"album"  json:"album,omitempty"`
	Song   []Child  `xml:"song"   json:"song,omitempty"`
}

// SearchResult3 wraps ID3 search results.
type SearchResult3 struct {
	Artist []ArtistID3 `xml:"artist" json:"artist,omitempty"`
	Album  []AlbumID3  `xml:"album"  json:"album,omitempty"`
	Song   []Child     `xml:"song"   json:"song,omitempty"`
}

// ── Playlists ─────────────────────────────────────────────────────────────────

// Playlists wraps the list of playlists.
type Playlists struct {
	Playlist []SubsonicPlaylist `xml:"playlist" json:"playlist,omitempty"`
}

// SubsonicPlaylist is a playlist summary (without songs).
type SubsonicPlaylist struct {
	ID        string `xml:"id,attr"        json:"id"`
	Name      string `xml:"name,attr"      json:"name"`
	Comment   string `xml:"comment,attr"   json:"comment,omitempty"`
	Owner     string `xml:"owner,attr"     json:"owner,omitempty"`
	Public    bool   `xml:"public,attr"    json:"public"`
	SongCount int    `xml:"songCount,attr" json:"songCount"`
	Duration  int    `xml:"duration,attr"  json:"duration"`
	Created   string `xml:"created,attr"   json:"created,omitempty"`
	CoverArt  string `xml:"coverArt,attr"  json:"coverArt,omitempty"`
}

// PlaylistWithSongs is a playlist including its songs.
type PlaylistWithSongs struct {
	SubsonicPlaylist
	Entry []Child `xml:"entry" json:"entry,omitempty"`
}

// ── Starred ───────────────────────────────────────────────────────────────────

// Starred is the legacy starred items response.
type Starred struct {
	Artist []Artist `xml:"artist" json:"artist,omitempty"`
	Album  []Child  `xml:"album"  json:"album,omitempty"`
	Song   []Child  `xml:"song"   json:"song,omitempty"`
}

// Starred2 is the ID3 starred items response.
type Starred2 struct {
	Artist []ArtistID3 `xml:"artist" json:"artist,omitempty"`
	Album  []AlbumID3  `xml:"album"  json:"album,omitempty"`
	Song   []Child     `xml:"song"   json:"song,omitempty"`
}

// ── Genres ────────────────────────────────────────────────────────────────────

// Genres wraps the genre list.
type Genres struct {
	Genre []Genre `xml:"genre" json:"genre,omitempty"`
}

// Genre is a single genre.
type Genre struct {
	SongCount  int    `xml:"songCount,attr"  json:"songCount"`
	AlbumCount int    `xml:"albumCount,attr" json:"albumCount"`
	Value      string `xml:",chardata"       json:"value"`
}

// ── User ──────────────────────────────────────────────────────────────────────

// SubsonicUser is the user info response.
type SubsonicUser struct {
	Username            string `xml:"username,attr"            json:"username"`
	Email               string `xml:"email,attr"               json:"email,omitempty"`
	ScrobblingEnabled   bool   `xml:"scrobblingEnabled,attr"   json:"scrobblingEnabled"`
	MaxBitRate          int    `xml:"maxBitRate,attr"          json:"maxBitRate,omitempty"`
	AdminRole           bool   `xml:"adminRole,attr"           json:"adminRole"`
	SettingsRole        bool   `xml:"settingsRole,attr"        json:"settingsRole"`
	DownloadRole        bool   `xml:"downloadRole,attr"        json:"downloadRole"`
	UploadRole          bool   `xml:"uploadRole,attr"          json:"uploadRole"`
	PlaylistRole        bool   `xml:"playlistRole,attr"        json:"playlistRole"`
	CoverArtRole        bool   `xml:"coverArtRole,attr"        json:"coverArtRole"`
	CommentRole         bool   `xml:"commentRole,attr"         json:"commentRole"`
	PodcastRole         bool   `xml:"podcastRole,attr"         json:"podcastRole"`
	StreamRole          bool   `xml:"streamRole,attr"          json:"streamRole"`
	JukeboxRole         bool   `xml:"jukeboxRole,attr"         json:"jukeboxRole"`
	ShareRole           bool   `xml:"shareRole,attr"           json:"shareRole"`
	VideoConversionRole bool   `xml:"videoConversionRole,attr" json:"videoConversionRole"`
}
