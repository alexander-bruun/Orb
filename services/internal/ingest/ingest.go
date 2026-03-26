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
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/alexander-bruun/orb/services/internal/kvkeys"
	"github.com/alexander-bruun/orb/services/internal/lyricfetch"
	"github.com/alexander-bruun/orb/services/internal/musicbrainz"
	"github.com/alexander-bruun/orb/services/internal/objstore"
	"github.com/alexander-bruun/orb/services/internal/similarity"
	"github.com/alexander-bruun/orb/services/internal/store"
	"github.com/dhowden/tag"
	"github.com/fsnotify/fsnotify"
	"github.com/redis/go-redis/v9"
)

var ErrSkipped = errors.New("skipped")

var featuredArtistRe = regexp.MustCompile(
	`(?i)[\[\(]?\s*(?:feat\.?|ft\.?|featuring)\s+([^\]\)]+)[\]\)]?\s*$`)

var featSplitRe = regexp.MustCompile(`(?i)\s*[,;&]\s*|\s+and\s+`)

var genreSplitRe = regexp.MustCompile(`\s*[;/\x00]\s*|\s*,\s*`)

func splitGenreList(s string) []string {
	var out []string
	for _, g := range genreSplitRe.Split(s, -1) {
		g = strings.TrimSpace(g)
		if g != "" {
			out = append(out, g)
		}
	}
	return out
}

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
	ExcludeGlobs      []string // glob patterns for directories/files to skip (supports **)
	DryRun            bool
	Workers           int
	ComputeSimilarity bool
	Enrich            bool
	GenerateWaveforms bool
	FetchLyrics       bool
	PollInterval      time.Duration
	StableTime        time.Duration // minimum age (mtime) before a file is considered ready to ingest; detects incomplete downloads
}

// ingestEntry is the in-memory record for a file that has been processed.
type ingestEntry struct {
	mtimeUnix int64
	fileSize  int64
}

// ProgressEvent is published to Redis during a scan so the admin SSE endpoint
// can stream real-time ingest progress to connected browsers.
type ProgressEvent struct {
	Type     string `json:"type"` // "progress" | "complete" | "error"
	Total    int    `json:"total"`
	Done     int    `json:"done"`
	Skipped  int    `json:"skipped"`
	Errors   int    `json:"errors"`
	FilePath string `json:"file_path,omitempty"`
	Message  string `json:"message,omitempty"`
}

// Ingester holds shared state used across the initial scan and the optional watcher.
type Ingester struct {
	db  *store.Store
	obj objstore.ObjectStore
	cfg Config
	kv  *redis.Client // optional; nil = no SSE event publishing

	stateMu sync.RWMutex
	state   map[string]ingestEntry

	folderImgCache sync.Map
	coveredAlbums  sync.Map

	mb              *musicbrainz.Client
	enrichedArtists sync.Map
	enrichedAlbums  sync.Map
}

// New creates a new Ingester with the given dependencies and config.
// kv is optional; when non-nil, progress events are published via Redis pub/sub
// so that the admin SSE endpoint can stream them to browsers.
func New(db *store.Store, obj objstore.ObjectStore, cfg Config, kv *redis.Client) *Ingester {
	g := &Ingester{
		db:    db,
		obj:   obj,
		cfg:   cfg,
		kv:    kv,
		state: make(map[string]ingestEntry),
	}
	if cfg.Enrich {
		g.mb = musicbrainz.New()
	}
	return g
}

// publishEvent sends a ProgressEvent to the Redis pub/sub channel if kv is set.
func (g *Ingester) publishEvent(ctx context.Context, ev ProgressEvent) {
	if g.kv == nil {
		return
	}
	data, _ := json.Marshal(ev)
	if err := g.kv.Publish(ctx, kvkeys.IngestEvents(), string(data)).Err(); err != nil {
		slog.Warn("publish ingest event failed", "action", "publish ingest event", "err", err)
	}
}

func (g *Ingester) loadState(ctx context.Context) error {
	rows, err := g.db.LoadIngestState(ctx)
	if err != nil {
		return fmt.Errorf("load ingest state: %w", err)
	}
	newState := make(map[string]ingestEntry, len(rows))
	for _, r := range rows {
		newState[r.Path] = ingestEntry{mtimeUnix: r.MtimeUnix, fileSize: r.FileSize}
	}
	g.stateMu.Lock()
	g.state = newState
	g.stateMu.Unlock()
	slog.Info("loaded ingest state", "known_files", len(newState))
	return nil
}

// ClearState wipes the in-memory ingest state so the next scan re-processes
// every file regardless of whether it has changed.
func (g *Ingester) ClearState() {
	g.stateMu.Lock()
	g.state = make(map[string]ingestEntry)
	g.stateMu.Unlock()
	slog.Info("ingest: state cleared for force rescan")
}

// isStable checks if a file is old enough to be considered ready for ingest.
// Files with recent modification times (being downloaded) are not considered stable.
func (g *Ingester) isStable(fi os.FileInfo) bool {
	if g.cfg.StableTime == 0 {
		return true // stability check disabled
	}
	age := time.Since(fi.ModTime())
	return age >= g.cfg.StableTime
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
	if !g.isStable(fi) {
		slog.Debug("ingest: skipping unstable file (in-progress download?)", "path", path, "mtime_age_sec", int(time.Since(fi.ModTime()).Seconds()))
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
	// Reload state from DB so already-ingested files are skipped even after a
	// restart or when Scan is called directly (e.g. HTTP-triggered scan) rather
	// than via Run which called loadState once at startup.
	if err := g.loadState(ctx); err != nil {
		slog.Warn("ingest: failed to reload state, proceeding with stale cache", "err", err)
	}

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
				if path != dir && g.isDirExcluded(path) {
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

	total := len(paths)
	var nDone, nSkipped, nErrs int64
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
				done := int(atomic.AddInt64(&nDone, 1))
				g.publishEvent(ctx, ProgressEvent{
					Type:     "progress",
					Total:    total,
					Done:     done,
					Skipped:  int(atomic.LoadInt64(&nSkipped)),
					Errors:   int(atomic.LoadInt64(&nErrs)),
					FilePath: p,
				})
			}
		}()
	}
	for _, p := range paths {
		pathCh <- p
	}
	close(pathCh)
	wg.Wait()

	// Prune DB records for files that were deleted from disk since the last scan.
	pruned, objKeys, err := g.db.PruneOrphanedTracks(ctx, paths)
	if err != nil {
		slog.Warn("ingest: prune orphaned tracks failed", "err", err)
	} else if pruned > 0 {
		slog.Info("ingest: pruned orphaned tracks", "count", pruned)
		for _, k := range objKeys {
			if err := g.obj.Delete(ctx, k); err != nil {
				slog.Warn("ingest: delete orphaned object failed", "key", k, "err", err)
			}
		}
	}

	g.publishEvent(ctx, ProgressEvent{
		Type:    "complete",
		Total:   total,
		Done:    int(nDone),
		Skipped: int(nSkipped),
		Errors:  int(nErrs),
	})
	return ids, int(nSkipped), int(nErrs)
}

// ReingestAlbum clears the ingest state for all tracks belonging to albumID,
// then re-processes only those files. Unlike Scan, it does not walk all dirs.
func (g *Ingester) ReingestAlbum(ctx context.Context, albumID string) (newTrackIDs []string, skipped, errs int) {
	paths, err := g.db.DeleteIngestStateForAlbum(ctx, albumID)
	if err != nil {
		slog.Error("reingest album: delete ingest state", "album_id", albumID, "err", err)
		return nil, 0, 1
	}
	if len(paths) == 0 {
		slog.Info("reingest album: no ingest state found, nothing to do", "album_id", albumID)
		return nil, 0, 0
	}

	// Remove paths from in-memory state so upToDate returns false.
	g.stateMu.Lock()
	for _, p := range paths {
		delete(g.state, p)
	}
	g.stateMu.Unlock()

	var mu sync.Mutex
	var ids []string
	for _, p := range paths {
		id, err := g.process(ctx, p)
		switch {
		case errors.Is(err, ErrSkipped):
			skipped++
		case err != nil:
			slog.Error("reingest album: process failed", "path", p, "err", err)
			errs++
		default:
			if id != "" {
				mu.Lock()
				ids = append(ids, id)
				mu.Unlock()
			}
		}
	}
	return ids, skipped, errs
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
	defer func() {
		if err := watcher.Close(); err != nil {
			slog.Warn("ingest: watcher close failed", "err", err)
		}
	}()

	for _, dir := range g.cfg.Dirs {
		dir = strings.TrimSpace(dir)
		if dir == "" {
			continue
		}
		_ = filepath.WalkDir(dir, func(path string, d os.DirEntry, e error) error {
			if e == nil && d.IsDir() {
				if path != dir && g.isDirExcluded(path) {
					return filepath.SkipDir
				}
				_ = watcher.Add(path)
			}
			return nil
		})
	}
	slog.Info("watching", "dirs", g.cfg.Dirs)

	pollTicker := time.NewTicker(g.cfg.PollInterval)
	defer pollTicker.Stop()

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
				if g.isDirExcluded(ev.Name) {
					continue
				}
				_ = watcher.Add(ev.Name)
				go func(p string) {
					_ = filepath.WalkDir(p, func(path string, d os.DirEntry, e error) error {
						if e != nil {
							return nil
						}
						if d.IsDir() {
							if path != p && g.isDirExcluded(path) {
								return filepath.SkipDir
							}
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
		case <-pollTicker.C:
			// Periodic safety-net scan: catches new files on filesystems where
			// fsnotify succeeds without error but silently delivers no events
			// (e.g. NTFS/CIFS mounts in WSL2, some network filesystems).
			go func() {
				newIDs, _, _ := g.Scan(ctx)
				if len(newIDs) > 0 && g.cfg.ComputeSimilarity {
					if err := g.runSimilarity(ctx, newIDs); err != nil {
						slog.Error("similarity computation failed", "err", err)
					}
				}
			}()
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
	defer func() {
		if cerr := f.Close(); cerr != nil {
			slog.Warn("ingest: file close failed", "path", path, "err", cerr)
		}
	}()

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
	if !strings.EqualFold(trackArtistName, albumArtistName) {
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

	_, err = g.db.UpsertTrack(ctx, store.UpsertTrackParams{
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

	if genreTag := m.Genre(); genreTag != "" {
		genreNames := splitGenreList(genreTag)
		g.persistGenres(ctx, genreNames, func(ids []string) {
			if err := g.db.SetTrackGenres(ctx, trackID, ids); err != nil {
				slog.Warn("set track genres failed", "track_id", trackID, "err", err)
			}
		})
	}

	if g.cfg.GenerateWaveforms {
		if peaks := generateWaveformPeaks(path); peaks != nil {
			if err := g.db.UpsertTrackWaveform(ctx, trackID, peaks); err != nil {
				slog.Warn("store waveform failed", "track_id", trackID, "err", err)
			}
		}
	}

	if g.cfg.FetchLyrics {
		// Only fetch if not already stored (covers re-ingest without re-fetching).
		existing, _ := g.db.GetTrackLyrics(ctx, trackID)
		if existing == "" {
			artistName := coalesce(m.AlbumArtist(), m.Artist())
			albumTitle := m.Album()
			trackTitle := coalesce(m.Title(), filepath.Base(path))
			res, err := lyricfetch.Search(ctx, artistName, albumTitle, trackTitle, int(durationMs))
			if err == nil {
				lrc := res.LRC
				if lrc == "" {
					lrc = res.Plain
				}
				if lrc != "" {
					if err := g.db.SetTrackLyrics(ctx, trackID, lrc); err != nil {
						slog.Warn("store lyrics failed", "track_id", trackID, "err", err)
					}
				}
			} else {
				slog.Debug("lyricfetch: no lyrics", "track_id", trackID, "err", err)
			}
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

// isDirExcluded returns true if the given directory path matches any of the
// configured exclude globs. It also tests each intermediate path component so
// that a simple pattern like "temp" matches /music/temp without a leading **/
func (g *Ingester) isDirExcluded(dirPath string) bool {
	for _, glob := range g.cfg.ExcludeGlobs {
		if matchExcludeGlob(glob, dirPath) {
			return true
		}
	}
	return false
}

// matchExcludeGlob returns true when path matches pattern. Supports standard
// filepath.Match syntax plus ** for any number of path segments.
//   - Simple name (no separators): matched against every path component.
//   - Pattern with ** : the ** is replaced by matching zero-or-more segments.
func matchExcludeGlob(pattern, path string) bool {
	pattern = filepath.Clean(pattern)
	path = filepath.Clean(path)

	// Full-path match (handles absolute patterns and single-level globs).
	if ok, _ := filepath.Match(pattern, path); ok {
		return true
	}

	// Simple name pattern (no path separators): test every component.
	if !strings.ContainsAny(pattern, "/\\") && !strings.Contains(pattern, "**") {
		for _, part := range strings.Split(filepath.ToSlash(path), "/") {
			if ok, _ := filepath.Match(pattern, part); ok {
				return true
			}
		}
		return false
	}

	// ** matching: strip each leading segment and try the pattern (minus its
	// own leading **/) against the remaining path suffix.
	if strings.Contains(pattern, "**") {
		sub := strings.TrimPrefix(filepath.ToSlash(pattern), "**/")
		parts := strings.Split(filepath.ToSlash(path), "/")
		for i := range parts {
			candidate := strings.Join(parts[i:], "/")
			if ok, _ := filepath.Match(sub, candidate); ok {
				return true
			}
			// Also try matching just the single component for patterns like **/temp
			if ok, _ := filepath.Match(sub, parts[i]); ok {
				return true
			}
		}
	}

	return false
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
	defer func() {
		if err := pr.Close(); err != nil {
			slog.Warn("ingest: cover art pipe close failed", "err", err)
		}
	}()
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

// --- Distributed ingest: leader coordinates, all instances process ---

// ScanAndEnqueue is the distributed leader path. It loads the latest ingest
// state from the DB, walks all configured directories to find files that need
// processing, and pushes their paths onto the Redis work queue for workers to
// consume. Returns the number of paths enqueued.
func (g *Ingester) ScanAndEnqueue(ctx context.Context, kv *redis.Client) (int, error) {
	if err := g.loadState(ctx); err != nil {
		return 0, fmt.Errorf("load ingest state: %w", err)
	}

	var toProcess []string
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
				if path != dir && g.isDirExcluded(path) {
					return filepath.SkipDir
				}
				return nil
			}
			if !isAudioFile(path) {
				return nil
			}
			fi, err := os.Stat(path)
			if err != nil {
				return nil
			}
			if g.upToDate(path, fi) {
				return nil
			}
			if !g.isStable(fi) {
				slog.Debug("ingest: skipping unstable file (in-progress download?)", "path", path, "mtime_age_sec", int(time.Since(fi.ModTime()).Seconds()))
				return nil
			}
			toProcess = append(toProcess, path)
			return nil
		}); err != nil {
			slog.Warn("walk error", "dir", dir, "err", err)
		}
	}

	if len(toProcess) == 0 {
		slog.Info("ingest leader: no new files to enqueue")
		return 0, nil
	}

	args := make([]interface{}, len(toProcess))
	for i, p := range toProcess {
		args[i] = p
	}
	if err := kv.LPush(ctx, kvkeys.IngestWorkQueue(), args...).Err(); err != nil {
		return 0, fmt.Errorf("enqueue paths: %w", err)
	}
	slog.Info("ingest leader: enqueued paths for workers", "count", len(toProcess))
	return len(toProcess), nil
}

// RunLeader performs leader coordination: initial scan-and-enqueue followed by
// an optional watch loop that forwards new/changed file paths to the work queue.
// Returns when ctx is cancelled or (non-watch mode) after the initial scan.
func (g *Ingester) RunLeader(ctx context.Context, kv *redis.Client) error {
	n, err := g.ScanAndEnqueue(ctx, kv)
	if err != nil {
		return err
	}
	slog.Info("ingest leader: initial scan complete", "enqueued", n)

	// Wait for workers to drain the queue, then compute similarity.
	if g.cfg.ComputeSimilarity && n > 0 {
		go func() {
			if err := g.waitForQueueDrain(ctx, kv); err != nil {
				slog.Warn("ingest leader: queue drain wait aborted", "err", err)
				return
			}
			// Pass nil newTrackIDs — we don't track which IDs workers processed,
			// so runSimilarity will check HasSimilarityData and do a full or
			// incremental recompute as appropriate.
			if err := g.runSimilarity(ctx, nil); err != nil {
				slog.Error("similarity computation failed", "err", err)
			}
		}()
	}

	enqueue := func(path string) {
		if err := kv.LPush(ctx, kvkeys.IngestWorkQueue(), path).Err(); err != nil {
			slog.Warn("ingest leader: enqueue failed", "path", path, "err", err)
		}
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
		return g.watchWithPollingLeader(ctx, kv)
	}
	defer func() {
		if err := watcher.Close(); err != nil {
			slog.Warn("ingest: leader watcher close failed", "err", err)
		}
	}()

	for _, dir := range g.cfg.Dirs {
		dir = strings.TrimSpace(dir)
		if dir == "" {
			continue
		}
		_ = filepath.WalkDir(dir, func(path string, d os.DirEntry, e error) error {
			if e == nil && d.IsDir() {
				if path != dir && g.isDirExcluded(path) {
					return filepath.SkipDir
				}
				_ = watcher.Add(path)
			}
			return nil
		})
	}
	slog.Info("ingest leader: watching for changes", "dirs", g.cfg.Dirs)

	pollTicker := time.NewTicker(g.cfg.PollInterval)
	defer pollTicker.Stop()

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
				if g.isDirExcluded(ev.Name) {
					continue
				}
				_ = watcher.Add(ev.Name)
				go func(p string) {
					_ = filepath.WalkDir(p, func(path string, d os.DirEntry, e error) error {
						if e != nil {
							return nil
						}
						if d.IsDir() {
							if path != p && g.isDirExcluded(path) {
								return filepath.SkipDir
							}
							return nil
						}
						if isAudioFile(path) {
							enqueue(path)
						}
						return nil
					})
				}(ev.Name)
				continue
			}
			if !isAudioFile(ev.Name) {
				continue
			}
			go enqueue(ev.Name)
		case err, ok := <-watcher.Errors:
			if !ok {
				return nil
			}
			slog.Warn("watcher error", "err", err)
		case <-pollTicker.C:
			// Periodic safety-net scan: catches new files on filesystems where
			// fsnotify succeeds without error but silently delivers no events
			// (e.g. NTFS/CIFS mounts in WSL2, some network filesystems).
			go func() {
				if n, err := g.ScanAndEnqueue(ctx, kv); err != nil {
					slog.Error("ingest leader: poll enqueue failed", "err", err)
				} else if n > 0 {
					slog.Info("ingest leader: poll safety-net enqueued", "count", n)
				}
			}()
		}
	}
}

func (g *Ingester) watchWithPollingLeader(ctx context.Context, kv *redis.Client) error {
	slog.Warn("leader polling fallback active", "interval", g.cfg.PollInterval, "dirs", g.cfg.Dirs)
	ticker := time.NewTicker(g.cfg.PollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			n, err := g.ScanAndEnqueue(ctx, kv)
			if err != nil {
				slog.Error("ingest leader: poll enqueue failed", "err", err)
			} else if n > 0 {
				slog.Info("ingest leader: poll enqueued", "count", n)
			}
		}
	}
}

// waitForQueueDrain polls the Redis work queue until it is empty, then returns.
// This lets the leader know when all enqueued files have been processed by workers.
func (g *Ingester) waitForQueueDrain(ctx context.Context, kv *redis.Client) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(3 * time.Second):
			n, err := kv.LLen(ctx, kvkeys.IngestWorkQueue()).Result()
			if err != nil {
				return fmt.Errorf("check queue length: %w", err)
			}
			if n == 0 {
				slog.Info("ingest leader: work queue drained, proceeding with similarity")
				return nil
			}
			slog.Debug("ingest leader: waiting for queue drain", "remaining", n)
		}
	}
}

// RunWorkers starts n goroutines that consume file paths from the Redis work
// queue and process them independently. Blocks until ctx is cancelled.
func (g *Ingester) RunWorkers(ctx context.Context, kv *redis.Client, n int) {
	if n < 1 {
		n = 1
	}
	slog.Info("ingest: starting distributed workers", "count", n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			g.workerLoop(ctx, kv)
		}()
	}
	wg.Wait()
}

func (g *Ingester) workerLoop(ctx context.Context, kv *redis.Client) {
	for {
		if ctx.Err() != nil {
			return
		}
		// BRPop blocks up to 5s waiting for a job; redis.Nil means timeout (queue empty).
		result, err := kv.BRPop(ctx, 5*time.Second, kvkeys.IngestWorkQueue()).Result()
		if err != nil {
			if errors.Is(err, redis.Nil) {
				continue
			}
			if ctx.Err() != nil {
				return
			}
			slog.Warn("ingest worker: queue pop error", "err", err)
			time.Sleep(time.Second)
			continue
		}

		path := result[1]
		fi, err := os.Stat(path)
		if err != nil {
			slog.Warn("ingest worker: stat failed", "path", path, "err", err)
			continue
		}
		if g.cfg.DryRun {
			slog.Info("ingest worker: would process (dry run)", "path", path)
			continue
		}
		// Re-check upToDate: the leader may have re-enqueued a file that a
		// concurrent worker (or a previous poll cycle) already finished.
		if g.upToDate(path, fi) {
			slog.Debug("ingest worker: already done, skipping", "path", path)
			continue
		}
		// Skip unstable files (in-progress downloads).
		if !g.isStable(fi) {
			slog.Debug("ingest worker: skipping unstable file (in-progress download?)", "path", path, "mtime_age_sec", int(time.Since(fi.ModTime()).Seconds()))
			// Re-enqueue for later processing.
			if err := g.kv.LPush(ctx, kvkeys.IngestWorkQueue(), path).Err(); err != nil {
				slog.Warn("enqueue ingest path failed", "action", "enqueue ingest path", "err", err)
			}
			continue
		}
		trackID, err := g.ingestFile(ctx, path, fi)
		if err != nil {
			slog.Error("ingest worker: failed", "path", path, "err", err)
			continue
		}
		g.markDone(ctx, path, fi, trackID)
		slog.Info("ingest worker: ingested", "path", path, "track_id", trackID)
	}
}

// generateWaveformPeaks runs audiowaveform on path and returns a normalised
// []float32 (0–1) suitable for waveform rendering. Returns nil when
// audiowaveform is not installed or fails — callers must treat nil as "no data".
func generateWaveformPeaks(path string) []float32 {
	if _, err := exec.LookPath("audiowaveform"); err != nil {
		return nil
	}
	// 4 px/s → ~240 data points for a 60-minute track; fast and compact.
	out, err := exec.Command(
		"audiowaveform", "-i", path,
		"--output-format", "json",
		"--pixels-per-second", "4",
		"--bits", "8",
		"-o", "-",
	).Output()
	if err != nil {
		return nil
	}
	var result struct {
		Data []int `json:"data"` // interleaved (min, max) pairs, 8-bit signed range
	}
	if err := json.Unmarshal(out, &result); err != nil || len(result.Data) < 2 {
		return nil
	}
	n := len(result.Data) / 2
	peaks := make([]float32, n)
	var globalMax float32
	for i := 0; i < n; i++ {
		// Each pair is (min, max); take the larger absolute value as amplitude.
		minAbs := float32(-result.Data[i*2])  // min is negative → negate
		maxAbs := float32(result.Data[i*2+1]) // max is positive
		amp := minAbs
		if maxAbs > amp {
			amp = maxAbs
		}
		peaks[i] = amp
		if amp > globalMax {
			globalMax = amp
		}
	}
	if globalMax > 0 {
		for i := range peaks {
			peaks[i] /= globalMax
		}
	}
	return peaks
}
