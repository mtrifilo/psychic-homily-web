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
// Until 06:00 the current night is still the PREVIOUS calendar date: a 01:00
// show is colloquially part of the night before, and it is the show a reader
// checking their phone at the venue is standing at. The same boundary the radio
// broadcast day uses, for the same reason.
//
// This decides only which date "tonight" POINTS AT. A dated page's contents are
// always a strict calendar day, matching the week view's day buckets and the
// dated permalinks — a day that meant 06:00→06:00 would list a show under a
// date the show's own permalink disagrees with.
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
	t := time.Date(c.year, c.month, c.day, 12, 0, 0, 0, time.UTC).AddDate(0, 0, n)
	return calendarDate{t.Year(), t.Month(), t.Day()}
}

// start is the first instant of this date in loc.
//
// Midnight, except where a DST jump means this date has no midnight — then the
// first instant that does exist. The check is on the DATE that came back, not
// on the clock: normalization moves the instant onto the previous date, which
// is precisely the confusion being ruled out.
func (c calendarDate) start(loc *time.Location) time.Time {
	t := time.Date(c.year, c.month, c.day, 0, 0, 0, 0, loc)
	if y, m, d := t.Date(); y != c.year || m != c.month || d != c.day {
		t = time.Date(c.year, c.month, c.day, 1, 0, 0, 0, loc)
	}
	return t
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
// The year bound mirrors ParseISOWeekKey's. The endpoint is public, and every
// out-of-range date is an empty day that still pays for a look-ahead query.
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
// The ONE definition of "a room this scene tracks": trackedVenues projects the
// names out of this for the week payload rather than running its own query, so
// the weekly and nightly pages cannot end up naming different rooms for the
// same city. The two PAYLOADS still differ in shape — the week sends bare
// strings — because that is a wire-format decision, not a definition.
func (s *SceneService) trackedVenueDetails(scope sceneScope) ([]contracts.SceneTrackedVenue, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	vp, vargs := scope.venuePredicate("v")
	var venues []contracts.SceneTrackedVenue
	// `website` lives on the venue row itself but is projected away by the venue
	// LIST endpoints, so it has to be selected explicitly here — a client cannot
	// assemble this from an existing list response.
	if err := s.db.Raw(`
		SELECT v.name AS name,
		       COALESCE(v.slug, '') AS slug,
		       COALESCE(v.website, '') AS website
		FROM venues v
		WHERE `+vp+`
		  AND v.verified = true
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

	loc := s.sceneLocation(scope, state)
	nowLocal := time.Now().In(loc)
	tonight := tonightDate(nowLocal)

	date := tonight
	if dateKey != "" {
		parsed, err := ParseCalendarDateKey(dateKey)
		if err != nil {
			return nil, apperrors.ErrSceneNotFound(err.Error())
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

	// Only a quiet night needs somewhere to point, so only a quiet night pays
	// for the look-ahead query.
	var nextShow *contracts.SceneShowSummary
	if len(shows) == 0 {
		// Anchored at NOW when the day is already behind us. "Next on our
		// calendar" is a claim about what is still to come, so an archived
		// permalink must not answer it with a show from the week after the
		// night it describes — which for a 2019 page is six years in the past.
		// It is also what makes the dead-quiet copy's "or in the next few
		// weeks" true: that sentence is about now, so the window it rests on
		// has to be too.
		from := end
		if from.Before(nowLocal) {
			from = nowLocal
		}
		upcoming, err := s.GetSceneShowsInRange(
			city, state,
			from.UTC(), from.AddDate(0, 0, sceneDayNextShowWindowDays).UTC(),
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
		Timezone:  loc.String(),
		ISOWeek:   ISOWeekKey(start),
		// The rows are the count, and the rows are what the page renders. Note
		// this reports the CAP when a night somehow exceeds it, exactly as the
		// week payload does.
		ShowCount:     len(shows),
		PrevDate:      date.addDays(-1).String(),
		NextDate:      date.addDays(1).String(),
		IsTonight:     date == tonight,
		IsPastDay:     dayHasEnded(nowLocal, end, date, tonight),
		Shows:         shows,
		TrackedVenues: venues,
		NextShow:      nextShow,
	}, nil
}
