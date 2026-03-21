package ingest

import (
	"regexp"
	"strings"
)

// ── Compiled patterns (module-level for zero-cost reuse) ─────────────────────

var (
	// reASIN matches a 10-character Amazon Standard ID enclosed in brackets,
	// optionally surrounded by spaces. E.g. "[B0015T963C]" or "Title [B0015T963C]".
	reASIN = regexp.MustCompile(`(?:^| )\[([A-Z0-9]{10})\](?:$| )`)

	// reNarratorsInBraces matches a trailing "{Narrator One, Narrator Two}" block
	// separated from the preceding title by at least one space.
	reNarratorsInBraces = regexp.MustCompile(`^(.*?)\s+\{([^}]+)\}$`)

	// reYearPrefixFolder matches a leading "(YYYY) - " or "YYYY - " annotation.
	// Group 1 = four-digit year, Group 2 = remainder of the folder name.
	reYearPrefixFolder = regexp.MustCompile(`^\s*\(?(\d{4})\)?\s*-\s*(.+)$`)

	// reNarratorInParens matches "(read by X)" or "(narrated by X)" annotations
	// in folder names. Group 1 = narrator name.
	reNarratorInParens = regexp.MustCompile(`(?i)\s*\((?:read|narrated)\s+by\s+([^)]+)\)`)
)

// ── Individual field extractors ───────────────────────────────────────────────

// parseASINFromFolder strips a 10-character ASIN tag like "[B0015T963C]" from a
// folder name. Returns the cleaned folder name and the ASIN (empty if not found).
//
// Examples:
//
//	"Book Title [B0015T963C]"        → ("Book Title", "B0015T963C")
//	"[B0015T963C] Title"             → ("Title", "B0015T963C")
//	"Title"                          → ("Title", "")
func parseASINFromFolder(folder string) (string, string) {
	m := reASIN.FindStringSubmatchIndex(folder)
	if m == nil {
		return folder, ""
	}
	asin := folder[m[2]:m[3]]
	cleaned := reASIN.ReplaceAllString(folder, " ")
	return strings.TrimSpace(cleaned), asin
}

// parseNarratorsFromBraces extracts a "{Narrator One, Narrator Two}" suffix from
// a folder name. Returns the cleaned title and the list of narrator names.
// Returns nil narrators if the pattern is absent.
//
// Examples:
//
//	"Book Title {Stephen Fry}"               → ("Book Title", ["Stephen Fry"])
//	"Title {John Smith, Jane Doe}"           → ("Title", ["John Smith", "Jane Doe"])
//	"Title"                                  → ("Title", nil)
func parseNarratorsFromBraces(folder string) (string, []string) {
	m := reNarratorsInBraces.FindStringSubmatch(folder)
	if m == nil {
		return folder, nil
	}
	title := strings.TrimSpace(m[1])
	var narrators []string
	for _, n := range strings.Split(m[2], ",") {
		n = strings.TrimSpace(n)
		if n != "" {
			narrators = append(narrators, n)
		}
	}
	return title, narrators
}

// parseYearPrefixFromFolder extracts a leading "(YYYY) - " or "YYYY - " year
// annotation from a folder name. Returns the cleaned remainder and the year
// string (empty if not found).
//
// Examples:
//
//	"(2020) - Book Title"   → ("Book Title", "2020")
//	"2020 - Book Title"     → ("Book Title", "2020")
//	"Book Title"            → ("Book Title", "")
func parseYearPrefixFromFolder(folder string) (string, string) {
	m := reYearPrefixFolder.FindStringSubmatch(folder)
	if m == nil {
		return folder, ""
	}
	return strings.TrimSpace(m[2]), m[1]
}

// parseNarratorFromParens strips "(read by X)" or "(narrated by X)" parenthetical
// narrator annotations from a folder name. Returns the cleaned folder and the
// narrator name (empty if not found).
//
// Examples:
//
//	"Book Title (read by Stephen Fry)"      → ("Book Title", "Stephen Fry")
//	"Title (narrated by Nick Podehl)"       → ("Title", "Nick Podehl")
//	"Title"                                 → ("Title", "")
func parseNarratorFromParens(folder string) (string, string) {
	m := reNarratorInParens.FindStringSubmatchIndex(folder)
	if m == nil {
		return folder, ""
	}
	sub := reNarratorInParens.FindStringSubmatch(folder)
	narrator := strings.TrimSpace(sub[1])
	// Remove the matched substring and clean up surrounding whitespace
	cleaned := strings.TrimSpace(folder[:m[0]] + folder[m[1]:])
	return cleaned, narrator
}

// ── High-level path parser ────────────────────────────────────────────────────

// FolderMetadata holds metadata extracted purely from a directory path and
// folder name annotations, without touching audio file tags.
type FolderMetadata struct {
	// Title is the cleaned book title derived from the leaf folder name.
	Title string
	// Authors is populated from the outermost path component when 3+ levels
	// are present (e.g. "Smith, John" → ["John Smith"]).
	Authors []string
	// SeriesName is the middle path component when 3+ levels are present.
	SeriesName string
	// SeriesIndex is the numeric index extracted from the leaf folder name
	// (e.g. "Book 3 - Title" → 3). Nil when absent.
	SeriesIndex *float64
	// Narrators are extracted from a "{Narrator}" suffix on the leaf folder.
	Narrators []string
	// PublishedYear is the 4-digit year found in the leaf folder name.
	// Empty string when absent.
	PublishedYear string
	// ASIN is the 10-character Amazon Standard ID found in the leaf folder.
	// Empty string when absent.
	ASIN string
	// Edition is the edition label extracted from the book folder name
	// (e.g. "Unabridged", "Abridged").
	Edition *string
}

// ParseFolderPath extracts metadata from a path relative to the library root.
// Supported layouts (and combinations):
//
//	Author / Series / Book N - Title
//	Author / Year - Series N - Title
//	Series (by Author) / Book N - Title        ← author in series folder annotation
//	Author - Series / Series N Title           ← author-dash-series folder
//	PlainSeries / Year - Title                 ← year-ordered series with no author layer
//	PlainSeries / Author - Title, Book N       ← author embedded in book folder name
//	Book Title                                 ← standalone
//
// Leaf-folder annotations (ASIN, narrator braces/parens, year prefix) are
// stripped and returned in the struct fields.
func ParseFolderPath(relPath string) FolderMetadata {
	parts := splitPathParts(relPath)

	var parentRaw, seriesRaw, folderName string
	switch len(parts) {
	case 0:
		return FolderMetadata{}
	case 1:
		folderName = parts[0]
	case 2:
		parentRaw = parts[0]
		folderName = parts[1]
	default:
		// Three or more levels: outermost = author, next = series, last = book.
		parentRaw = parts[len(parts)-3]
		seriesRaw = parts[len(parts)-2]
		folderName = parts[len(parts)-1]
	}

	// ── Strip leaf-folder annotations in a well-defined order ────────────────
	folderName, asin := parseASINFromFolder(folderName)
	folderName, narrators := parseNarratorsFromBraces(folderName)
	folderName, narratorFromParens := parseNarratorFromParens(folderName)
	folderName, year := parseYearPrefixFromFolder(folderName)

	if len(narrators) == 0 && narratorFromParens != "" {
		narrators = []string{narratorFromParens}
	}

	// ── Parse book folder name for title, index, edition ─────────────────────
	title, seriesIndex, edition := parseBookDirName(folderName)

	// ── Extract series name embedded in the book folder ───────────────────────
	// e.g. "A Song of Ice and Fire 1 - A Game of Thrones" → "A Song of Ice and Fire"
	seriesFromBookDir := parseSeriesPrefixFromBookDirName(folderName)

	// ── Parse the parent component for series/author/narrator annotations ─────
	// e.g. "Harry Potter (by J.K. Rowling) - The Complete Story (read by Stephen Fry)"
	//      "James S. A. Corey - The Expanse Series"
	var seriesFromParentAnnotation, authorFromParent, narratorFromParent string
	if parentRaw != "" {
		seriesFromParentAnnotation, authorFromParent, narratorFromParent = parseSeriesDirName(parentRaw)
		if len(narrators) == 0 && narratorFromParent != "" {
			narrators = []string{narratorFromParent}
		}
	}

	// ── Detect "Author - Title" embedded inside the book folder ──────────────
	// Used when the parent is a plain series name (no "(by X)" annotation) and
	// the book folder carries the author: "Andrzej Sapkowski - Blood of Elves…"
	// Only attempt extraction when seriesIndex is non-nil (author embeds are
	// typically accompanied by a series index like ", Book N").
	// Only applies to 2-level paths; in 3-level paths parentRaw is the author dir.
	var authorEmbedded string
	parentIsPlainSeries := len(parts) == 2 && parentRaw != "" && authorFromParent == "" && seriesFromParentAnnotation == ""
	if parentIsPlainSeries && seriesIndex != nil {
		if idx := strings.Index(title, " - "); idx > 0 {
			candidate := strings.TrimSpace(title[:idx])
			rest := strings.TrimSpace(title[idx+3:])
			// Accept the split only when the candidate looks like a person name
			// (not a bare number or a series-style phrase with digits).
			if !strings.ContainsAny(candidate, "0123456789") && strings.ContainsRune(candidate, ' ') {
				authorEmbedded = candidate
				// Strip the parent series name from the end of the remaining title.
				seriesClean := cleanSeriesName(parentRaw)
				rest = strings.TrimSuffix(rest, " "+seriesClean)
				rest = strings.TrimSuffix(rest, ", "+seriesClean)
				rest = strings.TrimSuffix(rest, " "+parentRaw)
				title = strings.TrimSpace(rest)
			}
		}
	}

	// ── Resolve final series name ─────────────────────────────────────────────
	// Priority:
	//   1. Parent annotation (explicit "(by X)" or "Author - Series" folder)
	//   2. Series embedded in book dir ("A Song of Ice and Fire 1 - Title")
	//   3. 2-level plain-series parent with year-ordering or book index
	//   4. 3-level middle component (only when book has an index or series)
	var seriesName string
	switch {
	case seriesFromParentAnnotation != "":
		// "HP (by J.K. Rowling) - ..." or "Author - SeriesName" parent folder
		seriesName = seriesFromParentAnnotation
	case seriesFromBookDir != "":
		// "A Song of Ice and Fire 1 - A Game of Thrones" → series in book dir
		seriesName = seriesFromBookDir
	case parentIsPlainSeries && (seriesIndex != nil || year != ""):
		// 2-level only: plain series folder like "Hyperion Cantos/" or "The Witcher/";
		// book has an index or is year-ordered — parent is the series.
		seriesName = cleanSeriesName(parentRaw)
	case len(parts) >= 3 && isSeriesDirComponent(seriesRaw):
		// 3-level: middle component is a proper series dir (not a grouping folder
		// distinguished by a leading special character such as "-" or "_").
		seriesName = cleanSeriesName(seriesRaw)
	}

	// ── Resolve authors ───────────────────────────────────────────────────────
	var authors []string
	switch {
	case authorEmbedded != "":
		authors = parseAuthorNames(authorEmbedded)
	case authorFromParent != "":
		// Parent annotation supplied the author.
		authors = parseAuthorNames(authorFromParent)
	case seriesName != "" && cleanSeriesName(parentRaw) == seriesName:
		// parentRaw was consumed as the series name — no author available.
	default:
		authors = parseAuthorNames(parentRaw)
	}

	return FolderMetadata{
		Title:         title,
		Authors:       authors,
		SeriesName:    seriesName,
		SeriesIndex:   seriesIndex,
		Narrators:     narrators,
		PublishedYear: year,
		ASIN:          asin,
		Edition:       edition,
	}
}

// ── Name parsing helpers ──────────────────────────────────────────────────────

// parseAuthorNames splits an author directory component into individual names.
// It handles the common formats:
//
//	"John Smith"            → ["John Smith"]
//	"Smith, John"           → ["John Smith"]
//	"John Smith & Jane Doe" → ["John Smith", "Jane Doe"]
//	"Smith, John & Doe, Jane" → ["John Smith", "Jane Doe"]
//	"Smith; John; Doe; Jane" → ["John Smith", "Jane Doe"]
//	"" → []
//
// CJK names (containing Chinese/Japanese/Korean characters) are returned as-is
// without reordering.
func parseAuthorNames(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}

	// Split on "&", "and", or ";" into individual "Last, First" or "First Last" tokens.
	var tokens []string
	switch {
	case strings.Contains(raw, "&"):
		for _, part := range strings.Split(raw, "&") {
			tokens = append(tokens, strings.TrimSpace(part))
		}
	case strings.Contains(strings.ToLower(raw), " and "):
		for _, part := range splitCaseInsensitive(raw, " and ") {
			tokens = append(tokens, strings.TrimSpace(part))
		}
	case strings.Contains(raw, ";"):
		for _, part := range strings.Split(raw, ";") {
			tokens = append(tokens, strings.TrimSpace(part))
		}
	default:
		tokens = []string{raw}
	}

	var result []string
	seen := make(map[string]bool)
	for _, tok := range tokens {
		name := normalizePersonName(tok)
		if name != "" && !seen[name] {
			result = append(result, name)
			seen[name] = true
		}
	}
	return result
}

// normalizePersonName converts "Last, First" to "First Last".
// Returns the input unchanged for CJK names or names with no comma.
func normalizePersonName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	if containsCJK(name) {
		return name
	}
	parts := strings.SplitN(name, ",", 2)
	if len(parts) != 2 {
		return name
	}
	last := strings.TrimSpace(parts[0])
	first := strings.TrimSpace(parts[1])
	if first == "" {
		return last
	}
	if last == "" {
		return first
	}
	return first + " " + last
}

// containsCJK reports whether s contains any Chinese, Japanese, or Korean
// Unicode code point. CJK names are returned without reordering.
func containsCJK(s string) bool {
	for _, r := range s {
		if (r >= 0x4E00 && r <= 0x9FFF) || // CJK Unified Ideographs
			(r >= 0x3040 && r <= 0x309F) || // Hiragana
			(r >= 0x30A0 && r <= 0x30FF) || // Katakana
			(r >= 0xAC00 && r <= 0xD7AF) { // Hangul Syllables
			return true
		}
	}
	return false
}

// splitPathParts splits a slash-separated relative path into non-empty
// components. Both "/" and "\" are treated as separators for portability.
func splitPathParts(relPath string) []string {
	relPath = strings.ReplaceAll(relPath, "\\", "/")
	var parts []string
	for _, p := range strings.Split(relPath, "/") {
		p = strings.TrimSpace(p)
		if p != "" && p != "." && p != ".." {
			parts = append(parts, p)
		}
	}
	return parts
}

// isSeriesDirComponent reports whether a middle path component looks like a
// real series directory rather than a grouping/organisational folder.
// Folders whose names start with a special character (-, _, ., ~, #, @) are
// treated as grouping folders (e.g. "-Short Stories and Novellas") and are
// excluded from series resolution.
func isSeriesDirComponent(name string) bool {
	if name == "" {
		return false
	}
	switch name[0] {
	case '-', '_', '.', '~', '#', '@':
		return false
	}
	return true
}

// splitCaseInsensitive splits s on sep (compared case-insensitively).
func splitCaseInsensitive(s, sep string) []string {
	lower := strings.ToLower(s)
	lsep := strings.ToLower(sep)
	var result []string
	for {
		idx := strings.Index(lower, lsep)
		if idx < 0 {
			result = append(result, s)
			break
		}
		result = append(result, s[:idx])
		s = s[idx+len(sep):]
		lower = lower[idx+len(sep):]
	}
	return result
}
