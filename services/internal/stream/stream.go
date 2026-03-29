// Package stream handles HTTP range request streaming and cover art serving.
package stream

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/http"
	"net/url"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/alexander-bruun/orb/services/internal/auth"
	"github.com/alexander-bruun/orb/services/internal/kvkeys"
	"github.com/alexander-bruun/orb/services/internal/objstore"
	"github.com/alexander-bruun/orb/services/internal/store"
	"github.com/go-chi/chi/v5"
	"github.com/redis/go-redis/v9"
)

const (
	trackMetaTTL   = time.Hour
	coverMaxAge    = 86400      // 1 day
	chunkSize      = 256 * 1024 // 256KB default chunk
	hlsSegmentSecs = 10.0       // target HLS segment duration in seconds
	userPrefsTTL   = 10 * time.Minute
)

// cachedUserPrefs mirrors store.UserStreamingPrefs but lives only in KV.
type cachedUserPrefs struct {
	Any    networkPrefs `json:"any"`
	Wifi   networkPrefs `json:"wifi"`
	Mobile networkPrefs `json:"mobile"`
}

// networkPrefs holds quality limits for a single network tier.
// A nil field means "inherit from the default (any) tier or no limit".
type networkPrefs struct {
	MaxBitrateKbps  *int    `json:"max_bitrate_kbps,omitempty"`
	MaxSampleRate   *int    `json:"max_sample_rate,omitempty"`
	MaxBitDepth     *int    `json:"max_bit_depth,omitempty"`
	TranscodeFormat *string `json:"transcode_format,omitempty"`
}

// effectivePrefs returns the resolved quality limits for a given network type by
// overlaying network-specific settings on top of the "any" defaults.
// netType should be "wifi", "mobile", or "" (treated as any).
func effectivePrefs(p *cachedUserPrefs, netType string) networkPrefs {
	result := p.Any
	var override networkPrefs
	switch netType {
	case "wifi":
		override = p.Wifi
	case "mobile":
		override = p.Mobile
	}
	if override.MaxBitrateKbps != nil {
		result.MaxBitrateKbps = override.MaxBitrateKbps
	}
	if override.MaxSampleRate != nil {
		result.MaxSampleRate = override.MaxSampleRate
	}
	if override.MaxBitDepth != nil {
		result.MaxBitDepth = override.MaxBitDepth
	}
	if override.TranscodeFormat != nil {
		result.TranscodeFormat = override.TranscodeFormat
	}
	return result
}

// throttledReader wraps an io.Reader and limits throughput to the configured
// byte rate. It uses a total-bytes-sent / desired-rate timing approach so
// short reads don't accumulate error over time.
type throttledReader struct {
	r           io.Reader
	bytesPerSec float64
	start       time.Time
	sent        int64
}

func newThrottledReader(r io.Reader, maxKbps int) *throttledReader {
	return &throttledReader{
		r:           r,
		bytesPerSec: float64(maxKbps) * 1000.0 / 8.0,
		start:       time.Now(),
	}
}

func (t *throttledReader) Read(p []byte) (int, error) {
	n, err := t.r.Read(p)
	if n > 0 {
		t.sent += int64(n)
		// Time it should have taken to send this many bytes at the target rate.
		expected := time.Duration(float64(t.sent) / t.bytesPerSec * float64(time.Second))
		if delay := expected - time.Since(t.start); delay > 0 {
			time.Sleep(delay)
		}
	}
	return n, err
}

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

	// Resolve streaming quality prefs for the authenticated user.
	userID := auth.UserIDFromCtx(r.Context())
	cachedPrefs, _ := s.resolveUserPrefs(r, userID)
	netType := r.URL.Query().Get("net") // "wifi", "mobile", or ""
	prefs := effectivePrefs(cachedPrefs, netType)

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

	// When a transcode format is configured, pipe through ffmpeg instead of
	// serving the raw file. Range requests are not supported for transcoded
	// streams (output size is unknown), so we serve from the beginning.
	if prefs.TranscodeFormat != nil {
		if netType != "" {
			w.Header().Set("X-Orb-Network-Tier", netType)
		}
		s.transcodeAndStream(w, r, meta, prefs)
		return
	}

	rc, err := s.obj.GetRange(r.Context(), meta.FileKey, offset, length)
	if err != nil {
		http.Error(w, "storage error", http.StatusInternalServerError)
		return
	}
	defer closeReadCloser(rc)

	// Set response headers.
	contentType := mimeForFormat(meta.Format)
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Accept-Ranges", "bytes")
	w.Header().Set("Cache-Control", "private, max-age=3600")
	w.Header().Set("X-Orb-Bit-Depth", strconv.Itoa(int(meta.BitDepth)))
	w.Header().Set("X-Orb-Sample-Rate", strconv.Itoa(int(meta.SampleRate)))

	// Apply quality advisory headers so the client knows which prefs are active.
	if prefs.MaxBitrateKbps != nil {
		w.Header().Set("X-Orb-Max-Bitrate", strconv.Itoa(*prefs.MaxBitrateKbps))
	}
	if prefs.MaxSampleRate != nil {
		w.Header().Set("X-Orb-Max-Sample-Rate", strconv.Itoa(*prefs.MaxSampleRate))
		if int(meta.SampleRate) > *prefs.MaxSampleRate {
			w.Header().Set("X-Orb-Quality-Advisory", "sample-rate-exceeds-limit")
		}
	}
	if prefs.MaxBitDepth != nil {
		w.Header().Set("X-Orb-Max-Bit-Depth", strconv.Itoa(*prefs.MaxBitDepth))
		if int(meta.BitDepth) > *prefs.MaxBitDepth {
			existing := w.Header().Get("X-Orb-Quality-Advisory")
			if existing != "" {
				w.Header().Set("X-Orb-Quality-Advisory", existing+",bit-depth-exceeds-limit")
			} else {
				w.Header().Set("X-Orb-Quality-Advisory", "bit-depth-exceeds-limit")
			}
		}
	}
	if netType != "" {
		w.Header().Set("X-Orb-Network-Tier", netType)
	}

	if rangeHeader != "" {
		w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", offset, offset+length-1, fileSize))
		w.Header().Set("Content-Length", strconv.FormatInt(length, 10))
		w.WriteHeader(http.StatusPartialContent)
	} else {
		w.Header().Set("Content-Length", strconv.FormatInt(fileSize, 10))
	}

	// Wrap reader with bandwidth throttle if a limit is configured.
	var src io.Reader = rc
	if prefs.MaxBitrateKbps != nil && *prefs.MaxBitrateKbps > 0 {
		src = newThrottledReader(rc, *prefs.MaxBitrateKbps)
	}

	// Stream in 64KB chunks — never buffer the whole file.
	buf := make([]byte, 64*1024)
	_, _ = io.CopyBuffer(w, src, buf)
}

// transcodeAndStream pipes the source file through ffmpeg and writes the
// transcoded output directly to the HTTP response. Content-Length is not set
// because the output size is unknown; Accept-Ranges is omitted so clients
// don't attempt byte-range seeks on the transcoded stream.
func (s *Service) transcodeAndStream(w http.ResponseWriter, r *http.Request, meta *trackMeta, prefs networkPrefs) {
	targetFmt := *prefs.TranscodeFormat

	// Build ffmpeg argument list.
	args := []string{
		"-hide_banner", "-loglevel", "error",
		"-i", "pipe:0",
		"-vn", // strip embedded cover art / video
	}

	// Downsample if the source exceeds the configured ceiling.
	if prefs.MaxSampleRate != nil && int(meta.SampleRate) > *prefs.MaxSampleRate {
		args = append(args, "-ar", strconv.Itoa(*prefs.MaxSampleRate))
	}

	switch targetFmt {
	case "mp3":
		args = append(args, "-f", "mp3", "-c:a", "libmp3lame")
		if prefs.MaxBitrateKbps != nil {
			args = append(args, "-b:a", fmt.Sprintf("%dk", *prefs.MaxBitrateKbps))
		}
	case "aac":
		args = append(args, "-f", "adts", "-c:a", "aac")
		if prefs.MaxBitrateKbps != nil {
			args = append(args, "-b:a", fmt.Sprintf("%dk", *prefs.MaxBitrateKbps))
		}
	case "opus":
		args = append(args, "-f", "ogg", "-c:a", "libopus")
		if prefs.MaxBitrateKbps != nil {
			args = append(args, "-b:a", fmt.Sprintf("%dk", *prefs.MaxBitrateKbps))
		}
	default:
		http.Error(w, "unsupported transcode format", http.StatusBadRequest)
		return
	}
	args = append(args, "pipe:1")

	// Open the full source file from object storage.
	src, err := s.obj.GetRange(r.Context(), meta.FileKey, 0, meta.FileSize)
	if err != nil {
		http.Error(w, "storage error", http.StatusInternalServerError)
		return
	}
	defer func() {
		if cerr := src.Close(); cerr != nil {
			slog.Warn("stream: obj store reader close failed", "err", cerr)
		}
	}()

	cmd := exec.CommandContext(r.Context(), "ffmpeg", args...)
	cmd.Stdin = src

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		http.Error(w, "transcode error", http.StatusInternalServerError)
		return
	}

	if err := cmd.Start(); err != nil {
		// ffmpeg is not installed or failed to start — fall back to raw stream.
		if cerr := stdout.Close(); cerr != nil {
			slog.Warn("stream: stdout close failed", "err", cerr)
		}
		http.Error(w, "transcoding unavailable: ffmpeg not found", http.StatusServiceUnavailable)
		return
	}

	w.Header().Set("Content-Type", mimeForTranscodeFormat(targetFmt))
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("X-Orb-Bit-Depth", strconv.Itoa(int(meta.BitDepth)))
	w.Header().Set("X-Orb-Sample-Rate", strconv.Itoa(int(meta.SampleRate)))
	w.Header().Set("X-Orb-Transcoded", "true")
	w.Header().Set("X-Orb-Transcode-Format", targetFmt)
	if prefs.MaxBitrateKbps != nil {
		w.Header().Set("X-Orb-Max-Bitrate", strconv.Itoa(*prefs.MaxBitrateKbps))
	}

	buf := make([]byte, 64*1024)
	_, _ = io.CopyBuffer(w, stdout, buf)

	_ = cmd.Wait()
}

// ServeByTrackID serves an audio track by its ID without relying on a chi URL
// parameter — intended for use by other packages (e.g. listenparty).
func (s *Service) ServeByTrackID(w http.ResponseWriter, r *http.Request, trackID string) {
	meta, err := s.resolveMeta(r, trackID)
	if err != nil {
		http.Error(w, "track not found", http.StatusNotFound)
		return
	}
	s.serveAudio(w, r, meta.FileKey, meta.FileSize, meta.Format, int(meta.BitDepth), int(meta.SampleRate))
}

// ServeByAudiobookID serves a single-file audiobook by its ID.
func (s *Service) ServeByAudiobookID(w http.ResponseWriter, r *http.Request, id string) {
	book, err := s.db.GetAudiobook(r.Context(), id)
	if err != nil || book.FileKey == nil {
		http.Error(w, "audiobook not found or multi-file", http.StatusNotFound)
		return
	}
	s.serveAudio(w, r, *book.FileKey, book.FileSize, book.Format, 0, 0)
}

// ServeByAudiobookChapterID serves a single chapter file.
func (s *Service) ServeByAudiobookChapterID(w http.ResponseWriter, r *http.Request, chapterID string) {
	chapter, err := s.db.GetAudiobookChapterByID(r.Context(), chapterID)
	if err != nil || chapter.FileKey == nil {
		http.Error(w, "chapter not found", http.StatusNotFound)
		return
	}
	size, err := s.obj.Size(r.Context(), *chapter.FileKey)
	if err != nil {
		http.Error(w, "storage error", http.StatusInternalServerError)
		return
	}
	ext := strings.ToLower(filepath.Ext(*chapter.FileKey))
	if len(ext) > 1 {
		ext = ext[1:]
	}
	s.serveAudio(w, r, *chapter.FileKey, size, ext, 0, 0)
}

func (s *Service) serveAudio(w http.ResponseWriter, r *http.Request, key string, fileSize int64, format string, bitDepth, sampleRate int) {
	rangeHeader := r.Header.Get("Range")

	var offset, length int64
	var err error
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

	rc, err := s.obj.GetRange(r.Context(), key, offset, length)
	if err != nil {
		http.Error(w, "storage error", http.StatusInternalServerError)
		return
	}
	defer closeReadCloser(rc)

	contentType := mimeForFormat(format)
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Accept-Ranges", "bytes")
	w.Header().Set("Cache-Control", "private, max-age=3600")
	if bitDepth > 0 {
		w.Header().Set("X-Orb-Bit-Depth", strconv.Itoa(bitDepth))
	}
	if sampleRate > 0 {
		w.Header().Set("X-Orb-Sample-Rate", strconv.Itoa(sampleRate))
	}

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

	// Collect up to 4 cover paths (relative to the API, e.g. /covers/{albumID})
	var coverURLs []string
	for _, t := range tracks {
		if t.AlbumID != nil {
			coverURLs = append(coverURLs, "/covers/"+url.PathEscape(*t.AlbumID))
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
	defer closeReadCloser(rc)
	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Cache-Control", fmt.Sprintf("public, max-age=%d", coverMaxAge))
	w.Header().Set("Content-Length", strconv.FormatInt(size, 10))
	_, _ = io.Copy(w, rc)
}

// resolveUserPrefs fetches streaming quality preferences for the given user,
// first consulting the KV cache and falling back to Postgres on a miss.
// It never returns an error — callers always get a best-effort result.
func (s *Service) resolveUserPrefs(r *http.Request, userID string) (*cachedUserPrefs, error) {
	if userID == "" {
		return &cachedUserPrefs{}, nil
	}
	// Try KV cache.
	raw, err := s.kv.Get(r.Context(), kvkeys.UserStreamingPrefs(userID)).Result()
	if err == nil {
		var p cachedUserPrefs
		if json.Unmarshal([]byte(raw), &p) == nil {
			return &p, nil
		}
	}
	// Fall back to Postgres.
	dbPrefs, err := s.db.GetUserStreamingPrefs(r.Context(), userID)
	if err != nil {
		// Non-fatal: proceed without limits.
		return &cachedUserPrefs{}, nil
	}
	out := &cachedUserPrefs{
		Any: networkPrefs{
			MaxBitrateKbps:  dbPrefs.MaxBitrateKbps,
			MaxSampleRate:   dbPrefs.MaxSampleRate,
			MaxBitDepth:     dbPrefs.MaxBitDepth,
			TranscodeFormat: dbPrefs.TranscodeFormat,
		},
		Wifi: networkPrefs{
			MaxBitrateKbps:  dbPrefs.WifiMaxBitrateKbps,
			MaxSampleRate:   dbPrefs.WifiMaxSampleRate,
			MaxBitDepth:     dbPrefs.WifiMaxBitDepth,
			TranscodeFormat: dbPrefs.WifiTranscodeFormat,
		},
		Mobile: networkPrefs{
			MaxBitrateKbps:  dbPrefs.MobileMaxBitrateKbps,
			MaxSampleRate:   dbPrefs.MobileMaxSampleRate,
			MaxBitDepth:     dbPrefs.MobileMaxBitDepth,
			TranscodeFormat: dbPrefs.MobileTranscodeFormat,
		},
	}
	if b, err := json.Marshal(out); err == nil {
		s.kv.Set(r.Context(), kvkeys.UserStreamingPrefs(userID), b, userPrefsTTL)
	}
	return out, nil
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

func mimeForTranscodeFormat(format string) string {
	switch format {
	case "mp3":
		return "audio/mpeg"
	case "aac":
		return "audio/aac"
	case "opus":
		return "audio/ogg"
	}
	return "application/octet-stream"
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
	case "m4b", "m4a":
		return "audio/mp4"
	}
	return "application/octet-stream"
}

// AudiobookStream serves an audiobook file with full HTTP range request support.
// Route: GET /stream/audiobook/{id}
// Only valid for single-file audiobooks (M4B/M4A). Multi-file audiobooks must
// stream each chapter via /stream/audiobook/chapter/{chapter_id}.
func (s *Service) AudiobookStream(w http.ResponseWriter, r *http.Request) {
	audiobookID := chi.URLParam(r, "id")

	book, err := s.db.GetAudiobook(r.Context(), audiobookID)
	if err != nil {
		http.Error(w, "audiobook not found", http.StatusNotFound)
		return
	}
	if book.FileKey == nil {
		http.Error(w, "multi-file audiobook: use chapter stream endpoint", http.StatusBadRequest)
		return
	}

	fileSize := book.FileSize
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

	rc, err := s.obj.GetRange(r.Context(), *book.FileKey, offset, length)
	if err != nil {
		http.Error(w, "storage error", http.StatusInternalServerError)
		return
	}
	defer closeReadCloser(rc)

	contentType := mimeForFormat(book.Format)
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Accept-Ranges", "bytes")
	w.Header().Set("Cache-Control", "private, max-age=3600")

	if rangeHeader != "" {
		w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", offset, offset+length-1, fileSize))
		w.Header().Set("Content-Length", strconv.FormatInt(length, 10))
		w.WriteHeader(http.StatusPartialContent)
	} else {
		w.Header().Set("Content-Length", strconv.FormatInt(fileSize, 10))
	}

	_, _ = io.Copy(w, rc)
}

// AudiobookChapterStream serves a single chapter file for multi-file audiobooks.
// Route: GET /stream/audiobook/chapter/{chapter_id}
func (s *Service) AudiobookChapterStream(w http.ResponseWriter, r *http.Request) {
	chapterID := chi.URLParam(r, "chapter_id")

	chapter, err := s.db.GetAudiobookChapterByID(r.Context(), chapterID)
	if err != nil || chapter.FileKey == nil {
		http.Error(w, "chapter not found", http.StatusNotFound)
		return
	}

	fileSize, err := s.obj.Size(r.Context(), *chapter.FileKey)
	if err != nil {
		http.Error(w, "storage error", http.StatusInternalServerError)
		return
	}

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

	rc, err := s.obj.GetRange(r.Context(), *chapter.FileKey, offset, length)
	if err != nil {
		http.Error(w, "storage error", http.StatusInternalServerError)
		return
	}
	defer closeReadCloser(rc)

	// Derive content-type from file extension.
	ext := strings.ToLower(filepath.Ext(*chapter.FileKey))
	if len(ext) > 1 {
		ext = ext[1:]
	}
	contentType := mimeForFormat(ext)

	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Accept-Ranges", "bytes")
	w.Header().Set("Cache-Control", "private, max-age=3600")

	if rangeHeader != "" {
		w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", offset, offset+length-1, fileSize))
		w.Header().Set("Content-Length", strconv.FormatInt(length, 10))
		w.WriteHeader(http.StatusPartialContent)
	} else {
		w.Header().Set("Content-Length", strconv.FormatInt(fileSize, 10))
	}

	_, _ = io.Copy(w, rc)
}

// AudiobookCover serves cover art for an audiobook.
// Route: GET /covers/audiobook/{id}
func (s *Service) AudiobookCover(w http.ResponseWriter, r *http.Request) {
	audiobookID := chi.URLParam(r, "id")
	s.ServeAudiobookCover(w, r, audiobookID)
}

// ServeAudiobookCover serves cover art for an audiobook by ID — intended for
// use by other packages (e.g. listenparty) that need to serve covers without
// chi URL params.
func (s *Service) ServeAudiobookCover(w http.ResponseWriter, r *http.Request, audiobookID string) {
	book, err := s.db.GetAudiobook(r.Context(), audiobookID)
	if err != nil || book.CoverArtKey == nil {
		http.NotFound(w, r)
		return
	}
	s.serveCover(w, r, *book.CoverArtKey)
}

// AvatarImage serves a user avatar from the object store. Public endpoint — no auth.
// Route: GET /covers/avatar/{key} where key is the uuid portion (without path prefix).
func (s *Service) AvatarImage(w http.ResponseWriter, r *http.Request) {
	key := chi.URLParam(r, "key")
	// Accept just the filename part; prepend the avatars/ directory.
	s.serveCover(w, r, "avatars/"+key)
}

func closeReadCloser(rc io.ReadCloser) {
	if err := rc.Close(); err != nil {
		slog.Warn("stream: reader close failed", "err", err)
	}
}
