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

// ParseCalendarDateKey parses "2026-07-31" into midnight of that day in loc.
//
// Validation is strict because these values arrive from URLs. time.ParseInLocation
// already rejects a malformed key, but it also NORMALIZES an out-of-range one —
// "2026-02-30" parses happily as March 2nd — so the round-trip check is what
// keeps one calendar day addressable by exactly one key. Without it every
// impossible date would be a second, indexable URL for a real day's content.
func ParseCalendarDateKey(key string, loc *time.Location) (time.Time, error) {
	if loc == nil {
		loc = time.UTC
	}
	parsed, err := time.ParseInLocation(calendarDateLayout, key, loc)
	if err != nil {
		return time.Time{}, fmt.Errorf("malformed date %q (want YYYY-MM-DD)", key)
	}
	if parsed.Format(calendarDateLayout) != key {
		return time.Time{}, fmt.Errorf("date %q does not exist", key)
	}
	return parsed, nil
}

// tonightDate returns the calendar date a reader standing in the scene right
// now would call "tonight", as midnight of that date in nowLocal's location.
//
// nowLocal must already be in the scene's zone; the whole point is that this
// answer belongs to the scene's clock and not the viewer's.
func tonightDate(nowLocal time.Time) time.Time {
	y, m, d := nowLocal.Date()
	date := time.Date(y, m, d, 0, 0, 0, 0, nowLocal.Location())
	if nowLocal.Hour() < nightStartHour {
		date = date.AddDate(0, 0, -1)
	}
	return date
}

// trackedVenueDetails lists the scene's verified rooms alphabetically, each with
// enough to link it.
//
// Separate from trackedVenues (which returns bare names for the week payload)
// rather than a widening of it: the week's share card and footer read a plain
// string list, and changing that shape to serve one new page would churn every
// consumer of a payload that does not need the extra fields.
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

	start := tonight
	if dateKey != "" {
		parsed, err := ParseCalendarDateKey(dateKey, loc)
		if err != nil {
			return nil, apperrors.ErrSceneNotFound(err.Error())
		}
		start = parsed
	}
	// Walked forward by a calendar day rather than by 24 hours, so a day
	// containing a DST transition still ends at local midnight.
	end := start.AddDate(0, 0, 1) // half-open [start, end)

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
		upcoming, err := s.GetSceneShowsInRange(
			city, state,
			end.UTC(), end.AddDate(0, 0, sceneDayNextShowWindowDays).UTC(),
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
		Slug:          buildSceneSlug(city, state),
		SceneName:     fmt.Sprintf("%s, %s", city, state),
		City:          city,
		State:         state,
		Date:          start.Format(calendarDateLayout),
		Timezone:      loc.String(),
		ISOWeek:       ISOWeekKey(start),
		ShowCount:     len(shows),
		PrevDate:      start.AddDate(0, 0, -1).Format(calendarDateLayout),
		NextDate:      end.Format(calendarDateLayout),
		IsTonight:     start.Equal(tonight),
		IsPastDay:     !nowLocal.Before(end),
		Shows:         shows,
		TrackedVenues: venues,
		NextShow:      nextShow,
	}, nil
}
