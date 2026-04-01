package dlna

import (
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/alexander-bruun/orb/services/internal/store"
)

// ── SOAP dispatchers ──────────────────────────────────────────────────────────

// handleContentDirectory handles SOAP ContentDirectory actions.
func (s *Server) handleContentDirectory(w http.ResponseWriter, r *http.Request) {
	action := extractSOAPAction(r.Header.Get("SOAPAction"))
	body, err := io.ReadAll(io.LimitReader(r.Body, 64*1024))
	if err != nil {
		soapFault(w, 501, "Action Failed")
		return
	}
	switch action {
	case "Browse":
		s.handleBrowse(w, r, body)
	case "GetSearchCapabilities":
		writeSoapOK(w, "ContentDirectory", "GetSearchCapabilitiesResponse",
			"<SearchCaps></SearchCaps>")
	case "GetSortCapabilities":
		writeSoapOK(w, "ContentDirectory", "GetSortCapabilitiesResponse",
			"<SortCaps></SortCaps>")
	case "GetSystemUpdateID":
		writeSoapOK(w, "ContentDirectory", "GetSystemUpdateIDResponse",
			"<Id>1</Id>")
	default:
		soapFault(w, 401, "Invalid Action")
	}
}

// handleConnectionManager handles SOAP ConnectionManager actions.
func (s *Server) handleConnectionManager(w http.ResponseWriter, r *http.Request) {
	action := extractSOAPAction(r.Header.Get("SOAPAction"))
	switch action {
	case "GetProtocolInfo":
		protocols := strings.Join([]string{
			"http-get:*:audio/flac:*",
			"http-get:*:audio/mpeg:*",
			"http-get:*:audio/ogg:*",
			"http-get:*:audio/wav:*",
			"http-get:*:audio/aiff:*",
			"http-get:*:audio/aac:*",
			"http-get:*:audio/mp4:*",
			"http-get:*:audio/x-dsf:*",
			"http-get:*:audio/*:*",
		}, ",")
		writeSoapOK(w, "ConnectionManager", "GetProtocolInfoResponse",
			"<Source>"+xmlEscape(protocols)+"</Source><Sink></Sink>")
	case "GetCurrentConnectionIDs":
		writeSoapOK(w, "ConnectionManager", "GetCurrentConnectionIDsResponse",
			"<ConnectionIDs>0</ConnectionIDs>")
	case "GetCurrentConnectionInfo":
		writeSoapOK(w, "ConnectionManager", "GetCurrentConnectionInfoResponse",
			"<RcsID>-1</RcsID><AVTransportID>-1</AVTransportID>"+
				"<ProtocolInfo></ProtocolInfo><PeerConnectionManager></PeerConnectionManager>"+
				"<PeerConnectionID>-1</PeerConnectionID><Direction>Output</Direction><Status>OK</Status>")
	default:
		soapFault(w, 401, "Invalid Action")
	}
}

// ── Browse ────────────────────────────────────────────────────────────────────

func (s *Server) handleBrowse(w http.ResponseWriter, r *http.Request, body []byte) {
	args := parseSoapBody(body)

	objectID := args["ObjectID"]
	if objectID == "" {
		objectID = "0"
	}
	browseFlag := args["BrowseFlag"]
	start, _ := strconv.Atoi(args["StartingIndex"])
	count, _ := strconv.Atoi(args["RequestedCount"])
	if count <= 0 {
		count = 5000
	}

	ctx := r.Context()
	var (
		didl     string
		total    int
		returned int
		err      error
	)

	if browseFlag == "BrowseMetadata" {
		didl, total, returned, err = s.browseMetadata(ctx, objectID)
	} else {
		didl, total, returned, err = s.browseDirectChildren(ctx, objectID, start, count)
	}

	if err != nil {
		soapFault(w, 501, "Action Failed")
		return
	}

	payload := fmt.Sprintf("<Result>%s</Result><NumberReturned>%d</NumberReturned><TotalMatches>%d</TotalMatches><UpdateID>1</UpdateID>",
		xmlEscape(didl), returned, total)
	writeSoapOK(w, "ContentDirectory", "BrowseResponse", payload)
}

// browseDirectChildren returns the children of the given object ID.
func (s *Server) browseDirectChildren(ctx context.Context, id string, start, count int) (string, int, int, error) {
	switch {
	case id == "0":
		return s.browseRoot(ctx, start, count)
	case id == "artists":
		return s.browseArtists(ctx, start, count)
	case id == "albums":
		return s.browseAlbums(ctx, start, count)
	case id == "playlists":
		return s.browsePlaylists(ctx, start, count)
	case strings.HasPrefix(id, "artist:"):
		return s.browseArtistAlbums(ctx, strings.TrimPrefix(id, "artist:"), start, count)
	case strings.HasPrefix(id, "album:"):
		return s.browseAlbumTracks(ctx, strings.TrimPrefix(id, "album:"), "album:"+strings.TrimPrefix(id, "album:"), start, count)
	case strings.HasPrefix(id, "playlist:"):
		return s.browsePlaylistTracks(ctx, strings.TrimPrefix(id, "playlist:"), start, count)
	default:
		return buildDIDL(nil), 0, 0, nil
	}
}

// browseMetadata returns metadata for the given object ID itself.
func (s *Server) browseMetadata(ctx context.Context, id string) (string, int, int, error) {
	var item string
	switch {
	case id == "0":
		item = containerItem("0", "-1", "Orb Music Library", "object.container.storageFolder", 3)
	case id == "artists":
		item = containerItem("artists", "0", "Artists", "object.container", 0)
	case id == "albums":
		item = containerItem("albums", "0", "Albums", "object.container", 0)
	case id == "playlists":
		item = containerItem("playlists", "0", "Playlists", "object.container", 0)
	case strings.HasPrefix(id, "artist:"):
		artistID := strings.TrimPrefix(id, "artist:")
		artist, err := s.db.GetArtistByID(ctx, artistID)
		if err != nil {
			return buildDIDL(nil), 0, 0, nil
		}
		item = containerItem(id, "artists", artist.Name, "object.container.person.musicArtist", 0)
	case strings.HasPrefix(id, "album:"):
		albumID := strings.TrimPrefix(id, "album:")
		album, err := s.db.GetAlbumByID(ctx, albumID)
		if err != nil {
			return buildDIDL(nil), 0, 0, nil
		}
		parentID := "albums"
		if album.ArtistID != nil {
			parentID = "artist:" + *album.ArtistID
		}
		item = albumContainer(id, parentID, album, s.baseURL)
	case strings.HasPrefix(id, "playlist:"):
		playlistID := strings.TrimPrefix(id, "playlist:")
		pl, err := s.db.GetPlaylistByID(ctx, playlistID)
		if err != nil {
			return buildDIDL(nil), 0, 0, nil
		}
		item = containerItem(id, "playlists", pl.Name, "object.container.playlistContainer", pl.TrackCount)
	case strings.HasPrefix(id, "track:"):
		trackID := strings.TrimPrefix(id, "track:")
		track, err := s.db.GetTrackByID(ctx, trackID)
		if err != nil {
			return buildDIDL(nil), 0, 0, nil
		}
		parentID := "albums"
		if track.AlbumID != nil {
			parentID = "album:" + *track.AlbumID
		}
		item = trackItem(parentID, track, s.baseURL)
	default:
		return buildDIDL(nil), 0, 0, nil
	}
	return buildDIDL([]string{item}), 1, 1, nil
}

// ── Container browse helpers ──────────────────────────────────────────────────

func (s *Server) browseRoot(_ context.Context, start, count int) (string, int, int, error) {
	all := []string{
		containerItem("artists", "0", "Artists", "object.container.person.musicArtist", 0),
		containerItem("albums", "0", "Albums", "object.container.album.musicAlbum", 0),
		containerItem("playlists", "0", "Playlists", "object.container.playlistContainer", 0),
	}
	slice, total, returned := paginate(all, start, count)
	return buildDIDL(slice), total, returned, nil
}

func (s *Server) browseArtists(ctx context.Context, start, count int) (string, int, int, error) {
	artists, err := s.db.ListArtists(ctx, store.ListArtistsParams{Limit: 5000, Offset: 0})
	if err != nil {
		return "", 0, 0, err
	}
	items := make([]string, len(artists))
	for i, a := range artists {
		items[i] = containerItem("artist:"+a.ID, "artists", a.Name, "object.container.person.musicArtist", 0)
	}
	slice, total, returned := paginate(items, start, count)
	return buildDIDL(slice), total, returned, nil
}

func (s *Server) browseAlbums(ctx context.Context, start, count int) (string, int, int, error) {
	albums, err := s.db.ListAlbums(ctx, store.ListAlbumsParams{Limit: 5000, Offset: 0, SortBy: "title"})
	if err != nil {
		return "", 0, 0, err
	}
	items := make([]string, len(albums))
	for i, a := range albums {
		parentID := "albums"
		if a.ArtistID != nil {
			parentID = "artist:" + *a.ArtistID
		}
		items[i] = albumContainer("album:"+a.ID, parentID, a, s.baseURL)
	}
	slice, total, returned := paginate(items, start, count)
	return buildDIDL(slice), total, returned, nil
}

func (s *Server) browsePlaylists(ctx context.Context, start, count int) (string, int, int, error) {
	playlists, err := s.db.ListPublicPlaylists(ctx)
	if err != nil {
		return "", 0, 0, err
	}
	items := make([]string, len(playlists))
	for i, pl := range playlists {
		items[i] = containerItem("playlist:"+pl.ID, "playlists", pl.Name, "object.container.playlistContainer", pl.TrackCount)
	}
	slice, total, returned := paginate(items, start, count)
	return buildDIDL(slice), total, returned, nil
}

func (s *Server) browseArtistAlbums(ctx context.Context, artistID string, start, count int) (string, int, int, error) {
	albums, err := s.db.ListAlbumsByArtist(ctx, artistID)
	if err != nil {
		return "", 0, 0, err
	}
	items := make([]string, len(albums))
	for i, a := range albums {
		items[i] = albumContainer("album:"+a.ID, "artist:"+artistID, a, s.baseURL)
	}
	slice, total, returned := paginate(items, start, count)
	return buildDIDL(slice), total, returned, nil
}

func (s *Server) browseAlbumTracks(ctx context.Context, albumID, parentID string, start, count int) (string, int, int, error) {
	tracks, err := s.db.ListTracksByAlbum(ctx, albumID)
	if err != nil {
		return "", 0, 0, err
	}
	items := make([]string, len(tracks))
	for i, t := range tracks {
		items[i] = trackItem(parentID, t, s.baseURL)
	}
	slice, total, returned := paginate(items, start, count)
	return buildDIDL(slice), total, returned, nil
}

func (s *Server) browsePlaylistTracks(ctx context.Context, playlistID string, start, count int) (string, int, int, error) {
	tracks, err := s.db.ListPlaylistTracks(ctx, playlistID)
	if err != nil {
		return "", 0, 0, err
	}
	items := make([]string, len(tracks))
	for i, t := range tracks {
		items[i] = trackItem("playlist:"+playlistID, t, s.baseURL)
	}
	slice, total, returned := paginate(items, start, count)
	return buildDIDL(slice), total, returned, nil
}

// ── DIDL-Lite XML builders ────────────────────────────────────────────────────

const didlHeader = `<DIDL-Lite xmlns="urn:schemas-upnp-org:metadata-1-0/DIDL-Lite/" xmlns:dc="http://purl.org/dc/elements/1.1/" xmlns:upnp="urn:schemas-upnp-org:metadata-1-0/upnp/" xmlns:dlna="urn:schemas-dlna-org:metadata-1-0/">`

func buildDIDL(items []string) string {
	var b strings.Builder
	b.WriteString(didlHeader)
	for _, item := range items {
		b.WriteString(item)
	}
	b.WriteString("</DIDL-Lite>")
	return b.String()
}

func containerItem(id, parentID, title, class string, childCount int) string {
	cc := ""
	if childCount > 0 {
		cc = fmt.Sprintf(` childCount="%d"`, childCount)
	}
	return fmt.Sprintf(`<container id="%s" parentID="%s" restricted="1"%s searchable="1"><dc:title>%s</dc:title><upnp:class>%s</upnp:class></container>`,
		xmlEscape(id), xmlEscape(parentID), cc, xmlEscape(title), class)
}

func albumContainer(id, parentID string, a store.Album, baseURL string) string {
	artTag := ""
	if a.CoverArtKey != nil && *a.CoverArtKey != "" {
		artTag = fmt.Sprintf(`<upnp:albumArtURI>%s</upnp:albumArtURI>`, xmlEscape(baseURL+"/covers/"+a.ID))
	}
	artistTag := ""
	if a.ArtistName != nil {
		artistTag = fmt.Sprintf(`<upnp:artist>%s</upnp:artist>`, xmlEscape(*a.ArtistName))
	}
	year := ""
	if a.ReleaseYear != nil {
		year = fmt.Sprintf(`<dc:date>%d-01-01</dc:date>`, *a.ReleaseYear)
	}
	return fmt.Sprintf(`<container id="%s" parentID="%s" restricted="1" childCount="%d" searchable="1"><dc:title>%s</dc:title><upnp:class>object.container.album.musicAlbum</upnp:class>%s%s%s</container>`,
		xmlEscape(id), xmlEscape(parentID), a.TrackCount, xmlEscape(a.Title), artistTag, year, artTag)
}

func trackItem(parentID string, t store.Track, baseURL string) string {
	var b strings.Builder

	b.WriteString(fmt.Sprintf(`<item id="track:%s" parentID="%s" restricted="1">`, xmlEscape(t.ID), xmlEscape(parentID)))
	b.WriteString(fmt.Sprintf(`<dc:title>%s</dc:title>`, xmlEscape(t.Title)))
	b.WriteString(`<upnp:class>object.item.audioItem.musicTrack</upnp:class>`)

	if t.ArtistName != nil {
		b.WriteString(fmt.Sprintf(`<upnp:artist>%s</upnp:artist>`, xmlEscape(*t.ArtistName)))
		b.WriteString(fmt.Sprintf(`<dc:creator>%s</dc:creator>`, xmlEscape(*t.ArtistName)))
	}
	if t.AlbumName != nil {
		b.WriteString(fmt.Sprintf(`<upnp:album>%s</upnp:album>`, xmlEscape(*t.AlbumName)))
	}
	if t.TrackNumber != nil {
		b.WriteString(fmt.Sprintf(`<upnp:originalTrackNumber>%d</upnp:originalTrackNumber>`, *t.TrackNumber))
	}

	// Album art
	if t.CoverArtKey != "" && t.AlbumID != nil {
		b.WriteString(fmt.Sprintf(`<upnp:albumArtURI dlna:profileID="JPEG_TN">%s</upnp:albumArtURI>`,
			xmlEscape(baseURL+"/covers/"+*t.AlbumID)))
	}

	// Resource element
	mime := mimeForFormat(t.Format)
	dur := formatDuration(t.DurationMs)
	streamURL := baseURL + "/cast/media/" + t.ID
	b.WriteString(fmt.Sprintf(`<res protocolInfo="http-get:*:%s:*" duration="%s" size="%d">%s</res>`,
		mime, dur, t.FileSize, xmlEscape(streamURL)))

	b.WriteString("</item>")
	return b.String()
}

// ── Utilities ─────────────────────────────────────────────────────────────────

// paginate slices all[start:start+count] and returns total, returned counts.
func paginate(all []string, start, count int) ([]string, int, int) {
	total := len(all)
	if start >= total {
		return nil, total, 0
	}
	end := start + count
	if end > total {
		end = total
	}
	slice := all[start:end]
	return slice, total, len(slice)
}

// formatDuration converts milliseconds to the UPnP duration format H:MM:SS.FFF.
func formatDuration(ms int) string {
	h := ms / 3_600_000
	m := (ms % 3_600_000) / 60_000
	s := (ms % 60_000) / 1_000
	f := ms % 1_000
	return fmt.Sprintf("%d:%02d:%02d.%03d", h, m, s, f)
}

// mimeForFormat maps a track format string to a MIME type.
func mimeForFormat(format string) string {
	switch strings.ToLower(format) {
	case "flac":
		return "audio/flac"
	case "mp3":
		return "audio/mpeg"
	case "wav":
		return "audio/wav"
	case "aiff", "aif":
		return "audio/aiff"
	case "ogg", "opus":
		return "audio/ogg"
	case "aac", "m4a", "mp4":
		return "audio/mp4"
	case "dsf", "dff", "dsd":
		return "audio/x-dsf"
	}
	return "application/octet-stream"
}

// ── SOAP helpers ──────────────────────────────────────────────────────────────

// extractSOAPAction extracts the action name from a SOAPAction header value.
// Header looks like: "urn:schemas-upnp-org:service:ContentDirectory:1#Browse"
func extractSOAPAction(header string) string {
	header = strings.Trim(header, `"`)
	if i := strings.LastIndex(header, "#"); i >= 0 {
		return header[i+1:]
	}
	return ""
}

// parseSoapBody parses a raw SOAP envelope and returns a map of argument
// name → text value for the action element's children (depth = Envelope > Body > Action > Arg).
func parseSoapBody(data []byte) map[string]string {
	result := make(map[string]string)
	d := xml.NewDecoder(bytes.NewReader(data))
	depth := 0
	var currentArg string
	for {
		tok, err := d.Token()
		if err != nil {
			break
		}
		switch t := tok.(type) {
		case xml.StartElement:
			depth++
			if depth == 4 { // Envelope(1) > Body(2) > Action(3) > Arg(4)
				currentArg = t.Name.Local
			}
		case xml.EndElement:
			if depth == 4 {
				currentArg = ""
			}
			depth--
		case xml.CharData:
			if depth == 4 && currentArg != "" {
				result[currentArg] = strings.TrimSpace(string(t))
			}
		}
	}
	return result
}

// writeSoapOK writes a successful SOAP response.
func writeSoapOK(w http.ResponseWriter, service, action, innerXML string) {
	w.Header().Set("Content-Type", `text/xml; charset="utf-8"`)
	w.Header().Set("EXT", "")
	body := fmt.Sprintf(
		`<?xml version="1.0" encoding="UTF-8"?>`+
			`<s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/" s:encodingStyle="http://schemas.xmlsoap.org/soap/encoding/">`+
			`<s:Body><u:%s xmlns:u="urn:schemas-upnp-org:service:%s:1">%s</u:%s></s:Body>`+
			`</s:Envelope>`,
		action, service, innerXML, action)
	_, _ = fmt.Fprint(w, body)
}

// soapFault writes a SOAP 1.1 fault response.
func soapFault(w http.ResponseWriter, code int, desc string) {
	w.Header().Set("Content-Type", `text/xml; charset="utf-8"`)
	w.WriteHeader(http.StatusInternalServerError)
	fmt.Fprintf(w,
		`<?xml version="1.0"?>`+
			`<s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/">`+
			`<s:Body><s:Fault><faultcode>s:Client</faultcode><faultstring>UPnPError</faultstring>`+
			`<detail><UPnPError xmlns="urn:schemas-upnp-org:control-1-0">`+
			`<errorCode>%d</errorCode><errorDescription>%s</errorDescription>`+
			`</UPnPError></detail></s:Fault></s:Body></s:Envelope>`,
		code, xmlEscape(desc))
}
