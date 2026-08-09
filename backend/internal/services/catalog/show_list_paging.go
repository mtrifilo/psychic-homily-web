package catalog

import (
	"fmt"

	"gorm.io/gorm"

	"psychic-homily-backend/internal/services/shared"
)

// Mechanics shared by every entity show list (an artist's, a venue's): the page
// window, and the venue-local year histogram behind its year picker.
//
// These live here rather than in either service because both already implement
// them identically, and the parts worth stating once are the parts a reader
// would otherwise have to trust two copies of: what GORM does with a negative
// bound, and why the histogram's aliases are quoted.

// Whether the CALLER's own select/group expressions dereference venue_tz,
// independent of what the WHERE clauses need. Named rather than a bare literal
// at the call site, where `true` would not say which of the two reasons applies.
//
// "Venue zone" is the right word on every entity's path, not just the venue's:
// a show is dated by the venue it happens at whichever list is asking.
const (
	venueZoneNotNeededBySelect = false
	venueZoneNeededBySelect    = true
)

// clampPageWindow floors a caller's page bounds at zero.
//
// It exists because GORM reads a negative value in each as a DIFFERENT
// instruction, and neither is what a caller with a miscomputed page size means:
//
//   - a negative offset becomes `OFFSET -1`, which Postgres rejects outright
//     (a 500);
//   - a negative limit CANCELS the limit clause entirely, which is worse
//     because it succeeds: the entity's whole history comes back hydrated with
//     every bill, silently.
//
// Clamping to zero matches what limit 0 already means on these paths (no rows,
// real total). It belongs in the service layer rather than the handlers because
// the huma `minimum:"0"` tags only guard the HTTP path, and non-HTTP callers
// build the query struct directly.
func clampPageWindow(limit, offset int) (int, int) {
	if limit < 0 {
		limit = 0
	}
	if offset < 0 {
		offset = 0
	}
	return limit, offset
}

// showYearBucket is the scan target for the histogram below: one venue-local
// calendar year and how many rows fell in it.
//
// Concrete rather than a type parameter on the scan, deliberately. A generic
// scan would let any caller's struct through, and GORM answers a column/field
// mismatch by leaving the fields zero rather than erroring — a picker rendering
// "0 (0)" with nothing logged anywhere. Each service maps these two fields into
// its own response type, so a mismatch is a compile error instead.
type showYearBucket struct {
	Year  int
	Count int64
}

// scanVenueLocalYearBuckets counts baseQuery's rows per VENUE-LOCAL calendar
// year, newest year first. Years with no rows are absent rather than zero,
// because the consumer is a year picker and an empty year is not a selectable
// option.
//
// baseQuery must already have joined shared.VenueTZJoin (pass
// venueZoneNeededBySelect to whichever factory builds it): the year expression
// dereferences venue_tz even when no WHERE clause does.
//
// Aliases are quoted and the ORDER BY repeats the expression rather than naming
// the alias: `year` and `count` are both keywords Postgres would otherwise be
// free to resolve against something else.
func scanVenueLocalYearBuckets(baseQuery func() *gorm.DB) ([]showYearBucket, error) {
	var buckets []showYearBucket
	if err := baseQuery().
		Select(shared.VenueLocalYearSQL + ` AS "year", COUNT(*) AS "count"`).
		Group(shared.VenueLocalYearSQL).
		Order(shared.VenueLocalYearSQL + " DESC").
		Scan(&buckets).Error; err != nil {
		return nil, fmt.Errorf("failed to count shows by year: %w", err)
	}
	return buckets, nil
}
