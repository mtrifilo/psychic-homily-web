package catalog

import (
	"errors"
	"fmt"
	"strconv"
	"time"

	apperrors "psychic-homily-backend/internal/errors"
	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/utils"
)

// showAlsoTonightCap bounds the "also tonight" rail. A rail is a glance, not a
// listing: twenty rows is already more than any night in the densest metro puts
// in front of a reader before they should be following the "see all" link to
// the scene-day page instead.
const showAlsoTonightCap = 20

// GetShowAlsoTonight returns the other shows in this show's metro on this show's
// own venue-local date.
//
// Three decisions worth stating, because each has a plausible-looking wrong
// version:
//
//   - The date is the SHOW's, resolved in the SCENE's zone — never the viewer's
//     "tonight" and never UTC. A reader in Berlin opening a Chicago show page is
//     asking what else is on that night in Chicago, and a 21:00 Chicago set is
//     already the next UTC day. The scene's clock rather than the room's own
//     because `Date` is a scene-day key: a metro straddling a zone boundary must
//     bucket its night the way /scenes/{slug}/day does, or the two disagree.
//     Only the no-scene branch, which has no scene clock to consult, falls back
//     to the venue's own zone.
//   - The window is a strict calendar day, [start, end), taken from the same
//     calendarDate arithmetic the scene-day page uses — not the 6am night
//     boundary. `Date` is emitted as the key for /scenes/{slug}/day/{date}, so
//     the two surfaces MUST bucket a 01:00 set onto the same date or the rail's
//     own "see all" link would lead to a page missing the shows it advertised.
//   - The scope comes from GetSceneShowsInRange, so "the metro's shows tonight"
//     has ONE definition shared with the scene-day page and the digest.
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
	// no scene clock to date the show on and the room's own zone answers instead
	// (see venueLocation).
	if subject.VenueCity == "" || subject.VenueState == "" {
		return emptyAlsoTonight(subject, subject.venueLocation()), nil
	}

	scope := s.scopeFor(subject.VenueCity, subject.VenueState)
	// The metro's principal city, not the venue's: a Mesa show belongs to the
	// Phoenix scene, and the slug emitted from this must be one /scenes/{slug}
	// answers. Passing it back into GetSceneShowsInRange re-resolves the SAME
	// scope, which is how every other scene caller addresses a metro.
	city, state := metroDisplayIdentity(scope.metro, scope.city, scope.state)
	loc := s.sceneLocation(scope, state)

	// Half-open [start, end), both ends from the CALENDAR — see calendarDate.start
	// for why this is not `time.Date(..., 0, 0, 0, 0, loc)`.
	date := dateOf(subject.EventDate.In(loc))
	start := date.start(loc)
	end := date.addDays(1).start(loc)

	// One over the cap: the subject show is itself in the metro's night and is
	// about to be dropped, so fetching exactly the cap would quietly serve
	// nineteen rows on a night that has twenty others.
	rows, err := s.GetSceneShowsInRange(city, state, start.UTC(), end.UTC(), loc, showAlsoTonightCap+1)
	if err != nil {
		// Below the scene threshold is not a failure here. The rail's question is
		// "what else is on tonight", and "this room is not part of a scene we
		// track" answers it with nothing — a 404 would break a show page that is
		// otherwise perfectly serveable.
		var sceneErr *apperrors.SceneError
		if errors.As(err, &sceneErr) && sceneErr.Code == apperrors.CodeSceneNotFound {
			// The venue's OWN zone, not the modal scene clock resolved above: the
			// scene clock earns its place only while there IS a scene page for Date
			// to agree with. sceneLocation also reads VERIFIED venues only, so the
			// rooms that land here are exactly the ones it answers worst for.
			return emptyAlsoTonight(subject, subject.venueLocation()), nil
		}
		return nil, err
	}

	shows := make([]contracts.SceneShowSummary, 0, len(rows))
	for _, row := range rows {
		if row.ID == subject.ShowID {
			continue
		}
		if len(shows) == showAlsoTonightCap {
			break
		}
		shows = append(shows, row)
	}

	// The slug is a LINK, so it is served only when the link resolves. The
	// scene-day surface refuses dates outside its tracked window, and an archive
	// show has a real scene and a real date but no page to point at; emitting the
	// slug anyway would advertise a URL this same service answers 404 to. Display
	// identity below is unconditional, because naming the metro is true either
	// way. Same split as SceneDayResponse.PrevDate/NextDate.
	sceneSlug := ""
	if dateIsServable(date, time.Now().In(loc)) {
		sceneSlug = buildSceneSlug(city, state)
	}

	return &contracts.ShowAlsoTonightResponse{
		SceneSlug: sceneSlug,
		SceneName: fmt.Sprintf("%s, %s", city, state),
		City:      city,
		State:     state,
		Date:      date.String(),
		Timezone:  loc.String(),
		ShowCount: len(shows),
		Shows:     shows,
	}, nil
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

// venueLocation is the room's own zone, for the branches with no scene clock to
// use. Precedence matches utils.EventLocation everywhere else: the venue's
// explicit IANA zone, then the US state map, then its Arizona default.
//
// The column has to be read rather than deriving everything from the state,
// because GetTimezoneForState answers America/Phoenix for every unknown or blank
// state. Without it a 01:00 Berlin set would be published under the previous
// date by the one field whose entire job is to name the right one.
func (a *alsoTonightSubject) venueLocation() *time.Location {
	if a.VenueTimezone == "" {
		return utils.EventLocation(nil, a.VenueState)
	}
	return utils.EventLocation(&a.VenueTimezone, a.VenueState)
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
func emptyAlsoTonight(subject *alsoTonightSubject, loc *time.Location) *contracts.ShowAlsoTonightResponse {
	return &contracts.ShowAlsoTonightResponse{
		Date:     dateOf(subject.EventDate.In(loc)).String(),
		Timezone: loc.String(),
		Shows:    []contracts.SceneShowSummary{},
	}
}
