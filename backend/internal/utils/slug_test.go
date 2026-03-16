package utils

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// --- GenerateSlug ---

func TestGenerateSlug_BasicText(t *testing.T) {
	assert.Equal(t, "hello-world", GenerateSlug("Hello World"))
}

func TestGenerateSlug_MultipleParts(t *testing.T) {
	assert.Equal(t, "foo-bar-baz", GenerateSlug("Foo", "Bar", "Baz"))
}

func TestGenerateSlug_SpecialCharacters(t *testing.T) {
	assert.Equal(t, "the-nationals-new-album", GenerateSlug("The National's New Album!"))
}

func TestGenerateSlug_MultipleSpacesAndHyphens(t *testing.T) {
	assert.Equal(t, "hello-world", GenerateSlug("Hello   ---   World"))
}

func TestGenerateSlug_LeadingTrailingHyphens(t *testing.T) {
	assert.Equal(t, "hello", GenerateSlug("---hello---"))
}

func TestGenerateSlug_EmptyString(t *testing.T) {
	assert.Equal(t, "", GenerateSlug(""))
}

func TestGenerateSlug_OnlySpecialChars(t *testing.T) {
	assert.Equal(t, "", GenerateSlug("!@#$%^&*()"))
}

func TestGenerateSlug_Numbers(t *testing.T) {
	assert.Equal(t, "track-42", GenerateSlug("Track 42"))
}

func TestGenerateSlug_Ampersand(t *testing.T) {
	assert.Equal(t, "rock-roll", GenerateSlug("Rock & Roll"))
}

func TestGenerateSlug_Unicode(t *testing.T) {
	// Non-ASCII characters get stripped by the regex
	assert.Equal(t, "caf", GenerateSlug("Café"))
}

func TestGenerateSlug_MixedCase(t *testing.T) {
	assert.Equal(t, "all-lowercase", GenerateSlug("ALL LOWERCASE"))
}

func TestGenerateSlug_AlreadySlugified(t *testing.T) {
	assert.Equal(t, "already-a-slug", GenerateSlug("already-a-slug"))
}

// --- GenerateArtistSlug ---

func TestGenerateArtistSlug(t *testing.T) {
	tests := []struct {
		name     string
		expected string
	}{
		{"The National", "the-national"},
		{"Turnstile", "turnstile"},
		{"100 gecs", "100-gecs"},
		{"MF DOOM", "mf-doom"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expected, GenerateArtistSlug(tc.name))
		})
	}
}

// --- GenerateVenueSlug ---

func TestGenerateVenueSlug(t *testing.T) {
	tests := []struct {
		name, city, state, expected string
	}{
		{"Valley Bar", "Phoenix", "AZ", "valley-bar-phoenix-az"},
		{"Crescent Ballroom", "Phoenix", "AZ", "crescent-ballroom-phoenix-az"},
		{"The Rebel Lounge", "Phoenix", "AZ", "the-rebel-lounge-phoenix-az"},
	}
	for _, tc := range tests {
		t.Run(tc.expected, func(t *testing.T) {
			assert.Equal(t, tc.expected, GenerateVenueSlug(tc.name, tc.city, tc.state))
		})
	}
}

// --- GenerateShowSlug ---

func TestGenerateShowSlug(t *testing.T) {
	// 8pm Phoenix time = 3am UTC next day (UTC-7)
	date := time.Date(2026, 1, 31, 3, 0, 0, 0, time.UTC)
	slug := GenerateShowSlug(date, "The National", "Valley Bar", "AZ")
	// Should use Phoenix local date (Jan 30), not UTC date (Jan 31)
	assert.Equal(t, "2026-01-30-the-national-at-valley-bar", slug)
}

func TestGenerateShowSlug_DateFormatting(t *testing.T) {
	// Verify single-digit month/day get zero-padded
	date := time.Date(2026, 3, 5, 0, 0, 0, 0, time.UTC)
	slug := GenerateShowSlug(date, "Turnstile", "Crescent Ballroom", "AZ")
	// 3/5 00:00 UTC = 3/4 5pm Phoenix, so local date is March 4
	assert.Equal(t, "2026-03-04-turnstile-at-crescent-ballroom", slug)
}

func TestGenerateShowSlug_TimezoneConversion(t *testing.T) {
	// A show at 7:30 PM Phoenix time on March 16 is stored as 2:30 AM UTC on March 17
	date := time.Date(2026, 3, 17, 2, 30, 0, 0, time.UTC)
	slug := GenerateShowSlug(date, "Radiohead", "Valley Bar", "AZ")
	// Slug should use the Phoenix local date (March 16), not UTC (March 17)
	assert.Equal(t, "2026-03-16-radiohead-at-valley-bar", slug)
}

func TestGenerateShowSlug_EasternTimezone(t *testing.T) {
	// 11pm ET on Jan 15 = 4am UTC Jan 16 (UTC-5)
	date := time.Date(2026, 1, 16, 4, 0, 0, 0, time.UTC)
	slug := GenerateShowSlug(date, "The National", "Brooklyn Steel", "NY")
	assert.Equal(t, "2026-01-15-the-national-at-brooklyn-steel", slug)
}

func TestGenerateShowSlug_EmptyState(t *testing.T) {
	// Empty state defaults to America/Phoenix
	date := time.Date(2026, 3, 17, 2, 30, 0, 0, time.UTC)
	slug := GenerateShowSlug(date, "Radiohead", "Valley Bar", "")
	assert.Equal(t, "2026-03-16-radiohead-at-valley-bar", slug)
}

func TestGenerateShowSlug_SameDateNoShift(t *testing.T) {
	// A show at 2pm Phoenix time on March 16 is stored as 9pm UTC on March 16
	// No date shift expected
	date := time.Date(2026, 3, 16, 21, 0, 0, 0, time.UTC)
	slug := GenerateShowSlug(date, "Radiohead", "Valley Bar", "AZ")
	assert.Equal(t, "2026-03-16-radiohead-at-valley-bar", slug)
}

// --- GenerateUniqueSlug ---

func TestGenerateUniqueSlug_NoCollision(t *testing.T) {
	slug := GenerateUniqueSlug("the-national", func(slug string) bool {
		return false // nothing exists
	})
	assert.Equal(t, "the-national", slug)
}

func TestGenerateUniqueSlug_FirstCollision(t *testing.T) {
	existing := map[string]bool{"the-national": true}
	slug := GenerateUniqueSlug("the-national", func(slug string) bool {
		return existing[slug]
	})
	assert.Equal(t, "the-national-2", slug)
}

func TestGenerateUniqueSlug_MultipleCollisions(t *testing.T) {
	existing := map[string]bool{
		"the-national":   true,
		"the-national-2": true,
		"the-national-3": true,
	}
	slug := GenerateUniqueSlug("the-national", func(slug string) bool {
		return existing[slug]
	})
	assert.Equal(t, "the-national-4", slug)
}

func TestGenerateUniqueSlug_Fallback(t *testing.T) {
	// All 2..100 are taken — should fall back to timestamp suffix
	slug := GenerateUniqueSlug("taken", func(slug string) bool {
		return true // everything exists
	})
	// Should start with "taken-" and have a numeric (timestamp) suffix
	assert.Contains(t, slug, "taken-")
	assert.NotEqual(t, "taken", slug)
	// The suffix should be a large number (unix nano timestamp)
	var ts int64
	_, err := fmt.Sscanf(slug, "taken-%d", &ts)
	assert.NoError(t, err)
	assert.Greater(t, ts, int64(1000000000), "expected unix nano timestamp")
}
