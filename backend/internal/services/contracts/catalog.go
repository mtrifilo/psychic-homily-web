// Package contracts defines the interfaces and shared types for services.
// Services must NEVER be imported by this package — only models and third-party libs.
package contracts

import (
	"time"

	catalogm "psychic-homily-backend/internal/models/catalog"

	"gorm.io/gorm"
)

// ──────────────────────────────────────────────
// Show types
// ──────────────────────────────────────────────

// CreateShowVenue represents a venue in a show creation request.
type CreateShowVenue struct {
	ID      *uint  `json:"id"`
	Name    string `json:"name"`
	City    string `json:"city"`
	State   string `json:"state"`
	Address string `json:"address,omitempty"`
}

// CreateShowArtist represents an artist in a show creation request.
//
// SetType carries the curated bill role and is AUTHORITATIVE whenever it is
// present: is_headliner is then derived from it, never the other way round.
// A nil SetType means the caller is not curating this act's slot, which falls
// back to the legacy IsHeadliner signal and otherwise to SetTypeDefault.
// Callers must not send a contradicting pair; set_type wins if they do.
//
// IsHeadliner is also used for duplicate prevention (headliners can't perform
// at the same venue on the same date), which is why the headliner slot -- and
// only the headliner slot -- may still be inferred from bill position.
//
// That positional inference is per-ACT, so it is not the whole story on a bill:
// the show CREATE and UPDATE paths apply a bill-level rule on top, disarming it
// for the acts a caller left silent once some other act names the headliner.
// In-process callers building either bill should read
// catalog.suppressPositionInferenceWhenHeadlinerNamed before relying on the
// paragraph above.
type CreateShowArtist struct {
	ID              *uint   `json:"id"`
	Name            string  `json:"name"`
	IsHeadliner     *bool   `json:"is_headliner"`
	SetType         *string `json:"set_type,omitempty"`
	InstagramHandle *string `json:"instagram_handle,omitempty"`
}

// CreateShowRequest represents the data needed to create a new show.
// The service will prevent duplicate headliners at the same venue on the same date/time
// and reuse existing venues by name and city (venues are unique by name within a city).
//
// Price / DoorPrice carry the advance/door split; see catalog.Show for what the
// pair means and why neither is derived from the other.
type CreateShowRequest struct {
	Title     string    `json:"title" validate:"required"`
	EventDate time.Time `json:"event_date" validate:"required"`
	// DoorsAt / MusicAt are optional display times. Nil means unknown; they
	// never substitute for EventDate.
	DoorsAt        *time.Time `json:"doors_at"`
	MusicAt        *time.Time `json:"music_at"`
	City           string     `json:"city"`
	State          string     `json:"state"`
	Price          *float64   `json:"price"`
	DoorPrice      *float64   `json:"door_price"`
	AgeRequirement string     `json:"age_requirement"`
	Description    string     `json:"description"`
	TicketURL      string     `json:"ticket_url"`
	// ImageURL is populated by the entity_request fulfiller (PSY-1037, the
	// payload's flyer). The direct create handler does not expose it yet (set
	// post-create via the update endpoint), so it leaves it nil here.
	ImageURL *string            `json:"image_url"`
	Venues   []CreateShowVenue  `json:"venues" validate:"required,min=1"`
	Artists  []CreateShowArtist `json:"artists" validate:"required,min=1"`

	// User context for determining show status
	SubmittedByUserID *uint `json:"-"` // User ID of submitter (set by handler)
	SubmitterIsAdmin  bool  `json:"-"` // Whether submitter is admin (set by handler)
	IsPrivate         bool  `json:"-"` // Whether show should be private (user's list only)
}

// UpdateShowRequest represents the basic show fields that can be updated.
// A nil field means "leave unchanged"; only non-nil fields are written.
// Artist and venue association replacement is handled separately via the
// venues/artists params on UpdateShowWithRelations.
type UpdateShowRequest struct {
	Title     *string    `json:"title"`
	EventDate *time.Time `json:"event_date"`
	// The four NULLABLE-AND-CLEARABLE fields. A *T here would mean a value can
	// be replaced but never RETRACTED, because a nil pointer reads the same
	// whether the caller omitted the key or asked for NULL; Nullable keeps the
	// two apart (PSY-1961). Read them with Present / Clears / Value, never by
	// comparing to nil.
	//
	// All four rather than door_price alone, which is the field that forced the
	// question: door_price is the OPT-IN half of the advance/door split, so
	// removing one is routine curation. But a form where one price clears and
	// its twin silently does not is a worse surface than either consistent
	// choice, and doors_at/music_at have the identical shape, so one mechanism
	// covers the set.
	//
	// The remaining nullable fields on this struct are TEXT, and they are left
	// alone here for a narrower reason than "they already have a gesture".
	// image_url does: showUpdatesToMap normalizes "" to SQL NULL through
	// utils.NilIfEmpty. age_requirement, description and ticket_url do NOT --
	// they store the empty string, and ShowForm sends `undefined` for a blank
	// one anyway, so blanking them is the same silent no-op PSY-1961 removed
	// for prices. That is a REAL remaining gap, not a solved one.
	//
	// It is out of scope here because "" and NULL are indistinguishable to every
	// reader of those three columns (each render guard is a falsy check), so the
	// no-op costs an edit rather than publishing a false fact. A number has no
	// empty spelling at all, which is why these four could not wait.
	DoorsAt        Nullable[time.Time] `json:"doors_at"`
	MusicAt        Nullable[time.Time] `json:"music_at"`
	City           *string             `json:"city"`
	State          *string             `json:"state"`
	Price          Nullable[float64]   `json:"price"`
	DoorPrice      Nullable[float64]   `json:"door_price"`
	AgeRequirement *string             `json:"age_requirement"`
	Description    *string             `json:"description"`
	TicketURL      *string             `json:"ticket_url"`
	ImageURL       *string             `json:"image_url"`
}

// ShowResponse represents the show data returned to clients.
//
// FOUR hand-maintained builders populate this struct, and a new field has to
// reach ALL of them or it silently serializes as null on whichever surface was
// missed:
//
//	(*catalog.ShowService).CreateShow               services/catalog/show.go
//	(*catalog.ShowService).buildUpdatedShowResponse services/catalog/show.go
//	catalog.assembleShowResponse                    services/catalog/show.go
//	                                                (detail + list reads)
//	(*engagement.SavedShowService).buildShowResponse
//	                                                services/engagement/saved_show.go
//	                                                (saved shows AND the
//	                                                personal ICS feed)
//
// The last one is the easy miss: it lives in a different package AND shares its
// name with (*catalog.ShowService).buildShowResponse, so grepping the obvious
// name lands in the wrong package first. PSY-1864 added DoorPrice and missed it.
//
// Note UpdateShow itself builds nothing -- it ends in GetShow -- so the update
// path's literal is buildUpdatedShowResponse.
type ShowResponse struct {
	ID        uint      `json:"id"`
	Slug      string    `json:"slug"`
	Title     string    `json:"title"`
	EventDate time.Time `json:"event_date"`
	// DoorsAt / MusicAt / Price / DoorPrice are null when unknown. Emitted
	// unconditionally rather than with omitempty so a client can tell "not set"
	// from "this response shape predates the field". The show page's ticket line
	// qualifies a price pair as `$35 ADV · DOOR $40` and prints a lone value bare.
	DoorsAt         *time.Time `json:"doors_at"`
	MusicAt         *time.Time `json:"music_at"`
	City            *string    `json:"city"`
	State           *string    `json:"state"`
	Price           *float64   `json:"price"`
	DoorPrice       *float64   `json:"door_price"`
	AgeRequirement  *string    `json:"age_requirement"`
	Description     *string    `json:"description"`
	TicketURL       *string    `json:"ticket_url,omitempty"`
	ImageURL        *string    `json:"image_url"` // Optional show flyer (PSY-521)
	Status          string     `json:"status"`
	SubmittedBy     *uint      `json:"submitted_by,omitempty"`
	RejectionReason *string    `json:"rejection_reason,omitempty"`
	// SubmittedByName / SubmittedByUsername are the resolved display identity
	// of SubmittedBy, so a client can credit the submitter without a second
	// round trip per show to turn the numeric id into a name.
	//
	// DETAIL READS ONLY. Populated by attachSubmitterAttribution, which the two
	// single-show reads (GetShow / GetShowBySlug) call and nothing else — the
	// same footing as Labels on ArtistResponse, and for the same reason: it
	// costs a query, and no list card renders a submitter byline. Both carry
	// omitempty so every list payload's shape is byte-for-byte what it was.
	//
	// ABSENCE IS DELIBERATELY OPAQUE. It means any of: not looked up (a list
	// payload), this show has no submitter (a scraped listing, or an account
	// since deleted), or the submitter may not be named here — they hid their
	// contributions, or their only resolvable name would come from their email
	// address (shared.ResolvePublicContributorCredit, PSY-1866/PSY-1940). The
	// response cannot tell those apart, and must not be made to: render no
	// credit, never a placeholder. "added Jul 12" claims nothing about who,
	// which is the honest reading of every one of those cases.
	//
	// Username is the /users/:username slug. It is absent when the account has
	// none AND when the profile is private, both of which the client must
	// render as unlinked text rather than as a dead link. A present Name with
	// an absent Username is a normal, expected pair.
	SubmittedByName     *string          `json:"submitted_by_name,omitempty"`
	SubmittedByUsername *string          `json:"submitted_by_username,omitempty"`
	RejectionCategory   *string          `json:"rejection_category,omitempty"`
	Venues              []VenueResponse  `json:"venues"`
	Artists             []ArtistResponse `json:"artists"`
	CreatedAt           time.Time        `json:"created_at"`
	UpdatedAt           time.Time        `json:"updated_at"`

	// Status flags (admin-controlled)
	IsSoldOut   bool `json:"is_sold_out"`
	IsCancelled bool `json:"is_cancelled"`

	// Source tracking (for admin view to identify discovered shows)
	Source      string     `json:"source,omitempty"`       // "user" or "discovery"
	SourceVenue *string    `json:"source_venue,omitempty"` // Venue slug for scraped shows
	ScrapedAt   *time.Time `json:"scraped_at,omitempty"`   // When the show was scraped

	// Duplicate detection context
	DuplicateOfShowID *uint `json:"duplicate_of_show_id,omitempty"` // ID of show this may duplicate
}

// VenueResponse represents venue data in show responses
type VenueResponse struct {
	ID       uint    `json:"id"`
	Slug     string  `json:"slug"`
	Name     string  `json:"name"`
	Address  *string `json:"address"`
	City     string  `json:"city"`
	State    string  `json:"state"`
	Timezone *string `json:"timezone"` // IANA zone for rendering this show's time in venue-local time (PSY-985)
	// Capacity and AgePolicy are venue facts a show consumer needs alongside the
	// show itself, so they ride here rather than costing a second round trip to
	// the venue endpoint. NOTE: no SHOW-page surface consumes them yet; that
	// venue module is a later ticket and this carries the data for it. Capacity
	// is already rendered elsewhere, from the venue endpoint, by the Atlas venue
	// panel (see venuePanelIdentityLine), so neither field is dead weight.
	//
	// Both are nullable and neither is sensitive, so (as on VenueDetailResponse)
	// they are served for unverified venues too, unlike Address above.
	//
	// AgePolicy is the venue's HOUSE DEFAULT. The show's own age_requirement is
	// the per-event override and wins wherever both are set; this is the default
	// it departs from. Null means unknown, NOT "all ages".
	Capacity   *int    `json:"capacity"`
	AgePolicy  *string `json:"age_policy"`
	Verified   bool    `json:"verified"`     // Admin-verified as legitimate venue
	IsNewVenue *bool   `json:"is_new_venue"` // True if venue was created during this show submission
}

// ShowArtistSocials represents social media links for artists in show responses
type ShowArtistSocials struct {
	Instagram  *string `json:"instagram"`
	Facebook   *string `json:"facebook"`
	Twitter    *string `json:"twitter"`
	YouTube    *string `json:"youtube"`
	Spotify    *string `json:"spotify"`
	SoundCloud *string `json:"soundcloud"`
	Bandcamp   *string `json:"bandcamp"`
	Website    *string `json:"website"`
}

// ShowArtistLabel is the minimal label reference rendered next to an artist on
// the show bill. Deliberately narrower than ArtistLabelResponse: the bill only
// needs a display name and a link target.
type ShowArtistLabel struct {
	ID   uint   `json:"id"`
	Name string `json:"name"`
	Slug string `json:"slug"`
}

// ArtistResponse represents artist data in show responses.
//
// Labels is populated by responses built through the full detail reads,
// ShowService.GetShow / GetShowBySlug. Both back the single route
// GET /shows/{show_id} (it takes an ID or a slug, see GetShowHandler), which is
// what renders the bill with each act's labels and home location. Responses
// assembled by the plain show-response builders instead - the list endpoints, create,
// approve/reject/publish, and the venue and saved-show projections - leave it
// nil so omitempty drops the key, rather than paying two extra queries per show
// for a field their cards never render. The rule is "did this response come
// from a detail read", not "is this a read": the mutations that return
// s.GetShow(id) (sold-out, cancelled, update) do carry labels.
//
// Mirrors ArtistDetailResponse.Stats: absent means "not looked up",
// present-but-empty means "looked up, artist is unsigned". Keep that
// distinction honest. A producer that forgets the field omits it rather than
// lying with [], and a failed lookup must omit it too.
type ArtistResponse struct {
	ID               uint               `json:"id"`
	Slug             string             `json:"slug"`
	Name             string             `json:"name"`
	State            *string            `json:"state"`
	City             *string            `json:"city"`
	Country          *string            `json:"country"`
	IsHeadliner      *bool              `json:"is_headliner"`
	SetType          string             `json:"set_type"`
	Position         int                `json:"position"`
	Labels           *[]ShowArtistLabel `json:"labels,omitempty"`
	IsNewArtist      *bool              `json:"is_new_artist"`
	BandcampEmbedURL *string            `json:"bandcamp_embed_url"`
	Socials          ShowArtistSocials  `json:"socials"`
}

// BatchShowResult contains the outcome of a batch approve/reject operation.
type BatchShowResult struct {
	Succeeded []uint           `json:"succeeded"`
	Errors    []BatchShowError `json:"errors"`
}

// BatchShowError describes a failure for a single show in a batch operation.
type BatchShowError struct {
	ShowID uint   `json:"show_id"`
	Error  string `json:"error"`
}

// PendingShowsFilter contains optional filters for pending shows queries.
type PendingShowsFilter struct {
	VenueID *uint
	Source  *string // "discovery" or "user"
}

// CityStateFilter represents a city+state pair for multi-city filtering.
type CityStateFilter struct {
	City  string
	State string
}

// UpcomingShowsFilter contains optional filters for GetUpcomingShows
type UpcomingShowsFilter struct {
	City   string
	State  string
	Cities []CityStateFilter
	// TagSlugs narrows results to shows tagged with these slugs.
	// Empty slice means "no tag filter".
	TagSlugs []string
	// TagMatchAny switches the tag filter to OR semantics. When false
	// (default) the shows must have every tag in TagSlugs (AND).
	TagMatchAny bool
}

// ShowCityResponse represents a city with the count of upcoming shows.
//
// Latitude/Longitude are the city's geocoded centroid (the same offline
// GeoNames source PSY-985 uses for venue coordinates), resolved from the
// (city, state) pair. They let the frontend pick the geographically NEAREST
// has-shows city for a new visitor whose exact city has no shows (PSY-981).
// Both are nil together when the geocoder can't resolve the city (an obscure
// place, or a non-US/CA city the GeoNames slice doesn't cover) — callers
// fall back to exact city-name matching, so a miss degrades gracefully.
type ShowCityResponse struct {
	City      string   `json:"city"`
	State     string   `json:"state"`
	ShowCount int      `json:"show_count"`
	Latitude  *float64 `json:"latitude,omitempty"`  // Geocoded city centroid (PSY-985 source, PSY-981)
	Longitude *float64 `json:"longitude,omitempty"` // Geocoded city centroid (PSY-985 source, PSY-981)
}

// ShowSearchResult is the row shape returned by GET /shows/search.
// Contains just enough data for the frontend's
// "{Headliner} @ {Venue} · {Date}" entity-search label, without the cost of
// hydrating the full ShowResponse (artists slice, venues slice, etc).
//
// The headliner is the show_artists row stating set_type = 'headliner',
// falling back to the lowest position and then the lowest artist_id. Do not
// restate the SQL here; catalog/headline_slot.go owns that rule and lists
// every site bound by it. There is no `is_headliner` column on show_artists.
type ShowSearchResult struct {
	ID            uint      `json:"id"`
	Slug          string    `json:"slug"`
	Title         string    `json:"title"`
	HeadlinerName string    `json:"headliner_name"`
	VenueName     string    `json:"venue_name"`
	EventDate     time.Time `json:"event_date"`
}

// OrphanedArtist represents an artist with no remaining show associations.
type OrphanedArtist struct {
	ID   uint   `json:"id"`
	Name string `json:"name"`
	Slug string `json:"slug"`
}

// AdminShowFilters contains filter criteria for admin show queries.
type AdminShowFilters struct {
	Status   string // pending, approved, rejected, private
	FromDate string // RFC3339 format
	ToDate   string // RFC3339 format
	City     string
}

// ParsedShowImport contains the parsed result of a markdown show import.
type ParsedShowImport struct {
	Frontmatter ExportFrontmatter
	Description string
}

// VenueMatchResult represents the result of matching a venue
type VenueMatchResult struct {
	Name       string `json:"name"`
	City       string `json:"city"`
	State      string `json:"state"`
	ExistingID *uint  `json:"existing_id,omitempty"`
	WillCreate bool   `json:"will_create"`
}

// ArtistMatchResult represents the result of matching an artist
type ArtistMatchResult struct {
	Name       string `json:"name"`
	Position   int    `json:"position"`
	SetType    string `json:"set_type"`
	ExistingID *uint  `json:"existing_id,omitempty"`
	WillCreate bool   `json:"will_create"`
}

// ImportPreviewResponse represents the preview response for show import
type ImportPreviewResponse struct {
	Show      ExportShowData      `json:"show"`
	Venues    []VenueMatchResult  `json:"venues"`
	Artists   []ArtistMatchResult `json:"artists"`
	Warnings  []string            `json:"warnings"`
	CanImport bool                `json:"can_import"`
}

// ExportShowData represents show data in export frontmatter
type ExportShowData struct {
	Title          string   `yaml:"title" json:"title"`
	EventDate      string   `yaml:"event_date" json:"event_date"`
	City           string   `yaml:"city,omitempty" json:"city,omitempty"`
	State          string   `yaml:"state,omitempty" json:"state,omitempty"`
	Price          *float64 `yaml:"price,omitempty" json:"price,omitempty"`
	DoorPrice      *float64 `yaml:"door_price,omitempty" json:"door_price,omitempty"`
	AgeRequirement string   `yaml:"age_requirement,omitempty" json:"age_requirement,omitempty"`
	Status         string   `yaml:"status" json:"status"`
}

// ExportVenueSocial represents venue social links in export
type ExportVenueSocial struct {
	Instagram  string `yaml:"instagram,omitempty"`
	Facebook   string `yaml:"facebook,omitempty"`
	Twitter    string `yaml:"twitter,omitempty"`
	YouTube    string `yaml:"youtube,omitempty"`
	Spotify    string `yaml:"spotify,omitempty"`
	SoundCloud string `yaml:"soundcloud,omitempty"`
	Bandcamp   string `yaml:"bandcamp,omitempty"`
	Website    string `yaml:"website,omitempty"`
}

// ExportVenueData represents a venue in the markdown frontmatter
type ExportVenueData struct {
	Name    string            `yaml:"name"`
	City    string            `yaml:"city"`
	State   string            `yaml:"state"`
	Address string            `yaml:"address,omitempty"`
	Zipcode string            `yaml:"zipcode,omitempty"`
	Social  ExportVenueSocial `yaml:"social,omitempty"`
}

// ExportArtistSocial represents artist social links in export
type ExportArtistSocial struct {
	Instagram  string `yaml:"instagram,omitempty"`
	Facebook   string `yaml:"facebook,omitempty"`
	Twitter    string `yaml:"twitter,omitempty"`
	YouTube    string `yaml:"youtube,omitempty"`
	Spotify    string `yaml:"spotify,omitempty"`
	SoundCloud string `yaml:"soundcloud,omitempty"`
	Bandcamp   string `yaml:"bandcamp,omitempty"`
	Website    string `yaml:"website,omitempty"`
}

// ExportArtistData represents an artist in the markdown frontmatter
type ExportArtistData struct {
	Name     string             `yaml:"name"`
	Position int                `yaml:"position"`
	SetType  string             `yaml:"set_type"`
	City     string             `yaml:"city,omitempty"`
	State    string             `yaml:"state,omitempty"`
	Social   ExportArtistSocial `yaml:"social,omitempty"`
}

// ExportFrontmatter represents the complete markdown frontmatter
type ExportFrontmatter struct {
	Version    string             `yaml:"version"`
	ExportedAt string             `yaml:"exported_at"`
	Show       ExportShowData     `yaml:"show"`
	Venues     []ExportVenueData  `yaml:"venues"`
	Artists    []ExportArtistData `yaml:"artists"`
}

// ──────────────────────────────────────────────
// Venue types
// ──────────────────────────────────────────────

// MaxVenueAgePolicyLength is the maximum length, in CHARACTERS, accepted for a
// venue's house-default age policy. It is the single source of truth for the
// bound: the venues.age_policy column is VARCHAR(100), and every write path
// (admin create, admin update, and the contributor suggest-edit queue) must
// reject at this length rather than let Postgres raise 22001 mid-write.
//
// Characters, not bytes, is load-bearing. Postgres VARCHAR(n) counts
// characters and huma's maxLength tag counts runes, so a byte-based check
// would disagree with both: a 40-character CJK door rule is 120 bytes and
// would be rejected by a byte check while the column accepts it happily.
// Compare with utf8.RuneCountInString, never len().
//
// A door rule is a handful of words ("all ages", "18+ w/ guardian"), so this is
// deliberately tighter than shows.age_requirement's VARCHAR(255).
const MaxVenueAgePolicyLength = 100

// MinVenueCapacity and MaxVenueCapacity bound the RANGE of venues.capacity on
// all three write paths: admin create, admin update, and the contributor
// suggest-edit queue. They are the single source of truth for the range, so the
// three surfaces cannot drift into disagreeing about what a legal capacity is.
//
// Range only. The three paths still differ on the CLEAR gesture: the
// contributor path can set the column back to NULL, while the admin bodies take
// a *int where nil means "not supplied", so they cannot express a clear at all.
// That asymmetry lives in the body contracts, not here.
//
// FOUR other copies of these numbers exist and only two are pinned by a test.
// Changing either constant means changing all of them:
//   - the huma minimum/maximum tags on both admin venue bodies
//     (handlers/catalog/venue.go) -- pinned by TestVenueCapacitySchemaTagsMatchContract
//   - VENUE_CAPACITY_BOUNDS in frontend/features/contributions/types.ts
//     (pre-validates the edit drawer) -- NOT pinned
//   - VENUE_CAPACITY_MIN/MAX in cli/src/commands/submit-venue.ts
//     (drops an out-of-range ingest capacity instead of failing the venue)
//     -- NOT pinned
//
// Nothing enforces the two TypeScript copies across the language boundary.
//
// Superseding an earlier note: the age_policy migration
// (20260801143000_add_venue_age_policy.up.sql) says capacity is "admin/ingest
// only" and therefore not a precedent for contributor curation. That was true
// when it was written and stopped being true in PSY-1694.
//
// The floor is 1, not 0. NULL already means "we do not know this room's
// capacity"; a stored 0 would be a second way to say the same thing, and one
// that reads downstream as a known fact ("this room holds nobody"). Negative
// values are nonsense for the same reason.
//
// The ceiling is deliberately generous rather than tight. The largest stadium
// on earth seats roughly 130,000, so 200,000 admits any real room while still
// catching the failure this bound exists for: a typo or a scraped garbage
// number landing as a capacity nobody notices. It is a sanity rail, not a
// domain claim about how big a venue can be.
//
// Callers must range-check BEFORE narrowing a JSON number to int: values
// arriving through the pending-edit queue are float64, and converting an
// out-of-range float64 to int is implementation-defined in Go.
const (
	MinVenueCapacity = 1
	MaxVenueCapacity = 200000
)

// MinShowPrice and MaxShowPrice bound both shows.price and shows.door_price.
// One rail for the pair: an advance price and a door price are the same kind of
// fact, and a ceiling that admitted one but not the other would be arbitrary.
//
// The ceiling is a typo guard, not a domain claim -- it exists so a fat-fingered
// "3500" instead of "35.00" is rejected at the boundary instead of being
// published on a show page.
//
// Enforced on the three SELF-SERVE write paths, and only those: the show create
// resolver, the show update handler, and the entity-request payload validator
// (which keeps its own copy of the ceiling -- see maxRequestPrice).
//
// NOT enforced on four ADMIN-ONLY paths:
//
//   - pipeline.DiscoveryService's import (cmd/discovery-import and
//     POST /admin/discovery/import) writes whatever parseEventPrices reads out
//     of a scraped price string, straight onto the row. That parser's own digit
//     bound is a text-shape guard on what counts as a stated amount, not a copy
//     of this rail.
//   - catalog.ConfirmShowImport (markdown frontmatter) and admin.importShow
//     (the JSON data-sync import) map a price straight onto the row. A value
//     above the rail but under the column's DECIMAL(10,2) width is stored and
//     published; one above the column width fails at INSERT -- as a 500 on the
//     markdown path, as a counted per-row error on the data-sync report.
//   - admin.RevisionService.Rollback writes whatever old_value the revision
//     recorded, with no allowlist and no bounds check. That is deliberate (see
//     the reasoning on Rollback: history can hold values that predate a bound,
//     so refusing them would break undo for exactly the rows most likely to
//     need it), and restore-only -- it cannot introduce a value the rails never
//     saw, except one that arrived via the import paths above.
//
// A PRE-EXISTING gap that `price` already had and `door_price` now inherits.
// All four are admin-gated, so it is a trusted-operator footgun, not an
// escalation. Closing the import half means validating at the service
// chokepoint (ShowService.CreateShow / showUpdatesToMap) rather than adding a
// fourth copy of the check.
//
// NOT registered in NumericEditFieldBounds: that registry is the pending-edit
// (suggest/approve) pipeline's, its bounds are ints, and shows have no
// contributor allowlist to route through it. A price is a decimal, so narrowing
// it to an int there would silently drop the cents.
const (
	MinShowPrice = 0
	MaxShowPrice = 10000
)

// NumericEditBounds is the accepted range for one whole-number column that the
// contributor pending-edit pipeline can write.
type NumericEditBounds struct {
	// DisplayName is the user-facing field name in a validation message.
	DisplayName string
	Min         int
	Max         int
	// LegacyTextEncoding marks a field that was editable through the
	// pending-edit pipeline BEFORE it was registered here, back when the edit
	// drawer submitted it as a string. Any such edit already stored in
	// pending_entity_edits or revisions.field_changes therefore holds "1985"
	// rather than 1985, and admin.NarrowNumericUpdates parses those rather than
	// refusing them (its doc comment carries the argument).
	//
	// Submit-side behaviour is unaffected: a string is refused at the door for
	// every field in this registry, flag or no flag. This only governs how the
	// apply and rollback paths read what is ALREADY stored.
	//
	// False for a field registered on the same day its drawer control became
	// numeric, because no string of it was ever written. Do not set it to buy
	// leniency for new fields; it is a statement about history, not a policy.
	LegacyTextEncoding bool
}

// MinCatalogYear is the floor for every year-valued catalog column a
// contributor can edit: labels.founded_year and releases.release_year.
//
// Both fields share one floor on purpose. They answer the same kind of
// question ("what year was this?"), a contributor typing into either makes the
// same mistakes, and two constants would only invite them to drift apart.
//
// 1000 is a four-digit-year sanity rail, not a domain claim. A tighter floor is
// available on paper (Berliner pressed the first commercial records in the
// 1890s, so nothing in this catalog can honestly predate that) and is
// deliberately not taken: the gate exists to catch a value NOBODY TYPED -- a
// zero, a negative, a three-digit slip, a fraction, a scraped string -- and not
// to adjudicate music history at the expense of some reissue, field recording
// or archival edge case the catalog has not met yet. This mirrors the reasoning
// behind MaxVenueCapacity's generous ceiling.
const MinCatalogYear = 1000

// MaxCatalogYear is the inclusive ceiling for those same columns: next year.
//
// Next year rather than this year because a release can be announced before it
// exists -- a record dated 2027 is routine press-cycle material in 2026 -- while
// anything further out is far likelier to be a typo than a real announcement.
//
// A FUNCTION, not a constant, and that is the load-bearing part. A package-level
// var initialised from time.Now() freezes at process start, so a server that had
// been up since November would spend the first days of January rejecting a
// legitimate next-year value with a message quoting last year's ceiling, and
// would heal only on the next deploy. Resolving per call costs nothing on these
// paths (one submit, one approve) and cannot go stale.
//
// UTC so the ceiling does not depend on which zone the server happens to run
// in, and so the frontend's pre-validator (which uses getUTCFullYear) computes
// the same number. For the handful of hours each New Year when UTC and a
// contributor's local zone disagree about the date, the +1 of slack is what
// absorbs the difference: a contributor west of UTC gets a ceiling one year
// early, one east of UTC gets it a few hours late.
func MaxCatalogYear() int {
	return time.Now().UTC().Year() + 1
}

// NumericEditFieldBounds is the registry both halves of the pending-edit
// pipeline read: the suggest-edit validator rejects out-of-range values at
// submit, and the approve path narrows the surviving JSONB float64 to an int
// before it reaches an untyped Updates().
//
// It exists as one registry rather than hand-written branches so those halves
// cannot drift. Adding a field here gives it BOTH gates; adding it to only one
// of two hardcoded call sites would give it neither reliably.
//
// A function returning a fresh map rather than a package var, because
// MaxCatalogYear has to be resolved when the value is checked (see its doc).
// A fresh map per call also means no caller can mutate a shared registry. Every
// call site is a single submit or a single approve, so the allocation is noise.
func NumericEditFieldBounds() map[string]NumericEditBounds {
	maxYear := MaxCatalogYear()
	return map[string]NumericEditBounds{
		// Registered in PSY-1694 in the same change that made its drawer control
		// numeric, so no capacity was ever stored as text: LegacyTextEncoding
		// stays false.
		"capacity": {DisplayName: "Capacity", Min: MinVenueCapacity, Max: MaxVenueCapacity},
		// Registered in PSY-1703, long after their drawer controls started
		// submitting text.
		"founded_year": {DisplayName: "Founded year", Min: MinCatalogYear, Max: maxYear, LegacyTextEncoding: true},
		"release_year": {DisplayName: "Release year", Min: MinCatalogYear, Max: maxYear, LegacyTextEncoding: true},
	}
}

// CreateVenueRequest represents the data needed to create a new venue
type CreateVenueRequest struct {
	Name        string  `json:"name" validate:"required"`
	Address     *string `json:"address"`
	City        string  `json:"city" validate:"required"`
	State       string  `json:"state" validate:"required"`
	Country     *string `json:"country"`
	Zipcode     *string `json:"zipcode"`
	Capacity    *int    `json:"capacity"`
	AgePolicy   *string `json:"age_policy"` // House-default age rule, free text (PSY-1682)
	Instagram   *string `json:"instagram"`
	Facebook    *string `json:"facebook"`
	Twitter     *string `json:"twitter"`
	YouTube     *string `json:"youtube"`
	Spotify     *string `json:"spotify"`
	SoundCloud  *string `json:"soundcloud"`
	Bandcamp    *string `json:"bandcamp"`
	Website     *string `json:"website"`
	Description *string `json:"description"`
	ImageURL    *string `json:"image_url"`
	SubmittedBy *uint   `json:"-"` // Set by handler, not from request body
}

// UpdateVenueRequest represents the data that can be updated on a venue.
// A nil field means "leave unchanged"; only non-nil fields are written.
//
// Name/City/State map to NOT NULL columns and are written as-is (the handler
// rejects empty values up front). The remaining optional string columns are
// nullable, so Description, ImageURL and AgePolicy normalize an empty string to
// SQL NULL in the service (utils.NilIfEmpty): that is how a caller CLEARS them.
// Address/Country/Zipcode and the social fields preserve the prior behavior of
// writing the value through verbatim.
type UpdateVenueRequest struct {
	Name        *string `json:"name"`
	Address     *string `json:"address"`
	City        *string `json:"city"`
	State       *string `json:"state"`
	Country     *string `json:"country"`
	Zipcode     *string `json:"zipcode"`
	Capacity    *int    `json:"capacity"`
	AgePolicy   *string `json:"age_policy"` // House-default age rule, free text (PSY-1682)
	Description *string `json:"description"`
	ImageURL    *string `json:"image_url"`
	Instagram   *string `json:"instagram"`
	Facebook    *string `json:"facebook"`
	Twitter     *string `json:"twitter"`
	YouTube     *string `json:"youtube"`
	Spotify     *string `json:"spotify"`
	SoundCloud  *string `json:"soundcloud"`
	Bandcamp    *string `json:"bandcamp"`
	Website     *string `json:"website"`
}

// VenueDetailResponse represents the venue data returned to clients
type VenueDetailResponse struct {
	ID        uint     `json:"id"`
	Slug      string   `json:"slug"`
	Name      string   `json:"name"`
	Address   *string  `json:"address"`
	City      string   `json:"city"`
	State     string   `json:"state"`
	Country   *string  `json:"country,omitempty"`
	Latitude  *float64 `json:"latitude,omitempty"`  // Geocoded city centroid (PSY-985)
	Longitude *float64 `json:"longitude,omitempty"` // Geocoded city centroid (PSY-985)
	// Street-precise coordinates of the venue's address (PSY-1536, Nominatim).
	// Present ONLY for verified venues with a fresh geocode — unverified venues
	// always omit them (mirrors the address/zipcode redaction, protecting
	// DIY/house venues from being street-mapped before human review).
	StreetLatitude   *float64       `json:"street_latitude,omitempty"`
	StreetLongitude  *float64       `json:"street_longitude,omitempty"`
	GeocodePrecision *string        `json:"geocode_precision,omitempty"` // rooftop|interpolated|city
	Timezone         *string        `json:"timezone"`                    // IANA zone resolved from location (PSY-985)
	Zipcode          *string        `json:"zipcode"`
	Capacity         *int           `json:"capacity"`   // Venue capacity (PSY-1179); not redacted for unverified venues
	AgePolicy        *string        `json:"age_policy"` // House-default age rule (PSY-1682); free text, not redacted
	Description      *string        `json:"description,omitempty"`
	ImageURL         *string        `json:"image_url"`    // Optional venue photo (PSY-521)
	Verified         bool           `json:"verified"`     // Admin-verified as legitimate venue
	SubmittedBy      *uint          `json:"submitted_by"` // User ID who originally submitted this venue
	Social           SocialResponse `json:"social"`
	CreatedAt        time.Time      `json:"created_at"`
	UpdatedAt        time.Time      `json:"updated_at"`
	// Provenance is the freshness/attribution stamp (PSY-1542). Filled by
	// exactly two reads — GetVenueDetail, and the Atlas city-scoped list under
	// IncludeRailFields. Every other producer of this type (create, update,
	// verify, search, browse, and the cheap identity lookups) leaves it nil, so
	// nothing pays for the aggregations to discard them.
	Provenance *VenueProvenance `json:"provenance,omitempty"`
}

// Venue provenance source keys. The set is deliberately small and each member
// is backed by a fact we actually store — nothing here is inferred.
const (
	// VenueProvenanceSourceIngest is emitted when venues.data_source is set,
	// which is the column the enrichment/ingest writers own. NOTE: no live
	// writer populates it for venues today, so this key is rare in practice
	// and clients must render the source segment only when it is present.
	VenueProvenanceSourceIngest = "ingest"
	// VenueProvenanceSourceCommunity is emitted when at least one human has
	// touched the venue — an approved entity edit, or a confirmation.
	VenueProvenanceSourceCommunity = "community"
)

// VenueProvenance is the "how fresh is this, and who says so" stamp rendered
// on the Atlas venue panel and the city rail (PSY-1542).
//
// It exists because a crowdsourced venue map dies of undetectable staleness:
// a reader can only trust a listing if the page tells them when it was last
// touched and by how many people. Every field is an aggregate over rows we
// already keep — nothing is denormalised onto venues, matching the
// no-counter-column precedent from likes.
//
// Honest degradation matters more than a complete-looking stamp: any count
// that cannot be sourced comes back zero and the client omits that segment
// rather than inventing a number.
type VenueProvenance struct {
	// UpdatedAt is venues.updated_at — the last write of any kind to the row.
	UpdatedAt time.Time `json:"updated_at"`
	// EditCount is the number of APPROVED community edits to this venue
	// (pending_entity_edits, status=approved). Pending and rejected edits are
	// excluded: they never changed what the reader is looking at. This is the
	// canonical per-venue edit event — the venue update path does not write
	// entity_edit_audit_logs (only artist/label/release/festival do), so
	// counting that table here would report zero for every venue.
	EditCount int `json:"edit_count"`
	// ContributorCount is the number of DISTINCT users behind EditCount.
	ContributorCount int `json:"contributor_count"`
	// ConfirmationCount is how many distinct users have tapped "confirm info".
	ConfirmationCount int `json:"confirmation_count"`
	// LastConfirmedAt is the most recent confirmation, nil when there is none.
	//
	// Deliberately WHEN, not WHO. venue_confirmations stores user_id and the
	// index would answer "who confirmed last" in one row, but naming that
	// person would attach an identity to a public, unauthenticated read that
	// currently exposes none — the whole stamp is counts. Whether a
	// confirmation should be publicly attributed to its author is a product and
	// privacy call, not one to make here; the aggregate ships until it's made.
	LastConfirmedAt *time.Time `json:"last_confirmed_at,omitempty"`
	// Sources lists the VenueProvenanceSource* keys that apply, in a stable
	// order (ingest before community). Empty when neither applies.
	Sources []string `json:"sources"`
}

// VenueConfirmationResponse is returned by POST /venues/{venue_id}/confirm.
//
// It carries the post-mutation aggregate so the client never needs a
// follow-up read, plus ViewerHasConfirmed — which is always true on a
// successful confirm, including the idempotent repeat. Reads deliberately do
// NOT carry viewer state: GET /venues is a public, cacheable endpoint and
// per-viewer fields there would poison a shared cache.
type VenueConfirmationResponse struct {
	ConfirmationCount  int        `json:"confirmation_count"`
	LastConfirmedAt    *time.Time `json:"last_confirmed_at,omitempty"`
	ViewerHasConfirmed bool       `json:"viewer_has_confirmed"`
}

// VenueWithShowCountResponse includes upcoming show count for a venue.
//
// The fields below UpcomingShowCount are the Atlas city-view venue rail's
// row payload: enough to render "NEXT <date> · <bill> · <genre family>"
// without a per-venue follow-up request. They are batched over the returned
// PAGE of venues (never per-venue), and every one of them is best-effort —
// an aggregation failure logs and leaves the field empty rather than failing
// the list, because a venue row without its meta line still lists.
type VenueWithShowCountResponse struct {
	VenueDetailResponse
	UpcomingShowCount int `json:"upcoming_show_count"`
	// ShowsThisWeek is the <=7-day slice of UpcomingShowCount — same
	// definition SceneListResponse.ShowsThisWeek uses at scene scope. Drives
	// the rail's "Next 7 days" filter chip and its header stat.
	//
	// ROLLING from now, so both of those are worded "next 7 days" and neither
	// says "this week" (PSY-1732). The field NAME is the stale half of that
	// mismatch; the labels are the correct half.
	ShowsThisWeek int `json:"shows_this_week"`
	// NextShowDate is the soonest upcoming approved show's date as an ISO
	// YYYY-MM-DD string, rendered in the VENUE's timezone (not UTC, not the
	// viewer's): a 9pm Friday show in Austin must not read as Saturday
	// because the row was serialized from a UTC timestamp. Empty when the
	// venue has no upcoming show.
	NextShowDate string `json:"next_show_date,omitempty"`
	// NextShowTitle is that show's own title, which is EMPTY for most shows —
	// the app composes display names from the bill everywhere else. Clients
	// must fall back to NextShowArtists (same contract as SceneShowSummary).
	NextShowTitle string `json:"next_show_title,omitempty"`
	// NextShowArtists is that show's bill in position order, so a titleless
	// show still carries band names (the PSY-1325 rationale, at venue scope).
	NextShowArtists []string `json:"next_show_artists,omitempty"`
	// DominantGenre is the venue's dominant genre-family key, or "" when no
	// family holds a confident share. Same rule and same family keys as
	// SceneListResponse.DominantGenre (one shared dominantGenreFamily), over
	// the venue's own booking mass: the tagged artists on its approved shows
	// inside a fixed recent window, past and upcoming alike. venueDominantGenres
	// owns the window and the reasons for it.
	DominantGenre string `json:"dominant_genre,omitempty"`
	// HostsAllAges reports that this venue carries the canonical all-ages tag
	// (catalogm.TagSlugAllAges) — it hosts all-ages shows AT LEAST SOMETIMES.
	// Drives the rail's "All-ages shows" chip (PSY-1573).
	//
	// THREE-VALUED, and the pointer is the whole point. false means UNTAGGED
	// (nobody has said, NOT 21+); NIL means NOT DETERMINED, which is what the
	// caller gets both without IncludeRailFields and when the tag lookup itself
	// failed. Those two are different questions and a plain bool answers them
	// with the same confident "no": a client would read "no venue here is
	// tagged" off a query that never ran, or off a browse response that never
	// asked. Coverage is near-zero, so that wrong reading would be the COMMON
	// one. Nil must degrade to "we don't know", never to a claim.
	//
	// It is also NOT the house-default age rule; that is AgePolicy on the
	// embedded venue. A 21+ AgePolicy alongside a true HostsAllAges is a
	// coherent room (a 21+ house booking an all-ages matinee), not a
	// contradiction, and is the case the chip exists to surface.
	HostsAllAges *bool `json:"hosts_all_ages,omitempty" doc:"True when the venue carries the canonical all-ages tag, meaning it hosts all-ages shows AT LEAST SOMETIMES. False means UNTAGGED, not 21+, and is not an age claim about the room. Absent means NOT DETERMINED: the field is only populated when include_rail=true, and is omitted if the tag lookup failed. This is not the venue's house-default age rule, which is age_policy."`
}

// VenueListingEntry is a venue reduced to the two fields a link needs: the slug
// that builds the href, and the name that labels it. The venue twin of
// ArtistListingEntry, for the same reason and by the same reasoning — read that
// one first; only what differs for venues is written down here.
//
// # Why /venues needed its own projection
//
// The `/venues` page's JSON-LD `ItemList` was built from `GET /venues?limit=100`,
// so it advertised the hundred most active venues and silently dropped the rest.
// The cap was not a product decision: the call had asked for 200, `GET /venues`
// declares `maximum:"100"` on `limit`, and huma rejected it with a 422 before the
// handler ran — which the frontend's fail-open turned into no `ItemList` at all
// for months. Capping at 100 fixed the 422 and left the truncation (PSY-1764).
//
// A projection removes the limit rather than raising it, which is what makes the
// truncation impossible rather than merely further away. Measured against
// production on 2026-08-09, 297 verified venues:
//
//	                                raw bytes    % of the 2 MB item cap (base64)
//	GET /venues?limit=100              71,172     4.5%   (100 of 297 venues)
//	GET /venues, all 297 rows         195,657    12.4%
//	GET /venues/listing, all 297       18,289     1.2%
//
// The row width is what decides the headroom, and that is why raising the
// `maximum` was rejected: at 659 raw bytes per full row the build gate (80% of
// the ~1.5 MB raw budget, lib/data-cache-budget) fires at ~1,900 venues, against
// 61.6 bytes here, which reaches it at ~20,400. The catalogue went from 198
// venues on 2026-07-29 to 297 on 2026-08-09 — 50% in eleven days — so ~1,900 is
// not a comfortable ceiling, and paginating would have kept carrying the unread
// fields while adding a round trip per page above the Suspense boundary.
//
// # ~20,400 IS NOT THE CEILING, AND THE CACHE GATE DOES NOT COVER THE REAL ONE
//
// Everything above weighs the API response. The artifact the change actually
// enlarges is the `/venues` HTML: one JSON-LD `ListItem` per venue, in the
// document every human and every crawler downloads. Measured on the production
// page the same day, before this change, its 100-item block was 12,735 raw bytes
// — 127.3 bytes per item, TWICE the 61.6 the cache budget counts, because each
// item carries `@type`, `position`, the absolute URL and the name.
//
// So the page grows at ~127 bytes per venue: ~38 KB at 297, and roughly 2.6 MB
// at the ~20,400 the cache figure suggests is safe. Nothing gates that. The
// build-time budget in lib/data-cache-budget weighs the fetch, and its
// request-time half only reports, so page weight is the constraint that binds
// first and it binds unmeasured. This is recorded rather than solved: at today's
// size the block is ~16% of a 239 KB document, which is not worth a gate of its
// own. When it is, the answer is not a limit — a truncated ItemList is the
// defect this endpoint exists to remove — but dropping the block and leaning on
// the sitemap, which is already sharded for exactly this reason (SitemapEntry).
//
// # The set this returns, and the order it does NOT share
//
// The SET is exactly what unfiltered `GET /venues` returns — verified venues —
// minus rows with a NULL or empty slug, which cannot form a URL. That gate is
// stated once, as catalog.venueBrowseGate. Note it differs from the artists one:
// venues are gated on `verified`, NOT on having an upcoming show, so a quiet
// venue is still listed and still indexed.
//
// The ORDER is by name, and deliberately NOT the browse page's activity sort
// that the artist twin reuses. The consumer stamps `position` from it, so an
// activity sort would renumber the whole list every time a show is booked, and
// reproducing it costs an aggregate over every upcoming booking on a public
// endpoint. GetVenueListing carries the full argument.
//
// The venues sitemap family does NOT share the gate: it filters on slug alone,
// so an unverified venue can appear there and not here. Identical sets today
// (both 297 on 2026-08-09), by data rather than by construction.
type VenueListingEntry struct {
	Slug string `json:"slug" doc:"URL slug for the venue"`
	Name string `json:"name" doc:"Venue display name"`
}

// VenueListFilters contains filter options for listing venues
type VenueListFilters struct {
	State    string
	City     string
	Cities   []CityStateFilter
	Verified *bool
	// TagSlugs narrows results to venues tagged with these slugs.
	// Empty slice means "no tag filter".
	TagSlugs []string
	// TagMatchAny switches the tag filter to OR semantics. When false
	// (default) the venues must have every tag in TagSlugs (AND).
	TagMatchAny bool
	// IncludeRailFields opts in to the Atlas city-view payload
	// (VenueWithShowCountResponse's next-show / next-7-days / dominant-genre /
	// all-ages fields). OFF by default and deliberately explicit: filling it
	// costs four extra batched aggregations, and the venue browse page — the
	// endpoint's other caller — renders none of those fields.
	IncludeRailFields bool
	// MetroRollup widens a City+State filter from that literal city to the
	// whole US Census CBSA metro it belongs to, so a Tempe venue lists under
	// Phoenix and an Oakland venue under San Francisco (PSY-1574). This is the
	// SAME scope the Atlas scene itself is computed from, which is the point:
	// the scene page already counts those venues, so the city rail beside it
	// must not disagree. Ignored when Cities is set (an explicit multi-city
	// pick is already saying which cities it wants) or when either City or
	// State is empty (a metro can only be resolved from a disambiguated place).
	MetroRollup bool
}

// VenueShowResponse represents a show in the venue shows endpoint
type VenueShowResponse struct {
	ID uint `json:"id"`
	// Slug is the canonical /shows/{slug} target, matching ShowResponse.Slug.
	// Empty when the show has no slug, and clients fall back to the id.
	Slug      string    `json:"slug"`
	Title     string    `json:"title"`
	EventDate time.Time `json:"event_date"`
	City      *string   `json:"city"`
	State     *string   `json:"state"`
	// Price and DoorPrice are the advance/door pair, and both are served
	// because a list that carried only the advance half would UNDER-REPORT a
	// split price: a reader scanning the venue archive saw $35 and opened a
	// show costing $40 at the door (PSY-1962). Null on either means "not
	// recorded"; zero means free.
	Price          *float64         `json:"price"`
	DoorPrice      *float64         `json:"door_price"`
	AgeRequirement *string          `json:"age_requirement"`
	// Status flags, so a venue listing can strike through a cancelled date and
	// badge a sold-out one without a second fetch per row.
	IsCancelled bool             `json:"is_cancelled"`
	IsSoldOut   bool             `json:"is_sold_out"`
	Artists     []ArtistResponse `json:"artists"`
}

// VenueShowsQuery carries the paging and filtering knobs for one venue's show
// list.
//
// A struct rather than positional arguments because Limit, Offset and Year are
// all ints: a transposed pair at a call site compiles cleanly and silently pages
// the wrong window. Grouping them also means the next knob is a field, not
// another parameter every caller has to re-thread.
type VenueShowsQuery struct {
	// TimeFilter is "upcoming", "past" or "all". Any other value is treated as
	// "upcoming", matching shared.VenueLocalDateCondition and the handler's own
	// default for an omitted time_filter.
	TimeFilter string
	// Limit caps the page. Zero returns no rows while still reporting the full
	// total. That is long-standing behaviour, which the handler shields callers
	// from by defaulting an omitted limit to 20. Negative is clamped to zero
	// rather than passed through, because GORM reads a negative limit as "no
	// limit" and would return the venue's entire history.
	Limit int
	// Offset skips this many rows of the ordered page. Negative is clamped to 0.
	Offset int
	// Year narrows to a single VENUE-LOCAL calendar year, and is reflected in
	// the total. Zero means every year.
	Year int
}

// VenueShowYearCount is one bucket of a venue's show-year histogram: a
// venue-local calendar year and how many of that venue's shows fall in it.
// Only years with at least one show are emitted.
type VenueShowYearCount struct {
	Year  int   `json:"year" doc:"Venue-local calendar year"`
	Count int64 `json:"count" doc:"Shows at this venue in that year, within the requested time filter"`
}

// VenueShowMonthCount is one bar of a venue's show histogram at MONTH
// resolution (PSY-1769).
//
// The year is part of the bucket, not context around it: the archive it feeds
// spans a venue's whole history by default, where a bare month number would fold
// every March together.
type VenueShowMonthCount struct {
	Year  int   `json:"year" doc:"Venue-local calendar year"`
	Month int   `json:"month" doc:"Venue-local calendar month, 1-12"`
	Count int64 `json:"count" doc:"Shows at this venue in that month, within the requested time filter"`
}

// VenueCityResponse represents a city with venue count for filtering
type VenueCityResponse struct {
	City       string `json:"city"`
	State      string `json:"state"`
	VenueCount int    `json:"venue_count"`
}

// UnverifiedVenueResponse represents an unverified venue for admin review
type UnverifiedVenueResponse struct {
	ID          uint      `json:"id"`
	Slug        string    `json:"slug"`
	Name        string    `json:"name"`
	Address     *string   `json:"address"`
	City        string    `json:"city"`
	State       string    `json:"state"`
	Zipcode     *string   `json:"zipcode"`
	SubmittedBy *uint     `json:"submitted_by"`
	CreatedAt   time.Time `json:"created_at"`
	ShowCount   int       `json:"show_count"` // Number of shows using this venue
}

// ──────────────────────────────────────────────
// Artist types
// ──────────────────────────────────────────────

// CreateArtistRequest represents the data needed to create a new artist
type CreateArtistRequest struct {
	Name        string  `json:"name" validate:"required"`
	State       *string `json:"state"`
	City        *string `json:"city"`
	Country     *string `json:"country"`
	Instagram   *string `json:"instagram"`
	Facebook    *string `json:"facebook"`
	Twitter     *string `json:"twitter"`
	YouTube     *string `json:"youtube"`
	Spotify     *string `json:"spotify"`
	SoundCloud  *string `json:"soundcloud"`
	Bandcamp    *string `json:"bandcamp"`
	Website     *string `json:"website"`
	Description *string `json:"description"`
	// ImageURL + BandcampEmbedURL are populated by the entity_request fulfiller
	// (PSY-1038). The direct admin create handler does not expose them yet (set
	// post-create via the update endpoints), so it leaves them nil here.
	ImageURL         *string `json:"image_url"`
	BandcampEmbedURL *string `json:"bandcamp_embed_url"`
}

// UpdateArtistRequest represents the data that can be updated on an artist.
// A nil field means "leave unchanged"; only non-nil fields are written.
//
// Every column here is nullable, so the service normalizes an empty string to
// SQL NULL (utils.NilIfEmpty) for all fields except Name, which maps to a NOT
// NULL column. Name additionally drives slug regeneration and a uniqueness
// check in the service. BandcampEmbedURL is the embed-specific column distinct
// from the Bandcamp social profile URL.
type UpdateArtistRequest struct {
	Name             *string `json:"name"`
	State            *string `json:"state"`
	City             *string `json:"city"`
	Country          *string `json:"country"`
	Description      *string `json:"description"`
	BandcampEmbedURL *string `json:"bandcamp_embed_url"`
	Instagram        *string `json:"instagram"`
	Facebook         *string `json:"facebook"`
	Twitter          *string `json:"twitter"`
	YouTube          *string `json:"youtube"`
	Spotify          *string `json:"spotify"`
	SoundCloud       *string `json:"soundcloud"`
	Bandcamp         *string `json:"bandcamp"`
	Website          *string `json:"website"`
}

// ArtistDetailResponse represents the artist data returned to clients
type ArtistDetailResponse struct {
	ID               uint           `json:"id"`
	Slug             string         `json:"slug"`
	Name             string         `json:"name"`
	State            *string        `json:"state"`
	City             *string        `json:"city"`
	Country          *string        `json:"country,omitempty"` // PSY-558: optional country (Australia, UK, etc.)
	BandcampEmbedURL *string        `json:"bandcamp_embed_url"`
	Description      *string        `json:"description,omitempty"`
	ImageURL         *string        `json:"image_url"`        // Optional artist photo (PSY-521)
	ImageSource      *string        `json:"image_source"`     // Image provider for attribution (PSY-1175)
	ImageSourceURL   *string        `json:"image_source_url"` // Deep linkback for attribution (PSY-1175)
	ImageLicense     *string        `json:"image_license"`    // CC license for a Commons photo (PSY-1232)
	ImageAuthor      *string        `json:"image_author"`     // Photographer credit for a Commons photo (PSY-1232)
	Social           SocialResponse `json:"social"`
	CreatedAt        time.Time      `json:"created_at"`
	UpdatedAt        time.Time      `json:"updated_at"`
	// Stats is populated only by detail-page lookups (GetArtist /
	// GetArtistBySlug — PSY-639). List, search, and mutation responses leave
	// it nil so the omitempty tag drops it from the wire.
	Stats *ArtistStatsResponse `json:"stats,omitempty"`
}

// ArtistStatsResponse carries the at-a-glance counts surfaced on the artist
// detail page sidebar (PSY-639). Folded into ArtistDetailResponse on the
// detail-page lookups; nil on list / search / mutation responses.
type ArtistStatsResponse struct {
	Releases            int `json:"releases"`
	Labels              int `json:"labels"`
	ShowsTracked        int `json:"shows_tracked"` // past + future
	SimilarArtists      int `json:"similar_artists"`
	FestivalAppearances int `json:"festival_appearances"`
}

// SocialResponse represents social media links
type SocialResponse struct {
	Instagram  *string `json:"instagram"`
	Facebook   *string `json:"facebook"`
	Twitter    *string `json:"twitter"`
	YouTube    *string `json:"youtube"`
	Spotify    *string `json:"spotify"`
	SoundCloud *string `json:"soundcloud"`
	Bandcamp   *string `json:"bandcamp"`
	Website    *string `json:"website"`
}

// ArtistWithShowCountResponse includes upcoming show count for an artist.
//
// LastShowDate is the most recent past approved show date for the artist.
// Only populated when the service runs in evergreen mode (e.g. tag-filtered
// /artists per PSY-495); stays nil on the default activity-gated path since
// the caller already knows there is at least one upcoming show.
type ArtistWithShowCountResponse struct {
	ArtistDetailResponse
	UpcomingShowCount int        `json:"upcoming_show_count"`
	LastShowDate      *time.Time `json:"last_show_date,omitempty"`
}

// ArtistListingEntry is an artist reduced to the two fields a link needs: the
// slug that builds the href, and the name that labels it.
//
// # Why a projection endpoint rather than a field on the list response
//
// `GET /artists` answers with sixteen fields per artist. The `/artists` page
// reads exactly two of them, to emit one JSON-LD `ItemList` entry per artist,
// and throws the other fourteen away. That is affordable until the response
// stops fitting a cache entry, at which point it is the whole problem.
//
// Measured against production on 2026-08-08, 6,279 artists. The `GET /artists`
// row is HISTORICAL: PSY-1774 gave that endpoint a real `limit` (default 50,
// max 200), so an unbounded response is no longer constructible and its cache
// entry is now sized by the page, not the catalogue.
//
//	                       raw bytes    base64      % of the 2 MB item cap
//	GET /artists           3,233,345    4,311,128   206%   (not cached; pre-PSY-1774)
//	GET /artists/listing     311,240      414,988    20%
//
// Vercel's Data Cache and Runtime Cache both cap a single item at 2 MB and
// "items larger won't be cached"; Next enforces the same cap and simply does
// not write the entry. It logs one console.warn while doing so — a signal, but
// one that fails nothing and scrolls past in a build log, which is why this
// went unnoticed for ten days. The body is stored base64-encoded, verified by
// decoding a real `.next/cache/fetch-cache` entry for this exact URL (1.334x),
// so the effective raw budget is ~1.5 MB. `GET /artists` crossed it between
// 2026-07-26 and 2026-07-29 and had been re-pulled from origin on every
// revalidation since.
//
// The projection is a 10.4x reduction. At ~50 bytes per entry that is ~32,000
// artists before the cap itself binds, against 6,279 today — but the number to
// plan against is ~25,400, where the build gate fires at 80% of the raw budget
// (lib/data-cache-budget). Trimming was chosen over sharding
// (the sitemap's answer, see SitemapEntry) and over paginating THIS endpoint
// because the fields, not the row count, are what blew the budget: sharding
// would keep carrying the fourteen unread fields and need its shard count
// revisited on every growth spurt, and truncating would silently drop URLs from
// the ItemList — the defect `/venues` exhibited at 100 of 297 until
// VenueListingEntry removed it the same way (PSY-1764).
//
// That argument is about the ItemList's feed, which needs the whole set in one
// read. It does NOT generalise to the browse list a human pages through, and
// PSY-1774 duly paginated `GET /artists`. Do not read this paragraph as a
// standing objection to pagination anywhere else.
//
// THAT PARAGRAPH IS ABOUT THIS ENDPOINT'S RESPONSE, NOT ABOUT THE ItemList, and
// PSY-1773 makes the distinction matter: the /artists page now slices this
// response to 100 entries before rendering its ItemList. The two decisions do
// not conflict, and neither reverses the other. Truncating the RESPONSE was
// rejected because it would not have fixed the cache budget without dropping
// URLs from a block that was, at the time, the only thing linking them.
// Truncating the RENDERED BLOCK was accepted for a different problem — page
// weight, since the entries were serialised into the HTML and again into the
// RSC flight payload — and is affordable because every artist URL is in the
// /sitemap/artists-b0.xml shard either way. So this endpoint still returns the
// whole set, deliberately: the projection is what keeps it cacheable, and the
// page decides how much of it to advertise. What that bound should be — and
// whether this endpoint should take a `limit` of its own now that its sibling
// has one — is PSY-1794.
//
// `/venues` answers that question differently and deliberately: its ItemList is
// NOT sliced, because the venue catalogue is two orders of magnitude smaller
// (297 against 6,279) and its rendered block is ~38 KB rather than ~0.7 MB. See
// VenueListingEntry, which records the measurement and the point at which page
// weight would force the same decision there.
//
// # Why not SitemapEntry
//
// It is {slug, updated_at} with no name, so reusing it would silently drop the
// label from every ItemList entry. It also covers a different set: every
// slugged artist (9,405) rather than the activity-gated browse set (6,279).
//
// # The set this returns
//
// The same GATE and ORDER as `GET /artists` — artists with at least one
// upcoming approved show, sorted upcoming-count DESC then name ASC then id ASC
// — minus rows with an empty slug, which cannot form a URL and which the page
// filtered out on its side anyway.
//
// The same gate, NOT the same rows: since PSY-1774 `GET /artists` answers with
// one page and this still answers with the whole set, so the two differ by two
// orders of magnitude in length while agreeing on membership. Keep the GATE in
// step — if the default gate on `GET /artists` changes, this changes with it,
// or the ItemList starts advertising a different set of artists than the page
// lists. `artistBrowseScope` owns that gate for the two paged reads; this
// endpoint restates it and is the copy that has to be kept honest by hand.
type ArtistListingEntry struct {
	Slug string `json:"slug" doc:"URL slug for the artist"`
	Name string `json:"name" doc:"Artist display name"`
}

// ArtistCityResponse represents a city with artist count for filtering
type ArtistCityResponse struct {
	City        string `json:"city"`
	State       string `json:"state"`
	ArtistCount int    `json:"artist_count"`
}

// ArtistLabelResponse represents a label associated with an artist
type ArtistLabelResponse struct {
	ID    uint    `json:"id"`
	Name  string  `json:"name"`
	Slug  string  `json:"slug"`
	City  *string `json:"city"`
	State *string `json:"state"`
}

// ArtistShowResponse represents a show in the artist shows endpoint
type ArtistShowResponse struct {
	ID uint `json:"id"`
	// Slug is the canonical /shows/{slug} target, matching ShowResponse.Slug.
	// Empty when the show has no slug, and clients fall back to the id.
	Slug      string    `json:"slug"`
	Title     string    `json:"title"`
	EventDate time.Time `json:"event_date"`
	// The advance/door pair. Served together for the reason stated on
	// VenueShowResponse: the advance half alone under-reports a split price
	// (PSY-1962).
	Price          *float64                 `json:"price"`
	DoorPrice      *float64                 `json:"door_price"`
	AgeRequirement *string                  `json:"age_requirement"`
	// Status flags, so an artist listing can strike through a cancelled date and
	// badge a sold-out one without a second fetch per row. Every producer of this
	// type must populate them: a default-false flag on a cancelled show is not a
	// missing field, it is a wrong one.
	IsCancelled bool                     `json:"is_cancelled"`
	IsSoldOut   bool                     `json:"is_sold_out"`
	Venue       *ArtistShowVenueResponse `json:"venue"`
	Artists     []ArtistShowArtist       `json:"artists"`
}

// ArtistShowsQuery carries the paging and filtering knobs for one artist's show
// list. The venue twin is VenueShowsQuery, and the two are deliberately
// identical: the artist and venue archives are the same reading surface pointed
// at a different entity, so a knob that exists on one and not the other is a
// bug report waiting to happen.
//
// A struct rather than positional arguments because Limit, Offset and Year are
// all ints: a transposed pair at a call site compiles cleanly and silently pages
// the wrong window.
type ArtistShowsQuery struct {
	// TimeFilter is "upcoming", "past" or "all". Any other value is treated as
	// "upcoming", matching shared.VenueLocalDateCondition and the handler's own
	// default for an omitted time_filter.
	TimeFilter string
	// Limit caps the page. Zero returns no rows while still reporting the full
	// total. That is long-standing behaviour, which the handler shields callers
	// from by defaulting an omitted limit to 20. Negative is clamped to zero
	// rather than passed through, because GORM reads a negative limit as "no
	// limit" and would return the artist's entire history.
	Limit int
	// Offset skips this many rows of the ordered page. Negative is clamped to 0.
	Offset int
	// Year narrows to a single VENUE-LOCAL calendar year, taken per show in its
	// own venue's zone rather than the artist's, and is reflected in the total.
	// Zero means every year.
	Year int
}

// ArtistShowYearCount is one bucket of an artist's show-year histogram: a
// venue-local calendar year and how many of the artist's shows fall in it.
// Only years with at least one show are emitted.
//
// A twin of VenueShowYearCount rather than one shared type. They serialise
// identically today, but they are two independent public response schemas over
// two different entities, and collapsing them would mean an artist-side field
// could only ever be added by also adding it to the venue payload.
type ArtistShowYearCount struct {
	Year  int   `json:"year" doc:"Venue-local calendar year"`
	Count int64 `json:"count" doc:"Shows the artist played in that year, within the requested time filter"`
}

// ArtistShowMonthCount is one bar of an artist's show histogram at MONTH
// resolution (PSY-1842), the artist twin of VenueShowMonthCount.
//
// The year is part of the bucket, not context around it: the archive it feeds
// spans an artist's whole history by default, where a bare month number would
// fold every March together.
//
// Each show is bucketed on ITS OWN venue's calendar, not on one zone for the
// whole artist — a touring artist's rows span venues, so a New Year's Eve date
// in Honolulu counts against the month it was played in, not the month it was in
// UTC. That is the same convention the year histogram and the list's year filter
// already use.
//
// It is NOT the axis the list is SORTED on, and a consumer mapping bucket
// ordinals onto row ordinals has to know that: the list orders by absolute
// instant, this orders by venue-local month, and for an artist those can
// disagree inside the ~1-day cross-zone band around a month boundary. See
// GetArtistShowMonths for the bound and the test that pins it.
//
// A twin of VenueShowMonthCount rather than one shared type, for the reason
// stated on ArtistShowYearCount: two independent public schemas over two
// entities.
type ArtistShowMonthCount struct {
	Year  int   `json:"year" doc:"Venue-local calendar year"`
	Month int   `json:"month" doc:"Venue-local calendar month, 1-12"`
	Count int64 `json:"count" doc:"Shows the artist played in that month, within the requested time filter"`
}

// ArtistShowVenueResponse represents venue info in artist show response
type ArtistShowVenueResponse struct {
	ID       uint    `json:"id"`
	Slug     string  `json:"slug"`
	Name     string  `json:"name"`
	City     string  `json:"city"`
	State    string  `json:"state"`
	Timezone *string `json:"timezone"` // IANA zone for rendering this show's time in venue-local time (PSY-985)
}

// ArtistShowArtist represents an artist on a show bill
type ArtistShowArtist struct {
	ID   uint   `json:"id"`
	Slug string `json:"slug"`
	Name string `json:"name"`
}

// ArtistAliasResponse represents an artist alias in API responses
type ArtistAliasResponse struct {
	ID        uint   `json:"id"`
	ArtistID  uint   `json:"artist_id"`
	Alias     string `json:"alias"`
	CreatedAt string `json:"created_at"`
}

// MergeArtistResult contains the outcome of merging two artists
type MergeArtistResult struct {
	CanonicalArtistID    uint   `json:"canonical_artist_id"`
	MergedArtistID       uint   `json:"merged_artist_id"`
	MergedArtistName     string `json:"merged_artist_name"`
	ShowsMoved           int64  `json:"shows_moved"`
	ReleasesMoved        int64  `json:"releases_moved"`
	LabelsMoved          int64  `json:"labels_moved"`
	FestivalsMoved       int64  `json:"festivals_moved"`
	RelationshipsMoved   int64  `json:"relationships_moved"`
	BookmarksMoved       int64  `json:"bookmarks_moved"`
	CollectionItemsMoved int64  `json:"crate_items_moved"`
	FiltersUpdated       int64  `json:"filters_updated"`
	AliasCreated         bool   `json:"alias_created"`
}

// ──────────────────────────────────────────────
// Scene types (computed city aggregations)
// ──────────────────────────────────────────────

// SceneListResponse represents a scene in the list endpoint. Under metro keying
// (PSY-1255 step C) City/State are the metro's PRINCIPAL city/state (or the
// literal city for a non-US / no-CBSA fallback scene).
type SceneListResponse struct {
	City              string `json:"city"`
	State             string `json:"state"`
	Slug              string `json:"slug"`
	VenueCount        int    `json:"venue_count"`
	UpcomingShowCount int    `json:"upcoming_show_count"`
	TotalShowCount    int    `json:"total_show_count"`
	// ShowsThisWeek is the ≤7-day slice of UpcomingShowCount (PSY-1309) — the
	// next-7-days activity signal that drives the Atlas globe's pulse
	// treatment. Same scene scoping as the other counts.
	//
	// A ROLLING window: [now, now+7d) in UTC. It is NOT the week any
	// /scenes/{slug}/week page serves, and the two disagree by a lot midweek —
	// use ShowsCalendarWeek for anything that LINKS to that page.
	ShowsThisWeek int `json:"shows_this_week"`
	// ShowsCalendarWeek is the count of shows the scene's
	// /scenes/{slug}/week page reports for the CURRENT week: the Monday-Sunday
	// window resolved in the scene's OWN venue timezone, not a rolling
	// seven days and not the caller's zone.
	//
	// It exists because a surface that shows a number NEXT TO a link to that
	// page has to show that page's number. Chicago read 76 from ShowsThisWeek
	// and 96 on its week page on 2026-08-02; a reader following the link would
	// have caught the site lying. Both fields ship because they answer
	// different questions — the Atlas pulse wants "is anything on soon", a
	// week link wants "how many are on that page".
	//
	// COUNTED OVER A DIFFERENT VENUE POPULATION from every other field here:
	// the week page includes unverified venues, and this matches it, while
	// VenueCount/TotalShowCount/UpcomingShowCount/ShowsThisWeek are
	// verified-only. A scene with unverified rooms can read "3 shows" and
	// "17 shows this week" on one card. That is the cost of the link telling
	// the truth; see sceneCalendarWeekCounts before "fixing" it.
	ShowsCalendarWeek int `json:"shows_calendar_week"`
	// Latitude/Longitude position the scene on the geographic-discovery map
	// (PSY-1212): the metro principal city's centroid (or the fallback city's,
	// geocoded the same way as ShowCityResponse — PSY-985/PSY-981), so a scene
	// plots at the same point here and on the shows-by-city map. Omitted (nil) on
	// a geocoder miss; the scene still lists, just unplaceable.
	Latitude  *float64 `json:"latitude,omitempty"`
	Longitude *float64 `json:"longitude,omitempty"`
	// DominantGenre is the scene's dominant genre-family key (PSY-1315) when one
	// family holds a confident share (>= 40%) of the roster's genre mass, else ""
	// (omitted). The Atlas globe tints the scene's dot by this family; "" keeps the
	// default orange. Family keys are owned by the catalog service's genre-family
	// map and mirrored by the frontend's GENRE_FAMILIES.
	DominantGenre string `json:"dominant_genre,omitempty"`
}

// SceneShowArtist is one band on a scene show's bill, reduced to the two fields
// a link needs. See SceneShowSummary.Artists for the empty-slug contract.
type SceneShowArtist struct {
	Name string `json:"name" doc:"Artist display name"`
	Slug string `json:"slug" doc:"URL slug for the artist; \"\" when the artist has no slug — render unlinked"`
}

// SceneShowSummary is one upcoming show in a scene's "Next 7 days" preview row
// (PSY-1309) — deliberately thin (the Atlas preview panel needs a line, not the
// full ShowResponse payload). VenueName is the first venue on the bill.
//
// Also the row shape behind the scene digest email's "Next 7 days" section, so
// the email and the panel can never disagree about which shows are in scope.
type SceneShowSummary struct {
	ID        uint   `json:"id"`
	Slug      string `json:"slug,omitempty"` // canonical /shows/{slug} target; "" when the show has no slug (clients fall back to the id)
	Title     string `json:"title"`
	EventDate string `json:"event_date"` // ISO date (YYYY-MM-DD)
	VenueName string `json:"venue_name,omitempty"`
	// Bill artists in position order (PSY-1325). Most shows have an empty
	// Title — display names are composed from artists everywhere else in the
	// app — so without these the preview row carries no band info at all.
	//
	// Names only, so a bill rendered from this field cannot link. Superseded by
	// Artists below and kept for the surfaces that still read it (JSON-LD
	// performers, the digest email's plain-text bill, chart-style meta lines):
	// those want a name list and would gain nothing from the pairs. Both fields
	// are built from the SAME resolved bill in the same order, so they cannot
	// disagree.
	ArtistNames []string `json:"artist_names,omitempty"`
	// Artists is the same bill as ArtistNames, in the same order, carrying the
	// slug each band needs to link to its own page — the linked-bill row grammar
	// every scene surface renders (PSY-1846).
	//
	// A band with no slug is present here with Slug "" and MUST be rendered as
	// plain text: `/artists/` + "" resolves to the artists INDEX, not a 404
	// (PSY-1754). Omitting it instead would silently shorten a titleless show's
	// display name, which is composed from this bill.
	//
	// Deliberately scene-local. ArtistListingEntry is the closest field match
	// but the WRONG contract — its endpoint gates OUT empty-slug rows, which
	// this field must carry. ArtistShowArtist is the closest semantic match
	// ("an artist on a show bill") and would also work, but it carries an ID
	// this deliberately-thin summary has no use for. ChartEntityReference's own
	// doc warns against sharing a struct merely because the wire fields overlap
	// today, and that reasoning applies here too.
	Artists []SceneShowArtist `json:"artists,omitempty"`
	// Status flags, needed by any surface that LISTS shows rather than linking
	// to one. A weekly city page that renders a cancelled show identically to a
	// live one is worse than omitting it, and the sold-out badge is part of the
	// blessed design — without these the client can only guess.
	IsSoldOut   bool `json:"is_sold_out"`
	IsCancelled bool `json:"is_cancelled"`

	// StartsAt is the show's absolute start instant, in UTC.
	//
	// EventDate above is a scene-local CALENDAR date and cannot be turned back
	// into an instant: re-parsing "2026-07-27" yields UTC midnight, which is the
	// previous evening anywhere west of Greenwich. Any consumer that needs a real
	// timestamp — structured-data `startDate`, a calendar export — must use this
	// and render it in the venue's own zone.
	StartsAt time.Time `json:"starts_at"`
	// The advance/door price pair (PSY-1962). Price is the ADVANCE half on rows
	// that also record a door price; it was described as the "door price" before
	// shows.door_price existed, and it never was one.
	//
	// Both halves are served because a scene day list carrying only the advance
	// price under-reports what the site knows: a reader scanning the night saw
	// $35 and turned up to a $40 door.
	//
	// NO currency is recorded anywhere in the schema — both columns are bare
	// numerics — so a consumer that needs one has to assume, and for a non-US
	// scene that assumption is wrong. Do not add a currency here without adding
	// the column.
	//
	// `omitempty` on a pointer omits only a nil, so an unrecorded price is
	// absent from the payload and a FREE show still serializes as 0.
	Price     *float64 `json:"price,omitempty"`
	DoorPrice *float64 `json:"door_price,omitempty"`

	// AgeRequirement is this event's own door policy, free text in the site-wide
	// vocabulary ("all ages", "17+", "21+"). It is the per-event OVERRIDE and
	// wins wherever it is present; VenueAgePolicy below is the room's house
	// default and the value a show without an override falls back to (PSY-1682).
	//
	// Both halves are served because either one alone answers the reader's
	// question wrongly on a real slice of rows: most shows record no override at
	// all, so an override-only field leaves the column blank on a night whose
	// rooms all have a published policy, and a house-default-only field states
	// 21+ for the all-ages matinee that room is running tonight.
	AgeRequirement string `json:"age_requirement,omitempty"`

	// The billed venue's own details, from the SAME venue row VenueName names —
	// enough to describe a place without a second round-trip per show.
	//
	// VenueAddress follows the site-wide privacy gate: street addresses are
	// served for VERIFIED venues only, so a DIY/house venue is never published
	// before human review. The remaining fields are city-level and always safe.
	VenueSlug     string `json:"venue_slug,omitempty"`
	VenueAddress  string `json:"venue_address,omitempty"`
	VenueCity     string `json:"venue_city,omitempty"`
	VenueState    string `json:"venue_state,omitempty"`
	VenueCountry  string `json:"venue_country,omitempty"`
	VenueTimezone string `json:"venue_timezone,omitempty"`
	// VenueAgePolicy is the room's HOUSE DEFAULT age rule, served unredacted for
	// unverified venues like the venue payload's own copy of it. See
	// AgeRequirement above for how the two combine.
	VenueAgePolicy string `json:"venue_age_policy,omitempty"`
}

// SceneWeekDay is one calendar day of a scene's week, in the scene's own
// timezone. Days with no shows are still emitted so the page can render the
// full week rather than silently collapsing quiet nights.
type SceneWeekDay struct {
	Date  string             `json:"date"` // ISO date (YYYY-MM-DD), scene-local
	Shows []SceneShowSummary `json:"shows"`
}

// SceneWeekResponse is one ISO week of a scene's shows — the payload behind the
// public weekly city page.
//
// The week is computed in the scene's own timezone, not UTC: a 21:00 Sunday
// show in Chicago is 02:00 Monday UTC, so a UTC week boundary would file it
// into the following week.
//
// TrackedVenues is not decoration. Coverage is a curated slice of each city's
// rooms, not an exhaustive listing, so the page must name the rooms it draws
// from rather than implying it lists everything happening in the city.
//
// Timezone is NULL under the rule SceneDayResponse states in full.
type SceneWeekResponse struct {
	Slug      string  `json:"slug"`
	SceneName string  `json:"scene_name"` // "City, ST"
	City      string  `json:"city"`
	State     string  `json:"state"`
	ISOWeek   string  `json:"iso_week"`   // "2026-W31"
	StartDate string  `json:"start_date"` // Monday, scene-local ISO date
	EndDate   string  `json:"end_date"`   // Sunday, scene-local ISO date
	Timezone  *string `json:"timezone"`   // IANA zone the week was computed in; NULL when unresolved
	ShowCount int     `json:"show_count"`
	// PrevWeek/NextWeek are ISO week keys for adjacent-week navigation.
	PrevWeek string `json:"prev_week"`
	NextWeek string `json:"next_week"`
	// IsCurrentWeek lets the client mark the rolling week without recomputing
	// the scene-local "now" itself (and getting a different answer).
	IsCurrentWeek bool `json:"is_current_week"`
	// IsPastWeek says the week is over and can no longer gain shows — the only
	// state in which a client may cache this payload hard.
	//
	// Deliberately NOT the negation of IsCurrentWeek: a FUTURE week is neither
	// current nor past, and it is the one a client must never freeze, because
	// the "next week" link is on every page and gets followed days before that
	// week goes live. Answered here because the boundary depends on the SCENE's
	// timezone; a client comparing dates in UTC would call a week over up to a
	// day early.
	IsPastWeek    bool                `json:"is_past_week"`
	Days          []SceneWeekDay      `json:"days"`
	TrackedVenues []SceneTrackedVenue `json:"tracked_venues"`
}

// SceneTrackedVenue is one room a scene draws from, with enough to LINK it.
//
// Day and week payloads both send this type so the rooms footer can link to
// /venues/{slug} when a slug is on file. Website is served from the venue
// row's own column — the venue LIST projections omit it, which is why this
// cannot be assembled client-side from an existing endpoint. The footer does
// not use website as an href.
type SceneTrackedVenue struct {
	Name    string `json:"name"`
	Slug    string `json:"slug,omitempty"`    // "" when the venue has no slug; clients then render it unlinked
	Website string `json:"website,omitempty"` // "" when none is on file
}

// SceneVenueSummary is one room on the scene page's rooms leaderboard: the same
// tracked-venue set SceneTrackedVenue describes, ranked by how much is coming up
// in it.
//
// A separate type from SceneTrackedVenue rather than extra fields on it: the
// day and week footers read a linkable list and need none of the ranking
// fields, and widening one wire format to serve a different page churns every
// consumer of the first. The two types must agree on the SET of rooms, which
// trackedVenuePredicate enforces; they are free to disagree on shape.
type SceneVenueSummary struct {
	// ID is the venue row's id, and it is not decoration: venue names are unique
	// only WITHIN a city (idx_venues_name_city_unique) and a metro spans several,
	// so two rooms on one leaderboard can share a name. Without an id, two
	// same-named rooms with no slug arrive as byte-identical objects that a
	// client cannot tell apart, link, or key a list on.
	ID      uint   `json:"id"`
	Name    string `json:"name"`
	Slug    string `json:"slug,omitempty"`    // "" when the venue has no slug; clients then render it unlinked
	Website string `json:"website,omitempty"` // "" when none is on file
	// City is the room's OWN city, which for a metro scene is often not the
	// scene's principal city — a Tempe room belongs to the Phoenix scene. The
	// leaderboard needs it to say where a reader would actually be going; the
	// scene's own City field cannot answer that.
	//
	// Always sent, unlike Slug/Website: venues.city is NOT NULL, so the field is
	// never missing and an optional one would push a needless fallback into
	// every consumer. It can still be the EMPTY string — NOT NULL does not
	// forbid '' — so a client must render around a blank, just not around an
	// absent key.
	City string `json:"city"`
	// State rides along with City for the same reason City rides along at all: a
	// metro is not confined to one of them. Philadelphia's spans into Camden NJ
	// and New York's into New Jersey, so a city-only row renders a bare "Camden"
	// on a Philadelphia page. Same pair the venue charts carry (ActiveVenue).
	State string `json:"state"`
	// UpcomingShowCount is the room's approved shows still to come, counted from
	// the START of the current UTC day — see sceneVenueLeaderboard for why an
	// instant bound would zero out a room whose only show is tonight.
	//
	// NOT a partition of SceneStats.UpcomingShowCount, and it misses in EVERY
	// direction, all three on purpose:
	//   - PAST it, because a show billed to two rooms is counted by both, and
	//     because today's shows are inside this bound and outside that one.
	//   - SHORT of it, because the scene total counts shows at UNVERIFIED rooms,
	//     which are not tracked and so have no row here.
	// Do not "reconcile" the two by editing the scene total: that silently
	// changes a number already being served.
	//
	// CANCELLED shows are counted, because they are still `approved` — the same
	// convention the scene total and the day/week payloads follow (those ship
	// is_cancelled per show so a reader sees the strike-through). The charts
	// exclude them when RANKING; that this ranks without excluding them is a
	// deliberate match to the scene total it sits beside, not an oversight.
	//
	// Zero is a real, kept value — a tracked room with nothing booked is still a
	// room the scene covers, and dropping it would quietly redefine the list as
	// "rooms with shows".
	UpcomingShowCount int `json:"upcoming_show_count"`
}

// SceneDayResponse is ONE calendar day of a scene's shows — the payload behind
// the public "tonight" page and its dated permalink.
//
// The day is computed in the scene's own timezone for the same reason the week
// is: a 21:00 show in Chicago is 02:00 the NEXT day in UTC, so a UTC boundary
// would file it under the wrong date.
//
// Timezone is NULL when the scene has no zone the site knows: no verified room
// carries one and the state is outside the US map. The Date above is still
// served, computed on the Arizona default, because a fallback DAY is the best
// available answer; the null zone is what tells a client not to build an HOUR on
// the same guess.
type SceneDayResponse struct {
	Slug      string  `json:"slug"`
	SceneName string  `json:"scene_name"` // "City, ST"
	City      string  `json:"city"`
	State     string  `json:"state"`
	Date      string  `json:"date"`     // scene-local ISO date (YYYY-MM-DD)
	Timezone  *string `json:"timezone"` // IANA zone the day was computed in; NULL when unresolved
	// ISOWeek is the week this day belongs to, so a client can link the week
	// view without redoing ISO calendar maths (which is subtle enough that the
	// two would eventually disagree at a year boundary).
	ISOWeek   string `json:"iso_week"`
	ShowCount int    `json:"show_count"`
	// PrevDate/NextDate are the adjacent calendar dates, for day-at-a-time
	// navigation through the dated permalinks. EMPTY at the edges of the
	// servable window — a client must render the control only when the field
	// is set, or it advertises a link this same service answers 404 to.
	PrevDate string `json:"prev_date"`
	NextDate string `json:"next_date"`
	// IsTonight says this date is the one a reader standing in the scene right
	// now would call "tonight" — which is NOT simply today's date. Until 06:00
	// scene-local the answer is still YESTERDAY's date, because a night is named
	// by the date it BEGAN on (the same 6am broadcast-day boundary the radio
	// schedule uses). Answered here because it depends on the SCENE's clock,
	// not the viewer's. It does not widen the day's window — see Shows.
	//
	// Computed on the fallback clock when Timezone is NULL, and NOT withheld
	// there. It is a same-day claim about a DATE, and this payload publishes the
	// date on that same fallback; withholding one and not the other would leave
	// a client a date it cannot label.
	IsTonight bool `json:"is_tonight"`
	// IsPastDay says the day is over and can no longer gain shows — the only
	// state in which a client may cache this payload hard.
	//
	// Deliberately not the negation of IsTonight, in BOTH directions. A FUTURE
	// date is neither. And between midnight and 06:00 the scene's clock is past
	// the date's end while IsTonight still points at it, so the two would
	// otherwise both be true and a client would freeze the live night for a day
	// under a heading reading "Tonight". Never both.
	IsPastDay bool `json:"is_past_day"`
	// Shows for this day, earliest first. Always non-nil, so a quiet night
	// marshals as `[]` rather than `null`.
	Shows         []SceneShowSummary  `json:"shows"`
	TrackedVenues []SceneTrackedVenue `json:"tracked_venues"`
	// NextShow is the scene's next show AFTER this day — a quiet night's whole
	// job is to point somewhere useful, and the alternative is a second
	// round-trip on exactly the page with the least to say.
	//
	// Populated only when the day is empty AND has not already happened.
	// Absent when the day HAS shows; absent when the scene has nothing upcoming
	// at all, which is what separates a quiet night from a dead-quiet one; and
	// absent on any past date, where "next" could only mean a show that is
	// itself long over, and where IsPastDay invites the client to freeze this
	// payload for a day it would not stay true for.
	NextShow *SceneShowSummary `json:"next_show,omitempty"`
}

// ShowAlsoTonightResponse is the "also tonight" rail on a show page: the OTHER
// shows in the same metro on the same date.
//
// A wrapper rather than a bare []SceneShowSummary because the rail's heading is
// part of the answer. The mock reads "ALSO / TONIGHT · CHICAGO", and neither
// half of that is derivable from the show payload the page already has: the
// metro's display name is not the venue's city (a Mesa show belongs to the
// Phoenix scene), and the date is a SCENE-LOCAL calendar date that a client
// re-deriving it from the show's UTC instant would get wrong for any late set.
// Serving both here also keeps the rail to one request.
type ShowAlsoTonightResponse struct {
	// SceneName/City/State are the scene's DISPLAY identity: the metro's
	// principal city, so the heading reads "Chicago" for an Evanston room.
	//
	// EMPTY whenever there is no scene to scope by, which is THREE cases and not
	// one: the show has no venue, its venue has no usable city/state, or its place
	// is below the scene threshold. Shows is empty in all three.
	SceneName string `json:"scene_name,omitempty"` // "City, ST"
	City      string `json:"city,omitempty"`
	State     string `json:"state,omitempty"`
	// SceneSlug is the "see all" LINK target, to be combined with Date as the
	// reader-facing /scenes/{slug}/{date} (a single date segment; the API's own
	// /scenes/{slug}/day/{date} is a different path).
	//
	// Present only when following it lands somewhere honest, which is why it is
	// separate from the display identity above. It is withheld when the
	// scene-day surface would refuse the date (it serves a bounded window of
	// years, so an archive show has a real scene and a real date but no page), and
	// when the subject show is not itself in the metro's night — the rail's scope
	// is resolved from the geocoder while the rows come from the venues.metro
	// column, so a room the metro backfill never reached would otherwise be sent
	// to a page that provably does not list the show it came from. Same contract
	// as SceneDayResponse.PrevDate/NextDate: render the control only when the
	// field is set.
	SceneSlug string `json:"scene_slug,omitempty"`
	// Date is the show's own calendar date (YYYY-MM-DD): the date the rail is
	// about, and the day key SceneSlug pairs with.
	//
	// The show's OWN date, never the viewer's "tonight": a reader in Berlin
	// opening a Chicago show page is asking what else is on that night in
	// Chicago. Read on the venue's own zone when it has one, and on the metro's
	// modal clock otherwise.
	//
	// A strict calendar date, so a 00:30 set files under the date it starts on.
	// "Tonight" is a different question, answered by IsTonight.
	Date string `json:"date"`
	// Timezone is the IANA zone the date and its window were computed in, NULL
	// when neither the subject room nor the metro names one. Same rule
	// SceneDayResponse states in full.
	Timezone *string `json:"timezone"`
	// IsTonight says this date is the one a reader standing in the scene right
	// now would call "tonight" — which is NOT simply Date == today. Until 06:00
	// local the answer is still YESTERDAY's date, because a night is named by the
	// date it BEGAN on. Answered here, exactly as SceneDayResponse does, because
	// it depends on the scene's clock rather than the viewer's device: a client
	// computing it locally would have to reimplement the 6am rule and would get a
	// different answer for a reader in another zone.
	IsTonight bool `json:"is_tonight"`
	// ShowCount is len(Shows): the number of rows served, which is the capped
	// count and not the metro's true total for the night. Read it with HasMore.
	ShowCount int `json:"show_count"`
	// HasMore says the night held more shows than the rail can carry, so a client
	// can say so rather than implying it listed everything. Note the cap keeps the
	// EARLIEST rows of the date, so on a dense night the dropped shows are the
	// late ones; a client that needs the full night follows SceneSlug + Date.
	HasMore bool `json:"has_more"`
	// Shows excludes the subject show and is capped, earliest first. Always
	// non-nil, so a quiet night marshals as `[]` rather than `null`.
	Shows []SceneShowSummary `json:"shows"`
}

// ShowTimelineEntry is one date on a show timeline: enough to print a line,
// link to it, and read its date on the right clock.
//
// Timezone is RESOLVED, never the venue's raw nullable column: each entry names
// a different room, so a client that received the column would have to redo the
// timezone-then-state fallback per entry to avoid printing a Berlin date on a
// Phoenix clock. The server already holds both inputs, so it answers once
// (shared.EventZone).
//
// NULL when neither input answers, under the rule SceneDayResponse states in
// full. That is a refusal, not a gap the client should fill: a client renders
// the DATE and no clock.
//
// VenueSlug and ShowSlug can be empty: both columns are nullable, and a client
// must render an empty slug as unlinked text rather than building `/venues/`,
// which resolves to the INDEX rather than a 404.
type ShowTimelineEntry struct {
	ShowID    uint      `json:"show_id"`
	ShowSlug  string    `json:"show_slug"`
	EventDate time.Time `json:"event_date"`
	VenueName string    `json:"venue_name"`
	VenueSlug string    `json:"venue_slug"`
	City      string    `json:"city"`
	State     string    `json:"state"`
	Timezone  *string   `json:"timezone"`
}

// ShowTimelineRecurrence is what this show's place already knows about one act
// on its bill.
//
// Carries the artist ID and no name: the caller is rendering a bill it already
// holds, and a second copy of the name here would be a second source of truth
// for it.
//
// Only acts with something to say are emitted, so an entry always has
// IsHometown set, a LastPlayed, or both.
type ShowTimelineRecurrence struct {
	ArtistID uint `json:"artist_id"`
	// IsHometown says the act is BASED in this show's place — the metro when
	// both sides have one, else a city+state match. Independent of LastPlayed:
	// a hometown band with no prior date on record sets this and nothing else.
	IsHometown bool `json:"is_hometown"`
	// LastPlayed is the act's most recent APPROVED date in this show's place,
	// strictly before this show. Nil when the archive holds none.
	LastPlayed *ShowTimelineEntry `json:"last_played"`
}

// ShowTimelineResponse is the archive read behind the show page's two corridor
// modules: the headliner's adjacent dates, and per-act recurrence in this
// show's place.
//
// Its own endpoint rather than fields on ShowResponse because only the show
// PAGE renders it, while the detail read also feeds share-card generation,
// JSON-LD and the edit form — surfaces that would pay for the archive queries
// and discard them.
//
// "Adjacent" and "before" are decided on the ABSOLUTE INSTANT, never on a
// venue-local calendar day: the neighbours are at other rooms in other zones,
// so a per-row local date gives no consistent ordering. Local dates are a
// DISPLAY concern, which is what each entry's Timezone is for.
//
// Every field is independently absent-able. A show whose headliner tours alone
// has no Previous or Next; a show in a place nobody on the bill has played has
// an empty Recurrence. Each module hides itself on its own evidence.
type ShowTimelineResponse struct {
	// HeadlinerArtistID is the act Previous/Next were resolved for, so a client
	// can label the spine and confirm it matches the name it printed as the
	// lead. Zero when the show has no bill.
	//
	// Resolved by the DISPLAY rule (curated 'headliner' first, then lowest bill
	// position), which is what ShowResponse.Artists carries and therefore what
	// the page's own heading resolves from. Deliberately NOT headlineSlotSQL:
	// that predicate CLASSIFIES rows for charts and returns no headline slot at
	// all on a partially curated bill, which would blank the spine on a show
	// whose heading names a headliner perfectly well.
	HeadlinerArtistID uint `json:"headliner_artist_id"`
	// Previous and Next are the headliner's own adjacent approved dates.
	// Non-approved shows are invisible here exactly as they are everywhere
	// else, so a pending date never leaks into a public spine.
	Previous *ShowTimelineEntry `json:"previous"`
	Next     *ShowTimelineEntry `json:"next"`
	// Recurrence is in BILL ORDER, headliner first, and always non-nil so a
	// bill with no history marshals as `[]` rather than `null`.
	Recurrence []ShowTimelineRecurrence `json:"recurrence"`
}

// SceneNewArtist is one "new band based here" row for the weekly scene digest
// (PSY-1342) — just enough to render a linked name, plus the moment the band
// entered the catalog.
type SceneNewArtist struct {
	ID   uint   `json:"id"`
	Slug string `json:"slug,omitempty"`
	Name string `json:"name"`
	// FirstListedAt is the band's catalog created_at — the same instant the
	// window is tested against, so a row can always state the date that put it
	// in the window. The digest ignores it; the scene page's new-bands module
	// renders it ("first listed Aug 10").
	FirstListedAt time.Time `json:"first_listed_at"`
}

// SceneNewArtistShow is the one show attached to a new-band row: the band's
// soonest UPCOMING approved show, or — when it has none — its most recent past
// one, which is why IsUpcoming exists. Any approved show counts, including one
// outside the scene: a band based here that is currently on tour still answers
// "where can I see them", and the venue fields say where.
type SceneNewArtistShow struct {
	ID   uint   `json:"id"`
	Slug string `json:"slug,omitempty"` // canonical /shows/{slug} target; "" → clients fall back to the id
	// EventDate is the calendar date at the VENUE (its own zone, UTC when
	// unknown). StartsAt is the same show as an absolute instant — EventDate
	// cannot be parsed back into one (see SceneShowSummary).
	EventDate string    `json:"event_date"`
	StartsAt  time.Time `json:"starts_at"`
	VenueName string    `json:"venue_name,omitempty"`
	VenueSlug string    `json:"venue_slug,omitempty"`
	// IsUpcoming compares EventDate against today ON THE VENUE'S OWN CALENDAR,
	// so a show graduates to past at venue-local midnight rather than at its
	// start instant — the site-wide listing boundary (shared.VenueLocalTodaySQL).
	// A show in progress therefore still reads as upcoming, and the flag can
	// never contradict the EventDate printed beside it.
	IsUpcoming bool `json:"is_upcoming"`
}

// SceneNewArtistRow is one row of the scene page's latest-additions module
// (PSY-1781, redefined by PSY-1844): a SceneNewArtist plus the show that makes
// the name actionable.
//
// The row TYPE is still shared with the weekly digest; the SELECTION no longer
// is. The module takes the roster's most recently listed bands with no window,
// the digest takes the bands listed inside its cursor window, and PSY-1844
// separated the two queries on purpose. FirstListedAt therefore still states
// the fact the module ordered on, which is the property that keeps the pulse's
// first-approved-show definition out of this payload.
type SceneNewArtistRow struct {
	SceneNewArtist
	// Show is absent when the band has no approved show at all — a real state
	// for a band added by an enrichment pass before its first booking lands.
	// `omitempty` is load-bearing: without it huma publishes the field as
	// REQUIRED and non-nullable while the wire sends `null`, and the generated
	// client type would promise an object that isn't there (the same shape
	// SceneVenueSummary.NextShow and ArtistGraphCardResponse.NextShow use).
	Show *SceneNewArtistShow `json:"show,omitempty"`
}

// SceneCollectionSummary is one public collection on a scene's "Collections ·
// {city}" rail. Collections carry no scene column, so a collection's relevance
// to a scene is DERIVED from how many of its members are scene-local — artists
// based in the scene's metro, venues in the scene, shows at scene venues (see
// SceneServiceInterface.GetSceneCollections for the qualifying rule).
//
// SceneLocalItemCount ships alongside ItemCount deliberately. The rail's claim
// is "this collection is about your city", and that claim is only checkable
// against the whole: "12 of 14" earns the slot, "5 of 200" would not have.
// Publishing the numerator without the denominator would make the rule
// unauditable from the payload it produced.
type SceneCollectionSummary struct {
	ID    uint   `json:"id"`
	Slug  string `json:"slug"`
	Title string `json:"title"`
	// CoverImageURL is copied straight off collections.cover_image_url — the
	// same, and only, cover field every other collection payload carries. Nil
	// when the curator never set one; the rail supplies its own fallback. No
	// new cover derivation is introduced here on purpose: a second definition
	// of "a collection's cover" would drift from the collection cards the
	// reader sees everywhere else.
	CoverImageURL *string `json:"cover_image_url"`
	// SceneLocalItemCount is the rule's numerator and the primary sort key.
	SceneLocalItemCount int `json:"scene_local_item_count"`
	// ItemCount is EVERY member, including entity types that can never be
	// scene-local (releases, labels, festivals) — the rule's denominator.
	ItemCount int `json:"item_count"`
	// ContributorCount is the rail's "Built by N": DISTINCT
	// collection_items.added_by_user_id, the same live derivation
	// CollectionService.batchCountContributors publishes. There is no builders
	// table and no counter column to read instead.
	ContributorCount int `json:"contributor_count"`
	// UpdatedAt is collections.updated_at, which moves on collection-ROW edits
	// only — CollectionService.AddItem does NOT touch it. It is therefore a
	// metadata-edit timestamp, not a curation-activity signal, which is why it
	// is published for display but is NOT the ranking tiebreak (see
	// GetSceneCollections).
	UpdatedAt time.Time `json:"updated_at"`
}

// SceneDetailResponse represents the full computed scene for a metro (or a
// no-CBSA fallback city); City/State are the principal city/state.
type SceneDetailResponse struct {
	City        string     `json:"city"`
	State       string     `json:"state"`
	Slug        string     `json:"slug"`
	Description *string    `json:"description"` // nil until scenes table exists
	// Tagline is the authored line the scene page renders under its H1, and the
	// route's og:description when set (PSY-1848). nil is the ordinary state —
	// the page renders NOTHING for it rather than falling back to Description,
	// which is prose the page does not show.
	Tagline *string    `json:"tagline"`
	Stats   SceneStats `json:"stats"`
	Pulse       ScenePulse `json:"pulse"`
	// Venues is the scene's tracked rooms, busiest first — the page's rooms
	// leaderboard. The SAME set the day payload names (verified rooms in the
	// scene's scope), so the two surfaces cannot claim to cover different rooms
	// for one city, and bounded by it: this is a leaderboard over a curated
	// list, not a directory of everywhere in the metro.
	//
	// Always non-nil, so a scene marshals `[]` rather than `null`. The generated
	// OpenAPI schema still types it nullable — every Go slice does — so that is
	// a generator artifact, not a shape the wire can actually take.
	Venues []SceneVenueSummary `json:"venues"`
}

// SceneStats holds aggregate counts for a scene
type SceneStats struct {
	VenueCount        int `json:"venue_count"`
	ArtistCount       int `json:"artist_count"`
	UpcomingShowCount int `json:"upcoming_show_count"`
	FestivalCount     int `json:"festival_count"`
}

// ScenePulse holds activity trend data for a scene
type ScenePulse struct {
	ShowsThisMonth        int    `json:"shows_this_month"`
	ShowsPrevMonth        int    `json:"shows_prev_month"`
	ShowsTrend            string `json:"shows_trend"`
	NewArtists30d         int    `json:"new_artists_30d"`
	ActiveVenuesThisMonth int    `json:"active_venues_this_month"`
	ShowsByMonth          []int  `json:"shows_by_month"` // last 6 months
}

// SceneGapsResponse holds a scene's completeness gaps — the counts behind the
// scene page's one-line "help finish this scene" prompt (PSY-1845). Gaps are
// published as CONTENT, with live numbers, so a reader sees exactly how much
// work is left rather than a generic call to contribute.
//
// Counts only, deliberately: the entity lists behind them are separate,
// pageable surfaces. Nothing here is per-request personal, so the payload is
// cacheable and the numbers may be minutes stale.
//
// The gap SET is locked at these two. Do not widen it without a product call —
// the value of the prompt is that it names one concrete, finishable task, and
// each extra number dilutes that. New gap types get their own field and their
// own decision, never a silent redefinition of these.
type SceneGapsResponse struct {
	City  string `json:"city"`
	State string `json:"state"`
	Slug  string `json:"slug"`
	// ArtistsMissingListenLink counts bands BASED in the scene (the same roster
	// scope as SceneStats.ArtistCount) with no music-platform link at all —
	// none of spotify / bandcamp / youtube / soundcloud. That four-column set
	// is the project's existing definition of a music-platform link, shared
	// with the streaming-discovery worklist; instagram / facebook / twitter /
	// website are NOT listen links and do not clear this gap.
	ArtistsMissingListenLink int `json:"artists_missing_listen_link"`
	// ArtistsOnBillsMissingLocation counts bands that have played the scene's
	// rooms (any approved show, any date) but carry no home city AND no home
	// state — so the roster predicate can never claim them as based here, no
	// matter how many local shows they play. Filling a location is what moves a
	// band from this number into ArtistCount.
	//
	// Note the two counts are over DIFFERENT populations (based-here vs
	// played-here) and can overlap or not at all; they are not parts of a whole
	// and must never be summed into a single "gaps" figure.
	ArtistsOnBillsMissingLocation int `json:"artists_on_bills_missing_location"`
}

// SceneArtistResponse represents an artist in a scene's roster. Under the
// metro-keyed model (PSY-1255 step C) the roster is every band BASED in the
// metro; IsActive flags the ones with an upcoming show or one in the active
// window (played anywhere), which the frontend highlights.
type SceneArtistResponse struct {
	ID        uint    `json:"id"`
	Slug      string  `json:"slug"`
	Name      string  `json:"name"`
	City      *string `json:"city"`
	State     *string `json:"state"`
	ShowCount int     `json:"show_count"`
	IsActive  bool    `json:"is_active"`
	// BandcampEmbedURL is the artist's embeddable Bandcamp /album|/track URL
	// (artists.bandcamp_embed_url, PSY-1187/1188/1189), nil when the artist has
	// none. The /atlas preview's player now uses the scene-level
	// RepresentativeEmbed (PSY-1294) rather than scanning this per-artist field,
	// but it stays on the roster payload.
	BandcampEmbedURL *string `json:"bandcamp_embed_url"`
}

// SceneRepresentativeEmbed identifies the single band whose Bandcamp embed the
// /atlas scene preview plays as the scene's "instant payoff" (PSY-1294). Unlike
// the client-side pick it replaced, it is computed over the FULL metro roster
// (not the fetched page), so a scene's only embed-having band can't silently
// fall below the fetched window. Active bands are preferred (active-first
// ordering); a dormant band is the fallback when no active band has an embed.
type SceneRepresentativeEmbed struct {
	EmbedURL   string `json:"embed_url"`
	ArtistName string `json:"artist_name"`
	ArtistSlug string `json:"artist_slug"`
}

// ──────────────────────────────────────────────
// Genre profile types (for scene and venue intelligence)
// ──────────────────────────────────────────────

// GenreCount represents a genre tag with its associated artist count
type GenreCount struct {
	TagID uint   `json:"tag_id"`
	Name  string `json:"name"`
	Slug  string `json:"slug"`
	Count int    `json:"count"`
}

// SceneGenreResponse represents the genre distribution for a scene (city)
type SceneGenreResponse struct {
	Genres         []GenreCount `json:"genres"`
	DiversityIndex float64      `json:"diversity_index"` // -1 if insufficient data
	DiversityLabel string       `json:"diversity_label"` // "Highly diverse", "Mixed", "Genre-focused", ""
}

// VenueGenreResponse represents the genre profile for a venue
type VenueGenreResponse struct {
	Genres []GenreCount `json:"genres"`
}

// ──────────────────────────────────────────────
// Scene graph (PSY-367) — derived per-scene artist relationship graph
// ──────────────────────────────────────────────

// SceneGraphResponse is the payload for GET /scenes/{slug}/graph.
// Cluster IDs are computed at query time from each artist's most-frequent venue
// in the scene (see docs/features/scene-graph-layout.md §4 for the rationale).
type SceneGraphResponse struct {
	Scene    SceneGraphInfo      `json:"scene"`
	Clusters []SceneGraphCluster `json:"clusters"`
	Nodes    []SceneGraphNode    `json:"nodes"`
	Links    []SceneGraphLink    `json:"links"`
}

// SceneGraphInfo holds scene metadata for the graph response.
type SceneGraphInfo struct {
	Slug             string `json:"slug"`
	City             string `json:"city"`
	State            string `json:"state"`
	ArtistCount      int    `json:"artist_count"`       // artists in the response (top-N cap applied)
	EdgeCount        int    `json:"edge_count"`         // total edges in the response (post type-filter)
	MetroRosterTotal int    `json:"metro_roster_total"` // full based-in metro roster before top-N cap
	RosterTruncated  bool   `json:"roster_truncated"`   // true when metro_roster_total > artist_count
	// LabelCount is the number of label hub nodes in the response. Counted
	// separately from ArtistCount so the roster-truncation phrasing keeps
	// describing artists only, and the header can name both populations.
	LabelCount int `json:"label_count"`
}

// SceneGraphCluster groups artists in the scene. v1 cluster signal is the
// artist's most-frequently-played venue within the scene. Clusters with fewer
// than the size threshold are rolled into a single "other" cluster.
type SceneGraphCluster struct {
	ID         string `json:"id"`          // "v_<venue_id>" or "other"
	Label      string `json:"label"`       // venue name or "Other"
	Size       int    `json:"size"`        // number of artists in this cluster
	ColorIndex int    `json:"color_index"` // 0-7 = Okabe-Ito index; -1 = "other" (grey)
}

// Scene graph node kinds. The scene graph is artist-centric, plus label hub
// nodes that stand in for a roster (see SceneEdgeTypeOnLabel). Every node
// carries its kind explicitly so consumers branch on a present field rather
// than inferring from a missing one.
const (
	SceneNodeKindArtist = "artist"
	SceneNodeKindLabel  = "label"
)

// SceneEdgeTypeOnLabel is the membership edge between a label hub node and one
// of its roster artists inside the artist set being drawn. It replaces the
// C(n,2) pairwise
// `shared_label` clique a roster would otherwise contribute: n spokes carry the
// same fact ("these artists share this label") in a shape the layout can draw.
//
// Unlike the stored relationship types this is built at query time and has no
// row in `artist_relationships`, so it is never a valid `types` filter value —
// it is emitted with the label hubs or not at all.
const SceneEdgeTypeOnLabel = "on_label"

// SceneGraphNode represents one node in the scene graph: an artist, or a label
// hub standing in for the part of its roster the payload contains. These types
// are named for the scene graph because it was the first payload to carry hubs;
// the hub builder itself is scope-agnostic.
type SceneGraphNode struct {
	// ID is the artist ID for artist nodes. Label hubs are offset into a
	// reserved range so both kinds can share this single numeric node-ID space
	// (see labelHubNodeIDOffset in services/catalog/label_hubs.go) — pair it
	// with EntityType before treating it
	// as a database key.
	ID uint `json:"id"`
	// EntityType is one of the SceneNodeKind* constants. Always populated.
	EntityType string `json:"entity_type"`
	Name       string `json:"name"`
	Slug       string `json:"slug"`
	City       string `json:"city,omitempty"`
	State      string `json:"state,omitempty"`
	// Country is populated for label hubs, whose home is captioned on the
	// canvas and can be known at country granularity only. Artist nodes carry
	// city/state and leave this empty.
	Country           string `json:"country,omitempty"`
	UpcomingShowCount int    `json:"upcoming_show_count"`
	// ClusterID matches SceneGraphCluster.ID; "other" for the tail. Empty on
	// label hubs — clusters describe where artists play, so a hub joins no
	// cluster, no hull, and no cluster-legend count.
	ClusterID string `json:"cluster_id"`
	IsIsolate bool   `json:"is_isolate"` // true when the artist has no in-scene edges (post type-filter)
	// NextShow summarizes the artist's soonest upcoming approved show so graph
	// consumers can render a date/venue chip without a per-node graph-card
	// fetch (PSY-1449). Reuses the graph-card's next-show type so the two
	// surfaces can't disagree on shape. Omitted when UpcomingShowCount is 0 —
	// both derive from the same "approved, event_date > now" cutoff.
	NextShow *ArtistGraphCardShow `json:"next_show,omitempty"`
	// HasPlayableAudio is true when selecting this node opens a playable embed
	// (PSY-1379): a stored Bandcamp embed URL, or a Spotify URL the frontend can
	// embed. Mirrors the ArtistContextPanel `hasPlayableAudio` gate so the canvas
	// marker never promises a player the select panel won't render.
	HasPlayableAudio bool `json:"has_playable_audio"`
}

// SceneGraphLink represents an in-scene relationship between two artists.
// Voting and user-vote data are intentionally omitted — scene graph is read-only
// per the spike's out-of-scope list (see docs/features/scene-graph-layout.md §8).
type SceneGraphLink struct {
	SourceID       uint    `json:"source_id"`
	TargetID       uint    `json:"target_id"`
	Type           string  `json:"type"`
	Score          float64 `json:"score"`
	Detail         any     `json:"detail,omitempty"`
	IsCrossCluster bool    `json:"is_cross_cluster"` // derived: source.cluster_id != target.cluster_id
}

// ──────────────────────────────────────────────
// Venue bill network (PSY-365) — co-bill network of artists at a single venue
// ──────────────────────────────────────────────
//
// The venue analog of the scene graph (PSY-367). Edges are weighted by the
// number of shows the two artists shared *at this specific venue* (not
// globally), which is the unfair-advantage signal called out in
// docs/research/knowledge-graph-viz-prior-art.md §6.
//
// Mirrors SceneGraphResponse field-for-field (`scene` → `venue`, `clusters`,
// `nodes`, `links`) so a shared frontend ForceGraphView can render either
// payload. Cluster-aware layout machinery is preserved on the type even when
// no clusters are returned (v1 ships without explicit clusters — see PSY-365
// PR notes for the rationale).

// VenueBillNetworkResponse is the payload for GET /venues/{id}/bill-network.
type VenueBillNetworkResponse struct {
	Venue    VenueBillNetworkInfo      `json:"venue"`
	Clusters []VenueBillNetworkCluster `json:"clusters"`
	Nodes    []VenueBillNetworkNode    `json:"nodes"`
	Links    []VenueBillNetworkLink    `json:"links"`
}

// VenueBillNetworkInfo holds venue metadata and aggregate counts for the graph.
// Fields mirror SceneGraphInfo (slug + counts) plus venue-specific identifiers.
type VenueBillNetworkInfo struct {
	ID          uint   `json:"id"`
	Slug        string `json:"slug"`
	Name        string `json:"name"`
	City        string `json:"city,omitempty"`
	State       string `json:"state,omitempty"`
	ArtistCount int    `json:"artist_count"` // artists in the graph — assigned from the built node list, so it always equals len(nodes) (time-window filtered AND node-capped)
	// ArtistTotal + RosterTruncated mirror the scene graph's
	// MetroRosterTotal + RosterTruncated (PSY-1081 "not a silent cap"
	// convention): the full in-window artist count before the node cap, and
	// whether the cap bit, so the frontend can render "top N of M" copy
	// without another API change.
	ArtistTotal     int  `json:"artist_total"`     // distinct in-window artists BEFORE the node cap
	RosterTruncated bool `json:"roster_truncated"` // true when artist_total > the node cap
	EdgeCount       int  `json:"edge_count"`       // co-bill pairs above the min-shared-shows threshold
	ShowCount       int  `json:"show_count"`       // approved shows used to derive the network
	// WindowLabel describes the active time window in the response so the
	// frontend can label the graph without reverse-engineering the filter.
	// One of: "all_time", "last_12m", "year".
	Window string `json:"window"`
	// Year is populated only when Window=="year"; carries the requested year.
	Year *int `json:"year,omitempty"`
}

// VenueBillNetworkCluster matches the SceneGraphCluster shape so the same
// ForceGraphView legend renders both. v1 ships without explicit clusters at
// venue scope (every artist's primary venue is, by definition, this venue —
// the scene graph's signal collapses), so the array is typically empty.
type VenueBillNetworkCluster struct {
	ID         string `json:"id"`
	Label      string `json:"label"`
	Size       int    `json:"size"`
	ColorIndex int    `json:"color_index"`
}

// VenueBillNetworkNode mirrors SceneGraphNode. ClusterID defaults to "other"
// when no clusters are computed (v1 default).
type VenueBillNetworkNode struct {
	ID                uint   `json:"id"`
	Name              string `json:"name"`
	Slug              string `json:"slug"`
	City              string `json:"city,omitempty"`
	State             string `json:"state,omitempty"`
	UpcomingShowCount int    `json:"upcoming_show_count"`
	ClusterID         string `json:"cluster_id"`
	IsIsolate         bool   `json:"is_isolate"`
	// AtVenueShowCount is the number of approved shows this artist has played
	// at the venue, within the active time window. Surfaces the rank signal
	// the user is intuiting from "who's a regular here".
	AtVenueShowCount int `json:"at_venue_show_count"`
}

// VenueBillNetworkLink mirrors SceneGraphLink. The Detail field carries
// `shared_count` (number of shared shows AT THIS VENUE in the active window)
// and `last_shared` (most recent shared event date) so the frontend tooltip
// stays edge-grammar-compatible with PSY-362.
type VenueBillNetworkLink struct {
	SourceID       uint    `json:"source_id"`
	TargetID       uint    `json:"target_id"`
	Type           string  `json:"type"`
	Score          float64 `json:"score"`
	Detail         any     `json:"detail,omitempty"`
	IsCrossCluster bool    `json:"is_cross_cluster"`
}

// ──────────────────────────────────────────────
// Show Service Interfaces
// ──────────────────────────────────────────────

// ShowsQuery carries the page window for the catalog-wide show list behind
// GET /shows. The per-entity twins are ArtistShowsQuery and VenueShowsQuery;
// this one has no TimeFilter or Year because /shows scopes with from_date and
// to_date instead, which are strictly more expressive and already shipped.
//
// A struct rather than two positional ints for the same reason its twins are
// structs: Limit and Offset are both ints, so a transposed pair at a call site
// compiles cleanly and silently pages the wrong window.
type ShowsQuery struct {
	// Limit caps the page. Zero returns no rows while still reporting the full
	// total, matching the twins. Negative is clamped to zero rather than
	// handed to GORM, which reads a negative limit as "no limit at all" and
	// would serialize the entire approved catalog, the 10 MB response this
	// window exists to make impossible (PSY-1748).
	Limit int
	// Offset skips this many rows of the ordered page. Negative is clamped to
	// 0; GORM emits `OFFSET -1` verbatim and Postgres rejects the statement.
	Offset int
}

// ShowServiceInterface defines the contract for core show CRUD and search operations.
type ShowServiceInterface interface {
	CreateShow(req *CreateShowRequest) (*ShowResponse, error)
	GetShow(showID uint) (*ShowResponse, error)
	GetShowBySlug(slug string) (*ShowResponse, error)
	// GetShowTimeline returns the headliner's adjacent dates and each billed
	// act's recurrence in this show's place, addressed by numeric id or slug.
	//
	// Approved-only, and an unknown show and a non-approved one return the same
	// not-found error: this is an anonymous surface enumerable by show id, so a
	// distinguishable answer would confirm that an unpublished id is real.
	GetShowTimeline(idOrSlug string) (*ShowTimelineResponse, error)
	// GetShows returns ONE PAGE of approved shows matching filters, plus the
	// filter-aware total across all pages.
	//
	// The page window is mandatory rather than optional. Before PSY-1748 this
	// method had no bound at all: it loaded every approved show ever, past
	// included, and ran buildShowResponse (two queries per show) over the lot:
	// 10,161,875 bytes and ~1+2N queries per request, measured against
	// production 2026-08-08, growing monotonically with the catalog.
	GetShows(filters map[string]interface{}, page ShowsQuery) ([]*ShowResponse, int64, error)
	GetUserSubmissions(userID uint, limit, offset int) ([]ShowResponse, int, error)
	UpdateShow(showID uint, req *UpdateShowRequest) (*ShowResponse, error)
	UpdateShowWithRelations(showID uint, req *UpdateShowRequest, venues []CreateShowVenue, artists []CreateShowArtist, isAdmin bool) (*ShowResponse, []OrphanedArtist, error)
	// GetUpcomingShows returns one page of upcoming shows plus a next-page
	// cursor and the filter-aware total matching set size (ignores cursor —
	// total is always the full matching upcoming catalog for these filters).
	//
	// "Upcoming" is each show's own venue-local calendar day, so the answer is
	// the same for every caller. The timezone parameter is accepted and IGNORED
	// (PSY-1678); it survives only because removing it is a breaking change.
	GetUpcomingShows(timezone string, cursor string, limit int, includeNonApproved bool, filters *UpcomingShowsFilter) ([]*ShowResponse, *string, int64, error)
	// GetShowCities counts the SAME venue-local upcoming partition
	// GetUpcomingShows lists, so a non-zero city count cannot dead-end at an
	// empty list. Its timezone parameter is inert for the same reason.
	GetShowCities(timezone string) ([]ShowCityResponse, error)
	DeleteShow(showID uint) error
	// SearchShows returns up to 20 shows matching the query in show title or
	// any bill artist name (case-insensitive), ordered by event_date DESC.
	// Empty query returns an empty slice. PSY-520.
	SearchShows(query string) ([]*ShowSearchResult, error)
}

// ShowAdminServiceInterface defines the contract for admin show management operations
// including pending/rejected queries, approval flows, and batch operations.
type ShowAdminServiceInterface interface {
	GetPendingShows(limit, offset int, filters *PendingShowsFilter) ([]*ShowResponse, int64, error)
	GetRejectedShows(limit, offset int, search string) ([]*ShowResponse, int64, error)
	ApproveShow(showID uint, verifyVenues bool) (*ShowResponse, error)
	RejectShow(showID uint, reason string) (*ShowResponse, error)
	BatchApproveShows(showIDs []uint) (*BatchShowResult, error)
	BatchRejectShows(showIDs []uint, reason string, category string) (*BatchShowResult, error)
	GetAdminShows(limit, offset int, filters AdminShowFilters) ([]*ShowResponse, int64, error)
}

// ShowImportServiceInterface defines the contract for show import/export operations.
type ShowImportServiceInterface interface {
	ExportShowToMarkdown(showID uint) ([]byte, string, error)
	ParseShowMarkdown(content []byte) (*ParsedShowImport, error)
	PreviewShowImport(content []byte) (*ImportPreviewResponse, error)
	ConfirmShowImport(content []byte, isAdmin bool) (*ShowResponse, error)
}

// ShowStateServiceInterface defines the contract for show state mutation operations
// such as publishing, unpublishing, and setting sold-out/cancelled flags.
type ShowStateServiceInterface interface {
	UnpublishShow(showID uint, userID uint, isAdmin bool) (*ShowResponse, error)
	MakePrivateShow(showID uint, userID uint, isAdmin bool) (*ShowResponse, error)
	PublishShow(showID uint, userID uint, isAdmin bool) (*ShowResponse, error)
	SetShowSoldOut(showID uint, isSoldOut bool) (*ShowResponse, error)
	SetShowCancelled(showID uint, isCancelled bool) (*ShowResponse, error)
}

// ShowFullServiceInterface is the composite interface that embeds all show service
// concerns. The concrete ShowService satisfies this. Useful for the service container
// and backward compatibility where a single reference to all methods is needed.
type ShowFullServiceInterface interface {
	ShowServiceInterface
	ShowAdminServiceInterface
	ShowImportServiceInterface
	ShowStateServiceInterface
}

// ──────────────────────────────────────────────
// Venue Service Interface
// ──────────────────────────────────────────────

// VenueServiceInterface defines the contract for venue operations.
type VenueServiceInterface interface {
	CreateVenue(req *CreateVenueRequest, isAdmin bool) (*VenueDetailResponse, error)
	// GetVenue / GetVenueBySlug are the CHEAP identity lookups — no provenance
	// stamp. Internal callers lean on them to snapshot a row before an edit or
	// to resolve a slug for an unrelated sub-resource, and none of them render
	// provenance. Use GetVenueDetail for the venue-detail read.
	GetVenue(venueID uint) (*VenueDetailResponse, error)
	GetVenueBySlug(slug string) (*VenueDetailResponse, error)
	// GetVenueDetail resolves by numeric ID or slug and attaches the provenance
	// stamp. This is the only read that pays for the aggregations.
	GetVenueDetail(idOrSlug string) (*VenueDetailResponse, error)
	GetVenues(filters map[string]interface{}) ([]*VenueDetailResponse, error)
	UpdateVenue(venueID uint, req *UpdateVenueRequest) (*VenueDetailResponse, error)
	DeleteVenue(venueID uint) error
	SearchVenues(query string) ([]*VenueDetailResponse, error)
	FindOrCreateVenue(name, city, state string, address, zipcode *string, db *gorm.DB, isAdmin bool) (*catalogm.Venue, bool, error)
	VerifyVenue(venueID uint) (*VenueDetailResponse, error)
	GetVenuesWithShowCounts(filters VenueListFilters, limit, offset int) ([]*VenueWithShowCountResponse, int64, error)
	// GetVenueListing is the slug+name projection behind GET /venues/listing.
	// Same SET as an unfiltered GetVenuesWithShowCounts, two columns wide and
	// unpaginated, ordered by name rather than by that path's activity sort; see
	// VenueListingEntry for why that earns its own endpoint and why the order is
	// deliberately not shared. The second return is the size of the browse set
	// BEFORE the slug filter, read in the same snapshot as the rows so a caller
	// can tell a complete listing from a short one — see the handler.
	GetVenueListing() ([]VenueListingEntry, int64, error)
	GetShowsForVenue(venueID uint, timezone string, query VenueShowsQuery) ([]*VenueShowResponse, int64, error)
	// GetVenueShowYears is the year histogram behind the show list's year
	// picker. It deliberately ignores VenueShowsQuery.Year: the picker has to
	// render every selectable year, including the ones the current page is
	// filtered away from.
	GetVenueShowYears(venueID uint, timeFilter string) ([]VenueShowYearCount, error)
	// GetVenueShowMonths is the same histogram one resolution finer, and for a
	// different consumer: the archive's PAGE LABELS, which need to name the
	// months behind a page number before the reader has fetched that page. Like
	// the year histogram it spans every year, so one read serves the all-years
	// archive and every year-scoped view of it.
	GetVenueShowMonths(venueID uint, timeFilter string) ([]VenueShowMonthCount, error)
	// HasPastShowsInYear is the ONE-BIT form of the same question, for the year
	// archive's existence probe: it takes the same venue-local year bucketing as
	// the histogram but answers from an indexed range with LIMIT 1 instead of
	// aggregating the venue's whole history. Past-only, because a year archive
	// is.
	HasPastShowsInYear(venueID uint, year int) (bool, error)
	GetVenueCities() ([]*VenueCityResponse, error)
	GetVenueModel(venueID uint) (*catalogm.Venue, error)
	GetUnverifiedVenues(limit, offset int) ([]*UnverifiedVenueResponse, int64, error)
	GetVenueGenreProfile(venueID uint) ([]GenreCount, error)
	// PSY-365: venue-rooted co-bill network. Edges are weighted by the
	// number of shared shows AT THIS VENUE (not globally) within the
	// requested time window. Window is one of "all", "12m", "year"; Year
	// is required iff Window=="year". Empty Window defaults to "all".
	GetVenueBillNetwork(venueID uint, window string, year *int) (*VenueBillNetworkResponse, error)
}

// VenueConfirmServiceInterface is the whole surface the confirm-current
// endpoint needs (PSY-1542).
//
// Deliberately its own one-method interface rather than an eighteenth method on
// VenueServiceInterface, matching how every other one-tap engagement toggle in
// this codebase is wired (CollectionLikeHandler, FollowHandler, SavedShowHandler
// each take a narrow service). A public, rate-limited, high-frequency mutation
// should not depend on the same broad surface as admin venue CRUD, and every
// handler test that mocks venue reads should not have to stub a confirm it never
// calls.
type VenueConfirmServiceInterface interface {
	// ConfirmVenue records that userID vouches for this venue's info being
	// current. Idempotent: a repeat confirm is a no-op that returns the same
	// aggregate, never an error.
	ConfirmVenue(venueID uint, userID uint) (*VenueConfirmationResponse, error)
}

// MergeVenueResult reports what a venue merge changes. The SAME shape is
// returned by PreviewMergeVenues (what WOULD change) and MergeVenues (what DID
// change), so an admin can diff the two and see whether anything moved between
// looking and committing.
//
// DuplicateShows and SupportActsRescued are the two counts worth reading
// closely before confirming: the first is shows this merge DELETES, the second
// is bill entries rescued off those shows onto the surviving show.
type MergeVenueResult struct {
	CanonicalVenueID   uint   `json:"canonical_venue_id"`
	CanonicalVenueName string `json:"canonical_venue_name"`
	MergedVenueID      uint   `json:"merged_venue_id"`
	MergedVenueName    string `json:"merged_venue_name"`

	// DuplicateShows counts shows on the losing venue that duplicate a show on
	// the canonical venue and are therefore deleted. Destructive — surface it.
	DuplicateShows int64 `json:"duplicate_shows"`
	// SupportActsRescued counts bill entries moved off a deleted duplicate show
	// onto the surviving show because the survivor's bill lacked that artist.
	SupportActsRescued int64 `json:"support_acts_rescued"`

	ShowVenuesMoved      int64 `json:"show_venues_moved"`
	ShowArtistsMoved     int64 `json:"show_artists_moved"`
	FestivalVenuesMoved  int64 `json:"festival_venues_moved"`
	FestivalArtistsMoved int64 `json:"festival_artists_moved"`
	ConfirmationsMoved   int64 `json:"confirmations_moved"`
	FiltersUpdated       int64 `json:"filters_updated"`
	// EntityRefsMoved totals the polymorphic (entity_type='venue', entity_id)
	// rows re-pointed across every table in polymorphicEntityRefs.
	EntityRefsMoved int64 `json:"entity_refs_moved"`
}

// VenueMergeServiceInterface is the admin venue-merge surface.
//
// Deliberately its own narrow interface rather than two more methods on
// VenueServiceInterface, for the same reason VenueConfirmServiceInterface is
// separate: a destructive admin-only operation should not widen the surface
// that every venue READ handler already depends on and mocks.
type VenueMergeServiceInterface interface {
	// PreviewMergeVenues reports what MergeVenues would do, without committing.
	PreviewMergeVenues(canonicalID, mergeFromID uint) (*MergeVenueResult, error)
	// MergeVenues folds mergeFromID into canonicalID and deletes the former.
	// actorUserID is recorded in the audit log.
	MergeVenues(canonicalID, mergeFromID, actorUserID uint) (*MergeVenueResult, error)
}

// ──────────────────────────────────────────────
// Artist Service Interface
// ──────────────────────────────────────────────

// EntityDeleteActor is who asked for a delete.
//
// It exists because DELETE /artists/{artist_id} is the one delete in the catalog
// that is NOT admin-only: its five siblings (venue, label, festival, release,
// show) sit behind the admin router, so their service methods can assume the
// caller may destroy anything the entity carries. The artist path is open to any
// authenticated user, because the show form offers to clean up an artist its
// edit just orphaned, so the service has to be told who is asking before it can
// decide how much it is allowed to destroy. See DeleteArtist.
//
// IsAdmin comes from the SAME field HumaAdminMiddleware gates on
// (authm.User.IsAdmin, re-read from the database on every request by
// JWTService.ValidateToken), so a caller cannot be an admin here and a non-admin
// on /admin routes.
type EntityDeleteActor struct {
	// UserID is the authenticated caller. Zero is rejected on the non-admin
	// path: the gate would have no way to tell the caller's own rows from a
	// stranger's, so the delete fails rather than running with a widened
	// definition of "inert".
	UserID uint
	// IsAdmin lifts the inertness gate entirely, matching the five admin-only
	// delete paths.
	IsAdmin bool
}

// ArtistServiceInterface defines the contract for artist operations.
type ArtistServiceInterface interface {
	CreateArtist(req *CreateArtistRequest) (*ArtistDetailResponse, error)
	GetArtist(artistID uint) (*ArtistDetailResponse, error)
	// GetArtistSummary / …BySlug: identity-only reads (no stats block) for hot
	// composition endpoints like the graph-card (PSY-1352).
	GetArtistSummary(artistID uint) (*ArtistDetailResponse, error)
	GetArtistSummaryBySlug(slug string) (*ArtistDetailResponse, error)
	GetArtistByName(name string) (*ArtistDetailResponse, error)
	GetArtistBySlug(slug string) (*ArtistDetailResponse, error)
	GetArtists(filters map[string]interface{}) ([]*ArtistDetailResponse, error)
	// GetArtistsWithShowCounts returns ONE PAGE of the browse list plus the
	// total matching the same filters. Its implementation records why the whole
	// set is not an option and what the page order has to guarantee.
	GetArtistsWithShowCounts(filters map[string]interface{}, limit, offset int) ([]*ArtistWithShowCountResponse, int64, error)
	// GetArtistListing is the slug+name projection behind GET /artists/listing.
	// Same GATE and order as GetArtistsWithShowCounts but the whole set rather
	// than one page, two columns wide; see ArtistListingEntry for why that
	// distinction is load-bearing.
	GetArtistListing() ([]ArtistListingEntry, error)
	UpdateArtist(artistID uint, req *UpdateArtistRequest) (*ArtistDetailResponse, error)
	// DeleteArtist removes an artist and the polymorphic references that would
	// otherwise be stranded. actor decides how much it may destroy: an admin
	// deletes outright, a non-admin only when no other user has engaged with the
	// artist. See the implementation for the inertness rule.
	DeleteArtist(artistID uint, actor EntityDeleteActor) error
	SearchArtists(query string) ([]*ArtistDetailResponse, error)
	GetShowsForArtist(artistID uint, timezone string, query ArtistShowsQuery) ([]*ArtistShowResponse, int64, error)
	// GetArtistShowYears is the year histogram behind the show list's year
	// picker. It deliberately ignores ArtistShowsQuery.Year: the picker has to
	// render every selectable year, including the ones the current page is
	// filtered away from.
	GetArtistShowYears(artistID uint, timeFilter string) ([]ArtistShowYearCount, error)
	// GetArtistShowMonths is the same histogram one resolution finer, and for a
	// different consumer: the archive's PAGE LABELS, which need to name the
	// months behind a page number before the reader has fetched that page. Like
	// the year histogram it spans every year, so one read serves the all-years
	// archive and every year-scoped view of it. The venue twin is
	// GetVenueShowMonths, and the two must keep answering the same question —
	// one shared archive component labels both surfaces (PSY-1842).
	GetArtistShowMonths(artistID uint, timeFilter string) ([]ArtistShowMonthCount, error)
	// GetNextShowForArtist: the soonest upcoming show only (no count, no bill) —
	// the graph-card's next-show glance (PSY-1352).
	GetNextShowForArtist(artistID uint, timezone string) (*ArtistShowResponse, error)
	GetArtistCities() ([]*ArtistCityResponse, error)
	GetLabelsForArtist(artistID uint) ([]*ArtistLabelResponse, error)
	AddArtistAlias(artistID uint, alias string) (*ArtistAliasResponse, error)
	RemoveArtistAlias(aliasID uint) error
	GetArtistAliases(artistID uint) ([]*ArtistAliasResponse, error)
	MergeArtists(canonicalID, mergeFromID uint) (*MergeArtistResult, error)
}

// ──────────────────────────────────────────────
// Scene Service Interface
// ──────────────────────────────────────────────

// SceneServiceInterface defines the contract for computed scene aggregations.
// Scenes are keyed by US Census CBSA metro (PSY-1255 step C); the (city, state)
// args are the metro's PRINCIPAL city/state (or the literal city for a non-US /
// no-CBSA fallback scene), as returned by ParseSceneSlug.
type SceneServiceInterface interface {
	ListScenes() ([]*SceneListResponse, error)
	GetSceneDetail(city, state string) (*SceneDetailResponse, error)
	// GetActiveArtists returns the scene's full roster — every band based in the
	// metro — with is_active flagged and sorted first. activeWindowDays is the
	// recency window (a band is active if it has a show within it or upcoming);
	// it is NOT a membership filter, so the returned total is the whole roster.
	GetActiveArtists(city, state string, activeWindowDays, limit, offset int) ([]*SceneArtistResponse, int64, error)
	// GetRepresentativeEmbed returns the single band whose Bandcamp embed
	// represents the scene — the first band with a non-null bandcamp_embed_url in
	// the roster's active-first ordering, computed over the FULL metro roster (not
	// a fetched page). nil when no band based here has an embed. Lets the /atlas
	// preview's player be independent of the fetched roster window (PSY-1294).
	// Same roster scope (artistPredicate) and active-first ordering as
	// GetActiveArtists; activeWindowDays defines "active" identically.
	GetRepresentativeEmbed(city, state string, activeWindowDays int) (*SceneRepresentativeEmbed, error)
	ParseSceneSlug(slug string) (string, string, error)
	// Scene registry (PSY-1339): scenes materialize a row lazily so id-keyed
	// features (follows) can reference them. GetOrCreateSceneID canonicalizes
	// the slug (member city → metro principal) and creates the row on first
	// need; LookupSceneID resolves without creating (read paths — absent row
	// means zero follows). Both 404 unknown slugs via ParseSceneSlug.
	GetOrCreateSceneID(slug string) (uint, error)
	LookupSceneID(slug string) (uint, bool, error)
	// GetSceneCollections returns the public collections relevant to a scene,
	// most scene-local first — the scene page's "Collections · {city}" rail.
	//
	// Collections have no scene field, so relevance is derived per-member: an
	// artist member counts as scene-local when it is BASED in the scene
	// (artistPredicate — the same roster rule as GetActiveArtists), a venue
	// member when it sits in the scene's scope, a show member when it is
	// booked at a scene venue. Release, label and festival members can never
	// be scene-local; they still count toward the total, so a collection is
	// never credited for members the rule cannot vouch for.
	//
	// A collection qualifies when at least half its members are scene-local OR
	// at least sceneCollectionAbsoluteThreshold of them are — the absolute arm
	// exists so a large, genuinely city-heavy collection is not shut out by a
	// long tail of releases, and the ratio arm exists so a small focused
	// collection is not shut out by having few members at all.
	//
	// Private collections are excluded. Returns an empty slice (never an
	// error) when nothing qualifies, so the rail can simply hide itself; 404s
	// only when the slug is not a scene at all, like its sibling rails.
	GetSceneCollections(city, state string, limit int) ([]SceneCollectionSummary, error)
	// GetSceneGaps returns the scene's completeness gap counts (PSY-1845) — the
	// numbers behind the scene page's "help finish this scene" line. Gated on
	// the SAME scene-existence threshold as GetSceneDetail, so a place that is
	// not yet a scene 404s here too rather than publishing a row of zeros.
	GetSceneGaps(city, state string) (*SceneGapsResponse, error)
	// UpdateSceneTagline sets the authored tagline for a scene, or clears it
	// when tagline is nil. Returns the registry row id it wrote (for the
	// caller's audit entry) and the CANONICAL slug of the scene it landed on,
	// which differs from the argument when a metro member city was addressed —
	// callers echo the canonical one, the same slug every scene read returns.
	// Materializes the row on first need: an authored tagline is exactly the
	// kind of curated reference that gives a computed scene a row. Callers own
	// the length guard; the column caps at 80.
	UpdateSceneTagline(slug string, tagline *string) (uint, string, error)
	GetSceneGenreDistribution(city, state string) ([]GenreCount, error)
	GetGenreDiversityIndex(city, state string) (float64, error)
	// clusterBy selects the cluster signal: "venue" (default) or "community"
	// (the persisted Leiden similarity partition, PSY-1262).
	GetSceneGraph(city, state string, types []string, clusterBy string) (*SceneGraphResponse, error)
	// GetSceneUpcomingShows returns the scene's next approved shows within
	// windowDays, soonest first, capped at limit — the preview panel's "This
	// week" row (PSY-1309). Metro-scoped like every other scene surface (a
	// Tempe show counts toward the Phoenix scene), which is why this isn't the
	// literal-city shows endpoint.
	GetSceneUpcomingShows(city, state string, windowDays, limit int) ([]SceneShowSummary, error)
	// GetSceneNewArtistsSince returns bands based in the scene created after
	// `since` (up to `now`), newest first, capped — plus the TOTAL in the
	// window so the caller can render "+N more" (the cap must not silently
	// drop bands the cursor then advances past). The weekly digest's "new
	// bands based here" stream (PSY-1342). Same roster scope as GetActiveArtists.
	GetSceneNewArtistsSince(city, state string, since, now time.Time, limit int) ([]SceneNewArtist, int, error)
	// GetSceneLatestArtists is the scene page's latest-additions module
	// (PSY-1781, redefined by PSY-1844): the scene roster's `limit` most
	// recently listed bands, newest first, each enriched with the band's next
	// (or, failing that, most recent) approved show.
	//
	// UNWINDOWED, and deliberately not GetSceneNewArtistsSince. A trailing
	// window made the module's content a function of when seeding batches ran
	// rather than of the scene — measured empty on 5 of 6 major scenes — while
	// the digest must keep its window because it advances a per-follow cursor.
	// The two surfaces answer different questions and no longer share a query.
	//
	// The rename stops HERE, on purpose. It exists because a method called
	// GetSceneNewArtists sitting beside GetSceneNewArtistsSince implied it
	// wrapped the windowed one, which stopped being true. The route
	// (/scenes/{slug}/new-artists), the wire types (GetSceneNewArtists*) and
	// SceneNewArtistRow keep the old spelling because changing them is a
	// BREAKING API change, not tidy-up. This is not a half-finished rename.
	GetSceneLatestArtists(city, state string, now time.Time, limit int) ([]SceneNewArtistRow, error)
	// GetSceneShowsInRange returns the scene's approved shows in the half-open
	// window [from, to), rendering dates in loc. Shared by the digest email and
	// the weekly city page so the two can never disagree about a scene's shows.
	GetSceneShowsInRange(city, state string, from, to time.Time, loc *time.Location, limit int) ([]SceneShowSummary, error)
	// GetSceneWeek returns one ISO week of a scene's shows grouped by day,
	// computed in the SCENE's timezone. weekKey is an ISO week key
	// ("2026-W31") or "" for the scene's current week.
	GetSceneWeek(city, state, weekKey string) (*SceneWeekResponse, error)
	// GetSceneDay returns ONE calendar day of a scene's shows, computed in the
	// SCENE's timezone. dateKey is an ISO calendar date ("2026-07-31") or "" for
	// the scene's current night — which is not the same as its current date, see
	// SceneDayResponse.IsTonight.
	GetSceneDay(city, state, dateKey string) (*SceneDayResponse, error)
	// GetShowAlsoTonight returns the OTHER shows in the subject show's metro on
	// the subject show's own venue-local date (read on the room's own zone when
	// it has one, the metro's modal clock otherwise) — the show page's "also tonight"
	// rail. idOrSlug is a numeric show id or a show slug, matching every other
	// /shows/{show_id} sub-resource.
	//
	// Scene-scoped rather than show-scoped because "same metro" is a scene
	// question, and there must be exactly one definition of a metro's shows on a
	// date or this rail and the scene-day page would disagree.
	//
	// An unknown or non-approved show is a not-found error; a show whose venue
	// cannot be scoped to a scene is an EMPTY rail, not an error.
	GetShowAlsoTonight(idOrSlug string) (*ShowAlsoTonightResponse, error)
}
