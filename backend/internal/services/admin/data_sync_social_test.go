package admin

import (
	"fmt"
	"time"

	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/contracts"
)

// TestImportData_DropsOffPlatformSocial pins the import as a write boundary: the
// body is JSON an admin hands us, not a value this system wrote, so a foreign
// host in a platform column must not reach the column the entity pages render
// under that platform's glyph.
//
// The COLUMN is dropped and the ROW is kept, which is the opposite of the embed
// rule beside it and deliberately so. Refusing the row here would not merely
// skip the artist: the show pass recreates it by name with a nil initializer, so
// one legacy value would cost the location, the verified flag and the seven
// links that were fine. The drop is named in the row's own message, so nothing
// is lost silently.
func (suite *DataSyncServiceIntegrationTestSuite) TestImportData_DropsOffPlatformSocial() {
	hostile := "https://spotify-account-verify.evil.test/"
	keep := "https://instagram.com/goodband"
	city := "Phoenix"
	name := fmt.Sprintf("Import Gate Band %d", time.Now().UnixNano())

	result, err := suite.service.ImportData(contracts.DataImportRequest{
		Artists: []contracts.ExportedArtist{
			{Name: name, City: &city, Spotify: &hostile, Instagram: &keep},
		},
	})
	suite.Require().NoError(err)
	suite.Equal(0, result.Artists.Errors)
	suite.Equal(1, result.Artists.Imported)
	suite.Require().Len(result.Artists.Messages, 1)
	suite.Contains(result.Artists.Messages[0], "dropped unusable social values: spotify")

	var stored catalogm.Artist
	suite.Require().NoError(suite.db.Where("name = ?", name).First(&stored).Error)
	suite.Nil(stored.Social.Spotify, "the off-platform host must not reach the column")
	suite.Require().NotNil(stored.Social.Instagram)
	suite.Equal(keep, *stored.Social.Instagram, "the conforming columns survive")
	suite.Require().NotNil(stored.City)
	suite.Equal(city, *stored.City, "the rest of the row survives")
}

// TestImportData_DropsOffPlatformVenueSocial is the same claim for the venue
// pass, which builds its row separately and would otherwise be guarded only by
// whichever entity someone remembered.
func (suite *DataSyncServiceIntegrationTestSuite) TestImportData_DropsOffPlatformVenueSocial() {
	hostile := "https://instagram.com.evil.test/x"
	name := fmt.Sprintf("Import Gate Venue %d", time.Now().UnixNano())

	result, err := suite.service.ImportData(contracts.DataImportRequest{
		Venues: []contracts.ExportedVenue{{Name: name, City: "Phoenix", State: "AZ", Instagram: &hostile}},
	})
	suite.Require().NoError(err)
	suite.Equal(0, result.Venues.Errors)
	suite.Equal(1, result.Venues.Imported)
	suite.Require().Len(result.Venues.Messages, 1)
	suite.Contains(result.Venues.Messages[0], "dropped unusable social values: instagram")

	var stored catalogm.Venue
	suite.Require().NoError(suite.db.Where("name = ?", name).First(&stored).Error)
	suite.Nil(stored.Social.Instagram)
}

// TestImportData_AcceptsLegacyHandleSocial is the other half, and the one that
// keeps the tool usable: an export of this project's own database carries bare
// handles in the social columns, so a gate that judged them as URLs would drop
// them on the export-to-import round trip the tool exists for. They are judged
// by where a reader resolves them instead.
func (suite *DataSyncServiceIntegrationTestSuite) TestImportData_AcceptsLegacyHandleSocial() {
	handle := "calexico"
	dotted := "fashion.club.la"
	name := fmt.Sprintf("Import Handle Band %d", time.Now().UnixNano())
	other := fmt.Sprintf("Import Dotted Band %d", time.Now().UnixNano())

	result, err := suite.service.ImportData(contracts.DataImportRequest{
		Artists: []contracts.ExportedArtist{
			{Name: name, Instagram: &handle},
			{Name: other, Instagram: &dotted},
		},
	})
	suite.Require().NoError(err)
	suite.Equal(0, result.Artists.Errors, "%v", result.Artists.Messages)
	suite.Equal(2, result.Artists.Imported)
	for _, msg := range result.Artists.Messages {
		suite.NotContains(msg, "dropped", "a legacy handle must survive the round trip: %s", msg)
	}

	var stored catalogm.Artist
	suite.Require().NoError(suite.db.Where("name = ?", name).First(&stored).Error)
	suite.Require().NotNil(stored.Social.Instagram)
	suite.Equal(handle, *stored.Social.Instagram, "the value is judged, never rewritten")
}
