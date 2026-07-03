package catalog

import (
	"fmt"

	catalogm "psychic-homily-backend/internal/models/catalog"
)

// Scene registry (PSY-1339, from the PSY-1314 spike): scenes stay computed
// aggregations, but id-keyed features (follows, curated descriptions) need a
// row. Rows are materialized lazily — created the first time something needs
// one — so there's no sync job keeping a table aligned with the aggregation.
//
// Canonicalization: any slug that reaches these methods is first resolved
// through ParseSceneSlug, so a metro MEMBER city's slug (mesa-az) lands on the
// metro's canonical row (phoenix-az) — following Mesa follows the Phoenix
// scene, matching the metro roster model (PSY-1255).

// GetOrCreateSceneID resolves a scene slug to its registry row id, creating
// the row on first need. Unknown slugs (no metro resolution AND no verified
// venue) return ParseSceneSlug's ErrSceneNotFound — you can't follow a scene
// that doesn't exist.
func (s *SceneService) GetOrCreateSceneID(slug string) (uint, error) {
	scope, canonicalSlug, err := s.canonicalScope(slug)
	if err != nil {
		return 0, err
	}

	// Fast path: the canonical slug is unique and 1:1 with the scope (both
	// derive from the same resolution), so a plain lookup covers the common
	// case. This also handles the drift window where a fallback row predates
	// a geocoder improvement: the old row keeps collecting follows until the
	// upgrade-scene-scopes backfill merges it into a metro row — follows are
	// preserved either way.
	if id, ok, err := s.lookupBySlug(canonicalSlug); err != nil {
		return 0, err
	} else if ok {
		return id, nil
	}

	// Targetless ON CONFLICT DO NOTHING: any unique collision (scope indexes
	// or slug) means a concurrent request won the race — the re-lookup below
	// returns the winner's row.
	var metro *string
	if scope.isMetro() {
		metro = &scope.metro
	}
	if err := s.db.Exec(
		`INSERT INTO scenes (metro, city, state, slug) VALUES (?, ?, ?, ?) ON CONFLICT DO NOTHING`,
		metro, scope.city, scope.state, canonicalSlug,
	).Error; err != nil {
		return 0, fmt.Errorf("failed to create scene row: %w", err)
	}

	id, ok, err := s.lookupBySlug(canonicalSlug)
	if err != nil {
		return 0, err
	}
	if !ok {
		return 0, fmt.Errorf("scene row missing after upsert for slug %q", canonicalSlug)
	}
	return id, nil
}

// LookupSceneID resolves a slug to its registry row id WITHOUT creating one —
// for read paths (follower counts) where an absent row just means zero
// follows. ok=false when no row exists yet. Unknown slugs still 404 via
// ParseSceneSlug so a typo'd URL doesn't read as an empty scene.
func (s *SceneService) LookupSceneID(slug string) (uint, bool, error) {
	_, canonicalSlug, err := s.canonicalScope(slug)
	if err != nil {
		return 0, false, err
	}
	return s.lookupBySlug(canonicalSlug)
}

// SceneRegistryRow returns the registry row for hydration (name/slug in
// /me/following). ok=false when the id has no row (e.g. deleted).
func (s *SceneService) SceneRegistryRow(id uint) (*catalogm.Scene, bool, error) {
	var row catalogm.Scene
	res := s.db.Where("id = ?", id).Limit(1).Find(&row)
	if res.Error != nil {
		return nil, false, fmt.Errorf("failed to load scene row: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return nil, false, nil
	}
	return &row, true, nil
}

// canonicalScope resolves an incoming slug to its scope + CANONICAL slug.
// ParseSceneSlug canonicalizes member cities to the metro principal; the slug
// is then re-derived from that canonical city/state, so every alias of a
// scene converges on one row. Metro resolution is attempted on EVERY call
// (never trusting a stored fallback row) so a geocoder improvement upgrades
// the scope immediately for new rows.
func (s *SceneService) canonicalScope(slug string) (sceneScope, string, error) {
	city, state, err := s.ParseSceneSlug(slug)
	if err != nil {
		return sceneScope{}, "", err
	}
	scope := s.scopeFor(city, state)
	return scope, buildSceneSlug(scope.city, scope.state), nil
}

// sceneDescription returns the curated description for a scene slug, or nil
// when no registry row (or no description) exists — the common case, since
// rows materialize lazily. Fills the GetSceneDetail field that was stubbed
// "nil until scenes table exists".
func (s *SceneService) sceneDescription(slug string) *string {
	var row catalogm.Scene
	res := s.db.Select("description").Where("slug = ?", slug).Limit(1).Find(&row)
	if res.Error != nil || res.RowsAffected == 0 {
		return nil
	}
	return row.Description
}

func (s *SceneService) lookupBySlug(slug string) (uint, bool, error) {
	var row catalogm.Scene
	res := s.db.Select("id").Where("slug = ?", slug).Limit(1).Find(&row)
	if res.Error != nil {
		return 0, false, fmt.Errorf("failed to look up scene row: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return 0, false, nil
	}
	return row.ID, true, nil
}
