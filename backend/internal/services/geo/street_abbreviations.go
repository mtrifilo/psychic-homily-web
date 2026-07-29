package geo

import (
	"regexp"
	"strings"
)

// Street-name abbreviation expansion for Nominatim structured lookups.
//
// Nominatim resolves what OSM actually stores. Venue 128 ("75 M.L.K. Jr Dr SW",
// Atlanta) is stored abbreviated and returns NO match, while the expanded
// "75 Martin Luther King Jr Dr SW" resolves rooftop — verified against live
// Nominatim during the PSY-1607 investigation.
//
// The table is deliberately TINY, and kept that way on purpose. A wide table is
// not a bigger win, it is a bigger blast radius: this path is shared by the
// inline venue write and the scheduled sweep, so a wrong expansion mangles
// addresses that currently resolve. Grow it only from an observed miss.
//
// The obvious candidates St./Ft./Mt. are deliberately EXCLUDED. "St." is a false
// friend — it means Saint at the head of a name ("St. Marks Pl") and Street at
// the tail ("Main St"), and expanding the tail produces "Main Saint", which
// resolves to nothing. Disambiguating by position is a heuristic that would need
// its own evidence; none of those forms has been observed missing. (Note the
// city-matching code in geo.go DOES expand St./Ft./Mt., which is safe there
// because a CITY name has no trailing-street sense.)

// streetAbbreviations maps a whole-token abbreviation to its expansion. Keys are
// matched case-insensitively as complete tokens, never as substrings, so
// "Stmlkst" is untouched.
var streetAbbreviations = map[string]string{
	"m.l.k.": "Martin Luther King",
	"mlk":    "Martin Luther King",
}

// streetTokenSplit splits on runs of whitespace while preserving the tokens.
var streetTokenSplit = regexp.MustCompile(`\s+`)

// ExpandStreetAbbreviations rewrites known abbreviations in a street line to the
// forms OSM is more likely to store, returning the input unchanged when nothing
// matches.
//
// Whole-token only: a token is compared after stripping surrounding punctuation
// that is not part of the abbreviation itself, so "M.L.K." matches with or
// without a trailing comma. Matching on substrings would corrupt real street
// names that merely contain the letters.
func ExpandStreetAbbreviations(street string) string {
	if strings.TrimSpace(street) == "" {
		return street
	}
	tokens := streetTokenSplit.Split(street, -1)
	changed := false
	for i, tok := range tokens {
		// Preserve any trailing comma so "M.L.K., Atlanta" keeps its separator.
		trailing := ""
		bare := tok
		for len(bare) > 0 && (bare[len(bare)-1] == ',' || bare[len(bare)-1] == ';') {
			trailing = string(bare[len(bare)-1]) + trailing
			bare = bare[:len(bare)-1]
		}
		if expanded, ok := streetAbbreviations[strings.ToLower(bare)]; ok {
			tokens[i] = expanded + trailing
			changed = true
		}
	}
	if !changed {
		return street
	}
	return strings.Join(tokens, " ")
}

// WithStreet returns a copy of the query with a different street line, leaving
// the scoping fields untouched. Used to retry a lookup with the address exactly
// as stored when the expanded form misses.
func (q AddressQuery) WithStreet(street string) AddressQuery {
	q.Street = street
	return q
}
