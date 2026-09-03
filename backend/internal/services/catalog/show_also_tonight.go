package catalog

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	apperrors "psychic-homily-backend/internal/errors"
	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/services/shared"
)

// showAlsoTonightCap bounds the "also tonight" rail. A rail is a glance, not a
// listing: twenty rows is already more than any night in the densest metro puts
// in front of a reader before they should be following the "see all" link to
// the scene-day page instead.
const showAlsoTonightCap = 20

// GetShowAlsoTonight returns the other shows in this show's metro on this show's
// own scene-local date.
//
// Four decisions worth stating, because each has a plausible-looking wrong
// version:
//
//   - The date is the SHOW's, never the viewer's "tonight" and never UTC. A
//     reader in Berlin opening a Chicago show page is asking what else is on that
//     night in Chicago, and a 21:00 Chicago set is already the next UTC day.
//   - The clock is the ROOM's own zone when it has one, and the metro's modal
//     clock otherwise (see alsoTonightClock for why that order and not the
//     reverse).
//   - The window is a strict calendar day, [start, end), taken from the same
//     calendarDate arithmetic the scene-day page uses — not the 6am night
//     boundary that names a NIGHT. `Date` is a scene-day key, so the two surfaces
//     must bucket a 01:00 set onto the same date. `IsTonight` carries the 6am
//     rule separately, so a client never has to reimplement it.
//   - The rows come from sceneShowsInRange, the same query GetSceneShowsInRange
//     serves, so "the metro's shows on a date" has ONE definition shared with the
//     scene-day page and the digest.
//
// A show whose venue cannot be scoped to a scene returns an empty rail at 200,
// not 404: the show itself is real, and a page that exists must not be broken by
// a rail that has nothing to say. Only an unknown show is a 404 — as is a
// non-approved one, which this anonymous surface must not distinguish from a
// show that does not exist (the same rule the public per-show ICS export
// applies, for the same reason).
func (s *SceneService) GetShowAlsoTonight(idOrSlug string) (*contracts.ShowAlsoTonightResponse, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database not initialized")
	}

	subject, err := s.resolveAlsoTonightSubject(idOrSlug)
	if err != nil {
		return nil, err
	}

	// No venue, or a venue with no usable place: nothing to scope by, so there is
	// no scene clock to date the show on and the room's own zone answers instead.
	//
	// TrimSpace, not `== ""`, so this agrees with the TRIM() the venue pick in
	// this same function uses. A whitespace-only city otherwise slips through to
	// scopeFor, where it folds to the empty key and produces a scope matching
	// EVERY blank-city venue in the state, plus a "---il" slug that resolves to
	// nothing.
	if strings.TrimSpace(subject.VenueCity) == "" || strings.TrimSpace(subject.VenueState) == "" {
		return emptyAlsoTonight(subject), nil
	}

	scope := s.scopeFor(subject.VenueCity, subject.VenueState)
	// The metro's principal city, not the venue's: a Mesa show belongs to the
	// Phoenix scene, and the slug emitted from this must be one /scenes/{slug}
	// answers. Passing it back into GetSceneShowsInRange re-resolves the SAME
	// scope, which is how every other scene caller addresses a metro.
	city, state := metroDisplayIdentity(scope.metro, scope.city, scope.state)
	loc, zone := s.alsoTonightClock(subject, scope, state)

	// Half-open [start, end), both ends from the CALENDAR — see calendarDate.start
	// for why this is not `time.Date(..., 0, 0, 0, 0, loc)`.
	date := dateOf(subject.EventDate.In(loc))
	start := date.start(loc)
	end := date.addDays(1).start(loc)

	// ONE clock read for the whole answer. The night's order, the link's
	// servability and IsTonight are three readings of the same instant, and a
	// second time.Now() could split them across a 6am or midnight boundary.
	nowLocal := time.Now().In(loc)
	isTonight := date == tonightDate(nowLocal)

	// On the LIVE night the rows a reader can still get to outrank the ones already
	// playing, and that ordering belongs to the QUERY because the cap is applied
	// there: a night longer than the rail would otherwise be truncated in clock
	// order, dropping exactly the late sets the promotion exists to surface. The
	// client applies the same rule to what it receives, so an ordering computed
	// here and a page hydrating minutes later agree.
	//
	// Only the live night. An archive or future night has no started rows to
	// separate from upcoming ones and stays in clock order.
	var sinkStartedAt *time.Time
	if isTonight {
		sinkStartedAt = &nowLocal
	}

	// TWO over the cap, and both are load-bearing. One pays for the subject, which
	// is itself in the metro's night and about to be dropped: fetching exactly the
	// cap would quietly serve nineteen rows on a night that has twenty others. The
	// second pays for HasMore, which otherwise cannot tell "exactly the cap" from
	// "the cap and then some" — with only cap+1 fetched, a subject inside the
	// window consumes the spare row and every full rail looks complete.
	rows, err := s.sceneShowsInRange(city, state, start.UTC(), end.UTC(), loc, showAlsoTonightCap+2, sinkStartedAt)
	if err != nil {
		// Below the scene threshold is not a failure here. The rail's question is
		// "what else is on tonight", and "this room is not part of a scene we
		// track" answers it with nothing — a 404 would break a show page that is
		// otherwise perfectly serveable.
		var sceneErr *apperrors.SceneError
		if errors.As(err, &sceneErr) && sceneErr.Code == apperrors.CodeSceneNotFound {
			return emptyAlsoTonight(subject), nil
		}
		return nil, err
	}

	shows := make([]contracts.SceneShowSummary, 0, len(rows))
	// A subject among the rows is already proof the scene lists it: these rows came
	// from the scene's own scope. Absence proves nothing, because the fetch is
	// capped and ordered, so that branch has to ask (see below).
	subjectOnTheRail := false
	for _, row := range rows {
		if row.ID == subject.ShowID {
			subjectOnTheRail = true
			continue
		}
		shows = append(shows, row)
	}
	// More rows than the cap survived the self-exclusion, so the night has more
	// than the rail can hold and the client needs to say so rather than implying
	// it listed everything.
	hasMore := len(shows) > showAlsoTonightCap
	if hasMore {
		shows = shows[:showAlsoTonightCap]
	}

	// The slug is a LINK, so it is served only when following it lands somewhere
	// honest. Two ways it would not:
	//
	//   - The scene-day surface refuses dates outside its tracked window, so an
	//     archive show has a real scene and a real date but no page to point at.
	//   - The subject's scope comes from the GEOCODER while the rows come from the
	//     venues.metro COLUMN, which a manually-run backfill maintains. When the
	//     subject's own room is missing from the column, the scene-day page for
	//     this date provably does not list the show the reader came from, and
	//     sending them there is worse than sending them nowhere.
	//
	// The membership half cannot be read off the rows alone: they are capped and
	// ordered, so a subject the scope does include can still be absent from them.
	// Only that branch pays for a query, and a date with no page to point at pays
	// for nothing.
	//
	// Display identity below stays unconditional, because naming the metro is
	// true either way. Same split as SceneDayResponse.PrevDate/NextDate.
	sceneSlug := ""
	if dateIsServable(date, nowLocal) {
		listed := subjectOnTheRail
		if !listed {
			listed, err = s.sceneScopeIncludesShow(scope, subject.ShowID)
			if err != nil {
				return nil, err
			}
		}
		if listed {
			sceneSlug = buildSceneSlug(city, state)
		}
	}

	return &contracts.ShowAlsoTonightResponse{
		SceneSlug: sceneSlug,
		SceneName: fmt.Sprintf("%s, %s", city, state),
		City:      city,
		State:     state,
		Date:      date.String(),
		Timezone:  zone,
		IsTonight: isTonight,
		ShowCount: len(shows),
		HasMore:   hasMore,
		Shows:     shows,
	}, nil
}

// sceneScopeIncludesShow answers whether one show sits in the metro's scope: the
// venue predicate and approved-status filter GetSceneShowsInRange applies, asked
// about a single row. It carries no date term and no clock.
//
// That is NARROWER than "the scene-day page for this date lists this show", and
// deliberately so. This rail dates a show on the room's own zone while the
// scene-day page dates it on the metro's modal clock (alsoTonightClock states
// why), so the two can file one show under different dates. What this answers is
// the failure the venues.metro backfill actually produces: a room the scene
// cannot see at all.
//
// EXISTS rather than a count: a show billed at several of the metro's rooms
// matches once per room, and only whether it matches at all is ever asked.
func (s *SceneService) sceneScopeIncludesShow(scope sceneScope, showID uint) (bool, error) {
	vp, vargs := scope.venuePredicate("v")
	args := append(append([]any{}, vargs...), showID, catalogm.ShowStatusApproved)

	var listed bool
	if err := s.db.Raw(`
		SELECT EXISTS (
			SELECT 1
			FROM shows s
			JOIN show_venues sv ON sv.show_id = s.id
			JOIN venues v ON v.id = sv.venue_id
			WHERE `+vp+`
			  AND s.id = ?
			  AND s.status = ?
		)
	`, args...).Scan(&listed).Error; err != nil {
		return false, fmt.Errorf("failed to check scene membership: %w", err)
	}
	return listed, nil
}

// alsoTonightClock is the clock this show's date is read on, plus the zone name
// the rail may publish: the room's own zone when it has one, otherwise the
// metro's modal clock.
//
// That order, and not the reverse, because the two answer different questions.
// sceneLocation is the right clock for a page the READER dated — it makes every
// room in the metro agree on which night a date names. Here the DATE is derived
// from one specific show, so a modal answer can misdate the very show the reader
// is looking at: sceneLocation reads verified venues only and otherwise falls
// back to a one-zone-per-state map, so a Pensacola room (America/Chicago, in a
// state the map calls America/New_York) would have its 23:30 set published under
// the following date, with a rail listing the following night.
//
// The room's own column cannot be wrong about the room. Where the two disagree
// the metro straddles a zone boundary, and the show's own night is the thing
// worth getting right.
func (s *SceneService) alsoTonightClock(subject *alsoTonightSubject, scope sceneScope, state string) (*time.Location, *string) {
	if strings.TrimSpace(subject.VenueTimezone) != "" {
		return subject.venueZone()
	}
	return s.sceneLocation(scope, state)
}

// alsoTonightSubject is the subject show reduced to what the rail needs: its
// identity, its instant, and the place that instant belongs to.
type alsoTonightSubject struct {
	ShowID        uint      `gorm:"column:show_id"`
	EventDate     time.Time `gorm:"column:event_date"`
	VenueCity     string    `gorm:"column:venue_city"`
	VenueState    string    `gorm:"column:venue_state"`
	VenueTimezone string    `gorm:"column:venue_timezone"`
}

// venueZone is the room's own zone, for the branches with no scene clock to
// use. Precedence matches the shared resolution everywhere else: the venue's
// explicit IANA zone, then the US state map, then its Arizona default.
//
// The column has to be read rather than deriving everything from the state,
// because GetTimezoneForState answers America/Phoenix for every unknown or blank
// state. Without it a 01:00 Berlin set would be published under the previous
// date by the one field whose entire job is to name the right one.
//
// The second return is nil when the precedence reached that Arizona default: the
// date is still read on it, and the rail publishes no zone for it.
func (a *alsoTonightSubject) venueZone() (*time.Location, *string) {
	// One call, not a branch on the empty string: EventZone already treats an
	// empty zone as absent and falls through to the state map.
	return shared.EventZone(&a.VenueTimezone, a.VenueState)
}

// resolveAlsoTonightSubject loads the subject show by numeric id or slug.
//
// The status filter is part of the WHERE clause rather than a check on the
// loaded row so that a hidden show and a nonexistent one are indistinguishable
// from the outside — this endpoint is anonymous, so a distinguishable answer
// would confirm that an unpublished show id is real.
//
// The venue join is LEFT so a bill with no venue still resolves (an empty rail,
// not a 404).
//
// The pick is name-ASC to match GetSceneShowsInRange's `DISTINCT ON (s.id) …
// ORDER BY s.id, v.name ASC, v.id ASC`, so a multi-room show is scoped by the
// same room the scene query would name — otherwise the rail could be computed
// for one venue's metro while every other surface attributes the show to
// another.
//
// Note this is a THIRD venue-pick rule in the package, and the one it does NOT
// follow is deliberate: primaryVenueLateralSQL (charts_service.go) picks the
// lowest venue_id for venue ATTRIBUTION. That rule is right for "which room owns
// this show"; this one has to match the SCENE query's pick instead, because the
// two are read together. A bill spanning two metros is therefore scoped by an
// alphabetical tie-break, which ShowAlsoTonightResponse documents so a client is
// not surprised when renaming a room moves the rail.
//
// It is name-ASC *among placeable rooms*, though, and that qualifier is
// load-bearing: the scene query applies its venue predicate in the WHERE, so its
// DISTINCT ON only ever ranks rooms already IN the scope, while this query ranks
// every billed room. venues.city/state are nullable, so an unplaceable room
// ("A Secret Location", no city) would otherwise win the alphabetical pick and
// hand a perfectly scopeable show an empty rail. Demoting those rows restores
// the agreement without giving up determinism.
func (s *SceneService) resolveAlsoTonightSubject(idOrSlug string) (*alsoTonightSubject, error) {
	// The only interpolated fragment is one of two COMPILE-TIME literals below;
	// the caller's value is always a bind arg. Split this way rather than as a
	// `s.id = ? OR s.slug = ?` single query because the two lookups must not be
	// able to cross-match — a purely numeric slug has to keep meaning the id, as
	// it does on every other /shows/{show_id} route.
	const (
		selectSubject = `
		SELECT s.id AS show_id,
		       s.event_date,
		       COALESCE(v.city, '') AS venue_city,
		       COALESCE(v.state, '') AS venue_state,
		       COALESCE(v.timezone, '') AS venue_timezone
		FROM shows s
		LEFT JOIN show_venues sv ON sv.show_id = s.id
		LEFT JOIN venues v ON v.id = sv.venue_id
		WHERE `
		andApproved = `
		  AND s.status = ?
		ORDER BY (v.city IS NULL OR TRIM(v.city) = '' OR v.state IS NULL OR TRIM(v.state) = '') ASC,
		         v.name ASC, v.id ASC
		LIMIT 1
	`
		byID   = "s.id = ?"
		bySlug = "s.slug = ?"
	)

	var subject alsoTonightSubject
	var err error
	if id, parseErr := strconv.ParseUint(idOrSlug, 10, 32); parseErr == nil {
		err = s.db.Raw(selectSubject+byID+andApproved, uint(id), catalogm.ShowStatusApproved).Scan(&subject).Error
	} else {
		err = s.db.Raw(selectSubject+bySlug+andApproved, idOrSlug, catalogm.ShowStatusApproved).Scan(&subject).Error
	}
	if err != nil {
		return nil, fmt.Errorf("failed to resolve show: %w", err)
	}
	if subject.ShowID == 0 {
		return nil, apperrors.ErrShowNotFound(0)
	}
	return &subject, nil
}

// emptyAlsoTonight is the answer for a show with no scene to look at: the date,
// in the best zone available, and nothing else. Scene identity is deliberately
// left blank rather than filled with the venue's raw city, so a client cannot
// render a "see all" link to a scene page that would 404.
func emptyAlsoTonight(subject *alsoTonightSubject) *contracts.ShowAlsoTonightResponse {
	loc, zone := subject.venueZone()
	return &contracts.ShowAlsoTonightResponse{
		Date:     dateOf(subject.EventDate.In(loc)).String(),
		Timezone: zone,
		Shows:    []contracts.SceneShowSummary{},
	}
}
