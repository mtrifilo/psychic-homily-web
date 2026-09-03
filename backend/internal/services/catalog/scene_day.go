package catalog

import (
	"fmt"
	"time"

	apperrors "psychic-homily-backend/internal/errors"
	"psychic-homily-backend/internal/services/contracts"
)

// sceneDayShowCap bounds one day's payload. Chicago, the densest scene, runs
// ~50 shows across a whole busy WEEK, so this is generous headroom for a single
// night while still refusing to serve an unbounded list.
const sceneDayShowCap = 100

// nightStartHour is the scene-local hour at which a new night begins.
//
// Until 06:00 the current night is still the PREVIOUS calendar date, because a
// night is named by the date it BEGAN on: at 01:00 on Saturday, "tonight" is
// still Friday night, and Friday's listing is the one a reader who went out
// that evening is looking for. The same boundary the radio broadcast day uses,
// for the same reason.
//
// This decides only which date "tonight" POINTS AT — it does NOT widen the
// window. A dated page's contents are always a strict calendar day, matching
// the week view's day buckets and the dated permalinks, so a set starting after
// midnight belongs to the following date's page. That is the deliberate
// trade-off: a 06:00→06:00 window would list such a show under a date its own
// permalink disagrees with. Do not "fix" this by widening the window without
// revisiting that decision.
const nightStartHour = 6

// sceneDayNextShowWindowDays bounds the look-ahead behind a quiet night's
// "next on our calendar" pointer.
//
// Six weeks, because the COPY is what this number has to keep honest: a night
// with nothing ahead of it inside this window tells the reader there is nothing
// "in the next few weeks", so the window must be at least as long as those
// words claim. Erring long is safe (the page under-claims what was checked);
// erring short would put an assertion on the page the data does not support.
const sceneDayNextShowWindowDays = 42

// calendarDateLayout is the wire format for every date key on the scene-day
// surface — the same `YYYY-MM-DD` the day payload emits, so a client can feed
// a response field straight back as a request key.
const calendarDateLayout = "2006-01-02"

// sceneFirstTrackedYear is the earliest year worth serving a day for. The site
// has no shows before it.
//
// Bounded HERE and not only at the edge. This endpoint is public and directly
// reachable, so the frontend's copy of the window protects the frontend and
// nothing else: 1970..9999 is nearly three million distinct valid keys per
// scene, every one of them an empty day, and an empty day is the MOST expensive
// response this endpoint has — it is the one that pays for the six-week
// look-ahead on top of everything else. A key space that large with a
// guaranteed cache miss is a load generator with a public URL.
const sceneFirstTrackedYear = 2015

// sceneDayFutureYears is how far ahead a day may be addressed. One year, which
// is past any real listing and keeps the adjacent-day chain finite — a crawler
// following "next day →" has to stop somewhere.
const sceneDayFutureYears = 1

// calendarDate is a date with NO zone attached: the thing a URL segment names,
// the thing a page is about, and the thing a reader means by "Friday".
//
// Kept deliberately apart from time.Time. Every subtle bug this file can have
// comes from doing date arithmetic on an INSTANT: `time.Date(y, m, d, 0,0,0,0,
// loc)` silently normalizes when local midnight does not exist, and `AddDate`
// then carries the normalized clock forward. In America/Havana, midnight on
// 2026-03-08 does not exist — Go answers 2026-03-07T23:00 — so an instant-based
// day would 404 a real date, label a page with the wrong one, and hand adjacent
// days overlapping windows that list the same show twice. Doing the arithmetic
// on the date and converting to an instant only at the edges makes all three
// impossible rather than merely unlikely. No US scene can reach that zone
// today; the catalog is going worldwide.
type calendarDate struct {
	year  int
	month time.Month
	day   int
}

func (c calendarDate) String() string {
	return fmt.Sprintf("%04d-%02d-%02d", c.year, c.month, c.day)
}

// addDays walks the CALENDAR, in UTC, where every day is exactly 24 hours and
// no transition can shift the result onto a neighbouring date.
func (c calendarDate) addDays(n int) calendarDate {
	t := c.noon().AddDate(0, 0, n)
	return calendarDate{t.Year(), t.Month(), t.Day()}
}

// notBefore reports whether this date is on or after other.
func (c calendarDate) notBefore(other calendarDate) bool {
	if c.year != other.year {
		return c.year > other.year
	}
	if c.month != other.month {
		return c.month > other.month
	}
	return c.day >= other.day
}

// start is the instant this date BEGINS at in loc: the EARLIEST instant whose
// local date is this one or later.
//
// Defined that way rather than as "local midnight" because local midnight is
// not always a moment. Three cases, and the definition is the only thing that
// gets all three right at once:
//
//   - An ordinary date: local midnight, as expected.
//   - A date whose midnight is skipped by a forward jump (America/Havana
//     2026-03-08 begins at 01:00): the transition instant.
//   - A date that never happened at all — a date-line move can delete a whole
//     day, and Pacific/Apia has no 2011-12-30 — where this equals the NEXT
//     date's start, so the day's window is empty. That is exactly true: no
//     time passed on that date, so no show can have happened on it.
//
// The definition is monotonic in the date, which is what makes end(D) equal
// start(D+1) for every D in every zone — no overlap, no gap, no case analysis
// at the call site.
//
// `time.Date` alone will NOT do. It normalizes a nonexistent wall-clock in an
// unspecified direction: for Havana it lands forward on the transition (right),
// but for Apia's deleted day it lands BACK on the previous date, which collapses
// the previous day's window to nothing and hands the deleted date a 48-hour one.
// This was tried; the zone sweep in scene_day_test.go is what caught it.
//
// The search is over a ±48h bracket at second granularity — ~17 iterations, and
// zone transitions land on whole seconds — anchored on a UTC noon whose local
// date is always this date or the next, so the predicate is guaranteed true at
// the top of the bracket and false at the bottom.
func (c calendarDate) start(loc *time.Location) time.Time {
	noon := c.noon()
	// Invariant: the predicate is false at lo and true at hi. Both stay on
	// whole seconds, so the answer lands exactly on the transition rather than
	// a fraction of a second past it.
	lo, hi := noon.Add(-48*time.Hour), noon
	for hi.Sub(lo) > time.Second {
		mid := lo.Add((hi.Sub(lo) / 2).Truncate(time.Second))
		if dateOf(mid.In(loc)).notBefore(c) {
			hi = mid
		} else {
			lo = mid
		}
	}
	return hi.In(loc)
}

// noon is a zone-free instant guaranteed to fall on this date, for the calendar
// questions that must not be asked of a boundary — the ISO week this date
// belongs to, above all. Asking `start` would get the previous date's week
// whenever local midnight was skipped.
func (c calendarDate) noon() time.Time {
	return time.Date(c.year, c.month, c.day, 12, 0, 0, 0, time.UTC)
}

// dateOf reads the calendar date an instant falls on.
func dateOf(t time.Time) calendarDate {
	y, m, d := t.Date()
	return calendarDate{y, m, d}
}

// ParseCalendarDateKey parses "2026-07-31" into the calendar date it names.
//
// Validation is strict because these values arrive from URLs. Parsing happens
// in UTC — deliberately zone-free, since a key names a DATE and not an instant:
// parsing it in the scene's zone would make a real date unparseable in a zone
// where its midnight is skipped.
//
// `time.Parse` rejects a malformed key but NORMALIZES an out-of-range one —
// "2026-02-30" parses happily as March 2nd — so the round-trip check is what
// keeps one calendar day addressable by exactly one key. Without it every
// impossible date would be a second, indexable URL for a real day's content.
//
// The year bound here mirrors ParseISOWeekKey's and only rules out nonsense;
// the SERVABLE window (sceneFirstTrackedYear..now+1) is narrower and is applied
// by GetSceneDay, which has the scene's clock to apply it against.
func ParseCalendarDateKey(key string) (calendarDate, error) {
	parsed, err := time.Parse(calendarDateLayout, key)
	if err != nil {
		return calendarDate{}, fmt.Errorf("malformed date %q (want YYYY-MM-DD)", key)
	}
	if parsed.Format(calendarDateLayout) != key {
		return calendarDate{}, fmt.Errorf("date %q does not exist", key)
	}
	if parsed.Year() < 1970 || parsed.Year() > 9999 {
		return calendarDate{}, fmt.Errorf("date %q is out of range", key)
	}
	return dateOf(parsed), nil
}

// tonightDate returns the calendar date a reader standing in the scene right
// now would call "tonight".
//
// nowLocal must already be in the scene's zone; the whole point is that this
// answer belongs to the scene's clock and not the viewer's.
func tonightDate(nowLocal time.Time) calendarDate {
	date := dateOf(nowLocal)
	if nowLocal.Hour() < nightStartHour {
		date = date.addDays(-1)
	}
	return date
}

// dateIsServable reports whether a date falls inside the window this surface
// will answer for, judged against the SCENE's clock rather than the process's —
// the same authority that decides every other date here.
//
// Applied at the service and not only at the edge. The endpoint is public and
// directly reachable, so a bound that lives only in the frontend protects the
// frontend and nothing else.
func dateIsServable(date calendarDate, nowLocal time.Time) bool {
	return date.year >= sceneFirstTrackedYear &&
		date.year <= nowLocal.Year()+sceneDayFutureYears
}

// servableDateKey renders a date as a request key, or "" when this service
// would refuse it — so a client can tell "here is the adjacent day" from
// "there is no adjacent day to offer".
func servableDateKey(date calendarDate, nowLocal time.Time) string {
	if !dateIsServable(date, nowLocal) {
		return ""
	}
	return date.String()
}

// dayHasEnded reports whether a scene's day is entirely behind it — the only
// state in which a client may freeze the payload.
//
// end is the day's EXCLUSIVE end, the same instant that closes the show query's
// half-open window, so the first clause asks "has the scene's own clock reached
// the start of the following day".
//
// The second clause is what the week's equivalent does not need. For a week,
// reaching the end instant necessarily means a different week key, so "over"
// and "current" are mutually exclusive for free. The 6am night boundary breaks
// that: between midnight and 06:00 the clock is past the date's end while
// `tonight` still POINTS AT that date. Reporting it over would let a client
// cache the live night for a day — under a heading that says "Tonight", on the
// canonical URL, for the one night where being wrong is most visible.
//
// False for a future day too. That is the whole distinction the caller needs:
// only a day that can no longer gain shows, and is no longer the night people
// are out on, is safe to freeze.
func dayHasEnded(nowLocal, end time.Time, date, tonight calendarDate) bool {
	return !nowLocal.Before(end) && date != tonight
}

// trackedVenueDetails lists the scene's verified rooms alphabetically, each with
// enough to link it.
//
// Day and week both call this rather than each running its own query, so the
// two pages cannot end up naming different rooms for the same city. The
// definition itself is trackedVenuePredicate, which the detail page's rooms
// leaderboard selects on too.
func (s *SceneService) trackedVenueDetails(scope sceneScope) ([]contracts.SceneTrackedVenue, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	tp, vargs := trackedVenuePredicate(scope, "v")
	var venues []contracts.SceneTrackedVenue
	// `website` lives on the venue row itself but is projected away by the venue
	// LIST endpoints, so it has to be selected explicitly here — a client cannot
	// assemble this from an existing list response.
	if err := s.db.Raw(`
		SELECT v.name AS name,
		       COALESCE(v.slug, '') AS slug,
		       COALESCE(v.website, '') AS website
		FROM venues v
		WHERE `+tp+`
		ORDER BY v.name ASC
	`, vargs...).Scan(&venues).Error; err != nil {
		return nil, fmt.Errorf("failed to list tracked venues: %w", err)
	}
	return venues, nil
}

// GetSceneDay returns one calendar day of a scene's shows, in the scene's own
// timezone.
//
// dateKey is an ISO calendar date ("2026-07-31") or "" for the scene's current
// NIGHT. Resolving that here rather than on the client is deliberate and is
// sharper than it is for the week: the answer depends on the scene's timezone
// AND on the 6am night boundary, so a reader in Berlin looking up a Phoenix
// night must get Phoenix's answer, not their own device's.
//
// A valid scene with no shows that day returns 200 with an empty day, not 404 —
// a real city having a quiet Tuesday is a fact, and 404ing it would break an
// already-shared permalink retroactively. Only an unknown or below-threshold
// scene, or an impossible date, is a 404.
func (s *SceneService) GetSceneDay(city, state, dateKey string) (*contracts.SceneDayResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	scope := s.scopeFor(city, state)
	if n, err := s.verifiedVenueCount(scope); err != nil {
		return nil, fmt.Errorf("failed to count venues: %w", err)
	} else if n < sceneMinVenues {
		return nil, apperrors.ErrSceneNotFound(fmt.Sprintf("scene not found: %s, %s", city, state))
	}

	loc, zone := s.sceneLocation(scope, state)
	nowLocal := time.Now().In(loc)
	tonight := tonightDate(nowLocal)

	date := tonight
	if dateKey != "" {
		parsed, err := ParseCalendarDateKey(dateKey)
		if err != nil {
			return nil, apperrors.ErrSceneNotFound(err.Error())
		}
		if !dateIsServable(parsed, nowLocal) {
			return nil, apperrors.ErrSceneNotFound(fmt.Sprintf("date %q is outside the tracked window", dateKey))
		}
		date = parsed
	}
	// Half-open [start, end). Both ends are derived from the CALENDAR, so
	// end(D) is exactly start(D+1) — adjacent days can neither overlap (listing
	// one show twice, under a date its own permalink disagrees with) nor leave
	// a gap that hides one.
	start := date.start(loc)
	end := date.addDays(1).start(loc)

	shows, err := s.GetSceneShowsInRange(city, state, start.UTC(), end.UTC(), loc, sceneDayShowCap)
	if err != nil {
		return nil, err
	}
	if shows == nil {
		shows = []contracts.SceneShowSummary{}
	}

	venues, err := s.trackedVenueDetails(scope)
	if err != nil {
		return nil, err
	}
	if venues == nil {
		venues = []contracts.SceneTrackedVenue{}
	}

	isPastDay := dayHasEnded(nowLocal, end, date, tonight)

	// Only a quiet night needs somewhere to point, so only a quiet night pays
	// for the look-ahead query — and only a night that has NOT already happened.
	//
	// "Next on our calendar" is a claim about what is still to come. A page
	// about a night in 2019 cannot make it: anchored on that night it would
	// name a show six years in the past, and anchored on now it would be a
	// live value inside a payload the client is told it may freeze for a day
	// (see IsPastDay). An archived night has the week and the rooms to offer
	// and needs no pointer; a page that is still ahead of the reader gets one.
	var nextShow *contracts.SceneShowSummary
	if len(shows) == 0 && !isPastDay {
		// The lower bound is the day's end OR now, whichever is later. On the
		// live night between midnight and 06:00 those differ: "tonight" still
		// points at yesterday, so `end` is already behind us, and an unclamped
		// search would offer a late set that started an hour ago as what is
		// coming NEXT — to the reader most likely to be standing in the room.
		//
		// The upper bound stays anchored on `end`, so "or in the next few
		// weeks" keeps measuring the same six weeks all night.
		from := end
		if nowLocal.After(from) {
			from = nowLocal
		}
		upcoming, err := s.GetSceneShowsInRange(
			city, state,
			from.UTC(), end.AddDate(0, 0, sceneDayNextShowWindowDays).UTC(),
			loc, 1,
		)
		if err != nil {
			return nil, err
		}
		if len(upcoming) > 0 {
			nextShow = &upcoming[0]
		}
	}

	return &contracts.SceneDayResponse{
		// Canonical slug, not whatever the caller asked for: a metro MEMBER slug
		// (mesa-az) resolves to its principal city (phoenix-az), and the client
		// builds every adjacent-day URL from this.
		Slug:      buildSceneSlug(city, state),
		SceneName: fmt.Sprintf("%s, %s", city, state),
		City:      city,
		State:     state,
		Date:      date.String(),
		Timezone:  zone,
		// From the DATE, not from `start`: a start that is a jump boundary can
		// render as the PREVIOUS date, and at a week edge that is the previous
		// week — a "Full week" chip pointing at the wrong seven days.
		ISOWeek: ISOWeekKey(date.noon()),
		// The rows are the count, and the rows are what the page renders. Note
		// this reports the CAP when a night somehow exceeds it, exactly as the
		// week payload does.
		ShowCount: len(shows),
		// Empty at the edges of the servable window rather than pointing at a
		// date this same service would 404. A page that renders a link it knows
		// is dead is worse than one that renders no link.
		PrevDate:  servableDateKey(date.addDays(-1), nowLocal),
		NextDate:  servableDateKey(date.addDays(1), nowLocal),
		IsTonight: date == tonight,
		// The SAME value the look-ahead was gated on above, not a second call.
		// These are two halves of one decision — whether this night can still
		// change — and a client that reads a "freezable" flag from a payload
		// assembled under the opposite answer gets exactly the two failures the
		// comments above name.
		IsPastDay:     isPastDay,
		Shows:         shows,
		TrackedVenues: venues,
		NextShow:      nextShow,
	}, nil
}
