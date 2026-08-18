package catalog

// ReleaseAllowedEditFields enumerates the columns a contributor (or trusted
// user on the auto-apply path) is allowed to change via the pending-edit
// pipeline. See ArtistAllowedEditFields for the full rationale.
//
// MUST stay in sync with frontend EDITABLE_FIELDS.release in
// frontend/features/contributions/types.ts.
//
// release_year is integer-backed and arrives as a JSON NUMBER; its type and
// range are gated through contracts.NumericEditFieldBounds (PSY-1703). See the
// numeric-field note on VenueAllowedEditFields. release_date is a separate,
// free-text column and is NOT covered by that registry.
var ReleaseAllowedEditFields = map[string]bool{
	"title":         true,
	"release_year":  true, // Integer column; see the numeric-field note above
	"release_date":  true,
	"release_type":  true,
	"cover_art_url": true,
	"description":   true,
}
