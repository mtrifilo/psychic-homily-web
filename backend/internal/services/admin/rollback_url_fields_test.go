package admin

import (
	"fmt"
	"time"

	adminm "psychic-homily-backend/internal/models/admin"
)

// The rollback hole is not one field wide (PSY-1966 review, round 2).
//
// FieldChange.OldValue is contributor input that nothing validates — the submit
// handler checks NewValue only — and Rollback writes it live. So EVERY forward
// rule is bypassed by going backwards, and SocialLinks renders each of these as
// an href under a trusted platform glyph with no read-time check of its own.
//
// The corpus pairs a legitimate NewValue with a hostile OldValue, which is
// exactly the shape a contributor submits to plant one.
func (s *RevisionServiceIntegrationTestSuite) TestRollback_RefusesHostileOldValueOnEveryURLField() {
	cases := []struct{ field, hostile, legitimate string }{
		{"spotify", "https://spotify-account-verify.evil.test/", "https://open.spotify.com/artist/4Z8W4fKeB5YxbusRsdQVPb"},
		{"bandcamp", "https://evil.test/phish", "https://realband.bandcamp.com"},
		{"instagram", "https://instagram.evil.test/login", "https://instagram.com/realband"},
		{"facebook", "https://facebook.evil.test/", "https://facebook.com/realband"},
		{"twitter", "https://twitter.evil.test/", "https://twitter.com/realband"},
		{"youtube", "https://youtube.evil.test/", "https://youtube.com/@realband"},
		{"soundcloud", "https://soundcloud.evil.test/", "https://soundcloud.com/realband"},
		// website is host-unrestricted by design, so only the scheme rule guards
		// it — but that rule must still run on the way back.
		{"website", "javascript:alert(1)", "https://realband.example.org"},
		// No platform to anchor to, so the scheme rule is the whole guard — and
		// it is still the difference between restoring a link and restoring a
		// javascript: or data: URL into a rendered attribute.
		{"image_url", "javascript:alert(1)", "https://cdn.example.org/a.jpg"},
		{"cover_art_url", "data:text/html,evil", "https://cdn.example.org/c.jpg"},
		{"flyer_url", "javascript:alert(1)", "https://cdn.example.org/f.jpg"},
	}

	for _, c := range cases {
		s.Run(c.field, func() {
			admin := s.createTestUser()
			artist := s.createTestArtist(
				fmt.Sprintf("Rollback %s %d", c.field, time.Now().UnixNano()), "Phoenix", "AZ", "")

			changes := []adminm.FieldChange{{
				Field:    c.field,
				OldValue: c.hostile,
				NewValue: c.legitimate,
			}}
			s.Require().NoError(s.svc.RecordRevision("artist", artist.ID, admin.ID, changes, "fix their link"))

			var revision adminm.Revision
			s.Require().NoError(s.db.Where("entity_type = ? AND entity_id = ?", "artist", artist.ID).
				Order("id DESC").First(&revision).Error)

			err := s.svc.Rollback(revision.ID, admin.ID)
			s.Require().Error(err, "rollback must refuse to write %q into %s", c.hostile, c.field)

			var stored map[string]interface{}
			s.Require().NoError(s.db.Table("artists").
				Where("id = ?", artist.ID).Take(&stored).Error)
			s.Nil(stored[c.field], "the hostile old_value must not reach the entity")
		})
	}
}

// The gate must not close ordinary undo: a legitimate old value on a social
// field still restores.
func (s *RevisionServiceIntegrationTestSuite) TestRollback_RestoresValidSocialURL() {
	admin := s.createTestUser()
	artist := s.createTestArtist(
		fmt.Sprintf("Rollback Social OK %d", time.Now().UnixNano()), "Phoenix", "AZ", "")

	const previous = "https://instagram.com/oldhandle"
	s.Require().NoError(s.db.Table("artists").Where("id = ?", artist.ID).
		Update("instagram", "https://instagram.com/newhandle").Error)

	changes := []adminm.FieldChange{{
		Field:    "instagram",
		OldValue: previous,
		NewValue: "https://instagram.com/newhandle",
	}}
	s.Require().NoError(s.svc.RecordRevision("artist", artist.ID, admin.ID, changes, "handle change"))

	var revision adminm.Revision
	s.Require().NoError(s.db.Where("entity_type = ? AND entity_id = ?", "artist", artist.ID).
		Order("id DESC").First(&revision).Error)

	s.Require().NoError(s.svc.Rollback(revision.ID, admin.ID))

	var stored map[string]interface{}
	s.Require().NoError(s.db.Table("artists").Where("id = ?", artist.ID).Take(&stored).Error)
	s.Equal(previous, stored["instagram"])
}

// A rollback that touches no URL field at all is untouched by any of this.
func (s *RevisionServiceIntegrationTestSuite) TestRollback_IgnoresNonURLFields() {
	admin := s.createTestUser()
	artist := s.createTestArtist(
		fmt.Sprintf("Rollback Plain %d", time.Now().UnixNano()), "Phoenix", "AZ", "")

	s.Require().NoError(s.db.Table("artists").Where("id = ?", artist.ID).
		Update("description", "new blurb").Error)

	changes := []adminm.FieldChange{{
		Field:    "description",
		OldValue: "old blurb",
		NewValue: "new blurb",
	}}
	s.Require().NoError(s.svc.RecordRevision("artist", artist.ID, admin.ID, changes, "blurb"))

	var revision adminm.Revision
	s.Require().NoError(s.db.Where("entity_type = ? AND entity_id = ?", "artist", artist.ID).
		Order("id DESC").First(&revision).Error)

	s.Require().NoError(s.svc.Rollback(revision.ID, admin.ID))

	var stored map[string]interface{}
	s.Require().NoError(s.db.Table("artists").Where("id = ?", artist.ID).Take(&stored).Error)
	s.Equal("old blurb", stored["description"])
}
