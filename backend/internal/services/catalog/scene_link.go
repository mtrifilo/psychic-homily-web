package catalog

import (
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"gorm.io/gorm"

	apperrors "psychic-homily-backend/internal/errors"
	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/services/geo"
	"psychic-homily-backend/internal/services/shared"
)

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

// artistHasAnyLocation reports whether an artist states any location. Only
// an artist with none takes its scene from a show's venue: an artist that
// names somewhere, even partially or abroad, keeps that as its location, and
// a scene borrowed from a show would contradict it.
func artistHasAnyLocation(artist *catalogm.Artist) bool {
	for _, v := range []*string{artist.City, artist.State, artist.Country} {
		if v != nil && strings.TrimSpace(*v) != "" {
			return true
		}
	}
	return false
}

// artistOwnPlace is the artist's own location as a scene-link place, and
// false when the artist lacks the city or the state a scene slug needs.
func artistOwnPlace(artist *catalogm.Artist) (sceneLinkPlace, bool) {
	if artist.City == nil || artist.State == nil {
		return sceneLinkPlace{}, false
	}
	return sceneLinkPlace{City: *artist.City, State: *artist.State, Country: artist.Country}, true
}

// servedSceneLink returns the scene page a detail page may link to for a
// place, or nil when no such page serves.
//
// It walks the path GET /scenes/{slug} walks, so a returned link is one whose
// page renders: the place's own slug is resolved by ParseSceneSlug (a metro
// member resolves to its principal city, a fallback group to its own city),
// the resulting display identity is rebuilt into the canonical slug, that slug
// is resolved again as the page resolves it, and GetSceneDetail's venue floor
// is applied to the scope it lands on.
//
// The SceneService built here has no slug-miss cache, so a lookup never
// poisons, or is answered from, the cache the scene routes share.
//
// A not-found anywhere on that path is nil with no error. Any other failure is
// returned, and callers log it and leave the link out: the link is optional,
// and a detail page is not failed over it.
func servedSceneLink(database *gorm.DB, g geo.Geocoder, place sceneLinkPlace) (*contracts.SceneLinkResponse, error) {
	if !place.isUSPlace() {
		return nil, nil
	}
	if g == nil {
		g = geo.Default()
	}
	sc := &SceneService{db: database, geocoder: g}

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

	scope, err := sc.scopeFor(city, state)
	if err != nil {
		return nil, err
	}
	venues, err := sc.verifiedVenueCount(scope)
	if err != nil {
		return nil, fmt.Errorf("failed to count the verified venues of scene %q: %w", slug, err)
	}
	if venues < sceneMinVenues {
		return nil, nil
	}
	return &contracts.SceneLinkResponse{Slug: slug, City: city, State: state}, nil
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

func ignoreSceneNotFound(err error) error {
	var sceneErr *apperrors.SceneError
	if errors.As(err, &sceneErr) && sceneErr.Code == apperrors.CodeSceneNotFound {
		return nil
	}
	return err
}

// latestShowVenuePlace is the primary venue of the artist's latest approved
// show that has a venue, by event date, and false when there is none.
func latestShowVenuePlace(database *gorm.DB, artistID uint) (sceneLinkPlace, bool, error) {
	var rows []struct {
		City    string
		State   string
		Country *string
	}
	err := database.Raw(`
		SELECT pv.city, pv.state, pv.country
		FROM shows s
		JOIN show_artists sa ON sa.show_id = s.id
		JOIN LATERAL `+shared.PrimaryVenueLateralSQL("iv.city, iv.state, iv.country", "s.id")+` pv ON true
		WHERE sa.artist_id = ?
		  AND s.status = ?
		ORDER BY s.event_date DESC, s.id DESC
		LIMIT 1
	`, artistID, catalogm.ShowStatusApproved).Scan(&rows).Error
	if err != nil {
		return sceneLinkPlace{}, false, fmt.Errorf("failed to read the latest show venue of artist %d: %w", artistID, err)
	}
	if len(rows) == 0 {
		return sceneLinkPlace{}, false, nil
	}
	return sceneLinkPlace{City: rows[0].City, State: rows[0].State, Country: rows[0].Country}, true, nil
}

// artistSceneLink is the scene link on an artist's detail page: from the
// artist's own city and state, or, for an artist with no location at all,
// from the venue of its latest approved show.
func artistSceneLink(database *gorm.DB, g geo.Geocoder, artist *catalogm.Artist) (*contracts.SceneLinkResponse, error) {
	if artistHasAnyLocation(artist) {
		place, ok := artistOwnPlace(artist)
		if !ok {
			return nil, nil
		}
		return servedSceneLink(database, g, place)
	}
	place, ok, err := latestShowVenuePlace(database, artist.ID)
	if err != nil || !ok {
		return nil, err
	}
	return servedSceneLink(database, g, place)
}
