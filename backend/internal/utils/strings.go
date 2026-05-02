package utils

// NilIfEmpty returns nil when s is the empty string, otherwise returns
// a pointer to s. It is intended for normalizing optional string fields
// before writing them to GORM update maps so that an empty input lands
// as SQL NULL rather than the empty string "".
//
// Use it at the boundary where a non-pointer string value (typically
// the dereferenced *req.X) is being assigned into a column that is
// nullable in the database. Both handlers and services may import this
// package; placing the helper here (rather than in the handler layer)
// keeps service-side update builders independent of HTTP plumbing.
func NilIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
