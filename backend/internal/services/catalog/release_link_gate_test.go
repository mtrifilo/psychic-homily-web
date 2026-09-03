package catalog

import (
	apperrors "psychic-homily-backend/internal/errors"
	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/contracts"
)

// The service is the chokepoint (PSY-1996), not just the HTTP handler: the
// enrichment sweep, the backfill CLI, the discography importer and the
// entity-request fulfiller all reach these two functions without passing
// through a handler, so a gate that lived only at the boundary would leave them
// writing rows the release page refuses to link.

func (suite *ReleaseServiceIntegrationTestSuite) TestAddExternalLink_RefusesForeignHost() {
	created, err := suite.releaseService.CreateRelease(&contracts.CreateReleaseRequest{Title: "Service Gate Album"})
	suite.Require().NoError(err)

	link, err := suite.releaseService.AddExternalLink(created.ID, "bandcamp", "https://evil.test/album/x")

	suite.Require().Error(err)
	suite.Nil(link)
	var releaseErr *apperrors.ReleaseError
	suite.Require().ErrorAs(err, &releaseErr)
	suite.Equal(apperrors.CodeReleaseInvalidField, releaseErr.Code)

	var count int64
	suite.db.Model(&catalogm.ReleaseExternalLink{}).Where("release_id = ?", created.ID).Count(&count)
	suite.EqualValues(0, count)
}

func (suite *ReleaseServiceIntegrationTestSuite) TestAddExternalLink_RefusesUnknownPlatform() {
	created, err := suite.releaseService.CreateRelease(&contracts.CreateReleaseRequest{Title: "Service Platform Album"})
	suite.Require().NoError(err)

	_, err = suite.releaseService.AddExternalLink(created.ID, "napster", "https://us.napster.com/album/x")

	suite.Require().Error(err)
	var releaseErr *apperrors.ReleaseError
	suite.Require().ErrorAs(err, &releaseErr)
	suite.Equal(apperrors.CodeReleaseInvalidField, releaseErr.Code)
}

// The provenance-carrying entry point is the one the enrichment sweep uses, so
// it gets its own case rather than relying on the thin wrapper above.
func (suite *ReleaseServiceIntegrationTestSuite) TestAddExternalLinkWithSource_RefusesForeignHost() {
	created, err := suite.releaseService.CreateRelease(&contracts.CreateReleaseRequest{Title: "Sourced Gate Album"})
	suite.Require().NoError(err)

	_, err = suite.releaseService.AddExternalLinkWithSource(
		created.ID, "spotify", "https://evil.test/album/x", "mb_backfill")

	suite.Require().Error(err)
	var releaseErr *apperrors.ReleaseError
	suite.Require().ErrorAs(err, &releaseErr)
	suite.Equal(apperrors.CodeReleaseInvalidField, releaseErr.Code)
}

// The create funnel refuses the whole release rather than dropping the bad
// link: a partially-applied create is a worse answer than a refusal the caller
// can act on.
func (suite *ReleaseServiceIntegrationTestSuite) TestCreateRelease_RefusesHostileExternalLink() {
	_, err := suite.releaseService.CreateRelease(&contracts.CreateReleaseRequest{
		Title: "Service Hostile Link Album",
		ExternalLinks: []contracts.CreateReleaseLinkEntry{
			{Platform: "spotify", URL: "https://spotify-account-verify.evil.test/album/x"},
		},
	})

	suite.Require().Error(err)
	var releaseErr *apperrors.ReleaseError
	suite.Require().ErrorAs(err, &releaseErr)
	suite.Equal(apperrors.CodeReleaseInvalidField, releaseErr.Code)

	var count int64
	suite.db.Model(&catalogm.Release{}).Where("title = ?", "Service Hostile Link Album").Count(&count)
	suite.EqualValues(0, count)
}
