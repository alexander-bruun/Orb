// Package castproxy provides Chromecast-compatible media endpoints.
//
// Chromecast devices use the Default Media Receiver to play audio from HTTP
// URLs. This package exposes unauthenticated streaming endpoints that the
// frontend (or any Chromecast sender) can pass to the cast device:
//
//   - /cast/media/{track_id}  — audio stream with range support & CORS
//   - /cast/art/{album_id}    — album cover art
//   - /cast/metadata/{track_id} — JSON metadata for the media receiver
//
// Discovery of Chromecast devices is done client-side (mDNS _googlecast._tcp)
// since Chromecast does not support server-initiated casting. The backend just
// needs to serve media at URLs the Chromecast can reach on the LAN.
package castproxy

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/alexander-bruun/orb/services/internal/objstore"
	"github.com/alexander-bruun/orb/services/internal/store"
	"github.com/go-chi/chi/v5"
)

// Service provides HTTP handlers for Chromecast-compatible media serving.
type Service struct {
	db      *store.Store
	obj     objstore.ObjectStore
	baseURL string
}

// New creates a new Chromecast proxy service.
// baseURL is the externally reachable HTTP base (e.g. "http://192.168.1.10:8080").
func New(db *store.Store, obj objstore.ObjectStore, baseURL string) *Service {
	return &Service{db: db, obj: obj, baseURL: strings.TrimRight(baseURL, "/")}
}

// Routes registers the cast media endpoints on a chi router.
func (s *Service) Routes(r interface {
	Get(string, http.HandlerFunc)
	HandleFunc(string, http.HandlerFunc)
}) {
	r.Get("/cast/media/{track_id}", s.handleMedia)
	r.Get("/cast/art/{album_id}", s.handleArt)
	r.Get("/cast/metadata/{track_id}", s.handleMetadata)
}

// CastMediaURL returns the full URL a Chromecast sender should load for a track.
func (s *Service) CastMediaURL(trackID string) string {
	return s.baseURL + "/cast/media/" + trackID
}

// CastMetadataURL returns the metadata endpoint URL for a track.
func (s *Service) CastMetadataURL(trackID string) string {
	return s.baseURL + "/cast/metadata/" + trackID
}

// handleMedia streams audio to the Chromecast with HTTP range support.
func (s *Service) handleMedia(w http.ResponseWriter, r *http.Request) {
	trackID := chi.URLParam(r, "track_id")
	if trackID == "" {
		http.Error(w, "missing track id", http.StatusBadRequest)
		return
	}

	track, err := s.db.GetTrackByID(r.Context(), trackID)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	s.setCORSHeaders(w)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	fileSize := track.FileSize
	mime := mimeForFormat(track.Format)

	rangeHeader := r.Header.Get("Range")
	var offset, length int64

	if rangeHeader != "" {
		var end int64
		offset, end, err = parseRange(rangeHeader, fileSize)
		if err != nil {
			w.Header().Set("Content-Range", fmt.Sprintf("bytes */%d", fileSize))
			http.Error(w, "invalid range", http.StatusRequestedRangeNotSatisfiable)
			return
		}
		length = end - offset + 1
	} else {
		offset = 0
		length = fileSize
	}

	rc, err := s.obj.GetRange(r.Context(), track.FileKey, offset, length)
	if err != nil {
		http.Error(w, "storage error", http.StatusInternalServerError)
		return
	}
	defer func() { _ = rc.Close() }()

	w.Header().Set("Content-Type", mime)
	w.Header().Set("Accept-Ranges", "bytes")
	w.Header().Set("Content-Length", strconv.FormatInt(length, 10))

	if rangeHeader != "" {
		w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", offset, offset+length-1, fileSize))
		w.WriteHeader(http.StatusPartialContent)
	}

	buf := make([]byte, 64*1024)
	_, _ = io.CopyBuffer(w, rc, buf)
}

// handleArt serves album cover art for the Chromecast media receiver UI.
func (s *Service) handleArt(w http.ResponseWriter, r *http.Request) {
	albumID := chi.URLParam(r, "album_id")
	if albumID == "" {
		http.Error(w, "missing album id", http.StatusBadRequest)
		return
	}

	s.setCORSHeaders(w)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	key := fmt.Sprintf("covers/%s.jpg", albumID)
	size, err := s.obj.Size(r.Context(), key)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	rc, err := s.obj.GetRange(r.Context(), key, 0, size)
	if err != nil {
		http.Error(w, "storage error", http.StatusInternalServerError)
		return
	}
	defer func() { _ = rc.Close() }()

	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Content-Length", strconv.FormatInt(size, 10))
	w.Header().Set("Cache-Control", "public, max-age=86400")
	_, _ = io.Copy(w, rc)
}

// castMetadata is the JSON response for /cast/metadata/{track_id}, providing
// all the information a Chromecast sender needs to build a MediaInfo object.
type castMetadata struct {
	TrackID     string  `json:"track_id"`
	Title       string  `json:"title"`
	Artist      string  `json:"artist,omitempty"`
	Album       string  `json:"album,omitempty"`
	AlbumArtURL string  `json:"album_art_url,omitempty"`
	MediaURL    string  `json:"media_url"`
	ContentType string  `json:"content_type"`
	DurationMs  int     `json:"duration_ms"`
	TrackNumber *int    `json:"track_number,omitempty"`
	BitDepth    *int    `json:"bit_depth,omitempty"`
	SampleRate  int     `json:"sample_rate"`
	Channels    int     `json:"channels"`
	BitrateKbps *int    `json:"bitrate_kbps,omitempty"`
	BPM         *float64 `json:"bpm,omitempty"`
}

// handleMetadata returns JSON metadata for a track, suitable for building
// a Chromecast MediaInfo object on the sender side.
func (s *Service) handleMetadata(w http.ResponseWriter, r *http.Request) {
	trackID := chi.URLParam(r, "track_id")
	if trackID == "" {
		http.Error(w, "missing track id", http.StatusBadRequest)
		return
	}

	s.setCORSHeaders(w)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	track, err := s.db.GetTrackByID(r.Context(), trackID)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	meta := castMetadata{
		TrackID:     track.ID,
		Title:       track.Title,
		MediaURL:    s.CastMediaURL(track.ID),
		ContentType: mimeForFormat(track.Format),
		DurationMs:  track.DurationMs,
		TrackNumber: track.TrackNumber,
		BitDepth:    track.BitDepth,
		SampleRate:  track.SampleRate,
		Channels:    track.Channels,
		BitrateKbps: track.BitrateKbps,
		BPM:         track.BPM,
	}

	// Resolve artist name.
	if track.ArtistID != nil {
		names, err := s.db.GetArtistNamesByIDs(r.Context(), []string{*track.ArtistID})
		if err == nil {
			meta.Artist = names[*track.ArtistID]
		}
	}

	// Resolve album title and art URL.
	if track.AlbumID != nil {
		titles, err := s.db.GetAlbumTitlesByIDs(r.Context(), []string{*track.AlbumID})
		if err == nil {
			meta.Album = titles[*track.AlbumID]
		}
		meta.AlbumArtURL = s.baseURL + "/cast/art/" + *track.AlbumID
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "private, max-age=3600")
	if err := json.NewEncoder(w).Encode(meta); err != nil {
		slog.Warn("cast: metadata encode", "err", err)
	}
}

func (s *Service) setCORSHeaders(w http.ResponseWriter) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Range, Content-Type")
	w.Header().Set("Access-Control-Expose-Headers", "Content-Range, Accept-Ranges, Content-Length")
}

func mimeForFormat(format string) string {
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

func parseRange(rangeHeader string, size int64) (start, end int64, err error) {
	if !strings.HasPrefix(rangeHeader, "bytes=") {
		return 0, 0, fmt.Errorf("unsupported range unit")
	}
	spec := strings.TrimPrefix(rangeHeader, "bytes=")
	parts := strings.SplitN(spec, "-", 2)
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("invalid range")
	}

	if parts[0] == "" {
		n, e := strconv.ParseInt(parts[1], 10, 64)
		if e != nil || n <= 0 {
			return 0, 0, fmt.Errorf("invalid range")
		}
		start = size - n
		end = size - 1
	} else {
		start, err = strconv.ParseInt(parts[0], 10, 64)
		if err != nil {
			return 0, 0, err
		}
		if parts[1] == "" {
			end = size - 1
		} else {
			end, err = strconv.ParseInt(parts[1], 10, 64)
			if err != nil {
				return 0, 0, err
			}
		}
	}

	if start < 0 || end >= size || start > end {
		return 0, 0, fmt.Errorf("range out of bounds")
	}
	return start, end, nil
}
