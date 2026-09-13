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
	if resp.CacheControl != showMonthHistogramCacheControl {
		t.Errorf("Cache-Control = %q, want %q", resp.CacheControl, showMonthHistogramCacheControl)
	}
	if len(resp.Body.Months) != 3 {
		t.Errorf("bars = %v, want the service's three", resp.Body.Months)
	}
}

// An empty histogram is an empty list with a real zero, not a null.
func TestGetShowMonthsHandler_EmptyHistogramTotalsZero(t *testing.T) {
	mock := &testhelpers.MockShowService{
		GetUpcomingShowMonthsFn: func(bool, *contracts.UpcomingShowsFilter) ([]contracts.ShowMonthCount, error) {
			return []contracts.ShowMonthCount{}, nil
		},
	}

	resp, err := newCalendarShowHandler(mock).GetShowMonthsHandler(context.Background(), &GetShowMonthsRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Total != 0 {
		t.Errorf("total = %d, want 0", resp.Body.Total)
	}
	if resp.Body.Months == nil {
		t.Error("months must serialize as [] rather than null")
	}
}

// The three readers of the upcoming partition share one filter parser, so a
// filter cannot select one set on the cursor list and another on the paged list
// or the strip above it.
func TestParseUpcomingShowsFilter(t *testing.T) {
	t.Run("nothing asked for is no filter", func(t *testing.T) {
		if got := parseUpcomingShowsFilter("", "", "", "", ""); got != nil {
			t.Errorf("got %+v, want nil", got)
		}
	})

	t.Run("cities wins over the legacy pair", func(t *testing.T) {
		got := parseUpcomingShowsFilter("Phoenix,AZ|Mesa,AZ", "Tucson", "AZ", "", "")
		want := []contracts.CityStateFilter{{City: "Phoenix", State: "AZ"}, {City: "Mesa", State: "AZ"}}
		if !reflect.DeepEqual(got.Cities, want) {
			t.Errorf("cities = %+v, want %+v", got.Cities, want)
		}
		if got.City != "" || got.State != "" {
			t.Errorf("legacy pair leaked through: %+v", got)
		}
	})

	t.Run("the city cap is enforced", func(t *testing.T) {
		pairs := ""
		for i := 0; i < maxUpcomingShowCities+5; i++ {
			if i > 0 {
				pairs += "|"
			}
			pairs += "City,AZ"
		}
		got := parseUpcomingShowsFilter(pairs, "", "", "", "")
		if len(got.Cities) != maxUpcomingShowCities {
			t.Errorf("cities = %d, want the cap of %d", len(got.Cities), maxUpcomingShowCities)
		}
	})

	t.Run("a malformed cities value filters nothing", func(t *testing.T) {
		if got := parseUpcomingShowsFilter("Phoenix|Mesa", "", "", "", ""); got != nil {
			t.Errorf("got %+v, want nil: a malformed filter that empties the list is indistinguishable from a quiet week", got)
		}
	})

	t.Run("tags carry their match mode", func(t *testing.T) {
		got := parseUpcomingShowsFilter("", "", "", "post-punk,shoegaze", "any")
		if !reflect.DeepEqual(got.TagSlugs, []string{"post-punk", "shoegaze"}) {
			t.Errorf("tags = %v", got.TagSlugs)
		}
		if !got.TagMatchAny {
			t.Error("tag_match=any must reach the service as OR semantics")
		}
	})

	t.Run("tags combine with cities", func(t *testing.T) {
		got := parseUpcomingShowsFilter("Phoenix,AZ", "", "", "post-punk", "all")
		if len(got.Cities) != 1 || len(got.TagSlugs) != 1 || got.TagMatchAny {
			t.Errorf("got %+v", got)
		}
	})
}
