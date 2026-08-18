package catalog

// LabelAllowedEditFields enumerates the columns a contributor (or trusted
// user on the auto-apply path) is allowed to change via the pending-edit
// pipeline. See ArtistAllowedEditFields for the full rationale.
//
// MUST stay in sync with frontend EDITABLE_FIELDS.label in
// frontend/features/contributions/types.ts.
//
// founded_year is integer-backed and arrives as a JSON NUMBER; its type and
// range are gated through contracts.NumericEditFieldBounds (PSY-1703). See the
// numeric-field note on VenueAllowedEditFields.
var LabelAllowedEditFields = map[string]bool{
	"name":         true,
	"founded_year": true, // Integer column; see the numeric-field note above
	"city":         true,
	"state":        true,
	"country":      true,
	"description":  true,
	"image_url":    true,
	"instagram":    true,
	"facebook":     true,
	"twitter":      true,
	"youtube":      true,
	"spotify":      true,
	"soundcloud":   true,
	"bandcamp":     true,
	"website":      true,
}
