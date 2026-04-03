package subsonic

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math/rand"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/alexander-bruun/orb/services/internal/objstore"
	"github.com/alexander-bruun/orb/services/internal/store"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/bcrypt"
)

// serverName is the OpenSubsonic "type" field.
const serverName = "Orb"

// Service implements the Subsonic REST API.
type Service struct {
	db  *store.Store
	obj objstore.ObjectStore
}

// New creates a new Subsonic service.
func New(db *store.Store, obj objstore.ObjectStore) *Service {
	return &Service{db: db, obj: obj}
}

// Routes registers all /rest/ endpoints.
func (s *Service) Routes(r chi.Router) {
	// Binary-response endpoints registered first as raw handlers.
	for _, path := range []string{"/stream", "/stream.view", "/download", "/download.view"} {
		r.Get(path, s.streamTrackHandler())
		r.Post(path, s.streamTrackHandler())
	}
	for _, path := range []string{"/getCoverArt", "/getCoverArt.view"} {
		r.Get(path, s.getCoverArtHandler())
		r.Post(path, s.getCoverArtHandler())
	}

	// All remaining endpoints use the generic JSON/XML wrapper.
	for _, method := range []string{http.MethodGet, http.MethodPost} {
		r.Method(method, "/ping", s.handler(s.ping))
		r.Method(method, "/ping.view", s.handler(s.ping))
		r.Method(method, "/getLicense", s.handler(s.getLicense))
		r.Method(method, "/getLicense.view", s.handler(s.getLicense))
		r.Method(method, "/getOpenSubsonicExtensions", s.handler(s.getOpenSubsonicExtensions))
		r.Method(method, "/getOpenSubsonicExtensions.view", s.handler(s.getOpenSubsonicExtensions))

		r.Method(method, "/getMusicFolders", s.handler(s.getMusicFolders))
		r.Method(method, "/getMusicFolders.view", s.handler(s.getMusicFolders))
		r.Method(method, "/getIndexes", s.handler(s.getIndexes))
		r.Method(method, "/getIndexes.view", s.handler(s.getIndexes))
		r.Method(method, "/getMusicDirectory", s.handler(s.getMusicDirectory))
		r.Method(method, "/getMusicDirectory.view", s.handler(s.getMusicDirectory))
		r.Method(method, "/getGenres", s.handler(s.getGenres))
		r.Method(method, "/getGenres.view", s.handler(s.getGenres))
		r.Method(method, "/getArtists", s.handler(s.getArtists))
		r.Method(method, "/getArtists.view", s.handler(s.getArtists))
		r.Method(method, "/getArtist", s.handler(s.getArtist))
		r.Method(method, "/getArtist.view", s.handler(s.getArtist))
		r.Method(method, "/getAlbum", s.handler(s.getAlbum))
		r.Method(method, "/getAlbum.view", s.handler(s.getAlbum))
		r.Method(method, "/getSong", s.handler(s.getSong))
		r.Method(method, "/getSong.view", s.handler(s.getSong))

		r.Method(method, "/getAlbumList", s.handler(s.getAlbumList))
		r.Method(method, "/getAlbumList.view", s.handler(s.getAlbumList))
		r.Method(method, "/getAlbumList2", s.handler(s.getAlbumList2))
		r.Method(method, "/getAlbumList2.view", s.handler(s.getAlbumList2))
		r.Method(method, "/getRandomSongs", s.handler(s.getRandomSongs))
		r.Method(method, "/getRandomSongs.view", s.handler(s.getRandomSongs))
		r.Method(method, "/getSongsByGenre", s.handler(s.getSongsByGenre))
		r.Method(method, "/getSongsByGenre.view", s.handler(s.getSongsByGenre))
		r.Method(method, "/getNowPlaying", s.handler(s.getNowPlaying))
		r.Method(method, "/getNowPlaying.view", s.handler(s.getNowPlaying))
		r.Method(method, "/getStarred", s.handler(s.getStarred))
		r.Method(method, "/getStarred.view", s.handler(s.getStarred))
		r.Method(method, "/getStarred2", s.handler(s.getStarred2))
		r.Method(method, "/getStarred2.view", s.handler(s.getStarred2))

		r.Method(method, "/search2", s.handler(s.search2))
		r.Method(method, "/search2.view", s.handler(s.search2))
		r.Method(method, "/search3", s.handler(s.search3))
		r.Method(method, "/search3.view", s.handler(s.search3))

		r.Method(method, "/getPlaylists", s.handler(s.getPlaylists))
		r.Method(method, "/getPlaylists.view", s.handler(s.getPlaylists))
		r.Method(method, "/getPlaylist", s.handler(s.getPlaylist))
		r.Method(method, "/getPlaylist.view", s.handler(s.getPlaylist))
		r.Method(method, "/createPlaylist", s.handler(s.createPlaylist))
		r.Method(method, "/createPlaylist.view", s.handler(s.createPlaylist))
		r.Method(method, "/updatePlaylist", s.handler(s.updatePlaylist))
		r.Method(method, "/updatePlaylist.view", s.handler(s.updatePlaylist))
		r.Method(method, "/deletePlaylist", s.handler(s.deletePlaylist))
		r.Method(method, "/deletePlaylist.view", s.handler(s.deletePlaylist))

		r.Method(method, "/star", s.handler(s.star))
		r.Method(method, "/star.view", s.handler(s.star))
		r.Method(method, "/unstar", s.handler(s.unstar))
		r.Method(method, "/unstar.view", s.handler(s.unstar))
		r.Method(method, "/setRating", s.handler(s.setRating))
		r.Method(method, "/setRating.view", s.handler(s.setRating))
		r.Method(method, "/scrobble", s.handler(s.scrobble))
		r.Method(method, "/scrobble.view", s.handler(s.scrobble))

		r.Method(method, "/getUser", s.handler(s.getUser))
		r.Method(method, "/getUser.view", s.handler(s.getUser))
	}
}

// ── Request context ───────────────────────────────────────────────────────────

type reqCtx struct {
	r      *http.Request
	format string
	user   *store.User
}

func newReqCtx(r *http.Request) *reqCtx {
	f := r.FormValue("f")
	if f != "json" {
		f = "xml"
	}
	return &reqCtx{r: r, format: f}
}

func (rc *reqCtx) param(name string) string {
	return rc.r.FormValue(name)
}

func (rc *reqCtx) paramInt(name string, def int) int {
	v := rc.r.FormValue(name)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return n
}

// ── Handler wrapper ───────────────────────────────────────────────────────────

type handlerFunc func(rc *reqCtx) *Response

func (s *Service) handler(fn handlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "bad form", http.StatusBadRequest)
			return
		}
		rc := newReqCtx(r)
		user, apiErr := s.authenticate(r.Context(), r)
		if apiErr != nil {
			writeResponse(w, rc.format, errResponse(apiErr.code, apiErr.msg))
			return
		}
		rc.user = user
		writeResponse(w, rc.format, fn(rc))
	}
}

// ── Authentication ────────────────────────────────────────────────────────────

type apiError struct {
	code int
	msg  string
}

func (s *Service) authenticate(ctx context.Context, r *http.Request) (*store.User, *apiError) {
	username := r.FormValue("u")
	password := r.FormValue("p")
	token := r.FormValue("t")
	salt := r.FormValue("s")

	if username == "" {
		return nil, &apiError{10, "Required parameter 'u' is missing."}
	}

	user, err := s.db.GetUserByUsername(ctx, username)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, &apiError{40, "Wrong username or password."}
		}
		slog.Error("subsonic: db lookup failed", "err", err)
		return nil, &apiError{0, "Internal error."}
	}
	if !user.IsActive {
		return nil, &apiError{40, "Wrong username or password."}
	}

	if token != "" && salt != "" {
		subPW, err := s.db.GetSubsonicPassword(ctx, user.ID)
		if err != nil {
			return nil, &apiError{0, "Internal error."}
		}
		if subPW == "" {
			return nil, &apiError{41, "Token-based auth requires a Subsonic password. Set it in Orb settings under Account."}
		}
		h := md5.Sum([]byte(subPW + salt))
		if hex.EncodeToString(h[:]) != strings.ToLower(token) {
			return nil, &apiError{40, "Wrong username or password."}
		}
		return &user, nil
	}

	if password != "" {
		plain := password
		if after, ok := strings.CutPrefix(password, "enc:"); ok {
			decoded, err := hex.DecodeString(after)
			if err != nil {
				return nil, &apiError{40, "Wrong username or password."}
			}
			plain = string(decoded)
		}
		if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(plain)); err != nil {
			return nil, &apiError{40, "Wrong username or password."}
		}
		return &user, nil
	}

	return nil, &apiError{10, "Required parameter 'p' or 't'+'s' is missing."}
}

// ── Response helpers ──────────────────────────────────────────────────────────

func okResponse() *Response {
	return &Response{
		XMLNS:         "http://subsonic.org/restapi",
		Status:        "ok",
		Version:       apiVersion,
		Type:          serverName,
		ServerVersion: "1.0.0",
		OpenSubsonic:  true,
	}
}

func errResponse(code int, message string) *Response {
	r := okResponse()
	r.Status = "failed"
	r.Error = &SubsonicError{Code: code, Message: message}
	return r
}

func writeResponse(w http.ResponseWriter, format string, resp *Response) {
	if format == "json" {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		wrapper := map[string]*Response{"subsonic-response": resp}
		if err := json.NewEncoder(w).Encode(wrapper); err != nil {
			slog.Error("subsonic: json encode failed", "err", err)
		}
		return
	}
	w.Header().Set("Content-Type", "text/xml; charset=utf-8")
	_, _ = w.Write([]byte(xml.Header))
	if err := xml.NewEncoder(w).Encode(resp); err != nil {
		slog.Error("subsonic: xml encode failed", "err", err)
	}
}

// ── System ────────────────────────────────────────────────────────────────────

func (s *Service) ping(_ *reqCtx) *Response { return okResponse() }

func (s *Service) getLicense(_ *reqCtx) *Response {
	r := okResponse()
	r.License = &License{Valid: true, Email: "orb@localhost", LicenseExpires: "2099-12-31T00:00:00"}
	return r
}

func (s *Service) getOpenSubsonicExtensions(_ *reqCtx) *Response {
	r := okResponse()
	r.OpenSubsonicExtensions = []OpenSubsonicExtension{{Name: "formPost", Versions: []int{1}}}
	return r
}

// ── Music folders ─────────────────────────────────────────────────────────────

func (s *Service) getMusicFolders(_ *reqCtx) *Response {
	r := okResponse()
	r.MusicFolders = &MusicFolders{MusicFolder: []MusicFolder{{ID: 1, Name: "Music"}}}
	return r
}

// ── Browsing ──────────────────────────────────────────────────────────────────

func (s *Service) getIndexes(rc *reqCtx) *Response {
	artists, err := s.db.ListArtists(rc.r.Context(), store.ListArtistsParams{Limit: 5000})
	if err != nil {
		slog.Error("subsonic: list artists", "err", err)
		return errResponse(0, "Database error.")
	}
	indexMap := map[string][]Artist{}
	for _, a := range artists {
		key := indexKey(a.Name)
		indexMap[key] = append(indexMap[key], Artist{ID: a.ID, Name: a.Name})
	}
	keys := sortedKeys(indexMap)
	var indexes []Index
	for _, k := range keys {
		indexes = append(indexes, Index{Name: k, Artist: indexMap[k]})
	}
	r := okResponse()
	r.Indexes = &Indexes{
		LastModified:    time.Now().UnixMilli(),
		IgnoredArticles: "The An A Die Das Ein",
		Index:           indexes,
	}
	return r
}

func (s *Service) getMusicDirectory(rc *reqCtx) *Response {
	id := rc.param("id")
	ctx := rc.r.Context()

	switch {
	case id == "artists" || id == "1":
		artists, err := s.db.ListArtists(ctx, store.ListArtistsParams{Limit: 5000})
		if err != nil {
			return errResponse(0, "Database error.")
		}
		dir := &Directory{ID: id, Name: "Music"}
		for _, a := range artists {
			dir.Child = append(dir.Child, Child{ID: "ar-" + a.ID, Title: a.Name, IsDir: true})
		}
		r := okResponse()
		r.Directory = dir
		return r

	case strings.HasPrefix(id, "ar-"):
		artistID := strings.TrimPrefix(id, "ar-")
		artist, err := s.db.GetArtistByID(ctx, artistID)
		if err != nil {
			return errResponse(70, "Artist not found.")
		}
		albums, err := s.db.ListAlbumsByArtist(ctx, artistID)
		if err != nil {
			return errResponse(0, "Database error.")
		}
		dir := &Directory{ID: id, Parent: "artists", Name: artist.Name}
		for _, al := range albums {
			ch := Child{ID: "al-" + al.ID, Title: al.Title, IsDir: true}
			if al.CoverArtKey != nil {
				ch.CoverArt = "al-" + al.ID
			}
			dir.Child = append(dir.Child, ch)
		}
		r := okResponse()
		r.Directory = dir
		return r

	case strings.HasPrefix(id, "al-"):
		albumID := strings.TrimPrefix(id, "al-")
		album, err := s.db.GetAlbumByID(ctx, albumID)
		if err != nil {
			return errResponse(70, "Album not found.")
		}
		tracks, err := s.db.ListTracksByAlbum(ctx, albumID)
		if err != nil {
			return errResponse(0, "Database error.")
		}
		parentID := ""
		if album.ArtistID != nil {
			parentID = "ar-" + *album.ArtistID
		}
		dir := &Directory{ID: id, Parent: parentID, Name: album.Title}
		for _, t := range tracks {
			dir.Child = append(dir.Child, trackToChild(t, album))
		}
		r := okResponse()
		r.Directory = dir
		return r
	}
	return errResponse(70, "Directory not found.")
}

func (s *Service) getGenres(rc *reqCtx) *Response {
	genres, err := s.db.ListGenres(rc.r.Context())
	if err != nil {
		return errResponse(0, "Database error.")
	}
	var out []Genre
	for _, g := range genres {
		out = append(out, Genre{Value: g.Name})
	}
	r := okResponse()
	r.Genres = &Genres{Genre: out}
	return r
}

func (s *Service) getArtists(rc *reqCtx) *Response {
	artists, err := s.db.ListArtists(rc.r.Context(), store.ListArtistsParams{Limit: 5000})
	if err != nil {
		return errResponse(0, "Database error.")
	}
	indexMap := map[string][]ArtistID3{}
	for _, a := range artists {
		key := indexKey(a.Name)
		imageURL := ""
		if a.ImageKey != nil {
			imageURL = "/covers/artist/" + a.ID
		}
		indexMap[key] = append(indexMap[key], ArtistID3{
			ID:             a.ID,
			Name:           a.Name,
			ArtistImageURL: imageURL,
		})
	}
	keys := sortedKeys(indexMap)
	var indexes []IndexID3
	for _, k := range keys {
		indexes = append(indexes, IndexID3{Name: k, Artist: indexMap[k]})
	}
	r := okResponse()
	r.Artists = &ArtistsID3{
		LastModified:    time.Now().UnixMilli(),
		IgnoredArticles: "The An A Die Das Ein",
		Index:           indexes,
	}
	return r
}

func (s *Service) getArtist(rc *reqCtx) *Response {
	id := rc.param("id")
	if id == "" {
		return errResponse(10, "Required parameter 'id' is missing.")
	}
	ctx := rc.r.Context()
	artist, err := s.db.GetArtistByID(ctx, id)
	if err != nil {
		return errResponse(70, "Artist not found.")
	}
	albums, err := s.db.ListAlbumsByArtist(ctx, id)
	if err != nil {
		return errResponse(0, "Database error.")
	}
	imageURL := ""
	if artist.ImageKey != nil {
		imageURL = "/covers/artist/" + artist.ID
	}
	resp := &ArtistWithAlbumsID3{
		ID:             artist.ID,
		Name:           artist.Name,
		AlbumCount:     len(albums),
		ArtistImageURL: imageURL,
	}
	for _, al := range albums {
		resp.Album = append(resp.Album, albumToID3(al))
	}
	r := okResponse()
	r.Artist = resp
	return r
}

func (s *Service) getAlbum(rc *reqCtx) *Response {
	id := rc.param("id")
	if id == "" {
		return errResponse(10, "Required parameter 'id' is missing.")
	}
	ctx := rc.r.Context()
	album, err := s.db.GetAlbumByID(ctx, id)
	if err != nil {
		return errResponse(70, "Album not found.")
	}
	tracks, err := s.db.ListTracksByAlbum(ctx, id)
	if err != nil {
		return errResponse(0, "Database error.")
	}
	albumID3 := albumToID3(album)
	albumID3.SongCount = len(tracks)
	totalDuration := 0
	for _, t := range tracks {
		totalDuration += t.DurationMs / 1000
	}
	albumID3.Duration = totalDuration
	resp := &AlbumWithSongsID3{AlbumID3: albumID3}
	for _, t := range tracks {
		resp.Song = append(resp.Song, trackToChild(t, album))
	}
	r := okResponse()
	r.Album = resp
	return r
}

func (s *Service) getSong(rc *reqCtx) *Response {
	id := rc.param("id")
	if id == "" {
		return errResponse(10, "Required parameter 'id' is missing.")
	}
	track, err := s.db.GetTrackByID(rc.r.Context(), id)
	if err != nil {
		return errResponse(70, "Song not found.")
	}
	var album store.Album
	if track.AlbumID != nil {
		album, _ = s.db.GetAlbumByID(rc.r.Context(), *track.AlbumID)
	}
	child := trackToChild(track, album)
	r := okResponse()
	r.Song = &child
	return r
}

// ── Album/Song lists ──────────────────────────────────────────────────────────

func (s *Service) getAlbumList(rc *reqCtx) *Response {
	albums, err := s.fetchAlbumList(rc)
	if err != nil {
		return errResponse(0, "Database error.")
	}
	children := make([]Child, 0, len(albums))
	for _, al := range albums {
		ch := Child{ID: al.ID, Title: al.Title, IsDir: true}
		if al.ArtistName != nil {
			ch.Artist = *al.ArtistName
		}
		if al.CoverArtKey != nil {
			ch.CoverArt = "al-" + al.ID
		}
		if al.ReleaseYear != nil {
			ch.Year = *al.ReleaseYear
		}
		children = append(children, ch)
	}
	r := okResponse()
	r.AlbumList = &AlbumList{Album: children}
	return r
}

func (s *Service) getAlbumList2(rc *reqCtx) *Response {
	albums, err := s.fetchAlbumList(rc)
	if err != nil {
		return errResponse(0, "Database error.")
	}
	id3s := make([]AlbumID3, 0, len(albums))
	for _, al := range albums {
		id3s = append(id3s, albumToID3(al))
	}
	r := okResponse()
	r.AlbumList2 = &AlbumList2{Album: id3s}
	return r
}

func (s *Service) fetchAlbumList(rc *reqCtx) ([]store.Album, error) {
	listType := rc.param("type")
	size := rc.paramInt("size", 10)
	offset := rc.paramInt("offset", 0)
	if size > 500 {
		size = 500
	}
	ctx := rc.r.Context()
	sortBy := "title"
	switch listType {
	case "newest":
		sortBy = "year"
	case "alphabeticalByArtist":
		sortBy = "artist"
	}
	return s.db.ListAlbums(ctx, store.ListAlbumsParams{
		Limit:  int32(size),
		Offset: int32(offset),
		SortBy: sortBy,
	})
}

func (s *Service) getRandomSongs(rc *reqCtx) *Response {
	size := rc.paramInt("size", 10)
	if size > 500 {
		size = 500
	}
	tracks, err := s.db.ListTracksByUser(rc.r.Context(), store.ListTracksByUserParams{
		Limit:  int32(min(size*10, 5000)),
		Offset: 0,
	})
	if err != nil {
		return errResponse(0, "Database error.")
	}
	rand.Shuffle(len(tracks), func(i, j int) { tracks[i], tracks[j] = tracks[j], tracks[i] })
	if len(tracks) > size {
		tracks = tracks[:size]
	}
	songs := make([]Child, 0, len(tracks))
	for _, t := range tracks {
		var album store.Album
		if t.AlbumID != nil {
			album, _ = s.db.GetAlbumByID(rc.r.Context(), *t.AlbumID)
		}
		songs = append(songs, trackToChild(t, album))
	}
	r := okResponse()
	r.RandomSongs = &Songs{Song: songs}
	return r
}

func (s *Service) getSongsByGenre(rc *reqCtx) *Response {
	genre := rc.param("genre")
	if genre == "" {
		return errResponse(10, "Required parameter 'genre' is missing.")
	}
	count := rc.paramInt("count", 10)
	if count > 500 {
		count = 500
	}
	tracks, err := s.db.SearchTracks(rc.r.Context(), store.SearchTracksParams{
		ToTsquery: genre,
		Limit:     count,
		Genre:     genre,
	})
	if err != nil {
		return errResponse(0, "Database error.")
	}
	songs := make([]Child, 0, len(tracks))
	for _, t := range tracks {
		var album store.Album
		if t.AlbumID != nil {
			album, _ = s.db.GetAlbumByID(rc.r.Context(), *t.AlbumID)
		}
		songs = append(songs, trackToChild(t, album))
	}
	r := okResponse()
	r.SongsByGenre = &Songs{Song: songs}
	return r
}

func (s *Service) getNowPlaying(_ *reqCtx) *Response {
	r := okResponse()
	r.NowPlaying = &NowPlaying{}
	return r
}

func (s *Service) getStarred(rc *reqCtx) *Response {
	tracks, err := s.db.ListFavorites(rc.r.Context(), rc.user.ID)
	if err != nil {
		return errResponse(0, "Database error.")
	}
	songs := s.tracksToChildren(rc.r.Context(), tracks, true)
	r := okResponse()
	r.Starred = &Starred{Song: songs}
	return r
}

func (s *Service) getStarred2(rc *reqCtx) *Response {
	tracks, err := s.db.ListFavorites(rc.r.Context(), rc.user.ID)
	if err != nil {
		return errResponse(0, "Database error.")
	}
	songs := s.tracksToChildren(rc.r.Context(), tracks, true)
	r := okResponse()
	r.Starred2 = &Starred2{Song: songs}
	return r
}

// ── Search ────────────────────────────────────────────────────────────────────

func (s *Service) search2(rc *reqCtx) *Response {
	query, result := rc.param("query"), &SearchResult2{}
	ctx := rc.r.Context()

	if query != "" {
		ac := rc.paramInt("artistCount", 20)
		if ac > 0 {
			artists, _ := s.db.SearchArtists(ctx, store.SearchArtistsParams{ToTsquery: query, Limit: ac})
			for _, a := range artists {
				result.Artist = append(result.Artist, Artist{ID: a.ID, Name: a.Name})
			}
		}
		alc := rc.paramInt("albumCount", 20)
		if alc > 0 {
			albums, _ := s.db.SearchAlbums(ctx, store.SearchAlbumsParams{ToTsquery: query, Limit: alc})
			for _, al := range albums {
				ch := Child{ID: al.ID, Title: al.Title, IsDir: true}
				if al.ArtistName != nil {
					ch.Artist = *al.ArtistName
				}
				if al.CoverArtKey != nil {
					ch.CoverArt = "al-" + al.ID
				}
				result.Album = append(result.Album, ch)
			}
		}
		sc := rc.paramInt("songCount", 20)
		if sc > 0 {
			tracks, _ := s.db.SearchTracks(ctx, store.SearchTracksParams{ToTsquery: query, Limit: sc})
			result.Song = s.tracksToChildren(ctx, tracks, false)
		}
	}

	r := okResponse()
	r.SearchResult2 = result
	return r
}

func (s *Service) search3(rc *reqCtx) *Response {
	query, result := rc.param("query"), &SearchResult3{}
	ctx := rc.r.Context()

	if query != "" {
		ac := rc.paramInt("artistCount", 20)
		if ac > 0 {
			artists, _ := s.db.SearchArtists(ctx, store.SearchArtistsParams{ToTsquery: query, Limit: ac})
			for _, a := range artists {
				imageURL := ""
				if a.ImageKey != nil {
					imageURL = "/covers/artist/" + a.ID
				}
				result.Artist = append(result.Artist, ArtistID3{ID: a.ID, Name: a.Name, ArtistImageURL: imageURL})
			}
		}
		alc := rc.paramInt("albumCount", 20)
		if alc > 0 {
			albums, _ := s.db.SearchAlbums(ctx, store.SearchAlbumsParams{ToTsquery: query, Limit: alc})
			for _, al := range albums {
				result.Album = append(result.Album, albumToID3(al))
			}
		}
		sc := rc.paramInt("songCount", 20)
		if sc > 0 {
			tracks, _ := s.db.SearchTracks(ctx, store.SearchTracksParams{ToTsquery: query, Limit: sc})
			result.Song = s.tracksToChildren(ctx, tracks, false)
		}
	}

	r := okResponse()
	r.SearchResult3 = result
	return r
}

// ── Playlists ─────────────────────────────────────────────────────────────────

func (s *Service) getPlaylists(rc *reqCtx) *Response {
	playlists, err := s.db.ListPlaylistsByUser(rc.r.Context(), rc.user.ID)
	if err != nil {
		return errResponse(0, "Database error.")
	}
	var out []SubsonicPlaylist
	for _, pl := range playlists {
		out = append(out, playlistToSubsonic(pl, rc.user.Username))
	}
	r := okResponse()
	r.Playlists = &Playlists{Playlist: out}
	return r
}

func (s *Service) getPlaylist(rc *reqCtx) *Response {
	id := rc.param("id")
	if id == "" {
		return errResponse(10, "Required parameter 'id' is missing.")
	}
	ctx := rc.r.Context()

	pl, err := s.db.GetPlaylistByID(ctx, id)
	if err != nil {
		return errResponse(70, "Playlist not found.")
	}
	if pl.UserID != rc.user.ID && !pl.IsPublic {
		return errResponse(50, "Access denied.")
	}
	tracks, err := s.db.ListPlaylistTracks(ctx, id)
	if err != nil {
		return errResponse(0, "Database error.")
	}
	totalDuration := 0
	for _, t := range tracks {
		totalDuration += t.DurationMs / 1000
	}
	songs := s.tracksToChildren(ctx, tracks, false)
	sub := playlistToSubsonic(pl, rc.user.Username)
	sub.Duration = totalDuration

	r := okResponse()
	r.Playlist = &PlaylistWithSongs{SubsonicPlaylist: sub, Entry: songs}
	return r
}

func (s *Service) createPlaylist(rc *reqCtx) *Response {
	name := rc.param("name")
	if name == "" {
		return errResponse(10, "Required parameter 'name' is missing.")
	}
	ctx := rc.r.Context()

	pl, err := s.db.CreatePlaylist(ctx, store.CreatePlaylistParams{
		ID:     uuid.NewString(),
		UserID: rc.user.ID,
		Name:   name,
	})
	if err != nil {
		return errResponse(0, "Database error.")
	}
	for i, sid := range rc.r.Form["songId"] {
		_ = s.db.AddTrackToPlaylist(ctx, store.AddTrackToPlaylistParams{
			PlaylistID: pl.ID,
			TrackID:    sid,
			Position:   i,
		})
	}
	r := okResponse()
	r.Playlist = &PlaylistWithSongs{SubsonicPlaylist: playlistToSubsonic(pl, rc.user.Username)}
	return r
}

func (s *Service) updatePlaylist(rc *reqCtx) *Response {
	id := rc.param("playlistId")
	if id == "" {
		return errResponse(10, "Required parameter 'playlistId' is missing.")
	}
	ctx := rc.r.Context()

	pl, err := s.db.GetPlaylistByID(ctx, id)
	if err != nil {
		return errResponse(70, "Playlist not found.")
	}
	if pl.UserID != rc.user.ID {
		return errResponse(50, "Access denied.")
	}
	name := rc.param("name")
	if name == "" {
		name = pl.Name
	}
	if err := s.db.UpdatePlaylist(ctx, store.UpdatePlaylistParams{
		ID:          pl.ID,
		Name:        name,
		Description: rc.param("comment"),
		IsPublic:    pl.IsPublic,
	}); err != nil {
		return errResponse(0, "Database error.")
	}
	for _, sid := range rc.r.Form["songIdToAdd"] {
		maxPos, _ := s.db.GetMaxPlaylistPosition(ctx, pl.ID)
		_ = s.db.AddTrackToPlaylist(ctx, store.AddTrackToPlaylistParams{
			PlaylistID: pl.ID,
			TrackID:    sid,
			Position:   maxPos + 1,
		})
	}
	return okResponse()
}

func (s *Service) deletePlaylist(rc *reqCtx) *Response {
	id := rc.param("id")
	if id == "" {
		return errResponse(10, "Required parameter 'id' is missing.")
	}
	ctx := rc.r.Context()

	pl, err := s.db.GetPlaylistByID(ctx, id)
	if err != nil {
		return errResponse(70, "Playlist not found.")
	}
	if pl.UserID != rc.user.ID {
		return errResponse(50, "Access denied.")
	}
	if err := s.db.DeletePlaylist(ctx, store.DeletePlaylistParams{ID: id}); err != nil {
		return errResponse(0, "Database error.")
	}
	return okResponse()
}

// ── Media (raw HTTP handlers) ─────────────────────────────────────────────────

func (s *Service) streamTrackHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "bad form", http.StatusBadRequest)
			return
		}
		format := r.FormValue("f")
		if format != "json" {
			format = "xml"
		}

		user, apiErr := s.authenticate(r.Context(), r)
		if apiErr != nil {
			writeResponse(w, format, errResponse(apiErr.code, apiErr.msg))
			return
		}
		_ = user

		trackID := r.FormValue("id")
		if trackID == "" {
			writeResponse(w, format, errResponse(10, "Required parameter 'id' is missing."))
			return
		}

		track, err := s.db.GetTrackByID(r.Context(), trackID)
		if err != nil {
			writeResponse(w, format, errResponse(70, "Song not found."))
			return
		}

		size, err := s.obj.Size(r.Context(), track.FileKey)
		if err != nil {
			writeResponse(w, format, errResponse(70, "File not found."))
			return
		}

		rc, err := s.obj.GetRange(r.Context(), track.FileKey, 0, size)
		if err != nil {
			writeResponse(w, format, errResponse(70, "File not found."))
			return
		}
		defer func() { _ = rc.Close() }()

		w.Header().Set("Content-Type", mimeForFormat(track.Format))
		w.Header().Set("Content-Length", strconv.FormatInt(size, 10))
		w.Header().Set("Accept-Ranges", "none")
		w.WriteHeader(http.StatusOK)
		_, _ = io.Copy(w, rc)
	}
}

func (s *Service) getCoverArtHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "bad form", http.StatusBadRequest)
			return
		}
		format := r.FormValue("f")
		if format != "json" {
			format = "xml"
		}

		_, apiErr := s.authenticate(r.Context(), r)
		if apiErr != nil {
			writeResponse(w, format, errResponse(apiErr.code, apiErr.msg))
			return
		}

		id := r.FormValue("id")
		if id == "" {
			writeResponse(w, format, errResponse(10, "Required parameter 'id' is missing."))
			return
		}

		ctx := r.Context()
		coverKey := s.resolveCoverKey(ctx, id)
		if coverKey == "" {
			http.NotFound(w, r)
			return
		}

		size, err := s.obj.Size(ctx, coverKey)
		if err != nil {
			http.NotFound(w, r)
			return
		}

		rc, err := s.obj.GetRange(ctx, coverKey, 0, size)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		defer func() { _ = rc.Close() }()

		w.Header().Set("Content-Type", "image/jpeg")
		w.Header().Set("Content-Length", strconv.FormatInt(size, 10))
		w.Header().Set("Cache-Control", "public, max-age=86400")
		_, _ = io.Copy(w, rc)
	}
}

func (s *Service) resolveCoverKey(ctx context.Context, id string) string {
	switch {
	case strings.HasPrefix(id, "al-"):
		albumID := strings.TrimPrefix(id, "al-")
		album, err := s.db.GetAlbumByID(ctx, albumID)
		if err == nil && album.CoverArtKey != nil {
			return *album.CoverArtKey
		}
	case strings.HasPrefix(id, "ar-"):
		artistID := strings.TrimPrefix(id, "ar-")
		artist, err := s.db.GetArtistByID(ctx, artistID)
		if err == nil && artist.ImageKey != nil {
			return *artist.ImageKey
		}
	default:
		track, err := s.db.GetTrackByID(ctx, id)
		if err == nil && track.AlbumID != nil {
			album, err2 := s.db.GetAlbumByID(ctx, *track.AlbumID)
			if err2 == nil && album.CoverArtKey != nil {
				return *album.CoverArtKey
			}
		}
	}
	return ""
}

// ── Annotation ────────────────────────────────────────────────────────────────

func (s *Service) star(rc *reqCtx) *Response {
	ctx := rc.r.Context()
	for _, id := range rc.r.Form["id"] {
		_ = s.db.AddFavorite(ctx, store.FavoriteParams{UserID: rc.user.ID, TrackID: id})
	}
	return okResponse()
}

func (s *Service) unstar(rc *reqCtx) *Response {
	ctx := rc.r.Context()
	for _, id := range rc.r.Form["id"] {
		_ = s.db.RemoveFavorite(ctx, store.FavoriteParams{UserID: rc.user.ID, TrackID: id})
	}
	return okResponse()
}

func (s *Service) setRating(rc *reqCtx) *Response {
	id := rc.param("id")
	if id == "" {
		return errResponse(10, "Required parameter 'id' is missing.")
	}
	rating := rc.paramInt("rating", 0)
	if rating == 0 {
		_ = s.db.DeleteRating(rc.r.Context(), rc.user.ID, id)
	} else {
		_ = s.db.SetRating(rc.r.Context(), store.RateTrackParams{UserID: rc.user.ID, TrackID: id, Rating: rating})
	}
	return okResponse()
}

func (s *Service) scrobble(rc *reqCtx) *Response {
	id := rc.param("id")
	if id == "" {
		return errResponse(10, "Required parameter 'id' is missing.")
	}
	track, err := s.db.GetTrackByID(rc.r.Context(), id)
	if err != nil {
		return errResponse(70, "Song not found.")
	}
	_ = s.db.RecordPlay(rc.r.Context(), store.RecordPlayParams{
		UserID:           rc.user.ID,
		TrackID:          track.ID,
		DurationPlayedMs: track.DurationMs,
	})
	return okResponse()
}

// ── User management ───────────────────────────────────────────────────────────

func (s *Service) getUser(rc *reqCtx) *Response {
	r := okResponse()
	r.User = userToSubsonic(rc.user)
	return r
}

// ── Conversion helpers ────────────────────────────────────────────────────────

func (s *Service) tracksToChildren(ctx context.Context, tracks []store.Track, starred bool) []Child {
	// Batch-fetch album IDs to avoid N+1.
	albumIDs := make([]string, 0, len(tracks))
	seen := map[string]bool{}
	for _, t := range tracks {
		if t.AlbumID != nil && !seen[*t.AlbumID] {
			albumIDs = append(albumIDs, *t.AlbumID)
			seen[*t.AlbumID] = true
		}
	}

	albumCache := map[string]store.Album{}
	if len(albumIDs) > 0 {
		titles, _ := s.db.GetAlbumTitlesByIDs(ctx, albumIDs)
		for _, id := range albumIDs {
			// We only have title from the batch, so build a minimal Album.
			if title, ok := titles[id]; ok {
				albumCache[id] = store.Album{ID: id, Title: title}
			}
		}
	}

	children := make([]Child, 0, len(tracks))
	for _, t := range tracks {
		var album store.Album
		if t.AlbumID != nil {
			album = albumCache[*t.AlbumID]
		}
		ch := trackToChild(t, album)
		if starred {
			ch.Starred = time.Now().UTC().Format(time.RFC3339)
		}
		children = append(children, ch)
	}
	return children
}

func trackToChild(t store.Track, album store.Album) Child {
	ch := Child{
		ID:          t.ID,
		IsDir:       false,
		Title:       t.Title,
		Duration:    t.DurationMs / 1000,
		Size:        t.FileSize,
		Type:        "music",
		Created:     t.CreatedAt.UTC().Format(time.RFC3339),
		Suffix:      strings.ToLower(t.Format),
		ContentType: mimeForFormat(t.Format),
	}
	if t.TrackNumber != nil {
		ch.Track = *t.TrackNumber
	}
	if t.DiscNumber != 0 {
		ch.DiscNumber = t.DiscNumber
	}
	if t.BitrateKbps != nil {
		ch.BitRate = *t.BitrateKbps
	}
	if t.AlbumID != nil {
		ch.Parent = "al-" + *t.AlbumID
		ch.AlbumID = *t.AlbumID
		ch.CoverArt = "al-" + *t.AlbumID
	}
	if t.ArtistID != nil {
		ch.ArtistID = *t.ArtistID
	}
	if album.ID != "" {
		ch.Album = album.Title
		if album.ReleaseYear != nil {
			ch.Year = *album.ReleaseYear
		}
		if album.ArtistName != nil {
			ch.Artist = *album.ArtistName
		}
	}
	return ch
}

func albumToID3(al store.Album) AlbumID3 {
	a := AlbumID3{
		ID:        al.ID,
		Name:      al.Title,
		SongCount: al.TrackCount,
		Created:   al.CreatedAt.UTC().Format(time.RFC3339),
	}
	if al.ArtistID != nil {
		a.ArtistID = *al.ArtistID
	}
	if al.ArtistName != nil {
		a.Artist = *al.ArtistName
	}
	if al.CoverArtKey != nil {
		a.CoverArt = "al-" + al.ID
	}
	if al.ReleaseYear != nil {
		a.Year = *al.ReleaseYear
	}
	return a
}

func playlistToSubsonic(pl store.Playlist, ownerUsername string) SubsonicPlaylist {
	sp := SubsonicPlaylist{
		ID:        pl.ID,
		Name:      pl.Name,
		Comment:   pl.Description,
		Owner:     ownerUsername,
		Public:    pl.IsPublic,
		SongCount: pl.TrackCount,
		Created:   pl.CreatedAt,
	}
	if pl.CoverArtKey != nil {
		sp.CoverArt = "pl-" + pl.ID
	}
	return sp
}

func userToSubsonic(u *store.User) *SubsonicUser {
	return &SubsonicUser{
		Username:          u.Username,
		Email:             u.Email,
		ScrobblingEnabled: false,
		AdminRole:         u.IsAdmin,
		SettingsRole:      true,
		DownloadRole:      true,
		PlaylistRole:      true,
		CoverArtRole:      true,
		CommentRole:       true,
		StreamRole:        true,
		ShareRole:         true,
	}
}

func mimeForFormat(format string) string {
	switch strings.ToLower(format) {
	case "mp3":
		return "audio/mpeg"
	case "flac":
		return "audio/flac"
	case "ogg":
		return "audio/ogg"
	case "opus":
		return "audio/ogg; codecs=opus"
	case "aac", "m4a":
		return "audio/mp4"
	case "wav":
		return "audio/wav"
	case "aiff", "aif":
		return "audio/aiff"
	default:
		return "audio/mpeg"
	}
}

func indexKey(name string) string {
	name = stripIgnoredArticles(name)
	for _, r := range name {
		if unicode.IsLetter(r) {
			return strings.ToUpper(string(r))
		}
	}
	return "#"
}

func stripIgnoredArticles(name string) string {
	lower := strings.ToLower(name)
	for _, article := range []string{"the ", "a ", "an "} {
		if strings.HasPrefix(lower, article) {
			return name[len(article):]
		}
	}
	return name
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// Ensure fmt is used.
var _ = fmt.Sprintf
