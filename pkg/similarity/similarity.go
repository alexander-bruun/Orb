// Package similarity provides audio fingerprint extraction and track similarity
// computation using Chromaprint and metadata signals.
package similarity

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"math/bits"
	"os/exec"
	"sort"

	"github.com/alexander-bruun/orb/pkg/store"
)

// Weights for the similarity score components.
const (
	WeightAudio    = 0.35
	WeightGenre    = 0.35
	WeightArtist   = 0.20
	WeightDuration = 0.10

	// MaxSimilarPerTrack is the number of top-similar tracks to store per track.
	MaxSimilarPerTrack = 50
)

// FpcalcAvailable returns true if the fpcalc binary is on PATH.
func FpcalcAvailable() bool {
	_, err := exec.LookPath("fpcalc")
	return err == nil
}

// ExtractChromaprint runs fpcalc on the given audio file and returns the raw
// integer fingerprint and the duration (seconds) that fpcalc analyzed.
// Returns an error if fpcalc is not installed or fails.
func ExtractChromaprint(ctx context.Context, audioPath string) ([]int32, float64, error) {
	cmd := exec.CommandContext(ctx, "fpcalc", "-json", "-raw", audioPath)
	out, err := cmd.Output()
	if err != nil {
		return nil, 0, fmt.Errorf("fpcalc %s: %w", audioPath, err)
	}
	var result struct {
		Duration    float64 `json:"duration"`
		Fingerprint []int32 `json:"fingerprint"`
	}
	if err := json.Unmarshal(out, &result); err != nil {
		return nil, 0, fmt.Errorf("parse fpcalc output: %w", err)
	}
	return result.Fingerprint, result.Duration, nil
}

// ChromaprintSimilarity computes the similarity between two chromaprint
// fingerprints using hamming distance (XOR + popcount). Returns a value
// in [0, 1] where 1 means identical.
func ChromaprintSimilarity(a, b []int32) float64 {
	n := min(len(a), len(b))
	if n == 0 {
		return 0
	}
	var diffBits int
	for i := range n {
		diffBits += bits.OnesCount32(uint32(a[i] ^ b[i]))
	}
	totalBits := n * 32
	return 1.0 - float64(diffBits)/float64(totalBits)
}

// GenreSimilarity computes the Jaccard index between two genre-ID sets.
func GenreSimilarity(a, b []string) float64 {
	if len(a) == 0 && len(b) == 0 {
		return 0
	}
	setA := make(map[string]struct{}, len(a))
	for _, g := range a {
		setA[g] = struct{}{}
	}
	var intersection int
	union := make(map[string]struct{}, len(a)+len(b))
	for _, g := range a {
		union[g] = struct{}{}
	}
	for _, g := range b {
		union[g] = struct{}{}
		if _, ok := setA[g]; ok {
			intersection++
		}
	}
	if len(union) == 0 {
		return 0
	}
	return float64(intersection) / float64(len(union))
}

// ArtistSimilarity returns a score based on the relationship between two
// artists. Same artist = 1.0, related = 0.7, shared genres = 0.3, else 0.
func ArtistSimilarity(artistA, artistB string, related map[[2]string]bool, sharedGenres bool) float64 {
	if artistA == artistB {
		return 1.0
	}
	pair := [2]string{artistA, artistB}
	pairRev := [2]string{artistB, artistA}
	if related[pair] || related[pairRev] {
		return 0.7
	}
	if sharedGenres {
		return 0.3
	}
	return 0
}

// DurationSimilarity returns a score in [0, 1] based on how close two
// track durations (in milliseconds) are. Tracks within 5 minutes score
// proportionally; beyond 5 min difference scores 0.
func DurationSimilarity(durAms, durBms int) float64 {
	diff := math.Abs(float64(durAms - durBms))
	const maxDiffMs = 300_000 // 5 minutes
	if diff >= maxDiffMs {
		return 0
	}
	return 1.0 - diff/maxDiffMs
}

// ComputeScore produces a weighted similarity score from individual signals.
func ComputeScore(audioSim, genreSim, artistSim, durSim float64, hasAudio bool) float64 {
	if hasAudio {
		return WeightAudio*audioSim + WeightGenre*genreSim +
			WeightArtist*artistSim + WeightDuration*durSim
	}
	// No audio features — redistribute audio weight to genre and artist.
	return 0.50*genreSim + 0.35*artistSim + 0.15*durSim
}

// trackInfo holds the data needed for pairwise comparison, loaded once in bulk.
type trackInfo struct {
	ID         string
	ArtistID   string
	DurationMs int
	Genres     []string // merged track+album+artist genre IDs
	Chroma     []int32
}

// similarPair is a candidate result before top-K selection.
type similarPair struct {
	TrackA, TrackB string
	Score          float64
}

// ComputeAll loads all track data, computes pairwise similarity, and stores
// the top-K most similar pairs per track in the track_similarity table.
func ComputeAll(ctx context.Context, db *store.Store) error {
	slog.Info("loading track data for similarity computation")

	tracks, err := loadTrackInfos(ctx, db)
	if err != nil {
		return fmt.Errorf("load track infos: %w", err)
	}
	if len(tracks) < 2 {
		slog.Info("fewer than 2 tracks, skipping similarity computation")
		return nil
	}

	// Load related artists lookup.
	relatedPairs, err := db.ListAllRelatedArtists(ctx)
	if err != nil {
		return fmt.Errorf("load related artists: %w", err)
	}
	related := make(map[[2]string]bool, len(relatedPairs))
	for _, r := range relatedPairs {
		related[[2]string{r.ArtistID, r.RelatedID}] = true
	}

	// Build artist-genre lookup for shared-genre detection.
	artistGenreMap, err := db.ListAllArtistGenresMap(ctx)
	if err != nil {
		return fmt.Errorf("load artist genres: %w", err)
	}

	slog.Info("computing pairwise similarity", "tracks", len(tracks))

	// For each track, keep a bounded heap of top-K similar tracks.
	topK := make(map[string][]similarPair, len(tracks))

	for i := 0; i < len(tracks); i++ {
		for j := i + 1; j < len(tracks); j++ {
			a, b := tracks[i], tracks[j]

			hasAudio := len(a.Chroma) > 0 && len(b.Chroma) > 0
			var audioSim float64
			if hasAudio {
				audioSim = ChromaprintSimilarity(a.Chroma, b.Chroma)
			}

			genreSim := GenreSimilarity(a.Genres, b.Genres)

			// Check if artists share genres.
			sharedGenres := artistsShareGenre(a.ArtistID, b.ArtistID, artistGenreMap)
			artistSim := ArtistSimilarity(a.ArtistID, b.ArtistID, related, sharedGenres)

			durSim := DurationSimilarity(a.DurationMs, b.DurationMs)

			score := ComputeScore(audioSim, genreSim, artistSim, durSim, hasAudio)

			// Only store pairs with meaningful similarity.
			if score < 0.05 {
				continue
			}

			// Canonical ordering for storage.
			ta, tb := a.ID, b.ID
			if ta > tb {
				ta, tb = tb, ta
			}
			pair := similarPair{TrackA: ta, TrackB: tb, Score: score}

			// Add to both tracks' top-K lists.
			topK[a.ID] = appendTopK(topK[a.ID], pair, MaxSimilarPerTrack)
			topK[b.ID] = appendTopK(topK[b.ID], pair, MaxSimilarPerTrack)
		}
	}

	// Deduplicate pairs (a pair may appear in both tracks' top-K).
	seen := make(map[[2]string]bool)
	var allPairs []store.TrackSimilarityRow
	for _, pairs := range topK {
		for _, p := range pairs {
			key := [2]string{p.TrackA, p.TrackB}
			if seen[key] {
				continue
			}
			seen[key] = true
			allPairs = append(allPairs, store.TrackSimilarityRow{
				TrackA: p.TrackA,
				TrackB: p.TrackB,
				Score:  p.Score,
			})
		}
	}

	slog.Info("storing similarity pairs", "pairs", len(allPairs))
	if err := db.BatchUpsertSimilarity(ctx, allPairs); err != nil {
		return fmt.Errorf("batch upsert similarity: %w", err)
	}

	return nil
}

// loadTrackInfos loads all tracks with their features and merged genres.
func loadTrackInfos(ctx context.Context, db *store.Store) ([]trackInfo, error) {
	basics, err := db.ListAllTracksBasic(ctx)
	if err != nil {
		return nil, err
	}

	features, err := db.ListAllTrackFeatures(ctx)
	if err != nil {
		return nil, err
	}
	featMap := make(map[string][]int32, len(features))
	for _, f := range features {
		featMap[f.TrackID] = f.Chromaprint
	}

	// Load all genre associations.
	trackGenres, err := db.ListAllTrackGenresMap(ctx)
	if err != nil {
		return nil, err
	}
	albumGenres, err := db.ListAllAlbumGenresMap(ctx)
	if err != nil {
		return nil, err
	}
	artistGenres, err := db.ListAllArtistGenresMap(ctx)
	if err != nil {
		return nil, err
	}

	infos := make([]trackInfo, 0, len(basics))
	for _, t := range basics {
		// Merge genre sets from track, album, and artist.
		genreSet := make(map[string]struct{})
		for _, g := range trackGenres[t.ID] {
			genreSet[g] = struct{}{}
		}
		if t.AlbumID != "" {
			for _, g := range albumGenres[t.AlbumID] {
				genreSet[g] = struct{}{}
			}
		}
		if t.ArtistID != "" {
			for _, g := range artistGenres[t.ArtistID] {
				genreSet[g] = struct{}{}
			}
		}
		genres := make([]string, 0, len(genreSet))
		for g := range genreSet {
			genres = append(genres, g)
		}

		infos = append(infos, trackInfo{
			ID:         t.ID,
			ArtistID:   t.ArtistID,
			DurationMs: t.DurationMs,
			Genres:     genres,
			Chroma:     featMap[t.ID],
		})
	}
	return infos, nil
}

func artistsShareGenre(a, b string, artistGenres map[string][]string) bool {
	if a == "" || b == "" {
		return false
	}
	ga, gb := artistGenres[a], artistGenres[b]
	set := make(map[string]struct{}, len(ga))
	for _, g := range ga {
		set[g] = struct{}{}
	}
	for _, g := range gb {
		if _, ok := set[g]; ok {
			return true
		}
	}
	return false
}

// appendTopK maintains a bounded slice of top-K pairs sorted by score descending.
func appendTopK(pairs []similarPair, p similarPair, k int) []similarPair {
	if len(pairs) < k {
		pairs = append(pairs, p)
		sort.Slice(pairs, func(i, j int) bool { return pairs[i].Score > pairs[j].Score })
		return pairs
	}
	// Check if this score beats the worst in the list.
	if p.Score <= pairs[len(pairs)-1].Score {
		return pairs
	}
	pairs[len(pairs)-1] = p
	sort.Slice(pairs, func(i, j int) bool { return pairs[i].Score > pairs[j].Score })
	return pairs
}
