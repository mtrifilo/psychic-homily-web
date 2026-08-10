package notification

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"psychic-homily-backend/internal/services/contracts"
)

// PSY-1766: the digest's show sections come from a ROLLING [now, now+7d)
// window, so nothing in this email may call that window "this week" — the
// vocabulary locked on PSY-1732 for every rolling surface.
//
// What these pin is the RULE, not the exact phrasing: refining the copy is
// expected to update the literal strings below. What must not come back is
// calendar-week wording over a rolling count.

func sceneDigestFixture() []contracts.SceneDigestGroup {
	return []contracts.SceneDigestGroup{
		{
			SceneName: "Chicago, IL",
			SceneURL:  "http://localhost:3000/scenes/chicago-il",
			Shows: []contracts.SceneDigestShow{
				{
					DisplayTitle: "Bottle Fest night one",
					Date:         "Fri, Aug 14",
					VenueName:    "Empty Bottle",
					ShowURL:      "http://localhost:3000/shows/bottle-fest-night-one",
				},
			},
			NewArtists: []contracts.SceneDigestArtist{
				{Name: "Dehd", ArtistURL: "http://localhost:3000/artists/dehd"},
			},
			MoreNewArtists: 2,
		},
	}
}

// A single-scene digest names the scene in its subject. The subject is the one
// string a recipient reads before opening anything, so it carries the window
// wording too.
func TestSendSceneDigest_SingleScene_WordsTheRollingWindow(t *testing.T) {
	svc, emails := setupDigestEmailTest(t)

	err := svc.SendSceneDigestEmail(
		"fan@example.com",
		sceneDigestFixture(),
		"http://api.test.local/unsubscribe/scene-digest?uid=42&sig=abc",
	)
	require.NoError(t, err)

	sent := <-emails
	assert.Equal(t, "The next 7 days in Chicago, IL", sent.Subject)

	// Section heading over the show list, and the body lede above it.
	assert.Contains(t, sent.Html, ">Next 7 days</p>")
	assert.Contains(t, sent.Html, "Your scenes: the next 7 days")
	assert.Contains(t, sent.Html, "Shows in the next 7 days and new bands")

	// The whole email, subject included, is free of calendar-week wording over
	// the rolling count. The weekly CADENCE is a different claim and stays: the
	// opt-out copy still says "weekly scene digests".
	assert.NotContains(t, strings.ToLower(sent.Subject), "this week")
	assert.NotContains(t, strings.ToLower(sent.Html), "this week")
	assert.Contains(t, sent.Html, "weekly scene digests")
}

// The multi-scene subject drops the scene name but keeps the same window
// wording — the two subjects must not disagree about what the numbers mean.
func TestSendSceneDigest_MultiScene_WordsTheRollingWindow(t *testing.T) {
	svc, emails := setupDigestEmailTest(t)

	groups := append(sceneDigestFixture(), contracts.SceneDigestGroup{
		SceneName: "Phoenix, AZ",
		SceneURL:  "http://localhost:3000/scenes/phoenix-az",
		NewArtists: []contracts.SceneDigestArtist{
			{Name: "Injury Reserve", ArtistURL: "http://localhost:3000/artists/injury-reserve"},
		},
	})

	err := svc.SendSceneDigestEmail(
		"fan@example.com",
		groups,
		"http://api.test.local/unsubscribe/scene-digest?uid=42&sig=abc",
	)
	require.NoError(t, err)

	sent := <-emails
	assert.Equal(t, "The next 7 days in your scenes on Psychic Homily", sent.Subject)
	assert.NotContains(t, strings.ToLower(sent.Html), "this week")

	// Phoenix contributed only new bands, so it gets no "Next 7 days" heading —
	// the heading must never sit over an empty list.
	assert.Equal(t, 1, strings.Count(sent.Html, ">Next 7 days</p>"))
}
