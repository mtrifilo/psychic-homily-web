package catalog

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/stretchr/testify/suite"
	"gorm.io/gorm"

	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/testutil"
)

// =============================================================================
// INTEGRATION TESTS — profile→album embed resolution wiring (PSY-1190)
//
// Proves the fill-when-empty + manual-immutability invariant end to end: setting
// an artist's Bandcamp social link to a PROFILE root fills bandcamp_embed_url with
// the resolved /album|/track URL stamped profile_resolved, but NEVER overwrites a
// pre-existing (manual) value.
// =============================================================================

type BandcampProfileResolveIntegrationTestSuite struct {
	suite.Suite
	testDB        *testutil.TestDatabase
	db            *gorm.DB
	artistService *ArtistService
	server        *httptest.Server
}

func (suite *BandcampProfileResolveIntegrationTestSuite) SetupSuite() {
	suite.testDB = testutil.SetupTestPostgres(suite.T())
	suite.db = suite.testDB.DB

	// httptest server that serves the album-first fixture for any profile fetch.
	html := loadFixture(suite.T(), "bandcamp_profile_album_first.html")
	suite.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(html))
	}))
	target, _ := url.Parse(suite.server.URL)

	// Resolver whose client routes any *.bandcamp.com URL to the fixture server
	// while the SSRF gate still sees the bandcamp.com host.
	resolver := NewBandcampProfileResolverWithClient(&http.Client{
		Transport: &rewriteHostRoundTripper{target: target, rt: http.DefaultTransport},
	})

	suite.artistService = &ArtistService{db: suite.db}
	suite.artistService.SetBandcampResolver(resolver)
	// Run the profile resolve inline so the assertions don't race a goroutine.
	suite.artistService.SetSyncDispatch()
}

func (suite *BandcampProfileResolveIntegrationTestSuite) TearDownSuite() {
	suite.server.Close()
	suite.testDB.Cleanup()
}

func (suite *BandcampProfileResolveIntegrationTestSuite) TearDownTest() {
	sqlDB, err := suite.db.DB()
	suite.Require().NoError(err)
	_, _ = sqlDB.Exec("DELETE FROM artists")
}

func TestBandcampProfileResolveIntegrationTestSuite(t *testing.T) {
	suite.Run(t, new(BandcampProfileResolveIntegrationTestSuite))
}

const fixtureResolvedEmbed = "https://boris.bandcamp.com/album/fangsanalsatan-vol-25-in-gifu"

// makeArtist inserts an artist with the given name + optional embed/source.
func (suite *BandcampProfileResolveIntegrationTestSuite) makeArtist(name string, embed, source *string) *catalogm.Artist {
	slug := name
	a := &catalogm.Artist{
		Name:                name,
		Slug:                &slug,
		BandcampEmbedURL:    embed,
		BandcampEmbedSource: source,
	}
	suite.Require().NoError(suite.db.Create(a).Error)
	return a
}

func (suite *BandcampProfileResolveIntegrationTestSuite) reload(id uint) *catalogm.Artist {
	var a catalogm.Artist
	suite.Require().NoError(suite.db.First(&a, id).Error)
	return &a
}

// Update setting a profile root on an embed-NULL artist fills the embed with the
// resolved album URL, stamped profile_resolved.
func (suite *BandcampProfileResolveIntegrationTestSuite) TestUpdateFillsEmbedFromProfile() {
	a := suite.makeArtist("Boris", nil, nil)
	profile := "https://boris.bandcamp.com"

	_, err := suite.artistService.UpdateArtist(a.ID, &contracts.UpdateArtistRequest{Bandcamp: &profile})
	suite.Require().NoError(err)

	got := suite.reload(a.ID)
	suite.Require().NotNil(got.BandcampEmbedURL)
	suite.Equal(fixtureResolvedEmbed, *got.BandcampEmbedURL)
	suite.Require().NotNil(got.BandcampEmbedSource)
	suite.Equal(catalogm.BandcampEmbedSourceProfileResolved, *got.BandcampEmbedSource)
	// The social profile link itself is also stored.
	suite.Require().NotNil(got.Social.Bandcamp)
	suite.Equal(profile, *got.Social.Bandcamp)
}

// A manual embed is IMMUTABLE: setting a profile root on an artist that already
// has a manual embed must NOT overwrite the embed or change its source.
func (suite *BandcampProfileResolveIntegrationTestSuite) TestUpdateDoesNotOverwriteManualEmbed() {
	manualEmbed := "https://boris.bandcamp.com/album/curated-pick"
	manualSrc := catalogm.BandcampEmbedSourceManual
	a := suite.makeArtist("BorisManual", &manualEmbed, &manualSrc)

	profile := "https://boris.bandcamp.com"
	_, err := suite.artistService.UpdateArtist(a.ID, &contracts.UpdateArtistRequest{Bandcamp: &profile})
	suite.Require().NoError(err)

	got := suite.reload(a.ID)
	suite.Require().NotNil(got.BandcampEmbedURL)
	suite.Equal(manualEmbed, *got.BandcampEmbedURL, "manual embed must not be overwritten")
	suite.Require().NotNil(got.BandcampEmbedSource)
	suite.Equal(catalogm.BandcampEmbedSourceManual, *got.BandcampEmbedSource, "manual source must be preserved")
}

// A release_derived embed is also left untouched (fill-when-empty only acts on a
// NULL embed; any present value — manual OR auto-derived — is skipped).
func (suite *BandcampProfileResolveIntegrationTestSuite) TestUpdateDoesNotOverwriteReleaseDerivedEmbed() {
	derivedEmbed := "https://boris.bandcamp.com/album/from-a-release"
	derivedSrc := catalogm.BandcampEmbedSourceReleaseDerived
	a := suite.makeArtist("BorisDerived", &derivedEmbed, &derivedSrc)

	profile := "https://boris.bandcamp.com"
	_, err := suite.artistService.UpdateArtist(a.ID, &contracts.UpdateArtistRequest{Bandcamp: &profile})
	suite.Require().NoError(err)

	got := suite.reload(a.ID)
	suite.Require().NotNil(got.BandcampEmbedURL)
	suite.Equal(derivedEmbed, *got.BandcampEmbedURL)
	suite.Require().NotNil(got.BandcampEmbedSource)
	suite.Equal(catalogm.BandcampEmbedSourceReleaseDerived, *got.BandcampEmbedSource)
}

// Setting an ALBUM/TRACK URL as the social bandcamp (not a profile root) does not
// trigger resolution — only a bare profile root is resolved.
func (suite *BandcampProfileResolveIntegrationTestSuite) TestUpdateAlbumURLNotResolvedAsProfile() {
	a := suite.makeArtist("BorisAlbumLink", nil, nil)
	albumURL := "https://boris.bandcamp.com/album/some-record"

	_, err := suite.artistService.UpdateArtist(a.ID, &contracts.UpdateArtistRequest{Bandcamp: &albumURL})
	suite.Require().NoError(err)

	got := suite.reload(a.ID)
	suite.Nil(got.BandcampEmbedURL, "an /album social link is not a profile root → no embed fill")
}

// Create with a profile root fills the embed on insert.
func (suite *BandcampProfileResolveIntegrationTestSuite) TestCreateFillsEmbedFromProfile() {
	profile := "https://boris.bandcamp.com"
	resp, err := suite.artistService.CreateArtist(&contracts.CreateArtistRequest{
		Name:     "BorisCreate",
		Bandcamp: &profile,
	})
	suite.Require().NoError(err)

	got := suite.reload(resp.ID)
	suite.Require().NotNil(got.BandcampEmbedURL)
	suite.Equal(fixtureResolvedEmbed, *got.BandcampEmbedURL)
	suite.Require().NotNil(got.BandcampEmbedSource)
	suite.Equal(catalogm.BandcampEmbedSourceProfileResolved, *got.BandcampEmbedSource)
}

// Create with BOTH a manual embed and a profile root keeps the manual embed
// (the manual stamp wins; the resolver's IS NULL guard skips the row).
func (suite *BandcampProfileResolveIntegrationTestSuite) TestCreateWithManualEmbedAndProfileKeepsManual() {
	profile := "https://boris.bandcamp.com"
	manualEmbed := "https://boris.bandcamp.com/album/curated-on-create"
	resp, err := suite.artistService.CreateArtist(&contracts.CreateArtistRequest{
		Name:             "BorisCreateManual",
		Bandcamp:         &profile,
		BandcampEmbedURL: &manualEmbed,
	})
	suite.Require().NoError(err)

	got := suite.reload(resp.ID)
	suite.Require().NotNil(got.BandcampEmbedURL)
	suite.Equal(manualEmbed, *got.BandcampEmbedURL)
	suite.Require().NotNil(got.BandcampEmbedSource)
	suite.Equal(catalogm.BandcampEmbedSourceManual, *got.BandcampEmbedSource)
}
