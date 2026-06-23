package contracts

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"

	catalogm "psychic-homily-backend/internal/models/catalog"
)

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
	ID             string             `json:"id"`                        // External event ID (from the venue's system)
	Title          string             `json:"title"`                     // Event title (typically artist names)
	Date           string             `json:"date"`                      // Event date in ISO format (e.g., "2026-01-25")
	Venue          string             `json:"venue"`                     // Venue name
	VenueSlug      string             `json:"venueSlug"`                 // Venue identifier (e.g., "valley-bar")
	ImageURL       *string            `json:"imageUrl"`                  // Event image URL (optional)
	DoorsTime      *string            `json:"doorsTime"`                 // Doors time (e.g., "6:30 pm")
	ShowTime       *string            `json:"showTime"`                  // Show time (e.g., "7:00 pm")
	TicketURL      *string            `json:"ticketUrl"`                 // Ticket purchase URL (optional)
	Artists        []string           `json:"artists"`                   // List of artists (from event detail page)
	BillingArtists []DiscoveredArtist `json:"billing_artists,omitempty"` // Artists with billing info (from AI extraction)
	ScrapedAt      string             `json:"scrapedAt"`                 // When the event was scraped (ISO timestamp)
	Price          *string            `json:"price"`                     // Price string (e.g., "$18", "Free")
	AgeRestriction *string            `json:"ageRestriction"`            // Age restriction (e.g., "16+", "All Ages")
	IsSoldOut      *bool              `json:"isSoldOut"`                 // Whether the event is sold out
	IsCancelled    *bool              `json:"isCancelled"`               // Whether the event is cancelled
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

// ──────────────────────────────────────────────
// Streaming Worklist types
// ──────────────────────────────────────────────

// StreamingWorklistEntry is one row in the admin streaming-discovery worklist.
// Returned by ListStreamingWorklist; shaped for direct rendering by the
// frontend triage UI.
type StreamingWorklistEntry struct {
	ArtistID                 uint      `json:"artist_id"`
	ArtistName               string    `json:"artist_name"`
	ArtistSlug               *string   `json:"artist_slug,omitempty"`
	StreamingDiscoveryStatus string    `json:"streaming_discovery_status"`
	SoonestEventDate         time.Time `json:"soonest_event_date"`
	VenueName                *string   `json:"venue_name,omitempty"`
	VenueCity                *string   `json:"venue_city,omitempty"`
	UpcomingShowCount        int64     `json:"upcoming_show_count"`
}

// StreamingWorklistResult is the paginated worklist response.
type StreamingWorklistResult struct {
	Entries []StreamingWorklistEntry `json:"entries"`
	Total   int64                    `json:"total"`
}

// UpdateStreamingDiscoveryStatusInput is the service-level input for the
// status mutation. Status is the raw string from the admin request; the
// service validates membership in the legal-transition matrix.
type UpdateStreamingDiscoveryStatusInput struct {
	ArtistID uint
	Status   string
	Reason   *string
}

// StreamingDiscoveryArtistResponse is the updated-artist payload returned
// after a successful status mutation. Mirrors the relevant columns from
// the artists table — no relationships are eagerly loaded.
type StreamingDiscoveryArtistResponse struct {
	ID                       uint      `json:"id"`
	Name                     string    `json:"name"`
	Slug                     *string   `json:"slug,omitempty"`
	StreamingDiscoveryStatus string    `json:"streaming_discovery_status"`
	StreamingDiscoveryReason *string   `json:"streaming_discovery_reason,omitempty"`
	UpdatedAt                time.Time `json:"updated_at"`
}

// ──────────────────────────────────────────────
// Streaming Worklist Service Interface
// ──────────────────────────────────────────────

// StreamingWorklistServiceInterface defines the contract for the admin
// streaming-discovery worklist + status mutation.
type StreamingWorklistServiceInterface interface {
	ListStreamingWorklist(status string, limit, offset int) (*StreamingWorklistResult, error)
	UpdateStreamingDiscoveryStatus(input UpdateStreamingDiscoveryStatusInput) (*StreamingDiscoveryArtistResponse, error)
}

// ErrInvalidStreamingStatusTransition is returned by
// UpdateStreamingDiscoveryStatus when the requested transition is not
// allowed by the matrix. Handler maps this to a 400.
var ErrInvalidStreamingStatusTransition = errors.New("invalid streaming-discovery status transition")

// ErrStreamingArtistNotFound is returned by UpdateStreamingDiscoveryStatus
// when the artist row does not exist. Handler maps this to a 404.
var ErrStreamingArtistNotFound = errors.New("artist not found")

// ──────────────────────────────────────────────
// Extraction Service Interface
// ──────────────────────────────────────────────

// ExtractionServiceInterface defines the contract for AI show extraction
// operations. Used by ShowHandler's AI show-from-text processing. (The
// venue-calendar extraction method was removed with the legacy pipeline, PSY-1165.)
type ExtractionServiceInterface interface {
	ExtractShow(req *ExtractShowRequest) (*ExtractShowResponse, error)
}

// ──────────────────────────────────────────────
// Discovery Service Interface
// ──────────────────────────────────────────────

// DiscoveryServiceInterface defines the contract for venue discovery/import operations.
type DiscoveryServiceInterface interface {
	ImportFromJSON(filepath string, dryRun bool) (*ImportResult, error)
	ImportFromJSONWithDB(filepath string, dryRun bool, database *gorm.DB) (*ImportResult, error)
	CheckEvents(events []CheckEventInput) (*CheckEventsResult, error)
	ImportEvents(events []DiscoveredEvent, dryRun bool, allowUpdates bool, initialStatus catalogm.ShowStatus) (*ImportResult, error)
}

// ──────────────────────────────────────────────
// Enrichment Service Interface
// ──────────────────────────────────────────────

// EnrichmentServiceInterface defines the contract for post-import enrichment operations.
type EnrichmentServiceInterface interface {
	QueueShowForEnrichment(showID uint, enrichmentType string) error
	ProcessQueue(ctx context.Context, batchSize int) (int, error)
	EnrichShow(ctx context.Context, showID uint) (*EnrichmentResult, error)
	GetQueueStats() (*EnrichmentQueueStats, error)
}

// ──────────────────────────────────────────────
// Enrichment Worker Interface
// ──────────────────────────────────────────────

// EnrichmentWorkerInterface defines the contract for the background enrichment worker.
type EnrichmentWorkerInterface interface {
	Start(ctx context.Context)
	Stop()
}
