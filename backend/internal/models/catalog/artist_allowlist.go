package catalog

// ArtistAllowedEditFields enumerates the columns a contributor (or trusted
// user on the auto-apply path) is allowed to change via the pending-edit
// pipeline. Anything not listed here is rejected by ApprovePendingEdit
// before Updates() is called, so columns like id, slug, submitted_by,
// data_source, is_admin, password_hash, etc. cannot be smuggled through
// the field_changes JSONB blob.
//
// MUST stay in sync with the frontend EDITABLE_FIELDS map in
// frontend/features/contributions/types.ts and with the backend suggest-edit
// validator. Adding a column here without also exposing it on the frontend
// is harmless (just unused); adding it on the frontend without listing it
// here will cause submissions to be rejected at approve time.
//
// Admin-only fields (status, verified, auto_approve, etc.) are intentionally
// excluded — those go through the typed admin Edit handlers (UpdateArtistHandler,
// etc.) which have per-field request struct schemas.
var ArtistAllowedEditFields = map[string]bool{
	"name":               true,
	"city":               true,
	"state":              true,
	"country":            true,
	"description":        true,
	"bandcamp_embed_url": true,
	"image_url":          true,
	"instagram":          true,
	"facebook":           true,
	"twitter":            true,
	"youtube":            true,
	"spotify":            true,
	"soundcloud":         true,
	"bandcamp":           true,
	"website":            true,
}
