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

// Artist new-release weekly roundups (PSY-1897).
//
// This is the fourth follow-driven alert loop, the second COALESCED one, and the
// first whose batch is keyed on the RECIPIENT rather than on a catalogue entity.
//
// # The grain, and why it is not per artist
//
// The owner decision (PSY-1892, refined 2026-08-26) is releases WEEKLY, one
// roundup per USER per week covering every artist they follow. Somebody
// following forty bands in a week when six of them put something out is owed one
// message, not six — and the per-artist shape would have produced six even
// though each was individually correct.
//
// That splits the work along a different seam from its siblings:
//
//   - ACCRUAL is per (artist, release), because that is the shape of the
//     OBSERVATION. It lives in services/catalog (enqueueArtistReleaseAlerts),
//     inside the single release create funnel, and resolves nothing.
//   - DELIVERY is per (user, week). The USER dimension enters only here, at
//     flush, where one pass over a week resolves followers, preferences and the
//     message once across every artist in it.
//
// Resolving at accrual time would mean deciding "who should hear about this" for
// every release in a label ingest and then trying to fold two hundred answers
// back into one message, which is the same work with a race in it.
//
// # Why the schedule is a WEEK and not a quiet window
//
// The venue sibling flushes when a batch goes quiet, because its bucket is a day
// and the reader wants the drop while it is news. Here the bucket IS the
// schedule: a week's roundup is only a week's roundup once the week is over.
// Flushing on quiet would mail Monday's record on Monday and then, because the
// claim is per (user, week), silently NEVER mail Wednesday's — it would reach the
// inbox row and no further. releaseDigestWeekHold is therefore measured from the
// bucket's own start, not from its last member.
//
// # Exactly once, across re-runs that are DESIGNED to happen
//
// The flush deliberately re-resolves weeks it has already delivered: a release
// accrued after the roundup went out joins the same week, and the whole week is
// re-read so the inbox row can grow. Every such re-run reaches the delivery
// claim.
//
// What makes that silent is uq_notification_log_artist_release_digest, a partial
// UNIQUE on (user_id, entity_id, alert_bucket, channel) claimed with ON CONFLICT
// DO NOTHING. A second flush finds RowsAffected == 0 and sends nothing. It is
// also what makes two concurrent ticks safe: the loser of the race sends no
// email, because the email is sent only when the claim succeeded.
//
// dispatched_at is bookkeeping, not the guard.
//
// # No cross-system dedup, and that is not an omission
//
// The show loops consult showsAlreadyNotified because three systems can tell one
// user about one show. NOTHING ELSE in the product notifies anyone about a
// release, so there is no second teller to defer to and no predicate to consult.
// If one ever ships, it and this loop need a shared "already told about this
// release" predicate built the way notifiedAboutShow is — do not add this
// discriminator to showAlertEntityTypes to get there. Its entity_id is a USER
// id.
//
// # No self-exclusion
//
// The show passes drop the submitter of a show from its own alert. `releases`
// has no submitted_by column and no self-serve create path at all — the create
// endpoint is admin-only — so there is no submitter to exclude.
//
// # No scope axis
//
// engagement.followAlertHasScopeAxis("artist", "releases") is false and this
// file must not invent one. A record has no location; "near me" has nothing to
// mean for it. A scope stored on a release subscription by some future client is
// stale data the resolver already ignores.
//
// # No backfill
//
// Starting this on a deploy cannot alert anyone about the existing catalogue:
// artist_release_alert_batch ships EMPTY and only accrual writes to it. On top of
// that structural guarantee, accrual and this flush both fence on
// catalogm.ReleaseAnnounceable, which is what keeps a label back-catalogue ingest
// from being announced as new music.

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

// artistReleaseFollowerRow is one artist follow. Unlike the venue pass there are
// MANY rows per user in a week — one per followed artist that released something
// — which is exactly the fan-in this alert exists to collapse.
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

// releaseDigestArtistGroup is one artist's records inside a roundup. The email
// renders one of these per artist, because a flat list of nine records from six
// bands reads as noise while the same nine grouped read as six pieces of news.
type releaseDigestArtistGroup struct {
	ArtistName string
	Releases   []releaseDigestRelease
}

// groupReleasesByArtist folds a recipient's releases into per-artist groups,
// preserving the caller's ordering for both the groups and the records inside
// them.
//
// A release credited to TWO artists the reader follows appears ONCE, under the
// first of them in the caller's order. Listing it twice would make the headline
// count disagree with the list beneath it, and "you follow both of these" is not
// worth a reader wondering whether they are being told about two records.
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
// returning how many weeks it dispatched.
//
// Exported so the poller (and tests) can drive one pass without owning any of
// the resolution logic.
//
// ctx is honoured BETWEEN weeks. A week is one SYNCHRONOUS provider request per
// email recipient, which the per-user daily cap does not bound — a popular week
// is an unbounded number of sequential sends, and Stop() blocks on the in-flight
// tick. Bailing is FREE here because nothing is claimed at week level: the rows
// stay undispatched and the next tick re-resolves them, silently, through the
// delivery claim.
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

	// retired counts weeks this tick RESOLVED, which is not the same as weeks
	// that reached a reader: a week whose members all turned out to be
	// unannounceable, and one whose artists have all lost their followers, both
	// resolve to "nothing to send" and are retired.
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

// releaseDigestWeeksReadyToFlush finds the week buckets that have closed.
//
// ONE bound, and it is a calendar bound rather than a quiet-window heuristic:
// a week is ready when its Monday is at least weekHold old, which at the default
// of 168 hours means "the week is over". There is deliberately no MAX HOLD
// sibling here — the venue loop needs one because a venue with a continuous
// trickle never goes quiet, and a bucket that closes on the calendar cannot be
// starved by a trickle.
//
// The bound is on the BUCKET, not on the rows. Bounding it on MAX(created_at)
// would mail a Monday release on Monday and then never mail Wednesday's at all,
// because the delivery claim is per (user, week) and the week's one email would
// already have gone.
//
// Ordered oldest-first so a backlog drains in the order it arrived.
func (s *NotificationFilterService) releaseDigestWeeksReadyToFlush(
	limit int,
	weekHold time.Duration,
) ([]string, error) {
	cutoff := time.Now().UTC().Add(-weekHold).Format(alertWeekLayout)
	var weeks []string
	// alert_week comes back as TEXT, not as a time.Time, for the same reason it
	// goes in as one: a DATE scanned into time.Time is a midnight whose zone
	// depends on the driver, and this value is only ever used as a day.
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
// The rows are marked AFTER delivery, never before. A crash in between costs a
// re-run, and a re-run is silent by construction (the delivery claim). Marking
// first would make a crash cost the roundup entirely.
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
		// Nothing in what we read is announceable: every member was deleted, is no
		// longer credited to its artist, or has aged out of ReleaseAnnounceable.
		// Retire exactly what we READ rather than re-examining it forever; a week
		// that has closed only gets further from announceable.
		s.markReleaseDigestWeekDispatched(week, batch.resolved)
		s.clearReleaseDigestFailure(week)
		return true
	}

	if err := s.deliverReleaseDigestWeek(ctx, batch); err != nil {
		// A CANCELLED tick is a deploy, not a failure. It must not advance the
		// poison-pill bound: doing so would let the single most common operational
		// event abandon a pending roundup, which is the opposite of what the
		// cancellation check was added for.
		if ctx.Err() != nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			log.Printf("release-digest flush: canceled mid-week for %s, left for the next tick", week)
			return false
		}
		// Leave the rows undispatched so the next tick retries. Every step is
		// idempotent, so a retry costs work rather than duplicate mail.
		log.Printf("release-digest flush: %v", err)
		s.noteReleaseDigestFailure(week, maxAge, err)
		return false
	}

	s.markReleaseDigestWeekDispatched(week, batch.resolved)
	s.clearReleaseDigestFailure(week)
	return true
}

// noteReleaseDigestFailure records that a week's flush failed, and abandons it
// once it has been failing for longer than maxAge.
//
// # Why a bound is needed at all
//
// releaseDigestWeeksReadyToFlush orders oldest-first and takes the first `limit`,
// so a week whose delivery errors deterministically is by definition at the HEAD
// of that ordering and re-occupies a slot on every tick. Enough of them and the
// release loop stops delivering platform-wide, with nothing but a repeating log
// line to show for it.
//
// # Why it measures FAILURE DURATION and not bucket age
//
// The obvious implementation — retire a week older than maxAge — is catastrophic
// HERE in a way it merely would have been wrong for the venue loop. A release
// week is ALWAYS at least seven days old by the time it is eligible, so a bound
// on bucket age would abandon every roundup on its first transient error,
// forever. Bucket age answers "how long has this been waiting"; the question is
// "how long has this been broken".
//
// State is in memory rather than a column, deliberately: a restart forgets the
// failure history and a week gets a fresh maxAge, which errs toward RETRYING
// rather than abandoning, and abandoning is the destructive direction. The map
// holds only weeks that are currently failing and every entry is removed on
// success or on retirement.
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

	// Stamps the WHOLE week (nil member set), unlike a normal dispatch. The point
	// is to make the week stop being selected at all; bounding it to what was read
	// would leave the newest rows behind to re-select it immediately.
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
	// Every member row in the week, with its release and its artist.
	//
	// Read WITHOUT any time bound, and the dispatch stamp is bounded by the pairs
	// this returns rather than by a clock. A clock bound looks equivalent and is
	// not: accrual stamps created_at from the Go process before its INSERT
	// commits, so a row can become visible AFTER a later watermark was taken while
	// carrying an EARLIER created_at. Such a row would be stamped dispatched by a
	// flush that never read it — and since the week is then never re-selected,
	// that record is silently never announced to anyone. During a bulk label
	// ingest, which is this feature's whole workload, concurrent accruals against
	// one week make that the common case rather than a rare race.
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
		ORDER BY a.name ASC, a.id ASC, r.release_date DESC NULLS LAST, r.id ASC
	`, week).Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("release digest members for week %s: %w", week, err)
	}

	// Re-check announceability at DELIVERY time with the SAME predicate accrual
	// used. release_date and release_year are both editable, and the window
	// between accrual and flush here is up to a WEEK — far wider than the venue
	// loop's minutes — so a record can genuinely age out or be corrected into
	// back catalogue in between.
	now := time.Now()
	members := make([]releaseDigestRelease, 0, len(rows))
	// resolved is EVERY member row this flush read, announceable or not. It must
	// include the ones being skipped: they were considered and rejected, so
	// leaving them undispatched would re-select the week forever.
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

	// There is deliberately NO post-load sort. The query's ORDER BY is the
	// reading order — artist name, then newest record first — and re-sorting in
	// Go would put a second definition of it somewhere a reviewer has to find.

	return &releaseDigestWeek{week: week, releases: members, resolved: resolved}, nil
}

// ──────────────────────────────────────────────
// Delivery
// ──────────────────────────────────────────────

// deliverReleaseDigestWeek resolves this week's recipients and delivers to each.
//
// A per-recipient failure is collected and returned rather than swallowed, so
// the caller leaves the week undispatched and the next tick retries. Retrying is
// safe by construction: every claim is ON CONFLICT DO NOTHING, so the recipients
// who already got through no-op on the second pass.
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
		// ctx is honoured between RECIPIENTS as well as between weeks, because one
		// week can be an unbounded number of sequential provider requests and that
		// is precisely what would overrun a shutdown. Returning an error leaves the
		// week undispatched, so the next tick finishes the list and the claim keeps
		// the already-delivered recipients silent.
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

// artistReleaseFollowers returns every follow of every artist in this week.
//
// One query for the whole week rather than one per artist: a week's roundup can
// span dozens of artists, and this runs on a poller tick shared with every other
// pending week.
func (s *NotificationFilterService) artistReleaseFollowers(artistIDs []uint) ([]artistReleaseFollowerRow, error) {
	if len(artistIDs) == 0 {
		return nil, nil
	}
	var rows []artistReleaseFollowerRow
	// = ANY(array) rather than IN ?, which GORM expands into one bind parameter
	// PER ELEMENT. A week's artist list is unbounded (a label ingest week can
	// carry hundreds), so that expansion walks toward Postgres's 65535-parameter
	// ceiling — and a week that fails is a week retried at the head of the queue
	// forever.
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
// they will receive.
//
// The fan-in that makes this alert what it is happens here: a user with six
// qualifying follows in one week produces SIX follower rows and ONE recipient.
//
// Channels are OR-ed across a user's follows, which is the opposite of the
// artist show alert's per-lane attribution — and the difference is the message,
// not a change of mind. That alert names ONE artist in its body, so a channel
// enabled for the opener must not send a mail that claims to be about the
// headliner. This one names EVERY artist it covers, so a user who switched email
// on for one of six bands gets an email that truthfully says why, and the other
// five records are in it because they are also records by bands they follow. The
// alternative would be a roundup that silently omits five sixths of the week.
//
// Pure, and takes everything it needs as arguments, so the batching rules can be
// tested without a database.
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
		// Releases is a POINTER on FollowAlertSettings and is nil for entity types
		// that never emit releases. Artists always do, so this is defence rather
		// than a live branch — but a nil deref on a poller tick would take the
		// whole loop down for every user.
		if resolved.Releases == nil {
			continue
		}
		pref := *resolved.Releases

		if !pref.Enabled || (!pref.InApp && !pref.Email) {
			continue
		}

		// NO SCOPE CHECK, deliberately. Release alerts have no scope axis
		// (engagement.followAlertHasScopeAxis returns false for them), so
		// pref.Scope is empty and comparing it against near-me would silence every
		// release follower on the planet.

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
		// The week's releases, in the week's order, narrowed to the artists THIS
		// user subscribed to with the release alert on. Filtering the shared slice
		// rather than rebuilding it keeps every reader's roundup in the same order.
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

// deliverReleaseDigest writes this recipient's rows and, when the email lane is
// on and its row was newly claimed, sends the email.
//
// The structure mirrors deliverVenueAlert exactly, including the three orderings
// that each took a paragraph to justify there:
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
// leave the week undispatched and retry. A failed SEND is not: the row is already
// claimed and claiming is permanent, so retrying could never send that email
// again and reporting it would only strand the whole week.
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

	if !r.email {
		return nil
	}
	if s.emailService == nil || !s.emailService.IsConfigured() {
		return nil
	}
	if !s.withinDailyAlertEmailBudget(r.userID) {
		log.Printf("rate limit: skipping release-digest email for user %d", r.userID)
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
// created it. A false with no error means the lane was already claimed — by an
// earlier flush of the same week, or by a concurrent one.
//
// entity_id is the USER id. That is not a placeholder: a roundup is about the
// reader's whole follow set, so there is no artist or release it could name
// without lying about the rest. See NotificationEntityArtistReleaseDigest.
//
// Raw SQL rather than GORM's Create for one reason: alert_bucket is a DATE and
// the week is a STRING with an explicit ::date cast, so the value cannot be
// re-interpreted through the session time zone. It is also what keeps ON CONFLICT
// DO NOTHING honest about RowsAffected.
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
// Bounded by the exact (artist, release) pairs that were read, never by a clock.
// A row that became visible after the read keeps dispatched_at NULL and
// re-selects the week on the next tick, where it is resolved properly — the
// delivery claim makes that re-run silent. A time bound cannot do this: accrual
// stamps created_at before its INSERT commits, so a row can appear late while
// carrying an early timestamp, and it would be retired by a flush that never saw
// it.
//
// A nil pair set means "the whole week", which is what retirement wants: the
// point there is to make the week stop being selected at all.
func (s *NotificationFilterService) markReleaseDigestWeekDispatched(
	week string,
	resolved []releaseDigestMember,
) {
	q := s.db.Table("artist_release_alert_batch").
		Where("alert_week = ?::date AND dispatched_at IS NULL", week)
	if resolved != nil {
		if len(resolved) == 0 {
			// Read the week and found nothing in it. There is nothing to stamp, and
			// stamping the week would be stamping rows that arrived since.
			return
		}
		// Matched as PAIRS, not as two independent id lists. A week holds many
		// artists and many releases, and an id-list predicate would stamp the cross
		// product — retiring a row this flush never read whenever some other
		// artist in the week happened to share a release id with it.
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
// # No edited-after-accrual guard, and the reason is evidence rather than taste
//
// The venue digest drops members whose show row was written after accrual,
// because a submitter can edit their own approved show with no re-review and the
// digest ships from the platform's DKIM-aligned sender. That risk does not exist
// here: there is NO self-serve release create or edit path. POST /releases is
// registered on the admin router, the community request fulfiller runs only
// after an admin approves, and the pending-edit allowlist for releases is
// admin-reviewed. Every title reaching this template was written by an admin, a
// CLI operator, or MusicBrainz.
//
// Applying the guard anyway would be actively harmful. releases.updated_at moves
// for ANY write to the row, and this feature's window is a WEEK during which the
// cover-art sweep, the link-enrichment sweep and the MBID stamper all touch
// exactly these rows as a matter of routine. The fence would drop most of the
// roundup most weeks — trading a risk that is not present for a broad silent
// loss.
func (s *NotificationFilterService) sendArtistReleaseDigestEmail(
	r releaseDigestRecipient,
	batch *releaseDigestWeek,
) {
	var email string
	if err := s.db.Table("users").Where("id = ?", r.userID).Pluck("email", &email).Error; err != nil || email == "" {
		log.Printf("release-digest notify: no email for user %d: %v", r.userID, err)
		return
	}

	// Its OWN scope, unlike the venue digest which shares the artist show alert's.
	//
	// The rule both decisions follow is one-mutation-one-name. Venue and artist
	// SHOW alerts share a scope because they share a mutation: alert_defaults
	// carries a single `shows` key covering both, so one setter genuinely stops
	// both streams and a second name for it would be two URLs doing one thing.
	// Release alerts are the other case: `releases` is a SEPARATE key with a
	// SEPARATE setter, so reusing UnsubscribeScopeArtistShowAlerts would silence
	// the reader's SHOW alerts when they asked to stop release emails, and leave
	// the release emails coming. An unsubscribe that unsubscribes the wrong stream
	// is worse than no link at all.
	// TestReleaseDigestUsesItsOwnUnsubscribeScope pins this.
	unsubscribeURL := engagement.GenerateScopedUnsubscribeURL(
		engagement.DeriveBackendURL(s.frontendURL),
		r.userID,
		engagement.UnsubscribeScopeArtistReleaseAlerts,
		s.jwtSecret,
	)
	manageURL := fmt.Sprintf("%s/settings/notifications", s.frontendURL)

	html := buildArtistReleaseDigestEmailHTML(r.releases, s.frontendURL, unsubscribeURL, manageURL)

	// The subject is a HEADER. An artist name on an ingest-created row is scraped
	// third-party text, and HTML escaping does nothing for headers: a CR or LF in
	// the name is how a header is split and another one injected. Bounded as well
	// as sanitized, because an unfolded multi-kilobyte subject is
	// provider-dependent behaviour ranging from truncation to outright rejection,
	// and a rejected send is logged and swallowed below.
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

// releaseDigestCountPhrase renders the count the way both the subject line and
// the headline need it. One function so the two can never disagree about whether
// a roundup of one is "1 new release".
func releaseDigestCountPhrase(n int) string {
	if n == 1 {
		return "New release"
	}
	return fmt.Sprintf("%d new releases", n)
}

// releaseDigestSubject builds the Subject header.
//
// A roundup of ONE names the artist, because that is the most useful thing an
// inbox line can say and there is no ambiguity about which one. A roundup of
// several does NOT name any of them: picking one would imply the message is
// about that band, and listing six would overflow every inbox preview.
func releaseDigestSubject(releases []releaseDigestRelease) string {
	if len(releases) == 1 {
		return fmt.Sprintf("New release from %s",
			truncateRunes(sanitizeEmailHeaderValue(releases[0].ArtistName), maxEmailSubjectEntityRunes))
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

// buildArtistReleaseDigestEmailHTML renders the roundup in the shared
// direction-A layout (PSY-1902), so it reads as the same publication as its show
// siblings rather than as a third house style. Figma: node 1584:2.
//
// The one thing it does that no sibling does is GROUP. A venue digest's records
// all share a venue, so a flat hairline-ruled list is the whole story. A release
// roundup spans artists, and a flat list of nine records from six bands is a
// wall — the reader's actual question is "which of my bands put something out",
// which the grouping answers before they read a single title. So it is one
// emailSectionLabel + emailListRows pair per artist.
//
// Every string reaching this is escaped by the builders. That matters here for
// the same reason it matters in the venue digest: an ingest-created release is
// scraped third-party text, and this message ships from the platform's own
// DKIM-aligned sender.
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
				// Each row is its own destination, which is the difference between a
				// roundup a reader can act on and one they have to go looking from.
				// The venue digest has a single CTA because all its shows are at one
				// venue; here every record has its own page.
				Href: rel.URL,
			})
		}
		body.WriteString(emailListRows(rows))
	}

	// The "why this arrived, and why only one of it" sentence. A coalesced alert
	// invites exactly that question, and no other surface answers it at the moment
	// the reader is asking. Capability-true: it promises grouping, not
	// completeness, because a record accrued after this was sent joins the inbox
	// row without triggering a second email.
	body.WriteString(emailParagraph(fmt.Sprintf(
		"You follow %s. New releases from artists you follow are grouped into one roundup a week, so a run of records reaches you as one message instead of one per release.",
		releaseDigestArtistPhrase(groups))))

	// The CTA is the reader's own follow list rather than a catalogue page: this
	// message is about their subscriptions, and /library is where they are managed
	// and where the rest of what they follow lives.
	body.WriteString(emailButton(fmt.Sprintf("%s/library", frontendURL), "View your follows"))

	body.WriteString(emailFineprintWithLinks(
		[]string{"You are getting this because you follow these artists with email alerts on."},
		[]emailFineprintLink{
			// "release alerts", precisely: unlike the show alerts' shared link, this
			// button stops ONLY this stream, and a label promising more than the
			// button does is how a recipient ends up at Report Spam when their show
			// alerts keep arriving.
			{Href: unsubscribeURL, Label: "Unsubscribe from release alerts"},
			{Href: manageURL, Label: "Manage alerts in Settings"},
		},
	))

	return emailShell("RELEASE ALERT · NEW MUSIC", body.String())
}

// releaseTypeLabel renders a release type for the list's mono left column.
//
// Upper-cased rather than title-cased because the column is monospace and reads
// as a tag, and because the stored values are lowercase enum tokens that would
// otherwise render as "lp". An unknown value passes through upper-cased rather
// than being blanked: a type this build has not heard of is still information.
func releaseTypeLabel(releaseType string) string {
	if strings.TrimSpace(releaseType) == "" {
		return "RELEASE"
	}
	return strings.ToUpper(releaseType)
}

// releaseDateLine renders the secondary line under a release title.
//
// Three cases, because the data has three: a full date, a year only, and — for a
// record that reached this template on the year branch of ReleaseAnnounceable —
// nothing more precise to say. An empty string renders no line at all rather
// than an empty one, so the row simply reads as title-only.
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
// subscribe on. Read from the model rather than spelled as a literal, matching
// the show loops: it is half of the WHERE clause that finds the followers AND the
// value engagement's resolver compares against.
var artistReleaseFollowEntityType = string(engagementm.BookmarkEntityArtist)

// alertWeekLayout is how a week bucket is spelled everywhere it crosses the
// Go/SQL boundary.
//
// Weeks are passed to Postgres as STRINGS with an explicit ::date cast, never as
// a time.Time: a time.Time parameter arrives as a timestamptz and is cast to
// date using the SESSION time zone, so the same instant would land on different
// days depending on a setting this package does not control.
//
// Spelled again here rather than imported from the accrual half in
// services/catalog, deliberately: importing it would make the delivery package
// depend on the catalogue service package for a five-character format string.
// TestAccrualAndFlushAgreeOnTheWeekLayout pins the two together instead.
const alertWeekLayout = "2006-01-02"

// ArtistReleaseAlertsEnabled reports whether the release-roundup loop is on.
//
// Read by BOTH halves — accrual in services/catalog and this flush — through the
// one flag NAME in models/notification. Two readers of one constant, rather than
// one reader imported across a package boundary, for the same reason
// alertWeekLayout is: the flag is the shared thing, not the function.
func ArtistReleaseAlertsEnabled() bool {
	return !shared.EnvServiceDisabled(notificationm.ArtistReleaseAlertsDisableFlag)
}

// ──────────────────────────────────────────────
// Inbox enrichment
// ──────────────────────────────────────────────

// releaseDigestSummaryLimit caps how many records an inbox row names.
//
// A week's roundup is unbounded — a reader following two hundred bands during a
// release wave is one row — and an inbox row is one or two lines. The count is
// reported separately and in full, so the row can say "and 12 more" rather than
// lying about the size. Two rather than the venue row's three because each entry
// here carries a title AND an artist name, so the line is roughly half again as
// wide per item.
const releaseDigestSummaryLimit = 2

// enrichArtistReleaseDigestNotifications populates the release count, preview
// and (for a roundup of one) the link on artist_release_digest rows (PSY-1897).
//
// The releases are resolved from artist_release_alert_batch AT READ TIME rather
// than stamped onto the row when it was created, and that is the deliberate half
// of this design: a release accrued later in the week joins the batch, so the
// row a reader is looking at grows to include it without a second notification
// ever having been sent. A snapshot taken at write time could not do that.
//
// # The member query re-applies the DELIVERY rules, and must
//
// Reading the batch live is what lets the row grow, but it also means this query
// is the ONLY thing standing between a member and the reader. Three filters
// mirror what the flush already decided, and each closes a real hole:
//
//   - ANNOUNCEABILITY. Only a deleted release leaves the batch (the foreign key
//     cascades), so without this fence a record whose date is later corrected to
//     1998 would keep rendering in every follower's inbox as new music.
//   - THE CREDIT. artist_releases is re-joined, so a record no longer credited to
//     the artist that accrued it drops out — the same join the flush makes.
//   - THE READER'S OWN SUBSCRIPTION. Only artists this reader currently follows
//     contribute, so a row cannot name records from a week's membership that were
//     never this reader's to hear about.
//
// Degrades rather than disappearing: a week whose members were all deleted
// leaves a count of zero and the frontend renders the row bare. The notification
// still happened, and a row that vanished from history would be a worse lie.
//
// # Two bounds it deliberately does NOT have
//
// The venue enrichment freezes its dedup at own.sent_at because its count is
// computed against a log that keeps growing. There is no cross-system dedup
// here — nothing else in the product notifies anyone about a release — so
// nothing can shrink retroactively for that reason.
//
// It CAN shrink if the reader unfollows an artist, and that is an accepted
// limit rather than an oversight: user_bookmarks records when a follow was
// created and not when it was removed, so the follow set as it stood at delivery
// cannot be reconstructed. The row then reads as a smaller, still-true roundup of
// the bands they still follow. Nor does it filter on the release-alert
// PREFERENCE, unlike the flush: a reader who has since switched the alert off
// keeps seeing the row they already received, because the switch governs future
// alerts rather than editing history.
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
		Title      string  `gorm:"column:title"`
		Slug       *string `gorm:"column:slug"`
		ArtistName string  `gorm:"column:artist_name"`
	}
	// The announceability window is passed as parameters computed from the SAME
	// exported constants ReleaseAnnounceable uses, so the SQL restatement cannot
	// drift from the Go one on its numbers. The predicate's SHAPE is restated,
	// which is the cost of not round-tripping every inbox read through Go;
	// TestInboxEnrichmentAgreesWithReleaseAnnounceable pins the two together.
	now := time.Now()
	if err := s.db.Raw(`
		SELECT DISTINCT ON (b.alert_week, b.release_id)
		       to_char(b.alert_week, 'YYYY-MM-DD') AS alert_week,
		       b.release_id, r.title, r.slug, a.name AS artist_name
		FROM artist_release_alert_batch b
		JOIN releases r ON r.id = b.release_id
		JOIN artists a ON a.id = b.artist_id
		-- The record must still be CREDITED to this artist: artist_releases is
		-- rewritten by a credit correction and never touches the batch row.
		JOIN artist_releases ar ON ar.artist_id = b.artist_id AND ar.release_id = b.release_id
		-- The reader's OWN follow of that artist. This is what makes the count
		-- THIS READER's roundup rather than the week's whole membership.
		JOIN user_bookmarks ub
		  ON ub.user_id = ?
		 AND ub.entity_type = ?
		 AND ub.action = ?
		 AND ub.entity_id = b.artist_id
		WHERE b.alert_week = ANY(?::date[])
		  -- The same fence accrual and the flush applied. See
		  -- catalogm.ReleaseAnnounceable: a dated record must be recent and not
		  -- implausibly far ahead, a year-only record must carry the current year
		  -- or later, and an undated record is never announceable.
		  AND (
		        (r.release_date IS NOT NULL AND r.release_date >= ?::date AND r.release_date <= ?::date)
		     OR (r.release_date IS NULL AND r.release_year IS NOT NULL AND r.release_year >= ?)
		      )
		ORDER BY b.alert_week, b.release_id, a.name ASC
	`,
		userID,
		artistReleaseFollowEntityType,
		string(engagementm.BookmarkActionFollow),
		pq.Array(buckets),
		now.Add(-catalogm.ReleaseRecencyWindow).Format(alertWeekLayout),
		now.Add(catalogm.ReleaseFutureWindow).Format(alertWeekLayout),
		now.Year(),
	).Scan(&members).Error; err != nil {
		log.Printf("warning: failed to load release digest batches for inbox enrichment: %v", err)
		return
	}

	// DISTINCT ON (week, release) collapses a record credited to two artists the
	// reader follows into ONE entry, matching what groupReleasesByArtist does for
	// the email. Without it the bell would say "3 new releases" for two records.
	type weekSummary struct {
		count   int
		preview []string
		soleURL string
	}
	byWeek := make(map[string]*weekSummary, len(buckets))
	for _, m := range members {
		w := byWeek[m.AlertWeek]
		if w == nil {
			w = &weekSummary{}
			byWeek[m.AlertWeek] = w
		}
		w.count++
		if w.count == 1 {
			w.soleURL = entityURL(s.frontendURL, "releases", m.Slug, m.ReleaseID)
		}
		if len(w.preview) < releaseDigestSummaryLimit {
			w.preview = append(w.preview, fmt.Sprintf("%s by %s", m.Title, m.ArtistName))
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
		e.AlertReleaseCount = w.count
		summary := w.preview
		if extra := w.count - len(summary); extra > 0 {
			summary = append(append([]string{}, summary...), fmt.Sprintf("and %d more", extra))
		}
		e.AlertReleaseSummary = strings.Join(summary, " · ")
		// Only a roundup of ONE has an honest single destination; see
		// AlertReleaseURL on the contract for why a multi-record row has none.
		if w.count == 1 {
			e.AlertReleaseURL = w.soleURL
		}
	}
}
