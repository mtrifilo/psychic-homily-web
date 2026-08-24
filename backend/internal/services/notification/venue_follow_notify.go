package notification

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"sort"
	"strings"
	"time"

	"github.com/getsentry/sentry-go"
	"github.com/lib/pq"
	"gorm.io/gorm"

	authm "psychic-homily-backend/internal/models/auth"
	catalogm "psychic-homily-backend/internal/models/catalog"
	engagementm "psychic-homily-backend/internal/models/engagement"
	notificationm "psychic-homily-backend/internal/models/notification"
	"psychic-homily-backend/internal/services/engagement"
	"psychic-homily-backend/internal/services/shared"
	"psychic-homily-backend/internal/utils"
)

// Venue new-show alerts (PSY-1895).
//
// This is the third follow-driven alert loop, and the first COALESCED one. The
// subscription (PSY-1893) and the trigger (PSY-1894) are consumed here, not
// reimplemented: a row in user_bookmarks with entity_type='venue',
// action='follow' IS the subscription, and MatchAndNotify already runs from
// both routes that decide a show is announceable.
//
// # Why this is not shaped like the artist alert
//
// A venue publishes its calendar in DROPS. One `discovery-import` run over a
// venue's events page is a season of dates entering the catalogue inside a few
// minutes, and PSY-1896's per-show shape would send a follower one notification
// per date. The owner decision (PSY-1892) is ONE alert per user per venue per
// venue-local calendar day.
//
// That splits the work in two, and the split is the whole design:
//
//   - ACCRUAL runs inside MatchAndNotify (accrueVenueShowAlerts). It records an
//     OBSERVATION — this show, at this venue, on this day — and resolves
//     nothing. It is one insert per followed venue, so it stays cheap enough to
//     sit on the show-write path, and it holds no opinion about who gets what.
//   - DELIVERY runs later, from the flush poller (FlushVenueShowAlerts). By then
//     the drop has finished, so followers, preferences, cross-system dedup and
//     the message itself are resolved ONCE against the whole day's set.
//
// Resolving at accrual time instead would mean deciding "who should hear about
// this" fifty times for one drop and then trying to suppress forty-nine of the
// answers, which is the same work with a race in it.
//
// # Exactly once, across re-runs that are DESIGNED to happen
//
// The flush deliberately re-resolves batches it has already delivered: a show
// announced later the same day joins an existing group, and the whole group is
// re-read so the inbox row can grow. Every such re-run reaches the delivery
// claim.
//
// What makes that silent is uq_notification_log_venue_show_alert, a partial
// UNIQUE on (user_id, entity_id, alert_bucket, channel) claimed with ON CONFLICT
// DO NOTHING. A second flush finds RowsAffected == 0 and sends nothing. This is
// the same mechanism PSY-1896 uses and NOT the Count-then-Create the scene pass
// still carries, which races. It is also what makes two concurrent flush ticks
// safe: the loser of the race sends no email, because the email is sent only
// when the claim succeeded.
//
// dispatched_at is bookkeeping, not the guard. Reading it as "already sent,
// skip" would be wrong in both directions: it would drop the late show from the
// user's row, and it would not by itself prevent a duplicate.
//
// # No scope axis
//
// engagement.followAlertHasScopeAxis("venue", "shows") is false, and this file
// must not invent one. A venue sits in one place, so "near me" has nothing to
// mean for it: either the user follows the venue or they do not. There is
// deliberately no call to EffectiveShowScope here, and a scope stored on a venue
// follow by some future client is stale data the resolver already ignores.
//
// # No backfill
//
// The rollout cannot alert anyone about the existing catalogue, and the reason
// is structural rather than a clock: venue_show_alert_batch ships EMPTY, and the
// only thing that writes to it is accrueVenueShowAlerts, which runs when a show
// becomes visible. A merge can move rows but never manufacture one.

// venueFollowEntityType is the bookmark entity type these alerts subscribe on.
// Read from the model rather than spelled as a literal, matching
// artistFollowEntityType: it is half of the WHERE clause that finds the
// followers AND the value engagement's resolver compares against.
var venueFollowEntityType = string(engagementm.BookmarkEntityVenue)

// alertDayLayout is how a venue-local calendar day is spelled everywhere it
// crosses the Go/SQL boundary.
//
// Days are passed to Postgres as STRINGS with an explicit ::date cast, never as
// a time.Time. A time.Time parameter arrives as a timestamptz and is cast to
// date using the SESSION time zone, so the same instant would land on different
// days depending on a setting this package does not control. A literal
// 'YYYY-MM-DD' has no such dependency.
const alertDayLayout = "2006-01-02"

// VenueShowAlertsEnabled reports whether the venue-alert loop is switched on.
//
// Read by BOTH halves — accrual and the flush poller — through the one flag name
// in models/notification. Gating only one is a trap either way: gating only the
// flush accrues rows that fan out in a burst when the flag clears, and gating
// only accrual leaves the poller spinning on a table nothing writes to.
func VenueShowAlertsEnabled() bool {
	return !shared.EnvServiceDisabled(notificationm.VenueShowAlertsDisableFlag)
}

// ──────────────────────────────────────────────
// Accrual (runs inside MatchAndNotify)
// ──────────────────────────────────────────────

// accruableVenue is a venue that has at least one follower, with the fields
// needed to decide which calendar day a show announced now belongs to.
type accruableVenue struct {
	ID       uint    `gorm:"column:id"`
	Timezone *string `gorm:"column:timezone"`
	State    string  `gorm:"column:state"`
}

// accrueVenueShowAlerts records this show's membership in each of its venues'
// batches for today.
//
// Best-effort like the rest of MatchAndNotify: every error is logged and
// swallowed, because a notification problem must never fail the approval or
// ingest write that triggered it. A dropped accrual costs one show its place in
// one day's alert, which is strictly better than failing the write.
//
// Only venues that ALREADY HAVE A FOLLOWER accrue. That is the write-side bound
// on a table which is deliberately never pruned (the inbox row reads its members
// live, so deleting them would blank delivered history).
//
// Be precise about what that bound does NOT do. It gates on whether the venue
// has ANY follower, not on which followers exist, and venueFollowers resolves
// the recipient list at FLUSH time with no created_at filter. So a user who
// follows the venue partway through the day DOES receive that day's whole batch,
// including shows announced before they subscribed — and because a late show
// re-opens an already-dispatched group, they can be enrolled as late as the end
// of the venue-local day.
//
// That is the intended behaviour rather than an accident: the alert's subject is
// "what is new at this venue today", the reader has just declared interest in
// exactly that venue, and the alternative (a same-day subscriber hearing nothing
// until tomorrow) is the silence the merged follow control exists to remove.
// Stated here because it is a real user-visible property that the accrual gate
// looks like it prevents and does not.
func (s *NotificationFilterService) accrueVenueShowAlerts(show *catalogm.Show, showVenueIDs pq.Int64Array) {
	if show == nil || len(showVenueIDs) == 0 {
		return
	}
	if !VenueShowAlertsEnabled() {
		return
	}

	// show is the CANONICAL row: MatchAndNotify re-reads it and fences the whole
	// fanout on its status before any pass runs.

	var venues []accruableVenue
	err := s.db.Raw(`
		SELECT v.id, v.timezone, v.state
		FROM venues v
		WHERE v.id = ANY(?)
		  AND EXISTS (
		        SELECT 1 FROM user_bookmarks b
		        WHERE b.entity_type = ?
		          AND b.action = ?
		          AND b.entity_id = v.id
		      )
	`, showVenueIDs, venueFollowEntityType, string(engagementm.BookmarkActionFollow)).Scan(&venues).Error
	if err != nil {
		log.Printf("venue-alert accrual: followed venues for show %d: %v", show.ID, err)
		return
	}

	now := time.Now()
	for _, v := range venues {
		// The day is resolved PER VENUE, because a show can list venues in
		// different zones and each venue's followers experience the drop on that
		// venue's clock.
		day := now.In(utils.EventLocation(v.Timezone, v.State)).Format(alertDayLayout)

		// ON CONFLICT DO NOTHING against the natural primary key, so re-running
		// MatchAndNotify for the same show (a reclaimed outbox row, an admin
		// approve after an ingest) accrues nothing new.
		if err := s.db.Exec(`
			INSERT INTO venue_show_alert_batch (venue_id, alert_day, show_id, created_at)
			VALUES (?, ?::date, ?, ?)
			ON CONFLICT DO NOTHING
		`, v.ID, day, show.ID, now).Error; err != nil {
			log.Printf("venue-alert accrual: venue %d, show %d, day %s: %v", v.ID, show.ID, day, err)
		}
	}
}

// ──────────────────────────────────────────────
// Flush (runs from the poller)
// ──────────────────────────────────────────────

// venueAlertGroupKey identifies one batch: a venue and a venue-local day.
type venueAlertGroupKey struct {
	VenueID  uint   `gorm:"column:venue_id"`
	AlertDay string `gorm:"column:alert_day"`
}

// venueAlertShow is one announceable member of a batch, with the fields the
// email and the inbox row need.
type venueAlertShow struct {
	ID          uint
	Title       string
	Slug        *string
	EventDate   time.Time
	SubmittedBy *uint
	ArtistText  string
}

// venueAlertBatch is one resolved batch: its venue, its day, and the members
// still worth announcing.
type venueAlertBatch struct {
	key       venueAlertGroupKey
	venueName string
	venueURL  string
	loc       *time.Location
	shows     []venueAlertShow
	// watermark is the instant the members were read. Only rows accrued at or
	// before it may be stamped dispatched, because only those were resolved.
	watermark time.Time
}

// venueFollowerRow is one venue follow. Exactly one row per (user, venue), so
// unlike the artist pass there is no bill order to break ties on and no lane
// contention to resolve: a user has one subscription to this batch's venue.
type venueFollowerRow struct {
	UserID   uint             `gorm:"column:user_id"`
	Settings *json.RawMessage `gorm:"column:settings"`
}

// venueAlertRecipient is what one user receives for one batch: the channels
// their single follow enables, and the members they have not already been told
// about by another system.
type venueAlertRecipient struct {
	userID uint
	inApp  bool
	email  bool
	shows  []venueAlertShow
}

// delivers reports whether this recipient has anything to receive. A recipient
// with no remaining shows delivers NOTHING even with both channels on: every
// show in the batch already reached them by a more specific route, and an alert
// that names nothing is worse than no alert.
func (r venueAlertRecipient) delivers() bool {
	return len(r.shows) > 0 && (r.inApp || r.email)
}

// FlushVenueShowAlerts resolves and delivers every batch that has gone quiet,
// returning how many batches it dispatched.
//
// Exported so the poller (and tests) can drive one pass without owning any of
// the resolution logic.
//
// ctx is honoured BETWEEN groups. A tick is up to `limit` groups and each group
// is one SYNCHRONOUS provider request per email recipient, which the per-user
// daily cap does not bound — so a popular venue is an unbounded number of
// sequential sends, and Stop() blocks on the in-flight tick. Without this check
// a deploy landing mid-drain waits for all of them and risks being SIGKILLed
// past the platform's grace period. Bailing is FREE here because nothing is
// claimed at group level: the rows stay undispatched and the next tick
// re-resolves them, silently, through the delivery claim.
func (s *NotificationFilterService) FlushVenueShowAlerts(
	ctx context.Context,
	limit int,
	quietWindow, maxHold, maxAge time.Duration,
) int {
	if s.db == nil {
		return 0
	}

	keys, err := s.venueAlertGroupsReadyToFlush(limit, quietWindow, maxHold)
	if err != nil {
		log.Printf("venue-alert flush: %v", err)
		return 0
	}

	dispatched := 0
	for _, key := range keys {
		if ctx.Err() != nil {
			log.Printf("venue-alert flush: tick canceled, %d groups left for the next tick",
				len(keys)-dispatched)
			break
		}
		if s.flushVenueAlertGroup(ctx, key, maxAge) {
			dispatched++
		}
	}
	return dispatched
}

// venueAlertGroupsReadyToFlush finds the batches whose drop looks finished.
//
// Two bounds, and only the second one is about correctness:
//
//   - QUIET WINDOW (MAX(created_at) older than the window). A drop arrives over
//     several minutes — the show-notify outbox drains a handful per tick at a
//     60s interval — so waiting for a gap is what makes one email name the whole
//     calendar instead of its first five entries. Each new accrual pushes the
//     deadline out.
//   - MAX HOLD (MIN(created_at) older than the hold). A venue with a continuous
//     trickle of accruals would never go quiet, and without this its followers
//     would be told nothing at all.
//
// The max hold is SAFE precisely because the quiet window is an optimisation
// rather than a guard. A batch flushed early delivers the shows it has; the rest
// of the drop accrues into the same (venue, day) group, the next flush re-reads
// it, the delivery claim no-ops, and the inbox row simply grows. The one visible
// cost is that the EMAIL, which is sent once, names only what had landed. That
// is a bounded, honest degradation, and the copy does not claim otherwise.
//
// Ordered oldest-first so a backlog drains in the order it arrived.
func (s *NotificationFilterService) venueAlertGroupsReadyToFlush(
	limit int,
	quietWindow, maxHold time.Duration,
) ([]venueAlertGroupKey, error) {
	now := time.Now()
	var keys []venueAlertGroupKey
	// alert_day comes back as TEXT, not as a time.Time, for the same reason it
	// goes in as one: a DATE scanned into time.Time is a midnight whose zone
	// depends on the driver, and this value is only ever used as a day.
	err := s.db.Raw(`
		SELECT venue_id, to_char(alert_day, 'YYYY-MM-DD') AS alert_day
		FROM venue_show_alert_batch
		WHERE dispatched_at IS NULL
		GROUP BY venue_id, alert_day
		HAVING MAX(created_at) < ? OR MIN(created_at) < ?
		ORDER BY MIN(created_at)
		LIMIT ?
	`, now.Add(-quietWindow), now.Add(-maxHold), limit).Scan(&keys).Error
	if err != nil {
		return nil, fmt.Errorf("venue alert groups ready to flush: %w", err)
	}
	return keys, nil
}

// flushVenueAlertGroup resolves and delivers one batch, then marks the rows it
// RESOLVED dispatched. Reports whether the batch was retired.
//
// The rows are marked AFTER delivery, never before. A crash in between costs a
// re-run, and a re-run is silent by construction (the delivery claim). Marking
// first would make a crash cost the alert entirely.
func (s *NotificationFilterService) flushVenueAlertGroup(
	ctx context.Context,
	key venueAlertGroupKey,
	maxAge time.Duration,
) bool {
	batch, err := s.loadVenueAlertBatch(key)
	if err != nil {
		log.Printf("venue-alert flush: %v", err)
		s.retireVenueAlertGroupIfTooOld(key, maxAge, err)
		return false
	}
	if batch == nil {
		// The venue is gone. Unreachable in practice — the foreign key CASCADES,
		// so a deleted venue takes its batches with it and this group could not
		// have been selected. Nothing to mark and nothing to send.
		return false
	}

	if len(batch.shows) == 0 {
		// Every member was deleted, cancelled, unpublished or has already
		// happened since it was accrued. Retire what we READ rather than
		// re-examining it forever: nothing here is announceable and nothing about
		// it will become announceable again inside this day.
		s.markVenueAlertGroupDispatched(key, batch.watermark)
		return true
	}

	if err := s.deliverVenueAlertBatch(ctx, batch); err != nil {
		// Leave the rows undispatched so the next tick retries. Every step is
		// idempotent, so a retry costs work rather than duplicate mail.
		log.Printf("venue-alert flush: %v", err)
		s.retireVenueAlertGroupIfTooOld(key, maxAge, err)
		return false
	}

	s.markVenueAlertGroupDispatched(key, batch.watermark)
	return true
}

// retireVenueAlertGroupIfTooOld is the poison-pill bound: a group that keeps
// failing is eventually stamped dispatched and abandoned, loudly.
//
// Without it a group whose delivery errors deterministically is retried forever,
// and the shape of the selection query makes that far worse than a wasted tick:
// venueAlertGroupsReadyToFlush orders by MIN(created_at) ASC and takes the first
// `limit`, so a permanently-failing group is by definition at the HEAD of that
// ordering and re-occupies a slot on every tick. Five of them (the default
// batch) and the venue-alert loop stops delivering for every venue on the
// platform, with nothing but a repeating log line to show for it.
//
// This is the same disposition the show-notify outbox gives an expired job, and
// for the same reason: an announcement that has been stuck for hours is no
// longer worth sending, and the bound has to be on the CLOCK rather than on an
// attempt counter, because the clock is the one thing that cannot be reset by a
// path that returns the row to the queue.
//
// Retiring stamps the WHOLE group (no watermark). That is deliberate: the point
// is to make the group stop being selected, and a watermark would leave the
// newest rows behind to re-select it immediately.
func (s *NotificationFilterService) retireVenueAlertGroupIfTooOld(
	key venueAlertGroupKey,
	maxAge time.Duration,
	cause error,
) {
	if maxAge <= 0 {
		return
	}

	var oldest time.Time
	err := s.db.Raw(`
		SELECT MIN(created_at) FROM venue_show_alert_batch
		WHERE venue_id = ? AND alert_day = ?::date AND dispatched_at IS NULL
	`, key.VenueID, key.AlertDay).Scan(&oldest).Error
	if err != nil || oldest.IsZero() || time.Since(oldest) <= maxAge {
		return
	}

	// Loud, and per-group: this is a silent user-visible loss otherwise. An
	// operator needs to know WHICH venue-day went unannounced, which an aggregate
	// counter cannot say.
	log.Printf("venue-alert flush: ABANDONING venue %d on %s after %s of failures; "+
		"its followers will not be told about that day's shows. last error: %v",
		key.VenueID, key.AlertDay, maxAge, cause)
	s.markVenueAlertGroupDispatched(key, time.Time{})
}

// loadVenueAlertBatch reads a batch's venue and its still-announceable members.
// A missing venue is (nil, nil).
func (s *NotificationFilterService) loadVenueAlertBatch(key venueAlertGroupKey) (*venueAlertBatch, error) {
	var venue struct {
		ID       uint
		Name     string
		Slug     *string
		Timezone *string
		State    string
	}
	err := s.db.Table("venues").
		Select("id, name, slug, timezone, state").
		Where("id = ?", key.VenueID).
		Take(&venue).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		// A vanished venue is an expected outcome, not an error to retry. In
		// practice it is unreachable: venue_show_alert_batch's foreign key
		// CASCADES, so a deleted venue takes its batches with it and this group
		// could not have been selected. Kept as defence, and kept SILENT because
		// there is nothing an operator would do about it.
		return nil, nil
	}
	if err != nil {
		// Every other failure is real and propagates, so the caller logs it and
		// leaves the batch undispatched for the next tick. Folding it into the
		// branch above would make a database outage look identical to a deleted
		// venue and lose the alert silently.
		return nil, fmt.Errorf("venue %d for alert batch: %w", key.VenueID, err)
	}

	// The WATERMARK is taken BEFORE the members are read, and it is what bounds
	// the dispatch stamp at the end of the flush.
	//
	// Delivery is slow — one synchronous provider request per email recipient —
	// and the show-notify outbox keeps accruing throughout. Stamping the whole
	// group afterwards would retire rows that arrived DURING delivery and were
	// never resolved, and the group would never be selected again. That is not
	// "the late show misses the email" (which is the documented, harmless case,
	// because it still joins the inbox row): on the paths that write no
	// notification_log row at all — every member unannounceable, no followers, or
	// a recipient whose whole batch was already covered elsewhere — it is an
	// announcement nobody ever receives.
	watermark := time.Now()

	var shows []catalogm.Show
	err = s.db.Raw(`
		SELECT s.*
		FROM shows s
		JOIN venue_show_alert_batch b ON b.show_id = s.id
		-- Re-validate the membership against the CURRENT bill. A show's venues
		-- are replaced wholesale by an ordinary edit and nothing touches the
		-- batch row, so without this join a follower of venue A can be mailed
		-- "new shows at A" naming a show that has since moved to venue B.
		JOIN show_venues sv ON sv.show_id = s.id AND sv.venue_id = b.venue_id
		WHERE b.venue_id = ? AND b.alert_day = ?::date
		  AND b.created_at <= ?
		  -- Announce the show that was APPROVED, not whatever it says now.
		  --
		  -- Unlike the artist alert, which sends inside MatchAndNotify with the
		  -- just-approved row, this send happens minutes later. A submitter can
		  -- edit their own approved show with no re-review, so that gap is a
		  -- window in which unreviewed text could be mailed to every follower of
		  -- a venue the submitter has no relationship with, from the platform's
		  -- own DKIM-aligned sender.
		  --
		  -- Dropping an edited show from the digest is the safe direction to
		  -- fail: it still reaches the inbox row, which is in-product and
		  -- attributable, whereas outbound mail is neither.
		  AND s.updated_at <= b.created_at
	`, key.VenueID, key.AlertDay, watermark).Scan(&shows).Error
	if err != nil {
		return nil, fmt.Errorf("venue alert batch members for venue %d on %s: %w",
			key.VenueID, key.AlertDay, err)
	}

	// Re-check announceability at DELIVERY time with the SAME predicate the
	// show-notify outbox uses at its own delivery. Between accrual and flush a
	// show can be unpublished, cancelled or deleted, and announcing something
	// that has been pulled is worse than announcing nothing. The window here is
	// the quiet window, not a poll interval, so it is minutes rather than
	// seconds.
	now := time.Now()
	members := make([]venueAlertShow, 0, len(shows))
	ids := make([]uint, 0, len(shows))
	for i := range shows {
		if ok, _ := catalogm.ShowAnnounceable(&shows[i], now); !ok {
			continue
		}
		members = append(members, venueAlertShow{
			ID:          shows[i].ID,
			Title:       shows[i].Title,
			Slug:        shows[i].Slug,
			EventDate:   shows[i].EventDate,
			SubmittedBy: shows[i].SubmittedBy,
		})
		ids = append(ids, shows[i].ID)
	}

	// Deterministic reading order: a digest that shuffled between renders would
	// look like different content. Event date ascending is also the order a
	// reader scanning a calendar expects.
	sort.SliceStable(members, func(i, j int) bool {
		if !members[i].EventDate.Equal(members[j].EventDate) {
			return members[i].EventDate.Before(members[j].EventDate)
		}
		return members[i].ID < members[j].ID
	})

	s.fillVenueAlertBills(members, ids)

	loc := utils.EventLocation(venue.Timezone, venue.State)
	return &venueAlertBatch{
		key:       key,
		venueName: venue.Name,
		venueURL:  entityURL(s.frontendURL, "venues", venue.Slug, venue.ID),
		loc:       loc,
		shows:     members,
		watermark: watermark,
	}, nil
}

// fillVenueAlertBills populates each member's bill in ONE query for the whole
// batch, rather than one per show. A season's calendar drop is a large batch and
// this runs on a poller tick shared with every other pending batch.
//
// A show with no artists keeps an empty bill and still appears: an
// artist-less listing is ordinary during ingest, and dropping it would make the
// count in the headline disagree with the list under it.
func (s *NotificationFilterService) fillVenueAlertBills(members []venueAlertShow, showIDs []uint) {
	if len(showIDs) == 0 {
		return
	}
	var rows []struct {
		ShowID uint   `gorm:"column:show_id"`
		Name   string `gorm:"column:name"`
	}
	err := s.db.Raw(`
		SELECT sa.show_id, a.name
		FROM show_artists sa
		JOIN artists a ON a.id = sa.artist_id
		WHERE sa.show_id = ANY(?)
		ORDER BY sa.show_id, sa.position ASC, a.id ASC
	`, pq.Array(showIDs)).Scan(&rows).Error
	if err != nil {
		log.Printf("venue-alert flush: bills for %d shows: %v", len(showIDs), err)
		return
	}

	byShow := make(map[uint][]string, len(showIDs))
	for _, r := range rows {
		byShow[r.ShowID] = append(byShow[r.ShowID], r.Name)
	}
	for i := range members {
		members[i].ArtistText = strings.Join(byShow[members[i].ID], ", ")
	}
}

// deliverVenueAlertBatch resolves the batch's recipients and delivers to each.
//
// Returns an error when ANY recipient's in-app row failed to be written, so the
// group stays undispatched and the next tick retries. Swallowing that failure
// and letting the caller retire the group would lose that user's bell entry
// permanently, with no path that ever reconsiders it — the email lane's
// claim-before-send trade is a deliberate one-way door, this was not.
//
// Retrying is safe by construction: every claim is ON CONFLICT DO NOTHING, so
// the recipients who already got through no-op on the second pass.
func (s *NotificationFilterService) deliverVenueAlertBatch(
	ctx context.Context,
	batch *venueAlertBatch,
) error {
	followers, err := s.venueFollowers(batch.key.VenueID)
	if err != nil {
		return err
	}
	if len(followers) == 0 {
		return nil
	}

	userIDs := make([]uint, 0, len(followers))
	for _, f := range followers {
		userIDs = append(userIDs, f.UserID)
	}

	prefs, err := s.alertPrefsForUserIDs(userIDs)
	if err != nil {
		return err
	}

	showIDs := make([]uint, 0, len(batch.shows))
	for _, sh := range batch.shows {
		showIDs = append(showIDs, sh.ID)
	}
	alreadyTold, err := s.showsAlreadyNotified(userIDs, showIDs)
	if err != nil {
		return err
	}

	recipients := resolveVenueAlertRecipients(followers, prefs, batch.shows, alreadyTold)

	now := time.Now().UTC()
	var failed int
	for i, r := range recipients {
		// ctx is honoured between RECIPIENTS as well as between groups, because
		// one group can be an unbounded number of sequential provider requests
		// and that is precisely what would overrun a shutdown. Returning an error
		// leaves the group undispatched, so the next tick finishes the list and
		// the claim keeps the already-delivered recipients silent.
		if ctx.Err() != nil {
			return fmt.Errorf("venue alert delivery for venue %d canceled after %d of %d recipients: %w",
				batch.key.VenueID, i, len(recipients), ctx.Err())
		}
		if err := s.deliverVenueAlert(r, batch, now); err != nil {
			log.Printf("venue-alert notify: user %d, venue %d: %v", r.userID, batch.key.VenueID, err)
			failed++
		}
	}
	if failed > 0 {
		return fmt.Errorf("venue alert delivery for venue %d: %d of %d recipients failed",
			batch.key.VenueID, failed, len(recipients))
	}
	return nil
}

// venueFollowers returns every follow of this venue.
func (s *NotificationFilterService) venueFollowers(venueID uint) ([]venueFollowerRow, error) {
	var rows []venueFollowerRow
	err := s.db.Raw(`
		SELECT b.user_id, b.settings
		FROM user_bookmarks b
		WHERE b.entity_type = ?
		  AND b.action = ?
		  AND b.entity_id = ?
	`, venueFollowEntityType, string(engagementm.BookmarkActionFollow), venueID).Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("venue followers for venue %d: %w", venueID, err)
	}
	return rows, nil
}

// alertPrefsForUserIDs reads the account alert row for a set of users in one
// query. The one implementation; alertPrefsForUsers de-duplicates an artist
// follower list and delegates here.
func (s *NotificationFilterService) alertPrefsForUserIDs(userIDs []uint) (map[uint]recipientAlertPrefs, error) {
	if len(userIDs) == 0 {
		return map[uint]recipientAlertPrefs{}, nil
	}
	var rows []recipientAlertPrefs
	err := s.db.Table("user_preferences").
		Select("user_id, home_metro, alert_defaults").
		Where("user_id IN ?", userIDs).
		Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("alert preferences for %d users: %w", len(userIDs), err)
	}
	byUser := make(map[uint]recipientAlertPrefs, len(rows))
	for _, r := range rows {
		byUser[r.UserID] = r
	}
	return byUser, nil
}

// showsAlreadyNotified returns, per user, the shows some other system has
// already told them about.
//
// PER SHOW rather than per batch, and that is the only shape that fits a
// coalesced alert. A user told about one date by an artist follow should still
// hear about the other eight; suppressing the whole batch would lose eight
// announcements to one overlap, and suppressing nothing would repeat the one.
//
// This relationship is deliberately ONE-DIRECTIONAL: venue alerts READ
// notifiedAboutShow but never appear in it, because their entity_id is a VENUE
// id and the predicate compares entity_id against show ids. That is also why
// NotificationEntityVenueShowAlert must never join showAlertEntityTypes.
//
// The ordering that makes the one direction the useful one is not accidental:
// the filter, artist and scene passes all run inside MatchAndNotify, BEFORE this
// batch's accrual, so by flush time their rows are already there to be seen.
func (s *NotificationFilterService) showsAlreadyNotified(
	userIDs, showIDs []uint,
) (map[uint]map[uint]struct{}, error) {
	out := map[uint]map[uint]struct{}{}
	if len(userIDs) == 0 || len(showIDs) == 0 {
		return out, nil
	}

	var rows []struct {
		UserID   uint `gorm:"column:user_id"`
		EntityID uint `gorm:"column:entity_id"`
	}
	// = ANY(array) rather than IN ?, which GORM expands into one bind parameter
	// PER ELEMENT. This query is the product of a venue's whole follower list and
	// a whole day's calendar drop, so on a popular venue that expansion walks
	// toward Postgres's 65535-parameter ceiling — and a group that fails is a
	// group that is retried at the head of the queue forever. Two array
	// parameters have no such ceiling, and it matches fillVenueAlertBills.
	err := s.db.Table("notification_log").
		Select("DISTINCT user_id, entity_id").
		Where("user_id = ANY(?) AND entity_id = ANY(?)", pq.Array(userIDs), pq.Array(showIDs)).
		Where(notifiedAboutShow("notification_log")).
		Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("cross-system dedup for %d shows: %w", len(showIDs), err)
	}

	for _, r := range rows {
		if out[r.UserID] == nil {
			out[r.UserID] = map[uint]struct{}{}
		}
		out[r.UserID][r.EntityID] = struct{}{}
	}
	return out, nil
}

// resolveVenueAlertRecipients turns the batch's followers into the alerts they
// will receive.
//
// Simpler than its artist counterpart in one important way: a user has exactly
// ONE follow of a venue, so there is no bill order, no tie-break, and no
// per-lane attribution to work out. Both channels come from the same
// subscription, and the message names the venue that subscription is for.
//
// Pure, and takes everything it needs as arguments, so the batching rules can be
// tested without a database.
func resolveVenueAlertRecipients(
	followers []venueFollowerRow,
	prefs map[uint]recipientAlertPrefs,
	shows []venueAlertShow,
	alreadyTold map[uint]map[uint]struct{},
) []venueAlertRecipient {
	// Deterministic output order from a query with no ORDER BY.
	ordered := make([]venueFollowerRow, len(followers))
	copy(ordered, followers)
	sort.SliceStable(ordered, func(i, j int) bool { return ordered[i].UserID < ordered[j].UserID })

	out := make([]venueAlertRecipient, 0, len(ordered))
	for _, f := range ordered {
		account := prefs[f.UserID]
		resolved := engagement.ResolveFollowAlerts(
			venueFollowEntityType, 0, f.Settings,
			authm.ResolveAccountAlertDefaults(account.AlertDefaults),
		)
		pref := resolved.Shows

		if !pref.Enabled || (!pref.InApp && !pref.Email) {
			continue
		}

		// NO SCOPE CHECK, deliberately. Venue show alerts have no scope axis
		// (engagement.followAlertHasScopeAxis returns false for them), so
		// pref.Scope is empty and comparing it against near-me would silence
		// every venue follower on the planet. The resolver already discards a
		// stored scope on an axis-less alert type, so there is nothing here to
		// honour even if a stale value exists in the JSONB.

		// Members this user has already heard about elsewhere drop out of THEIR
		// copy of the batch. Everyone else's copy is unaffected.
		told := alreadyTold[f.UserID]
		mine := make([]venueAlertShow, 0, len(shows))
		submittedEverything := true
		for _, sh := range shows {
			if _, done := told[sh.ID]; done {
				continue
			}
			if sh.SubmittedBy == nil || *sh.SubmittedBy != f.UserID {
				submittedEverything = false
			}
			mine = append(mine, sh)
		}

		// Self-exclusion, matching the artist and scene passes in intent but not
		// in shape. Those exclude the submitter of THE show; a batch has many,
		// with possibly many submitters. A user is excluded only when they
		// entered EVERY remaining member — that degrades to the per-show
		// behaviour for a batch of one, and it never silences someone about the
		// eight dates they did not type in.
		if len(mine) > 0 && submittedEverything {
			continue
		}

		r := venueAlertRecipient{userID: f.UserID, inApp: pref.InApp, email: pref.Email, shows: mine}
		if !r.delivers() {
			continue
		}
		out = append(out, r)
	}
	return out
}

// deliverVenueAlert writes this recipient's rows and, when the email lane is on
// and its row was newly claimed, sends the email.
//
// The structure mirrors deliverArtistAlert exactly, including the two orderings
// that took a paragraph each to justify there:
//
//   - the in-app row is written FIRST and its result does not gate the email, so
//     a user with in-app off still gets the email they asked for;
//   - the daily email budget is checked BEFORE the lane is claimed, because a
//     claim is PERMANENT and a lane claimed then refused is an email that can
//     never be sent;
//   - the lane is claimed BEFORE the send, because a crash between them loses an
//     email whereas claiming after risks sending two.
//
// A failed CLAIM is returned rather than logged-and-forgotten, so the caller can
// leave the group undispatched and retry. A failed SEND is not: the row is
// already claimed and claiming is permanent, so retrying the group could never
// send that email again and reporting it would only strand the whole group.
func (s *NotificationFilterService) deliverVenueAlert(
	r venueAlertRecipient,
	batch *venueAlertBatch,
	now time.Time,
) error {
	if r.inApp {
		if _, err := s.claimVenueAlertRow(
			r.userID, batch.key, notificationm.NotificationChannelInApp, now,
		); err != nil {
			return fmt.Errorf("in-app row: %w", err)
		}
	}

	if !r.email {
		return nil
	}
	if s.emailService == nil || !s.emailService.IsConfigured() {
		return nil
	}
	if !s.withinDailyAlertEmailBudget(r.userID) {
		log.Printf("rate limit: skipping venue-alert email for user %d", r.userID)
		return nil
	}

	claimed, err := s.claimVenueAlertRow(
		r.userID, batch.key, notificationm.NotificationChannelEmail, now)
	if err != nil {
		return fmt.Errorf("email row: %w", err)
	}
	if !claimed {
		return nil
	}
	s.sendVenueShowAlertEmail(r, batch)
	return nil
}

// claimVenueAlertRow inserts one lane's row, reporting whether THIS call created
// it. A false with no error means the lane was already claimed — by an earlier
// flush of the same batch, or by a concurrent one.
//
// Raw SQL rather than GORM's Create for one reason: alert_bucket is a DATE and
// the day is a STRING with an explicit ::date cast, so the value cannot be
// re-interpreted through the session time zone. It is also what keeps ON
// CONFLICT DO NOTHING honest about RowsAffected.
func (s *NotificationFilterService) claimVenueAlertRow(
	userID uint,
	key venueAlertGroupKey,
	channel string,
	now time.Time,
) (bool, error) {
	res := s.db.Exec(`
		INSERT INTO notification_log
		    (user_id, filter_id, entity_type, entity_id, subject_entity_id, channel, sent_at, alert_bucket)
		VALUES (?, NULL, ?, ?, NULL, ?, ?, ?::date)
		ON CONFLICT DO NOTHING
	`, userID, notificationm.NotificationEntityVenueShowAlert, key.VenueID, channel, now, key.AlertDay)
	if res.Error != nil {
		return false, res.Error
	}
	return res.RowsAffected > 0, nil
}

// markVenueAlertGroupDispatched stamps the rows this flush RESOLVED, so the
// group stops being selected until a new member joins it.
//
// Bounded by the watermark taken before the members were read. Rows that
// accrued during delivery are deliberately left undispatched, which re-selects
// the group on the next tick — the delivery claim makes that re-run silent, and
// it is the only thing that stops a slow delivery from retiring an announcement
// nobody was ever told about. See the watermark's comment in loadVenueAlertBatch.
//
// A zero watermark means "the whole group", which is what retirement wants: the
// point there is to make the group stop being selected at all.
func (s *NotificationFilterService) markVenueAlertGroupDispatched(
	key venueAlertGroupKey,
	watermark time.Time,
) {
	q := s.db.Table("venue_show_alert_batch").
		Where("venue_id = ? AND alert_day = ?::date AND dispatched_at IS NULL",
			key.VenueID, key.AlertDay)
	if !watermark.IsZero() {
		q = q.Where("created_at <= ?", watermark)
	}
	if err := q.UpdateColumn("dispatched_at", gorm.Expr("NOW()")).Error; err != nil {
		log.Printf("venue-alert flush: marking venue %d on %s dispatched: %v",
			key.VenueID, key.AlertDay, err)
	}
}

// ──────────────────────────────────────────────
// Email
// ──────────────────────────────────────────────

// sendVenueShowAlertEmail renders and sends the digest. The caller has already
// checked the daily budget and claimed the email lane.
func (s *NotificationFilterService) sendVenueShowAlertEmail(
	r venueAlertRecipient,
	batch *venueAlertBatch,
) {
	var email string
	if err := s.db.Table("users").Where("id = ?", r.userID).Pluck("email", &email).Error; err != nil || email == "" {
		log.Printf("venue-alert notify: no email for user %d: %v", r.userID, err)
		return
	}

	// The SAME scope the artist show alert email signs. Venue and artist show
	// alert emails are one stream to the recipient and, more to the point, one
	// stream to the SETTING: alert_defaults carries a single `shows` key covering
	// both, and UserService.UnsubscribeArtistShowAlertEmails already sweeps venue
	// follows as well as artist ones. A second scope would mint a second URL
	// performing an identical mutation, which is not extra precision — it is two
	// names for one action, and the day one of them drifts is the day an
	// unsubscribe stops unsubscribing.
	// TestVenueAndArtistAlertEmailsShareOneUnsubscribeScope pins this.
	unsubscribeURL := engagement.GenerateScopedUnsubscribeURL(
		engagement.DeriveBackendURL(s.frontendURL),
		r.userID,
		engagement.UnsubscribeScopeArtistShowAlerts,
		s.jwtSecret,
	)
	manageURL := fmt.Sprintf("%s/settings/notifications", s.frontendURL)

	html := buildVenueShowAlertEmailHTML(batch, r.shows, unsubscribeURL, manageURL)

	// The subject is a HEADER, and a venue name on an ingest-created row is
	// scraped third-party text. HTML escaping does nothing for headers: a CR or
	// LF in the name is how a header is split and another one injected.
	subject := fmt.Sprintf("%s at %s",
		venueAlertShowCountPhrase(len(r.shows)), sanitizeEmailHeaderValue(batch.venueName))

	if err := s.sendEmail(email, subject, html, unsubscribeURL); err != nil {
		sentry.WithScope(func(scope *sentry.Scope) {
			scope.SetTag("service", "notification_filter")
			scope.SetTag("email_type", "venue_show_alert")
			sentry.CaptureException(err)
		})
		log.Printf("venue-alert notify: failed to send alert email to user %d: %v", r.userID, err)
	}
}

// venueAlertShowCountPhrase renders the count the way both the subject line and
// the headline need it. One function so the two can never disagree about
// whether a batch of one is "1 new show".
func venueAlertShowCountPhrase(n int) string {
	if n == 1 {
		return "New show"
	}
	return fmt.Sprintf("%d new shows", n)
}

// buildVenueShowAlertEmailHTML renders the digest in the shared direction-A
// layout (PSY-1902), so it reads as the same publication as its artist sibling
// rather than as a second house style. Figma: node 1577:27.
//
// The one element that is NOT inherited is the show list. The artist alert's
// WHEN/WHERE/WITH block describes ONE event, and stacking three of them would
// read as three messages stapled together. This is a table instead: one
// hairline-separated row per show, date on the left, bill on the right.
//
// Every string reaching this is escaped by the builders. That matters more here
// than in most templates: an ingest-created venue calendar is scraped
// third-party text, and this message ships from the platform's own DKIM-aligned
// sender.
func buildVenueShowAlertEmailHTML(
	batch *venueAlertBatch,
	shows []venueAlertShow,
	unsubscribeURL, manageURL string,
) string {
	rows := make([]emailListRow, 0, len(shows))
	for _, sh := range shows {
		rows = append(rows, emailListRow{
			Label: strings.ToUpper(sh.EventDate.In(batch.loc).Format("Mon Jan 02")),
			Title: sh.Title,
			// A show with no bill keeps an empty detail line rather than being
			// dropped, so the count in the headline matches the list under it.
			Detail: sh.ArtistText,
		})
	}

	headline := fmt.Sprintf("%s at %s.", venueAlertShowCountPhrase(len(shows)), batch.venueName)

	// The "why this arrived, and why only one of it" sentence. A coalesced alert
	// invites exactly that question, and no other surface answers it at the
	// moment the reader is asking. Capability-true: it promises grouping, not
	// completeness, because a show announced after this was sent joins the inbox
	// row without triggering a second email.
	why := fmt.Sprintf(
		"You follow %s. New shows there are grouped into one alert a day, so a calendar drop reaches you as one message instead of one per show.",
		batch.venueName)

	body := emailHeadline(headline) +
		emailListRows(rows) +
		emailParagraph(why) +
		emailButton(batch.venueURL, "View venue") +
		emailFineprintWithLinks(
			[]string{fmt.Sprintf("You are getting this because you follow %s with email alerts on.", batch.venueName)},
			[]emailFineprintLink{
				// "show alerts", not "venue show alerts": the link genuinely stops
				// the artist stream too, and a label promising less than the button
				// does is the kind of surprise that ends in Report Spam.
				{Href: unsubscribeURL, Label: "Unsubscribe from show alerts"},
				{Href: manageURL, Label: "Manage alerts in Settings"},
			},
		)

	return emailShell("VENUE ALERT · NEW SHOWS", body)
}

// entityURL builds a public entity URL with the slug-then-id fallback every
// link builder in this package needs.
//
// Entity slugs are NULLABLE and GenerateSlug can return "", and "/venues/" with
// an empty slug resolves to the INDEX rather than 404ing — so a link built
// without this check silently sends the reader to a listing page and looks like
// the alert pointed at nothing.
func entityURL(frontendURL, segment string, slug *string, id uint) string {
	if slug != nil && *slug != "" {
		return fmt.Sprintf("%s/%s/%s", frontendURL, segment, *slug)
	}
	return fmt.Sprintf("%s/%s/%d", frontendURL, segment, id)
}
