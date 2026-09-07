package catalog

import (
	"errors"
	"fmt"
	"strconv"

	"gorm.io/gorm"

	"psychic-homily-backend/db"
	apperrors "psychic-homily-backend/internal/errors"
	catalogm "psychic-homily-backend/internal/models/catalog"
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
		return s.sceneExists(idOrSlug)
	default:
		return false, nil
	}
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

	if id, err := strconv.ParseUint(idOrSlug, 10, 32); err == nil {
		query = query.Where("id = ?", uint(id))
	} else {
		query = query.Where("slug = ?", idOrSlug)
	}

	var id uint
	if err := query.Select("id").Limit(1).Scan(&id).Error; err != nil {
		return false, err
	}
	return id != 0, nil
}

// sceneExists gates the proxy soft-404 for /scenes/{slug}.
//
// It runs the page's own rule rather than a probe-sized restatement of it: the
// slug resolved through ParseSceneSlug, scoped through scopeFor, counted
// against the >= sceneMinVenues floor GetSceneDetail gates on. Restating it
// gave the gate and the page different answers about the same slug, in both
// directions, and a gate that disagrees with the page it guards either hides a
// scene or announces one that 404s.
//
// An unresolvable slug is absence, not failure: ParseSceneSlug reports it as a
// scene-not-found error, and the probe answers false.
func (s *EntityExistenceService) sceneExists(slug string) (bool, error) {
	city, state, err := s.scenes.ParseSceneSlug(slug)
	if err != nil {
		var sceneErr *apperrors.SceneError
		if errors.As(err, &sceneErr) && sceneErr.Code == apperrors.CodeSceneNotFound {
			return false, nil
		}
		return false, err
	}
	scope, err := s.scenes.scopeFor(city, state)
	if err != nil {
		return false, err
	}
	n, err := s.scenes.verifiedVenueCount(scope)
	if err != nil {
		return false, err
	}
	return n >= sceneMinVenues, nil
}
