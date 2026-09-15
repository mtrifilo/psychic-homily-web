package catalog

import (
	"database/sql"
	"fmt"
	"time"

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
// because the paging readers run their count and their page inside one
// transaction and must hang both on that transaction's handle. GORM builders
// accumulate clauses, so a shared builder would leak the page's ORDER and LIMIT
// into the count.
//
// The window is applied LAST and narrows only the rows, never the meaning of
// "upcoming": a day in a past month intersects the upcoming partition at
// nothing and correctly returns an empty page.
func (s *ShowService) upcomingShowPredicates(
	includeNonApproved bool,
	filters *contracts.UpcomingShowsFilter,
	window contracts.ShowCalendarWindow,
) func(*gorm.DB) *gorm.DB {
	// The narrowest fragment the window names, built once outside the applier: it
	// is a pure function of the window, and the applier runs per read. Each
	// resolution already implies the wider one, so the narrowest fragment alone
	// IS the window, and one predicate does the work overlapping ones would;
	// which resolution that is belongs to the vocabulary, so the SQL package
	// owns the choice.
	//
	// An impossible day lands on its month. That is WIDER than the caller
	// addressed, not narrower, and it is a backstop rather than a path:
	// ShowCalendarWindow.Validate refuses every such triple at both entry points.
	// It is still the right backstop, because the alternative is no window.
	windowCondition, windowArgs := shared.VenueLocalWindowCondition(
		window.Year, window.Month, window.Day, window.Days,
	)

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
//
// The WINDOW is validated here; the page SIZE is the caller's obligation.
// clampPageWindow floors a negative Limit at zero, and zero means "no rows, real
// total" on every list that shares it, so a caller passing Limit 0 gets an empty
// page beside a non-zero total rather than a default page. Handlers resolve it
// through clampShowListLimit before calling.
//
// OFFSET paging over a set that rolls forward at venue-local midnight has one
// failure mode, and it is the quiet direction: a show graduating out of the
// partition between two page reads shifts every later row up one, so a reader
// walking pages SKIPS a row rather than seeing it twice. Nothing in the envelope
// reveals it. That is the price of a page number that survives being bookmarked,
// which is what this list is addressed by; the cursor endpoint is the one that
// cannot skip.
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
		//
		// The sort key is the stored INSTANT while membership and the window are
		// venue-local DATES, and the two orderings are the same only within one
		// zone. Across zones the instants of one venue-local date span more than
		// a day, so two rows within that band can be returned in an order that
		// inverts their local dates: a 22:00 Honolulu show and an 02:00 Boston
		// show the next local morning are four hours apart the other way in UTC.
		//
		// Deliberate, and the same call the artist archive made: this is the
		// ordering the cursor list uses, whose cursor IS event_date, and the two
		// share one predicate set so an unwindowed page here matches that list
		// row for row. Sorting venue-locally would change shipped behaviour and
		// give up the index. What it costs is bounded: rows can permute WITHIN a
		// window, never move between windows, because membership is decided by
		// the local date alone.
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

// The two bands around now that the addressable range holds open whatever the
// catalogue contains. Both exist so a URL the list's own chrome can produce is
// never outside the range, because outside it is a hard 404.
//
// BACKWARD is one day, which covers every inhabited UTC offset: the range is
// stated in whole months and a month has no zone, while the readers it bounds
// partition per row on their own venue's clock, and no wall clock on earth is
// more than a day from UTC's. Nothing points further back than today: the quick
// windows anchor on the later of today and their own start.
//
// FORWARD is a week, which covers the same offset band plus the furthest date
// the quick-window row can address: "This weekend" anchors on the coming Friday,
// four days out on a Monday. Without it, a Monday in the last days of a month
// with nothing upcoming next month sends that chip to a hard 404, the dead end
// the range exists to prevent. A week leaves margin for the run lengths those
// chips carry.
//
// Widening is the safe direction in both. A month inside the range that holds
// nothing renders as a quiet page, which is recoverable; a month outside it is
// a hard 404 on a URL the site itself is printing.
const (
	calendarRangeBackwardDays = 1
	calendarRangeForwardDays  = 7
)

// showCalendarMonthOf is the calendar month an instant falls in, read on the
// instant's own zone.
func showCalendarMonthOf(at time.Time) contracts.ShowCalendarMonth {
	return contracts.ShowCalendarMonth{Year: at.Year(), Month: int(at.Month())}
}

// showCalendarMonthOrdinal orders two months on one axis, so a comparison
// cannot read a December as earlier than the January after it.
func showCalendarMonthOrdinal(month contracts.ShowCalendarMonth) int {
	return month.Year*12 + month.Month
}

// GetUpcomingShowsCalendarRange returns the month span the date-addressed
// upcoming list is addressable over.
//
// See contracts.ShowServiceInterface for the contract. Both edges are computed
// here rather than left to the caller because they answer one question and a
// caller holding half of it would have to guess the other half.
//
// The LAST edge is read through upcomingShowPredicates, the same applier the
// windowed page and the month histogram build on, so the last addressable month
// and the last month the strip offers cannot disagree about which shows are
// upcoming. It reads ONE row: the partition's latest venue-local date, whose
// month is the edge. A COUNT or a histogram would carry every bucket back to
// learn the last one.
func (s *ShowService) GetUpcomingShowsCalendarRange() (contracts.ShowCalendarRange, error) {
	if s.db == nil {
		return contracts.ShowCalendarRange{}, fmt.Errorf("database not initialized")
	}

	now := time.Now().UTC()
	first := showCalendarMonthOf(now.AddDate(0, 0, -calendarRangeBackwardDays))
	last := showCalendarMonthOf(now.AddDate(0, 0, calendarRangeForwardDays))

	applyPredicates := s.upcomingShowPredicates(false, nil, contracts.ShowCalendarWindow{})

	var latest struct {
		Year  int
		Month int
	}
	// The aliases are quoted for the reason scanVenueLocalMonthBuckets quotes
	// its own: `year` and `month` are keywords Postgres is otherwise free to
	// resolve against something else.
	result := applyPredicates(s.db.Model(&catalogm.Show{})).
		Select(shared.VenueLocalYearSQL + ` AS "year", ` + shared.VenueLocalMonthSQL + ` AS "month"`).
		Order(shared.VenueLocalDateSQL + " DESC").
		Limit(1).
		Scan(&latest)
	if result.Error != nil {
		return contracts.ShowCalendarRange{}, fmt.Errorf("failed to read the last upcoming show month: %w", result.Error)
	}

	// RowsAffected, not a zero Year: an empty upcoming partition scans nothing
	// and leaves the struct at its zero value, which reads as year 0 month 0 and
	// would move the edge to the beginning of the calendar.
	if result.RowsAffected > 0 {
		latestMonth := contracts.ShowCalendarMonth{Year: latest.Year, Month: latest.Month}
		if showCalendarMonthOrdinal(latestMonth) > showCalendarMonthOrdinal(last) {
			last = latestMonth
		}
	}

	return contracts.ShowCalendarRange{FirstMonth: first, LastMonth: last}, nil
}
