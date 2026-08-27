package notification

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/utils"
)

// TestRenderEmailArtifact writes the exact HTML a send would hand Resend to the
// path in RENDER_OUT, and skips when that variable is unset.
//
// Assertions can only pin strings; whether the frame, rhythm and palette
// actually look right has to be judged by eye in a browser. This captures the
// real payload for that, rather than a hand-copied approximation that can drift
// away from what ships:
//
//	RENDER_OUT=/tmp/verify.html go test ./internal/services/notification/ \
//	  -run TestRenderEmailArtifact
func TestRenderEmailArtifact(t *testing.T) {
	out := os.Getenv("RENDER_OUT")
	if out == "" {
		t.Skip("set RENDER_OUT to a file path to capture the rendered email")
	}

	svc, emails, _ := setupEmailTest(t)
	svc.frontendURL = "https://psychichomily.com"

	// A fabricated token, signature and all. It is here only to give the
	// plain-link fallback a string of the length real recipients will see, so
	// the wrapping can be judged; it verifies against nothing.
	token := "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9." +
		"eyJzdWIiOiI0MjEiLCJwdXJwb3NlIjoiZW1haWxfdmVyaWZpY2F0aW9uIiwiZXhwIjoxNzcyMDAwMDAwfQ." +
		"qN3sQ7bXk1r8mZ0vD2wLpYtH5cA9fJgU6eR4iO1nS8k"

	require.NoError(t, svc.SendVerificationEmail("someone@example.com", token))
	require.NoError(t, os.WriteFile(out, []byte((<-emails).Html), 0o600))
	t.Logf("wrote %s", out)
}

// TestRenderArtistShowAlertArtifact does the same for the PSY-1896 artist
// new-show alert, so the direction-A frame, the mono details block and the
// footer links can be judged in a browser against the Figma mock rather than
// only through string assertions.
//
// It renders the builder directly rather than driving a send: the alert's HTML
// is built in this package and handed to the generic
// SendFilterNotificationEmail transport, so the builder's output IS the payload.
//
//	RENDER_OUT=/tmp/artist-alert.html go test ./internal/services/notification/ \
//	  -run TestRenderArtistShowAlertArtifact
func TestRenderArtistShowAlertArtifact(t *testing.T) {
	out := os.Getenv("RENDER_OUT")
	if out == "" {
		t.Skip("set RENDER_OUT to a file path to capture the rendered email")
	}

	content := showEmailContentParts{
		date:       "Saturday, August 29, 2026",
		venueText:  "Valley Bar",
		artistText: "Oneida, Din of Celestial Birds",
		priceText:  "$18",
		showURL:    "https://psychichomily.com/shows/oneida-valley-bar",
	}
	// A fabricated signature of the real length, so the footer's wrapping can be
	// judged. It verifies against nothing.
	unsubURL := "https://api.psychichomily.com/unsubscribe/artist-show-alerts" +
		"?uid=421&sig=9f2c1ad35b7e40c8a6d1e93b0f47c25ad8e6b1349f0a7c25e83bd1470c96af52"

	html := buildArtistShowAlertEmailHTML(
		"Oneida",
		contracts.FollowAlertScopeNearMe,
		content,
		unsubURL,
		"https://psychichomily.com/settings/notifications",
	)
	require.NoError(t, os.WriteFile(out, []byte(html), 0o600))
	t.Logf("wrote %s", out)
}

// TestRenderVenueShowAlertArtifact does the same for the PSY-1895 venue
// new-show digest, whose one un-inherited element is the multi-show list. A
// hairline-separated table is exactly the kind of thing string assertions
// cannot judge: whether the rules land, whether the mono date column stays
// aligned, and whether the block reads as terminated rather than running into
// the paragraph beneath it. Figma: node 1577:27.
//
//	RENDER_OUT=/tmp/venue-alert.html go test ./internal/services/notification/ \
//	  -run TestRenderVenueShowAlertArtifact
func TestRenderVenueShowAlertArtifact(t *testing.T) {
	out := os.Getenv("RENDER_OUT")
	if out == "" {
		t.Skip("set RENDER_OUT to a file path to capture the rendered email")
	}

	batch := &venueAlertBatch{
		key:       venueAlertGroupKey{VenueID: 3, AlertDay: "2026-08-24"},
		venueName: "Valley Bar",
		venueURL:  "https://psychichomily.com/venues/valley-bar",
		loc:       utils.EventLocation(nil, "AZ"),
	}
	shows := []venueAlertShow{
		{ID: 1, Title: "Oneida", ArtistText: "Oneida, Din of Celestial Birds",
			EventDate: time.Date(2026, 8, 29, 20, 0, 0, 0, time.UTC)},
		{ID: 2, Title: "Chat Pile", ArtistText: "Chat Pile, Agriculture",
			EventDate: time.Date(2026, 9, 5, 20, 0, 0, 0, time.UTC)},
		{ID: 3, Title: "Turnstile", ArtistText: "Turnstile, Mindforce, Zulu",
			EventDate: time.Date(2026, 9, 12, 20, 0, 0, 0, time.UTC)},
		// A bare listing, which is ordinary during ingest. It must still appear,
		// or the headline's count disagrees with the list under it.
		{ID: 4, Title: "TBA", EventDate: time.Date(2026, 9, 19, 20, 0, 0, 0, time.UTC)},
	}

	// A fabricated signature of the real length, so the footer's wrapping can be
	// judged. It verifies against nothing.
	unsubURL := "https://api.psychichomily.com/unsubscribe/artist-show-alerts" +
		"?uid=421&sig=9f2c1ad35b7e40c8a6d1e93b0f47c25ad8e6b1349f0a7c25e83bd1470c96af52"

	html := buildVenueShowAlertEmailHTML(
		batch, shows, unsubURL, "https://psychichomily.com/settings/notifications")
	require.NoError(t, os.WriteFile(out, []byte(html), 0o600))
	t.Logf("wrote %s", out)
}

// TestRenderSceneShowAlertArtifact does the same for the PSY-1926 scene
// new-show alert, which until then borrowed the criteria-filter template and
// therefore had neither the shared frame nor a working unsubscribe. Rendering
// it is how the footer's two links get judged side by side with its artist and
// venue siblings.
//
//	RENDER_OUT=/tmp/scene-alert.html go test ./internal/services/notification/ \
//	  -run TestRenderSceneShowAlertArtifact
func TestRenderSceneShowAlertArtifact(t *testing.T) {
	out := os.Getenv("RENDER_OUT")
	if out == "" {
		t.Skip("set RENDER_OUT to a file path to capture the rendered email")
	}

	content := showEmailContentParts{
		date:       "Saturday, August 29, 2026",
		venueText:  "Valley Bar",
		artistText: "Oneida, Din of Celestial Birds",
		priceText:  "$18",
		showURL:    "https://psychichomily.com/shows/oneida-valley-bar",
	}
	// A fabricated signature of the real length, so the footer's wrapping can be
	// judged. It verifies against nothing.
	unsubURL := "https://api.psychichomily.com/unsubscribe/artist-show-alerts" +
		"?uid=421&sig=9f2c1ad35b7e40c8a6d1e93b0f47c25ad8e6b1349f0a7c25e83bd1470c96af52"

	html := buildSceneShowAlertEmailHTML(
		"Phoenix, AZ",
		content,
		unsubURL,
		"https://psychichomily.com/settings/notifications",
	)
	require.NoError(t, os.WriteFile(out, []byte(html), 0o600))
	t.Logf("wrote %s", out)
}
