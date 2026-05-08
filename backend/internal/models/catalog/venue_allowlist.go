package catalog

// VenueAllowedEditFields enumerates the columns a contributor (or trusted
// user on the auto-apply path) is allowed to change via the pending-edit
// pipeline. See ArtistAllowedEditFields for the full rationale.
//
// MUST stay in sync with frontend EDITABLE_FIELDS.venue in
// frontend/features/contributions/types.ts.
var VenueAllowedEditFields = map[string]bool{
	"name":        true,
	"address":     true,
	"city":        true,
	"state":       true,
	"country":     true,
	"zipcode":     true,
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
