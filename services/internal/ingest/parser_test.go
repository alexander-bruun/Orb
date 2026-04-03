package ingest

import (
	"reflect"
	"testing"
)

// ── parseASINFromFolder ───────────────────────────────────────────────────────

func TestParseASINFromFolder(t *testing.T) {
	tests := []struct {
		input      string
		wantFolder string
		wantASIN   string
	}{
		// Standard trailing ASIN
		{"Book Title [B0015T963C]", "Book Title", "B0015T963C"},
		// Leading ASIN
		{"[B0015T963C] Book Title", "Book Title", "B0015T963C"},
		// ASIN only
		{"[B0015T963C]", "", "B0015T963C"},
		// No ASIN
		{"Just A Title", "Just A Title", ""},
		// ASIN embedded with other metadata
		{"(2001) - Goblet of Fire [B00KLZ51R0]", "(2001) - Goblet of Fire", "B00KLZ51R0"},
		// Too short – not an ASIN (9 chars)
		{"Book [B001T963C]", "Book [B001T963C]", ""},
		// Too long – not an ASIN (11 chars)
		{"Book [B0015T963CC]", "Book [B0015T963CC]", ""},
		// Lowercase letters – not an ASIN
		{"Book [b0015t963c]", "Book [b0015t963c]", ""},
		// Looks like an ASIN but has spaces inside brackets
		{"Book [B00 5T963C]", "Book [B00 5T963C]", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			gotFolder, gotASIN := parseASINFromFolder(tt.input)
			if gotFolder != tt.wantFolder || gotASIN != tt.wantASIN {
				t.Errorf("parseASINFromFolder(%q) = (%q, %q), want (%q, %q)",
					tt.input, gotFolder, gotASIN, tt.wantFolder, tt.wantASIN)
			}
		})
	}
}

// ── parseNarratorsFromBraces ──────────────────────────────────────────────────

func TestParseNarratorsFromBraces(t *testing.T) {
	tests := []struct {
		input         string
		wantTitle     string
		wantNarrators []string
	}{
		// Single narrator
		{
			"Book Title {Stephen Fry}",
			"Book Title",
			[]string{"Stephen Fry"},
		},
		// Multiple narrators (comma-separated)
		{
			"Title {John Smith, Jane Doe}",
			"Title",
			[]string{"John Smith", "Jane Doe"},
		},
		// Narrators with extra whitespace
		{
			"Title { Alice Cooper ,  Bob Dylan }",
			"Title",
			[]string{"Alice Cooper", "Bob Dylan"},
		},
		// No braces – unchanged
		{
			"Just A Title",
			"Just A Title",
			nil,
		},
		// Braces in middle – not matched (suffix-only pattern)
		{
			"{Narrator} Book Title",
			"{Narrator} Book Title",
			nil,
		},
		// Combined with series prefix
		{
			"Book 3 - The Prisoner of Azkaban {Stephen Fry}",
			"Book 3 - The Prisoner of Azkaban",
			[]string{"Stephen Fry"},
		},
		// Three narrators
		{
			"Novel {Alice, Bob, Carol}",
			"Novel",
			[]string{"Alice", "Bob", "Carol"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			gotTitle, gotNarrators := parseNarratorsFromBraces(tt.input)
			if gotTitle != tt.wantTitle {
				t.Errorf("parseNarratorsFromBraces(%q) title = %q, want %q",
					tt.input, gotTitle, tt.wantTitle)
			}
			if !reflect.DeepEqual(gotNarrators, tt.wantNarrators) {
				t.Errorf("parseNarratorsFromBraces(%q) narrators = %v, want %v",
					tt.input, gotNarrators, tt.wantNarrators)
			}
		})
	}
}

// ── parseNarratorFromParens ───────────────────────────────────────────────────

func TestParseNarratorFromParens(t *testing.T) {
	tests := []struct {
		input        string
		wantFolder   string
		wantNarrator string
	}{
		// Single narrator with "read by"
		{
			"Book Title (read by Stephen Fry)",
			"Book Title",
			"Stephen Fry",
		},
		// Single narrator with "narrated by"
		{
			"Title (narrated by Nick Podehl)",
			"Title",
			"Nick Podehl",
		},
		// Case insensitive
		{
			"Book (READ BY Roy Dotrice)",
			"Book",
			"Roy Dotrice",
		},
		// No parenthetical narrator
		{
			"Just A Title",
			"Just A Title",
			"",
		},
		// With other content after
		{
			"A Song of Ice and Fire 1 - A Game of Thrones (read by Roy Dotrice)",
			"A Song of Ice and Fire 1 - A Game of Thrones",
			"Roy Dotrice",
		},
		// Multiple spaces in narrator name
		{
			"Title (read by John Smith)",
			"Title",
			"John Smith",
		},
		// Empty narrator (malformed)
		{
			"Title (read by)",
			"Title (read by)",
			"",
		},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			gotFolder, gotNarrator := parseNarratorFromParens(tt.input)
			if gotFolder != tt.wantFolder || gotNarrator != tt.wantNarrator {
				t.Errorf("parseNarratorFromParens(%q) = (%q, %q), want (%q, %q)",
					tt.input, gotFolder, gotNarrator, tt.wantFolder, tt.wantNarrator)
			}
		})
	}
}

// ── parseYearPrefixFromFolder ─────────────────────────────────────────────────

func TestParseYearPrefixFromFolder(t *testing.T) {
	tests := []struct {
		input      string
		wantFolder string
		wantYear   string
	}{
		// Parenthesised year
		{"(2020) - Book Title", "Book Title", "2020"},
		// Year without parens
		{"2020 - Book Title", "Book Title", "2020"},
		// Year with extra spaces
		{"(1999)  -  Title Here", "Title Here", "1999"},
		// No year
		{"Just A Title", "Just A Title", ""},
		// Year at end – not matched
		{"Book Title (2020)", "Book Title (2020)", ""},
		// Year plus narrator braces (year first)
		{"(2001) - Goblet of Fire", "Goblet of Fire", "2001"},
		// Four-digit number that looks like a year but is NOT preceded by optional paren+hyphen
		{"Book 2020", "Book 2020", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			gotFolder, gotYear := parseYearPrefixFromFolder(tt.input)
			if gotFolder != tt.wantFolder || gotYear != tt.wantYear {
				t.Errorf("parseYearPrefixFromFolder(%q) = (%q, %q), want (%q, %q)",
					tt.input, gotFolder, gotYear, tt.wantFolder, tt.wantYear)
			}
		})
	}
}

// ── parseAuthorNames ──────────────────────────────────────────────────────────

func TestParseAuthorNames(t *testing.T) {
	tests := []struct {
		input string
		want  []string
	}{
		// Simple "First Last"
		{"John Smith", []string{"John Smith"}},
		// "Last, First" → reordered
		{"Smith, John", []string{"John Smith"}},
		// Two authors with &
		{"John Smith & Jane Doe", []string{"John Smith", "Jane Doe"}},
		// Two "Last, First" authors with &
		{"Smith, John & Doe, Jane", []string{"John Smith", "Jane Doe"}},
		// Semicolon-separated
		{"Smith, John; Doe, Jane", []string{"John Smith", "Jane Doe"}},
		// "and" separator
		{"John Smith and Jane Doe", []string{"John Smith", "Jane Doe"}},
		// Empty
		{"", nil},
		// CJK – returned as-is, no reordering
		{"田中太郎", []string{"田中太郎"}},
		// Deduplication
		{"John Smith & John Smith", []string{"John Smith"}},
		// Middle name preserved
		{"Smith, John William", []string{"John William Smith"}},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := parseAuthorNames(tt.input)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("parseAuthorNames(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

// ── ParseFolderPath ───────────────────────────────────────────────────────────

func TestParseFolderPath(t *testing.T) {
	pf64 := func(v float64) *float64 { return &v }
	ps := func(v string) *string { return &v }

	tests := []struct {
		name string
		path string
		want FolderMetadata
	}{
		{
			name: "full 3-level hierarchy with all metadata",
			path: "Smith, John/Harry Potter/Book 3 - The Prisoner of Azkaban {Stephen Fry}",
			want: FolderMetadata{
				Title:       "The Prisoner of Azkaban",
				Authors:     []string{"John Smith"},
				SeriesName:  "Harry Potter",
				SeriesIndex: pf64(3),
				Narrators:   []string{"Stephen Fry"},
			},
		},
		{
			name: "year prefix + ASIN",
			path: "(2001) - Goblet of Fire [B00KLZ51R0]",
			want: FolderMetadata{
				Title:         "Goblet of Fire",
				PublishedYear: "2001",
				ASIN:          "B00KLZ51R0",
			},
		},
		{
			name: "2-level author/book",
			path: "Tolkien, J.R.R./The Hobbit",
			want: FolderMetadata{
				Title:   "The Hobbit",
				Authors: []string{"J.R.R. Tolkien"},
			},
		},
		{
			name: "author with & and narrator braces",
			path: "Adams, Douglas/The Hitchhiker's Guide to the Galaxy {Stephen Fry}",
			want: FolderMetadata{
				Title:     "The Hitchhiker's Guide to the Galaxy",
				Authors:   []string{"Douglas Adams"},
				Narrators: []string{"Stephen Fry"},
			},
		},
		{
			name: "standalone folder (1 level)",
			path: "1984",
			want: FolderMetadata{
				Title: "1984",
			},
		},
		{
			name: "hash-prefix series index",
			path: "Rowling, J.K./Harry Potter/#1 - The Philosopher's Stone",
			want: FolderMetadata{
				Title:       "The Philosopher's Stone",
				Authors:     []string{"J.K. Rowling"},
				SeriesName:  "Harry Potter",
				SeriesIndex: pf64(1),
			},
		},
		{
			name: "edition tag stripped",
			path: "Author/Series/Book 2 - Great Novel (Unabridged)",
			want: FolderMetadata{
				Title:       "Great Novel",
				Authors:     []string{"Author"},
				SeriesName:  "Series",
				SeriesIndex: pf64(2),
				Edition:     ps("Unabridged"),
			},
		},
		{
			name: "multiple narrators",
			path: "Pratchett, Terry/Discworld/Vol. 1 - The Colour of Magic {Tony Robinson, Stephen Briggs}",
			want: FolderMetadata{
				Title:       "The Colour of Magic",
				Authors:     []string{"Terry Pratchett"},
				SeriesName:  "Discworld",
				SeriesIndex: pf64(1),
				Narrators:   []string{"Tony Robinson", "Stephen Briggs"},
			},
		},
		{
			name: "CJK author not reordered",
			path: "田中太郎/シリーズ/第一巻",
			want: FolderMetadata{
				Title:      "第一巻",
				Authors:    []string{"田中太郎"},
				SeriesName: "シリーズ",
			},
		},
		{
			name: "quality tag in series dir is stripped",
			path: "Rowling, J.K./Harry Potter [FLAC]/Book 1 - The Philosopher's Stone",
			want: FolderMetadata{
				Title:       "The Philosopher's Stone",
				Authors:     []string{"J.K. Rowling"},
				SeriesName:  "Harry Potter",
				SeriesIndex: pf64(1),
			},
		},
		{
			name: "empty path",
			path: "",
			want: FolderMetadata{},
		},
		{
			name: "embedded series with single-digit index and parens narrator",
			path: "George R. R. Martin/1996 - A Song of Ice and Fire 1 - A Game of Thrones (read by Roy Dotrice)",
			want: FolderMetadata{
				Title:         "A Game of Thrones",
				Authors:       []string{"George R. R. Martin"},
				SeriesName:    "A Song of Ice and Fire",
				SeriesIndex:   pf64(1),
				Narrators:     []string{"Roy Dotrice"},
				PublishedYear: "1996",
			},
		},
		{
			name: "embedded series with two-digit index and parens narrator",
			path: "Patrick Rothfuss/The Name of the Wind/Book 1.5 - The Story Begins (read by Nick Podehl)",
			want: FolderMetadata{
				Title:       "The Story Begins",
				Authors:     []string{"Patrick Rothfuss"},
				SeriesName:  "The Name of the Wind",
				SeriesIndex: pf64(1.5),
				Narrators:   []string{"Nick Podehl"},
			},
		},
		{
			name: "series folder with parens narrator in book subdir",
			path: "Dan Simmons/Hyperion Cantos/1 - Hyperion (read by Jonathan Davis)",
			want: FolderMetadata{
				Title:       "Hyperion",
				Authors:     []string{"Dan Simmons"},
				SeriesName:  "Hyperion Cantos",
				SeriesIndex: pf64(1),
				Narrators:   []string{"Jonathan Davis"},
			},
		},
		{
			name: "Hyperion Cantos - first book with year",
			path: "Dan Simmons/Hyperion Cantos/1989 - Hyperion",
			want: FolderMetadata{
				Title:         "Hyperion",
				Authors:       []string{"Dan Simmons"},
				SeriesName:    "Hyperion Cantos",
				PublishedYear: "1989",
			},
		},
		{
			name: "Hyperion Cantos - second book with year",
			path: "Dan Simmons/Hyperion Cantos/1990 - The Fall of Hyperion",
			want: FolderMetadata{
				Title:         "The Fall of Hyperion",
				Authors:       []string{"Dan Simmons"},
				SeriesName:    "Hyperion Cantos",
				PublishedYear: "1990",
			},
		},
		{
			name: "Hyperion Cantos - third book with year",
			path: "Dan Simmons/Hyperion Cantos/1996 - Endymion",
			want: FolderMetadata{
				Title:         "Endymion",
				Authors:       []string{"Dan Simmons"},
				SeriesName:    "Hyperion Cantos",
				PublishedYear: "1996",
			},
		},
		{
			name: "Hyperion Cantos - fourth book with year",
			path: "Dan Simmons/Hyperion Cantos/1997 - The Rise of Endymion",
			want: FolderMetadata{
				Title:         "The Rise of Endymion",
				Authors:       []string{"Dan Simmons"},
				SeriesName:    "Hyperion Cantos",
				PublishedYear: "1997",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseFolderPath(tt.path)
			checkFolderMetadata(t, got, tt.want)
		})
	}
}

// checkFolderMetadata compares two FolderMetadata values field by field so that
// test failures show exactly which field differs.
func checkFolderMetadata(t *testing.T, got, want FolderMetadata) {
	t.Helper()
	if got.Title != want.Title {
		t.Errorf("Title = %q, want %q", got.Title, want.Title)
	}
	if !reflect.DeepEqual(got.Authors, want.Authors) {
		t.Errorf("Authors = %v, want %v", got.Authors, want.Authors)
	}
	if got.SeriesName != want.SeriesName {
		t.Errorf("SeriesName = %q, want %q", got.SeriesName, want.SeriesName)
	}
	if !floatPtrEq(got.SeriesIndex, want.SeriesIndex) {
		t.Errorf("SeriesIndex = %v, want %v", got.SeriesIndex, want.SeriesIndex)
	}
	if !reflect.DeepEqual(got.Narrators, want.Narrators) {
		t.Errorf("Narrators = %v, want %v", got.Narrators, want.Narrators)
	}
	if got.PublishedYear != want.PublishedYear {
		t.Errorf("PublishedYear = %q, want %q", got.PublishedYear, want.PublishedYear)
	}
	if got.ASIN != want.ASIN {
		t.Errorf("ASIN = %q, want %q", got.ASIN, want.ASIN)
	}
	if !strPtrEq(got.Edition, want.Edition) {
		t.Errorf("Edition = %v, want %v", got.Edition, want.Edition)
	}
}

func floatPtrEq(a, b *float64) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return *a == *b
}

func strPtrEq(a, b *string) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return *a == *b
}
