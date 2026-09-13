package catalog

import (
	"context"
	"errors"
	"net/http"
	"reflect"
	"testing"

	"github.com/danielgtaylor/huma/v2"

	"psychic-homily-backend/internal/api/handlers/shared/testhelpers"
	"psychic-homily-backend/internal/services/contracts"
)

// newCalendarShowHandler wires a ShowHandler over a mock show service. Only the
// show service is exercised by the two calendar handlers, so the rest stay nil.
func newCalendarShowHandler(mock *testhelpers.MockShowService) *ShowHandler {
	return NewShowHandler(mock, nil, nil, nil, nil, nil, nil, nil)
}

// A half-stated window is a client error rather than a wider list. The service's
// own answer for one is "no narrowing", so a handler that passed it through
// would serve the whole upcoming catalog under a URL promising one day of it.
func TestGetShowsCalendarHandler_RefusesHalfStatedWindows(t *testing.T) {
	for _, tc := range []struct {
		name             string
		year, month, day int
	}{
		{"day with no month", 0, 0, 14},
		{"day and month with no year", 0, 11, 14},
		{"month with no year", 0, 11, 0},
		{"year with no month", 2026, 0, 0},
		{"year and day with no month", 2026, 0, 14},
		{"february 31", 2027, 2, 31},
		{"leap day in a common year", 2027, 2, 29},
	} {
		t.Run(tc.name, func(t *testing.T) {
			called := false
			mock := &testhelpers.MockShowService{
				GetUpcomingShowsPageFn: func(contracts.ShowCalendarQuery, bool, *contracts.UpcomingShowsFilter) ([]*contracts.ShowResponse, int64, error) {
					called = true
					return nil, 0, nil
				},
			}

			_, err := newCalendarShowHandler(mock).GetShowsCalendarHandler(context.Background(),
				&GetShowsCalendarRequest{Year: tc.year, Month: tc.month, Day: tc.day, Limit: 50})

			var status huma.StatusError
			if !errors.As(err, &status) {
				t.Fatalf("expected a huma.StatusError, got %T (%v)", err, err)
			}
			if status.GetStatus() != http.StatusUnprocessableEntity {
				t.Errorf("expected 422, got %d", status.GetStatus())
			}
			if called {
				t.Error("a refused window must not reach the service")
			}
		})
	}
}

// The shapes the window legitimately takes: none, a whole month, one date.
func TestGetShowsCalendarHandler_AcceptsWholeWindows(t *testing.T) {
	for _, tc := range []struct {
		name             string
		year, month, day int
	}{
		{"no window", 0, 0, 0},
		{"a whole month", 2026, 11, 0},
		{"one date", 2026, 11, 14},
		{"a leap day", 2028, 2, 29},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var got contracts.ShowCalendarQuery
			mock := &testhelpers.MockShowService{
				GetUpcomingShowsPageFn: func(query contracts.ShowCalendarQuery, _ bool, _ *contracts.UpcomingShowsFilter) ([]*contracts.ShowResponse, int64, error) {
					got = query
					return nil, 7, nil
				},
			}

			resp, err := newCalendarShowHandler(mock).GetShowsCalendarHandler(context.Background(),
				&GetShowsCalendarRequest{Year: tc.year, Month: tc.month, Day: tc.day, Limit: 50, Offset: 100})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if got.Year != tc.year || got.Month != tc.month || got.Day != tc.day {
				t.Errorf("window reached the service as %+v, want %d-%d-%d", got.ShowCalendarWindow, tc.year, tc.month, tc.day)
			}
			// The window is echoed so a client built from URL segments can assert
			// the page it rendered is the page it addressed.
			if resp.Body.Year != tc.year || resp.Body.Month != tc.month || resp.Body.Day != tc.day {
				t.Errorf("response echoed %d-%d-%d, want %d-%d-%d",
					resp.Body.Year, resp.Body.Month, resp.Body.Day, tc.year, tc.month, tc.day)
			}
			if resp.Body.Total != 7 || resp.Body.Limit != 50 || resp.Body.Offset != 100 {
				t.Errorf("envelope mismatch: %+v", resp.Body)
			}
		})
	}
}

// A caller-supplied limit is capped in the handler as well as by the schema,
// because the schema only guards the HTTP path.
func TestGetShowsCalendarHandler_ClampsTheLimit(t *testing.T) {
	for _, tc := range []struct{ sent, want int }{
		{0, defaultShowListLimit},
		{-5, defaultShowListLimit},
		{25, 25},
		{maxShowListLimit + 1, maxShowListLimit},
		{100000, maxShowListLimit},
	} {
		var got contracts.ShowCalendarQuery
		mock := &testhelpers.MockShowService{
			GetUpcomingShowsPageFn: func(query contracts.ShowCalendarQuery, _ bool, _ *contracts.UpcomingShowsFilter) ([]*contracts.ShowResponse, int64, error) {
				got = query
				return nil, 0, nil
			},
		}

		resp, err := newCalendarShowHandler(mock).GetShowsCalendarHandler(context.Background(),
			&GetShowsCalendarRequest{Limit: tc.sent})
		if err != nil {
			t.Fatalf("limit %d: unexpected error: %v", tc.sent, err)
		}
		if got.Limit != tc.want || resp.Body.Limit != tc.want {
			t.Errorf("limit %d: service got %d, response echoed %d, want %d",
				tc.sent, got.Limit, resp.Body.Limit, tc.want)
		}
	}
}

// The histogram publishes the sum of its own bars. A separately-counted total
// could disagree with the bars beside it, and a pager labelled from bars that do
// not sum to the list they label mislabels every page after the divergence.
func TestGetShowMonthsHandler_TotalIsTheSumOfTheBars(t *testing.T) {
	mock := &testhelpers.MockShowService{
		GetUpcomingShowMonthsFn: func(bool, *contracts.UpcomingShowsFilter) ([]contracts.ShowMonthCount, error) {
			return []contracts.ShowMonthCount{
				{Year: 2026, Month: 9, Count: 64},
				{Year: 2026, Month: 10, Count: 112},
				{Year: 2027, Month: 1, Count: 2},
			}, nil
		},
	}

	resp, err := newCalendarShowHandler(mock).GetShowMonthsHandler(context.Background(), &GetShowMonthsRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Total != 178 {
		t.Errorf("total = %d, want the sum of the bars (178)", resp.Body.Total)
	}
	if len(resp.Body.Months) != 3 {
		t.Errorf("bars = %v, want the service's three", resp.Body.Months)
	}
}

// Summing no bars is zero, not a panic and not the previous request's total.
// The [] rather than null guarantee belongs to the SERVICE, which is where it is
// asserted; a handler test could only observe its own mock's return value.
func TestGetShowMonthsHandler_EmptyHistogramTotalsZero(t *testing.T) {
	mock := &testhelpers.MockShowService{
		GetUpcomingShowMonthsFn: func(bool, *contracts.UpcomingShowsFilter) ([]contracts.ShowMonthCount, error) {
			return nil, nil
		},
	}

	resp, err := newCalendarShowHandler(mock).GetShowMonthsHandler(context.Background(), &GetShowMonthsRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Total != 0 {
		t.Errorf("total = %d, want 0", resp.Body.Total)
	}
}

// The handler WIRING, which is where the ticket's central invariant actually
// breaks: the filters, the viewer and the page bounds have to reach the service
// from the request, on BOTH endpoints. A test that only inspects the window
// stays green when a handler drops `cities` on one surface and honours it on the
// other, which is the strip offering a month its list refuses.
func TestShowListHandlers_PassTheSameFiltersAndViewerToTheService(t *testing.T) {
	const cities = "Phoenix,AZ|Mesa,AZ"
	wantCities := []contracts.CityStateFilter{{City: "Phoenix", State: "AZ"}, {City: "Mesa", State: "AZ"}}

	assertFilters := func(t *testing.T, surface string, got *contracts.UpcomingShowsFilter, gotViewer bool) {
		t.Helper()
		if got == nil {
			t.Fatalf("%s: filters did not reach the service", surface)
		}
		if !reflect.DeepEqual(got.Cities, wantCities) {
			t.Errorf("%s: cities reached the service as %+v, want %+v", surface, got.Cities, wantCities)
		}
		if !reflect.DeepEqual(got.TagSlugs, []string{"emo"}) || !got.TagMatchAny {
			t.Errorf("%s: tag filter reached the service as %+v", surface, got)
		}
		if gotViewer {
			t.Errorf("%s: an anonymous request must not ask the service for non-approved shows", surface)
		}
	}

	t.Run("calendar", func(t *testing.T) {
		var gotFilters *contracts.UpcomingShowsFilter
		var gotViewer bool
		mock := &testhelpers.MockShowService{
			GetUpcomingShowsPageFn: func(_ contracts.ShowCalendarQuery, includeNonApproved bool, filters *contracts.UpcomingShowsFilter) ([]*contracts.ShowResponse, int64, error) {
				gotFilters, gotViewer = filters, includeNonApproved
				return nil, 0, nil
			},
		}
		_, err := newCalendarShowHandler(mock).GetShowsCalendarHandler(context.Background(),
			&GetShowsCalendarRequest{Cities: cities, Tags: "emo", TagMatch: "any", Limit: 50})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		assertFilters(t, "GET /shows/calendar", gotFilters, gotViewer)
	})

	t.Run("months", func(t *testing.T) {
		var gotFilters *contracts.UpcomingShowsFilter
		var gotViewer bool
		mock := &testhelpers.MockShowService{
			GetUpcomingShowMonthsFn: func(includeNonApproved bool, filters *contracts.UpcomingShowsFilter) ([]contracts.ShowMonthCount, error) {
				gotFilters, gotViewer = filters, includeNonApproved
				return nil, nil
			},
		}
		_, err := newCalendarShowHandler(mock).GetShowMonthsHandler(context.Background(),
			&GetShowMonthsRequest{Cities: cities, Tags: "emo", TagMatch: "any"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		assertFilters(t, "GET /shows/months", gotFilters, gotViewer)
	})
}

// The histogram is PRIVATE where its venue and artist twins are public, because
// its body is computed from a viewer test. Pinned rather than left to the
// constant, so widening it back to `public` has to be a deliberate edit here.
func TestGetShowMonthsHandler_CacheControlIsPrivate(t *testing.T) {
	mock := &testhelpers.MockShowService{
		GetUpcomingShowMonthsFn: func(bool, *contracts.UpcomingShowsFilter) ([]contracts.ShowMonthCount, error) {
			return nil, nil
		},
	}

	resp, err := newCalendarShowHandler(mock).GetShowMonthsHandler(context.Background(), &GetShowMonthsRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.CacheControl != "private, max-age=60" {
		t.Errorf("Cache-Control = %q, want private: this payload is computed from a viewer test, "+
			"so a shared cache must not be allowed to hand one reader's counts to another", resp.CacheControl)
	}
}

// A negative offset is floored, and the envelope echoes the page that was read
// rather than the one that was asked for.
func TestGetShowsCalendarHandler_FloorsAndEchoesTheOffset(t *testing.T) {
	for _, tc := range []struct{ sent, want int }{
		{-1, 0},
		{0, 0},
		{150, 150},
	} {
		var got contracts.ShowCalendarQuery
		mock := &testhelpers.MockShowService{
			GetUpcomingShowsPageFn: func(query contracts.ShowCalendarQuery, _ bool, _ *contracts.UpcomingShowsFilter) ([]*contracts.ShowResponse, int64, error) {
				got = query
				return nil, 0, nil
			},
		}

		resp, err := newCalendarShowHandler(mock).GetShowsCalendarHandler(context.Background(),
			&GetShowsCalendarRequest{Offset: tc.sent})
		if err != nil {
			t.Fatalf("offset %d: unexpected error: %v", tc.sent, err)
		}
		if got.Offset != tc.want || resp.Body.Offset != tc.want {
			t.Errorf("offset %d: service got %d, response echoed %d, want %d",
				tc.sent, got.Offset, resp.Body.Offset, tc.want)
		}
	}
}
