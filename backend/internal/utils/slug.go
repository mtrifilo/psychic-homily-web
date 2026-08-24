package utils

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

var (
	// Match any character that's not alphanumeric or hyphen
	nonSlugChars = regexp.MustCompile(`[^a-z0-9-]+`)
	// Match multiple consecutive hyphens
	multipleHyphens = regexp.MustCompile(`-+`)
)

// GenerateSlug creates a URL-safe slug from input text parts
func GenerateSlug(parts ...string) string {
	combined := strings.Join(parts, "-")

	// Convert to lowercase
	slug := strings.ToLower(combined)

	// Replace spaces with hyphens
	slug = strings.ReplaceAll(slug, " ", "-")

	// Remove non-alphanumeric characters except hyphens
	slug = nonSlugChars.ReplaceAllString(slug, "")

	// Replace multiple hyphens with single hyphen
	slug = multipleHyphens.ReplaceAllString(slug, "-")

	// Trim leading/trailing hyphens
	slug = strings.Trim(slug, "-")

	return slug
}

// GenerateArtistSlug creates a slug from artist name
// Format: "the-national"
func GenerateArtistSlug(name string) string {
	return GenerateSlug(name)
}

// GenerateVenueSlug creates a slug from venue name, city, and state
// Format: "valley-bar-phoenix-az"
func GenerateVenueSlug(name, city, state string) string {
	return GenerateSlug(name, city, state)
}

// GenerateShowSlug creates a slug from date, headliner name, and venue name.
// Format: "2026-01-30-the-national-at-valley-bar"
//
// The slug carries the show's VENUE-LOCAL calendar date, because that is the
// date every render surface prints beside it; a slug read in a different zone
// than the row is displayed in contradicts the page it addresses. Resolution
// goes through EventLocation, so the zone precedence is stated in exactly one
// place: the venue's own IANA zone, then the US state map, then that map's
// Phoenix default.
//
// venueTimezone is what makes the answer right outside the US (PSY-1873).
// Keying the slug on `state` alone sent every non-US venue through the state
// map's Phoenix default: a Leeds show stored at 2026-10-24T03:00:00Z slugged
// itself "2026-10-23-..." (20:00 Phoenix) while the show page rendered
// "Sat, Oct 24" in Europe/London. Pass nil only where no venue row is in hand.
// The state map is a fallback for missing data, never a substitute for a zone
// the venue already knows.
func GenerateShowSlug(eventDate time.Time, headlinerName, venueName string, venueTimezone *string, state string) string {
	localDate := eventDate.In(EventLocation(venueTimezone, state))
	dateStr := localDate.Format("2006-01-02")
	return GenerateSlug(dateStr, headlinerName, "at", venueName)
}

// GenerateUniqueSlug creates a unique slug by appending a suffix if needed
// checkExists is a function that returns true if the slug already exists
func GenerateUniqueSlug(baseSlug string, checkExists func(slug string) bool) string {
	if !checkExists(baseSlug) {
		return baseSlug
	}

	// Append numeric suffix until unique
	for i := 2; i <= 100; i++ {
		candidate := fmt.Sprintf("%s-%d", baseSlug, i)
		if !checkExists(candidate) {
			return candidate
		}
	}

	// Fallback: append timestamp (should never reach here in practice)
	return fmt.Sprintf("%s-%d", baseSlug, time.Now().UnixNano())
}
