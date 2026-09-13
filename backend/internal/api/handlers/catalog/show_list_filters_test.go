package catalog

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"psychic-homily-backend/internal/api/middleware"
	authm "psychic-homily-backend/internal/models/auth"
	"psychic-homily-backend/internal/services/contracts"
)

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

	t.Run("the legacy pair still filters on its own", func(t *testing.T) {
		got := parseUpcomingShowsFilter("", "Tucson", "AZ", "", "")
		if got.City != "Tucson" || got.State != "AZ" || len(got.Cities) != 0 {
			t.Errorf("got %+v", got)
		}
	})

	t.Run("pairs are trimmed", func(t *testing.T) {
		got := parseUpcomingShowsFilter(" Phoenix , AZ ", "", "", "", "")
		want := []contracts.CityStateFilter{{City: "Phoenix", State: "AZ"}}
		if !reflect.DeepEqual(got.Cities, want) {
			t.Errorf("cities = %+v, want %+v", got.Cities, want)
		}
	})

	t.Run("the city cap is enforced", func(t *testing.T) {
		pairs := strings.TrimSuffix(strings.Repeat("City,AZ|", maxCityFilters+5), "|")
		got := parseUpcomingShowsFilter(pairs, "", "", "", "")
		if len(got.Cities) != maxCityFilters {
			t.Errorf("cities = %d, want the cap of %d", len(got.Cities), maxCityFilters)
		}
	})

	// "all" is the site's no-city sentinel and reaches the API verbatim from an
	// older client. It names no (city, state) pair, so it filters nothing, which
	// is what the sentinel means.
	t.Run("the all sentinel filters nothing", func(t *testing.T) {
		if got := parseUpcomingShowsFilter("all", "", "", "", ""); got != nil {
			t.Errorf("got %+v, want nil", got)
		}
	})

	t.Run("a malformed cities value filters nothing", func(t *testing.T) {
		for _, raw := range []string{"Phoenix|Mesa", "Phoenix,", ",AZ", "Phoenix, ", "Phoenix,AZ,extra"} {
			if got := parseUpcomingShowsFilter(raw, "", "", "", ""); got != nil {
				t.Errorf("%q -> %+v, want nil: a malformed filter that empties the list is "+
					"indistinguishable to the reader from a quiet week", raw, got)
			}
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

// Zero means "no rows" to the service rather than "the default", so the
// resolution has to happen above it, once, for every list handler.
func TestClampShowListLimit(t *testing.T) {
	for _, tc := range []struct{ sent, want int }{
		{0, defaultShowListLimit},
		{-1, defaultShowListLimit},
		{1, 1},
		{25, 25},
		{maxShowListLimit, maxShowListLimit},
		{maxShowListLimit + 1, maxShowListLimit},
		{100000, maxShowListLimit},
	} {
		if got := clampShowListLimit(tc.sent); got != tc.want {
			t.Errorf("clampShowListLimit(%d) = %d, want %d", tc.sent, got, tc.want)
		}
	}
}

// The viewer test is pinned in BOTH directions. Asserting only that an
// anonymous caller sees approved shows would stay green if the admin arm were
// deleted, and the month histogram's private Cache-Control is chosen from the
// fact that this reads a viewer at all.
func TestUpcomingListIncludesNonApproved(t *testing.T) {
	for _, tc := range []struct {
		name string
		ctx  context.Context
		want bool
	}{
		{"anonymous", context.Background(), false},
		{"a viewer key holding nothing", withUser(nil), false},
		{"signed in, not an admin", withUser(&authm.User{ID: 7}), false},
		{"admin", withUser(&authm.User{ID: 7, IsAdmin: true}), true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := upcomingListIncludesNonApproved(tc.ctx); got != tc.want {
				t.Errorf("upcomingListIncludesNonApproved = %v, want %v", got, tc.want)
			}
		})
	}
}

func withUser(user *authm.User) context.Context {
	return context.WithValue(context.Background(), middleware.UserContextKey, user)
}
