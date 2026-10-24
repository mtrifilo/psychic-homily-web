package notification

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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
			panic(cause)
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
	assert.Contains(t, string(p.stack), "TestRunNotifyPasses_PanicRunsRemainingPassesThenReraises.func")
}

func TestNotifyPassPanic_UnwrapIsNilForANonErrorValue(t *testing.T) {
	p := &notifyPassPanic{pass: "x", value: "a string"}
	assert.NoError(t, p.Unwrap())
}

// =============================================================================
// INTEGRATION TESTS (NotificationFilterSuite, real Postgres)
// =============================================================================

// breakFilterQuery makes the filter pass's matching query fail for the rest of
// the test by renaming a column it selects, then renames it back. A rename
// keeps the column's constraints and indexes, so the restore is exact.
func (s *NotificationFilterSuite) breakFilterQuery() {
	s.Require().NoError(s.db.Exec(
		`ALTER TABLE notification_filters RENAME COLUMN source TO source_unavailable`).Error)
	s.T().Cleanup(s.restoreFilterQuery)
}

// restoreFilterQuery undoes breakFilterQuery. Safe to call more than once.
func (s *NotificationFilterSuite) restoreFilterQuery() {
	var renamed int64
	s.Require().NoError(s.db.Raw(`
		SELECT count(*) FROM information_schema.columns
		WHERE table_name = 'notification_filters' AND column_name = 'source_unavailable'`).
		Scan(&renamed).Error)
	if renamed == 0 {
		return
	}
	s.Require().NoError(s.db.Exec(
		`ALTER TABLE notification_filters RENAME COLUMN source_unavailable TO source`).Error)
}

// requireFilterPassError asserts MatchAndNotify reported the filter pass, and
// only the filter pass, as failing.
func (s *NotificationFilterSuite) requireFilterPassError(err error) {
	s.Require().Error(err)
	s.Contains(err.Error(), notifyPassFilter+" pass: failed to find matching filters")
	for _, other := range []string{notifyPassArtistFollow, notifyPassVenueFollow, notifyPassSceneFollow} {
		s.NotContains(err.Error(), other+" pass")
	}
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

	s.breakFilterQuery()
	s.requireFilterPassError(s.svc.MatchAndNotify(s.loadShow(showID)))
	s.Equal(int64(1), s.inAppAlerts(userID, showID))

	s.restoreFilterQuery()
	s.Require().NoError(s.svc.MatchAndNotify(s.loadShow(showID)))

	s.Equal(int64(1), s.inAppAlerts(userID, showID))
	s.Equal(int64(0), s.sceneLogCount(userID, showID), "the filter pass defers to the artist alert")
}
