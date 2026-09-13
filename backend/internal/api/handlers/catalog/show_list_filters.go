package catalog

import (
	"context"

	"psychic-homily-backend/internal/api/middleware"
	"psychic-homily-backend/internal/services/contracts"
)

// The query vocabulary the catalog-wide upcoming show lists share: which shows a
// filter set selects, how many rows a page may carry, and which viewer is
// asking.
//
// One definition each, because three endpoints read the same partition
// (GET /shows/upcoming, GET /shows/calendar, GET /shows/months) and a difference
// between them is a strip offering a month its list cannot show, or a paged list
// that disagrees with the feed beside it.

// parseUpcomingShowsFilter builds the service filter from the query parameters
// those three endpoints share. Returns nil when nothing was asked for, which
// every caller reads as "no filter".
//
// `cities` wins over the legacy city/state pair when both are present. A
// `cities` value with no well-formed pair in it filters nothing rather than
// matching nothing, because a malformed filter that silently empties the list is
// indistinguishable to the reader from a quiet week.
func parseUpcomingShowsFilter(cities, city, state, tags, tagMatch string) *contracts.UpcomingShowsFilter {
	var filters *contracts.UpcomingShowsFilter

	if cityFilters := parseCityStateFilters(cities); len(cityFilters) > 0 {
		filters = &contracts.UpcomingShowsFilter{Cities: cityFilters}
	} else if cities == "" && (city != "" || state != "") {
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

// clampShowListLimit resolves a caller's page size against the catalog-wide
// bounds. Huma's `default` and `maximum` tags only reach the HTTP path, and zero
// means "no rows" to the service rather than "the default", so both ends are
// resolved here where every list handler can share the answer.
func clampShowListLimit(limit int) int {
	if limit < 1 {
		return defaultShowListLimit
	}
	if limit > maxShowListLimit {
		return maxShowListLimit
	}
	return limit
}

// upcomingListIncludesNonApproved reports whether the REQUEST'S VIEWER may see
// shows that are not approved: an admin may, and everyone else sees the public
// catalog. The viewer comes from the request context, so a route that carries no
// auth middleware resolves to false for every caller.
//
// Shared by all three catalog-wide list handlers so that the cursor list, the
// paged list and the month strip answer to one viewer. A strip counting shows
// the list beside it refuses would offer a month that opens empty.
func upcomingListIncludesNonApproved(ctx context.Context) bool {
	user := middleware.GetUserFromContext(ctx)
	return user != nil && user.IsAdmin
}
