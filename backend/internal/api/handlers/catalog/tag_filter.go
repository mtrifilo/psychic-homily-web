package catalog

import (
	"strings"

	"psychic-homily-backend/internal/services/catalog"
	"psychic-homily-backend/internal/services/contracts"
)

// maxCityFilters caps how many (city, state) pairs a single browse/facet
// request may carry — mirrors the same cap GetUpcomingShowsHandler applies.
const maxCityFilters = 10

// maxBrowseTagSlugs caps how many tag slugs a BROWSE LIST will filter by.
//
// `tags=` is otherwise an unbounded fan-out: every slug becomes a bound
// parameter in ApplyTagFilter's subquery, so the cost of one unauthenticated
// GET scales with the length of a query string the caller chooses. Past
// Postgres' 65,535-parameter limit it stops being merely slow and becomes a
// 500. Found by PSY-1774's adversarial review, which measured a burst of
// 60,000-slug requests degrading an ordinary GET /artists by ~47x — and that
// endpoint now binds the list twice, once for the page and once for the total.
//
// 10 mirrors maxCityFilters, and for the same reason: the facet UI selects a
// handful, and no reader filters a browse list by more than ten tags at once.
//
// NOT applied inside parseTagFilter, because the call sites genuinely do not
// share one contract. GetTagIntersectionHandler bounds the same input at
// intersectionMaxTags and answers 400 — for a fan-out endpoint a rejected
// request is better than a silently narrowed one — and a truncating parse
// would make that 400 unreachable. Truncation is the browse-list answer
// specifically: there the extra slugs are noise from a hand-edited URL, and a
// list that renders is better than a list that 400s.
//
// This is the SECOND of two bounds, and it only bounds the QUERY. Parsing an
// enormous `tags=` — splitting it, lowercasing each part, deduplicating through
// a map — happens before any cap can apply, and 60,000 slugs cost ~1.5s of CPU
// against a ~0.1s baseline even with the list truncated afterwards. The first
// bound is therefore a huma `maxLength` on the param itself, which rejects the
// string before the handler runs and is the one that makes the parse cheap.
// A cap without that length bound moves the amplifier, it does not remove it.
const maxBrowseTagSlugs = 10

// capBrowseTagSlugs bounds a browse list's tag filter at maxBrowseTagSlugs.
//
// Applied at the GET /artists call site by PSY-1774. The other browse lists
// (venues, shows, labels, festivals, releases) carry the same uncapped
// fan-out and are NOT fixed here: sweeping them is a security change on five
// endpoints this ticket does not touch, and it wants its own review rather
// than a ride-along. Tracked as PSY-1795.
func capBrowseTagSlugs(tf catalog.TagFilter) catalog.TagFilter {
	if len(tf.TagSlugs) > maxBrowseTagSlugs {
		tf.TagSlugs = tf.TagSlugs[:maxBrowseTagSlugs]
	}
	return tf
}

// parseTagFilter normalizes the `tags=` and `tag_match=` query params used
// by the multi-tag browse filter (PSY-309). It splits on commas, trims
// whitespace, lowercases each slug, and deduplicates. `match` accepts the
// string "any" (case-insensitive) for OR semantics; any other value —
// including "all" or empty — means AND.
//
// It does NOT bound the list; see capBrowseTagSlugs for why that belongs at
// the call site.
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
