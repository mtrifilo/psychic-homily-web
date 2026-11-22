package catalog

import (
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"gorm.io/gorm"

	"psychic-homily-backend/db"
	apperrors "psychic-homily-backend/internal/errors"
	catalogm "psychic-homily-backend/internal/models/catalog"
	communitym "psychic-homily-backend/internal/models/community"
	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/services/shared"
)

// EntityExistenceService answers lightweight public entity existence probes.
// It avoids the detail services' HYDRATION, which builds joins and response
// bodies the frontend proxy does not need before rendering a page. Scene
// existence is not hydration: it is slug resolution plus a count, and it runs
// through SceneService so the gate cannot answer differently than the page.
type EntityExistenceService struct {
	db     *gorm.DB
	scenes *SceneService
}

// NewEntityExistenceService builds the probe over a scene service. Pass the
// process's shared SceneService: ParseSceneSlug's negative cache is per
// instance, so a probe holding its own would answer a slug from a miss the page
// never took, and go on answering it for the cache's TTL.
func NewEntityExistenceService(database *gorm.DB, scenes *SceneService) *EntityExistenceService {
	if database == nil {
		database = db.GetDB()
	}
	if scenes == nil {
		scenes = NewSceneService(database)
	}
	return &EntityExistenceService{db: database, scenes: scenes}
}

func (s *EntityExistenceService) Exists(entityType, idOrSlug string) (bool, error) {
	if s.db == nil {
		return false, fmt.Errorf("database not initialized")
	}

	switch entityType {
	case "shows":
		return s.existsByIDOrSlug(
			&catalogm.Show{},
			idOrSlug,
			"status = ?",
			catalogm.ShowStatusApproved,
		)
	case "venues":
		return s.existsByIDOrSlug(&catalogm.Venue{}, idOrSlug)
	case "artists":
		return s.existsByIDOrSlug(&catalogm.Artist{}, idOrSlug)
	case "releases":
		return s.existsByIDOrSlug(&catalogm.Release{}, idOrSlug)
	case "labels":
		return s.existsByIDOrSlug(&catalogm.Label{}, idOrSlug)
	case "festivals":
		return s.existsByIDOrSlug(&catalogm.Festival{}, idOrSlug)
	case "tags":
		return s.existsByIDOrSlug(&catalogm.Tag{}, idOrSlug)
	case "scenes":
		_, exists, err := s.ServedSceneSlug(idOrSlug)
		return exists, err
	default:
		return false, nil
	}
}

// CurrentSlugForFormerSlug answers the probe for a URL segment Exists missed:
// the live slug of the show, venue or artist that segment is a former slug of,
// served under the same rule Exists applies, or "" when it names none. It
// returns "" without a query for every other entity type and for a segment
// that reads as an id. Call it only after Exists has answered false, so a live
// slug is never looked up here.
func (s *EntityExistenceService) CurrentSlugForFormerSlug(entityType, idOrSlug string) (string, error) {
	if s.db == nil {
		return "", fmt.Errorf("database not initialized")
	}
	entity, ok := formerSlugEntityByPrefix[entityType]
	if !ok {
		return "", nil
	}
	_, current, _, err := lookupFormerSlug(s.db, entity, idOrSlug)
	return current, err
}

func (s *EntityExistenceService) existsByIDOrSlug(model any, idOrSlug string, extraWhere ...any) (bool, error) {
	query := s.db.Model(model)

	if len(extraWhere) > 0 {
		where, ok := extraWhere[0].(string)
		if !ok {
			return false, fmt.Errorf("extra where clause must be a string")
		}
		query = query.Where(where, extraWhere[1:]...)
	}

	cond, arg := idOrSlugCondition("", idOrSlug)
	return firstIDExists(query.Where(cond, arg), "id")
}

// idOrSlugCondition is the condition matching the row a URL segment names in
// table (unqualified when table is empty): a segment that parses as a 32-bit
// unsigned integer names the id, anything else names the slug, exactly. It is
// the rule the detail handlers that take a slug or numeric id resolve with.
//
// table is SQL the CALLER controls and must be a literal in the calling code.
func idOrSlugCondition(table, idOrSlug string) (string, any) {
	prefix := ""
	if table != "" {
		prefix = table + "."
	}
	if id, err := strconv.ParseUint(idOrSlug, 10, 32); err == nil {
		return prefix + "id = ?", uint(id)
	}
	return prefix + "slug = ?", idOrSlug
}

// firstIDExists reports whether query reaches at least one row, reading
// idColumn of the first.
func firstIDExists(query *gorm.DB, idColumn string) (bool, error) {
	var id uint
	if err := query.Select(idColumn).Limit(1).Scan(&id).Error; err != nil {
		return false, err
	}
	return id != 0, nil
}

// RadioStationExists gates the proxy soft-404 for /radio/{station} and for
// /radio/{network}/channel/{local}, each of whose pages resolves one station.
// It resolves a slug or numeric id as GET /radio-stations/{slug} does.
func (s *EntityExistenceService) RadioStationExists(stationIDOrSlug string) (bool, error) {
	if s.db == nil {
		return false, fmt.Errorf("database not initialized")
	}
	return s.existsByIDOrSlug(&catalogm.RadioStation{}, stationIDOrSlug)
}

// RadioShowExists gates the proxy soft-404 for /radio/{station}/{show}: the
// show resolves by slug or numeric id AND the station segment names the
// station that carries it, by slug or numeric id. A show addressed under any
// other station segment is absent.
func (s *EntityExistenceService) RadioShowExists(stationIDOrSlug, showIDOrSlug string) (bool, error) {
	if s.db == nil {
		return false, fmt.Errorf("database not initialized")
	}
	return firstIDExists(s.radioShowUnderStation(stationIDOrSlug, showIDOrSlug), "radio_shows.id")
}

// RadioEpisodeExists gates the proxy soft-404 for
// /radio/{station}/{show}/{date}: the show resolves under its station as in
// RadioShowExists, and it has an episode whose air_date is airDate, the row
// GET /radio-shows/{slug}/episodes/{date} answers with. A date that is not a
// real YYYY-MM-DD day names no episode.
func (s *EntityExistenceService) RadioEpisodeExists(stationIDOrSlug, showIDOrSlug, airDate string) (bool, error) {
	if s.db == nil {
		return false, fmt.Errorf("database not initialized")
	}
	if _, err := time.Parse(time.DateOnly, airDate); err != nil {
		return false, nil
	}
	query := s.radioShowUnderStation(stationIDOrSlug, showIDOrSlug).Where(
		"EXISTS (SELECT 1 FROM radio_episodes WHERE radio_episodes.show_id = radio_shows.id AND radio_episodes.air_date = ?)",
		airDate,
	)
	return firstIDExists(query, "radio_shows.id")
}

// radioShowUnderStation selects the radio show showIDOrSlug names, restricted to
// the station stationIDOrSlug names.
func (s *EntityExistenceService) radioShowUnderStation(stationIDOrSlug, showIDOrSlug string) *gorm.DB {
	showCond, showArg := idOrSlugCondition("radio_shows", showIDOrSlug)
	stationCond, stationArg := idOrSlugCondition("radio_stations", stationIDOrSlug)
	return s.db.Model(&catalogm.RadioShow{}).
		Joins("JOIN radio_stations ON radio_stations.id = radio_shows.station_id").
		Where(showCond, showArg).
		Where(stationCond, stationArg)
}

// CollectionExists gates the proxy soft-404 for /collections/{slug}. It answers
// as the collection detail route answers a caller WITHOUT a session: true only
// for the collection a slug or numeric id names when it is served to the
// public (shared.ServedCollectionPredicateSQL with no viewer: public, with a
// live creator). A private collection, a closed creator's and an unused slug
// all answer false, so the answer never tells anyone a hidden collection
// exists. It takes no viewer, so it is never the answer to give a creator about
// their own private collection.
func (s *EntityExistenceService) CollectionExists(collectionIDOrSlug string) (bool, error) {
	if s.db == nil {
		return false, fmt.Errorf("database not initialized")
	}
	served, servedArgs := shared.ServedCollectionPredicateSQL("collections", contracts.ShowViewer{})
	cond, arg := idOrSlugCondition("collections", collectionIDOrSlug)
	query := s.db.Model(&communitym.Collection{}).Where(served, servedArgs...).Where(cond, arg)
	return firstIDExists(query, "collections.id")
}

// ServedSceneSlug gates the proxy soft-404 for /scenes/{slug}: whether the
// scene's page serves, and the slug that page is served at
// (CanonicalSceneSlug's answer). The slug is "" when the scene does not exist,
// and when no canonical slug can be named for it.
//
// It runs the page's own rule rather than a probe-sized restatement of it: the
// slug resolved through ParseSceneSlug, then sceneServingScope, the existence
// rule GetSceneDetail gates on. Restating it gave the gate and the page
// different answers about the same slug, in both directions, and a gate that
// disagrees with the page it guards either hides a scene or announces one that
// 404s.
//
// An unresolvable slug is absence, not failure: ParseSceneSlug reports it as a
// scene-not-found error, and the probe answers false. A failure to NAME the
// slug of a scene that serves is not a failure of the verdict: the slug is
// dropped and the verdict stands, as on the week routes.
func (s *EntityExistenceService) ServedSceneSlug(slug string) (canonicalSlug string, serves bool, err error) {
	city, state, err := s.scenes.ParseSceneSlug(slug)
	if err != nil {
		if apperrors.IsSceneNotFound(err) {
			return "", false, nil
		}
		return "", false, err
	}
	serves, err = s.scenes.sceneServesPage(city, state)
	if err != nil || !serves {
		return "", false, err
	}
	canonicalSlug, err = s.scenes.CanonicalSceneSlug(slug, city, state)
	if err != nil {
		slog.Warn("scene_probe_canonical_slug_failed", "slug", slug, "error", err)
		return "", true, nil
	}
	return canonicalSlug, true, nil
}
