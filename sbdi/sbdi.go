// Package sbdi is the library behind the sbdi command line:
// the HTTP client, request shaping, and the typed data models for the
// Swedish Biodiversity Data Infrastructure (SBDI).
//
// The Client here is the spine every command shares. It sets a real
// User-Agent, paces requests so a busy session stays polite, and retries the
// transient failures (429 and 5xx) that any public API throws under load.
// Build your endpoint calls and JSON decoding on top of it.
package sbdi

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// Host is the site this client talks to, and the host the URI driver in
// domain.go claims.
const Host = "records.biodiversitydata.se"

// BaseURL is the root every request is built from.
const BaseURL = "https://records.biodiversitydata.se/ws"

// Config holds all tunable parameters for a Client.
type Config struct {
	BaseURL   string
	Rate      time.Duration
	Retries   int
	Timeout   time.Duration
	UserAgent string
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() Config {
	return Config{
		BaseURL:   BaseURL,
		Rate:      500 * time.Millisecond,
		Retries:   3,
		Timeout:   30 * time.Second,
		UserAgent: "sbdi-cli/0.1.0 (github.com/tamnd/sbdi-cli)",
	}
}

// Wire types — match the SBDI JSON shapes exactly.

type wireOccurrence struct {
	UUID            string  `json:"uuid"`
	OccurrenceID    string  `json:"occurrenceID"`
	ScientificName  string  `json:"scientificName"`
	VernacularName  string  `json:"vernacularName"`
	TaxonRank       string  `json:"taxonRank"`
	Kingdom         string  `json:"kingdom"`
	Family          string  `json:"family"`
	Genus           string  `json:"genus"`
	EventDate       string  `json:"eventDate"`
	Year            int     `json:"year"`
	BasisOfRecord   string  `json:"basisOfRecord"`
	DataResource    string  `json:"dataResourceName"`
	InstitutionName string  `json:"institutionName"`
	Country         string  `json:"country"`
	StateProvince   string  `json:"stateProvince"`
	Latitude        float64 `json:"decimalLatitude"`
	Longitude       float64 `json:"decimalLongitude"`
	License         string  `json:"license"`
	Collector       string  `json:"collector"`
}

type wireSearchResp struct {
	TotalRecords int              `json:"totalRecords"`
	StartIndex   int              `json:"startIndex"`
	PageSize     int              `json:"pageSize"`
	Occurrences  []wireOccurrence `json:"occurrences"`
}

// Occurrence is the public record type: one biodiversity observation from SBDI.
type Occurrence struct {
	ID             string  `json:"id"                       kit:"id"` // = uuid
	OccurrenceID   string  `json:"occurrence_id,omitempty"`
	ScientificName string  `json:"scientific_name,omitempty"`
	VernacularName string  `json:"vernacular_name,omitempty"`
	TaxonRank      string  `json:"taxon_rank,omitempty"`
	Kingdom        string  `json:"kingdom,omitempty"`
	Family         string  `json:"family,omitempty"`
	EventDate      string  `json:"event_date,omitempty"`
	Year           int     `json:"year,omitempty"`
	BasisOfRecord  string  `json:"basis_of_record,omitempty"`
	DataResource   string  `json:"data_resource,omitempty"`
	Institution    string  `json:"institution,omitempty"`
	Country        string  `json:"country,omitempty"`
	StateProvince  string  `json:"state_province,omitempty"`
	Latitude       float64 `json:"latitude,omitempty"`
	Longitude      float64 `json:"longitude,omitempty"`
	License        string  `json:"license,omitempty"`
	Collector      string  `json:"collector,omitempty"`
}

func occurrenceFromWire(w wireOccurrence) *Occurrence {
	return &Occurrence{
		ID:             w.UUID,
		OccurrenceID:   w.OccurrenceID,
		ScientificName: w.ScientificName,
		VernacularName: w.VernacularName,
		TaxonRank:      w.TaxonRank,
		Kingdom:        w.Kingdom,
		Family:         w.Family,
		EventDate:      w.EventDate,
		Year:           w.Year,
		BasisOfRecord:  w.BasisOfRecord,
		DataResource:   w.DataResource,
		Institution:    w.InstitutionName,
		Country:        w.Country,
		StateProvince:  w.StateProvince,
		Latitude:       w.Latitude,
		Longitude:      w.Longitude,
		License:        w.License,
		Collector:      w.Collector,
	}
}

// Client talks to SBDI over HTTP.
type Client struct {
	cfg  Config
	http *http.Client
	last time.Time
}

// NewClient returns a Client with sensible defaults.
func NewClient() *Client {
	cfg := DefaultConfig()
	return &Client{
		cfg:  cfg,
		http: &http.Client{Timeout: cfg.Timeout},
	}
}

// NewClientAt returns a Client pointed at baseURL instead of the live API.
// Intended for tests.
func NewClientAt(baseURL string) *Client {
	cfg := DefaultConfig()
	cfg.BaseURL = baseURL
	cfg.Rate = 0
	return &Client{
		cfg:  cfg,
		http: &http.Client{Timeout: cfg.Timeout},
	}
}

// SearchOccurrences queries /occurrences/search and returns matching occurrences
// plus the total count.
func (c *Client) SearchOccurrences(ctx context.Context, query string, limit, offset int) ([]*Occurrence, int, error) {
	return c.SearchAt(ctx, c.cfg.BaseURL, query, limit, offset)
}

// SearchAt is like SearchOccurrences but uses baseURL instead of the configured one.
// Intended for tests.
func (c *Client) SearchAt(ctx context.Context, baseURL, query string, limit, offset int) ([]*Occurrence, int, error) {
	if limit <= 0 {
		limit = 20
	}
	u := baseURL + "/occurrences/search?q=" + url.QueryEscape(query) +
		"&pageSize=" + strconv.Itoa(limit) +
		"&startIndex=" + strconv.Itoa(offset)

	body, err := c.get(ctx, u)
	if err != nil {
		return nil, 0, err
	}
	return parseSearchResp(body)
}

// get fetches a URL and returns the response body, with pacing and retries.
func (c *Client) get(ctx context.Context, rawURL string) ([]byte, error) {
	var lastErr error
	for attempt := 0; attempt <= c.cfg.Retries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(backoff(attempt)):
			}
		}
		body, retry, err := c.do(ctx, rawURL)
		if err == nil {
			return body, nil
		}
		lastErr = err
		if !retry {
			return nil, err
		}
	}
	return nil, fmt.Errorf("get %s: %w", rawURL, lastErr)
}

func (c *Client) do(ctx context.Context, rawURL string) (body []byte, retry bool, err error) {
	c.pace()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, false, err
	}
	req.Header.Set("User-Agent", c.cfg.UserAgent)
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, true, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
		return nil, true, fmt.Errorf("http %d", resp.StatusCode)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, false, fmt.Errorf("http %d", resp.StatusCode)
	}

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, true, err
	}
	return b, false, nil
}

// pace blocks until at least Rate has passed since the previous request.
func (c *Client) pace() {
	if c.cfg.Rate <= 0 {
		return
	}
	if wait := c.cfg.Rate - time.Since(c.last); wait > 0 {
		time.Sleep(wait)
	}
	c.last = time.Now()
}

func backoff(attempt int) time.Duration {
	d := time.Duration(attempt) * 500 * time.Millisecond
	if d > 5*time.Second {
		d = 5 * time.Second
	}
	return d
}

// parseSearchResp decodes a raw /occurrences/search JSON response body into occurrences + total.
func parseSearchResp(body []byte) ([]*Occurrence, int, error) {
	var resp wireSearchResp
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, 0, fmt.Errorf("decode search response: %w", err)
	}
	out := make([]*Occurrence, 0, len(resp.Occurrences))
	for _, w := range resp.Occurrences {
		out = append(out, occurrenceFromWire(w))
	}
	return out, resp.TotalRecords, nil
}
