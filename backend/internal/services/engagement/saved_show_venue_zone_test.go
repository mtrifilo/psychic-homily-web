package engagement

import (
	"time"

	catalogm "psychic-homily-backend/internal/models/catalog"
)

// The saved-shows list is the fourth join shape that reaches
// shared.VenueTZJoin (through user_bookmarks), and the only one in this
// package. PSY-1761 made an unresolvable venues.timezone fall back to the US
// state map instead of raising in AT TIME ZONE; this pins that the fallback
// reaches here too, because the guard is a subquery the planner is free to
// place differently in each shape.

// TestGetUserSavedShows_UnknownVenueZoneStillAnswers seeds a venue whose zone is
// written straight past the model — the out-of-band path the PSY-1707 write gate
// cannot see — and requires the list to answer rather than 500.
func (suite *SavedShowServiceIntegrationTestSuite) TestGetUserSavedShows_UnknownVenueZoneStillAnswers() {
	venue := &catalogm.Venue{Name: "Poisoned Saved Room", City: "Phoenix", State: "AZ"}
	suite.Require().NoError(suite.db.Create(venue).Error)
	suite.Require().NoError(suite.db.Table("venues").
		Where("id = ?", venue.ID).Update("timezone", "Pacific/Atlantis").Error)

	user := suite.createTestUser()

	// 23:00 tonight in Phoenix, which is already tomorrow in UTC: a query that
	// fell back to UTC rather than to the AZ state map would drop it, so this
	// fixture also pins WHICH fallback the guard takes.
	phoenix, err := time.LoadLocation("America/Phoenix")
	suite.Require().NoError(err)
	now := time.Now().In(phoenix)
	show := &catalogm.Show{
		Title:       "Poisoned Saved Show",
		EventDate:   time.Date(now.Year(), now.Month(), now.Day(), 23, 0, 0, 0, phoenix),
		City:        stringPtr("Phoenix"),
		State:       stringPtr("AZ"),
		Status:      catalogm.ShowStatusApproved,
		SubmittedBy: &user.ID,
	}
	suite.Require().NoError(suite.db.Create(show).Error)
	suite.Require().NoError(suite.db.Exec(
		"INSERT INTO show_venues (show_id, venue_id) VALUES (?, ?)", show.ID, venue.ID).Error)
	suite.Require().NoError(suite.savedShowService.SaveShow(user.ID, show.ID))

	saved, total, err := suite.savedShowService.GetUserSavedShows(user.ID, 20, 0, "upcoming")
	suite.Require().NoError(err, "an unresolvable venue zone must not break the saved-shows list")
	suite.Require().Equal(int64(1), total)
	suite.Require().Len(saved, 1)
}
