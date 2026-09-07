package catalog

import (
	"fmt"
	"time"

	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/contracts"
)

// The scene roster's per-band upcoming figures (PSY-1813) — run as part of the
// SceneServiceIntegrationTestSuite (real Postgres, all migrations).

// phoenixLoc is the zone every venue in this suite's scene resolves to: stored
// on the venue where a fixture sets it, and reached through the US state map
// where it does not. Both arms must agree or the fixtures below would be
// describing two different calendars.
func (suite *SceneServiceIntegrationTestSuite) phoenixLoc() *time.Location {
	loc, err := time.LoadLocation("America/Phoenix")
	suite.Require().NoError(err)
	return loc
}

// venueLocalMidnight is the first instant of `dayOffset` days from today ON THE
// VENUE'S CALENDAR. Offset 0 is midnight this morning, which is ALREADY PAST as
// an instant — that is the whole point of the fixtures that use it.
func (suite *SceneServiceIntegrationTestSuite) venueLocalMidnight(dayOffset int) time.Time {
	loc := suite.phoenixLoc()
	now := time.Now().In(loc)
	return time.Date(now.Year(), now.Month(), now.Day()+dayOffset, 0, 0, 0, 0, loc)
}

// createRosterScene seeds the two verified venues the roster read requires
// (sceneMinVenues) plus a submitting user. The second venue carries an explicit
// timezone so at least one fixture exercises the stored-zone arm of the
// venue-local boundary rather than only the state-map fallback.
func (suite *SceneServiceIntegrationTestSuite) createRosterScene() (*catalogm.Venue, *catalogm.Venue, uint) {
	primary := suite.createVerifiedVenue("Valley Bar", "Phoenix", "AZ")
	suite.Require().NoError(suite.db.Model(primary).Update("timezone", "America/Phoenix").Error)
	secondary := suite.createVerifiedVenue("The Rebel Lounge", "Phoenix", "AZ")
	return primary, secondary, suite.createUser().ID
}

// rosterArtistByName finds one band on a roster page, failing the test rather
// than returning a zero value that a later assertion would misread as data.
func (suite *SceneServiceIntegrationTestSuite) rosterArtistByName(
	roster []*contracts.SceneArtistResponse, name string,
) *contracts.SceneArtistResponse {
	for _, a := range roster {
		if a.Name == name {
			return a
		}
	}
	suite.Require().Failf("band missing from roster", "no band named %q", name)
	return nil
}

// createCancelledShow seeds an approved but cancelled show. is_cancelled has a
// DB default of false, so the flag has to be written after the create (the GORM
// zero-value trap this suite's venue helpers already work around).
func (suite *SceneServiceIntegrationTestSuite) createCancelledShow(
	title string, venueID, artistID, userID uint, eventDate time.Time,
) *catalogm.Show {
	show := suite.createApprovedShow(title, venueID, artistID, userID, eventDate)
	suite.Require().NoError(suite.db.Model(show).Update("is_cancelled", true).Error)
	return show
}

// THE BOUNDARY PIN.
//
// A date-only listing for tonight sits at the start of its own venue-local day,
// so by the time anyone reads the page it is an instant in the PAST. Under an
// `event_date > now()` filter the band's count drops it and the band reads as
// having nothing booked on the evening it plays. Bounding at venue-local
// midnight is what keeps it countable all day.
//
// The past fixture is 23:00 on the venue's PREVIOUS calendar day: an instant
// only hours earlier, on the other side of the boundary this test pins. A
// filter that rounded to a UTC day, or that used a fixed offset, would put the
// two fixtures on the same side.
func (suite *SceneServiceIntegrationTestSuite) TestGetActiveArtists_UpcomingBoundedAtVenueLocalStartOfDay() {
	venue, _, userID := suite.createRosterScene()
	band := suite.createArtist("Saguaro Teeth")

	todayStart := suite.venueLocalMidnight(0)
	suite.Require().True(todayStart.Before(time.Now()),
		"fixture is only meaningful while venue-local midnight today is already past")
	suite.createApprovedShow("date-only tonight", venue.ID, band.ID, userID, todayStart)
	suite.createApprovedShow("last night", venue.ID, band.ID, userID,
		suite.venueLocalMidnight(0).Add(-time.Hour))

	roster, _, err := suite.sceneService.GetActiveArtists("Phoenix", "AZ", 365, 20, 0)
	suite.Require().NoError(err)
	got := suite.rosterArtistByName(roster, "Saguaro Teeth")

	suite.Equal(1, got.UpcomingShowCount,
		"tonight's date-only show counts all day; last night's does not count at all")
	suite.Require().NotNil(got.NextShow)
	suite.Equal(todayStart.Format("2006-01-02"), got.NextShow.EventDate,
		"the date printed is the venue-local calendar day the filter selected on")
}

// The payload the roster line is built from, end to end: how many shows a band
// has ahead of it, and enough about the soonest to name and link it.
func (suite *SceneServiceIntegrationTestSuite) TestGetActiveArtists_UpcomingCountAndNextShow() {
	venue, other, userID := suite.createRosterScene()
	slug := "next-show-slug"
	booked := suite.createArtist("Booked Band")
	quiet := suite.createArtist("Quiet Band")

	// Soonest first on the wire regardless of insertion order, and the count is
	// every upcoming show anywhere, not only the soonest one's room.
	suite.createApprovedShow("later", other.ID, booked.ID, userID, suite.venueLocalMidnight(9).Add(20*time.Hour))
	soonest := suite.createApprovedShow("soonest", venue.ID, booked.ID, userID, suite.venueLocalMidnight(2).Add(20*time.Hour))
	suite.Require().NoError(suite.db.Model(soonest).Update("slug", slug).Error)
	suite.Require().NoError(suite.db.Model(venue).Update("slug", "valley-bar").Error)

	// A band whose only show has already happened: it stays on the roster (the
	// roster is metro residence), with nothing booked.
	suite.createApprovedShow("months ago", venue.ID, quiet.ID, userID, suite.venueLocalMidnight(-90))

	roster, _, err := suite.sceneService.GetActiveArtists("Phoenix", "AZ", 365, 20, 0)
	suite.Require().NoError(err)

	got := suite.rosterArtistByName(roster, "Booked Band")
	suite.Equal(2, got.UpcomingShowCount)
	suite.Require().NotNil(got.NextShow)
	suite.Equal(soonest.ID, got.NextShow.ID)
	suite.Equal(slug, got.NextShow.Slug)
	suite.Equal(suite.venueLocalMidnight(2).Format("2006-01-02"), got.NextShow.EventDate)
	suite.Equal("Valley Bar", got.NextShow.VenueName)
	suite.Equal("valley-bar", got.NextShow.VenueSlug)

	silent := suite.rosterArtistByName(roster, "Quiet Band")
	suite.Equal(0, silent.UpcomingShowCount)
	suite.Nil(silent.NextShow, "no next show is the absent state, never a zero-dated one")
}

// Everything the count must refuse, in one roster: a cancelled show (which
// would read as a date a reader can turn up to), an unreviewed submission, and
// another band's booking.
func (suite *SceneServiceIntegrationTestSuite) TestGetActiveArtists_UpcomingExcludesCancelledAndUnapproved() {
	venue, _, userID := suite.createRosterScene()
	band := suite.createArtist("Saguaro Teeth")
	neighbour := suite.createArtist("Other Band")

	suite.createCancelledShow("called off", venue.ID, band.ID, userID, suite.venueLocalMidnight(3).Add(20*time.Hour))
	suite.createPendingShow("unreviewed", venue.ID, band.ID, userID, suite.venueLocalMidnight(4).Add(20*time.Hour))
	suite.createApprovedShow("someone else", venue.ID, neighbour.ID, userID, suite.venueLocalMidnight(5).Add(20*time.Hour))
	real := suite.createApprovedShow("the real one", venue.ID, band.ID, userID, suite.venueLocalMidnight(6).Add(20*time.Hour))

	roster, _, err := suite.sceneService.GetActiveArtists("Phoenix", "AZ", 365, 20, 0)
	suite.Require().NoError(err)

	got := suite.rosterArtistByName(roster, "Saguaro Teeth")
	suite.Equal(1, got.UpcomingShowCount)
	suite.Require().NotNil(got.NextShow)
	suite.Equal(real.ID, got.NextShow.ID,
		"a cancelled show must not be the soonest even when it is the nearest date")
}

// A booking with no room yet is still a booking. It counts, and the row names
// the date without inventing a venue.
func (suite *SceneServiceIntegrationTestSuite) TestGetActiveArtists_UpcomingCountsVenuelessShow() {
	_, _, userID := suite.createRosterScene()
	band := suite.createArtist("Saguaro Teeth")
	suite.createVenuelessApprovedShow("room not settled", band.ID, userID, suite.venueLocalMidnight(3).Add(20*time.Hour))

	roster, _, err := suite.sceneService.GetActiveArtists("Phoenix", "AZ", 365, 20, 0)
	suite.Require().NoError(err)

	got := suite.rosterArtistByName(roster, "Saguaro Teeth")
	suite.Equal(1, got.UpcomingShowCount)
	suite.Require().NotNil(got.NextShow)
	suite.Empty(got.NextShow.VenueName)
	suite.Empty(got.NextShow.VenueSlug)
}

// The equivalence the payload asserts: a next show is present exactly when the
// count is positive. Both come from one query over one boundary, and this holds
// them to it across a whole page rather than on the one band a fixture watches.
func (suite *SceneServiceIntegrationTestSuite) TestGetActiveArtists_NextShowPresentExactlyWhenCountPositive() {
	venue, _, userID := suite.createRosterScene()
	for i := 0; i < 6; i++ {
		band := suite.createArtist(fmt.Sprintf("Band %d", i))
		switch i % 3 {
		case 0: // nothing at all
		case 1:
			suite.createApprovedShow("past", venue.ID, band.ID, userID, suite.venueLocalMidnight(-i-1))
		case 2:
			suite.createApprovedShow("future", venue.ID, band.ID, userID, suite.venueLocalMidnight(i).Add(20*time.Hour))
		}
	}

	roster, _, err := suite.sceneService.GetActiveArtists("Phoenix", "AZ", 365, 20, 0)
	suite.Require().NoError(err)
	suite.Require().Len(roster, 6)
	for _, a := range roster {
		suite.Equal(a.UpcomingShowCount > 0, a.NextShow != nil, "band %q", a.Name)
	}
}
