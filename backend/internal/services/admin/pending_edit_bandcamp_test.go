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

// bandcampApprovalRejects is the corpus of values that could already be sitting
// in a pending_entity_edits row for bandcamp_embed_url: the field has been in
// ArtistAllowedEditFields and unvalidated since PSY-525, so every queued row
// predates the submit-time rule. Approval is where such a value goes live, and
// the stored value renders as an outbound link labelled "Listen to <artist> on
// Bandcamp" whenever the embed resolve comes back empty.
var bandcampApprovalRejects = []struct{ name, value string }{
	{"foreign host with a release-shaped path", "https://evil.test/album/checkout"},
	{"lookalike suffix", "https://bandcamp.com.attacker.test/album/x"},
	{"lookalike prefix", "https://evilbandcamp.com/album/x"},
	{"open redirect carrying a real release URL", "https://evil.test/?next=https://x.bandcamp.com/album/y"},
	{"userinfo spoofing the host", "https://x.bandcamp.com@evil.test/album/y"},
	{"release segment only in a foreign query string", "https://evil.test/checkout?ref=/album/x"},
	{"on-platform profile root, not a release", "https://kingbuffalo.bandcamp.com"},
	{"on-platform merch page mentioning an album", "https://kingbuffalo.bandcamp.com/merch/shirt?ref=/album/x"},
	{"http on a real release page renders nothing", "http://kingbuffalo.bandcamp.com/album/regenerator"},
	{"non-http scheme", "javascript:alert(1)//bandcamp.com/album/x"},
}

// TestRevalidateBandcampEmbed is the pure unit half: the gate reads the built
// updates map, so these are the shapes that map can hold.
func TestRevalidateBandcampEmbed(t *testing.T) {
	t.Run("absent, nil and empty pass", func(t *testing.T) {
		assert.NoError(t, revalidateBandcampEmbed(map[string]interface{}{}))
		assert.NoError(t, revalidateBandcampEmbed(map[string]interface{}{"bandcamp_embed_url": nil}))
		assert.NoError(t, revalidateBandcampEmbed(map[string]interface{}{"bandcamp_embed_url": ""}))
	})

	t.Run("a release page passes", func(t *testing.T) {
		assert.NoError(t, revalidateBandcampEmbed(map[string]interface{}{
			"bandcamp_embed_url": "https://kingbuffalo.bandcamp.com/album/regenerator",
		}))
	})

	t.Run("non-string is refused, not skipped", func(t *testing.T) {
		// Refusing is what keeps the row actionable: skipping would leave it to
		// 500 at the driver on every approve attempt and sit pending forever.
		for _, bad := range []any{42, true, map[string]any{"x": 1}, []any{"a"}} {
			assert.Error(t, revalidateBandcampEmbed(map[string]interface{}{"bandcamp_embed_url": bad}))
		}
	})

	t.Run("hostile and non-release values are refused", func(t *testing.T) {
		for _, c := range bandcampApprovalRejects {
			assert.Error(t,
				revalidateBandcampEmbed(map[string]interface{}{"bandcamp_embed_url": c.value}),
				"%s must not approve: %s", c.name, c.value)
		}
	})

	t.Run("other fields are untouched by this gate", func(t *testing.T) {
		// The social bandcamp field holds a profile root, not a release, and
		// must not be dragged into the stricter rule.
		assert.NoError(t, revalidateBandcampEmbed(map[string]interface{}{
			"bandcamp": "https://kingbuffalo.bandcamp.com",
		}))
	})
}

// TestApprovePendingEdit_RejectsStoredHostileBandcampEmbed is the acceptance
// case: a queued row carrying a value that is not a Bandcamp release page must
// fail approval outright rather than land on the live artist.
//
// The row must survive the refusal so an admin can still reject it with a reason
// and the contributor can still cancel it, which is what the status/reviewed_by
// assertions pin.
func (s *PendingEditServiceIntegrationTestSuite) TestApprovePendingEdit_RejectsStoredHostileBandcampEmbed() {
	for _, c := range bandcampApprovalRejects {
		s.Run(c.name, func() {
			user := s.createTestUser()
			reviewer := s.createTestUser()
			artist := s.createTestArtist(fmt.Sprintf("Bandcamp Test Artist %d", time.Now().UnixNano()))

			// Created through the service, which does NOT validate field values;
			// that is the submit handler's job. This is exactly the row shape the
			// gate exists for: one that never met the submit-time rule.
			created, err := s.svc.CreatePendingEdit(&contracts.CreatePendingEditRequest{
				EntityType: "artist",
				EntityID:   artist.ID,
				UserID:     user.ID,
				Changes: []adminm.FieldChange{
					{Field: "bandcamp_embed_url", OldValue: nil, NewValue: c.value},
				},
				Summary: "add their bandcamp",
			})
			s.Require().NoError(err)

			_, err = s.svc.ApprovePendingEdit(context.Background(), created.ID, reviewer.ID)
			s.Require().Error(err, "approval must fail for %s", c.value)

			var applied struct{ BandcampEmbedURL *string }
			s.Require().NoError(s.db.Table("artists").
				Select("bandcamp_embed_url").Where("id = ?", artist.ID).Scan(&applied).Error)
			s.Nil(applied.BandcampEmbedURL, "the hostile embed URL must not reach the entity")

			var row adminm.PendingEntityEdit
			s.Require().NoError(s.db.First(&row, created.ID).Error)
			s.Equal(adminm.PendingEditStatusPending, row.Status)
			s.Nil(row.ReviewedBy)
			s.Nil(row.ReviewedAt)

			_, rerr := s.svc.RejectPendingEdit(created.ID, reviewer.ID, "not a Bandcamp release page")
			s.NoError(rerr, "the refused edit must remain rejectable")
		})
	}
}

// TestApprovePendingEdit_AppliesBandcampReleaseURL confirms the gate did not
// close the ordinary path: a real album page still approves and applies.
func (s *PendingEditServiceIntegrationTestSuite) TestApprovePendingEdit_AppliesBandcampReleaseURL() {
	user := s.createTestUser()
	reviewer := s.createTestUser()
	artist := s.createTestArtist(fmt.Sprintf("Bandcamp Test Artist %d", time.Now().UnixNano()))

	const release = "https://kingbuffalo.bandcamp.com/album/regenerator"
	created, err := s.svc.CreatePendingEdit(&contracts.CreatePendingEditRequest{
		EntityType: "artist",
		EntityID:   artist.ID,
		UserID:     user.ID,
		Changes: []adminm.FieldChange{
			{Field: "bandcamp_embed_url", OldValue: nil, NewValue: release},
		},
		Summary: "add their bandcamp",
	})
	s.Require().NoError(err)

	_, err = s.svc.ApprovePendingEdit(context.Background(), created.ID, reviewer.ID)
	s.Require().NoError(err)

	var applied struct{ BandcampEmbedURL *string }
	s.Require().NoError(s.db.Table("artists").
		Select("bandcamp_embed_url").Where("id = ?", artist.ID).Scan(&applied).Error)
	s.Require().NotNil(applied.BandcampEmbedURL)
	s.Equal(release, *applied.BandcampEmbedURL)
}

// TestApprovePendingEdit_ClearsBandcampEmbed pins the clear-the-field gesture:
// an empty string is how a contributor removes a stale embed, and the gate must
// not turn that into an unapprovable row.
func (s *PendingEditServiceIntegrationTestSuite) TestApprovePendingEdit_ClearsBandcampEmbed() {
	user := s.createTestUser()
	reviewer := s.createTestUser()
	artist := s.createTestArtist(fmt.Sprintf("Bandcamp Test Artist %d", time.Now().UnixNano()))

	const release = "https://kingbuffalo.bandcamp.com/album/regenerator"
	s.Require().NoError(s.db.Table("artists").Where("id = ?", artist.ID).
		Update("bandcamp_embed_url", release).Error)

	created, err := s.svc.CreatePendingEdit(&contracts.CreatePendingEditRequest{
		EntityType: "artist",
		EntityID:   artist.ID,
		UserID:     user.ID,
		Changes: []adminm.FieldChange{
			{Field: "bandcamp_embed_url", OldValue: release, NewValue: ""},
		},
		Summary: "that release is gone",
	})
	s.Require().NoError(err)

	_, err = s.svc.ApprovePendingEdit(context.Background(), created.ID, reviewer.ID)
	s.Require().NoError(err)

	var applied struct{ BandcampEmbedURL *string }
	s.Require().NoError(s.db.Table("artists").
		Select("bandcamp_embed_url").Where("id = ?", artist.ID).Scan(&applied).Error)
	s.Require().NotNil(applied.BandcampEmbedURL)
	s.Equal("", *applied.BandcampEmbedURL)
}
