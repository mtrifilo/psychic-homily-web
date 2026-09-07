package catalog

import (
	catalogm "psychic-homily-backend/internal/models/catalog"
)

// descriptiveTagCategorySQL is the SQL spelling of
// catalogm.IsDescriptiveTagCategory, true for the tags a reader may rank
// against each other. TestDescriptiveTagCategoryPredicateMatchesSQL runs both
// over the same rows.
//
// Rankings filter HERE rather than over the result, because the rank positions,
// the page total and the LIMIT are the database's.
//
// Binds nothing, so a caller assembling SQL by concatenation can drop it in
// without shifting the positional arguments around it. alias is the table alias
// the enclosing query binds tags to and is a literal in the calling code.
func descriptiveTagCategorySQL(alias string) string {
	return "LOWER(TRIM(" + alias + ".category)) <> '" + catalogm.TagCategoryCrew + "'"
}
