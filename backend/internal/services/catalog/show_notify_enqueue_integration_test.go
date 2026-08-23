package catalog

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	"gorm.io/gorm"

	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/testutil"
)

// ShowNotifyEnqueueSuite covers the enqueue half of the PSY-1894 outbox: the show
// write funnel must enqueue exactly one job for a show that is born publicly
// visible, must stay silent for anything that is not, must be a no-op on
// re-enqueue, and must never be able to fail the show write it rides along with.
type ShowNotifyEnqueueSuite struct {
	suite.Suite
	testDB *testutil.TestDatabase
	db     *gorm.DB
	svc    *ShowService
}

func TestShowNotifyEnqueueSuite(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	suite.Run(t, new(ShowNotifyEnqueueSuite))
}

func (s *ShowNotifyEnqueueSuite) SetupSuite() {
	s.testDB = testutil.SetupTestPostgres(s.T())
	s.db = s.testDB.DB
	s.svc = NewShowService(s.db)
}

func (s *ShowNotifyEnqueueSuite) TearDownSuite() { s.testDB.Cleanup() }

func (s *ShowNotifyEnqueueSuite) TearDownTest() {
	s.db.Exec("DELETE FROM show_notify_queue")
	s.db.Exec("DELETE FROM show_artists")
	s.db.Exec("DELETE FROM show_venues")
	s.db.Exec("DELETE FROM shows")
	s.db.Exec("DELETE FROM artists")
	s.db.Exec("DELETE FROM venues")
}

func (s *ShowNotifyEnqueueSuite) queueCount() int64 {
	var n int64
	s.Require().NoError(s.db.Model(&catalogm.ShowNotifyQueueItem{}).Count(&n).Error)
	return n
}

// createShow drives the real funnel, the same one POST /shows (and therefore the
// `ph` ingest CLI) uses.
func (s *ShowNotifyEnqueueSuite) createShow(title string, private bool) *contracts.ShowResponse {
	unique := time.Now().UnixNano()
	resp, err := s.svc.CreateShow(&contracts.CreateShowRequest{
		Title:     title,
		EventDate: time.Now().Add(30 * 24 * time.Hour),
		City:      "Phoenix",
		State:     "AZ",
		IsPrivate: private,
		// Admin-submitted, i.e. the INGEST shape the trust gate admits: the `ph`
		// CLI's phk_ token resolves to an admin user, and the markdown import and
		// entity-request fulfiller both pass true. Self-serve submissions are covered
		// separately by TestSelfServeSubmissionDoesNotEnqueue.
		SubmitterIsAdmin: true,
		Venues: []contracts.CreateShowVenue{
			{Name: fmt.Sprintf("Venue %d", unique), City: "Phoenix", State: "AZ"},
		},
		Artists: []contracts.CreateShowArtist{
			{Name: fmt.Sprintf("Artist %d", unique)},
		},
	})
	s.Require().NoError(err)
	s.Require().NotNil(resp)
	return resp
}

// TestIngestCreateEnqueuesOnePendingJob: the funnel every ingest write goes
// through leaves exactly one pending job for the newly visible show. This is the
// hook that PSY-1894 was missing entirely.
func (s *ShowNotifyEnqueueSuite) TestIngestCreateEnqueuesOnePendingJob() {
	resp := s.createShow("Ingested show", false)

	var item catalogm.ShowNotifyQueueItem
	s.Require().NoError(s.db.Where("show_id = ?", resp.ID).First(&item).Error)
	s.Equal(catalogm.ShowNotifyStatusPending, item.Status)
	s.Equal(0, item.Attempts)
	s.Equal(int64(1), s.queueCount())
}

// TestPrivateShowIsNotEnqueued: a private show is not publicly visible, so
// announcing it would leak the submitter's own list to the scene.
func (s *ShowNotifyEnqueueSuite) TestPrivateShowIsNotEnqueued() {
	resp := s.createShow("Private show", true)

	var show catalogm.Show
	s.Require().NoError(s.db.First(&show, resp.ID).Error)
	s.Require().Equal(catalogm.ShowStatusPrivate, show.Status)
	s.Equal(int64(0), s.queueCount())
}

// loadShow reads the row back so tests can hand the real model to the enqueue.
func (s *ShowNotifyEnqueueSuite) loadShow(showID uint) *catalogm.Show {
	var show catalogm.Show
	s.Require().NoError(s.db.First(&show, showID).Error)
	return &show
}

// TestNonApprovedStatusesAreNotEnqueued exercises the status gate directly, so
// the discovery path's "force pending for a suspected duplicate" and the admin
// moderation states are covered without standing up those services.
func (s *ShowNotifyEnqueueSuite) TestNonApprovedStatusesAreNotEnqueued() {
	resp := s.createShow("Status gate", false)
	s.Require().NoError(s.db.Exec("DELETE FROM show_notify_queue").Error)
	show := s.loadShow(resp.ID)

	for _, st := range []catalogm.ShowStatus{
		catalogm.ShowStatusPending,
		catalogm.ShowStatusRejected,
		catalogm.ShowStatusPrivate,
	} {
		show.Status = st
		EnqueueShowNotify(s.db, show, ShowNotifyIngest)
		s.Equal(int64(0), s.queueCount(), "status %q must not enqueue", st)
	}

	show.Status = catalogm.ShowStatusApproved
	EnqueueShowNotify(s.db, show, ShowNotifyIngest)
	s.Equal(int64(1), s.queueCount())
}

// TestCancelledShowIsNotEnqueued: discovery scrapes cancellation off venue
// calendars and still writes the row `approved`, so without this gate an
// automated import would email "New show" for an event the venue has called off.
func (s *ShowNotifyEnqueueSuite) TestCancelledShowIsNotEnqueued() {
	resp := s.createShow("Cancelled at import", false)
	s.Require().NoError(s.db.Exec("DELETE FROM show_notify_queue").Error)

	show := s.loadShow(resp.ID)
	show.IsCancelled = true
	EnqueueShowNotify(s.db, show, ShowNotifyIngest)
	s.Equal(int64(0), s.queueCount())

	// Sold out is deliberately NOT a blocker: it is real information a follower
	// still wants, and the admin approve path has always notified for it.
	show.IsCancelled = false
	show.IsSoldOut = true
	EnqueueShowNotify(s.db, show, ShowNotifyIngest)
	s.Equal(int64(1), s.queueCount())
}

// TestPastShowIsNotEnqueued is the ARCHIVAL-IMPORT guard, and it is the same
// hazard as a rollout backfill arriving through a different door. CreateShow does
// no past-date validation, so ingesting a venue's back catalogue through
// POST /shows (the `ph` CLI) or the admin markdown import would otherwise fan
// years-old listings out as new announcements.
func (s *ShowNotifyEnqueueSuite) TestPastShowIsNotEnqueued() {
	unique := time.Now().UnixNano()
	resp, err := s.svc.CreateShow(&contracts.CreateShowRequest{
		Title:            "Archival 2019 show",
		EventDate:        time.Now().Add(-6 * 365 * 24 * time.Hour),
		City:             "Phoenix",
		State:            "AZ",
		SubmitterIsAdmin: true,
		Venues:           []contracts.CreateShowVenue{{Name: fmt.Sprintf("Venue %d", unique), City: "Phoenix", State: "AZ"}},
		Artists:          []contracts.CreateShowArtist{{Name: fmt.Sprintf("Artist %d", unique)}},
	})
	s.Require().NoError(err)

	var show catalogm.Show
	s.Require().NoError(s.db.First(&show, resp.ID).Error)
	s.Require().Equal(catalogm.ShowStatusApproved, show.Status, "the show itself still imports normally")
	s.Equal(int64(0), s.queueCount(), "a show that already happened must not be announced")
}

// TestSelfServeSubmissionDoesNotEnqueue is the abuse gate. POST /shows is a public
// endpoint any email-verified user can call, and determineShowStatus makes every
// non-private submission `approved`. Without this split, one user could email every
// matching subscriber and scene follower with an unmoderated title. Untrusted
// submissions keep their pre-ticket behaviour: visible on the site, notifying
// nobody until a human approves them.
func (s *ShowNotifyEnqueueSuite) TestSelfServeSubmissionDoesNotEnqueue() {
	unique := time.Now().UnixNano()
	resp, err := s.svc.CreateShow(&contracts.CreateShowRequest{
		Title:     "Submitted by a regular user",
		EventDate: time.Now().Add(30 * 24 * time.Hour),
		City:      "Phoenix",
		State:     "AZ",
		// The whole point: a non-admin submitter through the very same funnel.
		SubmitterIsAdmin: false,
		Venues:           []contracts.CreateShowVenue{{Name: fmt.Sprintf("Venue %d", unique), City: "Phoenix", State: "AZ"}},
		Artists:          []contracts.CreateShowArtist{{Name: fmt.Sprintf("Artist %d", unique)}},
	})
	s.Require().NoError(err)

	var show catalogm.Show
	s.Require().NoError(s.db.First(&show, resp.ID).Error)
	s.Require().Equal(catalogm.ShowStatusApproved, show.Status,
		"the show is still created and publicly visible — only the notification is withheld")
	s.Equal(int64(0), s.queueCount())
}

// TestUntrustedTrustValueNeverEnqueues pins the gate itself, so a future call site
// passing the zero value cannot silently opt into mass email.
func (s *ShowNotifyEnqueueSuite) TestUntrustedTrustValueNeverEnqueues() {
	resp := s.createShow("Trust gate", false)
	s.Require().NoError(s.db.Exec("DELETE FROM show_notify_queue").Error)

	EnqueueShowNotify(s.db, s.loadShow(resp.ID), ShowNotifyUntrusted)
	s.Equal(int64(0), s.queueCount())

	var zeroValue ShowNotifyTrust
	s.Equal(ShowNotifyUntrusted, zeroValue, "the zero value must be the safe one")

	EnqueueShowNotify(s.db, s.loadShow(resp.ID), ShowNotifyIngest)
	s.Equal(int64(1), s.queueCount())
}

// TestStaleShowIsRefused is the structural half of the no-backfill guarantee. The
// empty table keeps the ROLLOUT silent; this keeps a future call site — one line
// added to PublishShow, to an update path, or to a backfill CLI — from
// mass-enqueuing the existing catalogue.
func (s *ShowNotifyEnqueueSuite) TestStaleShowIsRefused() {
	resp := s.createShow("Old enough to refuse", false)
	s.Require().NoError(s.db.Exec("DELETE FROM show_notify_queue").Error)

	show := s.loadShow(resp.ID)
	show.CreatedAt = time.Now().Add(-2 * maxEnqueueShowAge)
	EnqueueShowNotify(s.db, show, ShowNotifyIngest)
	s.Equal(int64(0), s.queueCount(), "a show created long ago is not a show becoming visible")

	// The boundary is generous enough that a real create transaction is never near it.
	show.CreatedAt = time.Now().Add(-time.Minute)
	EnqueueShowNotify(s.db, show, ShowNotifyIngest)
	s.Equal(int64(1), s.queueCount())
}

// TestReEnqueueIsANoOp is the DEDUP acceptance criterion. Re-ingesting the same
// show (a venue calendar re-scrape, a repeated `ph batch`) or re-running the
// enqueue for any other reason must not produce a second job — the whole-table
// UNIQUE plus ON CONFLICT DO NOTHING makes that a database guarantee rather than
// a caller convention.
func (s *ShowNotifyEnqueueSuite) TestReEnqueueIsANoOp() {
	resp := s.createShow("Re-ingested show", false)
	s.Require().Equal(int64(1), s.queueCount())

	for i := 0; i < 3; i++ {
		EnqueueShowNotify(s.db, s.loadShow(resp.ID), ShowNotifyIngest)
	}
	s.Equal(int64(1), s.queueCount())
}

// TestTerminalRowBlocksReEnqueue: unlike the image-enrich outbox, a FINISHED job
// must keep blocking. The terminal row is the durable "this show has already been
// considered" record; if a done row could be superseded, an update or a
// republish would re-notify.
func (s *ShowNotifyEnqueueSuite) TestTerminalRowBlocksReEnqueue() {
	resp := s.createShow("Already notified", false)
	s.Require().NoError(s.db.Model(&catalogm.ShowNotifyQueueItem{}).
		Where("show_id = ?", resp.ID).
		Update("status", catalogm.ShowNotifyStatusDone).Error)

	EnqueueShowNotify(s.db, s.loadShow(resp.ID), ShowNotifyIngest)

	var items []catalogm.ShowNotifyQueueItem
	s.Require().NoError(s.db.Where("show_id = ?", resp.ID).Find(&items).Error)
	s.Require().Len(items, 1)
	s.Equal(catalogm.ShowNotifyStatusDone, items[0].Status, "terminal row must not be re-opened")
}

// TestDisabledFlagSuppressesEnqueue: the kill switch must stop rows being WRITTEN,
// not just drained. Gating only the drain would accumulate a backlog that fans out
// in one burst when the flag flips back on.
func (s *ShowNotifyEnqueueSuite) TestDisabledFlagSuppressesEnqueue() {
	s.T().Setenv(catalogm.ShowNotifyOutboxDisableFlag, "1")

	resp := s.createShow("Flagged off", false)

	var n int64
	s.Require().NoError(s.db.Model(&catalogm.ShowNotifyQueueItem{}).Where("show_id = ?", resp.ID).Count(&n).Error)
	s.Equal(int64(0), n)
}

// TestEnqueueRidesTheShowTransaction: the job and the show it describes commit
// together, so a poller can never claim a job whose show (or whose artists and
// venues) is not yet visible.
func (s *ShowNotifyEnqueueSuite) TestEnqueueRidesTheShowTransaction() {
	sentinel := errors.New("rollback")
	var showID uint

	err := s.db.Transaction(func(tx *gorm.DB) error {
		show := &catalogm.Show{
			Title:     "Rolled back",
			EventDate: time.Now().Add(24 * time.Hour),
			Status:    catalogm.ShowStatusApproved,
		}
		if err := tx.Create(show).Error; err != nil {
			return err
		}
		showID = show.ID
		EnqueueShowNotify(tx, show, ShowNotifyIngest)
		return sentinel
	})
	s.Require().ErrorIs(err, sentinel)

	var shows int64
	s.Require().NoError(s.db.Model(&catalogm.Show{}).Where("id = ?", showID).Count(&shows).Error)
	s.Equal(int64(0), shows)
	s.Equal(int64(0), s.queueCount(), "job must roll back with the show it describes")
}

// TestEnqueueFailureDoesNotPoisonTheShowWrite is the SAVEPOINT crux. In Postgres a
// failed statement aborts the whole transaction, so a naive failing insert inside
// the show-create tx would turn the later COMMIT into a silent ROLLBACK — the
// caller would get a show response, a nil error, and no row in the database.
//
// The failure is provoked with a show_id that violates the foreign key, which is
// exactly the shape of the realistic failure (a show deleted concurrently).
func (s *ShowNotifyEnqueueSuite) TestEnqueueFailureDoesNotPoisonTheShowWrite() {
	var showID uint

	err := s.db.Transaction(func(tx *gorm.DB) error {
		show := &catalogm.Show{
			Title:     "Survives a bad enqueue",
			EventDate: time.Now().Add(24 * time.Hour),
			Status:    catalogm.ShowStatusApproved,
		}
		if err := tx.Create(show).Error; err != nil {
			return err
		}
		showID = show.ID

		// No such show — the FK rejects it. The savepoint must absorb that.
		EnqueueShowNotify(tx, &catalogm.Show{
			ID:        999999999,
			Title:     "Not a real show",
			EventDate: time.Now().Add(24 * time.Hour),
			CreatedAt: time.Now(),
			Status:    catalogm.ShowStatusApproved,
		}, ShowNotifyIngest)
		return nil
	})
	s.Require().NoError(err, "outer transaction must still commit")

	var got catalogm.Show
	s.Require().NoError(s.db.First(&got, showID).Error, "the show must be durably committed")
	s.Equal("Survives a bad enqueue", got.Title)
	s.Equal(int64(0), s.queueCount())
}
