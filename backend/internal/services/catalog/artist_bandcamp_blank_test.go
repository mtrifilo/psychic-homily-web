package catalog

import (
	"fmt"
	"time"

	"psychic-homily-backend/internal/services/contracts"
)

// The column is NULL or a renderable URL, never blank-but-not-null (PSY-1966).
//
// The validator has to pass "" — that is how a curator clears the field — so
// every write path must normalize it rather than store it. A blank row renders
// exactly like NULL but is invisible to every `bandcamp_embed_url IS NULL` gate:
// the profile resolver, the release-derived fill,
// cmd/backfill-artist-bandcamp-embeds and cmd/sweep-link-suggestions all skip
// it, so the artist can never be repaired by any automated path again.
//
// One test per create/update path, because the round that introduced the
// invariant fixed UpdateArtist and left CreateArtist storing the raw pointer.
func (suite *ArtistServiceIntegrationTestSuite) TestCreateArtist_BlankEmbedBecomesNull() {
	blank := ""
	name := fmt.Sprintf("Blank Embed Create %d", time.Now().UnixNano())

	created, err := suite.artistService.CreateArtist(&contracts.CreateArtistRequest{
		Name:             name,
		BandcampEmbedURL: &blank,
	})
	suite.Require().NoError(err)

	var stored struct {
		BandcampEmbedURL    *string
		BandcampEmbedSource *string
	}
	suite.Require().NoError(suite.db.Table("artists").
		Select("bandcamp_embed_url", "bandcamp_embed_source").
		Where("id = ?", created.ID).Scan(&stored).Error)

	suite.Nil(stored.BandcampEmbedURL, "a cleared embed must be NULL, not a blank string")
	// Invariant from PSY-1188: NULL url and NULL source travel together.
	suite.Nil(stored.BandcampEmbedSource, "no URL means no provenance")
}

func (suite *ArtistServiceIntegrationTestSuite) TestUpdateArtist_BlankEmbedBecomesNull() {
	name := fmt.Sprintf("Blank Embed Update %d", time.Now().UnixNano())
	created, err := suite.artistService.CreateArtist(&contracts.CreateArtistRequest{Name: name})
	suite.Require().NoError(err)

	release := "https://kingbuffalo.bandcamp.com/album/regenerator"
	_, err = suite.artistService.UpdateArtist(created.ID, &contracts.UpdateArtistRequest{
		BandcampEmbedURL: &release,
	})
	suite.Require().NoError(err)

	blank := ""
	_, err = suite.artistService.UpdateArtist(created.ID, &contracts.UpdateArtistRequest{
		BandcampEmbedURL: &blank,
	})
	suite.Require().NoError(err)

	var stored struct {
		BandcampEmbedURL    *string
		BandcampEmbedSource *string
	}
	suite.Require().NoError(suite.db.Table("artists").
		Select("bandcamp_embed_url", "bandcamp_embed_source").
		Where("id = ?", created.ID).Scan(&stored).Error)

	suite.Nil(stored.BandcampEmbedURL, "a cleared embed must be NULL, not a blank string")
	suite.Nil(stored.BandcampEmbedSource, "clearing the URL clears the provenance")
}

// The service gate refuses a hostile value even though every HTTP boundary
// already does, and it refuses it as a 422-mapping error rather than the
// generic 500 a bare error would become.
func (suite *ArtistServiceIntegrationTestSuite) TestCreateArtist_RefusesNonReleaseEmbed() {
	hostile := "https://evil.test/album/checkout"
	_, err := suite.artistService.CreateArtist(&contracts.CreateArtistRequest{
		Name:             fmt.Sprintf("Hostile Embed %d", time.Now().UnixNano()),
		BandcampEmbedURL: &hostile,
	})
	suite.Require().Error(err)
	suite.Contains(err.Error(), "Bandcamp album or track page",
		"the refusal must carry the shared, actionable message")
}
