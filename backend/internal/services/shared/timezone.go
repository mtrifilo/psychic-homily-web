package shared

import (
	"fmt"
	"strings"

	"gorm.io/gorm"
)

// NormalizeIANATimezone validates a timezone against the SAME catalog the
// readers resolve it through — Postgres' pg_timezone_names — and returns that
// catalog's canonical spelling to persist.
//
// time.LoadLocation is deliberately NOT the validator here, and this is the
// whole point of the function. Go's tz catalog and Postgres' are not the same
// set: Go accepts abbreviations like "EST" and the alias "Local", which
// pg_timezone_names does not carry. Validating with Go would therefore accept
// values that Postgres later resolves to something else — and `AT TIME ZONE`
// does not fail quietly for a name it cannot find, it raises and takes the
// whole query down. The rule is: validate against whatever will consume it.
//
// Established for radio stations by PSY-1204 (RadioService.normalizeStationTimezone,
// which now delegates here) and generalized for venues by PSY-1707, because
// PSY-1695 wants to drop the per-row pg_timezone_names join from the show-list
// partition and simply trust venues.timezone. That is only safe if the write
// boundary guarantees what the read side assumes.
//
// nil or blank returns nil — store SQL NULL and let the caller's own fallback
// chain decide. A non-blank value absent from pg_timezone_names is an error;
// callers choose whether that is a 422 (the value came from a request) or a
// log-and-NULL (the value came from an internal derivation).
func NormalizeIANATimezone(db *gorm.DB, tz *string) (*string, error) {
	if tz == nil {
		return nil, nil
	}
	trimmed := strings.TrimSpace(*tz)
	if trimmed == "" {
		return nil, nil
	}
	if db == nil {
		return nil, fmt.Errorf("validate timezone %q: database not initialized", trimmed)
	}
	var canonical string
	if err := db.Raw(
		"SELECT name FROM pg_timezone_names WHERE lower(name) = lower(?) LIMIT 1", trimmed,
	).Scan(&canonical).Error; err != nil {
		return nil, fmt.Errorf("validate timezone %q: %w", trimmed, err)
	}
	if canonical == "" {
		return nil, fmt.Errorf("invalid timezone %q: not a recognized IANA zone name", trimmed)
	}
	return &canonical, nil
}
