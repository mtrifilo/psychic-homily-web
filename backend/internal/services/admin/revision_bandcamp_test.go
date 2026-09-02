package admin

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	adminm "psychic-homily-backend/internal/models/admin"
)

// A rollback WRITES a contributor-supplied value; it does not restore something
// this system vetted (PSY-1966).
//
// FieldChange.OldValue arrives on the suggest-edit body and nothing compares it
// to the entity's real current value: the submit handler validates NewValue
// only, and ApprovePendingEdit copies the pair verbatim into
// revisions.field_changes. So a contributor can pair a legitimate NewValue with
// an arbitrary OldValue, wait for the approve, and have Rollback write the
// OldValue live. Every gate on the forward paths is bypassed by going backwards.
//
// These build the revision row directly, which is exactly the shape that flow
// produces.
func (s *RevisionServiceIntegrationTestSuite) TestRollback_RefusesHostileOldValue() {
	for _, hostile := range []string{
		"https://evil.test/album/checkout",
		"https://bandcamp.com.attacker.test/album/x",
		"http://kingbuffalo.bandcamp.com/album/regenerator",
		"   ",
	} {
		s.Run(hostile, func() {
			admin := s.createTestUser()
			artist := s.createTestArtist(
				fmt.Sprintf("Rollback Guard %d", time.Now().UnixNano()), "Phoenix", "AZ", "")

			changes := []adminm.FieldChange{{
				Field:    "bandcamp_embed_url",
				OldValue: hostile,
				NewValue: "https://kingbuffalo.bandcamp.com/album/regenerator",
			}}
			s.Require().NoError(s.svc.RecordRevision("artist", artist.ID, admin.ID, changes, "add their bandcamp"))

			var revision adminm.Revision
			s.Require().NoError(s.db.Where("entity_type = ? AND entity_id = ?", "artist", artist.ID).
				Order("id DESC").First(&revision).Error)

			err := s.svc.Rollback(context.Background(), revision.ID, admin.ID)
			s.Require().Error(err, "rollback must refuse to write %q", hostile)

			var applied struct{ BandcampEmbedURL *string }
			s.Require().NoError(s.db.Table("artists").
				Select("bandcamp_embed_url").Where("id = ?", artist.ID).Scan(&applied).Error)
			s.Nil(applied.BandcampEmbedURL, "the hostile old_value must not reach the entity")
		})
	}
}

// The gate must not close ordinary undo: rolling back to a real release page,
// or to NULL, still works.
func (s *RevisionServiceIntegrationTestSuite) TestRollback_RestoresValidEmbed() {
	admin := s.createTestUser()
	artist := s.createTestArtist(
		fmt.Sprintf("Rollback OK %d", time.Now().UnixNano()), "Phoenix", "AZ", "")

	const previous = "https://kingbuffalo.bandcamp.com/album/regenerator"
	s.Require().NoError(s.db.Table("artists").Where("id = ?", artist.ID).
		Update("bandcamp_embed_url", "https://other.bandcamp.com/album/new").Error)

	changes := []adminm.FieldChange{{
		Field:    "bandcamp_embed_url",
		OldValue: previous,
		NewValue: "https://other.bandcamp.com/album/new",
	}}
	s.Require().NoError(s.svc.RecordRevision("artist", artist.ID, admin.ID, changes, "swap the embed"))

	var revision adminm.Revision
	s.Require().NoError(s.db.Where("entity_type = ? AND entity_id = ?", "artist", artist.ID).
		Order("id DESC").First(&revision).Error)

	s.Require().NoError(s.svc.Rollback(context.Background(), revision.ID, admin.ID))

	var applied struct{ BandcampEmbedURL *string }
	s.Require().NoError(s.db.Table("artists").
		Select("bandcamp_embed_url").Where("id = ?", artist.ID).Scan(&applied).Error)
	s.Require().NotNil(applied.BandcampEmbedURL)
	s.Equal(previous, *applied.BandcampEmbedURL)
}

// Undoing the FIRST time an embed was set restores NULL, which the gate must
// treat as "nothing to check" rather than as a value to refuse.
func (s *RevisionServiceIntegrationTestSuite) TestRollback_RestoresNullEmbed() {
	admin := s.createTestUser()
	artist := s.createTestArtist(
		fmt.Sprintf("Rollback Null %d", time.Now().UnixNano()), "Phoenix", "AZ", "")

	const added = "https://kingbuffalo.bandcamp.com/album/regenerator"
	s.Require().NoError(s.db.Table("artists").Where("id = ?", artist.ID).
		Update("bandcamp_embed_url", added).Error)

	raw := json.RawMessage(`[{"field":"bandcamp_embed_url","old_value":null,"new_value":"` + added + `"}]`)
	var changes []adminm.FieldChange
	s.Require().NoError(json.Unmarshal(raw, &changes))
	s.Require().NoError(s.svc.RecordRevision("artist", artist.ID, admin.ID, changes, "add their bandcamp"))

	var revision adminm.Revision
	s.Require().NoError(s.db.Where("entity_type = ? AND entity_id = ?", "artist", artist.ID).
		Order("id DESC").First(&revision).Error)

	s.Require().NoError(s.svc.Rollback(context.Background(), revision.ID, admin.ID))

	var applied struct{ BandcampEmbedURL *string }
	s.Require().NoError(s.db.Table("artists").
		Select("bandcamp_embed_url").Where("id = ?", artist.ID).Scan(&applied).Error)
	s.Nil(applied.BandcampEmbedURL, "undoing the first set must restore NULL")
}
