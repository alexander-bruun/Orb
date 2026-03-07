// Package share implements one-time-use share links for tracks and albums.
//
// POST /share                           – JWT required; creates a one-time token.
// GET  /share/{token}                   – Public; redeems the token (single use);
//
//	returns metadata + a stream session.
//
// GET  /share/stream/{session}/{track_id} – Public; streams audio within a
//
//	redeemed share session.
package share

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/alexander-bruun/orb/services/internal/kvkeys"
	"github.com/alexander-bruun/orb/services/internal/lyricfetch"
	"github.com/alexander-bruun/orb/services/internal/objstore"
	"github.com/alexander-bruun/orb/services/internal/store"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

const (
	shareTTL   = 7 * 24 * time.Hour // one-time share token lifetime
	sessionTTL = 24 * time.Hour     // streaming session lifetime after redemption
)

// sharePayload is stored in KV for a share token.
type sharePayload struct {
	Type string `json:"type"` // "track" or "album"
	ID   string `json:"id"`
}

// streamSession is stored in KV once a share is redeemed.
type streamSession struct {
	TrackIDs []string `json:"track_ids"`
}

type createReq struct {
	Type string `json:"type"`
	ID   string `json:"id"`
}

type createResp struct {
	Token string `json:"token"`
}

type redeemResp struct {
	Type          string        `json:"type"`
	Track         *store.Track  `json:"track,omitempty"`
	Album         *store.Album  `json:"album,omitempty"`
	Tracks        []store.Track `json:"tracks,omitempty"`
	StreamSession string        `json:"stream_session"`
	SessionTTL    int           `json:"session_ttl_seconds"`
}

// Service handles share HTTP routes.
type Service struct {
	db    *store.Store
	kv    *redis.Client
	obj   objstore.ObjectStore
	jwtMW func(http.Handler) http.Handler
}

// New returns a new share Service.
func New(db *store.Store, kv *redis.Client, obj objstore.ObjectStore, jwtMW func(http.Handler) http.Handler) *Service {
	return &Service{db: db, kv: kv, obj: obj, jwtMW: jwtMW}
}

// LyricLine is a single timed lyric line.
type LyricLine struct {
	TimeMs int    `json:"time_ms"`
	Text   string `json:"text"`
}

var lrcLineRe = regexp.MustCompile(`\[(\d{2}):(\d{2})\.(\d{2,3})\](.*)`)

func parseLRC(raw string) []LyricLine {
	var lines []LyricLine
	for _, line := range strings.Split(raw, "\n") {
		m := lrcLineRe.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		min, _ := strconv.Atoi(m[1])
		sec, _ := strconv.Atoi(m[2])
		ms, _ := strconv.Atoi(m[3])
		if len(m[3]) == 2 {
			ms *= 10
		}
		text := strings.TrimSpace(m[4])
		if text == "" {
			continue
		}
		lines = append(lines, LyricLine{TimeMs: (min*60+sec)*1000 + ms, Text: text})
	}
	sort.Slice(lines, func(i, j int) bool { return lines[i].TimeMs < lines[j].TimeMs })
	return lines
}

// Routes registers all share endpoints under a single mount point.
func (s *Service) Routes(r chi.Router) {
	// Public endpoints
	r.Get("/stream/{session}/{track_id}", s.stream)
	r.Get("/lyrics/{session}/{track_id}", s.lyrics)
	r.Get("/{token}", s.redeem)
	// JWT-protected creation
	r.Group(func(r chi.Router) {
		r.Use(s.jwtMW)
		r.Post("/", s.create)
	})
}

// POST /share
func (s *Service) create(w http.ResponseWriter, r *http.Request) {
	var req createReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Type != "track" && req.Type != "album" {
		writeErr(w, http.StatusBadRequest, "type must be 'track' or 'album'")
		return
	}
	if req.ID == "" {
		writeErr(w, http.StatusBadRequest, "id is required")
		return
	}

	ctx := r.Context()
	if req.Type == "track" {
		if _, err := s.db.GetTrackByID(ctx, req.ID); err != nil {
			writeErr(w, http.StatusNotFound, "track not found")
			return
		}
	} else {
		if _, err := s.db.GetAlbumByID(ctx, req.ID); err != nil {
			writeErr(w, http.StatusNotFound, "album not found")
			return
		}
	}

	token := uuid.New().String()
	raw, _ := json.Marshal(sharePayload{Type: req.Type, ID: req.ID})
	if err := s.kv.Set(ctx, kvkeys.ShareToken(token), raw, shareTTL).Err(); err != nil {
		writeErr(w, http.StatusInternalServerError, "could not create share token")
		return
	}

	writeJSON(w, http.StatusCreated, createResp{Token: token})
}

// GET /share/{token}
func (s *Service) redeem(w http.ResponseWriter, r *http.Request) {
	token := chi.URLParam(r, "token")
	ctx := r.Context()

	raw, err := s.kv.GetDel(ctx, kvkeys.ShareToken(token)).Bytes()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			writeErr(w, http.StatusGone, "share link has already been used or has expired")
			return
		}
		writeErr(w, http.StatusInternalServerError, "could not redeem token")
		return
	}

	var payload sharePayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		writeErr(w, http.StatusInternalServerError, "malformed share token")
		return
	}

	resp := redeemResp{Type: payload.Type, SessionTTL: int(sessionTTL.Seconds())}
	var allowedIDs []string

	switch payload.Type {
	case "track":
		track, err := s.db.GetTrackByID(ctx, payload.ID)
		if err != nil {
			writeErr(w, http.StatusNotFound, "track not found")
			return
		}
		resp.Track = &track
		allowedIDs = []string{track.ID}

	case "album":
		album, err := s.db.GetAlbumByID(ctx, payload.ID)
		if err != nil {
			writeErr(w, http.StatusNotFound, "album not found")
			return
		}
		tracks, _ := s.db.ListTracksByAlbum(context.Background(), payload.ID)
		resp.Album = &album
		resp.Tracks = tracks
		for _, t := range tracks {
			allowedIDs = append(allowedIDs, t.ID)
		}
	}

	sessRaw, _ := json.Marshal(streamSession{TrackIDs: allowedIDs})
	sessionToken := uuid.New().String()
	_ = s.kv.Set(ctx, kvkeys.ShareStreamSession(sessionToken), sessRaw, sessionTTL).Err()
	resp.StreamSession = sessionToken

	writeJSON(w, http.StatusOK, resp)
}

// GET /share/stream/{session}/{track_id}
func (s *Service) stream(w http.ResponseWriter, r *http.Request) {
	sessionToken := chi.URLParam(r, "session")
	trackID := chi.URLParam(r, "track_id")
	ctx := r.Context()

	raw, err := s.kv.Get(ctx, kvkeys.ShareStreamSession(sessionToken)).Bytes()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			http.Error(w, "streaming session expired or invalid", http.StatusForbidden)
			return
		}
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	var sess streamSession
	if err := json.Unmarshal(raw, &sess); err != nil {
		http.Error(w, "malformed session", http.StatusInternalServerError)
		return
	}

	allowed := false
	for _, id := range sess.TrackIDs {
		if id == trackID {
			allowed = true
			break
		}
	}
	if !allowed {
		http.Error(w, "track not covered by this share session", http.StatusForbidden)
		return
	}

	track, err := s.db.GetTrackByID(ctx, trackID)
	if err != nil {
		http.Error(w, "track not found", http.StatusNotFound)
		return
	}

	fileSize := track.FileSize
	rangeHeader := r.Header.Get("Range")
	var offset, length int64

	if rangeHeader != "" {
		var end int64
		offset, end, err = parseRange(rangeHeader, fileSize)
		if err != nil {
			w.Header().Set("Content-Range", "bytes */*")
			http.Error(w, "invalid range", http.StatusRequestedRangeNotSatisfiable)
			return
		}
		length = end - offset + 1
	} else {
		offset = 0
		length = fileSize
	}

	rc, err := s.obj.GetRange(ctx, track.FileKey, offset, length)
	if err != nil {
		http.Error(w, "storage error", http.StatusInternalServerError)
		return
	}
	defer rc.Close()

	w.Header().Set("Content-Type", mimeForFormat(track.Format))
	w.Header().Set("Accept-Ranges", "bytes")
	w.Header().Set("Cache-Control", "private, max-age=3600")

	if rangeHeader != "" {
		w.Header().Set("Content-Range", "bytes "+i64(offset)+"-"+i64(offset+length-1)+"/"+i64(fileSize))
		w.Header().Set("Content-Length", i64(length))
		w.WriteHeader(http.StatusPartialContent)
	} else {
		w.Header().Set("Content-Length", i64(fileSize))
	}

	buf := make([]byte, 64*1024)
	for {
		n, readErr := rc.Read(buf)
		if n > 0 {
			_, _ = w.Write(buf[:n])
		}
		if readErr != nil {
			break
		}
	}
}

// --- helpers ---

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func mimeForFormat(format string) string {
	switch format {
	case "flac":
		return "audio/flac"
	case "wav":
		return "audio/wav"
	case "mp3":
		return "audio/mpeg"
	default:
		return "application/octet-stream"
	}
}

func i64(n int64) string { return strconv.FormatInt(n, 10) }

func parseRange(header string, total int64) (int64, int64, error) {
	if len(header) < 6 || header[:6] != "bytes=" {
		return 0, 0, errors.New("invalid range format")
	}
	ranges := header[6:]
	dashIdx := -1
	for i, c := range ranges {
		if c == '-' {
			dashIdx = i
			break
		}
	}
	if dashIdx < 0 {
		return 0, 0, errors.New("invalid range format")
	}
	startStr := ranges[:dashIdx]
	endStr := ranges[dashIdx+1:]

	var start, end int64
	if startStr == "" {
		n, err := strconv.ParseInt(endStr, 10, 64)
		if err != nil || n <= 0 {
			return 0, 0, errors.New("invalid suffix range")
		}
		start = total - n
		end = total - 1
	} else {
		var err error
		start, err = strconv.ParseInt(startStr, 10, 64)
		if err != nil {
			return 0, 0, errors.New("invalid range start")
		}
		if endStr == "" {
			end = total - 1
		} else {
			end, err = strconv.ParseInt(endStr, 10, 64)
			if err != nil {
				return 0, 0, errors.New("invalid range end")
			}
		}
	}
	if start < 0 || end >= total || start > end {
		return 0, 0, errors.New("range out of bounds")
	}
	return start, end, nil
}

// GET /share/lyrics/{session}/{track_id}
func (s *Service) lyrics(w http.ResponseWriter, r *http.Request) {
	sessionToken := chi.URLParam(r, "session")
	trackID := chi.URLParam(r, "track_id")
	ctx := r.Context()

	// Validate session
	raw, err := s.kv.Get(ctx, kvkeys.ShareStreamSession(sessionToken)).Bytes()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			writeJSON(w, http.StatusForbidden, []LyricLine{})
			return
		}
		writeJSON(w, http.StatusInternalServerError, []LyricLine{})
		return
	}

	var sess streamSession
	if err := json.Unmarshal(raw, &sess); err != nil {
		writeJSON(w, http.StatusInternalServerError, []LyricLine{})
		return
	}

	allowed := false
	for _, id := range sess.TrackIDs {
		if id == trackID {
			allowed = true
			break
		}
	}
	if !allowed {
		writeJSON(w, http.StatusForbidden, []LyricLine{})
		return
	}

	// Fetch lyrics from cache
	lrcRaw, err := s.db.GetTrackLyrics(ctx, trackID)
	if err != nil {
		writeJSON(w, http.StatusOK, []LyricLine{})
		return
	}

	if lrcRaw == "" {
		// Auto-fetch via external providers
		track, err := s.db.GetTrackByID(ctx, trackID)
		if err != nil {
			writeJSON(w, http.StatusOK, []LyricLine{})
			return
		}
		artistName := ""
		if track.ArtistID != nil {
			if a, aErr := s.db.GetArtistByID(ctx, *track.ArtistID); aErr == nil {
				artistName = a.Name
			}
		}
		albumTitle := ""
		if track.AlbumID != nil {
			if al, alErr := s.db.GetAlbumByID(ctx, *track.AlbumID); alErr == nil {
				albumTitle = al.Title
			}
		}

		res, err := lyricfetch.Search(ctx, artistName, albumTitle, track.Title, track.DurationMs)
		if err != nil || res == nil {
			writeJSON(w, http.StatusOK, []LyricLine{})
			return
		}

		lrcRaw = res.LRC
		if lrcRaw == "" {
			lrcRaw = res.Plain
		}

		if lrcRaw != "" {
			if cErr := s.db.SetTrackLyrics(ctx, trackID, lrcRaw); cErr != nil {
				log.Printf("share: failed to cache lyrics for %s: %v", trackID, cErr)
			}
		}
	}

	lines := parseLRC(lrcRaw)
	if lines == nil {
		lines = []LyricLine{}
	}
	writeJSON(w, http.StatusOK, lines)
}
