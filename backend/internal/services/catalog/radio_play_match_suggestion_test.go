package catalog

import (
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	"gorm.io/gorm"

	authm "psychic-homily-backend/internal/models/auth"
	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/testutil"
)

// RadioPlayMatchSuggestionSuite covers create / accept / reject / resubmit
// rules for PSY-1494 against real Postgres.
type RadioPlayMatchSuggestionSuite struct {
	suite.Suite
	testDB  *testutil.TestDatabase
	db      *gorm.DB
	radio   *RadioService
	service *RadioPlayMatchSuggestionService
}

func TestRadioPlayMatchSuggestionSuite(t *testing.T) {
	suite.Run(t, new(RadioPlayMatchSuggestionSuite))
}

func (s *RadioPlayMatchSuggestionSuite) SetupSuite() {
	s.testDB = testutil.SetupTestPostgres(s.T())
	s.db = s.testDB.DB
	s.radio = &RadioService{db: s.db}
	s.service = NewRadioPlayMatchSuggestionService(s.db, s.radio)
}

func (s *RadioPlayMatchSuggestionSuite) TearDownSuite() {
	s.testDB.Cleanup()
}

func (s *RadioPlayMatchSuggestionSuite) SetupTest() {
	s.cleanup()
}

func (s *RadioPlayMatchSuggestionSuite) TearDownTest() {
	s.cleanup()
}

func (s *RadioPlayMatchSuggestionSuite) cleanup() {
	sqlDB, err := s.db.DB()
	s.Require().NoError(err)
	_, _ = sqlDB.Exec("DELETE FROM radio_play_match_suggestions")
	_, _ = sqlDB.Exec("DELETE FROM radio_plays")
	_, _ = sqlDB.Exec("DELETE FROM radio_episodes")
	_, _ = sqlDB.Exec("DELETE FROM radio_shows")
	_, _ = sqlDB.Exec("DELETE FROM radio_stations")
	_, _ = sqlDB.Exec("DELETE FROM artists")
	_, _ = sqlDB.Exec("DELETE FROM users")
}

func (s *RadioPlayMatchSuggestionSuite) createUser(username string) *authm.User {
	u := &authm.User{Username: &username}
	s.Require().NoError(s.db.Create(u).Error)
	return u
}

func (s *RadioPlayMatchSuggestionSuite) createArtist(name, slug string) *catalogm.Artist {
	a := &catalogm.Artist{Name: name, Slug: &slug}
	s.Require().NoError(s.db.Create(a).Error)
	return a
}

func (s *RadioPlayMatchSuggestionSuite) createPlay(artistName string, matchState string, artistID *uint) *catalogm.RadioPlay {
	station := &catalogm.RadioStation{
		Name:          "KEXP-" + artistName + matchState,
		Slug:          "kexp-" + artistName + "-" + matchState + "-" + time.Now().Format("150405.000"),
		BroadcastType: "internet",
	}
	s.Require().NoError(s.db.Create(station).Error)
	show := &catalogm.RadioShow{
		StationID: station.ID,
		Name:      "Show",
		Slug:      "show-" + time.Now().Format("150405.000000"),
	}
	s.Require().NoError(s.db.Create(show).Error)
	ep := &catalogm.RadioEpisode{
		ShowID:  show.ID,
		AirDate: "2026-07-01",
	}
	s.Require().NoError(s.db.Create(ep).Error)

	var existing int64
	s.Require().NoError(s.db.Model(&catalogm.RadioPlay{}).Where("episode_id = ?", ep.ID).Count(&existing).Error)
	play := &catalogm.RadioPlay{
		EpisodeID:  ep.ID,
		ArtistName: artistName,
		ArtistID:   artistID,
		MatchState: matchState,
		Position:   int(existing),
	}
	s.Require().NoError(s.db.Create(play).Error)
	return play
}

func (s *RadioPlayMatchSuggestionSuite) TestCreate_HappyPathDoesNotMutatePlay() {
	user := s.createUser("suggester")
	artist := s.createArtist("Boy Harsher", "boy-harsher")
	play := s.createPlay("Boy Harsher", catalogm.RadioPlayMatchStateUnmatched, nil)

	entry, err := s.service.CreateSuggestion(play.ID, user.ID, &contracts.CreateRadioPlayMatchSuggestionRequest{
		ArtistID: artist.ID,
		Note:     matchNote("sounds like them"),
	})
	s.Require().NoError(err)
	s.Equal(catalogm.RadioPlayMatchSuggestionStatusPending, entry.Status)
	s.Equal(play.ID, entry.PlayID)
	s.Equal(artist.ID, entry.SuggestedArtistID)
	s.Equal(user.ID, entry.SubmittedBy)

	var updated catalogm.RadioPlay
	s.Require().NoError(s.db.First(&updated, play.ID).Error)
	s.Nil(updated.ArtistID)
	s.Equal(catalogm.RadioPlayMatchStateUnmatched, updated.MatchState)
}

func (s *RadioPlayMatchSuggestionSuite) TestCreate_DuplicatePendingRejected() {
	user := s.createUser("suggester")
	artist := s.createArtist("Boy Harsher", "boy-harsher-dup")
	play := s.createPlay("Boy Harsher", catalogm.RadioPlayMatchStateUnmatched, nil)

	_, err := s.service.CreateSuggestion(play.ID, user.ID, &contracts.CreateRadioPlayMatchSuggestionRequest{ArtistID: artist.ID})
	s.Require().NoError(err)

	_, err = s.service.CreateSuggestion(play.ID, user.ID, &contracts.CreateRadioPlayMatchSuggestionRequest{ArtistID: artist.ID})
	s.ErrorIs(err, contracts.ErrRadioPlayMatchSuggestionDuplicatePending)
}

func (s *RadioPlayMatchSuggestionSuite) TestCreate_SuggestableStates() {
	user := s.createUser("suggester")
	artist := s.createArtist("Target", "target-states")

	for _, state := range []string{
		catalogm.RadioPlayMatchStateUnmatched,
		catalogm.RadioPlayMatchStateAmbiguous,
		catalogm.RadioPlayMatchStateNoMatch,
	} {
		play := s.createPlay("Name-"+state, state, nil)
		entry, err := s.service.CreateSuggestion(play.ID, user.ID, &contracts.CreateRadioPlayMatchSuggestionRequest{ArtistID: artist.ID})
		s.Require().NoError(err, "state=%s", state)
		s.Equal(state, entry.PlayMatchState)
	}
}

func (s *RadioPlayMatchSuggestionSuite) TestCreate_RejectsMatchedPlay() {
	user := s.createUser("suggester")
	artist := s.createArtist("Already", "already-matched")
	play := s.createPlay("Already", catalogm.RadioPlayMatchStateMatched, &artist.ID)

	_, err := s.service.CreateSuggestion(play.ID, user.ID, &contracts.CreateRadioPlayMatchSuggestionRequest{ArtistID: artist.ID})
	s.ErrorIs(err, contracts.ErrRadioPlayMatchSuggestionPlayNotSuggestable)
}

func (s *RadioPlayMatchSuggestionSuite) TestCreate_MissingArtist() {
	user := s.createUser("suggester")
	play := s.createPlay("Ghost", catalogm.RadioPlayMatchStateUnmatched, nil)

	_, err := s.service.CreateSuggestion(play.ID, user.ID, &contracts.CreateRadioPlayMatchSuggestionRequest{ArtistID: 999999})
	s.ErrorIs(err, contracts.ErrRadioPlayMatchSuggestionArtistNotFound)
}

func (s *RadioPlayMatchSuggestionSuite) TestAccept_LinksPlayViaLinkPlay() {
	user := s.createUser("suggester")
	admin := s.createUser("admin")
	artist := s.createArtist("Linked", "linked-accept")
	play := s.createPlay("Linked", catalogm.RadioPlayMatchStateAmbiguous, nil)

	entry, err := s.service.CreateSuggestion(play.ID, user.ID, &contracts.CreateRadioPlayMatchSuggestionRequest{ArtistID: artist.ID})
	s.Require().NoError(err)

	result, err := s.service.AcceptSuggestion(entry.ID, admin.ID, &contracts.AcceptRadioPlayMatchSuggestionRequest{})
	s.Require().NoError(err)
	s.Equal(catalogm.RadioPlayMatchSuggestionStatusAccepted, result.Status)
	s.Nil(result.BulkUpdated)

	var updated catalogm.RadioPlay
	s.Require().NoError(s.db.First(&updated, play.ID).Error)
	s.Require().NotNil(updated.ArtistID)
	s.Equal(artist.ID, *updated.ArtistID)
	s.Equal(catalogm.RadioPlayMatchStateMatched, updated.MatchState)
}

func (s *RadioPlayMatchSuggestionSuite) TestAccept_AlsoBulkLinkName() {
	user := s.createUser("suggester")
	admin := s.createUser("admin")
	artist := s.createArtist("Bulk Band", "bulk-band")

	// Two unmatched plays sharing artist_name; suggest on the first.
	play1 := s.createPlay("Bulk Band", catalogm.RadioPlayMatchStateUnmatched, nil)
	// Second play on a fresh episode/station with same name.
	play2 := s.createPlay("Bulk Band", catalogm.RadioPlayMatchStateNoMatch, nil)

	entry, err := s.service.CreateSuggestion(play1.ID, user.ID, &contracts.CreateRadioPlayMatchSuggestionRequest{ArtistID: artist.ID})
	s.Require().NoError(err)

	result, err := s.service.AcceptSuggestion(entry.ID, admin.ID, &contracts.AcceptRadioPlayMatchSuggestionRequest{
		AlsoBulkLinkName: true,
	})
	s.Require().NoError(err)
	s.Require().NotNil(result.BulkUpdated)
	// BulkLink updates all artist_id IS NULL with that name. play1 was already
	// linked by LinkPlay, so only play2 remains for bulk — but BulkLink also
	// matches play1's name after LinkPlay set artist_id, so RowsAffected is
	// typically 1 (play2). Accept either 1+.
	s.GreaterOrEqual(*result.BulkUpdated, 1)

	var p2 catalogm.RadioPlay
	s.Require().NoError(s.db.First(&p2, play2.ID).Error)
	s.Require().NotNil(p2.ArtistID)
	s.Equal(artist.ID, *p2.ArtistID)
}

func (s *RadioPlayMatchSuggestionSuite) TestReject_StampsReasonAndAllowsResubmit() {
	user := s.createUser("suggester")
	admin := s.createUser("admin")
	artist := s.createArtist("Wrong", "wrong-reject")
	artist2 := s.createArtist("Right", "right-resubmit")
	play := s.createPlay("Maybe", catalogm.RadioPlayMatchStateUnmatched, nil)

	entry, err := s.service.CreateSuggestion(play.ID, user.ID, &contracts.CreateRadioPlayMatchSuggestionRequest{ArtistID: artist.ID})
	s.Require().NoError(err)

	_, err = s.service.RejectSuggestion(entry.ID, admin.ID, &contracts.RejectRadioPlayMatchSuggestionRequest{})
	s.ErrorIs(err, contracts.ErrRadioPlayMatchSuggestionRejectReasonRequired)

	result, err := s.service.RejectSuggestion(entry.ID, admin.ID, &contracts.RejectRadioPlayMatchSuggestionRequest{
		Reason: "wrong artist",
	})
	s.Require().NoError(err)
	s.Equal(catalogm.RadioPlayMatchSuggestionStatusRejected, result.Status)
	s.Require().NotNil(result.RejectionReason)
	s.Equal("wrong artist", *result.RejectionReason)

	// Resubmit after reject is allowed (partial unique only covers pending).
	entry2, err := s.service.CreateSuggestion(play.ID, user.ID, &contracts.CreateRadioPlayMatchSuggestionRequest{ArtistID: artist2.ID})
	s.Require().NoError(err)
	s.Equal(catalogm.RadioPlayMatchSuggestionStatusPending, entry2.Status)
	s.Equal(artist2.ID, entry2.SuggestedArtistID)
}

func (s *RadioPlayMatchSuggestionSuite) TestGetOwnPending() {
	user := s.createUser("suggester")
	artist := s.createArtist("Own", "own-pending")
	play := s.createPlay("Own", catalogm.RadioPlayMatchStateUnmatched, nil)

	none, err := s.service.GetOwnPendingSuggestion(play.ID, user.ID)
	s.Require().NoError(err)
	s.Nil(none)

	_, err = s.service.CreateSuggestion(play.ID, user.ID, &contracts.CreateRadioPlayMatchSuggestionRequest{ArtistID: artist.ID})
	s.Require().NoError(err)

	got, err := s.service.GetOwnPendingSuggestion(play.ID, user.ID)
	s.Require().NoError(err)
	s.Require().NotNil(got)
	s.Equal(artist.ID, got.SuggestedArtistID)
}

func (s *RadioPlayMatchSuggestionSuite) TestListPending() {
	user := s.createUser("suggester")
	artist := s.createArtist("Listed", "listed")
	play := s.createPlay("Listed", catalogm.RadioPlayMatchStateUnmatched, nil)

	_, err := s.service.CreateSuggestion(play.ID, user.ID, &contracts.CreateRadioPlayMatchSuggestionRequest{ArtistID: artist.ID})
	s.Require().NoError(err)

	result, err := s.service.ListPendingSuggestions(50, 0)
	s.Require().NoError(err)
	s.Equal(int64(1), result.Total)
	s.Require().Len(result.Suggestions, 1)
	s.Equal("Listed", result.Suggestions[0].PlayArtistName)
}

func (s *RadioPlayMatchSuggestionSuite) TestAccept_IdempotentReplay() {
	user := s.createUser("suggester")
	admin := s.createUser("admin")
	artist := s.createArtist("Idem", "idem-accept")
	play := s.createPlay("Idem", catalogm.RadioPlayMatchStateUnmatched, nil)

	entry, err := s.service.CreateSuggestion(play.ID, user.ID, &contracts.CreateRadioPlayMatchSuggestionRequest{ArtistID: artist.ID})
	s.Require().NoError(err)

	first, err := s.service.AcceptSuggestion(entry.ID, admin.ID, &contracts.AcceptRadioPlayMatchSuggestionRequest{})
	s.Require().NoError(err)

	second, err := s.service.AcceptSuggestion(entry.ID, admin.ID+1, &contracts.AcceptRadioPlayMatchSuggestionRequest{})
	s.Require().NoError(err)
	s.Equal(first.Status, second.Status)
	s.Equal(*first.ReviewedBy, *second.ReviewedBy)
}

func matchNote(s string) *string { return &s }
