package dlna

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/alexander-bruun/orb/services/internal/store"
)

// Container IDs for the virtual directory tree:
//
//	0          → root
//	music      → "Music" container
//	artists    → all artists
//	albums     → all albums
//	tracks     → all tracks
//	artist:{id} → single artist (children = their albums)
//	album:{id}  → single album (children = tracks)
//	track:{id}  → single track (leaf item)

const didlHeader = `<DIDL-Lite xmlns="urn:schemas-upnp-org:metadata-1-0/DIDL-Lite/" xmlns:dc="http://purl.org/dc/elements/1.1/" xmlns:upnp="urn:schemas-upnp-org:metadata-1-0/upnp/">`
const didlFooter = `</DIDL-Lite>`

// browseMetadata returns DIDL-Lite XML for a single object.
func (s *Server) browseMetadata(ctx context.Context, objectID string) (string, int) {
	switch objectID {
	case "0":
		return s.containerDIDL("0", "-1", "Root", 3), 1
	case "artists":
		return s.containerDIDL("artists", "0", "Artists", 0), 1
	case "albums":
		return s.containerDIDL("albums", "0", "Albums", 0), 1
	case "tracks":
		return s.containerDIDL("tracks", "0", "All Tracks", 0), 1
	}

	if strings.HasPrefix(objectID, "artist:") {
		id := strings.TrimPrefix(objectID, "artist:")
		artist, err := s.db.GetArtistByID(ctx, id)
		if err != nil {
			return didlHeader + didlFooter, 0
		}
		return s.artistContainerDIDL(artist), 1
	}
	if strings.HasPrefix(objectID, "album:") {
		id := strings.TrimPrefix(objectID, "album:")
		album, err := s.db.GetAlbumByID(ctx, id)
		if err != nil {
			return didlHeader + didlFooter, 0
		}
		return s.albumContainerDIDL(album), 1
	}
	if strings.HasPrefix(objectID, "track:") {
		id := strings.TrimPrefix(objectID, "track:")
		track, err := s.db.GetTrackByID(ctx, id)
		if err != nil {
			return didlHeader + didlFooter, 0
		}
		artistName := s.resolveArtistName(ctx, track.ArtistID)
		albumTitle := s.resolveAlbumTitle(ctx, track.AlbumID)
		return s.trackItemDIDL(track, artistName, albumTitle, trackParentID(track.AlbumID)), 1
	}

	return didlHeader + didlFooter, 0
}

// browseChildren returns DIDL-Lite XML for the children of an object.
func (s *Server) browseChildren(ctx context.Context, objectID string, start, count int) (string, int, int) {
	switch objectID {
	case "0":
		// Root children: Artists, Albums, All Tracks
		items := []string{
			s.containerXML("artists", "0", "Artists", 0),
			s.containerXML("albums", "0", "Albums", 0),
			s.containerXML("tracks", "0", "All Tracks", 0),
		}
		return s.paginateDIDL(items, start, count), min(len(items)-start, count), len(items)

	case "artists":
		return s.browseArtists(ctx, start, count)
	case "albums":
		return s.browseAlbums(ctx, start, count)
	case "tracks":
		return s.browseTracks(ctx, start, count)
	}

	if strings.HasPrefix(objectID, "artist:") {
		id := strings.TrimPrefix(objectID, "artist:")
		return s.browseArtistAlbums(ctx, id, start, count)
	}
	if strings.HasPrefix(objectID, "album:") {
		id := strings.TrimPrefix(objectID, "album:")
		return s.browseAlbumTracks(ctx, id, start, count)
	}

	return didlHeader + didlFooter, 0, 0
}

func (s *Server) browseArtists(ctx context.Context, start, count int) (string, int, int) {
	artists, err := s.db.ListArtists(ctx, store.ListArtistsParams{
		Limit:  int32(count),
		Offset: int32(start),
	})
	if err != nil {
		slog.Warn("dlna: list artists", "err", err)
		return didlHeader + didlFooter, 0, 0
	}

	// Get total count for accurate paging.
	total := start + len(artists)
	if len(artists) == count {
		// There might be more; do a rough estimate.
		total = start + count + 1
	}

	var sb strings.Builder
	sb.WriteString(didlHeader)
	for _, a := range artists {
		sb.WriteString(s.artistContainerXML(a))
	}
	sb.WriteString(didlFooter)
	return sb.String(), len(artists), total
}

func (s *Server) browseAlbums(ctx context.Context, start, count int) (string, int, int) {
	albums, err := s.db.ListAlbums(ctx, store.ListAlbumsParams{
		Limit:  int32(count),
		Offset: int32(start),
		SortBy: "title",
	})
	if err != nil {
		slog.Warn("dlna: list albums", "err", err)
		return didlHeader + didlFooter, 0, 0
	}

	total := start + len(albums)
	if len(albums) == count {
		total = start + count + 1
	}

	var sb strings.Builder
	sb.WriteString(didlHeader)
	for _, a := range albums {
		sb.WriteString(s.albumContainerXML(a))
	}
	sb.WriteString(didlFooter)
	return sb.String(), len(albums), total
}

func (s *Server) browseTracks(ctx context.Context, start, count int) (string, int, int) {
	// List all tracks with pagination. We need a new store method for this.
	tracks, err := s.db.ListAllTracks(ctx, int32(count), int32(start))
	if err != nil {
		slog.Warn("dlna: list all tracks", "err", err)
		return didlHeader + didlFooter, 0, 0
	}

	total := start + len(tracks)
	if len(tracks) == count {
		total = start + count + 1
	}

	// Batch resolve artist names and album titles.
	artistIDs := make([]string, 0)
	albumIDs := make([]string, 0)
	for _, t := range tracks {
		if t.ArtistID != nil {
			artistIDs = append(artistIDs, *t.ArtistID)
		}
		if t.AlbumID != nil {
			albumIDs = append(albumIDs, *t.AlbumID)
		}
	}
	artistNames, _ := s.db.GetArtistNamesByIDs(ctx, artistIDs)
	albumTitles, _ := s.db.GetAlbumTitlesByIDs(ctx, albumIDs)

	var sb strings.Builder
	sb.WriteString(didlHeader)
	for _, t := range tracks {
		artistName := ""
		if t.ArtistID != nil {
			artistName = artistNames[*t.ArtistID]
		}
		albumTitle := ""
		if t.AlbumID != nil {
			albumTitle = albumTitles[*t.AlbumID]
		}
		sb.WriteString(s.trackItemXML(t, artistName, albumTitle, "tracks"))
	}
	sb.WriteString(didlFooter)
	return sb.String(), len(tracks), total
}

func (s *Server) browseArtistAlbums(ctx context.Context, artistID string, start, count int) (string, int, int) {
	albums, err := s.db.ListAlbumsByArtist(ctx, artistID)
	if err != nil {
		slog.Warn("dlna: list albums by artist", "err", err)
		return didlHeader + didlFooter, 0, 0
	}
	total := len(albums)
	end := start + count
	if end > total {
		end = total
	}
	if start >= total {
		return didlHeader + didlFooter, 0, total
	}
	page := albums[start:end]

	var sb strings.Builder
	sb.WriteString(didlHeader)
	for _, a := range page {
		sb.WriteString(s.albumContainerXML(a))
	}
	sb.WriteString(didlFooter)
	return sb.String(), len(page), total
}

func (s *Server) browseAlbumTracks(ctx context.Context, albumID string, start, count int) (string, int, int) {
	tracks, err := s.db.ListTracksByAlbum(ctx, albumID)
	if err != nil {
		slog.Warn("dlna: list tracks by album", "err", err)
		return didlHeader + didlFooter, 0, 0
	}

	total := len(tracks)
	end := start + count
	if end > total {
		end = total
	}
	if start >= total {
		return didlHeader + didlFooter, 0, total
	}
	page := tracks[start:end]

	// Resolve artist names.
	artistIDs := make([]string, 0)
	for _, t := range page {
		if t.ArtistID != nil {
			artistIDs = append(artistIDs, *t.ArtistID)
		}
	}
	artistNames, _ := s.db.GetArtistNamesByIDs(ctx, artistIDs)

	album, _ := s.db.GetAlbumByID(ctx, albumID)

	var sb strings.Builder
	sb.WriteString(didlHeader)
	for _, t := range page {
		artistName := ""
		if t.ArtistID != nil {
			artistName = artistNames[*t.ArtistID]
		}
		sb.WriteString(s.trackItemXML(t, artistName, album.Title, "album:"+albumID))
	}
	sb.WriteString(didlFooter)
	return sb.String(), len(page), total
}

// --- DIDL-Lite XML helpers ---

func (s *Server) containerDIDL(id, parentID, title string, childCount int) string {
	return didlHeader + s.containerXML(id, parentID, title, childCount) + didlFooter
}

func (s *Server) containerXML(id, parentID, title string, childCount int) string {
	cc := ""
	if childCount > 0 {
		cc = fmt.Sprintf(` childCount="%d"`, childCount)
	}
	return fmt.Sprintf(
		`<container id="%s" parentID="%s" restricted="1"%s>`+
			`<dc:title>%s</dc:title>`+
			`<upnp:class>object.container</upnp:class>`+
			`</container>`,
		xmlEscape(id), xmlEscape(parentID), cc, xmlEscape(title))
}

func (s *Server) artistContainerDIDL(a store.Artist) string {
	return didlHeader + s.artistContainerXML(a) + didlFooter
}

func (s *Server) artistContainerXML(a store.Artist) string {
	return fmt.Sprintf(
		`<container id="artist:%s" parentID="artists" restricted="1">`+
			`<dc:title>%s</dc:title>`+
			`<upnp:class>object.container.person.musicArtist</upnp:class>`+
			`</container>`,
		xmlEscape(a.ID), xmlEscape(a.Name))
}

func (s *Server) albumContainerDIDL(a store.Album) string {
	return didlHeader + s.albumContainerXML(a) + didlFooter
}

func (s *Server) albumContainerXML(a store.Album) string {
	parentID := "albums"
	if a.ArtistID != nil {
		parentID = "artist:" + *a.ArtistID
	}
	artXML := ""
	if a.CoverArtKey != nil && *a.CoverArtKey != "" {
		artXML = fmt.Sprintf(`<upnp:albumArtURI>%s/dlna/art/%s</upnp:albumArtURI>`, s.baseURL, xmlEscape(a.ID))
	}
	artistXML := ""
	if a.ArtistName != nil {
		artistXML = fmt.Sprintf(`<dc:creator>%s</dc:creator><upnp:artist>%s</upnp:artist>`, xmlEscape(*a.ArtistName), xmlEscape(*a.ArtistName))
	}
	yearXML := ""
	if a.ReleaseYear != nil {
		yearXML = fmt.Sprintf(`<dc:date>%d-01-01</dc:date>`, *a.ReleaseYear)
	}
	return fmt.Sprintf(
		`<container id="album:%s" parentID="%s" restricted="1" childCount="%d">`+
			`<dc:title>%s</dc:title>`+
			`%s%s%s`+
			`<upnp:class>object.container.album.musicAlbum</upnp:class>`+
			`</container>`,
		xmlEscape(a.ID), xmlEscape(parentID), a.TrackCount,
		xmlEscape(a.Title), artistXML, yearXML, artXML)
}

func (s *Server) trackItemDIDL(t store.Track, artistName, albumTitle, parentID string) string {
	return didlHeader + s.trackItemXML(t, artistName, albumTitle, parentID) + didlFooter
}

func (s *Server) trackItemXML(t store.Track, artistName, albumTitle, parentID string) string {
	mime := dlnaMime(t.Format)
	protocolInfo := fmt.Sprintf("http-get:*:%s:*", mime)
	resURL := fmt.Sprintf("%s/dlna/media/%s", s.baseURL, t.ID)

	durationStr := formatDuration(t.DurationMs)
	sizeAttr := fmt.Sprintf(` size="%d"`, t.FileSize)
	bitrateAttr := ""
	if t.BitrateKbps != nil {
		bitrateAttr = fmt.Sprintf(` bitrate="%d"`, *t.BitrateKbps*1000/8)
	}
	sampleAttr := fmt.Sprintf(` sampleFrequency="%d"`, t.SampleRate)
	channelsAttr := fmt.Sprintf(` nrAudioChannels="%d"`, t.Channels)
	bitsAttr := ""
	if t.BitDepth != nil {
		bitsAttr = fmt.Sprintf(` bitsPerSample="%d"`, *t.BitDepth)
	}

	artistXML := ""
	if artistName != "" {
		artistXML = fmt.Sprintf(`<dc:creator>%s</dc:creator><upnp:artist>%s</upnp:artist>`, xmlEscape(artistName), xmlEscape(artistName))
	}
	albumXML := ""
	if albumTitle != "" {
		albumXML = fmt.Sprintf(`<upnp:album>%s</upnp:album>`, xmlEscape(albumTitle))
	}
	trackNumXML := ""
	if t.TrackNumber != nil {
		trackNumXML = fmt.Sprintf(`<upnp:originalTrackNumber>%d</upnp:originalTrackNumber>`, *t.TrackNumber)
	}
	artXML := ""
	if t.AlbumID != nil {
		artXML = fmt.Sprintf(`<upnp:albumArtURI>%s/dlna/art/%s</upnp:albumArtURI>`, s.baseURL, xmlEscape(*t.AlbumID))
	}

	return fmt.Sprintf(
		`<item id="track:%s" parentID="%s" restricted="1">`+
			`<dc:title>%s</dc:title>`+
			`%s%s%s%s`+
			`<upnp:class>object.item.audioItem.musicTrack</upnp:class>`+
			`<res protocolInfo="%s" duration="%s"%s%s%s%s%s>%s</res>`+
			`</item>`,
		xmlEscape(t.ID), xmlEscape(parentID),
		xmlEscape(t.Title),
		artistXML, albumXML, trackNumXML, artXML,
		protocolInfo, durationStr, sizeAttr, bitrateAttr, sampleAttr, channelsAttr, bitsAttr,
		xmlEscape(resURL))
}

func (s *Server) paginateDIDL(items []string, start, count int) string {
	total := len(items)
	end := start + count
	if end > total {
		end = total
	}
	if start >= total {
		return didlHeader + didlFooter
	}
	var sb strings.Builder
	sb.WriteString(didlHeader)
	for _, item := range items[start:end] {
		sb.WriteString(item)
	}
	sb.WriteString(didlFooter)
	return sb.String()
}

func (s *Server) resolveArtistName(ctx context.Context, artistID *string) string {
	if artistID == nil {
		return ""
	}
	names, err := s.db.GetArtistNamesByIDs(ctx, []string{*artistID})
	if err != nil {
		return ""
	}
	return names[*artistID]
}

func (s *Server) resolveAlbumTitle(ctx context.Context, albumID *string) string {
	if albumID == nil {
		return ""
	}
	titles, err := s.db.GetAlbumTitlesByIDs(ctx, []string{*albumID})
	if err != nil {
		return ""
	}
	return titles[*albumID]
}

func trackParentID(albumID *string) string {
	if albumID != nil {
		return "album:" + *albumID
	}
	return "tracks"
}

func dlnaMime(format string) string {
	switch format {
	case "flac":
		return "audio/flac"
	case "mp3":
		return "audio/mpeg"
	case "wav":
		return "audio/wav"
	case "aiff", "aif":
		return "audio/aiff"
	}
	return "application/octet-stream"
}

// formatDuration converts milliseconds to HH:MM:SS.mmm (DLNA duration format).
func formatDuration(ms int) string {
	totalSec := ms / 1000
	h := totalSec / 3600
	m := (totalSec % 3600) / 60
	sec := totalSec % 60
	frac := ms % 1000
	return fmt.Sprintf("%d:%02d:%02d.%03d", h, m, sec, frac)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
