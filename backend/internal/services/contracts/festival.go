package contracts

import (
	"encoding/json"
	"time"
)

// ──────────────────────────────────────────────
// Festival types
// ──────────────────────────────────────────────

// CreateFestivalRequest represents the data needed to create a new festival
type CreateFestivalRequest struct {
	Name         string           `json:"name" validate:"required"`
	SeriesSlug   string           `json:"series_slug" validate:"required"`
	EditionYear  int              `json:"edition_year" validate:"required"`
	Description  *string          `json:"description"`
	LocationName *string          `json:"location_name"`
	City         *string          `json:"city"`
	State        *string          `json:"state"`
	Country      *string          `json:"country"`
	StartDate    string           `json:"start_date" validate:"required"`
	EndDate      string           `json:"end_date" validate:"required"`
	Website      *string          `json:"website"`
	TicketURL    *string          `json:"ticket_url"`
	FlyerURL     *string          `json:"flyer_url"`
	Status       string           `json:"status"`
	Social       *json.RawMessage `json:"social"`
}

// UpdateFestivalRequest represents the data that can be updated on a festival
type UpdateFestivalRequest struct {
	Name         *string          `json:"name"`
	SeriesSlug   *string          `json:"series_slug"`
	EditionYear  *int             `json:"edition_year"`
	Description  *string          `json:"description"`
	LocationName *string          `json:"location_name"`
	City         *string          `json:"city"`
	State        *string          `json:"state"`
	Country      *string          `json:"country"`
	StartDate    *string          `json:"start_date"`
	EndDate      *string          `json:"end_date"`
	Website      *string          `json:"website"`
	TicketURL    *string          `json:"ticket_url"`
	FlyerURL     *string          `json:"flyer_url"`
	Status       *string          `json:"status"`
	Social       *json.RawMessage `json:"social"`
}

// FestivalDetailResponse represents the festival data returned to clients
type FestivalDetailResponse struct {
	ID           uint             `json:"id"`
	Name         string           `json:"name"`
	Slug         string           `json:"slug"`
	SeriesSlug   string           `json:"series_slug"`
	EditionYear  int              `json:"edition_year"`
	Description  *string          `json:"description"`
	LocationName *string          `json:"location_name"`
	City         *string          `json:"city"`
	State        *string          `json:"state"`
	Country      *string          `json:"country"`
	StartDate    string           `json:"start_date"`
	EndDate      string           `json:"end_date"`
	Website      *string          `json:"website"`
	TicketURL    *string          `json:"ticket_url"`
	FlyerURL     *string          `json:"flyer_url"`
	Status       string           `json:"status"`
	Social       *json.RawMessage `json:"social"`
	ArtistCount  int              `json:"artist_count"`
	VenueCount   int              `json:"venue_count"`
	CreatedAt    time.Time        `json:"created_at"`
	UpdatedAt    time.Time        `json:"updated_at"`
}

// FestivalListResponse represents a festival in list views
type FestivalListResponse struct {
	ID          uint    `json:"id"`
	Name        string  `json:"name"`
	Slug        string  `json:"slug"`
	SeriesSlug  string  `json:"series_slug"`
	EditionYear int     `json:"edition_year"`
	City        *string `json:"city"`
	State       *string `json:"state"`
	StartDate   string  `json:"start_date"`
	EndDate     string  `json:"end_date"`
	Status      string  `json:"status"`
	ArtistCount int     `json:"artist_count"`
	VenueCount  int     `json:"venue_count"`
}

// FestivalArtistResponse represents an artist on a festival lineup
type FestivalArtistResponse struct {
	ID          uint    `json:"id"`
	ArtistID    uint    `json:"artist_id"`
	ArtistSlug  string  `json:"artist_slug"`
	ArtistName  string  `json:"artist_name"`
	BillingTier string  `json:"billing_tier"`
	Position    int     `json:"position"`
	DayDate     *string `json:"day_date"`
	Stage       *string `json:"stage"`
	SetTime     *string `json:"set_time"`
	VenueID     *uint   `json:"venue_id"`
}

// FestivalVenueResponse represents a venue at a festival
type FestivalVenueResponse struct {
	ID        uint   `json:"id"`
	VenueID   uint   `json:"venue_id"`
	VenueName string `json:"venue_name"`
	VenueSlug string `json:"venue_slug"`
	City      string `json:"city"`
	State     string `json:"state"`
	IsPrimary bool   `json:"is_primary"`
}

// ArtistFestivalListResponse represents a festival in an artist's festival history
type ArtistFestivalListResponse struct {
	FestivalListResponse
	BillingTier string  `json:"billing_tier"`
	DayDate     *string `json:"day_date"`
	Stage       *string `json:"stage"`
}

// AddFestivalArtistRequest represents the data needed to add an artist to a festival
type AddFestivalArtistRequest struct {
	ArtistID    uint    `json:"artist_id" validate:"required"`
	BillingTier string  `json:"billing_tier"`
	Position    int     `json:"position"`
	DayDate     *string `json:"day_date"`
	Stage       *string `json:"stage"`
	SetTime     *string `json:"set_time"`
	VenueID     *uint   `json:"venue_id"`
}

// UpdateFestivalArtistRequest represents the data that can be updated on a festival artist
type UpdateFestivalArtistRequest struct {
	BillingTier *string `json:"billing_tier"`
	Position    *int    `json:"position"`
	DayDate     *string `json:"day_date"`
	Stage       *string `json:"stage"`
	SetTime     *string `json:"set_time"`
	VenueID     *uint   `json:"venue_id"`
}

// AddFestivalVenueRequest represents the data needed to add a venue to a festival
type AddFestivalVenueRequest struct {
	VenueID   uint `json:"venue_id" validate:"required"`
	IsPrimary bool `json:"is_primary"`
}
