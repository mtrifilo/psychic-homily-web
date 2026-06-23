package utils

import "strings"

// stateAbbrevByName maps a full US state (and DC) name to its two-letter
// abbreviation. Used to align an external full-name area (e.g. MusicBrainz tags
// a US-state "Subdivision" area by its full name "Minnesota") with the
// abbreviation stored in venues.state (PSY-1191). Keys are lowercased at lookup
// time so callers don't have to pre-normalize casing.
var stateAbbrevByName = map[string]string{
	"alabama":              "AL",
	"alaska":               "AK",
	"arizona":              "AZ",
	"arkansas":             "AR",
	"california":           "CA",
	"colorado":             "CO",
	"connecticut":          "CT",
	"delaware":             "DE",
	"district of columbia": "DC",
	"florida":              "FL",
	"georgia":              "GA",
	"hawaii":               "HI",
	"idaho":                "ID",
	"illinois":             "IL",
	"indiana":              "IN",
	"iowa":                 "IA",
	"kansas":               "KS",
	"kentucky":             "KY",
	"louisiana":            "LA",
	"maine":                "ME",
	"maryland":             "MD",
	"massachusetts":        "MA",
	"michigan":             "MI",
	"minnesota":            "MN",
	"mississippi":          "MS",
	"missouri":             "MO",
	"montana":              "MT",
	"nebraska":             "NE",
	"nevada":               "NV",
	"new hampshire":        "NH",
	"new jersey":           "NJ",
	"new mexico":           "NM",
	"new york":             "NY",
	"north carolina":       "NC",
	"north dakota":         "ND",
	"ohio":                 "OH",
	"oklahoma":             "OK",
	"oregon":               "OR",
	"pennsylvania":         "PA",
	"rhode island":         "RI",
	"south carolina":       "SC",
	"south dakota":         "SD",
	"tennessee":            "TN",
	"texas":                "TX",
	"utah":                 "UT",
	"vermont":              "VT",
	"virginia":             "VA",
	"washington":           "WA",
	"west virginia":        "WV",
	"wisconsin":            "WI",
	"wyoming":              "WY",
}

// StateNameToAbbrev returns the two-letter abbreviation for a full US state
// name. ok is false for an unknown name (a non-US area, a country, etc.). The
// input is trimmed and case-folded before lookup. An input that is ALREADY a
// valid two-letter abbreviation is returned as-is, so callers can pass either
// form (a defensive convenience — external sources are inconsistent).
func StateNameToAbbrev(name string) (abbrev string, ok bool) {
	trimmed := strings.TrimSpace(name)
	if abbr, found := stateAbbrevByName[strings.ToLower(trimmed)]; found {
		return abbr, true
	}
	// Already an abbreviation?
	upper := strings.ToUpper(trimmed)
	if _, isTZ := StateTimezones[upper]; isTZ {
		return upper, true
	}
	return "", false
}
