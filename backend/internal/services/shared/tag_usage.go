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
// So every PUBLIC response body carries the VISIBLE count, computed here, and the
// two public listings ORDER BY it as well (services/catalog/tag_counts.go states
// why the number and the ordering key have to be one expression). The raw column
// is still read in two places, and each is named rather than left to be
// discovered:
//
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

	aggregate, aggregateArgs := VisibleTagUsageCountSQL()

	type countRow struct {
		TagID uint
		Count int64
	}
	var rows []countRow
	args := append(append([]interface{}{}, aggregateArgs...), tagIDs)
	err := db.Raw(
		"SELECT tag_id, count FROM ("+aggregate+") AS visible_tag_usage WHERE tag_id IN ?",
		args...,
	).Scan(&rows).Error
	if err != nil {
		logger.Default().Error("visible_tag_usage_count_failed", "error", err.Error())
		return nil, err
	}
	for _, r := range rows {
		counts[r.TagID] = int(r.Count)
	}
	return counts, nil
}

// VisibleTagUsageCountSQL returns the (tag_id, count) aggregate over the whole
// entity_tags table, counting only rows whose tagged entity a public reader may
// see, plus its bind arguments.
//
// THE NUMBER AND THE ORDERING KEY ARE ONE EXPRESSION. A /tags page is SELECTED
// by an ORDER BY and RENDERED with these counts, and if the two came from
// different expressions the page would be ordered by one number and show
// another: a tag displaying 0 could sit above a tag displaying 40, and position
// beside number would state a lower bound on the rows the count withholds. So
// this is the aggregate the listing joins to order itself, and the aggregate
// VisibleTagUsageCounts reads the displayed number from.
//
// UNRESTRICTED BY TAG, because the ordering has to be computed before the page
// is chosen; callers wanting a subset filter the result.
func VisibleTagUsageCountSQL() (string, []interface{}) {
	visible, visibleArgs := VisibleEntityTagsSQL("entity_tags", contracts.ShowViewer{})
	return "SELECT entity_tags.tag_id AS tag_id, COUNT(*) AS count" +
		" FROM entity_tags WHERE " + visible +
		" GROUP BY entity_tags.tag_id", visibleArgs
}
