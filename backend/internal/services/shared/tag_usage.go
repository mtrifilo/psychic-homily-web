package shared

import (
	"gorm.io/gorm"

	"psychic-homily-backend/internal/logger"
	"psychic-homily-backend/internal/services/contracts"
)

// =============================================================================
// WHAT A TAG'S usage_count MEANS
// =============================================================================
//
// `tags.usage_count` is a denormalised counter incremented once per entity_tags
// row, with no visibility term in it. Some taggable entity types are gated — a
// show can be unapproved and a collection can be private — so the column counts
// rows their own listings withhold. Which ones those are is the registry's
// decision, in entity_visibility.go, not a list repeated here.
//
// That matters because the number is rendered BESIDE those listings. A tag page
// serves the gated membership list, the per-type breakdown and the count in one
// response, and a browse page serves the count next to a link into the list, so
// a count that included the withheld rows would report how many there are by
// subtraction. The withheld count published as arithmetic is the same disclosure
// as the rows.
//
// So every PUBLIC response body carries the VISIBLE count, computed here. The
// raw column is still read in three places, and each is named rather than left
// to be discovered:
//
//   - ORDER BY, where it ranks popularity and no reader sees the number. The
//     page is selected on the column and the counts rendered on it are the
//     visible ones, so a listing's order and its numbers can disagree at the
//     margin. That is the same trade the per-entity-type facet count already
//     makes, and it is the recoverable one.
//   - The low-quality tag queue's THRESHOLDS (catalog/tag_low_quality.go), where
//     the subject is the tag's own hygiene: a tag applied only to private
//     collections is in use, and calling it unused would put it in front of a
//     moderator for deletion.
//   - The two ADMIN tag queues' response bodies, /admin/tags/low-quality and
//     /admin/tags/hierarchy, which show the same column their thresholds and
//     their tree are built from. An admin subtracting those from a public count
//     learns how many gated entities carry a tag, which is far less than
//     /admin/comments/pending already hands them; the moderation queues named in
//     collection_visibility.go are the same exception on the same terms.
//
// PUBLIC TIER, always. These counts sit on anonymous routes, and a shared
// listing whose contents varied by credential would report the difference.
// Passing a viewer would also make the number vary between two callers looking
// at the same page, which is a second way to say the same thing.

// VisibleEntityTagsSQL returns a condition, true for the entity_tags rows whose
// TAGGED ENTITY viewer may see, plus its bind arguments.
//
// The polymorphic gate applied to entity_tags. It delegates to
// VisibleCommentEntitySQL rather than writing its own arms, so entity_tags rows
// and comment rows are decided by one registry: a tag row naming an entity type
// nobody dispositioned does not pass, exactly as a comment row naming one does
// not.
//
// alias is the table alias the enclosing query binds entity_tags to and is a
// literal in the calling code.
func VisibleEntityTagsSQL(alias string, viewer contracts.ShowViewer) (string, []interface{}) {
	return VisibleCommentEntitySQL(alias+".entity_type", alias+".entity_id", viewer)
}

// PublicEntityTagsSQL is VisibleEntityTagsSQL's public tier, inlined so it binds
// nothing.
//
// For a leaderboard or any other shared ranking assembled by concatenation,
// where the placeholders are counted at a distance and one more `?` would shift
// every argument after it. It is the same rule for every caller, which is what a
// public ranking has to be: a rank that counted rows nobody else can see would
// publish those rows as a position.
//
// alias is the table alias the enclosing query binds entity_tags to and is a
// literal in the calling code.
func PublicEntityTagsSQL(alias string) string {
	return PublicEntityTypeArmsSQL(alias+".entity_type", alias+".entity_id")
}

// VisibleTagUsageCounts returns, per tag id, the number of entity_tags rows
// whose tagged entity a public reader may see.
//
// The counterpart of the per-type breakdown on the tag detail page: same
// predicate, grouped by tag instead of by entity type, so a tag's count and the
// sum of its breakdown are the same number by construction rather than by two
// queries agreeing.
//
// A tag with no visible rows is ABSENT from the map, and callers read a missing
// key as zero. That is the honest answer: the tag exists and nothing a public
// reader may see carries it.
//
// A failed query returns the error. Callers must not fall back to the raw
// column, because a fallback is the disclosure with an extra step.
func VisibleTagUsageCounts(db *gorm.DB, tagIDs []uint) (map[uint]int, error) {
	counts := make(map[uint]int, len(tagIDs))
	if db == nil || len(tagIDs) == 0 {
		return counts, nil
	}

	visible, visibleArgs := VisibleEntityTagsSQL("entity_tags", contracts.ShowViewer{})

	type countRow struct {
		TagID uint
		Count int64
	}
	var rows []countRow
	err := db.Table("entity_tags").
		Select("tag_id, COUNT(*) AS count").
		Where("tag_id IN ?", tagIDs).
		Where(visible, visibleArgs...).
		Group("tag_id").
		Scan(&rows).Error
	if err != nil {
		logger.Default().Error("visible_tag_usage_count_failed", "error", err.Error())
		return nil, err
	}
	for _, r := range rows {
		counts[r.TagID] = int(r.Count)
	}
	return counts, nil
}
