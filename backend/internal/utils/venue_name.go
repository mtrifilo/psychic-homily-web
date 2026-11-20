package utils

import (
	"regexp"
	"strings"
)

// venueNameAbbreviations maps abbreviation forms to their expanded form. Both
// sides normalize to the expanded token, so "7th St Entry" and
// "7th Street Entry" collapse to the same key.
//
// Deliberately SMALL. Every entry here is a claim that two venue names refer to
// the same physical room, and a wrong entry silently merges two real venues —
// a far more expensive mistake than leaving a duplicate for a human to catch.
// Street-type and venue-type words only; nothing that could distinguish two
// rooms operated by the same business.
// Each entry must have exactly ONE common expansion. Ambiguous abbreviations are
// deliberately excluded, because a wrong expansion merges two real venues:
//
//	"n"  → and / north          "dr" → drive / doctor
//	"co" → company / county     "pk" → park / peak
//
// Those would still normalize consistently, so they only bite when two venues in
// one city collide after the transform — but that is exactly the silent-merge
// failure this map exists to avoid, so they stay out.
var venueNameAbbreviations = map[string]string{
	"st":           "street",
	"ave":          "avenue",
	"av":           "avenue",
	"rd":           "road",
	"blvd":         "boulevard",
	"ln":           "lane",
	"hwy":          "highway",
	"sq":           "square",
	"theatre":      "theater",
	"amphitheatre": "amphitheater",
	"bros":         "brothers",
}

// Apostrophes are DELETED rather than spaced, so "Schuba's" and "Schubas"
// collapse together — spacing them would produce "schuba s" and defeat the
// match. Covers the curly variants venue calendars paste in from word
// processors.
var venueNameApostrophes = regexp.MustCompile("['‘’ʼ`]+")

// Every other punctuation run becomes a space: "Rock-n-Roll" and "Rock n Roll"
// should agree, and "Venue/Annex" should split into words rather than fusing.
var venueNamePunctuation = regexp.MustCompile(`[^\p{L}\p{N}\s]+`)
var venueNameWhitespace = regexp.MustCompile(`\s+`)

// NormalizeVenueName reduces a venue name to a comparison key that survives the
// spelling variance venue calendars actually contain.
//
// This exists because `venues` is already uniquely indexed on
// (lower(name), lower(city)) — and that constraint did NOT stop
// "7th St Entry" from being created alongside "7th Street Entry" in Minneapolis,
// because the two lowercase strings genuinely differ. The duplicate then
// silently duplicated ~90 shows, since show dedup keys on
// (artist_id, venue_id, event_date) and a second venue row means a second
// venue_id.
//
// Normalization is intentionally conservative: case, punctuation, whitespace, a
// leading "the", and a small explicit abbreviation map. It does NOT do fuzzy or
// edit-distance matching — "Salt Shed" and "Salt Shed Fairgrounds" are
// genuinely different rooms at the same complex, and any similarity threshold
// loose enough to merge "Metro Gallery" with "Metro Baltimore" would also merge
// those. Catching the abbreviation class is worth doing; guessing is not.
func NormalizeVenueName(name string) string {
	s := strings.ToLower(strings.TrimSpace(name))
	if s == "" {
		return ""
	}

	// Ampersand becomes a word before punctuation stripping removes it.
	s = strings.ReplaceAll(s, "&", " and ")

	// Order matters: delete apostrophes BEFORE the general punctuation pass,
	// which would otherwise turn them into word breaks.
	s = venueNameApostrophes.ReplaceAllString(s, "")
	s = venueNamePunctuation.ReplaceAllString(s, " ")
	s = venueNameWhitespace.ReplaceAllString(s, " ")
	s = strings.TrimSpace(s)

	// A leading article is noise: "The Empty Bottle" and "Empty Bottle" are the
	// same room. Only leading — "Thalia Hall The Annex" keeps its interior words.
	s = strings.TrimPrefix(s, "the ")

	fields := strings.Fields(s)
	out := make([]string, 0, len(fields))
	for _, f := range fields {
		if expanded, ok := venueNameAbbreviations[f]; ok {
			f = expanded
		}
		out = append(out, f)
	}
	return strings.Join(out, " ")
}

// VenueNameWithoutTrailingCity returns the normalized key of name with the
// normalized words of city removed from its end, and true when name did end
// with them. It returns false when name does not end with city, and when name is
// nothing but the city, so the key is never empty when ok is true.
//
// Both sides go through NormalizeVenueName, so "Myth Live St Paul" in
// "St. Paul" strips to "myth live", a key comparable with NormalizeVenueName of
// any other room's name. A city's leading "the" is kept, because inside a name
// it is not leading and NormalizeVenueName keeps it there: "Dosey Doe The
// Woodlands" in "The Woodlands" strips to "dosey doe". Spellings the
// normalization does not unify do not match ("Saint Paul" is not "St. Paul"),
// which leaves a twin for a person to find rather than joining the wrong room.
func VenueNameWithoutTrailingCity(name, city string) (string, bool) {
	nameWords := strings.Fields(NormalizeVenueName(name))
	cityWords := strings.Fields(NormalizeVenueName(city))
	if raw := strings.Fields(strings.ToLower(city)); len(raw) > 1 && raw[0] == "the" {
		cityWords = append([]string{"the"}, cityWords...)
	}
	if len(cityWords) == 0 || len(nameWords) <= len(cityWords) {
		return "", false
	}
	head := len(nameWords) - len(cityWords)
	for i, w := range cityWords {
		if nameWords[head+i] != w {
			return "", false
		}
	}
	return strings.Join(nameWords[:head], " "), true
}
