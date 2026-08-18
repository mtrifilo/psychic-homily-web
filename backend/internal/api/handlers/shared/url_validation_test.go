package shared

import (
	"context"
	"errors"
	"math"
	"os"
	"strings"
	"testing"

	"github.com/danielgtaylor/huma/v2"

	"psychic-homily-backend/internal/api/handlers/shared/testhelpers"
	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/utils/urlguard"
)

var bg = context.Background()

// TestMain pins the SSRF host guard (PSY-1675) to a fixed resolution table, so
// image_url is validated against a known address rather than whatever the
// machine's DNS says about example.com — which would make this package
// network-dependent and, worse, pass or fail on a hijacked answer. The guard
// panics if a test reaches the real resolver, so a new test package that
// exercises a guarded handler fails loudly instead of quietly going online.
//
// Package-level state: no test here may call t.Parallel.
//
// Only image_url is `fetched`, so only image_url hosts are ever looked up —
// this table needs an entry for every host used in an image_url that a test
// expects to PASS, and nothing else. A test using a host that is not here gets
// "could not be resolved", which is not an error: unresolvable hosts pass.
func TestMain(m *testing.M) {
	restore := urlguard.Default.UseResolver(urlguard.MapResolver{
		"example.com": {"93.184.216.34"},
	})
	code := m.Run()
	restore()
	os.Exit(code)
}

// ============================================================================
// ValidateImageURL
// ============================================================================

func TestValidateImageURL_NilPasses(t *testing.T) {
	if err := ValidateImageURL(bg, nil); err != nil {
		t.Errorf("nil should pass, got: %v", err)
	}
}

func TestValidateImageURL_EmptyPasses(t *testing.T) {
	if err := ValidateImageURL(bg, PtrString("")); err != nil {
		t.Errorf("empty string should pass, got: %v", err)
	}
}

func TestValidateImageURL_ValidHTTPS(t *testing.T) {
	if err := ValidateImageURL(bg, PtrString("https://example.com/img.jpg")); err != nil {
		t.Errorf("https URL should pass, got: %v", err)
	}
}

func TestValidateImageURL_RejectsJavaScriptScheme(t *testing.T) {
	err := ValidateImageURL(bg, PtrString("javascript:alert(1)"))
	testhelpers.AssertHumaError(t, err, 422)
}

func TestValidateImageURL_RejectsDataScheme(t *testing.T) {
	err := ValidateImageURL(bg, PtrString("data:image/png;base64,AAAA"))
	testhelpers.AssertHumaError(t, err, 422)
}

// ============================================================================
// ValidateSocialURLs
// ============================================================================

func TestValidateSocialURLs_AllNilPasses(t *testing.T) {
	if err := ValidateSocialURLs(nil, nil, nil, nil, nil, nil, nil, nil); err != nil {
		t.Errorf("all nil should pass, got: %v", err)
	}
}

func TestValidateSocialURLs_AllValidHTTPSPasses(t *testing.T) {
	err := ValidateSocialURLs(
		PtrString("https://instagram.com/x"),
		PtrString("https://facebook.com/x"),
		PtrString("https://twitter.com/x"),
		PtrString("https://youtube.com/x"),
		PtrString("https://spotify.com/x"),
		PtrString("https://soundcloud.com/x"),
		PtrString("https://x.bandcamp.com"),
		PtrString("https://example.com"),
	)
	if err != nil {
		t.Errorf("all valid should pass, got: %v", err)
	}
}

func TestValidateSocialURLs_FirstFailureWins(t *testing.T) {
	// Two bad fields — the first one in the iteration order (instagram)
	// determines the error.
	err := ValidateSocialURLs(
		PtrString("javascript:bad"),
		nil, nil, nil, nil, nil, nil,
		PtrString("ftp://also-bad"),
	)
	testhelpers.AssertHumaError(t, err, 422)
	var he *huma.ErrorModel
	errors.As(err, &he)
	if !strings.Contains(he.Detail, "Instagram") {
		t.Errorf("expected error to mention Instagram (first failing field), got: %s", he.Detail)
	}
}

func TestValidateSocialURLs_PartialNilSkipsThoseFields(t *testing.T) {
	// Only Website is provided, others nil → only Website is validated.
	err := ValidateSocialURLs(nil, nil, nil, nil, nil, nil, nil, PtrString("https://example.com"))
	if err != nil {
		t.Errorf("partial nil with valid website should pass, got: %v", err)
	}
}

func TestValidateSocialURLs_RejectsBareHandle(t *testing.T) {
	// "@matt" is not a valid URL (no scheme, parses as a relative ref).
	err := ValidateSocialURLs(PtrString("@matt"), nil, nil, nil, nil, nil, nil, nil)
	testhelpers.AssertHumaError(t, err, 422)
}

func TestValidateSocialURLs_AcceptsPlatformSubdomainVariants(t *testing.T) {
	// PSY-1113: the host allowlist matches subdomains + known alternates.
	err := ValidateSocialURLs(
		PtrString("https://www.instagram.com/x"),
		PtrString("https://m.facebook.com/x"),
		PtrString("https://x.com/x"), // twitter ↔ x.com
		PtrString("https://music.youtube.com/x"),
		PtrString("https://open.spotify.com/artist/0WThQFCFaU1YR5s0bNLvtP"),
		PtrString("https://soundcloud.com/x"),
		PtrString("https://artist.bandcamp.com/album/y"),
		PtrString("https://some-random-domain.io/page"), // website: any host
	)
	if err != nil {
		t.Errorf("legit platform variants should pass, got: %v", err)
	}
}

func TestValidateSocialURLs_RejectsForeignHostInPlatformField(t *testing.T) {
	cases := []struct {
		name  string
		run   func() error
		field string
	}{
		{"spotify→foreign host", func() error {
			return ValidateSocialURLs(nil, nil, nil, nil, PtrString("https://evil.test/artist/x"), nil, nil, nil)
		}, "Spotify"},
		{"bandcamp→internal IP", func() error {
			return ValidateSocialURLs(nil, nil, nil, nil, nil, nil, PtrString("https://169.254.169.254/album/x"), nil)
		}, "Bandcamp"},
		{"instagram→lookalike suffix", func() error {
			return ValidateSocialURLs(PtrString("https://instagram.com.evil.test/x"), nil, nil, nil, nil, nil, nil, nil)
		}, "Instagram"},
		{"twitter→foreign host", func() error {
			return ValidateSocialURLs(nil, nil, PtrString("https://evil.test/x"), nil, nil, nil, nil, nil)
		}, "Twitter"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := c.run()
			testhelpers.AssertHumaError(t, err, 422)
			var he *huma.ErrorModel
			errors.As(err, &he)
			if !strings.Contains(he.Detail, c.field) {
				t.Errorf("expected error to mention %s, got: %s", c.field, he.Detail)
			}
		})
	}
}

func TestValidateSocialURLs_WebsiteAcceptsAnyHost(t *testing.T) {
	if err := ValidateSocialURLs(nil, nil, nil, nil, nil, nil, nil, PtrString("https://anything.example.org/page")); err != nil {
		t.Errorf("website should accept any host, got: %v", err)
	}
}

func TestValidateFieldChangeValue_HostAnchorsSocialFields(t *testing.T) {
	// The suggest-edit path host-anchors platform fields too (PSY-1113).
	if err := ValidateFieldChangeValue(bg, "spotify", "https://open.spotify.com/artist/0WThQFCFaU1YR5s0bNLvtP"); err != nil {
		t.Errorf("canonical spotify URL should pass, got: %v", err)
	}
	err := ValidateFieldChangeValue(bg, "spotify", "https://evil.test/artist/x")
	testhelpers.AssertHumaError(t, err, 422)
	if err := ValidateFieldChangeValue(bg, "website", "https://anything.example.org"); err != nil {
		t.Errorf("website should accept any host, got: %v", err)
	}
}

// ============================================================================
// Bounded non-URL text fields on the suggest-edit path
// ============================================================================

// These cases guard the ONLY server-side check standing between a contributor
// and a length-bounded venue column. Without it an over-length or non-string
// value is accepted into pending_entity_edits and then fails at the column with
// Postgres 22001 during a later ADMIN's approve request, which surfaces as an
// opaque 500 on a pending row nobody can clear.
func TestValidateFieldChangeValue_AgePolicyLengthAndType(t *testing.T) {
	if err := ValidateFieldChangeValue(bg, "age_policy", "all ages"); err != nil {
		t.Errorf("a normal age policy should pass, got: %v", err)
	}

	// nil is the clear-the-field gesture and the column is nullable.
	if err := ValidateFieldChangeValue(bg, "age_policy", nil); err != nil {
		t.Errorf("nil should pass (clear gesture), got: %v", err)
	}
	if err := ValidateFieldChangeValue(bg, "age_policy", ""); err != nil {
		t.Errorf("empty string should pass (clear gesture), got: %v", err)
	}

	// Exactly at the bound passes; one past it is rejected at submit time.
	testhelpers.AssertHumaError(t, ValidateFieldChangeValue(bg, "age_policy", strings.Repeat("a", 101)), 422)
	if err := ValidateFieldChangeValue(bg, "age_policy", strings.Repeat("a", 100)); err != nil {
		t.Errorf("exactly 100 characters should pass, got: %v", err)
	}

	// Runes, not bytes. 100 three-byte characters is 300 bytes but fits a
	// VARCHAR(100); a byte-based check would wrongly reject it here while the
	// create body's huma maxLength tag (which counts runes) accepted it.
	if err := ValidateFieldChangeValue(bg, "age_policy", strings.Repeat("あ", 100)); err != nil {
		t.Errorf("100 multibyte characters should pass, got: %v", err)
	}
	testhelpers.AssertHumaError(t, ValidateFieldChangeValue(bg, "age_policy", strings.Repeat("あ", 101)), 422)

	// NewValue is `any` decoded from JSONB, so non-strings are reachable and
	// would otherwise reach an untyped GORM Updates() map.
	for _, bad := range []any{42, true, map[string]any{"x": 1}, []any{"a"}} {
		testhelpers.AssertHumaError(t, ValidateFieldChangeValue(bg, "age_policy", bad), 422)
	}
}

// ============================================================================
// Bounded whole-number fields on the suggest-edit path
// ============================================================================

// capacity is the only field the edit drawer submits as a JSON number, so it is
// the only suggest-edit value that is not a string. These cases guard the sole
// server-side check between a contributor and an integer column that
// ApprovePendingEdit writes through an untyped Updates().
//
// The gate matters because the layers below it do NOT complain: an unnarrowed
// numeric string or fraction is accepted and coerced with no error at any layer
// (measurements in the utils.WholeNumber doc comment). The wrong value lands
// silently rather than failing loudly, so only these assertions stand between a
// contributor and a capacity nobody typed.
func TestValidateFieldChangeValue_CapacityTypeAndRange(t *testing.T) {
	// The wire shape: encoding/json decodes every JSON number into an
	// interface{} as float64, so that is what actually arrives here.
	if err := ValidateFieldChangeValue(bg, "capacity", float64(550)); err != nil {
		t.Errorf("a normal capacity should pass, got: %v", err)
	}
	// A plain int is reachable from internal callers and tests.
	if err := ValidateFieldChangeValue(bg, "capacity", 550); err != nil {
		t.Errorf("an int capacity should pass, got: %v", err)
	}

	// nil is the clear-the-field gesture and the column is nullable.
	if err := ValidateFieldChangeValue(bg, "capacity", nil); err != nil {
		t.Errorf("nil should pass (clear gesture), got: %v", err)
	}

	// Both bounds are inclusive; one step outside either is rejected at submit.
	for _, ok := range []any{
		float64(contracts.MinVenueCapacity),
		float64(contracts.MaxVenueCapacity),
	} {
		if err := ValidateFieldChangeValue(bg, "capacity", ok); err != nil {
			t.Errorf("capacity %v is on the bound and should pass, got: %v", ok, err)
		}
	}
	for _, bad := range []any{
		float64(0),                              // zero: NULL already means "unknown"
		float64(-1),                             // negative
		float64(contracts.MaxVenueCapacity + 1), // one past the ceiling
		float64(1e18),                           // absurd but finite
		math.Inf(1),                             // only reachable from a hand-built value
		math.NaN(),                              // ditto
	} {
		testhelpers.AssertHumaError(t, ValidateFieldChangeValue(bg, "capacity", bad), 422)
	}

	// A fraction is rejected rather than silently coerced: unnarrowed, 550.7
	// reaches the column as 550 (measured; see the utils.WholeNumber doc).
	testhelpers.AssertHumaError(t, ValidateFieldChangeValue(bg, "capacity", 550.7), 422)

	// Wrong types, including a numeric STRING. The column is an integer, so the
	// wire type has to be a number; parsing "550" here would leave two
	// encodings of one edit in pending_entity_edits and revisions.field_changes.
	for _, bad := range []any{"550", "", "many", true, map[string]any{"x": 1}, []any{1}} {
		testhelpers.AssertHumaError(t, ValidateFieldChangeValue(bg, "capacity", bad), 422)
	}
}

// PSY-1703: labels.founded_year and releases.release_year are integer columns
// that rode through this queue as TEXT until now, so the corpus below mirrors
// the capacity one case for case. Same exposure, same gate, same assertions;
// the only thing that differs is the range.
//
// Table-driven over both fields deliberately: they share one floor and one
// ceiling, and a test that only exercised founded_year would let release_year
// silently fall out of the registry.
func TestValidateFieldChangeValue_CatalogYearTypeAndRange(t *testing.T) {
	// Resolved once here, the same way the validator resolves it per call. A
	// literal would make this test start failing on 1 January.
	maxYear := contracts.MaxCatalogYear()

	for _, field := range []string{"founded_year", "release_year"} {
		t.Run(field, func(t *testing.T) {
			// The wire shape: encoding/json decodes every JSON number into an
			// interface{} as float64, so that is what actually arrives here.
			if err := ValidateFieldChangeValue(bg, field, float64(1985)); err != nil {
				t.Errorf("a normal year should pass, got: %v", err)
			}
			// A plain int is reachable from internal callers and tests.
			if err := ValidateFieldChangeValue(bg, field, 1985); err != nil {
				t.Errorf("an int year should pass, got: %v", err)
			}

			// nil is the clear-the-field gesture and the column is nullable.
			if err := ValidateFieldChangeValue(bg, field, nil); err != nil {
				t.Errorf("nil should pass (clear gesture), got: %v", err)
			}

			// Both bounds are inclusive. The ceiling is next year, so a release
			// announced ahead of its pressing is submittable.
			for _, ok := range []any{
				float64(contracts.MinCatalogYear),
				float64(maxYear),
				float64(maxYear - 1), // the current year
			} {
				if err := ValidateFieldChangeValue(bg, field, ok); err != nil {
					t.Errorf("year %v is in range and should pass, got: %v", ok, err)
				}
			}
			for _, bad := range []any{
				float64(0),                            // zero: NULL already means "unknown"
				float64(-1),                           // negative
				float64(999),                          // one below the floor
				float64(contracts.MinCatalogYear - 1), // same, spelled from the constant
				float64(maxYear + 1),                  // one past the ceiling
				float64(19850),                        // the trailing-digit typo this exists for
				float64(1e18),                         // absurd but finite
				math.Inf(1),                           // only reachable from a hand-built value
				math.NaN(),                            // ditto
			} {
				testhelpers.AssertHumaError(t, ValidateFieldChangeValue(bg, field, bad), 422)
			}

			// A fraction is rejected rather than silently coerced: unnarrowed,
			// 1985.7 reaches the column as 1985 (measured; see utils.WholeNumber).
			testhelpers.AssertHumaError(t, ValidateFieldChangeValue(bg, field, 1985.7), 422)

			// Wrong types, including the numeric STRING this drawer field used to
			// submit. It is refused so nothing NEW is stored in the old encoding;
			// admin.NarrowNumericUpdates still parses the rows that already are.
			for _, bad := range []any{"1985", "", "1985 approx", true, map[string]any{"x": 1}, []any{1}} {
				testhelpers.AssertHumaError(t, ValidateFieldChangeValue(bg, field, bad), 422)
			}
		})
	}
}

// ============================================================================
// ValidateFieldChangeValue (PSY-549)
// ============================================================================

func TestValidateFieldChangeValue_UnknownFieldPasses(t *testing.T) {
	// Non-URL fields (name, city, description, etc.) pass through —
	// the helper has no opinion on them.
	cases := []struct {
		field string
		value any
	}{
		{"name", "Some Artist"},
		{"city", "Phoenix"},
		{"description", "Long markdown text here"},
		// founded_year used to sit here as an example of a field this helper
		// has no opinion on. It has one since PSY-1703, so its own corpus
		// covers it now; release_date is the year fields' unregistered
		// neighbour and takes its place.
		{"release_date", "1991-09-24"},
		{"verified", true},
	}
	for _, c := range cases {
		t.Run(c.field, func(t *testing.T) {
			if err := ValidateFieldChangeValue(bg, c.field, c.value); err != nil {
				t.Errorf("non-URL field %q should pass, got: %v", c.field, err)
			}
		})
	}
}

func TestValidateFieldChangeValue_NilValuePasses(t *testing.T) {
	for _, field := range []string{"image_url", "instagram", "website", "bandcamp"} {
		t.Run(field, func(t *testing.T) {
			if err := ValidateFieldChangeValue(bg, field, nil); err != nil {
				t.Errorf("nil value should pass for %q, got: %v", field, err)
			}
		})
	}
}

func TestValidateFieldChangeValue_EmptyStringPasses(t *testing.T) {
	// Empty string means "clear the field" — should pass through.
	for _, field := range []string{"image_url", "instagram", "website"} {
		t.Run(field, func(t *testing.T) {
			if err := ValidateFieldChangeValue(bg, field, ""); err != nil {
				t.Errorf("empty string should pass for %q, got: %v", field, err)
			}
		})
	}
}

func TestValidateFieldChangeValue_NonStringValueRejected(t *testing.T) {
	// URL fields must be strings. Non-string values from JSON (number, bool,
	// object, array) fail with 422.
	cases := []struct {
		name  string
		value any
	}{
		{"number", 42},
		{"bool", true},
		{"object", map[string]string{"href": "https://example.com"}},
		{"slice", []string{"https://example.com"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := ValidateFieldChangeValue(bg, "image_url", c.value)
			testhelpers.AssertHumaError(t, err, 422)
		})
	}
}

func TestValidateFieldChangeValue_RejectsNonHTTPSchemes(t *testing.T) {
	cases := []struct {
		name  string
		value string
	}{
		{"javascript", "javascript:alert(1)"},
		{"data", "data:image/png;base64,AAAA"},
		{"ftp", "ftp://example.com/file.zip"},
		{"file", "file:///etc/passwd"},
		{"mailto", "mailto:matt@example.com"},
		// scheme-relative URL (no scheme) — parses, but Scheme=="" so rejected
		{"scheme-relative", "//example.com/img.jpg"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := ValidateFieldChangeValue(bg, "image_url", c.value)
			testhelpers.AssertHumaError(t, err, 422)
		})
	}
}

func TestValidateFieldChangeValue_AcceptsValidURLs(t *testing.T) {
	cases := []struct {
		field string
		value string
	}{
		{"image_url", "https://example.com/img.jpg"},
		{"image_url", "http://example.com/img.jpg"},
		{"instagram", "https://instagram.com/someone"},
		{"website", "https://example.com"},
		{"bandcamp", "https://artist.bandcamp.com"},
	}
	for _, c := range cases {
		t.Run(c.field+"="+c.value, func(t *testing.T) {
			if err := ValidateFieldChangeValue(bg, c.field, c.value); err != nil {
				t.Errorf("valid URL should pass: %v", err)
			}
		})
	}
}

func TestValidateFieldChangeValue_RejectsLengthExceeded(t *testing.T) {
	// instagram cap is 255. Build a 256-char URL.
	base := "https://instagram.com/"
	long := base + strings.Repeat("a", 256-len(base)+1)
	err := ValidateFieldChangeValue(bg, "instagram", long)
	testhelpers.AssertHumaError(t, err, 422)
	var he *huma.ErrorModel
	errors.As(err, &he)
	if !strings.Contains(he.Detail, "255") {
		t.Errorf("expected error to mention 255 char cap, got: %s", he.Detail)
	}
}

func TestValidateFieldChangeValue_AcceptsAtCap(t *testing.T) {
	// instagram cap is 255 — exactly 255 chars should pass.
	base := "https://instagram.com/"
	exactly255 := base + strings.Repeat("a", 255-len(base))
	if len(exactly255) != 255 {
		t.Fatalf("test setup: expected 255 chars, got %d", len(exactly255))
	}
	if err := ValidateFieldChangeValue(bg, "instagram", exactly255); err != nil {
		t.Errorf("exactly-at-cap value should pass, got: %v", err)
	}
}

func TestValidateFieldChangeValue_ImageURLLargerCap(t *testing.T) {
	// image_url cap is 2048. A 1500-char URL should pass; 2049 should fail.
	base := "https://example.com/"
	at1500 := base + strings.Repeat("a", 1500-len(base))
	if err := ValidateFieldChangeValue(bg, "image_url", at1500); err != nil {
		t.Errorf("1500-char image URL should pass, got: %v", err)
	}
	too2049 := base + strings.Repeat("a", 2049-len(base))
	err := ValidateFieldChangeValue(bg, "image_url", too2049)
	testhelpers.AssertHumaError(t, err, 422)
}

// ============================================================================
// ValidateURLField — PSY-747: collection cover_image_url, release
// cover_art_url, show ticket_url (typed *string boundary).
// ============================================================================

func TestValidateURLField_NilAndEmptyPass(t *testing.T) {
	for _, field := range []string{"cover_image_url", "cover_art_url", "ticket_url"} {
		t.Run(field, func(t *testing.T) {
			if err := ValidateURLField(bg, field, nil); err != nil {
				t.Errorf("nil should pass, got: %v", err)
			}
			if err := ValidateURLField(bg, field, PtrString("")); err != nil {
				t.Errorf("empty string should pass, got: %v", err)
			}
		})
	}
}

func TestValidateURLField_UnknownFieldPasses(t *testing.T) {
	// A field name not in urlFieldSpecs degrades to no-op rather than crashing.
	if err := ValidateURLField(bg, "not_a_url_field", PtrString("javascript:alert(1)")); err != nil {
		t.Errorf("unknown field should pass, got: %v", err)
	}
}

func TestValidateURLField_AcceptsValidHTTPAndHTTPS(t *testing.T) {
	cases := []struct{ field, value string }{
		{"cover_image_url", "https://example.com/cover.jpg"},
		{"cover_image_url", "http://example.com/cover.jpg"},
		{"cover_art_url", "https://example.com/art.png"},
		{"ticket_url", "https://tickets.example.com/event/1"},
	}
	for _, c := range cases {
		t.Run(c.field+"="+c.value, func(t *testing.T) {
			if err := ValidateURLField(bg, c.field, PtrString(c.value)); err != nil {
				t.Errorf("valid URL should pass, got: %v", err)
			}
		})
	}
}

func TestValidateURLField_RejectsJavaScriptScheme(t *testing.T) {
	for _, field := range []string{"cover_image_url", "cover_art_url", "ticket_url"} {
		t.Run(field, func(t *testing.T) {
			err := ValidateURLField(bg, field, PtrString("javascript:alert(1)"))
			testhelpers.AssertHumaError(t, err, 422)
		})
	}
}

func TestValidateURLField_RejectsDataScheme(t *testing.T) {
	for _, field := range []string{"cover_image_url", "cover_art_url", "ticket_url"} {
		t.Run(field, func(t *testing.T) {
			err := ValidateURLField(bg, field, PtrString("data:text/html,<script>alert(1)</script>"))
			testhelpers.AssertHumaError(t, err, 422)
		})
	}
}

func TestValidateURLField_RejectsLengthExceeded(t *testing.T) {
	// ticket_url cap is 500; build a 501-char URL.
	base := "https://tickets.example.com/"
	long := base + strings.Repeat("a", 501-len(base)+1)
	err := ValidateURLField(bg, "ticket_url", PtrString(long))
	testhelpers.AssertHumaError(t, err, 422)
	var he *huma.ErrorModel
	errors.As(err, &he)
	if !strings.Contains(he.Detail, "500") {
		t.Errorf("expected error to mention 500 char cap, got: %s", he.Detail)
	}
}

// ============================================================================
// URLSchemeError — PSY-747: the huma Resolve()-style path (show ticket_url
// create), which collects field-level *huma.ErrorDetail with a Location.
// ============================================================================

func TestURLSchemeError_ValidAndEmptyReturnNil(t *testing.T) {
	if err := URLSchemeError("ticket_url", ""); err != nil {
		t.Errorf("empty should return nil, got: %v", err)
	}
	if err := URLSchemeError("ticket_url", "https://tickets.example.com/1"); err != nil {
		t.Errorf("valid URL should return nil, got: %v", err)
	}
	if err := URLSchemeError("not_a_url_field", "javascript:alert(1)"); err != nil {
		t.Errorf("unknown field should return nil, got: %v", err)
	}
}

func TestURLSchemeError_RejectsNonHTTPScheme(t *testing.T) {
	// Returns a plain (unwrapped) error so the Resolve() caller can wrap it as
	// an *huma.ErrorDetail with a Location.
	err := URLSchemeError("ticket_url", "javascript:alert(1)")
	if err == nil {
		t.Fatal("expected error for javascript: scheme, got nil")
	}
	var isHuma huma.StatusError
	if errors.As(err, &isHuma) {
		t.Errorf("expected a plain error (not huma StatusError), got: %T", err)
	}
}

func TestURLSchemeError_RejectsLengthExceeded(t *testing.T) {
	base := "https://tickets.example.com/"
	long := base + strings.Repeat("a", 501-len(base)+1)
	err := URLSchemeError("ticket_url", long)
	if err == nil || !strings.Contains(err.Error(), "500") {
		t.Errorf("expected 500-char cap error, got: %v", err)
	}
}

// ============================================================================
// SSRF host guard — PSY-1675. image_url is fetched server-side by the share-card
// renderer, so these two entry points must resolve the host, not just check the
// scheme. The exhaustive address corpus lives in internal/utils/urlguard; what
// is asserted HERE is that each entry point actually reaches it, and returns a
// 422 rather than a bare error.
// ============================================================================

// ssrfBypassCorpus is a representative slice of the forms PSY-1672's edge guard
// was hardened against, plus the hostname case only a resolver can catch.
var ssrfBypassCorpus = []struct{ name, value string }{
	{"cloud metadata literal", "https://169.254.169.254/latest/meta-data/"},
	{"ipv6-mapped metadata", "https://[::ffff:169.254.169.254]/x.jpg"},
	{"ipv6-mapped metadata, hex groups", "https://[::ffff:a9fe:a9fe]/x.jpg"},
	{"decimal loopback", "https://2130706433/x.jpg"},
	{"octal loopback", "https://0177.0.0.1/x.jpg"},
	{"hex loopback", "https://0x7f000001/x.jpg"},
	{"short-form loopback", "https://127.1/x.jpg"},
	{"userinfo hiding the host", "https://example.com@169.254.169.254/x.jpg"},
	{"localhost with a trailing dot", "https://localhost./x.jpg"},
	{"rfc1918", "https://10.0.0.5/x.jpg"},
	{"name resolving to cloud metadata", "https://rebind.example.test/x.jpg"},
}

// withHostileDNS points rebind.example.test at the metadata address for the
// duration of one test, leaving the package-wide table otherwise intact.
func withHostileDNS(t *testing.T) {
	t.Helper()
	t.Cleanup(urlguard.Default.UseResolver(urlguard.MapResolver{
		"example.com":         {"93.184.216.34"},
		"rebind.example.test": {"169.254.169.254"},
	}))
}

func TestValidateImageURL_RejectsSSRFTargets(t *testing.T) {
	withHostileDNS(t)
	for _, c := range ssrfBypassCorpus {
		t.Run(c.name, func(t *testing.T) {
			err := ValidateImageURL(bg, PtrString(c.value))
			testhelpers.AssertHumaError(t, err, 422)
		})
	}
	if err := ValidateImageURL(bg, PtrString("https://example.com/flyer.jpg")); err != nil {
		t.Errorf("a public image host must still pass, got: %v", err)
	}
}

func TestValidateFieldChangeValue_RejectsSSRFTargetsOnImageURL(t *testing.T) {
	withHostileDNS(t)
	for _, c := range ssrfBypassCorpus {
		t.Run(c.name, func(t *testing.T) {
			err := ValidateFieldChangeValue(bg, "image_url", c.value)
			testhelpers.AssertHumaError(t, err, 422)
		})
	}
	if err := ValidateFieldChangeValue(bg, "image_url", "https://example.com/flyer.jpg"); err != nil {
		t.Errorf("a public image host must still pass, got: %v", err)
	}
}

// urlSchemeErrorFields is every field name passed to URLSchemeError anywhere in
// the codebase. URLSchemeError cannot run the host guard (huma's Resolve gives
// it a huma.Context, not a context.Context), so a field validated through it is
// a field the `fetched` flag silently does not reach.
//
// Keep this in sync with the call sites: today the only one is
// catalog/show.go's CreateShowRequestBody.Resolve, with "ticket_url".
var urlSchemeErrorFields = []string{"ticket_url"}

// TestFetchedFieldsAvoidURLSchemeError is the tripwire the `fetched` comment
// points at. Marking a field fetched guards ValidateImageURL / ValidateURLField
// / ValidateFieldChangeValue but NOT URLSchemeError, so flipping the flag on
// ticket_url would guard PUT /shows/{id} and leave POST /shows unguarded, with
// nothing failing to say so. This fails instead.
//
// If it fires: thread a real context into that field's create path rather than
// deleting the case.
func TestFetchedFieldsAvoidURLSchemeError(t *testing.T) {
	for _, field := range urlSchemeErrorFields {
		spec, ok := urlFieldSpecs[field]
		if !ok {
			t.Errorf("%q is passed to URLSchemeError but is not a known URL field", field)
			continue
		}
		if spec.fetched {
			t.Errorf("%q is marked fetched but is validated via URLSchemeError, which cannot run "+
				"the host guard — its create path would silently skip SSRF validation", field)
		}
	}
}

// TestFetchHostGuard_OnlyAppliesToFetchedFields is the guard on the guard: the
// host check costs a DNS lookup on the write path, so it must run for the field
// that is fetched and stay off the ones that are only ever rendered as an href
// or an <img>. If this fails, an unrelated write path just grew a resolver
// dependency.
func TestFetchHostGuard_OnlyAppliesToFetchedFields(t *testing.T) {
	withHostileDNS(t)
	unfetched := []struct{ field, value string }{
		{"cover_image_url", "https://nowhere.example.test/cover.jpg"},
		{"cover_art_url", "https://nowhere.example.test/art.png"},
		{"ticket_url", "https://nowhere.example.test/event/1"},
	}
	for _, c := range unfetched {
		t.Run(c.field, func(t *testing.T) {
			if err := ValidateURLField(bg, c.field, PtrString(c.value)); err != nil {
				t.Errorf("%s is not fetched server-side and must not be resolved, got: %v", c.field, err)
			}
			if err := ValidateFieldChangeValue(bg, c.field, c.value); err != nil {
				t.Errorf("%s via field-change must not be resolved, got: %v", c.field, err)
			}
		})
	}
	// The social fields keep their host anchor and gain no resolver either.
	if err := ValidateFieldChangeValue(bg, "website", "https://nowhere.example.test/page"); err != nil {
		t.Errorf("website must not be resolved, got: %v", err)
	}
}
