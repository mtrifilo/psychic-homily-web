package shared

import (
	"errors"
	"strings"
	"testing"

	"github.com/danielgtaylor/huma/v2"

	"psychic-homily-backend/internal/api/handlers/shared/testhelpers"
)

// ============================================================================
// ValidateImageURL
// ============================================================================

func TestValidateImageURL_NilPasses(t *testing.T) {
	if err := ValidateImageURL(nil); err != nil {
		t.Errorf("nil should pass, got: %v", err)
	}
}

func TestValidateImageURL_EmptyPasses(t *testing.T) {
	if err := ValidateImageURL(PtrString("")); err != nil {
		t.Errorf("empty string should pass, got: %v", err)
	}
}

func TestValidateImageURL_ValidHTTPS(t *testing.T) {
	if err := ValidateImageURL(PtrString("https://example.com/img.jpg")); err != nil {
		t.Errorf("https URL should pass, got: %v", err)
	}
}

func TestValidateImageURL_RejectsJavaScriptScheme(t *testing.T) {
	err := ValidateImageURL(PtrString("javascript:alert(1)"))
	testhelpers.AssertHumaError(t, err, 422)
}

func TestValidateImageURL_RejectsDataScheme(t *testing.T) {
	err := ValidateImageURL(PtrString("data:image/png;base64,AAAA"))
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
		{"founded_year", 1985},
		{"verified", true},
	}
	for _, c := range cases {
		t.Run(c.field, func(t *testing.T) {
			if err := ValidateFieldChangeValue(c.field, c.value); err != nil {
				t.Errorf("non-URL field %q should pass, got: %v", c.field, err)
			}
		})
	}
}

func TestValidateFieldChangeValue_NilValuePasses(t *testing.T) {
	for _, field := range []string{"image_url", "instagram", "website", "bandcamp"} {
		t.Run(field, func(t *testing.T) {
			if err := ValidateFieldChangeValue(field, nil); err != nil {
				t.Errorf("nil value should pass for %q, got: %v", field, err)
			}
		})
	}
}

func TestValidateFieldChangeValue_EmptyStringPasses(t *testing.T) {
	// Empty string means "clear the field" — should pass through.
	for _, field := range []string{"image_url", "instagram", "website"} {
		t.Run(field, func(t *testing.T) {
			if err := ValidateFieldChangeValue(field, ""); err != nil {
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
			err := ValidateFieldChangeValue("image_url", c.value)
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
			err := ValidateFieldChangeValue("image_url", c.value)
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
			if err := ValidateFieldChangeValue(c.field, c.value); err != nil {
				t.Errorf("valid URL should pass: %v", err)
			}
		})
	}
}

func TestValidateFieldChangeValue_RejectsLengthExceeded(t *testing.T) {
	// instagram cap is 255. Build a 256-char URL.
	base := "https://instagram.com/"
	long := base + strings.Repeat("a", 256-len(base)+1)
	err := ValidateFieldChangeValue("instagram", long)
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
	if err := ValidateFieldChangeValue("instagram", exactly255); err != nil {
		t.Errorf("exactly-at-cap value should pass, got: %v", err)
	}
}

func TestValidateFieldChangeValue_ImageURLLargerCap(t *testing.T) {
	// image_url cap is 2048. A 1500-char URL should pass; 2049 should fail.
	base := "https://example.com/"
	at1500 := base + strings.Repeat("a", 1500-len(base))
	if err := ValidateFieldChangeValue("image_url", at1500); err != nil {
		t.Errorf("1500-char image URL should pass, got: %v", err)
	}
	too2049 := base + strings.Repeat("a", 2049-len(base))
	err := ValidateFieldChangeValue("image_url", too2049)
	testhelpers.AssertHumaError(t, err, 422)
}
