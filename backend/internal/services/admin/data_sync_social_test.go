package admin

import (
	"fmt"
	"time"

	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/contracts"
)

// TestImportData_RefusesOffPlatformSocial pins the import as a write boundary:
// the body is JSON an admin hands us, not a value this system wrote, so a
// foreign host in a platform column is refused rather than copied into the
// column the entity pages render under that platform's glyph.
//
// Refusing rather than blanking the field is deliberate: a silently blanked
// column would report the artist as imported while losing what the operator
// meant to move, and the per-row message already says which value failed.
func (suite *DataSyncServiceIntegrationTestSuite) TestImportData_RefusesOffPlatformSocial() {
	hostile := "https://spotify-account-verify.evil.test/"
	name := fmt.Sprintf("Import Gate Band %d", time.Now().UnixNano())

	result, err := suite.service.ImportData(contracts.DataImportRequest{
		Artists: []contracts.ExportedArtist{{Name: name, Spotify: &hostile}},
	})
	suite.Require().NoError(err)
	suite.Equal(1, result.Artists.Errors)
	suite.Equal(0, result.Artists.Imported)
	suite.Require().Len(result.Artists.Messages, 1)
	suite.Contains(result.Artists.Messages[0], "spotify.com")

	var count int64
	suite.Require().NoError(suite.db.Model(&catalogm.Artist{}).Where("name = ?", name).Count(&count).Error)
	suite.Zero(count, "a refused row must not be created at all")
}

// TestImportData_RefusesOffPlatformVenueSocial is the same claim for the venue
// pass, which builds its row separately and would otherwise be guarded only by
// whichever entity someone remembered.
func (suite *DataSyncServiceIntegrationTestSuite) TestImportData_RefusesOffPlatformVenueSocial() {
	hostile := "https://instagram.com.evil.test/x"
	name := fmt.Sprintf("Import Gate Venue %d", time.Now().UnixNano())

	result, err := suite.service.ImportData(contracts.DataImportRequest{
		Venues: []contracts.ExportedVenue{{Name: name, City: "Phoenix", State: "AZ", Instagram: &hostile}},
	})
	suite.Require().NoError(err)
	suite.Equal(1, result.Venues.Errors)
	suite.Equal(0, result.Venues.Imported)

	var count int64
	suite.Require().NoError(suite.db.Model(&catalogm.Venue{}).Where("name = ?", name).Count(&count).Error)
	suite.Zero(count)
}

// TestImportData_AcceptsLegacyHandleSocial is the other half, and the one that
// keeps the tool usable: an export of this project's own database carries bare
// handles in the social columns, so a gate that judged them as URLs would fail
// the export-to-import round trip the tool exists for. They are judged by where
// a reader resolves them instead.
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

	var stored catalogm.Artist
	suite.Require().NoError(suite.db.Where("name = ?", name).First(&stored).Error)
	suite.Require().NotNil(stored.Social.Instagram)
	suite.Equal(handle, *stored.Social.Instagram, "the value is judged, never rewritten")
}
