package catalog

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	apperrors "psychic-homily-backend/internal/errors"
	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/services/shared"
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

// sceneCalendarWeekWindow returns the half-open [start, end) instants bounding
// the Monday-Sunday week that CONTAINS now, in loc.
//
// The same two instants GetSceneWeek derives for its current week, reached
// through the same ISOWeekStart: the calendar maths lives in exactly one place,
// so a list count and the page it links to cannot drift onto different Mondays.
// end is walked forward by seven CALENDAR days rather than 168 hours, so a week
// containing a DST transition still closes at local midnight.
func sceneCalendarWeekWindow(now time.Time, loc *time.Location) (start, end time.Time) {
	if loc == nil {
		loc = time.UTC
	}
	year, week := now.In(loc).ISOWeek()
	start = ISOWeekStart(year, week, loc)
	return start, start.AddDate(0, 0, 7)
}

// sceneTimezonesByKey resolves every scene's week-boundary timezone in ONE
// query, keyed the way ListScenes groups venues.
//
// The modal rule is sceneLocation's, restated over all scenes at once rather
// than per scene: most common explicit timezone among the scene's VERIFIED
// venues, ties broken alphabetically (the ROW_NUMBER ordering reproduces that
// query's ORDER BY, over the same venue rows).
//
// SCOPING IS NOT IDENTICAL TO sceneScope.venuePredicate, and the difference is
// worth knowing before trusting an equality here:
//   - METRO scene: exact. Its key IS the CBSA, so COALESCE(v.metro, …) = key
//     selects the same rows as `v.metro = ?`. CBSA codes are numeric and
//     fallback keys always contain '|', so the two key spaces cannot collide.
//   - FALLBACK (no-CBSA) scene: a SUBSET. This key requires v.metro IS NULL;
//     venuePredicate matches the (city, state) whether or not the venue has a
//     metro. They agree only while every venue in that place agrees about its
//     metro — which is what the geocoder writes, but venues.metro DRIFTS (see
//     metro_backfill.go: the background location writers move city/state
//     without touching metro). A place holding both NULL-metro and CBSA venues
//     already splits into two ListScenes groups sharing one slug; that is
//     pre-existing, and it is the case where this count can undercount the
//     week page.
//
// Scenes missing from the returned map have no verified venue carrying a
// timezone; the caller falls back to the state map exactly as sceneLocation
// does.
func (s *SceneService) sceneTimezonesByKey() (map[string]string, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	type tzRow struct {
		SceneKey string `gorm:"column:scene_key"`
		Timezone string `gorm:"column:timezone"`
	}
	var rows []tzRow
	// The window function ranks each scene's timezones by venue count; the
	// outer WHERE keeps the winner. A NULL key (a venue with no metro and no
	// usable city/state) can match no scene, so it is dropped here rather than
	// scanning into an empty-string key that looks like a real one.
	err := s.db.Raw(`
		SELECT scene_key, timezone FROM (
			SELECT ` + sceneGroupKeySQL + ` AS scene_key,
			       v.timezone AS timezone,
			       ROW_NUMBER() OVER (
			           PARTITION BY ` + sceneGroupKeySQL + `
			           ORDER BY COUNT(*) DESC, v.timezone ASC
			       ) AS rn
			FROM venues v
			WHERE v.verified = true
			  AND v.timezone IS NOT NULL
			  AND v.timezone <> ''
			GROUP BY ` + sceneGroupKeySQL + `, v.timezone
		) ranked
		WHERE rn = 1 AND scene_key IS NOT NULL
	`).Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("failed to resolve scene timezones: %w", err)
	}
	out := make(map[string]string, len(rows))
	for _, r := range rows {
		out[r.SceneKey] = r.Timezone
	}
	return out, nil
}

// sceneCalendarWeekTarget is one scene to count a calendar week for: the key
// ListScenes grouped it under, plus the display state that backs the timezone
// fallback.
type sceneCalendarWeekTarget struct {
	key   string
	state string
}

// sceneCalendarWeekCounts returns, per scene key, how many shows that scene's
// /scenes/{slug}/week page reports for the week containing now.
//
// Two queries for the whole list, not two per scene: one resolves every
// scene's timezone, one counts every scene's own week. The count query joins a
// VALUES list of per-scene windows, which is what lets a single statement apply
// a DIFFERENT Monday to Chicago than to Honolulu.
//
// The predicate is GetSceneShowsInRange's, deliberately down to its omissions:
// approved shows at ANY venue in scope, verified or not, cancelled ones
// included. This number's only job is to equal the destination page's total, so
// it has to be counted the way that page counts, not the way that page ideally
// would. Capped at sceneWeekShowCap for the same reason — the page's total is
// the length of a capped list, so an uncapped count would overstate it for a
// scene busy enough to hit the ceiling.
//
// CONSEQUENCE, because it will look like a bug from the outside: this counts a
// DIFFERENT venue population from every other number on SceneListResponse. The
// grouped ListScenes query applies sceneVenueEligibilitySQL (verified venues
// with a usable city/state), so venue_count, total_show_count,
// upcoming_show_count and shows_this_week are verified-only. A scene with
// unverified rooms can therefore read "3 shows · 17 shows this week". Adding a
// verified filter here would make the card self-consistent and make the LINK
// wrong, which is the trade this field exists to refuse. Fix it, if it is worth
// fixing, by narrowing the week PAGE — then this follows for free.
//
// Grouping caveat: see sceneTimezonesByKey. The metro branch is exact; a
// fallback scene whose place holds both NULL-metro and CBSA venues can
// undercount.
func (s *SceneService) sceneCalendarWeekCounts(now time.Time, targets []sceneCalendarWeekTarget) (map[string]int, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}
	if len(targets) == 0 {
		return map[string]int{}, nil
	}

	timezones, err := s.sceneTimezonesByKey()
	if err != nil {
		return nil, err
	}

	values := make([]string, 0, len(targets))
	args := make([]any, 0, len(targets)*3+1)
	for _, t := range targets {
		var loc *time.Location
		if tz, ok := timezones[t.key]; ok && tz != "" {
			loc = utils.EventLocation(&tz, t.state)
		} else {
			loc = utils.EventLocation(nil, t.state)
		}
		start, end := sceneCalendarWeekWindow(now, loc)
		// Casts on every row, not just the first: without them Postgres types
		// the VALUES columns as `unknown` and the join comparison fails.
		values = append(values, "(?::text, ?::timestamptz, ?::timestamptz)")
		args = append(args, t.key, start.UTC(), end.UTC())
	}
	args = append(args, catalogm.ShowStatusApproved)

	type countRow struct {
		SceneKey string `gorm:"column:scene_key"`
		Count    int    `gorm:"column:count"`
	}
	var rows []countRow
	if err := s.db.Raw(`
		SELECT w.scene_key AS scene_key,
		       LEAST(COUNT(DISTINCT s.id), `+strconv.Itoa(sceneWeekShowCap)+`) AS count
		FROM (VALUES `+strings.Join(values, ", ")+`) AS w(scene_key, wk_start, wk_end)
		JOIN venues v ON `+sceneGroupKeySQL+` = w.scene_key
		JOIN show_venues sv ON sv.venue_id = v.id
		JOIN shows s ON s.id = sv.show_id
		WHERE s.status = ?
		  AND s.event_date >= w.wk_start
		  AND s.event_date < w.wk_end
		GROUP BY w.scene_key
	`, args...).Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("failed to count scene calendar weeks: %w", err)
	}

	out := make(map[string]int, len(rows))
	for _, r := range rows {
		out[r.SceneKey] = r.Count
	}
	return out, nil
}

// weekHasEnded reports whether a scene's week is entirely behind it.
//
// end is the week's EXCLUSIVE end — the same instant that closes the show
// query's half-open window — so this asks "has the scene's own clock reached
// the following Monday". Both arguments are absolute instants, which is what
// keeps it right across a DST transition: end was walked forward by calendar
// days rather than by 168 hours, so a week containing a transition still ends
// at local midnight rather than an hour either side of it.
//
// False for the current week and for every future week alike. That is the whole
// distinction the caller needs and the reason this is not simply
// !IsCurrentWeek: only a week that can no longer gain shows is safe to freeze.
func weekHasEnded(now, end time.Time) bool {
	return !now.Before(end)
}

// sceneLocation resolves the timezone a scene's week boundaries are computed
// in: the most common explicit timezone among its verified venues, falling back
// to the state map, both resolved through shared.EventZone.
//
// Modal rather than first-row so one mis-geocoded venue cannot shift the whole
// city's week. Metro scopes stay within a single zone in practice (a CBSA is
// geographically compact), so a single location per scene is well defined.
//
// The second return is the publishable zone NAME, nil when neither the modal
// venue zone nor the state map answered and the location is the Arizona
// default. Both are returned together so a caller cannot compute the boundaries
// on one input and publish a zone derived from another.
func (s *SceneService) sceneLocation(scope sceneScope, state string) (*time.Location, *string) {
	if s.db == nil {
		return shared.EventZone(nil, state)
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
		return shared.EventZone(nil, state)
	}
	return shared.EventZone(&tz, state)
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

	scope, err := s.scopeFor(city, state)
	if err != nil {
		return nil, err
	}
	if n, err := s.verifiedVenueCount(scope); err != nil {
		return nil, fmt.Errorf("failed to count venues: %w", err)
	} else if n < sceneMinVenues {
		return nil, apperrors.ErrSceneNotFound(fmt.Sprintf("scene not found: %s, %s", city, state))
	}

	loc, zone := s.sceneLocation(scope, state)
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
	// Seeded with empty (non-nil) slices so a quiet day marshals as `[]` rather
	// than `null`. A nil slice would force every client to handle two shapes
	// for "no shows", and a TS type declaring `shows: Show[]` would silently
	// receive null.
	byDate := make(map[string][]contracts.SceneShowSummary, 7)
	days := make([]contracts.SceneWeekDay, 0, 7)
	for i := 0; i < 7; i++ {
		key := start.AddDate(0, 0, i).Format("2006-01-02")
		byDate[key] = []contracts.SceneShowSummary{}
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

	venues, err := s.trackedVenueDetails(scope)
	if err != nil {
		return nil, err
	}
	if venues == nil {
		venues = []contracts.SceneTrackedVenue{}
	}

	return &contracts.SceneWeekResponse{
		// Canonical slug, not whatever the caller asked for: a metro MEMBER
		// slug (mesa-az) resolves to its principal city (phoenix-az), and the
		// client builds prev/next week URLs from this.
		Slug:          buildSceneSlug(city, state),
		SceneName:     fmt.Sprintf("%s, %s", city, state),
		City:          city,
		State:         state,
		ISOWeek:       ISOWeekKey(start),
		StartDate:     start.Format("2006-01-02"),
		EndDate:       start.AddDate(0, 0, 6).Format("2006-01-02"),
		Timezone:      zone,
		ShowCount:     len(shows),
		PrevWeek:      ISOWeekKey(start.AddDate(0, 0, -7)),
		NextWeek:      ISOWeekKey(start.AddDate(0, 0, 7)),
		IsCurrentWeek: ISOWeekKey(start) == currentKey,
		IsPastWeek:    weekHasEnded(nowLocal, end),
		Days:          days,
		TrackedVenues: venues,
	}, nil
}
