package catalog

import (
	"time"

	"gorm.io/gorm"

	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/contracts"
)

// countShows is the assertion these tests are really making: a refused create
// must leave the catalog with one show, not two.
func (suite *ShowServiceIntegrationTestSuite) countShows() int64 {
	var n int64
	suite.Require().NoError(suite.db.Model(&catalogm.Show{}).Count(&n).Error)
	return n
}

// TestMultiVenueDedup_SecondVenueIsCovered pins the case the denormalized
// lowest-venue stamping could not reach: an existing show listed at two venues,
// then a second show at only the SECOND of them, billing an artist the first
// show carried as a curated opener.
//
// The name-keyed create guard cannot see this pair. It matches an existing
// billing only by set_type='headliner' OR position=0, and a curated opener is
// neither, so the guard passes and the database is the only thing left. Cover
// every venue of the bill and the pair is refused; cover one and it is not.
func (suite *ShowServiceIntegrationTestSuite) TestMultiVenueDedup_SecondVenueIsCovered() {
	user := suite.createTestUser()
	eventDate := time.Date(2027, 9, 10, 20, 0, 0, 0, time.UTC)

	_, err := suite.showService.CreateShow(&contracts.CreateShowRequest{
		Title: "Multi Venue Existing", EventDate: eventDate, City: "Phoenix", State: "AZ",
		Venues: []contracts.CreateShowVenue{
			{Name: "Dedup Venue A", City: "Phoenix", State: "AZ"},
			{Name: "Dedup Venue B", City: "Phoenix", State: "AZ"},
		},
		Artists: []contracts.CreateShowArtist{
			{Name: "Dedup Opener", SetType: strPtr("opener")},
			{Name: "Dedup Headliner", SetType: strPtr("headliner")},
		},
		SubmittedByUserID: &user.ID, SubmitterIsAdmin: true,
	})
	suite.Require().NoError(err)

	_, err = suite.showService.CreateShow(&contracts.CreateShowRequest{
		Title: "Multi Venue Duplicate", EventDate: eventDate, City: "Phoenix", State: "AZ",
		Venues:            []contracts.CreateShowVenue{{Name: "Dedup Venue B", City: "Phoenix", State: "AZ"}},
		Artists:           []contracts.CreateShowArtist{{Name: "Dedup Opener", SetType: strPtr("headliner")}},
		SubmittedByUserID: &user.ID, SubmitterIsAdmin: true,
	})
	suite.Require().Error(err, "a second show at the multi-venue bill's other venue must be refused")
	suite.Require().EqualValues(1, suite.countShows())
}

// TestMultiVenueDedup_RefusedWithoutTheApplicationGuard is the acceptance case:
// the duplicate is refused by the DATABASE, with no service-layer guard in the
// path at all. The second show is assembled from bare association-table writes,
// which is what an importer, a CLI, a psql session or any future rewrite of the
// guard looks like from the schema's point of view.
func (suite *ShowServiceIntegrationTestSuite) TestMultiVenueDedup_RefusedWithoutTheApplicationGuard() {
	user := suite.createTestUser()
	eventDate := time.Date(2027, 9, 14, 20, 0, 0, 0, time.UTC)

	existing, err := suite.showService.CreateShow(&contracts.CreateShowRequest{
		Title: "Bypass Existing", EventDate: eventDate, City: "Phoenix", State: "AZ",
		Venues: []contracts.CreateShowVenue{
			{Name: "Bypass Venue A", City: "Phoenix", State: "AZ"},
			{Name: "Bypass Venue B", City: "Phoenix", State: "AZ"},
		},
		Artists: []contracts.CreateShowArtist{
			{Name: "Bypass Opener", SetType: strPtr("opener")},
			{Name: "Bypass Headliner", SetType: strPtr("headliner")},
		},
		SubmittedByUserID: &user.ID, SubmitterIsAdmin: true,
	})
	suite.Require().NoError(err)

	// The venue that is NOT the lowest id of the bill: the one the denormalized
	// stamping could never reach.
	var venueIDs []uint
	suite.Require().NoError(suite.db.Raw(
		`SELECT venue_id FROM show_venues WHERE show_id = ? ORDER BY venue_id`, existing.ID).
		Scan(&venueIDs).Error)
	suite.Require().Len(venueIDs, 2)
	secondVenueID := venueIDs[1]

	var openerID uint
	suite.Require().NoError(suite.db.Raw(
		`SELECT id FROM artists WHERE LOWER(name) = LOWER(?)`, "Bypass Opener").
		Scan(&openerID).Error)
	suite.Require().NotZero(openerID)

	err = suite.db.Transaction(func(tx *gorm.DB) error {
		var dupID uint
		if err := tx.Raw(`
			INSERT INTO shows (title, event_date, city, state, status, slug, created_at, updated_at)
			VALUES ('Bypass Duplicate', ?, 'Phoenix', 'AZ', 'approved', 'bypass-duplicate', NOW(), NOW())
			RETURNING id`, eventDate).Scan(&dupID).Error; err != nil {
			return err
		}
		if err := tx.Exec(
			`INSERT INTO show_venues (show_id, venue_id) VALUES (?, ?)`, dupID, secondVenueID).Error; err != nil {
			return err
		}
		return tx.Exec(
			`INSERT INTO show_artists (show_id, artist_id, position, set_type) VALUES (?, ?, 0, 'headliner')`,
			dupID, openerID).Error
	})

	suite.Require().Error(err, "the database must refuse the duplicate billing on its own")
	suite.Require().ErrorIs(err, gorm.ErrDuplicatedKey)
	suite.Require().EqualValues(1, suite.countShows())
}

// TestMultiVenueDedup_FirstVenueStaysCovered is the same pair aimed at the
// LOWEST venue id, the one the old denormalized column stamped. It guards
// against a fix that moves coverage rather than widening it.
func (suite *ShowServiceIntegrationTestSuite) TestMultiVenueDedup_FirstVenueStaysCovered() {
	user := suite.createTestUser()
	eventDate := time.Date(2027, 9, 11, 20, 0, 0, 0, time.UTC)

	_, err := suite.showService.CreateShow(&contracts.CreateShowRequest{
		Title: "First Venue Existing", EventDate: eventDate, City: "Phoenix", State: "AZ",
		Venues: []contracts.CreateShowVenue{
			{Name: "First Venue A", City: "Phoenix", State: "AZ"},
			{Name: "First Venue B", City: "Phoenix", State: "AZ"},
		},
		Artists: []contracts.CreateShowArtist{
			{Name: "First Opener", SetType: strPtr("opener")},
			{Name: "First Headliner", SetType: strPtr("headliner")},
		},
		SubmittedByUserID: &user.ID, SubmitterIsAdmin: true,
	})
	suite.Require().NoError(err)

	_, err = suite.showService.CreateShow(&contracts.CreateShowRequest{
		Title: "First Venue Duplicate", EventDate: eventDate, City: "Phoenix", State: "AZ",
		Venues:            []contracts.CreateShowVenue{{Name: "First Venue A", City: "Phoenix", State: "AZ"}},
		Artists:           []contracts.CreateShowArtist{{Name: "First Opener", SetType: strPtr("headliner")}},
		SubmittedByUserID: &user.ID, SubmitterIsAdmin: true,
	})
	suite.Require().Error(err)
	suite.Require().EqualValues(1, suite.countShows())
}

// TestMultiVenueDedup_DistinctVenueStillAllowed keeps the widened coverage from
// swallowing a real second booking: same artist, same night, a venue that is on
// neither leg of the first bill.
func (suite *ShowServiceIntegrationTestSuite) TestMultiVenueDedup_DistinctVenueStillAllowed() {
	user := suite.createTestUser()
	eventDate := time.Date(2027, 9, 12, 20, 0, 0, 0, time.UTC)

	_, err := suite.showService.CreateShow(&contracts.CreateShowRequest{
		Title: "Distinct Existing", EventDate: eventDate, City: "Phoenix", State: "AZ",
		Venues: []contracts.CreateShowVenue{
			{Name: "Distinct Venue A", City: "Phoenix", State: "AZ"},
			{Name: "Distinct Venue B", City: "Phoenix", State: "AZ"},
		},
		Artists: []contracts.CreateShowArtist{
			{Name: "Distinct Opener", SetType: strPtr("opener")},
			{Name: "Distinct Headliner", SetType: strPtr("headliner")},
		},
		SubmittedByUserID: &user.ID, SubmitterIsAdmin: true,
	})
	suite.Require().NoError(err)

	_, err = suite.showService.CreateShow(&contracts.CreateShowRequest{
		Title: "Distinct Elsewhere", EventDate: eventDate, City: "Phoenix", State: "AZ",
		Venues:            []contracts.CreateShowVenue{{Name: "Distinct Venue C", City: "Phoenix", State: "AZ"}},
		Artists:           []contracts.CreateShowArtist{{Name: "Distinct Opener", SetType: strPtr("headliner")}},
		SubmittedByUserID: &user.ID, SubmitterIsAdmin: true,
	})
	suite.Require().NoError(err, "a different venue on the same night is a different show")
	suite.Require().EqualValues(2, suite.countShows())
}

// dedupKeysFor reads back the derived key rows for one show.
func (suite *ShowServiceIntegrationTestSuite) dedupKeysFor(showID uint) []struct {
	ArtistID  uint      `gorm:"column:artist_id"`
	VenueID   uint      `gorm:"column:venue_id"`
	EventDate time.Time `gorm:"column:event_date"`
} {
	var rows []struct {
		ArtistID  uint      `gorm:"column:artist_id"`
		VenueID   uint      `gorm:"column:venue_id"`
		EventDate time.Time `gorm:"column:event_date"`
	}
	suite.Require().NoError(suite.db.Raw(
		`SELECT artist_id, venue_id, event_date FROM show_dedup_keys WHERE show_id = ? ORDER BY artist_id, venue_id`,
		showID).Scan(&rows).Error)
	return rows
}

// TestMultiVenueDedup_KeysAreDerivedFromTheBill is the derivation contract the
// constraint rests on: the key rows are the whole artist-by-venue cross product,
// they follow a moved date, and they leave with the show. A key that outlived
// its bill would refuse a booking nobody has made.
func (suite *ShowServiceIntegrationTestSuite) TestMultiVenueDedup_KeysAreDerivedFromTheBill() {
	user := suite.createTestUser()
	eventDate := time.Date(2027, 9, 15, 20, 0, 0, 0, time.UTC)
	movedDate := time.Date(2027, 9, 22, 20, 0, 0, 0, time.UTC)

	created, err := suite.showService.CreateShow(&contracts.CreateShowRequest{
		Title: "Derivation Bill", EventDate: eventDate, City: "Phoenix", State: "AZ",
		Venues: []contracts.CreateShowVenue{
			{Name: "Derivation Venue A", City: "Phoenix", State: "AZ"},
			{Name: "Derivation Venue B", City: "Phoenix", State: "AZ"},
		},
		Artists: []contracts.CreateShowArtist{
			{Name: "Derivation Opener", SetType: strPtr("opener")},
			{Name: "Derivation Headliner", SetType: strPtr("headliner")},
		},
		SubmittedByUserID: &user.ID, SubmitterIsAdmin: true,
	})
	suite.Require().NoError(err)

	suite.Require().Len(suite.dedupKeysFor(created.ID), 4,
		"two acts across two rooms is four keys, not one")

	_, err = suite.showService.UpdateShow(created.ID, &contracts.UpdateShowRequest{EventDate: &movedDate})
	suite.Require().NoError(err)
	moved := suite.dedupKeysFor(created.ID)
	suite.Require().Len(moved, 4)
	for _, key := range moved {
		suite.True(key.EventDate.UTC().Equal(movedDate.UTC()),
			"a moved event_date must carry every key with it (got %v)", key.EventDate.UTC())
	}

	// Dropping a room drops that room's half of the cross product.
	suite.Require().NoError(suite.db.Exec(
		`DELETE FROM show_venues WHERE show_id = ? AND venue_id = (
		     SELECT MAX(venue_id) FROM show_venues WHERE show_id = ?)`,
		created.ID, created.ID).Error)
	suite.Require().Len(suite.dedupKeysFor(created.ID), 2)

	suite.Require().NoError(suite.db.Exec(`DELETE FROM shows WHERE id = ?`, created.ID).Error)
	suite.Require().Empty(suite.dedupKeysFor(created.ID),
		"keys must not outlive the show they describe")
}

// TestMultiVenueDedup_MatineeAndEveningStillDistinct keeps the full-timestamp
// dedup key intact at every venue of a multi-venue bill: two sets the same day
// at the same venue are two shows (PSY-559).
func (suite *ShowServiceIntegrationTestSuite) TestMultiVenueDedup_MatineeAndEveningStillDistinct() {
	user := suite.createTestUser()
	matinee := time.Date(2027, 9, 13, 13, 0, 0, 0, time.UTC)
	evening := time.Date(2027, 9, 13, 20, 0, 0, 0, time.UTC)

	_, err := suite.showService.CreateShow(&contracts.CreateShowRequest{
		Title: "Matinee", EventDate: matinee, City: "Phoenix", State: "AZ",
		Venues: []contracts.CreateShowVenue{
			{Name: "Matinee Venue A", City: "Phoenix", State: "AZ"},
			{Name: "Matinee Venue B", City: "Phoenix", State: "AZ"},
		},
		Artists:           []contracts.CreateShowArtist{{Name: "Matinee Act", SetType: strPtr("headliner")}},
		SubmittedByUserID: &user.ID, SubmitterIsAdmin: true,
	})
	suite.Require().NoError(err)

	_, err = suite.showService.CreateShow(&contracts.CreateShowRequest{
		Title: "Evening", EventDate: evening, City: "Phoenix", State: "AZ",
		Venues:            []contracts.CreateShowVenue{{Name: "Matinee Venue B", City: "Phoenix", State: "AZ"}},
		Artists:           []contracts.CreateShowArtist{{Name: "Matinee Act", SetType: strPtr("headliner")}},
		SubmittedByUserID: &user.ID, SubmitterIsAdmin: true,
	})
	suite.Require().NoError(err, "a later set the same day is a different show")
	suite.Require().EqualValues(2, suite.countShows())
}
