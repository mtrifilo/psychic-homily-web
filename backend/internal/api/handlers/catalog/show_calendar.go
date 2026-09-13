package catalog

import (
	"context"
	"strings"

	"github.com/danielgtaylor/huma/v2"

	"psychic-homily-backend/internal/api/middleware"
	"psychic-homily-backend/internal/logger"
	"psychic-homily-backend/internal/services/contracts"
	servicesshared "psychic-homily-backend/internal/services/shared"
)

// The date-addressed half of the /shows list: an offset-paged window of the
// upcoming partition, and the month histogram that supplies its jump targets.
//
// Both are SIBLINGS of GET /shows/upcoming rather than modes of it. That
// endpoint's contract is a cursor, and a cursor and an offset are two mutually
// exclusive ways to name a position that one operation cannot publish without
// leaving every generated client free to send both. They are also not modes of
// GET /shows, whose from_date/to_date are matched against the stored UTC instant
// verbatim and whose set includes past shows.
//
// maxUpcomingShowCities, the city cap and the tag parsing are shared with the
// cursor endpoint through parseUpcomingShowsFilter, so the three surfaces cannot
// disagree about what a filter set selects.

// maxUpcomingShowCities caps the multi-city filter on every reader of the
// upcoming partition. Each city adds an OR branch to the predicate, so the cap
// bounds the planner's work on an anonymous public read.
const maxUpcomingShowCities = 10

// parseUpcomingShowsFilter builds the service filter from the query parameters
// shared by the cursor list, the calendar window and the month histogram.
//
// Returns nil when nothing was asked for, which every caller reads as "no
// filter". `cities` wins over the legacy city/state pair when both are present;
// a `cities` value with no well-formed pair in it filters nothing rather than
// matching nothing, because a malformed filter that silently empties the list is
// indistinguishable to the reader from a quiet week.
func parseUpcomingShowsFilter(cities, city, state, tags, tagMatch string) *contracts.UpcomingShowsFilter {
	var filters *contracts.UpcomingShowsFilter

	if cities != "" {
		// Pipe-delimited multi-city param: "Phoenix,AZ|Mesa,AZ"
		var cityFilters []contracts.CityStateFilter
		for _, pair := range strings.Split(cities, "|") {
			parts := strings.SplitN(pair, ",", 2)
			if len(parts) == 2 && parts[0] != "" && parts[1] != "" {
				cityFilters = append(cityFilters, contracts.CityStateFilter{
					City:  strings.TrimSpace(parts[0]),
					State: strings.TrimSpace(parts[1]),
				})
			}
		}
		if len(cityFilters) > maxUpcomingShowCities {
			cityFilters = cityFilters[:maxUpcomingShowCities]
		}
		if len(cityFilters) > 0 {
			filters = &contracts.UpcomingShowsFilter{Cities: cityFilters}
		}
	} else if city != "" || state != "" {
		filters = &contracts.UpcomingShowsFilter{City: city, State: state}
	}

	if tf := parseTagFilter(tags, tagMatch); tf.HasTags() {
		if filters == nil {
			filters = &contracts.UpcomingShowsFilter{}
		}
		filters.TagSlugs = tf.TagSlugs
		filters.TagMatchAny = tf.MatchAny
	}

	return filters
}

// GetShowsCalendarRequest represents the HTTP request for an offset page of the
// upcoming shows list, optionally narrowed to one venue-local month or date.
type GetShowsCalendarRequest struct {
	Year     int    `query:"year" minimum:"0" maximum:"9999" doc:"Venue-local calendar year of the window. Omit (or 0) with month and day for the whole upcoming list."`
	Month    int    `query:"month" minimum:"0" maximum:"12" doc:"Venue-local calendar month, 1-12. Requires year."`
	Day      int    `query:"day" minimum:"0" maximum:"31" doc:"Venue-local calendar day of month, 1-31. Requires year and month."`
	Limit    int    `query:"limit" default:"50" minimum:"1" maximum:"200" doc:"Number of shows per page (max 200). Defaults to 50."`
	Offset   int    `query:"offset" default:"0" minimum:"0" doc:"Offset for pagination"`
	City     string `query:"city" doc:"Filter by city name (exact match). Legacy — prefer 'cities' param."`
	State    string `query:"state" doc:"Filter by state code (exact match, e.g., 'AZ'). Legacy — prefer 'cities' param."`
	Cities   string `query:"cities" doc:"Filter by multiple cities. Pipe-delimited pairs: 'Phoenix,AZ|Mesa,AZ|Tucson,AZ'. Max 10 cities."`
	Tags     string `query:"tags" doc:"Comma-separated tag slugs. AND by default; set tag_match=any for OR." example:"post-punk,phoenix"`
	TagMatch string `query:"tag_match" doc:"Tag matching mode: 'all' (default, AND) or 'any' (OR)" example:"all" enum:"all,any"`
}

// GetShowsCalendarResponse represents the HTTP response for the windowed
// upcoming shows list.
//
// The window is echoed back for the reason the venue archive echoes its year: a
// client that built the request from a URL segment needs to be able to assert
// that the page it rendered is the page it addressed.
type GetShowsCalendarResponse struct {
	Body struct {
		Shows  []*contracts.ShowResponse `json:"shows" doc:"Page of upcoming shows in the requested window, soonest first"`
		Total  int64                     `json:"total" doc:"Upcoming shows matching the window and filters, across all pages"`
		Limit  int                       `json:"limit" doc:"Limit used in query"`
		Offset int                       `json:"offset" doc:"Offset used in query"`
		Year   int                       `json:"year" doc:"Venue-local year the window was taken on, 0 when unwindowed"`
		Month  int                       `json:"month" doc:"Venue-local month the window was taken on, 0 when unwindowed"`
		Day    int                       `json:"day" doc:"Venue-local day the window was taken on, 0 when the window is a whole month or unwindowed"`
	}
}

// GetShowsCalendarHandler handles GET /shows/calendar - one offset page of the
// upcoming list, optionally narrowed to a venue-local month or date.
//
// An empty window with rows behind it and an empty window with none are both
// 200s with total 0. Whether an addressable month that has no shows is a page or
// a 404 is the frontend route's call, and it has the total it needs to make it.
func (h *ShowHandler) GetShowsCalendarHandler(ctx context.Context, req *GetShowsCalendarRequest) (*GetShowsCalendarResponse, error) {
	window := contracts.ShowCalendarWindow{Year: req.Year, Month: req.Month, Day: req.Day}
	if err := validateShowCalendarWindow(window); err != nil {
		return nil, err
	}

	// Reads the same viewer the cursor endpoint does, so a reader cannot see one
	// set of shows on the paged list and another on the feed.
	user := middleware.GetUserFromContext(ctx)
	includeNonApproved := user != nil && user.IsAdmin

	limit := req.Limit
	if limit < 1 {
		limit = defaultShowListLimit
	}
	if limit > maxShowListLimit {
		limit = maxShowListLimit
	}

	filters := parseUpcomingShowsFilter(req.Cities, req.City, req.State, req.Tags, req.TagMatch)

	shows, total, err := h.showService.GetUpcomingShowsPage(
		contracts.ShowCalendarQuery{ShowCalendarWindow: window, Limit: limit, Offset: req.Offset},
		includeNonApproved,
		filters,
	)
	if err != nil {
		requestID := logger.GetRequestID(ctx)
		logger.FromContext(ctx).Error("shows_calendar_failed",
			"error", err.Error(),
			"request_id", requestID,
		)
		return nil, huma.Error500InternalServerError("Failed to get upcoming shows")
	}

	resp := &GetShowsCalendarResponse{}
	resp.Body.Shows = shows
	resp.Body.Total = total
	resp.Body.Limit = limit
	resp.Body.Offset = req.Offset
	resp.Body.Year = req.Year
	resp.Body.Month = req.Month
	resp.Body.Day = req.Day
	return resp, nil
}

// validateShowCalendarWindow refuses the window shapes the service cannot
// distinguish from "no window".
//
// A half-stated window is a client error, not a wider list: `day=14` with no
// month, or `month=11` with no year, would otherwise fall through to the whole
// upcoming catalog under a URL that promises one day of it. 31 February is
// refused on the same grounds — it names no date, and silently returning every
// upcoming show for it would make a typo look like a working page.
func validateShowCalendarWindow(window contracts.ShowCalendarWindow) error {
	switch {
	case window.Year <= 0 && window.Month <= 0 && window.Day <= 0:
		return nil
	case window.Year <= 0:
		return huma.Error422UnprocessableEntity("month and day require a year")
	case window.Month <= 0:
		return huma.Error422UnprocessableEntity("year requires a month")
	case window.Day <= 0:
		return nil
	case !servicesshared.ValidCalendarDate(window.Year, window.Month, window.Day):
		return huma.Error422UnprocessableEntity("year, month and day do not name a real calendar date")
	default:
		return nil
	}
}

// GetShowMonthsRequest represents the HTTP request for the catalog-wide upcoming
// month histogram. It takes the list's filters and no window: the histogram is
// what enumerates the months.
type GetShowMonthsRequest struct {
	City     string `query:"city" doc:"Filter by city name (exact match). Legacy — prefer 'cities' param."`
	State    string `query:"state" doc:"Filter by state code (exact match, e.g., 'AZ'). Legacy — prefer 'cities' param."`
	Cities   string `query:"cities" doc:"Filter by multiple cities. Pipe-delimited pairs: 'Phoenix,AZ|Mesa,AZ|Tucson,AZ'. Max 10 cities."`
	Tags     string `query:"tags" doc:"Comma-separated tag slugs. AND by default; set tag_match=any for OR." example:"post-punk,phoenix"`
	TagMatch string `query:"tag_match" doc:"Tag matching mode: 'all' (default, AND) or 'any' (OR)" example:"all" enum:"all,any"`
}

// GetShowMonthsResponse represents the HTTP response for the upcoming month
// histogram.
type GetShowMonthsResponse struct {
	// CacheControl: public, viewer-independent and stale-tolerant, on the same
	// terms and the same window as the venue and artist month histograms.
	CacheControl string `header:"Cache-Control"`
	Body         struct {
		Months []contracts.ShowMonthCount `json:"months" doc:"Venue-local calendar months that have at least one upcoming show, soonest first"`
		Total  int64                      `json:"total" doc:"Sum of the month counts, which is the unwindowed total of the list under the same filters"`
	}
}

// GetShowMonthsHandler handles GET /shows/months - the venue-local month
// histogram of the upcoming list under the list's own filters.
//
// Total is the SUM of the buckets rather than a second COUNT. The number a
// caller needs from this payload is the one its own bars add up to: a
// separately-counted total could disagree with the bars beside it, and a pager
// labelled from bars that do not sum to the list it labels mislabels every page
// after the first divergence.
func (h *ShowHandler) GetShowMonthsHandler(ctx context.Context, req *GetShowMonthsRequest) (*GetShowMonthsResponse, error) {
	user := middleware.GetUserFromContext(ctx)
	includeNonApproved := user != nil && user.IsAdmin

	filters := parseUpcomingShowsFilter(req.Cities, req.City, req.State, req.Tags, req.TagMatch)

	months, err := h.showService.GetUpcomingShowMonths(includeNonApproved, filters)
	if err != nil {
		requestID := logger.GetRequestID(ctx)
		logger.FromContext(ctx).Error("shows_months_failed",
			"error", err.Error(),
			"request_id", requestID,
		)
		return nil, huma.Error500InternalServerError("Failed to count upcoming shows by month")
	}

	var total int64
	for _, month := range months {
		total += month.Count
	}

	resp := &GetShowMonthsResponse{CacheControl: showMonthHistogramCacheControl}
	resp.Body.Months = months
	resp.Body.Total = total
	return resp, nil
}
