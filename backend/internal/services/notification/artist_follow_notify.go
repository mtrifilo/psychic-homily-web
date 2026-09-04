package notification

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"sort"
	"time"

	"github.com/getsentry/sentry-go"
	"github.com/lib/pq"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	authm "psychic-homily-backend/internal/models/auth"
	catalogm "psychic-homily-backend/internal/models/catalog"
	engagementm "psychic-homily-backend/internal/models/engagement"
	notificationm "psychic-homily-backend/internal/models/notification"
	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/services/engagement"
)

// artistFollowEntityType is the bookmark entity type these alerts subscribe on.
//
// Read from the model rather than spelled as a literal because it is load
// bearing twice over: it is half of the WHERE clause that finds the followers,
// AND the value engagement.followAlertHasScopeAxis compares against to decide
// whether artist show alerts have a scope axis at all. A literal that drifted
// from the constant would not fail loudly — it would silently resolve every
// follow to no scope, which reads as "everywhere" and mails people about the
// whole country.
var artistFollowEntityType = string(engagementm.BookmarkEntityArtist)

// Artist new-show alerts (PSY-1896).
//
// This is the DELIVERY half of the artist alert loop. The other two halves
// already shipped and are consumed, not reimplemented, here:
//
//   - SUBSCRIPTION is the follow itself (PSY-1893). There is no subscription
//     table: a row in user_bookmarks with entity_type='artist', action='follow'
//     IS the subscription, and its settings JSONB carries whatever the user
//     overrode. engagement.ResolveFollowAlerts layers that over the account
//     matrix over the shipped defaults, and it is the ONLY thing that decides
//     whether a channel is on.
//   - TRIGGER is the show becoming publicly visible (PSY-1894). This function
//     runs inside MatchAndNotify, so it fires from both routes that already
//     decide a show is announceable: the admin approve handlers, and the
//     show-notify outbox poller that drains ingest-created shows.
//
// # Where it sits in MatchAndNotify, and why there
//
// Three systems can tell one user about one show, and the product semantic is
// ONE notification per (user, show) across all of them. They run most specific
// first, so the most informative sentence wins the single slot:
//
//	criteria filters  ->  artist follows (here)  ->  scene follows
//
// A filter is an explicit query the user wrote, so it outranks an implicit
// follow. A followed band outranks "something is happening in your city".
// Each stage skips a user an earlier stage already reached, and the scene stage
// was taught to count this stage's rows for the same reason.
//
// # Scope
//
// Each artist follow carries near_me or everywhere (default near_me, PSY-1892
// decision 2). near_me matches the user's home CBSA against the show's VENUE
// metros, never city strings: city equality silently drops a Mesa show for a
// Phoenix-area user, which is the finding that put this rule in the ticket.
// A user with no home area cannot be scoped, so engagement.EffectiveShowScope
// degrades them to everywhere rather than delivering nothing.
//
// # Fail open
//
// Decision 10 lives here: a venue whose metro cannot be resolved never EXCLUDES
// a show. Missing derived data is a gap in our own geocoding, and the cost of
// being wrong in the two directions is not symmetric — an extra alert about a
// band you follow is noise, while a silently dropped one looks to the user like
// the feature does not work.

// artistFollowerRow is one artist follow whose target is on this show's bill.
// One row per (user, followed artist), so a user who follows three bands on the
// same bill appears three times and is collapsed below.
type artistFollowerRow struct {
	UserID     uint             `gorm:"column:user_id"`
	ArtistID   uint             `gorm:"column:artist_id"`
	ArtistName string           `gorm:"column:artist_name"`
	Position   int              `gorm:"column:position"`
	Settings   *json.RawMessage `gorm:"column:settings"`
}

// artistAlertLane is one channel's delivery: which followed artist the alert is
// attributed to, and the scope that follow resolved to.
type artistAlertLane struct {
	artistID   uint
	artistName string
	scope      string
}

// artistAlertRecipient is what one user receives for one show: at most one
// delivery per CHANNEL LANE, each attributed independently.
//
// Per lane rather than one winner for both, because a user's follows can
// disagree about channels and collapsing them to a single winner drops one. The
// case that forced this: following the headliner with in-app on and email off,
// and an opener with in-app off and email on. A single winner picks one follow
// and writes only its lane, so the user gets an email and NO bell entry despite
// having explicitly enabled in-app alerts for the headliner.
//
// Attributing each lane separately also keeps the copy honest, which is what a
// plain OR of the channels would have broken: the email names the artist whose
// follow actually has email switched on, never a bandmate who does not.
type artistAlertRecipient struct {
	userID uint
	inApp  *artistAlertLane
	email  *artistAlertLane
}

// delivers reports whether this recipient has anything to receive.
func (r artistAlertRecipient) delivers() bool { return r.inApp != nil || r.email != nil }

// recipientAlertPrefs is a user's account-level alert row. Read in bulk for the
// whole follower set: GetFollowAlertSettings would cost two queries per user.
type recipientAlertPrefs struct {
	UserID        uint             `gorm:"column:user_id"`
	HomeMetro     *string          `gorm:"column:home_metro"`
	AlertDefaults *json.RawMessage `gorm:"column:alert_defaults"`
}

// notifyArtistFollowers fans a newly visible show out to the followers of the
// artists on its bill. Best-effort like the rest of MatchAndNotify: every error
// is logged and swallowed, because a notification problem must never fail the
// approval or ingest write that triggered it.
func (s *NotificationFilterService) notifyArtistFollowers(show *catalogm.Show, showArtistIDs pq.Int64Array) {
	if show == nil || len(showArtistIDs) == 0 {
		return
	}

	// show is the CANONICAL row: MatchAndNotify re-reads it and fences the whole
	// fanout on its status before any pass runs, so Status and SubmittedBy are
	// trustworthy here even though half the call sites hand over a partial
	// literal. See the comment on that read.

	followers, err := s.artistFollowersForShow(show.ID, showArtistIDs)
	if err != nil {
		log.Printf("artist-follow notify: %v", err)
		return
	}
	if len(followers) == 0 {
		return
	}

	prefs, err := s.alertPrefsForUsers(followers)
	if err != nil {
		log.Printf("artist-follow notify: %v", err)
		return
	}

	area, err := s.resolveShowArea(show.ID)
	if err != nil {
		log.Printf("artist-follow notify: %v", err)
		return
	}

	// Self-exclusion, matching the scene-follow pass: whoever entered the show
	// does not need to be told it exists.
	var submitter uint
	if show.SubmittedBy != nil {
		submitter = *show.SubmittedBy
	}

	recipients := resolveArtistAlertRecipients(followers, prefs, area, submitter)
	if len(recipients) == 0 {
		return
	}

	alreadyNotified, err := s.usersAlreadyNotifiedAboutShow(show.ID, recipients)
	if err != nil {
		log.Printf("artist-follow notify: %v", err)
		return
	}

	now := time.Now().UTC()
	for _, r := range recipients {
		if _, done := alreadyNotified[r.userID]; done {
			continue
		}
		s.deliverArtistAlert(r, show, now)
	}
}

// loadShowForAlert reads the show back by id. A missing row is (nil, nil): a
// show deleted between the trigger and this goroutine is an expected outcome,
// not an error worth retrying.
func (s *NotificationFilterService) loadShowForAlert(showID uint) (*catalogm.Show, error) {
	var show catalogm.Show
	err := s.db.Where("id = ?", showID).First(&show).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("load show %d for alert: %w", showID, err)
	}
	return &show, nil
}

// artistFollowersForShow returns every artist follow whose target plays this
// show, joined to the artist's name and its slot on the bill.
//
// Restricted to the show's own artist ids (which MatchAndNotify already
// gathered) as well as joining show_artists, so the planner has a bounded set to
// probe user_bookmarks with instead of scanning every artist follow in the
// table.
func (s *NotificationFilterService) artistFollowersForShow(
	showID uint,
	showArtistIDs pq.Int64Array,
) ([]artistFollowerRow, error) {
	var rows []artistFollowerRow
	err := s.db.Raw(`
		SELECT b.user_id,
		       b.entity_id AS artist_id,
		       a.name      AS artist_name,
		       -- NOT NULL DEFAULT 0 in the schema, so this only ever reads the
		       -- stored value. Worth knowing that an ingest path which never sets
		       -- it puts the WHOLE bill at 0, which makes the lane tie-break in
		       -- resolveArtistAlertRecipients fall through to artist id.
		       sa.position AS position,
		       b.settings
		FROM user_bookmarks b
		JOIN show_artists sa ON sa.artist_id = b.entity_id AND sa.show_id = ?
		JOIN artists a ON a.id = b.entity_id
		WHERE b.entity_type = ?
		  AND b.action = ?
		  AND b.entity_id = ANY(?)
	`, showID, artistFollowEntityType, string(engagementm.BookmarkActionFollow), showArtistIDs).Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("artist followers query for show %d: %w", showID, err)
	}
	return rows, nil
}

// alertPrefsForUsers reads the account alert row for every follower in one
// query. A user with no row is absent from the map, which resolves to the
// shipped defaults and no home area — exactly what a NULL row means.
// Its whole job over alertPrefsForUserIDs is the DE-DUPLICATION: an artist
// follower list legitimately repeats a user (three bands on one bill), and a
// venue one cannot. Having it collapse the ids and delegate keeps one copy of
// the query rather than two that drift.
func (s *NotificationFilterService) alertPrefsForUsers(
	followers []artistFollowerRow,
) (map[uint]recipientAlertPrefs, error) {
	ids := make([]uint, 0, len(followers))
	seen := make(map[uint]struct{}, len(followers))
	for _, f := range followers {
		if _, ok := seen[f.UserID]; ok {
			continue
		}
		seen[f.UserID] = struct{}{}
		ids = append(ids, f.UserID)
	}
	return s.alertPrefsForUserIDs(ids)
}

// showAreaMetros is the show's area for near-me matching: the set of CBSA codes
// its venues resolve to, plus whether any venue failed to resolve one.
type showAreaMetros struct {
	// codes holds every resolved venue metro. Plural because a show can list
	// more than one venue.
	codes map[string]struct{}
	// unresolved is true when the show has no venues at all, or when at least
	// one venue carries no metro. Either way the show's location cannot be
	// ruled OUT of a user's home area, so near-me delivers (decision 10).
	unresolved bool
}

// matches reports whether a show in this area should reach a near-me subscriber
// whose home area is homeMetro.
//
// Fail-open is applied per VENUE, not per show: one unresolvable venue is enough
// to deliver, even when a sibling venue resolved elsewhere. A multi-venue show
// is rare and a partially-geocoded one rarer still, and in that corner the
// honest statement is that we do not know where the show is.
func (a showAreaMetros) matches(homeMetro string) bool {
	if a.unresolved {
		return true
	}
	_, ok := a.codes[homeMetro]
	return ok
}

// resolveShowArea reads the show's venues into a showAreaMetros. venues.metro is
// derived by the same offline Census dataset that fills user_preferences.
// home_metro, so the two are directly comparable as codes.
func (s *NotificationFilterService) resolveShowArea(showID uint) (showAreaMetros, error) {
	type venueMetroRow struct {
		Metro *string `gorm:"column:metro"`
	}
	var rows []venueMetroRow
	err := s.db.Raw(`
		SELECT v.metro
		FROM show_venues sv
		JOIN venues v ON v.id = sv.venue_id
		WHERE sv.show_id = ?
	`, showID).Scan(&rows).Error
	if err != nil {
		return showAreaMetros{}, fmt.Errorf("venue metros for show %d: %w", showID, err)
	}

	area := showAreaMetros{codes: make(map[string]struct{}, len(rows))}
	if len(rows) == 0 {
		// A show with no venue row at all. Nothing to compare, so nothing to
		// exclude on.
		area.unresolved = true
		return area, nil
	}
	for _, r := range rows {
		if r.Metro == nil || *r.Metro == "" {
			area.unresolved = true
			continue
		}
		area.codes[*r.Metro] = struct{}{}
	}
	return area, nil
}

// resolveArtistAlertRecipients collapses each user's qualifying follows into the
// at-most-one-per-lane alert they will receive.
//
// A user can follow several bands on one bill, and those follows can disagree
// about scope and channels. A follow QUALIFIES when its alerts are enabled, at
// least one channel is on, and the show's area satisfies its scope. Among the
// qualifying follows, each CHANNEL LANE is claimed independently by the first
// follow (in bill order) that enables it.
//
// Per lane rather than one winner for both, and per lane rather than OR-ing the
// channels together, because the two obvious shortcuts each break something:
//
//   - One winner DROPS A LANE. Follow the headliner with in-app on and email
//     off, and an opener with in-app off and email on: whichever follow wins
//     writes only its own lane, and the user loses a channel they explicitly
//     turned on.
//   - OR-ing the channels onto one winner LIES. It would send an email whose
//     body says "you follow <headliner>" on the strength of an opt-in the user
//     made for the opener, and "why am I getting this" is the one sentence in an
//     alert email that has to be true.
//
// Claiming each lane separately gives both properties: no enabled channel is
// dropped, and each message names a follow that actually enables the channel it
// arrived on.
func resolveArtistAlertRecipients(
	followers []artistFollowerRow,
	prefs map[uint]recipientAlertPrefs,
	area showAreaMetros,
	submitter uint,
) []artistAlertRecipient {
	// Deterministic order in, deterministic attribution out. The follower query
	// has no ORDER BY (the planner may return rows in any order), and "highest on
	// the bill wins the lane" only means anything if ties break the same way on
	// every run.
	//
	// NOTE on position: show_artists.position is NOT NULL DEFAULT 0, so an ingest
	// path that never sets it puts the whole bill at 0 and this degenerates to
	// lowest artist id. That is an arbitrary but STABLE choice, which is all this
	// tie-break has to be.
	ordered := make([]artistFollowerRow, len(followers))
	copy(ordered, followers)
	sort.SliceStable(ordered, func(i, j int) bool {
		if ordered[i].Position != ordered[j].Position {
			return ordered[i].Position < ordered[j].Position
		}
		return ordered[i].ArtistID < ordered[j].ArtistID
	})

	byUser := make(map[uint]*artistAlertRecipient, len(ordered))
	order := make([]uint, 0, len(ordered))

	for _, f := range ordered {
		if f.UserID == submitter && submitter != 0 {
			continue
		}

		account := prefs[f.UserID]
		resolved := engagement.ResolveFollowAlerts(
			artistFollowEntityType, f.ArtistID, f.Settings,
			authm.ResolveAccountAlertDefaults(account.AlertDefaults),
		)
		pref := resolved.Shows

		if !pref.Enabled || (!pref.InApp && !pref.Email) {
			continue
		}

		homeMetro := ""
		if account.HomeMetro != nil {
			homeMetro = *account.HomeMetro
		}
		scope := engagement.EffectiveShowScope(pref.Scope, homeMetro != "")
		if scope == contracts.FollowAlertScopeNearMe && !area.matches(homeMetro) {
			continue
		}

		recipient := byUser[f.UserID]
		if recipient == nil {
			recipient = &artistAlertRecipient{userID: f.UserID}
			byUser[f.UserID] = recipient
			order = append(order, f.UserID)
		}

		// Each lane is claimed by the FIRST qualifying follow that enables it.
		// `ordered` is bill order, so that is the highest-billed such follow, and
		// a lane one follow leaves off is still available to a later one. This is
		// what stops a follow of the opener from silencing the in-app alert the
		// user turned on for the headliner.
		lane := &artistAlertLane{artistID: f.ArtistID, artistName: f.ArtistName, scope: scope}
		if pref.InApp && recipient.inApp == nil {
			recipient.inApp = lane
		}
		if pref.Email && recipient.email == nil {
			recipient.email = lane
		}
	}

	out := make([]artistAlertRecipient, 0, len(order))
	for _, userID := range order {
		if r := byUser[userID]; r.delivers() {
			out = append(out, *r)
		}
	}
	return out
}

// usersAlreadyNotifiedAboutShow returns the subset of recipients some other
// system has already told about this show.
//
// Shares notifiedAboutShow with the filter and scene passes, so all three agree
// about which rows count as "already told". One query for the whole set rather
// than one per user, which is what the filter pass still does.
func (s *NotificationFilterService) usersAlreadyNotifiedAboutShow(
	showID uint,
	recipients []artistAlertRecipient,
) (map[uint]struct{}, error) {
	ids := make([]uint, 0, len(recipients))
	for _, r := range recipients {
		ids = append(ids, r.userID)
	}

	var notified []uint
	err := s.db.Model(&notificationm.NotificationLog{}).
		Where("user_id IN ? AND entity_id = ?", ids, showID).
		Where(notifiedAboutShow("notification_log")).
		Distinct().
		Pluck("user_id", &notified).Error
	if err != nil {
		return nil, fmt.Errorf("cross-system dedup for show %d: %w", showID, err)
	}

	out := make(map[uint]struct{}, len(notified))
	for _, id := range notified {
		out[id] = struct{}{}
	}
	return out, nil
}

// deliverArtistAlert writes this recipient's alert rows and, when the email lane
// is on and its row was newly claimed, sends the email.
//
// One row per LANE, and the insert is what claims the lane. Both go through
// ON CONFLICT DO NOTHING against uq_notification_log_artist_show_alert, so a
// re-processed outbox job or a second poller replica finds RowsAffected == 0 and
// does nothing at all. That is the exactly-once guarantee, held by the database
// rather than by a Count-then-Create that races (the scene pass's known bug).
//
// The in-app row is written FIRST and its result does not gate the email, so a
// user with in-app off still gets the email they asked for.
func (s *NotificationFilterService) deliverArtistAlert(
	r artistAlertRecipient,
	show *catalogm.Show,
	now time.Time,
) {
	if r.inApp != nil {
		if _, err := s.claimArtistAlertRow(
			r.userID, *r.inApp, show.ID, notificationm.NotificationChannelInApp, now,
		); err != nil {
			log.Printf("artist-follow notify: in-app row for user %d, show %d: %v", r.userID, show.ID, err)
		}
	}

	if r.email == nil {
		return
	}
	if s.emailService == nil || !s.emailService.IsConfigured() {
		return
	}

	// The daily budget is checked BEFORE the lane is claimed. Claiming first
	// looks safer and is not: the claim is PERMANENT — the partial UNIQUE means a
	// later attempt finds RowsAffected == 0 — so a row claimed and then refused by
	// the budget is an email that can never be sent, to a user who may have in-app
	// switched off and would therefore receive nothing at all.
	//
	// Checking first costs the opposite risk, that the count is stale by the time
	// the send happens. That is the same window every other sender here already
	// accepts, and it errs toward delivering an email the user asked for.
	if !s.withinDailyAlertEmailBudget(r.userID) {
		log.Printf("rate limit: skipping artist-alert email for user %d", r.userID)
		return
	}

	// The lane is claimed BEFORE the send, not after. A crash between the two
	// loses an email; claiming after would risk sending two, and a duplicate alert
	// is the failure the recipient notices and the one that costs sending
	// reputation.
	claimed, err := s.claimArtistAlertRow(
		r.userID, *r.email, show.ID, notificationm.NotificationChannelEmail, now)
	if err != nil {
		log.Printf("artist-follow notify: email row for user %d, show %d: %v", r.userID, show.ID, err)
		return
	}
	if !claimed {
		return
	}
	s.sendArtistShowAlertEmail(r.userID, *r.email, show)
}

// withinDailyAlertEmailBudget reports whether the user has room in the daily
// allowance for FOLLOW-DRIVEN ALERT emails.
//
// It counts only the email-lane rows of the two-lane alert types, and that
// narrowness is the point rather than an oversight.
//
// The obvious alternative — reusing the whole-channel count the filter and
// scene senders do — looked like sharing one budget and is not, because that
// count is over ROWS, not over emails. Both of those writers stamp
// channel='email' unconditionally, including on rows that are a user's only
// IN-APP record and for which no mail was ever sent. A user following a busy
// scene with in-app-only delivery therefore accumulates a full day's "email"
// allowance without receiving a single message, and an alert email they
// explicitly opted into would be refused. Worse, it would be refused
// PERMANENTLY: the in-app lane for that show is already claimed, so the next
// outbox pass skips the user entirely and never retries.
//
// Counting this feature's own sent-email rows keeps the cap meaning what it
// says. maxFilterEmailsPerDay is reused as the threshold so there is one number
// to tune rather than two.
func (s *NotificationFilterService) withinDailyAlertEmailBudget(userID uint) bool {
	var emailCount int64
	dayAgo := time.Now().UTC().Add(-24 * time.Hour)
	if err := s.db.Model(&notificationm.NotificationLog{}).
		Where("user_id = ? AND channel = ? AND sent_at > ? AND entity_type IN ?",
			userID, notificationm.NotificationChannelEmail, dayAgo, emailLaneAlertTypes).
		Count(&emailCount).Error; err != nil {
		// Fail CLOSED. An unreadable budget is not permission to send: the cap
		// exists to bound outbound mail, and an unbounded burst is the failure it
		// was put there to prevent.
		log.Printf("artist-follow notify: daily email budget check for user %d: %v", userID, err)
		return false
	}
	return emailCount < int64(maxFilterEmailsPerDay)
}

// claimArtistAlertRow inserts one lane's row, reporting whether THIS call
// created it. A false with no error means someone already claimed the lane.
func (s *NotificationFilterService) claimArtistAlertRow(
	userID uint,
	lane artistAlertLane,
	showID uint,
	channel string,
	now time.Time,
) (bool, error) {
	artistID := lane.artistID
	row := notificationm.NotificationLog{
		UserID: userID,
		// No filter row backs a follow-driven alert. The read path uses this
		// NULL plus the entity_type to pick the row's label.
		FilterID:        nil,
		EntityType:      notificationm.NotificationEntityArtistShowAlert,
		EntityID:        showID,
		SubjectEntityID: &artistID,
		Channel:         channel,
		SentAt:          now,
	}
	res := s.db.Clauses(clause.OnConflict{DoNothing: true}).Create(&row)
	if res.Error != nil {
		return false, res.Error
	}
	return res.RowsAffected > 0, nil
}

// sendArtistShowAlertEmail renders and sends the alert. The caller has already
// checked the daily budget and claimed the email lane.
//
// lane is the EMAIL lane's attribution, so the artist this message names is one
// whose follow actually has email switched on.
func (s *NotificationFilterService) sendArtistShowAlertEmail(
	userID uint,
	lane artistAlertLane,
	show *catalogm.Show,
) {
	var email string
	if err := s.db.Table("users").Where("id = ?", userID).Pluck("email", &email).Error; err != nil || email == "" {
		log.Printf("artist-follow notify: no email for user %d: %v", userID, err)
		return
	}

	c := s.showEmailContent(show)
	unsubscribeURL := engagement.GenerateScopedUnsubscribeURL(
		engagement.DeriveBackendURL(s.frontendURL),
		userID,
		engagement.UnsubscribeScopeArtistShowAlerts,
		s.jwtSecret,
	)
	manageURL := fmt.Sprintf("%s/settings/notifications", s.frontendURL)

	html := buildArtistShowAlertEmailHTML(lane.artistName, lane.scope, c, unsubscribeURL, manageURL)
	// The subject is a HEADER, so HTML escaping does nothing for it: a CR or LF
	// in an artist name is how a header is split and another one injected. The
	// body builders escape their own inputs; this does not go through them.
	subject := fmt.Sprintf("%s announced a show", sanitizeEmailHeaderValue(lane.artistName))

	if err := s.sendEmail(email, subject, html, unsubscribeURL); err != nil {
		sentry.WithScope(func(scope *sentry.Scope) {
			scope.SetTag("service", "notification_filter")
			scope.SetTag("email_type", "artist_show_alert")
			sentry.CaptureException(err)
		})
		log.Printf("artist-follow notify: failed to send alert email to user %d: %v", userID, err)
	}
}

// buildArtistShowAlertEmailHTML renders the alert in the shared direction-A
// layout (PSY-1902), so it reads as the same publication as the verification
// email rather than as a second house style.
//
// Every string reaching it is escaped by the builders, which matters here more
// than in most templates: artist names, show titles and venue names on an
// ingest-created show are scraped third-party text, and this message is sent
// from the platform's own DKIM-aligned sender.
func buildArtistShowAlertEmailHTML(
	artistName, scope string,
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

	// The scope sentence is the answer to "why this show and not the rest of the
	// tour", which is the question a scoped alert invites and which no other
	// surface answers at the moment the user is asking it.
	why := fmt.Sprintf("You follow %s. Your show alerts for them cover every city.", artistName)
	if scope == contracts.FollowAlertScopeNearMe {
		why = fmt.Sprintf(
			"You follow %s. Your show alerts for them are set to your home area, so dates outside it will not reach you.",
			artistName)
	}

	body := emailHeadline(fmt.Sprintf("%s announced a show.", artistName)) +
		emailMonoDetails(details) +
		emailParagraph(why) +
		emailButton(c.showURL, "View show") +
		emailFineprintWithLinks(
			[]string{fmt.Sprintf("You are getting this because you follow %s with email alerts on.", artistName)},
			[]emailFineprintLink{
				{Href: unsubscribeURL, Label: "Unsubscribe from artist show alerts"},
				{Href: manageURL, Label: "Manage alerts in Settings"},
			},
		)

	return emailShell("ARTIST ALERT · NEW SHOW", body)
}
