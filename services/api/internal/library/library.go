// Package library handles browsing tracks, albums, artists, search, and recently played.
package library

import (
	"encoding/json"
	"log"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/alexander-bruun/orb/pkg/store"
	"github.com/alexander-bruun/orb/services/api/internal/auth"
	"github.com/alexander-bruun/orb/services/api/internal/lyricfetch"
	"github.com/go-chi/chi/v5"
)

// Service handles library HTTP routes.
type Service struct {
	db *store.Store
}

// New returns a new library Service.
func New(db *store.Store) *Service {
	return &Service{db: db}
}

// Routes registers library endpoints.
func (s *Service) Routes(r chi.Router) {
	r.Get("/tracks", s.listTracks)
	r.Get("/albums", s.listAlbums)
	r.Get("/artists", s.listArtists)
	r.Get("/albums/{id}", s.albumDetail)
	r.Get("/artists/{id}", s.artistDetail)
	r.Get("/tracks/{id}", s.trackDetail)
	r.Post("/tracks/{id}", s.addTrack)
	r.Delete("/tracks/{id}", s.removeTrack)
	r.Get("/search", s.search)
	r.Get("/recently-played", s.recentlyPlayed)
	r.Get("/recently-played/albums", s.recentlyPlayedAlbums)
	r.Get("/most-played", s.mostPlayed)
	r.Get("/recently-added/albums", s.recentlyAddedAlbums)
	r.Post("/history", s.recordPlay)
	r.Get("/favorites", s.listFavorites)
	r.Get("/favorites/ids", s.listFavoriteIDs)
	r.Post("/favorites/{track_id}", s.addFavorite)
	r.Delete("/favorites/{track_id}", s.removeFavorite)
	r.Get("/tracks/{id}/lyrics", s.getTrackLyrics)
	r.Put("/tracks/{id}/lyrics", s.setTrackLyrics)
}

func (s *Service) listTracks(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromCtx(r.Context())
	limit, offset := pagination(r)
	tracks, err := s.db.ListTracksByUser(r.Context(), store.ListTracksByUserParams{
		UserID: userID,
		Limit:  int32(limit),
		Offset: int32(offset),
	})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, tracks)
}

func (s *Service) listAlbums(w http.ResponseWriter, r *http.Request) {
	limit, offset := pagination(r)
	sortBy := r.URL.Query().Get("sort_by")
	switch sortBy {
	case "artist", "year":
		// valid
	default:
		sortBy = "title"
	}
	albums, err := s.db.ListAlbums(r.Context(), store.ListAlbumsParams{
		Limit:  int32(limit),
		Offset: int32(offset),
		SortBy: sortBy,
	})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	total, err := s.db.CountAlbums(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": albums, "total": total})
}

func (s *Service) listArtists(w http.ResponseWriter, r *http.Request) {
	limit, offset := pagination(r)
	artists, err := s.db.ListArtists(r.Context(), store.ListArtistsParams{
		Limit:  int32(limit),
		Offset: int32(offset),
	})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, artists)
}

func (s *Service) albumDetail(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	album, err := s.db.GetAlbumByID(r.Context(), id)
	if err != nil {
		writeErr(w, http.StatusNotFound, "album not found")
		return
	}
	tracks, err := s.db.ListTracksByAlbum(r.Context(), id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	resp := map[string]any{"album": album, "tracks": tracks}
	if album.ArtistID != nil {
		if artist, err := s.db.GetArtistByID(r.Context(), *album.ArtistID); err == nil {
			resp["artist"] = artist
		}
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Service) artistDetail(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	artist, err := s.db.GetArtistByID(r.Context(), id)
	if err != nil {
		writeErr(w, http.StatusNotFound, "artist not found")
		return
	}
	albums, err := s.db.ListAlbumsByArtist(r.Context(), id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"artist": artist, "albums": albums})
}

func (s *Service) trackDetail(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	track, err := s.db.GetTrackByID(r.Context(), id)
	if err != nil {
		writeErr(w, http.StatusNotFound, "track not found")
		return
	}
	writeJSON(w, http.StatusOK, track)
}

func (s *Service) addTrack(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromCtx(r.Context())
	trackID := chi.URLParam(r, "id")
	if err := s.db.AddTrackToLibrary(r.Context(), store.AddTrackToLibraryParams{
		UserID:  userID,
		TrackID: trackID,
	}); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Service) removeTrack(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromCtx(r.Context())
	trackID := chi.URLParam(r, "id")
	if err := s.db.RemoveTrackFromLibrary(r.Context(), userID, trackID); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Service) search(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		writeErr(w, http.StatusBadRequest, "q is required")
		return
	}
	tracks, _ := s.db.SearchTracks(r.Context(), store.SearchTracksParams{
		ToTsquery: q,
		Limit:     20,
	})
	albums, _ := s.db.SearchAlbums(r.Context(), store.SearchAlbumsParams{
		ToTsquery: q,
		Limit:     20,
	})
	artists, _ := s.db.SearchArtists(r.Context(), store.SearchArtistsParams{
		ToTsquery: q,
		Limit:     20,
	})
	writeJSON(w, http.StatusOK, map[string]any{
		"tracks":  tracks,
		"albums":  albums,
		"artists": artists,
	})
}

func (s *Service) recentlyPlayed(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromCtx(r.Context())
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	rows, err := s.db.ListRecentlyPlayed(r.Context(), store.ListRecentlyPlayedParams{
		UserID: userID,
		Limit:  limit,
		From:   parseDateParam(r.URL.Query().Get("from")),
		To:     parseDateParam(r.URL.Query().Get("to")),
	})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, rows)
}

func (s *Service) mostPlayed(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromCtx(r.Context())
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	rows, err := s.db.ListMostPlayed(r.Context(), store.ListMostPlayedParams{
		UserID: userID,
		Limit:  limit,
		From:   parseDateParam(r.URL.Query().Get("from")),
		To:     parseDateParam(r.URL.Query().Get("to")),
	})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, rows)
}

func (s *Service) recentlyPlayedAlbums(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromCtx(r.Context())
	albums, err := s.db.ListRecentlyPlayedAlbums(r.Context(), store.ListRecentlyPlayedParams{
		UserID: userID,
		Limit:  20,
	})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, albums)
}

func (s *Service) recentlyAddedAlbums(w http.ResponseWriter, r *http.Request) {
	limit := 20
	albums, err := s.db.ListRecentAlbums(r.Context(), store.ListRecentAlbumsParams{Limit: limit})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, albums)
}

func (s *Service) recordPlay(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromCtx(r.Context())
	var body struct {
		TrackID          string `json:"track_id"`
		DurationPlayedMs int    `json:"duration_played_ms"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.TrackID == "" {
		writeErr(w, http.StatusBadRequest, "track_id required")
		return
	}
	if err := s.db.RecordPlay(r.Context(), store.RecordPlayParams{
		UserID:           userID,
		TrackID:          body.TrackID,
		DurationPlayedMs: body.DurationPlayedMs,
	}); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Service) listFavorites(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromCtx(r.Context())
	tracks, err := s.db.ListFavorites(r.Context(), userID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, tracks)
}

func (s *Service) listFavoriteIDs(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromCtx(r.Context())
	ids, err := s.db.ListFavoriteIDs(r.Context(), userID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if ids == nil {
		ids = []string{}
	}
	writeJSON(w, http.StatusOK, ids)
}

func (s *Service) addFavorite(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromCtx(r.Context())
	trackID := chi.URLParam(r, "track_id")
	if err := s.db.AddFavorite(r.Context(), store.FavoriteParams{
		UserID:  userID,
		TrackID: trackID,
	}); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Service) removeFavorite(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromCtx(r.Context())
	trackID := chi.URLParam(r, "track_id")
	if err := s.db.RemoveFavorite(r.Context(), store.FavoriteParams{
		UserID:  userID,
		TrackID: trackID,
	}); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- lyrics ---

// LyricLine represents a single timed lyric line.
type LyricLine struct {
	TimeMs int    `json:"time_ms"`
	Text   string `json:"text"`
}

var lrcLineRe = regexp.MustCompile(`\[(\d{2}):(\d{2})\.(\d{2,3})\](.*)`)

// parseLRC parses LRC-format text into a sorted slice of LyricLines.
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

func (s *Service) getTrackLyrics(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	// 1. Check if we already have lyrics cached in the DB.
	raw, err := s.db.GetTrackLyrics(r.Context(), id)
	if err != nil {
		writeErr(w, http.StatusNotFound, "track not found")
		return
	}
	if raw != "" {
		lines := parseLRC(raw)
		if lines == nil {
			lines = []LyricLine{}
		}
		writeJSON(w, http.StatusOK, lines)
		return
	}

	// 2. No cached lyrics — auto-fetch from external providers.
	track, err := s.db.GetTrackByID(r.Context(), id)
	if err != nil {
		writeJSON(w, http.StatusOK, []LyricLine{})
		return
	}
	artistName := ""
	if track.ArtistID != nil {
		if a, err := s.db.GetArtistByID(r.Context(), *track.ArtistID); err == nil {
			artistName = a.Name
		}
	}
	albumTitle := ""
	if track.AlbumID != nil {
		if al, err := s.db.GetAlbumByID(r.Context(), *track.AlbumID); err == nil {
			albumTitle = al.Title
		}
	}

	res, err := lyricfetch.Search(r.Context(), artistName, albumTitle, track.Title, track.DurationMs)
	if err != nil || res == nil {
		writeJSON(w, http.StatusOK, []LyricLine{})
		return
	}

	// Prefer synced LRC; fall back to plain text wrapped as unsynced.
	lrc := res.LRC
	if lrc == "" {
		lrc = res.Plain
	}

	// 3. Cache in DB for future requests.
	if lrc != "" {
		if err := s.db.SetTrackLyrics(r.Context(), id, lrc); err != nil {
			log.Printf("lyricfetch: failed to cache lyrics for %s: %v", id, err)
		}
	}

	lines := parseLRC(lrc)
	if lines == nil {
		lines = []LyricLine{}
	}
	writeJSON(w, http.StatusOK, lines)
}

func (s *Service) setTrackLyrics(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var body struct {
		Lyrics string `json:"lyrics"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid body")
		return
	}
	if err := s.db.SetTrackLyrics(r.Context(), id, body.Lyrics); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- helpers ---

func parseDateParam(s string) *time.Time {
	if s == "" {
		return nil
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return &t
	}
	if t, err := time.Parse("2006-01-02", s); err == nil {
		return &t
	}
	return nil
}

func pagination(r *http.Request) (limit, offset int) {
	limit, _ = strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ = strconv.Atoi(r.URL.Query().Get("offset"))
	if limit <= 0 || limit > 500 {
		limit = 50
	}
	return
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
