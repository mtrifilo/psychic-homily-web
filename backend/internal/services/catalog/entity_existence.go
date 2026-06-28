package catalog

import (
	"fmt"
	"strconv"

	"gorm.io/gorm"

	"psychic-homily-backend/db"
	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/geo"
)

// EntityExistenceService answers lightweight public entity existence probes.
// It intentionally avoids the detail services, which hydrate joins and response
// bodies that the frontend proxy does not need before rendering a page.
type EntityExistenceService struct {
	db *gorm.DB
}

func NewEntityExistenceService(database *gorm.DB) *EntityExistenceService {
	if database == nil {
		database = db.GetDB()
	}
	return &EntityExistenceService{db: database}
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

// sceneExists gates the proxy soft-404 for /scenes/{slug}. It mirrors
// GetSceneDetail's existence rule (>= sceneMinVenues verified venues), now
// metro-aware (PSY-1255 step C): a US slug whose (city,state) pins a CBSA counts
// verified venues across the WHOLE metro (so a Twin Cities slug, or an old
// suburb slug rolled into its metro, resolves), while a no-CBSA slug keeps the
// literal city-state venue match.
func (s *EntityExistenceService) sceneExists(slug string) (bool, error) {
	q := s.db.Model(&catalogm.Venue{}).Where("verified = true")
	city, state := parseSceneSlugParts(slug)
	if m, ok := geo.Default().ResolveMetro(city, state, usCountry); ok {
		q = q.Where("metro = ?", m.CBSACode)
	} else {
		// Same case-insensitive + trimmed match the detail gate (verifiedVenueCount)
		// uses for a no-CBSA scene, so the proxy soft-404 and the page agree.
		q = q.Where("LOWER(TRIM(city)) = LOWER(TRIM(?)) AND LOWER(TRIM(state)) = LOWER(TRIM(?))", city, state)
	}
	var verifiedVenueCount int64
	if err := q.Distinct("id").Count(&verifiedVenueCount).Error; err != nil {
		return false, err
	}
	return verifiedVenueCount >= sceneMinVenues, nil
}
