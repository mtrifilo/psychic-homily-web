package notification

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	"gorm.io/gorm"

	authm "psychic-homily-backend/internal/models/auth"
	catalogm "psychic-homily-backend/internal/models/catalog"
	notificationm "psychic-homily-backend/internal/models/notification"
	catalogsvc "psychic-homily-backend/internal/services/catalog"
	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/testutil"
)

// ShowNotifyOutboxSuite covers the drain half of the PSY-1894 outbox end to end:
// a show created through the real ingest funnel must notify a subscribed user
// exactly once, a re-ingest must not re-notify, a show that predates the feature
// must never notify, and a show that travels both the outbox and the admin
// approval route must still yield one notification.
type ShowNotifyOutboxSuite struct {
	suite.Suite
	testDB  *testutil.TestDatabase
	db      *gorm.DB
	filters *NotificationFilterService
	shows   *catalogsvc.ShowService
	poller  *ShowNotifyOutboxPoller
}

func TestShowNotifyOutboxSuite(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	suite.Run(t, new(ShowNotifyOutboxSuite))
}

func (s *ShowNotifyOutboxSuite) SetupSuite() {
	s.testDB = testutil.SetupTestPostgres(s.T())
	s.db = s.testDB.DB
	s.filters = NewNotificationFilterService(s.db, &mockEmailService{}, "test-secret", "http://localhost:3000")
	s.shows = catalogsvc.NewShowService(s.db)
	s.poller = NewShowNotifyOutboxPoller(s.db, s.filters)
}

func (s *ShowNotifyOutboxSuite) TearDownSuite() { s.testDB.Cleanup() }

func (s *ShowNotifyOutboxSuite) TearDownTest() {
	s.db.Exec("DELETE FROM notification_log")
	s.db.Exec("DELETE FROM notification_filters")
	s.db.Exec("DELETE FROM user_bookmarks")
	s.db.Exec("DELETE FROM scenes")
	s.db.Exec("DELETE FROM show_notify_queue")
	s.db.Exec("DELETE FROM show_artists")
	s.db.Exec("DELETE FROM show_venues")
	s.db.Exec("DELETE FROM shows")
	s.db.Exec("DELETE FROM artists")
	s.db.Exec("DELETE FROM venues")
	s.db.Exec("DELETE FROM users")
}

// --- fixtures ---

func (s *ShowNotifyOutboxSuite) createUser() uint {
	email := fmt.Sprintf("outbox-%d@example.com", time.Now().UnixNano())
	user := authm.User{Email: &email}
	s.Require().NoError(s.db.Create(&user).Error)
	return user.ID
}

// subscribe gives the user a filter that matches any show featuring artistName.
func (s *ShowNotifyOutboxSuite) subscribe(userID uint, artistID uint) {
	_, err := s.filters.CreateFilter(userID, contracts.CreateFilterInput{
		Name:        "Outbox subscription",
		ArtistIDs:   []int64{int64(artistID)},
		NotifyEmail: true,
	})
	s.Require().NoError(err)
}

// ingestShow drives the REAL show write funnel — the one POST /shows and the `ph`
// ingest CLI use — so the test exercises the production enqueue, not a hand-placed
// queue row. Returns the show id and the artist id on the bill.
func (s *ShowNotifyOutboxSuite) ingestShow(title string) (showID uint, artistID uint) {
	unique := time.Now().UnixNano()
	artistName := fmt.Sprintf("Ingest Band %d", unique)
	resp, err := s.shows.CreateShow(&contracts.CreateShowRequest{
		Title:     title,
		EventDate: time.Now().Add(30 * 24 * time.Hour),
		City:      "Phoenix",
		State:     "AZ",
		// Admin-submitted, i.e. the INGEST shape the trust gate admits (the `ph`
		// CLI's phk_ token resolves to an admin user).
		SubmitterIsAdmin: true,
		Venues: []contracts.CreateShowVenue{
			{Name: fmt.Sprintf("Ingest Venue %d", unique), City: "Phoenix", State: "AZ"},
		},
		Artists: []contracts.CreateShowArtist{{Name: artistName}},
	})
	s.Require().NoError(err)
	s.Require().Len(resp.Artists, 1)
	return resp.ID, resp.Artists[0].ID
}

func (s *ShowNotifyOutboxSuite) notificationCount(userID, showID uint) int64 {
	var n int64
	s.Require().NoError(s.db.Model(&notificationm.NotificationLog{}).
		Where("user_id = ? AND entity_type = ? AND entity_id = ?", userID, "show", showID).
		Count(&n).Error)
	return n
}

func (s *ShowNotifyOutboxSuite) loadShow(showID uint) *catalogm.Show {
	var show catalogm.Show
	s.Require().NoError(s.db.First(&show, showID).Error)
	return &show
}

func (s *ShowNotifyOutboxSuite) jobFor(showID uint) catalogm.ShowNotifyQueueItem {
	var item catalogm.ShowNotifyQueueItem
	s.Require().NoError(s.db.Where("show_id = ?", showID).First(&item).Error)
	return item
}

// --- acceptance criteria ---

// TestIngestCreatedShowNotifiesExactlyOnce is the headline acceptance criterion:
// before PSY-1894 this show would have notified nobody, because MatchAndNotify
// fired only from the admin approve handlers and ingest shows are born approved.
//
// It also asserts the poller is IDEMPOTENT across ticks — a second RunNow finds no
// pending row and delivers nothing further.
func (s *ShowNotifyOutboxSuite) TestIngestCreatedShowNotifiesExactlyOnce() {
	userID := s.createUser()
	showID, artistID := s.ingestShow("Ingest notifies")
	s.subscribe(userID, artistID)

	s.Require().Equal(int64(0), s.notificationCount(userID, showID), "nothing before the poller runs")

	s.poller.RunNow(context.Background())
	s.Equal(int64(1), s.notificationCount(userID, showID))
	s.Equal(catalogm.ShowNotifyStatusDone, s.jobFor(showID).Status)

	s.poller.RunNow(context.Background())
	s.poller.RunNow(context.Background())
	s.Equal(int64(1), s.notificationCount(userID, showID), "extra ticks must not re-notify")
}

// TestReIngestDoesNotReNotify is the dedup acceptance criterion at the level that
// matters to a subscriber: after the notification has gone out, re-running the
// enqueue (a venue calendar re-scrape, a repeated `ph batch`) and polling again
// produces nothing.
func (s *ShowNotifyOutboxSuite) TestReIngestDoesNotReNotify() {
	userID := s.createUser()
	showID, artistID := s.ingestShow("Re-ingest")
	s.subscribe(userID, artistID)

	s.poller.RunNow(context.Background())
	s.Require().Equal(int64(1), s.notificationCount(userID, showID))

	// Re-ingest: the same show is written again by the same funnel.
	catalogsvc.EnqueueShowNotify(s.db, s.loadShow(showID), catalogsvc.ShowNotifyIngest)
	s.poller.RunNow(context.Background())

	s.Equal(int64(1), s.notificationCount(userID, showID))

	var jobs int64
	s.Require().NoError(s.db.Model(&catalogm.ShowNotifyQueueItem{}).Where("show_id = ?", showID).Count(&jobs).Error)
	s.Equal(int64(1), jobs, "the terminal row must keep blocking re-enqueue")
}

// TestRolloutDoesNotNotifyForPreExistingShows is the NO-BLAST assertion, and the
// one this ticket says must be test-asserted rather than assumed.
//
// The scenario is the deploy itself: shows already in the catalogue, subscriptions
// that match them, and a poller starting for the first time. It must deliver
// nothing, because those shows have no queue row and nothing backfills one. Note
// there is no clock involved — the guard is the empty table, not a watermark, so
// this holds no matter how the rows' timestamps compare to the deploy.
func (s *ShowNotifyOutboxSuite) TestRolloutDoesNotNotifyForPreExistingShows() {
	userID := s.createUser()

	// A catalogue that predates the feature: rows written directly, exactly as the
	// existing ~7,600 approved shows sit in production today, with no queue row.
	var preExisting []uint
	for i := 0; i < 5; i++ {
		showID, artistID := s.ingestShow(fmt.Sprintf("Historical show %d", i))
		s.subscribe(userID, artistID)
		preExisting = append(preExisting, showID)
	}
	// Erase every trace of the feature, leaving only the shows themselves. This is
	// the state a first deploy actually finds.
	s.Require().NoError(s.db.Exec("DELETE FROM show_notify_queue").Error)

	var queued int64
	s.Require().NoError(s.db.Model(&catalogm.ShowNotifyQueueItem{}).Count(&queued).Error)
	s.Require().Equal(int64(0), queued)

	// Several ticks, i.e. the poller running for a while after the deploy.
	for i := 0; i < 3; i++ {
		s.poller.RunNow(context.Background())
	}

	for _, showID := range preExisting {
		s.Equal(int64(0), s.notificationCount(userID, showID),
			"pre-existing show %d must not notify on rollout", showID)
	}

	var totalLogs int64
	s.Require().NoError(s.db.Model(&notificationm.NotificationLog{}).Count(&totalLogs).Error)
	s.Equal(int64(0), totalLogs, "rollout must produce no notifications at all")

	// And the feature still works for the NEXT show, so the guarantee is not just
	// "the poller does nothing".
	newShowID, newArtistID := s.ingestShow("First post-deploy show")
	s.subscribe(userID, newArtistID)
	s.poller.RunNow(context.Background())
	s.Equal(int64(1), s.notificationCount(userID, newShowID))
}

// TestAdminApprovalPathStillNotifiesOnce guards the no-double-fire requirement.
// The admin approve handlers keep their direct MatchAndNotify call, so a show that
// travels BOTH routes (born approved and enqueued, then later re-approved through
// the moderation queue) must still produce one notification. The per-(user, show)
// dedup in notification_log is what makes the second pass a no-op, and this test
// is what stops a future change from removing it silently.
func (s *ShowNotifyOutboxSuite) TestAdminApprovalPathStillNotifiesOnce() {
	userID := s.createUser()
	showID, artistID := s.ingestShow("Both routes")
	s.subscribe(userID, artistID)

	// Route 1: the outbox.
	s.poller.RunNow(context.Background())
	s.Require().Equal(int64(1), s.notificationCount(userID, showID))

	// Route 2: the admin approve handler's direct call, replayed exactly as
	// ApproveShowHandler makes it.
	var show catalogm.Show
	s.Require().NoError(s.db.First(&show, showID).Error)
	s.Require().NoError(s.filters.MatchAndNotify(&show))

	s.Equal(int64(1), s.notificationCount(userID, showID), "the two routes must not double-fire")
}

// TestAdminApprovalIsUnchangedForModeratedShows: a show that enters the moderation
// queue (status pending) is NOT enqueued, so the admin path remains the only thing
// that notifies for it — exactly as before this ticket.
func (s *ShowNotifyOutboxSuite) TestAdminApprovalIsUnchangedForModeratedShows() {
	userID := s.createUser()
	showID, artistID := s.ingestShow("Moderated")
	s.subscribe(userID, artistID)

	// Put the show back in the state a moderated submission has, and drop the row
	// the funnel wrote, i.e. pretend it was never born visible.
	s.Require().NoError(s.db.Model(&catalogm.Show{}).Where("id = ?", showID).
		Update("status", catalogm.ShowStatusPending).Error)
	s.Require().NoError(s.db.Exec("DELETE FROM show_notify_queue").Error)

	catalogsvc.EnqueueShowNotify(s.db, s.loadShow(showID), catalogsvc.ShowNotifyIngest)
	s.poller.RunNow(context.Background())
	s.Require().Equal(int64(0), s.notificationCount(userID, showID), "the outbox must stay out of the moderation queue")

	// The admin approve transition, and its existing direct call.
	s.Require().NoError(s.db.Model(&catalogm.Show{}).Where("id = ?", showID).
		Update("status", catalogm.ShowStatusApproved).Error)
	var show catalogm.Show
	s.Require().NoError(s.db.First(&show, showID).Error)
	s.Require().NoError(s.filters.MatchAndNotify(&show))

	s.Equal(int64(1), s.notificationCount(userID, showID))
}

// --- delivery-time safety ---

// A show pulled between enqueue and delivery must not be announced. The two cases
// are covered separately below rather than here, because "not publicly visible" is
// the one non-announceable reason that routinely un-happens:
// TestTransientUnpublishDoesNotBurnTheOnlyChance (retried, then delivered) and
// TestPermanentlyUnpublishedJobExpiresRatherThanRetryingForever (bounded by
// maxJobAge). Cancelled and deleted shows, which do not un-happen, are terminal
// and are covered by their own tests.

// TestDeletedShowIsSkipped pins the defensive branch for a job whose show no
// longer exists.
//
// In the shipped schema the FK cascade normally takes the job with the show, so
// this branch is defense in depth rather than a routine path — which is exactly
// why it needs a test: without one, a future schema change that drops the cascade
// would turn "show deleted" into a permanent retry loop ending in `failed`, and
// nobody would notice until an operator asked why the queue had a failed tail.
// The test detaches the FK to reproduce a job that outlived its show.
func (s *ShowNotifyOutboxSuite) TestDeletedShowIsSkipped() {
	showID, _ := s.ingestShow("Deleted before delivery")

	s.Require().NoError(s.db.Exec("ALTER TABLE show_notify_queue DROP CONSTRAINT show_notify_queue_show_id_fkey").Error)
	defer func() {
		// The orphan must go before the constraint comes back, or the re-add fails
		// validating it. TearDownTest would clear the table anyway; doing it here
		// keeps the schema restore self-contained.
		s.Require().NoError(s.db.Exec("DELETE FROM show_notify_queue").Error)
		s.Require().NoError(s.db.Exec(
			"ALTER TABLE show_notify_queue ADD CONSTRAINT show_notify_queue_show_id_fkey " +
				"FOREIGN KEY (show_id) REFERENCES shows(id) ON DELETE CASCADE").Error)
	}()
	s.Require().NoError(s.db.Exec("DELETE FROM show_artists WHERE show_id = ?", showID).Error)
	s.Require().NoError(s.db.Exec("DELETE FROM show_venues WHERE show_id = ?", showID).Error)
	s.Require().NoError(s.db.Exec("DELETE FROM shows WHERE id = ?", showID).Error)

	s.poller.RunNow(context.Background())

	job := s.jobFor(showID)
	s.Equal(catalogm.ShowNotifyStatusSkipped, job.Status)
	s.Require().NotNil(job.LastError)
	s.Equal(catalogm.NotAnnounceableGone, *job.LastError)
}

// TestFkCascadeRemovesJobWithShow documents the routine case the branch above
// backstops: deleting a show (cmd/dedup-shows merging a duplicate away, an admin
// delete) takes its job with it, so the queue cannot accumulate rows pointing at
// nothing.
func (s *ShowNotifyOutboxSuite) TestFkCascadeRemovesJobWithShow() {
	showID, _ := s.ingestShow("Cascaded away")
	s.Require().NotZero(s.jobFor(showID).ID)

	s.Require().NoError(s.db.Exec("DELETE FROM show_artists WHERE show_id = ?", showID).Error)
	s.Require().NoError(s.db.Exec("DELETE FROM show_venues WHERE show_id = ?", showID).Error)
	s.Require().NoError(s.db.Exec("DELETE FROM shows WHERE id = ?", showID).Error)

	var n int64
	s.Require().NoError(s.db.Model(&catalogm.ShowNotifyQueueItem{}).Where("show_id = ?", showID).Count(&n).Error)
	s.Equal(int64(0), n)
}

// --- failure handling ---

// failingMatcher fails a fixed number of times, then succeeds, so the retry and
// give-up paths can both be driven.
type failingMatcher struct {
	failures int
	calls    int
	delegate showMatcher
	// failWith overrides the default error, so a test can drive the
	// cancellation-classification path with a real context error.
	failWith error
}

func (m *failingMatcher) MatchAndNotify(show *catalogm.Show) error {
	m.calls++
	if m.calls <= m.failures {
		if m.failWith != nil {
			return m.failWith
		}
		return errors.New("transient match failure")
	}
	if m.delegate != nil {
		return m.delegate.MatchAndNotify(show)
	}
	return nil
}

// TestMatchFailureRetriesThenSucceeds: a transient failure must return the job to
// pending rather than swallow it, and the eventual success must still notify. A
// silently dropped job is a subscriber who never hears about the show.
func (s *ShowNotifyOutboxSuite) TestMatchFailureRetriesThenSucceeds() {
	userID := s.createUser()
	showID, artistID := s.ingestShow("Retry me")
	s.subscribe(userID, artistID)

	matcher := &failingMatcher{failures: 1, delegate: s.filters}
	poller := NewShowNotifyOutboxPoller(s.db, matcher)

	poller.RunNow(context.Background())
	job := s.jobFor(showID)
	s.Require().Equal(catalogm.ShowNotifyStatusPending, job.Status)
	s.Require().Equal(1, job.Attempts)
	s.Require().Equal(int64(0), s.notificationCount(userID, showID))

	poller.RunNow(context.Background())
	s.Equal(catalogm.ShowNotifyStatusDone, s.jobFor(showID).Status)
	s.Equal(int64(1), s.notificationCount(userID, showID))
}

// TestMatchFailureGivesUpAfterMaxAttempts: the job must go terminal rather than
// spin forever. Failing closed on notifications is the right trade, and `failed`
// with the cause recorded is what makes the miss visible to an operator.
func (s *ShowNotifyOutboxSuite) TestMatchFailureGivesUpAfterMaxAttempts() {
	showID, _ := s.ingestShow("Always fails")

	matcher := &failingMatcher{failures: 99}
	poller := NewShowNotifyOutboxPoller(s.db, matcher)

	for i := 0; i < 4; i++ {
		poller.RunNow(context.Background())
	}

	job := s.jobFor(showID)
	s.Equal(catalogm.ShowNotifyStatusFailed, job.Status)
	s.Equal(3, job.Attempts, "must stop at max_attempts, not keep burning attempts")
	s.Require().NotNil(job.LastError)
	s.Contains(*job.LastError, "transient match failure")
}

// TestCancelledBetweenEnqueueAndDeliveryIsSkipped: the delivery-time re-check has
// to cover cancellation, not just visibility. A venue calling a show off in the
// minute between ingest and the poll is ordinary, and "New show" for a cancelled
// event is the single worst email this feature could send.
func (s *ShowNotifyOutboxSuite) TestCancelledBetweenEnqueueAndDeliveryIsSkipped() {
	userID := s.createUser()
	showID, artistID := s.ingestShow("Cancelled after ingest")
	s.subscribe(userID, artistID)

	s.Require().NoError(s.db.Model(&catalogm.Show{}).Where("id = ?", showID).
		Update("is_cancelled", true).Error)

	s.poller.RunNow(context.Background())

	s.Equal(int64(0), s.notificationCount(userID, showID))
	job := s.jobFor(showID)
	s.Equal(catalogm.ShowNotifyStatusSkipped, job.Status)
	s.Require().NotNil(job.LastError)
	s.Equal(catalogm.NotAnnounceableCancelled, *job.LastError)
}

// TestShowThatHappenedBeforeDeliveryIsSkipped covers the poller-outage case: a job
// enqueued for a real future show, drained only after the show has been and gone.
// Announcing it then is noise, and it is the same rule that keeps an archival
// ingest quiet at the enqueue end.
func (s *ShowNotifyOutboxSuite) TestShowThatHappenedBeforeDeliveryIsSkipped() {
	userID := s.createUser()
	showID, artistID := s.ingestShow("Happened during the outage")
	s.subscribe(userID, artistID)

	s.Require().NoError(s.db.Model(&catalogm.Show{}).Where("id = ?", showID).
		Update("event_date", time.Now().Add(-48*time.Hour)).Error)

	s.poller.RunNow(context.Background())

	s.Equal(int64(0), s.notificationCount(userID, showID))
	job := s.jobFor(showID)
	s.Equal(catalogm.ShowNotifyStatusSkipped, job.Status)
	s.Require().NotNil(job.LastError)
	s.Equal(catalogm.NotAnnounceablePastEvent, *job.LastError)
}

// TestSoldOutShowStillNotifies pins the deliberate asymmetry with cancellation: a
// sold-out show is real information a follower wants, and the admin approve path
// has always notified for one. Excluding it would be a silent behaviour change
// dressed as safety.
func (s *ShowNotifyOutboxSuite) TestSoldOutShowStillNotifies() {
	userID := s.createUser()
	showID, artistID := s.ingestShow("Sold out")
	s.subscribe(userID, artistID)

	s.Require().NoError(s.db.Model(&catalogm.Show{}).Where("id = ?", showID).
		Update("is_sold_out", true).Error)

	s.poller.RunNow(context.Background())

	s.Equal(int64(1), s.notificationCount(userID, showID))
	s.Equal(catalogm.ShowNotifyStatusDone, s.jobFor(showID).Status)
}

// TestMissingMatcherLeavesJobsPending: a wiring bug must not consume the queue.
// If the tick claimed rows and then failed each one, every job would burn its
// attempts against a condition no retry can fix and land `failed` — which is
// terminal, so those shows could never be notified even after the config was
// corrected.
func (s *ShowNotifyOutboxSuite) TestMissingMatcherLeavesJobsPending() {
	showID, _ := s.ingestShow("Poller misconfigured")

	NewShowNotifyOutboxPoller(s.db, nil).RunNow(context.Background())

	job := s.jobFor(showID)
	s.Equal(catalogm.ShowNotifyStatusPending, job.Status)
	s.Equal(0, job.Attempts, "a misconfigured poller must not burn attempts")

	// A correctly wired poller then picks the job up untouched.
	s.poller.RunNow(context.Background())
	s.Equal(catalogm.ShowNotifyStatusDone, s.jobFor(showID).Status)
}

// TestTransientUnpublishDoesNotBurnTheOnlyChance: unpublish, fix a typo, republish
// is an ordinary user correction flow, and the window is a whole poll interval. A
// terminal skip there would spend the show's only notification silently — the
// whole-table UNIQUE blocks re-enqueue and PublishShow does not enqueue — so
// "not publicly visible" must be retried rather than given up on.
func (s *ShowNotifyOutboxSuite) TestTransientUnpublishDoesNotBurnTheOnlyChance() {
	userID := s.createUser()
	showID, artistID := s.ingestShow("Briefly unpublished")
	s.subscribe(userID, artistID)

	s.Require().NoError(s.db.Model(&catalogm.Show{}).Where("id = ?", showID).
		Update("status", catalogm.ShowStatusPrivate).Error)
	s.poller.RunNow(context.Background())

	job := s.jobFor(showID)
	s.Require().Equal(catalogm.ShowNotifyStatusPending, job.Status, "must stay retryable, not go terminal")
	s.Equal(1, job.Attempts, "the retry burns an attempt, which is what keeps it self-limiting")
	s.Equal(int64(0), s.notificationCount(userID, showID))

	// Republished: the next tick delivers.
	s.Require().NoError(s.db.Model(&catalogm.Show{}).Where("id = ?", showID).
		Update("status", catalogm.ShowStatusApproved).Error)
	s.poller.RunNow(context.Background())

	s.Equal(catalogm.ShowNotifyStatusDone, s.jobFor(showID).Status)
	s.Equal(int64(1), s.notificationCount(userID, showID))
}

// TestPermanentlyUnpublishedJobGoesTerminalAfterMaxAttempts: the retry above must
// be SELF-LIMITING. Claim orders by created_at ASC and never rewrites it, so a row
// re-pended without burning an attempt sits at the head of the queue forever.
// Burning the attempt caps it at max_attempts ticks, after which it is `skipped`
// (nothing malfunctioned) rather than `failed`.
func (s *ShowNotifyOutboxSuite) TestPermanentlyUnpublishedJobGoesTerminalAfterMaxAttempts() {
	showID, _ := s.ingestShow("Unpublished for good")

	s.Require().NoError(s.db.Model(&catalogm.Show{}).Where("id = ?", showID).
		Update("status", catalogm.ShowStatusPrivate).Error)

	for i := 0; i < 5; i++ {
		s.poller.RunNow(context.Background())
	}

	job := s.jobFor(showID)
	s.Equal(catalogm.ShowNotifyStatusSkipped, job.Status)
	s.Equal(3, job.Attempts, "the retry must stop at max_attempts, not cycle forever")
	s.Require().NotNil(job.LastError)
	s.Equal(catalogm.NotAnnounceableNotPublic, *job.LastError)
}

// TestKillSwitchStopsDrainingWithoutARestart: an operator setting the flag during
// an incident wants sending to stop NOW. cmd/server only consults it at boot, so a
// boot-time-only gate would keep draining the backlog after they believed they had
// stopped it.
func (s *ShowNotifyOutboxSuite) TestKillSwitchStopsDrainingWithoutARestart() {
	userID := s.createUser()
	showID, artistID := s.ingestShow("Enqueued before the brake")
	s.subscribe(userID, artistID)

	// The poller was constructed while the feature was on, as it is at boot.
	s.T().Setenv(catalogm.ShowNotifyOutboxDisableFlag, "1")
	s.poller.RunNow(context.Background())

	s.Equal(int64(0), s.notificationCount(userID, showID))
	s.Equal(catalogm.ShowNotifyStatusPending, s.jobFor(showID).Status,
		"the job is held, not consumed, so clearing the flag resumes it")
}

// TestStuckJobsDoNotStarveNewerOnes is the regression test for the failure the
// round-2 panel found: a full batch of rows that keep re-pending would monopolise
// every claim, because the queue is FIFO on created_at and those rows are the
// oldest. Newer shows would then sit unclaimed until maxJobAge discarded them
// silently, which is the exact opposite of what this feature is for.
//
// Deliberately seeded with `batch` stuck rows, since any smaller number cannot
// expose it.
func (s *ShowNotifyOutboxSuite) TestStuckJobsDoNotStarveNewerOnes() {
	userID := s.createUser()

	// A full batch of shows that are enqueued and then unpublished, so every one of
	// them is claimable but undeliverable.
	stuck := make([]uint, 0, defaultShowNotifyBatch)
	for i := 0; i < defaultShowNotifyBatch; i++ {
		showID, _ := s.ingestShow(fmt.Sprintf("Stuck %d", i))
		s.Require().NoError(s.db.Model(&catalogm.Show{}).Where("id = ?", showID).
			Update("status", catalogm.ShowStatusPrivate).Error)
		stuck = append(stuck, showID)
	}

	// One good show enqueued AFTER them, so FIFO order puts it behind all five.
	freshID, freshArtist := s.ingestShow("Behind the stuck ones")
	s.subscribe(userID, freshArtist)

	// max_attempts ticks retire the stuck rows; the next tick reaches the fresh one.
	for i := 0; i < 5; i++ {
		s.poller.RunNow(context.Background())
	}

	for _, showID := range stuck {
		s.Equal(catalogm.ShowNotifyStatusSkipped, s.jobFor(showID).Status,
			"stuck show %d must go terminal instead of camping the queue", showID)
	}
	s.Equal(catalogm.ShowNotifyStatusDone, s.jobFor(freshID).Status)
	s.Equal(int64(1), s.notificationCount(userID, freshID),
		"a newer announcement must not be starved by undeliverable older rows")
}

// TestExpiredBacklogIsReapedAsASetBeforeClaiming: expired rows must never occupy a
// batch slot. Reaping them one-at-a-time through the claim would cap backlog
// recovery at batch-per-tick, so a large backlog would spend hours draining rows
// that were never going to be delivered while newer, deliverable ones aged out
// behind them.
func (s *ShowNotifyOutboxSuite) TestExpiredBacklogIsReapedAsASetBeforeClaiming() {
	userID := s.createUser()

	// A backlog several times the batch size, all aged out.
	expired := make([]uint, 0, defaultShowNotifyBatch*3)
	for i := 0; i < defaultShowNotifyBatch*3; i++ {
		showID, _ := s.ingestShow(fmt.Sprintf("Aged out %d", i))
		expired = append(expired, showID)
	}
	s.Require().NoError(s.db.Model(&catalogm.ShowNotifyQueueItem{}).
		Where("show_id IN ?", expired).
		Update("created_at", time.Now().Add(-72*time.Hour)).Error)

	// One deliverable show behind them all.
	freshID, freshArtist := s.ingestShow("Still worth sending")
	s.subscribe(userID, freshArtist)

	// ONE tick must clear the whole backlog and still deliver the fresh show.
	s.poller.RunNow(context.Background())

	for _, showID := range expired {
		job := s.jobFor(showID)
		s.Equal(catalogm.ShowNotifyStatusSkipped, job.Status)
		s.Require().NotNil(job.LastError)
		s.Equal(skipJobTooOld, *job.LastError)
	}
	s.Equal(int64(1), s.notificationCount(userID, freshID),
		"backlog recovery must not delay a deliverable announcement")
}

// TestStaleBacklogIsDroppedRatherThanBurst is the burst fail-safe. However a
// backlog accumulated — a poller outage, a deploy freeze, or the kill switch set on
// the server while a separately-configured ingest process kept writing rows —
// clearing the condition must not fan a week of announcements out at once.
func (s *ShowNotifyOutboxSuite) TestStaleBacklogIsDroppedRatherThanBurst() {
	userID := s.createUser()
	showID, artistID := s.ingestShow("Queued a week ago")
	s.subscribe(userID, artistID)

	// A perfectly announceable show (future date, approved, not cancelled) whose job
	// simply waited too long.
	s.Require().NoError(s.db.Model(&catalogm.ShowNotifyQueueItem{}).Where("show_id = ?", showID).
		Update("created_at", time.Now().Add(-7*24*time.Hour)).Error)

	s.poller.RunNow(context.Background())

	s.Equal(int64(0), s.notificationCount(userID, showID))
	job := s.jobFor(showID)
	s.Equal(catalogm.ShowNotifyStatusSkipped, job.Status)
	s.Require().NotNil(job.LastError)
	s.Equal(skipJobTooOld, *job.LastError)
}

// TestMidItemCancellationDoesNotBurnAnAttempt closes the one-item-wide hole in
// processTick's between-items cancellation check: a SIGTERM landing DURING
// loadShow or MatchAndNotify surfaces as context.Canceled, and counting that as a
// failed attempt would let rolling restarts walk a row to terminal `failed`
// without the match ever having been tried.
func (s *ShowNotifyOutboxSuite) TestMidItemCancellationDoesNotBurnAnAttempt() {
	showID, _ := s.ingestShow("Canceled mid-item")

	poller := NewShowNotifyOutboxPoller(s.db, &failingMatcher{failures: 1, failWith: context.Canceled})
	poller.RunNow(context.Background())

	job := s.jobFor(showID)
	s.Equal(catalogm.ShowNotifyStatusPending, job.Status)
	s.Equal(0, job.Attempts, "a cancellation is a deploy, not an attempt")

	// And the work is not lost.
	s.poller.RunNow(context.Background())
	s.Equal(catalogm.ShowNotifyStatusDone, s.jobFor(showID).Status)
}

// cancelingMatcher cancels the tick context after its first call, reproducing a
// deploy landing partway through a claimed batch.
type cancelingMatcher struct {
	cancel   context.CancelFunc
	calls    int
	delegate showMatcher
}

func (m *cancelingMatcher) MatchAndNotify(show *catalogm.Show) error {
	m.calls++
	err := m.delegate.MatchAndNotify(show)
	if m.calls == 1 {
		m.cancel()
	}
	return err
}

// TestCanceledTickRequeuesWithoutBurningAnAttempt: a deploy that lands partway
// through a claimed batch must requeue the untouched jobs and DECREMENT the
// claim-time attempt, so rolling restarts cannot walk a job to `failed` without
// ever having tried it. A show silently stranded that way is a subscriber who
// never hears about it.
//
// It also pins the other half of the contract: the job that DID deliver before the
// cancellation is finalized as done rather than left `processing`, because the
// finalize context is detached from the tick's cancellation. Left processing it
// would be reclaimed and notify a second time.
func (s *ShowNotifyOutboxSuite) TestCanceledTickRequeuesWithoutBurningAnAttempt() {
	userID := s.createUser()
	firstShow, firstArtist := s.ingestShow("Delivered before shutdown")
	s.subscribe(userID, firstArtist)
	// Enqueued second, so the created_at ordering of the claim is deterministic.
	secondShow, secondArtist := s.ingestShow("Interrupted by shutdown")
	s.subscribe(userID, secondArtist)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	poller := NewShowNotifyOutboxPoller(s.db, &cancelingMatcher{cancel: cancel, delegate: s.filters})
	poller.RunNow(ctx)

	delivered := s.jobFor(firstShow)
	s.Equal(catalogm.ShowNotifyStatusDone, delivered.Status, "a delivered job must be finalized despite the cancel")
	s.Equal(int64(1), s.notificationCount(userID, firstShow))

	interrupted := s.jobFor(secondShow)
	s.Equal(catalogm.ShowNotifyStatusPending, interrupted.Status)
	s.Equal(0, interrupted.Attempts, "a shutdown must not count as an attempt")
	s.Equal(int64(0), s.notificationCount(userID, secondShow))

	// The interrupted job is not lost: a later tick delivers it, exactly once.
	s.poller.RunNow(context.Background())
	s.Equal(catalogm.ShowNotifyStatusDone, s.jobFor(secondShow).Status)
	s.Equal(int64(1), s.notificationCount(userID, secondShow))
	s.Equal(int64(1), s.notificationCount(userID, firstShow), "the delivered job must not re-notify")
}

// TestStaleProcessingRowIsReclaimed: a job orphaned by a crash between claim and
// finalize is the only way a row can be lost. reclaimStale is what recovers it.
func (s *ShowNotifyOutboxSuite) TestStaleProcessingRowIsReclaimed() {
	userID := s.createUser()
	showID, artistID := s.ingestShow("Orphaned")
	s.subscribe(userID, artistID)

	// A row abandoned in `processing` long enough to be stale.
	s.Require().NoError(s.db.Model(&catalogm.ShowNotifyQueueItem{}).
		Where("show_id = ?", showID).
		Updates(map[string]interface{}{
			"status":     catalogm.ShowNotifyStatusProcessing,
			"attempts":   1,
			"updated_at": time.Now().Add(-2 * time.Hour),
		}).Error)

	s.poller.RunNow(context.Background())

	s.Equal(catalogm.ShowNotifyStatusDone, s.jobFor(showID).Status)
	s.Equal(int64(1), s.notificationCount(userID, showID))
}
