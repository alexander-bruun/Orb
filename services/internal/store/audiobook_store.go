package store

import (
	"context"
	"database/sql"
	"fmt"
)

// ── Audiobook store methods ───────────────────────────────────────────────────

// UpsertAudiobook inserts or updates an audiobook record.
func (s *Store) UpsertAudiobook(ctx context.Context, p UpsertAudiobookParams) (Audiobook, error) {
	var a Audiobook
	row := s.pool.QueryRow(ctx, `
		INSERT INTO audiobooks
			(id, title, subtitle, edition, author_id, cover_art_key, description, series, series_index,
			 series_source, series_confidence, published_year, isbn, asin, ol_key, file_key,
			 file_size, format, duration_ms, fingerprint)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20)
		ON CONFLICT (id) DO UPDATE SET
			title          = EXCLUDED.title,
			subtitle       = COALESCE(EXCLUDED.subtitle, audiobooks.subtitle),
			edition        = COALESCE(EXCLUDED.edition, audiobooks.edition),
			author_id      = EXCLUDED.author_id,
			cover_art_key  = COALESCE(EXCLUDED.cover_art_key, audiobooks.cover_art_key),
			description    = COALESCE(EXCLUDED.description, audiobooks.description),
			series         = COALESCE(EXCLUDED.series, audiobooks.series),
			series_index   = COALESCE(EXCLUDED.series_index, audiobooks.series_index),
			series_source  = COALESCE(EXCLUDED.series_source, audiobooks.series_source),
			series_confidence = COALESCE(EXCLUDED.series_confidence, audiobooks.series_confidence),
			published_year = EXCLUDED.published_year,
			isbn           = EXCLUDED.isbn,
			asin           = COALESCE(EXCLUDED.asin, audiobooks.asin),
			ol_key         = COALESCE(EXCLUDED.ol_key, audiobooks.ol_key),
			file_key       = COALESCE(EXCLUDED.file_key, audiobooks.file_key),
			file_size      = EXCLUDED.file_size,
			format         = EXCLUDED.format,
			duration_ms    = EXCLUDED.duration_ms,
			fingerprint    = EXCLUDED.fingerprint
		RETURNING id, title, subtitle, edition, author_id, cover_art_key, description, series, series_index,
		          series_source, series_confidence, published_year, isbn, asin, ol_key, file_key,
		          file_size, format, duration_ms, fingerprint, created_at`,
		p.ID, p.Title, p.Subtitle, p.Edition, p.AuthorID, p.CoverArtKey, p.Description, p.Series, p.SeriesIndex,
		p.SeriesSource, p.SeriesConfidence, p.PublishedYear, p.ISBN, p.ASIN, p.OLKey, p.FileKey,
		p.FileSize, p.Format, p.DurationMs, p.Fingerprint,
	)
	var subtitle, edition, authorID, coverArtKey, description, series, seriesSource, isbn, asin, olKey, fileKey, fingerprint sql.NullString
	var seriesIndex, seriesConfidence sql.NullFloat64
	var publishedYear sql.NullInt32
	err := row.Scan(
		&a.ID, &a.Title, &subtitle, &edition, &authorID, &coverArtKey, &description, &series, &seriesIndex,
		&seriesSource, &seriesConfidence, &publishedYear, &isbn, &asin, &olKey, &fileKey,
		&a.FileSize, &a.Format, &a.DurationMs, &fingerprint, &a.CreatedAt,
	)
	if err != nil {
		return a, err
	}
	if subtitle.Valid {
		a.Subtitle = &subtitle.String
	}
	if edition.Valid {
		a.Edition = &edition.String
	}
	if authorID.Valid {
		a.AuthorID = &authorID.String
	}
	if coverArtKey.Valid {
		a.CoverArtKey = &coverArtKey.String
	}
	if description.Valid {
		a.Description = &description.String
	}
	if series.Valid {
		a.Series = &series.String
	}
	if seriesIndex.Valid {
		a.SeriesIndex = &seriesIndex.Float64
	}
	if seriesSource.Valid {
		a.SeriesSource = &seriesSource.String
	}
	if seriesConfidence.Valid {
		a.SeriesConfidence = &seriesConfidence.Float64
	}
	if publishedYear.Valid {
		v := int(publishedYear.Int32)
		a.PublishedYear = &v
	}
	if isbn.Valid {
		a.ISBN = &isbn.String
	}
	if asin.Valid {
		a.ASIN = &asin.String
	}
	if olKey.Valid {
		a.OLKey = &olKey.String
	}
	if fileKey.Valid {
		a.FileKey = &fileKey.String
	}
	if fingerprint.Valid {
		a.Fingerprint = fingerprint.String
	}
	return a, nil
}

// GetAudiobook returns a single audiobook by ID, including chapters and narrators.
func (s *Store) GetAudiobook(ctx context.Context, id string) (Audiobook, error) {
	var a Audiobook
	row := s.pool.QueryRow(ctx, `
		SELECT ab.id, ab.title, ab.subtitle, ab.edition, ab.author_id, ar.name,
		       ab.cover_art_key, ab.description, ab.series, ab.series_index, ab.series_source, ab.series_confidence,
		       ab.published_year, ab.isbn, ab.asin, ab.ol_key,
		       ab.file_key, ab.file_size, ab.format, ab.duration_ms, ab.fingerprint, ab.created_at
		FROM audiobooks ab
		LEFT JOIN artists ar ON ar.id = ab.author_id
		WHERE ab.id = $1`, id)
	var subtitle, edition, authorID, authorName, coverArtKey, description, series, seriesSource, isbn, asin, olKey, fileKey, fingerprint sql.NullString
	var seriesIndex, seriesConfidence sql.NullFloat64
	var publishedYear sql.NullInt32
	err := row.Scan(
		&a.ID, &a.Title, &subtitle, &edition, &authorID, &authorName,
		&coverArtKey, &description, &series, &seriesIndex, &seriesSource, &seriesConfidence,
		&publishedYear, &isbn, &asin, &olKey,
		&fileKey, &a.FileSize, &a.Format, &a.DurationMs, &fingerprint, &a.CreatedAt,
	)
	if err != nil {
		return a, err
	}
	if subtitle.Valid {
		a.Subtitle = &subtitle.String
	}
	if edition.Valid {
		a.Edition = &edition.String
	}
	if authorID.Valid {
		a.AuthorID = &authorID.String
	}
	if authorName.Valid {
		a.AuthorName = &authorName.String
	}
	if coverArtKey.Valid {
		a.CoverArtKey = &coverArtKey.String
	}
	if description.Valid {
		a.Description = &description.String
	}
	if series.Valid {
		a.Series = &series.String
	}
	if seriesIndex.Valid {
		a.SeriesIndex = &seriesIndex.Float64
	}
	if seriesSource.Valid {
		a.SeriesSource = &seriesSource.String
	}
	if seriesConfidence.Valid {
		a.SeriesConfidence = &seriesConfidence.Float64
	}
	if publishedYear.Valid {
		v := int(publishedYear.Int32)
		a.PublishedYear = &v
	}
	if isbn.Valid {
		a.ISBN = &isbn.String
	}
	if asin.Valid {
		a.ASIN = &asin.String
	}
	if olKey.Valid {
		a.OLKey = &olKey.String
	}
	if fileKey.Valid {
		a.FileKey = &fileKey.String
	}
	if fingerprint.Valid {
		a.Fingerprint = fingerprint.String
	}

	chapters, err := s.GetAudiobookChapters(ctx, id)
	if err != nil {
		return a, err
	}
	a.Chapters = chapters

	narrators, err := s.GetAudiobookNarrators(ctx, id)
	if err != nil {
		return a, err
	}
	a.Narrators = narrators

	return a, nil
}

// ListAudiobooks returns a paginated list of audiobooks with author names.
func (s *Store) ListAudiobooks(ctx context.Context, p ListAudiobooksParams) ([]Audiobook, error) {
	var orderBy string
	switch p.SortBy {
	case "author":
		orderBy = "ar.name NULLS LAST, ab.title"
	case "year":
		orderBy = "ab.published_year DESC NULLS LAST, ab.title"
	case "series":
		orderBy = "ab.series NULLS LAST, ab.series_index NULLS LAST, ab.title"
	default:
		orderBy = "ab.title"
	}
	q := fmt.Sprintf(`
		SELECT ab.id, ab.title, ab.edition, ab.author_id, ar.name,
		       ab.cover_art_key, ab.series, ab.series_index, ab.series_source, ab.series_confidence,
		       ab.published_year, ab.duration_ms, ab.format, ab.created_at
		FROM audiobooks ab
		LEFT JOIN artists ar ON ar.id = ab.author_id
		ORDER BY %s
		LIMIT $1 OFFSET $2`, orderBy)
	rows, err := s.pool.Query(ctx, q, p.Limit, p.Offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []Audiobook
	for rows.Next() {
		var a Audiobook
		var edition, authorID, authorName, coverArtKey, series, seriesSource sql.NullString
		var seriesIndex, seriesConfidence sql.NullFloat64
		var publishedYear sql.NullInt32
		if err := rows.Scan(
			&a.ID, &a.Title, &edition, &authorID, &authorName,
			&coverArtKey, &series, &seriesIndex, &seriesSource, &seriesConfidence,
			&publishedYear, &a.DurationMs, &a.Format, &a.CreatedAt,
		); err != nil {
			return nil, err
		}
		if edition.Valid {
			a.Edition = &edition.String
		}
		if authorID.Valid {
			a.AuthorID = &authorID.String
		}
		if authorName.Valid {
			a.AuthorName = &authorName.String
		}
		if coverArtKey.Valid {
			a.CoverArtKey = &coverArtKey.String
		}
		if series.Valid {
			a.Series = &series.String
		}
		if seriesIndex.Valid {
			a.SeriesIndex = &seriesIndex.Float64
		}
		if seriesSource.Valid {
			a.SeriesSource = &seriesSource.String
		}
		if seriesConfidence.Valid {
			a.SeriesConfidence = &seriesConfidence.Float64
		}
		if publishedYear.Valid {
			v := int(publishedYear.Int32)
			a.PublishedYear = &v
		}
		result = append(result, a)
	}
	return result, rows.Err()
}

// GetAudiobookChapters returns all chapters for an audiobook ordered by chapter_num.
func (s *Store) GetAudiobookChapters(ctx context.Context, audiobookID string) ([]AudiobookChapter, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, audiobook_id, title, start_ms, end_ms, chapter_num, file_key
		FROM audiobook_chapters
		WHERE audiobook_id = $1
		ORDER BY chapter_num`, audiobookID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []AudiobookChapter
	for rows.Next() {
		var c AudiobookChapter
		var fileKey sql.NullString
		if err := rows.Scan(&c.ID, &c.AudiobookID, &c.Title, &c.StartMs, &c.EndMs, &c.ChapterNum, &fileKey); err != nil {
			return nil, err
		}
		if fileKey.Valid {
			c.FileKey = &fileKey.String
		}
		result = append(result, c)
	}
	return result, rows.Err()
}

// GetAudiobookChapterByID returns a single chapter by its ID (for chapter-file streaming).
func (s *Store) GetAudiobookChapterByID(ctx context.Context, chapterID string) (AudiobookChapter, error) {
	var c AudiobookChapter
	var fileKey sql.NullString
	err := s.pool.QueryRow(ctx, `
		SELECT id, audiobook_id, title, start_ms, end_ms, chapter_num, file_key
		FROM audiobook_chapters WHERE id = $1`, chapterID).
		Scan(&c.ID, &c.AudiobookID, &c.Title, &c.StartMs, &c.EndMs, &c.ChapterNum, &fileKey)
	if err != nil {
		return c, err
	}
	if fileKey.Valid {
		c.FileKey = &fileKey.String
	}
	return c, nil
}

// ReplaceAudiobookChapters deletes existing chapters and inserts the new set atomically.
func (s *Store) ReplaceAudiobookChapters(ctx context.Context, audiobookID string, chapters []AudiobookChapter) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer rollbackTx(ctx, tx)

	if _, err := tx.Exec(ctx, `DELETE FROM audiobook_chapters WHERE audiobook_id = $1`, audiobookID); err != nil {
		return err
	}
	for _, c := range chapters {
		if _, err := tx.Exec(ctx, `
			INSERT INTO audiobook_chapters (id, audiobook_id, title, start_ms, end_ms, chapter_num, file_key)
			VALUES ($1, $2, $3, $4, $5, $6, $7)`,
			c.ID, audiobookID, c.Title, c.StartMs, c.EndMs, c.ChapterNum, c.FileKey,
		); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

// UpsertAudiobookNarrator inserts or updates a narrator.
func (s *Store) UpsertAudiobookNarrator(ctx context.Context, p UpsertAudiobookNarratorParams) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO audiobook_narrators (id, name, sort_name)
		VALUES ($1, $2, $3)
		ON CONFLICT (id) DO UPDATE SET name = EXCLUDED.name, sort_name = EXCLUDED.sort_name`,
		p.ID, p.Name, p.SortName)
	return err
}

// SetAudiobookNarrators replaces all narrator links for an audiobook.
func (s *Store) SetAudiobookNarrators(ctx context.Context, audiobookID string, narratorIDs []string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer rollbackTx(ctx, tx)

	if _, err := tx.Exec(ctx, `DELETE FROM audiobook_narrator_links WHERE audiobook_id = $1`, audiobookID); err != nil {
		return err
	}
	for i, nid := range narratorIDs {
		if _, err := tx.Exec(ctx, `
			INSERT INTO audiobook_narrator_links (audiobook_id, narrator_id, position)
			VALUES ($1, $2, $3)`, audiobookID, nid, i); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

// GetAudiobookNarrators returns all narrators for an audiobook.
func (s *Store) GetAudiobookNarrators(ctx context.Context, audiobookID string) ([]AudiobookNarrator, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT n.id, n.name, n.sort_name, n.image_key, n.created_at
		FROM audiobook_narrators n
		JOIN audiobook_narrator_links l ON l.narrator_id = n.id
		WHERE l.audiobook_id = $1
		ORDER BY l.position`, audiobookID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []AudiobookNarrator
	for rows.Next() {
		var n AudiobookNarrator
		var imageKey sql.NullString
		if err := rows.Scan(&n.ID, &n.Name, &n.SortName, &imageKey, &n.CreatedAt); err != nil {
			return nil, err
		}
		if imageKey.Valid {
			n.ImageKey = &imageKey.String
		}
		result = append(result, n)
	}
	return result, rows.Err()
}

// GetAudiobookProgress returns the playback progress for a user/audiobook pair.
func (s *Store) GetAudiobookProgress(ctx context.Context, userID, audiobookID string) (AudiobookProgress, error) {
	var p AudiobookProgress
	err := s.pool.QueryRow(ctx, `
		SELECT user_id, audiobook_id, position_ms, completed, updated_at
		FROM audiobook_progress
		WHERE user_id = $1 AND audiobook_id = $2`, userID, audiobookID).
		Scan(&p.UserID, &p.AudiobookID, &p.PositionMs, &p.Completed, &p.UpdatedAt)
	return p, err
}

// UpsertAudiobookProgress inserts or updates playback progress.
func (s *Store) UpsertAudiobookProgress(ctx context.Context, p UpsertAudiobookProgressParams) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO audiobook_progress (user_id, audiobook_id, position_ms, completed, updated_at)
		VALUES ($1, $2, $3, $4, now())
		ON CONFLICT (user_id, audiobook_id) DO UPDATE SET
			position_ms = EXCLUDED.position_ms,
			completed   = EXCLUDED.completed,
			updated_at  = now()`,
		p.UserID, p.AudiobookID, p.PositionMs, p.Completed)
	return err
}

// ListAudiobookBookmarks returns all bookmarks for a user/audiobook pair.
func (s *Store) ListAudiobookBookmarks(ctx context.Context, userID, audiobookID string) ([]AudiobookBookmark, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, user_id, audiobook_id, position_ms, note, created_at
		FROM audiobook_bookmarks
		WHERE user_id = $1 AND audiobook_id = $2
		ORDER BY position_ms`, userID, audiobookID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []AudiobookBookmark
	for rows.Next() {
		var b AudiobookBookmark
		var note sql.NullString
		if err := rows.Scan(&b.ID, &b.UserID, &b.AudiobookID, &b.PositionMs, &note, &b.CreatedAt); err != nil {
			return nil, err
		}
		if note.Valid {
			b.Note = &note.String
		}
		result = append(result, b)
	}
	return result, rows.Err()
}

// CreateAudiobookBookmark inserts a new bookmark and returns it.
func (s *Store) CreateAudiobookBookmark(ctx context.Context, p CreateAudiobookBookmarkParams) (AudiobookBookmark, error) {
	var b AudiobookBookmark
	var note sql.NullString
	err := s.pool.QueryRow(ctx, `
		INSERT INTO audiobook_bookmarks (id, user_id, audiobook_id, position_ms, note)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, user_id, audiobook_id, position_ms, note, created_at`,
		p.ID, p.UserID, p.AudiobookID, p.PositionMs, p.Note,
	).Scan(&b.ID, &b.UserID, &b.AudiobookID, &b.PositionMs, &note, &b.CreatedAt)
	if note.Valid {
		b.Note = &note.String
	}
	return b, err
}

// DeleteAudiobookBookmark removes a bookmark, verifying ownership.
func (s *Store) DeleteAudiobookBookmark(ctx context.Context, id, userID string) error {
	_, err := s.pool.Exec(ctx, `
		DELETE FROM audiobook_bookmarks WHERE id = $1 AND user_id = $2`, id, userID)
	return err
}

// UpdateAudiobookCoverArt sets the cover_art_key for an audiobook.
func (s *Store) UpdateAudiobookCoverArt(ctx context.Context, audiobookID, coverKey string) error {
	_, err := s.pool.Exec(ctx, `UPDATE audiobooks SET cover_art_key = $2 WHERE id = $1`, audiobookID, coverKey)
	return err
}

// UpdateAudiobookDuration updates the stored duration for an audiobook.
func (s *Store) UpdateAudiobookDuration(ctx context.Context, audiobookID string, durationMs int64) error {
	_, err := s.pool.Exec(ctx, `UPDATE audiobooks SET duration_ms = $2 WHERE id = $1`, audiobookID, durationMs)
	return err
}

// UpdateAudiobookEnrichment sets Open Library metadata fields.
func (s *Store) UpdateAudiobookEnrichment(ctx context.Context, audiobookID string, description, olKey, isbn *string, publishedYear *int) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE audiobooks SET
			description    = COALESCE($2, description),
			ol_key         = COALESCE($3, ol_key),
			isbn           = COALESCE($4, isbn),
			published_year = COALESCE($5, published_year)
		WHERE id = $1`,
		audiobookID, description, olKey, isbn, publishedYear)
	return err
}

// UpdateAudiobookSeriesFromLookup sets series from a lookup only if missing.
func (s *Store) UpdateAudiobookSeriesFromLookup(ctx context.Context, audiobookID, series string, confidence float64) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE audiobooks SET
			series = $2,
			series_source = 'lookup',
			series_confidence = $3
		WHERE id = $1 AND (series IS NULL OR series = '')`,
		audiobookID, series, confidence)
	return err
}

// ClearAudiobookIngestState removes all audiobook ingest state rows.
func (s *Store) ClearAudiobookIngestState(ctx context.Context) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM audiobook_ingest_state`)
	return err
}

// ── Audiobook ingest state ────────────────────────────────────────────────────

// LoadAudiobookIngestState loads all audiobook ingest state rows for dedup.
func (s *Store) LoadAudiobookIngestState(ctx context.Context) ([]AudiobookIngestStateRow, error) {
	rows, err := s.pool.Query(ctx, `SELECT path, mtime_unix, file_size, audiobook_id FROM audiobook_ingest_state`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []AudiobookIngestStateRow
	for rows.Next() {
		var r AudiobookIngestStateRow
		if err := rows.Scan(&r.Path, &r.MtimeUnix, &r.FileSize, &r.AudiobookID); err != nil {
			return nil, err
		}
		result = append(result, r)
	}
	return result, rows.Err()
}

// UpsertAudiobookIngestState records that a file has been ingested.
func (s *Store) UpsertAudiobookIngestState(ctx context.Context, r AudiobookIngestStateRow) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO audiobook_ingest_state (path, mtime_unix, file_size, audiobook_id)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (path) DO UPDATE SET
			mtime_unix   = EXCLUDED.mtime_unix,
			file_size    = EXCLUDED.file_size,
			audiobook_id = EXCLUDED.audiobook_id,
			ingested_at  = now()`,
		r.Path, r.MtimeUnix, r.FileSize, r.AudiobookID)
	return err
}

// DeleteAudiobookIngestStateByID removes audiobook_ingest_state rows for the
// given audiobook and returns the filesystem paths that were deleted.
func (s *Store) DeleteAudiobookIngestStateByID(ctx context.Context, audiobookID string) ([]string, error) {
	rows, err := s.pool.Query(ctx,
		`DELETE FROM audiobook_ingest_state WHERE audiobook_id = $1 RETURNING path`,
		audiobookID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var paths []string
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			return nil, err
		}
		paths = append(paths, p)
	}
	return paths, rows.Err()
}

// ListInProgressAudiobooks returns audiobooks that the user has started but not completed,
// ordered by most recently updated progress. Used for "Continue Listening" on the home page.
func (s *Store) ListInProgressAudiobooks(ctx context.Context, userID string, limit int) ([]AudiobookWithProgress, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT ab.id, ab.title, ab.edition, ab.author_id, ar.name,
		       ab.cover_art_key, ab.series, ab.series_index, ab.series_source, ab.series_confidence,
		       ab.published_year, ab.duration_ms, ab.format, ab.created_at,
		       ap.position_ms, ap.completed, ap.updated_at
		FROM audiobook_progress ap
		JOIN audiobooks ab ON ab.id = ap.audiobook_id
		LEFT JOIN artists ar ON ar.id = ab.author_id
		WHERE ap.user_id = $1 AND ap.position_ms > 0 AND NOT ap.completed
		ORDER BY ap.updated_at DESC
		LIMIT $2`, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []AudiobookWithProgress
	for rows.Next() {
		var r AudiobookWithProgress
		var edition, authorID, authorName, coverArtKey, series, seriesSource sql.NullString
		var seriesIndex, seriesConfidence sql.NullFloat64
		var publishedYear sql.NullInt32
		if err := rows.Scan(
			&r.ID, &r.Title, &edition, &authorID, &authorName,
			&coverArtKey, &series, &seriesIndex, &seriesSource, &seriesConfidence,
			&publishedYear, &r.DurationMs, &r.Format, &r.CreatedAt,
			&r.PositionMs, &r.Completed, &r.ProgressUpdatedAt,
		); err != nil {
			return nil, err
		}
		if edition.Valid {
			r.Edition = &edition.String
		}
		if authorID.Valid {
			r.AuthorID = &authorID.String
		}
		if authorName.Valid {
			r.AuthorName = &authorName.String
		}
		if coverArtKey.Valid {
			r.CoverArtKey = &coverArtKey.String
		}
		if series.Valid {
			r.Series = &series.String
		}
		if seriesIndex.Valid {
			r.SeriesIndex = &seriesIndex.Float64
		}
		if seriesSource.Valid {
			r.SeriesSource = &seriesSource.String
		}
		if seriesConfidence.Valid {
			r.SeriesConfidence = &seriesConfidence.Float64
		}
		if publishedYear.Valid {
			v := int(publishedYear.Int32)
			r.PublishedYear = &v
		}
		result = append(result, r)
	}
	return result, rows.Err()
}

// ListAudiobooksBySeries returns all audiobooks in a given series, ordered by series_index.
func (s *Store) ListAudiobooksBySeries(ctx context.Context, series string) ([]Audiobook, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT ab.id, ab.title, ab.edition, ab.author_id, ar.name,
		       ab.cover_art_key, ab.series, ab.series_index, ab.series_source, ab.series_confidence,
		       ab.published_year, ab.duration_ms, ab.format, ab.created_at
		FROM audiobooks ab
		LEFT JOIN artists ar ON ar.id = ab.author_id
		WHERE ab.series = $1
		ORDER BY ab.series_index NULLS LAST, ab.title`, series)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []Audiobook
	for rows.Next() {
		var a Audiobook
		var edition, authorID, authorName, coverArtKey, ser, seriesSource sql.NullString
		var seriesIndex, seriesConfidence sql.NullFloat64
		var publishedYear sql.NullInt32
		if err := rows.Scan(
			&a.ID, &a.Title, &edition, &authorID, &authorName,
			&coverArtKey, &ser, &seriesIndex, &seriesSource, &seriesConfidence,
			&publishedYear, &a.DurationMs, &a.Format, &a.CreatedAt,
		); err != nil {
			return nil, err
		}
		if edition.Valid {
			a.Edition = &edition.String
		}
		if authorID.Valid {
			a.AuthorID = &authorID.String
		}
		if authorName.Valid {
			a.AuthorName = &authorName.String
		}
		if coverArtKey.Valid {
			a.CoverArtKey = &coverArtKey.String
		}
		if ser.Valid {
			a.Series = &ser.String
		}
		if seriesIndex.Valid {
			a.SeriesIndex = &seriesIndex.Float64
		}
		if seriesSource.Valid {
			a.SeriesSource = &seriesSource.String
		}
		if seriesConfidence.Valid {
			a.SeriesConfidence = &seriesConfidence.Float64
		}
		if publishedYear.Valid {
			v := int(publishedYear.Int32)
			a.PublishedYear = &v
		}
		result = append(result, a)
	}
	return result, rows.Err()
}

// GetAudiobookByFingerprint looks up an audiobook by file fingerprint.
func (s *Store) GetAudiobookByFingerprint(ctx context.Context, fingerprint string) (string, error) {
	var id string
	err := s.pool.QueryRow(ctx, `SELECT id FROM audiobooks WHERE fingerprint = $1`, fingerprint).Scan(&id)
	return id, err
}

// ListAudiobooksNoCover lists audiobooks without cover art. Returns the list and total count.
func (s *Store) ListAudiobooksNoCover(ctx context.Context, limit, offset int32) ([]Audiobook, int32, error) {
	var total int32
	err := s.pool.QueryRow(ctx, `SELECT COUNT(*) FROM audiobooks WHERE cover_art_key IS NULL`).Scan(&total)
	if err != nil {
		return nil, 0, err
	}

	rows, err := s.pool.Query(ctx, `
		SELECT id, title, author_id, cover_art_key, series, series_index, file_size, format, duration_ms, created_at
		FROM audiobooks
		WHERE cover_art_key IS NULL
		ORDER BY title ASC
		LIMIT $1 OFFSET $2
	`, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var result []Audiobook
	for rows.Next() {
		var a Audiobook
		var authorID, series sql.NullString
		var seriesIndex sql.NullFloat64
		err := rows.Scan(
			&a.ID, &a.Title, &authorID, &a.CoverArtKey, &series, &seriesIndex, &a.FileSize, &a.Format, &a.DurationMs, &a.CreatedAt,
		)
		if err != nil {
			return nil, 0, err
		}
		if authorID.Valid {
			a.AuthorID = &authorID.String
		}
		if series.Valid {
			a.Series = &series.String
		}
		if seriesIndex.Valid {
			a.SeriesIndex = &seriesIndex.Float64
		}
		result = append(result, a)
	}
	return result, total, rows.Err()
}

// ListAudiobooksNoSeries lists audiobooks without series information. Returns the list and total count.
func (s *Store) ListAudiobooksNoSeries(ctx context.Context, limit, offset int32) ([]Audiobook, int32, error) {
	var total int32
	err := s.pool.QueryRow(ctx, `SELECT COUNT(*) FROM audiobooks WHERE series IS NULL`).Scan(&total)
	if err != nil {
		return nil, 0, err
	}

	rows, err := s.pool.Query(ctx, `
		SELECT id, title, author_id, cover_art_key, series, series_index, file_size, format, duration_ms, created_at
		FROM audiobooks
		WHERE series IS NULL
		ORDER BY title ASC
		LIMIT $1 OFFSET $2
	`, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var result []Audiobook
	for rows.Next() {
		var a Audiobook
		var authorID, coverArtKey, series sql.NullString
		var seriesIndex sql.NullFloat64
		err := rows.Scan(
			&a.ID, &a.Title, &authorID, &coverArtKey, &series, &seriesIndex, &a.FileSize, &a.Format, &a.DurationMs, &a.CreatedAt,
		)
		if err != nil {
			return nil, 0, err
		}
		if authorID.Valid {
			a.AuthorID = &authorID.String
		}
		if coverArtKey.Valid {
			a.CoverArtKey = &coverArtKey.String
		}
		if series.Valid {
			a.Series = &series.String
		}
		if seriesIndex.Valid {
			a.SeriesIndex = &seriesIndex.Float64
		}
		result = append(result, a)
	}
	return result, total, rows.Err()
}

// PruneOrphanedAudiobooks removes audiobooks whose source paths are no longer
// present on disk. foundPaths is the complete set of audiobook file or
// directory paths collected during the current scan. Ingest-state rows absent
// from that set are treated as orphaned. The method also cleans up narrators
// with no remaining links and returns object-store keys the caller should
// delete.
func (s *Store) PruneOrphanedAudiobooks(ctx context.Context, foundPaths []string) (int, []string, error) {
	foundSet := make(map[string]struct{}, len(foundPaths))
	for _, p := range foundPaths {
		foundSet[p] = struct{}{}
	}

	stateRows, err := s.LoadAudiobookIngestState(ctx)
	if err != nil {
		return 0, nil, fmt.Errorf("prune audiobooks: load ingest state: %w", err)
	}

	seenIDs := make(map[string]struct{})
	var orphanPaths []string
	var orphanIDs []string
	for _, r := range stateRows {
		if _, ok := foundSet[r.Path]; !ok {
			orphanPaths = append(orphanPaths, r.Path)
			if r.AudiobookID != "" {
				if _, dup := seenIDs[r.AudiobookID]; !dup {
					seenIDs[r.AudiobookID] = struct{}{}
					orphanIDs = append(orphanIDs, r.AudiobookID)
				}
			}
		}
	}
	if len(orphanIDs) == 0 {
		return 0, nil, nil
	}

	// Collect object-store keys before deletion.
	var objKeys []string

	// Audiobook file keys and cover art keys.
	abRows, err := s.pool.Query(ctx,
		`SELECT file_key, cover_art_key FROM audiobooks WHERE id = ANY($1)`, orphanIDs)
	if err == nil {
		for abRows.Next() {
			var fk, cak sql.NullString
			if abRows.Scan(&fk, &cak) == nil {
				if fk.Valid && fk.String != "" {
					objKeys = append(objKeys, fk.String)
				}
				if cak.Valid && cak.String != "" {
					objKeys = append(objKeys, cak.String)
				}
			}
		}
		abRows.Close()
	}

	// Chapter file keys (multi-file audiobooks store a file_key per chapter).
	chapRows, err := s.pool.Query(ctx,
		`SELECT file_key FROM audiobook_chapters
		 WHERE audiobook_id = ANY($1) AND file_key IS NOT NULL`, orphanIDs)
	if err == nil {
		for chapRows.Next() {
			var k string
			if chapRows.Scan(&k) == nil && k != "" {
				objKeys = append(objKeys, k)
			}
		}
		chapRows.Close()
	}

	// Delete ingest-state rows.
	if _, err := s.pool.Exec(ctx,
		`DELETE FROM audiobook_ingest_state WHERE path = ANY($1)`, orphanPaths); err != nil {
		return 0, nil, fmt.Errorf("prune audiobook_ingest_state: %w", err)
	}

	// Delete audiobooks (cascades to chapters, progress, bookmarks, narrator links).
	if _, err := s.pool.Exec(ctx,
		`DELETE FROM audiobooks WHERE id = ANY($1)`, orphanIDs); err != nil {
		return 0, nil, fmt.Errorf("prune orphaned audiobooks: %w", err)
	}

	// Clean up narrators that have no remaining links.
	if _, err := s.pool.Exec(ctx,
		`DELETE FROM audiobook_narrators
		 WHERE id NOT IN (SELECT DISTINCT narrator_id FROM audiobook_narrator_links)`); err != nil {
		return 0, nil, fmt.Errorf("prune orphaned narrators: %w", err)
	}

	return len(orphanIDs), objKeys, nil
}
