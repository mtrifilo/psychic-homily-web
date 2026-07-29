package geo

import "strings"

// Street-name abbreviation expansion for Nominatim structured lookups.
//
// Nominatim resolves what OSM actually stores. Venue 128 ("75 M.L.K. Jr Dr SW",
// Atlanta) is stored abbreviated and returns NO match, while the expanded
// "75 Martin Luther King Jr Dr SW" resolves rooftop — verified against live
// Nominatim in both directions.
//
// THIS FILE OWNS THE POLICY. Callers point here rather than restating it.
//
// The table is deliberately TINY and stays that way without evidence. A wide
// table is not a bigger win, it is a bigger blast radius: this runs on the path
// shared by the inline venue write AND the scheduled sweep, so a wrong expansion
// mangles addresses that resolve today — a regression affecting venues nobody was
// complaining about. To add an entry: confirm against live Nominatim that the
// abbreviated form actually misses and the expanded form hits, then add a test
// for both that case and a near-miss that must NOT expand.
//
// St./Ft./Mt. are deliberately EXCLUDED despite being the obvious candidates.
// "St." is a false friend — Saint at the head of a name ("St. Marks Pl"), Street
// at the tail ("Main St") — and expanding the tail yields "Main Saint", which
// resolves to nothing. Disambiguating by position is a heuristic that needs its
// own evidence, and none of those forms has been observed missing.
//
// geo.go's expandPlaceAbbreviations DOES expand St./Ft./Mt., and that is not an
// inconsistency: it operates on CITY names (no trailing-street sense), runs
// inside foldKey on already-normalized text, and produces a match key that is
// never sent anywhere — so it may be lossy and is applied symmetrically to both
// the dataset and the query. This function produces a string sent verbatim to a
// third party and is one-sided. Same idea, incompatible constraints; keep the
// tables disjoint.

// streetAbbreviations maps a whole-token abbreviation to its expansion.
//
// KEYS MUST BE LOWERCASE and a single token — lookup is by strings.ToLower on
// one whitespace-delimited token, so an uppercase key is silently dead and a
// multi-word key can never match. Only a trailing "," or ";" is tolerated on the
// token; internal punctuation is part of the key ("m.l.k." matches, "mlk." does
// not).
var streetAbbreviations = map[string]string{
	"m.l.k.": "Martin Luther King",
	"m.l.k":  "Martin Luther King", // no trailing period
	"mlk":    "Martin Luther King",
	"mlk.":   "Martin Luther King",
}

// ExpandStreetAbbreviations rewrites known abbreviations in a street line to the
// forms OSM is more likely to store, returning the input unchanged when nothing
// matches.
//
// Tokenised with strings.Fields rather than a regexp because Fields splits on
// unicode.IsSpace while Go's regexp \s is ASCII-only. An address containing a
// non-breaking space — routine in text pasted from a venue's calendar page —
// would otherwise leave "75 MLK" as a single token and silently skip the
// expansion, which is exactly the failure this function exists to fix.
//
// When something IS expanded, whitespace is normalised to single spaces as a
// side effect. Harmless here: the result goes to Nominatim's structured search
// and into Key(), and neither treats runs of whitespace as significant.
func ExpandStreetAbbreviations(street string) string {
	tokens := strings.Fields(street)
	changed := false
	for i, tok := range tokens {
		// Preserve a trailing separator so "M.L.K., Atlanta" keeps its comma.
		// Street is a structured field and should not normally carry one, but
		// PSY-1607 exists because stored addresses do pick up ", city, state"
		// tails — dropping it here would corrupt an address rather than fix one.
		bare := strings.TrimRight(tok, ",;")
		if expanded, ok := streetAbbreviations[strings.ToLower(bare)]; ok {
			tokens[i] = expanded + tok[len(bare):]
			changed = true
		}
	}
	if !changed {
		// Return the ORIGINAL rather than the re-joined tokens: with nothing to
		// expand there is no reason to alter the caller's spacing.
		return street
	}
	return strings.Join(tokens, " ")
}
