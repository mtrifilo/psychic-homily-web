package catalog

import (
	"strings"

	"gorm.io/gorm"
	catalogm "psychic-homily-backend/internal/models/catalog"
)

// TagFilter captures the tag-filter inputs for browse-list queries
// across the six entity types (artist, show, venue, release, label,
// festival). It supports AND (default) and OR matching semantics.
type TagFilter struct {
	// TagSlugs is the set of tag slugs (already trimmed/lowercased) to
	// filter the current entity by. Empty slice means "no tag filter".
	TagSlugs []string
	// MatchAny is true when the caller wants OR semantics (any of the
	// given tags). False (default) means AND semantics (entity must
	// have every listed tag).
	MatchAny bool
}

// HasTags reports whether the filter should be applied at all.
func (f TagFilter) HasTags() bool { return len(f.TagSlugs) > 0 }

// ParseTagFilter parses comma-separated tag slugs and a `tag_match`
// param into a TagFilter. Slugs are lowercased and trimmed; empty
// entries are dropped. `match` accepts the string "any" for OR
// semantics; anything else (including "all" or "") means AND.
func ParseTagFilter(tags string, match string) TagFilter {
	if tags == "" {
		return TagFilter{}
	}
	parts := strings.Split(tags, ",")
	out := make([]string, 0, len(parts))
	seen := make(map[string]struct{}, len(parts))
	for _, p := range parts {
		s := strings.ToLower(strings.TrimSpace(p))
		if s == "" {
			continue
		}
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return TagFilter{
		TagSlugs: out,
		MatchAny: strings.EqualFold(match, "any"),
	}
}

// ApplyTagFilter narrows a GORM query so that the entity identified by
// `entityType` and `idColumn` (fully qualified, e.g. `artists.id`) is
// constrained to rows matching the tag filter. It uses a subquery with
// `entity_tags JOIN tags` to translate slugs → tag IDs and then filters
// by `entity_id IN (...)` with `HAVING COUNT(DISTINCT tag_id)` equal to
// the number of distinct slugs for AND matching. For OR matching, a
// plain `IN` subquery suffices.
//
// Returns the (possibly unchanged) query.
func ApplyTagFilter(query *gorm.DB, db *gorm.DB, entityType, idColumn string, filter TagFilter) *gorm.DB {
	if !filter.HasTags() {
		return query
	}
	sub := db.Table("entity_tags").
		Select("entity_tags.entity_id").
		Joins("JOIN tags ON tags.id = entity_tags.tag_id").
		Where("entity_tags.entity_type = ? AND LOWER(tags.slug) IN ?", entityType, filter.TagSlugs).
		Group("entity_tags.entity_id")
	if !filter.MatchAny {
		sub = sub.Having("COUNT(DISTINCT entity_tags.tag_id) = ?", len(filter.TagSlugs))
	}
	return query.Where(idColumn+" IN (?)", sub)
}

// ApplyTransitiveArtistTagFilter narrows a GORM query so that rows from a
// container entity (show or festival) are constrained to those whose lineup
// includes artists matching the tag filter (PSY-499).
//
// Shows and festivals are not directly tagged with genre/locale tags — they
// inherit meaning from the artists on their bill. This function encodes that
// semantic: "a show matches `shoegaze` when any billed artist has the
// `shoegaze` tag". It resolves tag slugs via `tags`, joins through the
// lineup junction table (e.g. `show_artists`) and `entity_tags` scoped to
// `entity_type='artist'`, and constrains the outer query to the resulting
// distinct container IDs.
//
// For multi-tag AND semantics the filter requires the collective lineup to
// cover all N tags (any combination across artists) — this is more useful
// for discovery than requiring a single artist to carry every tag. For OR
// semantics, any lineup artist having any of the tags is sufficient.
//
// `junctionTable` is the lineup junction (e.g. "show_artists",
// "festival_artists"), `containerIDColumn` is its container FK column (e.g.
// "show_id", "festival_id"), `artistIDColumn` is its artist FK column
// (always "artist_id" for both junctions), and `idColumn` is the outer
// query's qualified ID (e.g. "shows.id", "festivals.id").
//
// Returns the (possibly unchanged) query.
func ApplyTransitiveArtistTagFilter(
	query *gorm.DB,
	db *gorm.DB,
	junctionTable, containerIDColumn, artistIDColumn, idColumn string,
	filter TagFilter,
) *gorm.DB {
	if !filter.HasTags() {
		return query
	}
	sub := db.Table(junctionTable).
		Select(junctionTable+"."+containerIDColumn).
		Joins("JOIN entity_tags ON entity_tags.entity_type = ? AND entity_tags.entity_id = "+junctionTable+"."+artistIDColumn, catalogm.TagEntityArtist).
		Joins("JOIN tags ON tags.id = entity_tags.tag_id").
		Where("LOWER(tags.slug) IN ?", filter.TagSlugs).
		Group(junctionTable + "." + containerIDColumn)
	if !filter.MatchAny {
		sub = sub.Having("COUNT(DISTINCT tags.id) = ?", len(filter.TagSlugs))
	}
	return query.Where(idColumn+" IN (?)", sub)
}

// CountTransitiveArtistTagUsage returns a map of tag_id → count of distinct
// container entities (shows or festivals) whose lineup includes an artist
// tagged with that tag (PSY-499).
//
// Used by the `/tags?entity_type=show` and `/tags?entity_type=festival`
// facet to surface transitive counts — "shoegaze: 3 shows" when 3 distinct
// shows have at least one billed artist tagged `shoegaze`, even though no
// show is directly tagged `shoegaze`.
//
// `tagIDs` are the tag IDs to compute counts for (empty → empty map).
// `junctionTable` is the lineup junction (e.g. "show_artists").
// `containerIDColumn` is its container FK (e.g. "show_id"). `artistIDColumn`
// is the artist FK (always "artist_id"). Tags with zero matches are absent
// from the returned map (callers should treat missing keys as zero).
func CountTransitiveArtistTagUsage(
	db *gorm.DB,
	junctionTable, containerIDColumn, artistIDColumn string,
	tagIDs []uint,
) (map[uint]int64, error) {
	out := make(map[uint]int64)
	if len(tagIDs) == 0 {
		return out, nil
	}
	type row struct {
		TagID uint
		Count int64
	}
	var rows []row
	err := db.Table(junctionTable).
		Select("entity_tags.tag_id AS tag_id, COUNT(DISTINCT "+junctionTable+"."+containerIDColumn+") AS count").
		Joins("JOIN entity_tags ON entity_tags.entity_type = ? AND entity_tags.entity_id = "+junctionTable+"."+artistIDColumn, catalogm.TagEntityArtist).
		Where("entity_tags.tag_id IN ?", tagIDs).
		Group("entity_tags.tag_id").
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		out[r.TagID] = r.Count
	}
	return out, nil
}
