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
	// Only the two columns the YAML schema carries; VenueData and ArtistData
	// have no field for the other six.
	assertPair := func(t *testing.T, kind, name, instagram, website string) {
		t.Helper()
		assert.NoError(t,
			utils.ValidateStoredSocialValue("instagram", utils.SocialFieldLabels["instagram"], instagram),
			"%s %q", kind, name)
		assert.NoError(t,
			utils.ValidateStoredSocialValue("website", utils.SocialFieldLabels["website"], website),
			"%s %q", kind, name)
	}

	venues := loadFixture[VenueData](t, "../../../data/venues.yaml")
	require.NotEmpty(t, venues, "the fixture path is wrong if this is empty")
	for _, venue := range venues {
		assertPair(t, "venue", venue.Name, venue.Social.Instagram, venue.Social.Website)
	}

	artists := loadFixture[ArtistData](t, "../../../data/bands.yaml")
	require.NotEmpty(t, artists, "the fixture path is wrong if this is empty")
	for _, artist := range artists {
		assertPair(t, "artist", artist.Name, artist.Social.Instagram, artist.Social.Website)
	}
}

// TestExemplarSocialClearsTheGate covers the Go-literal half of the seed: the
// rich exemplars build every social column from one handle, so a typo in the
// template would stop each of them seeding.
//
// The template, not the rows: main() now calls mustHoldSocialColumns at every
// exemplar create, so a row that overrides one column after fullSocial is
// checked by the binary itself.
func TestExemplarSocialClearsTheGate(t *testing.T) {
	social := fullSocial("marissanadler")
	assert.NoError(t, utils.ValidateStoredSocialColumns(utils.SocialColumns{
		Instagram:  social.Instagram,
		Facebook:   social.Facebook,
		Twitter:    social.Twitter,
		YouTube:    social.YouTube,
		Spotify:    social.Spotify,
		SoundCloud: social.SoundCloud,
		Bandcamp:   social.Bandcamp,
		Website:    social.Website,
	}))
}
