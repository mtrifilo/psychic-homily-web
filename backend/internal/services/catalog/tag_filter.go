package catalog

import (
	"strings"

	"gorm.io/gorm"
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
