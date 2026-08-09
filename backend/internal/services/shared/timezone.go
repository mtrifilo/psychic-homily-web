package shared

import (
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"gorm.io/gorm"
)

// ErrUnknownTimezone means the server's zone catalog does not carry the name.
// It is deliberately distinguishable from a database failure: callers degrade a
// bad NAME to NULL, but must not do that when the query merely failed, or a
// transient blip silently replaces a correct timezone with nothing.
var ErrUnknownTimezone = errors.New("not a recognized IANA zone name")

// NormalizeIANATimezone validates a timezone against the SAME catalog the
// readers resolve it through — the server's own pg_timezone_names — and returns
// that catalog's canonical spelling to persist.
//
// This is an ALLOWLIST, deliberately NARROWER than what any single reader
// accepts. Do not read it as "validate against whatever will consume it" — an
// earlier version of this comment claimed that and it was wrong. Measured on
// postgres:18:
//
//	SELECT now() AT TIME ZONE 'EST'           -> works (via pg_timezone_abbrevs)
//	SELECT now() AT TIME ZONE 'Asia/Calcutta' -> ERROR: not recognized
//	SELECT now() AT TIME ZONE 'Not/AZone'     -> ERROR: not recognized
//
// So AT TIME ZONE accepts the union of pg_timezone_names and
// pg_timezone_abbrevs, and this function rejects the abbreviations. That is a
// POLICY choice, not a raising-vs-not fact: a fixed-offset abbreviation like
// EST carries no DST rule, so storing it as a venue's zone would freeze that
// venue on standard time and silently mis-date half its shows. Rejecting it is
// the point, and "Go accepts it" is beside the point.
//
// What the narrowness buys is that a stored value is a real region zone. What
// it does NOT buy is immunity from a reader raising — see the tzdata note below.
//
// WHAT pg_timezone_names ACTUALLY CONTAINS is a property of the server's tzdata
// PACKAGING, not of Postgres, and it is not stable across images. Measured:
//
//	postgres:18       (Debian) 487 zones — no EST, no US/Pacific, no EST5EDT
//	postgres:16-alpine         599 zones — all three present
//
// Debian splits the tzdata `backward` compatibility links into a separate
// tzdata-legacy package; alpine's bundled tzdata keeps them. Two consequences
// worth carrying forward:
//
//   - Tests must not hard-code "EST is rejected". Ask the catalog first.
//   - A value validated at write time can still become unknown LATER — a
//     Postgres upgrade, a tzdata refresh (US/Pacific-New really was deleted in
//     2020b), or a restore onto a differently-packaged build. So this guard
//     raises the floor; it does NOT make readers unconditionally safe.
//
// That last point is load-bearing for PSY-1695: a reader that would RAISE on an
// unknown zone must keep its own COALESCE fallback. This guard is not a licence
// to drop it. Go-side readers (utils.EventLocation, the ICS feeds, Discord) are
// validated against Go's catalog, not this one, and are likewise not covered:
// "localtime" and "Factory" pass here and fail time.LoadLocation.
//
// Established for radio stations by PSY-1204 (RadioService.normalizeStationTimezone,
// which now delegates here) and generalized for venues by PSY-1707.
//
// nil or blank returns nil — store SQL NULL and let the caller's fallback chain
// decide. A non-blank value the catalog does not carry returns ErrUnknownTimezone;
// any other error is a database failure and means "unknown", not "invalid".
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
		return nil, fmt.Errorf("timezone %q: %w", trimmed, ErrUnknownTimezone)
	}
	return &canonical, nil
}

// NormalizedGeocodedTimezoneOrNull canonicalizes a zone that was DERIVED
// internally (the offline GeoNames geocoder), returning nil when the catalog
// does not carry it.
//
// One home for the log-and-NULL policy, because that policy is the part most
// likely to change. THE authoritative census of who must call it — keep this
// list current and do not start a second copy elsewhere; venues.timezone gained
// a silently-stale writer (PSY-1709) precisely because the count lived in two
// comments and only one got updated:
//
//   - catalog.VenueService.applyGeocoding (venue create + update)
//   - admin.applyDerivedVenueLocation, reached through admin.applyDerivedLocation
//     (pending-edit approval AND revision rollback)
//   - admin.data_sync importVenue and importShow (two seams)
//   - catalog.backfillVenuePass (the backfill CLI)
//
// Degrades rather than failing the write, deliberately: the value is ours, not
// the caller's, so a bad one is our bug and refusing the user's venue would be
// the wrong end of it. NULL is a shape every reader already handles — it is what
// a geocode MISS produces — and it falls back to the state map. A
// REQUEST-supplied timezone would want the opposite treatment (reject with 422),
// which is why NormalizeIANATimezone still surfaces the error for callers that
// want it. No venue write path accepts one today.
//
// A DATABASE failure is not a bad zone: the derived value passes through
// untouched rather than being nulled, because "the pool timed out" must never
// look like "this venue has no timezone". logCtx is appended to the log line so
// an operator can find the affected row.
func NormalizedGeocodedTimezoneOrNull(db *gorm.DB, tz *string, logCtx ...any) *string {
	// No database means nothing is being persisted (some callers use the
	// surrounding derivation helpers as pure functions in unit tests), so there
	// is nothing to guard and the derived value passes through.
	if db == nil {
		return tz
	}
	canonical, err := NormalizeIANATimezone(db, tz)
	switch {
	case err == nil:
		return canonical
	case errors.Is(err, ErrUnknownTimezone):
		slog.Error("geocoded timezone is not in the server's zone catalog; storing NULL",
			append([]any{"rejected_timezone", DerefOrEmpty(tz), "error", err}, logCtx...)...)
		return nil
	default:
		slog.Error("could not validate geocoded timezone; keeping the derived value",
			append([]any{"timezone", DerefOrEmpty(tz), "error", err}, logCtx...)...)
		return tz
	}
}

// DerefOrEmpty reads a nullable string as the empty string — the shape both the
// log fields above and the geocoder take for "not set". Exported because the
// admin location derivation needs the same unwrap, and the alternative was one
// more private copy of these five lines (the tree already has several).
func DerefOrEmpty(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
