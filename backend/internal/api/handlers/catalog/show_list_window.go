package catalog

import (
	"context"

	"github.com/danielgtaylor/huma/v2"

	"psychic-homily-backend/internal/logger"
	"psychic-homily-backend/internal/services/contracts"
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
// The filters, the page bounds and the viewer test come from show_list_filters.go,
// shared with the cursor endpoint.

// GetShowsCalendarRequest represents the HTTP request for an offset page of the
// upcoming shows list, optionally narrowed to one venue-local month or date.
type GetShowsCalendarRequest struct {
	Year     int    `query:"year" minimum:"0" maximum:"9999" doc:"Venue-local calendar year of the window. Omit (or 0) with month and day for the whole upcoming list."`
	Month    int    `query:"month" minimum:"0" maximum:"12" doc:"Venue-local calendar month, 1-12. Requires year."`
	Day      int    `query:"day" minimum:"0" maximum:"31" doc:"Venue-local calendar day of month, 1-31. Requires year and month."`
	Days     int    `query:"days" minimum:"0" maximum:"14" doc:"Length in venue-local days of a run beginning on the requested day, 1-14. Requires year, month and day; omit (or 0 or 1) for that day alone."`
	Limit    int    `query:"limit" default:"50" minimum:"1" maximum:"200" doc:"Number of shows per page (max 200). Defaults to 50."`
	Offset   int    `query:"offset" default:"0" minimum:"0" doc:"Offset for pagination"`
	City     string `query:"city" doc:"Filter by city name (exact match). Legacy, prefer 'cities' param."`
	State    string `query:"state" doc:"Filter by state code (exact match, e.g., 'AZ'). Legacy, prefer 'cities' param."`
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
		Days   int                       `json:"days" doc:"Run length echoed back exactly as requested, 0 when the request named none. A run of 1 reads the same rows as the bare day and is echoed as 1"`
	}
}

// GetShowsCalendarHandler handles GET /shows/calendar - one offset page of the
// upcoming list, optionally narrowed to a venue-local month or date.
//
// An empty window with rows behind it and an empty window with none are both
// 200s with total 0. Whether an addressable month that has no shows is a page or
// a 404 is the frontend route's call, and it has the total it needs to make it.
func (h *ShowHandler) GetShowsCalendarHandler(ctx context.Context, req *GetShowsCalendarRequest) (*GetShowsCalendarResponse, error) {
	window := contracts.ShowCalendarWindow{Year: req.Year, Month: req.Month, Day: req.Day, Days: req.Days}
	// A half-stated window is a client error, not a wider list. The rule and the
	// message both live on the window type; the service refuses the same shapes
	// for a caller that never passes through here.
	if err := window.Validate(); err != nil {
		return nil, huma.Error422UnprocessableEntity(err.Error())
	}

	// Both page bounds are resolved here so the envelope echoes the page that was
	// actually read. The service floors a negative offset too; echoing the raw
	// request value would tell a client it is on a page nothing served.
	limit := clampShowListLimit(req.Limit)
	offset := req.Offset
	if offset < 0 {
		offset = 0
	}
	filters := parseUpcomingShowsFilter(req.Cities, req.City, req.State, req.Tags, req.TagMatch)

	shows, total, err := h.showService.GetUpcomingShowsPage(
		contracts.ShowCalendarQuery{ShowCalendarWindow: window, Limit: limit, Offset: offset},
		upcomingListIncludesNonApproved(ctx),
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
	resp.Body.Offset = offset
	resp.Body.Year = window.Year
	resp.Body.Month = window.Month
	resp.Body.Day = window.Day
	resp.Body.Days = window.Days
	return resp, nil
}

// GetShowMonthsRequest represents the HTTP request for the catalog-wide upcoming
// month histogram. It takes the list's filters and no window: the histogram is
// what enumerates the months.
type GetShowMonthsRequest struct {
	City     string `query:"city" doc:"Filter by city name (exact match). Legacy, prefer 'cities' param."`
	State    string `query:"state" doc:"Filter by state code (exact match, e.g., 'AZ'). Legacy, prefer 'cities' param."`
	Cities   string `query:"cities" doc:"Filter by multiple cities. Pipe-delimited pairs: 'Phoenix,AZ|Mesa,AZ|Tucson,AZ'. Max 10 cities."`
	Tags     string `query:"tags" doc:"Comma-separated tag slugs. AND by default; set tag_match=any for OR." example:"post-punk,phoenix"`
	TagMatch string `query:"tag_match" doc:"Tag matching mode: 'all' (default, AND) or 'any' (OR)" example:"all" enum:"all,any"`
}

// upcomingMonthHistogramCacheControl is PRIVATE, and the sixty seconds match the
// window the venue and artist month histograms use.
//
// Private because GetShowMonthsHandler computes this body from
// upcomingListIncludesNonApproved: a response whose contents depend on who asked
// is not shareable, whatever any one caller currently resolves to. It is NOT an
// abuse control, and nothing here bounds repeat hits on the aggregate.
const upcomingMonthHistogramCacheControl = "private, max-age=60"

// GetShowMonthsResponse represents the HTTP response for the upcoming month
// histogram.
type GetShowMonthsResponse struct {
	// CacheControl: see upcomingMonthHistogramCacheControl for why this one is
	// private where its venue and artist twins are public.
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
	filters := parseUpcomingShowsFilter(req.Cities, req.City, req.State, req.Tags, req.TagMatch)

	months, err := h.showService.GetUpcomingShowMonths(upcomingListIncludesNonApproved(ctx), filters)
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

	resp := &GetShowMonthsResponse{CacheControl: upcomingMonthHistogramCacheControl}
	resp.Body.Months = months
	resp.Body.Total = total
	return resp, nil
}
