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

	// SCOPE-keyed lookup first — the scope IS the identity anchor. A slug-first
	// lookup was tried and rejected in review: it adopts a wrong-scope row when
	// two scopes derive the same slug, and it strands a metro row whose slug
	// drifted (principal-city text changed in the geo dataset).
	if id, ok, err := s.lookupByScope(scope); err != nil {
		return 0, err
	} else if ok {
		return id, nil
	}

	// Targetless ON CONFLICT DO NOTHING: any unique collision (scope indexes
	// or slug) means some existing row owns part of this identity — the
	// re-lookups below converge on it.
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

	// Re-lookup by scope: the common success path (our insert, or a concurrent
	// request's identical one).
	if id, ok, err := s.lookupByScope(scope); err != nil {
		return 0, err
	} else if ok {
		return id, nil
	}

	// The insert was blocked by the SLUG index: a row of a different scope
	// holds our canonical slug. Same canonical slug ⇒ same scene under the
	// metro model, so converge rather than error:
	//   - metro scope vs a fallback squatter (the drift case — a row created
	//     before the geocoder could resolve the metro): upgrade it in place,
	//     carrying its follows and description.
	//   - otherwise adopt the row as-is (e.g. a fallback resolution colliding
	//     with an existing metro row after a geocoder regression).
	var squatter catalogm.Scene
	res := s.db.Where("slug = ?", canonicalSlug).Limit(1).Find(&squatter)
	if res.Error != nil {
		return 0, fmt.Errorf("failed to look up scene row: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return 0, fmt.Errorf("scene row missing after upsert for slug %q", canonicalSlug)
	}
	if scope.isMetro() && squatter.Metro == nil {
		if err := s.db.Model(&catalogm.Scene{}).Where("id = ? AND metro IS NULL", squatter.ID).
			Updates(map[string]any{"metro": scope.metro, "city": scope.city, "state": scope.state}).
			Error; err != nil {
			return 0, fmt.Errorf("failed to upgrade scene row scope: %w", err)
		}
	}
	return squatter.ID, nil
}

// LookupSceneID resolves a slug to its registry row id WITHOUT creating one —
// for read paths (follower counts) where an absent row just means zero
// follows. ok=false when no row exists yet. Unknown slugs still 404 via
// ParseSceneSlug so a typo'd URL doesn't read as an empty scene. Scope-keyed
// like GetOrCreateSceneID, with a slug fallback for the drift window where an
// old-scope row still holds the identity.
func (s *SceneService) LookupSceneID(slug string) (uint, bool, error) {
	scope, canonicalSlug, err := s.canonicalScope(slug)
	if err != nil {
		return 0, false, err
	}
	if id, ok, err := s.lookupByScope(scope); err != nil || ok {
		return id, ok, err
	}
	return s.lookupBySlug(canonicalSlug)
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

// lookupByScope finds the row anchored on the scene's identity: the CBSA code
// for metro scenes, the literal (city, state) for fallbacks.
func (s *SceneService) lookupByScope(scope sceneScope) (uint, bool, error) {
	var row catalogm.Scene
	q := s.db.Select("id")
	if scope.isMetro() {
		q = q.Where("metro = ?", scope.metro)
	} else {
		q = q.Where("metro IS NULL AND city = ? AND state = ?", scope.city, scope.state)
	}
	res := q.Limit(1).Find(&row)
	if res.Error != nil {
		return 0, false, fmt.Errorf("failed to look up scene row: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return 0, false, nil
	}
	return row.ID, true, nil
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
