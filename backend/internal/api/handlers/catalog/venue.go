package catalog

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"

	"psychic-homily-backend/internal/api/handlers/shared"
	"psychic-homily-backend/internal/api/middleware"
	apperrors "psychic-homily-backend/internal/errors"
	"psychic-homily-backend/internal/logger"
	"psychic-homily-backend/internal/services/contracts"
	servicesshared "psychic-homily-backend/internal/services/shared"
	"psychic-homily-backend/internal/services/shared/revisiondiff"

	"github.com/danielgtaylor/huma/v2"
)

type VenueHandler struct {
	venueService    contracts.VenueServiceInterface
	discordService  contracts.DiscordServiceInterface
	auditLogService contracts.AuditLogServiceInterface
	revisionService contracts.RevisionServiceInterface
}

func NewVenueHandler(venueService contracts.VenueServiceInterface, discordService contracts.DiscordServiceInterface, auditLogService contracts.AuditLogServiceInterface, revisionService contracts.RevisionServiceInterface) *VenueHandler {
	return &VenueHandler{
		venueService:    venueService,
		discordService:  discordService,
		auditLogService: auditLogService,
		revisionService: revisionService,
	}
}

type SearchVenuesRequest struct {
	Query string `query:"q" maxLength:"200" doc:"Search query for venue autocomplete" example:"empty bottle"`
}

type SearchVenuesResponse struct {
	Body struct {
		Venues []*contracts.VenueDetailResponse `json:"venues" doc:"Matching venues"`
		Count  int                              `json:"count" doc:"Number of results"`
	}
}

func (h *VenueHandler) SearchVenuesHandler(ctx context.Context, req *SearchVenuesRequest) (*SearchVenuesResponse, error) {
	venues, err := h.venueService.SearchVenues(req.Query)
	if err != nil {
		return nil, err
	}

	resp := &SearchVenuesResponse{}
	resp.Body.Venues = venues
	resp.Body.Count = len(venues)

	return resp, nil
}

// ListVenuesRequest represents the request parameters for listing venues
type ListVenuesRequest struct {
	State    string `query:"state" doc:"Filter by state" example:"AZ"`
	City     string `query:"city" doc:"Filter by city" example:"Phoenix"`
	Cities   string `query:"cities" doc:"Pipe-delimited multi-city filter (max 10): Phoenix,AZ|Tucson,AZ" example:"Phoenix,AZ|Tucson,AZ"`
	Limit    int    `query:"limit" default:"50" minimum:"1" maximum:"100" doc:"Maximum number of venues to return"`
	Offset   int    `query:"offset" default:"0" minimum:"0" doc:"Offset for pagination"`
	Tags     string `query:"tags" doc:"Comma-separated tag slugs. Multi-tag filter (PSY-309): AND by default; set tag_match=any for OR." example:"diy,phoenix"`
	TagMatch string `query:"tag_match" doc:"Tag matching mode: 'all' (default, AND) or 'any' (OR)" example:"all" enum:"all,any"`
	// Opt-in because filling these fields costs three extra batched
	// aggregations per page. Only the Atlas city-view rail renders them.
	IncludeRail bool `query:"include_rail" doc:"Include the Atlas city-view rail fields: next_show_date/title/artists, shows_this_week, dominant_genre"`
	// Opt-in for the same reason include_rail is: it changes which venues the
	// browse page's city filter returns, and that filter means the literal
	// city there. Only the Atlas city rail wants the metro reading.
	MetroRollup bool `query:"metro_rollup" doc:"Widen the city+state filter to the whole US Census CBSA metro, matching how Atlas scenes are keyed (Tempe lists under Phoenix). Requires both city and state; ignored when 'cities' is set."`
}

// ListVenuesResponse represents the response for the list venues endpoint
type ListVenuesResponse struct {
	Body struct {
		Venues []*contracts.VenueWithShowCountResponse `json:"venues" doc:"List of venues with show counts"`
		Total  int64                                   `json:"total" doc:"Total number of venues"`
		Limit  int                                     `json:"limit" doc:"Limit used in query"`
		Offset int                                     `json:"offset" doc:"Offset used in query"`
	}
}

// ListVenuesHandler handles GET /venues - returns verified venues with upcoming show counts
func (h *VenueHandler) ListVenuesHandler(ctx context.Context, req *ListVenuesRequest) (*ListVenuesResponse, error) {
	filters := contracts.VenueListFilters{}

	if req.Cities != "" {
		// Parse pipe-delimited multi-city param: "Phoenix,AZ|Tucson,AZ"
		pairs := strings.Split(req.Cities, "|")
		var cityFilters []contracts.CityStateFilter
		for _, pair := range pairs {
			parts := strings.SplitN(pair, ",", 2)
			if len(parts) == 2 && parts[0] != "" && parts[1] != "" {
				cityFilters = append(cityFilters, contracts.CityStateFilter{
					City:  strings.TrimSpace(parts[0]),
					State: strings.TrimSpace(parts[1]),
				})
			}
		}
		// Cap at 10 cities
		if len(cityFilters) > 10 {
			cityFilters = cityFilters[:10]
		}
		filters.Cities = cityFilters
	} else {
		filters.State = req.State
		filters.City = req.City
		filters.MetroRollup = req.MetroRollup
	}
	if tf := parseTagFilter(req.Tags, req.TagMatch); tf.HasTags() {
		filters.TagSlugs = tf.TagSlugs
		filters.TagMatchAny = tf.MatchAny
	}
	filters.IncludeRailFields = req.IncludeRail

	limit := req.Limit
	if limit == 0 {
		limit = 50
	}

	venues, total, err := h.venueService.GetVenuesWithShowCounts(filters, limit, req.Offset)
	if err != nil {
		return nil, huma.Error500InternalServerError("Failed to fetch venues", err)
	}

	resp := &ListVenuesResponse{}
	resp.Body.Venues = venues
	resp.Body.Total = total
	resp.Body.Limit = limit
	resp.Body.Offset = req.Offset

	return resp, nil
}

// ListVenueListingRequest is deliberately empty.
//
// No filters, and none should be added without a consumer that needs them: this
// endpoint exists to be small and complete, and every query parameter is another
// cache key whose payload nobody is watching. A `limit` in particular is what
// this endpoint was created to remove. Filtering belongs on GET /venues.
type ListVenueListingRequest struct{}

// venueListingCacheControl bounds repeat hits from callers that are NOT the
// /venues page, for the reasons spelled out on sitemapEntriesCacheControl — the
// same shape of endpoint (public, unpaginated, viewer-independent projection)
// and the same 5 minutes, short enough that a shared cache never becomes the
// reason a new venue is invisible. The page itself is already bounded to one
// origin hit per hour by Next's fetch Data Cache; this covers everything else
// that can point at a public URL, and an uncached hit here scans every verified
// venue.
//
// IT IS A COURTESY TO COOPERATIVE CALLERS, NOT A CEILING. The request struct is
// empty and huma ignores unknown query parameters, so `?cb=1`, `?cb=2`, … are
// distinct cache keys that all reach the origin. What bounds a hostile caller is
// the global PublicReadRateLimiter this route inherits — which is itself gated
// on ENABLE_PUBLIC_READ_RATE_LIMITS, so that flag being set is a deploy-time
// precondition for this endpoint, not an optimisation.
//
// The artist twin carries no equivalent header. That is a gap in the twin, not a
// venue-only decision.
const venueListingCacheControl = "public, max-age=300"

// ListVenueListingResponse is the slug+name projection of the venue list.
//
// Total is NOT pagination metadata — there is no next page. It is the size of
// the browse set the projection was taken from, read in the SAME statement and
// therefore the same snapshot as the rows, so a caller can compare the two and
// read a gap as exactly one thing: venues that cannot form a URL. See
// GetVenueListing.
type ListVenueListingResponse struct {
	CacheControl string `header:"Cache-Control"`
	Body         struct {
		Venues []contracts.VenueListingEntry `json:"venues" doc:"Venues reduced to slug and name, ordered by name"`
		Count  int                           `json:"count" doc:"Number of venues in this response"`
		Total  int64                         `json:"total" doc:"Size of the browse set this was projected from, read in the same snapshot as the rows. Not pagination metadata: this endpoint has no next page. Equal to count unless some venue cannot form a URL."`
	}
}

// ListVenueListingHandler handles GET /venues/listing.
//
// The narrow twin of ListVenuesHandler, for callers that build one link per
// venue and read nothing else — see contracts.VenueListingEntry for the measured
// reason that distinction earns its own endpoint rather than a wider `maximum`
// on GET /venues.
func (h *VenueHandler) ListVenueListingHandler(ctx context.Context, _ *ListVenueListingRequest) (*ListVenueListingResponse, error) {
	entries, total, err := h.venueService.GetVenueListing()
	if err != nil {
		logger.FromContext(ctx).Error("venue_listing_failed",
			"error", err.Error(),
			"request_id", logger.GetRequestID(ctx),
		)
		// The error is logged, not returned: it would be serialised into the
		// body for an unauthenticated caller, handing over raw driver text.
		return nil, huma.Error500InternalServerError("Failed to fetch venue listing")
	}

	resp := &ListVenueListingResponse{CacheControl: venueListingCacheControl}
	resp.Body.Venues = entries
	resp.Body.Count = len(entries)
	resp.Body.Total = total

	return resp, nil
}

// GetVenueRequest represents the request parameters for getting a single venue
type GetVenueRequest struct {
	VenueID string `path:"venue_id" doc:"Venue ID or slug" example:"valley-bar-phoenix-az"`
}

// GetVenueResponse represents the response for the get venue endpoint
type GetVenueResponse struct {
	Body *contracts.VenueDetailResponse
}

// GetVenueHandler handles GET /venues/{venue_id} - returns a single venue by ID or slug
func (h *VenueHandler) GetVenueHandler(ctx context.Context, req *GetVenueRequest) (*GetVenueResponse, error) {
	// GetVenueDetail owns the id-or-slug resolution AND the provenance stamp —
	// the cheap GetVenue/GetVenueBySlug lookups deliberately carry no stamp.
	venue, err := h.venueService.GetVenueDetail(req.VenueID)
	if err != nil {
		var venueErr *apperrors.VenueError
		if errors.As(err, &venueErr) && venueErr.Code == apperrors.CodeVenueNotFound {
			return nil, huma.Error404NotFound("Venue not found")
		}
		return nil, huma.Error500InternalServerError("Failed to fetch venue", err)
	}

	return &GetVenueResponse{Body: venue}, nil
}

// defaultVenueShowsTimeFilter is what an omitted time_filter means on BOTH the
// venue show list and its year histogram. One constant because the histogram
// drives the list's year picker: if the two defaulted differently, a caller that
// omitted the param would get a picker counting a set the list never shows.
const defaultVenueShowsTimeFilter = "upcoming"

// GetVenueShowsRequest represents the request parameters for getting shows at a venue
type GetVenueShowsRequest struct {
	VenueID    string `path:"venue_id" doc:"Venue ID or slug" example:"valley-bar-phoenix-az"`
	Timezone   string `query:"timezone" doc:"Deprecated and ignored. The upcoming/past split is made in each show's own venue-local timezone, so a caller's zone no longer moves the boundary. Accepted for backward compatibility only." example:"America/Phoenix"`
	Limit      int    `query:"limit" default:"20" minimum:"1" maximum:"200" doc:"Maximum number of shows to return (max 200)"`
	Offset     int    `query:"offset" default:"0" minimum:"0" doc:"Offset for pagination"`
	TimeFilter string `query:"time_filter" doc:"Filter shows by time: upcoming, past, or all" example:"upcoming" enum:"upcoming,past,all"`
	// Bounded rather than open so a nonsense year is a 422 naming the field
	// rather than an empty page the caller has to diagnose. The ceiling matches
	// shared.VenueLocalYearCondition's own, above which it stops emitting the
	// sargable bounds because the Go timestamps stop round-tripping.
	Year int `query:"year" default:"0" minimum:"0" maximum:"9999" doc:"Filter to a single venue-local calendar year. 0 (default) returns every year. Use /venues/{venue_id}/shows/years to discover which years have shows."`
}

// GetVenueShowsResponse represents the response for the venue shows endpoint
type GetVenueShowsResponse struct {
	Body struct {
		Shows   []*contracts.VenueShowResponse `json:"shows" doc:"Page of shows at this venue"`
		VenueID uint                           `json:"venue_id" doc:"Venue ID"`
		Total   int64                          `json:"total" doc:"Total shows matching the time filter and year, across all pages"`
		Limit   int                            `json:"limit" doc:"Limit used in query"`
		Offset  int                            `json:"offset" doc:"Offset used in query"`
		Year    int                            `json:"year" doc:"Year filter used in query (0 = all years)"`
	}
}

// GetVenueShowsHandler handles GET /venues/{venue_id}/shows - returns a page of
// shows at a venue.
func (h *VenueHandler) GetVenueShowsHandler(ctx context.Context, req *GetVenueShowsRequest) (*GetVenueShowsResponse, error) {
	limit := req.Limit
	if limit == 0 {
		limit = 20
	}

	timeFilter := req.TimeFilter
	if timeFilter == "" {
		timeFilter = defaultVenueShowsTimeFilter
	}

	venueID, err := h.resolveVenueID(req.VenueID)
	if err != nil {
		return nil, err
	}

	shows, total, err := h.venueService.GetShowsForVenue(venueID, req.Timezone, contracts.VenueShowsQuery{
		TimeFilter: timeFilter,
		Limit:      limit,
		Offset:     req.Offset,
		Year:       req.Year,
	})
	if err != nil {
		var venueErr *apperrors.VenueError
		if errors.As(err, &venueErr) && venueErr.Code == apperrors.CodeVenueNotFound {
			return nil, huma.Error404NotFound("Venue not found")
		}
		return nil, huma.Error500InternalServerError("Failed to fetch shows", err)
	}

	resp := &GetVenueShowsResponse{}
	resp.Body.Shows = shows
	resp.Body.VenueID = venueID
	resp.Body.Total = total
	resp.Body.Limit = limit
	resp.Body.Offset = req.Offset
	resp.Body.Year = req.Year

	return resp, nil
}

// GetVenueShowYearsRequest represents the request parameters for a venue's
// show-year histogram.
type GetVenueShowYearsRequest struct {
	VenueID    string `path:"venue_id" doc:"Venue ID or slug" example:"valley-bar-phoenix-az"`
	TimeFilter string `query:"time_filter" doc:"Count shows by time: upcoming, past, or all" example:"past" enum:"upcoming,past,all"`
}

// GetVenueShowYearsResponse represents the response for the venue show-years endpoint
type GetVenueShowYearsResponse struct {
	Body struct {
		Years      []contracts.VenueShowYearCount `json:"years" doc:"Venue-local calendar years that have at least one show, newest first"`
		VenueID    uint                           `json:"venue_id" doc:"Venue ID"`
		TimeFilter string                         `json:"time_filter" doc:"Time filter the counts were taken under"`
	}
}

// GetVenueShowYearsHandler handles GET /venues/{venue_id}/shows/years - returns
// the venue-local year histogram behind the show list's year picker.
//
// A sibling of the show list rather than a field on it: the picker has to offer
// every year, so the histogram must NOT narrow to the list's `year` param, and
// it does not change as the reader pages, so recomputing it per page would be
// waste. Both surfaces take the same time_filter, and must be requested with the
// same one or the picker will offer years the list cannot show.
func (h *VenueHandler) GetVenueShowYearsHandler(ctx context.Context, req *GetVenueShowYearsRequest) (*GetVenueShowYearsResponse, error) {
	timeFilter := req.TimeFilter
	if timeFilter == "" {
		timeFilter = defaultVenueShowsTimeFilter
	}

	venueID, err := h.resolveVenueID(req.VenueID)
	if err != nil {
		return nil, err
	}

	years, err := h.venueService.GetVenueShowYears(venueID, timeFilter)
	if err != nil {
		var venueErr *apperrors.VenueError
		if errors.As(err, &venueErr) && venueErr.Code == apperrors.CodeVenueNotFound {
			return nil, huma.Error404NotFound("Venue not found")
		}
		return nil, huma.Error500InternalServerError("Failed to count shows by year", err)
	}

	resp := &GetVenueShowYearsResponse{}
	resp.Body.Years = years
	resp.Body.VenueID = venueID
	resp.Body.TimeFilter = timeFilter

	return resp, nil
}

// VenueYearArchiveExistsRequest addresses one venue-local calendar year of one
// venue's archive.
//
// The year is BOUNDED here so an out-of-range segment is a 422 naming the field
// rather than a round trip to the database, and the ceiling matches
// GetVenueShowsRequest.Year's for the reason given there. The floor is 1 rather
// than the frontend's 1900: this endpoint owns representability, not editorial
// range, and the caller that has an opinion about 1900 (the proxy) already
// rejects anything below it without asking.
type VenueYearArchiveExistsRequest struct {
	VenueID string `path:"venue_id" doc:"Venue ID or slug" example:"valley-bar-phoenix-az"`
	Year    int    `path:"year" minimum:"1" maximum:"9999" doc:"Venue-local calendar year"`
}

// VenueYearArchiveExistsResponse carries no body on purpose: the STATUS is the
// whole answer, which is what lets the caller use HEAD.
type VenueYearArchiveExistsResponse struct{}

// VenueYearArchiveExistsHandler handles HEAD
// /venues/{venue_id}/shows/{year}/exists — 200 when the venue has at least one
// approved PAST show in that venue-local year, 404 when the venue is unknown or
// the year is empty.
//
// 200 rather than the 204 a body-less output would normally get: huma v2.34.1
// special-cases HEAD and pins DefaultStatus to 200 before the body-less rule
// applies. The generated OpenAPI document says 200 for the same reason. A huma
// bump that drops that special case would flip the generated contract to 204
// without breaking anything — the caller only asks `res.ok` — so treat this as a
// fact about the pinned version, not about huma.
//
// WHY A STATUS AND NOT A BODY. Its caller is the frontend proxy, which turns
// `/venues/{slug}/shows/{year}` into a real HTTP 404 for a year that is not a
// document — the page cannot do it itself, because a `notFound()` reached after
// the shell has streamed commits a 404 body at HTTP 200. Before this endpoint
// the proxy had to GET the show LIST scoped to the year and read `total` out of
// the body, because that endpoint answers 200 for any venue that exists; a
// status-bearing probe makes it a HEAD like every other branch in that file, and
// the response never leaves the connection.
//
// The two failure modes answer alike IN THE RESPONSE, and it takes deliberate
// effort rather than indifference. An unknown venue and an empty year both
// return the detail below, because huma writes the error body even for a HEAD
// and Go still derives a Content-Length from it — so two different messages
// would be two different Content-Lengths, and a crawler could separate "no such
// venue" from "no shows that year" off a body it never receives. Measured before
// they were unified: 121 bytes against 147.
//
// They are NOT indistinguishable by TIMING, and this file should not pretend
// otherwise: an unknown slug returns after one GetVenueBySlug, while a known
// venue pays that plus a probe whose cost scales with its history (see below).
// That oracle is wide open and this change widens it. It is tolerable only
// because nothing secret rides on the distinction — venue existence is already
// public through GET /venues/{slug} — which is also the reason the byte-level
// unification is worth doing rather than worth relying on.
//
// COST, stated accurately because the shape invites an assumption. This is two
// statements for a slug, not one: resolveVenueID goes through GetVenueBySlug,
// which selects the venue row and builds a full detail response purely to read
// its id, and the probe below is a second query. Using the shared resolver is
// deliberate — every venue sub-resource must agree on what a bad venue reference
// returns — but an id-only fast path on that resolver is the obvious win if this
// ever shows up in the latency profile, and it would benefit its siblings too.
//
// Its budget is the ordinary anonymous public-read one, and DO NOT read that as
// "enable ENABLE_PUBLIC_READ_RATE_LIMITS and this endpoint is defended". It is
// not, and the reason is worth knowing before anyone acts on the enumeration
// risk below. The frontend proxy's existence probes forward no client headers,
// so every one of them — for every entity type, not just this route — arrives
// from the Vercel egress IP and shares ONE anonymous per-IP bucket, while a
// crawler hitting the site directly gets a bucket of its own. Turning the flag
// on under crawl load therefore throttles the site's own probes first;
// existenceCheck fails open on a 429, and empty years start soft-404ing at HTTP
// 200 — the exact outcome the venue-year branch exists to prevent, with the
// crawler unaffected.
//
// The exposure is real — the URL space is every venue times 8,100 years, and an
// EMPTY year is the case LIMIT 1 cannot short-circuit — but bounding it wants a
// key the proxy's traffic can be told apart by, not this flag.
func (h *VenueHandler) VenueYearArchiveExistsHandler(ctx context.Context, req *VenueYearArchiveExistsRequest) (*VenueYearArchiveExistsResponse, error) {
	venueID, err := h.resolveVenueID(req.VenueID)
	if err != nil {
		// Restated rather than surfaced, so an unknown venue is byte-identical to
		// an empty year. resolveVenueID's own "Venue not found" is the right
		// answer for its other callers and the wrong one here.
		var statusErr huma.StatusError
		if errors.As(err, &statusErr) && statusErr.GetStatus() == http.StatusNotFound {
			return nil, huma.Error404NotFound(venueYearArchiveAbsent)
		}
		return nil, err
	}

	exists, err := h.venueService.HasPastShowsInYear(venueID, req.Year)
	if err != nil {
		return nil, huma.Error500InternalServerError("Failed to check venue year archive", err)
	}
	if !exists {
		return nil, huma.Error404NotFound(venueYearArchiveAbsent)
	}

	return &VenueYearArchiveExistsResponse{}, nil
}

// venueYearArchiveAbsent is the ONE detail both 404 branches carry. Declared
// once because the whole point is that the two are indistinguishable, and two
// string literals is how that stops being true.
const venueYearArchiveAbsent = "No archived shows for that venue and year"

// resolveVenueID turns the shared {venue_id} path parameter, a numeric id or a
// slug, into an id, returning a ready-to-surface huma error. Shared by the
// venue sub-resource reads so they cannot drift apart on what a bad venue
// reference returns.
func (h *VenueHandler) resolveVenueID(idOrSlug string) (uint, error) {
	if id, parseErr := strconv.ParseUint(idOrSlug, 10, 32); parseErr == nil {
		return uint(id), nil
	}

	venue, err := h.venueService.GetVenueBySlug(idOrSlug)
	if err != nil {
		var venueErr *apperrors.VenueError
		if errors.As(err, &venueErr) && venueErr.Code == apperrors.CodeVenueNotFound {
			return 0, huma.Error404NotFound("Venue not found")
		}
		return 0, huma.Error500InternalServerError("Failed to fetch venue", err)
	}
	return venue.ID, nil
}

// GetVenueCitiesRequest represents the request for getting venue cities (empty, no params needed)
type GetVenueCitiesRequest struct{}

// GetVenueCitiesResponse represents the response for the venue cities endpoint
type GetVenueCitiesResponse struct {
	Body struct {
		Cities []*contracts.VenueCityResponse `json:"cities" doc:"List of cities with venue counts"`
	}
}

// GetVenueCitiesHandler handles GET /venues/cities - returns distinct cities with venue counts
func (h *VenueHandler) GetVenueCitiesHandler(ctx context.Context, req *GetVenueCitiesRequest) (*GetVenueCitiesResponse, error) {
	cities, err := h.venueService.GetVenueCities()
	if err != nil {
		return nil, huma.Error500InternalServerError("Failed to fetch cities", err)
	}

	resp := &GetVenueCitiesResponse{}
	resp.Body.Cities = cities

	return resp, nil
}

// ============================================================================
// Admin Venue Creation
// ============================================================================

// AdminCreateVenueRequest represents the request for creating a venue directly
type AdminCreateVenueRequest struct {
	Body struct {
		Name    string  `json:"name" required:"true" doc:"Venue name" maxLength:"255"`
		City    string  `json:"city" required:"true" doc:"Venue city" maxLength:"100"`
		State   string  `json:"state" required:"true" doc:"Venue state" maxLength:"100"`
		Address *string `json:"address" required:"false" doc:"Street address" maxLength:"500"`
		Zipcode *string `json:"zipcode" required:"false" doc:"ZIP code" maxLength:"20"`
		// PSY-1179: capacity + description were silently dropped on create — the
		// service contract + CLI sent them but this HTTP body omitted them.
		// Bounds mirror contracts.MinVenueCapacity / MaxVenueCapacity, which the
		// contributor suggest-edit queue enforces too. Tag values must be
		// literals, so TestVenueCapacitySchemaTagsMatchContract pins them.
		Capacity *int `json:"capacity" required:"false" minimum:"1" maximum:"200000" doc:"Venue capacity"`
		// House-default age rule. Free text mirroring the show-level
		// age_requirement vocabulary; the show's own value is the per-event override.
		AgePolicy   *string `json:"age_policy" required:"false" doc:"House-default age policy, e.g. all ages, 17+, 21+" maxLength:"100"`
		Description *string `json:"description" required:"false" doc:"Markdown description (max 5000 chars)" maxLength:"5000"`
		Instagram   *string `json:"instagram" required:"false" doc:"Instagram URL" maxLength:"255"`
		Facebook    *string `json:"facebook" required:"false" doc:"Facebook URL" maxLength:"500"`
		Twitter     *string `json:"twitter" required:"false" doc:"Twitter URL" maxLength:"255"`
		YouTube     *string `json:"youtube" required:"false" doc:"YouTube URL" maxLength:"500"`
		Spotify     *string `json:"spotify" required:"false" doc:"Spotify URL" maxLength:"500"`
		SoundCloud  *string `json:"soundcloud" required:"false" doc:"SoundCloud URL" maxLength:"500"`
		Bandcamp    *string `json:"bandcamp" required:"false" doc:"Bandcamp URL" maxLength:"500"`
		Website     *string `json:"website" required:"false" doc:"Website URL" maxLength:"500"`
		Country     *string `json:"country,omitempty" required:"false" doc:"Venue country" maxLength:"100"`
	}
}

// AdminCreateVenueResponse represents the response for creating a venue
type AdminCreateVenueResponse struct {
	Body *contracts.VenueDetailResponse
}

// validateCapacityBound rejects an admin-supplied capacity outside the range the
// contributor suggest-edit queue enforces, so all three write paths agree.
//
// This duplicates the bodies' minimum/maximum schema tags on purpose. Those tags
// are real (huma reads its own schema tags, unlike the inert `validate:"..."`
// ones elsewhere in this package) but they only fire on a full huma round trip,
// which every handler test in this package bypasses by calling the handler
// directly. Below this point there is no backstop: VenueService copies a
// non-nil req.Capacity into the update map without inspecting it, and the
// column has no CHECK constraint. An inline guard is the only form of this
// bound a test in this file can prove.
//
// nil means "not supplied" on both bodies and passes through untouched, so
// these routes cannot express a CLEAR. That predates this check (a *int body
// has never had a way to say NULL) but the bound narrows the workaround: an
// admin used to be able to overwrite a bad capacity with 0, and now cannot.
// The remedy is the edit drawer, which routes an admin's save through the
// contributor auto-apply path and does clear to NULL. Giving these routes an
// explicit clear gesture is a body-contract change and its own ticket.
func validateCapacityBound(capacity *int) error {
	if capacity == nil {
		return nil
	}
	if *capacity < contracts.MinVenueCapacity || *capacity > contracts.MaxVenueCapacity {
		return huma.Error422UnprocessableEntity(fmt.Sprintf(
			"Capacity must be between %d and %d", contracts.MinVenueCapacity, contracts.MaxVenueCapacity))
	}
	return nil
}

// AdminCreateVenueHandler handles POST /admin/venues - creates a venue directly (admin only)
func (h *VenueHandler) AdminCreateVenueHandler(ctx context.Context, req *AdminCreateVenueRequest) (*AdminCreateVenueResponse, error) {
	requestID := logger.GetRequestID(ctx)

	user := middleware.GetUserFromContext(ctx)

	// PSY-525: URL scheme validation (http/https only) for social URL fields.
	if err := shared.ValidateSocialURLs(req.Body.Instagram, req.Body.Facebook, req.Body.Twitter,
		req.Body.YouTube, req.Body.Spotify, req.Body.SoundCloud, req.Body.Bandcamp, req.Body.Website); err != nil {
		return nil, err
	}
	if err := validateCapacityBound(req.Body.Capacity); err != nil {
		return nil, err
	}

	// Build service request
	serviceReq := &contracts.CreateVenueRequest{
		Name:        req.Body.Name,
		City:        req.Body.City,
		State:       req.Body.State,
		Country:     req.Body.Country,
		Address:     req.Body.Address,
		Zipcode:     req.Body.Zipcode,
		Capacity:    req.Body.Capacity,
		AgePolicy:   req.Body.AgePolicy,
		Description: req.Body.Description,
		Instagram:   req.Body.Instagram,
		Facebook:    req.Body.Facebook,
		Twitter:     req.Body.Twitter,
		YouTube:     req.Body.YouTube,
		Spotify:     req.Body.Spotify,
		SoundCloud:  req.Body.SoundCloud,
		Bandcamp:    req.Body.Bandcamp,
		Website:     req.Body.Website,
		SubmittedBy: &user.ID,
	}

	venue, err := h.venueService.CreateVenue(serviceReq, true)
	if err != nil {
		logger.FromContext(ctx).Error("admin_create_venue_failed",
			"error", err.Error(),
			"admin_id", user.ID,
			"request_id", requestID,
		)
		return nil, huma.Error422UnprocessableEntity(
			fmt.Sprintf("Failed to create venue: %s", err.Error()),
		)
	}

	// Audit log (fire and forget)
	if h.auditLogService != nil {
		servicesshared.GoSafe(ctx, "audit_log", func() {
			h.auditLogService.LogAction(user.ID, "create_venue", "venue", venue.ID, map[string]interface{}{
				"name":  venue.Name,
				"city":  venue.City,
				"state": venue.State,
			})
		})
	}

	logger.FromContext(ctx).Info("admin_venue_created",
		"venue_id", venue.ID,
		"venue_slug", venue.Slug,
		"admin_id", user.ID,
		"request_id", requestID,
	)

	return &AdminCreateVenueResponse{Body: venue}, nil
}

// ============================================================================
// Venue Editing Handlers
// ============================================================================

// UpdateVenueRequest represents the HTTP request for updating a venue
// All body fields are optional - only changed fields need to be sent
type UpdateVenueRequest struct {
	VenueID string `path:"venue_id" validate:"required" doc:"Venue ID"`
	Body    struct {
		Name        *string `json:"name,omitempty" required:"false" doc:"Venue name"`
		Address     *string `json:"address,omitempty" required:"false" doc:"Venue address"`
		City        *string `json:"city,omitempty" required:"false" doc:"Venue city"`
		State       *string `json:"state,omitempty" required:"false" doc:"Venue state"`
		Country     *string `json:"country,omitempty" required:"false" doc:"Venue country"`
		Zipcode     *string `json:"zipcode,omitempty" required:"false" doc:"Venue zipcode"`
		Capacity    *int    `json:"capacity,omitempty" required:"false" minimum:"1" maximum:"200000" doc:"Venue capacity"`
		AgePolicy   *string `json:"age_policy,omitempty" required:"false" doc:"House-default age policy, e.g. all ages, 17+, 21+ (max 100)"`
		Instagram   *string `json:"instagram,omitempty" required:"false" doc:"Instagram URL"`
		Facebook    *string `json:"facebook,omitempty" required:"false" doc:"Facebook URL"`
		Twitter     *string `json:"twitter,omitempty" required:"false" doc:"Twitter URL"`
		YouTube     *string `json:"youtube,omitempty" required:"false" doc:"YouTube URL"`
		Spotify     *string `json:"spotify,omitempty" required:"false" doc:"Spotify URL"`
		SoundCloud  *string `json:"soundcloud,omitempty" required:"false" doc:"SoundCloud URL"`
		Bandcamp    *string `json:"bandcamp,omitempty" required:"false" doc:"Bandcamp URL"`
		Website     *string `json:"website,omitempty" required:"false" doc:"Website URL"`
		Description *string `json:"description,omitempty" required:"false" doc:"Markdown description (max 5000 chars)"`
		ImageURL    *string `json:"image_url,omitempty" required:"false" doc:"Venue photo URL (max 2048 chars)"`
		Summary     *string `json:"summary,omitempty" required:"false" doc:"Revision summary describing the change"`
	}
}

// UpdateVenueResponse represents the HTTP response for updating a venue.
type UpdateVenueResponse struct {
	Body *contracts.VenueDetailResponse
}

// UpdateVenueHandler handles PUT /venues/{venue_id} — admin-only direct update.
// Non-admin users (including venue submitters) go through PUT /venues/{id}/suggest-edit,
// which routes through the unified pending_entity_edits queue.
func (h *VenueHandler) UpdateVenueHandler(ctx context.Context, req *UpdateVenueRequest) (*UpdateVenueResponse, error) {
	requestID := logger.GetRequestID(ctx)

	user := middleware.GetUserFromContext(ctx)

	// Parse venue ID
	venueID, err := strconv.ParseUint(req.VenueID, 10, 32)
	if err != nil {
		return nil, huma.Error400BadRequest("Invalid venue ID")
	}

	// Validate required fields aren't being set to empty strings
	if req.Body.Name != nil && *req.Body.Name == "" {
		return nil, huma.Error422UnprocessableEntity("Venue name cannot be empty")
	}
	if req.Body.City != nil && *req.Body.City == "" {
		return nil, huma.Error422UnprocessableEntity("City cannot be empty")
	}
	if req.Body.State != nil && *req.Body.State == "" {
		return nil, huma.Error422UnprocessableEntity("State cannot be empty")
	}
	if req.Body.Description != nil && len(*req.Body.Description) > 5000 {
		return nil, huma.Error422UnprocessableEntity("Description must be 5000 characters or fewer")
	}
	// Bound the free-text age policy. Mirrors the Description check rather than
	// the create body's schema tag, matching this handler's existing convention
	// of validating body lengths inline. Counts RUNES so this agrees with both
	// the column (VARCHAR counts characters) and the create body's maxLength
	// tag (huma counts runes); len() would reject a legal CJK policy here that
	// create accepts.
	if req.Body.AgePolicy != nil && utf8.RuneCountInString(*req.Body.AgePolicy) > contracts.MaxVenueAgePolicyLength {
		return nil, huma.Error422UnprocessableEntity(
			fmt.Sprintf("Age policy must be %d characters or fewer", contracts.MaxVenueAgePolicyLength))
	}
	if err := validateCapacityBound(req.Body.Capacity); err != nil {
		return nil, err
	}

	// PSY-525 scheme check + PSY-1675 SSRF host guard (resolves DNS; see urlguard)
	// for image_url; scheme + host anchor for the social URL fields.
	// Length check first (cheaper, reports bytes); URL scheme check second.
	if req.Body.ImageURL != nil && len(*req.Body.ImageURL) > 2048 {
		return nil, huma.Error422UnprocessableEntity("Image URL must be 2048 characters or fewer")
	}
	if err := shared.ValidateImageURL(ctx, req.Body.ImageURL); err != nil {
		return nil, err
	}
	if err := shared.ValidateSocialURLs(req.Body.Instagram, req.Body.Facebook, req.Body.Twitter,
		req.Body.YouTube, req.Body.Spotify, req.Body.SoundCloud, req.Body.Bandcamp, req.Body.Website); err != nil {
		return nil, err
	}

	logger.FromContext(ctx).Info("admin_venue_update",
		"venue_id", venueID,
		"admin_id", user.ID,
		"request_id", requestID,
	)

	// Capture old values for revision diff (fire-and-forget safe)
	var oldVenue *contracts.VenueDetailResponse
	if h.revisionService != nil {
		oldVenue, _ = h.venueService.GetVenue(uint(venueID))
	}

	// Image URL length + scheme already validated above; the service
	// normalizes empty Description/ImageURL to SQL NULL.
	serviceReq := &contracts.UpdateVenueRequest{
		Name:        req.Body.Name,
		Address:     req.Body.Address,
		City:        req.Body.City,
		State:       req.Body.State,
		Country:     req.Body.Country,
		Zipcode:     req.Body.Zipcode,
		Capacity:    req.Body.Capacity,
		AgePolicy:   req.Body.AgePolicy,
		Description: req.Body.Description,
		ImageURL:    req.Body.ImageURL,
		Instagram:   req.Body.Instagram,
		Facebook:    req.Body.Facebook,
		Twitter:     req.Body.Twitter,
		YouTube:     req.Body.YouTube,
		Spotify:     req.Body.Spotify,
		SoundCloud:  req.Body.SoundCloud,
		Bandcamp:    req.Body.Bandcamp,
		Website:     req.Body.Website,
	}

	updatedVenue, err := h.venueService.UpdateVenue(uint(venueID), serviceReq)
	if err != nil {
		var venueErr *apperrors.VenueError
		if errors.As(err, &venueErr) && venueErr.Code == apperrors.CodeVenueNotFound {
			return nil, huma.Error404NotFound("Venue not found")
		}
		logger.FromContext(ctx).Error("admin_venue_update_failed",
			"venue_id", venueID,
			"error", err.Error(),
			"request_id", requestID,
		)
		return nil, huma.Error422UnprocessableEntity(
			fmt.Sprintf("Failed to update venue (request_id: %s)", requestID),
		)
	}

	// Record revision (fire and forget)
	if h.revisionService != nil && oldVenue != nil {
		servicesshared.GoSafe(ctx, "record_revision", func() {
			changes := revisiondiff.Compare(oldVenue, updatedVenue, revisiondiff.VenueFields)
			if len(changes) > 0 {
				summary := ""
				if req.Body.Summary != nil {
					summary = *req.Body.Summary
				}
				if err := h.revisionService.RecordRevision("venue", uint(venueID), user.ID, changes, summary); err != nil {
					logger.Default().Error("record_venue_revision_failed",
						"venue_id", venueID,
						"error", err.Error(),
					)
				}
			}
		})
	}

	return &UpdateVenueResponse{Body: updatedVenue}, nil
}

// ============================================================================
// Venue Deletion Handlers
// ============================================================================

// DeleteVenueRequest represents the request for deleting a venue
type DeleteVenueRequest struct {
	VenueID string `path:"venue_id" validate:"required" doc:"Venue ID"`
}

// DeleteVenueResponse represents the response for deleting a venue
type DeleteVenueResponse struct {
	Body struct {
		Message string `json:"message" doc:"Success message"`
	}
}

// DeleteVenueHandler handles DELETE /venues/{venue_id}
// Admin: Can delete any venue
// Non-admin: Can delete venues they submitted (via submitted_by field)
// Constraint: Venues with associated shows cannot be deleted
func (h *VenueHandler) DeleteVenueHandler(ctx context.Context, req *DeleteVenueRequest) (*DeleteVenueResponse, error) {
	requestID := logger.GetRequestID(ctx)

	// Get authenticated user
	user := middleware.GetUserFromContext(ctx)
	if user == nil {
		return nil, huma.Error401Unauthorized("Authentication required")
	}

	// Parse venue ID
	venueID, err := strconv.ParseUint(req.VenueID, 10, 32)
	if err != nil {
		return nil, huma.Error400BadRequest("Invalid venue ID")
	}

	// Get the venue to check ownership
	venue, err := h.venueService.GetVenueModel(uint(venueID))
	if err != nil {
		var venueErr *apperrors.VenueError
		if errors.As(err, &venueErr) && venueErr.Code == apperrors.CodeVenueNotFound {
			return nil, huma.Error404NotFound("Venue not found")
		}
		return nil, huma.Error500InternalServerError(
			fmt.Sprintf("Failed to get venue (request_id: %s)", requestID),
		)
	}

	// Check permissions: admin can delete any venue, non-admin can only delete their own
	if !user.IsAdmin {
		if venue.SubmittedBy == nil || *venue.SubmittedBy != user.ID {
			logger.FromContext(ctx).Warn("venue_delete_forbidden",
				"venue_id", venueID,
				"user_id", user.ID,
				"venue_submitted_by", venue.SubmittedBy,
				"request_id", requestID,
			)
			return nil, huma.Error403Forbidden("You can only delete venues you submitted")
		}
	}

	logger.FromContext(ctx).Info("venue_delete_attempt",
		"venue_id", venueID,
		"user_id", user.ID,
		"is_admin", user.IsAdmin,
		"request_id", requestID,
	)

	// Delete the venue (service checks for associated shows)
	if err := h.venueService.DeleteVenue(uint(venueID)); err != nil {
		logger.FromContext(ctx).Error("venue_delete_failed",
			"venue_id", venueID,
			"user_id", user.ID,
			"error", err.Error(),
			"request_id", requestID,
		)

		// Check if the error is due to associated shows
		var venueErr *apperrors.VenueError
		if errors.As(err, &venueErr) && venueErr.Code == apperrors.CodeVenueHasShows {
			return nil, huma.Error422UnprocessableEntity(venueErr.Message)
		}

		return nil, huma.Error500InternalServerError(
			fmt.Sprintf("Failed to delete venue (request_id: %s)", requestID),
		)
	}

	logger.FromContext(ctx).Info("venue_deleted",
		"venue_id", venueID,
		"user_id", user.ID,
		"is_admin", user.IsAdmin,
		"request_id", requestID,
	)

	return &DeleteVenueResponse{
		Body: struct {
			Message string `json:"message" doc:"Success message"`
		}{
			Message: "Venue deleted successfully",
		},
	}, nil
}

// ============================================================================
// Get Venue Genres
// ============================================================================

// GetVenueGenresRequest represents the request for getting a venue's genre profile.
type GetVenueGenresRequest struct {
	VenueID string `path:"venue_id" doc:"Venue ID or slug" example:"the-rebel-lounge-phoenix-az"`
}

// GetVenueGenresResponse represents the response for venue genre profile.
type GetVenueGenresResponse struct {
	Body *contracts.VenueGenreResponse
}

// GetVenueGenresHandler handles GET /venues/{venue_id}/genres — returns top genre tags for a venue.
func (h *VenueHandler) GetVenueGenresHandler(ctx context.Context, req *GetVenueGenresRequest) (*GetVenueGenresResponse, error) {
	// Resolve venue by ID or slug
	var venueID uint
	if id, err := strconv.ParseUint(req.VenueID, 10, 32); err == nil {
		venueID = uint(id)
	} else {
		// Try slug lookup
		venue, err := h.venueService.GetVenueBySlug(req.VenueID)
		if err != nil {
			return nil, huma.Error404NotFound("Venue not found")
		}
		venueID = venue.ID
	}

	genres, err := h.venueService.GetVenueGenreProfile(venueID)
	if err != nil {
		return nil, huma.Error500InternalServerError("Failed to get venue genre profile", err)
	}
	if genres == nil {
		genres = []contracts.GenreCount{}
	}

	return &GetVenueGenresResponse{
		Body: &contracts.VenueGenreResponse{
			Genres: genres,
		},
	}, nil
}

// ============================================================================
// Get Venue Bill Network (PSY-365)
// ============================================================================

// GetVenueBillNetworkRequest is the request shape for the venue co-bill graph.
//
// `Window` accepts "all" (default), "12m" (rolling last 12 months), or "year"
// (paired with `Year`). Unknown values are coerced to "all" by the service so
// a client mistake degrades gracefully rather than 500ing.
//
// `Year` is required when Window=="year". Huma forbids pointer query params,
// so we use an int with the zero-value sentinel; the handler validates
// presence before passing to the service.
type GetVenueBillNetworkRequest struct {
	VenueID string `path:"venue_id" doc:"Venue ID or slug" example:"valley-bar-phoenix-az"`
	Window  string `query:"window" doc:"Time window: 'all' (default), '12m' (rolling), or 'year' (with year=YYYY)" example:"all" enum:"all,12m,year"`
	Year    int    `query:"year" doc:"Calendar year for window=year (required when window=year)" example:"2025" minimum:"2000" maximum:"2100"`
}

// GetVenueBillNetworkResponse wraps the contracts payload for huma.
type GetVenueBillNetworkResponse struct {
	Body *contracts.VenueBillNetworkResponse
}

// GetVenueBillNetworkHandler handles GET /venues/{venue_id}/bill-network.
//
// PSY-365 — venue-rooted co-bill graph. Mirrors GET /scenes/{slug}/graph in
// shape and intent (same shared frontend component renders both), with the
// scope narrowed to a single venue and edges weighted by AT-VENUE shared
// shows rather than global ones.
func (h *VenueHandler) GetVenueBillNetworkHandler(ctx context.Context, req *GetVenueBillNetworkRequest) (*GetVenueBillNetworkResponse, error) {
	// Resolve venue by ID or slug — same pattern as /venues/{id}/genres.
	var venueID uint
	if id, err := strconv.ParseUint(req.VenueID, 10, 32); err == nil {
		venueID = uint(id)
	} else {
		venue, err := h.venueService.GetVenueBySlug(req.VenueID)
		if err != nil {
			return nil, huma.Error404NotFound("Venue not found")
		}
		venueID = venue.ID
	}

	// Validate window=year requires year. Empty / "all" / "12m" pass through.
	var yearPtr *int
	window := strings.ToLower(strings.TrimSpace(req.Window))
	if window == "year" {
		if req.Year == 0 {
			return nil, huma.Error422UnprocessableEntity("Year is required when window=year")
		}
		y := req.Year
		yearPtr = &y
	}

	graph, err := h.venueService.GetVenueBillNetwork(venueID, window, yearPtr)
	if err != nil {
		var venueErr *apperrors.VenueError
		if errors.As(err, &venueErr) && venueErr.Code == apperrors.CodeVenueNotFound {
			return nil, huma.Error404NotFound("Venue not found")
		}
		return nil, huma.Error500InternalServerError("Failed to get venue bill network", err)
	}

	return &GetVenueBillNetworkResponse{Body: graph}, nil
}
