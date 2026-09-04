package admin

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	apperrors "psychic-homily-backend/internal/errors"
	adminm "psychic-homily-backend/internal/models/admin"
	"psychic-homily-backend/internal/services/contracts"
)

// TestValidateApplyURLs is the pure half: the gate reads the built updates map,
// so these are the shapes that map can hold on either apply path.
func TestValidateApplyURLs(t *testing.T) {
	t.Run("absent, nil and empty pass", func(t *testing.T) {
		assert.NoError(t, validateApplyURLs(map[string]interface{}{}, approveURLFields))
		assert.NoError(t, validateApplyURLs(map[string]interface{}{"spotify": nil}, approveURLFields))
		assert.NoError(t, validateApplyURLs(map[string]interface{}{"spotify": ""}, approveURLFields))
	})

	t.Run("an on-platform value passes", func(t *testing.T) {
		assert.NoError(t, validateApplyURLs(map[string]interface{}{
			"spotify":   "https://open.spotify.com/artist/4Z8W4fKeB5YxbusRsdQVPb",
			"instagram": "https://www.instagram.com/realband/",
			"website":   "https://realband.example.org/tour",
		}, approveURLFields))
	})

	t.Run("non-string is refused, not skipped", func(t *testing.T) {
		// Skipping would leave the row to 500 at the driver on every approve
		// attempt and sit pending forever. A 422 is actionable.
		for _, bad := range []any{42, true, map[string]any{"x": 1}, []any{"a"}} {
			assert.Error(t, validateApplyURLs(map[string]interface{}{"spotify": bad}, approveURLFields))
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
			assert.Error(t, validateApplyURLs(map[string]interface{}{field: hostile}, approveURLFields),
				"%s must not approve %q", field, hostile)
		}
	})

	t.Run("the unanchored fields still meet the scheme rule", func(t *testing.T) {
		// website anchors no host by design, so the scheme rule is the whole
		// guard, and it is still the difference between a link and a
		// javascript: URL in a rendered attribute.
		for _, field := range []string{"website", "ticket_url", "cover_art_url"} {
			assert.Error(t, validateApplyURLs(map[string]interface{}{field: "javascript:alert(1)"}, approveURLFields), field)
			assert.NoError(t, validateApplyURLs(map[string]interface{}{field: "https://example.org/x"}, approveURLFields), field)
		}
	})

	// flyer_url is the one field the two apply paths differ on, and this pins
	// the difference from both sides: an approve must not refuse a shape the
	// submit handler accepted, while a rollback still refuses it.
	t.Run("flyer_url is checked on rollback and not on approve", func(t *testing.T) {
		// The seed itself stores a relative "/seed-placeholders/festival.svg"
		// here, so a relative value is the ordinary shape, not an attack.
		relative := map[string]interface{}{"flyer_url": "/uploads/flyer.jpg"}
		assert.NoError(t, validateApplyURLs(relative, approveURLFields),
			"approve must not strand an edit the submit handler accepted")
		assert.Error(t, validateApplyURLs(relative, applyURLFields),
			"rollback writes an OldValue nothing validated, so it keeps refusing this")
	})

	t.Run("a bare handle is refused, unlike the offline writers", func(t *testing.T) {
		// This path is a contributor form's value, not a legacy row's, so the
		// legacy tolerance (utils.ValidateStoredSocialValue) deliberately does
		// not apply: the submit handler refuses a handle and so must approve.
		assert.Error(t, validateApplyURLs(map[string]interface{}{"instagram": "calexico"}, approveURLFields))
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

	// The CODE, not just an error: the AC asks for a 422, and
	// shared.MapPendingEditError turns exactly this code into one. Asserting the
	// message alone would survive a remap to a 500.
	var editErr *apperrors.PendingEditError
	s.Require().True(errors.As(err, &editErr))
	s.Equal(apperrors.CodePendingEditInvalidRequest, editErr.Code)

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
//
// It asserts what the column HOLDS afterwards only to record today's behaviour,
// which is "" rather than NULL. That is the blank-but-not-null shape
// BlankBandcampEmbedToNil exists to prevent on the shaped fields; the social
// columns have no such normalizer and no `IS NULL` repair path reading them.
// A change that normalizes them is free to update this line.
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
