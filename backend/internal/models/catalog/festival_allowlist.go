package catalog

// FestivalAllowedEditFields enumerates the columns a contributor (or trusted
// user on the auto-apply path) is allowed to change via the pending-edit
// pipeline. See ArtistAllowedEditFields for the full rationale.
//
// MUST stay in sync with frontend EDITABLE_FIELDS.festival in
// frontend/features/contributions/types.ts.
var FestivalAllowedEditFields = map[string]bool{
	"name":          true,
	"description":   true,
	"location_name": true,
	"city":          true,
	"state":         true,
	"country":       true,
	"website":       true,
	"ticket_url":    true,
	"flyer_url":     true,
}
