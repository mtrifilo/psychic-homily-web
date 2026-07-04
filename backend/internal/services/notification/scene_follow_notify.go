package notification

import (
	"fmt"
	"log"
	"time"

	"github.com/lib/pq"

	catalogm "psychic-homily-backend/internal/models/catalog"
	notificationm "psychic-homily-backend/internal/models/notification"
)

// Scene-follow new-show notifications (PSY-1341, from the PSY-1314 spike).
// Runs inside MatchAndNotify AFTER the filter pass, so both admin approval
// call sites get it and the cross-system dedup below can defer to filter
// notifications already logged for the same show.
//
// Deliberately NOT modeled as auto-managed notification_filters rows: a
// filter's artist_ids is a static snapshot, and the "followed bands only"
// mode must track the user's LIVE artist follows.

const (
	sceneNotifyModeAll           = "all"
	sceneNotifyModeFollowedBands = "followed_bands_only"
)

// sceneFollower is one scene follow joined with its notify mode.
type sceneFollower struct {
	UserID    uint    `gorm:"column:user_id"`
	Mode      *string `gorm:"column:mode"`
	SceneCity string  `gorm:"column:city"`
	SceneSt   string  `gorm:"column:state"`
	SceneSlug string  `gorm:"column:slug"`
}

// notifySceneFollowers fans a newly approved show out to followers of its
// scene(s). Best-effort like the rest of the pipeline: errors are logged, the
// approval flow never fails on notification problems.
func (s *NotificationFilterService) notifySceneFollowers(show *catalogm.Show, showArtistIDs pq.Int64Array) {
	followers, err := s.sceneFollowersForShow(show.ID)
	if err != nil {
		log.Printf("scene-follow notify: %v", err)
		return
	}
	if len(followers) == 0 {
		return
	}

	// Group per user: a show can map to multiple followed scene rows (multi-
	// venue shows, scope-drift duplicates), and the user qualifies if ANY of
	// their follows does — an explicit "all" subscription on one scene must
	// not be vetoed by a stricter mode on another (review-caught: iteration
	// order was deciding).
	type userAgg struct {
		anyAll   bool
		city, st string
	}
	byUser := make(map[uint]*userAgg, len(followers))
	for _, f := range followers {
		agg := byUser[f.UserID]
		if agg == nil {
			agg = &userAgg{city: f.SceneCity, st: f.SceneSt}
			byUser[f.UserID] = agg
		}
		if f.Mode == nil || *f.Mode != sceneNotifyModeFollowedBands {
			agg.anyAll = true
		}
	}

	// Self-exclusion: the submitter following their own scene shouldn't be
	// emailed about the show they entered.
	var submitter uint
	if show.SubmittedBy != nil {
		submitter = *show.SubmittedBy
	}

	now := time.Now().UTC()
	for userID, agg := range byUser {
		f := sceneFollower{UserID: userID, SceneCity: agg.city, SceneSt: agg.st}
		if userID == submitter && submitter != 0 {
			continue
		}
		if !agg.anyAll {
			ok, err := s.userFollowsAnyArtist(f.UserID, showArtistIDs)
			if err != nil {
				log.Printf("scene-follow notify: artist intersection for user %d: %v", f.UserID, err)
				continue
			}
			if !ok {
				continue
			}
		}

		// Cross-system dedup: skip anyone already notified about this show (a
		// filter match — including in-app-only filters, whose log row IS the
		// bell notification — or a prior approval cycle). One notification per
		// (user, show) across both systems is the deliberate semantic. The
		// table's UNIQUE includes filter_id — NULLs compare distinct — so this
		// check, not the constraint, is what prevents scene-follow duplicates.
		var existing int64
		if err := s.db.Model(&notificationm.NotificationLog{}).
			Where("user_id = ? AND entity_type = ? AND entity_id = ? AND channel = ?",
				f.UserID, "show", show.ID, "email").
			Count(&existing).Error; err != nil {
			log.Printf("scene-follow notify: dedup check for user %d: %v", f.UserID, err)
			continue
		}
		if existing > 0 {
			continue
		}

		logEntry := notificationm.NotificationLog{
			UserID:     f.UserID,
			FilterID:   nil, // scene follows have no filter row
			EntityType: "show",
			EntityID:   show.ID,
			Channel:    "email",
			SentAt:     now,
		}
		if err := s.db.Create(&logEntry).Error; err != nil {
			log.Printf("scene-follow notify: log insert for user %d, show %d: %v", f.UserID, show.ID, err)
			continue
		}

		// Log row first, email best-effort — the same order as the filter
		// path: the row is the durable in-app record (the bell reads it), and
		// a rate-limited or failed email doesn't erase that the user was
		// notified in-app.
		if s.emailService != nil && s.emailService.IsConfigured() {
			sceneName := fmt.Sprintf("%s, %s", f.SceneCity, f.SceneSt)
			s.sendSceneFollowEmail(f.UserID, sceneName, show)
		}
	}
}

// sceneFollowersForShow resolves the show's venue(s) to existing scene
// registry rows (metro scope first, city/state fallback — mirroring the
// catalog sceneScope keying) and returns their followers with notify modes.
// Rows materialize lazily (PSY-1339), so "no scenes row" simply means no
// followers — nothing is created here.
func (s *NotificationFilterService) sceneFollowersForShow(showID uint) ([]sceneFollower, error) {
	var followers []sceneFollower
	err := s.db.Raw(`
		WITH show_scenes AS (
			SELECT DISTINCT sc.id, sc.city, sc.state, sc.slug
			FROM show_venues sv
			JOIN venues v ON v.id = sv.venue_id
			JOIN scenes sc ON (
				(v.metro IS NOT NULL AND sc.metro = v.metro)
				-- Fallback rows match by normalized city/state REGARDLESS of the
				-- venue's metro: a later venue-metro backfill must not strand the
				-- followers of a pre-existing fallback row (it converges once
				-- upgrade-scene-scopes runs). Normalization mirrors the canonical
				-- venuePredicate matching in catalog/scene.go.
				OR (sc.metro IS NULL
					AND LOWER(TRIM(sc.city)) = LOWER(TRIM(v.city))
					AND LOWER(TRIM(sc.state)) = LOWER(TRIM(v.state)))
			)
			WHERE sv.show_id = ?
		)
		SELECT b.user_id,
		       b.settings->>'scene_notify_mode' AS mode,
		       ss.city, ss.state, ss.slug
		FROM user_bookmarks b
		JOIN show_scenes ss ON ss.id = b.entity_id
		WHERE b.entity_type = 'scene' AND b.action = 'follow'
	`, showID).Scan(&followers).Error
	if err != nil {
		return nil, fmt.Errorf("scene followers query: %w", err)
	}
	return followers, nil
}

// userFollowsAnyArtist reports whether the user follows at least one of the
// show's artists — the "followed bands only" gate, checked against LIVE
// artist follows at notify time.
func (s *NotificationFilterService) userFollowsAnyArtist(userID uint, artistIDs pq.Int64Array) (bool, error) {
	if len(artistIDs) == 0 {
		return false, nil
	}
	var n int64
	err := s.db.Table("user_bookmarks").
		Where("user_id = ? AND entity_type = 'artist' AND action = 'follow' AND entity_id = ANY(?)",
			userID, artistIDs).
		Count(&n).Error
	if err != nil {
		return false, err
	}
	return n > 0, nil
}
