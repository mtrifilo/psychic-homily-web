package catalog

import (
	"psychic-homily-backend/internal/services/catalog"
)

// parseTagFilter normalizes the `tags=` and `tag_match=` query params used
// by the multi-tag browse filter (PSY-309). It splits on commas, trims
// whitespace, lowercases each slug, and deduplicates. `match` accepts the
// string "any" (case-insensitive) for OR semantics; any other value —
// including "all" or empty — means AND.
func parseTagFilter(tags, match string) catalog.TagFilter {
	return catalog.ParseTagFilter(tags, match)
}
