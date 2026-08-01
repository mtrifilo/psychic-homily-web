package catalog

import (
	"fmt"
	"time"

	apperrors "psychic-homily-backend/internal/errors"
	catalogm "psychic-homily-backend/internal/models/catalog"
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

func (suite *SceneServiceIntegrationTestSuite) alsoTonightLoc() *time.Location {
	loc, err := time.LoadLocation(alsoTonightZone)
	suite.Require().NoError(err)
	return loc
}

// createAlsoTonightVenue seeds a venue with the metro and timezone the
// production write paths derive, so the scope and the scene clock under test are
// the ones a real row would produce.
func (suite *SceneServiceIntegrationTestSuite) createAlsoTonightVenue(name, city, state string, verified bool) *catalogm.Venue {
	tz := alsoTonightZone
	venue := &catalogm.Venue{
		Name:     name,
		City:     city,
		State:    state,
		Metro:    seedMetro(city, state),
		Timezone: &tz,
	}
	suite.Require().NoError(suite.db.Create(venue).Error)
	// GORM writes no column for a false bool on Create, so the DB default wins;
	// both directions have to be set explicitly.
	suite.Require().NoError(suite.db.Model(venue).Update("verified", verified).Error)
	return venue
}

// createAlsoTonightShow seeds one show at one venue. `at` is a venue-LOCAL wall
// time; storing its UTC instant is what makes the window assertions meaningful.
func (suite *SceneServiceIntegrationTestSuite) createAlsoTonightShow(title string, venueID uint, at time.Time, status catalogm.ShowStatus) *catalogm.Show {
	slug := fmt.Sprintf("also-tonight-%s-%d", title, time.Now().UnixNano())
	show := &catalogm.Show{
		Title:     title,
		Slug:      &slug,
		EventDate: at.UTC(),
		Status:    status,
	}
	suite.Require().NoError(suite.db.Create(show).Error)
	suite.Require().NoError(suite.db.Create(&catalogm.ShowVenue{ShowID: show.ID, VenueID: venueID}).Error)
	return show
}

func (suite *SceneServiceIntegrationTestSuite) showIDs(idOrSlug string) []uint {
	rail, err := suite.sceneService.GetShowAlsoTonight(idOrSlug)
	suite.Require().NoError(err)
	suite.Require().NotNil(rail)
	ids := make([]uint, 0, len(rail.Shows))
	for _, show := range rail.Shows {
		ids = append(ids, show.ID)
	}
	return ids
}

// The two acceptance properties that define the rail: it never lists the show
// the reader is already looking at, and its reach is the METRO, not the city.
func (suite *SceneServiceIntegrationTestSuite) TestGetShowAlsoTonight_ExcludesSelfAndScopesToMetro() {
	loc := suite.alsoTonightLoc()
	chicago := suite.createAlsoTonightVenue("Empty Bottle", "Chicago", "IL", true)
	evanston := suite.createAlsoTonightVenue("Space", "Evanston", "IL", true)
	milwaukee := suite.createAlsoTonightVenue("Cactus Club", "Milwaukee", "WI", true)

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
	suite.Equal(alsoTonightZone, rail.Timezone)
}

// The window is a half-open venue-local calendar day. A minute either side of it
// is a different night, and getting this wrong in UTC would move every evening
// show in the western hemisphere onto the following date.
func (suite *SceneServiceIntegrationTestSuite) TestGetShowAlsoTonight_WindowIsTheVenueLocalCalendarDay() {
	loc := suite.alsoTonightLoc()
	chicago := suite.createAlsoTonightVenue("Empty Bottle", "Chicago", "IL", true)
	evanston := suite.createAlsoTonightVenue("Space", "Evanston", "IL", true)

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

	suite.ElementsMatch([]uint{firstMinute.ID, lastMinute.ID}, suite.showIDs(fmt.Sprint(subject.ID)),
		"the window must be [local midnight, next local midnight) — inclusive of the first "+
			"instant of the date and exclusive of the first instant of the next")
}

// A set that starts after midnight belongs to the date it starts on, not to the
// evening it feels like a continuation of. That is the scene-day contract, and
// the rail emits `Date` as a scene-day key — if the two bucketed a 00:30 set
// differently, the rail's own "see all" link would lead to a page missing it.
func (suite *SceneServiceIntegrationTestSuite) TestGetShowAlsoTonight_AfterMidnightSetBelongsToItsOwnDate() {
	loc := suite.alsoTonightLoc()
	chicago := suite.createAlsoTonightVenue("Empty Bottle", "Chicago", "IL", true)
	evanston := suite.createAlsoTonightVenue("Space", "Evanston", "IL", true)

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
}

// A show page must never be broken by a rail that has nothing to say. A room
// whose place does not resolve to a scene we track answers with an empty rail at
// 200, and withholds the scene identity so no client renders a "see all" link to
// a page that would 404.
func (suite *SceneServiceIntegrationTestSuite) TestGetShowAlsoTonight_UnscopeableVenueIsEmptyNotAnError() {
	loc := suite.alsoTonightLoc()
	// Marfa pins no CBSA, and one verified room is below the scene threshold.
	marfa := suite.createAlsoTonightVenue("Capri", "Marfa", "TX", true)
	suite.Require().Nil(marfa.Metro)
	chicago := suite.createAlsoTonightVenue("Empty Bottle", "Chicago", "IL", true)

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
	// The date is still the answer to a real question, so it is still served.
	suite.Equal("2026-09-18", rail.Date)
	suite.Equal(alsoTonightZone, rail.Timezone)
}

// The counterpart to the case above, and the reason "no metro" is not by itself
// the empty condition: a no-CBSA city that DOES qualify as a scene is scoped by
// its literal (city, state) — the same fallback every other scene surface uses —
// and gets a real rail.
func (suite *SceneServiceIntegrationTestSuite) TestGetShowAlsoTonight_NoCBSACityWithAQualifyingSceneStillGetsARail() {
	loc := suite.alsoTonightLoc()
	capri := suite.createAlsoTonightVenue("Capri", "Marfa", "TX", true)
	planet := suite.createAlsoTonightVenue("Planet Marfa", "Marfa", "TX", true)
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
	suite.Empty(rail.SceneSlug)
	suite.NotEmpty(rail.Date)
	suite.NotEmpty(rail.Timezone)
}

// Unknown and non-approved must be INDISTINGUISHABLE. This surface is anonymous,
// so a different answer for a hidden show would confirm that its id is real.
func (suite *SceneServiceIntegrationTestSuite) TestGetShowAlsoTonight_UnknownAndHiddenShowsAreBothNotFound() {
	loc := suite.alsoTonightLoc()
	chicago := suite.createAlsoTonightVenue("Empty Bottle", "Chicago", "IL", true)
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
	chicago := suite.createAlsoTonightVenue("Empty Bottle", "Chicago", "IL", true)
	evanston := suite.createAlsoTonightVenue("Space", "Evanston", "IL", true)

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
	chicago := suite.createAlsoTonightVenue("Empty Bottle", "Chicago", "IL", true)
	evanston := suite.createAlsoTonightVenue("Space", "Evanston", "IL", true)

	// The subject goes FIRST in the night's ordering, so a naive cap-sized fetch
	// would spend one of its rows on the show being excluded.
	subject := suite.createAlsoTonightShow("subject", chicago.ID,
		time.Date(2026, time.September, 18, 18, 0, 0, 0, loc), catalogm.ShowStatusApproved)
	for i := 0; i < showAlsoTonightCap+2; i++ {
		suite.createAlsoTonightShow(fmt.Sprintf("sibling-%d", i), evanston.ID,
			time.Date(2026, time.September, 18, 19, i, 0, 0, loc), catalogm.ShowStatusApproved)
	}

	ids := suite.showIDs(fmt.Sprint(subject.ID))
	suite.Len(ids, showAlsoTonightCap)
	suite.NotContains(ids, subject.ID)
}
