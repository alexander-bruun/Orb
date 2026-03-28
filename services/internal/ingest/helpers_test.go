package ingest

import (
	"regexp"
	"strings"
	"testing"
)

// ---- splitGenreList ----

func TestSplitGenreList(t *testing.T) {
	tests := []struct {
		input string
		want  []string
	}{
		{"", nil},
		{"Rock", []string{"Rock"}},
		{"Rock, Pop", []string{"Rock", "Pop"}},
		{"Rock; Pop", []string{"Rock", "Pop"}},
		{"Rock/Pop", []string{"Rock", "Pop"}},
		{"Rock\x00Pop", []string{"Rock", "Pop"}},
		{"Rock,  Pop ,Jazz", []string{"Rock", "Pop", "Jazz"}},
		{"  Rock  ", []string{"Rock"}},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := splitGenreList(tt.input)
			if len(got) != len(tt.want) {
				t.Fatalf("splitGenreList(%q) = %v, want %v", tt.input, got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("splitGenreList(%q)[%d] = %q, want %q", tt.input, i, got[i], tt.want[i])
				}
			}
		})
	}
}

// ---- dedupeArtistNames ----

func TestDedupeArtistNames(t *testing.T) {
	tests := []struct {
		name    string
		names   []string
		exclude []string
		want    []string
	}{
		{"empty", nil, nil, []string{}},
		{"no duplicates", []string{"Alice", "Bob"}, nil, []string{"Alice", "Bob"}},
		{"case-insensitive dup", []string{"Alice", "alice", "ALICE"}, nil, []string{"Alice"}},
		{"whitespace trimmed", []string{"  Alice  ", "Alice"}, nil, []string{"Alice"}},
		{"blank entries skipped", []string{"", "  ", "Bob"}, nil, []string{"Bob"}},
		{"exclude removes entry", []string{"Alice", "Bob"}, []string{"Alice"}, []string{"Bob"}},
		{"exclude case-insensitive", []string{"Alice", "Bob"}, []string{"alice"}, []string{"Bob"}},
		{"all excluded", []string{"Alice"}, []string{"Alice"}, []string{}},
		{"preserves original casing", []string{"The Beatles", "the beatles"}, nil, []string{"The Beatles"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := dedupeArtistNames(tt.names, tt.exclude...)
			if len(got) != len(tt.want) {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("[%d] got %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

// ---- albumEditionFromDir ----

func TestAlbumEditionFromDir(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"Dark Side of the Moon", ""},
		{"Dark Side of the Moon [Remaster]", "[Remaster]"},
		{"Dark Side of the Moon {Deluxe Edition}", "{Deluxe Edition}"},
		{"Abbey Road [2019 Mix] {Super Deluxe}", "[2019 Mix] {Super Deluxe}"},
		{"(Parentheses are ignored)", ""},
		{"Album [Disc 1] [Bonus Tracks]", "[Disc 1] [Bonus Tracks]"},
		{"[Just Brackets]", "[Just Brackets]"},
		{"{Just Braces}", "{Just Braces}"},
		{"Normal (Feat. Someone) [Live]", "[Live]"},
	// Edition embedded in album tag (not just dir name)
	{"12 Notes [16 Notes]", "[16 Notes]"},
	{"12 Notes (Deluxe) [16 Notes]", "[16 Notes]"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := albumEditionFromDir(tt.input)
			if got != tt.want {
				t.Errorf("albumEditionFromDir(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// ---- editionParenRe / album title stripping ----

func TestEditionParenRe(t *testing.T) {
	strip := func(s string) string {
		return strings.TrimSpace(editionParenRe.ReplaceAllString(
			editionBracketsRe.ReplaceAllString(s, ""), ""))
	}

	tests := []struct {
		input string
		want  string
	}{
		{"12 Notes", "12 Notes"},
		{"12 Notes (Deluxe)", "12 Notes"},
		{"12 Notes (Deluxe Edition)", "12 Notes"},
		{"12 Notes (Deluxe) [16 Notes]", "12 Notes"},
		{"Abbey Road (2019 Remaster)", "Abbey Road"},
		{"Abbey Road (Remastered)", "Abbey Road"},
		{"OK Computer (Special Edition)", "OK Computer"},
		{"The Wall (Limited Edition)", "The Wall"},
		{"Rumours (Expanded Edition)", "Rumours"},
		{"Kind of Blue (50th Anniversary Edition)", "Kind of Blue"},
		{"Thriller (Bonus Track Edition)", "Thriller"},
		// Should NOT strip — no edition keyword
		{"The Dark Side of the Moon", "The Dark Side of the Moon"},
		{"Live at Wembley (Original Soundtrack)", "Live at Wembley (Original Soundtrack)"},
		{"Something (A Song)", "Something (A Song)"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := strip(tt.input)
			if got != tt.want {
				t.Errorf("strip(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// ---- parseBPMTag ----

func TestParseBPMTag(t *testing.T) {
	tests := []struct {
		input string
		want  float64
	}{
		{"", 0},
		{"   ", 0},
		{"abc", 0},
		{"0", 0},
		{"-1", 0},
		{"401", 0},
		{"400", 400},
		{"120", 120},
		{"120.5", 120.5},
		{"120.15", 120.2},
		{"0.1", 0.1},
		{"1", 1},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := parseBPMTag(tt.input)
			if got != tt.want {
				t.Errorf("parseBPMTag(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

// ---- normalizeKeyTag ----

func TestNormalizeKeyTag(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"", ""},
		{"   ", ""},
		{"none", ""},
		{"NONE", ""},
		{"None", ""},
		{"unknown", ""},
		{"UNKNOWN", ""},
		{"Unknown", ""},
		{"Am", "Am"},
		{"am", "Am"},
		{"cM", "CM"},
		{"  Bb  ", "Bb"},
		{"F#m", "F#m"},
		{"a", "A"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := normalizeKeyTag(tt.input)
			if got != tt.want {
				t.Errorf("normalizeKeyTag(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// ---- parseReplayGainTag ----

func TestParseReplayGainTag(t *testing.T) {
	tests := []struct {
		input string
		want  float64
	}{
		{"", 0},
		{"   ", 0},
		{"abc", 0},
		{"-6.0 dB", -6.0},
		{"-6.0 DB", -6.0},
		{"-6.0 dB", -6.0},
		{"-6.0db", -6.0},
		{"-6.0DB", -6.0},
		{"+3.5 dB", 3.5},
		{"0 dB", 0},
		{"-6.0", -6.0},
		{"3.14", 3.14},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := parseReplayGainTag(tt.input)
			if got != tt.want {
				t.Errorf("parseReplayGainTag(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

// ---- rawTagString ----

func TestRawTagString(t *testing.T) {
	tests := []struct {
		name string
		raw  map[string]interface{}
		keys []string
		want string
	}{
		{"no keys", map[string]interface{}{"a": "x"}, nil, ""},
		{"miss", map[string]interface{}{"a": "x"}, []string{"b"}, ""},
		{"string hit", map[string]interface{}{"a": "Alice"}, []string{"a"}, "Alice"},
		{"empty string skipped", map[string]interface{}{"a": "", "b": "Bob"}, []string{"a", "b"}, "Bob"},
		{"slice joined", map[string]interface{}{"a": []string{"Alice", "Bob"}}, []string{"a"}, "Alice, Bob"},
		{"empty slice skipped", map[string]interface{}{"a": []string{}, "b": "Carol"}, []string{"a", "b"}, "Carol"},
		{"first wins", map[string]interface{}{"a": "Alice", "b": "Bob"}, []string{"a", "b"}, "Alice"},
		{"non-string ignored", map[string]interface{}{"a": 42, "b": "Bob"}, []string{"a", "b"}, "Bob"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := rawTagString(tt.raw, tt.keys...)
			if got != tt.want {
				t.Errorf("rawTagString(%v, %v) = %q, want %q", tt.raw, tt.keys, got, tt.want)
			}
		})
	}
}

// ---- strPtr ----

func TestStrPtr(t *testing.T) {
	if strPtr("") != nil {
		t.Error("strPtr(\"\") should return nil")
	}
	got := strPtr("hello")
	if got == nil {
		t.Fatal("strPtr(\"hello\") should not be nil")
	}
	if *got != "hello" {
		t.Errorf("*strPtr(\"hello\") = %q, want \"hello\"", *got)
	}
	if p := strPtr("  "); p == nil || *p != "  " {
		t.Error("strPtr(\"  \") should return a pointer (whitespace is non-empty)")
	}
}

// ---- isAudioFile ----

func TestIsAudioFile(t *testing.T) {
	yes := []string{
		"song.flac", "song.FLAC", "song.FlAc",
		"song.mp3", "song.MP3",
		"song.wav", "song.WAV",
		"song.aiff", "song.AIFF",
		"song.aif", "song.AIF",
		"/path/to/song.flac",
		"song.remix.mp3",
	}
	no := []string{
		"song.txt", "song.jpg", "song.pdf", "song",
		"song.mp4", "song.ogg", "song.m4a",
		"flac", ".flac_backup",
	}
	for _, p := range yes {
		if !isAudioFile(p) {
			t.Errorf("isAudioFile(%q) = false, want true", p)
		}
	}
	for _, p := range no {
		if isAudioFile(p) {
			t.Errorf("isAudioFile(%q) = true, want false", p)
		}
	}
}

// ---- matchExcludeGlob ----

func TestMatchExcludeGlob(t *testing.T) {
	tests := []struct {
		pattern string
		path    string
		want    bool
	}{
		// Simple name matches any component
		{"temp", "/music/temp", true},
		{"temp", "/music/temp/files", true},
		{"temp", "/music/temporary", false},
		{"*.tmp", "/music/cache.tmp", true},
		{"*.tmp", "/music/sub/cache.tmp", true},
		{"*.tmp", "/music/cache.mp3", false},

		// Full path match
		{"/music/temp", "/music/temp", true},
		{"/music/temp", "/music/temp/sub", false},

		// ** patterns
		{"**/temp", "/music/temp", true},
		{"**/temp", "/a/b/temp", true},
		{"**/temp", "/a/b/temporary", false},
		{"**/*.log", "/music/logs/error.log", true},
		{"**/*.log", "/music/error.log", true},
		{"**/*.log", "/music/error.mp3", false},
	}
	for _, tt := range tests {
		t.Run(tt.pattern+"|"+tt.path, func(t *testing.T) {
			got := matchExcludeGlob(tt.pattern, tt.path)
			if got != tt.want {
				t.Errorf("matchExcludeGlob(%q, %q) = %v, want %v", tt.pattern, tt.path, got, tt.want)
			}
		})
	}
}

// ---- deterministicID ----

func TestDeterministicID(t *testing.T) {
	id := deterministicID("hello")
	if len(id) != 16 {
		t.Errorf("deterministicID length = %d, want 16", len(id))
	}
	if matched, _ := regexp.MatchString(`^[0-9a-f]{16}$`, id); !matched {
		t.Errorf("deterministicID(%q) = %q, not hex", "hello", id)
	}
	// Deterministic
	if deterministicID("hello") != id {
		t.Error("deterministicID is not deterministic")
	}
	// Different seeds produce different IDs
	if deterministicID("world") == id {
		t.Error("deterministicID collision on different seeds")
	}
	// Empty seed works
	empty := deterministicID("")
	if len(empty) != 16 {
		t.Errorf("deterministicID(\"\") length = %d, want 16", len(empty))
	}
}

// ---- deterministicUUID ----

func TestDeterministicUUID(t *testing.T) {
	uuidRe := regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

	u := deterministicUUID("some-fingerprint")
	if !uuidRe.MatchString(u) {
		t.Errorf("deterministicUUID = %q, not a valid v4 UUID", u)
	}
	// Deterministic
	if deterministicUUID("some-fingerprint") != u {
		t.Error("deterministicUUID is not deterministic")
	}
	// Different inputs differ
	if deterministicUUID("other-fingerprint") == u {
		t.Error("deterministicUUID collision on different inputs")
	}
	// Version nibble must be 4
	parts := strings.Split(u, "-")
	if parts[2][0] != '4' {
		t.Errorf("UUID version nibble = %c, want 4", parts[2][0])
	}
	// Variant bits: first char of group 4 must be 8, 9, a, or b
	v := parts[3][0]
	if v != '8' && v != '9' && v != 'a' && v != 'b' {
		t.Errorf("UUID variant nibble = %c, want 8/9/a/b", v)
	}
}

// ---- sortName ----

func TestSortName(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"The Beatles", "Beatles, The"},
		{"A Tribe Called Quest", "Tribe Called Quest, A"},
		{"An Horse", "Horse, An"},
		{"Radiohead", "Radiohead"},
		{"Them Crooked Vultures", "Them Crooked Vultures"},
		{"the lowercase", "the lowercase"}, // case-sensitive: only matches "The "
		{"Theatre of Tragedy", "Theatre of Tragedy"},
		{"Another Day", "Another Day"}, // "An" prefix but "Another" ≠ "An "
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := sortName(tt.input)
			if got != tt.want {
				t.Errorf("sortName(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// ---- coalesce ----

func TestCoalesce(t *testing.T) {
	if coalesce() != "" {
		t.Error("coalesce() should return empty string")
	}
	if coalesce("", "", "") != "" {
		t.Error("coalesce of all-empty should return empty string")
	}
	if got := coalesce("", "first", "second"); got != "first" {
		t.Errorf("coalesce = %q, want \"first\"", got)
	}
	if got := coalesce("zero", "first"); got != "zero" {
		t.Errorf("coalesce = %q, want \"zero\"", got)
	}
	if got := coalesce("  "); got != "  " {
		t.Error("coalesce should treat whitespace-only as non-empty")
	}
}

// ---- abs ----

func TestAbs(t *testing.T) {
	tests := []struct{ in, want int }{
		{0, 0},
		{1, 1},
		{-1, 1},
		{100, 100},
		{-100, 100},
	}
	for _, tt := range tests {
		if got := abs(tt.in); got != tt.want {
			t.Errorf("abs(%d) = %d, want %d", tt.in, got, tt.want)
		}
	}
}
