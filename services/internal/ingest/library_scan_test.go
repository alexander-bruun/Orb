package ingest

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestLibraryFolderParsing verifies that ParseFolderPath (and the underlying
// helpers used by the ingest path) produce the expected metadata for every
// known audiobook folder layout in the real library.
//
// Run with:
//
//	go test ./internal/ingest/ -run TestLibraryFolderParsing -v
func TestLibraryFolderParsing(t *testing.T) {
	const root = "/mnt/d/Audiobooks"
	if _, err := os.Stat(root); err != nil {
		t.Skip("audiobook library not mounted at " + root)
	}

	type want struct {
		title    string
		author   string // "" means "don't assert"
		series   string
		idx      string // "" means no index expected; e.g. "1", "2.5"
		year     string
		narrator string // "" means "don't assert"
	}

	// rel is relative to the library root.
	tests := []struct {
		rel  string
		want want
	}{
		// ── George R. R. Martin ──────────────────────────────────────────────
		{
			rel: "George R. R. Martin/1996 - A Song of Ice and Fire 1 - A Game of Thrones (read by Roy Dotrice)",
			want: want{
				title:    "A Game of Thrones",
				author:   "George R. R. Martin",
				series:   "A Song of Ice and Fire",
				idx:      "1",
				year:     "1996",
				narrator: "Roy Dotrice",
			},
		},
		{
			rel: "George R. R. Martin/1998 - A Song of Ice and Fire 2 - A Clash of Kings (read by Roy Dotrice)",
			want: want{
				title:  "A Clash of Kings",
				author: "George R. R. Martin",
				series: "A Song of Ice and Fire",
				idx:    "2",
				year:   "1998",
			},
		},
		{
			rel: "George R. R. Martin/2005 - A Song of Ice and Fire 4 - A Feast for Crows (read by John Lee)",
			want: want{
				title:    "A Feast for Crows",
				author:   "George R. R. Martin",
				series:   "A Song of Ice and Fire",
				idx:      "4",
				year:     "2005",
				narrator: "John Lee",
			},
		},
		// Standalone story in sub-group folder
		{
			rel: "George R. R. Martin/-Short Stories and Novellas/1980 - The Ice Dragon (read by Maggi-Meg Reed)",
			want: want{
				title:    "The Ice Dragon",
				author:   "George R. R. Martin",
				series:   "",
				year:     "1980",
				narrator: "Maggi-Meg Reed",
			},
		},
		// Dunk and Egg subseries inside the short stories folder
		{
			rel: "George R. R. Martin/-Short Stories and Novellas/1998 - Tales of Dunk and Egg 1 - The Hedge Knight (read by Frank Muller)",
			want: want{
				title:    "The Hedge Knight",
				author:   "George R. R. Martin",
				series:   "Tales of Dunk and Egg",
				idx:      "1",
				year:     "1998",
				narrator: "Frank Muller",
			},
		},
		{
			rel: "George R. R. Martin/-Short Stories and Novellas/2010 - Tales of Dunk and Egg 3 - The Mystery Knight (read by Patrick Lawlor)",
			want: want{
				title:    "The Mystery Knight",
				author:   "George R. R. Martin",
				series:   "Tales of Dunk and Egg",
				idx:      "3",
				year:     "2010",
				narrator: "Patrick Lawlor",
			},
		},

		// ── Harry Potter ─────────────────────────────────────────────────────
		{
			rel: "Harry Potter (by J.K. Rowling) - The Complete Story (read by Stephen Fry) [V0]/Book 1 - Harry Potter and the Philosopher's Stone",
			want: want{
				title:    "Harry Potter and the Philosopher's Stone",
				author:   "J.K. Rowling",
				series:   "Harry Potter",
				idx:      "1",
				narrator: "Stephen Fry",
			},
		},
		{
			rel: "Harry Potter (by J.K. Rowling) - The Complete Story (read by Stephen Fry) [V0]/Book 7 - Harry Potter and the Deathly Hallows",
			want: want{
				title:    "Harry Potter and the Deathly Hallows",
				author:   "J.K. Rowling",
				series:   "Harry Potter",
				idx:      "7",
				narrator: "Stephen Fry",
			},
		},

		// ── Hyperion Cantos (Series/YearBook, no author layer) ────────────────
		{
			rel: "Hyperion Cantos/1989 - Hyperion",
			want: want{
				title:  "Hyperion",
				author: "",  // no author dir above series; acceptable as "Unknown Author"
				series: "Hyperion Cantos",
				year:   "1989",
			},
		},
		{
			rel: "Hyperion Cantos/1990 - The Fall of Hyperion",
			want: want{
				title:  "The Fall of Hyperion",
				series: "Hyperion Cantos",
				year:   "1990",
			},
		},
		{
			rel: "Hyperion Cantos/1996 - Endymion",
			want: want{
				title:  "Endymion",
				series: "Hyperion Cantos",
				year:   "1996",
			},
		},
		{
			rel: "Hyperion Cantos/1997 - The Rise of Endymion",
			want: want{
				title:  "The Rise of Endymion",
				series: "Hyperion Cantos",
				year:   "1997",
			},
		},

		// ── The Expanse ───────────────────────────────────────────────────────
		{
			rel: "James S. A. Corey - The Expanse Series/The Expanse 01 Leviathan Wakes",
			want: want{
				title:  "Leviathan Wakes",
				author: "James S. A. Corey",
				series: "The Expanse",
				idx:    "1",
			},
		},
		{
			rel: "James S. A. Corey - The Expanse Series/The Expanse 02.5 Gods of Risk",
			want: want{
				title:  "Gods of Risk",
				author: "James S. A. Corey",
				series: "The Expanse",
				idx:    "2.5",
			},
		},
		{
			rel: "James S. A. Corey - The Expanse Series/The Expanse 00 The Churn",
			want: want{
				title:  "The Churn",
				author: "James S. A. Corey",
				series: "The Expanse",
				idx:    "0",
			},
		},

		// ── The Witcher ───────────────────────────────────────────────────────
		{
			rel: "The Witcher/Andrzej Sapkowski - Blood of Elves The Witcher, Book 1 (Unabridged)",
			want: want{
				title:  "Blood of Elves",
				author: "Andrzej Sapkowski",
				series: "The Witcher",
				idx:    "1",
			},
		},
		{
			rel: "The Witcher/Andrzej Sapkowski - Baptism of Fire The Witcher, Book 3 (Unabridged)",
			want: want{
				title:  "Baptism of Fire",
				author: "Andrzej Sapkowski",
				series: "The Witcher",
				idx:    "3",
			},
		},
	}

	pass, fail := 0, 0
	for _, tt := range tests {
		t.Run(tt.rel, func(t *testing.T) {
			// Skip entries whose directory doesn't exist on disk.
			if _, err := os.Stat(filepath.Join(root, tt.rel)); err != nil {
				t.Skipf("directory not found on disk: %s", tt.rel)
			}

			got := ParseFolderPath(tt.rel)

			type check struct{ field, got, want string }
			checks := []check{
				{"title", got.Title, tt.want.title},
				{"series", got.SeriesName, tt.want.series},
				{"year", got.PublishedYear, tt.want.year},
			}
			if tt.want.author != "" {
				checks = append(checks, check{"author", strings.Join(got.Authors, ", "), tt.want.author})
			}
			if tt.want.narrator != "" {
				checks = append(checks, check{"narrator", strings.Join(got.Narrators, ", "), tt.want.narrator})
			}
			if tt.want.idx != "" {
				gotIdx := ""
				if got.SeriesIndex != nil {
					gotIdx = fmt.Sprintf("%g", *got.SeriesIndex)
				}
				checks = append(checks, check{"idx", gotIdx, tt.want.idx})
			} else if got.SeriesIndex != nil {
				t.Errorf("idx: got %g, want <none>", *got.SeriesIndex)
				fail++
				return
			}

			ok := true
			for _, c := range checks {
				if c.got != c.want {
					t.Errorf("%s: got %q, want %q", c.field, c.got, c.want)
					ok = false
				}
			}
			if ok {
				pass++
			} else {
				fail++
			}
		})
	}

	t.Logf("results: %d pass, %d fail", pass, fail)
}
