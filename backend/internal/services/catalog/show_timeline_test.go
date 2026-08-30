package catalog

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	"gorm.io/gorm"

	apperrors "psychic-homily-backend/internal/errors"
	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/testutil"
)

// GetShowTimeline against a real database, because every property that decides
// what the spine and the recurrence module say is a SQL property: a row
// comparison over (event_date, id), a status filter, a metro-or-city predicate,
// and a bill ordering that has to agree with the frontend's splitBill.
//
// Chicago is the fixture metro. Chicago and Evanston share CBSA 16980 while
// Milwaukee is 33340, so the three make a metro-scoping assertion a same-city
// fixture could not.

const timelineChicagoZone = "America/Chicago"

// timelineRoom is the seed spec for one venue. Metro presence and the timezone
// column are explicit because several tests turn on a room that has neither.
type timelineRoom struct {
	name, city, state string
	tz                string // "" => no timezone column, so the state map decides
	noMetro           bool   // simulate a row the metro backfill never reached
}

type ShowTimelineIntegrationTestSuite struct {
	suite.Suite
	testDB *testutil.TestDatabase
	db     *gorm.DB
	svc    *ShowService
	seq    int
}

func (s *ShowTimelineIntegrationTestSuite) SetupSuite() {
	s.testDB = testutil.SetupTestPostgres(s.T())
	s.db = s.testDB.DB
	s.svc = NewShowService(s.db)
}

func (s *ShowTimelineIntegrationTestSuite) TearDownSuite() { s.testDB.Cleanup() }

func (s *ShowTimelineIntegrationTestSuite) TearDownTest() {
	sqlDB, err := s.db.DB()
	s.Require().NoError(err)
	// FK-safe order: the junctions before the rows they point at.
	_, _ = sqlDB.Exec("DELETE FROM show_artists")
	_, _ = sqlDB.Exec("DELETE FROM show_venues")
	_, _ = sqlDB.Exec("DELETE FROM shows")
	_, _ = sqlDB.Exec("DELETE FROM artists")
	_, _ = sqlDB.Exec("DELETE FROM venues")
}

func TestShowTimelineIntegrationTestSuite(t *testing.T) {
	suite.Run(t, new(ShowTimelineIntegrationTestSuite))
}

// =============================================================================
// FIXTURES
// =============================================================================

// next returns a per-suite counter, so every seeded slug is unique across tests
// that share the suite's database.
func (s *ShowTimelineIntegrationTestSuite) next() int {
	s.seq++
	return s.seq
}

func (s *ShowTimelineIntegrationTestSuite) loc(name string) *time.Location {
	loc, err := time.LoadLocation(name)
	s.Require().NoError(err)
	return loc
}

// seedRoom writes the venue. Defaults mirror the production write paths, which
// stamp the geocoder's metro alongside the geocoding.
func (s *ShowTimelineIntegrationTestSuite) seedRoom(spec timelineRoom) *catalogm.Venue {
	slug := fmt.Sprintf("timeline-venue-%d", s.next())
	venue := &catalogm.Venue{
		Name:  spec.name,
		Slug:  &slug,
		City:  spec.city,
		State: spec.state,
	}
	if !spec.noMetro {
		venue.Metro = seedMetro(spec.city, spec.state)
	}
	if spec.tz != "" {
		tz := spec.tz
		venue.Timezone = &tz
	}
	s.Require().NoError(s.db.Create(venue).Error)
	return venue
}

// seedChicagoRoom is the common case: on the fixture metro, on its clock.
func (s *ShowTimelineIntegrationTestSuite) seedChicagoRoom(name, city string) *catalogm.Venue {
	return s.seedRoom(timelineRoom{name: name, city: city, state: "IL", tz: timelineChicagoZone})
}

// seedAct writes an artist based in (city, state), with the metro column the
// geocoder would stamp.
func (s *ShowTimelineIntegrationTestSuite) seedAct(name, city, state string) *catalogm.Artist {
	slug := fmt.Sprintf("timeline-artist-%d", s.next())
	artist := &catalogm.Artist{
		Name:  name,
		Slug:  &slug,
		City:  &city,
		State: &state,
		Metro: seedMetro(city, state),
	}
	s.Require().NoError(s.db.Create(artist).Error)
	return artist
}

// seedDate writes one show at one room, billing each act at its slice position
// as a plain performer. A nil room is a show with no venue.
func (s *ShowTimelineIntegrationTestSuite) seedDate(
	room *catalogm.Venue, at time.Time, status catalogm.ShowStatus, acts ...*catalogm.Artist,
) *catalogm.Show {
	n := s.next()
	slug := fmt.Sprintf("timeline-show-%d", n)
	show := &catalogm.Show{
		Title:     fmt.Sprintf("timeline show %d", n),
		Slug:      &slug,
		EventDate: at.UTC(),
		Status:    status,
	}
	s.Require().NoError(s.db.Create(show).Error)
	if room != nil {
		s.Require().NoError(s.db.Create(&catalogm.ShowVenue{ShowID: show.ID, VenueID: room.ID}).Error)
	}
	for i, act := range acts {
		s.billAct(show, act, i, contracts.SetTypePerformer)
	}
	return show
}

// billAct puts one act on a bill at an explicit position and set type. An empty
// setType writes SQL NULL, which the column allows and most rows hold.
func (s *ShowTimelineIntegrationTestSuite) billAct(
	show *catalogm.Show, act *catalogm.Artist, position int, setType string,
) {
	var stored any
	if setType != "" {
		stored = setType
	}
	s.Require().NoError(s.db.Exec(
		`INSERT INTO show_artists (show_id, artist_id, position, set_type) VALUES (?, ?, ?, ?)`,
		show.ID, act.ID, position, stored,
	).Error)
}

func (s *ShowTimelineIntegrationTestSuite) timelineFor(show *catalogm.Show) *contracts.ShowTimelineResponse {
	timeline, err := s.svc.GetShowTimeline(fmt.Sprint(show.ID))
	s.Require().NoError(err)
	s.Require().NotNil(timeline)
	return timeline
}

// =============================================================================
// THE SPINE
// =============================================================================

// The headliner's own dates either side of the subject, nearest first, and never
// the subject itself. The same-instant sibling is what pins the self-exclusion
// mechanism: the bound is a (event_date, id) row comparison, so a show sharing
// the subject's instant still lands on a definite side rather than tying with it.
func (s *ShowTimelineIntegrationTestSuite) TestShowTimeline_AdjacentDatesFlankTheSubject() {
	loc := s.loc(timelineChicagoZone)
	room := s.seedChicagoRoom("Empty Bottle", "Chicago")
	act := s.seedAct("Spine Act", "Portland", "OR")

	s.seedDate(room, time.Date(2026, time.January, 10, 20, 0, 0, 0, loc), catalogm.ShowStatusApproved, act)
	nearestPrevious := s.seedDate(room, time.Date(2026, time.July, 4, 20, 0, 0, 0, loc), catalogm.ShowStatusApproved, act)
	subjectAt := time.Date(2026, time.September, 18, 20, 0, 0, 0, loc)
	subject := s.seedDate(room, subjectAt, catalogm.ShowStatusApproved, act)
	sameInstant := s.seedDate(room, subjectAt, catalogm.ShowStatusApproved, act)
	s.seedDate(room, time.Date(2027, time.March, 1, 20, 0, 0, 0, loc), catalogm.ShowStatusApproved, act)

	timeline := s.timelineFor(subject)
	s.Equal(act.ID, timeline.HeadlinerArtistID)
	s.Require().NotNil(timeline.Previous)
	s.Require().NotNil(timeline.Next)
	s.Equal(nearestPrevious.ID, timeline.Previous.ShowID, "the spine holds the NEAREST prior date")
	s.Equal(sameInstant.ID, timeline.Next.ShowID,
		"a show sharing the subject's instant is ordered by id, so it is the next date rather than a tie")
	s.NotEqual(subject.ID, timeline.Previous.ShowID)
	s.NotEqual(subject.ID, timeline.Next.ShowID)

	// The entry is enough to print and link a line on its own.
	s.Equal(*nearestPrevious.Slug, timeline.Previous.ShowSlug)
	s.Equal("Empty Bottle", timeline.Previous.VenueName)
	s.Equal(*room.Slug, timeline.Previous.VenueSlug)
	s.Equal("Chicago", timeline.Previous.City)
	s.Equal("IL", timeline.Previous.State)
	s.Equal(timelineChicagoZone, timeline.Previous.Timezone)

	// The spine hangs off /shows/{show_id}, which every sibling addresses by id
	// OR slug.
	bySlug, err := s.svc.GetShowTimeline(*subject.Slug)
	s.Require().NoError(err)
	s.Require().NotNil(bySlug.Previous)
	s.Equal(nearestPrevious.ID, bySlug.Previous.ShowID)
}

// Adjacency is an ABSOLUTE INSTANT comparison. Both neighbours here are seeded
// so that a venue-local reading would swap them: the Berlin room is 30 minutes
// before the subject but reads as the following local date, and the Honolulu
// room is an hour after but reads as the preceding one.
func (s *ShowTimelineIntegrationTestSuite) TestShowTimeline_AdjacencyOrdersByInstantNotVenueLocalDate() {
	chicago := s.loc(timelineChicagoZone)
	berlin := s.loc("Europe/Berlin")
	honolulu := s.loc("Pacific/Honolulu")

	subjectAt := time.Date(2026, time.September, 19, 1, 0, 0, 0, time.UTC)
	previousAt := subjectAt.Add(-30 * time.Minute)
	nextAt := subjectAt.Add(time.Hour)

	// The premise the assertions rest on: instant order and venue-local date
	// order disagree about both neighbours.
	s.Equal("2026-09-18", subjectAt.In(chicago).Format("2006-01-02"))
	s.Equal("2026-09-19", previousAt.In(berlin).Format("2006-01-02"))
	s.Equal("2026-09-18", nextAt.In(honolulu).Format("2006-01-02"))

	subjectRoom := s.seedChicagoRoom("Empty Bottle", "Chicago")
	berlinRoom := s.seedRoom(timelineRoom{name: "Berghain", city: "Berlin", state: "BE", tz: "Europe/Berlin"})
	honoluluRoom := s.seedRoom(timelineRoom{name: "Hawaiian Brian's", city: "Honolulu", state: "HI", tz: "Pacific/Honolulu"})
	act := s.seedAct("Touring Act", "Portland", "OR")

	earlierInstant := s.seedDate(berlinRoom, previousAt, catalogm.ShowStatusApproved, act)
	subject := s.seedDate(subjectRoom, subjectAt, catalogm.ShowStatusApproved, act)
	laterInstant := s.seedDate(honoluluRoom, nextAt, catalogm.ShowStatusApproved, act)

	timeline := s.timelineFor(subject)
	s.Require().NotNil(timeline.Previous)
	s.Require().NotNil(timeline.Next)
	s.Equal(earlierInstant.ID, timeline.Previous.ShowID,
		"the earlier INSTANT is the previous date, whatever its room's local calendar says")
	s.Equal(laterInstant.ID, timeline.Next.ShowID,
		"the later INSTANT is the next date, whatever its room's local calendar says")

	// Local dates remain a display concern, which is what each entry's resolved
	// zone is for.
	s.Equal("Europe/Berlin", timeline.Previous.Timezone)
	s.Equal("Pacific/Honolulu", timeline.Next.Timezone)
}

// Non-approved shows are invisible here exactly as they are on every other
// anonymous surface. Each hidden status sits BETWEEN the subject and its real
// neighbour, so a missing status filter would surface it rather than merely
// reorder the answer.
func (s *ShowTimelineIntegrationTestSuite) TestShowTimeline_NonApprovedNeighboursNeverEnterTheSpine() {
	loc := s.loc(timelineChicagoZone)
	room := s.seedChicagoRoom("Empty Bottle", "Chicago")
	act := s.seedAct("Hidden Neighbour Act", "Portland", "OR")

	approvedPrevious := s.seedDate(room, time.Date(2026, time.August, 1, 20, 0, 0, 0, loc), catalogm.ShowStatusApproved, act)
	hidden := []*catalogm.Show{
		s.seedDate(room, time.Date(2026, time.September, 10, 20, 0, 0, 0, loc), catalogm.ShowStatusPending, act),
		s.seedDate(room, time.Date(2026, time.September, 11, 20, 0, 0, 0, loc), catalogm.ShowStatusPrivate, act),
		s.seedDate(room, time.Date(2026, time.September, 12, 20, 0, 0, 0, loc), catalogm.ShowStatusRejected, act),
		s.seedDate(room, time.Date(2026, time.September, 25, 20, 0, 0, 0, loc), catalogm.ShowStatusPending, act),
	}
	subject := s.seedDate(room, time.Date(2026, time.September, 18, 20, 0, 0, 0, loc), catalogm.ShowStatusApproved, act)

	timeline := s.timelineFor(subject)
	s.Require().NotNil(timeline.Previous)
	s.Equal(approvedPrevious.ID, timeline.Previous.ShowID,
		"the previous date is the approved one further out, never the nearer hidden ones")
	s.Nil(timeline.Next, "a pending date ahead of the subject is not a next date")
	for _, show := range hidden {
		s.NotEqual(show.ID, timeline.Previous.ShowID)
	}
}

// A headliner with no other dates has an empty spine, and the other acts playing
// this room are not its dates: adjacency is scoped to the resolved headliner.
func (s *ShowTimelineIntegrationTestSuite) TestShowTimeline_HeadlinerWithNoOtherDatesHasAnEmptySpine() {
	loc := s.loc(timelineChicagoZone)
	room := s.seedChicagoRoom("Empty Bottle", "Chicago")
	act := s.seedAct("Only Date Act", "Portland", "OR")
	stranger := s.seedAct("Stranger Act", "Portland", "OR")

	s.seedDate(room, time.Date(2026, time.July, 4, 20, 0, 0, 0, loc), catalogm.ShowStatusApproved, stranger)
	s.seedDate(room, time.Date(2026, time.November, 1, 20, 0, 0, 0, loc), catalogm.ShowStatusApproved, stranger)
	subject := s.seedDate(room, time.Date(2026, time.September, 18, 20, 0, 0, 0, loc), catalogm.ShowStatusApproved, act)

	timeline := s.timelineFor(subject)
	s.Equal(act.ID, timeline.HeadlinerArtistID)
	s.Nil(timeline.Previous)
	s.Nil(timeline.Next)
}

// The spine's act is resolved by the DISPLAY rule the page's own heading uses:
// a curated 'headliner' first, then the lowest bill position. Each arm seeds a
// prior date for its expected act ONLY, so the resolution is observable in the
// spine and not merely in the reported id.
func (s *ShowTimelineIntegrationTestSuite) TestShowTimeline_HeadlinerIsTheCuratedActThenTheLowestPosition() {
	loc := s.loc(timelineChicagoZone)
	subjectAt := time.Date(2026, time.September, 18, 20, 0, 0, 0, loc)
	priorAt := time.Date(2026, time.July, 4, 20, 0, 0, 0, loc)

	s.Run("a curated headliner outranks a lower position", func() {
		room := s.seedChicagoRoom("Empty Bottle", "Chicago")
		opener := s.seedAct("Curated Opener", "Portland", "OR")
		headliner := s.seedAct("Curated Headliner", "Portland", "OR")

		headlinerPrior := s.seedDate(room, priorAt, catalogm.ShowStatusApproved, headliner)
		subject := s.seedDate(room, subjectAt, catalogm.ShowStatusApproved)
		s.billAct(subject, opener, 0, contracts.SetTypePerformer)
		s.billAct(subject, headliner, 3, contracts.SetTypeHeadliner)

		timeline := s.timelineFor(subject)
		s.Equal(headliner.ID, timeline.HeadlinerArtistID,
			"a curated headliner leads the bill however far down it is positioned")
		s.Require().NotNil(timeline.Previous)
		s.Equal(headlinerPrior.ID, timeline.Previous.ShowID, "the spine follows the resolved headliner")
	})

	s.TearDownTest()

	s.Run("an uncurated bill takes the lowest position", func() {
		room := s.seedChicagoRoom("Empty Bottle", "Chicago")
		lead := s.seedAct("Uncurated Lead", "Portland", "OR")
		support := s.seedAct("Uncurated Support", "Portland", "OR")

		leadPrior := s.seedDate(room, priorAt, catalogm.ShowStatusApproved, lead)
		subject := s.seedDate(room, subjectAt, catalogm.ShowStatusApproved)
		// A NULL set_type is the shape most rows hold, and it states nothing: it
		// must not outrank the position ordering.
		s.billAct(subject, support, 2, "")
		s.billAct(subject, lead, 0, contracts.SetTypePerformer)

		timeline := s.timelineFor(subject)
		s.Equal(lead.ID, timeline.HeadlinerArtistID,
			"with nothing curated the lowest bill position leads, matching splitBill")
		s.Require().NotNil(timeline.Previous)
		s.Equal(leadPrior.ID, timeline.Previous.ShowID)
	})
}

// =============================================================================
// RECURRENCE
// =============================================================================

// The place is the METRO when the room has one, so an act that played Evanston
// has played this show's place. Milwaukee is a different CBSA and is not it.
func (s *ShowTimelineIntegrationTestSuite) TestShowTimeline_LastPlayedSpansTheMetroNotTheCity() {
	loc := s.loc(timelineChicagoZone)
	chicago := s.seedChicagoRoom("Empty Bottle", "Chicago")
	evanston := s.seedChicagoRoom("Space", "Evanston")
	milwaukee := s.seedRoom(timelineRoom{name: "Cactus Club", city: "Milwaukee", state: "WI", tz: timelineChicagoZone})

	// The premise: Evanston shares the subject's CBSA and Milwaukee does not.
	s.Require().NotNil(chicago.Metro)
	s.Require().NotNil(evanston.Metro)
	s.Require().NotNil(milwaukee.Metro)
	s.Equal(*chicago.Metro, *evanston.Metro)
	s.NotEqual(*chicago.Metro, *milwaukee.Metro)

	returning := s.seedAct("Returning Act", "Portland", "OR")
	passingThrough := s.seedAct("Passing Through Act", "Seattle", "WA")

	inMetro := s.seedDate(evanston, time.Date(2026, time.March, 2, 20, 0, 0, 0, loc), catalogm.ShowStatusApproved, returning)
	s.seedDate(milwaukee, time.Date(2026, time.March, 3, 20, 0, 0, 0, loc), catalogm.ShowStatusApproved, passingThrough)
	subject := s.seedDate(chicago, time.Date(2026, time.September, 18, 20, 0, 0, 0, loc),
		catalogm.ShowStatusApproved, returning, passingThrough)

	timeline := s.timelineFor(subject)
	s.Require().Len(timeline.Recurrence, 1, "only the act with a date in this metro has something to say")
	s.Equal(returning.ID, timeline.Recurrence[0].ArtistID)
	s.False(timeline.Recurrence[0].IsHometown)
	s.Require().NotNil(timeline.Recurrence[0].LastPlayed)
	s.Equal(inMetro.ID, timeline.Recurrence[0].LastPlayed.ShowID)
	s.Equal("Space", timeline.Recurrence[0].LastPlayed.VenueName)
	s.Equal("Evanston", timeline.Recurrence[0].LastPlayed.City,
		"the entry names the room actually played, not the metro's principal city")
}

// "Last played" is the most recent date STRICTLY BEFORE the subject. A date
// ahead of the subject is not history, and on its own it is not something to
// say at all.
func (s *ShowTimelineIntegrationTestSuite) TestShowTimeline_LastPlayedIsTheMostRecentDateStrictlyBeforeTheSubject() {
	loc := s.loc(timelineChicagoZone)
	chicago := s.seedChicagoRoom("Empty Bottle", "Chicago")
	evanston := s.seedChicagoRoom("Space", "Evanston")

	returning := s.seedAct("Returning Act", "Portland", "OR")
	announced := s.seedAct("Announced Act", "Seattle", "WA")

	s.seedDate(evanston, time.Date(2025, time.September, 18, 20, 0, 0, 0, loc), catalogm.ShowStatusApproved, returning)
	mostRecent := s.seedDate(evanston, time.Date(2026, time.July, 4, 20, 0, 0, 0, loc), catalogm.ShowStatusApproved, returning)
	subject := s.seedDate(chicago, time.Date(2026, time.September, 18, 20, 0, 0, 0, loc),
		catalogm.ShowStatusApproved, returning, announced)
	s.seedDate(evanston, time.Date(2026, time.November, 1, 20, 0, 0, 0, loc), catalogm.ShowStatusApproved, returning)
	s.seedDate(evanston, time.Date(2026, time.November, 2, 20, 0, 0, 0, loc), catalogm.ShowStatusApproved, announced)

	timeline := s.timelineFor(subject)
	s.Require().Len(timeline.Recurrence, 1,
		"an act whose only date in this place is still ahead of the subject has nothing to say")
	s.Equal(returning.ID, timeline.Recurrence[0].ArtistID)
	s.Require().NotNil(timeline.Recurrence[0].LastPlayed)
	s.Equal(mostRecent.ID, timeline.Recurrence[0].LastPlayed.ShowID,
		"the latest date before the subject wins, and the later date is not one")
}

// A room the metro backfill never reached still has a place: its city and state.
// The match TRIMs and case-folds both sides, because those columns are free text
// and nothing normalizes them on write.
func (s *ShowTimelineIntegrationTestSuite) TestShowTimeline_PlaceFallsBackToCityAndStateWhenTheRoomHasNoMetro() {
	loc := s.loc(timelineChicagoZone)
	capri := s.seedRoom(timelineRoom{name: "Capri", city: "Marfa", state: "TX"})
	planet := s.seedRoom(timelineRoom{name: "Planet Marfa", city: "Marfa", state: "TX"})
	alpine := s.seedRoom(timelineRoom{name: "Railroad Blues", city: "Alpine", state: "TX"})
	s.Require().Nil(capri.Metro, "a no-CBSA city pins no metro, which is what makes this the fallback path")
	s.Require().NoError(s.db.Model(planet).Update("city", "  marfa  ").Error)

	sameCity := s.seedAct("Same City Act", "Portland", "OR")
	otherCity := s.seedAct("Other City Act", "Seattle", "WA")

	inPlace := s.seedDate(planet, time.Date(2026, time.March, 2, 20, 0, 0, 0, loc), catalogm.ShowStatusApproved, sameCity)
	s.seedDate(alpine, time.Date(2026, time.March, 3, 20, 0, 0, 0, loc), catalogm.ShowStatusApproved, otherCity)
	subject := s.seedDate(capri, time.Date(2026, time.September, 18, 20, 0, 0, 0, loc),
		catalogm.ShowStatusApproved, sameCity, otherCity)

	timeline := s.timelineFor(subject)
	s.Require().Len(timeline.Recurrence, 1, "a different city in the same state is a different place")
	s.Equal(sameCity.ID, timeline.Recurrence[0].ArtistID)
	s.Require().NotNil(timeline.Recurrence[0].LastPlayed)
	s.Equal(inPlace.ID, timeline.Recurrence[0].LastPlayed.ShowID,
		"padding and case on a free-text city must not hide a date in the same place")
}

// Hometown is two independent positive signals: matching metros, or a matching
// city and state. Either is sufficient, because the metro column is populated by
// a manually-run backfill and is NULL on the rows it has not reached.
func (s *ShowTimelineIntegrationTestSuite) TestShowTimeline_HometownIsTheMetroThenTheCityAndState() {
	loc := s.loc(timelineChicagoZone)
	subjectAt := time.Date(2026, time.September, 18, 20, 0, 0, 0, loc)
	priorAt := time.Date(2026, time.March, 2, 20, 0, 0, 0, loc)

	s.Run("matching metros are home even from another city", func() {
		chicago := s.seedChicagoRoom("Empty Bottle", "Chicago")
		evanston := s.seedChicagoRoom("Space", "Evanston")
		local := s.seedAct("Local Act", "Evanston", "IL")
		visitor := s.seedAct("Visiting Act", "Milwaukee", "WI")
		s.Require().NotNil(local.Metro)
		s.Require().NotNil(chicago.Metro)
		s.Equal(*chicago.Metro, *local.Metro)
		s.NotEqual(*chicago.Metro, *visitor.Metro)

		s.seedDate(evanston, priorAt, catalogm.ShowStatusApproved, visitor)
		subject := s.seedDate(chicago, subjectAt, catalogm.ShowStatusApproved, local, visitor)

		timeline := s.timelineFor(subject)
		s.Require().Len(timeline.Recurrence, 2)
		s.Equal(local.ID, timeline.Recurrence[0].ArtistID, "recurrence is in bill order")
		s.True(timeline.Recurrence[0].IsHometown, "an Evanston band is home in the Chicago metro")
		s.Nil(timeline.Recurrence[0].LastPlayed, "hometown stands alone when the archive holds no prior date")
		s.Equal(visitor.ID, timeline.Recurrence[1].ArtistID)
		s.False(timeline.Recurrence[1].IsHometown, "a Milwaukee band is a visitor here")
		s.NotNil(timeline.Recurrence[1].LastPlayed)
	})

	s.TearDownTest()

	s.Run("a matching city and state is home when neither side has a metro", func() {
		capri := s.seedRoom(timelineRoom{name: "Capri", city: "Marfa", state: "TX"})
		local := s.seedAct("Marfa Act", "Marfa", "TX")
		visitor := s.seedAct("Alpine Act", "Alpine", "TX")
		s.Require().Nil(capri.Metro)
		s.Require().Nil(local.Metro, "the city arm is the only signal left for this pair")

		subject := s.seedDate(capri, subjectAt, catalogm.ShowStatusApproved, local, visitor)

		timeline := s.timelineFor(subject)
		s.Require().Len(timeline.Recurrence, 1, "the act from another city has nothing to say")
		s.Equal(local.ID, timeline.Recurrence[0].ArtistID)
		s.True(timeline.Recurrence[0].IsHometown)
		s.Nil(timeline.Recurrence[0].LastPlayed)
	})
}

// Recurrence is emitted in BILL ORDER, headliner first, so the module's rows
// line up with the bill the page already printed. The curated headliner sits at
// a LATER position than the support act and both have a prior date here, so a
// plain position ordering would swap the two rows rather than merely relabel
// them.
func (s *ShowTimelineIntegrationTestSuite) TestShowTimeline_RecurrenceIsInBillOrderHeadlinerFirst() {
	loc := s.loc(timelineChicagoZone)
	chicago := s.seedChicagoRoom("Empty Bottle", "Chicago")
	evanston := s.seedChicagoRoom("Space", "Evanston")
	headliner := s.seedAct("Curated Headliner", "Portland", "OR")
	support := s.seedAct("Support Act", "Seattle", "WA")

	headlinerPrior := s.seedDate(evanston, time.Date(2026, time.March, 2, 20, 0, 0, 0, loc), catalogm.ShowStatusApproved, headliner)
	supportPrior := s.seedDate(evanston, time.Date(2026, time.March, 3, 20, 0, 0, 0, loc), catalogm.ShowStatusApproved, support)
	subject := s.seedDate(chicago, time.Date(2026, time.September, 18, 20, 0, 0, 0, loc), catalogm.ShowStatusApproved)
	s.billAct(subject, support, 0, contracts.SetTypePerformer)
	s.billAct(subject, headliner, 4, contracts.SetTypeHeadliner)

	timeline := s.timelineFor(subject)
	s.Require().Len(timeline.Recurrence, 2, "both acts have a prior date in this metro")
	s.Equal(headliner.ID, timeline.Recurrence[0].ArtistID,
		"the curated headliner leads the module however far down it is positioned")
	s.Equal(support.ID, timeline.Recurrence[1].ArtistID)
	// Each act's row carries ITS date, so a reordering that kept the ids in place
	// but moved the payloads would still be caught.
	s.Require().NotNil(timeline.Recurrence[0].LastPlayed)
	s.Equal(headlinerPrior.ID, timeline.Recurrence[0].LastPlayed.ShowID)
	s.Require().NotNil(timeline.Recurrence[1].LastPlayed)
	s.Equal(supportPrior.ID, timeline.Recurrence[1].LastPlayed.ShowID)
}

// An entry that states nothing is not a fact a client can render, so acts that
// are neither home nor returning are dropped. What is left is an empty slice
// rather than a null, which is also what a show with no bill answers.
func (s *ShowTimelineIntegrationTestSuite) TestShowTimeline_ActsWithNothingToSayAreDroppedAndRecurrenceIsNeverNil() {
	loc := s.loc(timelineChicagoZone)
	room := s.seedChicagoRoom("Empty Bottle", "Chicago")
	first := s.seedAct("First Visitor", "Portland", "OR")
	second := s.seedAct("Second Visitor", "Seattle", "WA")

	subject := s.seedDate(room, time.Date(2026, time.September, 18, 20, 0, 0, 0, loc),
		catalogm.ShowStatusApproved, first, second)

	timeline := s.timelineFor(subject)
	s.Equal(first.ID, timeline.HeadlinerArtistID, "the bill still resolves a headliner")
	s.NotNil(timeline.Recurrence, "an empty module must marshal as [] rather than null")
	s.Empty(timeline.Recurrence)

	billless := s.seedDate(room, time.Date(2026, time.September, 19, 20, 0, 0, 0, loc), catalogm.ShowStatusApproved)
	empty := s.timelineFor(billless)
	s.Zero(empty.HeadlinerArtistID, "a show with no bill names no headliner")
	s.Nil(empty.Previous)
	s.Nil(empty.Next)
	s.NotNil(empty.Recurrence)
	s.Empty(empty.Recurrence)
}

// =============================================================================
// RESOLUTION AND DEGRADED SHAPES
// =============================================================================

// Unknown and non-approved must be INDISTINGUISHABLE. This surface is anonymous,
// so a different answer for a hidden show would confirm that its id is real.
func (s *ShowTimelineIntegrationTestSuite) TestShowTimeline_UnknownAndNonApprovedShowsAreBothNotFound() {
	loc := s.loc(timelineChicagoZone)
	room := s.seedChicagoRoom("Empty Bottle", "Chicago")
	act := s.seedAct("Hidden Act", "Portland", "OR")

	addresses := []string{"999999", "no-such-show"}
	for _, status := range []catalogm.ShowStatus{
		catalogm.ShowStatusPending,
		catalogm.ShowStatusPrivate,
		catalogm.ShowStatusRejected,
	} {
		hidden := s.seedDate(room, time.Date(2026, time.September, 18, 20, 0, 0, 0, loc), status, act)
		addresses = append(addresses, fmt.Sprint(hidden.ID), *hidden.Slug)
	}

	for _, address := range addresses {
		_, err := s.svc.GetShowTimeline(address)
		s.Require().Error(err, "GetShowTimeline(%q) should not resolve", address)
		var showErr *apperrors.ShowError
		s.Require().ErrorAs(err, &showErr)
		s.Equal(apperrors.CodeShowNotFound, showErr.Code)
	}
}

// A subject billed at MORE THAN ONE room is placed by a room that can be placed,
// not by the lowest venue id. The unplaceable room here holds the lower id, and
// picking it would leave the subject with no place and drop every act's
// recurrence on a show whose other room answers perfectly well.
func (s *ShowTimelineIntegrationTestSuite) TestShowTimeline_MultiRoomSubjectIsPlacedByThePlaceableRoom() {
	loc := s.loc(timelineChicagoZone)
	// Blank city and state with no metro is the shape an ungeocoded room holds:
	// venues.city and venues.state are NOT NULL, so blank is as absent as they get.
	secret := s.seedRoom(timelineRoom{name: "A Secret Location", noMetro: true})
	s.Require().Nil(secret.Metro)
	chicago := s.seedChicagoRoom("Empty Bottle", "Chicago")
	s.Require().Less(secret.ID, chicago.ID,
		"the unplaceable room must hold the LOWER id, or a plain id ordering would pass this")

	act := s.seedAct("Returning Act", "Portland", "OR")
	prior := s.seedDate(chicago, time.Date(2026, time.March, 2, 20, 0, 0, 0, loc), catalogm.ShowStatusApproved, act)

	subject := s.seedDate(secret, time.Date(2026, time.September, 18, 20, 0, 0, 0, loc), catalogm.ShowStatusApproved, act)
	s.Require().NoError(s.db.Create(&catalogm.ShowVenue{ShowID: subject.ID, VenueID: chicago.ID}).Error)

	timeline := s.timelineFor(subject)
	s.Require().Len(timeline.Recurrence, 1,
		"the placeable room decides the place, so the archive still has a question to answer")
	s.Equal(act.ID, timeline.Recurrence[0].ArtistID)
	s.Require().NotNil(timeline.Recurrence[0].LastPlayed)
	s.Equal(prior.ID, timeline.Recurrence[0].LastPlayed.ShowID)
}

// Each module hides itself on its own evidence. A show with no room AND no
// denormalized city on its own row has no place to ask the archive about, so
// recurrence is empty even for an act based in the metro its other dates are in.
// The spine does not depend on a place and is still served.
func (s *ShowTimelineIntegrationTestSuite) TestShowTimeline_ShowWithNoVenueKeepsTheSpineAndEmptiesRecurrence() {
	loc := s.loc(timelineChicagoZone)
	room := s.seedChicagoRoom("Empty Bottle", "Chicago")
	act := s.seedAct("Placeless Subject Act", "Evanston", "IL")
	s.Require().NotNil(act.Metro)

	prior := s.seedDate(room, time.Date(2026, time.July, 4, 20, 0, 0, 0, loc), catalogm.ShowStatusApproved, act)
	subject := s.seedDate(nil, time.Date(2026, time.September, 18, 20, 0, 0, 0, loc), catalogm.ShowStatusApproved, act)
	// The premise: seedDate writes no city or state on the show row either, which
	// is what leaves this subject with nothing to fall back to.
	s.Require().Nil(subject.City)
	s.Require().Nil(subject.State)

	timeline := s.timelineFor(subject)
	s.Equal(act.ID, timeline.HeadlinerArtistID)
	s.Require().NotNil(timeline.Previous)
	s.Equal(prior.ID, timeline.Previous.ShowID)
	s.Equal(timelineChicagoZone, timeline.Previous.Timezone)
	s.Nil(timeline.Next)
	s.NotNil(timeline.Recurrence)
	s.Empty(timeline.Recurrence, "no room means no place, so no act can be home here or returning here")
}

// A roomless subject that DOES carry a city and state on its own show row is
// placed from them, so its recurrence module still answers. No venue joins, so
// venues.metro is absent and the place is the city arm.
func (s *ShowTimelineIntegrationTestSuite) TestShowTimeline_RoomlessSubjectIsPlacedFromItsOwnShowRow() {
	loc := s.loc(timelineChicagoZone)
	room := s.seedChicagoRoom("Empty Bottle", "Chicago")
	returning := s.seedAct("Returning Act", "Portland", "OR")
	local := s.seedAct("Chicago Act", "Chicago", "IL")

	prior := s.seedDate(room, time.Date(2026, time.March, 2, 20, 0, 0, 0, loc), catalogm.ShowStatusApproved, returning)
	subject := s.seedDate(nil, time.Date(2026, time.September, 18, 20, 0, 0, 0, loc),
		catalogm.ShowStatusApproved, returning, local)
	s.Require().NoError(s.db.Model(subject).Updates(map[string]any{"city": "Chicago", "state": "IL"}).Error)

	timeline := s.timelineFor(subject)
	s.Require().Len(timeline.Recurrence, 2, "the show row's city is a place, so both acts have something to say")
	s.Equal(returning.ID, timeline.Recurrence[0].ArtistID)
	s.False(timeline.Recurrence[0].IsHometown, "a Portland band is a visitor here")
	s.Require().NotNil(timeline.Recurrence[0].LastPlayed)
	s.Equal(prior.ID, timeline.Recurrence[0].LastPlayed.ShowID)
	s.Equal(local.ID, timeline.Recurrence[1].ArtistID)
	s.True(timeline.Recurrence[1].IsHometown,
		"the city arm decides home, because a subject with no room has no metro to match on")
	s.Nil(timeline.Recurrence[1].LastPlayed)
}

// The venue lateral is LEFT joined on this path, so a roomless neighbour
// matches no lateral row and every venue column arrives NULL. It still belongs
// on the spine: the date is the fact, and the room is the annotation.
func (s *ShowTimelineIntegrationTestSuite) TestShowTimeline_NeighbourWithNoVenueStillLandsOnTheSpine() {
	loc := s.loc(timelineChicagoZone)
	room := s.seedChicagoRoom("Empty Bottle", "Chicago")
	act := s.seedAct("Roomless Neighbour Act", "Milwaukee", "WI")

	roomless := s.seedDate(nil, time.Date(2026, time.July, 4, 20, 0, 0, 0, loc), catalogm.ShowStatusApproved, act)
	subject := s.seedDate(room, time.Date(2026, time.September, 18, 20, 0, 0, 0, loc), catalogm.ShowStatusApproved, act)

	timeline := s.timelineFor(subject)
	s.Require().NotNil(timeline.Previous)
	s.Equal(roomless.ID, timeline.Previous.ShowID)
	s.Empty(timeline.Previous.VenueName)
	s.Empty(timeline.Previous.VenueSlug)
	s.Empty(timeline.Previous.City)
	s.Empty(timeline.Previous.State)
	// A stop with neither a zone nor a state still names one, because every
	// date has to be read on some clock.
	s.NotEmpty(timeline.Previous.Timezone)
}

// A roomless neighbour that carries a city and state on its own show row is
// placed and dated from them. The zone resolves through the state map, so the
// entry reads on the same clock the neighbour's own page reads it on rather than
// on utils.EventLocation's default.
func (s *ShowTimelineIntegrationTestSuite) TestShowTimeline_RoomlessNeighbourIsDatedOnItsOwnShowRow() {
	loc := s.loc(timelineChicagoZone)
	room := s.seedChicagoRoom("Empty Bottle", "Chicago")
	act := s.seedAct("Roomless Neighbour Act", "Portland", "OR")

	roomless := s.seedDate(nil, time.Date(2026, time.July, 4, 20, 0, 0, 0, loc), catalogm.ShowStatusApproved, act)
	s.Require().NoError(s.db.Model(roomless).Updates(map[string]any{"city": "Chicago", "state": "IL"}).Error)
	subject := s.seedDate(room, time.Date(2026, time.September, 18, 20, 0, 0, 0, loc), catalogm.ShowStatusApproved, act)

	timeline := s.timelineFor(subject)
	s.Require().NotNil(timeline.Previous)
	s.Equal(roomless.ID, timeline.Previous.ShowID)
	s.Empty(timeline.Previous.VenueName, "there is still no room to name")
	s.Equal("Chicago", timeline.Previous.City)
	s.Equal("IL", timeline.Previous.State)
	s.Equal(timelineChicagoZone, timeline.Previous.Timezone,
		"the show row's state decides the clock, not the Arizona default")
}
