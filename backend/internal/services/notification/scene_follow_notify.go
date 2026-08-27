package notification

import (
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/getsentry/sentry-go"
	"github.com/lib/pq"

	authm "psychic-homily-backend/internal/models/auth"
	catalogm "psychic-homily-backend/internal/models/catalog"
	engagementm "psychic-homily-backend/internal/models/engagement"
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
// # Channels (PSY-1926)
//
// The MODE decides which of the scene's shows qualify. It has never decided
// whether a qualifying show is EMAILED: until PSY-1926 the only gate on the
// email was whether an email provider was configured at all, so following a
// scene silently started a per-show email stream nobody opted into. That
// contradicts PSY-1892 decision 4 (every email alert off until the user turns
// it on), and it was the last stream in the product still doing it.
//
// The channels now resolve through engagement.ResolveFollowAlerts, the same
// three-layer chain artist and venue alerts use: shipped defaults, then the
// user's account alert matrix, then the follow's own stored overrides. Email is
// off in the shipped layer, so an existing scene follow that stored no override
// resolves to email OFF with no data migration: absent has always meant
// inherit, and what it now inherits is the locked posture.
//
// The IN-APP lane is deliberately NOT switchable here, and
// engagement.followAlertHasInAppAxis is where that is stated: the single row
// this pass writes is at once the bell entry and the cross-system dedup marker
// notifiedAboutShow reads, so it cannot be suppressed without letting the next
// pass over the same show notify the user again. Turning scene notifications
// off entirely remains scene_notify_mode's job.

// sceneFollowEntityType is the bookmark entity type these follows subscribe on.
// Read from the model rather than spelled as a literal because it is the value
// engagement.followAlertHasInAppAxis compares against: a literal that drifted
// would silently start reading an in-app switch this pass cannot honour.
var sceneFollowEntityType = string(engagementm.BookmarkEntityScene)

// sceneFollower is one scene follow joined with its notify mode.
type sceneFollower struct {
	UserID    uint             `gorm:"column:user_id"`
	SceneID   uint             `gorm:"column:scene_id"`
	Mode      *string          `gorm:"column:mode"`
	Settings  *json.RawMessage `gorm:"column:settings"`
	SceneCity string           `gorm:"column:city"`
	SceneSt   string           `gorm:"column:state"`
	SceneSlug string           `gorm:"column:slug"`
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

	// The account alert matrix for every follower, read in bulk. One query for
	// the whole set rather than one per follow, matching the artist and venue
	// passes; a user with no preferences row is absent from the map, which
	// resolves to the shipped defaults, which is what a NULL row means.
	//
	// A failed read ABANDONS the pass rather than falling back to the shipped
	// defaults. Substituting them would be a guess about an opt-in, and the
	// direction it guesses wrong in is emailing somebody who never asked.
	userIDs := make([]uint, 0, len(followers))
	seen := make(map[uint]struct{}, len(followers))
	for _, f := range followers {
		if _, dup := seen[f.UserID]; dup {
			continue
		}
		seen[f.UserID] = struct{}{}
		userIDs = append(userIDs, f.UserID)
	}
	prefs, err := s.alertPrefsForUserIDs(userIDs)
	if err != nil {
		log.Printf("scene-follow notify: %v", err)
		return
	}

	// Group per user: a show can map to multiple followed scene rows (multi-
	// venue shows, scope-drift duplicates), and the user qualifies if ANY of
	// their follows does — an explicit "all" subscription on one scene must
	// not be vetoed by a stricter (or off) mode on another (review-caught:
	// iteration order was deciding). "off" contributes to NEITHER bucket, so
	// a scene followed with "off" can never veto a qualifying follow on
	// another scene; a user whose EVERY matching follow is "off" gets no
	// notification at all (checked below).
	//
	// Email aggregates the SAME way and for the same reason: one qualifying
	// follow with email switched on is an opt-in, and a sibling follow that
	// left email off must not veto it. emailCity/emailSt record the scene of
	// the FIRST such follow (rows arrive ordered by scene id), so the message
	// names a scene whose follow actually has email on rather than whichever
	// row the planner happened to return first. That is the attribution rule
	// the artist pass states at length; scenes get the same honesty for free
	// because the ordering is deterministic.
	type userAgg struct {
		anyAll               bool
		anyFollowedBandsOnly bool
		city, st             string
		emailOn              bool
		emailCity, emailSt   string
	}
	byUser := make(map[uint]*userAgg, len(followers))
	for _, f := range followers {
		// The subscription's channels, resolved per FOLLOW: two follows of the
		// same user can carry different overrides.
		resolved := engagement.ResolveFollowAlerts(
			sceneFollowEntityType, f.SceneID, f.Settings,
			authm.ResolveAccountAlertDefaults(prefs[f.UserID].AlertDefaults),
		)
		pref := resolved.Shows
		if !pref.Enabled {
			// The subscription's master switch, off. Contributes nothing, the
			// same as mode "off", so it cannot veto a sibling follow either.
			continue
		}

		agg := byUser[f.UserID]
		if agg == nil {
			agg = &userAgg{city: f.SceneCity, st: f.SceneSt}
			byUser[f.UserID] = agg
		}
		mode := engagement.SceneNotifyModeAll
		if f.Mode != nil {
			mode = *f.Mode
		}
		switch mode {
		case engagement.SceneNotifyModeOff:
			// Contributes nothing — must not veto another qualifying follow.
			// Its channels contribute nothing either: an "off" follow is not a
			// place to read an email opt-in from.
			continue
		case engagement.SceneNotifyModeFollowedBands:
			agg.anyFollowedBandsOnly = true
		default:
			// "all" and any unrecognized/legacy value default to "all"
			// (matches FollowService.SceneNotifyMode's read-side default).
			agg.anyAll = true
		}
		if pref.Email && !agg.emailOn {
			agg.emailOn = true
			agg.emailCity, agg.emailSt = f.SceneCity, f.SceneSt
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
			if !agg.anyFollowedBandsOnly {
				// Every matching follow for this user is "off" — no
				// qualifying subscription, regardless of artist follows.
				continue
			}
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
		// bell notification — an ARTIST-follow alert from the pass that runs
		// immediately before this one, or a prior approval cycle). One
		// notification per (user, show) across all three systems is the
		// deliberate semantic, and notifiedAboutShow is the one place that
		// spells which rows count. The table's UNIQUE includes filter_id — NULLs
		// compare distinct — so this check, not the constraint, is what prevents
		// scene-follow duplicates.
		var existing int64
		if err := s.db.Model(&notificationm.NotificationLog{}).
			Where("user_id = ? AND entity_id = ?", f.UserID, show.ID).
			Where(notifiedAboutShow("notification_log")).
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
			EntityType: notificationm.NotificationEntityShow,
			EntityID:   show.ID,
			Channel:    notificationm.NotificationChannelEmail,
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
		//
		// The row's channel is stamped 'email' whether or not one is sent, and
		// that is not a bug to tidy away: on this path the column marks a LANE,
		// and notifiedAboutShow keys the cross-system dedup on exactly
		// (entity_type='show', channel='email'). A row stamped otherwise would
		// stop counting as "already told" and the filter pass would notify the
		// same user about the same show again.
		if !agg.emailOn {
			continue
		}
		if s.emailService != nil && s.emailService.IsConfigured() {
			sceneName := fmt.Sprintf("%s, %s", agg.emailCity, agg.emailSt)
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
		       b.entity_id AS scene_id,
		       b.settings->>'scene_notify_mode' AS mode,
		       b.settings,
		       ss.city, ss.state, ss.slug
		FROM user_bookmarks b
		JOIN show_scenes ss ON ss.id = b.entity_id
		WHERE b.entity_type = 'scene' AND b.action = 'follow'
		-- Deterministic, because attribution reads the FIRST qualifying row:
		-- which scene the email names, and which scene's city the in-app row
		-- is labelled with, would otherwise be the planner's choice and could
		-- differ between two runs over the same data.
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
// already established that this user's subscription has email switched on and
// has written the notification row.
//
// Two things changed here in PSY-1926, and both were the same defect wearing
// different clothes: the message advertised RFC 8058 one-click unsubscribe over
// a FRONTEND page (/following?tab=scene, which redirects to the library), so
// the POST a mailbox provider sends could not be honoured, and the link a human
// clicked landed somewhere that could not turn the email off either. It now
// signs the shared show-alert scope and points at the backend route that serves
// both verbs.
func (s *NotificationFilterService) sendSceneFollowEmail(userID uint, sceneName string, show *catalogm.Show) {
	if !s.withinDailySceneEmailBudget(userID) {
		log.Printf("rate limit: skipping scene-follow email for user %d", userID)
		return
	}

	var email string
	if err := s.db.Table("users").Where("id = ?", userID).Pluck("email", &email).Error; err != nil || email == "" {
		log.Printf("scene-follow notify: no email for user %d: %v", userID, err)
		return
	}

	// The SAME scope the artist and venue show-alert emails sign. All three are
	// one stream to the recipient and, more to the point, one stream to the
	// SETTING: alert_defaults carries a single `shows` key covering them, and
	// UserService.UnsubscribeArtistShowAlertEmails sweeps scene follows as well.
	// A scene-specific scope would mint a second URL performing an identical
	// mutation, which is two names for one action rather than extra precision.
	unsubscribeURL := engagement.GenerateScopedUnsubscribeURL(
		engagement.DeriveBackendURL(s.frontendURL),
		userID,
		engagement.UnsubscribeScopeArtistShowAlerts,
		s.jwtSecret,
	)
	manageURL := fmt.Sprintf("%s/settings/notifications", s.frontendURL)

	c := s.showEmailContent(show)
	html := buildSceneShowAlertEmailHTML(sceneName, c, unsubscribeURL, manageURL)

	// The subject is a HEADER. The scene name is assembled from our own scenes
	// registry rather than scraped, but it is sanitized on the same rule the
	// sibling senders follow: a CR or LF anywhere in a header value is how a
	// header is split and another one injected, and which strings are "ours" is
	// not a fact a future editor of this line should have to re-derive.
	subject := fmt.Sprintf("New show in %s", sanitizeEmailHeaderValue(sceneName))

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
// allowance for scene-follow emails.
//
// It counts the same rows this pass writes (entity_type='show',
// channel='email') against the one shared threshold, which is the comparison
// this sender has always used. What changed is the failure mode: the count's
// error used to be dropped, leaving emailCount at zero, so an unreadable budget
// READ AS EMPTY and the send proceeded. A cap exists to bound outbound mail and
// an unbounded burst is the exact failure it was put there to prevent, so it
// now fails CLOSED like the sibling budget in artist_follow_notify.go.
func (s *NotificationFilterService) withinDailySceneEmailBudget(userID uint) bool {
	var emailCount int64
	dayAgo := time.Now().UTC().Add(-24 * time.Hour)
	if err := s.db.Model(&notificationm.NotificationLog{}).
		Where("user_id = ? AND channel = ? AND sent_at > ?",
			userID, notificationm.NotificationChannelEmail, dayAgo).
		Count(&emailCount).Error; err != nil {
		log.Printf("scene-follow notify: daily email budget check for user %d: %v", userID, err)
		return false
	}
	return emailCount < int64(maxFilterEmailsPerDay)
}

// buildSceneShowAlertEmailHTML renders the scene alert in the shared
// direction-A layout (PSY-1902), so it reads as the same publication as its
// artist and venue siblings.
//
// It replaces a call to buildFilterEmailHTML, whose template is a CRITERIA
// FILTER's template: it headlined `New show matching "Phoenix, AZ scene"` as
// though the user had authored a query, and its footer offered to "Pause this
// filter" over a link that pauses nothing. Borrowing a neighbouring message's
// copy is how an email ends up describing a feature the reader does not have.
//
// Every string reaching this is escaped by the builders. That matters here more
// than in most templates: show titles, artist names and venue names on an
// ingest-created show are scraped third-party text, and this message ships from
// the platform's own DKIM-aligned sender.
func buildSceneShowAlertEmailHTML(
	sceneName string,
	c showEmailContentParts,
	unsubscribeURL, manageURL string,
) string {
	details := []string{
		fmt.Sprintf("WHEN .... %s", c.date),
	}
	if c.venueText != "" {
		details = append(details, fmt.Sprintf("WHERE ... %s", c.venueText))
	}
	if c.artistText != "" {
		details = append(details, fmt.Sprintf("WITH .... %s", c.artistText))
	}
	if c.priceText != "" {
		details = append(details, fmt.Sprintf("PRICE ... %s", c.priceText))
	}

	body := emailHeadline(fmt.Sprintf("A new show in the %s scene.", sceneName)) +
		emailMonoDetails(details) +
		emailParagraph(fmt.Sprintf(
			"You follow %s. Which shows count is set on the scene itself: every show, or only the bands you follow.",
			sceneName)) +
		emailButton(c.showURL, "View show") +
		emailFineprintWithLinks(
			[]string{fmt.Sprintf(
				"You are getting this because you follow %s with email alerts on.", sceneName)},
			[]emailFineprintLink{
				{Href: unsubscribeURL, Label: "Unsubscribe from show alerts"},
				{Href: manageURL, Label: "Manage alerts in Settings"},
			},
		)

	return emailShell("SCENE ALERT · NEW SHOW", body)
}
