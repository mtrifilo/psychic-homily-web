package shared

import (
	"context"
	"fmt"
	"math"
	"sort"
	"unicode/utf8"

	"github.com/danielgtaylor/huma/v2"

	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/utils"
	"psychic-homily-backend/internal/utils/urlguard"
)

// urlFieldSpec defines validation rules for a known URL field across the
// catalog handler request structs (PSY-525) and the pending-edit suggest
// path (PSY-549). The display name is the user-facing identifier in error
// messages; max length matches the strictest catalog handler request-struct
// maxLength tag.
type urlFieldSpec struct {
	displayName string
	maxLength   int
	// fetched marks a field whose stored value is later FETCHED server-side, so
	// it must clear the SSRF host guard (urlguard) on top of the scheme check.
	// The other URL fields here are rendered as an href or an <img> by the
	// browser, never requested by us, so resolving them would buy no safety and
	// would put a DNS lookup on unrelated write paths.
	//
	// The flag is keyed on the FIELD NAME, not on the entity, so marking
	// image_url covers show, venue, label and artist alike. Only the show's is
	// actually fetched (the share-card renderer draws the flyer — PSY-1672);
	// the rest is defense-in-depth on a field that is one feature away from
	// being fetched too, and is cheap because these writes are rare.
	//
	// BEFORE FLIPPING THIS ON ANOTHER FIELD: URLSchemeError does NOT honour it
	// (it has no context.Context to resolve with), and POST /shows validates
	// ticket_url through exactly that path — see the note on URLSchemeError.
	// Flip the flag there and the create path silently stays unguarded while
	// the update path is guarded. TestFetchedFieldsAvoidURLSchemeError is the
	// tripwire; if it fails, thread a context rather than deleting the case.
	fetched bool
	// shape is an extra rule about the FORM of the value, for a field whose
	// column means something narrower than "a URL": bandcamp_embed_url must name
	// one Bandcamp release, not merely sit on a Bandcamp host. nil means the
	// scheme and length rules are the whole contract.
	//
	// It lives on the spec rather than beside it so the registry stays the one
	// place that answers "what is enforced on this field". The `fetched` flag
	// above and the socialHostSuffixes table are the two older answers to the
	// same question; a third free-standing hook would have made the coverage of
	// any given field unreadable without opening every validator.
	//
	// It returns a plain error, not a huma one, because the rules it points at
	// live in internal/utils and know nothing about HTTP. The dispatcher wraps.
	//
	// BEFORE ADDING ONE: like `fetched`, URLSchemeError does NOT honour this
	// (see the note there), so a field validated through that helper stays
	// shape-unchecked. TestShapeRuledFieldsAvoidURLSchemeError is the tripwire.
	shape func(value, fieldName string) error
}

// urlFieldSpecs is the canonical list of URL fields validated at the API
// boundary. PSY-525 introduced http/https scheme validation for these fields
// in the catalog handlers; PSY-549 extends the same rules to the
// pending-edit suggest path so the two write surfaces agree on what
// constitutes a valid URL. PSY-747 closes the gaps for cover_image_url
// (collection), cover_art_url (release), and ticket_url (show), which were
// previously length-only and accepted javascript:/data: schemes.
//
// Intentionally omitted: flyer_url. PSY-525 left it to a length-only check,
// and the suggest-edit path matches that scope so it doesn't enforce stricter
// rules than the catalog handler would.
//
// bandcamp_embed_url is reachable through this path: it sits in
// ArtistAllowedEditFields, so any email-verified contributor can set it with a
// direct suggest-edit call, and the stored value renders as an outbound link
// under a Bandcamp label rather than only inside a sandboxed iframe. It carries
// a `shape` rule rather than a host suffix because a release page is a path as
// much as a host.
var urlFieldSpecs = map[string]urlFieldSpec{
	"image_url":       {displayName: "Image URL", maxLength: 2048, fetched: true},
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
	// Keyed by the constant, not a literal, because the apply-side gate in
	// internal/services/admin selects on the same constant. Last in the map so
	// its comment does not split the block above into two alignment runs.
	//
	// 2048, not the 500 the social fields use: the column is TEXT, and the
	// entity-request queue already caps this same field at 2048. Two boundaries
	// onto one column must not disagree about what fits.
	utils.BandcampEmbedURLField: {
		displayName: utils.BandcampEmbedURLLabel,
		maxLength:   2048,
		shape:       utils.ValidateBandcampEmbedURL,
	},
}

// URLFieldNames returns every field this registry treats as a URL.
//
// Exported so internal/services/admin can ask "is this field a URL?" without
// restating the answer. Its rollback gate has to cover every URL field the
// per-entity edit allowlists expose, and a test that decided for itself what
// counted as a URL would drift from this registry the moment one changed.
func URLFieldNames() []string {
	names := make([]string, 0, len(urlFieldSpecs))
	for field := range urlFieldSpecs {
		names = append(names, field)
	}
	sort.Strings(names)
	return names
}

// ShapeRuledURLFieldNames returns the field names carrying a `shape` rule: the
// URL fields whose stored value must take a particular FORM, beyond being a URL.
//
// Exported for the same reason FetchedURLFieldNames is: internal/services/admin
// re-runs these rules at pending-edit approval time and keeps its own list
// (services do not import handler packages), and its test compares the two so
// adding a rule here without adding it there fails loudly instead of silently
// leaving the approve path ungated.
func ShapeRuledURLFieldNames() []string {
	names := make([]string, 0, len(urlFieldSpecs))
	for field, spec := range urlFieldSpecs {
		if spec.shape != nil {
			names = append(names, field)
		}
	}
	sort.Strings(names)
	return names
}

// FetchedURLFieldNames returns the field names marked `fetched` above: the URL
// fields whose stored value is later requested server-side, and which therefore
// must clear the SSRF host guard.
//
// It is exported so the apply-side copy of this list can be checked against the
// canonical one. internal/services/admin re-runs the guard at pending-edit
// approval time (PSY-1692); it keeps its own map because services do not import
// handler packages (a layering rule here, not a compile constraint), and its
// test compares the two and fails when they drift.
//
// The only caller is that drift test. If a production consumer ever appears,
// prefer moving the registry down into internal/utils/urlguard over widening
// this accessor.
func FetchedURLFieldNames() []string {
	names := make([]string, 0, len(urlFieldSpecs))
	for field, spec := range urlFieldSpecs {
		if spec.fetched {
			names = append(names, field)
		}
	}
	sort.Strings(names)
	return names
}

// boundedTextFieldSpecs caps NON-URL free-text fields that are reachable
// through the contributor suggest-edit queue and are backed by a length-bounded
// column. urlFieldSpecs above cannot serve them: it also imposes the
// http/https scheme rule, which is nonsense for prose. Nothing here is ever
// `fetched`, so no member of this map reaches the SSRF host guard.
//
// Why this exists: the suggest-edit validator's contract is that a contributor
// cannot land an oversize value in the pending queue, because
// ApprovePendingEdit applies values blindly with an untyped Updates(). Without
// a cap here an over-length value is accepted, then fails at the column with
// Postgres 22001 at APPROVE time, which surfaces as an opaque 500 on a pending
// row no admin can ever clear. Rejecting at submission keeps the failure where
// the user can act on it.
//
// Only fields whose column is genuinely bounded belong here. Unbounded TEXT
// columns (description) are deliberately absent: adding them would be a new
// behavioral limit, not the enforcement of an existing one.
var boundedTextFieldSpecs = map[string]urlFieldSpec{
	"age_policy": {displayName: "Age policy", maxLength: contracts.MaxVenueAgePolicyLength},
}

// The whole-number counterpart of boundedTextFieldSpecs lives in
// contracts.NumericEditFieldBounds, because the approve path needs the same
// registry and cannot import this package. See utils.WholeNumber for what the
// layers underneath do with an unchecked numeric value: they accept it silently.

// ValidateImageURL applies the http/https scheme check AND the SSRF host guard
// to an optional image URL. Empty strings pass through so callers that allow
// "clear via empty string" semantics keep working.
//
// Length is enforced separately by the request struct's maxLength tag at
// JSON decode time; this helper checks the scheme rule and where the host
// actually points.
//
// PSY-1675: image_url is writable by any email-verified user and is fetched
// server-side by the share-card renderer, so the host is resolved here and a
// value that lands on a private/loopback/link-local/metadata address is
// refused. The edge renderer keeps its own literal-host guard as the fetch-time
// backstop (it has no resolver, which is why this layer exists). ctx bounds the
// lookup: a disconnected client cancels it.
func ValidateImageURL(ctx context.Context, imageURL *string) error {
	if imageURL == nil {
		return nil
	}
	if err := validateScheme(*imageURL, urlFieldSpecs["image_url"].displayName); err != nil {
		return err
	}
	return validateFetchHost(ctx, "image_url", *imageURL)
}

// validateFetchHost applies the SSRF host guard to a field marked `fetched`,
// and is a no-op for every other field. Assumes the scheme check already ran.
func validateFetchHost(ctx context.Context, field, value string) error {
	spec, ok := urlFieldSpecs[field]
	if !ok || !spec.fetched {
		return nil
	}
	if err := urlguard.Default.Validate(ctx, value, spec.displayName); err != nil {
		return huma.Error422UnprocessableEntity(err.Error())
	}
	return nil
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
func ValidateURLField(ctx context.Context, fieldName string, value *string) error {
	if value == nil {
		return nil
	}
	if err := URLSchemeError(fieldName, *value); err != nil {
		return huma.Error422UnprocessableEntity(err.Error())
	}
	// Shape rules apply here as well as on the suggest-edit path: a handler that
	// reaches for the obvious shared helper must not get partial validation.
	// Empty is the clear-the-field gesture and each shape rule passes it.
	if err := validateShape(fieldName, *value); err != nil {
		return err
	}
	return validateFetchHost(ctx, fieldName, *value)
}

// validateSocialHost rejects a social-platform URL whose host isn't on that
// platform's allowlist, as a huma 422.
//
// The allowlist and the matching rule live in utils.SocialHostSuffixes /
// utils.ValidateSocialHost, because the rollback gate in internal/services/admin
// needs the same rule and cannot import a handler package. This is the HTTP
// wrapper; the policy is one table.
//
// This is a broad HOST floor for the free-form social fields. The stricter,
// path-aware rules are utils.ValidateBandcampEmbedURL (the `shape` rule on
// bandcamp_embed_url above) and isValidSpotifyURL (catalog/artist.go): change
// platform-host rules with all of them in mind.
func validateSocialHost(field, value string) error {
	if err := utils.ValidateSocialHost(field, urlFieldSpecs[field].displayName, value); err != nil {
		return huma.Error422UnprocessableEntity(err.Error())
	}
	return nil
}

// validateShape applies a field's `shape` rule, and is a no-op for every field
// that has none. Assumes the scheme and length checks already ran.
//
// It is separate from validateSocialHost because a host allowlist is not enough
// for every field. social.bandcamp holds a profile root, so "is it on
// bandcamp.com" is the whole question; bandcamp_embed_url names ONE release, so
// the path carries as much meaning as the host, and a value that clears the host
// floor but not the path renders nothing on either surface that consumes it.
func validateShape(field, value string) error {
	spec, ok := urlFieldSpecs[field]
	if !ok || spec.shape == nil {
		return nil
	}
	if err := spec.shape(value, spec.displayName); err != nil {
		return huma.Error422UnprocessableEntity(err.Error())
	}
	return nil
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
func ValidateFieldChangeValue(ctx context.Context, fieldName string, value any) error {
	// Bounded non-URL text is checked first and returns early: these fields are
	// never `fetched`, so they must not fall through to the URL branch's scheme
	// rule or host guard.
	if spec, ok := boundedTextFieldSpecs[fieldName]; ok {
		return validateBoundedText(spec, value)
	}
	if bounds, ok := contracts.NumericEditFieldBounds()[fieldName]; ok {
		return validateBoundedInt(bounds, value)
	}
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
	if err := validateSocialHost(fieldName, s); err != nil {
		return err
	}
	if err := validateShape(fieldName, s); err != nil {
		return err
	}
	return validateFetchHost(ctx, fieldName, s)
}

// validateBoundedText enforces the type and length contract for a non-URL
// bounded text field arriving through the suggest-edit queue.
//
// The type check is the load-bearing half: FieldChange.NewValue is `any`
// decoded from JSONB, so a caller can put a number, bool, object or array where
// prose belongs. ApprovePendingEdit assigns it straight into an untyped
// Updates() map, so a non-string would fail at the driver during an ADMIN's
// approve request rather than the contributor's submit request.
//
// nil passes: it is the clear-the-field gesture, and the column is nullable.
func validateBoundedText(spec urlFieldSpec, value any) error {
	if value == nil {
		return nil
	}
	s, ok := value.(string)
	if !ok {
		return huma.Error422UnprocessableEntity(
			fmt.Sprintf("%s must be a string", spec.displayName),
		)
	}
	// Runes, not bytes: the backing columns are VARCHAR(n), which counts
	// characters. A byte check would reject a legal multibyte value.
	if utf8.RuneCountInString(s) > spec.maxLength {
		return huma.Error422UnprocessableEntity(
			fmt.Sprintf("%s must be %d characters or fewer", spec.displayName, spec.maxLength),
		)
	}
	return nil
}

// validateBoundedInt enforces the type and range contract for a whole-number
// field arriving through the suggest-edit queue.
//
// Like validateBoundedText, the type check is the load-bearing half:
// FieldChange.NewValue is `any` decoded from JSONB, so a caller can put a
// string, a bool, an object or a fraction where a count belongs, and
// ApprovePendingEdit assigns it straight into an untyped Updates(). The layers
// below do not object to any of that (see utils.WholeNumber for the measured
// behavior), so this is the only place that can tell a bad value from a real
// capacity or year while someone is still around to fix it.
//
// nil passes: it is the clear-the-field gesture, and the column is nullable.
// A numeric STRING ("3600", "1985") is rejected on purpose rather than parsed.
// The column is an integer, so the wire type should be a number; accepting both
// would leave two encodings of the same edit in pending_entity_edits and in
// revisions.field_changes, and the difference would surface later as a
// rendering or comparison bug rather than here as a 422.
//
// admin.NarrowNumericUpdates DOES parse such a string, and that is not a
// contradiction: it reads rows the system stored before founded_year and
// release_year were registered (PSY-1703), whereas this gate decides what is
// allowed to be stored from now on. See its doc comment for the full argument.
func validateBoundedInt(bounds contracts.NumericEditBounds, value any) error {
	if value == nil {
		return nil
	}
	n, ok := utils.WholeNumber(value)
	if !ok {
		// A finite integral float that WholeNumber still refuses is one too
		// large to hold in an int. Telling that caller "must be a whole number"
		// would be false: 1e19 is a whole number, it is just absurd for this
		// field. Every ceiling in the registry is many orders of magnitude
		// below MaxInt, so out-of-int necessarily means out-of-range.
		if f, isFloat := value.(float64); isFloat && !math.IsInf(f, 0) && f == math.Trunc(f) {
			return outOfRangeError(bounds)
		}
		return huma.Error422UnprocessableEntity(
			fmt.Sprintf("%s must be a whole number", bounds.DisplayName),
		)
	}
	if n < bounds.Min || n > bounds.Max {
		return outOfRangeError(bounds)
	}
	return nil
}

func outOfRangeError(bounds contracts.NumericEditBounds) error {
	return huma.Error422UnprocessableEntity(
		fmt.Sprintf("%s must be between %d and %d", bounds.DisplayName, bounds.Min, bounds.Max),
	)
}

// URLSchemeError validates the http/https scheme and per-field length cap for
// a known URL field and returns the underlying error (NOT wrapped as a huma
// 422). It exists for the huma Resolve()-style request bodies that collect
// field-level *huma.ErrorDetail entries with a Location — the caller wraps the
// returned error so the rejection keeps its body-field attribution (PSY-747,
// show ticket_url create path). Returns nil for unknown fields, nil/empty
// values, or valid URLs.
//
// It does NOT apply the SSRF host guard, because huma's Resolve carries a
// huma.Context rather than a context.Context and the guard resolves DNS. No
// field it is used for is marked `fetched`; before marking one, move that
// field's validation off this helper (or thread a real context in), or the
// guard will silently not run on that path.
//
// It does NOT apply `shape` rules either, for a plainer reason: this helper
// returns a bare error for a caller that attaches its own body-field Location,
// while a shape rule's refusal is already a complete sentence. Same hazard, same
// answer, no field it is used for carries one, and marking one means moving
// that field off this helper. TestShapeRuledFieldsAvoidURLSchemeError pins it.
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
