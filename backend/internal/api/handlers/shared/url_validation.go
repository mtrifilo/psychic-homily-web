package shared

import (
	"fmt"
	"net/url"
	"strings"

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
// constitutes a valid URL. PSY-747 closes the gaps for cover_image_url
// (collection), cover_art_url (release), and ticket_url (show), which were
// previously length-only and accepted javascript:/data: schemes.
//
// Intentionally omitted: flyer_url and bandcamp_embed_url — PSY-525 left them
// to length-only / domain-specific checks (e.g. isValidBandcampURL), and the
// suggest-edit path matches that scope so it doesn't enforce stricter rules
// than the catalog handler would.
var urlFieldSpecs = map[string]urlFieldSpec{
	"image_url":       {displayName: "Image URL", maxLength: 2048},
	"cover_image_url": {displayName: "Cover image URL", maxLength: 2048},
	"cover_art_url":   {displayName: "Cover art URL", maxLength: 2048},
	"ticket_url":      {displayName: "Ticket URL", maxLength: 500},
	"instagram":       {displayName: "Instagram URL", maxLength: 255},
	"facebook":        {displayName: "Facebook URL", maxLength: 500},
	"twitter":         {displayName: "Twitter URL", maxLength: 255},
	"youtube":         {displayName: "YouTube URL", maxLength: 500},
	"spotify":         {displayName: "Spotify URL", maxLength: 500},
	"soundcloud":      {displayName: "SoundCloud URL", maxLength: 500},
	"bandcamp":        {displayName: "Bandcamp URL", maxLength: 500},
	"website":         {displayName: "Website URL", maxLength: 500},
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

// ValidateURLField applies both the http/https scheme check and the per-field
// length cap to an optional URL identified by its urlFieldSpecs key, returning
// a huma 422. Unlike ValidateImageURL (scheme-only, length enforced by a struct
// maxLength tag), this helper also caps length, so it works for boundary fields
// that lack a struct-tag length cap (e.g. collection cover_image_url, release
// cover_art_url, show ticket_url — PSY-747).
//
// Empty strings and nil pass through so callers that allow "clear via empty
// string" semantics keep working. Unknown field names return nil rather than
// panicking, so a typo degrades to no-op validation rather than a runtime
// crash; callers pass a literal key matched against urlFieldSpecs.
func ValidateURLField(fieldName string, value *string) error {
	if value == nil {
		return nil
	}
	if err := URLSchemeError(fieldName, *value); err != nil {
		return huma.Error422UnprocessableEntity(err.Error())
	}
	return nil
}

// socialHostSuffixes anchors each platform social field to its known hosts, so
// a hostile value (e.g. https://evil.test/artist/x in the spotify field, which
// renders as a SocialLinks href / embed source) can't be stored (PSY-1113). A
// field absent here (website) accepts any host. A host matches when it equals a
// base or is a subdomain of it — covering open.spotify.com, <artist>.bandcamp.com,
// m.facebook.com, music.youtube.com, www.*, etc.
//
// This is a broad HOST floor for the free-form social fields. The dedicated
// embed endpoints use the stricter, path-aware isValidBandcampURL /
// isValidSpotifyURL (catalog/artist.go) — change platform-host rules with both
// in mind.
//
// Redirector / short-link hosts (fb.me, t.co, youtube-nocookie.com) are
// intentionally excluded: they can't be statically verified to land on-platform,
// which is the point of the anchor. `website` is the escape hatch for any host.
var socialHostSuffixes = map[string][]string{
	"instagram":  {"instagram.com"},
	"facebook":   {"facebook.com", "fb.com"},
	"twitter":    {"twitter.com", "x.com"},
	"youtube":    {"youtube.com", "youtu.be"},
	"spotify":    {"spotify.com"},
	"soundcloud": {"soundcloud.com"},
	"bandcamp":   {"bandcamp.com"},
}

// validateSocialHost rejects a social-platform URL whose host isn't on that
// platform's allowlist. Fields not in socialHostSuffixes (website, image_url,
// ...) are unrestricted. Assumes `value` already passed the scheme check, so a
// parse failure here just means "skip" (the scheme check already rejected it).
func validateSocialHost(field, value string) error {
	bases, restricted := socialHostSuffixes[field]
	if !restricted || strings.TrimSpace(value) == "" {
		return nil
	}
	u, err := url.Parse(strings.TrimSpace(value))
	if err != nil {
		return nil
	}
	host := strings.ToLower(u.Hostname())
	for _, base := range bases {
		if host == base || strings.HasSuffix(host, "."+base) {
			return nil
		}
	}
	return huma.Error422UnprocessableEntity(
		fmt.Sprintf(
			"%s must be a link on %s",
			urlFieldSpecs[field].displayName,
			strings.Join(bases, " or "),
		),
	)
}

// ValidateSocialURLs applies the http/https scheme check AND a per-platform host
// allowlist (PSY-1113) to the standard set of social URL fields shared by
// artist, venue, label, and festival request bodies. Pass nil for fields the
// surface doesn't accept (e.g. festival only takes Website, so the other 7 args
// are nil). `website` is host-unrestricted (any https host).
//
// Length is enforced separately by the request struct's maxLength tag at
// JSON decode time.
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
		if err := validateSocialHost(p.field, *p.value); err != nil {
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
	if err := validateScheme(s, spec.displayName); err != nil {
		return err
	}
	return validateSocialHost(fieldName, s)
}

// URLSchemeError validates the http/https scheme and per-field length cap for
// a known URL field and returns the underlying error (NOT wrapped as a huma
// 422). It exists for the huma Resolve()-style request bodies that collect
// field-level *huma.ErrorDetail entries with a Location — the caller wraps the
// returned error so the rejection keeps its body-field attribution (PSY-747,
// show ticket_url create path). Returns nil for unknown fields, nil/empty
// values, or valid URLs.
func URLSchemeError(fieldName, value string) error {
	spec, ok := urlFieldSpecs[fieldName]
	if !ok || value == "" {
		return nil
	}
	if len(value) > spec.maxLength {
		return fmt.Errorf("%s must be %d characters or fewer", spec.displayName, spec.maxLength)
	}
	return utils.ValidateHTTPURL(value, spec.displayName)
}

// validateScheme calls utils.ValidateHTTPURL and translates failures into
// huma 422 errors.
func validateScheme(value, displayName string) error {
	if err := utils.ValidateHTTPURL(value, displayName); err != nil {
		return huma.Error422UnprocessableEntity(err.Error())
	}
	return nil
}
