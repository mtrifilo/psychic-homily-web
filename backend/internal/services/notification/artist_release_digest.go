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
	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/services/engagement"
	"psychic-homily-backend/internal/services/shared"
)

// Artist new-release weekly roundups: a coalesced alert loop whose batch is
// keyed on the RECIPIENT rather than on a catalogue entity.
//
// Accrual is per (artist, release), in services/catalog
// (enqueueArtistReleaseAlerts), and resolves nothing. Delivery is per
// (user, week): one flush pass over a closed week resolves followers,
// preferences and the message once across every artist in it.
//
// The bucket is the schedule. A week is flushed only once it is over (the week
// hold is measured from the bucket's Monday, not from its last member), because
// the claim is per (user, week) and an early flush would never email what landed
// later in the week.
//
// The flush re-resolves already-delivered weeks when a late member joins, so the
// inbox row can grow. The UNIQUE claim (uq_notification_log_artist_release_digest
// with ON CONFLICT DO NOTHING) is the exactly-once guarantee, and the loser of a
// concurrent tick claims and sends nothing. dispatched_at is bookkeeping.
//
// No cross-system dedup: nothing else notifies about a release. If something
// does, build a shared predicate like notifiedAboutShow; never add this
// discriminator to showAlertEntityTypes, because its entity_id is a USER id.
//
// No self-exclusion: releases have no submitted_by and the create endpoint is
// admin-only.
//
// No scope axis for releases: engagement.followAlertHasScopeAxis("artist",
// "releases") is false, and this file must not invent one.
//
// Accrual and the flush both fence on catalogm.ReleaseAnnounceable, which keeps
// a back-catalogue ingest from being announced as new music.

// ──────────────────────────────────────────────
// Types
// ──────────────────────────────────────────────

// releaseDigestRelease is one announceable member of a week, with the fields the
// email and the inbox row need.
type releaseDigestRelease struct {
	ReleaseID   uint
	ArtistID    uint
	ArtistName  string
	Title       string
	Slug        *string
	ReleaseType string
	ReleaseDate *string
	ReleaseYear *int
	URL         string
}

// releaseDigestMember identifies one accrued pair. The dispatch stamp is bounded
// by the exact set of these that a flush read.
type releaseDigestMember struct {
	ArtistID  uint
	ReleaseID uint
}

// releaseDigestWeek is one resolved week: its bucket and the members still worth
// announcing.
type releaseDigestWeek struct {
	week     string
	releases []releaseDigestRelease
	// resolved is the exact set of member pairs this flush read. The dispatch
	// stamp is bounded by it, so a row that became visible after the read cannot
	// be retired without ever having been considered.
	resolved []releaseDigestMember
}

// artistIDs returns the distinct credited artists in this week's announceable
// members, which is the input to the follower query.
func (w *releaseDigestWeek) artistIDs() []uint {
	seen := make(map[uint]struct{}, len(w.releases))
	out := make([]uint, 0, len(w.releases))
	for _, r := range w.releases {
		if _, dup := seen[r.ArtistID]; dup {
			continue
		}
		seen[r.ArtistID] = struct{}{}
		out = append(out, r.ArtistID)
	}
	return out
}

// artistReleaseFollowerRow is one artist follow. A user has one row per followed
// artist that released something in the week; resolveReleaseDigestRecipients
// collapses them.
type artistReleaseFollowerRow struct {
	UserID   uint             `gorm:"column:user_id"`
	ArtistID uint             `gorm:"column:artist_id"`
	Settings *json.RawMessage `gorm:"column:settings"`
}

// releaseDigestRecipient is what one user receives for one week: the channels
// their follows enable, and the releases those follows entitle them to.
type releaseDigestRecipient struct {
	userID   uint
	inApp    bool
	email    bool
	releases []releaseDigestRelease
}

// delivers reports whether this recipient has anything to receive.
func (r releaseDigestRecipient) delivers() bool {
	return len(r.releases) > 0 && (r.inApp || r.email)
}

// releaseDigestArtistGroup is one artist's records inside a roundup; the email
// renders one group per artist.
type releaseDigestArtistGroup struct {
	ArtistName string
	Releases   []releaseDigestRelease
}

// groupReleasesByArtist folds a recipient's releases into per-artist groups,
// preserving the caller's ordering for both the groups and the records inside
// them.
func groupReleasesByArtist(releases []releaseDigestRelease) []releaseDigestArtistGroup {
	groups := make([]releaseDigestArtistGroup, 0, len(releases))
	index := make(map[uint]int, len(releases))
	for _, rel := range releases {
		at, ok := index[rel.ArtistID]
		if !ok {
			index[rel.ArtistID] = len(groups)
			groups = append(groups, releaseDigestArtistGroup{ArtistName: rel.ArtistName})
			at = len(groups) - 1
		}
		groups[at].Releases = append(groups[at].Releases, rel)
	}
	return groups
}

// ──────────────────────────────────────────────
// Flush
// ──────────────────────────────────────────────

// FlushArtistReleaseDigests resolves and delivers every week that has closed,
// returning how many weeks it retired.
//
// ctx is honoured between weeks: a week can be an unbounded number of
// sequential sends and Stop() blocks on the in-flight tick. Bailing is free
// because nothing is claimed at week level; the rows stay undispatched and the
// next tick re-resolves them silently through the delivery claim.
func (s *NotificationFilterService) FlushArtistReleaseDigests(
	ctx context.Context,
	limit int,
	weekHold, maxAge time.Duration,
) int {
	if s.db == nil {
		return 0
	}

	weeks, err := s.releaseDigestWeeksReadyToFlush(limit, weekHold)
	if err != nil {
		log.Printf("release-digest flush: %v", err)
		return 0
	}

	// retired counts weeks this tick resolved, including weeks that resolved to
	// nothing to send.
	retired, processed := 0, 0
	for i, week := range weeks {
		if ctx.Err() != nil {
			log.Printf("release-digest flush: tick canceled, %d weeks left for the next tick",
				len(weeks)-i)
			break
		}
		processed++
		if s.flushReleaseDigestWeek(ctx, week, maxAge) {
			retired++
		}
	}
	if processed > retired {
		log.Printf("release-digest flush: %d of %d weeks left undispatched for the next tick",
			processed-retired, processed)
	}
	return retired
}

// releaseDigestWeeksReadyToFlush finds the week buckets that have closed: a week
// is ready when its Monday is at least weekHold old. The bound is on the
// BUCKET, not on MAX(created_at), so a trickle cannot starve a week and no max
// hold is needed. Oldest first, so a backlog drains in arrival order.
func (s *NotificationFilterService) releaseDigestWeeksReadyToFlush(
	limit int,
	weekHold time.Duration,
) ([]string, error) {
	cutoff := time.Now().UTC().Add(-weekHold).Format(alertWeekLayout)
	var weeks []string
	// alert_week comes back as TEXT: it is only ever used as a day, and a DATE
	// scanned into time.Time carries a driver-dependent zone.
	err := s.db.Raw(`
		SELECT to_char(alert_week, 'YYYY-MM-DD') AS alert_week
		FROM artist_release_alert_batch
		WHERE dispatched_at IS NULL
		  AND alert_week <= ?::date
		GROUP BY alert_week
		ORDER BY alert_week
		LIMIT ?
	`, cutoff, limit).Scan(&weeks).Error
	if err != nil {
		return nil, fmt.Errorf("release digest weeks ready to flush: %w", err)
	}
	return weeks, nil
}

// flushReleaseDigestWeek resolves and delivers one week, then marks the rows it
// RESOLVED dispatched. Reports whether the week was retired.
//
// Rows are marked after delivery, never before: a crash in between costs a
// silent re-run, whereas marking first would lose the roundup.
func (s *NotificationFilterService) flushReleaseDigestWeek(
	ctx context.Context,
	week string,
	maxAge time.Duration,
) bool {
	batch, err := s.loadReleaseDigestWeek(week)
	if err != nil {
		log.Printf("release-digest flush: %v", err)
		s.noteReleaseDigestFailure(week, maxAge, err)
		return false
	}

	if len(batch.releases) == 0 {
		// Nothing read is announceable. Retire exactly what was read; a closed
		// week only gets further from announceable.
		s.markReleaseDigestWeekDispatched(week, batch.resolved)
		s.clearReleaseDigestFailure(week)
		return true
	}

	if err := s.deliverReleaseDigestWeek(ctx, batch); err != nil {
		// A cancelled tick is a deploy, not a failure, and must not advance the
		// poison-pill bound.
		if ctx.Err() != nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			log.Printf("release-digest flush: canceled mid-week for %s, left for the next tick", week)
			return false
		}
		// Leave the rows undispatched so the next tick retries; every step is
		// idempotent.
		log.Printf("release-digest flush: %v", err)
		s.noteReleaseDigestFailure(week, maxAge, err)
		return false
	}

	s.markReleaseDigestWeekDispatched(week, batch.resolved)
	s.clearReleaseDigestFailure(week)
	return true
}

// noteReleaseDigestFailure records that a week's flush failed, and abandons it
// once it has been failing for longer than maxAge. Without a bound, a week that
// fails deterministically sits at the head of the oldest-first selection and
// re-occupies a slot every tick.
//
// It measures FAILURE DURATION, not bucket age: every eligible week is already
// at least seven days old, so a bucket-age bound would abandon every roundup on
// its first transient error. State is in memory, so a restart errs toward
// retrying; entries are removed on success or retirement.
func (s *NotificationFilterService) noteReleaseDigestFailure(
	week string,
	maxAge time.Duration,
	cause error,
) {
	if maxAge <= 0 {
		return
	}

	s.releaseDigestFailuresMu.Lock()
	if s.releaseDigestFailures == nil {
		s.releaseDigestFailures = make(map[string]time.Time)
	}
	first, seen := s.releaseDigestFailures[week]
	if !seen {
		first = time.Now()
		s.releaseDigestFailures[week] = first
	}
	s.releaseDigestFailuresMu.Unlock()

	failingFor := time.Since(first)
	if failingFor <= maxAge {
		return
	}

	// Loud, and per-week: this is a silent user-visible loss otherwise.
	log.Printf("release-digest flush: ABANDONING week %s after %s of continuous failures; "+
		"its followers will not be told about that week's releases. last error: %v",
		week, failingFor.Round(time.Second), cause)

	// Stamps the whole week (nil member set) so it stops being selected at all.
	s.markReleaseDigestWeekDispatched(week, nil)
	s.clearReleaseDigestFailure(week)
}

// clearReleaseDigestFailure forgets a week's failure history, so a week that
// recovers gets the full bound again next time it breaks.
func (s *NotificationFilterService) clearReleaseDigestFailure(week string) {
	s.releaseDigestFailuresMu.Lock()
	delete(s.releaseDigestFailures, week)
	s.releaseDigestFailuresMu.Unlock()
}

// loadReleaseDigestWeek reads a week's members and the ones still worth
// announcing.
func (s *NotificationFilterService) loadReleaseDigestWeek(week string) (*releaseDigestWeek, error) {
	// Every member row in the week, read without a time bound: the dispatch
	// stamp is bounded by the resolved member set, never by a clock. Accrual
	// stamps created_at before its INSERT commits, so a row can become visible
	// after a watermark while carrying an earlier created_at, and a clock bound
	// would retire it unread.
	var rows []struct {
		ArtistID    uint    `gorm:"column:artist_id"`
		ArtistName  string  `gorm:"column:artist_name"`
		ReleaseID   uint    `gorm:"column:release_id"`
		Title       string  `gorm:"column:title"`
		Slug        *string `gorm:"column:slug"`
		ReleaseType string  `gorm:"column:release_type"`
		ReleaseDate *string `gorm:"column:release_date"`
		ReleaseYear *int    `gorm:"column:release_year"`
	}
	err := s.db.Raw(`
		SELECT b.artist_id, a.name AS artist_name, b.release_id,
		       r.title, r.slug, r.release_type,
		       to_char(r.release_date, 'YYYY-MM-DD') AS release_date,
		       r.release_year
		FROM artist_release_alert_batch b
		JOIN releases r ON r.id = b.release_id
		JOIN artists a ON a.id = b.artist_id
		-- Re-validate the credit against the CURRENT artist_releases rows. Only a
		-- DELETED release or artist leaves the batch (the foreign keys cascade), so
		-- without this join an artist merge or a credit correction could still put
		-- a record under a band that is no longer on it.
		JOIN artist_releases ar ON ar.artist_id = b.artist_id AND ar.release_id = b.release_id
		WHERE b.alert_week = ?::date
		GROUP BY b.artist_id, a.name, b.release_id, r.title, r.slug,
		         r.release_type, r.release_date, r.release_year
		ORDER BY a.name ASC, b.artist_id ASC, r.release_date DESC NULLS LAST, b.release_id ASC
	`, week).Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("release digest members for week %s: %w", week, err)
	}

	// Re-check announceability with the predicate accrual used: release_date and
	// release_year are editable during the week between accrual and flush.
	now := time.Now()
	members := make([]releaseDigestRelease, 0, len(rows))
	// resolved is every member row read, announceable or not; leaving rejected
	// rows undispatched would re-select the week forever.
	resolved := make([]releaseDigestMember, 0, len(rows))
	for _, row := range rows {
		resolved = append(resolved, releaseDigestMember{ArtistID: row.ArtistID, ReleaseID: row.ReleaseID})

		release := &catalogm.Release{
			ID:          row.ReleaseID,
			Title:       row.Title,
			ReleaseDate: row.ReleaseDate,
			ReleaseYear: row.ReleaseYear,
		}
		if ok, _ := catalogm.ReleaseAnnounceable(release, now); !ok {
			continue
		}

		members = append(members, releaseDigestRelease{
			ReleaseID:   row.ReleaseID,
			ArtistID:    row.ArtistID,
			ArtistName:  row.ArtistName,
			Title:       row.Title,
			Slug:        row.Slug,
			ReleaseType: row.ReleaseType,
			ReleaseDate: row.ReleaseDate,
			ReleaseYear: row.ReleaseYear,
			URL:         entityURL(s.frontendURL, "releases", row.Slug, row.ReleaseID),
		})
	}

	// No post-load sort: the query's ORDER BY (artist name, then newest record)
	// is the one definition of reading order.

	return &releaseDigestWeek{week: week, releases: members, resolved: resolved}, nil
}

// ──────────────────────────────────────────────
// Delivery
// ──────────────────────────────────────────────

// deliverReleaseDigestWeek resolves this week's recipients and delivers to each.
// A per-recipient failure is returned so the caller leaves the week
// undispatched; the retry is safe because every claim is ON CONFLICT DO NOTHING.
func (s *NotificationFilterService) deliverReleaseDigestWeek(
	ctx context.Context,
	batch *releaseDigestWeek,
) error {
	followers, err := s.artistReleaseFollowers(batch.artistIDs())
	if err != nil {
		return err
	}
	if len(followers) == 0 {
		return nil
	}

	userIDs := make([]uint, 0, len(followers))
	seenUser := make(map[uint]struct{}, len(followers))
	for _, f := range followers {
		if _, dup := seenUser[f.UserID]; dup {
			continue
		}
		seenUser[f.UserID] = struct{}{}
		userIDs = append(userIDs, f.UserID)
	}

	prefs, err := s.alertPrefsForUserIDs(userIDs)
	if err != nil {
		return err
	}

	recipients := resolveReleaseDigestRecipients(followers, prefs, batch.releases)

	now := time.Now().UTC()
	var failed int
	for i, r := range recipients {
		// ctx is honoured between recipients too, since one week can be an
		// unbounded number of sends. The error leaves the week undispatched and
		// the claim keeps already-delivered recipients silent on the next tick.
		if ctx.Err() != nil {
			return fmt.Errorf("release digest delivery for week %s canceled after %d of %d recipients: %w",
				batch.week, i, len(recipients), ctx.Err())
		}
		if err := s.deliverReleaseDigest(r, batch, now); err != nil {
			log.Printf("release-digest notify: user %d, week %s: %v", r.userID, batch.week, err)
			failed++
		}
	}
	if failed > 0 {
		return fmt.Errorf("release digest delivery for week %s: %d of %d recipients failed",
			batch.week, failed, len(recipients))
	}
	return nil
}

// artistReleaseFollowers returns every follow of every artist in this week, in
// one query.
func (s *NotificationFilterService) artistReleaseFollowers(artistIDs []uint) ([]artistReleaseFollowerRow, error) {
	if len(artistIDs) == 0 {
		return nil, nil
	}
	var rows []artistReleaseFollowerRow
	// = ANY(array) rather than IN, which GORM expands into one bind parameter per
	// element: a week's artist list is unbounded, and Postgres caps a statement
	// at 65535 parameters.
	err := s.db.Raw(`
		SELECT b.user_id, b.entity_id AS artist_id, b.settings
		FROM user_bookmarks b
		WHERE b.entity_type = ?
		  AND b.action = ?
		  AND b.entity_id = ANY(?)
	`, artistReleaseFollowEntityType, string(engagementm.BookmarkActionFollow),
		pq.Array(artistIDs)).Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("artist release followers for %d artists: %w", len(artistIDs), err)
	}
	return rows, nil
}

// resolveReleaseDigestRecipients turns the week's followers into the roundups
// they will receive: many follower rows per user, one recipient per user.
//
// Channels are OR-ed across a user's follows, unlike the artist show alert's
// per-lane attribution, because this message names every artist it covers: a
// user who enabled email for one of six bands gets an email that truthfully says
// why. Pure, so the batching rules are testable without a database.
func resolveReleaseDigestRecipients(
	followers []artistReleaseFollowerRow,
	prefs map[uint]recipientAlertPrefs,
	releases []releaseDigestRelease,
) []releaseDigestRecipient {
	// Deterministic output order from a query with no ORDER BY.
	ordered := make([]artistReleaseFollowerRow, len(followers))
	copy(ordered, followers)
	sort.SliceStable(ordered, func(i, j int) bool {
		if ordered[i].UserID != ordered[j].UserID {
			return ordered[i].UserID < ordered[j].UserID
		}
		return ordered[i].ArtistID < ordered[j].ArtistID
	})

	type userState struct {
		inApp   bool
		email   bool
		artists map[uint]struct{}
	}
	byUser := make(map[uint]*userState, len(ordered))
	order := make([]uint, 0, len(ordered))

	for _, f := range ordered {
		account := prefs[f.UserID]
		resolved := engagement.ResolveFollowAlerts(
			artistReleaseFollowEntityType, f.ArtistID, f.Settings,
			authm.ResolveAccountAlertDefaults(account.AlertDefaults),
		)
		// Releases is a pointer, nil for entity types that never emit releases.
		// Artists always do; the guard keeps a nil deref from killing the tick.
		if resolved.Releases == nil {
			continue
		}
		pref := *resolved.Releases

		if !pref.Enabled || (!pref.InApp && !pref.Email) {
			continue
		}

		// No scope check: release alerts have no scope axis, so pref.Scope is
		// empty and comparing it against near-me would silence every follower.

		state := byUser[f.UserID]
		if state == nil {
			state = &userState{artists: map[uint]struct{}{}}
			byUser[f.UserID] = state
			order = append(order, f.UserID)
		}
		state.inApp = state.inApp || pref.InApp
		state.email = state.email || pref.Email
		state.artists[f.ArtistID] = struct{}{}
	}

	out := make([]releaseDigestRecipient, 0, len(order))
	for _, userID := range order {
		state := byUser[userID]
		// The week's releases, in the week's order, narrowed to the artists this
		// user subscribed to with the release alert on.
		mine := make([]releaseDigestRelease, 0, len(releases))
		for _, rel := range releases {
			if _, ok := state.artists[rel.ArtistID]; !ok {
				continue
			}
			mine = append(mine, rel)
		}
		r := releaseDigestRecipient{
			userID:   userID,
			inApp:    state.inApp,
			email:    state.email,
			releases: mine,
		}
		if !r.delivers() {
			continue
		}
		out = append(out, r)
	}
	return out
}

// releaseDigestEmailLaneEnabled is the build-time gate on the digest's email
// lane. While it is false a flush never claims an email-lane row and never
// sends, whatever the reader's email preference says, so the in-app row is the
// whole delivery and the email code below is unreachable. Enabling it is a
// deliberate code change; TestReleaseDigestEmailLaneIsGatedOff pins it off.
const releaseDigestEmailLaneEnabled = false

// deliverReleaseDigest writes this recipient's rows and, when the email lane is
// on and its row was newly claimed, sends the email. Ordering mirrors
// deliverVenueAlert:
//
//   - the in-app row first, and its result does not gate the email;
//   - the daily email budget before the claim, because a claim is permanent and
//     a lane claimed then refused can never be sent;
//   - the claim before the send, because a crash between them loses an email
//     whereas claiming after risks sending two.
//
// A failed claim is returned so the week is retried. A failed send is not: the
// row is already claimed, so a retry could never send it.
func (s *NotificationFilterService) deliverReleaseDigest(
	r releaseDigestRecipient,
	batch *releaseDigestWeek,
	now time.Time,
) error {
	if r.inApp {
		if _, err := s.claimReleaseDigestRow(
			r.userID, batch.week, notificationm.NotificationChannelInApp, now,
		); err != nil {
			return fmt.Errorf("in-app row: %w", err)
		}
	}

	if !r.email || !releaseDigestEmailLaneEnabled {
		return nil
	}
	if s.emailService == nil || !s.emailService.IsConfigured() {
		return nil
	}
	// A failed budget check skips the email like a refusal does; the check has
	// already logged it.
	if within, err := s.withinDailyAlertEmailBudget(r.userID); err != nil || !within {
		if err == nil {
			log.Printf("rate limit: skipping release-digest email for user %d", r.userID)
		}
		return nil
	}

	claimed, err := s.claimReleaseDigestRow(
		r.userID, batch.week, notificationm.NotificationChannelEmail, now)
	if err != nil {
		return fmt.Errorf("email row: %w", err)
	}
	if !claimed {
		return nil
	}
	s.sendArtistReleaseDigestEmail(r, batch)
	return nil
}

// claimReleaseDigestRow inserts one lane's row, reporting whether THIS call
// created it. False with no error means the lane was already claimed, by an
// earlier or concurrent flush of the same week.
//
// entity_id is the reader's USER id (see NotificationEntityArtistReleaseDigest).
// Raw SQL so the week goes in as a string with an explicit ::date cast and
// RowsAffected reflects ON CONFLICT DO NOTHING.
func (s *NotificationFilterService) claimReleaseDigestRow(
	userID uint,
	week string,
	channel string,
	now time.Time,
) (bool, error) {
	res := s.db.Exec(`
		INSERT INTO notification_log
		    (user_id, filter_id, entity_type, entity_id, subject_entity_id, channel, sent_at, alert_bucket)
		VALUES (?, NULL, ?, ?, NULL, ?, ?, ?::date)
		ON CONFLICT DO NOTHING
	`, userID, notificationm.NotificationEntityArtistReleaseDigest, userID, channel, now, week)
	if res.Error != nil {
		return false, res.Error
	}
	return res.RowsAffected > 0, nil
}

// markReleaseDigestWeekDispatched stamps the rows this flush RESOLVED, so the
// week stops being selected until a new member joins it.
//
// Bounded by the exact (artist, release) pairs that were read, never by a
// clock: a row that became visible after the read keeps dispatched_at NULL and
// re-selects the week, where the claim makes the re-run silent. A nil pair set
// means the whole week, which is what retirement wants.
func (s *NotificationFilterService) markReleaseDigestWeekDispatched(
	week string,
	resolved []releaseDigestMember,
) {
	q := s.db.Table("artist_release_alert_batch").
		Where("alert_week = ?::date AND dispatched_at IS NULL", week)
	if resolved != nil {
		if len(resolved) == 0 {
			// Nothing was read; stamping the week would retire rows that arrived
			// since.
			return
		}
		// Matched as pairs, not two independent id lists, which would stamp the
		// cross product and retire rows this flush never read.
		artistIDs := make([]uint, 0, len(resolved))
		releaseIDs := make([]uint, 0, len(resolved))
		for _, m := range resolved {
			artistIDs = append(artistIDs, m.ArtistID)
			releaseIDs = append(releaseIDs, m.ReleaseID)
		}
		q = q.Where(
			"(artist_id, release_id) IN (SELECT * FROM UNNEST(?::bigint[], ?::bigint[]))",
			pq.Array(artistIDs), pq.Array(releaseIDs))
	}
	if err := q.UpdateColumn("dispatched_at", gorm.Expr("NOW()")).Error; err != nil {
		log.Printf("release-digest flush: marking week %s dispatched: %v", week, err)
	}
}

// ──────────────────────────────────────────────
// Email
// ──────────────────────────────────────────────

// sendArtistReleaseDigestEmail renders and sends the roundup. The caller has
// already checked the daily budget and claimed the email lane.
//
// No edited-after-accrual guard, unlike the venue digest: releases have no
// self-serve create or edit path (POST /releases is admin-only), and
// releases.updated_at moves on routine enrichment writes, so such a guard would
// drop most of a week's roundup.
func (s *NotificationFilterService) sendArtistReleaseDigestEmail(
	r releaseDigestRecipient,
	batch *releaseDigestWeek,
) {
	var email string
	if err := s.db.Table("users").Where("id = ?", r.userID).Pluck("email", &email).Error; err != nil || email == "" {
		log.Printf("release-digest notify: no email for user %d: %v", r.userID, err)
		return
	}

	// Its own scope: `releases` is a separate alert_defaults key with its own
	// setter, so reusing UnsubscribeScopeArtistShowAlerts would stop the wrong
	// stream. TestReleaseDigestUsesItsOwnUnsubscribeScope pins this.
	unsubscribeURL := engagement.GenerateScopedUnsubscribeURL(
		s.backendURL,
		r.userID,
		engagement.UnsubscribeScopeArtistReleaseAlerts,
		s.jwtSecret,
	)
	manageURL := fmt.Sprintf("%s/settings/notifications", s.frontendURL)

	html := buildArtistReleaseDigestEmailHTML(r.releases, s.frontendURL, unsubscribeURL, manageURL)

	// The subject is a header carrying scraped artist text, so it must be free
	// of CR/LF (header injection) and bounded in length.
	subject := releaseDigestSubject(r.releases)

	if err := s.sendEmail(email, subject, html, unsubscribeURL); err != nil {
		sentry.WithScope(func(scope *sentry.Scope) {
			scope.SetTag("service", "notification_filter")
			scope.SetTag("email_type", "artist_release_digest")
			sentry.CaptureException(err)
		})
		log.Printf("release-digest notify: failed to send roundup email to user %d: %v", r.userID, err)
	}
}

// releaseDigestCountPhrase renders the count for both the subject line and the
// headline, so the two cannot disagree.
func releaseDigestCountPhrase(n int) string {
	if n == 1 {
		return "New release"
	}
	return fmt.Sprintf("%d new releases", n)
}

// releaseDigestSubject builds the Subject header. A roundup of one names the
// artist; a roundup of several names none, since picking one would misdescribe
// the message.
func releaseDigestSubject(releases []releaseDigestRelease) string {
	if len(releases) == 1 {
		return fmt.Sprintf("New release from %s",
			entityNameForSubject(releases[0].ArtistName))
	}
	return fmt.Sprintf("%s from artists you follow", releaseDigestCountPhrase(len(releases)))
}

// releaseDigestArtistPhrase names the artists in a roundup for the "why you got
// this" sentence, capped so the sentence stays a sentence.
func releaseDigestArtistPhrase(groups []releaseDigestArtistGroup) string {
	names := make([]string, 0, len(groups))
	for _, g := range groups {
		names = append(names, g.ArtistName)
	}
	switch {
	case len(names) == 1:
		return names[0]
	case len(names) == 2:
		return names[0] + " and " + names[1]
	case len(names) <= releaseDigestWhyArtistLimit:
		return strings.Join(names[:len(names)-1], ", ") + " and " + names[len(names)-1]
	default:
		return fmt.Sprintf("%s and %d other artists",
			strings.Join(names[:releaseDigestWhyArtistLimit], ", "),
			len(names)-releaseDigestWhyArtistLimit)
	}
}

// releaseDigestWhyArtistLimit caps how many artists the "why you got this"
// sentence names before it switches to a count.
const releaseDigestWhyArtistLimit = 3

// buildArtistReleaseDigestEmailHTML renders the roundup in the shared email
// layout, as one emailSectionLabel + emailListRows pair per artist.
//
// Every string reaching this is escaped by the builders: an ingest-created
// release is scraped third-party text, and the message ships from the
// platform's own DKIM-aligned sender.
func buildArtistReleaseDigestEmailHTML(
	releases []releaseDigestRelease,
	frontendURL string,
	unsubscribeURL, manageURL string,
) string {
	groups := groupReleasesByArtist(releases)

	var body strings.Builder
	body.WriteString(emailHeadline(fmt.Sprintf("%s from artists you follow.",
		releaseDigestCountPhrase(len(releases)))))

	for _, group := range groups {
		body.WriteString(emailSectionLabel(group.ArtistName))
		rows := make([]emailListRow, 0, len(group.Releases))
		for _, rel := range group.Releases {
			rows = append(rows, emailListRow{
				Label:  releaseTypeLabel(rel.ReleaseType),
				Title:  rel.Title,
				Detail: releaseDateLine(rel),
				// Each record links to its own page.
				Href: rel.URL,
			})
		}
		body.WriteString(emailListRows(rows))
	}

	// Promises grouping, not completeness: a record accrued after this was sent
	// joins the inbox row without a second email.
	body.WriteString(emailParagraph(fmt.Sprintf(
		"You follow %s. New releases from artists you follow are grouped into one roundup a week, so a run of records reaches you as one message instead of one per release.",
		releaseDigestArtistPhrase(groups))))

	// The CTA is the reader's follows (/library), not a catalogue page: the
	// message is about their subscriptions.
	body.WriteString(emailButton(fmt.Sprintf("%s/library", frontendURL), "View your follows"))

	body.WriteString(emailFineprintWithLinks(
		[]string{"You are getting this because you follow these artists with email alerts on."},
		[]emailFineprintLink{
			// "release alerts", precisely: this link stops only this stream.
			{Href: unsubscribeURL, Label: "Unsubscribe from release alerts"},
			{Href: manageURL, Label: "Manage alerts in Settings"},
		},
	))

	return emailShell("RELEASE ALERT · NEW MUSIC", body.String())
}

// releaseTypeLabel renders a release type for the list's mono left column:
// upper-cased, since stored values are lowercase enum tokens. An unknown value
// passes through upper-cased rather than blanked.
func releaseTypeLabel(releaseType string) string {
	if strings.TrimSpace(releaseType) == "" {
		return "RELEASE"
	}
	return strings.ToUpper(releaseType)
}

// releaseDateLine renders the secondary line under a release title: a full
// date, a year only, or empty (which renders no line).
func releaseDateLine(rel releaseDigestRelease) string {
	if rel.ReleaseDate != nil && *rel.ReleaseDate != "" {
		if day, err := time.Parse(alertWeekLayout, *rel.ReleaseDate); err == nil {
			return "Released " + day.Format("Jan 2, 2006")
		}
	}
	if rel.ReleaseYear != nil {
		return fmt.Sprintf("Released %d", *rel.ReleaseYear)
	}
	return ""
}

// artistReleaseFollowEntityType is the bookmark entity type these roundups
// subscribe on, read from the model so the follower WHERE clause and
// engagement's resolver compare the same value.
var artistReleaseFollowEntityType = string(engagementm.BookmarkEntityArtist)

// alertWeekLayout is how a week bucket is spelled where it crosses the Go/SQL
// boundary: passed as a string with an explicit ::date cast, never as a
// time.Time, which Postgres would cast to date in the SESSION time zone. Must
// match alertWeekLayout in services/catalog; it is duplicated so this package
// does not import the catalogue service package for a format string.
const alertWeekLayout = "2006-01-02"

// ArtistReleaseAlertsEnabled reports whether the release-roundup loop is on.
// Accrual in services/catalog reads the same flag name from models/notification.
func ArtistReleaseAlertsEnabled() bool {
	return !shared.EnvServiceDisabled(notificationm.ArtistReleaseAlertsDisableFlag)
}

// ──────────────────────────────────────────────
// Inbox enrichment
// ──────────────────────────────────────────────

// releaseDigestSummaryLimit caps how many artist names an inbox row carries in
// AlertReleaseArtists; AlertReleaseArtistCount carries the full count.
const releaseDigestSummaryLimit = 3

// enrichArtistReleaseDigestNotifications fills AlertReleaseCount,
// AlertReleaseArtistCount, AlertReleaseArtists and (for a one-release week)
// AlertReleaseURL on artist_release_digest rows.
//
// Members are read from artist_release_alert_batch at read time, so a release
// accrued after delivery grows the row without a second notification. Because
// the read is live, the query re-applies the delivery rules:
//
//   - announceability, through catalogm.ReleaseAnnounceableBounds, since only a
//     deleted release leaves the batch;
//   - the credit, by re-joining artist_releases, as the flush does;
//   - the reader's own current follow of each artist.
//
// A week whose members are all gone leaves the counts at zero rather than
// hiding the row. The row can shrink if the reader unfollows an artist
// (user_bookmarks keeps no unfollow history), and it deliberately ignores the
// current release-alert preference, which governs future alerts only.
func (s *NotificationFilterService) enrichArtistReleaseDigestNotifications(
	userID uint,
	entries []contracts.NotificationLogEntry,
) {
	if len(entries) == 0 {
		return
	}

	buckets := make([]string, 0, len(entries))
	seenBucket := make(map[string]struct{}, len(entries))
	for _, e := range entries {
		if e.EntityType != notificationm.NotificationEntityArtistReleaseDigest || e.AlertBucket == "" {
			continue
		}
		if _, dup := seenBucket[e.AlertBucket]; dup {
			continue
		}
		seenBucket[e.AlertBucket] = struct{}{}
		buckets = append(buckets, e.AlertBucket)
	}
	if len(buckets) == 0 {
		return
	}

	var members []struct {
		AlertWeek  string  `gorm:"column:alert_week"`
		ReleaseID  uint    `gorm:"column:release_id"`
		Slug       *string `gorm:"column:slug"`
		ArtistID   uint    `gorm:"column:artist_id"`
		ArtistName string  `gorm:"column:artist_name"`
	}
	// Whole UTC days from catalogm.ReleaseAnnounceableBounds, the bounds
	// ReleaseAnnounceable applies, so this SQL restatement agrees with the Go
	// predicate on boundary days.
	oldest, latest, minYear := catalogm.ReleaseAnnounceableBounds(time.Now())
	if err := s.db.Raw(`
		SELECT to_char(b.alert_week, 'YYYY-MM-DD') AS alert_week,
		       b.release_id, r.slug, b.artist_id, a.name AS artist_name
		FROM artist_release_alert_batch b
		JOIN releases r ON r.id = b.release_id
		JOIN artists a ON a.id = b.artist_id
		-- The record must still be CREDITED to this artist: artist_releases is
		-- rewritten by a credit correction and never touches the batch row.
		JOIN artist_releases ar ON ar.artist_id = b.artist_id AND ar.release_id = b.release_id
		-- The reader's OWN follow of that artist, so the row describes this
		-- reader's roundup rather than the week's whole membership.
		JOIN user_bookmarks ub
		  ON ub.user_id = ?
		 AND ub.entity_type = ?
		 AND ub.action = ?
		 AND ub.entity_id = b.artist_id
		WHERE b.alert_week = ANY(?::date[])
		  AND (
		        (r.release_date IS NOT NULL AND r.release_date >= ?::date AND r.release_date <= ?::date)
		     OR (r.release_date IS NULL AND r.release_year IS NOT NULL AND r.release_year >= ?)
		      )
		-- artist_releases is keyed by role, so one credit can join twice.
		GROUP BY b.alert_week, b.release_id, r.slug, b.artist_id, a.name
		ORDER BY b.alert_week, a.name ASC, b.artist_id ASC, b.release_id ASC
	`,
		userID,
		artistReleaseFollowEntityType,
		string(engagementm.BookmarkActionFollow),
		pq.Array(buckets),
		oldest, latest, minYear,
	).Scan(&members).Error; err != nil {
		log.Printf("warning: failed to load release digest batches for inbox enrichment: %v", err)
		return
	}

	// A release credited to two followed artists counts once as a release and
	// once per artist, so the row's artist list names both.
	type weekSummary struct {
		releases map[uint]struct{}
		artists  map[uint]struct{}
		names    []string
		soleURL  string
	}
	byWeek := make(map[string]*weekSummary, len(buckets))
	for _, m := range members {
		w := byWeek[m.AlertWeek]
		if w == nil {
			w = &weekSummary{releases: map[uint]struct{}{}, artists: map[uint]struct{}{}}
			byWeek[m.AlertWeek] = w
		}
		if _, seen := w.releases[m.ReleaseID]; !seen {
			w.releases[m.ReleaseID] = struct{}{}
			w.soleURL = entityURL(s.frontendURL, "releases", m.Slug, m.ReleaseID)
		}
		if _, seen := w.artists[m.ArtistID]; !seen {
			w.artists[m.ArtistID] = struct{}{}
			w.names = append(w.names, m.ArtistName)
		}
	}

	for i := range entries {
		e := &entries[i]
		if e.EntityType != notificationm.NotificationEntityArtistReleaseDigest || e.AlertBucket == "" {
			continue
		}
		w := byWeek[e.AlertBucket]
		if w == nil {
			continue
		}
		e.AlertReleaseCount = len(w.releases)
		e.AlertReleaseArtistCount = len(w.artists)
		e.AlertReleaseArtists = releaseDigestArtistList(w.names)
		// Only a one-release week has an honest single destination.
		if len(w.releases) == 1 {
			e.AlertReleaseURL = w.soleURL
		}
	}
}

// releaseDigestArtistList joins the inbox row's artist names, naming at most
// releaseDigestSummaryLimit and counting the rest.
func releaseDigestArtistList(names []string) string {
	if len(names) <= releaseDigestSummaryLimit {
		return strings.Join(names, ", ")
	}
	return fmt.Sprintf("%s and %d more",
		strings.Join(names[:releaseDigestSummaryLimit], ", "),
		len(names)-releaseDigestSummaryLimit)
}
