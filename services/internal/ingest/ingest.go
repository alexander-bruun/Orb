package ingest

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	_ "image/png"
	"io"
	"log/slog"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/alexander-bruun/orb/services/internal/musicbrainz"
	"github.com/alexander-bruun/orb/services/internal/objstore"
	"github.com/alexander-bruun/orb/services/internal/similarity"
	"github.com/alexander-bruun/orb/services/internal/store"
	"github.com/dhowden/tag"
	"github.com/fsnotify/fsnotify"
)

var ErrSkipped = errors.New("skipped")

var featuredArtistRe = regexp.MustCompile(
	`(?i)[\[\(]?\s*(?:feat\.?|ft\.?|featuring)\s+([^\]\)]+)[\]\)]?\s*$`)

var featSplitRe = regexp.MustCompile(`(?i)\s*[,&]\s*|\s+and\s+`)

var editionBracketsRe = regexp.MustCompile(`\{[^}]+\}|\[[^\]]+\]`)

func splitArtistList(s string) []string {
	var out []string
	for _, name := range featSplitRe.Split(s, -1) {
		name = strings.Trim(strings.TrimSpace(name), "()[]")
		if name != "" {
			out = append(out, name)
		}
	}
	return out
}

func parseFeaturedArtists(title string) (cleanTitle string, featuredNames []string) {
	m := featuredArtistRe.FindStringSubmatchIndex(title)
	if m == nil {
		return title, nil
	}
	raw := strings.TrimSpace(title[m[2]:m[3]])
	cleanTitle = strings.TrimRight(strings.TrimSpace(title[:m[0]]), " -–")
	return cleanTitle, splitArtistList(raw)
}

func albumEditionFromDir(dirName string) string {
	matches := editionBracketsRe.FindAllString(dirName, -1)
	if len(matches) == 0 {
		return ""
	}
	return strings.Join(matches, " ")
}

func parseBPMTag(s string) float64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil || v <= 0 || v > 400 {
		return 0
	}
	return math.Round(v*10) / 10
}

func normalizeKeyTag(s string) string {
	s = strings.TrimSpace(s)
	if s == "" || strings.EqualFold(s, "none") || strings.EqualFold(s, "unknown") {
		return ""
	}
	if len(s) > 0 {
		return strings.ToUpper(s[:1]) + s[1:]
	}
	return s
}

func parseReplayGainTag(s string) float64 {
	s = strings.TrimSpace(s)
	s = strings.TrimSuffix(strings.ToLower(s), " db")
	s = strings.TrimSuffix(s, "db")
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return v
}

func rawTagString(raw map[string]interface{}, keys ...string) string {
	for _, k := range keys {
		if v, ok := raw[k]; ok {
			switch s := v.(type) {
			case string:
				if s != "" {
					return s
				}
			case []string:
				if len(s) > 0 {
					return strings.Join(s, ", ")
				}
			}
		}
	}
	return ""
}

func extractFeaturedArtists(m interface {
	Title() string
	Raw() map[string]interface{}
}, fallbackPath string) (cleanTitle string, featuredNames []string) {
	title := coalesce(m.Title(), filepath.Base(fallbackPath))
	raw := m.Raw()

	featRaw := rawTagString(raw,
		"feat", "featured_artist", "featured artist", "featuredartist",
		"TXXX:FEAT", "TXXX:Featured Artist", "TXXX:featured artist",
		"TXXX:FEATUREDARTIST", "TXXX:ARTISTS",
		"----:com.apple.iTunes:FEAT",
		"----:com.apple.iTunes:FEATURED ARTIST",
		"----:com.apple.iTunes:ARTISTS",
	)
	if featRaw != "" {
		cleanTitle, _ = parseFeaturedArtists(title)
		return cleanTitle, splitArtistList(featRaw)
	}

	artistsRaw := rawTagString(raw, "artists", "ARTISTS")
	primaryArtist := rawTagString(raw, "artist", "TPE1")
	if artistsRaw != "" && artistsRaw != primaryArtist {
		all := splitArtistList(artistsRaw)
		primary := strings.ToLower(strings.TrimSpace(primaryArtist))
		var feat []string
		for _, a := range all {
			if strings.ToLower(strings.TrimSpace(a)) != primary {
				feat = append(feat, a)
			}
		}
		if len(feat) > 0 {
			cleanTitle, _ = parseFeaturedArtists(title)
			return cleanTitle, feat
		}
	}

	return parseFeaturedArtists(title)
}

// Config holds all ingest configuration.
type Config struct {
	Dirs              []string
	UserID            string
	DryRun            bool
	Watch             bool
	Workers           int
	ComputeSimilarity bool
	Enrich            bool
	PollInterval      time.Duration
}

// ingestEntry is the in-memory record for a file that has been processed.
type ingestEntry struct {
	mtimeUnix int64
	fileSize  int64
}

// Ingester holds shared state used across the initial scan and the optional watcher.
type Ingester struct {
	db  *store.Store
	obj objstore.ObjectStore
	cfg Config

	stateMu sync.RWMutex
	state   map[string]ingestEntry

	folderImgCache sync.Map
	coveredAlbums  sync.Map

	mb              *musicbrainz.Client
	enrichedArtists sync.Map
	enrichedAlbums  sync.Map
}

// New creates a new Ingester with the given dependencies and config.
func New(db *store.Store, obj objstore.ObjectStore, cfg Config) *Ingester {
	g := &Ingester{
		db:  db,
		obj: obj,
		cfg: cfg,
	}
	if cfg.Enrich {
		g.mb = musicbrainz.New()
	}
	return g
}

func (g *Ingester) loadState(ctx context.Context) error {
	rows, err := g.db.LoadIngestState(ctx)
	if err != nil {
		return fmt.Errorf("load ingest state: %w", err)
	}
	g.state = make(map[string]ingestEntry, len(rows))
	for _, r := range rows {
		g.state[r.Path] = ingestEntry{mtimeUnix: r.MtimeUnix, fileSize: r.FileSize}
	}
	slog.Info("loaded ingest state", "known_files", len(g.state))
	return nil
}

func (g *Ingester) upToDate(path string, fi os.FileInfo) bool {
	g.stateMu.RLock()
	e, ok := g.state[path]
	g.stateMu.RUnlock()
	return ok && e.mtimeUnix == fi.ModTime().Unix() && e.fileSize == fi.Size()
}

func (g *Ingester) markDone(ctx context.Context, path string, fi os.FileInfo, trackID string) {
	if err := g.db.UpsertIngestState(ctx, store.IngestStateRow{
		Path:      path,
		MtimeUnix: fi.ModTime().Unix(),
		FileSize:  fi.Size(),
		TrackID:   trackID,
	}); err != nil {
		slog.Warn("persist ingest state failed", "path", path, "err", err)
	}
	g.stateMu.Lock()
	g.state[path] = ingestEntry{mtimeUnix: fi.ModTime().Unix(), fileSize: fi.Size()}
	g.stateMu.Unlock()
}

func (g *Ingester) process(ctx context.Context, path string) (trackID string, err error) {
	fi, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	if g.upToDate(path, fi) {
		return "", ErrSkipped
	}
	if g.cfg.DryRun {
		slog.Info("would ingest", "path", path)
		return "", nil
	}
	trackID, err = g.ingestFile(ctx, path, fi)
	if err != nil {
		return "", err
	}
	g.markDone(ctx, path, fi, trackID)
	slog.Info("ingested", "path", path, "track_id", trackID)
	return trackID, nil
}

// Scan performs a single scan of all configured dirs and returns new track IDs, skipped count, and error count.
func (g *Ingester) Scan(ctx context.Context) (newTrackIDs []string, skipped, errs int) {
	var paths []string
	for _, dir := range g.cfg.Dirs {
		dir = strings.TrimSpace(dir)
		if dir == "" {
			continue
		}
		if err := filepath.WalkDir(dir, func(path string, d os.DirEntry, walkErr error) error {
			if walkErr != nil {
				slog.Warn("walk error", "path", path, "err", walkErr)
				return nil
			}
			if d.IsDir() {
				if !g.cfg.Watch && path != dir {
					return filepath.SkipDir
				}
				return nil
			}
			if isAudioFile(path) {
				paths = append(paths, path)
			}
			return nil
		}); err != nil {
			slog.Warn("walk error", "dir", dir, "err", err)
		}
	}

	var nSkipped, nErrs int64
	var mu sync.Mutex
	var ids []string

	workers := g.cfg.Workers
	if workers < 1 {
		workers = 1
	}
	pathCh := make(chan string, workers*2)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for p := range pathCh {
				id, err := g.process(ctx, p)
				switch {
				case errors.Is(err, ErrSkipped):
					atomic.AddInt64(&nSkipped, 1)
				case err != nil:
					slog.Error("ingest failed", "path", p, "err", err)
					atomic.AddInt64(&nErrs, 1)
				default:
					if id != "" {
						mu.Lock()
						ids = append(ids, id)
						mu.Unlock()
					}
				}
			}
		}()
	}
	for _, p := range paths {
		pathCh <- p
	}
	close(pathCh)
	wg.Wait()
	return ids, int(nSkipped), int(nErrs)
}

func (g *Ingester) cachedFolderImage(dir string) []byte {
	if v, ok := g.folderImgCache.Load(dir); ok {
		return v.([]byte)
	}
	data := bestFolderImage(dir)
	if actual, loaded := g.folderImgCache.LoadOrStore(dir, data); loaded {
		return actual.([]byte)
	}
	return data
}

// Run performs an initial scan, then optionally enters watch mode.
// Returns when ctx is cancelled or (in non-watch mode) after the scan completes.
func (g *Ingester) Run(ctx context.Context) error {
	if err := g.loadState(ctx); err != nil {
		return err
	}

	newIDs, skipped, errs := g.Scan(ctx)
	slog.Info("ingest scan complete", "ingested", len(newIDs), "skipped", skipped, "errors", errs)

	if g.cfg.ComputeSimilarity {
		if err := g.runSimilarity(ctx, newIDs); err != nil {
			slog.Error("similarity computation failed", "err", err)
		}
	}

	if !g.cfg.Watch {
		return nil
	}

	watcher, watchErr := fsnotify.NewWatcher()
	if watchErr == nil {
		for _, dir := range g.cfg.Dirs {
			dir = strings.TrimSpace(dir)
			if dir == "" {
				continue
			}
			if addErr := watcher.Add(dir); addErr != nil {
				_ = watcher.Close()
				watchErr = addErr
				break
			}
		}
	}
	if watchErr != nil {
		slog.Warn("inotify unavailable, falling back to polling",
			"interval", g.cfg.PollInterval, "err", watchErr)
		return g.watchWithPolling(ctx)
	}
	defer watcher.Close()

	for _, dir := range g.cfg.Dirs {
		dir = strings.TrimSpace(dir)
		if dir == "" {
			continue
		}
		_ = filepath.WalkDir(dir, func(path string, d os.DirEntry, e error) error {
			if e == nil && d.IsDir() {
				_ = watcher.Add(path)
			}
			return nil
		})
	}
	slog.Info("watching", "dirs", g.cfg.Dirs)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case ev, ok := <-watcher.Events:
			if !ok {
				return nil
			}
			if ev.Op&(fsnotify.Create|fsnotify.Write|fsnotify.Rename) == 0 {
				continue
			}
			fi, err := os.Stat(ev.Name)
			if err != nil {
				continue
			}
			if fi.IsDir() {
				_ = watcher.Add(ev.Name)
				go func(p string) {
					_ = filepath.WalkDir(p, func(path string, d os.DirEntry, e error) error {
						if e != nil || d.IsDir() {
							return nil
						}
						if isAudioFile(path) {
							if _, err := g.process(ctx, path); err != nil && !errors.Is(err, ErrSkipped) {
								slog.Error("ingest failed", "path", path, "err", err)
							}
						}
						return nil
					})
				}(ev.Name)
				continue
			}
			if !isAudioFile(ev.Name) {
				continue
			}
			go func(p string) {
				if _, err := g.process(ctx, p); err != nil && !errors.Is(err, ErrSkipped) {
					slog.Error("ingest failed", "path", p, "err", err)
				}
			}(ev.Name)
		case err, ok := <-watcher.Errors:
			if !ok {
				return nil
			}
			slog.Warn("watcher error", "err", err)
		case <-time.After(10 * time.Second):
			// keep alive
		}
	}
}

func (g *Ingester) runSimilarity(ctx context.Context, newTrackIDs []string) error {
	hasData, err := g.db.HasSimilarityData(ctx)
	if err != nil {
		slog.Warn("could not query similarity table, falling back to full recompute", "err", err)
		hasData = false
	}
	if len(newTrackIDs) == 0 && hasData {
		slog.Info("no new tracks ingested — skipping similarity recompute")
		return nil
	}
	if !hasData {
		slog.Info("similarity table is empty — running full recompute")
		if err := similarity.ComputeAll(ctx, g.db); err != nil {
			return err
		}
		slog.Info("similarity computation complete")
		return nil
	}
	slog.Info("running incremental similarity update", "new_tracks", len(newTrackIDs))
	if err := similarity.ComputeForTracks(ctx, g.db, newTrackIDs); err != nil {
		return err
	}
	slog.Info("incremental similarity update complete")
	return nil
}

func (g *Ingester) watchWithPolling(ctx context.Context) error {
	slog.Warn("polling fallback active", "interval", g.cfg.PollInterval, "dirs", g.cfg.Dirs)
	ticker := time.NewTicker(g.cfg.PollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			newIDs, skipped, errs := g.Scan(ctx)
			if len(newIDs) > 0 || errs > 0 {
				slog.Info("poll scan complete", "ingested", len(newIDs), "skipped", skipped, "errors", errs)
				if g.cfg.ComputeSimilarity && len(newIDs) > 0 {
					if err := g.runSimilarity(ctx, newIDs); err != nil {
						slog.Error("similarity computation failed", "err", err)
					}
				}
			}
		}
	}
}

func (g *Ingester) ingestFile(ctx context.Context, path string, fi os.FileInfo) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", fmt.Errorf("hash: %w", err)
	}
	fingerprint := hex.EncodeToString(h.Sum(nil))
	trackID := deterministicUUID(fingerprint)

	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return "", err
	}
	m, err := tag.ReadFrom(f)
	if err != nil {
		return "", fmt.Errorf("read tags: %w", err)
	}

	albumArtistName := coalesce(m.AlbumArtist(), m.Artist(), "Unknown Artist")
	if parts := splitArtistList(albumArtistName); len(parts) > 0 {
		albumArtistName = parts[0]
	}

	trackArtistName := coalesce(m.Artist(), albumArtistName)
	var artistTagExtras []string
	if parts := splitArtistList(trackArtistName); len(parts) > 1 {
		trackArtistName = parts[0]
		for _, p := range parts[1:] {
			cleanP, featFromP := parseFeaturedArtists(p)
			if cleanP != "" {
				artistTagExtras = append(artistTagExtras, cleanP)
			}
			artistTagExtras = append(artistTagExtras, featFromP...)
		}
	} else if len(parts) == 1 {
		trackArtistName = parts[0]
	}

	albumArtistID := deterministicID("artist:" + strings.ToLower(albumArtistName))
	if _, err = g.db.UpsertArtist(ctx, store.UpsertArtistParams{
		ID:       albumArtistID,
		Name:     albumArtistName,
		SortName: sortName(albumArtistName),
	}); err != nil {
		return "", fmt.Errorf("upsert artist: %w", err)
	}

	trackArtistID := albumArtistID
	if strings.ToLower(trackArtistName) != strings.ToLower(albumArtistName) {
		trackArtistID = deterministicID("artist:" + strings.ToLower(trackArtistName))
		if _, err = g.db.UpsertArtist(ctx, store.UpsertArtistParams{
			ID:       trackArtistID,
			Name:     trackArtistName,
			SortName: sortName(trackArtistName),
		}); err != nil {
			return "", fmt.Errorf("upsert track artist: %w", err)
		}
	}

	albumTitle := coalesce(m.Album(), "Unknown Album")
	albumBase := strings.ToLower(albumArtistName) + ":" + strings.ToLower(albumTitle)
	albumGroupID := deterministicID("album:" + albumBase)
	albumDir := filepath.Dir(path)
	albumID := deterministicID("album:" + albumBase + ":" + albumDir)
	var albumEditionStr string
	if ed := albumEditionFromDir(filepath.Base(albumDir)); ed != "" {
		albumEditionStr = ed
	}

	coverKey := fmt.Sprintf("covers/%s.jpg", albumID)
	var coverArtKeyPtr *string
	if _, done := g.coveredAlbums.Load(albumID); done {
		coverArtKeyPtr = &coverKey
	} else {
		var picData []byte
		if pic := m.Picture(); pic != nil && len(pic.Data) > 0 {
			picData = pic.Data
		} else {
			picData = g.cachedFolderImage(filepath.Dir(path))
		}
		if len(picData) > 0 {
			if err := storeCoverArt(ctx, g.obj, coverKey, picData); err != nil {
				slog.Warn("cover art storage failed", "album", albumTitle, "err", err)
			} else {
				coverArtKeyPtr = &coverKey
				g.coveredAlbums.Store(albumID, struct{}{})
			}
		}
	}

	var releaseYearPtr *int
	if y := m.Year(); y > 0 {
		releaseYearPtr = &y
	}

	var albumEditionPtr *string
	if albumEditionStr != "" {
		albumEditionPtr = &albumEditionStr
	}

	if _, err = g.db.UpsertAlbum(ctx, store.UpsertAlbumParams{
		ID:           albumID,
		ArtistID:     &albumArtistID,
		Title:        albumTitle,
		ReleaseYear:  releaseYearPtr,
		CoverArtKey:  coverArtKeyPtr,
		AlbumGroupID: &albumGroupID,
		Edition:      albumEditionPtr,
	}); err != nil {
		return "", fmt.Errorf("upsert album: %w", err)
	}

	ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(path)), ".")
	if ext == "aiff" || ext == "aif" {
		ext = "wav"
	}

	fileKey := fmt.Sprintf("audio/%s/%s/%s.%s", albumArtistID, albumID, trackID, ext)

	var trackNumPtr *int
	if n, _ := m.Track(); n != 0 {
		tmp := n
		trackNumPtr = &tmp
	}
	discNum := 1
	if d, _ := m.Disc(); d != 0 {
		discNum = d
	}

	bitDepth, sampleRate, durationMs := readFLACInfo(f, ext)
	if sampleRate == 0 {
		sampleRate = 44100
	}
	var bitDepthPtr *int
	if bitDepth > 0 {
		tmp := bitDepth
		bitDepthPtr = &tmp
	}

	seekTableJSON, _ := json.Marshal([]int{})

	cleanTitle, featuredNames := extractFeaturedArtists(m, path)
	if len(artistTagExtras) > 0 {
		seen := make(map[string]bool, len(featuredNames))
		for _, n := range featuredNames {
			seen[strings.ToLower(n)] = true
		}
		for _, n := range artistTagExtras {
			if n != "" && !seen[strings.ToLower(n)] {
				featuredNames = append(featuredNames, n)
				seen[strings.ToLower(n)] = true
			}
		}
	}

	track, err := g.db.UpsertTrack(ctx, store.UpsertTrackParams{
		ID:          trackID,
		AlbumID:     &albumID,
		ArtistID:    &trackArtistID,
		Title:       cleanTitle,
		TrackNumber: trackNumPtr,
		DiscNumber:  discNum,
		DurationMs:  int(durationMs),
		FileKey:     fileKey,
		FileSize:    fi.Size(),
		Format:      ext,
		BitDepth:    bitDepthPtr,
		SampleRate:  int(sampleRate),
		Channels:    2,
		SeekTable:   seekTableJSON,
		Fingerprint: fingerprint,
	})
	if err != nil {
		return "", fmt.Errorf("upsert track: %w", err)
	}

	if len(featuredNames) > 0 {
		featuredIDs := make([]string, 0, len(featuredNames))
		for _, name := range featuredNames {
			fid := deterministicID("artist:" + strings.ToLower(name))
			if _, err := g.db.UpsertArtist(ctx, store.UpsertArtistParams{
				ID:       fid,
				Name:     name,
				SortName: sortName(name),
			}); err != nil {
				slog.Warn("upsert featured artist failed", "name", name, "err", err)
				continue
			}
			featuredIDs = append(featuredIDs, fid)
			if g.mb != nil {
				g.enrichAfterIngest(ctx, fid, name, "", "", false)
			}
		}
		if len(featuredIDs) > 0 {
			if err := g.db.SetTrackFeaturedArtists(ctx, trackID, featuredIDs); err != nil {
				slog.Warn("set featured artists failed", "track_id", trackID, "err", err)
			}
		}
	}

	if exists, _ := g.obj.Exists(ctx, fileKey); !exists {
		if _, err := f.Seek(0, io.SeekStart); err != nil {
			return "", err
		}
		if err := g.obj.Put(ctx, fileKey, f, fi.Size()); err != nil {
			return "", fmt.Errorf("store audio: %w", err)
		}
	}

	if g.cfg.UserID != "" {
		if err := g.db.AddTrackToLibrary(ctx, store.AddTrackToLibraryParams{
			UserID:  g.cfg.UserID,
			TrackID: track.ID,
		}); err != nil {
			slog.Warn("add to library failed", "track_id", track.ID, "err", err)
		}
	}

	if g.mb != nil {
		g.enrichAfterIngest(ctx, albumArtistID, albumArtistName, albumID, albumTitle, coverArtKeyPtr == nil)
	}

	raw := m.Raw()
	bpm := parseBPMTag(rawTagString(raw, "bpm", "TEMPO", "TBPM"))
	keyEst := normalizeKeyTag(rawTagString(raw, "key", "initialkey", "TKEY"))
	rgain := parseReplayGainTag(rawTagString(raw, "replaygain_track_gain", "TXXX:REPLAYGAIN_TRACK_GAIN"))
	if bpm != 0 || keyEst != "" || rgain != 0 {
		if err := g.db.UpsertTrackFeatures(ctx, trackID, bpm, keyEst, rgain); err != nil {
			slog.Warn("upsert track features failed", "track_id", trackID, "err", err)
		}
	}

	return trackID, nil
}

func (g *Ingester) enrichAfterIngest(ctx context.Context, artistID, artistName, albumID, albumTitle string, missingCoverArt bool) {
	if _, done := g.enrichedArtists.LoadOrStore(artistID, struct{}{}); !done {
		result, err := g.mb.EnrichArtist(ctx, artistName)
		if err != nil {
			slog.Warn("enrich artist failed", "artist", artistName, "err", err)
		} else if result != nil {
			var imageKeyPtr *string
			if imgData, imgErr := g.mb.FetchArtistImage(ctx, result.URLRelations); imgErr != nil {
				slog.Warn("fetch artist image failed", "artist", artistName, "err", imgErr)
			} else if imgData != nil {
				imageKey := fmt.Sprintf("artists/%s.jpg", artistID)
				if err := storeCoverArt(ctx, g.obj, imageKey, imgData); err != nil {
					slog.Warn("store artist image failed", "artist", artistName, "err", err)
				} else {
					imageKeyPtr = &imageKey
				}
			}
			if err := g.db.UpdateArtistEnrichment(ctx, store.UpdateArtistEnrichmentParams{
				ID:             artistID,
				Mbid:           strPtr(result.Mbid),
				ArtistType:     strPtr(result.ArtistType),
				Country:        strPtr(result.Country),
				BeginDate:      strPtr(result.BeginDate),
				EndDate:        strPtr(result.EndDate),
				Disambiguation: strPtr(result.Disambiguation),
				ImageKey:       imageKeyPtr,
			}); err != nil {
				slog.Warn("update artist enrichment failed", "artist", artistName, "err", err)
			}
			g.persistGenres(ctx, result.Genres, func(ids []string) {
				if err := g.db.SetArtistGenres(ctx, artistID, ids); err != nil {
					slog.Warn("set artist genres failed", "artist", artistName, "err", err)
				}
			})
		}
	}

	if albumID == "" {
		return
	}
	if _, done := g.enrichedAlbums.LoadOrStore(albumID, struct{}{}); !done {
		result, err := g.mb.EnrichAlbum(ctx, albumTitle, artistName)
		if err != nil {
			slog.Warn("enrich album failed", "album", albumTitle, "err", err)
		} else if result != nil {
			if err := g.db.UpdateAlbumEnrichment(ctx, store.UpdateAlbumEnrichmentParams{
				ID:               albumID,
				Mbid:             strPtr(result.ReleaseGroupMbid),
				Label:            strPtr(result.Label),
				AlbumType:        strPtr(result.AlbumType),
				ReleaseDate:      strPtr(result.ReleaseDate),
				ReleaseGroupMbid: strPtr(result.ReleaseGroupMbid),
			}); err != nil {
				slog.Warn("update album enrichment failed", "album", albumTitle, "err", err)
			}
			g.persistGenres(ctx, result.Genres, func(ids []string) {
				if err := g.db.SetAlbumGenres(ctx, albumID, ids); err != nil {
					slog.Warn("set album genres failed", "album", albumTitle, "err", err)
				}
			})
			if missingCoverArt && result.ReleaseGroupMbid != "" {
				if imgData, imgErr := g.mb.FetchAlbumCoverArt(ctx, result.ReleaseGroupMbid); imgErr != nil {
					slog.Warn("fetch album cover art failed", "album", albumTitle, "err", imgErr)
				} else if imgData != nil {
					coverKey := fmt.Sprintf("covers/%s.jpg", albumID)
					if err := storeCoverArt(ctx, g.obj, coverKey, imgData); err != nil {
						slog.Warn("store album cover art failed", "album", albumTitle, "err", err)
					} else {
						g.coveredAlbums.Store(albumID, struct{}{})
						if err := g.db.UpdateAlbumCoverArt(ctx, albumID, coverKey); err != nil {
							slog.Warn("update album cover art key failed", "album", albumTitle, "err", err)
						}
					}
				}
			}
		}
	}
}

func (g *Ingester) persistGenres(ctx context.Context, names []string, setFn func([]string)) {
	if len(names) == 0 {
		return
	}
	ids := make([]string, 0, len(names))
	for _, name := range names {
		id := musicbrainz.GenreID(name)
		if err := g.db.UpsertGenre(ctx, id, name); err != nil {
			slog.Warn("upsert genre failed", "genre", name, "err", err)
			continue
		}
		ids = append(ids, id)
	}
	if len(ids) > 0 {
		setFn(ids)
	}
}

func strPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func isAudioFile(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".flac", ".wav", ".mp3", ".aiff", ".aif":
		return true
	}
	return false
}

func bestFolderImage(dir string) []byte {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var bestData []byte
	bestDelta := int(^uint(0) >> 1)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := strings.ToLower(entry.Name())
		if !strings.HasSuffix(name, ".jpg") && !strings.HasSuffix(name, ".jpeg") && !strings.HasSuffix(name, ".png") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil || len(b) == 0 {
			continue
		}
		img, _, err := image.Decode(bytes.NewReader(b))
		if err != nil {
			continue
		}
		w, h := img.Bounds().Dx(), img.Bounds().Dy()
		if delta := abs(w - h); delta < bestDelta {
			bestDelta = delta
			bestData = b
		}
	}
	return bestData
}

func storeCoverArt(ctx context.Context, obj objstore.ObjectStore, key string, data []byte) error {
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return obj.Put(ctx, key, bytes.NewReader(data), int64(len(data)))
	}
	pr, pw := io.Pipe()
	go func() {
		pw.CloseWithError(jpeg.Encode(pw, img, &jpeg.Options{Quality: 90}))
	}()
	defer pr.Close()
	return obj.Put(ctx, key, pr, -1)
}

func readFLACInfo(f *os.File, ext string) (bitDepth, sampleRate int, durationMs int64) {
	if ext != "flac" {
		return
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return
	}
	buf := make([]byte, 42)
	if _, err := io.ReadFull(f, buf); err != nil {
		return
	}
	if string(buf[0:4]) != "fLaC" || buf[4]&0x7F != 0 {
		return
	}
	if binary.BigEndian.Uint32([]byte{0, buf[5], buf[6], buf[7]}) != 34 {
		return
	}
	si := buf[8:]
	sampleRate = int(uint32(si[10])<<12 | uint32(si[11])<<4 | uint32(si[12])>>4)
	bitDepth = int((si[12]&0x01)<<4|si[13]>>4) + 1
	totalSamples := int64(si[13]&0x0F)<<32 |
		int64(si[14])<<24 | int64(si[15])<<16 |
		int64(si[16])<<8 | int64(si[17])
	if sampleRate > 0 && totalSamples > 0 {
		durationMs = totalSamples * 1000 / int64(sampleRate)
	}
	return
}

func deterministicID(seed string) string {
	h := sha256.Sum256([]byte(seed))
	return hex.EncodeToString(h[:8])
}

func deterministicUUID(fingerprint string) string {
	h := sha256.Sum256([]byte("track:" + fingerprint))
	h[6] = (h[6] & 0x0f) | 0x40
	h[8] = (h[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", h[0:4], h[4:6], h[6:8], h[8:10], h[10:16])
}

func sortName(name string) string {
	for _, p := range []string{"The ", "A ", "An "} {
		if strings.HasPrefix(name, p) {
			return strings.TrimPrefix(name, p) + ", " + strings.TrimSuffix(p, " ")
		}
	}
	return name
}

func coalesce(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
