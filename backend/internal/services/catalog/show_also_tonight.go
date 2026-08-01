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
//   - The date is the SHOW's, resolved in the VENUE's zone — never the viewer's
//     "tonight" and never UTC. A reader in Berlin opening a Chicago show page is
//     asking what else is on that night in Chicago, and a 21:00 Chicago set is
//     already the next UTC day.
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

	// No venue, or a venue with no usable place: nothing to scope by. The date
	// still has to be answered, so it is resolved against whatever zone the
	// venue's state implies, exactly as the calendar export does for a
	// venue-less show.
	if subject.VenueCity == "" || subject.VenueState == "" {
		return emptyAlsoTonight(subject, utils.EventLocation(nil, subject.VenueState)), nil
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
			return emptyAlsoTonight(subject, loc), nil
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

	return &contracts.ShowAlsoTonightResponse{
		SceneSlug: buildSceneSlug(city, state),
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
	ShowID     uint      `gorm:"column:show_id"`
	EventDate  time.Time `gorm:"column:event_date"`
	VenueCity  string    `gorm:"column:venue_city"`
	VenueState string    `gorm:"column:venue_state"`
}

// resolveAlsoTonightSubject loads the subject show by numeric id or slug.
//
// The status filter is part of the WHERE clause rather than a check on the
// loaded row so that a hidden show and a nonexistent one are indistinguishable
// from the outside — this endpoint is anonymous, so a distinguishable answer
// would confirm that an unpublished show id is real.
//
// The venue join is LEFT so a bill with no venue still resolves (an empty rail,
// not a 404), and the pick is `v.name ASC, v.id ASC` — the SAME pick
// GetSceneShowsInRange's DISTINCT ON makes. A multi-room show must be scoped by
// the same room both queries would name, or the rail could be computed for one
// venue's metro while every other surface attributes the show to another.
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
		       COALESCE(v.state, '') AS venue_state
		FROM shows s
		LEFT JOIN show_venues sv ON sv.show_id = s.id
		LEFT JOIN venues v ON v.id = sv.venue_id
		WHERE `
		andApproved = `
		  AND s.status = ?
		ORDER BY v.name ASC, v.id ASC
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
