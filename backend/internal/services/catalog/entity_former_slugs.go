package catalog

import (
	"fmt"
	"strconv"

	"gorm.io/gorm"

	catalogm "psychic-homily-backend/internal/models/catalog"
)

// entity_former_slugs maps a slug a show, venue or artist no longer answers at
// to the entity that now stands for it. The table's triggers record a slug
// change and drop a slug a live row takes; the merges in this package record
// the loser's slug against the winner; the shared entity-ref inventory moves a
// merged loser's rows to the winner and drops a deleted entity's rows. So a
// row always names an entity id, never a slug, and the slug served for it is
// the entity's live one: every former slug is one redirect from its page.

// formerSlugFamily is one entity family entity_former_slugs records.
type formerSlugFamily struct {
	// table is the family's table. Interpolated into SQL, so it MUST stay a
	// literal in formerSlugFamilies.
	table string
	// servedWhere, when set, is the predicate (on alias e) the family's
	// existence probe also applies, with servedArg bound to it, so a former
	// slug resolves only to an entity whose page the probe serves.
	servedWhere string
	servedArg   any
}

// formerSlugFamilies is every entity type entity_former_slugs holds. The
// table's CHECK constraint names the same three values.
var formerSlugFamilies = map[polymorphicEntityType]formerSlugFamily{
	entityTypeShow:   {table: "shows", servedWhere: "e.status = ?", servedArg: catalogm.ShowStatusApproved},
	entityTypeVenue:  {table: "venues"},
	entityTypeArtist: {table: "artists"},
}

// formerSlugEntityByPrefix maps the existence probe's {entity_type} path value
// to the family it names.
var formerSlugEntityByPrefix = map[string]polymorphicEntityType{
	"shows":   entityTypeShow,
	"venues":  entityTypeVenue,
	"artists": entityTypeArtist,
}

// isEntityIDSegment reports whether a URL segment names an entity by numeric
// id, the rule idOrSlugCondition applies. An id segment has no former slug,
// and a live slug of this shape cannot be served as a slug.
func isEntityIDSegment(segment string) bool {
	_, err := strconv.ParseUint(segment, 10, 32)
	return err == nil
}

// lookupFormerSlug resolves a former slug of entity to the entity that stands
// for it now: its id and its live slug. found is false when the slug is not a
// former slug of that family, when the entity no longer exists or is not
// served, and when its live slug is empty, equal to the former slug, or reads
// as an id. A segment that reads as an id is never looked up.
func lookupFormerSlug(db *gorm.DB, entity polymorphicEntityType, slug string) (id uint, currentSlug string, found bool, err error) {
	family, ok := formerSlugFamilies[entity]
	if !ok {
		return 0, "", false, fmt.Errorf("former slug lookup: %q keeps no former slugs", string(entity))
	}
	if slug == "" || isEntityIDSegment(slug) {
		return 0, "", false, nil
	}

	args := []any{string(entity), slug}
	served := ""
	if family.servedWhere != "" {
		served = " AND " + family.servedWhere
		args = append(args, family.servedArg)
	}
	// #nosec G201 -- the table and the predicate come from formerSlugFamilies,
	// never from caller input; the type, the slug and the predicate's value are
	// bound parameters.
	query := fmt.Sprintf(`
		SELECT e.id, e.slug
		  FROM entity_former_slugs f
		  JOIN %s e ON e.id = f.entity_id
		 WHERE f.entity_type = ?
		   AND f.former_slug = ?
		   AND e.slug IS NOT NULL
		   AND e.slug <> ''
		   AND e.slug <> f.former_slug%s
		 LIMIT 1`, family.table, served)

	var row struct {
		ID   uint
		Slug string
	}
	if err := db.Raw(query, args...).Scan(&row).Error; err != nil {
		return 0, "", false, fmt.Errorf("former slug lookup: %w", err)
	}
	if row.ID == 0 || isEntityIDSegment(row.Slug) {
		return 0, "", false, nil
	}
	return row.ID, row.Slug, true, nil
}

// VenueForFormerSlug resolves a former venue slug to the venue that stands for
// it now. See lookupFormerSlug for when found is false.
func (s *VenueService) VenueForFormerSlug(slug string) (venueID uint, currentSlug string, found bool, err error) {
	if s.db == nil {
		return 0, "", false, fmt.Errorf("database not initialized")
	}
	return lookupFormerSlug(s.db, entityTypeVenue, slug)
}

// recordMergedAwaySlug points the losing entity's slug at the winner. It runs
// inside the merge's transaction, before the loser is deleted. A loser with no
// slug records nothing.
func recordMergedAwaySlug(tx *gorm.DB, entity polymorphicEntityType, winnerID, loserID uint) error {
	family, ok := formerSlugFamilies[entity]
	if !ok {
		return fmt.Errorf("record merged slug: %q keeps no former slugs", string(entity))
	}
	if winnerID == 0 || loserID == 0 || winnerID == loserID {
		return fmt.Errorf("record merged slug: winner and loser must be two distinct %ss (got %d, %d)",
			string(entity), winnerID, loserID)
	}
	// #nosec G201 -- the table comes from formerSlugFamilies, never from caller
	// input; the type and both ids are bound parameters.
	stmt := fmt.Sprintf(`
		INSERT INTO entity_former_slugs (entity_type, former_slug, entity_id)
		SELECT ?, l.slug, ?
		  FROM %s l
		 WHERE l.id = ?
		   AND l.slug IS NOT NULL
		   AND l.slug <> ''
		ON CONFLICT (entity_type, former_slug)
		DO UPDATE SET entity_id = EXCLUDED.entity_id, created_at = now()`, family.table)
	if err := tx.Exec(stmt, string(entity), winnerID, loserID).Error; err != nil {
		return fmt.Errorf("record merged %s slug: %w", string(entity), err)
	}
	return nil
}
