package main

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
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/alexander-bruun/orb/pkg/config"
	"github.com/alexander-bruun/orb/pkg/musicbrainz"
	"github.com/alexander-bruun/orb/pkg/objstore"
	"github.com/alexander-bruun/orb/pkg/store"
	"github.com/dhowden/tag"
	"github.com/fsnotify/fsnotify"
	"github.com/spf13/cobra"
)

var ErrSkipped = errors.New("skipped")

// ---------------------------------------------------------------------------
// CLI flags
// ---------------------------------------------------------------------------

var (
	flagDir       string
	flagDB        string
	flagBackend   string
	flagStoreRoot string
	flagBucket    string
	flagS3Ep      string
	flagS3Key     string
	flagS3Secret  string
	flagUserID    string
	flagRecursive bool
	flagDryRun    bool
	flagWatch   bool
	flagWorkers int
)

var rootCmd = &cobra.Command{
	Use:   "orb-ingest",
	Short: "Index a music directory into the Orb database",
	RunE:  run,
}

func init() {
	rootCmd.Flags().StringVar(&flagDir, "dir", config.Env("INGEST_DIR", "/music"), "Music directory to scan")
	rootCmd.Flags().StringVar(&flagDB, "db", config.DSN(), "Postgres DSN")
	rootCmd.Flags().StringVar(&flagBackend, "store-backend", config.Env("STORE_BACKEND", "local"), "Storage backend: local | s3")
	rootCmd.Flags().StringVar(&flagStoreRoot, "store-root", config.Env("STORE_ROOT", "./data/audio"), "Root path for local backend")
	rootCmd.Flags().StringVar(&flagBucket, "store-bucket", config.Env("STORE_BUCKET", "orb-audio"), "S3 bucket name")
	rootCmd.Flags().StringVar(&flagS3Ep, "s3-endpoint", config.Env("S3_ENDPOINT", "http://localhost:9000"), "S3 endpoint")
	rootCmd.Flags().StringVar(&flagS3Key, "s3-access-key", config.Env("S3_ACCESS_KEY", "orb"), "S3 access key")
	rootCmd.Flags().StringVar(&flagS3Secret, "s3-secret-key", config.Env("S3_SECRET_KEY", "orbsecret"), "S3 secret key")
	rootCmd.Flags().StringVar(&flagUserID, "user-id", "", "Assign ingested tracks to this user's library")
	rootCmd.Flags().BoolVar(&flagRecursive, "recursive", false, "Scan subdirectories recursively")
	rootCmd.Flags().BoolVar(&flagDryRun, "dry-run", false, "Print what would be done without modifying anything")
	rootCmd.Flags().BoolVar(&flagWatch, "watch", false, "Watch directory for new files and ingest continuously")
	rootCmd.Flags().IntVar(&flagWorkers, "workers", runtime.NumCPU(), "Number of parallel ingest workers")

}

func main() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}


// ---------------------------------------------------------------------------
// ingester: holds all runtime state for an ingest session
// ---------------------------------------------------------------------------

// ingestEntry is the in-memory record for a file that has been processed.
// Only mtime+size are kept in memory; track_id lives in the DB only.
type ingestEntry struct {
	mtimeUnix int64
	fileSize  int64
}

// ingester holds shared state used across the initial scan and the optional watcher.
type ingester struct {
	db     *store.Store
	obj    objstore.ObjectStore
	userID string
	dryRun bool

	// stateMu guards state. The watcher fires goroutines, so concurrent access
	// to the map is possible in --watch mode.
	stateMu sync.RWMutex
	state   map[string]ingestEntry // absolute path → last-seen mtime+size

	// folderImgCache memoises bestFolderImage per directory so that every
	// track in a multi-track album doesn't re-read and re-decode the same
	// folder images.
	folderImgCache sync.Map // dir string → []byte

	// coveredAlbums tracks which album IDs have had their cover art stored in
	// this session, preventing redundant re-encoding and re-uploading.
	coveredAlbums sync.Map // albumID string → struct{}

	// MusicBrainz enrichment (nil when --enrich is false).
	mb              *musicbrainz.Client
	enrichedArtists sync.Map // artistID → struct{} — skip duplicate lookups
	enrichedAlbums  sync.Map // albumID → struct{}
}

// loadState performs one bulk SELECT to populate the in-memory state map.
// After this call there are zero per-file DB queries during the scan: each
// file is checked with an O(1) map lookup and a cheap os.Stat() syscall.
func (g *ingester) loadState(ctx context.Context) error {
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

// upToDate returns true when path has been ingested and its mtime+size are
// unchanged on disk — meaning we can skip it without reading the file at all.
func (g *ingester) upToDate(path string, fi os.FileInfo) bool {
	g.stateMu.RLock()
	e, ok := g.state[path]
	g.stateMu.RUnlock()
	return ok && e.mtimeUnix == fi.ModTime().Unix() && e.fileSize == fi.Size()
}

// markDone persists a successfully ingested file's state to Postgres and
// updates the in-memory map so subsequent files in the same run also skip it.
func (g *ingester) markDone(ctx context.Context, path string, fi os.FileInfo, trackID string) {
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

// process handles one audio file. It is safe to call concurrently.
// Returns ErrSkipped when the file is already up-to-date.
func (g *ingester) process(ctx context.Context, path string) error {
	fi, err := os.Stat(path)
	if err != nil {
		return err
	}

	// Fast path: stat only — no file read, no hash, no DB query.
	if g.upToDate(path, fi) {
		return ErrSkipped
	}

	if g.dryRun {
		slog.Info("would ingest", "path", path)
		return nil
	}

	trackID, err := g.ingestFile(ctx, path, fi)
	if err != nil {
		return err
	}

	g.markDone(ctx, path, fi, trackID)
	slog.Info("ingested", "path", path, "track_id", trackID)
	return nil
}

// scan walks flagDir, calling process on each audio file, and returns counts.
// Files are processed concurrently using up to flagWorkers goroutines.
func (g *ingester) scan(ctx context.Context) (ingested, skipped, errs int) {
	// Collect all paths first (pure directory walk — no file I/O beyond stat).
	var paths []string
	if err := filepath.WalkDir(flagDir, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			slog.Warn("walk error", "path", path, "err", walkErr)
			return nil
		}
		if d.IsDir() {
			if !flagRecursive && path != flagDir {
				return filepath.SkipDir
			}
			return nil
		}
		if isAudioFile(path) {
			paths = append(paths, path)
		}
		return nil
	}); err != nil {
		slog.Warn("walk error", "dir", flagDir, "err", err)
	}

	// Fan out to a bounded worker pool.
	var nIngested, nSkipped, nErrs int64

	workers := flagWorkers
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
				switch err := g.process(ctx, p); {
				case errors.Is(err, ErrSkipped):
					atomic.AddInt64(&nSkipped, 1)
				case err != nil:
					slog.Error("ingest failed", "path", p, "err", err)
					atomic.AddInt64(&nErrs, 1)
				default:
					atomic.AddInt64(&nIngested, 1)
				}
			}
		}()
	}
	for _, p := range paths {
		pathCh <- p
	}
	close(pathCh)
	wg.Wait()

	return int(nIngested), int(nSkipped), int(nErrs)
}

// cachedFolderImage returns the best folder image for dir, memoising the result
// so that multiple tracks in the same album directory only do the I/O once.
func (g *ingester) cachedFolderImage(dir string) []byte {
	if v, ok := g.folderImgCache.Load(dir); ok {
		return v.([]byte)
	}
	data := bestFolderImage(dir)
	// LoadOrStore: if another goroutine raced and stored first, use its result.
	if actual, loaded := g.folderImgCache.LoadOrStore(dir, data); loaded {
		return actual.([]byte)
	}
	return data
}

// ---------------------------------------------------------------------------
// run: entry point after flag parsing
// ---------------------------------------------------------------------------

func run(cmd *cobra.Command, _ []string) error {
	ctx := context.Background()

	db, err := store.Connect(ctx, flagDB)
	if err != nil {
		return fmt.Errorf("connect store: %w", err)
	}
	defer db.Close()

	if flagUserID != "" {
		if _, err := db.GetUserByID(ctx, flagUserID); err != nil {
			slog.Warn("user for --user-id not found; disabling library adds", "user_id", flagUserID, "err", err)
			flagUserID = ""
		}
	}

	var obj objstore.ObjectStore
	switch flagBackend {
	case "local":
		obj, err = objstore.NewLocalFS(flagStoreRoot)
		if err != nil {
			return fmt.Errorf("local store: %w", err)
		}
	case "s3":
		obj, err = objstore.NewS3(ctx, objstore.S3Config{
			Endpoint:  flagS3Ep,
			AccessKey: flagS3Key,
			SecretKey: flagS3Secret,
			Bucket:    flagBucket,
		})
		if err != nil {
			return fmt.Errorf("s3 store: %w", err)
		}
	default:
		return fmt.Errorf("unknown store backend %q", flagBackend)
	}

	if flagDir == "" {
		return fmt.Errorf("--dir is required")
	}

	g := &ingester{
		db:     db,
		obj:    obj,
		userID: flagUserID,
		dryRun: flagDryRun,
	}
	g.mb = musicbrainz.New()
	slog.Info("MusicBrainz enrichment enabled")

	// One bulk load — no per-file DB queries during the scan.
	if err := g.loadState(ctx); err != nil {
		return err
	}

	if !flagWatch {
		ingested, skipped, errs := g.scan(ctx)
		slog.Info("ingest complete", "ingested", ingested, "skipped", skipped, "errors", errs)
		return nil
	}

	// Watch mode: initial full scan, then listen for filesystem events.
	ingested, skipped, errs := g.scan(ctx)
	slog.Info("initial scan complete", "ingested", ingested, "skipped", skipped, "errors", errs)

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("create watcher: %w", err)
	}
	defer watcher.Close()

	// Register watchers for all existing directories under flagDir.
	_ = filepath.WalkDir(flagDir, func(path string, d os.DirEntry, e error) error {
		if e == nil && d.IsDir() {
			_ = watcher.Add(path)
		}
		return nil
	})

	slog.Info("watching", "dir", flagDir)

	for {
		select {
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
							if err := g.process(ctx, path); err != nil && !errors.Is(err, ErrSkipped) {
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
				if err := g.process(ctx, p); err != nil && !errors.Is(err, ErrSkipped) {
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

// ---------------------------------------------------------------------------
// ingestFile: hash, tag, upsert, copy — only called for new/changed files
// ---------------------------------------------------------------------------

// ingestFile reads the file at path, extracts metadata, upserts artist/album/track
// records in Postgres, copies the audio blob to the object store, and returns
// the deterministic track ID. fi is the already-stat'd FileInfo for the file.
func (g *ingester) ingestFile(ctx context.Context, path string, fi os.FileInfo) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	// SHA-256 fingerprint — stable across renames, used as the track's dedup key.
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", fmt.Errorf("hash: %w", err)
	}
	fingerprint := hex.EncodeToString(h.Sum(nil))
	trackID := deterministicUUID(fingerprint)

	// Rewind to read tags (the hash consumed the reader).
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return "", err
	}
	m, err := tag.ReadFrom(f)
	if err != nil {
		return "", fmt.Errorf("read tags: %w", err)
	}

	// AlbumArtist is the canonical grouping key; fall back to track Artist.
	albumArtistName := coalesce(m.AlbumArtist(), m.Artist(), "Unknown Artist")
	trackArtistName := coalesce(m.Artist(), albumArtistName)

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
	albumID := deterministicID("album:" + strings.ToLower(albumArtistName) + ":" + strings.ToLower(albumTitle))

	// Cover art: only process once per album per session.
	coverKey := fmt.Sprintf("covers/%s.jpg", albumID)
	var coverArtKeyPtr *string
	if _, done := g.coveredAlbums.Load(albumID); done {
		// Already stored this session — reuse the key without re-encoding.
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
		} else {
			slog.Debug("no cover art", "album", albumTitle, "path", path)
		}
	}

	var releaseYearPtr *int
	if y := m.Year(); y > 0 {
		releaseYearPtr = &y
	}

	if _, err = g.db.UpsertAlbum(ctx, store.UpsertAlbumParams{
		ID:          albumID,
		ArtistID:    &albumArtistID,
		Title:       albumTitle,
		ReleaseYear: releaseYearPtr,
		CoverArtKey: coverArtKeyPtr,
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

	track, err := g.db.UpsertTrack(ctx, store.UpsertTrackParams{
		ID:          trackID,
		AlbumID:     &albumID,
		ArtistID:    &trackArtistID,
		Title:       coalesce(m.Title(), filepath.Base(path)),
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

	// Copy audio blob only if not already present (idempotent re-runs).
	if exists, _ := g.obj.Exists(ctx, fileKey); !exists {
		if _, err := f.Seek(0, io.SeekStart); err != nil {
			return "", err
		}
		if err := g.obj.Put(ctx, fileKey, f, fi.Size()); err != nil {
			return "", fmt.Errorf("store audio: %w", err)
		}
	}

	if g.userID != "" {
		if err := g.db.AddTrackToLibrary(ctx, store.AddTrackToLibraryParams{
			UserID:  g.userID,
			TrackID: track.ID,
		}); err != nil {
			slog.Warn("add to library failed", "track_id", track.ID, "err", err)
		}
	}

	// MusicBrainz enrichment (artist + album only; rate-limited, best-effort).
	if g.mb != nil {
		g.enrichAfterIngest(ctx, albumArtistID, albumArtistName, albumID, albumTitle, coverArtKeyPtr == nil)
	}

	return trackID, nil
}

// enrichAfterIngest enriches artist and album metadata from MusicBrainz.
// Lookups are deduplicated per session via sync.Maps so each artist/album
// is only queried once regardless of how many tracks belong to it.
func (g *ingester) enrichAfterIngest(ctx context.Context, artistID, artistName, albumID, albumTitle string, missingCoverArt bool) {
	// Artist: enrich once per artist per session.
	if _, done := g.enrichedArtists.LoadOrStore(artistID, struct{}{}); !done {
		result, err := g.mb.EnrichArtist(ctx, artistName)
		if err != nil {
			slog.Warn("enrich artist failed", "artist", artistName, "err", err)
		} else if result != nil {
			// Fetch and store artist image from Wikidata/Wikimedia Commons.
			var imageKeyPtr *string
			if imgData, imgErr := g.mb.FetchArtistImage(ctx, result.URLRelations); imgErr != nil {
				slog.Warn("fetch artist image failed", "artist", artistName, "err", imgErr)
			} else if imgData != nil {
				imageKey := fmt.Sprintf("artists/%s.jpg", artistID)
				if err := storeCoverArt(ctx, g.obj, imageKey, imgData); err != nil {
					slog.Warn("store artist image failed", "artist", artistName, "err", err)
				} else {
					imageKeyPtr = &imageKey
					slog.Info("stored artist image", "artist", artistName, "key", imageKey)
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

	// Album: enrich once per album per session.
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

			// Fetch cover art from Cover Art Archive when no local art was found.
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
						} else {
							slog.Info("stored album cover art from CAA", "album", albumTitle, "key", coverKey)
						}
					}
				}
			}
		}
	}
}

// persistGenres upserts genre records and calls setFn with the resulting IDs.
func (g *ingester) persistGenres(ctx context.Context, names []string, setFn func([]string)) {
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

// strPtr returns a pointer to s, or nil if s is empty.
func strPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func isAudioFile(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".flac", ".wav", ".mp3", ".aiff", ".aif":
		return true
	}
	return false
}

// bestFolderImage scans dir for image files and returns the bytes of the one
// closest to square (preferred for album art). Returns nil if none found.
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
		// Already a JPEG or undecodable — store raw.
		return obj.Put(ctx, key, bytes.NewReader(data), int64(len(data)))
	}
	pr, pw := io.Pipe()
	go func() {
		pw.CloseWithError(jpeg.Encode(pw, img, &jpeg.Options{Quality: 90}))
	}()
	defer pr.Close()
	return obj.Put(ctx, key, pr, -1)
}

// readFLACInfo reads the FLAC STREAMINFO block for bit depth, sample rate, and
// duration using the already-open file f, avoiding a redundant os.Open call.
// ext should be the lowercased extension without the dot (e.g. "flac").
// Returns zeros for non-FLAC files or unparseable headers.
func readFLACInfo(f *os.File, ext string) (bitDepth, sampleRate int, durationMs int64) {
	if ext != "flac" {
		return
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return
	}
	// 4-byte "fLaC" marker + 4-byte block header + 34-byte STREAMINFO = 42 bytes.
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
	si := buf[8:] // 34-byte STREAMINFO payload
	// Bit layout (FLAC spec, big-endian):
	//   bits  80–99:  sample rate (20 bits)
	//   bits 103–107: bits per sample – 1 (5 bits)
	//   bits 108–143: total samples (36 bits)
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

// deterministicID returns a 16-hex-char ID stable for the given seed string.
func deterministicID(seed string) string {
	h := sha256.Sum256([]byte(seed))
	return hex.EncodeToString(h[:8])
}

// deterministicUUID returns a UUID v4-format string derived from a fingerprint.
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
