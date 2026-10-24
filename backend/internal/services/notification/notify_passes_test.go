package notification

import (
	"errors"
	"testing"

	"github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	notificationm "psychic-homily-backend/internal/models/notification"
	"psychic-homily-backend/internal/services/contracts"
)

// =============================================================================
// UNIT TESTS (no database)
// =============================================================================

// recordingPass returns a pass that appends its name to ran and then returns err.
func recordingPass(name string, ran *[]string, err error) notifyPass {
	return notifyPass{name: name, run: func() error {
		*ran = append(*ran, name)
		return err
	}}
}

// panicInsideNotifyPass is a named frame, so a test can tell the stack captured
// inside the panicking pass from one captured at the re-raise.
//
//go:noinline
func panicInsideNotifyPass(v any) {
	panic(v)
}

func TestRunNotifyPasses_AllSucceed(t *testing.T) {
	var ran []string
	err := runNotifyPasses(7, []notifyPass{
		recordingPass("a", &ran, nil),
		recordingPass("b", &ran, nil),
		recordingPass("c", &ran, nil),
	})

	assert.NoError(t, err)
	assert.Equal(t, []string{"a", "b", "c"}, ran)
}

func TestRunNotifyPasses_ErrorDoesNotStopLaterPasses(t *testing.T) {
	firstErr := errors.New("first broke")
	thirdErr := errors.New("third broke")
	var ran []string
	err := runNotifyPasses(7, []notifyPass{
		recordingPass("first", &ran, firstErr),
		recordingPass("second", &ran, nil),
		recordingPass("third", &ran, thirdErr),
		recordingPass("fourth", &ran, nil),
	})

	assert.Equal(t, []string{"first", "second", "third", "fourth"}, ran)
	require.Error(t, err)
	assert.ErrorIs(t, err, firstErr)
	assert.ErrorIs(t, err, thirdErr)
	assert.Equal(t, "first pass: first broke\nthird pass: third broke", err.Error())
}

func TestRunNotifyPasses_PanicRunsRemainingPassesThenReraises(t *testing.T) {
	cause := errors.New("nil map")
	laterErr := errors.New("later broke")
	var ran []string
	passes := []notifyPass{
		recordingPass("before", &ran, nil),
		{name: "boom", run: func() error {
			ran = append(ran, "boom")
			panicInsideNotifyPass(cause)
			return nil
		}},
		recordingPass("after", &ran, laterErr),
		{name: "second-boom", run: func() error {
			ran = append(ran, "second-boom")
			panic("not the first")
		}},
		recordingPass("last", &ran, nil),
	}

	var recovered any
	func() {
		defer func() { recovered = recover() }()
		_ = runNotifyPasses(7, passes)
	}()

	assert.Equal(t, []string{"before", "boom", "after", "second-boom", "last"}, ran)

	p, ok := recovered.(*notifyPassPanic)
	require.True(t, ok, "re-raised value should be a *notifyPassPanic, got %T", recovered)
	assert.Equal(t, "boom", p.pass, "the FIRST panic is the one re-raised")
	assert.Equal(t, uint(7), p.showID)
	assert.ErrorIs(t, p, cause)
	assert.Contains(t, p.Error(), "boom pass panicked for show 7: nil map")
	// The stack is the one captured where the pass panicked, not the re-raise site.
	assert.Contains(t, string(p.stack), "panicInsideNotifyPass")
}

func TestNotifyPassPanic_UnwrapIsNilForANonErrorValue(t *testing.T) {
	p := &notifyPassPanic{pass: "x", value: "a string"}
	assert.NoError(t, p.Unwrap())
}

// =============================================================================
// INTEGRATION TESTS (NotificationFilterSuite, real Postgres)
// =============================================================================

// breakFilterQuery makes the filter pass's matching query fail by renaming a
// column it selects, and returns the function that renames it back. A rename
// keeps the column's constraints and indexes, so the restore is exact. The
// restore also runs at test cleanup and is a no-op once it has run.
func (s *NotificationFilterSuite) breakFilterQuery() (restore func()) {
	s.Require().NoError(s.db.Exec(
		`ALTER TABLE notification_filters RENAME COLUMN source TO source_unavailable`).Error)
	restored := false
	restore = func() {
		if restored {
			return
		}
		restored = true
		s.Require().NoError(s.db.Exec(
			`ALTER TABLE notification_filters RENAME COLUMN source_unavailable TO source`).Error)
	}
	s.T().Cleanup(restore)
	return restore
}

// filterLogRows counts the filter pass's notification rows for a user and show.
func (s *NotificationFilterSuite) filterLogRows(userID, showID uint) int64 {
	var n int64
	s.Require().NoError(s.db.Model(&notificationm.NotificationLog{}).
		Where("user_id = ? AND entity_type = ? AND entity_id = ? AND filter_id IS NOT NULL",
			userID, notificationm.NotificationEntityShow, showID).
		Count(&n).Error)
	return n
}

// requireFilterPassError asserts MatchAndNotify reported the filter pass failing.
func (s *NotificationFilterSuite) requireFilterPassError(err error) {
	s.Require().Error(err)
	s.Contains(err.Error(), notifyPassFilter+" pass: failed to find matching filters")
}

func (s *NotificationFilterSuite) TestMatchAndNotify_FilterFailureStillAlertsArtistFollowers() {
	userID := s.createTestUser()
	artistID := s.createTestArtist("Oneida")
	s.followArtist(userID, artistID)
	venueID := s.createTestVenue("Valley Bar")
	showID := s.createTestShow("Oneida at Valley Bar", []uint{artistID}, []uint{venueID})

	s.breakFilterQuery()
	s.requireFilterPassError(s.svc.MatchAndNotify(s.loadShow(showID)))

	s.Equal(int64(1), s.inAppAlerts(userID, showID))
}

func (s *NotificationFilterSuite) TestMatchAndNotify_FilterFailureStillAccruesVenueFollows() {
	userID := s.createTestUser()
	venueID := s.createTestVenue("Valley Bar")
	s.followVenueWithAlerts(userID, venueID, `{}`)
	day := venueLocalToday()
	showID := s.createTestShow("Show at Valley Bar", nil, []uint{venueID})

	s.breakFilterQuery()
	s.requireFilterPassError(s.svc.MatchAndNotify(s.loadShow(showID)))

	s.Equal(int64(1), s.batchMembers(venueID, day))
}

func (s *NotificationFilterSuite) TestMatchAndNotify_FilterFailureStillNotifiesSceneFollowers() {
	userID := s.createTestUser()
	s.seedSceneFollow(userID, "")
	artistID := s.createTestArtist("Some Band")
	venueID := s.createTestVenue("The Rebel Lounge")
	showID := s.createTestShow("Scene Show", []uint{artistID}, []uint{venueID})

	s.breakFilterQuery()
	s.requireFilterPassError(s.svc.MatchAndNotify(s.loadShow(showID)))

	s.Equal(int64(1), s.sceneLogCount(userID, showID))
}

// A retry after a failed filter pass runs every pass again. The artist alert
// the first run delivered is not repeated, and the filter pass defers to it.
func (s *NotificationFilterSuite) TestMatchAndNotify_RetryAfterFilterFailureDoesNotRenotify() {
	userID := s.createTestUser()
	artistID := s.createTestArtist("Oneida")
	s.followArtist(userID, artistID)
	_, err := s.svc.CreateFilter(userID, contracts.CreateFilterInput{
		Name:        "Oneida anywhere",
		ArtistIDs:   []int64{int64(artistID)},
		NotifyInApp: true,
	})
	s.Require().NoError(err)
	venueID := s.createTestVenue("Valley Bar")
	showID := s.createTestShow("Oneida at Valley Bar", []uint{artistID}, []uint{venueID})

	restore := s.breakFilterQuery()
	s.requireFilterPassError(s.svc.MatchAndNotify(s.loadShow(showID)))
	s.Equal(int64(1), s.inAppAlerts(userID, showID))

	restore()
	s.Require().NoError(s.svc.MatchAndNotify(s.loadShow(showID)))

	s.Equal(int64(1), s.inAppAlerts(userID, showID))
	s.Equal(int64(0), s.filterLogRows(userID, showID), "the filter pass defers to the artist alert")
}

// subscribeToArtist gives the user an in-app filter matching any show with the
// artist on the bill.
func (s *NotificationFilterSuite) subscribeToArtist(userID, artistID uint) {
	_, err := s.svc.CreateFilter(userID, contracts.CreateFilterInput{
		Name:        "Artist filter",
		ArtistIDs:   []int64{int64(artistID)},
		NotifyInApp: true,
	})
	s.Require().NoError(err)
}

// The filter query fails, the venue batch flushes and tells the user about the
// show, then the filter retry succeeds: the user is told once, by the venue
// alert.
func (s *NotificationFilterSuite) TestMatchAndNotify_FilterRetryAfterVenueFlushDoesNotRenotify() {
	userID := s.createTestUser()
	artistID := s.createTestArtist("Oneida")
	venueID := s.createTestVenue("Valley Bar")
	s.followVenueWithAlerts(userID, venueID, `{}`)
	s.subscribeToArtist(userID, artistID)
	day := venueLocalToday()
	showID := s.createTestShow("Oneida at Valley Bar", []uint{artistID}, []uint{venueID})

	restore := s.breakFilterQuery()
	s.requireFilterPassError(s.svc.MatchAndNotify(s.loadShow(showID)))
	s.Equal(1, s.flushNow())
	s.Require().Equal(int64(1), s.venueInAppAlerts(userID, venueID, day))

	restore()
	s.Require().NoError(s.svc.MatchAndNotify(s.loadShow(showID)))

	s.Equal(int64(1), s.venueInAppAlerts(userID, venueID, day))
	s.Equal(int64(0), s.filterLogRows(userID, showID), "the venue alert already listed the show")
}

// The per-user re-check inside the filter pass applies the same venue-alert
// rule as the matching query, for a match that query returned before the venue
// alert existed.
func (s *NotificationFilterSuite) TestProcessUserMatches_SkipsAUserTheVenueAlertAlreadyTold() {
	userID := s.createTestUser()
	artistID := s.createTestArtist("Oneida")
	venueID := s.createTestVenue("Valley Bar")
	s.followVenueWithAlerts(userID, venueID, `{}`)
	s.subscribeToArtist(userID, artistID)
	showID := s.createTestShow("Oneida at Valley Bar", []uint{artistID}, []uint{venueID})
	show := s.loadShow(showID)

	matches, err := s.svc.findMatchingFilters(showID, pq.Int64Array{int64(artistID)}, pq.Int64Array{int64(venueID)}, nil, nil, nil, nil)
	s.Require().NoError(err)
	s.Require().Len(matches, 1)

	s.Require().NoError(s.svc.accrueVenueShowAlerts(show, pq.Int64Array{int64(venueID)}))
	s.Require().Equal(1, s.flushNow())

	s.Require().NoError(s.svc.processUserMatches(userID, show, matches))
	s.Equal(int64(0), s.filterLogRows(userID, showID))
}

// Only the in-app lane counts as the venue alert listing a show: an email-only
// venue alert does not stop the filter retry.
func (s *NotificationFilterSuite) TestMatchAndNotify_EmailOnlyVenueAlertDoesNotStopTheFilterRetry() {
	s.withCapturedEmail()
	userID := s.createTestUser()
	artistID := s.createTestArtist("Oneida")
	venueID := s.createTestVenue("Valley Bar")
	s.followVenueWithAlerts(userID, venueID, `{"alerts":{"shows":{"in_app":false,"email":true}}}`)
	s.subscribeToArtist(userID, artistID)
	day := venueLocalToday()
	showID := s.createTestShow("Oneida at Valley Bar", []uint{artistID}, []uint{venueID})

	restore := s.breakFilterQuery()
	s.requireFilterPassError(s.svc.MatchAndNotify(s.loadShow(showID)))
	s.Equal(1, s.flushNow())
	s.Require().Equal(int64(1), s.venueAlertRows(userID, venueID, day, notificationm.NotificationChannelEmail))

	restore()
	s.Require().NoError(s.svc.MatchAndNotify(s.loadShow(showID)))

	s.Equal(int64(1), s.filterLogRows(userID, showID))
}

// breakArtistFollowerQuery makes the artist-follow pass's followers query fail
// by renaming a column only it reads, and returns the function that restores
// it. The restore also runs at test cleanup and is a no-op once it has run.
func (s *NotificationFilterSuite) breakArtistFollowerQuery() (restore func()) {
	s.Require().NoError(s.db.Exec(
		`ALTER TABLE show_artists RENAME COLUMN position TO position_unavailable`).Error)
	restored := false
	restore = func() {
		if restored {
			return
		}
		restored = true
		s.Require().NoError(s.db.Exec(
			`ALTER TABLE show_artists RENAME COLUMN position_unavailable TO position`).Error)
	}
	s.T().Cleanup(restore)
	return restore
}

// A follow pass's error is returned, so the outbox retries, and the retry
// delivers the alert the failed run missed, once.
func (s *NotificationFilterSuite) TestMatchAndNotify_ArtistFollowFailureIsReturnedAndRetryDeliversOnce() {
	userID := s.createTestUser()
	artistID := s.createTestArtist("Oneida")
	s.followArtist(userID, artistID)
	venueID := s.createTestVenue("Valley Bar")
	showID := s.createTestShow("Oneida at Valley Bar", []uint{artistID}, []uint{venueID})

	restore := s.breakArtistFollowerQuery()
	err := s.svc.MatchAndNotify(s.loadShow(showID))
	s.Require().Error(err)
	s.Contains(err.Error(), notifyPassArtistFollow+" pass: artist followers query")
	s.NotContains(err.Error(), notifyPassFilter+" pass")
	s.Equal(int64(0), s.inAppAlerts(userID, showID))

	restore()
	s.Require().NoError(s.svc.MatchAndNotify(s.loadShow(showID)))
	s.Equal(int64(1), s.inAppAlerts(userID, showID))

	s.Require().NoError(s.svc.MatchAndNotify(s.loadShow(showID)))
	s.Equal(int64(1), s.inAppAlerts(userID, showID))
}

// A show that joins a venue-day batch after the user's alert for that day was
// delivered is not one that alert announced, so a filter retry still tells the
// user, and the venue re-flush adds no second notification.
func (s *NotificationFilterSuite) TestMatchAndNotify_ShowJoiningADeliveredVenueAlertStillGetsTheFilterRetry() {
	userID := s.createTestUser()
	artistID := s.createTestArtist("Oneida")
	otherArtistID := s.createTestArtist("Someone Else")
	venueID := s.createTestVenue("Valley Bar")
	s.followVenueWithAlerts(userID, venueID, `{}`)
	s.subscribeToArtist(userID, artistID)
	day := venueLocalToday()

	earlierShowID := s.createTestShow("Earlier show at Valley Bar", []uint{otherArtistID}, []uint{venueID})
	s.announce(earlierShowID)
	s.Require().Equal(1, s.flushNow())
	s.Require().Equal(int64(1), s.venueInAppAlerts(userID, venueID, day))

	showID := s.createTestShow("Oneida at Valley Bar", []uint{artistID}, []uint{venueID})
	restore := s.breakFilterQuery()
	s.requireFilterPassError(s.svc.MatchAndNotify(s.loadShow(showID)))
	s.Require().Equal(int64(2), s.batchMembers(venueID, day))

	restore()
	s.Require().NoError(s.svc.MatchAndNotify(s.loadShow(showID)))
	s.Equal(int64(1), s.filterLogRows(userID, showID))

	s.flushNow()
	s.Equal(int64(1), s.venueInAppAlerts(userID, venueID, day))
	s.Equal(int64(1), s.filterLogRows(userID, showID))
}

// A failed log insert in the filter pass is returned, so the outbox retries,
// and the retry delivers the missing filter notification once.
func (s *NotificationFilterSuite) TestMatchAndNotify_FilterLogInsertFailureIsReturnedAndRetryDeliversOnce() {
	userID := s.createTestUser()
	artistID := s.createTestArtist("Oneida")
	s.subscribeToArtist(userID, artistID)
	venueID := s.createTestVenue("Valley Bar")
	showID := s.createTestShow("Oneida at Valley Bar", []uint{artistID}, []uint{venueID})

	// Rejects only filter rows; every other writer stores a NULL filter_id.
	s.Require().NoError(s.db.Exec(
		`ALTER TABLE notification_log ADD CONSTRAINT ck_test_no_filter_rows CHECK (filter_id IS NULL) NOT VALID`).Error)
	dropped := false
	drop := func() {
		if dropped {
			return
		}
		dropped = true
		s.Require().NoError(s.db.Exec(
			`ALTER TABLE notification_log DROP CONSTRAINT ck_test_no_filter_rows`).Error)
	}
	s.T().Cleanup(drop)

	err := s.svc.MatchAndNotify(s.loadShow(showID))
	s.Require().Error(err)
	s.Contains(err.Error(), notifyPassFilter+" pass: log insert for user")
	s.Equal(int64(0), s.filterLogRows(userID, showID))

	drop()
	s.Require().NoError(s.svc.MatchAndNotify(s.loadShow(showID)))
	s.Equal(int64(1), s.filterLogRows(userID, showID))

	s.Require().NoError(s.svc.MatchAndNotify(s.loadShow(showID)))
	s.Equal(int64(1), s.filterLogRows(userID, showID))
}
