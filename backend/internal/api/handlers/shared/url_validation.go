package shared

import (
	"fmt"

	"github.com/danielgtaylor/huma/v2"

	"psychic-homily-backend/internal/utils"
)

// urlFieldSpec defines validation rules for a known URL field across the
// catalog handler request structs (PSY-525) and the pending-edit suggest
// path (PSY-549). The display name is the user-facing identifier in error
// messages; max length matches the strictest catalog handler request-struct
// maxLength tag.
type urlFieldSpec struct {
	displayName string
	maxLength   int
}

// urlFieldSpecs is the canonical list of URL fields validated at the API
// boundary. PSY-525 introduced http/https scheme validation for these fields
// in the catalog handlers; PSY-549 extends the same rules to the
// pending-edit suggest path so the two write surfaces agree on what
// constitutes a valid URL.
//
// Intentionally omitted: flyer_url, ticket_url, cover_art_url, and
// bandcamp_embed_url. PSY-525 left them to length-only / domain-specific
// checks (e.g. isValidBandcampURL); PSY-549 matches that scope so the
// suggest-edit path doesn't enforce stricter rules than the catalog handler
// would.
var urlFieldSpecs = map[string]urlFieldSpec{
	"image_url":  {displayName: "Image URL", maxLength: 2048},
	"instagram":  {displayName: "Instagram URL", maxLength: 255},
	"facebook":   {displayName: "Facebook URL", maxLength: 500},
	"twitter":    {displayName: "Twitter URL", maxLength: 255},
	"youtube":    {displayName: "YouTube URL", maxLength: 500},
	"spotify":    {displayName: "Spotify URL", maxLength: 500},
	"soundcloud": {displayName: "SoundCloud URL", maxLength: 500},
	"bandcamp":   {displayName: "Bandcamp URL", maxLength: 500},
	"website":    {displayName: "Website URL", maxLength: 500},
}

// ValidateImageURL applies the http/https scheme check to an optional image
// URL. Empty strings pass through so callers that allow "clear via empty
// string" semantics keep working.
//
// Length is enforced separately by the request struct's maxLength tag at
// JSON decode time; this helper only checks the scheme rule.
func ValidateImageURL(imageURL *string) error {
	if imageURL == nil {
		return nil
	}
	return validateScheme(*imageURL, urlFieldSpecs["image_url"].displayName)
}

// ValidateSocialURLs applies the http/https scheme check to the standard set
// of social URL fields shared by artist, venue, label, and festival request
// bodies. Pass nil for fields the surface doesn't accept (e.g. festival
// only takes Website, so the other 7 args are nil).
//
// Length is enforced separately by the request struct's maxLength tag at
// JSON decode time; this helper only checks the scheme rule.
func ValidateSocialURLs(instagram, facebook, twitter, youtube, spotify, soundcloud, bandcamp, website *string) error {
	pairs := [...]struct {
		field string
		value *string
	}{
		{"instagram", instagram},
		{"facebook", facebook},
		{"twitter", twitter},
		{"youtube", youtube},
		{"spotify", spotify},
		{"soundcloud", soundcloud},
		{"bandcamp", bandcamp},
		{"website", website},
	}
	for _, p := range pairs {
		if p.value == nil {
			continue
		}
		if err := validateScheme(*p.value, urlFieldSpecs[p.field].displayName); err != nil {
			return err
		}
	}
	return nil
}

// ValidateFieldChangeValue applies URL validation to a single FieldChange
// proposed via the pending-edit suggest path. For known URL fields it
// enforces both the http/https scheme rule and the per-field length cap;
// other field names pass through (the caller retains authority over fields
// this helper doesn't recognize).
//
// The value is `any` because admin.FieldChange stores OldValue/NewValue as
// interface{} (the underlying row is JSON in pending_entity_edits.field_changes).
// For URL fields, only strings or nil are valid — non-string types are
// rejected with 422.
//
// The length cap is enforced here (unlike ValidateImageURL / ValidateSocialURLs)
// because pending_edits has no Huma struct-tag length enforcement: the
// FieldChange shape carries arbitrary values from the contributor.
//
// Returns a huma.Error422UnprocessableEntity. Empty strings and nil pass
// through (caller decides whether empty means "clear the field").
func ValidateFieldChangeValue(fieldName string, value any) error {
	spec, ok := urlFieldSpecs[fieldName]
	if !ok {
		return nil
	}
	if value == nil {
		return nil
	}
	s, ok := value.(string)
	if !ok {
		return huma.Error422UnprocessableEntity(
			fmt.Sprintf("%s must be a string", spec.displayName),
		)
	}
	if s == "" {
		return nil
	}
	if len(s) > spec.maxLength {
		return huma.Error422UnprocessableEntity(
			fmt.Sprintf("%s must be %d characters or fewer", spec.displayName, spec.maxLength),
		)
	}
	return validateScheme(s, spec.displayName)
}

// validateScheme calls utils.ValidateHTTPURL and translates failures into
// huma 422 errors.
func validateScheme(value, displayName string) error {
	if err := utils.ValidateHTTPURL(value, displayName); err != nil {
		return huma.Error422UnprocessableEntity(err.Error())
	}
	return nil
}
