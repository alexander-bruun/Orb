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
	// StableTime is the minimum age (mtime) before a file is considered ready to ingest; detects incomplete downloads.
	StableTime time.Duration
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
	case ".m4b", ".m4a", ".mp3", ".flac", ".opus", ".ogg", ".aac", ".wma":
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

// isStable checks if a file is old enough to be considered ready for ingest.
// Files with recent modification times (being downloaded) are not considered stable.
func (g *AudiobookIngester) isStable(fi os.FileInfo) bool {
	if g.cfg.StableTime == 0 {
		return true // stability check disabled
	}
	age := time.Since(fi.ModTime())
	return age >= g.cfg.StableTime
}

// isDirStable checks if a directory (by its max mtime) is old enough for ingest.
func (g *AudiobookIngester) isDirStable(maxMtime int64) bool {
	if g.cfg.StableTime == 0 {
		return true // stability check disabled
	}
	age := time.Since(time.Unix(maxMtime, 0))
	return age >= g.cfg.StableTime
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

// ReingestAudiobook clears the ingest state for the given audiobook and
// re-processes only that audiobook's paths (file or directory).
func (g *AudiobookIngester) ReingestAudiobook(ctx context.Context, audiobookID string) {
	paths, err := g.db.DeleteAudiobookIngestStateByID(ctx, audiobookID)
	if err != nil {
		slog.Error("reingest audiobook: delete ingest state", "audiobook_id", audiobookID, "err", err)
		return
	}
	if len(paths) == 0 {
		if g.reingestAudiobookByScan(ctx, audiobookID) {
			slog.Info("audiobook reingest complete", "audiobook_id", audiobookID,
				"ingested", 1, "skipped", 0, "errors", 0)
			return
		}
		if g.reingestAudiobookFromObjectStore(ctx, audiobookID) {
			slog.Info("audiobook reingest complete", "audiobook_id", audiobookID,
				"ingested", 1, "skipped", 0, "errors", 0)
			return
		}
		slog.Info("reingest audiobook: no ingest state found, nothing to do", "audiobook_id", audiobookID)
		return
	}

	// Remove paths from in-memory state so upToDate / upToDateDir return false.
	g.stateMu.Lock()
	for _, p := range paths {
		delete(g.state, p)
	}
	g.stateMu.Unlock()

	var ingested, skipped, errs int
	opts := ingestOptions{forceID: audiobookID, allowExistingFingerprint: true}

	for _, p := range paths {
		fi, err := os.Stat(p)
		if err != nil {
			slog.Warn("audiobook reingest: stat failed", "path", p, "err", err)
			errs++
			continue
		}
		if fi.IsDir() {
			candCh := make(chan audiobookCandidate, 8)
			go func() {
				defer close(candCh)
				g.scanDir(ctx, p, candCh)
			}()
			for cand := range candCh {
				if cand.singleFile {
					fp := cand.files[0]
					finfo, err := os.Stat(fp)
					if err != nil {
						slog.Warn("audiobook reingest: stat failed", "path", fp, "err", err)
						errs++
						continue
					}
					id, err := g.ingestFileWithOptions(ctx, fp, finfo, opts)
					if err != nil {
						if errors.Is(err, ErrSkipped) {
							skipped++
						} else {
							slog.Error("audiobook reingest failed", "path", fp, "err", err)
							errs++
						}
						continue
					}
					if id != "" {
						g.markDone(ctx, fp, finfo, id)
						ingested++
					}
				} else {
					var totalSize, maxMtime int64
					allValid := true
					for _, fp := range cand.files {
						finfo, err := os.Stat(fp)
						if err != nil {
							slog.Warn("audiobook reingest: dir stat failed", "path", fp, "err", err)
							allValid = false
							break
						}
						totalSize += finfo.Size()
						if m := finfo.ModTime().Unix(); m > maxMtime {
							maxMtime = m
						}
					}
					if !allValid {
						errs++
						continue
					}
					id, err := g.ingestDirectoryWithOptions(ctx, cand, opts)
					if err != nil {
						if errors.Is(err, ErrSkipped) {
							skipped++
						} else {
							slog.Error("audiobook dir reingest failed", "dir", cand.dirPath, "err", err)
							errs++
						}
						continue
					}
					if id != "" {
						g.markDoneDir(ctx, cand.dirPath, totalSize, maxMtime, id)
						ingested++
					}
				}
			}
		} else {
			id, err := g.ingestFileWithOptions(ctx, p, fi, opts)
			if err != nil {
				if errors.Is(err, ErrSkipped) {
					skipped++
				} else {
					slog.Error("audiobook reingest failed", "path", p, "err", err)
					errs++
				}
				continue
			}
			if id != "" {
				g.markDone(ctx, p, fi, id)
				ingested++
			}
		}
	}

	slog.Info("audiobook reingest complete", "audiobook_id", audiobookID,
		"ingested", ingested, "skipped", skipped, "errors", errs)
}

// reingestAudiobookByScan attempts to locate the audiobook on disk when ingest
// state rows are missing, then reingests it in-place to refresh metadata.
func (g *AudiobookIngester) reingestAudiobookByScan(ctx context.Context, audiobookID string) bool {
	if len(g.cfg.Dirs) == 0 {
		slog.Warn("reingest audiobook: no scan dirs configured", "audiobook_id", audiobookID)
		return false
	}
	slog.Info("reingest audiobook: ingest state missing, scanning dirs", "audiobook_id", audiobookID)

	candCh := make(chan audiobookCandidate, 64)
	go func() {
		defer close(candCh)
		for _, dir := range g.cfg.Dirs {
			dir = strings.TrimSpace(dir)
			if dir == "" || g.isDirExcluded(dir) {
				continue
			}
			g.scanDir(ctx, dir, candCh)
		}
	}()

	opts := ingestOptions{forceID: audiobookID, allowExistingFingerprint: true}
	for cand := range candCh {
		if cand.singleFile {
			if len(cand.files) == 0 {
				continue
			}
			fp := cand.files[0]
			fingerprint, err := fileFingerprint(fp)
			if err != nil {
				slog.Warn("reingest audiobook: fingerprint failed", "path", fp, "err", err)
				continue
			}
			if deterministicUUID(fingerprint) != audiobookID {
				continue
			}
			slog.Info("reingest audiobook: matched single-file candidate", "audiobook_id", audiobookID, "path", fp)
			fi, err := os.Stat(fp)
			if err != nil {
				slog.Warn("reingest audiobook: stat failed", "path", fp, "err", err)
				continue
			}
			id, err := g.ingestFileWithOptions(ctx, fp, fi, opts)
			if err != nil {
				slog.Warn("reingest audiobook: ingest failed", "path", fp, "err", err)
				return false
			}
			if id != "" {
				g.markDone(ctx, fp, fi, id)
				return true
			}
			return false
		}

		sortedFiles := sortAudiobookFilesByTrack(cand.files)
		fingerprint, err := dirFingerprint(sortedFiles)
		if err != nil {
			slog.Warn("reingest audiobook: dir fingerprint failed", "dir", cand.dirPath, "err", err)
			continue
		}
		if deterministicUUID(fingerprint) != audiobookID {
			continue
		}
		slog.Info("reingest audiobook: matched directory candidate", "audiobook_id", audiobookID, "dir", cand.dirPath)
		id, err := g.ingestDirectoryWithOptions(ctx, cand, opts)
		if err != nil {
			slog.Warn("reingest audiobook: dir ingest failed", "dir", cand.dirPath, "err", err)
			return false
		}
		if id != "" {
			var totalSize, maxMtime int64
			for _, fp := range cand.files {
				finfo, err := os.Stat(fp)
				if err != nil {
					slog.Warn("reingest audiobook: dir stat failed", "path", fp, "err", err)
					return false
				}
				totalSize += finfo.Size()
				if m := finfo.ModTime().Unix(); m > maxMtime {
					maxMtime = m
				}
			}
			g.markDoneDir(ctx, cand.dirPath, totalSize, maxMtime, id)
			return true
		}
		return false
	}
	slog.Warn("reingest audiobook: scan completed without match", "audiobook_id", audiobookID)
	return false
}

// reingestAudiobookFromObjectStore attempts a targeted reingest using the stored
// file_key when ingest state rows are missing. Returns true if it performed a
// reingest (even if no chapters were found), false if it could not proceed.
func (g *AudiobookIngester) reingestAudiobookFromObjectStore(ctx context.Context, audiobookID string) bool {
	ab, err := g.db.GetAudiobook(ctx, audiobookID)
	if err != nil {
		slog.Warn("reingest audiobook: lookup failed", "audiobook_id", audiobookID, "err", err)
		return false
	}
	if ab.FileKey == nil || *ab.FileKey == "" {
		return false
	}
	ext := strings.ToLower(ab.Format)
	if ext != "m4b" && ext != "m4a" {
		return false
	}
	info, err := probeM4BFromObjectStore(ctx, g.obj, *ab.FileKey)
	if err != nil {
		slog.Warn("reingest audiobook: object-store probe failed", "audiobook_id", audiobookID, "err", err)
		return false
	}
	if info.durationMs > 0 {
		if err := g.db.UpdateAudiobookDuration(ctx, audiobookID, info.durationMs); err != nil {
			slog.Warn("reingest audiobook: update duration failed", "audiobook_id", audiobookID, "err", err)
		}
	}
	if len(info.chapters) == 0 {
		return true
	}
	chapters := make([]store.AudiobookChapter, 0, len(info.chapters))
	for i, rc := range info.chapters {
		chapTitle := normalizeChapterTitle(rc.title, i+1)
		startMs := rc.startMs
		var endMs int64
		if i+1 < len(info.chapters) {
			endMs = info.chapters[i+1].startMs
		} else {
			endMs = info.durationMs
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
		slog.Warn("reingest audiobook: replace chapters failed", "audiobook_id", audiobookID, "err", err)
	}
	return true
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
//
// Special case: when a directory has no direct audio files and every
// non-excluded sub-directory matches a "part-like" name (Part N, Disc N,
// CD N, Side N/A-D), all of their audio files are merged into a single
// candidate rooted at the parent directory.
func (g *AudiobookIngester) scanDir(ctx context.Context, root string, results chan<- audiobookCandidate) {
	entries, err := os.ReadDir(root)
	if err != nil {
		slog.Warn("audiobook scan: read dir failed", "dir", root, "err", err)
		return
	}

	var audioFiles []string
	var hasContainerFmt bool // M4B/M4A: single-file with embedded chapters
	var hasMultiFileFmt bool // MP3, FLAC, etc.: one file per chapter
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
			if isSingleFileAudiobook(p) {
				hasContainerFmt = true
			} else {
				hasMultiFileFmt = true
			}
		}
	}

	// Part-merging: when this directory has no direct audio files and every
	// non-excluded sub-directory is "part-like", treat them as one audiobook.
	if len(audioFiles) == 0 && len(subDirs) >= 2 {
		if merged := g.collectPartFiles(subDirs); len(merged) > 0 {
			results <- audiobookCandidate{dirPath: root, files: merged, singleFile: false}
			return // sub-directories consumed; do not recurse further
		}
	}

	// Recurse into sub-directories so deeper books are found first.
	for _, sub := range subDirs {
		g.scanDir(ctx, sub, results)
	}

	// If this looks like a series folder and it contains audio files directly,
	// ignore those files (series folders should contain book subdirs), but still
	// recurse into subfolders. Only apply when there are sub-directories; leaf
	// folders should be treated as books even if they resemble series names.
	if len(audioFiles) > 0 && len(subDirs) > 0 && isSeriesDirName(filepath.Base(root)) {
		slog.Info("audiobook scan: ignoring audio files in series folder", "dir", root)
		audioFiles = nil
		hasContainerFmt = false
		hasMultiFileFmt = false
	}

	if len(audioFiles) == 0 {
		return
	}

	// Decide mode for this directory's files:
	//   Single-file: exactly one M4B/M4A and no multi-file formats.
	//   Directory mode: anything else.
	if !hasMultiFileFmt && hasContainerFmt && len(audioFiles) == 1 {
		results <- audiobookCandidate{
			dirPath:    root,
			files:      audioFiles,
			singleFile: true,
		}
	} else {
		sort.Strings(audioFiles) // pre-sort; ingestDirectory will re-sort by track tag
		results <- audiobookCandidate{
			dirPath:    root,
			files:      audioFiles,
			singleFile: false,
		}
	}
}

// collectPartFiles checks whether all subDirs are "part-like" directories
// (Part N, Disc N, CD N, Side N/A-D) and, if so, returns all audio files
// from them concatenated in part order. Returns nil if not all dirs match.
func (g *AudiobookIngester) collectPartFiles(subDirs []string) []string {
	type partEntry struct {
		path string
		num  int
	}
	parts := make([]partEntry, 0, len(subDirs))
	for _, sub := range subDirs {
		n, ok := parsePartDirNum(filepath.Base(sub))
		if !ok {
			return nil
		}
		parts = append(parts, partEntry{path: sub, num: n})
	}
	sort.Slice(parts, func(i, j int) bool { return parts[i].num < parts[j].num })
	var files []string
	for _, p := range parts {
		files = append(files, audioFilesInDir(p.path)...)
	}
	return files
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
	// foundPaths collects the ingest-state key for every candidate seen this
	// scan (file path for single-file books, dirPath for multi-file). Used
	// after the scan to prune DB records whose source paths no longer exist.
	var foundPaths []string

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
				// Record candidate key regardless of skip/error outcome.
				mu.Lock()
				if cand.singleFile {
					foundPaths = append(foundPaths, cand.files[0])
				} else {
					foundPaths = append(foundPaths, cand.dirPath)
				}
				mu.Unlock()

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
					if !g.isStable(fi) {
						slog.Debug("audiobook ingest: skipping unstable file (in-progress download?)", "path", p, "mtime_age_sec", int(time.Since(fi.ModTime()).Seconds()))
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
					if !g.isDirStable(maxMtime) {
						slog.Debug("audiobook ingest: skipping unstable directory (in-progress download?)", "dir", cand.dirPath, "mtime_age_sec", int(time.Since(time.Unix(maxMtime, 0)).Seconds()))
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

	// Prune DB records for audiobooks whose source paths were deleted from disk.
	pruned, objKeys, err := g.db.PruneOrphanedAudiobooks(ctx, foundPaths)
	if err != nil {
		slog.Warn("audiobook ingest: prune orphaned audiobooks failed", "err", err)
	} else if pruned > 0 {
		slog.Info("audiobook ingest: pruned orphaned audiobooks", "count", pruned)
		for _, k := range objKeys {
			if err := g.obj.Delete(ctx, k); err != nil {
				slog.Warn("audiobook ingest: delete orphaned object failed", "key", k, "err", err)
			}
		}
	}

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
	case ".flac":
		return flacDurationMs(path)
	}
	return 0, nil
}

// ── Directory name parsing helpers ───────────────────────────────────────────

var (
	reAuthorInDir   = regexp.MustCompile(`\(by ([^)]+)\)`)
	reNarratorInDir = regexp.MustCompile(`(?i)\((?:read|narrated)\s+by\s+([^)]+)\)`)
	// "Series by Author" without surrounding parentheses, optionally followed by
	// quality/format tags like "[FLAC]" or a year "(2019)".
	reAuthorBareBy = regexp.MustCompile(`(?i)^(.+?)\s+(?:written\s+)?by\s+([^(\[{]+)(?:[\[({].*)?$`)
	// Trailing audio quality/format tags: "[FLAC]", "[V0]", "(320kbps)", etc.
	reQualityTag = regexp.MustCompile(`(?i)\s*[\[\(](?:FLAC|MP3|AAC|OGG|OPUS|ALAC|V\d|320|256|128|lossless)[\]\)]\s*$`)

	// Book directory name patterns, tried in order (most specific first).
	reBookKeywordPrefix = regexp.MustCompile(`(?i)^(?:book|part|vol(?:ume)?\.?)\s+(\d+(?:\.\d+)?)\s*[-–]\s*(.+)$`)
	reHashPrefix        = regexp.MustCompile(`^#(\d+(?:\.\d+)?)\s*[-–]\s*(.+)$`)
	reTitleWithIndex    = regexp.MustCompile(`(?i)^(.+?)\s+\((?:book|part|vol(?:ume)?\.?)\s+(\d+(?:\.\d+)?)\)\s*$`)
	reNumericPrefix     = regexp.MustCompile(`^(\d{1,3}(?:\.\d+)?)\s*[-–]\s*(.+)$`)
	rePeriodPrefix      = regexp.MustCompile(`^(\d{1,3}(?:\.\d+)?)\.[ \t]+(.+)$`)
	reSeriesInfixIndex  = regexp.MustCompile(`^(.+?)\s+(0\d|\d{2,3}|\d+\.\d+)\s+(.+)$`)
	// reSeriesInfixDash matches "Series Name N - Book Title" with any digit count,
	// where the dash separator is the key signal that N is a series index.
	reSeriesInfixDash = regexp.MustCompile(`^(.+?)\s+(\d+(?:\.\d+)?)\s*[-–]\s*(.+)$`)
	reBookSuffixIndex = regexp.MustCompile(`(?i)\b(?:book|part|vol(?:ume)?\.?)\s*([0-9]+(?:\.\d+)?)\b`)

	// Strips common edition/quality tags from the end of a book title.
	reEditionTag = regexp.MustCompile(`(?i)\s*[\(\[](unabridged|abridged|commercial\s*audiobook|audiobook)[\)\]]\s*$`)
	// Strips leading/trailing "Book N" or "Part N" decorations in titles.
	reBookTitlePrefix = regexp.MustCompile(`(?i)^(?:book|part|vol(?:ume)?\.?)\s+(\d+(?:\.\d+)?)\s*[-–:]\s*`)
	reBookTitleSuffix = regexp.MustCompile(`(?i)\s*[-–:]?\s*(?:book|part|vol(?:ume)?\.?)\s+(\d+(?:\.\d+)?)\s*$`)
	// Strips trailing edition words without parentheses.
	reEditionWordSuffix = regexp.MustCompile(`(?i)\s*[-–:]?\s*(unabridged|abridged|commercial\s+audiobook|audiobook)\s*$`)
	// Strips a standalone year suffix like " (2019)" from titles.
	reYearSuffix = regexp.MustCompile(`\s+\(\d{4}\)\s*$`)

	// Part/disc directory detection for multi-part audiobook merging.
	rePartDir = regexp.MustCompile(`(?i)^(part|disc|disk|cd|side)\s*(\d+|[abcdABCD])$`)

	// Chapter filename numeric prefix stripper: "01 - ", "001. ", "1_", etc.
	reChapterFilePrefix = regexp.MustCompile(`^\d{1,4}[\s\-–_.]+`)
)

func normalizeEditionLabel(s string) string {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "unabridged":
		return "Unabridged"
	case "abridged":
		return "Abridged"
	case "commercial audiobook":
		return "Commercial Audiobook"
	case "audiobook":
		return "Audiobook"
	default:
		return strings.TrimSpace(s)
	}
}

func stripEditionSuffix(title string) (string, *string) {
	t := strings.TrimSpace(title)
	if t == "" {
		return t, nil
	}
	if m := reEditionTag.FindStringSubmatch(t); m != nil {
		edition := normalizeEditionLabel(m[1])
		cleaned := strings.TrimSpace(reEditionTag.ReplaceAllString(t, ""))
		if cleaned == "" {
			return t, nil
		}
		return cleaned, &edition
	}
	if m := reEditionWordSuffix.FindStringSubmatch(t); m != nil {
		edition := normalizeEditionLabel(m[1])
		cleaned := strings.TrimSpace(reEditionWordSuffix.ReplaceAllString(t, ""))
		if cleaned == "" {
			return t, nil
		}
		return cleaned, &edition
	}
	return t, nil
}

// parseSeriesDirName extracts series name, author and narrator from a directory
// name. Patterns are tried in order (most specific first):
//
//	"Harry Potter (by J.K. Rowling) - The Complete Story (read by Stephen Fry) [V0]"
//	"Wheel of Time by Robert Jordan [FLAC]"
//	"Wheel of Time by Robert Jordan (2021)"
//	"James S. A. Corey - The Expanse Series"
//
// Trailing quality tags ("[FLAC]", "[V0]", etc.), edition tags ("(Unabridged)"),
// and year suffixes ("(2019)") are stripped from the returned series name.
// Any field may be empty if not found.
func parseSeriesDirName(name string) (series, author, narrator string) {
	// "(by Author)" parenthetical — highest confidence.
	if m := reAuthorInDir.FindStringSubmatchIndex(name); m != nil {
		author = strings.TrimSpace(name[m[2]:m[3]])
		// Series is everything before the first "(by …)" occurrence.
		series = strings.TrimSpace(name[:m[0]])
		// Strip trailing " - " or similar separators from series name.
		series = strings.TrimRight(series, " \t-–")
	}
	// "(read/narrated by Narrator)" parenthetical.
	if m := reNarratorInDir.FindStringSubmatch(name); m != nil {
		narrator = strings.TrimSpace(m[1])
	}
	// Bare "Series by Author" format without parentheses.
	// Guard: skip if the only "by" belongs to a read/narrated phrase already captured above.
	if author == "" {
		lower := strings.ToLower(name)
		if !strings.Contains(lower, "read by") && !strings.Contains(lower, "narrated by") {
			if m := reAuthorBareBy.FindStringSubmatch(name); m != nil {
				if series == "" {
					series = strings.TrimSpace(m[1])
				}
				author = strings.TrimSpace(m[2])
			}
		}
	}
	// Fallback: "Author - Series" dash-separated (no other markers found).
	if author == "" && series == "" {
		if idx := strings.Index(name, " - "); idx > 0 {
			author = strings.TrimSpace(name[:idx])
			series = strings.TrimSpace(name[idx+3:])
		}
	}
	// Clean trailing quality/format tags, edition tags, and year suffixes from series.
	if series != "" {
		series = cleanSeriesName(series)
	}
	return series, author, narrator
}

// isSeriesDirName reports whether the directory name looks like a series folder
// (contains a series name and an explicit author/narrator marker).
func isSeriesDirName(name string) bool {
	if looksLikeBookDirName(name) {
		return false
	}
	series, author, narrator := parseSeriesDirName(name)
	return series != "" && (author != "" || narrator != "")
}

// looksLikeBookDirName reports whether the name matches a book-index pattern.
// This prevents "Book 5 - Title" or "... Book 1" folders from being treated
// as series directories.
func looksLikeBookDirName(name string) bool {
	_, idx, _ := parseBookDirName(name)
	return idx != nil
}

// parseBookDirName extracts a title and optional series index from a book
// directory name. Patterns are tried in order (most specific first):
//
//	"Book 1 - Title"     keyword prefix (Book / Part / Vol)
//	"#1 - Title"         hash prefix
//	"Title (Book 2)"     title-first with index in parens
//	"01 - Title"         numeric prefix (1–3 digits)
//	"01. Title"          period separator
//
// Any trailing edition tag like "(Unabridged)" or year "(2019)" is stripped
// from the returned title.
func parseBookDirName(name string) (title string, seriesIndex *float64, edition *string) {
	parseIdx := func(s string) *float64 {
		f, err := strconv.ParseFloat(s, 64)
		if err != nil {
			return nil
		}
		return &f
	}

	if m := reBookKeywordPrefix.FindStringSubmatch(name); m != nil {
		title, edition = cleanBookTitleWithEdition(m[2])
		return title, parseIdx(m[1]), edition
	}
	if m := reHashPrefix.FindStringSubmatch(name); m != nil {
		title, edition = cleanBookTitleWithEdition(m[2])
		return title, parseIdx(m[1]), edition
	}
	if m := reTitleWithIndex.FindStringSubmatch(name); m != nil {
		title, edition = cleanBookTitleWithEdition(m[1])
		return title, parseIdx(m[2]), edition
	}
	if m := reNumericPrefix.FindStringSubmatch(name); m != nil {
		title, edition = cleanBookTitleWithEdition(m[2])
		return title, parseIdx(m[1]), edition
	}
	if m := rePeriodPrefix.FindStringSubmatch(name); m != nil {
		title, edition = cleanBookTitleWithEdition(m[2])
		return title, parseIdx(m[1]), edition
	}
	if m := reSeriesInfixIndex.FindStringSubmatch(name); m != nil {
		if hasLetter(m[1]) {
			title, edition = cleanBookTitleWithEdition(m[3])
			return title, parseIdx(m[2]), edition
		}
	}
	if m := reSeriesInfixDash.FindStringSubmatch(name); m != nil {
		if hasLetter(m[1]) {
			title, edition = cleanBookTitleWithEdition(m[3])
			return title, parseIdx(m[2]), edition
		}
	}
	if m := reBookSuffixIndex.FindStringSubmatchIndex(name); m != nil {
		idx := parseIdx(name[m[2]:m[3]])
		if idx != nil {
			title = strings.TrimSpace(name[:m[0]])
			title = strings.TrimRight(title, " \t-–,:")
			title, edition = cleanBookTitleWithEdition(title)
			return title, idx, edition
		}
	}
	title, edition = cleanBookTitleWithEdition(name)
	return title, nil, edition
}

func cleanBookTitleWithEdition(title string) (string, *string) {
	title = reYearSuffix.ReplaceAllString(title, "")
	title, edition := stripEditionSuffix(title)
	title = reBookTitlePrefix.ReplaceAllString(title, "")
	title = reBookTitleSuffix.ReplaceAllString(title, "")
	return strings.TrimSpace(title), edition
}

// cleanSeriesName strips common tags and a trailing "Series" suffix.
func cleanSeriesName(series string) string {
	series = reQualityTag.ReplaceAllString(series, "")
	series = reEditionTag.ReplaceAllString(series, "")
	series = reYearSuffix.ReplaceAllString(series, "")
	series = strings.TrimSpace(series)
	lower := strings.ToLower(series)
	if strings.HasSuffix(lower, " series") {
		series = strings.TrimSpace(series[:len(series)-len(" series")])
	}
	return series
}

func isSuspiciousSeries(series, title string) bool {
	series = strings.TrimSpace(series)
	title = strings.TrimSpace(title)
	if series == "" || title == "" {
		return false
	}
	ns := normalizeSeriesText(series)
	nt := normalizeSeriesText(title)
	if ns == "" || nt == "" {
		return false
	}
	if ns == nt {
		return true
	}
	if strings.HasPrefix(ns, nt) && len(ns)-len(nt) <= 10 {
		return true
	}
	if strings.Contains(ns, nt) && strings.Contains(ns, "book") {
		return true
	}
	return false
}

func isSeriesRootDir(name string, roots []string) bool {
	if name == "" {
		return false
	}
	for _, r := range roots {
		base := filepath.Base(strings.TrimSpace(r))
		if base != "" && base == name {
			return true
		}
	}
	return false
}

func isSeriesTitleMatch(series, title string) bool {
	ns := normalizeSeriesText(series)
	nt := normalizeSeriesText(title)
	if ns == "" || nt == "" {
		return false
	}
	if ns == nt {
		return true
	}
	// Allow small suffixes like "unabridged", "abridged", etc.
	if strings.HasPrefix(nt, ns) && len(nt)-len(ns) <= 20 {
		return true
	}
	if strings.HasPrefix(ns, nt) && len(ns)-len(nt) <= 20 {
		return true
	}
	return false
}

func parseSeriesPrefixFromBookDirName(name string) string {
	// Guard: "Book 3 - Title" or "Vol. 1 - Title" should not yield "Book"/"Vol." as
	// a series prefix — they are keyword-indexed book names, not series annotations.
	if reBookKeywordPrefix.MatchString(name) {
		return ""
	}
	if m := reSeriesInfixIndex.FindStringSubmatch(name); m != nil {
		if hasLetter(m[1]) {
			return strings.TrimSpace(m[1])
		}
	}
	if m := reSeriesInfixDash.FindStringSubmatch(name); m != nil {
		if hasLetter(m[1]) {
			return strings.TrimSpace(m[1])
		}
	}
	return ""
}

func normalizeSeriesText(s string) string {
	s = strings.ToLower(s)
	var b strings.Builder
	prevSpace := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		if (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') {
			b.WriteByte(c)
			prevSpace = false
			continue
		}
		if c == ' ' || c == '-' || c == '_' || c == ',' || c == '.' || c == ':' || c == ';' {
			if !prevSpace {
				b.WriteByte(' ')
				prevSpace = true
			}
		}
	}
	return strings.TrimSpace(b.String())
}

func hasLetter(s string) bool {
	for i := 0; i < len(s); i++ {
		if (s[i] >= 'A' && s[i] <= 'Z') || (s[i] >= 'a' && s[i] <= 'z') {
			return true
		}
	}
	return false
}

func parseSeriesIndexValue(s string) *float64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	if idx := strings.Index(s, "/"); idx >= 0 {
		s = s[:idx]
	}
	re := regexp.MustCompile(`\d+(?:\.\d+)?`)
	m := re.FindString(s)
	if m == "" {
		return nil
	}
	v, err := strconv.ParseFloat(m, 64)
	if err != nil {
		return nil
	}
	return &v
}

func seriesFromTags(raw map[string]interface{}) (string, *float64) {
	series := rawTagString(raw,
		"series", "SERIES",
		"TXXX:SERIES", "TXXX:Series", "TXXX:SERIES_NAME", "TXXX:SERIESNAME",
		"----:com.apple.iTunes:SERIES", "----:com.apple.iTunes:Series",
		"com.apple.iTunes:SERIES",
	)
	indexRaw := rawTagString(raw,
		"series_index", "SERIES_INDEX", "SERIESINDEX",
		"TXXX:SERIES_INDEX", "TXXX:SERIESINDEX", "TXXX:SERIES PART",
		"TXXX:SERIES_PART", "TXXX:Series Part",
		"----:com.apple.iTunes:SERIES_INDEX", "----:com.apple.iTunes:SERIESINDEX",
		"----:com.apple.iTunes:SERIES_PART", "----:com.apple.iTunes:SERIES PART",
		"com.apple.iTunes:SERIES_INDEX",
	)
	if series != "" {
		series = cleanSeriesName(series)
	}
	return series, parseSeriesIndexValue(indexRaw)
}

// chapterTitleFromFilename returns a human-readable chapter title from a
// filename stem (no extension). Leading numeric track prefixes like "01 - ",
// "001. ", or "1_" are stripped and underscores are converted to spaces.
func chapterTitleFromFilename(stem string) string {
	title := reChapterFilePrefix.ReplaceAllString(stem, "")
	title = strings.ReplaceAll(title, "_", " ")
	title = strings.TrimSpace(title)
	if title == "" {
		return stem
	}
	return title
}

// parsePartDirNum returns the sort order for a part/disc directory name like
// "Part 1", "Disc 2", "CD 3", "Side A". Returns (0, false) if no match.
func parsePartDirNum(name string) (int, bool) {
	m := rePartDir.FindStringSubmatch(name)
	if m == nil {
		return 0, false
	}
	s := strings.ToUpper(m[2])
	if len(s) == 1 && s[0] >= 'A' && s[0] <= 'D' {
		return int(s[0]-'A') + 1, true
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0, false
	}
	return n, true
}

// audioFilesInDir returns all audiobook files directly inside dir, sorted.
func audioFilesInDir(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var files []string
	for _, e := range entries {
		if !e.IsDir() && isAudiobookFile(e.Name()) {
			files = append(files, filepath.Join(dir, e.Name()))
		}
	}
	sort.Strings(files)
	return files
}

// isValidDirName reports whether name is a meaningful directory component
// (not empty, ".", or "..").
func isValidDirName(name string) bool {
	return name != "" && name != "." && name != ".."
}

// ── ID3 track number helper ───────────────────────────────────────────────────

// readTrackTag opens an audio file and reads its track number from ID3/iTunes
// tags. Returns 0 on failure.
func readTrackTag(path string) int {
	f, err := os.Open(path)
	if err != nil {
		return 0
	}
	defer func() {
		if cerr := f.Close(); cerr != nil {
			slog.Warn("audiobook: track tag file close failed", "path", path, "err", cerr)
		}
	}()
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
		if _, err := fmt.Fprintf(h, "%s:%d\n", filepath.Base(p), fi.Size()); err != nil {
			return "", err
		}
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func fileFingerprint(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer func() {
		if cerr := f.Close(); cerr != nil {
			slog.Warn("audiobook: fingerprint file close failed", "path", path, "err", cerr)
		}
	}()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", fmt.Errorf("hash: %w", err)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func sortAudiobookFilesByTrack(files []string) []string {
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
	sorted := make([]string, len(tagged))
	for i, ft := range tagged {
		sorted[i] = ft.path
	}
	return sorted
}

// ── Main ingest logic ─────────────────────────────────────────────────────────

type ingestOptions struct {
	forceID                  string
	allowExistingFingerprint bool
}

func (g *AudiobookIngester) ingestFile(ctx context.Context, path string, fi os.FileInfo) (string, error) {
	return g.ingestFileWithOptions(ctx, path, fi, ingestOptions{})
}

func (g *AudiobookIngester) ingestFileWithOptions(ctx context.Context, path string, fi os.FileInfo, opts ingestOptions) (string, error) {
	fingerprint, err := fileFingerprint(path)
	if err != nil {
		return "", err
	}

	// If we've already ingested this exact file content, skip unless forced.
	existingID, lookupErr := g.db.GetAudiobookByFingerprint(ctx, fingerprint)
	if lookupErr == nil && existingID != "" && !opts.allowExistingFingerprint {
		return existingID, ErrSkipped
	}
	if opts.forceID != "" && existingID != "" && existingID != opts.forceID {
		return "", fmt.Errorf("fingerprint mismatch for reingest: existing_id=%s", existingID)
	}

	audiobookID := opts.forceID
	if audiobookID == "" {
		audiobookID = deterministicUUID(fingerprint)
	}

	// ── Read tags (title, author, etc.) ──────────────────────────────────────
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	m, tagErr := tag.ReadFrom(f)
	_ = f.Close()

	var title, authorName, narratorName string
	var edition *string
	var publishedYear *int
	var seriesName string
	var seriesIndex *float64
	var seriesSource string
	var seriesConfidence float64
	var seriesFromTagsValue string
	var seriesIndexFromTags *float64
	if tagErr == nil {
		title = m.Title()
		if title != "" {
			title, edition = stripEditionSuffix(title)
		}
		authorName = coalesce(m.AlbumArtist(), m.Artist())
		raw := m.Raw()
		// Common narrator tags: "narrator", "©nrt", "TXXX:NARRATOR"
		narratorName = rawTagString(raw,
			"narrator", "narrated_by", "©nrt",
			"TXXX:NARRATOR", "----:com.apple.iTunes:NARRATOR",
			"com.apple.iTunes:narrator",
		)
		if s, idx := seriesFromTags(raw); s != "" {
			seriesFromTagsValue = s
			seriesIndexFromTags = idx
		} else if idx != nil {
			seriesIndex = idx
		}
	}
	if title == "" {
		title = strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	}
	if title != "" {
		if cleaned, ed := stripEditionSuffix(title); ed != nil {
			title = cleaned
			if edition == nil {
				edition = ed
			}
		}
	}
	if seriesFromTagsValue != "" && !isSuspiciousSeries(seriesFromTagsValue, title) {
		seriesName = seriesFromTagsValue
		seriesSource = "metadata"
		seriesConfidence = 1.0
		seriesIndex = seriesIndexFromTags
	}
	parentDir := filepath.Base(filepath.Dir(path))
	parentPath := filepath.Dir(path)
	seriesFromParent, authorFromParent, narratorFromParent := parseSeriesDirName(parentDir)
	if authorName == "" {
		authorName = authorFromParent
	}
	if narratorName == "" {
		narratorName = narratorFromParent
	}

	baseName := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	// Strip folder-name annotations before pattern-matching the title.
	baseName, narratorsFromBraces := parseNarratorsFromBraces(baseName)
	baseName, narratorFromParens := parseNarratorFromParens(baseName)
	baseName, asinFromName := parseASINFromFolder(baseName)
	baseName, yearFromName := parseYearPrefixFromFolder(baseName)
	if narratorName == "" && len(narratorsFromBraces) > 0 {
		narratorName = narratorsFromBraces[0]
	}
	if narratorName == "" && narratorFromParens != "" {
		narratorName = narratorFromParens
	}
	if publishedYear == nil && yearFromName != "" {
		if y, err := strconv.Atoi(yearFromName); err == nil && y > 0 {
			publishedYear = &y
		}
	}
	_, seriesIndexFromName, editionFromName := parseBookDirName(baseName)
	if seriesIndex == nil && seriesIndexFromName != nil {
		seriesIndex = seriesIndexFromName
	}
	if edition == nil && editionFromName != nil {
		edition = editionFromName
	}
	// If the file name doesn't carry a series index, fall back to the book folder.
	if seriesIndex == nil {
		_, seriesIndexFromParentDir, editionFromParentDir := parseBookDirName(parentDir)
		if seriesIndexFromParentDir != nil {
			seriesIndex = seriesIndexFromParentDir
		}
		if edition == nil && editionFromParentDir != nil {
			edition = editionFromParentDir
		}
	}
	// If the parent dir is a "Series 01 Title" pattern, extract the series prefix.
	if seriesIndex != nil && seriesFromParent == "" {
		if s := parseSeriesPrefixFromBookDirName(parentDir); s != "" {
			seriesFromParent = s
		}
	}
	// If the parent directory looks like a book folder (has a series index),
	// treat the grandparent as the series name (single-file mode layout).
	// Also handle year-prefixed parent dirs (e.g. "1989 - Hyperion") as implicit
	// series ordering — the grandparent ("Hyperion Cantos") is the series.
	_, parentYearStr := parseYearPrefixFromFolder(parentDir)
	_, idxFromParentDir, _ := parseBookDirName(parentDir)
	parentHasYearOrdering := parentYearStr != "" && idxFromParentDir == nil
	usedGrandparentSeries := false
	if idxFromParentDir != nil || parentHasYearOrdering {
		grandparentDir := filepath.Base(filepath.Dir(parentPath))
		if isValidDirName(grandparentDir) && !isSeriesRootDir(grandparentDir, g.cfg.Dirs) {
			seriesFromParent = grandparentDir
			usedGrandparentSeries = true
		}
	} else if seriesIndex != nil && seriesFromParent == "" && isValidDirName(parentDir) {
		seriesFromParent = parentDir
	}
	// If the parent dir looks like an "Author - Title" folder, prefer the
	// grandparent (series root) instead of reusing the book title as series.
	if !usedGrandparentSeries && seriesFromParent != "" && isSeriesTitleMatch(seriesFromParent, title) {
		grandparentDir := filepath.Base(filepath.Dir(parentPath))
		if isValidDirName(grandparentDir) && !isSeriesRootDir(grandparentDir, g.cfg.Dirs) {
			seriesFromParent = grandparentDir
			usedGrandparentSeries = true
		}
	}
	if seriesName == "" && seriesFromParent != "" && (seriesIndex != nil || usedGrandparentSeries || parentHasYearOrdering) {
		seriesName = cleanSeriesName(seriesFromParent)
		seriesSource = "folder"
		seriesConfidence = 0.7
	}
	slog.Info("audiobook series resolved (single-file)",
		"path", path,
		"title", title,
		"parent_dir", parentDir,
		"grandparent_dir", filepath.Base(filepath.Dir(parentPath)),
		"series_from_tags", seriesFromTagsValue,
		"series_from_parent", seriesFromParent,
		"series_index", seriesIndex,
		"used_grandparent", usedGrandparentSeries,
		"series_final", seriesName,
		"series_source", seriesSource,
		"series_confidence", seriesConfidence,
	)

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
				if cerr := rf.Close(); cerr != nil {
					slog.Warn("audiobook: close after reading tags", "path", path, "err", cerr)
				}
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
			if cerr := rf.Close(); cerr != nil {
				slog.Warn("audiobook: close after failed put", "path", path, "err", cerr)
			}
			return "", fmt.Errorf("store audiobook: %w", err2)
		}
		if cerr := rf.Close(); cerr != nil {
			slog.Warn("audiobook: close after store", "path", path, "err", cerr)
		}
	}

	// ── Upsert audiobook ──────────────────────────────────────────────────
	var seriesSourcePtr *string
	var seriesConfidencePtr *float64
	var seriesPtr *string
	if seriesName != "" {
		seriesPtr = &seriesName
		seriesSourcePtr = &seriesSource
		seriesConfidencePtr = &seriesConfidence
	}

	var asinPtr *string
	if asinFromName != "" {
		asinPtr = &asinFromName
	}
	_, err = g.db.UpsertAudiobook(ctx, store.UpsertAudiobookParams{
		ID:               audiobookID,
		Title:            title,
		Edition:          edition,
		AuthorID:         &authorID,
		CoverArtKey:      coverArtKeyPtr,
		Series:           seriesPtr,
		SeriesIndex:      seriesIndex,
		SeriesSource:     seriesSourcePtr,
		SeriesConfidence: seriesConfidencePtr,
		PublishedYear:    publishedYear,
		ASIN:             asinPtr,
		FileKey:          &fileKey,
		FileSize:         fi.Size(),
		Format:           ext,
		DurationMs:       durationMs,
		Fingerprint:      fingerprint,
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
			chapTitle := normalizeChapterTitle(rc.title, i+1)
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

func normalizeChapterTitle(title string, fallbackNum int) string {
	t := strings.TrimSpace(title)
	if t == "" {
		return fmt.Sprintf("Chapter %d", fallbackNum)
	}
	isNumeric := true
	for i := 0; i < len(t); i++ {
		if t[i] < '0' || t[i] > '9' {
			isNumeric = false
			break
		}
	}
	if isNumeric {
		if n, err := strconv.Atoi(t); err == nil && n > 0 {
			return fmt.Sprintf("Chapter %d", n)
		}
		return fmt.Sprintf("Chapter %d", fallbackNum)
	}
	return t
}

// ingestDirectory handles a directory of audio files where each file is a chapter.
func (g *AudiobookIngester) ingestDirectory(ctx context.Context, cand audiobookCandidate) (string, error) {
	return g.ingestDirectoryWithOptions(ctx, cand, ingestOptions{})
}

func (g *AudiobookIngester) ingestDirectoryWithOptions(ctx context.Context, cand audiobookCandidate, opts ingestOptions) (string, error) {
	files := cand.files
	if len(files) == 0 {
		return "", fmt.Errorf("no audio files in directory: %s", cand.dirPath)
	}

	// ── Sort files by track number from tag, fallback to filename ─────────
	sortedFiles := sortAudiobookFilesByTrack(files)

	// ── Compute fingerprint for dedup ─────────────────────────────────────
	fingerprint, err := dirFingerprint(sortedFiles)
	if err != nil {
		return "", fmt.Errorf("fingerprint: %w", err)
	}

	existingID, lookupErr := g.db.GetAudiobookByFingerprint(ctx, fingerprint)
	if lookupErr == nil && existingID != "" && !opts.allowExistingFingerprint {
		return existingID, ErrSkipped
	}
	if opts.forceID != "" && existingID != "" && existingID != opts.forceID {
		return "", fmt.Errorf("fingerprint mismatch for reingest: existing_id=%s", existingID)
	}

	audiobookID := opts.forceID
	if audiobookID == "" {
		audiobookID = deterministicUUID(fingerprint)
	}

	// ── Read book-level metadata from first file ──────────────────────────
	firstFile := sortedFiles[0]
	var bookTitle, authorName, narratorName string
	var edition *string
	var publishedYear *int
	var seriesName string
	var seriesIndex *float64
	var seriesSource string
	var seriesConfidence float64
	var seriesFromTagsValue string
	var seriesIndexFromTags *float64

	if f, err := os.Open(firstFile); err == nil {
		if m, err := tag.ReadFrom(f); err == nil {
			// album → book title
			bookTitle = m.Album()
			if bookTitle != "" {
				bookTitle, edition = stripEditionSuffix(bookTitle)
			}
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
			if s, idx := seriesFromTags(raw); s != "" {
				seriesFromTagsValue = s
				seriesIndexFromTags = idx
			} else if idx != nil {
				seriesIndex = idx
			}
			// date → published year
			if yr := rawTagString(raw, "date", "TDRC", "TYER", "©day"); yr != "" {
				if y, err := strconv.Atoi(strings.TrimSpace(yr[:min4(len(yr), 4)])); err == nil && y > 0 {
					publishedYear = &y
				}
			}
		}
		if cerr := f.Close(); cerr != nil {
			slog.Warn("audiobook: metadata file close failed", "path", firstFile, "err", cerr)
		}
	}

	// ── Parse parent directory name for series/author/narrator fallbacks ──
	parentPath := filepath.Dir(cand.dirPath)
	parentDir := filepath.Base(parentPath)
	seriesFromParent, authorFromParent, narratorFromParent := parseSeriesDirName(parentDir)

	if authorName == "" {
		authorName = authorFromParent
	}
	if narratorName == "" {
		narratorName = narratorFromParent
	}

	// ── Parse book directory name for series index + title ────────────────
	bookDirBase := filepath.Base(cand.dirPath)
	// Strip folder-name annotations before pattern-matching the title.
	bookDirBase, narratorsFromDir := parseNarratorsFromBraces(bookDirBase)
	bookDirBase, narratorFromParens := parseNarratorFromParens(bookDirBase)
	bookDirBase, asinFromDir := parseASINFromFolder(bookDirBase)
	bookDirBase, yearFromDir := parseYearPrefixFromFolder(bookDirBase)
	if narratorName == "" && len(narratorsFromDir) > 0 {
		narratorName = narratorsFromDir[0]
	}
	if narratorName == "" && narratorFromParens != "" {
		narratorName = narratorFromParens
	}
	if publishedYear == nil && yearFromDir != "" {
		if y, err := strconv.Atoi(yearFromDir); err == nil && y > 0 {
			publishedYear = &y
		}
	}
	titleFromDir, seriesIndexFromDir, editionFromDir := parseBookDirName(bookDirBase)
	if seriesIndex == nil && seriesIndexFromDir != nil {
		seriesIndex = seriesIndexFromDir
	}
	if edition == nil && editionFromDir != nil {
		edition = editionFromDir
	}
	if seriesIndex != nil && seriesFromParent == "" {
		if s := parseSeriesPrefixFromBookDirName(bookDirBase); s != "" {
			seriesFromParent = s
		}
	}

	// Resolve final title: prefer album tag, then dir parse.
	if bookTitle == "" {
		bookTitle = titleFromDir
	}
	if bookTitle == "" {
		bookTitle = bookDirBase
	}
	if bookTitle != "" {
		if cleaned, ed := stripEditionSuffix(bookTitle); ed != nil {
			bookTitle = cleaned
			if edition == nil {
				edition = ed
			}
		}
	}

	if seriesFromTagsValue != "" && !isSuspiciousSeries(seriesFromTagsValue, bookTitle) {
		seriesName = seriesFromTagsValue
		seriesSource = "metadata"
		seriesConfidence = 1.0
		seriesIndex = seriesIndexFromTags
	}

	// ── Deeper directory fallbacks for author and series ──────────────────
	// Handles two common library layouts:
	//   3-level: Author/Series/Book N - Title/  → grandparent=Author, parent=Series
	//   2-level: Author/Book Title/             → parent=Author
	//
	// Year-prefixed layouts (e.g. "1989 - Hyperion") also imply ordering:
	// the parent dir is the series, not the author.
	hasYearOrdering := yearFromDir != "" && seriesIndex == nil
	if (seriesIndex != nil || hasYearOrdering) && seriesFromParent == "" && isValidDirName(parentDir) {
		seriesFromParent = parentDir
	}

	if authorName == "" {
		if seriesIndex != nil || hasYearOrdering {
			// Book has an index or year ordering → likely 3-level: grandparent is
			// the author, parent is the series name.
			// Guard: if grandparent is the library root there is no author layer.
			grandparentDir := filepath.Base(filepath.Dir(parentPath))
			if isValidDirName(grandparentDir) && !isSeriesRootDir(grandparentDir, g.cfg.Dirs) {
				_, grandAuthor, grandNarrator := parseSeriesDirName(grandparentDir)
				if grandAuthor != "" {
					authorName = grandAuthor
				} else {
					authorName = grandparentDir
				}
				if narratorName == "" && grandNarrator != "" {
					narratorName = grandNarrator
				}
			}
		} else if isValidDirName(parentDir) {
			// No series index or year ordering → likely 2-level: parent dir is the author.
			authorName = parentDir
		}
	}

	// Resolve series: use parent dir series if we successfully parsed a book index.
	var seriesPtr *string
	var seriesSourcePtr *string
	var seriesConfidencePtr *float64
	usedGrandparentSeries := false
	if seriesName == "" && seriesFromParent != "" && isSeriesTitleMatch(seriesFromParent, bookTitle) {
		grandparentDir := filepath.Base(filepath.Dir(parentPath))
		if isValidDirName(grandparentDir) && !isSeriesRootDir(grandparentDir, g.cfg.Dirs) {
			seriesFromParent = grandparentDir
			usedGrandparentSeries = true
		}
	}
	if seriesName == "" && seriesFromParent != "" && (seriesIndex != nil || usedGrandparentSeries || hasYearOrdering) {
		seriesName = cleanSeriesName(seriesFromParent)
		seriesSource = "folder"
		seriesConfidence = 0.7
	}
	slog.Info("audiobook series resolved (directory)",
		"dir", cand.dirPath,
		"title", bookTitle,
		"parent_dir", parentDir,
		"series_from_tags", seriesFromTagsValue,
		"series_from_parent", seriesFromParent,
		"series_index", seriesIndex,
		"series_final", seriesName,
		"series_source", seriesSource,
		"series_confidence", seriesConfidence,
	)
	if seriesName != "" {
		seriesPtr = &seriesName
		seriesSourcePtr = &seriesSource
		seriesConfidencePtr = &seriesConfidence
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
			if cerr := f.Close(); cerr != nil {
				slog.Warn("audiobook: cover art file close failed", "path", firstFile, "err", cerr)
			}
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
				if cerr := cf.Close(); cerr != nil {
					slog.Warn("audiobook: close chapter file failed", "path", p, "err", cerr)
				}
			}
		}

		// Get chapter duration.
		chapDurationMs, err := probeFileDuration(p)
		if err != nil {
			slog.Warn("audiobook chapter probe failed", "path", p, "err", err)
			chapDurationMs = 0
		}

		// Determine chapter title: prefer tagged title, then parse from filename.
		nameNoExt := strings.TrimSuffix(filepath.Base(p), filepath.Ext(p))
		chapTitle := chapterTitleFromFilename(nameNoExt)
		if f, err := os.Open(p); err == nil {
			if m, err := tag.ReadFrom(f); err == nil {
				if t := m.Title(); t != "" {
					chapTitle = t
				}
			}
			if cerr := f.Close(); cerr != nil {
				slog.Warn("audiobook: close chapter tag file failed", "path", p, "err", cerr)
			}
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

	// ── Compute total file size and predominant format ────────────────────
	var totalFileSize int64
	fmtCounts := make(map[string]int)
	for _, p := range sortedFiles {
		if fi, err := os.Stat(p); err == nil {
			totalFileSize += fi.Size()
		}
		fmtCounts[strings.TrimPrefix(strings.ToLower(filepath.Ext(p)), ".")]++
	}
	predominantFmt := "mp3"
	maxFmtCount := 0
	for ext, cnt := range fmtCounts {
		if cnt > maxFmtCount {
			maxFmtCount = cnt
			predominantFmt = ext
		}
	}

	// ── Upsert audiobook (FileKey = nil for multi-file) ───────────────────
	params := store.UpsertAudiobookParams{
		ID:               audiobookID,
		Title:            bookTitle,
		Edition:          edition,
		AuthorID:         &authorID,
		CoverArtKey:      coverArtKeyPtr,
		FileKey:          nil, // multi-file: no single file key
		FileSize:         totalFileSize,
		Format:           predominantFmt,
		DurationMs:       totalDurationMs,
		Fingerprint:      fingerprint,
		Series:           seriesPtr,
		SeriesIndex:      seriesIndex,
		SeriesSource:     seriesSourcePtr,
		SeriesConfidence: seriesConfidencePtr,
	}
	if publishedYear != nil {
		params.PublishedYear = publishedYear
	}
	if asinFromDir != "" {
		params.ASIN = &asinFromDir
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

	if len(result.Series) > 0 {
		seriesName := cleanSeriesName(result.Series[0])
		if seriesName != "" {
			if err := g.db.UpdateAudiobookSeriesFromLookup(ctx, audiobookID, seriesName, 0.5); err != nil {
				slog.Warn("audiobook series update failed", "audiobook_id", audiobookID, "err", err)
			}
		}
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
	defer func() {
		if cerr := pr.Close(); cerr != nil {
			slog.Warn("audiobook: cover art pipe close failed", "err", cerr)
		}
	}()
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

// TriggerForceScan clears ingest state so all files are re-processed.
func (s *AudiobookIngestService) TriggerForceScan(ctx context.Context) error {
	if s.running.Load() {
		return errors.New("scan already in progress")
	}
	if err := s.ingester.db.ClearAudiobookIngestState(ctx); err != nil {
		return err
	}
	s.ingester.ClearState()
	return s.TriggerScan(ctx)
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

// TriggerReingestAudiobook clears the ingest state for a specific audiobook and
// re-ingests it in the background. Returns an error if a scan is already running.
func (s *AudiobookIngestService) TriggerReingestAudiobook(audiobookID string) error {
	if !s.running.CompareAndSwap(false, true) {
		return errors.New("scan already in progress")
	}
	go func() {
		defer s.running.Store(false)
		s.ingester.ReingestAudiobook(s.rootCtx, audiobookID)
	}()
	return nil
}
