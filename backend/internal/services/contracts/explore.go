package contracts

import "time"

// ──────────────────────────────────────────────
// /explore landing — read-side DTOs + interface
// ──────────────────────────────────────────────
//
// The /explore page (now a permanent redirect to /graph; PSY-1457) still
// exposes two public read slices used by leftover explore UI / discovery:
//
//  1. Upcoming Shows — chronological list (event_date ASC). No trending,
//     no ranking; just "what's next" with deterministic pagination.
//  2. Shuffle Target — a random pick from the "currently relevant"
//     pool (artists with a show in a ±90-day window). Backs the
//     "Surprise me" affordance (also via DISCOVERY.RANDOM_ARTIST_TARGET).
//
// Featured Bill/Collection editorial slots were retired in PSY-1480.

// ExploreUpcomingShowItem is the wire shape for one row on the
// Upcoming Shows list. Headliner is a denormalized convenience —
// the frontend renders headliner-as-title and links to the show
// detail page for the rest of the bill.
type ExploreUpcomingShowItem struct {
	ID        uint      `json:"id"`
	Slug      string    `json:"slug"`
	Title     string    `json:"title"`
	EventDate time.Time `json:"event_date"`
	// HeadlinerName is the name of the show's headliner artist
	// (show_artists.set_type = 'headliner'). When the show has no
	// headliner row, the first artist by position is used. Empty
	// when the show has no artists at all (shouldn't happen but the
	// pipeline doesn't enforce it).
	HeadlinerName string `json:"headliner_name"`
	// VenueName / VenueCity / VenueState carry the first associated
	// venue. Most shows have exactly one; when there are multiple
	// (unusual), we surface the first by ID so the tile renders.
	VenueName  string  `json:"venue_name"`
	VenueCity  string  `json:"venue_city"`
	VenueState string  `json:"venue_state"`
	City       *string `json:"city,omitempty"`
	State      *string `json:"state,omitempty"`
}

// ExploreUpcomingShowsResponse wraps the page slice + pagination
// metadata for GET /explore/upcoming-shows. Total is the count of
// matching rows (event_date >= NOW(), status = approved) so the
// frontend can render "showing N of M" or hide the next-page control
// once the cursor reaches the end.
type ExploreUpcomingShowsResponse struct {
	Shows  []ExploreUpcomingShowItem `json:"shows"`
	Total  int64                     `json:"total"`
	Limit  int                       `json:"limit"`
	Offset int                       `json:"offset"`
}

// ExploreShuffleTargetResponse is the wire shape for GET
// /explore/shuffle-target — a single random artist drawn from the
// "currently relevant" pool (any artist with a show in the ±90-day
// window). The frontend uses ArtistID/ArtistSlug to navigate to the
// artist detail page. Pointer fields so a truly empty database
// (no qualifying artists) returns {artist_id: null, artist_slug: null,
// artist_name: null} rather than a hard 404 — the surface gracefully
// degrades.
type ExploreShuffleTargetResponse struct {
	ArtistID   *uint   `json:"artist_id"`
	ArtistSlug *string `json:"artist_slug"`
	ArtistName *string `json:"artist_name"`
}

// ExploreServiceInterface defines the contract for the /explore
// read-side endpoints. Implementation lives at
// internal/services/explore. The handler depends on this interface so
// generated mocks (via the contracts scanner) cover unit tests.
type ExploreServiceInterface interface {
	// GetUpcomingShows returns approved shows with event_date >= NOW(),
	// ordered by (event_date ASC, id ASC) for deterministic pagination.
	// Limit is clamped to [1, 50]; offset must be non-negative. Returns
	// the page + total count for matching rows. When cities is non-empty,
	// results are restricted to shows whose (city, state) matches any
	// pair — the same shows.city/state predicate /shows uses (PSY-840).
	GetUpcomingShows(limit, offset int, cities []CityStateFilter) (*ExploreUpcomingShowsResponse, error)

	// GetShuffleTarget returns one random artist from the pool of
	// artists with a show within the past 90 days OR the next 90
	// days. When the pool is empty (cold start, test fixture, etc.),
	// returns a response with all-nil fields rather than an error.
	GetShuffleTarget() (*ExploreShuffleTargetResponse, error)
}
