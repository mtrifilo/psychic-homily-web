package admin

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	adminm "psychic-homily-backend/internal/models/admin"
	"psychic-homily-backend/internal/services/contracts"
)

// TestValidateApplyURLs is the pure half: the gate reads the built updates map,
// so these are the shapes that map can hold on either apply path.
func TestValidateApplyURLs(t *testing.T) {
	t.Run("absent, nil and empty pass", func(t *testing.T) {
		assert.NoError(t, validateApplyURLs(map[string]interface{}{}))
		assert.NoError(t, validateApplyURLs(map[string]interface{}{"spotify": nil}))
		assert.NoError(t, validateApplyURLs(map[string]interface{}{"spotify": ""}))
	})

	t.Run("an on-platform value passes", func(t *testing.T) {
		assert.NoError(t, validateApplyURLs(map[string]interface{}{
			"spotify":   "https://open.spotify.com/artist/4Z8W4fKeB5YxbusRsdQVPb",
			"instagram": "https://www.instagram.com/realband/",
			"website":   "https://realband.example.org/tour",
		}))
	})

	t.Run("non-string is refused, not skipped", func(t *testing.T) {
		// Skipping would leave the row to 500 at the driver on every approve
		// attempt and sit pending forever. A 422 is actionable.
		for _, bad := range []any{42, true, map[string]any{"x": 1}, []any{"a"}} {
			assert.Error(t, validateApplyURLs(map[string]interface{}{"spotify": bad}))
		}
	})

	// The queue outlives the gate, so these are the values a pending_entity_edits
	// row can hold: filed before the host anchor shipped, or through a path that
	// never met the submit handler.
	t.Run("an off-platform host is refused on every anchored field", func(t *testing.T) {
		cases := map[string]string{
			"instagram":  "https://instagram.evil.test/login",
			"facebook":   "https://facebook.com.evil.test/",
			"twitter":    "https://nottwitter.com/realband",
			"youtube":    "https://evil.test/?x=youtube.com",
			"spotify":    "https://spotify-account-verify.evil.test/",
			"soundcloud": "https://soundcloud.evil.test/",
			"bandcamp":   "https://169.254.169.254/",
		}
		for field, hostile := range cases {
			assert.Error(t, validateApplyURLs(map[string]interface{}{field: hostile}),
				"%s must not approve %q", field, hostile)
		}
	})

	t.Run("the unanchored fields still meet the scheme rule", func(t *testing.T) {
		// website anchors no host by design, so the scheme rule is the whole
		// guard, and it is still the difference between a link and a
		// javascript: URL in a rendered attribute.
		for _, field := range []string{"website", "ticket_url", "flyer_url", "cover_art_url"} {
			assert.Error(t, validateApplyURLs(map[string]interface{}{field: "javascript:alert(1)"}), field)
			assert.NoError(t, validateApplyURLs(map[string]interface{}{field: "https://example.org/x"}), field)
		}
	})

	t.Run("a bare handle is refused, unlike the offline writers", func(t *testing.T) {
		// This path is a contributor form's value, not a legacy row's, so the
		// legacy tolerance (utils.ValidateStoredSocialValue) deliberately does
		// not apply: the submit handler refuses a handle and so must approve.
		assert.Error(t, validateApplyURLs(map[string]interface{}{"instagram": "calexico"}))
	})
}

// TestApprovePendingEdit_RefusesQueuedOffPlatformSocial is the acceptance case:
// a row queued before the host anchor existed must fail approval rather than
// land on the live artist under the platform's glyph.
//
// The row is created through the service, which does NOT validate field values;
// that is the submit handler's job. This is exactly the row shape the gate
// exists for: one that never met the submit-time rule.
//
// ONE representative value, not the whole table: the shapes are settled by the
// pure test above. What is left to prove here is the wiring and the row's fate,
// which the status assertions pin — a refused edit must stay actionable.
func (s *PendingEditServiceIntegrationTestSuite) TestApprovePendingEdit_RefusesQueuedOffPlatformSocial() {
	const hostile = "https://spotify-account-verify.evil.test/"
	user := s.createTestUser()
	reviewer := s.createTestUser()
	artist := s.createTestArtist(fmt.Sprintf("Social Gate Artist %d", time.Now().UnixNano()))

	created, err := s.svc.CreatePendingEdit(&contracts.CreatePendingEditRequest{
		EntityType: "artist",
		EntityID:   artist.ID,
		UserID:     user.ID,
		Changes: []adminm.FieldChange{
			{Field: "spotify", OldValue: nil, NewValue: hostile},
		},
		Summary: "add their spotify",
	})
	s.Require().NoError(err)

	_, err = s.svc.ApprovePendingEdit(context.Background(), created.ID, reviewer.ID)
	s.Require().Error(err, "approval must fail for %s", hostile)
	s.Contains(err.Error(), "spotify.com", "the refusal must name the hosts an admin can act on")

	var applied struct{ Spotify *string }
	s.Require().NoError(s.db.Table("artists").
		Select("spotify").Where("id = ?", artist.ID).Scan(&applied).Error)
	s.Nil(applied.Spotify, "the off-platform host must not reach the entity")

	var row adminm.PendingEntityEdit
	s.Require().NoError(s.db.First(&row, created.ID).Error)
	s.Equal(adminm.PendingEditStatusPending, row.Status)
	s.Nil(row.ReviewedBy)
	s.Nil(row.ReviewedAt)

	_, rerr := s.svc.RejectPendingEdit(created.ID, reviewer.ID, "not a Spotify link")
	s.NoError(rerr, "the refused edit must remain rejectable")
}

// TestApprovePendingEdit_AppliesOnPlatformSocial confirms the gate did not close
// the ordinary path: a real profile URL still approves and applies verbatim.
func (s *PendingEditServiceIntegrationTestSuite) TestApprovePendingEdit_AppliesOnPlatformSocial() {
	const profile = "https://open.spotify.com/artist/4Z8W4fKeB5YxbusRsdQVPb"
	user := s.createTestUser()
	reviewer := s.createTestUser()
	artist := s.createTestArtist(fmt.Sprintf("Social Gate Artist %d", time.Now().UnixNano()))

	created, err := s.svc.CreatePendingEdit(&contracts.CreatePendingEditRequest{
		EntityType: "artist",
		EntityID:   artist.ID,
		UserID:     user.ID,
		Changes: []adminm.FieldChange{
			{Field: "spotify", OldValue: nil, NewValue: profile},
		},
		Summary: "add their spotify",
	})
	s.Require().NoError(err)

	_, err = s.svc.ApprovePendingEdit(context.Background(), created.ID, reviewer.ID)
	s.Require().NoError(err)

	var applied struct{ Spotify *string }
	s.Require().NoError(s.db.Table("artists").
		Select("spotify").Where("id = ?", artist.ID).Scan(&applied).Error)
	s.Require().NotNil(applied.Spotify)
	s.Equal(profile, *applied.Spotify)
}

// TestApprovePendingEdit_ClearsSocialField pins the clear-the-field gesture: an
// empty string is how a contributor removes a stale link, and the gate must not
// turn that into an unapprovable row.
func (s *PendingEditServiceIntegrationTestSuite) TestApprovePendingEdit_ClearsSocialField() {
	user := s.createTestUser()
	reviewer := s.createTestUser()
	artist := s.createTestArtist(fmt.Sprintf("Social Gate Artist %d", time.Now().UnixNano()))
	s.Require().NoError(s.db.Table("artists").Where("id = ?", artist.ID).
		Update("spotify", "https://open.spotify.com/artist/4Z8W4fKeB5YxbusRsdQVPb").Error)

	created, err := s.svc.CreatePendingEdit(&contracts.CreatePendingEditRequest{
		EntityType: "artist",
		EntityID:   artist.ID,
		UserID:     user.ID,
		Changes: []adminm.FieldChange{
			{Field: "spotify", OldValue: nil, NewValue: ""},
		},
		Summary: "remove their stale spotify",
	})
	s.Require().NoError(err)

	_, err = s.svc.ApprovePendingEdit(context.Background(), created.ID, reviewer.ID)
	s.Require().NoError(err)

	var applied struct{ Spotify *string }
	s.Require().NoError(s.db.Table("artists").
		Select("spotify").Where("id = ?", artist.ID).Scan(&applied).Error)
	s.Require().NotNil(applied.Spotify)
	s.Equal("", *applied.Spotify)
}
