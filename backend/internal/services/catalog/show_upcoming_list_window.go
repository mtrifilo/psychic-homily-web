package catalog

import (
	"database/sql"
	"fmt"

	"gorm.io/gorm"

	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/services/shared"
)

// The date-windowed readers of the venue-local upcoming partition: an offset
// page of one month or one day, and the month histogram that labels its jump
// targets.
//
// They sit beside GetUpcomingShows rather than replacing it because the two
// answer different questions about the same set. The cursor list is a feed being
// scrolled, where a stable position matters more than a page number; these are
// addresses, where a page number has to survive being bookmarked. Both build
// their predicates through upcomingShowPredicates below, so neither can drift
// about which shows are upcoming, which cities count, or how a tag filter
// reaches a bill.

// upcomingShowPredicates returns the predicate applier every reader of the
// upcoming partition shares: status, city/state, tags, the venue-local date
// rule, and an optional venue-local calendar window.
//
// An applier taking a caller-supplied *gorm.DB rather than a builder factory,
// because the readers run their count and their page inside one transaction and
// must hang both on that transaction's handle. GORM builders accumulate clauses,
// so a shared builder would leak the page's ORDER and LIMIT into the count.
//
// The window is applied LAST and narrows only the rows, never the meaning of
// "upcoming": a day in a past month intersects the upcoming partition at
// nothing and correctly returns an empty page.
func (s *ShowService) upcomingShowPredicates(
	includeNonApproved bool,
	filters *contracts.UpcomingShowsFilter,
	window contracts.ShowCalendarWindow,
) func(*gorm.DB) *gorm.DB {
	// The narrowest fragment the window names, built once outside the applier:
	// it is a pure function of the window, and the applier runs per read. A day
	// already implies its month, so asking for the day first and falling back is
	// one predicate rather than two overlapping ones. A day that names no real
	// date falls back to its month, which is the fail-closed direction.
	windowCondition, windowArgs := shared.VenueLocalDayCondition(window.Year, window.Month, window.Day)
	if windowCondition == "" {
		windowCondition, windowArgs = shared.VenueLocalMonthCondition(window.Year, window.Month)
	}

	return func(query *gorm.DB) *gorm.DB {
		// Every predicate below is table-qualified. The venue-local lateral
		// aliases its own columns (shared.VenueTZJoin) so none of them can
		// collide with a `shows` column, which means this is hygiene rather than
		// load-bearing. It is still worth doing: this query now spans two
		// relations, and a reader should not have to know the lateral's
		// projection to tell which one a bare column came from.

		// Filter by status for non-admin users (public view shows only approved)
		if !includeNonApproved {
			query = query.Where("shows.status = ?", catalogm.ShowStatusApproved)
		} else {
			// For admin view, still exclude private shows (those are personal to the submitter)
			query = query.Where("shows.status != ?", catalogm.ShowStatusPrivate)
		}

		// Apply city/state filters if provided
		if filters != nil {
			if len(filters.Cities) > 0 {
				// Multi-city filter: (city = ? AND state = ?) OR ...
				conditions := s.db
				for i, cs := range filters.Cities {
					if i == 0 {
						conditions = conditions.Where("(shows.city = ? AND shows.state = ?)", cs.City, cs.State)
					} else {
						conditions = conditions.Or("(shows.city = ? AND shows.state = ?)", cs.City, cs.State)
					}
				}
				query = query.Where(conditions)
			} else {
				// Legacy single-city filter
				if filters.City != "" {
					query = query.Where("shows.city = ?", filters.City)
				}
				if filters.State != "" {
					query = query.Where("shows.state = ?", filters.State)
				}
			}
			if len(filters.TagSlugs) > 0 {
				// Transitive artist-based tag filtering: shows match when any
				// billed artist has the tag. Direct `entity_type='show'` tags
				// are ignored because shows are not directly tagged with genres.
				query = ApplyTransitiveArtistTagFilter(
					query, s.db,
					"show_artists", "show_id", "artist_id",
					"shows.id",
					TagFilter{
						TagSlugs: filters.TagSlugs,
						MatchAny: filters.TagMatchAny,
					},
				)
			}
		}

		// Partition on each show's own venue-local calendar day.
		query = query.
			Joins(shared.VenueTZJoin).
			Where(shared.VenueLocalDateCondition("upcoming"))

		// The window fragment dereferences venue_tz, which the join above has
		// already supplied.
		if windowCondition != "" {
			query = query.Where(windowCondition, windowArgs...)
		}
		return query
	}
}

// GetUpcomingShowsPage returns one OFFSET page of the venue-local upcoming
// partition plus the filter-aware total for the requested window.
//
// See contracts.ShowServiceInterface for the contract. The transaction is
// RepeatableRead and ReadOnly for the two reasons GetUpcomingShows states at
// length: ONE CLOCK, because the venue-local boundary is evaluated per row
// against Postgres' transaction timestamp and two autocommit statements would
// each get their own; and ONE SNAPSHOT, because an approval or an ingest insert
// committing between the count and the page would otherwise put a total on
// screen that the page contradicts.
func (s *ShowService) GetUpcomingShowsPage(
	query contracts.ShowCalendarQuery,
	includeNonApproved bool,
	filters *contracts.UpcomingShowsFilter,
) ([]*contracts.ShowResponse, int64, error) {
	if s.db == nil {
		return nil, 0, fmt.Errorf("database not initialized")
	}
	// Fail closed: a half-stated window narrows to nothing in SQL, which would
	// return the whole upcoming catalog to a caller who asked for one day of it.
	// The HTTP boundary refuses these first; this is the guard for a caller that
	// builds the query struct directly.
	if err := query.Validate(); err != nil {
		return nil, 0, fmt.Errorf("invalid show calendar window: %w", err)
	}

	limit, offset := clampPageWindow(query.Limit, query.Offset)
	applyPredicates := s.upcomingShowPredicates(includeNonApproved, filters, query.ShowCalendarWindow)

	var total int64
	var shows []catalogm.Show
	err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := applyPredicates(tx.Model(&catalogm.Show{})).Count(&total).Error; err != nil {
			return fmt.Errorf("failed to count upcoming shows: %w", err)
		}

		// `shows.*` is explicit so the lateral's columns cannot widen the
		// projection, for the reason GetUpcomingShows gives.
		page := applyPredicates(tx.Preload("Venues").Select("shows.*"))

		// `shows.id ASC` is not decoration: event_date is not unique, and an
		// unstable tiebreak makes an offset page overlap or skip rows against
		// the page before it.
		if err := page.
			Order("shows.event_date ASC, shows.id ASC").
			Limit(limit).
			Offset(offset).
			Find(&shows).Error; err != nil {
			return fmt.Errorf("failed to get upcoming shows page: %w", err)
		}
		return nil
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return nil, 0, err
	}

	return s.buildShowResponses(shows), total, nil
}

// GetUpcomingShowMonths returns the venue-local month histogram of the upcoming
// partition, soonest month first.
//
// See contracts.ShowServiceInterface for the contract. It takes no window for
// the reason the venue and artist histograms take no year: the histogram is the
// thing that ENUMERATES the months, so narrowing it to one would erase every
// other jump target from the strip it fills.
//
// scanVenueLocalMonthBuckets orders newest-first for every caller, so the
// reversal here is this surface's own: an upcoming list runs forward, and its
// strip has to read the same direction.
func (s *ShowService) GetUpcomingShowMonths(
	includeNonApproved bool,
	filters *contracts.UpcomingShowsFilter,
) ([]contracts.ShowMonthCount, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	applyPredicates := s.upcomingShowPredicates(includeNonApproved, filters, contracts.ShowCalendarWindow{})
	baseQuery := func() *gorm.DB {
		return applyPredicates(s.db.Model(&catalogm.Show{}))
	}

	buckets, err := scanVenueLocalMonthBuckets(baseQuery)
	if err != nil {
		return nil, err
	}

	// Non-nil even when empty: the histogram must serialize as [] rather than null.
	months := make([]contracts.ShowMonthCount, len(buckets))
	for i, bucket := range buckets {
		months[len(buckets)-1-i] = contracts.ShowMonthCount{
			Year:  bucket.Year,
			Month: bucket.Month,
			Count: bucket.Count,
		}
	}
	return months, nil
}
