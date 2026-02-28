// Package stream handles HTTP range request streaming and cover art serving.
package stream

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/alexander-bruun/orb/pkg/kvkeys"
	"github.com/alexander-bruun/orb/pkg/objstore"
	"github.com/alexander-bruun/orb/pkg/store"
	"github.com/go-chi/chi/v5"
	"github.com/redis/go-redis/v9"
)

const (
	trackMetaTTL   = time.Hour
	coverMaxAge    = 86400      // 1 day
	chunkSize      = 256 * 1024 // 256KB default chunk
	hlsSegmentSecs = 10.0       // target HLS segment duration in seconds
)

// trackMeta is cached in KeyVal to avoid a DB round-trip per chunk.
type trackMeta struct {
	FileKey    string `json:"file_key"`
	FileSize   int64  `json:"file_size"`
	Format     string `json:"format"`
	BitDepth   int32  `json:"bit_depth"`
	SampleRate int32  `json:"sample_rate"`
	Channels   int32  `json:"channels"`
	DurationMs int32  `json:"duration_ms"`
}

// Service handles streaming HTTP routes.
type Service struct {
	db  *store.Store
	obj objstore.ObjectStore
	kv  *redis.Client
}

// New returns a new stream Service.
func New(db *store.Store, obj objstore.ObjectStore, kv *redis.Client) *Service {
	return &Service{db: db, obj: obj, kv: kv}
}

// Stream serves an audio file with full HTTP range request support.
func (s *Service) Stream(w http.ResponseWriter, r *http.Request) {
	trackID := chi.URLParam(r, "track_id")

	// Resolve track metadata (KeyVal first, then Postgres).
	meta, err := s.resolveMeta(r, trackID)
	if err != nil {
		http.Error(w, "track not found", http.StatusNotFound)
		return
	}

	fileSize := meta.FileSize
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

	rc, err := s.obj.GetRange(r.Context(), meta.FileKey, offset, length)
	if err != nil {
		http.Error(w, "storage error", http.StatusInternalServerError)
		return
	}
	defer rc.Close()

	// Set response headers.
	contentType := mimeForFormat(meta.Format)
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Accept-Ranges", "bytes")
	w.Header().Set("Cache-Control", "private, max-age=3600")
	w.Header().Set("X-Orb-Bit-Depth", strconv.Itoa(int(meta.BitDepth)))
	w.Header().Set("X-Orb-Sample-Rate", strconv.Itoa(int(meta.SampleRate)))

	if rangeHeader != "" {
		w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", offset, offset+length-1, fileSize))
		w.Header().Set("Content-Length", strconv.FormatInt(length, 10))
		w.WriteHeader(http.StatusPartialContent)
	} else {
		w.Header().Set("Content-Length", strconv.FormatInt(fileSize, 10))
	}

	// Stream in 64KB chunks — never buffer the whole file.
	buf := make([]byte, 64*1024)
	_, _ = io.CopyBuffer(w, rc, buf)
}

// ServeByTrackID serves an audio track by its ID without relying on a chi URL
// parameter — intended for use by other packages (e.g. listenparty).
func (s *Service) ServeByTrackID(w http.ResponseWriter, r *http.Request, trackID string) {
	meta, err := s.resolveMeta(r, trackID)
	if err != nil {
		http.Error(w, "track not found", http.StatusNotFound)
		return
	}

	fileSize := meta.FileSize
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

	rc, err := s.obj.GetRange(r.Context(), meta.FileKey, offset, length)
	if err != nil {
		http.Error(w, "storage error", http.StatusInternalServerError)
		return
	}
	defer rc.Close()

	contentType := mimeForFormat(meta.Format)
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Accept-Ranges", "bytes")
	w.Header().Set("Cache-Control", "private, max-age=3600")
	w.Header().Set("X-Orb-Bit-Depth", strconv.Itoa(int(meta.BitDepth)))
	w.Header().Set("X-Orb-Sample-Rate", strconv.Itoa(int(meta.SampleRate)))

	if rangeHeader != "" {
		w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", offset, offset+length-1, fileSize))
		w.Header().Set("Content-Length", strconv.FormatInt(length, 10))
		w.WriteHeader(http.StatusPartialContent)
	} else {
		w.Header().Set("Content-Length", strconv.FormatInt(fileSize, 10))
	}

	buf := make([]byte, 64*1024)
	_, _ = io.CopyBuffer(w, rc, buf)
}

// Cover serves album cover art.
func (s *Service) Cover(w http.ResponseWriter, r *http.Request) {
	albumID := chi.URLParam(r, "album_id")
	s.serveCover(w, r, fmt.Sprintf("covers/%s.jpg", albumID))
}

// ArtistImage serves an artist's photo from the object store.
func (s *Service) ArtistImage(w http.ResponseWriter, r *http.Request) {
	artistID := chi.URLParam(r, "artist_id")
	s.serveCover(w, r, fmt.Sprintf("artists/%s.jpg", artistID))
}

// PlaylistCover serves playlist cover art.
func (s *Service) PlaylistCover(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	s.serveCover(w, r, fmt.Sprintf("covers/playlist/%s", id))
}

// PlaylistCoverComposite generates and serves a composite cover for a playlist from its top 4 most played tracks.
func (s *Service) PlaylistCoverComposite(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	// Fetch top 4 most played tracks in the playlist
	tracks, err := s.db.ListPlaylistTopPlayedTracks(r.Context(), id)
	if err != nil || len(tracks) == 0 {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	// Collect up to 4 cover URLs (without .jpg extension)
	var coverURLs []string
	baseURL := "/api/covers/"
	for _, t := range tracks {
		if t.AlbumID != nil {
			coverURLs = append(coverURLs, baseURL+url.PathEscape(*t.AlbumID))
		}
		if len(coverURLs) == 4 {
			break
		}
	}

	if len(coverURLs) == 0 {
		http.Error(w, "no covers", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	_ = json.NewEncoder(w).Encode(coverURLs)
}

// ServeCover serves album cover art by album ID — intended for use by other
// packages (e.g. listenparty) that need to serve covers without chi URL params.
func (s *Service) ServeCover(w http.ResponseWriter, r *http.Request, albumID string) {
	s.serveCover(w, r, fmt.Sprintf("covers/%s.jpg", albumID))
}

func (s *Service) serveCover(w http.ResponseWriter, r *http.Request, key string) {
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
	defer rc.Close()
	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Cache-Control", fmt.Sprintf("public, max-age=%d", coverMaxAge))
	w.Header().Set("Content-Length", strconv.FormatInt(size, 10))
	_, _ = io.Copy(w, rc)
}

func (s *Service) resolveMeta(r *http.Request, trackID string) (*trackMeta, error) {
	// Try KeyVal cache.
	raw, err := s.kv.Get(r.Context(), kvkeys.TrackMeta(trackID)).Result()
	if err == nil {
		var m trackMeta
		if json.Unmarshal([]byte(raw), &m) == nil {
			return &m, nil
		}
	}

	// Fall back to Postgres.
	track, err := s.db.GetTrackByID(r.Context(), trackID)
	if err != nil {
		return nil, err
	}
	m := &trackMeta{
		FileKey:    track.FileKey,
		FileSize:   track.FileSize,
		Format:     track.Format,
		SampleRate: int32(track.SampleRate),
		Channels:   int32(track.Channels),
		DurationMs: int32(track.DurationMs),
		BitDepth:   0,
	}
	if track.BitDepth != nil {
		m.BitDepth = int32(*track.BitDepth)
	}

	// Cache in KeyVal.
	if b, err := json.Marshal(m); err == nil {
		s.kv.Set(r.Context(), kvkeys.TrackMeta(trackID), b, trackMetaTTL)
	}
	return m, nil
}

// Manifest serves an HLS VOD playlist (.m3u8) for the given track.
// Segments reference the stream endpoint via EXT-X-BYTERANGE so no separate
// segment storage is needed. The URI is relative so it works behind any
// reverse proxy regardless of path prefix.
func (s *Service) Manifest(w http.ResponseWriter, r *http.Request) {
	trackID := chi.URLParam(r, "track_id")
	meta, err := s.resolveMeta(r, trackID)
	if err != nil {
		http.Error(w, "track not found", http.StatusNotFound)
		return
	}
	if meta.DurationMs <= 0 || meta.FileSize <= 0 {
		http.Error(w, "track metadata incomplete", http.StatusInternalServerError)
		return
	}

	token := tokenFromRequest(r)
	// Relative URI: resolves to /…/stream/{trackID}?token=… from the manifest
	// at /…/stream/{trackID}/index.m3u8
	segURI := fmt.Sprintf("../%s?token=%s", url.PathEscape(trackID), url.QueryEscape(token))

	durationSec := float64(meta.DurationMs) / 1000.0
	bytesPerSec := float64(meta.FileSize) / durationSec
	segCount := int(math.Ceil(durationSec / hlsSegmentSecs))
	if segCount < 1 {
		segCount = 1
	}

	var sb strings.Builder
	sb.WriteString("#EXTM3U\n")
	sb.WriteString("#EXT-X-VERSION:4\n")
	fmt.Fprintf(&sb, "#EXT-X-TARGETDURATION:%d\n", int(math.Ceil(hlsSegmentSecs)))
	sb.WriteString("#EXT-X-PLAYLIST-TYPE:VOD\n")
	sb.WriteString("#EXT-X-INDEPENDENT-SEGMENTS\n")

	var offset int64
	for i := 0; i < segCount; i++ {
		var segDur float64
		var segBytes int64
		if i == segCount-1 {
			segDur = durationSec - float64(i)*hlsSegmentSecs
			segBytes = meta.FileSize - offset
		} else {
			segDur = hlsSegmentSecs
			segBytes = int64(math.Round(bytesPerSec * hlsSegmentSecs))
		}
		if segDur <= 0 || segBytes <= 0 {
			break
		}
		fmt.Fprintf(&sb, "#EXTINF:%.6f,\n", segDur)
		fmt.Fprintf(&sb, "#EXT-X-BYTERANGE:%d@%d\n", segBytes, offset)
		sb.WriteString(segURI)
		sb.WriteByte('\n')
		offset += segBytes
	}
	sb.WriteString("#EXT-X-ENDLIST\n")

	w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
	w.Header().Set("Cache-Control", "private, max-age=3600")
	_, _ = io.WriteString(w, sb.String())
}

// tokenFromRequest extracts the raw JWT from the request (Bearer header or
// ?token= query param). The middleware already validated it, so this is just
// for embedding in playlist segment URIs.
func tokenFromRequest(r *http.Request) string {
	if hdr := r.Header.Get("Authorization"); strings.HasPrefix(hdr, "Bearer ") {
		return strings.TrimPrefix(hdr, "Bearer ")
	}
	return r.URL.Query().Get("token")
}

// parseRange parses an HTTP Range header. Returns (start, end, error).
// end is inclusive.
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
		// Suffix range: bytes=-N
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
