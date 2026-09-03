package catalog

import (
	"fmt"
	"time"

	apperrors "psychic-homily-backend/internal/errors"
	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/contracts"
)

// The "also tonight" rail, exercised against a real database because every
// interesting property of it is a SQL-and-timezone property: which venues the
// metro predicate reaches, and which instants a venue-local calendar day holds.
//
// Chicago is the fixture metro throughout. Chicago and Evanston share CBSA
// 16980 while Milwaukee is 33340, so the two make a metro-scoping assertion that
// a same-city test could not: Evanston is a DIFFERENT city that must still be on
// the rail.

const alsoTonightZone = "America/Chicago"

// zoneOf reads a payload's nullable zone for a NAME assertion, mapping a
// withheld zone to the empty string so the comparison fails instead of panicking
// on a nil dereference. It cannot tell nil from a literal empty string, so a
// test asserting the WITHHOLDING asserts Nil on the field itself.
func zoneOf(zone *string) string {
	if zone == nil {
		return ""
	}
	return *zone
}

func (suite *SceneServiceIntegrationTestSuite) alsoTonightLoc() *time.Location {
	loc, err := time.LoadLocation(alsoTonightZone)
	suite.Require().NoError(err)
	return loc
}

// alsoTonightVenue is the seed spec for one room. Every field the rail's scope
// and clock depend on is explicit, because several tests turn on the difference
// between a venue whose metro column is set and one whose is not.
type alsoTonightVenue struct {
	name, city, state string
	tz                string // "" => no timezone column, so the state map decides
	noMetro           bool   // simulate a row the metro backfill never reached
	unverified        bool
}

// seedVenue writes the room. Defaults mirror the production write paths, which
// stamp the geocoder's metro alongside the geocoding.
func (suite *SceneServiceIntegrationTestSuite) seedVenue(spec alsoTonightVenue) *catalogm.Venue {
	venue := &catalogm.Venue{
		Name:  spec.name,
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
	suite.Require().NoError(suite.db.Create(venue).Error)
	// GORM writes no column for a false bool on Create, so the DB default wins;
	// both directions have to be set explicitly.
	suite.Require().NoError(suite.db.Model(venue).Update("verified", !spec.unverified).Error)
	return venue
}

// createAlsoTonightVenue is the common case: verified, metro stamped, on the
// fixture metro's clock.
func (suite *SceneServiceIntegrationTestSuite) createAlsoTonightVenue(name, city, state string) *catalogm.Venue {
	return suite.seedVenue(alsoTonightVenue{name: name, city: city, state: state, tz: alsoTonightZone})
}

// createAlsoTonightShow seeds one show at one venue. `at` is a venue-LOCAL wall
// time; storing its UTC instant is what makes the window assertions meaningful.
func (suite *SceneServiceIntegrationTestSuite) createAlsoTonightShow(title string, venueID uint, at time.Time, status catalogm.ShowStatus) *catalogm.Show {
	return suite.createAlsoTonightShowAt(title, []uint{venueID}, at, status)
}

// createAlsoTonightShowAt bills the show at every listed venue, which is the
// only way to exercise the multi-room pick that decides a show's scope.
func (suite *SceneServiceIntegrationTestSuite) createAlsoTonightShowAt(title string, venueIDs []uint, at time.Time, status catalogm.ShowStatus) *catalogm.Show {
	slug := fmt.Sprintf("also-tonight-%s-%d", title, time.Now().UnixNano())
	show := &catalogm.Show{
		Title:     title,
		Slug:      &slug,
		EventDate: at.UTC(),
		Status:    status,
	}
	suite.Require().NoError(suite.db.Create(show).Error)
	for _, venueID := range venueIDs {
		suite.Require().NoError(suite.db.Create(&catalogm.ShowVenue{ShowID: show.ID, VenueID: venueID}).Error)
	}
	return show
}

func (suite *SceneServiceIntegrationTestSuite) showIDs(idOrSlug string) []uint {
	rail, err := suite.sceneService.GetShowAlsoTonight(idOrSlug)
	suite.Require().NoError(err)
	suite.Require().NotNil(rail)
	return idsOf(rail.Shows)
}

// The two acceptance properties that define the rail: it never lists the show
// the reader is already looking at, and its reach is the METRO, not the city.
func (suite *SceneServiceIntegrationTestSuite) TestGetShowAlsoTonight_ExcludesSelfAndScopesToMetro() {
	loc := suite.alsoTonightLoc()
	chicago := suite.createAlsoTonightVenue("Empty Bottle", "Chicago", "IL")
	evanston := suite.createAlsoTonightVenue("Space", "Evanston", "IL")
	milwaukee := suite.createAlsoTonightVenue("Cactus Club", "Milwaukee", "WI")

	subject := suite.createAlsoTonightShow("subject", chicago.ID,
		time.Date(2026, time.September, 18, 20, 0, 0, 0, loc), catalogm.ShowStatusApproved)
	sibling := suite.createAlsoTonightShow("sibling", evanston.ID,
		time.Date(2026, time.September, 18, 21, 0, 0, 0, loc), catalogm.ShowStatusApproved)
	suite.createAlsoTonightShow("out-of-metro", milwaukee.ID,
		time.Date(2026, time.September, 18, 20, 0, 0, 0, loc), catalogm.ShowStatusApproved)
	suite.createAlsoTonightShow("previous-night", evanston.ID,
		time.Date(2026, time.September, 17, 21, 0, 0, 0, loc), catalogm.ShowStatusApproved)

	rail, err := suite.sceneService.GetShowAlsoTonight(fmt.Sprint(subject.ID))
	suite.Require().NoError(err)

	suite.Equal([]uint{sibling.ID}, suite.showIDs(fmt.Sprint(subject.ID)),
		"the rail must hold every OTHER show in the metro that night and nothing else")
	suite.Equal(1, rail.ShowCount)
	// The metro's principal city, not the subject venue's — the slug has to be
	// one /scenes/{slug} actually serves.
	suite.Equal("chicago-il", rail.SceneSlug)
	suite.Equal("Chicago", rail.City)
	suite.Equal("IL", rail.State)
	suite.Equal("Chicago, IL", rail.SceneName)
	suite.Equal("2026-09-18", rail.Date)
	suite.Equal(alsoTonightZone, zoneOf(rail.Timezone))
}

// The window is a half-open venue-local calendar day. A minute either side of it
// is a different night, and getting this wrong in UTC would move every evening
// show in the western hemisphere onto the following date.
func (suite *SceneServiceIntegrationTestSuite) TestGetShowAlsoTonight_WindowIsTheShowLocalCalendarDay() {
	loc := suite.alsoTonightLoc()
	chicago := suite.createAlsoTonightVenue("Empty Bottle", "Chicago", "IL")
	evanston := suite.createAlsoTonightVenue("Space", "Evanston", "IL")

	subject := suite.createAlsoTonightShow("subject", chicago.ID,
		time.Date(2026, time.September, 18, 20, 0, 0, 0, loc), catalogm.ShowStatusApproved)
	lastMinute := suite.createAlsoTonightShow("last-minute", evanston.ID,
		time.Date(2026, time.September, 18, 23, 59, 0, 0, loc), catalogm.ShowStatusApproved)
	firstMinute := suite.createAlsoTonightShow("first-minute", evanston.ID,
		time.Date(2026, time.September, 18, 0, 0, 0, 0, loc), catalogm.ShowStatusApproved)
	suite.createAlsoTonightShow("one-second-early", evanston.ID,
		time.Date(2026, time.September, 17, 23, 59, 59, 0, loc), catalogm.ShowStatusApproved)
	suite.createAlsoTonightShow("midnight-after", evanston.ID,
		time.Date(2026, time.September, 19, 0, 0, 0, 0, loc), catalogm.ShowStatusApproved)

	// Ordered, not merely present: the rail renders top to bottom, and
	// GetSceneShowsInRange's earliest-first ordering has to survive the
	// self-exclusion filter.
	suite.Equal([]uint{firstMinute.ID, lastMinute.ID}, suite.showIDs(fmt.Sprint(subject.ID)),
		"the window must be [local midnight, next local midnight) — inclusive of the first "+
			"instant of the date and exclusive of the first instant of the next, earliest first")
}

// The two days a year that are not 24 hours long. The window is built from
// calendarDate.start, whose whole purpose is to survive these, so the rail must
// hold a short day's whole 23 hours and a long day's whole 25 — including the
// hour a fall-back repeats, where two DIFFERENT instants share one wall clock
// and both genuinely belong to the date.
func (suite *SceneServiceIntegrationTestSuite) TestGetShowAlsoTonight_HoldsTheWholeDayAcrossDSTTransitions() {
	loc := suite.alsoTonightLoc()
	chicago := suite.createAlsoTonightVenue("Empty Bottle", "Chicago", "IL")
	evanston := suite.createAlsoTonightVenue("Space", "Evanston", "IL")

	suite.Run("spring forward loses an hour but not a show", func() {
		// 2026-03-08 in Chicago is 23 hours long: 02:00 does not exist.
		subject := suite.createAlsoTonightShow("dst-spring-subject", chicago.ID,
			time.Date(2026, time.March, 8, 20, 0, 0, 0, loc), catalogm.ShowStatusApproved)
		beforeJump := suite.createAlsoTonightShow("dst-before-jump", evanston.ID,
			time.Date(2026, time.March, 8, 1, 30, 0, 0, loc), catalogm.ShowStatusApproved)
		afterJump := suite.createAlsoTonightShow("dst-after-jump", evanston.ID,
			time.Date(2026, time.March, 8, 3, 30, 0, 0, loc), catalogm.ShowStatusApproved)
		suite.createAlsoTonightShow("dst-next-day", evanston.ID,
			time.Date(2026, time.March, 9, 0, 0, 0, 0, loc), catalogm.ShowStatusApproved)

		rail, err := suite.sceneService.GetShowAlsoTonight(fmt.Sprint(subject.ID))
		suite.Require().NoError(err)
		suite.Equal("2026-03-08", rail.Date)
		suite.Equal([]uint{beforeJump.ID, afterJump.ID}, suite.showIDs(fmt.Sprint(subject.ID)),
			"a 23-hour day must still span local midnight to local midnight")
	})

	suite.TearDownTest()

	suite.Run("fall back repeats an hour and both instants belong to the date", func() {
		chicago := suite.createAlsoTonightVenue("Empty Bottle", "Chicago", "IL")
		evanston := suite.createAlsoTonightVenue("Space", "Evanston", "IL")

		// 2026-11-01 in Chicago is 25 hours long: 01:30 happens twice, once at
		// UTC-5 and once at UTC-6. Constructed as absolute instants because
		// time.Date cannot express "the second 01:30".
		dayStart := time.Date(2026, time.November, 1, 0, 0, 0, 0, loc)
		subject := suite.createAlsoTonightShow("dst-fall-subject", chicago.ID,
			time.Date(2026, time.November, 1, 20, 0, 0, 0, loc), catalogm.ShowStatusApproved)
		firstPass := suite.createAlsoTonightShow("dst-first-0130", evanston.ID,
			dayStart.Add(90*time.Minute), catalogm.ShowStatusApproved)
		secondPass := suite.createAlsoTonightShow("dst-second-0130", evanston.ID,
			dayStart.Add(150*time.Minute), catalogm.ShowStatusApproved)
		// The last instant of the 25-hour day, and the first of the next.
		lastHour := suite.createAlsoTonightShow("dst-last-hour", evanston.ID,
			dayStart.Add(25*time.Hour-time.Minute), catalogm.ShowStatusApproved)
		suite.createAlsoTonightShow("dst-next-day", evanston.ID,
			dayStart.Add(25*time.Hour), catalogm.ShowStatusApproved)

		// Sanity: the two 01:30 rows really are distinct instants sharing a wall
		// clock, which is the only reason this test says anything.
		suite.Equal("01:30", firstPass.EventDate.In(loc).Format("15:04"))
		suite.Equal("01:30", secondPass.EventDate.In(loc).Format("15:04"))
		suite.NotEqual(firstPass.EventDate.UTC(), secondPass.EventDate.UTC())

		rail, err := suite.sceneService.GetShowAlsoTonight(fmt.Sprint(subject.ID))
		suite.Require().NoError(err)
		suite.Equal("2026-11-01", rail.Date)
		suite.Equal([]uint{firstPass.ID, secondPass.ID, lastHour.ID}, suite.showIDs(fmt.Sprint(subject.ID)),
			"a 25-hour day must hold both passes of the repeated hour and end at the NEXT local midnight")
	})
}

// A set that starts after midnight belongs to the date it starts on, not to the
// evening it feels like a continuation of. That is the scene-day contract, and
// the rail emits `Date` as a scene-day key — if the two bucketed a 00:30 set
// differently, the rail's own "see all" link would lead to a page missing it.
func (suite *SceneServiceIntegrationTestSuite) TestGetShowAlsoTonight_AfterMidnightSetBelongsToItsOwnDate() {
	loc := suite.alsoTonightLoc()
	chicago := suite.createAlsoTonightVenue("Empty Bottle", "Chicago", "IL")
	evanston := suite.createAlsoTonightVenue("Space", "Evanston", "IL")

	subject := suite.createAlsoTonightShow("subject", chicago.ID,
		time.Date(2026, time.September, 19, 0, 30, 0, 0, loc), catalogm.ShowStatusApproved)
	sameDate := suite.createAlsoTonightShow("same-date", evanston.ID,
		time.Date(2026, time.September, 19, 21, 0, 0, 0, loc), catalogm.ShowStatusApproved)
	suite.createAlsoTonightShow("previous-evening", evanston.ID,
		time.Date(2026, time.September, 18, 21, 0, 0, 0, loc), catalogm.ShowStatusApproved)

	rail, err := suite.sceneService.GetShowAlsoTonight(fmt.Sprint(subject.ID))
	suite.Require().NoError(err)
	suite.Equal("2026-09-19", rail.Date)
	suite.Equal([]uint{sameDate.ID}, suite.showIDs(fmt.Sprint(subject.ID)))
	// The ROW's date too, not just the payload's. Each row's event_date is
	// rendered in the loc handed to GetSceneShowsInRange; rendering it in UTC
	// would file a 21:00 Chicago set under the following day inside a payload
	// whose own date says otherwise.
	suite.Require().Len(rail.Shows, 1)
	suite.Equal("2026-09-19", rail.Shows[0].EventDate)
}

// A show billed at rooms in two different metros has to pick ONE, and it must be
// the same room GetSceneShowsInRange's DISTINCT ON would pick (`v.name ASC,
// v.id ASC`) — otherwise the rail is computed for one metro while every other
// surface files the show under another. Two hand-written raw SQL strings hold
// this invariant between them, so it needs a fixture, not a comment.
func (suite *SceneServiceIntegrationTestSuite) TestGetShowAlsoTonight_MultiVenueBillPicksTheSameRoomAsTheSceneQuery() {
	loc := suite.alsoTonightLoc()
	at := time.Date(2026, time.September, 18, 20, 0, 0, 0, loc)
	siblingAt := time.Date(2026, time.September, 18, 21, 0, 0, 0, loc)

	// Both metros must qualify as scenes, or the losing branch would come back
	// empty for the wrong reason and the test would pass by accident.
	chicagoA := suite.createAlsoTonightVenue("Empty Bottle", "Chicago", "IL")
	suite.createAlsoTonightVenue("Space", "Evanston", "IL")
	milwaukeeA := suite.createAlsoTonightVenue("Cactus Club", "Milwaukee", "WI")
	suite.createAlsoTonightVenue("Shank Hall", "Milwaukee", "WI")

	chicagoSibling := suite.createAlsoTonightShow("chicago-sibling", chicagoA.ID, siblingAt, catalogm.ShowStatusApproved)
	milwaukeeSibling := suite.createAlsoTonightShow("milwaukee-sibling", milwaukeeA.ID, siblingAt, catalogm.ShowStatusApproved)

	// "Cactus Club" < "Empty Bottle", so the Milwaukee room wins the name-ASC
	// pick regardless of which venue was billed first.
	subject := suite.createAlsoTonightShowAt("multi-venue",
		[]uint{chicagoA.ID, milwaukeeA.ID}, at, catalogm.ShowStatusApproved)

	rail, err := suite.sceneService.GetShowAlsoTonight(fmt.Sprint(subject.ID))
	suite.Require().NoError(err)
	suite.Equal("milwaukee-wi", rail.SceneSlug,
		"the scope must follow the name-ASC venue pick, matching GetSceneShowsInRange's DISTINCT ON")
	suite.Equal([]uint{milwaukeeSibling.ID}, suite.showIDs(fmt.Sprint(subject.ID)))
	suite.NotContains(suite.showIDs(fmt.Sprint(subject.ID)), chicagoSibling.ID)

	// The mirror image, so the assertion cannot be satisfied by "Milwaukee always
	// wins": with an alphabetically-earlier Chicago room the pick flips.
	//
	// Created LAST on purpose. It is the alphabetically first room but the highest
	// venue id, so name-order and id-order disagree — without that, a tiebreak
	// changed to `v.id ASC` on either side of the invariant would still pass.
	chicagoB := suite.createAlsoTonightVenue("Alpha Room", "Chicago", "IL")
	suite.Require().Greater(chicagoB.ID, chicagoA.ID)
	flipped := suite.createAlsoTonightShowAt("multi-venue-flipped",
		[]uint{chicagoB.ID, milwaukeeA.ID}, at, catalogm.ShowStatusApproved)

	flippedRail, err := suite.sceneService.GetShowAlsoTonight(fmt.Sprint(flipped.ID))
	suite.Require().NoError(err)
	suite.Equal("chicago-il", flippedRail.SceneSlug)
	suite.Contains(suite.showIDs(fmt.Sprint(flipped.ID)), chicagoSibling.ID)

	// The OTHER half of the invariant, and the only assertion that reads both
	// queries at once: when the multi-venue show appears as a ROW on someone
	// else's rail, GetSceneShowsInRange's own DISTINCT ON must have named the same
	// room this service picked. Asserting only the scope would leave scene.go's
	// tiebreak free to drift to `v.id` with nothing failing.
	rowRail, err := suite.sceneService.GetShowAlsoTonight(fmt.Sprint(chicagoSibling.ID))
	suite.Require().NoError(err)
	var flippedRow *contracts.SceneShowSummary
	for i := range rowRail.Shows {
		if rowRail.Shows[i].ID == flipped.ID {
			flippedRow = &rowRail.Shows[i]
		}
	}
	suite.Require().NotNil(flippedRow, "the flipped multi-venue show belongs to the Chicago night")
	suite.Equal("Alpha Room", flippedRow.VenueName,
		"the scene query must carry the SAME room the subject scope was computed from")
}

// The clock is the ROOM's own zone, not the metro's modal one, and the two can
// disagree: sceneLocation reads VERIFIED venues only and otherwise falls back to
// a one-zone-per-state map, so a room in a state that spans two zones can be
// misdated by the metro's answer. The date here names the subject's own night,
// so the room's own column has to win.
func (suite *SceneServiceIntegrationTestSuite) TestGetShowAlsoTonight_RoomZoneBeatsTheMetroModalClock() {
	mountain, err := time.LoadLocation("America/Denver")
	suite.Require().NoError(err)

	// Two verified El Paso rooms make the scene, both on Mountain time. The state
	// map calls TX America/Chicago, and these rooms are the reason that is wrong.
	subjectRoom := suite.seedVenue(alsoTonightVenue{
		name: "Lowbrow Palace", city: "El Paso", state: "TX", tz: "America/Denver",
	})
	sibling := suite.seedVenue(alsoTonightVenue{
		name: "Tricky Falls", city: "El Paso", state: "TX", tz: "America/Denver",
	})

	// 23:30 Mountain on the 18th is 00:30 Central on the 19th, so the two
	// candidate clocks file this show under different dates. That is the test.
	at := time.Date(2026, time.September, 18, 23, 30, 0, 0, mountain)
	subject := suite.createAlsoTonightShow("subject", subjectRoom.ID, at, catalogm.ShowStatusApproved)
	sameNight := suite.createAlsoTonightShow("same-night", sibling.ID,
		time.Date(2026, time.September, 18, 21, 0, 0, 0, mountain), catalogm.ShowStatusApproved)

	rail, err := suite.sceneService.GetShowAlsoTonight(fmt.Sprint(subject.ID))
	suite.Require().NoError(err)
	suite.Equal("America/Denver", zoneOf(rail.Timezone),
		"the room's own zone must decide the date, not the state map's answer for TX")
	suite.Equal("2026-09-18", rail.Date,
		"a 23:30 Mountain set belongs to the 18th; the Central reading would publish it as the 19th")
	suite.Equal([]uint{sameNight.ID}, suite.showIDs(fmt.Sprint(subject.ID)))
}

// A capped rail must say it is capped, or a client renders 20 rows as if that
// were the whole night. The cap+1 fetch already knows the answer.
func (suite *SceneServiceIntegrationTestSuite) TestGetShowAlsoTonight_ReportsWhenTheNightOverflowsTheCap() {
	loc := suite.alsoTonightLoc()
	chicago := suite.createAlsoTonightVenue("Empty Bottle", "Chicago", "IL")
	evanston := suite.createAlsoTonightVenue("Space", "Evanston", "IL")

	subject := suite.createAlsoTonightShow("subject", chicago.ID,
		time.Date(2026, time.September, 18, 18, 0, 0, 0, loc), catalogm.ShowStatusApproved)

	// Exactly the cap: full rail, nothing withheld.
	for i := 0; i < showAlsoTonightCap; i++ {
		suite.createAlsoTonightShow(fmt.Sprintf("sibling-%d", i), evanston.ID,
			time.Date(2026, time.September, 18, 19, i, 0, 0, loc), catalogm.ShowStatusApproved)
	}
	atCap, err := suite.sceneService.GetShowAlsoTonight(fmt.Sprint(subject.ID))
	suite.Require().NoError(err)
	suite.Len(atCap.Shows, showAlsoTonightCap)
	suite.Equal(showAlsoTonightCap, atCap.ShowCount)
	suite.False(atCap.HasMore, "a night with exactly the cap is complete, not truncated")

	// One more, and the rail has to admit it.
	suite.createAlsoTonightShow("sibling-overflow", evanston.ID,
		time.Date(2026, time.September, 18, 23, 0, 0, 0, loc), catalogm.ShowStatusApproved)
	overflowing, err := suite.sceneService.GetShowAlsoTonight(fmt.Sprint(subject.ID))
	suite.Require().NoError(err)
	suite.Len(overflowing.Shows, showAlsoTonightCap)
	suite.True(overflowing.HasMore, "the night held more than the rail can carry")
}

// The cap is applied by the QUERY, so the ordering has to be too: a night longer
// than the rail, ordered by the clock, drops exactly the late sets a reader can
// still get to. Twenty-five shows, five of them already started, and the rail
// holds twenty rows.
func (suite *SceneServiceIntegrationTestSuite) TestSceneShowsInRange_StartedRowsSinkBeforeTheCap() {
	loc := suite.alsoTonightLoc()
	chicago := suite.createAlsoTonightVenue("Empty Bottle", "Chicago", "IL")
	evanston := suite.createAlsoTonightVenue("Space", "Evanston", "IL")

	day := time.Date(2026, time.September, 18, 0, 0, 0, 0, loc)
	sinkStartedAt := day.Add(20 * time.Hour)

	var started, upcoming []uint
	for i := 0; i < 5; i++ {
		show := suite.createAlsoTonightShow(fmt.Sprintf("started-%d", i), chicago.ID,
			day.Add(18*time.Hour+time.Duration(i)*time.Minute), catalogm.ShowStatusApproved)
		started = append(started, show.ID)
	}
	for i := 0; i < 20; i++ {
		show := suite.createAlsoTonightShow(fmt.Sprintf("upcoming-%d", i), evanston.ID,
			day.Add(21*time.Hour+time.Duration(i)*time.Minute), catalogm.ShowStatusApproved)
		upcoming = append(upcoming, show.ID)
	}

	from, to := day.UTC(), day.AddDate(0, 0, 1).UTC()

	sunk, err := suite.sceneService.sceneShowsInRange(
		"Chicago", "IL", from, to, loc, showAlsoTonightCap, &sinkStartedAt)
	suite.Require().NoError(err)
	suite.Equal(upcoming, idsOf(sunk),
		"every show still to come must survive a cap that would otherwise spend rows on started ones")

	// Two rows more than there are upcoming shows: the started rows appear, in
	// their own clock order, after every upcoming one.
	withStarted, err := suite.sceneService.sceneShowsInRange(
		"Chicago", "IL", from, to, loc, len(upcoming)+2, &sinkStartedAt)
	suite.Require().NoError(err)
	suite.Equal(append(append([]uint{}, upcoming...), started[0], started[1]), idsOf(withStarted),
		"started rows keep clock order among themselves, below the upcoming ones")

	// No instant, no promotion: an archive or future night is read in the order it
	// happens, and the cap keeps the earliest rows.
	clockOrder, err := suite.sceneService.sceneShowsInRange(
		"Chicago", "IL", from, to, loc, showAlsoTonightCap, nil)
	suite.Require().NoError(err)
	suite.Equal(append(append([]uint{}, started...), upcoming[:15]...), idsOf(clockOrder),
		"without an instant the night stays earliest-first")
}

// The same rule through the rail itself, on a night that is live RIGHT NOW: the
// subject is a show that has already started, twenty-five others share its night,
// and every one of the twenty still to come has to be on the rail.
//
// The fixture's rooms are placed on a zone chosen from the wall clock rather than
// from geography, because the rail reads a show's night on the ROOM's own
// timezone column. Seeding both halves of a live night on one calendar date is
// otherwise only possible during part of the day, and the test would quietly stop
// asserting anything for the rest of it.
func (suite *SceneServiceIntegrationTestSuite) TestGetShowAlsoTonight_LiveNightPromotesEveryUpcomingShowOverTheCap() {
	loc, nowLocal := suite.liveNightZone()

	chicago := suite.seedVenue(alsoTonightVenue{
		name: "Empty Bottle", city: "Chicago", state: "IL", tz: loc.String(),
	})
	evanston := suite.seedVenue(alsoTonightVenue{
		name: "Space", city: "Evanston", state: "IL", tz: loc.String(),
	})

	// The subject is the LATEST of the started shows, so on a night this dense it
	// falls outside the rows the query returns. That is deliberate: it is what
	// separates asking the scene whether it lists this show from reading the
	// answer off rows the cap has already shortened.
	subject := suite.createAlsoTonightShow("live-subject", chicago.ID,
		nowLocal.Add(-1*time.Minute), catalogm.ShowStatusApproved)
	for i := 2; i <= 6; i++ {
		suite.createAlsoTonightShow(fmt.Sprintf("live-started-%d", i), chicago.ID,
			nowLocal.Add(-time.Duration(i)*time.Minute), catalogm.ShowStatusApproved)
	}
	var upcoming []uint
	for i := 0; i < showAlsoTonightCap; i++ {
		show := suite.createAlsoTonightShow(fmt.Sprintf("live-upcoming-%d", i), evanston.ID,
			nowLocal.Add(time.Duration(2+i)*time.Minute), catalogm.ShowStatusApproved)
		upcoming = append(upcoming, show.ID)
	}

	rail, err := suite.sceneService.GetShowAlsoTonight(fmt.Sprint(subject.ID))
	suite.Require().NoError(err)
	suite.True(rail.IsTonight, "the fixture is seeded on the scene's own live night")
	suite.Equal(upcoming, idsOf(rail.Shows),
		"the cap must not spend a row on a set that has already started while one still to come is dropped")
	suite.True(rail.HasMore, "the night held more than the rail can carry")
	suite.Equal("chicago-il", rail.SceneSlug,
		"the link asks whether the scene lists this show, not whether the capped rows happened to include it")
}

// "Tonight" is not "Date == today": until 06:00 local a night is still named by
// the date it began on. The server answers it because it depends on the scene's
// clock, not the reader's device.
func (suite *SceneServiceIntegrationTestSuite) TestGetShowAlsoTonight_IsTonightUsesTheSceneClockNotTheDate() {
	loc := suite.alsoTonightLoc()
	chicago := suite.createAlsoTonightVenue("Empty Bottle", "Chicago", "IL")
	suite.createAlsoTonightVenue("Space", "Evanston", "IL")

	nowLocal := time.Now().In(loc)
	tonight := tonightDate(nowLocal)

	onTonight := suite.createAlsoTonightShow("tonight",
		chicago.ID, tonight.start(loc).Add(20*time.Hour), catalogm.ShowStatusApproved)
	nextWeek := suite.createAlsoTonightShow("next-week",
		chicago.ID, tonight.addDays(7).start(loc).Add(20*time.Hour), catalogm.ShowStatusApproved)

	tonightRail, err := suite.sceneService.GetShowAlsoTonight(fmt.Sprint(onTonight.ID))
	suite.Require().NoError(err)
	suite.Equal(tonight.String(), tonightRail.Date)
	suite.True(tonightRail.IsTonight)

	futureRail, err := suite.sceneService.GetShowAlsoTonight(fmt.Sprint(nextWeek.ID))
	suite.Require().NoError(err)
	suite.False(futureRail.IsTonight, "a show a week out is not tonight, whatever the reader's clock says")
}

// The scope comes from the geocoder, the rows come from the venues.metro column,
// and a manually-run backfill maintains that column. When the subject's own room
// is missing from it, the scene-day page for this date does NOT list the show the
// reader came from, so the link must be withheld rather than misleading them.
func (suite *SceneServiceIntegrationTestSuite) TestGetShowAlsoTonight_WithholdsTheLinkWhenTheSubjectIsNotOnItsOwnScenePage() {
	phx, err := time.LoadLocation("America/Phoenix")
	suite.Require().NoError(err)
	at := time.Date(2026, time.September, 18, 20, 0, 0, 0, phx)

	mesa := suite.seedVenue(alsoTonightVenue{
		name: "Nile Theater", city: "Mesa", state: "AZ", tz: "America/Phoenix", noMetro: true,
	})
	crescent := suite.seedVenue(alsoTonightVenue{
		name: "Crescent Ballroom", city: "Phoenix", state: "AZ", tz: "America/Phoenix",
	})
	valley := suite.seedVenue(alsoTonightVenue{
		name: "Valley Bar", city: "Phoenix", state: "AZ", tz: "America/Phoenix",
	})

	orphan := suite.createAlsoTonightShow("orphan", mesa.ID, at, catalogm.ShowStatusApproved)
	inColumn := suite.createAlsoTonightShow("in-column", crescent.ID, at.Add(time.Hour), catalogm.ShowStatusApproved)
	suite.createAlsoTonightShow("also-in-column", valley.ID, at.Add(2*time.Hour), catalogm.ShowStatusApproved)

	orphanRail, err := suite.sceneService.GetShowAlsoTonight(fmt.Sprint(orphan.ID))
	suite.Require().NoError(err)
	suite.NotEmpty(orphanRail.Shows, "the rail itself is still real and still useful")
	suite.Equal("Phoenix", orphanRail.City, "the metro is still named")
	suite.Empty(orphanRail.SceneSlug,
		"the scene-day page would not list this show, so it must not be linked")

	// A show whose room IS in the column gets the link, proving the guard is not
	// simply always-off.
	linkedRail, err := suite.sceneService.GetShowAlsoTonight(fmt.Sprint(inColumn.ID))
	suite.Require().NoError(err)
	suite.Equal("phoenix-az", linkedRail.SceneSlug)
}

// The Go guard and the SQL demotion must agree on what "no usable place" means.
// Nothing trims venue city/state on write, and a whitespace-only city folds to
// the empty geocoder key, producing a scope that matches every blank-city room in
// the state plus a "---il" slug that resolves to nothing.
func (suite *SceneServiceIntegrationTestSuite) TestGetShowAlsoTonight_WhitespaceOnlyPlaceIsTreatedAsNoPlace() {
	loc := suite.alsoTonightLoc()
	at := time.Date(2026, time.September, 18, 20, 0, 0, 0, loc)

	// Two blank-city IL rooms: enough to clear the scene threshold if the blank
	// bucket were ever allowed to become a scope.
	blankA := suite.createAlsoTonightVenue("Blank One", "Chicago", "IL")
	blankB := suite.createAlsoTonightVenue("Blank Two", "Chicago", "IL")
	suite.Require().NoError(suite.db.Model(blankA).Update("city", "   ").Error)
	suite.Require().NoError(suite.db.Model(blankB).Update("city", "").Error)

	subject := suite.createAlsoTonightShow("subject", blankA.ID, at, catalogm.ShowStatusApproved)
	suite.createAlsoTonightShow("other-blank", blankB.ID, at.Add(time.Hour), catalogm.ShowStatusApproved)

	rail, err := suite.sceneService.GetShowAlsoTonight(fmt.Sprint(subject.ID))
	suite.Require().NoError(err)
	suite.Empty(rail.Shows, "a whitespace-only city is no place, not a scope full of other placeless rooms")
	suite.Empty(rail.SceneSlug, "and it must never emit a slug like \"---il\"")
	suite.Empty(rail.City)
}

// A room with no usable place must not win the venue pick. venues.city/state
// are nullable, so an alphabetically-early "A Secret Location" on a bill that
// also names a real room would otherwise take the LIMIT 1 and hand a perfectly
// scopeable show an empty rail. The scene query never has this problem: its
// venue predicate is in the WHERE, so its DISTINCT ON only ever ranks rooms that
// are already in scope.
func (suite *SceneServiceIntegrationTestSuite) TestGetShowAlsoTonight_UnplaceableRoomLosesTheVenuePick() {
	loc := suite.alsoTonightLoc()
	at := time.Date(2026, time.September, 18, 20, 0, 0, 0, loc)

	chicago := suite.createAlsoTonightVenue("Empty Bottle", "Chicago", "IL")
	evanston := suite.createAlsoTonightVenue("Space", "Evanston", "IL")
	// Sorts before "Empty Bottle", and has no place at all.
	secret := suite.createAlsoTonightVenue("A Secret Location", "Chicago", "IL")
	suite.Require().NoError(suite.db.Model(secret).Updates(map[string]any{"city": "", "state": ""}).Error)

	sibling := suite.createAlsoTonightShow("sibling", evanston.ID,
		time.Date(2026, time.September, 18, 21, 0, 0, 0, loc), catalogm.ShowStatusApproved)
	subject := suite.createAlsoTonightShowAt("subject", []uint{secret.ID, chicago.ID}, at, catalogm.ShowStatusApproved)

	rail, err := suite.sceneService.GetShowAlsoTonight(fmt.Sprint(subject.ID))
	suite.Require().NoError(err)
	suite.Equal("chicago-il", rail.SceneSlug,
		"an unplaceable room must be demoted so the bill's real room decides the scope")
	suite.Equal([]uint{sibling.ID}, suite.showIDs(fmt.Sprint(subject.ID)))
}

// SceneSlug is a link, and the scene-day surface serves a bounded window of
// dates. An archive show has a real scene and a real date but no page to point
// at, so the slug is withheld rather than advertising a 404. The display
// identity stays, because naming the metro is true either way.
func (suite *SceneServiceIntegrationTestSuite) TestGetShowAlsoTonight_WithholdsTheLinkForANonServableDate() {
	loc := suite.alsoTonightLoc()
	chicago := suite.createAlsoTonightVenue("Empty Bottle", "Chicago", "IL")
	evanston := suite.createAlsoTonightVenue("Space", "Evanston", "IL")

	// Before sceneFirstTrackedYear, so /scenes/{slug}/day/{date} would refuse it.
	archive := time.Date(sceneFirstTrackedYear-1, time.September, 18, 20, 0, 0, 0, loc)
	subject := suite.createAlsoTonightShow("archive-subject", chicago.ID, archive, catalogm.ShowStatusApproved)
	sibling := suite.createAlsoTonightShow("archive-sibling", evanston.ID,
		archive.Add(time.Hour), catalogm.ShowStatusApproved)

	rail, err := suite.sceneService.GetShowAlsoTonight(fmt.Sprint(subject.ID))
	suite.Require().NoError(err)
	suite.Empty(rail.SceneSlug, "no scene-day page exists for this date, so no link may be offered")
	suite.Equal("Chicago", rail.City, "the metro is still named; only the link is withheld")
	suite.Equal("Chicago, IL", rail.SceneName)
	suite.Equal(fmt.Sprintf("%d-09-18", sceneFirstTrackedYear-1), rail.Date)
	// The rail itself is still real. The rows are what the reader came for.
	suite.Equal([]uint{sibling.ID}, suite.showIDs(fmt.Sprint(subject.ID)))
}

// Both no-scene branches must date the show on the ROOM's own zone. The state
// map defaults every unrecognized state to America/Phoenix, and sceneLocation
// only reads VERIFIED venues, so a room that reaches either branch would
// otherwise have its date computed on Arizona's clock: a 01:00 Berlin set would
// be published under the previous date by the one field whose entire job is to
// name the right one.
//
// 01:00 on the 19th in Berlin is still the 18th in Phoenix, so the two candidate
// zones disagree about the date. That disagreement is what these assert.
func (suite *SceneServiceIntegrationTestSuite) TestGetShowAlsoTonight_NoSceneBranchesDateOnTheVenuesOwnZone() {
	berlinLoc, err := time.LoadLocation("Europe/Berlin")
	suite.Require().NoError(err)
	at := time.Date(2026, time.September, 19, 1, 0, 0, 0, berlinLoc)

	suite.Run("below-threshold place", func() {
		// A real city/state, so this takes the scope branch and fails the scene
		// threshold. sceneLocation finds nothing (the room is unverified), which is
		// exactly when the venue column has to win.
		berlin := suite.seedVenue(alsoTonightVenue{
			name: "Berghain", city: "Berlin", state: "BE", tz: "Europe/Berlin", unverified: true,
		})
		suite.Require().Nil(berlin.Metro, "a non-US place pins no CBSA")
		subject := suite.createAlsoTonightShow("berlin-subject", berlin.ID, at, catalogm.ShowStatusApproved)

		rail, err := suite.sceneService.GetShowAlsoTonight(fmt.Sprint(subject.ID))
		suite.Require().NoError(err)
		suite.Equal("Europe/Berlin", zoneOf(rail.Timezone),
			"the venue's own zone must win; the state map would have said America/Phoenix")
		suite.Equal("2026-09-19", rail.Date)
		suite.Empty(rail.Shows)
	})

	suite.TearDownTest()

	suite.Run("room with no usable place at all", func() {
		// Blank city/state short-circuits before any scope is computed, so the
		// timezone column is the only signal left.
		placeless := suite.seedVenue(alsoTonightVenue{
			name: "Unlisted", city: "Berlin", state: "BE", tz: "Europe/Berlin",
		})
		suite.Require().NoError(suite.db.Model(placeless).
			Updates(map[string]any{"city": "", "state": ""}).Error)
		subject := suite.createAlsoTonightShow("placeless-subject", placeless.ID, at, catalogm.ShowStatusApproved)

		rail, err := suite.sceneService.GetShowAlsoTonight(fmt.Sprint(subject.ID))
		suite.Require().NoError(err)
		suite.Equal("Europe/Berlin", zoneOf(rail.Timezone))
		suite.Equal("2026-09-19", rail.Date)
		suite.Empty(rail.SceneSlug)
	})
}

// A room with no stored zone whose state is outside the US map is the case the
// fallback used to answer with America/Phoenix. The rail now publishes no zone
// at all, so a client cannot mistake the surrender for an answer and print an
// hour on it.
func (suite *SceneServiceIntegrationTestSuite) TestGetShowAlsoTonight_WithholdsTheZoneItCannotName() {
	at := time.Date(2026, time.September, 19, 1, 0, 0, 0, time.UTC)

	suite.Run("non-US state, no timezone column", func() {
		windmill := suite.seedVenue(alsoTonightVenue{
			name: "The Windmill", city: "London", state: "England", unverified: true,
		})
		subject := suite.createAlsoTonightShow("windmill-subject", windmill.ID, at, catalogm.ShowStatusApproved)

		rail, err := suite.sceneService.GetShowAlsoTonight(fmt.Sprint(subject.ID))
		suite.Require().NoError(err)
		suite.Nil(rail.Timezone, "England is not in the US map, so no zone is nameable")
		// 01:00Z on the 19th is 18:00 on the 18th in the fallback, which is the
		// day the rail still publishes.
		suite.Equal("2026-09-18", rail.Date)
	})

	suite.TearDownTest()

	suite.Run("blank state, no timezone column", func() {
		unknown := suite.seedVenue(alsoTonightVenue{
			name: "Hall Ohne Zone", city: "Berlin", state: "BE", unverified: true,
		})
		suite.Require().NoError(suite.db.Model(unknown).Update("state", "").Error)
		subject := suite.createAlsoTonightShow("zoneless-subject", unknown.ID, at, catalogm.ShowStatusApproved)

		rail, err := suite.sceneService.GetShowAlsoTonight(fmt.Sprint(subject.ID))
		suite.Require().NoError(err)
		suite.Nil(rail.Timezone)
	})

	suite.TearDownTest()

	suite.Run("a US state alone still names a zone", func() {
		rebel := suite.seedVenue(alsoTonightVenue{
			name: "The Rebel Lounge", city: "Phoenix", state: "AZ", unverified: true,
		})
		subject := suite.createAlsoTonightShow("az-subject", rebel.ID, at, catalogm.ShowStatusApproved)

		rail, err := suite.sceneService.GetShowAlsoTonight(fmt.Sprint(subject.ID))
		suite.Require().NoError(err)
		suite.Equal("America/Phoenix", zoneOf(rail.Timezone),
			"the state map is an answer, and only the surrender to it is withheld")
	})
}

// The scope's DISPLAY identity is the METRO's principal city, not the subject
// venue's. Every other fixture here happens to sit in Chicago proper, where the
// two coincide and metroDisplayIdentity could be the identity function without
// anything noticing; an Evanston subject is what makes it load-bearing, and the
// emitted slug is the "see all" link's target.
func (suite *SceneServiceIntegrationTestSuite) TestGetShowAlsoTonight_DisplayIdentityIsTheMetroPrincipalCity() {
	loc := suite.alsoTonightLoc()
	evanston := suite.createAlsoTonightVenue("Space", "Evanston", "IL")
	chicago := suite.createAlsoTonightVenue("Empty Bottle", "Chicago", "IL")

	subject := suite.createAlsoTonightShow("subject", evanston.ID,
		time.Date(2026, time.September, 18, 20, 0, 0, 0, loc), catalogm.ShowStatusApproved)
	sibling := suite.createAlsoTonightShow("sibling", chicago.ID,
		time.Date(2026, time.September, 18, 21, 0, 0, 0, loc), catalogm.ShowStatusApproved)

	rail, err := suite.sceneService.GetShowAlsoTonight(fmt.Sprint(subject.ID))
	suite.Require().NoError(err)
	suite.Equal("chicago-il", rail.SceneSlug, "an Evanston show belongs to the Chicago scene")
	suite.Equal("Chicago", rail.City)
	suite.Equal("Chicago, IL", rail.SceneName)
	suite.Equal([]uint{sibling.ID}, suite.showIDs(fmt.Sprint(subject.ID)))
}

// The subject's scope comes from the GEOCODER (city, state), but the rail's rows
// come from the metro PREDICATE (`v.metro = ?`). The two read different sources,
// and this pins what that means in both directions:
//
//   - A subject at a room the metro backfill never reached still gets its metro's
//     rail. This is the case the literal reading of "venue with no metro => empty"
//     would have broken.
//   - A SIBLING at such a room is invisible to the rail, because the predicate is
//     a column read. That is inherited from GetSceneShowsInRange and is therefore
//     identical to what /scenes/{slug}/day shows. Pinned so it stays a shared
//     property of the scene query rather than becoming a quiet divergence.
func (suite *SceneServiceIntegrationTestSuite) TestGetShowAlsoTonight_NullMetroColumnScopesBySubjectCityButRowsFollowTheColumn() {
	phx, err := time.LoadLocation("America/Phoenix")
	suite.Require().NoError(err)
	at := time.Date(2026, time.September, 18, 20, 0, 0, 0, phx)
	siblingAt := time.Date(2026, time.September, 18, 21, 0, 0, 0, phx)

	mesa := suite.seedVenue(alsoTonightVenue{
		name: "Nile Theater", city: "Mesa", state: "AZ", tz: "America/Phoenix", noMetro: true,
	})
	suite.Require().Nil(mesa.Metro)
	crescent := suite.seedVenue(alsoTonightVenue{
		name: "Crescent Ballroom", city: "Phoenix", state: "AZ", tz: "America/Phoenix",
	})
	valley := suite.seedVenue(alsoTonightVenue{
		name: "Valley Bar", city: "Phoenix", state: "AZ", tz: "America/Phoenix",
	})
	suite.Require().NotNil(crescent.Metro)
	tempeNoMetro := suite.seedVenue(alsoTonightVenue{
		name: "Yucca Tap Room", city: "Tempe", state: "AZ", tz: "America/Phoenix", noMetro: true,
	})

	subject := suite.createAlsoTonightShow("subject", mesa.ID, at, catalogm.ShowStatusApproved)
	phoenixSibling := suite.createAlsoTonightShow("phoenix-sibling", crescent.ID, siblingAt, catalogm.ShowStatusApproved)
	suite.createAlsoTonightShow("valley-sibling", valley.ID, siblingAt.Add(time.Minute), catalogm.ShowStatusApproved)
	unreachable := suite.createAlsoTonightShow("tempe-null-metro", tempeNoMetro.ID, siblingAt, catalogm.ShowStatusApproved)

	rail, err := suite.sceneService.GetShowAlsoTonight(fmt.Sprint(subject.ID))
	suite.Require().NoError(err)
	suite.Equal("Phoenix", rail.City,
		"a Mesa room with no metro column still scopes to Phoenix via the geocoder")
	suite.Equal("Phoenix, AZ", rail.SceneName)
	// The LINK is withheld here even though the scope resolved, because the
	// subject's own room is missing from venues.metro and the scene-day page for
	// this date therefore would not list it. Pinned in its own test; asserted here
	// so this fixture cannot silently start advertising it.
	suite.Empty(rail.SceneSlug)
	ids := suite.showIDs(fmt.Sprint(subject.ID))
	suite.Contains(ids, phoenixSibling.ID)
	suite.NotContains(ids, unreachable.ID,
		"the row predicate is a column read, so a null-metro sibling is invisible here exactly as it is on the scene-day page")
}

// The rail LISTS shows rather than linking to one, so it carries the status
// flags and filters neither. A cancelled show that silently vanished would make
// the rail disagree with the scene-day page, and one rendered identically to a
// live show would be worse than omitting it, which is why the flags travel.
func (suite *SceneServiceIntegrationTestSuite) TestGetShowAlsoTonight_CarriesCancelledAndSoldOutRatherThanFilteringThem() {
	loc := suite.alsoTonightLoc()
	chicago := suite.createAlsoTonightVenue("Empty Bottle", "Chicago", "IL")
	evanston := suite.createAlsoTonightVenue("Space", "Evanston", "IL")

	subject := suite.createAlsoTonightShow("subject", chicago.ID,
		time.Date(2026, time.September, 18, 20, 0, 0, 0, loc), catalogm.ShowStatusApproved)
	cancelled := suite.createAlsoTonightShow("cancelled", evanston.ID,
		time.Date(2026, time.September, 18, 21, 0, 0, 0, loc), catalogm.ShowStatusApproved)
	soldOut := suite.createAlsoTonightShow("sold-out", evanston.ID,
		time.Date(2026, time.September, 18, 22, 0, 0, 0, loc), catalogm.ShowStatusApproved)
	suite.Require().NoError(suite.db.Model(cancelled).Update("is_cancelled", true).Error)
	suite.Require().NoError(suite.db.Model(soldOut).Update("is_sold_out", true).Error)

	rail, err := suite.sceneService.GetShowAlsoTonight(fmt.Sprint(subject.ID))
	suite.Require().NoError(err)
	suite.Require().Len(rail.Shows, 2, "neither flag removes a show from the rail")
	suite.True(rail.Shows[0].IsCancelled)
	suite.False(rail.Shows[0].IsSoldOut)
	suite.True(rail.Shows[1].IsSoldOut)
	suite.False(rail.Shows[1].IsCancelled)
}

// A slug made entirely of digits must keep meaning the ID, as it does on every
// other /shows/{show_id} route. The two lookups are split precisely so they
// cannot cross-match.
func (suite *SceneServiceIntegrationTestSuite) TestGetShowAlsoTonight_NumericAddressAlwaysMeansTheID() {
	loc := suite.alsoTonightLoc()
	at := time.Date(2026, time.September, 18, 20, 0, 0, 0, loc)

	chicago := suite.createAlsoTonightVenue("Empty Bottle", "Chicago", "IL")
	suite.createAlsoTonightVenue("Space", "Evanston", "IL")
	milwaukeeA := suite.createAlsoTonightVenue("Cactus Club", "Milwaukee", "WI")
	suite.createAlsoTonightVenue("Shank Hall", "Milwaukee", "WI")

	byID := suite.createAlsoTonightShow("by-id", chicago.ID, at, catalogm.ShowStatusApproved)

	// A DIFFERENT show whose slug is the first show's id, in another metro so the
	// two answers are distinguishable.
	impostorSlug := fmt.Sprint(byID.ID)
	impostor := &catalogm.Show{
		Title:     "impostor",
		Slug:      &impostorSlug,
		EventDate: at.UTC(),
		Status:    catalogm.ShowStatusApproved,
	}
	suite.Require().NoError(suite.db.Create(impostor).Error)
	suite.Require().NoError(suite.db.Create(&catalogm.ShowVenue{ShowID: impostor.ID, VenueID: milwaukeeA.ID}).Error)

	rail, err := suite.sceneService.GetShowAlsoTonight(impostorSlug)
	suite.Require().NoError(err)
	suite.Equal("chicago-il", rail.SceneSlug,
		"a numeric address must resolve the show with that ID, never the show with that slug")
}

// A show page must never be broken by a rail that has nothing to say. A room
// whose place does not resolve to a scene we track answers with an empty rail at
// 200, and withholds the scene identity so no client renders a "see all" link to
// a page that would 404.
func (suite *SceneServiceIntegrationTestSuite) TestGetShowAlsoTonight_UnscopeableVenueIsEmptyNotAnError() {
	loc := suite.alsoTonightLoc()
	// Marfa pins no CBSA, and one verified room is below the scene threshold.
	//
	// Its timezone column deliberately DISAGREES with the TX state map (Texas
	// spans two zones, so this is a real row shape). That is what makes the
	// timezone assertion below load-bearing: it can only pass if the venue's own
	// column won, not the state fallback.
	marfa := suite.seedVenue(alsoTonightVenue{
		name: "Capri", city: "Marfa", state: "TX", tz: "America/Denver",
	})
	suite.Require().Nil(marfa.Metro)
	chicago := suite.createAlsoTonightVenue("Empty Bottle", "Chicago", "IL")

	subject := suite.createAlsoTonightShow("subject", marfa.ID,
		time.Date(2026, time.September, 18, 20, 0, 0, 0, loc), catalogm.ShowStatusApproved)
	suite.createAlsoTonightShow("elsewhere", chicago.ID,
		time.Date(2026, time.September, 18, 20, 0, 0, 0, loc), catalogm.ShowStatusApproved)

	rail, err := suite.sceneService.GetShowAlsoTonight(fmt.Sprint(subject.ID))
	suite.Require().NoError(err)
	suite.Empty(rail.Shows)
	suite.NotNil(rail.Shows, "an empty rail must marshal as [] rather than null")
	suite.Equal(0, rail.ShowCount)
	suite.Empty(rail.SceneSlug)
	suite.Empty(rail.SceneName)
	suite.Empty(rail.City)
	suite.Empty(rail.State)
	// The date is still the answer to a real question, so it is still served.
	suite.Equal("2026-09-18", rail.Date)
	suite.Equal("America/Denver", zoneOf(rail.Timezone))
}

// The counterpart to the case above, and the reason "no metro" is not by itself
// the empty condition: a no-CBSA city that DOES qualify as a scene is scoped by
// its literal (city, state) — the same fallback every other scene surface uses —
// and gets a real rail.
func (suite *SceneServiceIntegrationTestSuite) TestGetShowAlsoTonight_NoCBSACityWithAQualifyingSceneStillGetsARail() {
	loc := suite.alsoTonightLoc()
	capri := suite.createAlsoTonightVenue("Capri", "Marfa", "TX")
	planet := suite.createAlsoTonightVenue("Planet Marfa", "Marfa", "TX")
	suite.Require().Nil(capri.Metro)

	subject := suite.createAlsoTonightShow("subject", capri.ID,
		time.Date(2026, time.September, 18, 20, 0, 0, 0, loc), catalogm.ShowStatusApproved)
	sibling := suite.createAlsoTonightShow("sibling", planet.ID,
		time.Date(2026, time.September, 18, 22, 0, 0, 0, loc), catalogm.ShowStatusApproved)

	rail, err := suite.sceneService.GetShowAlsoTonight(fmt.Sprint(subject.ID))
	suite.Require().NoError(err)
	suite.Equal([]uint{sibling.ID}, suite.showIDs(fmt.Sprint(subject.ID)))
	suite.Equal("marfa-tx", rail.SceneSlug)
}

// A bill with no venue has no place to scope by at all — still a 200, still an
// answer, never a 404.
func (suite *SceneServiceIntegrationTestSuite) TestGetShowAlsoTonight_ShowWithNoVenueIsEmptyNotAnError() {
	slug := "also-tonight-venueless"
	subject := &catalogm.Show{
		Title:     "venueless",
		Slug:      &slug,
		EventDate: time.Date(2026, time.September, 18, 20, 0, 0, 0, time.UTC),
		Status:    catalogm.ShowStatusApproved,
	}
	suite.Require().NoError(suite.db.Create(subject).Error)

	rail, err := suite.sceneService.GetShowAlsoTonight(fmt.Sprint(subject.ID))
	suite.Require().NoError(err)
	suite.Empty(rail.Shows)
	suite.NotNil(rail.Shows, "an empty rail must marshal as [] rather than null")
	suite.Equal(0, rail.ShowCount)
	suite.Empty(rail.SceneSlug)
	// With no venue there is no state either, so nothing names a zone and the
	// rail publishes none. The DATE is still served, read on the fallback, where
	// 20:00 UTC is still the 18th: a fallback day is the best available answer,
	// and it is the hour built on the same fallback that is refused.
	suite.Nil(rail.Timezone)
	suite.Equal("2026-09-18", rail.Date)
}

// Unknown and non-approved must be INDISTINGUISHABLE. This surface is anonymous,
// so a different answer for a hidden show would confirm that its id is real.
func (suite *SceneServiceIntegrationTestSuite) TestGetShowAlsoTonight_UnknownAndHiddenShowsAreBothNotFound() {
	loc := suite.alsoTonightLoc()
	chicago := suite.createAlsoTonightVenue("Empty Bottle", "Chicago", "IL")
	pending := suite.createAlsoTonightShow("pending", chicago.ID,
		time.Date(2026, time.September, 18, 20, 0, 0, 0, loc), catalogm.ShowStatusPending)

	for _, idOrSlug := range []string{
		"999999",
		"no-such-show",
		fmt.Sprint(pending.ID),
		*pending.Slug,
	} {
		_, err := suite.sceneService.GetShowAlsoTonight(idOrSlug)
		suite.Require().Error(err, "GetShowAlsoTonight(%q) should not resolve", idOrSlug)
		var showErr *apperrors.ShowError
		suite.Require().ErrorAs(err, &showErr)
		suite.Equal(apperrors.CodeShowNotFound, showErr.Code)
	}
}

// The rail hangs off /shows/{show_id}, which every sibling addresses by id OR
// slug. A rail reachable by only one of the two would break whichever form the
// page happens to hold.
func (suite *SceneServiceIntegrationTestSuite) TestGetShowAlsoTonight_ResolvesByIDOrSlug() {
	loc := suite.alsoTonightLoc()
	chicago := suite.createAlsoTonightVenue("Empty Bottle", "Chicago", "IL")
	evanston := suite.createAlsoTonightVenue("Space", "Evanston", "IL")

	subject := suite.createAlsoTonightShow("subject", chicago.ID,
		time.Date(2026, time.September, 18, 20, 0, 0, 0, loc), catalogm.ShowStatusApproved)
	sibling := suite.createAlsoTonightShow("sibling", evanston.ID,
		time.Date(2026, time.September, 18, 21, 0, 0, 0, loc), catalogm.ShowStatusApproved)

	suite.Equal([]uint{sibling.ID}, suite.showIDs(fmt.Sprint(subject.ID)))
	suite.Equal([]uint{sibling.ID}, suite.showIDs(*subject.Slug))
}

// The cap is fetched one over so that dropping the subject cannot silently cost
// the rail a row — the failure this pins is an off-by-one that only ever shows
// up on a night busier than the cap.
func (suite *SceneServiceIntegrationTestSuite) TestGetShowAlsoTonight_CapsTheRailWithoutLosingARowToSelfExclusion() {
	loc := suite.alsoTonightLoc()
	chicago := suite.createAlsoTonightVenue("Empty Bottle", "Chicago", "IL")
	evanston := suite.createAlsoTonightVenue("Space", "Evanston", "IL")

	// The subject goes FIRST in the night's ordering, so a naive cap-sized fetch
	// would spend one of its rows on the show being excluded.
	subject := suite.createAlsoTonightShow("subject", chicago.ID,
		time.Date(2026, time.September, 18, 18, 0, 0, 0, loc), catalogm.ShowStatusApproved)
	siblings := make([]uint, 0, showAlsoTonightCap+2)
	for i := 0; i < showAlsoTonightCap+2; i++ {
		show := suite.createAlsoTonightShow(fmt.Sprintf("sibling-%d", i), evanston.ID,
			time.Date(2026, time.September, 18, 19, i, 0, 0, loc), catalogm.ShowStatusApproved)
		siblings = append(siblings, show.ID)
	}

	// Identity and ORDER, not merely length: the cap keeps the SOONEST rows, so a
	// reversed ordering would still return the right count while showing the
	// reader the wrong end of the night. That premise is also what makes the
	// off-by-one assertion mean anything.
	suite.Equal(siblings[:showAlsoTonightCap], suite.showIDs(fmt.Sprint(subject.ID)),
		"the rail must be the first %d siblings of the night, earliest first", showAlsoTonightCap)
	suite.NotContains(suite.showIDs(fmt.Sprint(subject.ID)), subject.ID)
}

// The row query filters on the metro predicate and approval, NOT on venue
// verification: only scene EXISTENCE counts verified rooms. So a show at an
// unverified room is on the rail, with its street address redacted, exactly as
// it is on the scene-day page. Pinned because the two rules are easy to conflate.
func (suite *SceneServiceIntegrationTestSuite) TestGetShowAlsoTonight_IncludesUnverifiedRoomsButRedactsTheirAddress() {
	loc := suite.alsoTonightLoc()
	chicago := suite.createAlsoTonightVenue("Empty Bottle", "Chicago", "IL")
	suite.createAlsoTonightVenue("Space", "Evanston", "IL")

	address := "123 Basement St"
	house := suite.seedVenue(alsoTonightVenue{
		name: "The Basement", city: "Chicago", state: "IL", tz: alsoTonightZone, unverified: true,
	})
	suite.Require().NoError(suite.db.Model(house).Update("address", address).Error)
	suite.Require().NoError(suite.db.Model(house).Update("age_policy", "all ages").Error)

	subject := suite.createAlsoTonightShow("subject", chicago.ID,
		time.Date(2026, time.September, 18, 20, 0, 0, 0, loc), catalogm.ShowStatusApproved)
	diy := suite.createAlsoTonightShow("diy", house.ID,
		time.Date(2026, time.September, 18, 21, 0, 0, 0, loc), catalogm.ShowStatusApproved)

	rail, err := suite.sceneService.GetShowAlsoTonight(fmt.Sprint(subject.ID))
	suite.Require().NoError(err)
	suite.Equal([]uint{diy.ID}, suite.showIDs(fmt.Sprint(subject.ID)))
	suite.Require().Len(rail.Shows, 1)
	suite.Equal("The Basement", rail.Shows[0].VenueName)
	suite.Empty(rail.Shows[0].VenueAddress,
		"an unverified room's street address must never be published, here as anywhere else")
	suite.Equal("all ages", rail.Shows[0].VenueAgePolicy,
		"the house age policy is NOT address-gated: it is served for unverified rooms here exactly as the venue payload serves it")
}

// The rail row carries BOTH halves of the age answer: the event's own override
// and the room's house default. Either one alone is wrong on a real slice of
// rows, so a row that inherits the house policy and a row that overrides it are
// asserted together.
func (suite *SceneServiceIntegrationTestSuite) TestGetShowAlsoTonight_CarriesEventAgeOverrideAndHouseDefault() {
	loc := suite.alsoTonightLoc()
	chicago := suite.createAlsoTonightVenue("Empty Bottle", "Chicago", "IL")
	house := suite.createAlsoTonightVenue("Sleeping Village", "Chicago", "IL")
	suite.Require().NoError(suite.db.Model(house).Update("age_policy", "21+").Error)

	subject := suite.createAlsoTonightShow("subject", chicago.ID,
		time.Date(2026, time.September, 18, 20, 0, 0, 0, loc), catalogm.ShowStatusApproved)
	inherits := suite.createAlsoTonightShow("inherits", house.ID,
		time.Date(2026, time.September, 18, 21, 0, 0, 0, loc), catalogm.ShowStatusApproved)
	overrides := suite.createAlsoTonightShow("overrides", house.ID,
		time.Date(2026, time.September, 18, 22, 0, 0, 0, loc), catalogm.ShowStatusApproved)
	suite.Require().NoError(suite.db.Model(overrides).Update("age_requirement", "all ages").Error)

	rail, err := suite.sceneService.GetShowAlsoTonight(fmt.Sprint(subject.ID))
	suite.Require().NoError(err)
	suite.Require().Equal([]uint{inherits.ID, overrides.ID}, suite.showIDs(fmt.Sprint(subject.ID)))
	suite.Require().Len(rail.Shows, 2)

	suite.Empty(rail.Shows[0].AgeRequirement, "a show with no override of its own states none")
	suite.Equal("21+", rail.Shows[0].VenueAgePolicy)
	suite.Equal("all ages", rail.Shows[1].AgeRequirement)
	suite.Equal("21+", rail.Shows[1].VenueAgePolicy,
		"the house default travels alongside the override rather than being replaced by it")
}

// idsOf reads the ids off a run of rail rows, so an ordering assertion reads as
// the sequence it is about.
func idsOf(shows []contracts.SceneShowSummary) []uint {
	ids := make([]uint, 0, len(shows))
	for _, show := range shows {
		ids = append(ids, show.ID)
	}
	return ids
}

// liveNightZone is a real IANA zone in which the current instant sits well
// inside one calendar date, with that instant. A live-night fixture needs both
// started and still-to-come shows on ONE date, past the 06:00 night boundary;
// picking the zone from the clock rather than the clock from the zone is what
// makes such a test mean the same thing at every hour the suite might run.
//
// Four zones roughly six hours apart cover the 08:00-21:59 window from any UTC
// instant, since the window is wider than the widest gap between them.
func (suite *SceneServiceIntegrationTestSuite) liveNightZone() (*time.Location, time.Time) {
	now := time.Now()
	for _, name := range []string{"America/Chicago", "UTC", "Asia/Shanghai", "Pacific/Auckland"} {
		loc, err := time.LoadLocation(name)
		if err != nil {
			continue
		}
		nowLocal := now.In(loc)
		if hour := nowLocal.Hour(); hour >= 8 && hour < 22 {
			return loc, nowLocal
		}
	}
	suite.Require().FailNow("no zone in the table is inside its own day right now")
	return nil, time.Time{}
}
