package catalog

import (
	"strings"

	"psychic-homily-backend/internal/services/catalog"
	"psychic-homily-backend/internal/services/contracts"
)

// maxCityFilters caps how many (city, state) pairs a single browse/facet
// request may carry — mirrors the same cap GetUpcomingShowsHandler applies.
const maxCityFilters = 10

// parseTagFilter normalizes the `tags=` and `tag_match=` query params used
// by the multi-tag browse filter (PSY-309). It splits on commas, trims
// whitespace, lowercases each slug, and deduplicates. `match` accepts the
// string "any" (case-insensitive) for OR semantics; any other value —
// including "all" or empty — means AND.
func parseTagFilter(tags, match string) catalog.TagFilter {
	return catalog.ParseTagFilter(tags, match)
}

// parseCityStateFilters turns the pipe-delimited "City,ST|City,ST" query
// param into typed filters, using the same wire format as the /shows handler
// (PSY-982 reuses it for the city-scoped tag facet). Malformed pairs (not
// exactly city,state, or blank after trimming) are skipped. The list is
// capped at maxCityFilters. Empty input ⇒ nil (no filter).
func parseCityStateFilters(raw string) []contracts.CityStateFilter {
	if raw == "" {
		return nil
	}
	var filters []contracts.CityStateFilter
	for _, pair := range strings.Split(raw, "|") {
		parts := strings.Split(pair, ",")
		if len(parts) != 2 {
			continue
		}
		city := strings.TrimSpace(parts[0])
		state := strings.TrimSpace(parts[1])
		if city == "" || state == "" {
			continue
		}
		filters = append(filters, contracts.CityStateFilter{City: city, State: state})
		if len(filters) >= maxCityFilters {
			break
		}
	}
	return filters
}
