package notification

import (
	"fmt"
	"log"
	"time"

	"github.com/getsentry/sentry-go"
	"github.com/lib/pq"

	authm "psychic-homily-backend/internal/models/auth"
	catalogm "psychic-homily-backend/internal/models/catalog"
	notificationm "psychic-homily-backend/internal/models/notification"
	"psychic-homily-backend/internal/services/engagement"
)

// Scene-follow new-show notifications (PSY-1341, from the PSY-1314 spike;
// +off mode in PSY-1466). Runs inside MatchAndNotify AFTER the filter pass,
// so both admin approval call sites get it and the cross-system dedup below
// can defer to filter notifications already logged for the same show.
//
// Deliberately NOT modeled as auto-managed notification_filters rows: a
// filter's artist_ids is a static snapshot, and the "followed bands only"
// mode must track the user's LIVE artist follows.
//
// Mode constants are shared from the engagement package (the single owner
// of scene_notify_mode's accepted values) rather than duplicated here.
//
// Channels. scene_notify_mode decides WHICH shows qualify. Whether a qualifying
// show is also emailed is the account alert matrix's `shows` email channel, the
// same setting that governs artist and venue show-alert email, and it defaults
// OFF. Scene follows carry no per-follow alert overrides (scenes are absent from
// engagement's followAlertEntityTypes, so no write path stores one), which makes
// the account row the whole email gate: the show-alert unsubscribe clears it and
// thereby silences this stream completely.
//
// The in-app row is written for every qualifying user whatever the matrix's
// in-app channel says. That row is also the cross-system dedup marker
// notifiedAboutShow reads, so suppressing it would let a later pass announce
// the same show again. Mode "off" is how a scene is silenced.

// sceneFollower is one scene follow joined with its notify mode.
type sceneFollower struct {
	UserID    uint    `gorm:"column:user_id"`
	Mode      *string `gorm:"column:mode"`
	SceneCity string  `gorm:"column:city"`
	SceneSt   string  `gorm:"column:state"`
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
	// not be vetoed by a stricter (or off) mode on another (review-caught:
	// iteration order was deciding). "off" contributes nothing, so a scene
	// followed with "off" can never veto a qualifying follow on another scene,
	// and a user whose EVERY matching follow is "off" never enters byUser.
	//
	// The scene an email names is the first follow of the mode that made the
	// user qualify (rows arrive ordered by scene id): an "all" follow when there
	// is one, otherwise a followed-bands one. An "off" follow is never named.
	// A non-empty field is also the record that a follow of that mode exists.
	type userAgg struct {
		allScene, bandsScene string
	}
	byUser := make(map[uint]*userAgg, len(followers))
	for _, f := range followers {
		mode := engagement.SceneNotifyModeAll
		if f.Mode != nil {
			mode = *f.Mode
		}
		if mode == engagement.SceneNotifyModeOff {
			continue
		}
		agg := byUser[f.UserID]
		if agg == nil {
			agg = &userAgg{}
			byUser[f.UserID] = agg
		}
		// "all" and any unrecognized/legacy value default to "all"
		// (matches FollowService.SceneNotifyMode's read-side default).
		slot := &agg.allScene
		if mode == engagement.SceneNotifyModeFollowedBands {
			slot = &agg.bandsScene
		}
		if *slot == "" {
			*slot = fmt.Sprintf("%s, %s", f.SceneCity, f.SceneSt)
		}
	}
	if len(byUser) == 0 {
		return
	}

	// The account alert matrix for every candidate, in one query. A user with no
	// preferences row is absent from the map, which resolves to the shipped
	// defaults (email off). The matrix gates only the email, so a failed read
	// still writes every in-app row and sends no email in this pass: off is the
	// fail-closed direction for an opt-in.
	userIDs := make([]uint, 0, len(byUser))
	for userID := range byUser {
		userIDs = append(userIDs, userID)
	}
	prefs, err := s.alertPrefsForUserIDs(userIDs)
	emailGateReadable := err == nil
	if err != nil {
		log.Printf("scene-follow notify: %v; sending no scene emails for show %d", err, show.ID)
	}

	// Self-exclusion: the submitter following their own scene shouldn't be
	// emailed about the show they entered.
	var submitter uint
	if show.SubmittedBy != nil {
		submitter = *show.SubmittedBy
	}

	now := time.Now().UTC()
	for userID, agg := range byUser {
		if userID == submitter && submitter != 0 {
			continue
		}
		sceneName := agg.allScene
		if sceneName == "" {
			ok, err := s.userFollowsAnyArtist(userID, showArtistIDs)
			if err != nil {
				log.Printf("scene-follow notify: artist intersection for user %d: %v", userID, err)
				continue
			}
			if !ok {
				continue
			}
			sceneName = agg.bandsScene
		}

		// Cross-system dedup: skip anyone already notified about this show (a
		// filter match — including in-app-only filters, whose log row IS the
		// bell notification — an ARTIST-follow alert from the pass that runs
		// immediately before this one, or a prior approval cycle). One
		// notification per (user, show) across all three systems is the
		// deliberate semantic, and notifiedAboutShow is the one place that
		// spells which rows count. The table's UNIQUE includes filter_id — NULLs
		// compare distinct — so this check, not the constraint, is what prevents
		// scene-follow duplicates.
		var existing int64
		if err := s.db.Model(&notificationm.NotificationLog{}).
			Where("user_id = ? AND entity_id = ?", userID, show.ID).
			Where(notifiedAboutShow("notification_log")).
			Count(&existing).Error; err != nil {
			log.Printf("scene-follow notify: dedup check for user %d: %v", userID, err)
			continue
		}
		if existing > 0 {
			continue
		}

		// Channel is stamped 'email' whether or not a message is sent: on this
		// path it marks the lane notifiedAboutShow keys the dedup on, not a send.
		logEntry := notificationm.NotificationLog{
			UserID:     userID,
			FilterID:   nil, // scene follows have no filter row
			EntityType: notificationm.NotificationEntityShow,
			EntityID:   show.ID,
			Channel:    notificationm.NotificationChannelEmail,
			SentAt:     now,
		}
		if err := s.db.Create(&logEntry).Error; err != nil {
			log.Printf("scene-follow notify: log insert for user %d, show %d: %v", userID, show.ID, err)
			continue
		}

		// Log row first, email best-effort — the same order as the filter
		// path: the row is the durable in-app record (the bell reads it), and
		// a rate-limited or failed email doesn't erase that the user was
		// notified in-app.
		if !emailGateReadable || !authm.ResolveAccountAlertDefaults(prefs[userID].AlertDefaults).Shows.Email {
			continue
		}
		if s.emailService != nil && s.emailService.IsConfigured() {
			s.sendSceneFollowEmail(userID, sceneName, show)
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
			SELECT DISTINCT sc.id, sc.city, sc.state
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
		       ss.city, ss.state
		FROM user_bookmarks b
		JOIN show_scenes ss ON ss.id = b.entity_id
		WHERE b.entity_type = 'scene' AND b.action = 'follow'
		-- Ordered so the scene an email names does not depend on the planner.
		ORDER BY b.user_id, b.entity_id
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

// ──────────────────────────────────────────────
// Email
// ──────────────────────────────────────────────

// sendSceneFollowEmail renders and sends the scene alert. The caller has
// already established that the user's account show-alert email is on and has
// written the notification row.
//
// The unsubscribe URL targets the BACKEND route, which serves the RFC 8058
// one-click POST as well as the human GET. A frontend URL cannot honour the POST.
func (s *NotificationFilterService) sendSceneFollowEmail(userID uint, sceneName string, show *catalogm.Show) {
	if !s.withinDailySceneEmailBudget(userID, show.ID) {
		log.Printf("rate limit: skipping scene-follow email for user %d", userID)
		return
	}

	var email string
	if err := s.db.Table("users").Where("id = ?", userID).Pluck("email", &email).Error; err != nil || email == "" {
		log.Printf("scene-follow notify: no email for user %d: %v", userID, err)
		return
	}

	// The same scope the artist and venue show-alert emails sign: one account
	// `shows` email setting governs all three, and this scope's handler clears it.
	unsubscribeURL := engagement.GenerateScopedUnsubscribeURL(
		engagement.DeriveBackendURL(s.frontendURL),
		userID,
		engagement.UnsubscribeScopeArtistShowAlerts,
		s.jwtSecret,
	)
	manageURL := fmt.Sprintf("%s/settings/notifications", s.frontendURL)

	c := s.showEmailContent(show)
	html := buildSceneShowAlertEmailHTML(sceneName, c, unsubscribeURL, manageURL)
	subject := fmt.Sprintf("New show in %s", entityNameForSubject(sceneName))

	if err := s.sendEmail(email, subject, html, unsubscribeURL); err != nil {
		sentry.WithScope(func(scope *sentry.Scope) {
			scope.SetTag("service", "notification_filter")
			scope.SetTag("email_type", "scene_follow")
			sentry.CaptureException(err)
		})
		log.Printf("scene-follow notify: failed to send alert email to user %d: %v", userID, err)
	}
}

// withinDailySceneEmailBudget reports whether the user has room in the daily
// allowance for scene-follow emails. It fails CLOSED: an unreadable budget is
// not permission to send.
//
// It counts every channel='email' row from the last day except this show's own
// scene row, which the caller has already written. The filter and scene writers
// stamp that channel on in-app-only rows too, so the count can exceed the mail
// actually sent and refuse an email the user opted into: the hazard
// withinDailyAlertEmailBudget in artist_follow_notify.go avoids by counting
// only email-lane rows. Scene rows cannot be counted that way until the scene
// pass writes separate in-app and email rows.
func (s *NotificationFilterService) withinDailySceneEmailBudget(userID, showID uint) bool {
	var emailCount int64
	dayAgo := time.Now().UTC().Add(-24 * time.Hour)
	if err := s.db.Model(&notificationm.NotificationLog{}).
		Where("user_id = ? AND channel = ? AND sent_at > ?",
			userID, notificationm.NotificationChannelEmail, dayAgo).
		Where("NOT (entity_type = ? AND entity_id = ? AND filter_id IS NULL)",
			notificationm.NotificationEntityShow, showID).
		Count(&emailCount).Error; err != nil {
		log.Printf("scene-follow notify: daily email budget check for user %d: %v", userID, err)
		return false
	}
	return emailCount < int64(maxFilterEmailsPerDay)
}

// buildSceneShowAlertEmailHTML renders the scene alert in the shared layout its
// artist and venue siblings use. It must not reuse the criteria-filter body,
// whose copy describes a user-authored filter that a scene follow does not have.
//
// Every string reaching it is escaped by the layout builders; show titles and
// artist and venue names can be scraped third-party text.
func buildSceneShowAlertEmailHTML(
	sceneName string,
	c showEmailContentParts,
	unsubscribeURL, manageURL string,
) string {
	body := emailHeadline(fmt.Sprintf("A new show in the %s scene.", sceneName)) +
		emailMonoDetails(c.detailLines()) +
		emailButton(c.showURL, "View show") +
		emailFineprintWithLinks(
			[]string{fmt.Sprintf(
				"You are getting this because you follow %s and show alert emails are on in your settings.", sceneName)},
			[]emailFineprintLink{
				{Href: unsubscribeURL, Label: "Unsubscribe from show alerts"},
				{Href: manageURL, Label: "Manage alerts in Settings"},
			},
		)

	return emailShell("SCENE ALERT · NEW SHOW", body)
}
