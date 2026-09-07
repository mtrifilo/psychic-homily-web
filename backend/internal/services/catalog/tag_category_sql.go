package catalog

import (
	catalogm "psychic-homily-backend/internal/models/catalog"
)

// descriptiveTagCategorySQL is the SQL spelling of
// catalogm.IsDescriptiveTagCategory, true for the tags a reader may rank
// against each other.
//
// The readers that produce a ranking filter HERE rather than dropping rows
// after the query: the rank positions, the page total and the LIMIT are all
// computed by the database, so a row removed afterwards would leave a gap in
// the ranking and a total that counts what is not shown.
//
// Binds nothing, so a caller assembling SQL by concatenation can drop it in
// without shifting the positional arguments around it. The category it compares
// against is a compile-time constant, never a value from a request.
//
// alias is the table alias the enclosing query binds tags to and is a literal
// in the calling code.
func descriptiveTagCategorySQL(alias string) string {
	return "LOWER(TRIM(" + alias + ".category)) <> '" + catalogm.TagCategoryCrew + "'"
}
