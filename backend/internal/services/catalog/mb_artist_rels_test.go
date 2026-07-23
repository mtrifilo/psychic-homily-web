package catalog

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	catalogm "psychic-homily-backend/internal/models/catalog"
)

func TestMapMBArtistRels_MemberOfAndIsPerson(t *testing.T) {
	intents := MapMBArtistRels([]MBArtistRel{
		{Type: "member of band", PeerMBID: "band-1", Ended: true, Attributes: []string{"guitar"}},
		{Type: "member of band", PeerMBID: "band-1", Ended: true, Attributes: []string{"vocals"}}, // dedupe
		{Type: "is person", PeerMBID: "alias-1", Ended: false},
		{Type: "is person", PeerMBID: "alias-1", Ended: true}, // ended OR'd onto same intent
		{Type: "married", PeerMBID: "person-2"},               // ignored
		{Type: "member of band", PeerMBID: ""},                // no peer
		{Type: "collaboration", PeerMBID: "collab-1"},         // ignored
	})

	require.Len(t, intents, 2)

	byPeer := map[string]mbArtistEdgeIntent{}
	for _, i := range intents {
		byPeer[i.PeerMBID] = i
	}

	member := byPeer["band-1"]
	assert.Equal(t, catalogm.RelationshipTypeMemberOf, member.RelType)
	assert.True(t, member.Ended)

	side := byPeer["alias-1"]
	assert.Equal(t, catalogm.RelationshipTypeSideProject, side.RelType)
	assert.True(t, side.Ended, "ended=true on any contributing row wins")
}

func TestMapMBArtistRels_IncludesCurrentMemberships(t *testing.T) {
	intents := MapMBArtistRels([]MBArtistRel{
		{Type: "member of band", PeerMBID: "band-1", Ended: false},
	})
	require.Len(t, intents, 1)
	assert.False(t, intents[0].Ended)
	assert.Equal(t, catalogm.RelationshipTypeMemberOf, intents[0].RelType)
}

// fakeMBRels is a test double for artistArtistRelsClient.
type fakeMBRels struct {
	byMBID map[string][]MBArtistRel
	err    error
}

func (f *fakeMBRels) LookupArtistArtistRelations(_ context.Context, mbid string) ([]MBArtistRel, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.byMBID[mbid], nil
}

func (suite *ArtistRelationshipServiceIntegrationTestSuite) createArtistWithMBID(name, mbid string) uint {
	slug := fmt.Sprintf("%s-%d", name, time.Now().UnixNano())
	artist := &catalogm.Artist{Name: name, Slug: &slug, MusicBrainzArtistID: &mbid}
	err := suite.db.Create(artist).Error
	suite.Require().NoError(err)
	return artist.ID
}

func (suite *ArtistRelationshipServiceIntegrationTestSuite) TestDeriveMusicBrainzArtistRels_MemberAndSideProject() {
	thurston := suite.createArtistWithMBID("Thurston Moore", "mbid-thurston")
	sonic := suite.createArtistWithMBID("Sonic Youth", "mbid-sonic")
	caribou := suite.createArtistWithMBID("Caribou", "mbid-caribou")
	_ = suite.createArtistWithMBID("Orphan", "mbid-orphan") // no rels

	fake := &fakeMBRels{byMBID: map[string][]MBArtistRel{
		"mbid-thurston": {
			{Type: "member of band", PeerMBID: "mbid-sonic", Ended: true, Attributes: []string{"guitar"}},
			{Type: "member of band", PeerMBID: "mbid-sonic", Ended: true, Attributes: []string{"vocals"}},
			{Type: "is person", PeerMBID: "mbid-caribou", Ended: false},
			{Type: "member of band", PeerMBID: "mbid-missing-peer", Ended: false}, // skip
			{Type: "married", PeerMBID: "mbid-caribou"},                          // ignore
		},
		"mbid-sonic": {
			{Type: "member of band", PeerMBID: "mbid-thurston", Ended: true}, // reverse; same undirected edge
		},
		"mbid-caribou": {
			{Type: "is person", PeerMBID: "mbid-thurston", Ended: false},
		},
	}}
	suite.svc.SetArtistRelsClient(fake)

	result, err := suite.svc.DeriveMusicBrainzArtistRels(context.Background())
	suite.Require().NoError(err)
	suite.Equal(4, result.ArtistsScanned)
	suite.Equal(1, result.PeersSkipped)
	suite.GreaterOrEqual(result.MemberOfUpserted, int64(1))
	suite.GreaterOrEqual(result.SideProjectUpserted, int64(1))

	var member catalogm.ArtistRelationship
	err = suite.db.Where("relationship_type = ? AND auto_derived = true", catalogm.RelationshipTypeMemberOf).First(&member).Error
	suite.Require().NoError(err)
	src, tgt := catalogm.CanonicalOrder(thurston, sonic)
	suite.Equal(src, member.SourceArtistID)
	suite.Equal(tgt, member.TargetArtistID)
	suite.Equal(float32(1.0), member.Score)

	var side catalogm.ArtistRelationship
	err = suite.db.Where("relationship_type = ? AND auto_derived = true", catalogm.RelationshipTypeSideProject).First(&side).Error
	suite.Require().NoError(err)
	src, tgt = catalogm.CanonicalOrder(thurston, caribou)
	suite.Equal(src, side.SourceArtistID)
	suite.Equal(tgt, side.TargetArtistID)

	// Re-run is idempotent (reconcile keeps the same set).
	result2, err := suite.svc.DeriveMusicBrainzArtistRels(context.Background())
	suite.Require().NoError(err)
	suite.GreaterOrEqual(result2.MemberOfUpserted, int64(1))

	var memberCount int64
	suite.db.Model(&catalogm.ArtistRelationship{}).
		Where("relationship_type = ? AND auto_derived = true", catalogm.RelationshipTypeMemberOf).
		Count(&memberCount)
	suite.Equal(int64(1), memberCount)
}

func (suite *ArtistRelationshipServiceIntegrationTestSuite) TestDeriveMusicBrainzArtistRels_ReconcileRemovesStale() {
	a := suite.createArtistWithMBID("A", "mbid-a")
	b := suite.createArtistWithMBID("B", "mbid-b")
	c := suite.createArtistWithMBID("C", "mbid-c")

	// Seed a stale auto_derived member_of A↔C that the next derive will not emit.
	src, tgt := catalogm.CanonicalOrder(a, c)
	stale := catalogm.ArtistRelationship{
		SourceArtistID:   src,
		TargetArtistID:   tgt,
		RelationshipType: catalogm.RelationshipTypeMemberOf,
		Score:            1.0,
		AutoDerived:      true,
		CreatedAt:        time.Now().Add(-time.Hour),
		UpdatedAt:        time.Now().Add(-time.Hour),
	}
	suite.Require().NoError(suite.db.Create(&stale).Error)

	fake := &fakeMBRels{byMBID: map[string][]MBArtistRel{
		"mbid-a": {{Type: "member of band", PeerMBID: "mbid-b"}},
		"mbid-b": {{Type: "member of band", PeerMBID: "mbid-a"}},
		"mbid-c": {},
	}}
	suite.svc.SetArtistRelsClient(fake)

	_, err := suite.svc.DeriveMusicBrainzArtistRels(context.Background())
	suite.Require().NoError(err)

	var count int64
	suite.db.Model(&catalogm.ArtistRelationship{}).
		Where("relationship_type = ? AND auto_derived = true", catalogm.RelationshipTypeMemberOf).
		Count(&count)
	suite.Equal(int64(1), count)

	var kept catalogm.ArtistRelationship
	suite.Require().NoError(suite.db.Where("relationship_type = ?", catalogm.RelationshipTypeMemberOf).First(&kept).Error)
	src, tgt = catalogm.CanonicalOrder(a, b)
	suite.Equal(src, kept.SourceArtistID)
	suite.Equal(tgt, kept.TargetArtistID)
}

func (suite *ArtistRelationshipServiceIntegrationTestSuite) TestDeriveMusicBrainzArtistRels_NilClientNoop() {
	suite.svc.SetArtistRelsClient(nil)
	_ = suite.createArtistWithMBID("X", "mbid-x")
	result, err := suite.svc.DeriveMusicBrainzArtistRels(context.Background())
	suite.Require().NoError(err)
	suite.Equal(int64(0), result.MemberOfUpserted)
	suite.Equal(int64(0), result.SideProjectUpserted)
}

func (suite *ArtistRelationshipServiceIntegrationTestSuite) TestDeriveMusicBrainzArtistRels_LookupFailureSkipsReconcile() {
	a := suite.createArtistWithMBID("A", "mbid-a")
	b := suite.createArtistWithMBID("B", "mbid-b")
	c := suite.createArtistWithMBID("C", "mbid-c")

	// Existing A↔C edge that must survive when B's lookup fails and A emits only A↔B.
	src, tgt := catalogm.CanonicalOrder(a, c)
	stale := catalogm.ArtistRelationship{
		SourceArtistID:   src,
		TargetArtistID:   tgt,
		RelationshipType: catalogm.RelationshipTypeMemberOf,
		Score:            1.0,
		AutoDerived:      true,
		CreatedAt:        time.Now().Add(-time.Hour),
		UpdatedAt:        time.Now().Add(-time.Hour),
	}
	suite.Require().NoError(suite.db.Create(&stale).Error)

	calls := 0
	// Custom client that fails for mbid-c
	suite.svc.SetArtistRelsClient(&failingMBRels{
		ok: map[string][]MBArtistRel{
			"mbid-a": {{Type: "member of band", PeerMBID: "mbid-b"}},
			"mbid-b": {{Type: "member of band", PeerMBID: "mbid-a"}},
		},
		failMBID: "mbid-c",
		calls:    &calls,
	})

	result, err := suite.svc.DeriveMusicBrainzArtistRels(context.Background())
	suite.Require().NoError(err)
	suite.Equal(1, result.LookupsFailed)

	var count int64
	suite.db.Model(&catalogm.ArtistRelationship{}).
		Where("relationship_type = ? AND auto_derived = true", catalogm.RelationshipTypeMemberOf).
		Count(&count)
	// A↔B upserted AND A↔C preserved (no stale reconcile on lookup failure).
	suite.Equal(int64(2), count)
	_ = b
}

// failingMBRels fails Lookup for one MBID and succeeds for others.
type failingMBRels struct {
	ok       map[string][]MBArtistRel
	failMBID string
	calls    *int
}

func (f *failingMBRels) LookupArtistArtistRelations(_ context.Context, mbid string) ([]MBArtistRel, error) {
	if f.calls != nil {
		*f.calls++
	}
	if mbid == f.failMBID {
		return nil, fmt.Errorf("simulated mb failure")
	}
	return f.ok[mbid], nil
}

func (suite *ArtistRelationshipServiceIntegrationTestSuite) TestDeriveMusicBrainzArtistRels_DryRunNoWrite() {
	a := suite.createArtistWithMBID("A", "mbid-a")
	b := suite.createArtistWithMBID("B", "mbid-b")
	_ = a
	_ = b

	fake := &fakeMBRels{byMBID: map[string][]MBArtistRel{
		"mbid-a": {{Type: "member of band", PeerMBID: "mbid-b"}},
		"mbid-b": {},
	}}
	suite.svc.SetArtistRelsClient(fake)

	result, err := suite.svc.DeriveMusicBrainzArtistRelsWithOptions(context.Background(), MusicBrainzArtistRelsOptions{DryRun: true})
	suite.Require().NoError(err)
	suite.Equal(int64(1), result.MemberOfUpserted)

	var count int64
	suite.db.Model(&catalogm.ArtistRelationship{}).Count(&count)
	suite.Equal(int64(0), count)
}
