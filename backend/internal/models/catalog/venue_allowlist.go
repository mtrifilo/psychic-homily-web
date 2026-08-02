package catalog

// VenueAllowedEditFields enumerates the columns a contributor (or trusted
// user on the auto-apply path) is allowed to change via the pending-edit
// pipeline. See ArtistAllowedEditFields for the full rationale.
//
// MUST stay in sync with frontend EDITABLE_FIELDS.venue in
// frontend/features/contributions/types.ts.
//
// capacity is the only NUMERIC field on any entity's allowlist. Its value
// therefore arrives as a JSON number rather than a string, which both the
// suggest-edit validator (validateBoundedInt) and the approve path
// (normalizeCapacityUpdate) have to handle explicitly: everything else in the
// pending-edit pipeline can assume text.
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
