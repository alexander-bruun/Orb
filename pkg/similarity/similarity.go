// Package similarity implements advanced track similarity computation using
// a multi-signal metadata algorithm. No external binaries or system
// dependencies are required — all signals are derived from data already
// stored in the database.
//
// # Algorithm Overview
//
// Eight independent signals are computed for each track pair, each normalised
// to [0, 1], then blended using fixed weights that sum to 1.0:
//
//  1. Genre IDF similarity  (0.30) — IDF-weighted Jaccard over merged genre sets
//  2. Artist graph          (0.25) — multi-hop relationship traversal
//  3. Release era           (0.12) — Gaussian decay over release year distance
//  4. Audio tech profile    (0.10) — format tier, resolution tier, channel count
//  5. Album context         (0.08) — same album / variant group / album type
//  6. Duration proximity    (0.07) — exponential decay over duration difference
//  7. Title token overlap   (0.05) — token Jaccard, penalising near-identical titles
//  8. Co-play behavior      (0.03) — sessions where both tracks were played together
package similarity

import (
"context"
"fmt"
"log/slog"
"math"
"regexp"
"sort"
"strings"

"github.com/alexander-bruun/orb/pkg/store"
)

// ---------------------------------------------------------------------------
// Weights — must sum to 1.0
// ---------------------------------------------------------------------------

const (
wGenreIDF    = 0.30 // Signal 1: IDF-weighted genre Jaccard similarity
wArtistGraph = 0.25 // Signal 2: multi-hop artist relationship graph
wEra         = 0.12 // Signal 3: release year / decade proximity
wAudioTech   = 0.10 // Signal 4: format / bit-depth / sample-rate profile
wAlbumCtx    = 0.08 // Signal 5: same album / variant group / album type
wDuration    = 0.07 // Signal 6: song length proximity
wTitleToken  = 0.05 // Signal 7: title token overlap (version detection)
wCoPlay      = 0.03 // Signal 8: behavioral co-play session count

// MaxSimilarPerTrack is the maximum number of similar tracks stored per track.
MaxSimilarPerTrack = 50

// MinScore is the minimum combined score required to persist a pair.
MinScore = 0.05
)

// ---------------------------------------------------------------------------
// Internal data types
// ---------------------------------------------------------------------------

// trackFeatureVec holds all signals for one track, assembled before the
// pairwise comparison loop begins.
type trackFeatureVec struct {
ID                string
ArtistID          string
AlbumID           string
Title             string
TitleTokens       []string
DurationMs        int
Format            string
BitDepth          int    // 0 for lossy formats
SampleRate        int    // Hz
Channels          int    // 1 = mono, 2 = stereo, etc.
BitrateKbps       int    // 0 for lossless
ReleaseYear       int    // 0 = unknown
AlbumType         string // "Album" | "EP" | "Single" | "Live" | "Soundtrack" | …
AlbumGroupID      string // links alternate editions of the same record
Country           string // ISO 3166-1 alpha-2 artist country
ArtistType        string // "Person" | "Group" | …
Genres            []string // merged track + album + artist genre IDs
FeaturedArtistIDs []string // artists featured on this specific track
}

// similarPair is an intermediate result before top-K selection.
type similarPair struct {
TrackA, TrackB string
Score          float64
}

// ---------------------------------------------------------------------------
// Public entry point
// ---------------------------------------------------------------------------

// ComputeAll loads all track data, computes pairwise similarity using the
// multi-signal metadata algorithm, and stores the top-K most-similar pairs
// per track in the track_similarity table.
//
// This is the only exported function; it is called from the ingest pipeline
// after a library scan completes.
func ComputeAll(ctx context.Context, db *store.Store) error {
slog.Info("loading track data for similarity computation")

// ── 1. Core track data (tracks + albums + artists joined) ─────────────
basics, err := db.ListAllTrackInfosFull(ctx)
if err != nil {
return fmt.Errorf("load track infos: %w", err)
}
if len(basics) < 2 {
slog.Info("fewer than 2 tracks — skipping similarity computation")
return nil
}

// ── 2. Genre associations (three levels: track, album, artist) ─────────
trackGenres, err := db.ListAllTrackGenresMap(ctx)
if err != nil {
return fmt.Errorf("load track genres: %w", err)
}
albumGenres, err := db.ListAllAlbumGenresMap(ctx)
if err != nil {
return fmt.Errorf("load album genres: %w", err)
}
artistGenresMap, err := db.ListAllArtistGenresMap(ctx)
if err != nil {
return fmt.Errorf("load artist genres: %w", err)
}

// ── 3. Artist relationship graph ───────────────────────────────────────
relatedPairs, err := db.ListAllRelatedArtists(ctx)
if err != nil {
return fmt.Errorf("load related artists: %w", err)
}
// related[A][B] = true means A and B are directly related.
related := make(map[string]map[string]bool, 64)
for _, rp := range relatedPairs {
if related[rp.ArtistID] == nil {
related[rp.ArtistID] = make(map[string]bool)
}
related[rp.ArtistID][rp.RelatedID] = true
}

// ── 4. Featured-artist map (track → []artistID) ────────────────────────
featMap, err := db.ListAllFeaturedArtistsMap(ctx)
if err != nil {
return fmt.Errorf("load featured artists: %w", err)
}

// ── 5. Co-play behavioral matrix ───────────────────────────────────────
coPlayRaw, err := db.ListCoPlayCounts(ctx)
if err != nil {
return fmt.Errorf("load co-play data: %w", err)
}
coPlay := make(map[[2]string]int, len(coPlayRaw))
for _, cp := range coPlayRaw {
coPlay[[2]string{cp.TrackA, cp.TrackB}] = cp.Count
}

// ── 6. Assemble feature vectors ────────────────────────────────────────
vecs := make([]trackFeatureVec, 0, len(basics))
for _, b := range basics {
genres := mergeGenres(b.ID, b.AlbumID, b.ArtistID, trackGenres, albumGenres, artistGenresMap)
vecs = append(vecs, trackFeatureVec{
ID:                b.ID,
ArtistID:          b.ArtistID,
AlbumID:           b.AlbumID,
Title:             b.Title,
TitleTokens:       tokenizeTitle(b.Title),
DurationMs:        b.DurationMs,
Format:            b.Format,
BitDepth:          b.BitDepth,
SampleRate:        b.SampleRate,
Channels:          b.Channels,
BitrateKbps:       b.BitrateKbps,
ReleaseYear:       b.ReleaseYear,
AlbumType:         b.AlbumType,
AlbumGroupID:      b.AlbumGroupID,
Country:           b.Country,
ArtistType:        b.ArtistType,
Genres:            genres,
FeaturedArtistIDs: featMap[b.ID],
})
}

// ── 7. Compute genre IDF weights (used by Signal 1) ───────────────────
allGenreSets := make([][]string, len(vecs))
for i := range vecs {
allGenreSets[i] = vecs[i].Genres
}
genreIDF := computeGenreIDF(allGenreSets)

// ── 8. Build co-feature artist-pair set (used by Signal 2) ───────────
coFeatArtists := buildCoFeatArtistPairs(vecs)

slog.Info("computing pairwise similarity", "tracks", len(vecs))

// ── 9. Pairwise comparison with bounded top-K accumulation ────────────
topK := make(map[string][]similarPair, len(vecs))
for i := 0; i < len(vecs); i++ {
for j := i + 1; j < len(vecs); j++ {
a, b := &vecs[i], &vecs[j]
score := computeScore(a, b, genreIDF, related, coFeatArtists, coPlay)
if score < MinScore {
continue
}
ta, tb := canonicalPair(a.ID, b.ID)
pair := similarPair{TrackA: ta, TrackB: tb, Score: score}
topK[a.ID] = appendTopK(topK[a.ID], pair, MaxSimilarPerTrack)
topK[b.ID] = appendTopK(topK[b.ID], pair, MaxSimilarPerTrack)
}
}

// ── 10. Deduplicate and persist ────────────────────────────────────────
seen := make(map[[2]string]bool)
var rows []store.TrackSimilarityRow
for _, pairs := range topK {
for _, p := range pairs {
key := [2]string{p.TrackA, p.TrackB}
if seen[key] {
continue
}
seen[key] = true
rows = append(rows, store.TrackSimilarityRow{
TrackA: p.TrackA,
TrackB: p.TrackB,
Score:  p.Score,
})
}
}

slog.Info("storing similarity pairs", "pairs", len(rows))
if err := db.BatchUpsertSimilarity(ctx, rows); err != nil {
return fmt.Errorf("batch upsert similarity: %w", err)
}
return nil
}

// ---------------------------------------------------------------------------
// Score composition
// ---------------------------------------------------------------------------

// computeScore combines all eight signals into a single score in [0, 1].
func computeScore(
a, b *trackFeatureVec,
genreIDF map[string]float64,
related map[string]map[string]bool,
coFeatArtists map[[2]string]bool,
coPlay map[[2]string]int,
) float64 {
gSim := genreIDFSimilarity(a.Genres, b.Genres, genreIDF)
artSim := artistGraphSimilarity(a, b, related, coFeatArtists)
eraSim := eraSimilarity(a.ReleaseYear, b.ReleaseYear)
techSim := audioProfileSimilarity(a, b)
albSim := albumContextSimilarity(a, b)
durSim := durationSimilarity(a.DurationMs, b.DurationMs)
titleSim := titleTokenSimilarity(a.TitleTokens, b.TitleTokens)
cpSim := coPlaySimilarity(a.ID, b.ID, coPlay)

score := wGenreIDF*gSim +
wArtistGraph*artSim +
wEra*eraSim +
wAudioTech*techSim +
wAlbumCtx*albSim +
wDuration*durSim +
wTitleToken*titleSim +
wCoPlay*cpSim

return math.Min(1.0, math.Max(0.0, score))
}

// ---------------------------------------------------------------------------
// Signal 1: IDF-weighted Genre Jaccard Similarity
// ---------------------------------------------------------------------------

// computeGenreIDF calculates smoothed inverse-document-frequency weights for
// all genre IDs encountered across the library. Common genres (e.g. "rock")
// receive lower weight than rare ones (e.g. "baroque pop").
func computeGenreIDF(allGenreSets [][]string) map[string]float64 {
n := float64(len(allGenreSets))
freq := make(map[string]int)
for _, gs := range allGenreSets {
seen := make(map[string]struct{})
for _, g := range gs {
if _, ok := seen[g]; !ok {
freq[g]++
seen[g] = struct{}{}
}
}
}
idf := make(map[string]float64, len(freq))
for g, c := range freq {
// Smoothed IDF: log((N+1)/(c+1)) + 1  — never zero, rare genres weighted higher.
idf[g] = math.Log((n+1)/(float64(c)+1)) + 1.0
}
return idf
}

// genreIDFSimilarity returns an IDF-weighted Jaccard index in [0, 1].
// When neither track has any genres the result is 0 (no signal).
func genreIDFSimilarity(a, b []string, idf map[string]float64) float64 {
if len(a) == 0 && len(b) == 0 {
return 0
}
setA := make(map[string]struct{}, len(a))
for _, g := range a {
setA[g] = struct{}{}
}
var intersectW, unionW float64
seenUnion := make(map[string]struct{})
for _, g := range a {
if _, ok := seenUnion[g]; ok {
continue
}
seenUnion[g] = struct{}{}
unionW += idf[g]
}
for _, g := range b {
w := idf[g]
if _, ok := setA[g]; ok {
intersectW += w
}
if _, ok := seenUnion[g]; !ok {
seenUnion[g] = struct{}{}
unionW += w
}
}
if unionW == 0 {
return 0
}
return intersectW / unionW
}

// ---------------------------------------------------------------------------
// Signal 2: Artist Relationship Graph (multi-hop)
// ---------------------------------------------------------------------------

// artistGraphSimilarity traverses the MusicBrainz-derived artist relationship
// graph up to two hops. It also checks for co-featured artists and geographic /
// type affinity as a soft fallback.
//
// Score mapping:
//   - Same artist          → 1.00
//   - Direct graph edge    → 0.75
//   - Co-featured on track → 0.60
//   - Two-hop path         → 0.35
//   - Same country + type  → 0.20
//   - Same country only    → 0.10
//   - No relation found    → 0.00
func artistGraphSimilarity(
a, b *trackFeatureVec,
related map[string]map[string]bool,
coFeatArtists map[[2]string]bool,
) float64 {
if a.ArtistID == "" || b.ArtistID == "" {
return 0
}
if a.ArtistID == b.ArtistID {
return 1.0
}
if related[a.ArtistID][b.ArtistID] || related[b.ArtistID][a.ArtistID] {
return 0.75
}
if coFeatArtists[canonicalArtistPair(a.ArtistID, b.ArtistID)] {
return 0.60
}
// Two-hop: a → intermediary → b
for nb := range related[a.ArtistID] {
if related[nb][b.ArtistID] || related[b.ArtistID][nb] {
return 0.35
}
}
sameCountry := a.Country != "" && a.Country == b.Country
sameType := a.ArtistType != "" && a.ArtistType == b.ArtistType
if sameCountry && sameType {
return 0.20
}
if sameCountry {
return 0.10
}
return 0.0
}

// buildCoFeatArtistPairs builds a canonical (artistA, artistB) pair set for
// all artists that appear together on the same track (main + featured).
func buildCoFeatArtistPairs(vecs []trackFeatureVec) map[[2]string]bool {
out := make(map[[2]string]bool)
for _, v := range vecs {
all := make([]string, 0, 1+len(v.FeaturedArtistIDs))
if v.ArtistID != "" {
all = append(all, v.ArtistID)
}
all = append(all, v.FeaturedArtistIDs...)
for i := 0; i < len(all); i++ {
for j := i + 1; j < len(all); j++ {
out[canonicalArtistPair(all[i], all[j])] = true
}
}
}
return out
}

func canonicalArtistPair(a, b string) [2]string {
if a < b {
return [2]string{a, b}
}
return [2]string{b, a}
}

// ---------------------------------------------------------------------------
// Signal 3: Release Era Proximity
// ---------------------------------------------------------------------------

// eraSimilarity applies a Gaussian decay to the release year difference.
// σ = 12 years: same decade ≈ 1.0, 20 years apart ≈ 0.25, 40 years ≈ 0.03.
// Unknown years (0) return 0.5 — neutral, no penalty.
func eraSimilarity(yearA, yearB int) float64 {
if yearA == 0 || yearB == 0 {
return 0.5
}
diff := float64(yearA - yearB)
const sigma = 12.0
return math.Exp(-(diff * diff) / (2 * sigma * sigma))
}

// ---------------------------------------------------------------------------
// Signal 4: Audio Technical Profile
// ---------------------------------------------------------------------------

// audioProfileSimilarity scores three sub-signals:
//   - Format tier (lossless vs. lossy)
//   - Resolution tier (sample-rate × bit-depth or bitrate for lossy)
//   - Channel count
func audioProfileSimilarity(a, b *trackFeatureVec) float64 {
var score, weight float64

// Format tier: lossless vs. lossy.
tA, tB := formatTier(a), formatTier(b)
if tA == tB {
score += 1.0
} else {
score += 0.30
}
weight += 1.0

// Resolution tier: higher difference = lower similarity.
rA, rB := resolutionTier(a), resolutionTier(b)
diff := math.Abs(float64(rA - rB))
score += math.Max(0, 1.0-diff*0.30)
weight += 1.0

// Channel count.
if a.Channels > 0 && b.Channels > 0 {
if a.Channels == b.Channels {
score += 1.0
} else {
score += 0.40
}
weight += 0.5
}

if weight == 0 {
return 0.5
}
return score / weight
}

// formatTier classifies a track as lossless (1) or lossy (0).
func formatTier(t *trackFeatureVec) int {
switch strings.ToLower(t.Format) {
case "flac", "wav", "aiff", "alac", "ape", "wv", "wavpack":
return 1
default:
return 0
}
}

// resolutionTier maps a track's technical spec to an integer tier 0–6.
//
// Lossless tiers (by sample rate × bit depth):
//
//6 = ≥176.4 kHz
//5 = ≥88.2 kHz
//4 = ≥48 kHz / 24-bit
//3 = 44.1 kHz / 24-bit  (HiRes CD)
//2 = 44.1 kHz / 16-bit  (CD)
//1 = <44.1 kHz
//
// Lossy tiers (by bitrate):
//
//3 = ≥320 kbps
//2 = ≥192 kbps
//1 = ≥128 kbps
//0 = <128 kbps
func resolutionTier(t *trackFeatureVec) int {
if t.BitDepth > 0 {
switch {
case t.SampleRate >= 176400:
return 6
case t.SampleRate >= 88200:
return 5
case t.SampleRate >= 48000 && t.BitDepth >= 24:
return 4
case t.SampleRate >= 44100 && t.BitDepth >= 24:
return 3
case t.SampleRate >= 44100 && t.BitDepth >= 16:
return 2
default:
return 1
}
}
switch {
case t.BitrateKbps >= 320:
return 3
case t.BitrateKbps >= 192:
return 2
case t.BitrateKbps >= 128:
return 1
default:
return 0
}
}

// ---------------------------------------------------------------------------
// Signal 5: Album Context
// ---------------------------------------------------------------------------

// albumContextSimilarity returns a score based on how closely related two
// tracks' albums are:
//
//   - Same album                   → 0.90 (strong structural signal)
//   - Same album group (variant)   → 0.65 (e.g. deluxe vs standard edition)
//   - Same album type (both Live)  → type-dependent bonus 0.10–0.35
//   - No relation                  → 0.00
func albumContextSimilarity(a, b *trackFeatureVec) float64 {
if a.AlbumID != "" && a.AlbumID == b.AlbumID {
return 0.90
}
if a.AlbumGroupID != "" && a.AlbumGroupID == b.AlbumGroupID {
return 0.65
}
if a.AlbumType != "" && a.AlbumType == b.AlbumType {
switch strings.ToLower(a.AlbumType) {
case "live":
return 0.35
case "soundtrack":
return 0.30
case "compilation":
return 0.15
case "single", "ep":
return 0.12
default: // "album"
return 0.08
}
}
return 0.0
}

// ---------------------------------------------------------------------------
// Signal 6: Duration Proximity
// ---------------------------------------------------------------------------

// durationSimilarity applies an exponential decay: similarity halves for every
// ~2 minutes of difference. Unknown durations (0) return a neutral 0.5.
func durationSimilarity(msA, msB int) float64 {
if msA == 0 || msB == 0 {
return 0.5
}
diff := math.Abs(float64(msA - msB))
return math.Exp(-diff / 120_000.0)
}

// ---------------------------------------------------------------------------
// Signal 7: Title Token Similarity (version / variant detection)
// ---------------------------------------------------------------------------

// nonAlphanumRe strips everything that is not a letter, digit, or whitespace.
var nonAlphanumRe = regexp.MustCompile(`[^a-z0-9\s]`)

// musicStopWords are words that carry little semantic content in track titles.
var musicStopWords = map[string]bool{
"the": true, "a": true, "an": true, "and": true, "or": true,
"of": true, "in": true, "to": true, "ft": true, "feat": true,
"featuring": true, "with": true, "vs": true, "remix": true,
"live": true, "edit": true, "mix": true, "version": true,
"remaster": true, "remastered": true, "extended": true,
"radio": true, "acoustic": true, "bonus": true, "track": true,
}

// tokenizeTitle returns a slice of lowercase, punctuation-free, stop-word-free
// tokens from a track title. Single-character tokens are discarded.
func tokenizeTitle(title string) []string {
lower := strings.ToLower(title)
clean := nonAlphanumRe.ReplaceAllString(lower, " ")
parts := strings.Fields(clean)
out := make([]string, 0, len(parts))
for _, p := range parts {
if len(p) > 1 && !musicStopWords[p] {
out = append(out, p)
}
}
return out
}

// titleTokenSimilarity computes the Jaccard index over title token sets.
// Very high overlap (>0.80) is softened because near-identical titles likely
// denote the same song rather than a "similar but different" track.
func titleTokenSimilarity(tokA, tokB []string) float64 {
if len(tokA) == 0 || len(tokB) == 0 {
return 0.0
}
setA := make(map[string]struct{}, len(tokA))
for _, t := range tokA {
setA[t] = struct{}{}
}
setB := make(map[string]struct{}, len(tokB))
for _, t := range tokB {
setB[t] = struct{}{}
}
var inter int
for t := range setB {
if _, ok := setA[t]; ok {
inter++
}
}
union := len(setA) + len(setB) - inter
if union == 0 {
return 0
}
j := float64(inter) / float64(union)
// Penalise near-identical titles: they are probably the same song in a
// different edition, not genuinely "similar" content.
if j > 0.80 {
return 0.50 + 0.50*j
}
return j
}

// ---------------------------------------------------------------------------
// Signal 8: Co-play Behavioral Similarity
// ---------------------------------------------------------------------------

// coPlaySimilarity returns a score in [0, 1] based on how many distinct user
// sessions contained both tracks. Uses log-normalisation that saturates at 100
// co-play sessions.
func coPlaySimilarity(idA, idB string, coPlay map[[2]string]int) float64 {
ta, tb := canonicalPair(idA, idB)
count := coPlay[[2]string{ta, tb}]
if count == 0 {
return 0
}
return math.Min(1.0, math.Log1p(float64(count))/math.Log1p(100))
}

// ---------------------------------------------------------------------------
// Helpers shared across signals
// ---------------------------------------------------------------------------

// mergeGenres returns a deduplicated slice of genre IDs for a track, drawing
// from track-level, album-level, and artist-level genre associations.
func mergeGenres(
trackID, albumID, artistID string,
trackGenres, albumGenres, artistGenres map[string][]string,
) []string {
seen := make(map[string]struct{})
var out []string
add := func(gs []string) {
for _, g := range gs {
if _, ok := seen[g]; !ok {
seen[g] = struct{}{}
out = append(out, g)
}
}
}
add(trackGenres[trackID])
add(albumGenres[albumID])
add(artistGenres[artistID])
return out
}

// canonicalPair returns (a, b) with the lexicographically smaller ID first,
// ensuring the same pair is always represented the same way regardless of
// which track appeared in the outer / inner loop position.
func canonicalPair(a, b string) (string, string) {
if a < b {
return a, b
}
return b, a
}

// appendTopK maintains a fixed-size sorted slice of the top-K pairs by score.
// When the new entry beats the current worst, it replaces it and re-sorts.
func appendTopK(pairs []similarPair, p similarPair, k int) []similarPair {
if len(pairs) < k {
pairs = append(pairs, p)
sort.Slice(pairs, func(i, j int) bool { return pairs[i].Score > pairs[j].Score })
return pairs
}
if p.Score <= pairs[len(pairs)-1].Score {
return pairs
}
pairs[len(pairs)-1] = p
sort.Slice(pairs, func(i, j int) bool { return pairs[i].Score > pairs[j].Score })
return pairs
}
