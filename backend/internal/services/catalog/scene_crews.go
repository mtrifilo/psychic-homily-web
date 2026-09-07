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
	// discriminator and the show status sit in JOINs above the WHERE, then the
	// crew category, then the venue predicate inside the EXISTS. A swap here
	// returns wrong rows rather than erroring, since all four bind strings.
	args := make([]any, 0, len(vargs)+3)
	args = append(args, catalogm.TagEntityShow, catalogm.ShowStatusApproved, catalogm.TagCategoryCrew)
	args = append(args, vargs...)

	// Notes on the query below:
	//
	//   - Venue membership is a SEMI-join. show_venues is a many-to-many and
	//     nothing here projects from it, so EXISTS stops at a show's first room
	//     in scope; joining it in would fan a two-room show out to two rows and
	//     count it twice.
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
	var crews []contracts.SceneCrewSummary
	if err := s.db.Raw(`
		SELECT t.slug AS slug,
		       t.name AS name,
		       COUNT(*) AS show_count
		FROM tags t
		JOIN entity_tags et ON et.tag_id = t.id AND et.entity_type = ?
		JOIN shows s ON s.id = et.entity_id AND s.status = ?
		WHERE t.category = ?
		  AND EXISTS (
		      SELECT 1
		      FROM show_venues sv
		      JOIN venues v ON v.id = sv.venue_id
		      WHERE sv.show_id = s.id AND `+vp+`
		  )
		GROUP BY t.id
		ORDER BY show_count DESC, t.name ASC
	`, args...).Scan(&crews).Error; err != nil {
		return nil, fmt.Errorf("failed to list scene crews: %w", err)
	}
	if crews == nil {
		crews = []contracts.SceneCrewSummary{}
	}
	return crews, nil
}
