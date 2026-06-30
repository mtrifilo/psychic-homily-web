package catalog

import (
	"testing"

	"github.com/stretchr/testify/suite"
	"gorm.io/gorm"

	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/testutil"
)

// ReleaseDedupTestSuite covers the release-group-MBID dedup keystone (PSY-1281):
// the FindOrCreateReleaseByReleaseGroupMBID resolution order (RG-MBID dedup →
// artist-anchored exact-title fill-when-empty → create) and the partial-unique
// index (NULLs unconstrained).
type ReleaseDedupTestSuite struct {
	suite.Suite
	testDB *testutil.TestDatabase
	db     *gorm.DB
}

func (s *ReleaseDedupTestSuite) SetupSuite() {
	s.testDB = testutil.SetupTestPostgres(s.T())
	s.db = s.testDB.DB
}
func (s *ReleaseDedupTestSuite) TearDownSuite() { s.testDB.Cleanup() }
func (s *ReleaseDedupTestSuite) TearDownTest() {
	sqlDB, err := s.db.DB()
	s.Require().NoError(err)
	for _, t := range []string{"artist_releases", "release_external_links", "image_enrich_queue", "releases", "artists"} {
		_, _ = sqlDB.Exec("DELETE FROM " + t)
	}
}
func TestReleaseDedupTestSuite(t *testing.T) { suite.Run(t, new(ReleaseDedupTestSuite)) }

const (
	mbidA = "11111111-1111-1111-1111-111111111111"
	mbidB = "22222222-2222-2222-2222-222222222222"
)

// --- helpers ---

func (s *ReleaseDedupTestSuite) artist(name string) uint {
	a, _, err := FindOrCreateArtistTx(s.db, name, nil)
	s.Require().NoError(err)
	return a.ID
}

func releaseReq(title string, artistIDs ...uint) *contracts.CreateReleaseRequest {
	entries := make([]contracts.CreateReleaseArtistEntry, 0, len(artistIDs))
	for _, id := range artistIDs {
		entries = append(entries, contracts.CreateReleaseArtistEntry{ArtistID: id, Role: "main"})
	}
	return &contracts.CreateReleaseRequest{Title: title, Artists: entries}
}

func (s *ReleaseDedupTestSuite) releaseCount() int64 {
	var n int64
	s.Require().NoError(s.db.Model(&catalogm.Release{}).Count(&n).Error)
	return n
}

func (s *ReleaseDedupTestSuite) reload(id uint) catalogm.Release {
	var r catalogm.Release
	s.Require().NoError(s.db.First(&r, id).Error)
	return r
}

// createPlain creates a release via the interactive (no-dedup) path; it leaves
// musicbrainz_release_group_id NULL — the legacy-row shape the importer reconciles.
func (s *ReleaseDedupTestSuite) createPlain(title string, artistIDs ...uint) uint {
	resp, err := NewReleaseService(s.db).CreateRelease(releaseReq(title, artistIDs...))
	s.Require().NoError(err)
	return resp.ID
}

// --- tests ---

// Acceptance: a new RG-MBID is stored (create path).
func (s *ReleaseDedupTestSuite) TestStampsRGMBIDWhenAbsent() {
	a := s.artist("Sleep")
	r, created, err := FindOrCreateReleaseByReleaseGroupMBID(s.db, mbidA, releaseReq("Dopesmoker", a))
	s.Require().NoError(err)
	s.True(created)
	s.Require().NotNil(r.MusicBrainzReleaseGroupID)
	s.Equal(mbidA, *r.MusicBrainzReleaseGroupID)
	s.Equal(int64(1), s.releaseCount())
}

// Acceptance: an existing RG-MBID is not duplicated (idempotent re-import) — and the
// RG-MBID match wins regardless of the title passed on the re-import.
func (s *ReleaseDedupTestSuite) TestExistingRGMBIDNotDuplicated() {
	a := s.artist("Sleep")
	first, created, err := FindOrCreateReleaseByReleaseGroupMBID(s.db, mbidA, releaseReq("Dopesmoker", a))
	s.Require().NoError(err)
	s.Require().True(created)

	again, created2, err := FindOrCreateReleaseByReleaseGroupMBID(s.db, mbidA, releaseReq("Dopesmoker", a))
	s.Require().NoError(err)
	s.False(created2)
	s.Equal(first.ID, again.ID)

	// Same RG-MBID, different title supplied: still the same release (RG-MBID is the key).
	third, created3, err := FindOrCreateReleaseByReleaseGroupMBID(s.db, mbidA, releaseReq("Totally Different Title", a))
	s.Require().NoError(err)
	s.False(created3)
	s.Equal(first.ID, third.ID)
	s.Equal(int64(1), s.releaseCount())
}

// Acceptance: NULL RG-MBID rows are unconstrained — two same-title legacy releases
// (both NULL) coexist; the partial-unique index only constrains non-NULL values.
func (s *ReleaseDedupTestSuite) TestNullRGMBIDsUnconstrained() {
	a := s.artist("Earth")
	id1 := s.createPlain("Earth 2", a)
	id2 := s.createPlain("Earth 2", a)
	s.NotEqual(id1, id2)
	s.Equal(int64(2), s.releaseCount())
	s.Nil(s.reload(id1).MusicBrainzReleaseGroupID)
	s.Nil(s.reload(id2).MusicBrainzReleaseGroupID)
}

// Step 2: an artist-anchored, exact-title (case-insensitive) legacy release whose
// RG-MBID is NULL gets stamped in place — no new release.
func (s *ReleaseDedupTestSuite) TestTitleMatchFillWhenEmpty() {
	a := s.artist("Sleep")
	legacy := s.createPlain("Holy Mountain", a)
	s.Require().Nil(s.reload(legacy).MusicBrainzReleaseGroupID)

	r, created, err := FindOrCreateReleaseByReleaseGroupMBID(s.db, mbidA, releaseReq("holy mountain", a)) // different case
	s.Require().NoError(err)
	s.False(created)
	s.Equal(legacy, r.ID)
	s.Require().NotNil(r.MusicBrainzReleaseGroupID)
	s.Equal(mbidA, *r.MusicBrainzReleaseGroupID)
	s.Equal(int64(1), s.releaseCount())
	s.Equal(mbidA, *s.reload(legacy).MusicBrainzReleaseGroupID)
}

// Step 2 safety: an AMBIGUOUS title match (two legacy same-title releases for the
// same artist) is never merged — a fresh release is created instead.
func (s *ReleaseDedupTestSuite) TestAmbiguousTitleMatchCreatesNew() {
	a := s.artist("Boris")
	leg1 := s.createPlain("Heavy Rocks", a)
	leg2 := s.createPlain("Heavy Rocks", a) // Boris really did this; perfect ambiguity
	s.Require().Equal(int64(2), s.releaseCount())

	r, created, err := FindOrCreateReleaseByReleaseGroupMBID(s.db, mbidA, releaseReq("Heavy Rocks", a))
	s.Require().NoError(err)
	s.True(created)
	s.NotEqual(leg1, r.ID)
	s.NotEqual(leg2, r.ID)
	s.Equal(int64(3), s.releaseCount())
	// The two ambiguous legacy rows stay untouched (NULL).
	s.Nil(s.reload(leg1).MusicBrainzReleaseGroupID)
	s.Nil(s.reload(leg2).MusicBrainzReleaseGroupID)
}

// Step 2 anchor: a same-title release by a DIFFERENT artist is not matched.
func (s *ReleaseDedupTestSuite) TestDifferentArtistNotMatched() {
	a := s.artist("Artist A")
	b := s.artist("Artist B")
	legacy := s.createPlain("Untitled", a)

	r, created, err := FindOrCreateReleaseByReleaseGroupMBID(s.db, mbidA, releaseReq("Untitled", b))
	s.Require().NoError(err)
	s.True(created)
	s.NotEqual(legacy, r.ID)
	s.Equal(int64(2), s.releaseCount())
	s.Nil(s.reload(legacy).MusicBrainzReleaseGroupID)
}

// Step 2 anchor: with no credited artist there is no safe anchor, so we never
// title-match — a fresh release is created.
func (s *ReleaseDedupTestSuite) TestNoArtistAnchorCreatesNew() {
	legacy := s.createPlain("Compilation")
	r, created, err := FindOrCreateReleaseByReleaseGroupMBID(s.db, mbidA, releaseReq("Compilation"))
	s.Require().NoError(err)
	s.True(created)
	s.NotEqual(legacy, r.ID)
	s.Equal(int64(2), s.releaseCount())
}

// Concurrency recovery (step 2): when the title-match UPDATE collides with an
// already-claimed RG-MBID, it must (a) converge on the existing owner and (b) NOT
// poison the caller's transaction — the SAVEPOINT fix. We call the unexported step-2
// helper directly: in the public flow step 1 would short-circuit a pre-existing
// RG-MBID, so the UPDATE collision is only reachable under a real concurrent race,
// which this simulates deterministically.
func (s *ReleaseDedupTestSuite) TestTitleMatchDupKeyRecoveryDoesNotPoisonTx() {
	a := s.artist("Sleep")
	winner, created, err := FindOrCreateReleaseByReleaseGroupMBID(s.db, mbidA, releaseReq("Volume One", a))
	s.Require().NoError(err)
	s.Require().True(created)
	cand := s.createPlain("Holy Mountain", a) // NULL-RG-MBID legacy title-match candidate

	err = s.db.Transaction(func(tx *gorm.DB) error {
		got, ok, ferr := fillReleaseGroupMBIDOnTitleMatch(tx, mbidA, releaseReq("Holy Mountain", a))
		s.Require().NoError(ferr)
		s.Require().True(ok)
		s.Equal(winner.ID, got.ID) // converged on the existing owner, not the candidate

		// A poisoned tx would fail this next statement with SQLSTATE 25P02.
		var n int64
		return tx.Model(&catalogm.Release{}).Count(&n).Error
	})
	s.Require().NoError(err)
	s.Nil(s.reload(cand).MusicBrainzReleaseGroupID) // candidate untouched (update rolled back)
}

// Trust boundary: a malformed RG-MBID is rejected before any write.
func (s *ReleaseDedupTestSuite) TestInvalidMBIDRejected() {
	a := s.artist("Sleep")
	_, _, err := FindOrCreateReleaseByReleaseGroupMBID(s.db, "not-a-uuid", releaseReq("Dopesmoker", a))
	s.Require().Error(err)
	s.Equal(int64(0), s.releaseCount())
}

// Trust boundary: a blank title is rejected (it cannot generate a slug).
func (s *ReleaseDedupTestSuite) TestBlankTitleRejected() {
	a := s.artist("Sleep")
	_, _, err := FindOrCreateReleaseByReleaseGroupMBID(s.db, mbidA, releaseReq("   ", a))
	s.Require().Error(err)
	s.Equal(int64(0), s.releaseCount())
}
