package contracts

import "errors"

// ──────────────────────────────────────────────
// Extraction types
// ──────────────────────────────────────────────

// ExtractShowRequest represents the extraction request
type ExtractShowRequest struct {
	Type      string `json:"type"`       // "text", "image", or "both"
	Text      string `json:"text"`       // Text content
	ImageData string `json:"image_data"` // Base64-encoded image
	MediaType string `json:"media_type"` // MIME type of image
}

// MatchSuggestion represents a close-but-not-exact match from the database
type MatchSuggestion struct {
	ID   uint   `json:"id"`
	Name string `json:"name"`
	Slug string `json:"slug"`
}

// VenueMatchSuggestion includes location info
type VenueMatchSuggestion struct {
	ID    uint   `json:"id"`
	Name  string `json:"name"`
	Slug  string `json:"slug"`
	City  string `json:"city"`
	State string `json:"state"`
}

// ExtractedArtist represents an extracted artist with optional DB match
type ExtractedArtist struct {
	Name            string            `json:"name"`
	IsHeadliner     bool              `json:"is_headliner"`
	SetType         string            `json:"set_type,omitempty"`
	BillingOrder    int               `json:"billing_order,omitempty"`
	InstagramHandle string            `json:"instagram_handle,omitempty"`
	MatchedID       *uint             `json:"matched_id,omitempty"`
	MatchedName     *string           `json:"matched_name,omitempty"`
	MatchedSlug     *string           `json:"matched_slug,omitempty"`
	Suggestions     []MatchSuggestion `json:"suggestions,omitempty"`
}

// ExtractedVenue represents an extracted venue with optional DB match
type ExtractedVenue struct {
	Name        string                 `json:"name"`
	City        string                 `json:"city,omitempty"`
	State       string                 `json:"state,omitempty"`
	MatchedID   *uint                  `json:"matched_id,omitempty"`
	MatchedName *string                `json:"matched_name,omitempty"`
	MatchedSlug *string                `json:"matched_slug,omitempty"`
	Suggestions []VenueMatchSuggestion `json:"suggestions,omitempty"`
}

// ExtractedShowData is the full extraction result
type ExtractedShowData struct {
	Artists     []ExtractedArtist `json:"artists"`
	Venue       *ExtractedVenue   `json:"venue,omitempty"`
	Date        string            `json:"date,omitempty"`
	Time        string            `json:"time,omitempty"`
	Cost        string            `json:"cost,omitempty"`
	Ages        string            `json:"ages,omitempty"`
	Description string            `json:"description,omitempty"`
}

// ExtractShowResponse is the API response wrapper
type ExtractShowResponse struct {
	Success  bool               `json:"success"`
	Data     *ExtractedShowData `json:"data,omitempty"`
	Error    string             `json:"error,omitempty"`
	Warnings []string           `json:"warnings,omitempty"`
}

// ──────────────────────────────────────────────
// Calendar Extraction types
// ──────────────────────────────────────────────

// CalendarEvent represents a single event extracted from a venue calendar page.
type CalendarEvent struct {
	Date         string           `json:"date"`
	Time         *string          `json:"time,omitempty"`
	Title        string           `json:"title"`
	Artists      []CalendarArtist `json:"artists"`
	Cost         *string          `json:"cost,omitempty"`
	Ages         *string          `json:"ages,omitempty"`
	TicketURL    *string          `json:"ticket_url,omitempty"`
	IsMusicEvent *bool            `json:"is_music_event,omitempty"`
}

// CalendarArtist represents an artist entry within a calendar event.
type CalendarArtist struct {
	Name         string `json:"name"`
	IsHeadliner  bool   `json:"is_headliner"`
	SetType      string `json:"set_type,omitempty"`
	BillingOrder int    `json:"billing_order,omitempty"`
}

// CalendarExtractionResponse is the response from calendar page extraction.
type CalendarExtractionResponse struct {
	Success  bool            `json:"success"`
	Events   []CalendarEvent `json:"events,omitempty"`
	Error    string          `json:"error,omitempty"`
	Warnings []string        `json:"warnings,omitempty"`
}

// ──────────────────────────────────────────────
// Fetcher types
// ──────────────────────────────────────────────

// FetchResult contains the result of an HTTP fetch with change detection.
type FetchResult struct {
	Changed     bool   // Whether content changed since last fetch
	Body        string // HTML content (empty if unchanged)
	ContentHash string // SHA256 hex of body
	ETag        string // ETag from response header
	HTTPStatus  int    // HTTP status code
	RedirectURL string // New URL if 301/308 redirect
	ContentType string // Content-Type header value
}

// FetchError wraps HTTP status errors for callers that need to inspect the code.
type FetchError struct {
	StatusCode int
	URL        string
	Err        error
}

func (e *FetchError) Error() string {
	return e.Err.Error()
}

func (e *FetchError) Unwrap() error {
	return e.Err
}

// IsFetchError checks if an error is a FetchError and returns it.
func IsFetchError(err error) (*FetchError, bool) {
	var fe *FetchError
	if errors.As(err, &fe) {
		return fe, true
	}
	return nil, false
}

// RenderMethod constants for the three rendering tiers.
const (
	RenderMethodStatic     = "static"
	RenderMethodDynamic    = "dynamic"
	RenderMethodScreenshot = "screenshot"
)

// ──────────────────────────────────────────────
// Pipeline types
// ──────────────────────────────────────────────

// PipelineResult contains the outcome of a single venue extraction run.
type PipelineResult struct {
	VenueID              uint     `json:"venue_id"`
	VenueName            string   `json:"venue_name"`
	RenderMethod         string   `json:"render_method"`
	EventsExtracted      int      `json:"events_extracted"`
	EventsImported       int      `json:"events_imported"`
	EventsSkippedNonMusic int     `json:"events_skipped_non_music"`
	DurationMs           int64    `json:"duration_ms"`
	Skipped              bool     `json:"skipped"`
	SkipReason           string   `json:"skip_reason,omitempty"`
	Error                string   `json:"error,omitempty"`
	Warnings             []string `json:"warnings,omitempty"`
	DryRun               bool     `json:"dry_run"`
	InitialStatus        string   `json:"initial_status"`
}

// VenueRejectionStats contains rejection breakdown and approval rate for a venue's pipeline shows.
type VenueRejectionStats struct {
	TotalExtracted       int64            `json:"total_extracted"`
	Approved             int64            `json:"approved"`
	Rejected             int64            `json:"rejected"`
	Pending              int64            `json:"pending"`
	RejectionBreakdown   map[string]int64 `json:"rejection_breakdown"`
	ApprovalRate         float64          `json:"approval_rate"`
	SuggestedAutoApprove bool             `json:"suggested_auto_approve"`
}

// ──────────────────────────────────────────────
// Enrichment types
// ──────────────────────────────────────────────

// EnrichmentResult holds the combined results of all enrichment steps for a show.
type EnrichmentResult struct {
	ShowID         uint                    `json:"show_id"`
	ArtistMatches  []ArtistMatchEnrichment `json:"artist_matches,omitempty"`
	MusicBrainz    []MBEnrichment          `json:"musicbrainz,omitempty"`
	SeatGeek       *SeatGeekEnrichment     `json:"seatgeek,omitempty"`
	CompletedSteps []string                `json:"completed_steps"`
	Errors         []string                `json:"errors,omitempty"`
}

// ArtistMatchEnrichment holds the result of fuzzy matching for one artist.
type ArtistMatchEnrichment struct {
	ArtistName  string  `json:"artist_name"`
	MatchedID   *uint   `json:"matched_id,omitempty"`
	MatchedName *string `json:"matched_name,omitempty"`
	Confidence  float64 `json:"confidence"`
	AutoLinked  bool    `json:"auto_linked"`
}

// MBEnrichment holds MusicBrainz lookup results for one artist.
type MBEnrichment struct {
	ArtistName     string `json:"artist_name"`
	ArtistID       uint   `json:"artist_id"`
	MBID           string `json:"mbid,omitempty"`
	MBName         string `json:"mb_name,omitempty"`
	Score          int    `json:"score,omitempty"`
	Found          bool   `json:"found"`
	AlreadyHadMBID bool   `json:"already_had_mbid"`
}

// SeatGeekEnrichment holds SeatGeek cross-reference results.
type SeatGeekEnrichment struct {
	Found        bool     `json:"found"`
	EventID      int      `json:"event_id,omitempty"`
	LowestPrice  *float64 `json:"lowest_price,omitempty"`
	HighestPrice *float64 `json:"highest_price,omitempty"`
	AveragePrice *float64 `json:"average_price,omitempty"`
	Genres       []string `json:"genres,omitempty"`
	EventType    string   `json:"event_type,omitempty"`
}

// EnrichmentQueueStats holds summary statistics about the enrichment queue.
type EnrichmentQueueStats struct {
	Pending        int64 `json:"pending"`
	Processing     int64 `json:"processing"`
	CompletedToday int64 `json:"completed_today"`
	FailedToday    int64 `json:"failed_today"`
}

// ──────────────────────────────────────────────
// Discovery types
// ──────────────────────────────────────────────

// DiscoveredArtist represents an artist with billing information from AI extraction.
type DiscoveredArtist struct {
	Name         string `json:"name"`
	SetType      string `json:"set_type,omitempty"`
	BillingOrder int    `json:"billing_order,omitempty"`
}

// DiscoveredEvent represents an event from the Node.js discovery app JSON output
type DiscoveredEvent struct {
	ID         string   `json:"id"`         // External event ID (from the venue's system)
	Title      string   `json:"title"`      // Event title (typically artist names)
	Date       string   `json:"date"`       // Event date in ISO format (e.g., "2026-01-25")
	Venue      string   `json:"venue"`      // Venue name
	VenueSlug  string   `json:"venueSlug"`  // Venue identifier (e.g., "valley-bar")
	ImageURL   *string  `json:"imageUrl"`   // Event image URL (optional)
	DoorsTime  *string  `json:"doorsTime"`  // Doors time (e.g., "6:30 pm")
	ShowTime   *string  `json:"showTime"`   // Show time (e.g., "7:00 pm")
	TicketURL  *string  `json:"ticketUrl"`  // Ticket purchase URL (optional)
	Artists        []string           `json:"artists"`        // List of artists (from event detail page)
	BillingArtists []DiscoveredArtist `json:"billing_artists,omitempty"` // Artists with billing info (from AI extraction)
	ScrapedAt      string   `json:"scrapedAt"`      // When the event was scraped (ISO timestamp)
	Price          *string  `json:"price"`          // Price string (e.g., "$18", "Free")
	AgeRestriction *string  `json:"ageRestriction"` // Age restriction (e.g., "16+", "All Ages")
	IsSoldOut      *bool    `json:"isSoldOut"`      // Whether the event is sold out
	IsCancelled    *bool    `json:"isCancelled"`    // Whether the event is cancelled
}

// ImportResult contains statistics about the import operation
type ImportResult struct {
	Total         int      `json:"total"`          // Total events processed
	Imported      int      `json:"imported"`       // Successfully imported
	Duplicates    int      `json:"duplicates"`     // Skipped due to deduplication
	Rejected      int      `json:"rejected"`       // Skipped due to matching rejected shows
	PendingReview int      `json:"pending_review"` // Flagged as potential duplicates for admin review
	Updated       int      `json:"updated"`        // Updated existing shows with new data
	Errors        int      `json:"errors"`         // Failed to import
	Messages      []string `json:"messages"`       // Detailed messages for each event
}

// CheckEventInput represents the input for checking whether an event exists
type CheckEventInput struct {
	ID        string `json:"id"`
	VenueSlug string `json:"venueSlug"`
	Date      string `json:"date"` // YYYY-MM-DD, used for venue+date fallback match
}

// ShowCurrentData contains the current stored data for a show, used for diff comparison
type ShowCurrentData struct {
	Price          *float64 `json:"price,omitempty"`
	AgeRequirement *string  `json:"ageRequirement,omitempty"`
	Description    *string  `json:"description,omitempty"`
	EventDate      string   `json:"eventDate,omitempty"`
	IsSoldOut      bool     `json:"isSoldOut"`
	IsCancelled    bool     `json:"isCancelled"`
	Artists        []string `json:"artists,omitempty"`
}

// CheckEventStatus represents the import status of a single event
type CheckEventStatus struct {
	Exists      bool             `json:"exists"`
	ShowID      uint             `json:"showId,omitempty"`
	Status      string           `json:"status,omitempty"`
	CurrentData *ShowCurrentData `json:"currentData,omitempty"`
}

// CheckEventsResult contains the import status of multiple events
type CheckEventsResult struct {
	Events map[string]CheckEventStatus `json:"events"`
}
