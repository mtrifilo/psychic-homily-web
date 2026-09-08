package catalog

import (
	"strings"

	"gorm.io/gorm"

	"psychic-homily-backend/internal/services/geo"
)

// The `missing=listen` filter on GET /artists (PSY-2050).
//
// The scene front page states a gap as content ("N Phoenix bands have no listen
// link") and links to the list of those bands. The count and the list are one
// population or the sentence is false, so this filter is defined by reuse of
// GetSceneGaps' two rules rather than by a second spelling of either:
// noListenLinkSQL for the gap itself, and the scene scope (sceneScopeFor plus
// sceneScope.artistPredicate) for which bands belong to the place.
//
// The scope reuse is why the filter is more than an extra WHERE clause: it also
// REPLACES the browse list's own city matching, which is exact, case-sensitive
// and knows nothing about metro membership. Under this filter a place means the
// scene's roster, so a Mesa band answers for Phoenix and a row storing
// "phoenix" is a Phoenix band; without it, `?cities=` keeps the literal
// matching every other caller gets.
//
// It also drops the browse list's default "has an upcoming show" gate, for the
// same reason: the gap count applies no such gate, and a filter that answered
// with only the bands playing soon would report a number the scene page never
// stated.

// FilterMissingListenLink is the filters-map key GET /artists' `missing=listen`
// sets. Value type bool. Exported so the handler that sets it and the scope
// that reads it spell it once: a typo here is a filter that silently does
// nothing, which reads as an unfiltered list rather than as an error.
//
// Honoured by artistBrowseScope, so by the two paged browse reads. GetArtists,
// which reads the same map, does not honour it.
const FilterMissingListenLink = "missing_listen_link"

// artistCityPair is one place a browse request scopes to.
type artistCityPair struct {
	city  string
	state string
}

// missingListenLinkEngaged reports whether the filters ask for the gap
// population.
func missingListenLinkEngaged(filters map[string]interface{}) bool {
	engaged, _ := filters[FilterMissingListenLink].(bool)
	return engaged
}

// browseSkipsActiveGate reports whether the browse read drops its default
// "artist has an upcoming approved show" gate.
//
// Two filters drop it and they must agree on what dropping it means, so the
// question is asked here rather than by reading the tag filter's own flag at
// each site: PSY-495's tag mode (evergreen discovery) and the
// missing-listen-link filter (whose total is compared against a gap count taken
// over the whole roster).
func browseSkipsActiveGate(filters map[string]interface{}) bool {
	if skip, _ := filters["skip_active_filter"].(bool); skip {
		return true
	}
	return missingListenLinkEngaged(filters)
}

// browseCityPairs returns the (city, state) places a browse request names, from
// either the multi-city filter or the single city/state pair. A `state` with no
// `city` names no place: a state is not a scene.
func browseCityPairs(filters map[string]interface{}) []artistCityPair {
	if cities, ok := filters["cities"].([]map[string]string); ok {
		pairs := make([]artistCityPair, 0, len(cities))
		for _, cs := range cities {
			if cs["city"] != "" && cs["state"] != "" {
				pairs = append(pairs, artistCityPair{city: cs["city"], state: cs["state"]})
			}
		}
		return pairs
	}
	city, _ := filters["city"].(string)
	state, _ := filters["state"].(string)
	if city != "" && state != "" {
		return []artistCityPair{{city: city, state: state}}
	}
	return nil
}

// sceneRosterPredicate returns the WHERE fragment (on the given artists alias)
// selecting the union of the given places' scene rosters, plus its bind args.
// An empty place list returns an empty fragment, which the caller reads as "do
// not constrain by place".
//
// Each place is resolved through sceneScopeFor, the same resolution
// GetSceneGaps performs. Resolution reads the venue rows, so callers resolve
// once and reuse the fragment across the count and the page.
//
// Resolved scopes are DEDUPLICATED, because several places collapse onto one:
// every member city of a metro resolves to that metro's scope, and a metro
// scope expands into one predicate term per member place. Without the dedup, a
// ten-city request naming ten places in one metro would OR ten copies of the
// same several-hundred-term predicate into both the count and the page.
func sceneRosterPredicate(
	database *gorm.DB,
	g geo.Geocoder,
	pairs []artistCityPair,
	alias string,
) (string, []any, error) {
	if len(pairs) == 0 {
		return "", nil, nil
	}

	parts := make([]string, 0, len(pairs))
	args := make([]any, 0, len(pairs))
	seen := make(map[sceneScope]struct{}, len(pairs))
	for _, pair := range pairs {
		scope, err := sceneScopeFor(database, g, pair.city, pair.state)
		if err != nil {
			return "", nil, err
		}
		if _, dup := seen[scope]; dup {
			continue
		}
		seen[scope] = struct{}{}

		pred, predArgs := scope.artistPredicate(alias)
		parts = append(parts, "("+pred+")")
		args = append(args, predArgs...)
	}
	return strings.Join(parts, " OR "), args, nil
}
