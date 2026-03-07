package musicbrainz

import (
	"context"
	"crypto/md5"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// FetchArtistImage attempts to download an artist photo using Wikidata.
// It extracts the Wikidata QID from the artist's URL relationships, queries
// the Wikidata API for the P18 (image) property, and downloads the image
// from Wikimedia Commons. Returns nil if no image is available.
func (c *Client) FetchArtistImage(ctx context.Context, relations []MBRelation) ([]byte, error) {
	// Find the Wikidata URL in the artist's relationships.
	var qid string
	for _, rel := range relations {
		if rel.URL == nil {
			continue
		}
		resource := rel.URL.Resource
		// Match https://www.wikidata.org/wiki/Q12345
		if strings.Contains(resource, "wikidata.org/wiki/Q") {
			parts := strings.Split(resource, "/")
			for _, p := range parts {
				if strings.HasPrefix(p, "Q") {
					qid = p
					break
				}
			}
		}
		if qid != "" {
			break
		}
	}
	if qid == "" {
		return nil, nil
	}

	// Query Wikidata for the image filename (P18 property).
	filename, err := wikidataImageFilename(ctx, qid)
	if err != nil || filename == "" {
		return nil, err
	}

	// Download the image from Wikimedia Commons.
	return downloadCommonsImage(ctx, filename)
}

// wikidataImageFilename queries the Wikidata API for the P18 (image) claim.
func wikidataImageFilename(ctx context.Context, qid string) (string, error) {
	u := fmt.Sprintf("https://www.wikidata.org/w/api.php?action=wbgetclaims&property=P18&entity=%s&format=json", qid)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", userAgent)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return "", nil
	}

	var result struct {
		Claims map[string][]struct {
			MainSnak struct {
				DataValue struct {
					Value string `json:"value"`
				} `json:"datavalue"`
			} `json:"mainsnak"`
		} `json:"claims"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}

	if claims, ok := result.Claims["P18"]; ok && len(claims) > 0 {
		return claims[0].MainSnak.DataValue.Value, nil
	}
	return "", nil
}

// FetchAlbumCoverArt downloads the front cover image from the Cover Art Archive
// for the given release group MBID. Returns nil if no cover art is available.
func (c *Client) FetchAlbumCoverArt(ctx context.Context, releaseGroupMbid string) ([]byte, error) {
	if releaseGroupMbid == "" {
		return nil, nil
	}

	u := fmt.Sprintf("https://coverartarchive.org/release-group/%s/front-500", releaseGroupMbid)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == 404 {
		return nil, nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("cover art archive: http %d for %s", resp.StatusCode, releaseGroupMbid)
	}

	return io.ReadAll(resp.Body)
}

// downloadCommonsImage downloads an image from Wikimedia Commons.
func downloadCommonsImage(ctx context.Context, filename string) ([]byte, error) {
	// Wikimedia Commons URL structure uses MD5 hash of the filename.
	filename = strings.ReplaceAll(filename, " ", "_")
	hash := fmt.Sprintf("%x", md5.Sum([]byte(filename)))
	u := fmt.Sprintf("https://upload.wikimedia.org/wikipedia/commons/thumb/%s/%s/%s/400px-%s",
		hash[:1], hash[:2], filename, filename)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		// Try without thumbnail (some file types don't support thumbnailing).
		u = fmt.Sprintf("https://upload.wikimedia.org/wikipedia/commons/%s/%s/%s",
			hash[:1], hash[:2], filename)
		req2, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		if err != nil {
			return nil, err
		}
		req2.Header.Set("User-Agent", userAgent)
		resp2, err := client.Do(req2)
		if err != nil {
			return nil, err
		}
		defer resp2.Body.Close()
		if resp2.StatusCode != 200 {
			return nil, nil
		}
		return io.ReadAll(resp2.Body)
	}

	return io.ReadAll(resp.Body)
}
