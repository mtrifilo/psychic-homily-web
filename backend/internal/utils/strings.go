package utils

import "strings"

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

// NilIfBlank is NilIfEmpty with surrounding whitespace stripped first, and it
// returns the TRIMMED value. Use it for curated free-text columns where a
// whitespace-only entry means the same thing as an empty one, and where stored
// values should not differ by incidental padding ("21+" vs " 21+ ") because
// something downstream will group on them.
func NilIfBlank(s string) *string {
	return NilIfEmpty(strings.TrimSpace(s))
}

// NilIfBlankPtr is the pointer-in form of NilIfBlank, for the create-path
// builders that assign an optional *string straight into a model field rather
// than into an update map.
//
// It collapses BOTH nil and blank to nil. That is only correct where "not
// supplied" and "supplied blank" mean the same thing, which is true on a create
// path (both mean: store NULL). Do NOT reach for it on an update path before a
// nil check: there, nil means "leave unchanged" and blank means "clear", and
// collapsing them would silently swallow the clear gesture.
func NilIfBlankPtr(s *string) *string {
	if s == nil {
		return nil
	}
	return NilIfBlank(*s)
}
