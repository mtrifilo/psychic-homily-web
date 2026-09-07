package catalog

import (
	"fmt"

	apperrors "psychic-homily-backend/internal/errors"
	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/contracts"
)

// Scene-scoped crew tags — the scene page's crews chip row.
//
// A crew is a tag (tags.category = crew), not an entity, so "the crews of this
// scene" is derived: crew tags applied to shows whose venues sit in the scene's
// scope. The scope comes from venuePredicate rather than a second spelling of
// "a room in this town", so a show's scene membership here cannot disagree with
// the shows, gaps and collections rails on the same page.

// sceneCrewRow is the flat scan target for the ranked crew query.
type sceneCrewRow struct {
	Slug      string `gorm:"column:slug"`
	Name      string `gorm:"column:name"`
	ShowCount int    `gorm:"column:show_count"`
}

// GetSceneCrews implements contracts.SceneServiceInterface. The ranking rule
// and the payload's shape are documented there; what follows is how it is
// computed.
func (s *SceneService) GetSceneCrews(city, state string) ([]contracts.SceneCrewSummary, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	scope := s.scopeFor(city, state)

	// Same existence gate as /gaps and /collections: a slug that resolves to a
	// real place but not to a scene 404s here too, so the crews row does not
	// answer 200 with an empty list while every sibling route on the page
	// answers 404. The permissive /new-artists form is the wrong model for a
	// list whose emptiness reads as an editorial claim about the town.
	if n, err := s.verifiedVenueCount(scope); err != nil {
		return nil, fmt.Errorf("failed to count venues: %w", err)
	} else if n < sceneMinVenues {
		return nil, apperrors.ErrSceneNotFound(fmt.Sprintf("scene not found: %s, %s", city, state))
	}

	vp, vargs := scope.venuePredicate("v")

	// Bind args go in SQL TEXT order, not logical order: the entity-type
	// discriminator sits in a JOIN above the WHERE, so it leads, then the venue
	// predicate, then the crew category and the show status. A swap here
	// returns wrong rows rather than erroring, since all four bind strings.
	args := make([]any, 0, len(vargs)+3)
	args = append(args, catalogm.TagEntityShow)
	args = append(args, vargs...)
	args = append(args, catalogm.TagCategoryCrew, catalogm.ShowStatusApproved)

	// Notes on the query below:
	//
	//   - COUNT(DISTINCT s.id), not COUNT(*): show_venues is a many-to-many, so
	//     a show booked into two rooms of the same scene joins twice and would
	//     otherwise count twice for its crew.
	//   - The venue scope is the BARE venuePredicate, not trackedVenuePredicate.
	//     `verified` is a publication gate on a room's address, and no address is
	//     published here. A crew that books only DIY rooms is exactly the
	//     knowledge this row exists to surface, so it counts.
	//   - Shows only. entity_tags is polymorphic and the same crew tag may sit
	//     on artists or venues; entity_type is pinned to show so the count means
	//     one thing. Widening the edge would change what the published number
	//     claims, not just its size.
	//   - Approved shows only, all dates. Any status filter looser than this
	//     would let submissions no reader can see move a chip up the row.
	//   - Count then name is a TOTAL order: migration 000051 puts a unique index
	//     on LOWER(tags.name), so no two crews can tie on both legs and no
	//     further tiebreak is reachable.
	var rows []sceneCrewRow
	if err := s.db.Raw(`
		SELECT t.slug AS slug,
		       t.name AS name,
		       COUNT(DISTINCT s.id) AS show_count
		FROM tags t
		JOIN entity_tags et ON et.tag_id = t.id AND et.entity_type = ?
		JOIN shows s ON s.id = et.entity_id
		JOIN show_venues sv ON sv.show_id = s.id
		JOIN venues v ON v.id = sv.venue_id
		WHERE `+vp+`
		  AND t.category = ?
		  AND s.status = ?
		GROUP BY t.id, t.slug, t.name
		ORDER BY show_count DESC, t.name ASC
	`, args...).Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("failed to list scene crews: %w", err)
	}

	crews := make([]contracts.SceneCrewSummary, 0, len(rows))
	for _, r := range rows {
		crews = append(crews, contracts.SceneCrewSummary{
			Slug:      r.Slug,
			Name:      r.Name,
			ShowCount: r.ShowCount,
		})
	}
	return crews, nil
}
