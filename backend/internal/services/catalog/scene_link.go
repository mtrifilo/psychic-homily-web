package catalog

import (
	"fmt"
	"log/slog"
	"strings"

	"gorm.io/gorm"

	apperrors "psychic-homily-backend/internal/errors"
	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/services/geo"
	"psychic-homily-backend/internal/services/shared"
	"psychic-homily-backend/internal/utils"
)

// sceneServingScope is the one existence rule for a scene addressed by its
// display identity: the scope its rooms form, their verified-venue count, and
// whether that count clears the floor. GetSceneDetail, the /scenes soft-404
// gate (sceneExists) and the detail-page scene links all decide through it.
func (s *SceneService) sceneServingScope(city, state string) (sceneScope, int64, bool, error) {
	scope, err := s.scopeFor(city, state)
	if err != nil {
		return sceneScope{}, 0, false, err
	}
	n, err := s.verifiedVenueCount(scope)
	if err != nil {
		return sceneScope{}, 0, false, fmt.Errorf("failed to count the verified venues of scene %s, %s: %w", city, state, err)
	}
	return scope, n, n >= sceneMinVenues, nil
}

// sceneServesPage is sceneServingScope's verdict alone.
func (s *SceneService) sceneServesPage(city, state string) (bool, error) {
	_, _, serves, err := s.sceneServingScope(city, state)
	return serves, err
}

// newUncachedSceneService is a SceneService with no slug-miss cache, for a
// caller whose lookups must neither read nor write the cache the scene routes
// share.
func newUncachedSceneService(database *gorm.DB, g geo.Geocoder) *SceneService {
	if g == nil {
		g = geo.Default()
	}
	return &SceneService{db: database, geocoder: g}
}

// sceneLinkPlace is the place a detail page links to its scene from.
type sceneLinkPlace struct {
	City    string
	State   string
	Country *string
}

// isUSPlace reports whether the place can carry a scene link at all: a city,
// a US state code, and no country naming anywhere else. A US state code with
// a non-US country ("Perth", "WA", "Australia") is not a US place, which is
// stricter than the geocoder's own reading, where the state wins.
func (p sceneLinkPlace) isUSPlace() bool {
	if strings.TrimSpace(p.City) == "" || !geo.IsUSStateCode(p.State) {
		return false
	}
	if p.Country == nil || strings.TrimSpace(*p.Country) == "" {
		return true
	}
	iso, ok := geo.CountryToISO(*p.Country)
	return ok && iso == "US"
}

// artistHasAnyLocation reports whether an artist states any location.
func artistHasAnyLocation(artist *catalogm.Artist) bool {
	for _, v := range []*string{artist.City, artist.State, artist.Country} {
		if v != nil && strings.TrimSpace(*v) != "" {
			return true
		}
	}
	return false
}

// servedSceneLink returns the scene page a detail page may link to for a
// place, or nil when no such page serves.
//
// It walks the path GET /scenes/{slug} walks, so a returned link is one whose
// page renders: the place's own slug is resolved by ParseSceneSlug (a metro
// member resolves to its principal city, a fallback group to its own city),
// the resulting display identity is rebuilt into the canonical slug, that slug
// is resolved again as the page resolves it, and sceneServesPage decides.
//
// The lookup runs on an uncached SceneService, so it never poisons, or is
// answered from, the slug-miss cache the scene routes share.
//
// A not-found anywhere on that path is nil with no error. Any other failure is
// returned, and callers log it and leave the link out: the link is optional,
// and a detail page is not failed over it.
func servedSceneLink(database *gorm.DB, g geo.Geocoder, place sceneLinkPlace) (*contracts.SceneLinkResponse, error) {
	if !place.isUSPlace() {
		return nil, nil
	}
	sc := newUncachedSceneService(database, g)

	placeSlug := buildSceneSlug(strings.TrimSpace(place.City), strings.TrimSpace(place.State))
	city, state, err := sc.ParseSceneSlug(placeSlug)
	if err != nil {
		return nil, ignoreSceneNotFound(err)
	}
	slug := buildSceneSlug(city, state)
	if slug != placeSlug {
		pageCity, pageState, err := sc.ParseSceneSlug(slug)
		if err != nil {
			return nil, ignoreSceneNotFound(err)
		}
		if buildSceneSlug(pageCity, pageState) != slug {
			return nil, nil
		}
		city, state = pageCity, pageState
	}

	serves, err := sc.sceneServesPage(city, state)
	if err != nil || !serves {
		return nil, err
	}
	return &contracts.SceneLinkResponse{Slug: slug, City: city, State: state}, nil
}

func ignoreSceneNotFound(err error) error {
	if apperrors.IsSceneNotFound(err) {
		return nil
	}
	return err
}

// sceneLinkOrNil is how a detail read attaches a scene link: the link, or nil
// with a warning when the lookup failed.
func sceneLinkOrNil(entity string, id uint, link *contracts.SceneLinkResponse, err error) *contracts.SceneLinkResponse {
	if err != nil {
		slog.Warn("scene_link_lookup_failed", "entity", entity, "id", id, "error", err)
		return nil
	}
	return link
}

// venueSceneLink is the scene link a show or venue detail read attaches for
// one venue, logged and dropped on failure. entity and id name the read.
func venueSceneLink(database *gorm.DB, g geo.Geocoder, entity string, id uint, venue *catalogm.Venue) *contracts.SceneLinkResponse {
	link, err := servedSceneLink(database, g, sceneLinkPlace{City: venue.City, State: venue.State, Country: venue.Country})
	return sceneLinkOrNil(entity, id, link, err)
}

// latestShowVenuePlace is the primary venue of the show an artist falls back
// to (see artistSceneLink), and false when there is none.
// Only approved, non-cancelled shows with a venue count. The most recent one
// dated up to today on its venue's own calendar wins; only when there is none
// does the nearest upcoming one.
func latestShowVenuePlace(database *gorm.DB, artistID uint) (sceneLinkPlace, bool, error) {
	onOrBeforeToday := shared.VenueLocalDateSQL + ` <= ` + shared.VenueLocalTodaySQL
	var rows []sceneLinkPlace
	// shows stays UNALIASED: shared.VenueTZJoin's lateral correlates on shows.id.
	err := database.Raw(`
		SELECT pv.city, pv.state, pv.country
		FROM shows
		JOIN show_artists sa ON sa.show_id = shows.id
		JOIN LATERAL `+shared.PrimaryVenueLateralSQL("iv.city, iv.state, iv.country", "shows.id")+` pv ON true
		`+shared.VenueTZJoin+`
		WHERE sa.artist_id = ?
		  AND shows.status = ?
		  AND shows.is_cancelled = false
		ORDER BY (`+onOrBeforeToday+`) DESC,
		         CASE WHEN `+onOrBeforeToday+` THEN shows.event_date END DESC NULLS LAST,
		         shows.event_date ASC,
		         shows.id DESC
		LIMIT 1
	`, artistID, catalogm.ShowStatusApproved).Scan(&rows).Error
	if err != nil {
		return sceneLinkPlace{}, false, fmt.Errorf("failed to read the latest show venue of artist %d: %w", artistID, err)
	}
	if len(rows) == 0 {
		return sceneLinkPlace{}, false, nil
	}
	return rows[0], true, nil
}

// artistOwnPlace is the artist's own location as a scene-link place, with a
// spelled-out US state ("Arizona") normalised to its code through the shared
// state map. Any other state value is kept as stored.
func artistOwnPlace(artist *catalogm.Artist) sceneLinkPlace {
	state := strings.TrimSpace(derefString(artist.State))
	if abbr, ok := utils.StateNameToAbbrev(state); ok {
		state = abbr
	}
	return sceneLinkPlace{City: derefString(artist.City), State: state, Country: artist.Country}
}

// isKnownNonUSPlace reports whether a location names somewhere outside the
// US: its country resolves to another country, its state is itself another
// country's name, or the geocoder places it outside the US. A location the
// geocoder misses, or places in the US, is not known to be non-US.
func isKnownNonUSPlace(g geo.Geocoder, p sceneLinkPlace) bool {
	if country := strings.TrimSpace(derefString(p.Country)); country != "" {
		if iso, ok := geo.CountryToISO(country); ok {
			return iso != "US"
		}
	}
	if state := strings.TrimSpace(p.State); len(state) > 2 {
		if iso, ok := geo.CountryToISO(state); ok && iso != "US" {
			return true
		}
	}
	if strings.TrimSpace(p.City) == "" {
		return false
	}
	if g == nil {
		g = geo.Default()
	}
	r, ok := g.Resolve(p.City, p.State, derefString(p.Country))
	return ok && r.Country != "" && r.Country != "US"
}

// artistSceneLink is the scene link on an artist's detail page.
//
//   - A city with a US state (a spelled-out state normalised to its code)
//     links its own scene, or nothing when no scene serves it.
//   - Otherwise, a location isKnownNonUSPlace places outside the US links
//     nothing.
//   - Everything else falls back to the venue of the show latestShowVenuePlace
//     picks: no location at all, a state the state map does not know
//     ("Phoenix, Arizonaa"), a city or state alone, a country alone that is
//     the US, and a place the geocoder misses.
func artistSceneLink(database *gorm.DB, g geo.Geocoder, artist *catalogm.Artist) (*contracts.SceneLinkResponse, error) {
	if artistHasAnyLocation(artist) {
		place := artistOwnPlace(artist)
		if place.isUSPlace() {
			return servedSceneLink(database, g, place)
		}
		if isKnownNonUSPlace(g, place) {
			return nil, nil
		}
	}
	place, ok, err := latestShowVenuePlace(database, artist.ID)
	if err != nil || !ok {
		return nil, err
	}
	return servedSceneLink(database, g, place)
}
