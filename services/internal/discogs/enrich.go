package discogs

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
)

// ArtistEnrichment holds metadata extracted from Discogs for an artist.
type ArtistEnrichment struct {
	DiscogsID  int
	Name       string
	Profile    string
	Genres     []string // genres gathered from search result
	ImageURI   string   // primary image URL (requires auth to download)
}

// AlbumEnrichment holds metadata extracted from Discogs for an album.
type AlbumEnrichment struct {
	DiscogsID  int
	Title      string
	Year       string   // "2021"
	Genres     []string // broad genres
	Styles     []string // more specific styles (e.g. "Alternative Rock")
	Label      string
	CoverImage string // URL to download (requires auth)
}

// EnrichArtist searches Discogs for an artist by name and returns enrichment data.
// Returns nil (no error) when no confident match is found.
func (c *Client) EnrichArtist(ctx context.Context, name string) (*ArtistEnrichment, error) {
	searchResp, err := c.SearchArtists(ctx, name)
	if err != nil {
		return nil, fmt.Errorf("discogs artist search: %w", err)
	}
	if searchResp == nil || len(searchResp.Results) == 0 {
		slog.Debug("discogs: no artist results", "name", name)
		return nil, nil
	}

	// Find the first result whose title matches the queried name.
	var best *SearchResult
	for i := range searchResp.Results {
		r := &searchResp.Results[i]
		if r.Type != "artist" {
			continue
		}
		if titleMatch(r.Title, name) {
			best = r
			break
		}
	}
	if best == nil {
		slog.Debug("discogs: artist name mismatch, skipping", "name", name, "first_result", searchResp.Results[0].Title)
		return nil, nil
	}

	// Fetch full artist detail for the image.
	detail, err := c.GetArtist(ctx, best.ID)
	if err != nil {
		slog.Warn("discogs: failed to get artist detail", "id", best.ID, "err", err)
	}

	enrichment := &ArtistEnrichment{
		DiscogsID: best.ID,
		Name:      best.Title,
	}

	if detail != nil {
		enrichment.Profile = detail.Profile
		// Prefer the primary image.
		for _, img := range detail.Images {
			if img.Type == "primary" && img.URI != "" {
				enrichment.ImageURI = img.URI
				break
			}
		}
		// Fall back to the first available image.
		if enrichment.ImageURI == "" && len(detail.Images) > 0 && detail.Images[0].URI != "" {
			enrichment.ImageURI = detail.Images[0].URI
		}
	}

	slog.Info("discogs: enriched artist", "name", name, "id", best.ID)
	return enrichment, nil
}

// EnrichAlbum searches Discogs for a master release by title and artist and
// returns enrichment data. Returns nil (no error) when no confident match is found.
func (c *Client) EnrichAlbum(ctx context.Context, title, artistName string) (*AlbumEnrichment, error) {
	searchResp, err := c.SearchMasters(ctx, title, artistName)
	if err != nil {
		return nil, fmt.Errorf("discogs master search: %w", err)
	}
	if searchResp == nil || len(searchResp.Results) == 0 {
		slog.Debug("discogs: no master results", "title", title, "artist", artistName)
		return nil, nil
	}

	// Find the first result whose title matches.
	// Discogs master search result titles are formatted as "Artist - Title".
	var best *SearchResult
	for i := range searchResp.Results {
		r := &searchResp.Results[i]
		if r.Type != "master" {
			continue
		}
		// Title format is "Artist - Album" in search results.
		albumPart := albumTitleFromResult(r.Title)
		if titleMatch(albumPart, title) {
			best = r
			break
		}
	}
	if best == nil {
		slog.Debug("discogs: album title mismatch, skipping", "title", title, "first_result", searchResp.Results[0].Title)
		return nil, nil
	}

	enrichment := &AlbumEnrichment{
		DiscogsID:  best.ID,
		Year:       best.Year,
		Genres:     best.Genres,
		Styles:     best.Styles,
		CoverImage: best.CoverImage,
	}

	// Fetch full master detail for richer data.
	master, err := c.GetMaster(ctx, best.ID)
	if err != nil {
		slog.Warn("discogs: failed to get master detail", "id", best.ID, "err", err)
	}
	if master != nil {
		enrichment.Title = master.Title
		if master.Year > 0 {
			enrichment.Year = strconv.Itoa(master.Year)
		}
		if len(master.Genres) > 0 {
			enrichment.Genres = master.Genres
		}
		if len(master.Styles) > 0 {
			enrichment.Styles = master.Styles
		}
		// Prefer primary image from master detail over search thumb.
		for _, img := range master.Images {
			if img.Type == "primary" && img.URI != "" {
				enrichment.CoverImage = img.URI
				break
			}
		}
		if enrichment.CoverImage == "" && len(master.Images) > 0 {
			enrichment.CoverImage = master.Images[0].URI
		}
	}

	slog.Info("discogs: enriched album", "title", title, "artist", artistName, "id", best.ID,
		"genres", len(enrichment.Genres), "styles", len(enrichment.Styles))
	return enrichment, nil
}

// albumTitleFromResult extracts the album title from a Discogs search result
// title, which has the format "Artist - Title" or just "Title".
func albumTitleFromResult(s string) string {
	if idx := findSeparator(s); idx >= 0 {
		return s[idx+3:] // " - " is 3 chars
	}
	return s
}

// findSeparator finds the " - " separator in a Discogs title string.
func findSeparator(s string) int {
	for i := 0; i+2 < len(s); i++ {
		if s[i] == ' ' && s[i+1] == '-' && s[i+2] == ' ' {
			return i
		}
	}
	return -1
}
