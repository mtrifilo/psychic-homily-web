package main

import (
	"os"
	"testing"

	"github.com/goccy/go-yaml"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"psychic-homily-backend/internal/utils"
)

// The fixture paths are relative to this package's directory, where `go test`
// runs, rather than to backend/, where main() runs. Loaded here rather than
// through getVenueData/getArtistData because those exit the process on a read
// error, which in a test reports nothing.
func loadFixture[T any](t *testing.T, path string) map[string]T {
	t.Helper()
	raw, err := os.ReadFile(path)
	require.NoError(t, err, "the seed reads this file; a move must update both")
	var rows map[string]T
	require.NoError(t, yaml.Unmarshal(raw, &rows))
	return rows
}

func loadVenueFixture(t *testing.T) map[string]VenueData {
	return loadFixture[VenueData](t, "../../../data/venues.yaml")
}

func loadArtistFixture(t *testing.T) map[string]ArtistData {
	return loadFixture[ArtistData](t, "../../../data/bands.yaml")
}

// TestSeedFixturesClearTheSocialGate reads the checked-in YAML the seed reads
// and asserts every social value in it survives the gate main() now applies.
//
// It is the tripwire for the failure this gate could otherwise cause: the seed
// stops on the first unusable value, so a handle shape the rule does not
// tolerate takes down every local environment at once, and the person who
// tightened the rule is not the person who finds out.
//
// It reads the fixture rather than a copy of it, so adding a row with a foreign
// host to bands.yaml fails here instead of in somebody's terminal.
func TestSeedFixturesClearTheSocialGate(t *testing.T) {
	venues := loadVenueFixture(t)
	require.NotEmpty(t, venues, "the fixture path is wrong if this is empty")
	for _, venue := range venues {
		assert.NoError(t,
			utils.ValidateStoredSocialValue("instagram", "Instagram URL", venue.Social.Instagram),
			"venue %q", venue.Name)
		assert.NoError(t,
			utils.ValidateStoredSocialValue("website", "Website URL", venue.Social.Website),
			"venue %q", venue.Name)
	}

	artists := loadArtistFixture(t)
	require.NotEmpty(t, artists, "the fixture path is wrong if this is empty")
	for _, artist := range artists {
		assert.NoError(t,
			utils.ValidateStoredSocialValue("instagram", "Instagram URL", artist.Social.Instagram),
			"artist %q", artist.Name)
		assert.NoError(t,
			utils.ValidateStoredSocialValue("website", "Website URL", artist.Social.Website),
			"artist %q", artist.Name)
	}
}

// TestExemplarSocialClearsTheGate covers the Go-literal half of the seed: the
// rich exemplars build every social column from one handle, so a typo in the
// template would refuse to seed each of them.
func TestExemplarSocialClearsTheGate(t *testing.T) {
	social := fullSocial("marissanadler")
	assert.NoError(t, utils.ValidateStoredSocialColumns(
		social.Instagram, social.Facebook, social.Twitter, social.YouTube,
		social.Spotify, social.SoundCloud, social.Bandcamp, social.Website,
	))
}
