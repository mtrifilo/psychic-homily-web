package catalog

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// =============================================================================
// TableName Tests
// =============================================================================

func TestArtistRelationshipTableName(t *testing.T) {
	r := ArtistRelationship{}
	assert.Equal(t, "artist_relationships", r.TableName())
}

func TestArtistRelationshipVoteTableName(t *testing.T) {
	v := ArtistRelationshipVote{}
	assert.Equal(t, "artist_relationship_votes", v.TableName())
}

// =============================================================================
// CanonicalOrder Tests
// =============================================================================

func TestCanonicalOrder_AlreadyOrdered(t *testing.T) {
	a, b := CanonicalOrder(1, 5)
	assert.Equal(t, uint(1), a)
	assert.Equal(t, uint(5), b)
}

func TestCanonicalOrder_ReversedOrder(t *testing.T) {
	a, b := CanonicalOrder(10, 3)
	assert.Equal(t, uint(3), a)
	assert.Equal(t, uint(10), b)
}

func TestCanonicalOrder_Equal(t *testing.T) {
	a, b := CanonicalOrder(7, 7)
	assert.Equal(t, uint(7), a)
	assert.Equal(t, uint(7), b)
}

func TestCanonicalOrder_Zero(t *testing.T) {
	a, b := CanonicalOrder(0, 5)
	assert.Equal(t, uint(0), a)
	assert.Equal(t, uint(5), b)
}

func TestCanonicalOrder_LargeValues(t *testing.T) {
	a, b := CanonicalOrder(999999, 1)
	assert.Equal(t, uint(1), a)
	assert.Equal(t, uint(999999), b)
}

// =============================================================================
// WilsonScore Tests
// =============================================================================

func TestArtistRelationshipWilsonScore_NoVotes(t *testing.T) {
	r := &ArtistRelationship{}
	score := r.WilsonScore(0, 0)
	assert.Equal(t, 0.0, score)
}

func TestArtistRelationshipWilsonScore_AllUpvotes(t *testing.T) {
	r := &ArtistRelationship{}
	score := r.WilsonScore(10, 0)
	// With 10 upvotes and 0 downvotes, Wilson score should be high but < 1
	assert.Greater(t, score, 0.8)
	assert.Less(t, score, 1.0)
}

func TestArtistRelationshipWilsonScore_AllDownvotes(t *testing.T) {
	r := &ArtistRelationship{}
	score := r.WilsonScore(0, 10)
	// With 0 upvotes and 10 downvotes, Wilson score should be 0
	assert.Equal(t, 0.0, score)
}

func TestArtistRelationshipWilsonScore_MixedVotes(t *testing.T) {
	r := &ArtistRelationship{}
	score := r.WilsonScore(8, 2)
	// 80% positive with 10 total votes
	assert.Greater(t, score, 0.5)
	assert.Less(t, score, 0.9)
}

func TestArtistRelationshipWilsonScore_HighConfidence_BeatsLowSample(t *testing.T) {
	r := &ArtistRelationship{}
	// 95/100 upvotes should outrank 3/3 upvotes
	highN := r.WilsonScore(95, 5)
	lowN := r.WilsonScore(3, 0)
	assert.Greater(t, highN, lowN, "high-N 95%% should outrank low-N 100%%")
}

func TestArtistRelationshipWilsonScore_SingleUpvote(t *testing.T) {
	r := &ArtistRelationship{}
	score := r.WilsonScore(1, 0)
	// With only 1 vote, Wilson score should be conservative (low)
	assert.Greater(t, score, 0.0)
	assert.Less(t, score, 0.5)
}

func TestArtistRelationshipWilsonScore_FiftyFifty(t *testing.T) {
	r := &ArtistRelationship{}
	score := r.WilsonScore(50, 50)
	// 50/50 split — Wilson lower bound should be below 0.5
	assert.Less(t, score, 0.5)
	assert.Greater(t, score, 0.3)
}

// =============================================================================
// Relationship Type Constants Tests
// =============================================================================

func TestRelationshipTypeConstants(t *testing.T) {
	assert.Equal(t, "similar", RelationshipTypeSimilar)
	assert.Equal(t, "shared_bills", RelationshipTypeSharedBills)
	assert.Equal(t, "shared_label", RelationshipTypeSharedLabel)
	assert.Equal(t, "side_project", RelationshipTypeSideProject)
	assert.Equal(t, "member_of", RelationshipTypeMemberOf)
}
