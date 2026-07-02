package catalog

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/suite"
	"gorm.io/gorm"

	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/testutil"
)

// =============================================================================
// INTEGRATION TESTS (Require Database)
// =============================================================================

type CommunityDetectionSuite struct {
	suite.Suite
	testDB *testutil.TestDatabase
	db     *gorm.DB
	svc    *RadioService
}

func TestCommunityDetectionSuite(t *testing.T) {
	suite.Run(t, new(CommunityDetectionSuite))
}

func (s *CommunityDetectionSuite) SetupSuite() {
	s.testDB = testutil.SetupTestPostgres(s.T())
	s.db = s.testDB.DB
	s.svc = &RadioService{db: s.db}
}

func (s *CommunityDetectionSuite) TearDownSuite() {
	s.testDB.Cleanup()
}

func (s *CommunityDetectionSuite) TearDownTest() {
	sqlDB, err := s.db.DB()
	s.Require().NoError(err)
	_, _ = sqlDB.Exec("DELETE FROM artist_communities")
	_, _ = sqlDB.Exec("DELETE FROM artist_relationships")
	_, _ = sqlDB.Exec("DELETE FROM artists")
}

func (s *CommunityDetectionSuite) createArtist(name string) *catalogm.Artist {
	slug := fmt.Sprintf("%s-cd", name)
	artist := &catalogm.Artist{Name: name, Slug: &slug}
	s.Require().NoError(s.db.Create(artist).Error)
	return artist
}

func (s *CommunityDetectionSuite) insertRelScore(aID, bID uint, typ string, score float32, sig *float64) {
	lo, hi := min(aID, bID), max(aID, bID)
	s.Require().NoError(s.db.Create(&catalogm.ArtistRelationship{
		SourceArtistID:       lo,
		TargetArtistID:       hi,
		RelationshipType:     typ,
		Score:                score,
		AutoDerived:          true,
		BackboneSignificance: sig,
	}).Error)
}

func (s *CommunityDetectionSuite) communityOf(artistID uint) *int {
	var a catalogm.Artist
	s.Require().NoError(s.db.First(&a, artistID).Error)
	return a.CommunityID
}

// Two dense shared_bills groups → two communities, each with an "Around"
// label anchored on its highest-strength member. Radio edges join the graph
// only when backbone-significant.
func (s *CommunityDetectionSuite) TestCompute_PartitionsAndLabels() {
	// Group 1: a triangle where A carries the most strength.
	a := s.createArtist("Alpha")
	b := s.createArtist("Beta")
	c := s.createArtist("Gamma")
	s.insertRelScore(a.ID, b.ID, catalogm.RelationshipTypeSharedBills, 0.9, nil)
	s.insertRelScore(a.ID, c.ID, catalogm.RelationshipTypeSharedBills, 0.9, nil)
	s.insertRelScore(b.ID, c.ID, catalogm.RelationshipTypeSharedBills, 0.5, nil)

	// Group 2: another triangle, D strongest.
	d := s.createArtist("Delta")
	e := s.createArtist("Epsilon")
	f := s.createArtist("Zeta")
	s.insertRelScore(d.ID, e.ID, catalogm.RelationshipTypeSharedBills, 0.8, nil)
	s.insertRelScore(d.ID, f.ID, catalogm.RelationshipTypeSharedBills, 0.8, nil)
	s.insertRelScore(e.ID, f.ID, catalogm.RelationshipTypeSharedBills, 0.4, nil)

	// A NON-significant radio edge bridging the groups: it must be EXCLUDED
	// from the partition input, or the two groups would tend to merge.
	noise := 0.9
	s.insertRelScore(a.ID, d.ID, catalogm.RelationshipTypeRadioCooccurrence, 1.0, &noise)

	result, err := s.svc.ComputeArtistCommunities()
	s.Require().NoError(err)
	s.Assert().Equal(2, result.Communities)
	s.Assert().Equal(6, result.AssignedArtists)

	// Groups intact and separate.
	s.Require().NotNil(s.communityOf(a.ID))
	s.Assert().Equal(*s.communityOf(a.ID), *s.communityOf(b.ID))
	s.Assert().Equal(*s.communityOf(a.ID), *s.communityOf(c.ID))
	s.Assert().Equal(*s.communityOf(d.ID), *s.communityOf(e.ID))
	s.Assert().NotEqual(*s.communityOf(a.ID), *s.communityOf(d.ID))

	// Labels anchor on the highest-strength member of each community.
	var comms []catalogm.ArtistCommunity
	s.Require().NoError(s.db.Order("id").Find(&comms).Error)
	s.Require().Len(comms, 2)
	labelAnchors := map[uint]bool{}
	for _, cm := range comms {
		labelAnchors[cm.LabelArtistID] = true
		s.Assert().Equal(3, cm.MemberCount)
	}
	s.Assert().True(labelAnchors[a.ID], "Alpha anchors group 1 (strength 1.8)")
	s.Assert().True(labelAnchors[d.ID], "Delta anchors group 2 (strength 1.6)")
}

// A significant radio edge IS part of the input graph.
func (s *CommunityDetectionSuite) TestCompute_SignificantRadioIncluded() {
	a := s.createArtist("R1")
	b := s.createArtist("R2")
	sig := 0.02
	s.insertRelScore(a.ID, b.ID, catalogm.RelationshipTypeRadioCooccurrence, 0.8, &sig)

	result, err := s.svc.ComputeArtistCommunities()
	s.Require().NoError(err)
	s.Assert().Equal(1, result.Communities)
	s.Require().NotNil(s.communityOf(a.ID))
	s.Assert().Equal(*s.communityOf(a.ID), *s.communityOf(b.ID))
}

// The rebuild clears stale assignments transactionally: an artist that left
// the graph loses its community_id, and the label table is fully replaced.
func (s *CommunityDetectionSuite) TestCompute_ClearsStaleAssignments() {
	stale := s.createArtist("Stale")
	s.Require().NoError(s.db.Model(&catalogm.Artist{}).
		Where("id = ?", stale.ID).Update("community_id", 99).Error)

	a := s.createArtist("Live1")
	b := s.createArtist("Live2")
	s.insertRelScore(a.ID, b.ID, catalogm.RelationshipTypeSharedBills, 0.9, nil)

	result, err := s.svc.ComputeArtistCommunities()
	s.Require().NoError(err)
	s.Assert().Equal(int64(1), result.ClearedArtists)
	s.Assert().Nil(s.communityOf(stale.ID), "artist outside the graph is cleared")
	s.Require().NotNil(s.communityOf(a.ID))
}

// Same data → identical partition and labels (the fixed-seed requirement).
func (s *CommunityDetectionSuite) TestCompute_Deterministic() {
	a := s.createArtist("D1")
	b := s.createArtist("D2")
	c := s.createArtist("D3")
	s.insertRelScore(a.ID, b.ID, catalogm.RelationshipTypeSharedBills, 0.9, nil)
	s.insertRelScore(b.ID, c.ID, catalogm.RelationshipTypeSharedBills, 0.9, nil)

	_, err := s.svc.ComputeArtistCommunities()
	s.Require().NoError(err)
	first := map[uint]int{a.ID: *s.communityOf(a.ID), b.ID: *s.communityOf(b.ID), c.ID: *s.communityOf(c.ID)}

	_, err = s.svc.ComputeArtistCommunities()
	s.Require().NoError(err)
	for id, want := range first {
		s.Assert().Equal(want, *s.communityOf(id), "assignment for artist %d reshuffled across identical recomputes", id)
	}
}

// No edges at all: valid empty partition, everything cleared.
func (s *CommunityDetectionSuite) TestCompute_EmptyGraph() {
	a := s.createArtist("Lonely")
	result, err := s.svc.ComputeArtistCommunities()
	s.Require().NoError(err)
	s.Assert().Equal(0, result.Communities)
	s.Assert().Nil(s.communityOf(a.ID))
}
