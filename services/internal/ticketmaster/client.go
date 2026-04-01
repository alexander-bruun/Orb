// Package ticketmaster provides a client for the Ticketmaster Discovery API.
// See https://developer.ticketmaster.com/products-and-docs/apis/discovery-api/v2/
// A free API key can be obtained at https://developer-acct.ticketmaster.com/
package ticketmaster

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

const baseURL = "https://app.ticketmaster.com/discovery/v2"

// Client is a Ticketmaster Discovery API client.
type Client struct {
	http   *http.Client
	apiKey string
}

// New creates a new Ticketmaster client.
func New(apiKey string) *Client {
	return &Client{
		http:   &http.Client{Timeout: 10 * time.Second},
		apiKey: apiKey,
	}
}

// Venue holds location info for an event.
type Venue struct {
	Name    string `json:"name"`
	City    string `json:"city"`
	Region  string `json:"region"`
	Country string `json:"country"`
}

// Offer represents a ticketing link for an event.
type Offer struct {
	Type   string `json:"type"`
	URL    string `json:"url"`
	Status string `json:"status"`
}

// Event is a normalized upcoming concert event.
type Event struct {
	ID       string   `json:"id"`
	URL      string   `json:"url"`
	Datetime string   `json:"datetime"`
	Title    string   `json:"title"`
	Venue    Venue    `json:"venue"`
	Offers   []Offer  `json:"offers"`
	Lineup   []string `json:"lineup"`
}

// --- internal response types ---

type tmAttractionsResp struct {
	Embedded *struct {
		Attractions []struct {
			ID string `json:"id"`
		} `json:"attractions"`
	} `json:"_embedded"`
}

type tmEventsResp struct {
	Embedded *struct {
		Events []tmEvent `json:"events"`
	} `json:"_embedded"`
}

type tmEvent struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	URL  string `json:"url"`
	Dates struct {
		Start struct {
			LocalDate string `json:"localDate"`
			LocalTime string `json:"localTime"`
		} `json:"start"`
	} `json:"dates"`
	Embedded *struct {
		Venues []struct {
			Name    string         `json:"name"`
			City    map[string]any `json:"city"`
			State   map[string]any `json:"state"`
			Country map[string]any `json:"country"`
		} `json:"venues"`
		Attractions []struct {
			Name string `json:"name"`
		} `json:"attractions"`
	} `json:"_embedded"`
}

func (c *Client) get(ctx context.Context, path string, params url.Values) ([]byte, error) {
	params.Set("apikey", c.apiKey)
	u := baseURL + path + "?" + params.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _, _ = io.Copy(io.Discard, resp.Body); resp.Body.Close() }()

	if resp.StatusCode == 404 {
		return nil, nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("ticketmaster: http %d: %s", resp.StatusCode, body)
	}
	return io.ReadAll(resp.Body)
}

// GetArtistEvents does a two-step lookup: find the attraction ID for the
// artist name, then fetch upcoming events for that specific attraction.
func (c *Client) GetArtistEvents(ctx context.Context, artistName string) ([]Event, error) {
	// Step 1: resolve artist name → Ticketmaster attraction ID.
	attrBody, err := c.get(ctx, "/attractions.json", url.Values{
		"keyword":            {artistName},
		"classificationName": {"music"},
		"size":               {"1"},
	})
	if err != nil {
		return nil, fmt.Errorf("ticketmaster: attraction search: %w", err)
	}
	if attrBody == nil {
		return []Event{}, nil
	}

	var attrResp tmAttractionsResp
	if err := json.Unmarshal(attrBody, &attrResp); err != nil {
		return nil, fmt.Errorf("ticketmaster: parse attractions: %w", err)
	}
	if attrResp.Embedded == nil || len(attrResp.Embedded.Attractions) == 0 {
		return []Event{}, nil
	}
	attractionID := attrResp.Embedded.Attractions[0].ID

	// Step 2: fetch upcoming events for that attraction.
	evBody, err := c.get(ctx, "/events.json", url.Values{
		"attractionId": {attractionID},
		"sort":         {"date,asc"},
		"size":         {"20"},
	})
	if err != nil {
		return nil, fmt.Errorf("ticketmaster: events fetch: %w", err)
	}
	if evBody == nil {
		return []Event{}, nil
	}

	var evResp tmEventsResp
	if err := json.Unmarshal(evBody, &evResp); err != nil {
		return nil, fmt.Errorf("ticketmaster: parse events: %w", err)
	}
	if evResp.Embedded == nil {
		return []Event{}, nil
	}

	events := make([]Event, 0, len(evResp.Embedded.Events))
	for _, e := range evResp.Embedded.Events {
		ev := Event{
			ID:    e.ID,
			URL:   e.URL,
			Title: e.Name,
			Offers: []Offer{{
				Type:   "Tickets",
				URL:    e.URL,
				Status: "available",
			}},
		}

		dt := e.Dates.Start.LocalDate
		if t := e.Dates.Start.LocalTime; t != "" {
			dt += "T" + t
		}
		ev.Datetime = dt

		if e.Embedded != nil && len(e.Embedded.Venues) > 0 {
			v := e.Embedded.Venues[0]
			ev.Venue = Venue{
				Name:    v.Name,
				City:    strField(v.City, "name"),
				Region:  strField(v.State, "stateCode"),
				Country: strField(v.Country, "countryCode"),
			}
		}

		if e.Embedded != nil {
			for _, a := range e.Embedded.Attractions {
				ev.Lineup = append(ev.Lineup, a.Name)
			}
		}

		events = append(events, ev)
	}
	return events, nil
}

func strField(m map[string]any, key string) string {
	if m == nil {
		return ""
	}
	v, _ := m[key].(string)
	return v
}
