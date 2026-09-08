package catalog

import (
	"errors"

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
//
// ONE PLACE PER REQUEST, for two reasons that point the same way. A gap count is
// published per scene, so a union of several scenes' rosters is a total no
// published number agrees with, and agreement is this filter's whole contract.
// And a metro scope expands into one predicate term per member place, which for
// the largest CBSAs in the shipped geo dataset is several hundred: unioning ten
// of those would let one unauthenticated request build a predicate of thousands
// of terms and have it planned twice, for the count and for the page.
//
// The scope rule applies to a PLACE, which is a (city, state) pair. A request
// that names only a `state` names no place, so it keeps the browse list's own
// literal, case-sensitive state match; the gap predicate still applies.

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

// errGapFilterPlaces is returned when a gap-filtered request names places the
// filter cannot answer for. The handler refuses these before the service is
// reached; this is the service defending its own invariant for any other caller,
// because the failure it prevents is silent: a request that named a place and
// got a list scoped to somewhere else, or to nowhere.
var errGapFilterPlaces = errors.New(
	"the missing-listen-link filter scopes to exactly one complete (city, state) place")

// browseGapPlace returns the single place a gap-filtered browse request scopes
// to. The second return is false when the request names no place at all, which
// is a request for every band with the gap and is allowed.
//
// It errors when the request names places the filter cannot answer for: more
// than one, or one whose state is missing. See the file header for why one.
func browseGapPlace(filters map[string]interface{}) (artistCityPair, bool, error) {
	pairs := browseCityPairs(filters)
	if len(pairs) > 1 {
		return artistCityPair{}, false, errGapFilterPlaces
	}
	if len(pairs) == 1 {
		return pairs[0], true, nil
	}
	if browseNamesACity(filters) {
		return artistCityPair{}, false, errGapFilterPlaces
	}
	return artistCityPair{}, false, nil
}

// browseNamesACity reports whether the request asked to be scoped to a city at
// all, however incompletely. It is what separates "no place named" from "a place
// named that could not be read".
func browseNamesACity(filters map[string]interface{}) bool {
	if cities, ok := filters["cities"].([]map[string]string); ok {
		return len(cities) > 0
	}
	city, _ := filters["city"].(string)
	return city != ""
}

// sceneRosterPredicate returns the WHERE fragment (on the given artists alias)
// selecting one place's scene roster, plus its bind args.
//
// The place is resolved through sceneScopeFor, the same resolution GetSceneGaps
// performs, so the roster here is the roster counted there. Resolution reads the
// venue rows, so callers resolve once and reuse the fragment across the count
// and the page.
func sceneRosterPredicate(
	database *gorm.DB,
	g geo.Geocoder,
	place artistCityPair,
	alias string,
) (string, []any, error) {
	scope, err := sceneScopeFor(database, g, place.city, place.state)
	if err != nil {
		return "", nil, err
	}
	pred, args := scope.artistPredicate(alias)
	return pred, args, nil
}
