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

// ──────────────────────────────────────────────
// Festival graph (PSY-1080) — co-bill subgraph of a single festival's lineup
// ──────────────────────────────────────────────

// FestivalGraphResponse is the payload for GET /festivals/{festival_id}/graph.
// Mirrors SceneGraphResponse field-for-field (`scene` → `festival`) so the
// frontend ForceGraphView renders either payload unchanged.
type FestivalGraphResponse struct {
	Festival FestivalGraphInfo      `json:"festival"`
	Clusters []FestivalGraphCluster `json:"clusters"`
	Nodes    []FestivalGraphNode    `json:"nodes"`
	Links    []FestivalGraphLink    `json:"links"`
}

// FestivalGraphInfo holds festival metadata for the graph response. Fields
// mirror SceneGraphInfo (slug + counts) plus festival-specific identifiers.
type FestivalGraphInfo struct {
	ID          uint   `json:"id"`
	Slug        string `json:"slug"`
	Name        string `json:"name"`
	Year        int    `json:"year"`
	ArtistCount int    `json:"artist_count"` // lineup artists in the response (includes isolates; capped)
	EdgeCount   int    `json:"edge_count"`   // total edges in the response (post type-filter)
}

// FestivalGraphCluster groups lineup artists by billing tier (the cluster
// signal at festival scope — see PSY-1080). Matches the SceneGraphCluster
// shape so the same ForceGraphView legend renders both.
type FestivalGraphCluster struct {
	ID         string `json:"id"`          // "tier_<billing_tier>" or "other"
	Label      string `json:"label"`       // human-readable tier name or "Other"
	Size       int    `json:"size"`        // number of lineup artists in this tier
	ColorIndex int    `json:"color_index"` // 0-7 = Okabe-Ito index; -1 = "other" (grey)
}

// FestivalGraphNode mirrors SceneGraphNode — an artist on the festival's bill.
type FestivalGraphNode struct {
	ID                uint   `json:"id"`
	Name              string `json:"name"`
	Slug              string `json:"slug"`
	City              string `json:"city,omitempty"`
	State             string `json:"state,omitempty"`
	UpcomingShowCount int    `json:"upcoming_show_count"`
	ClusterID         string `json:"cluster_id"` // matches FestivalGraphCluster.ID
	IsIsolate         bool   `json:"is_isolate"` // true when the artist has no in-lineup edges (post type-filter)
}

// FestivalGraphLink mirrors SceneGraphLink — a relationship between two
// lineup artists. `type` is preserved from the source signal (shared_bills,
// shared_label, similar, radio_cooccurrence, festival_cobill).
type FestivalGraphLink struct {
	SourceID       uint    `json:"source_id"`
	TargetID       uint    `json:"target_id"`
	Type           string  `json:"type"`
	Score          float64 `json:"score"`
	Detail         any     `json:"detail,omitempty"`
	IsCrossCluster bool    `json:"is_cross_cluster"` // derived: source.cluster_id != target.cluster_id
}

// ──────────────────────────────────────────────
// Festival Service Interface
// ──────────────────────────────────────────────

// FestivalServiceInterface defines the contract for festival operations.
type FestivalServiceInterface interface {
	CreateFestival(req *CreateFestivalRequest) (*FestivalDetailResponse, error)
	GetFestival(festivalID uint) (*FestivalDetailResponse, error)
	GetFestivalBySlug(slug string) (*FestivalDetailResponse, error)
	ListFestivals(filters map[string]interface{}) ([]*FestivalListResponse, error)
	SearchFestivals(query string) ([]*FestivalListResponse, error)
	UpdateFestival(festivalID uint, req *UpdateFestivalRequest) (*FestivalDetailResponse, error)
	DeleteFestival(festivalID uint) error
	GetFestivalArtists(festivalID uint, dayDate *string) ([]*FestivalArtistResponse, error)
	AddFestivalArtist(festivalID uint, req *AddFestivalArtistRequest) (*FestivalArtistResponse, error)
	UpdateFestivalArtist(festivalID, artistID uint, req *UpdateFestivalArtistRequest) (*FestivalArtistResponse, error)
	RemoveFestivalArtist(festivalID, artistID uint) error
	GetFestivalVenues(festivalID uint) ([]*FestivalVenueResponse, error)
	AddFestivalVenue(festivalID uint, req *AddFestivalVenueRequest) (*FestivalVenueResponse, error)
	RemoveFestivalVenue(festivalID, venueID uint) error
	GetFestivalsForArtist(artistID uint) ([]*ArtistFestivalListResponse, error)
	GetFestivalGraph(festivalID uint, types []string) (*FestivalGraphResponse, error)
}
