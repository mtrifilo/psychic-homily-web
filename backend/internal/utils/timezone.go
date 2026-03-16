package utils

import "strings"

// StateTimezones maps US state abbreviations to IANA timezone names.
var StateTimezones = map[string]string{
	"AZ": "America/Phoenix",
	"CA": "America/Los_Angeles",
	"NV": "America/Los_Angeles",
	"CO": "America/Denver",
	"NM": "America/Denver",
	"IL": "America/Chicago",
	"TX": "America/Chicago",
	"NY": "America/New_York",
}

// GetTimezoneForState returns the IANA timezone for a US state abbreviation.
// Defaults to "America/Phoenix" if the state is not found.
func GetTimezoneForState(state string) string {
	if tz, ok := StateTimezones[strings.ToUpper(state)]; ok {
		return tz
	}
	return "America/Phoenix"
}
