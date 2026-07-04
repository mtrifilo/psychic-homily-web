package notification

import (
	catalogm "psychic-homily-backend/internal/models/catalog"
	notificationm "psychic-homily-backend/internal/models/notification"
	"psychic-homily-backend/internal/services/contracts"
)

// Scene-follow fan-out tests (PSY-1341) — run inside NotificationFilterSuite
// (real Postgres, all migrations). Scene rows + follows are seeded directly:
// the registry's get-or-create is catalog's concern (tested there); this suite
// owns the notify semantics.

// seedSceneFollow creates a fallback-scope scene row for Phoenix/AZ (matching
// createTestVenue's city/state, no metro) and a follow for the user, with the
// optional notify mode stored in settings.
func (s *NotificationFilterSuite) seedSceneFollow(userID uint, mode string) uint {
	var sceneID uint
	s.Require().NoError(s.db.Raw(`
		INSERT INTO scenes (metro, city, state, slug)
		VALUES (NULL, 'Phoenix', 'AZ', 'phoenix-az')
		ON CONFLICT DO NOTHING
		RETURNING id`).Scan(&sceneID).Error)
	if sceneID == 0 {
		s.Require().NoError(s.db.Raw(`SELECT id FROM scenes WHERE slug = 'phoenix-az'`).Scan(&sceneID).Error)
	}
	if mode == "" {
		s.Require().NoError(s.db.Exec(`
			INSERT INTO user_bookmarks (user_id, entity_type, entity_id, action, created_at)
			VALUES (?, 'scene', ?, 'follow', now())`, userID, sceneID).Error)
	} else {
		s.Require().NoError(s.db.Exec(`
			INSERT INTO user_bookmarks (user_id, entity_type, entity_id, action, created_at, settings)
			VALUES (?, 'scene', ?, 'follow', now(), jsonb_build_object('scene_notify_mode', ?::text))`,
			userID, sceneID, mode).Error)
	}
	return sceneID
}

func (s *NotificationFilterSuite) followArtist(userID, artistID uint) {
	s.Require().NoError(s.db.Exec(`
		INSERT INTO user_bookmarks (user_id, entity_type, entity_id, action, created_at)
		VALUES (?, 'artist', ?, 'follow', now())`, userID, artistID).Error)
}

func (s *NotificationFilterSuite) sceneLogCount(userID, showID uint) int64 {
	var n int64
	s.Require().NoError(s.db.Model(&notificationm.NotificationLog{}).
		Where("user_id = ? AND entity_type = 'show' AND entity_id = ? AND channel = 'email'", userID, showID).
		Count(&n).Error)
	return n
}

func (s *NotificationFilterSuite) loadShow(showID uint) *catalogm.Show {
	var show catalogm.Show
	s.Require().NoError(s.db.First(&show, showID).Error)
	return &show
}

func (s *NotificationFilterSuite) TestSceneFollow_DefaultModeNotifiesAllShows() {
	userID := s.createTestUser()
	s.seedSceneFollow(userID, "")

	artistID := s.createTestArtist("Some Band")
	venueID := s.createTestVenue("The Rebel Lounge")
	showID := s.createTestShow("Scene Show", []uint{artistID}, []uint{venueID})

	// No filters exist at all — the scene fan-out must still run (the filter
	// pass's zero-match case must not early-return past it).
	s.Require().NoError(s.svc.MatchAndNotify(s.loadShow(showID)))
	s.Equal(int64(1), s.sceneLogCount(userID, showID))

	// Idempotent on re-approval: the dedup check, not the UNIQUE constraint
	// (filter_id NULLs compare distinct), prevents a second row.
	s.Require().NoError(s.svc.MatchAndNotify(s.loadShow(showID)))
	s.Equal(int64(1), s.sceneLogCount(userID, showID))
}

func (s *NotificationFilterSuite) TestSceneFollow_FollowedBandsOnlyGate() {
	fan := s.createTestUser()     // follows the artist → notified
	tourist := s.createTestUser() // follows only the scene → gated out
	s.seedSceneFollow(fan, "followed_bands_only")
	s.seedSceneFollow(tourist, "followed_bands_only")

	artistID := s.createTestArtist("Followed Band")
	s.followArtist(fan, artistID)

	venueID := s.createTestVenue("The Rebel Lounge")
	showID := s.createTestShow("Gated Show", []uint{artistID}, []uint{venueID})

	s.Require().NoError(s.svc.MatchAndNotify(s.loadShow(showID)))
	s.Equal(int64(1), s.sceneLogCount(fan, showID))
	s.Equal(int64(0), s.sceneLogCount(tourist, showID))
}

func (s *NotificationFilterSuite) TestSceneFollow_DedupsAgainstFilterMatch() {
	userID := s.createTestUser()
	s.seedSceneFollow(userID, "")

	artistID := s.createTestArtist("Filtered Band")
	venueID := s.createTestVenue("The Rebel Lounge")

	// A filter matching the artist — the user would match via BOTH systems.
	_, err := s.svc.CreateFilter(userID, contracts.CreateFilterInput{
		Name:        "Filtered Band shows",
		ArtistIDs:   []int64{int64(artistID)},
		NotifyEmail: true,
		NotifyInApp: true,
	})
	s.Require().NoError(err)

	showID := s.createTestShow("Doubly Matched Show", []uint{artistID}, []uint{venueID})
	s.Require().NoError(s.svc.MatchAndNotify(s.loadShow(showID)))

	// Exactly one notification: the filter's. The scene pass defers.
	s.Equal(int64(1), s.sceneLogCount(userID, showID))
}
