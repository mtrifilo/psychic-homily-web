package catalog

// VenueAllowedEditFields enumerates the columns a contributor (or trusted
// user on the auto-apply path) is allowed to change via the pending-edit
// pipeline. See ArtistAllowedEditFields for the full rationale.
//
// MUST stay in sync with frontend EDITABLE_FIELDS.venue in
// frontend/features/contributions/types.ts.
//
// capacity is the only field HERE whose value arrives as a JSON NUMBER rather
// than a string, so the suggest-edit validator (validateBoundedInt) and the
// approve path (NarrowNumericUpdates) both handle it explicitly.
//
// The other integer-backed contributor-editable columns are on sibling
// allowlists: labels.founded_year and releases.release_year, gated the same way
// since PSY-1703. All three read one registry,
// contracts.NumericEditFieldBounds.
var VenueAllowedEditFields = map[string]bool{
	"name":        true,
	"address":     true,
	"city":        true,
	"state":       true,
	"country":     true,
	"zipcode":     true,
	"capacity":    true, // Room capacity; see the numeric-field note above
	"age_policy":  true, // House-default age rule, free text
	"description": true,
	"image_url":   true,
	"instagram":   true,
	"facebook":    true,
	"twitter":     true,
	"youtube":     true,
	"spotify":     true,
	"soundcloud":  true,
	"bandcamp":    true,
	"website":     true,
}
