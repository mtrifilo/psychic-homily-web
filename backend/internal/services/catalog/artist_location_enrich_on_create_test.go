package catalog

import (
	"testing"

	"github.com/stretchr/testify/suite"
	"gorm.io/gorm"

	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/testutil"
)

// PSY-1251 (Phase B): CreateArtist fires the on-create location enricher with the new
// artist's id on the CREATED path only, and the dispatch is a safe no-op when no
// enricher is wired (the feature-off default). The enricher itself (the actual MB/
// Bandcamp resolve) is covered by enrich.TestEnrichArtistLocationByID — here we use a
// spy + SetSyncDispatch so we assert the WIRING without racing the goroutine.
type ArtistLocationEnrichOnCreateTestSuite struct {
	suite.Suite
	testDB *testutil.TestDatabase
	db     *gorm.DB
}

func (s *ArtistLocationEnrichOnCreateTestSuite) SetupSuite() {
	s.testDB = testutil.SetupTestPostgres(s.T())
	s.db = s.testDB.DB
}

func (s *ArtistLocationEnrichOnCreateTestSuite) TearDownSuite() { s.testDB.Cleanup() }

func (s *ArtistLocationEnrichOnCreateTestSuite) TearDownTest() {
	sqlDB, err := s.db.DB()
	s.Require().NoError(err)
	_, _ = sqlDB.Exec("DELETE FROM artists")
}

func TestArtistLocationEnrichOnCreateTestSuite(t *testing.T) {
	suite.Run(t, new(ArtistLocationEnrichOnCreateTestSuite))
}

// newService builds a service whose dispatch runs INLINE (SetSyncDispatch) so the spy
// records deterministically; a nil spy leaves the enricher unwired (feature-off).
func (s *ArtistLocationEnrichOnCreateTestSuite) newService(spy *[]uint) *ArtistService {
	svc := &ArtistService{db: s.db}
	svc.SetSyncDispatch()
	if spy != nil {
		svc.SetLocationEnricher(func(id uint) { *spy = append(*spy, id) })
	}
	return svc
}

func (s *ArtistLocationEnrichOnCreateTestSuite) TestFiresOnceOnCreate() {
	var called []uint
	svc := s.newService(&called)
	_, err := svc.CreateArtist(&contracts.CreateArtistRequest{Name: "On Create Band"})
	s.Require().NoError(err)
	s.Require().Len(called, 1, "the location enricher should fire exactly once on create")
	s.NotZero(called[0], "it should fire with the new artist's id")
}

func (s *ArtistLocationEnrichOnCreateTestSuite) TestNoEnricherIsSafeNoOp() {
	svc := s.newService(nil) // feature off — no enricher wired
	_, err := svc.CreateArtist(&contracts.CreateArtistRequest{Name: "Quiet Band"})
	s.Require().NoError(err, "a nil enricher must be a safe no-op, not a panic/error")
}

func (s *ArtistLocationEnrichOnCreateTestSuite) TestNotFiredOnDuplicate() {
	var called []uint
	svc := s.newService(&called)
	const name = "Dup Band"
	_, err := svc.CreateArtist(&contracts.CreateArtistRequest{Name: name})
	s.Require().NoError(err)
	called = called[:0] // reset after the genuine create

	_, err = svc.CreateArtist(&contracts.CreateArtistRequest{Name: name})
	s.Require().Error(err, "a duplicate create returns ErrArtistExists")
	s.Empty(called, "a duplicate (not-created) path must NOT fire the enricher")
}
