package musicbrainz

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"log/slog"
	"strings"
)

// ArtistEnrichment holds metadata extracted from MusicBrainz for an artist.
type ArtistEnrichment struct {
	Mbid           string
	ArtistType     string
	Country        string
	BeginDate      string
	EndDate        string
	Disambiguation string
	Genres         []string // genre names
	RelatedArtists []RelatedArtistInfo
	URLRelations   []MBRelation // URL relationships for image fetching
}

// RelatedArtistInfo describes a related artist from MusicBrainz.
type RelatedArtistInfo struct {
	Name    string
	Mbid    string
	RelType string
}

// AlbumEnrichment holds metadata extracted from MusicBrainz for an album.
type AlbumEnrichment struct {
	ReleaseGroupMbid string
	AlbumType        string
	Label            string
	ReleaseDate      string
	Genres           []string
}

// TrackEnrichment holds metadata extracted from MusicBrainz for a track.
type TrackEnrichment struct {
	RecordingMbid string
	Isrc          string
	Genres        []string
}

// EnrichArtist searches MusicBrainz for an artist, picks the best match, and returns enrichment data.
func (c *Client) EnrichArtist(ctx context.Context, name string) (*ArtistEnrichment, error) {
	searchResp, err := c.SearchArtist(ctx, name)
	if err != nil {
		return nil, err
	}
	if len(searchResp.Artists) == 0 {
		slog.Debug("musicbrainz: no artist results", "name", name)
		return nil, nil
	}

	best := searchResp.Artists[0]
	if best.Score < 90 {
		slog.Debug("musicbrainz: artist score too low", "name", name, "score", best.Score, "match", best.Name)
		return nil, nil
	}

	// Fetch full details with genres and relationships.
	detail, err := c.GetArtist(ctx, best.ID)
	if err != nil {
		slog.Warn("musicbrainz: failed to get artist detail", "mbid", best.ID, "err", err)
		// Fall back to search result data.
		detail = &best
	}

	enrichment := &ArtistEnrichment{
		Mbid:           detail.ID,
		ArtistType:     detail.Type,
		Country:        detail.Country,
		BeginDate:      detail.LifeSpan.Begin,
		EndDate:        detail.LifeSpan.End,
		Disambiguation: detail.Disambiguation,
		Genres:         extractGenres(detail.Genres, detail.Tags),
	}

	// Extract related artists and URL relationships from relationships.
	for _, rel := range detail.Relations {
		if rel.Artist != nil && rel.TargetType == "artist" {
			enrichment.RelatedArtists = append(enrichment.RelatedArtists, RelatedArtistInfo{
				Name:    rel.Artist.Name,
				Mbid:    rel.Artist.ID,
				RelType: rel.Type,
			})
		}
		if rel.URL != nil && rel.TargetType == "url" {
			enrichment.URLRelations = append(enrichment.URLRelations, rel)
		}
	}

	slog.Info("musicbrainz: enriched artist", "name", name, "mbid", detail.ID, "genres", len(enrichment.Genres))
	return enrichment, nil
}

// EnrichAlbum searches MusicBrainz for a release group, picks the best match, and returns enrichment data.
func (c *Client) EnrichAlbum(ctx context.Context, title, artistName string) (*AlbumEnrichment, error) {
	searchResp, err := c.SearchReleaseGroup(ctx, title, artistName)
	if err != nil {
		return nil, err
	}
	if len(searchResp.ReleaseGroups) == 0 {
		slog.Debug("musicbrainz: no release group results", "title", title, "artist", artistName)
		return nil, nil
	}

	best := searchResp.ReleaseGroups[0]
	if best.Score < 85 {
		slog.Debug("musicbrainz: release group score too low", "title", title, "score", best.Score, "match", best.Title)
		return nil, nil
	}

	// Fetch full details with genres and releases (for label).
	detail, err := c.GetReleaseGroup(ctx, best.ID)
	if err != nil {
		slog.Warn("musicbrainz: failed to get release group detail", "mbid", best.ID, "err", err)
		detail = &best
	}

	enrichment := &AlbumEnrichment{
		ReleaseGroupMbid: detail.ID,
		AlbumType:        detail.PrimaryType,
		ReleaseDate:      detail.FirstRelease,
		Genres:           extractGenres(detail.Genres, detail.Tags),
	}

	// Extract label from the first release that has label info.
	for _, rel := range detail.Releases {
		for _, li := range rel.LabelInfo {
			if li.Label.Name != "" {
				enrichment.Label = li.Label.Name
				break
			}
		}
		if enrichment.Label != "" {
			break
		}
	}

	slog.Info("musicbrainz: enriched album", "title", title, "artist", artistName, "mbid", detail.ID, "genres", len(enrichment.Genres))
	return enrichment, nil
}

// EnrichTrack searches MusicBrainz for a recording, picks the best match, and returns enrichment data.
func (c *Client) EnrichTrack(ctx context.Context, title, artistName string) (*TrackEnrichment, error) {
	searchResp, err := c.SearchRecording(ctx, title, artistName)
	if err != nil {
		return nil, err
	}
	if len(searchResp.Recordings) == 0 {
		slog.Debug("musicbrainz: no recording results", "title", title, "artist", artistName)
		return nil, nil
	}

	best := searchResp.Recordings[0]
	if best.Score < 80 {
		slog.Debug("musicbrainz: recording score too low", "title", title, "score", best.Score, "match", best.Title)
		return nil, nil
	}

	// Fetch full details with genres and ISRCs.
	detail, err := c.GetRecording(ctx, best.ID)
	if err != nil {
		slog.Warn("musicbrainz: failed to get recording detail", "mbid", best.ID, "err", err)
		detail = &best
	}

	enrichment := &TrackEnrichment{
		RecordingMbid: detail.ID,
		Genres:        extractGenres(detail.Genres, detail.Tags),
	}

	if len(detail.ISRCs) > 0 {
		enrichment.Isrc = detail.ISRCs[0]
	}

	slog.Info("musicbrainz: enriched track", "title", title, "artist", artistName, "mbid", detail.ID)
	return enrichment, nil
}

// extractGenres returns genre names from MusicBrainz genres and tags.
// Prefers curated genres; falls back to user-submitted tags with count > 0.
func extractGenres(genres []MBGenre, tags []MBTag) []string {
	if len(genres) > 0 {
		names := make([]string, 0, len(genres))
		for _, g := range genres {
			if g.Name != "" {
				names = append(names, g.Name)
			}
		}
		if len(names) > 0 {
			return names
		}
	}
	// Fall back to tags.
	names := make([]string, 0)
	for _, t := range tags {
		if t.Count > 0 && isGenreLike(t.Name) {
			names = append(names, t.Name)
		}
	}
	return names
}

// isGenreLike returns true if a tag name looks like a genre (lowercase, no special chars beyond hyphens/spaces).
func isGenreLike(name string) bool {
	if name == "" {
		return false
	}
	for _, r := range name {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-' || r == ' ' {
			continue
		}
		return false
	}
	return true
}

// GenreID returns a deterministic ID for a genre name.
func GenreID(name string) string {
	h := sha256.Sum256([]byte("genre:" + strings.ToLower(name)))
	return hex.EncodeToString(h[:8])
}
