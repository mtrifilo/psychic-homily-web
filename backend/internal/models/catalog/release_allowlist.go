package catalog

// ReleaseAllowedEditFields enumerates the columns a contributor (or trusted
// user on the auto-apply path) is allowed to change via the pending-edit
// pipeline. See ArtistAllowedEditFields for the full rationale.
//
// MUST stay in sync with frontend EDITABLE_FIELDS.release in
// frontend/features/contributions/types.ts.
var ReleaseAllowedEditFields = map[string]bool{
	"title":         true,
	"release_year":  true,
	"release_date":  true,
	"release_type":  true,
	"cover_art_url": true,
	"description":   true,
}
