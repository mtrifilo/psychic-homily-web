package catalog

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	apperrors "psychic-homily-backend/internal/errors"
	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/utils"
)

// sceneWeekShowCap bounds one week's payload. Chicago, the densest scene,
// runs ~50 shows in a busy week, so this leaves comfortable headroom while
// still refusing to serve an unbounded list.
const sceneWeekShowCap = 300

// ISOWeekKey formats a time as its ISO-8601 week key ("2026-W31"). The ISO year
// is NOT always the calendar year — 2027-01-01 belongs to ISO week 2026-W53 —
// so the year must come from ISOWeek(), never from Year().
func ISOWeekKey(t time.Time) string {
	y, w := t.ISOWeek()
	return fmt.Sprintf("%04d-W%02d", y, w)
}

// ParseISOWeekKey parses "2026-W31" into its ISO year and week number.
//
// Validation is strict because these values arrive from URLs: the week must be
// 1..53 AND must actually exist in that year. Most years have 52 weeks, so
// "2026-W53" may be a real week or may not; the round-trip check below is what
// distinguishes them, and it is the reason this cannot be a bare regex.
func ParseISOWeekKey(key string, loc *time.Location) (year, week int, err error) {
	parts := strings.Split(strings.ToUpper(strings.TrimSpace(key)), "-W")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("malformed ISO week key %q (want YYYY-Www)", key)
	}
	year, err = strconv.Atoi(parts[0])
	if err != nil || year < 1970 || year > 9999 {
		return 0, 0, fmt.Errorf("malformed ISO week year in %q", key)
	}
	week, err = strconv.Atoi(parts[1])
	if err != nil || week < 1 || week > 53 {
		return 0, 0, fmt.Errorf("ISO week out of range in %q", key)
	}
	// Round-trip: a 53rd week in a 52-week year lands in the NEXT ISO year, so
	// the computed Monday disagrees with the requested key.
	start := ISOWeekStart(year, week, loc)
	gotY, gotW := start.ISOWeek()
	if gotY != year || gotW != week {
		return 0, 0, fmt.Errorf("ISO week %q does not exist", key)
	}
	return year, week, nil
}

// ISOWeekStart returns midnight on the Monday opening the given ISO week, in
// loc.
//
// Anchored on January 4th, which ISO-8601 guarantees falls in week 1 of its
// year. Anchoring on January 1st would be wrong: it can belong to the last week
// of the PREVIOUS ISO year.
func ISOWeekStart(year, week int, loc *time.Location) time.Time {
	if loc == nil {
		loc = time.UTC
	}
	jan4 := time.Date(year, time.January, 4, 0, 0, 0, 0, loc)
	// Go weekdays run Sunday=0..Saturday=6; ISO runs Monday=1..Sunday=7.
	weekday := int(jan4.Weekday())
	if weekday == 0 {
		weekday = 7
	}
	week1Monday := jan4.AddDate(0, 0, -(weekday - 1))
	return week1Monday.AddDate(0, 0, (week-1)*7)
}

// sceneLocation resolves the timezone a scene's week boundaries are computed
// in: the most common explicit timezone among its verified venues, falling back
// to the state map via utils.EventLocation.
//
// Modal rather than first-row so one mis-geocoded venue cannot shift the whole
// city's week. Metro scopes stay within a single zone in practice (a CBSA is
// geographically compact), so a single location per scene is well defined.
func (s *SceneService) sceneLocation(scope sceneScope, state string) *time.Location {
	if s.db == nil {
		return utils.EventLocation(nil, state)
	}
	vp, vargs := scope.venuePredicate("v")
	var tz string
	err := s.db.Raw(`
		SELECT v.timezone
		FROM venues v
		WHERE `+vp+`
		  AND v.verified = true
		  AND v.timezone IS NOT NULL
		  AND v.timezone <> ''
		GROUP BY v.timezone
		ORDER BY COUNT(*) DESC, v.timezone ASC
		LIMIT 1
	`, vargs...).Scan(&tz).Error
	if err != nil || tz == "" {
		return utils.EventLocation(nil, state)
	}
	return utils.EventLocation(&tz, state)
}

// trackedVenues lists the scene's verified venue names, alphabetically.
//
// The weekly page names these explicitly. Coverage is a curated slice — 11
// rooms in Chicago, not all of Chicago — so a page that implied it listed
// everything happening in the city would be false, and a local would notice
// immediately.
func (s *SceneService) trackedVenues(scope sceneScope) ([]string, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	vp, vargs := scope.venuePredicate("v")
	var names []string
	if err := s.db.Raw(`
		SELECT v.name
		FROM venues v
		WHERE `+vp+`
		  AND v.verified = true
		ORDER BY v.name ASC
	`, vargs...).Scan(&names).Error; err != nil {
		return nil, fmt.Errorf("failed to list tracked venues: %w", err)
	}
	return names, nil
}

// GetSceneWeek returns one ISO week of a scene's shows, grouped by day in the
// scene's own timezone.
//
// weekKey is either an ISO week key ("2026-W31") or "" for the scene's current
// week. Resolving "current" here rather than on the client is deliberate: the
// answer depends on the SCENE's timezone, not the viewer's, so a reader in
// Berlin and a reader in Chicago must get the same Chicago week.
//
// A valid scene with no shows in the window returns a populated response with
// zero shows, not an error — a real city having a quiet week is a fact, and
// 404ing it would make an already-shared permalink break retroactively. Only an
// unknown or below-threshold scene is a 404.
func (s *SceneService) GetSceneWeek(city, state, weekKey string) (*contracts.SceneWeekResponse, error) {
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
	currentKey := ISOWeekKey(nowLocal)

	if strings.TrimSpace(weekKey) == "" {
		weekKey = currentKey
	}
	year, week, err := ParseISOWeekKey(weekKey, loc)
	if err != nil {
		return nil, apperrors.ErrSceneNotFound(err.Error())
	}

	start := ISOWeekStart(year, week, loc)
	end := start.AddDate(0, 0, 7) // half-open [start, end)

	shows, err := s.GetSceneShowsInRange(city, state, start.UTC(), end.UTC(), loc, sceneWeekShowCap)
	if err != nil {
		return nil, err
	}

	// Pre-seed all seven days so quiet nights render as empty rather than
	// vanishing, then bucket by the scene-local date the query already
	// formatted.
	byDate := make(map[string][]contracts.SceneShowSummary, 7)
	days := make([]contracts.SceneWeekDay, 0, 7)
	for i := 0; i < 7; i++ {
		key := start.AddDate(0, 0, i).Format("2006-01-02")
		byDate[key] = nil
		days = append(days, contracts.SceneWeekDay{Date: key})
	}
	for _, sh := range shows {
		if _, ok := byDate[sh.EventDate]; ok {
			byDate[sh.EventDate] = append(byDate[sh.EventDate], sh)
		}
	}
	for i := range days {
		days[i].Shows = byDate[days[i].Date]
	}

	venues, err := s.trackedVenues(scope)
	if err != nil {
		return nil, err
	}

	return &contracts.SceneWeekResponse{
		SceneName:     fmt.Sprintf("%s, %s", city, state),
		City:          city,
		State:         state,
		ISOWeek:       ISOWeekKey(start),
		StartDate:     start.Format("2006-01-02"),
		EndDate:       start.AddDate(0, 0, 6).Format("2006-01-02"),
		Timezone:      loc.String(),
		ShowCount:     len(shows),
		PrevWeek:      ISOWeekKey(start.AddDate(0, 0, -7)),
		NextWeek:      ISOWeekKey(start.AddDate(0, 0, 7)),
		IsCurrentWeek: ISOWeekKey(start) == currentKey,
		Days:          days,
		TrackedVenues: venues,
	}, nil
}
