package catalog

import (
	"fmt"
	"strings"

	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/geo"
	"psychic-homily-backend/internal/services/venueaddress"
)

// venueLikePlaceTypes are the OSM category/type pairs a name-search hit may
// carry to be taken as a music venue. A "building" of any type is also
// accepted (see AcceptPlaceCandidate). Anything else, a road, a
// neighbourhood, a shop, an office, is refused.
var venueLikePlaceTypes = map[string]map[string]bool{
	"amenity": {
		"arts_centre": true, "bar": true, "biergarten": true, "cafe": true,
		"cinema": true, "community_centre": true, "concert_hall": true,
		"events_venue": true, "music_venue": true, "nightclub": true,
		"pub": true, "restaurant": true, "social_centre": true, "theatre": true,
	},
	"craft":   {"brewery": true},
	"leisure": {"stadium": true},
}

// numberFirstCountries write the house number before the street name ("123
// Main St"); elsewhere the stored line puts it after ("Hauptstrasse 12").
var numberFirstCountries = map[string]bool{
	"US": true, "CA": true, "GB": true, "IE": true, "AU": true, "NZ": true, "FR": true,
}

// AcceptPlaceCandidate decides whether a name-search result is the venue, and
// returns the street line to store when it is. All of these must hold:
//   - the feature is venue-like (venueLikePlaceTypes) or a building;
//   - the feature is named, and its name matches the venue's
//     (venueaddress.NamesMatch);
//   - one of its locality components, or a component of its display name,
//     names the venue's city;
//   - for a venue in a US state, the feature's ISO subdivision is that state,
//     when the result carries one;
//   - the feature is in the searched country, when both are known;
//   - the result carries a house number and a road, so the stored address is
//     a postal address rather than a street name.
//
// why explains a refusal.
func AcceptPlaceCandidate(c geo.PlaceCandidate, v *catalogm.Venue, countryCode string) (street string, ok bool, why string) {
	if !venueLikePlaceTypes[c.Category][c.Type] && c.Category != "building" {
		return "", false, "not a venue-like place or a building"
	}
	if strings.TrimSpace(c.Name) == "" {
		return "", false, "the feature has no name"
	}
	if !venueaddress.NamesMatch(c.Name, v.Name) {
		return "", false, fmt.Sprintf("name %q does not match %q", c.Name, v.Name)
	}
	if !placeInCity(c, v.City) {
		return "", false, fmt.Sprintf("not in %s (%s)", v.City, c.DisplayName)
	}
	if geo.IsUSStateCode(v.State) && c.StateCode != "" &&
		!strings.EqualFold(c.StateCode, "US-"+strings.TrimSpace(v.State)) {
		return "", false, fmt.Sprintf("in %s, not US-%s", c.StateCode, strings.ToUpper(strings.TrimSpace(v.State)))
	}
	if countryCode != "" && c.CountryCode != "" && !strings.EqualFold(countryCode, c.CountryCode) {
		return "", false, fmt.Sprintf("in country %s, not %s", strings.ToUpper(c.CountryCode), countryCode)
	}
	if c.HouseNumber == "" || c.Road == "" {
		return "", false, "the result carries no house number and road"
	}
	cc := strings.ToUpper(c.CountryCode)
	if cc == "" {
		cc = strings.ToUpper(countryCode)
	}
	if numberFirstCountries[cc] {
		return c.HouseNumber + " " + c.Road, true, ""
	}
	return c.Road + " " + c.HouseNumber, true, ""
}

// Report flags for rows a reviewer must check: Apply writes a flagged row only
// when the reviewer sets its approve_review. They never change what the
// lookup accepts.
const (
	CautionPartialName = "REVIEW: partial name match"
	CautionBuilding    = "REVIEW: matched a building, not a venue-like place"
	// CautionPageUnread: the venue's own pages failed to load this run, so
	// whether they print a better address is unknown.
	CautionPageUnread = "REVIEW: the venue's own pages could not be read this run"
	// CautionUnverified: the venue is unverified, often a private home, and an
	// earlier removal of its address may have left no trace to check.
	CautionUnverified = "REVIEW: unverified venue; confirm this is a public venue address"
)

// placeMatchCautions returns the review flags for an accepted candidate: the
// names agree only by one containing the other's words, or the feature is a
// building rather than a venue-like place.
func placeMatchCautions(c geo.PlaceCandidate, v *catalogm.Venue) []string {
	var out []string
	if !venueaddress.NamesEqual(c.Name, v.Name) {
		out = append(out, CautionPartialName)
	}
	if !venueLikePlaceTypes[c.Category][c.Type] {
		out = append(out, CautionBuilding)
	}
	return out
}

// placeInCity reports whether any locality component, or any comma-separated
// component of the display name, is the same place name as city.
func placeInCity(c geo.PlaceCandidate, city string) bool {
	parts := append([]string{}, c.Localities...)
	parts = append(parts, strings.Split(c.DisplayName, ",")...)
	for _, part := range parts {
		if geo.SamePlaceName(strings.TrimSpace(part), city) {
			return true
		}
	}
	return false
}
