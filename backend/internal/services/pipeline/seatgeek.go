package pipeline

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sync"
	"time"
)

const (
	sgBaseURL   = "https://api.seatgeek.com/2"
	sgRateLimit = 500 * time.Millisecond // 2 requests per second
)

// SGEventsResponse is the SeatGeek events API response.
type SGEventsResponse struct {
	Events []SGEvent `json:"events"`
	Meta   SGMeta    `json:"meta"`
}

// SGEvent represents a single SeatGeek event.
type SGEvent struct {
	ID          int           `json:"id"`
	Title       string        `json:"title"`
	Type        string        `json:"type"`
	DateTimeUTC string        `json:"datetime_utc"`
	Venue       SGVenue       `json:"venue"`
	Performers  []SGPerformer `json:"performers"`
	Stats       SGStats       `json:"stats"`
	Taxonomies  []SGTaxonomy  `json:"taxonomies"`
}

// SGVenue represents a SeatGeek venue.
type SGVenue struct {
	ID       int    `json:"id"`
	Name     string `json:"name"`
	City     string `json:"city"`
	State    string `json:"state"`
	Country  string `json:"country"`
	Timezone string `json:"timezone"`
}

// SGPerformer represents a SeatGeek performer.
type SGPerformer struct {
	ID     int       `json:"id"`
	Name   string    `json:"name"`
	Type   string    `json:"type"`
	Genres []SGGenre `json:"genres"`
}

// SGGenre represents a SeatGeek genre.
type SGGenre struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
	Slug string `json:"slug"`
}

// SGStats holds SeatGeek event pricing statistics.
type SGStats struct {
	LowestPrice  *float64 `json:"lowest_price"`
	HighestPrice *float64 `json:"highest_price"`
	AveragePrice *float64 `json:"average_price"`
}

// SGTaxonomy represents a SeatGeek taxonomy/category.
type SGTaxonomy struct {
	ID       int    `json:"id"`
	Name     string `json:"name"`
	ParentID *int   `json:"parent_id"`
}

// SGMeta holds pagination metadata.
type SGMeta struct {
	Total   int `json:"total"`
	PerPage int `json:"per_page"`
	Page    int `json:"page"`
}

// SeatGeekLookupResult holds enrichment data from SeatGeek.
type SeatGeekLookupResult struct {
	EventID      int      `json:"event_id"`
	Title        string   `json:"title"`
	LowestPrice  *float64 `json:"lowest_price,omitempty"`
	HighestPrice *float64 `json:"highest_price,omitempty"`
	AveragePrice *float64 `json:"average_price,omitempty"`
	Genres       []string `json:"genres,omitempty"`
	EventType    string   `json:"event_type,omitempty"`
}

// SeatGeekClient provides rate-limited access to the SeatGeek API.
type SeatGeekClient struct {
	client    *http.Client
	clientID  string
	mu        sync.Mutex
	lastReq   time.Time
	rateLimit time.Duration
}

// NewSeatGeekClient creates a new rate-limited SeatGeek API client.
// clientID is the SeatGeek API client_id. If empty, all lookups return nil (skip).
func NewSeatGeekClient(clientID string) *SeatGeekClient {
	return &SeatGeekClient{
		client: &http.Client{
			Timeout: 15 * time.Second,
		},
		clientID:  clientID,
		rateLimit: sgRateLimit,
	}
}

// IsConfigured returns true if the SeatGeek client_id is set.
func (c *SeatGeekClient) IsConfigured() bool {
	return c.clientID != ""
}

// SearchEvent searches SeatGeek for an event by venue name and date.
// Returns the best matching event or nil if not found.
func (c *SeatGeekClient) SearchEvent(venueName string, eventDate time.Time) (*SeatGeekLookupResult, error) {
	if !c.IsConfigured() {
		return nil, nil
	}

	c.throttle()

	dateStr := eventDate.Format("2006-01-02")
	params := url.Values{}
	params.Set("client_id", c.clientID)
	params.Set("venue.name", venueName)
	params.Set("datetime_utc.gte", dateStr+"T00:00:00")
	params.Set("datetime_utc.lte", dateStr+"T23:59:59")
	params.Set("per_page", "5")
	params.Set("type", "concert")

	searchURL := fmt.Sprintf("%s/events?%s", sgBaseURL, params.Encode())

	body, err := c.doRequest(searchURL)
	if err != nil {
		return nil, fmt.Errorf("seatgeek search failed: %w", err)
	}

	var result SGEventsResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse seatgeek response: %w", err)
	}

	if len(result.Events) == 0 {
		return nil, nil
	}

	// Return the first matching event
	event := result.Events[0]
	lookupResult := &SeatGeekLookupResult{
		EventID:      event.ID,
		Title:        event.Title,
		LowestPrice:  event.Stats.LowestPrice,
		HighestPrice: event.Stats.HighestPrice,
		AveragePrice: event.Stats.AveragePrice,
		EventType:    event.Type,
	}

	// Extract genres from performers
	genreSet := make(map[string]bool)
	for _, performer := range event.Performers {
		for _, genre := range performer.Genres {
			genreSet[genre.Name] = true
		}
	}
	for genre := range genreSet {
		lookupResult.Genres = append(lookupResult.Genres, genre)
	}

	return lookupResult, nil
}

// throttle enforces the rate limit.
func (c *SeatGeekClient) throttle() {
	c.mu.Lock()
	defer c.mu.Unlock()

	elapsed := time.Since(c.lastReq)
	if elapsed < c.rateLimit {
		time.Sleep(c.rateLimit - elapsed)
	}
	c.lastReq = time.Now()
}

// doRequest performs an HTTP GET.
func (c *SeatGeekClient) doRequest(reqURL string) ([]byte, error) {
	req, err := http.NewRequest("GET", reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusTooManyRequests {
		return nil, fmt.Errorf("rate limited (HTTP 429)")
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	return body, nil
}
