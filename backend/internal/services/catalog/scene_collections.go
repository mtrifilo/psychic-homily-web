package catalog

import (
	"fmt"
	"time"

	apperrors "psychic-homily-backend/internal/errors"
	catalogm "psychic-homily-backend/internal/models/catalog"
	communitym "psychic-homily-backend/internal/models/community"
	"psychic-homily-backend/internal/services/contracts"
)

// Scene-scoped public collections — the scene page's "Collections · {city}"
// rail.
//
// Collections are user-curated and carry no scene column, so this file is
// entirely about DERIVING a collection's relevance to a place. It lives on
// SceneService, in package catalog, for one reason: the definition of "in this
// scene" is `sceneScope` + `artistPredicate` + `venuePredicate`, all unexported
// here. A second, independently-written spelling of "artists based in Phoenix"
// living in package community would give two different answers to one question
// the moment either is edited — the same argument metroScopeFor's doc comment
// makes for the Atlas city rail. Reading the collections tables from catalog is
// already the house pattern for exactly this reason (charts_featured_collection.go).

const (
	// sceneCollectionAbsoluteThreshold is one arm of the rule: a collection with
	// this many scene-local members qualifies regardless of how large it is.
	// It exists so a 40-item national comp that genuinely holds 8 local bands
	// is not shut out by its 32 out-of-town ones, while the ratio arm keeps a
	// tightly-focused 3-of-4 city collection eligible.
	sceneCollectionAbsoluteThreshold = 5

	// sceneCollectionsDefaultLimit backstops a caller that passes no limit.
	// The HTTP layer publishes its own default and bounds; this is the guard
	// for direct service callers, not a second policy.
	sceneCollectionsDefaultLimit = 5
)

// sceneCollectionRow is the flat scan target for the qualifying query.
type sceneCollectionRow struct {
	ID                  uint      `gorm:"column:id"`
	Slug                string    `gorm:"column:slug"`
	Title               string    `gorm:"column:title"`
	CoverImageURL       *string   `gorm:"column:cover_image_url"`
	SceneLocalItemCount int       `gorm:"column:scene_local_item_count"`
	ItemCount           int       `gorm:"column:item_count"`
	ContributorCount    int       `gorm:"column:contributor_count"`
	UpdatedAt           time.Time `gorm:"column:updated_at"`
}

// GetSceneCollections implements contracts.SceneServiceInterface. The
// qualifying rule and the payload's shape are documented there; what follows is
// how it is computed.
func (s *SceneService) GetSceneCollections(city, state string, limit int) ([]contracts.SceneCollectionSummary, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	if limit <= 0 {
		limit = sceneCollectionsDefaultLimit
	}

	scope := s.scopeFor(city, state)

	// Same existence gate as the scene's other front-page rails
	// (GetSceneShowsInRange, GetActiveArtists, GetSceneGraph): a slug that
	// resolves to a real place but not to a scene must 404 here too, or
	// /scenes/{not-a-scene}/collections would answer 200 with an empty rail
	// while every sibling route on the same page answered 404.
	if n, err := s.verifiedVenueCount(scope); err != nil {
		return nil, fmt.Errorf("failed to count venues: %w", err)
	} else if n < sceneMinVenues {
		return nil, apperrors.ErrSceneNotFound(fmt.Sprintf("scene not found: %s, %s", city, state))
	}

	ap, aargs := s.artistPredicate(scope, "a")
	vp, vargs := scope.venuePredicate("v")

	// Bind args go in SQL TEXT order, not logical order — a swap here returns
	// wrong rows rather than erroring, since most of these bind strings (see
	// trackedVenuePredicate's warning). Text order is: artist predicate,
	// venue predicate, the show status, the three entity-type discriminators
	// inside the FILTER, the absolute threshold, then the limit.
	args := make([]any, 0, len(aargs)+len(vargs)+6)
	args = append(args, aargs...)
	args = append(args, vargs...)
	args = append(args,
		catalogm.ShowStatusApproved,
		communitym.CollectionEntityArtist,
		communitym.CollectionEntityVenue,
		communitym.CollectionEntityShow,
		sceneCollectionAbsoluteThreshold,
		limit,
	)

	// Notes on the query below:
	//
	//   - scene_venues uses the BARE venuePredicate, not trackedVenuePredicate:
	//     a venue member counts as scene-local on GEOGRAPHY, not on whether the
	//     room has been verified. `verified` is a PUBLICATION gate (it is what
	//     decides whether a DIY/house venue's street address may be served),
	//     and nothing about a room's address is published here — only whether
	//     it is in town. The scene's upcoming_show_count draws the same line
	//     for the same reason. The consequence is deliberate: a collection of
	//     Phoenix DIY spaces qualifies for the Phoenix rail even though the
	//     scene page's rooms leaderboard, which IS a publication surface, lists
	//     none of them. That collection is precisely the community knowledge
	//     this rail exists to surface.
	//   - scene_shows derives from scene_venues rather than re-spelling the
	//     venue predicate, so a show's scene membership can never disagree
	//     with its venue's. It counts APPROVED shows only, matching how every
	//     other scene surface defines the scene's shows (GetSceneDetail's
	//     upcoming_show_count, the pulse, the week/day pages) — a scene is
	//     never credited here for a booking it does not publish anywhere else.
	//     That also closes a ranking exploit: pending submissions are invisible
	//     to readers but would otherwise be countable, so anyone could pad a
	//     collection onto the rail with shows nobody can see.
	//   - collection_items.entity_id has NO foreign key (the table is
	//     polymorphic), so membership is an IN against the scoped id sets
	//     rather than a join. A member pointing at a deleted entity simply
	//     never matches, and still counts toward item_count — the honest
	//     denominator for "how much of this collection is about here".
	//   - is_public is asserted in exactly ONE place: the outer SELECT, the
	//     only clause that emits a row. The CTEs deliberately do not repeat
	//     it. Two spellings of the privacy gate is two places to forget one.
	//   - local_counts leads, and `totals` reads only the collections it
	//     found. That ordering is the difference between work proportional to
	//     the SCENE and work proportional to the whole site: aggregating
	//     collection_items unfiltered would seq-scan and hash-aggregate every
	//     collection item in the catalog on every request to an anonymous
	//     endpoint. Every other collections aggregate here is scoped by
	//     collection_id first (batchCountItems and friends) and so is this.
	//   - a collection reaches local_counts only by having at least one
	//     scene-local member, since COUNT(*) over a group is never 0. That is
	//     why the outer WHERE carries no `> 0` guard, and why an empty
	//     collection can never qualify.
	//   - the ratio arm is spelled `scene_local * 2 >= item_count` rather than
	//     a division, so the 50% boundary is exact integer arithmetic with no
	//     float rounding: a 7-of-14 collection qualifies, a 6-of-13 does not.
	//   - the whole thing is one round trip. Per-collection follow-up reads
	//     for counts would be the N+1 the batch-read convention exists to
	//     prevent, and contributor_count is derived live (COUNT DISTINCT)
	//     rather than from a counter column, matching batchCountContributors.
	sql := `
		WITH scene_artists AS (
			SELECT a.id FROM artists a WHERE ` + ap + `
		),
		scene_venues AS (
			SELECT v.id FROM venues v WHERE ` + vp + `
		),
		scene_shows AS (
			SELECT DISTINCT sv.show_id AS id
			FROM show_venues sv
			JOIN shows s ON s.id = sv.show_id
			WHERE sv.venue_id IN (SELECT id FROM scene_venues)
			  AND s.status = ?
		),
		local_counts AS (
			SELECT ci.collection_id, COUNT(*) AS scene_local_item_count
			FROM collection_items ci
			WHERE (ci.entity_type = ? AND ci.entity_id IN (SELECT id FROM scene_artists))
			   OR (ci.entity_type = ? AND ci.entity_id IN (SELECT id FROM scene_venues))
			   OR (ci.entity_type = ? AND ci.entity_id IN (SELECT id FROM scene_shows))
			GROUP BY ci.collection_id
		),
		totals AS (
			SELECT ci.collection_id,
			       COUNT(*) AS item_count,
			       COUNT(DISTINCT ci.added_by_user_id) AS contributor_count
			FROM collection_items ci
			WHERE ci.collection_id IN (SELECT collection_id FROM local_counts)
			GROUP BY ci.collection_id
		)
		SELECT c.id,
		       c.slug,
		       c.title,
		       c.cover_image_url,
		       lc.scene_local_item_count,
		       t.item_count,
		       t.contributor_count,
		       c.updated_at
		FROM collections c
		JOIN local_counts lc ON lc.collection_id = c.id
		JOIN totals t ON t.collection_id = c.id
		WHERE c.is_public = true
		  AND (
		        lc.scene_local_item_count >= ?
		     OR lc.scene_local_item_count * 2 >= t.item_count
		      )
		ORDER BY lc.scene_local_item_count DESC,
		         t.contributor_count DESC,
		         c.updated_at DESC,
		         c.id ASC
		LIMIT ?
	`

	// Ranking: scene-local count is the rule, so it leads. The tiebreak is
	// CONTRIBUTOR COUNT, not recency, and that is a deliberate call: adding an
	// item does not touch collections.updated_at (CollectionService.AddItem
	// writes only the collection_items row), so updated_at reflects when
	// someone last edited a title or description — a weak proxy for curation
	// activity, and the wrong thing to promote a collection on. Contributor
	// count is the rail's own displayed metric ("Built by N") and a real
	// signal that a collection is community knowledge rather than one
	// person's list. updated_at still ranks third, and id last, so the order
	// is total and the rail does not reshuffle between requests.
	var rows []sceneCollectionRow
	if err := s.db.Raw(sql, args...).Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("failed to get scene collections: %w", err)
	}

	out := make([]contracts.SceneCollectionSummary, len(rows))
	for i, r := range rows {
		out[i] = contracts.SceneCollectionSummary{
			ID:                  r.ID,
			Slug:                r.Slug,
			Title:               r.Title,
			CoverImageURL:       r.CoverImageURL,
			SceneLocalItemCount: r.SceneLocalItemCount,
			ItemCount:           r.ItemCount,
			ContributorCount:    r.ContributorCount,
			UpdatedAt:           r.UpdatedAt,
		}
	}
	return out, nil
}
