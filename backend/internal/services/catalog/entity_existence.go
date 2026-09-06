package catalog

import (
	"fmt"
	"strconv"
	"strings"

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
// GetSceneDetail's existence rule (>= sceneMinVenues verified venues) in the
// readings ParseSceneSlug resolves a slug through, so the gate and the page
// agree about which slugs have a scene behind them.
//
// The slug's own venue group answers first, through the rule the scenes
// directory publishes by: a city whose verified rooms all carry a NULL
// venues.metro is a scene the metro count below cannot see, and gating it off
// soft-404s a page the directory links to.
//
// A slug no group publishes is canonicalized the way ParseSceneSlug
// canonicalizes it, and counted in the scope that identity resolves to: a metro
// member slug (mesa-az) names its metro's principal city, so the gate answers
// for the page that slug actually serves. Counting the CBSA's own rooms instead
// would gate a member slug off whenever the metro it rolls up to is itself a
// drifted fallback group.
//
// A slug with no CBSA at all keeps the literal city-state venue match.
func (s *EntityExistenceService) sceneExists(slug string) (bool, error) {
	city, state := parseSceneSlugParts(slug)
	cbsa := ""
	if m, ok := geo.Default().ResolveMetro(city, state, usCountry); ok {
		cbsa = m.CBSACode
	}
	if _, ok, err := publishedSceneGroup(s.db, geo.Default(), slug, cbsa); err != nil {
		return false, err
	} else if ok {
		return true, nil
	}

	if cbsa != "" {
		if principal, ok := geo.MetroPrincipalByCBSA(cbsa); ok {
			scope := sceneScopeFor(s.db, geo.Default(), principal.City, principal.State)
			n, err := verifiedVenueCountIn(s.db, scope)
			if err != nil {
				return false, err
			}
			return n >= sceneMinVenues, nil
		}
	}

	// No-CBSA fallback: match the SAME slug form ParseSceneSlug's no-CBSA
	// resolver uses (scene.go), so the proxy gate and the page agree. It LOWERs
	// both sides (handles mixed-case venue data) AND is lossless for hyphenated
	// city names like "Winston-Salem", unlike re-parsing the slug, which would
	// collapse the hyphen to a space and miss the stored row.
	q := s.db.Model(&catalogm.Venue{}).
		Where("verified = true").
		Where(sceneSlugExprSQL+" = ?", strings.ToLower(slug))
	var verifiedVenueCount int64
	if err := q.Distinct("id").Count(&verifiedVenueCount).Error; err != nil {
		return false, err
	}
	return verifiedVenueCount >= sceneMinVenues, nil
}
