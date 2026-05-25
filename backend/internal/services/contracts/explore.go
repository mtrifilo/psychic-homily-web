package contracts

import "time"

// ──────────────────────────────────────────────
// /explore landing — read-side DTOs + interface
// ──────────────────────────────────────────────
//
// The /explore page surfaces three slices of content:
//
//  1. Upcoming Shows — chronological list (event_date ASC). No trending,
//     no ranking; just "what's next" with deterministic pagination.
//  2. Featured — the admin-curated bill + collection from
//     featured_slots (admin sets via the surface introduced by PSY-835).
//     Both fields are nullable: the frontend collapses the section when
//     a curator hasn't picked one.
//  3. Shuffle Target — a random pick from the "currently relevant"
//     pool (artists with a show in a ±90-day window). Backs the
//     "Surprise me" affordance.
//
// The shapes here are intentionally minimal — only the fields a tile or
// hero card needs. We do NOT echo the full ShowResponse / Collection
// detail because they're heavier (artists slice with social links,
// preloaded items, etc.) and the /explore surface doesn't render them.
// A dedicated DTO keeps the endpoint payload small and the JSON shape
// explicit.

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

// ExploreFeaturedBill is the bill referent for the Featured slot.
// CuratorNote / CuratorNoteHTML mirror the admin-side wire shape — raw
// markdown source + sanitized HTML render (rendered fresh on each read,
// per the same boundary policy collections + comments use).
type ExploreFeaturedBill struct {
	ID              uint      `json:"id"`
	Slug            string    `json:"slug"`
	Title           string    `json:"title"`
	EventDate       time.Time `json:"event_date"`
	HeadlinerName   string    `json:"headliner_name"`
	VenueName       string    `json:"venue_name"`
	VenueCity       string    `json:"venue_city"`
	VenueState      string    `json:"venue_state"`
	ImageURL        *string   `json:"image_url,omitempty"`
	CuratorNote     *string   `json:"curator_note,omitempty"`
	CuratorNoteHTML string    `json:"curator_note_html,omitempty"`
}

// ExploreFeaturedCollection is the collection referent for the
// Featured slot. Only fields the /explore hero card renders are
// surfaced — the full CollectionDetailResponse with items + tags
// stays gated behind the dedicated collection-detail endpoint.
type ExploreFeaturedCollection struct {
	ID              uint    `json:"id"`
	Slug            string  `json:"slug"`
	Title           string  `json:"title"`
	Description     string  `json:"description,omitempty"`
	DescriptionHTML string  `json:"description_html,omitempty"`
	CoverImageURL   *string `json:"cover_image_url,omitempty"`
	CuratorNote     *string `json:"curator_note,omitempty"`
	CuratorNoteHTML string  `json:"curator_note_html,omitempty"`
}

// ExploreFeaturedResponse is the wire shape for GET /explore/featured.
// Both fields are nullable — when the admin hasn't curated a slot, the
// referent has been deleted, or the referent is no longer publicly
// visible (private collection), the corresponding field is nil. The
// frontend collapses the section when both are nil.
type ExploreFeaturedResponse struct {
	Bill       *ExploreFeaturedBill       `json:"bill"`
	Collection *ExploreFeaturedCollection `json:"collection"`
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
	// the page + total count for matching rows.
	GetUpcomingShows(limit, offset int) (*ExploreUpcomingShowsResponse, error)

	// GetFeatured returns the currently-active bill + collection from
	// featured_slots. Returns nil for either field when:
	//   - the admin hasn't set that slot yet, OR
	//   - the referent has been deleted, OR
	//   - the referent is no longer publicly visible (private collection).
	// Never returns an error solely because a slot is empty; only
	// genuine I/O failures bubble up.
	GetFeatured() (*ExploreFeaturedResponse, error)

	// GetShuffleTarget returns one random artist from the pool of
	// artists with a show within the past 90 days OR the next 90
	// days. When the pool is empty (cold start, test fixture, etc.),
	// returns a response with all-nil fields rather than an error.
	GetShuffleTarget() (*ExploreShuffleTargetResponse, error)
}
