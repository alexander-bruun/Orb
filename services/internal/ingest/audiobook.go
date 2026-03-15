package ingest

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/alexander-bruun/orb/services/internal/objstore"
	"github.com/alexander-bruun/orb/services/internal/openlibrary"
	"github.com/alexander-bruun/orb/services/internal/store"
	"github.com/dhowden/tag"
)

// AudiobookConfig holds configuration specific to audiobook ingest.
type AudiobookConfig struct {
	// Dirs is the list of directories to scan for audiobook files.
	Dirs []string
	// ExcludeGlobs are glob patterns for paths to skip.
	ExcludeGlobs []string
	// Workers is the number of parallel file-processing goroutines.
	Workers int
	// Enrich enables Open Library metadata fetching.
	Enrich bool
	// PollInterval is the interval between automatic re-scans.
	PollInterval time.Duration
}

// AudiobookIngester processes audiobook files and persists them to the store.
type AudiobookIngester struct {
	db  *store.Store
	obj objstore.ObjectStore
	cfg AudiobookConfig

	stateMu sync.RWMutex
	state   map[string]ingestEntry

	folderImgCache sync.Map
	coveredBooks   sync.Map
	enrichedBooks  sync.Map

	ol *openlibrary.Client
}

// NewAudiobookIngester creates a new AudiobookIngester.
func NewAudiobookIngester(db *store.Store, obj objstore.ObjectStore, cfg AudiobookConfig) *AudiobookIngester {
	g := &AudiobookIngester{
		db:    db,
		obj:   obj,
		cfg:   cfg,
		state: make(map[string]ingestEntry),
	}
	if cfg.Enrich {
		g.ol = openlibrary.New()
	}
	return g
}

// isAudiobookFile returns true for file extensions commonly used for audiobooks.
func isAudiobookFile(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".m4b", ".m4a", ".mp3":
		return true
	}
	return false
}

// isSingleFileAudiobook returns true for container formats that carry chapters
// internally (M4B/M4A). MP3 is multi-file only.
func isSingleFileAudiobook(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".m4b", ".m4a":
		return true
	}
	return false
}

func (g *AudiobookIngester) loadState(ctx context.Context) error {
	rows, err := g.db.LoadAudiobookIngestState(ctx)
	if err != nil {
		return fmt.Errorf("load audiobook ingest state: %w", err)
	}
	g.state = make(map[string]ingestEntry, len(rows))
	for _, r := range rows {
		g.state[r.Path] = ingestEntry{mtimeUnix: r.MtimeUnix, fileSize: r.FileSize}
	}
	slog.Info("loaded audiobook ingest state", "known_files", len(g.state))
	return nil
}

func (g *AudiobookIngester) upToDate(path string, fi os.FileInfo) bool {
	g.stateMu.RLock()
	e, ok := g.state[path]
	g.stateMu.RUnlock()
	return ok && e.mtimeUnix == fi.ModTime().Unix() && e.fileSize == fi.Size()
}

func (g *AudiobookIngester) upToDateDir(dirPath string, totalSize, maxMtime int64) bool {
	g.stateMu.RLock()
	e, ok := g.state[dirPath]
	g.stateMu.RUnlock()
	return ok && e.mtimeUnix == maxMtime && e.fileSize == totalSize
}

func (g *AudiobookIngester) markDone(ctx context.Context, path string, fi os.FileInfo, id string) {
	if err := g.db.UpsertAudiobookIngestState(ctx, store.AudiobookIngestStateRow{
		Path:        path,
		MtimeUnix:   fi.ModTime().Unix(),
		FileSize:    fi.Size(),
		AudiobookID: id,
	}); err != nil {
		slog.Warn("persist audiobook ingest state failed", "path", path, "err", err)
	}
	g.stateMu.Lock()
	g.state[path] = ingestEntry{mtimeUnix: fi.ModTime().Unix(), fileSize: fi.Size()}
	g.stateMu.Unlock()
}

func (g *AudiobookIngester) markDoneDir(ctx context.Context, dirPath string, totalSize, maxMtime int64, id string) {
	if err := g.db.UpsertAudiobookIngestState(ctx, store.AudiobookIngestStateRow{
		Path:        dirPath,
		MtimeUnix:   maxMtime,
		FileSize:    totalSize,
		AudiobookID: id,
	}); err != nil {
		slog.Warn("persist audiobook ingest state failed", "path", dirPath, "err", err)
	}
	g.stateMu.Lock()
	g.state[dirPath] = ingestEntry{mtimeUnix: maxMtime, fileSize: totalSize}
	g.stateMu.Unlock()
}

func (g *AudiobookIngester) isDirExcluded(dirPath string) bool {
	for _, glob := range g.cfg.ExcludeGlobs {
		if matchExcludeGlob(glob, dirPath) {
			return true
		}
	}
	return false
}

// ClearState wipes in-memory state so the next scan re-processes all files.
func (g *AudiobookIngester) ClearState() {
	g.stateMu.Lock()
	g.state = make(map[string]ingestEntry)
	g.stateMu.Unlock()
}

// ── audiobookCandidate ────────────────────────────────────────────────────────

// audiobookCandidate represents a leaf directory (or single file) to be ingested.
type audiobookCandidate struct {
	// dirPath is the directory that contains the audio files.
	dirPath string
	// files is the sorted list of audio file paths inside the directory.
	files []string
	// singleFile is true when there is exactly one M4B/M4A file and no MP3s,
	// meaning it should be handled by ingestFile (single-file mode).
	singleFile bool
}

// scanDir recursively finds leaf directories containing audiobook files and
// sends them as candidates. A "leaf" directory is one where audio files were
// found directly (not only in sub-directories).
func (g *AudiobookIngester) scanDir(ctx context.Context, root string, results chan<- audiobookCandidate) {
	entries, err := os.ReadDir(root)
	if err != nil {
		slog.Warn("audiobook scan: read dir failed", "dir", root, "err", err)
		return
	}

	var audioFiles []string
	var hasMp3 bool
	var hasM4 bool
	var subDirs []string

	for _, e := range entries {
		if e.IsDir() {
			sub := filepath.Join(root, e.Name())
			if !g.isDirExcluded(sub) {
				subDirs = append(subDirs, sub)
			}
			continue
		}
		if isAudiobookFile(e.Name()) {
			p := filepath.Join(root, e.Name())
			audioFiles = append(audioFiles, p)
			ext := strings.ToLower(filepath.Ext(e.Name()))
			if ext == ".mp3" {
				hasMp3 = true
			} else {
				hasM4 = true
			}
		}
	}

	// Recurse into sub-directories first so deeper books are found.
	for _, sub := range subDirs {
		g.scanDir(ctx, sub, results)
	}

	if len(audioFiles) == 0 {
		return
	}

	// Decide mode:
	// - Single-file: exactly one M4B/M4A, no MP3s → reuse ingestFile.
	// - Directory mode: multiple files OR any MP3s.
	if !hasMp3 && hasM4 && len(audioFiles) == 1 {
		results <- audiobookCandidate{
			dirPath:    root,
			files:      audioFiles,
			singleFile: true,
		}
	} else {
		sort.Strings(audioFiles) // natural pre-sort; ingestDirectory will re-sort by track tag
		results <- audiobookCandidate{
			dirPath:    root,
			files:      audioFiles,
			singleFile: false,
		}
	}
}

// Scan walks all configured dirs and processes audiobook files.
func (g *AudiobookIngester) Scan(ctx context.Context) (newIDs []string, skipped, errs int) {
	if err := g.loadState(ctx); err != nil {
		slog.Error("audiobook ingest: load state", "err", err)
		return nil, 0, 1
	}

	// Collect candidates via channel so workers can start while scanning.
	candidateCh := make(chan audiobookCandidate, 64)
	go func() {
		defer close(candidateCh)
		for _, dir := range g.cfg.Dirs {
			dir = strings.TrimSpace(dir)
			if dir == "" {
				continue
			}
			if g.isDirExcluded(dir) {
				continue
			}
			g.scanDir(ctx, dir, candidateCh)
		}
	}()

	var nDone, nSkipped, nErrs int64
	var mu sync.Mutex
	var ids []string

	workers := g.cfg.Workers
	if workers < 1 {
		workers = 1
	}

	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for cand := range candidateCh {
				if cand.singleFile {
					p := cand.files[0]
					fi, err := os.Stat(p)
					if err != nil {
						atomic.AddInt64(&nErrs, 1)
						atomic.AddInt64(&nDone, 1)
						continue
					}
					if g.upToDate(p, fi) {
						atomic.AddInt64(&nSkipped, 1)
						atomic.AddInt64(&nDone, 1)
						continue
					}
					id, err := g.ingestFile(ctx, p, fi)
					if err != nil {
						if errors.Is(err, ErrSkipped) {
							atomic.AddInt64(&nSkipped, 1)
						} else {
							slog.Error("audiobook ingest failed", "path", p, "err", err)
							atomic.AddInt64(&nErrs, 1)
						}
					} else if id != "" {
						g.markDone(ctx, p, fi, id)
						mu.Lock()
						ids = append(ids, id)
						mu.Unlock()
						slog.Info("audiobook ingested", "path", p, "audiobook_id", id)
					}
				} else {
					// Directory mode: compute aggregate size/mtime for change detection.
					var totalSize, maxMtime int64
					allValid := true
					for _, p := range cand.files {
						fi, err := os.Stat(p)
						if err != nil {
							slog.Warn("audiobook dir stat failed", "path", p, "err", err)
							allValid = false
							break
						}
						totalSize += fi.Size()
						if m := fi.ModTime().Unix(); m > maxMtime {
							maxMtime = m
						}
					}
					if !allValid {
						atomic.AddInt64(&nErrs, 1)
						atomic.AddInt64(&nDone, 1)
						continue
					}
					if g.upToDateDir(cand.dirPath, totalSize, maxMtime) {
						atomic.AddInt64(&nSkipped, 1)
						atomic.AddInt64(&nDone, 1)
						continue
					}
					id, err := g.ingestDirectory(ctx, cand)
					if err != nil {
						if errors.Is(err, ErrSkipped) {
							atomic.AddInt64(&nSkipped, 1)
						} else {
							slog.Error("audiobook dir ingest failed", "dir", cand.dirPath, "err", err)
							atomic.AddInt64(&nErrs, 1)
						}
					} else if id != "" {
						g.markDoneDir(ctx, cand.dirPath, totalSize, maxMtime, id)
						mu.Lock()
						ids = append(ids, id)
						mu.Unlock()
						slog.Info("audiobook directory ingested", "dir", cand.dirPath, "audiobook_id", id)
					}
				}
				atomic.AddInt64(&nDone, 1)
			}
		}()
	}
	wg.Wait()

	return ids, int(nSkipped), int(nErrs)
}

// probeFileDuration returns the duration of an audio file in milliseconds
// using pure-Go parsers — no external tools required.
func probeFileDuration(path string) (int64, error) {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".mp3":
		return mp3DurationMs(path), nil
	case ".m4b", ".m4a":
		info, err := probeM4B(path)
		if err != nil {
			return 0, err
		}
		return info.durationMs, nil
	}
	return 0, nil
}

// ── Directory name parsing helpers ───────────────────────────────────────────

var (
	reAuthorInDir   = regexp.MustCompile(`\(by ([^)]+)\)`)
	reNarratorInDir = regexp.MustCompile(`\(read by ([^)]+)\)`)
	reBookDirName   = regexp.MustCompile(`(?i)^Book\s+(\d+(?:\.\d+)?)\s+-\s+(.+)$`)
)

// parseSeriesDirName extracts series name, author and narrator from a directory
// name. Supported formats:
//
//	"Harry Potter (by J.K. Rowling) - The Complete Story (read by Stephen Fry) [V0]"
//	"James S. A. Corey - The Expanse Series"
//
// Any field may be empty if not found.
func parseSeriesDirName(name string) (series, author, narrator string) {
	if m := reAuthorInDir.FindStringSubmatchIndex(name); m != nil {
		author = strings.TrimSpace(name[m[2]:m[3]])
		// Series is everything before the first "(by …)" occurrence.
		series = strings.TrimSpace(name[:m[0]])
		// Strip trailing " - " or similar separators from series name.
		series = strings.TrimRight(series, " \t-–")
	}
	if m := reNarratorInDir.FindStringSubmatch(name); m != nil {
		narrator = strings.TrimSpace(m[1])
	}
	// Fallback: "Author - Series" style (no parenthetical markers).
	if author == "" && series == "" {
		if idx := strings.Index(name, " - "); idx > 0 {
			author = strings.TrimSpace(name[:idx])
			series = strings.TrimSpace(name[idx+3:])
		}
	}
	return series, author, narrator
}

// parseBookDirName extracts a title and optional series index from a directory
// name that follows the pattern:
//
//	"Book 1 - Harry Potter and the Philosopher's Stone"
//
// Returns the title and a pointer to the parsed series index (nil if no match).
func parseBookDirName(name string) (title string, seriesIndex *float64) {
	if m := reBookDirName.FindStringSubmatch(name); m != nil {
		idx, err := strconv.ParseFloat(m[1], 64)
		if err == nil {
			seriesIndex = &idx
		}
		title = strings.TrimSpace(m[2])
		return title, seriesIndex
	}
	return name, nil
}

// ── ID3 track number helper ───────────────────────────────────────────────────

// parseTrackNum returns the numeric portion from a "track/total" or plain
// numeric tag value. Returns 0 if parsing fails.
func parseTrackNum(s string) int {
	s = strings.TrimSpace(s)
	if idx := strings.Index(s, "/"); idx >= 0 {
		s = s[:idx]
	}
	n, _ := strconv.Atoi(strings.TrimSpace(s))
	return n
}

// readTrackTag opens an audio file and reads its track number from ID3/iTunes
// tags. Returns 0 on failure.
func readTrackTag(path string) int {
	f, err := os.Open(path)
	if err != nil {
		return 0
	}
	defer f.Close()
	m, err := tag.ReadFrom(f)
	if err != nil {
		return 0
	}
	n, _ := m.Track()
	return n
}

// dirFingerprint returns a SHA-256 fingerprint for a directory-based audiobook
// by hashing the sorted "filename:size\n" for every chapter file.
func dirFingerprint(files []string) (string, error) {
	h := sha256.New()
	for _, p := range files {
		fi, err := os.Stat(p)
		if err != nil {
			return "", err
		}
		fmt.Fprintf(h, "%s:%d\n", filepath.Base(p), fi.Size())
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// ── Main ingest logic ─────────────────────────────────────────────────────────

func (g *AudiobookIngester) ingestFile(ctx context.Context, path string, fi os.FileInfo) (string, error) {
	// Hash the file for dedup.
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

	// If we've already ingested this exact file content, skip.
	existingID, lookupErr := g.db.GetAudiobookByFingerprint(ctx, fingerprint)
	if lookupErr == nil && existingID != "" {
		return existingID, ErrSkipped
	}

	audiobookID := deterministicUUID(fingerprint)

	// ── Read tags (title, author, etc.) ──────────────────────────────────────
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return "", err
	}
	m, tagErr := tag.ReadFrom(f)
	_ = f.Close()

	var title, authorName, narratorName string
	if tagErr == nil {
		title = m.Title()
		authorName = coalesce(m.AlbumArtist(), m.Artist())
		raw := m.Raw()
		// Common narrator tags: "narrator", "©nrt", "TXXX:NARRATOR"
		narratorName = rawTagString(raw,
			"narrator", "narrated_by", "©nrt",
			"TXXX:NARRATOR", "----:com.apple.iTunes:NARRATOR",
			"com.apple.iTunes:narrator",
		)
	}
	if title == "" {
		title = strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	}
	if authorName == "" {
		authorName = "Unknown Author"
	}

	// ── Duration and chapters via pure-Go M4B parser ──────────────────────
	var durationMs int64
	var rawChapters []m4bChapter
	if m4b, err := probeM4B(path); err != nil {
		slog.Warn("audiobook probe failed, duration unknown", "path", path, "err", err)
	} else {
		durationMs = m4b.durationMs
		rawChapters = m4b.chapters
	}

	// ── Upsert author (reuse artists table) ───────────────────────────────
	authorID := deterministicID("artist:" + strings.ToLower(authorName))
	if _, err := g.db.UpsertArtist(ctx, store.UpsertArtistParams{
		ID:       authorID,
		Name:     authorName,
		SortName: sortName(authorName),
	}); err != nil {
		return "", fmt.Errorf("upsert author: %w", err)
	}

	// ── Cover art ─────────────────────────────────────────────────────────
	coverKey := fmt.Sprintf("audiobook-covers/%s.jpg", audiobookID)
	var coverArtKeyPtr *string
	if _, done := g.coveredBooks.Load(audiobookID); done {
		coverArtKeyPtr = &coverKey
	} else {
		var picData []byte
		// Try tag-embedded art.
		if tagErr == nil {
			rf, err2 := os.Open(path)
			if err2 == nil {
				if m2, err2 := tag.ReadFrom(rf); err2 == nil {
					if pic := m2.Picture(); pic != nil && len(pic.Data) > 0 {
						picData = pic.Data
					}
				}
				rf.Close()
			}
		}
		if len(picData) == 0 {
			picData = g.cachedFolderImage(filepath.Dir(path))
		}
		if len(picData) > 0 {
			if err := storeAudiobookCoverArt(ctx, g.obj, coverKey, picData); err != nil {
				slog.Warn("audiobook cover art storage failed", "title", title, "err", err)
			} else {
				coverArtKeyPtr = &coverKey
				g.coveredBooks.Store(audiobookID, struct{}{})
			}
		}
	}

	// ── File storage ──────────────────────────────────────────────────────
	ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(path)), ".")
	fileKey := fmt.Sprintf("audiobooks/%s/%s.%s", authorID, audiobookID, ext)

	if exists, _ := g.obj.Exists(ctx, fileKey); !exists {
		rf, err2 := os.Open(path)
		if err2 != nil {
			return "", fmt.Errorf("re-open for storage: %w", err2)
		}
		if err2 := g.obj.Put(ctx, fileKey, rf, fi.Size()); err2 != nil {
			rf.Close()
			return "", fmt.Errorf("store audiobook: %w", err2)
		}
		rf.Close()
	}

	// ── Upsert audiobook ──────────────────────────────────────────────────
	_, err = g.db.UpsertAudiobook(ctx, store.UpsertAudiobookParams{
		ID:          audiobookID,
		Title:       title,
		AuthorID:    &authorID,
		CoverArtKey: coverArtKeyPtr,
		FileKey:     &fileKey,
		FileSize:    fi.Size(),
		Format:      ext,
		DurationMs:  durationMs,
		Fingerprint: fingerprint,
	})
	if err != nil {
		return "", fmt.Errorf("upsert audiobook: %w", err)
	}

	// ── Narrator ──────────────────────────────────────────────────────────
	if narratorName != "" {
		narratorID := deterministicID("narrator:" + strings.ToLower(narratorName))
		if err := g.db.UpsertAudiobookNarrator(ctx, store.UpsertAudiobookNarratorParams{
			ID:       narratorID,
			Name:     narratorName,
			SortName: sortName(narratorName),
		}); err != nil {
			slog.Warn("upsert narrator failed", "name", narratorName, "err", err)
		} else if err := g.db.SetAudiobookNarrators(ctx, audiobookID, []string{narratorID}); err != nil {
			slog.Warn("set audiobook narrators failed", "audiobook_id", audiobookID, "err", err)
		}
	}

	// ── Chapters ──────────────────────────────────────────────────────────
	if len(rawChapters) > 0 {
		chapters := make([]store.AudiobookChapter, 0, len(rawChapters))
		for i, rc := range rawChapters {
			chapTitle := rc.title
			if chapTitle == "" {
				chapTitle = fmt.Sprintf("Chapter %d", i+1)
			}
			startMs := rc.startMs
			var endMs int64
			if i+1 < len(rawChapters) {
				endMs = rawChapters[i+1].startMs
			} else {
				endMs = durationMs
			}
			chapID := deterministicID(fmt.Sprintf("chapter:%s:%d", audiobookID, i))
			chapters = append(chapters, store.AudiobookChapter{
				ID:          chapID,
				AudiobookID: audiobookID,
				Title:       chapTitle,
				StartMs:     startMs,
				EndMs:       endMs,
				ChapterNum:  i,
			})
		}
		if err := g.db.ReplaceAudiobookChapters(ctx, audiobookID, chapters); err != nil {
			slog.Warn("replace audiobook chapters failed", "audiobook_id", audiobookID, "err", err)
		}
	}

	// ── Open Library enrichment (best-effort, async) ──────────────────────
	if g.ol != nil {
		go g.enrichAudiobook(context.Background(), audiobookID, title, authorName, coverArtKeyPtr == nil)
	}

	return audiobookID, nil
}

// ingestDirectory handles a directory of audio files where each file is a chapter.
func (g *AudiobookIngester) ingestDirectory(ctx context.Context, cand audiobookCandidate) (string, error) {
	files := cand.files
	if len(files) == 0 {
		return "", fmt.Errorf("no audio files in directory: %s", cand.dirPath)
	}

	// ── Sort files by track number from tag, fallback to filename ─────────
	type fileWithTrack struct {
		path  string
		track int
	}
	tagged := make([]fileWithTrack, len(files))
	for i, p := range files {
		tagged[i] = fileWithTrack{path: p, track: readTrackTag(p)}
	}
	sort.SliceStable(tagged, func(i, j int) bool {
		ti, tj := tagged[i].track, tagged[j].track
		if ti != 0 && tj != 0 {
			return ti < tj
		}
		if ti != 0 {
			return true
		}
		if tj != 0 {
			return false
		}
		// Both zero: fall back to filename natural sort.
		return tagged[i].path < tagged[j].path
	})
	sortedFiles := make([]string, len(tagged))
	for i, ft := range tagged {
		sortedFiles[i] = ft.path
	}

	// ── Compute fingerprint for dedup ─────────────────────────────────────
	fingerprint, err := dirFingerprint(sortedFiles)
	if err != nil {
		return "", fmt.Errorf("fingerprint: %w", err)
	}

	existingID, lookupErr := g.db.GetAudiobookByFingerprint(ctx, fingerprint)
	if lookupErr == nil && existingID != "" {
		return existingID, ErrSkipped
	}

	audiobookID := deterministicUUID(fingerprint)

	// ── Read book-level metadata from first file ──────────────────────────
	firstFile := sortedFiles[0]
	var bookTitle, authorName, narratorName string
	var publishedYear *int

	if f, err := os.Open(firstFile); err == nil {
		if m, err := tag.ReadFrom(f); err == nil {
			// album → book title
			bookTitle = m.Album()
			raw := m.Raw()
			// composer → author (preferred); fall back to album_artist / artist
			composerName := rawTagString(raw, "composer", "TCOM", "©wrt",
				"----:com.apple.iTunes:COMPOSER")
			if composerName != "" {
				authorName = composerName
				// When composer holds the author, artist typically holds the narrator.
				narratorName = coalesce(m.Artist(), m.AlbumArtist())
			} else {
				// No composer tag: author is album_artist or artist.
				authorName = coalesce(m.AlbumArtist(), m.Artist())
				// Narrator comes from explicit narrator tags only in this case.
			}
			// Narrator-specific tags always win over the artist fallback above.
			if explicit := rawTagString(raw,
				"narrator", "narrated_by", "©nrt",
				"TXXX:NARRATOR", "----:com.apple.iTunes:NARRATOR",
				"com.apple.iTunes:narrator",
			); explicit != "" {
				narratorName = explicit
			}
			// date → published year
			if yr := rawTagString(raw, "date", "TDRC", "TYER", "©day"); yr != "" {
				if y, err := strconv.Atoi(strings.TrimSpace(yr[:min4(len(yr), 4)])); err == nil && y > 0 {
					publishedYear = &y
				}
			}
		}
		f.Close()
	}

	// ── Parse parent directory name for series/author/narrator fallbacks ──
	parentDir := filepath.Base(filepath.Dir(cand.dirPath))
	seriesFromParent, authorFromParent, narratorFromParent := parseSeriesDirName(parentDir)

	if authorName == "" {
		authorName = authorFromParent
	}
	if narratorName == "" {
		narratorName = narratorFromParent
	}

	// ── Parse book directory name for series index + title ────────────────
	bookDirBase := filepath.Base(cand.dirPath)
	titleFromDir, seriesIndex := parseBookDirName(bookDirBase)

	// Resolve final title: prefer album tag, then dir parse.
	if bookTitle == "" {
		bookTitle = titleFromDir
	}
	if bookTitle == "" {
		bookTitle = bookDirBase
	}

	// Resolve series: use parent dir series if we successfully parsed a book index.
	var seriesPtr *string
	if seriesFromParent != "" && seriesIndex != nil {
		seriesPtr = &seriesFromParent
	}

	if authorName == "" {
		authorName = "Unknown Author"
	}

	// ── Upsert author ─────────────────────────────────────────────────────
	authorID := deterministicID("artist:" + strings.ToLower(authorName))
	if _, err := g.db.UpsertArtist(ctx, store.UpsertArtistParams{
		ID:       authorID,
		Name:     authorName,
		SortName: sortName(authorName),
	}); err != nil {
		return "", fmt.Errorf("upsert author: %w", err)
	}

	// ── Cover art ─────────────────────────────────────────────────────────
	coverKey := fmt.Sprintf("audiobook-covers/%s.jpg", audiobookID)
	var coverArtKeyPtr *string
	if _, done := g.coveredBooks.Load(audiobookID); done {
		coverArtKeyPtr = &coverKey
	} else {
		var picData []byte
		// Try embedded art in first file.
		if f, err := os.Open(firstFile); err == nil {
			if m, err := tag.ReadFrom(f); err == nil {
				if pic := m.Picture(); pic != nil && len(pic.Data) > 0 {
					picData = pic.Data
				}
			}
			f.Close()
		}
		// Fallback: folder.jpg / cover.jpg in book dir or parent dir.
		if len(picData) == 0 {
			picData = g.cachedFolderImage(cand.dirPath)
		}
		if len(picData) == 0 {
			picData = g.cachedFolderImage(filepath.Dir(cand.dirPath))
		}
		if len(picData) > 0 {
			if err := storeAudiobookCoverArt(ctx, g.obj, coverKey, picData); err != nil {
				slog.Warn("audiobook cover art storage failed", "title", bookTitle, "err", err)
			} else {
				coverArtKeyPtr = &coverKey
				g.coveredBooks.Store(audiobookID, struct{}{})
			}
		}
	}

	// ── Store chapter files and build chapter records ─────────────────────
	var totalDurationMs int64
	chapters := make([]store.AudiobookChapter, 0, len(sortedFiles))
	var cumulativeMs int64

	for i, p := range sortedFiles {
		ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(p)), ".")
		chapterFileKey := fmt.Sprintf("audiobook-chapters/%s/%04d.%s", audiobookID, i+1, ext)

		// Store the chapter file in objstore.
		if exists, _ := g.obj.Exists(ctx, chapterFileKey); !exists {
			cf, err := os.Open(p)
			if err != nil {
				slog.Warn("audiobook chapter open failed", "path", p, "err", err)
			} else {
				fi, _ := cf.Stat()
				var size int64
				if fi != nil {
					size = fi.Size()
				}
				if err := g.obj.Put(ctx, chapterFileKey, cf, size); err != nil {
					slog.Warn("audiobook chapter store failed", "path", p, "err", err)
				}
				cf.Close()
			}
		}

		// Get chapter duration.
		chapDurationMs, err := probeFileDuration(p)
		if err != nil {
			slog.Warn("audiobook chapter probe failed", "path", p, "err", err)
			chapDurationMs = 0
		}

		// Determine chapter title: prefer tagged title, else filename without ext.
		chapTitle := strings.TrimSuffix(filepath.Base(p), filepath.Ext(p))
		if f, err := os.Open(p); err == nil {
			if m, err := tag.ReadFrom(f); err == nil {
				if t := m.Title(); t != "" {
					chapTitle = t
				}
			}
			f.Close()
		}

		startMs := cumulativeMs
		endMs := cumulativeMs + chapDurationMs
		cumulativeMs = endMs
		totalDurationMs += chapDurationMs

		chapID := deterministicID(fmt.Sprintf("chapter:%s:%d", audiobookID, i))
		fk := chapterFileKey
		chapters = append(chapters, store.AudiobookChapter{
			ID:          chapID,
			AudiobookID: audiobookID,
			Title:       chapTitle,
			StartMs:     startMs,
			EndMs:       endMs,
			ChapterNum:  i,
			FileKey:     &fk,
		})
	}

	// ── Compute total file size for the record ────────────────────────────
	var totalFileSize int64
	for _, p := range sortedFiles {
		if fi, err := os.Stat(p); err == nil {
			totalFileSize += fi.Size()
		}
	}

	// ── Upsert audiobook (FileKey = nil for multi-file) ───────────────────
	params := store.UpsertAudiobookParams{
		ID:          audiobookID,
		Title:       bookTitle,
		AuthorID:    &authorID,
		CoverArtKey: coverArtKeyPtr,
		FileKey:     nil, // multi-file: no single file key
		FileSize:    totalFileSize,
		Format:      "mp3", // predominant format
		DurationMs:  totalDurationMs,
		Fingerprint: fingerprint,
		Series:      seriesPtr,
		SeriesIndex: seriesIndex,
	}
	if publishedYear != nil {
		params.PublishedYear = publishedYear
	}
	if _, err := g.db.UpsertAudiobook(ctx, params); err != nil {
		return "", fmt.Errorf("upsert audiobook: %w", err)
	}

	// ── Narrator ──────────────────────────────────────────────────────────
	if narratorName != "" {
		narratorID := deterministicID("narrator:" + strings.ToLower(narratorName))
		if err := g.db.UpsertAudiobookNarrator(ctx, store.UpsertAudiobookNarratorParams{
			ID:       narratorID,
			Name:     narratorName,
			SortName: sortName(narratorName),
		}); err != nil {
			slog.Warn("upsert narrator failed", "name", narratorName, "err", err)
		} else if err := g.db.SetAudiobookNarrators(ctx, audiobookID, []string{narratorID}); err != nil {
			slog.Warn("set audiobook narrators failed", "audiobook_id", audiobookID, "err", err)
		}
	}

	// ── Chapters ──────────────────────────────────────────────────────────
	if len(chapters) > 0 {
		if err := g.db.ReplaceAudiobookChapters(ctx, audiobookID, chapters); err != nil {
			slog.Warn("replace audiobook chapters failed", "audiobook_id", audiobookID, "err", err)
		}
	}

	// ── Open Library enrichment (best-effort, async) ──────────────────────
	if g.ol != nil {
		go g.enrichAudiobook(context.Background(), audiobookID, bookTitle, authorName, coverArtKeyPtr == nil)
	}

	return audiobookID, nil
}

// min4 is a local helper to avoid importing slices just for min.
func min4(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func (g *AudiobookIngester) enrichAudiobook(ctx context.Context, audiobookID, title, author string, missingCover bool) {
	if _, done := g.enrichedBooks.LoadOrStore(audiobookID, struct{}{}); done {
		return
	}

	result, err := g.ol.Search(ctx, title, author)
	if err != nil {
		slog.Debug("open library search failed", "title", title, "err", err)
		return
	}
	if result == nil {
		slog.Debug("open library: no match", "title", title)
		return
	}

	var descPtr *string
	if result.Description != "" {
		descPtr = &result.Description
	}
	var olKeyPtr *string
	if result.OLKey != "" {
		olKeyPtr = &result.OLKey
	}
	var isbnPtr *string
	if result.ISBN != "" {
		isbnPtr = &result.ISBN
	}
	var yearPtr *int
	if result.PublishedYear > 0 {
		yearPtr = &result.PublishedYear
	}

	if err := g.db.UpdateAudiobookEnrichment(ctx, audiobookID, descPtr, olKeyPtr, isbnPtr, yearPtr); err != nil {
		slog.Warn("audiobook enrichment update failed", "audiobook_id", audiobookID, "err", err)
	}

	if missingCover && result.CoverID > 0 {
		imgData, imgErr := g.ol.FetchCoverArt(ctx, result.CoverID)
		if imgErr != nil {
			slog.Debug("open library cover fetch failed", "audiobook_id", audiobookID, "err", imgErr)
		} else if imgData != nil {
			coverKey := fmt.Sprintf("audiobook-covers/%s.jpg", audiobookID)
			if err := g.obj.Put(ctx, coverKey, bytes.NewReader(imgData), int64(len(imgData))); err != nil {
				slog.Warn("store open library cover failed", "audiobook_id", audiobookID, "err", err)
			} else {
				g.coveredBooks.Store(audiobookID, struct{}{})
				if err := g.db.UpdateAudiobookCoverArt(ctx, audiobookID, coverKey); err != nil {
					slog.Warn("update audiobook cover art key failed", "audiobook_id", audiobookID, "err", err)
				}
			}
		}
	}

	slog.Info("audiobook enriched", "audiobook_id", audiobookID, "ol_key", result.OLKey)
}

func (g *AudiobookIngester) cachedFolderImage(dir string) []byte {
	if v, ok := g.folderImgCache.Load(dir); ok {
		return v.([]byte)
	}
	data := bestFolderImage(dir)
	if actual, loaded := g.folderImgCache.LoadOrStore(dir, data); loaded {
		return actual.([]byte)
	}
	return data
}

func storeAudiobookCoverArt(ctx context.Context, obj objstore.ObjectStore, key string, data []byte) error {
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

// AudiobookIngestService wraps AudiobookIngester and exposes scan triggering.
type AudiobookIngestService struct {
	ingester *AudiobookIngester
	rootCtx  context.Context
	running  atomic.Bool
}

// NewAudiobookIngestService creates the service.
// serverCtx is the top-level server context; scans use it so they survive HTTP request cancellation.
func NewAudiobookIngestService(serverCtx context.Context, db *store.Store, obj objstore.ObjectStore, cfg AudiobookConfig) *AudiobookIngestService {
	return &AudiobookIngestService{
		ingester: NewAudiobookIngester(db, obj, cfg),
		rootCtx:  serverCtx,
	}
}

// RootCtx returns the server-level context for use by HTTP handlers.
func (s *AudiobookIngestService) RootCtx() context.Context { return s.rootCtx }

// TriggerScan starts an async scan and returns immediately.
// Returns an error if a scan is already running.
func (s *AudiobookIngestService) TriggerScan(ctx context.Context) error {
	if !s.running.CompareAndSwap(false, true) {
		return errors.New("scan already in progress")
	}
	go func() {
		defer s.running.Store(false)
		newIDs, skipped, errs := s.ingester.Scan(ctx)
		slog.Info("audiobook scan complete", "ingested", len(newIDs), "skipped", skipped, "errors", errs)
	}()
	return nil
}

// StartWatch runs an initial scan on startup, then polls for changes.
// Blocks until ctx is cancelled. Call in a goroutine.
func (s *AudiobookIngestService) StartWatch(ctx context.Context) {
	_ = s.TriggerScan(ctx)

	// Polling loop — re-scan on the configured interval.
	interval := s.ingester.cfg.PollInterval
	if interval <= 0 {
		interval = 30 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			_ = s.TriggerScan(ctx)
		}
	}
}

// IsRunning reports whether a scan is currently in progress.
func (s *AudiobookIngestService) IsRunning() bool { return s.running.Load() }
