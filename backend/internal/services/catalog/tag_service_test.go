package catalog

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"gorm.io/gorm"

	apperrors "psychic-homily-backend/internal/errors"
	"psychic-homily-backend/internal/models"
	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/testutil"
)

// =============================================================================
// UNIT TESTS (No Database Required)
// =============================================================================

func TestWilsonScore(t *testing.T) {
	t.Run("zero votes returns 0", func(t *testing.T) {
		score := wilsonScore(0, 0)
		assert.Equal(t, float64(0), score)
	})

	t.Run("all upvotes returns positive", func(t *testing.T) {
		score := wilsonScore(10, 0)
		assert.Greater(t, score, float64(0))
	})

	t.Run("all downvotes returns near zero", func(t *testing.T) {
		score := wilsonScore(0, 10)
		assert.LessOrEqual(t, score, float64(0))
	})

	t.Run("mixed votes returns moderate", func(t *testing.T) {
		score := wilsonScore(7, 3)
		assert.Greater(t, score, float64(0))
		assert.Less(t, score, float64(1))
	})

	t.Run("more votes increases confidence", func(t *testing.T) {
		// 7/10 vs 70/100 — same ratio but more data should give higher lower bound
		scoreLow := wilsonScore(7, 3)
		scoreHigh := wilsonScore(70, 30)
		assert.Greater(t, scoreHigh, scoreLow)
	})
}

// =============================================================================
// INTEGRATION TESTS (With Real Database)
// =============================================================================

type TagServiceIntegrationTestSuite struct {
	suite.Suite
	testDB     *testutil.TestDatabase
	db         *gorm.DB
	tagService *TagService
}

func (suite *TagServiceIntegrationTestSuite) SetupSuite() {
	suite.testDB = testutil.SetupTestPostgres(suite.T())
	suite.db = suite.testDB.DB

	suite.db = suite.testDB.DB
	suite.tagService = NewTagService(suite.testDB.DB)
}

func (suite *TagServiceIntegrationTestSuite) TearDownSuite() {
	suite.testDB.Cleanup()
}

func (suite *TagServiceIntegrationTestSuite) SetupTest() {
	sqlDB, _ := suite.db.DB()
	_, _ = sqlDB.Exec("DELETE FROM tag_votes")
	_, _ = sqlDB.Exec("DELETE FROM entity_tags")
	_, _ = sqlDB.Exec("DELETE FROM tag_aliases")
	_, _ = sqlDB.Exec("DELETE FROM tags")
	_, _ = sqlDB.Exec("DELETE FROM show_artists")
	_, _ = sqlDB.Exec("DELETE FROM show_venues")
	_, _ = sqlDB.Exec("DELETE FROM shows")
	_, _ = sqlDB.Exec("DELETE FROM artists")
	_, _ = sqlDB.Exec("DELETE FROM venues")
	_, _ = sqlDB.Exec("DELETE FROM users")
}

func (suite *TagServiceIntegrationTestSuite) createTestUser(name string) *models.User {
	email := fmt.Sprintf("%s-%d@test.com", name, time.Now().UnixNano())
	user := &models.User{
		Email:         &email,
		FirstName:     &name,
		IsActive:      true,
		EmailVerified: true,
	}
	err := suite.db.Create(user).Error
	suite.Require().NoError(err)
	return user
}

func (suite *TagServiceIntegrationTestSuite) createTag(name, category string) *models.Tag {
	tag, err := suite.tagService.CreateTag(name, nil, nil, category, false, nil)
	suite.Require().NoError(err)
	return tag
}

// createArtist creates a minimal artist for tagging tests
func (suite *TagServiceIntegrationTestSuite) createArtist(name string) uint {
	slug := fmt.Sprintf("%s-%d", name, time.Now().UnixNano())
	artist := &models.Artist{Name: name, Slug: &slug}
	err := suite.db.Create(artist).Error
	suite.Require().NoError(err)
	return artist.ID
}

// ──────────────────────────────────────────────
// CRUD Tests
// ──────────────────────────────────────────────

func (suite *TagServiceIntegrationTestSuite) TestCreateTag_Success() {
	desc := "A subgenre of punk rock"
	tag, err := suite.tagService.CreateTag("Post-Punk", &desc, nil, "genre", true, nil)
	suite.Require().NoError(err)
	suite.Require().NotNil(tag)

	suite.Assert().Equal("Post-Punk", tag.Name)
	suite.Assert().Equal("post-punk", tag.Slug)
	suite.Assert().Equal("genre", tag.Category)
	suite.Assert().True(tag.IsOfficial)
	suite.Assert().NotNil(tag.Description)
	suite.Assert().Equal("A subgenre of punk rock", *tag.Description)
	suite.Assert().Equal(0, tag.UsageCount)
	suite.Assert().Nil(tag.CreatedByUserID)
}

func (suite *TagServiceIntegrationTestSuite) TestCreateTag_InvalidCategory() {
	tag, err := suite.tagService.CreateTag("Test", nil, nil, "invalid", false, nil)
	suite.Assert().Error(err)
	suite.Assert().Contains(err.Error(), "invalid tag category")
	suite.Assert().Nil(tag)
}

func (suite *TagServiceIntegrationTestSuite) TestCreateTag_DuplicateName() {
	suite.createTag("rock", "genre")

	tag, err := suite.tagService.CreateTag("Rock", nil, nil, "genre", false, nil)
	suite.Assert().Error(err)
	var tagErr *apperrors.TagError
	suite.Assert().ErrorAs(err, &tagErr)
	suite.Assert().Equal(apperrors.CodeTagExists, tagErr.Code)
	suite.Assert().Nil(tag)
}

func (suite *TagServiceIntegrationTestSuite) TestCreateTag_WithParent() {
	parent := suite.createTag("rock", "genre")

	child, err := suite.tagService.CreateTag("post-punk", nil, &parent.ID, "genre", false, nil)
	suite.Require().NoError(err)
	suite.Assert().NotNil(child.ParentID)
	suite.Assert().Equal(parent.ID, *child.ParentID)
}

func (suite *TagServiceIntegrationTestSuite) TestCreateTag_WithUserID() {
	user := suite.createTestUser("creator")

	tag, err := suite.tagService.CreateTag("Ambient", nil, nil, "genre", false, &user.ID)
	suite.Require().NoError(err)
	suite.Require().NotNil(tag)
	suite.Assert().NotNil(tag.CreatedByUserID)
	suite.Assert().Equal(user.ID, *tag.CreatedByUserID)

	// Verify persisted and preloaded via GetTag
	fetched, err := suite.tagService.GetTag(tag.ID)
	suite.Require().NoError(err)
	suite.Require().NotNil(fetched)
	suite.Assert().NotNil(fetched.CreatedByUserID)
	suite.Assert().Equal(user.ID, *fetched.CreatedByUserID)
	suite.Assert().NotNil(fetched.CreatedBy)
}

func (suite *TagServiceIntegrationTestSuite) TestCreateTag_NilUserID() {
	tag, err := suite.tagService.CreateTag("NoCreator", nil, nil, "genre", false, nil)
	suite.Require().NoError(err)
	suite.Require().NotNil(tag)
	suite.Assert().Nil(tag.CreatedByUserID)
}

func (suite *TagServiceIntegrationTestSuite) TestGetTag_ByID() {
	created := suite.createTag("electronic", "genre")

	tag, err := suite.tagService.GetTag(created.ID)
	suite.Require().NoError(err)
	suite.Require().NotNil(tag)
	suite.Assert().Equal("electronic", tag.Name)
}

func (suite *TagServiceIntegrationTestSuite) TestGetTag_NotFound() {
	tag, err := suite.tagService.GetTag(99999)
	suite.Assert().NoError(err)
	suite.Assert().Nil(tag)
}

func (suite *TagServiceIntegrationTestSuite) TestGetTagBySlug() {
	suite.createTag("shoegaze", "genre")

	tag, err := suite.tagService.GetTagBySlug("shoegaze")
	suite.Require().NoError(err)
	suite.Require().NotNil(tag)
	suite.Assert().Equal("shoegaze", tag.Name)
}

func (suite *TagServiceIntegrationTestSuite) TestGetTagBySlug_NotFound() {
	tag, err := suite.tagService.GetTagBySlug("nonexistent")
	suite.Assert().NoError(err)
	suite.Assert().Nil(tag)
}

func (suite *TagServiceIntegrationTestSuite) TestListTags_All() {
	suite.createTag("rock", "genre")
	suite.createTag("jazz", "genre")
	suite.createTag("1990s", "other")

	tags, total, err := suite.tagService.ListTags("", "", nil, "name", 50, 0)
	suite.Require().NoError(err)
	suite.Assert().Equal(int64(3), total)
	suite.Assert().Len(tags, 3)
}

func (suite *TagServiceIntegrationTestSuite) TestListTags_FilterByCategory() {
	suite.createTag("rock", "genre")
	suite.createTag("jazz", "genre")
	suite.createTag("1990s", "other")

	tags, total, err := suite.tagService.ListTags("genre", "", nil, "name", 50, 0)
	suite.Require().NoError(err)
	suite.Assert().Equal(int64(2), total)
	suite.Assert().Len(tags, 2)
}

func (suite *TagServiceIntegrationTestSuite) TestListTags_Search() {
	suite.createTag("post-punk", "genre")
	suite.createTag("post-rock", "genre")
	suite.createTag("jazz", "genre")

	tags, total, err := suite.tagService.ListTags("", "post", nil, "name", 50, 0)
	suite.Require().NoError(err)
	suite.Assert().Equal(int64(2), total)
	suite.Assert().Len(tags, 2)
}

func (suite *TagServiceIntegrationTestSuite) TestListTags_FilterByParent() {
	parent := suite.createTag("rock", "genre")
	suite.createTag("post-punk", "genre")
	// Make post-rock a child of rock
	suite.tagService.CreateTag("post-rock", nil, &parent.ID, "genre", false, nil)

	tags, total, err := suite.tagService.ListTags("", "", &parent.ID, "name", 50, 0)
	suite.Require().NoError(err)
	suite.Assert().Equal(int64(1), total)
	suite.Assert().Len(tags, 1)
	suite.Assert().Equal("post-rock", tags[0].Name)
}

func (suite *TagServiceIntegrationTestSuite) TestUpdateTag_Success() {
	tag := suite.createTag("typo-tag", "genre")

	newName := "corrected-tag"
	newCategory := "locale"
	updated, err := suite.tagService.UpdateTag(tag.ID, &newName, nil, nil, &newCategory, nil)
	suite.Require().NoError(err)
	suite.Assert().Equal("corrected-tag", updated.Name)
	suite.Assert().Equal("locale", updated.Category)
}

func (suite *TagServiceIntegrationTestSuite) TestUpdateTag_NotFound() {
	name := "test"
	_, err := suite.tagService.UpdateTag(99999, &name, nil, nil, nil, nil)
	suite.Assert().Error(err)
	var tagErr *apperrors.TagError
	suite.Assert().ErrorAs(err, &tagErr)
	suite.Assert().Equal(apperrors.CodeTagNotFound, tagErr.Code)
}

func (suite *TagServiceIntegrationTestSuite) TestDeleteTag_Success() {
	tag := suite.createTag("to-delete", "genre")

	err := suite.tagService.DeleteTag(tag.ID)
	suite.Assert().NoError(err)

	// Verify deleted
	found, _ := suite.tagService.GetTag(tag.ID)
	suite.Assert().Nil(found)
}

func (suite *TagServiceIntegrationTestSuite) TestDeleteTag_NotFound() {
	err := suite.tagService.DeleteTag(99999)
	suite.Assert().Error(err)
	var tagErr *apperrors.TagError
	suite.Assert().ErrorAs(err, &tagErr)
	suite.Assert().Equal(apperrors.CodeTagNotFound, tagErr.Code)
}

// ──────────────────────────────────────────────
// Entity Tagging Tests
// ──────────────────────────────────────────────

func (suite *TagServiceIntegrationTestSuite) TestAddTagToEntity_ByID() {
	user := suite.createTestUser("tagger")
	tag := suite.createTag("indie", "genre")
	artistID := suite.createArtist("Test Band")

	et, err := suite.tagService.AddTagToEntity(tag.ID, "", "artist", artistID, user.ID, "")
	suite.Require().NoError(err)
	suite.Assert().Equal(tag.ID, et.TagID)
	suite.Assert().Equal("artist", et.EntityType)
	suite.Assert().Equal(artistID, et.EntityID)
	suite.Assert().Equal(user.ID, et.AddedByUserID)

	// Verify usage count incremented
	updated, _ := suite.tagService.GetTag(tag.ID)
	suite.Assert().Equal(1, updated.UsageCount)
}

func (suite *TagServiceIntegrationTestSuite) TestAddTagToEntity_ByName() {
	user := suite.createTestUser("tagger")
	tag := suite.createTag("ambient", "genre")
	artistID := suite.createArtist("Ambient Artist")

	et, err := suite.tagService.AddTagToEntity(0, "ambient", "artist", artistID, user.ID, "")
	suite.Require().NoError(err)
	suite.Assert().Equal(tag.ID, et.TagID)
}

func (suite *TagServiceIntegrationTestSuite) TestAddTagToEntity_ByAlias() {
	user := suite.createTestUser("tagger")
	tag := suite.createTag("post-punk", "genre")
	_, err := suite.tagService.CreateAlias(tag.ID, "post punk")
	suite.Require().NoError(err)
	artistID := suite.createArtist("Post Punk Band")

	et, err := suite.tagService.AddTagToEntity(0, "post punk", "artist", artistID, user.ID, "")
	suite.Require().NoError(err)
	suite.Assert().Equal(tag.ID, et.TagID)
}

func (suite *TagServiceIntegrationTestSuite) TestAddTagToEntity_Duplicate() {
	user := suite.createTestUser("tagger")
	tag := suite.createTag("rock", "genre")
	artistID := suite.createArtist("Rock Band")

	_, err := suite.tagService.AddTagToEntity(tag.ID, "", "artist", artistID, user.ID, "")
	suite.Require().NoError(err)

	_, err = suite.tagService.AddTagToEntity(tag.ID, "", "artist", artistID, user.ID, "")
	suite.Assert().Error(err)
	var tagErr *apperrors.TagError
	suite.Assert().ErrorAs(err, &tagErr)
	suite.Assert().Equal(apperrors.CodeEntityTagExists, tagErr.Code)
}

func (suite *TagServiceIntegrationTestSuite) TestAddTagToEntity_InvalidEntityType() {
	user := suite.createTestUser("tagger")
	tag := suite.createTag("rock", "genre")

	_, err := suite.tagService.AddTagToEntity(tag.ID, "", "invalid", 1, user.ID, "")
	suite.Assert().Error(err)
	suite.Assert().Contains(err.Error(), "invalid entity type")
}

func (suite *TagServiceIntegrationTestSuite) TestRemoveTagFromEntity_Success() {
	user := suite.createTestUser("tagger")
	tag := suite.createTag("metal", "genre")
	artistID := suite.createArtist("Metal Band")

	_, err := suite.tagService.AddTagToEntity(tag.ID, "", "artist", artistID, user.ID, "")
	suite.Require().NoError(err)

	err = suite.tagService.RemoveTagFromEntity(tag.ID, "artist", artistID)
	suite.Assert().NoError(err)

	// Verify usage count decremented
	updated, _ := suite.tagService.GetTag(tag.ID)
	suite.Assert().Equal(0, updated.UsageCount)
}

func (suite *TagServiceIntegrationTestSuite) TestRemoveTagFromEntity_NotFound() {
	err := suite.tagService.RemoveTagFromEntity(99999, "artist", 99999)
	suite.Assert().Error(err)
	var tagErr *apperrors.TagError
	suite.Assert().ErrorAs(err, &tagErr)
	suite.Assert().Equal(apperrors.CodeEntityTagNotFound, tagErr.Code)
}

func (suite *TagServiceIntegrationTestSuite) TestListEntityTags() {
	user := suite.createTestUser("tagger")
	tag1 := suite.createTag("indie", "genre")
	tag2 := suite.createTag("lo-fi", "other")
	artistID := suite.createArtist("Indie Lo-Fi Band")

	suite.tagService.AddTagToEntity(tag1.ID, "", "artist", artistID, user.ID, "")
	suite.tagService.AddTagToEntity(tag2.ID, "", "artist", artistID, user.ID, "")

	tags, err := suite.tagService.ListEntityTags("artist", artistID, 0)
	suite.Require().NoError(err)
	suite.Assert().Len(tags, 2)
}

func (suite *TagServiceIntegrationTestSuite) TestListEntityTags_WithUserVote() {
	user := suite.createTestUser("voter")
	tag := suite.createTag("punk", "genre")
	artistID := suite.createArtist("Punk Band")

	suite.tagService.AddTagToEntity(tag.ID, "", "artist", artistID, user.ID, "")
	suite.tagService.VoteOnTag(tag.ID, "artist", artistID, user.ID, true)

	tags, err := suite.tagService.ListEntityTags("artist", artistID, user.ID)
	suite.Require().NoError(err)
	suite.Require().Len(tags, 1)
	suite.Assert().NotNil(tags[0].UserVote)
	suite.Assert().Equal(1, *tags[0].UserVote)
	suite.Assert().Equal(1, tags[0].Upvotes)
}

// PSY-479: ListEntityTags must surface AddedByUserID, AddedByUsername, and
// AddedAt so the entity tag pill hover card can show "Added by @user · 5m ago".
// Username is *string so the frontend can distinguish "user has no username"
// (older accounts that never set one — render "Source: system seed") from a
// real attribution. AddedByUserID and AddedAt are *time.Time / *uint with
// no omitempty on the JSON tag, so they always serialize (null vs value)
// instead of disappearing from the response when zero.
func (suite *TagServiceIntegrationTestSuite) TestListEntityTags_AttributionWithUsername() {
	user := suite.createTestUserWithUsername("attributed", "attributed")
	tag := suite.createTag("noise-rock", "genre")
	artistID := suite.createArtist("Noise Rock Band")

	beforeAdd := time.Now().Add(-1 * time.Second)
	_, err := suite.tagService.AddTagToEntity(tag.ID, "", "artist", artistID, user.ID, "")
	suite.Require().NoError(err)

	tags, err := suite.tagService.ListEntityTags("artist", artistID, 0)
	suite.Require().NoError(err)
	suite.Require().Len(tags, 1)

	suite.Require().NotNil(tags[0].AddedByUserID, "AddedByUserID must be populated")
	suite.Assert().Equal(user.ID, *tags[0].AddedByUserID)

	suite.Require().NotNil(tags[0].AddedByUsername, "AddedByUsername must be populated when user has a username")
	suite.Assert().Equal("attributed", *tags[0].AddedByUsername)

	suite.Require().NotNil(tags[0].AddedAt, "AddedAt must be populated")
	suite.Assert().True(tags[0].AddedAt.After(beforeAdd), "AddedAt should reflect when the tag was applied")
}

// Some users (older accounts, OAuth flows that didn't enforce username) have
// username=null. ListEntityTags still surfaces AddedByUserID + AddedAt so the
// frontend can render a "Source: system seed" provenance line; AddedByUsername
// is null so the frontend doesn't display "Added by @undefined".
func (suite *TagServiceIntegrationTestSuite) TestListEntityTags_AttributionNullUsername() {
	// createTestUser does NOT set a username (this matches the seed/fixture
	// behavior the dogfood report flagged).
	user := suite.createTestUser("anon")
	tag := suite.createTag("dream-pop", "genre")
	artistID := suite.createArtist("Dream Pop Band")

	_, err := suite.tagService.AddTagToEntity(tag.ID, "", "artist", artistID, user.ID, "")
	suite.Require().NoError(err)

	tags, err := suite.tagService.ListEntityTags("artist", artistID, 0)
	suite.Require().NoError(err)
	suite.Require().Len(tags, 1)

	suite.Require().NotNil(tags[0].AddedByUserID, "AddedByUserID is always populated even when username is null")
	suite.Assert().Equal(user.ID, *tags[0].AddedByUserID)

	suite.Require().NotNil(tags[0].AddedAt, "AddedAt is always populated")

	suite.Assert().Nil(tags[0].AddedByUsername, "AddedByUsername must be nil when the user has no username")
}

// ──────────────────────────────────────────────
// Voting Tests
// ──────────────────────────────────────────────

func (suite *TagServiceIntegrationTestSuite) TestVoteOnTag_Upvote() {
	user := suite.createTestUser("voter")
	tag := suite.createTag("synth", "genre")
	artistID := suite.createArtist("Synth Band")

	suite.tagService.AddTagToEntity(tag.ID, "", "artist", artistID, user.ID, "")

	err := suite.tagService.VoteOnTag(tag.ID, "artist", artistID, user.ID, true)
	suite.Assert().NoError(err)
}

func (suite *TagServiceIntegrationTestSuite) TestVoteOnTag_ChangeVote() {
	user := suite.createTestUser("voter")
	tag := suite.createTag("grunge", "genre")
	artistID := suite.createArtist("Grunge Band")

	suite.tagService.AddTagToEntity(tag.ID, "", "artist", artistID, user.ID, "")
	suite.tagService.VoteOnTag(tag.ID, "artist", artistID, user.ID, true)

	// Change to downvote
	err := suite.tagService.VoteOnTag(tag.ID, "artist", artistID, user.ID, false)
	suite.Assert().NoError(err)

	// Verify vote changed
	tags, _ := suite.tagService.ListEntityTags("artist", artistID, user.ID)
	suite.Require().Len(tags, 1)
	suite.Assert().NotNil(tags[0].UserVote)
	suite.Assert().Equal(-1, *tags[0].UserVote)
}

func (suite *TagServiceIntegrationTestSuite) TestVoteOnTag_TagNotApplied() {
	user := suite.createTestUser("voter")
	tag := suite.createTag("doom", "genre")

	err := suite.tagService.VoteOnTag(tag.ID, "artist", 99999, user.ID, true)
	suite.Assert().Error(err)
	var tagErr *apperrors.TagError
	suite.Assert().ErrorAs(err, &tagErr)
	suite.Assert().Equal(apperrors.CodeEntityTagNotFound, tagErr.Code)
}

func (suite *TagServiceIntegrationTestSuite) TestRemoveTagVote() {
	user := suite.createTestUser("voter")
	tag := suite.createTag("noise", "genre")
	artistID := suite.createArtist("Noise Band")

	suite.tagService.AddTagToEntity(tag.ID, "", "artist", artistID, user.ID, "")
	suite.tagService.VoteOnTag(tag.ID, "artist", artistID, user.ID, true)

	err := suite.tagService.RemoveTagVote(tag.ID, "artist", artistID, user.ID)
	suite.Assert().NoError(err)

	// Verify vote removed
	tags, _ := suite.tagService.ListEntityTags("artist", artistID, user.ID)
	suite.Require().Len(tags, 1)
	suite.Assert().Nil(tags[0].UserVote)
}

// ──────────────────────────────────────────────
// Alias Tests
// ──────────────────────────────────────────────

func (suite *TagServiceIntegrationTestSuite) TestCreateAlias_Success() {
	tag := suite.createTag("post-punk", "genre")

	alias, err := suite.tagService.CreateAlias(tag.ID, "post punk")
	suite.Require().NoError(err)
	suite.Assert().Equal("post punk", alias.Alias)
	suite.Assert().Equal(tag.ID, alias.TagID)
}

func (suite *TagServiceIntegrationTestSuite) TestCreateAlias_Duplicate() {
	tag := suite.createTag("hip-hop", "genre")
	suite.tagService.CreateAlias(tag.ID, "hiphop")

	_, err := suite.tagService.CreateAlias(tag.ID, "HipHop")
	suite.Assert().Error(err)
	var tagErr *apperrors.TagError
	suite.Assert().ErrorAs(err, &tagErr)
	suite.Assert().Equal(apperrors.CodeTagAliasExists, tagErr.Code)
}

func (suite *TagServiceIntegrationTestSuite) TestCreateAlias_ConflictsWithTagName() {
	suite.createTag("electronic", "genre")
	tag2 := suite.createTag("techno", "genre")

	_, err := suite.tagService.CreateAlias(tag2.ID, "electronic")
	suite.Assert().Error(err)
	var tagErr *apperrors.TagError
	suite.Assert().ErrorAs(err, &tagErr)
	suite.Assert().Equal(apperrors.CodeTagAliasExists, tagErr.Code)
}

func (suite *TagServiceIntegrationTestSuite) TestCreateAlias_TagNotFound() {
	_, err := suite.tagService.CreateAlias(99999, "test")
	suite.Assert().Error(err)
	var tagErr *apperrors.TagError
	suite.Assert().ErrorAs(err, &tagErr)
	suite.Assert().Equal(apperrors.CodeTagNotFound, tagErr.Code)
}

func (suite *TagServiceIntegrationTestSuite) TestDeleteAlias() {
	tag := suite.createTag("rnb", "genre")
	alias, _ := suite.tagService.CreateAlias(tag.ID, "r&b")

	err := suite.tagService.DeleteAlias(alias.ID)
	suite.Assert().NoError(err)
}

func (suite *TagServiceIntegrationTestSuite) TestListAliases() {
	tag := suite.createTag("post-punk", "genre")
	suite.tagService.CreateAlias(tag.ID, "post punk")
	suite.tagService.CreateAlias(tag.ID, "postpunk")

	aliases, err := suite.tagService.ListAliases(tag.ID)
	suite.Require().NoError(err)
	suite.Assert().Len(aliases, 2)
}

func (suite *TagServiceIntegrationTestSuite) TestResolveAlias_Found() {
	tag := suite.createTag("post-punk", "genre")
	suite.tagService.CreateAlias(tag.ID, "post punk")

	resolved, err := suite.tagService.ResolveAlias("Post Punk")
	suite.Require().NoError(err)
	suite.Require().NotNil(resolved)
	suite.Assert().Equal(tag.ID, resolved.ID)
}

func (suite *TagServiceIntegrationTestSuite) TestResolveAlias_NotFound() {
	resolved, err := suite.tagService.ResolveAlias("nonexistent")
	suite.Assert().NoError(err)
	suite.Assert().Nil(resolved)
}

// ──────────────────────────────────────────────
// Utility Tests
// ──────────────────────────────────────────────

func (suite *TagServiceIntegrationTestSuite) TestSearchTags_ByName() {
	suite.createTag("post-punk", "genre")
	suite.createTag("post-rock", "genre")
	suite.createTag("jazz", "genre")

	results, err := suite.tagService.SearchTags("post", 10, "")
	suite.Require().NoError(err)
	suite.Assert().Len(results, 2)
	// Name-match results should not report any alias provenance.
	for _, r := range results {
		suite.Assert().Empty(r.MatchedAlias, "expected no MatchedAlias for name-match row %q", r.Tag.Name)
	}
}

func (suite *TagServiceIntegrationTestSuite) TestSearchTags_ByAlias() {
	tag := suite.createTag("post-punk", "genre")
	_, err := suite.tagService.CreateAlias(tag.ID, "post punk revival")
	suite.Require().NoError(err)

	results, err := suite.tagService.SearchTags("revival", 10, "")
	suite.Require().NoError(err)
	suite.Assert().Len(results, 1)
	suite.Assert().Equal(tag.ID, results[0].Tag.ID)
	// Alias-match should surface which alias produced the match.
	suite.Assert().Equal("post punk revival", results[0].MatchedAlias)
}

func (suite *TagServiceIntegrationTestSuite) TestSearchTags_FilterByCategory() {
	suite.createTag("post-punk", "genre")
	suite.createTag("post-office", "other")
	suite.createTag("portland", "locale")

	// Without category filter: "post" matches post-punk and post-office
	results, err := suite.tagService.SearchTags("post", 10, "")
	suite.Require().NoError(err)
	suite.Assert().Len(results, 2)

	// With genre filter: only post-punk
	results, err = suite.tagService.SearchTags("post", 10, "genre")
	suite.Require().NoError(err)
	suite.Assert().Len(results, 1)
	suite.Assert().Equal("post-punk", results[0].Tag.Name)

	// With locale filter: "port" matches portland only
	results, err = suite.tagService.SearchTags("port", 10, "locale")
	suite.Require().NoError(err)
	suite.Assert().Len(results, 1)
	suite.Assert().Equal("portland", results[0].Tag.Name)
}

// TestSearchTags_AliasTransparency covers PSY-442 — the autocomplete response
// must surface which alias produced a match so the UI can tell the user
// "you typed `punk-rock`, using `punk`". Queries that hit the canonical
// name stay silent so the caption only fires when it's useful.
func (suite *TagServiceIntegrationTestSuite) TestSearchTags_AliasTransparency_AliasMatch() {
	tag := suite.createTag("punk", "genre")
	_, err := suite.tagService.CreateAlias(tag.ID, "punk-rock")
	suite.Require().NoError(err)

	results, err := suite.tagService.SearchTags("punk-rock", 10, "")
	suite.Require().NoError(err)
	suite.Require().Len(results, 1)
	suite.Assert().Equal(tag.ID, results[0].Tag.ID)
	suite.Assert().Equal("punk", results[0].Tag.Name)
	suite.Assert().Equal("punk-rock", results[0].MatchedAlias,
		"alias match should carry the specific alias that matched")
}

func (suite *TagServiceIntegrationTestSuite) TestSearchTags_AliasTransparency_NameMatchHasNoProvenance() {
	tag := suite.createTag("punk", "genre")
	// Add an alias that also happens to contain the query substring — the
	// canonical name match still wins and we should not emit MatchedAlias.
	_, err := suite.tagService.CreateAlias(tag.ID, "punks")
	suite.Require().NoError(err)

	results, err := suite.tagService.SearchTags("punk", 10, "")
	suite.Require().NoError(err)
	suite.Require().Len(results, 1)
	suite.Assert().Equal(tag.ID, results[0].Tag.ID)
	suite.Assert().Empty(results[0].MatchedAlias,
		"when the tag name matches directly the canonical form is the signal")
}

func (suite *TagServiceIntegrationTestSuite) TestSearchTags_AliasTransparency_DeterministicWhenMultipleAliasesMatch() {
	tag := suite.createTag("drum-and-bass", "genre")
	_, err := suite.tagService.CreateAlias(tag.ID, "zeta-alias")
	suite.Require().NoError(err)
	_, err = suite.tagService.CreateAlias(tag.ID, "alpha-alias")
	suite.Require().NoError(err)

	// Both aliases contain "alias"; either is a defensible choice, but the
	// service must pick one deterministically (alphabetical) so UI behaviour
	// is stable across runs. "alpha-alias" comes before "zeta-alias" under
	// any reasonable collation.
	results, err := suite.tagService.SearchTags("alias", 10, "")
	suite.Require().NoError(err)
	suite.Require().Len(results, 1)
	suite.Assert().Equal(tag.ID, results[0].Tag.ID)
	suite.Assert().Equal("alpha-alias", results[0].MatchedAlias)
}

func (suite *TagServiceIntegrationTestSuite) TestSearchTags_AliasTransparency_MixedResults() {
	// One tag matches by name, one by alias — each row should carry the
	// correct provenance independently.
	nameMatch := suite.createTag("electro-punk", "genre")
	aliasTag := suite.createTag("punk", "genre")
	_, err := suite.tagService.CreateAlias(aliasTag.ID, "punk-rock")
	suite.Require().NoError(err)

	results, err := suite.tagService.SearchTags("punk", 10, "")
	suite.Require().NoError(err)
	suite.Require().Len(results, 2)

	byID := make(map[uint]string, len(results))
	for _, r := range results {
		byID[r.Tag.ID] = r.MatchedAlias
	}

	// The electro-punk tag matched by name.
	suite.Assert().Empty(byID[nameMatch.ID])
	// The punk tag also matched by name ("punk" is a substring of "punk"),
	// not by alias — even though the alias also contains the query.
	suite.Assert().Empty(byID[aliasTag.ID])
}

func (suite *TagServiceIntegrationTestSuite) TestGetTrendingTags() {
	user := suite.createTestUser("tagger")
	tag1 := suite.createTag("rock", "genre")
	tag2 := suite.createTag("jazz", "genre")
	artistID := suite.createArtist("Multi-Genre")

	// Apply tags to entities to increase usage count
	suite.tagService.AddTagToEntity(tag1.ID, "", "artist", artistID, user.ID, "")
	// tag2 not applied, so usage_count stays 0

	_ = tag2

	tags, err := suite.tagService.GetTrendingTags(10, "")
	suite.Require().NoError(err)
	suite.Assert().Len(tags, 1) // Only tag1 has usage_count > 0
	suite.Assert().Equal(tag1.ID, tags[0].ID)
}

func (suite *TagServiceIntegrationTestSuite) TestGetTrendingTags_FilterByCategory() {
	user := suite.createTestUser("tagger")
	tag1 := suite.createTag("rock", "genre")
	tag2 := suite.createTag("1990s", "other")
	artistID := suite.createArtist("Band")

	suite.tagService.AddTagToEntity(tag1.ID, "", "artist", artistID, user.ID, "")
	artist2 := suite.createArtist("Band2")
	suite.tagService.AddTagToEntity(tag2.ID, "", "artist", artist2, user.ID, "")

	tags, err := suite.tagService.GetTrendingTags(10, "other")
	suite.Require().NoError(err)
	suite.Assert().Len(tags, 1)
	suite.Assert().Equal("1990s", tags[0].Name)
}

func (suite *TagServiceIntegrationTestSuite) TestPruneDownvotedTags() {
	user1 := suite.createTestUser("voter1")
	user2 := suite.createTestUser("voter2")
	tag := suite.createTag("questionable", "genre")
	artistID := suite.createArtist("Some Band")

	// Apply tag and get downvoted
	suite.tagService.AddTagToEntity(tag.ID, "", "artist", artistID, user1.ID, "")
	suite.tagService.VoteOnTag(tag.ID, "artist", artistID, user1.ID, false)
	suite.tagService.VoteOnTag(tag.ID, "artist", artistID, user2.ID, false)

	pruned, err := suite.tagService.PruneDownvotedTags()
	suite.Require().NoError(err)
	suite.Assert().Equal(int64(1), pruned)

	// Verify entity_tag was removed
	tags, _ := suite.tagService.ListEntityTags("artist", artistID, 0)
	suite.Assert().Len(tags, 0)
}

func (suite *TagServiceIntegrationTestSuite) TestPruneDownvotedTags_OfficialImmune() {
	user1 := suite.createTestUser("voter1")
	user2 := suite.createTestUser("voter2")
	tag, _ := suite.tagService.CreateTag("official-tag", nil, nil, "genre", true, nil)
	artistID := suite.createArtist("Some Official Band")

	suite.tagService.AddTagToEntity(tag.ID, "", "artist", artistID, user1.ID, "")
	suite.tagService.VoteOnTag(tag.ID, "artist", artistID, user1.ID, false)
	suite.tagService.VoteOnTag(tag.ID, "artist", artistID, user2.ID, false)

	pruned, err := suite.tagService.PruneDownvotedTags()
	suite.Require().NoError(err)
	suite.Assert().Equal(int64(0), pruned)

	// Official tag still applied
	tags, _ := suite.tagService.ListEntityTags("artist", artistID, 0)
	suite.Assert().Len(tags, 1)
}

// ──────────────────────────────────────────────
// Inline Tag Creation Tests
// ──────────────────────────────────────────────

func (suite *TagServiceIntegrationTestSuite) createTestUserWithTier(name, tier string) *models.User {
	user := suite.createTestUser(name)
	suite.db.Model(user).Update("user_tier", tier)
	user.UserTier = tier
	return user
}

func (suite *TagServiceIntegrationTestSuite) createAdminUser(name string) *models.User {
	user := suite.createTestUser(name)
	suite.db.Model(user).Updates(map[string]interface{}{"is_admin": true, "user_tier": "new_user"})
	user.IsAdmin = true
	return user
}

func (suite *TagServiceIntegrationTestSuite) TestAddTagToEntity_InlineCreate_ContributorSuccess() {
	user := suite.createTestUserWithTier("contributor-user", "contributor")
	artistID := suite.createArtist("Shoegaze Band")

	et, err := suite.tagService.AddTagToEntity(0, "shoegaze-revival", "artist", artistID, user.ID, "genre")
	suite.Require().NoError(err)
	suite.Assert().NotNil(et)

	// Verify the tag was created
	tag, err := suite.tagService.GetTagBySlug("shoegaze-revival")
	suite.Require().NoError(err)
	suite.Require().NotNil(tag)
	suite.Assert().Equal("shoegaze-revival", tag.Name)
	suite.Assert().Equal("genre", tag.Category)
	suite.Assert().False(tag.IsOfficial)
	suite.Assert().Equal(1, tag.UsageCount)

	// Verify entity_tag was created
	suite.Assert().Equal(tag.ID, et.TagID)
	suite.Assert().Equal("artist", et.EntityType)
	suite.Assert().Equal(artistID, et.EntityID)

	// Verify auto-upvote
	tags, err := suite.tagService.ListEntityTags("artist", artistID, user.ID)
	suite.Require().NoError(err)
	suite.Require().Len(tags, 1)
	suite.Assert().NotNil(tags[0].UserVote)
	suite.Assert().Equal(1, *tags[0].UserVote)
	suite.Assert().Equal(1, tags[0].Upvotes)
}

func (suite *TagServiceIntegrationTestSuite) TestAddTagToEntity_InlineCreate_NewUserForbidden() {
	user := suite.createTestUserWithTier("new-user", "new_user")
	artistID := suite.createArtist("Some Band")

	_, err := suite.tagService.AddTagToEntity(0, "brand-new-tag", "artist", artistID, user.ID, "genre")
	suite.Require().Error(err)

	var tagErr *apperrors.TagError
	suite.Assert().ErrorAs(err, &tagErr)
	suite.Assert().Equal(apperrors.CodeTagCreationForbidden, tagErr.Code)
	suite.Assert().Contains(tagErr.Message, "Contributor tier")
}

func (suite *TagServiceIntegrationTestSuite) TestAddTagToEntity_InlineCreate_AdminSuccess() {
	admin := suite.createAdminUser("admin-tagger")
	artistID := suite.createArtist("Admin Band")

	et, err := suite.tagService.AddTagToEntity(0, "admin-created-tag", "artist", artistID, admin.ID, "other")
	suite.Require().NoError(err)
	suite.Assert().NotNil(et)

	// Admin can create tags even with new_user tier because they're admin
	tag, err := suite.tagService.GetTagBySlug("admin-created-tag")
	suite.Require().NoError(err)
	suite.Require().NotNil(tag)
	suite.Assert().Equal("admin-created-tag", tag.Name)
	suite.Assert().Equal("other", tag.Category)
}

func (suite *TagServiceIntegrationTestSuite) TestAddTagToEntity_InlineCreate_TrustedContributorSuccess() {
	user := suite.createTestUserWithTier("trusted-user", "trusted_contributor")
	artistID := suite.createArtist("Trusted Band")

	et, err := suite.tagService.AddTagToEntity(0, "trusted-tag", "artist", artistID, user.ID, "locale")
	suite.Require().NoError(err)
	suite.Assert().NotNil(et)

	tag, err := suite.tagService.GetTagBySlug("trusted-tag")
	suite.Require().NoError(err)
	suite.Require().NotNil(tag)
	suite.Assert().Equal("locale", tag.Category)
}

func (suite *TagServiceIntegrationTestSuite) TestAddTagToEntity_InlineCreate_ExistingTagByName_NoDuplicate() {
	user := suite.createTestUserWithTier("contributor", "contributor")
	suite.createTag("ambient", "genre")
	artistID := suite.createArtist("Ambient Band")

	// This should use the existing "ambient" tag, not create a new one
	et, err := suite.tagService.AddTagToEntity(0, "ambient", "artist", artistID, user.ID, "genre")
	suite.Require().NoError(err)
	suite.Assert().NotNil(et)

	// Verify only one tag with that name exists
	tags, total, err := suite.tagService.ListTags("", "ambient", nil, "name", 50, 0)
	suite.Require().NoError(err)
	suite.Assert().Equal(int64(1), total)
	suite.Assert().Len(tags, 1)
}

func (suite *TagServiceIntegrationTestSuite) TestAddTagToEntity_InlineCreate_AliasResolution() {
	user := suite.createTestUserWithTier("contributor", "contributor")
	tag := suite.createTag("post-punk", "genre")
	_, err := suite.tagService.CreateAlias(tag.ID, "post punk")
	suite.Require().NoError(err)
	artistID := suite.createArtist("Post Punk Band 2")

	// "post punk" should resolve to existing "post-punk" via alias
	et, err := suite.tagService.AddTagToEntity(0, "post punk", "artist", artistID, user.ID, "genre")
	suite.Require().NoError(err)
	suite.Assert().Equal(tag.ID, et.TagID)
}

func (suite *TagServiceIntegrationTestSuite) TestAddTagToEntity_InlineCreate_DefaultCategoryOther() {
	user := suite.createTestUserWithTier("contributor", "contributor")
	artistID := suite.createArtist("Category Test Band")

	// Empty category should default to "other"
	et, err := suite.tagService.AddTagToEntity(0, "no-category-tag", "artist", artistID, user.ID, "")
	suite.Require().NoError(err)
	suite.Assert().NotNil(et)

	tag, err := suite.tagService.GetTagBySlug("no-category-tag")
	suite.Require().NoError(err)
	suite.Require().NotNil(tag)
	suite.Assert().Equal("other", tag.Category)
}

func (suite *TagServiceIntegrationTestSuite) TestAddTagToEntity_InlineCreate_WithCategory() {
	user := suite.createTestUserWithTier("contributor", "contributor")
	artistID := suite.createArtist("Genre Band")

	et, err := suite.tagService.AddTagToEntity(0, "darkwave", "artist", artistID, user.ID, "genre")
	suite.Require().NoError(err)
	suite.Assert().NotNil(et)

	tag, err := suite.tagService.GetTagBySlug("darkwave")
	suite.Require().NoError(err)
	suite.Require().NotNil(tag)
	suite.Assert().Equal("genre", tag.Category)
}

func (suite *TagServiceIntegrationTestSuite) TestAddTagToEntity_InlineCreate_TooShortName() {
	user := suite.createTestUserWithTier("contributor", "contributor")
	artistID := suite.createArtist("Short Tag Band")

	// Single character should fail after normalization
	_, err := suite.tagService.AddTagToEntity(0, "x", "artist", artistID, user.ID, "genre")
	suite.Require().Error(err)

	var tagErr *apperrors.TagError
	suite.Assert().ErrorAs(err, &tagErr)
	suite.Assert().Equal(apperrors.CodeTagNameInvalid, tagErr.Code)
}

func (suite *TagServiceIntegrationTestSuite) TestAddTagToEntity_InlineCreate_EmptyAfterNormalization() {
	user := suite.createTestUserWithTier("contributor", "contributor")
	artistID := suite.createArtist("Empty Tag Band")

	// Only special characters — normalizes to empty
	_, err := suite.tagService.AddTagToEntity(0, "!!!@@@", "artist", artistID, user.ID, "genre")
	suite.Require().Error(err)

	var tagErr *apperrors.TagError
	suite.Assert().ErrorAs(err, &tagErr)
	suite.Assert().Equal(apperrors.CodeTagNameInvalid, tagErr.Code)
}

// ──────────────────────────────────────────────
// GetTagDetail Tests
// ──────────────────────────────────────────────

// createTestUserWithUsername creates a user with a specific username for attribution tests.
func (suite *TagServiceIntegrationTestSuite) createTestUserWithUsername(name, username string) *models.User {
	user := suite.createTestUser(name)
	suite.db.Model(user).Update("username", username)
	user.Username = &username
	return user
}

// createVenue creates a minimal venue for breakdown tests.
func (suite *TagServiceIntegrationTestSuite) createVenue(name string) uint {
	slug := fmt.Sprintf("%s-%d", name, time.Now().UnixNano())
	venue := &models.Venue{Name: name, Slug: &slug, City: "Phoenix", State: "AZ"}
	err := suite.db.Create(venue).Error
	suite.Require().NoError(err)
	return venue.ID
}

func (suite *TagServiceIntegrationTestSuite) TestGetTagDetail_NotFound() {
	resp, err := suite.tagService.GetTagDetail(9999)
	suite.Require().NoError(err)
	suite.Assert().Nil(resp)
}

func (suite *TagServiceIntegrationTestSuite) TestGetTagDetail_Minimal() {
	// Bare tag: no description, no parent, no children, no usage, no creator, no related.
	tag := suite.createTag("empty-tag", "genre")

	resp, err := suite.tagService.GetTagDetail(tag.ID)
	suite.Require().NoError(err)
	suite.Require().NotNil(resp)

	suite.Assert().Equal(tag.ID, resp.ID)
	suite.Assert().Equal("empty-tag", resp.Name)
	suite.Assert().Equal("genre", resp.Category)
	suite.Assert().Equal(0, resp.UsageCount)
	suite.Assert().Equal("", resp.DescriptionHTML)
	suite.Assert().Nil(resp.Parent)
	suite.Assert().Empty(resp.Children)
	suite.Assert().Nil(resp.CreatedBy)
	suite.Assert().Empty(resp.Aliases)
	suite.Assert().Empty(resp.TopContributors)
	suite.Assert().Empty(resp.RelatedTags)

	// Usage breakdown is always fully populated for every valid entity type (zero counts).
	suite.Assert().Len(resp.UsageBreakdown, len(models.TagEntityTypes))
	for _, et := range models.TagEntityTypes {
		count, ok := resp.UsageBreakdown[et]
		suite.Assert().True(ok, "breakdown missing entity type %s", et)
		suite.Assert().Equal(int64(0), count)
	}
}

func (suite *TagServiceIntegrationTestSuite) TestGetTagDetail_DescriptionRenderedAsHTML() {
	desc := "This is **bold** and *italic* with a [link](https://example.com)."
	tag, err := suite.tagService.CreateTag("with-desc", &desc, nil, "genre", false, nil)
	suite.Require().NoError(err)

	resp, err := suite.tagService.GetTagDetail(tag.ID)
	suite.Require().NoError(err)
	suite.Require().NotNil(resp)

	suite.Require().NotNil(resp.Description)
	suite.Assert().Equal(desc, *resp.Description)
	suite.Assert().Contains(resp.DescriptionHTML, "<strong>bold</strong>")
	suite.Assert().Contains(resp.DescriptionHTML, "<em>italic</em>")
	suite.Assert().Contains(resp.DescriptionHTML, `href="https://example.com"`)
}

func (suite *TagServiceIntegrationTestSuite) TestGetTagDetail_DescriptionSanitized() {
	// Bluemonday policy strips dangerous tags and attributes.
	desc := `Safe text <script>alert('xss')</script> and <img src=x onerror=alert(1)>.`
	tag, err := suite.tagService.CreateTag("xss-tag", &desc, nil, "other", false, nil)
	suite.Require().NoError(err)

	resp, err := suite.tagService.GetTagDetail(tag.ID)
	suite.Require().NoError(err)
	suite.Require().NotNil(resp)

	suite.Assert().NotContains(resp.DescriptionHTML, "<script>")
	suite.Assert().NotContains(resp.DescriptionHTML, "onerror")
}

func (suite *TagServiceIntegrationTestSuite) TestGetTagDetail_ParentAndChildren() {
	parent := suite.createTag("post-punk", "genre")
	parentID := parent.ID
	childA, err := suite.tagService.CreateTag("dream-pop", nil, &parentID, "genre", false, nil)
	suite.Require().NoError(err)
	childB, err := suite.tagService.CreateTag("shoegaze", nil, &parentID, "genre", true, nil)
	suite.Require().NoError(err)

	resp, err := suite.tagService.GetTagDetail(parent.ID)
	suite.Require().NoError(err)
	suite.Require().NotNil(resp)

	// Parent has no parent of its own.
	suite.Assert().Nil(resp.Parent)

	// Parent exposes both children.
	suite.Assert().Len(resp.Children, 2)
	childIDs := []uint{resp.Children[0].ID, resp.Children[1].ID}
	suite.Assert().Contains(childIDs, childA.ID)
	suite.Assert().Contains(childIDs, childB.ID)

	// Child view: parent summary is populated.
	childResp, err := suite.tagService.GetTagDetail(childA.ID)
	suite.Require().NoError(err)
	suite.Require().NotNil(childResp)
	suite.Require().NotNil(childResp.Parent)
	suite.Assert().Equal(parent.ID, childResp.Parent.ID)
	suite.Assert().Equal("post-punk", childResp.Parent.Name)
	suite.Assert().Equal("post-punk", childResp.ParentName) // legacy field still populated for backwards compat
}

func (suite *TagServiceIntegrationTestSuite) TestGetTagDetail_UsageBreakdown() {
	user := suite.createTestUserWithTier("tagger", "contributor")
	tag := suite.createTag("noisy", "genre")

	// Tag 3 artists, 2 venues, 0 of everything else.
	artist1 := suite.createArtist("Band A")
	artist2 := suite.createArtist("Band B")
	artist3 := suite.createArtist("Band C")
	venue1 := suite.createVenue("Venue A")
	venue2 := suite.createVenue("Venue B")
	for _, id := range []uint{artist1, artist2, artist3} {
		_, err := suite.tagService.AddTagToEntity(tag.ID, "", "artist", id, user.ID, "")
		suite.Require().NoError(err)
	}
	for _, id := range []uint{venue1, venue2} {
		_, err := suite.tagService.AddTagToEntity(tag.ID, "", "venue", id, user.ID, "")
		suite.Require().NoError(err)
	}

	resp, err := suite.tagService.GetTagDetail(tag.ID)
	suite.Require().NoError(err)
	suite.Require().NotNil(resp)

	suite.Assert().Equal(int64(3), resp.UsageBreakdown["artist"])
	suite.Assert().Equal(int64(2), resp.UsageBreakdown["venue"])
	suite.Assert().Equal(int64(0), resp.UsageBreakdown["show"])
	suite.Assert().Equal(int64(0), resp.UsageBreakdown["release"])
	suite.Assert().Equal(int64(0), resp.UsageBreakdown["label"])
	suite.Assert().Equal(int64(0), resp.UsageBreakdown["festival"])
}

func (suite *TagServiceIntegrationTestSuite) TestGetTagDetail_TopContributors() {
	alice := suite.createTestUserWithUsername("alice", "alice")
	alice.UserTier = "contributor"
	suite.db.Model(alice).Update("user_tier", "contributor")
	bob := suite.createTestUserWithUsername("bob", "bob")
	bob.UserTier = "contributor"
	suite.db.Model(bob).Update("user_tier", "contributor")
	carol := suite.createTestUserWithUsername("carol", "carol")
	carol.UserTier = "contributor"
	suite.db.Model(carol).Update("user_tier", "contributor")

	tag := suite.createTag("contrib-tag", "genre")

	// Alice: 3 applications, Bob: 2, Carol: 1
	for i := 0; i < 3; i++ {
		a := suite.createArtist(fmt.Sprintf("Alice-Artist-%d", i))
		_, err := suite.tagService.AddTagToEntity(tag.ID, "", "artist", a, alice.ID, "")
		suite.Require().NoError(err)
	}
	for i := 0; i < 2; i++ {
		a := suite.createArtist(fmt.Sprintf("Bob-Artist-%d", i))
		_, err := suite.tagService.AddTagToEntity(tag.ID, "", "artist", a, bob.ID, "")
		suite.Require().NoError(err)
	}
	a := suite.createArtist("Carol-Artist-0")
	_, err := suite.tagService.AddTagToEntity(tag.ID, "", "artist", a, carol.ID, "")
	suite.Require().NoError(err)

	resp, err := suite.tagService.GetTagDetail(tag.ID)
	suite.Require().NoError(err)
	suite.Require().NotNil(resp)

	suite.Require().Len(resp.TopContributors, 3)
	// Sorted by count DESC
	suite.Assert().Equal("alice", resp.TopContributors[0].User.Username)
	suite.Assert().Equal(int64(3), resp.TopContributors[0].Count)
	suite.Assert().Equal("bob", resp.TopContributors[1].User.Username)
	suite.Assert().Equal(int64(2), resp.TopContributors[1].Count)
	suite.Assert().Equal("carol", resp.TopContributors[2].User.Username)
	suite.Assert().Equal(int64(1), resp.TopContributors[2].Count)
}

func (suite *TagServiceIntegrationTestSuite) TestGetTagDetail_TopContributors_CapAtFive() {
	tag := suite.createTag("six-contrib", "genre")
	// Create 6 contributors each with 1 application → only top 5 returned.
	for i := 0; i < 6; i++ {
		u := suite.createTestUserWithUsername(fmt.Sprintf("user%d", i), fmt.Sprintf("user%d", i))
		suite.db.Model(u).Update("user_tier", "contributor")
		a := suite.createArtist(fmt.Sprintf("artist-%d", i))
		_, err := suite.tagService.AddTagToEntity(tag.ID, "", "artist", a, u.ID, "")
		suite.Require().NoError(err)
	}

	resp, err := suite.tagService.GetTagDetail(tag.ID)
	suite.Require().NoError(err)
	suite.Require().NotNil(resp)
	suite.Assert().Len(resp.TopContributors, 5)
}

func (suite *TagServiceIntegrationTestSuite) TestGetTagDetail_CreatedBy() {
	creator := suite.createTestUserWithUsername("creator-user", "creatoruser")
	creatorID := creator.ID

	tag, err := suite.tagService.CreateTag("attributed-tag", nil, nil, "genre", false, &creatorID)
	suite.Require().NoError(err)

	resp, err := suite.tagService.GetTagDetail(tag.ID)
	suite.Require().NoError(err)
	suite.Require().NotNil(resp)
	suite.Require().NotNil(resp.CreatedBy)
	suite.Assert().Equal(creatorID, resp.CreatedBy.ID)
	suite.Assert().Equal("creatoruser", resp.CreatedBy.Username)
	// Legacy pointer field also populated for backwards compatibility.
	suite.Require().NotNil(resp.CreatedByUsername)
	suite.Assert().Equal("creatoruser", *resp.CreatedByUsername)
}

func (suite *TagServiceIntegrationTestSuite) TestGetTagDetail_CreatedBy_UnknownWhenNil() {
	tag, err := suite.tagService.CreateTag("anonymous-tag", nil, nil, "genre", false, nil)
	suite.Require().NoError(err)

	resp, err := suite.tagService.GetTagDetail(tag.ID)
	suite.Require().NoError(err)
	suite.Require().NotNil(resp)
	suite.Assert().Nil(resp.CreatedBy)
}

func (suite *TagServiceIntegrationTestSuite) TestGetTagDetail_RelatedTags_CoOccurrence() {
	user := suite.createTestUserWithTier("rel-user", "contributor")
	focus := suite.createTag("focus", "genre")
	relatedA := suite.createTag("related-a", "genre")
	relatedB := suite.createTag("related-b", "genre")
	unrelated := suite.createTag("unrelated", "genre")

	// Three artists tagged with focus; two of them also with relatedA; one with relatedB.
	artist1 := suite.createArtist("Shared-1")
	artist2 := suite.createArtist("Shared-2")
	artist3 := suite.createArtist("Shared-3")
	otherArtist := suite.createArtist("Other")

	for _, id := range []uint{artist1, artist2, artist3} {
		_, err := suite.tagService.AddTagToEntity(focus.ID, "", "artist", id, user.ID, "")
		suite.Require().NoError(err)
	}
	// relatedA on artist1 + artist2 → co-occurrence with focus = 2
	for _, id := range []uint{artist1, artist2} {
		_, err := suite.tagService.AddTagToEntity(relatedA.ID, "", "artist", id, user.ID, "")
		suite.Require().NoError(err)
	}
	// relatedB on artist3 → co-occurrence with focus = 1
	_, err := suite.tagService.AddTagToEntity(relatedB.ID, "", "artist", artist3, user.ID, "")
	suite.Require().NoError(err)
	// unrelated on an entity that focus is NOT tagged on → no co-occurrence
	_, err = suite.tagService.AddTagToEntity(unrelated.ID, "", "artist", otherArtist, user.ID, "")
	suite.Require().NoError(err)

	resp, err := suite.tagService.GetTagDetail(focus.ID)
	suite.Require().NoError(err)
	suite.Require().NotNil(resp)

	suite.Require().Len(resp.RelatedTags, 2, "should return exactly the two co-occurring tags, not unrelated")
	suite.Assert().Equal(relatedA.ID, resp.RelatedTags[0].ID, "relatedA should rank first (co-occurrence=2)")
	suite.Assert().Equal(relatedB.ID, resp.RelatedTags[1].ID, "relatedB should rank second (co-occurrence=1)")

	// Self is never in the list.
	for _, rt := range resp.RelatedTags {
		suite.Assert().NotEqual(focus.ID, rt.ID, "focus tag must not appear in its own related list")
	}
}

func (suite *TagServiceIntegrationTestSuite) TestGetTagDetail_RelatedTags_CapAtFive() {
	user := suite.createTestUserWithTier("rel-cap", "contributor")
	focus := suite.createTag("cap-focus", "genre")
	artistID := suite.createArtist("One Artist Many Tags")

	// Apply focus + 6 other tags to the same artist; related list should cap at 5.
	_, err := suite.tagService.AddTagToEntity(focus.ID, "", "artist", artistID, user.ID, "")
	suite.Require().NoError(err)
	for i := 0; i < 6; i++ {
		t := suite.createTag(fmt.Sprintf("other-%d", i), "genre")
		_, err := suite.tagService.AddTagToEntity(t.ID, "", "artist", artistID, user.ID, "")
		suite.Require().NoError(err)
	}

	resp, err := suite.tagService.GetTagDetail(focus.ID)
	suite.Require().NoError(err)
	suite.Require().NotNil(resp)
	suite.Assert().Len(resp.RelatedTags, 5)
}

func (suite *TagServiceIntegrationTestSuite) TestGetTagDetail_Aliases() {
	tag := suite.createTag("with-aliases", "genre")
	_, err := suite.tagService.CreateAlias(tag.ID, "aka-one")
	suite.Require().NoError(err)
	_, err = suite.tagService.CreateAlias(tag.ID, "aka-two")
	suite.Require().NoError(err)

	resp, err := suite.tagService.GetTagDetail(tag.ID)
	suite.Require().NoError(err)
	suite.Require().NotNil(resp)
	suite.Assert().ElementsMatch([]string{"aka-one", "aka-two"}, resp.Aliases)
}

// ──────────────────────────────────────────────
// Normalization Unit Tests
// ──────────────────────────────────────────────

func TestNormalizeTagName(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"Shoe Gaze", "shoe-gaze"},
		{"POST-PUNK!!!", "post-punk"},
		{"  shoegaze-revival-2026  ", "shoegaze-revival-2026"},
		{"hip  hop", "hip-hop"},
		{"r&b", "rb"},
		{"lo-fi", "lo-fi"},
		{"Alt---Rock", "alt-rock"},
		{"  ", ""},
		{"DARK  WAVE", "dark-wave"},
		{"genre123", "genre123"},
		{"!!!", ""},
		{"a", "a"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := NormalizeTagName(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// ──────────────────────────────────────────────
// Global Alias Listing / Bulk Import (PSY-307)
// ──────────────────────────────────────────────

func (suite *TagServiceIntegrationTestSuite) TestListAllAliases_EmptyReturnsEmptySlice() {
	items, total, err := suite.tagService.ListAllAliases("", 50, 0)
	suite.Require().NoError(err)
	suite.Assert().Equal(int64(0), total)
	suite.Assert().NotNil(items)
	suite.Assert().Len(items, 0)
}

func (suite *TagServiceIntegrationTestSuite) TestListAllAliases_ReturnsCanonicalInfo() {
	tag := suite.createTag("post-punk", "genre")
	_, err := suite.tagService.CreateAlias(tag.ID, "postpunk")
	suite.Require().NoError(err)

	items, total, err := suite.tagService.ListAllAliases("", 50, 0)
	suite.Require().NoError(err)
	suite.Assert().Equal(int64(1), total)
	suite.Require().Len(items, 1)
	suite.Assert().Equal("postpunk", items[0].Alias)
	suite.Assert().Equal(tag.ID, items[0].TagID)
	suite.Assert().Equal("post-punk", items[0].TagName)
	suite.Assert().Equal(tag.Slug, items[0].TagSlug)
	suite.Assert().Equal("genre", items[0].TagCategory)
}

func (suite *TagServiceIntegrationTestSuite) TestListAllAliases_SearchByAlias() {
	tagA := suite.createTag("post-punk", "genre")
	tagB := suite.createTag("hip-hop", "genre")
	suite.tagService.CreateAlias(tagA.ID, "postpunk")
	suite.tagService.CreateAlias(tagB.ID, "hiphop")

	items, total, err := suite.tagService.ListAllAliases("post", 50, 0)
	suite.Require().NoError(err)
	suite.Assert().Equal(int64(1), total)
	suite.Require().Len(items, 1)
	suite.Assert().Equal("postpunk", items[0].Alias)
}

func (suite *TagServiceIntegrationTestSuite) TestListAllAliases_SearchByCanonicalName() {
	tagA := suite.createTag("post-punk", "genre")
	tagB := suite.createTag("hip-hop", "genre")
	suite.tagService.CreateAlias(tagA.ID, "xyz")
	suite.tagService.CreateAlias(tagB.ID, "abc")

	items, total, err := suite.tagService.ListAllAliases("hip", 50, 0)
	suite.Require().NoError(err)
	suite.Assert().Equal(int64(1), total)
	suite.Require().Len(items, 1)
	suite.Assert().Equal("abc", items[0].Alias)
	suite.Assert().Equal("hip-hop", items[0].TagName)
}

func (suite *TagServiceIntegrationTestSuite) TestListAllAliases_Pagination() {
	tag := suite.createTag("punk", "genre")
	for _, a := range []string{"a-alias", "b-alias", "c-alias", "d-alias", "e-alias"} {
		_, err := suite.tagService.CreateAlias(tag.ID, a)
		suite.Require().NoError(err)
	}

	items, total, err := suite.tagService.ListAllAliases("", 2, 0)
	suite.Require().NoError(err)
	suite.Assert().Equal(int64(5), total)
	suite.Require().Len(items, 2)
	suite.Assert().Equal("a-alias", items[0].Alias)
	suite.Assert().Equal("b-alias", items[1].Alias)

	items2, _, err := suite.tagService.ListAllAliases("", 2, 2)
	suite.Require().NoError(err)
	suite.Require().Len(items2, 2)
	suite.Assert().Equal("c-alias", items2[0].Alias)
	suite.Assert().Equal("d-alias", items2[1].Alias)
}

func (suite *TagServiceIntegrationTestSuite) TestBulkImportAliases_AllValid() {
	tagA := suite.createTag("drum-and-bass", "genre")
	tagB := suite.createTag("hip-hop", "genre")

	items := []contracts.BulkAliasImportItem{
		{Alias: "dnb", Canonical: "drum-and-bass"},
		{Alias: "d&b", Canonical: "drum-and-bass"},
		{Alias: "hiphop", Canonical: "hip-hop"},
	}
	result, err := suite.tagService.BulkImportAliases(items)
	suite.Require().NoError(err)
	suite.Assert().Equal(3, result.Imported)
	suite.Assert().Len(result.Skipped, 0)

	aliasesA, _ := suite.tagService.ListAliases(tagA.ID)
	suite.Assert().Len(aliasesA, 2)
	aliasesB, _ := suite.tagService.ListAliases(tagB.ID)
	suite.Assert().Len(aliasesB, 1)
}

func (suite *TagServiceIntegrationTestSuite) TestBulkImportAliases_MixedValidAndInvalid() {
	suite.createTag("drum-and-bass", "genre")

	items := []contracts.BulkAliasImportItem{
		{Alias: "dnb", Canonical: "drum-and-bass"},
		{Alias: "cheddar", Canonical: "nonexistent-genre"},
		{Alias: "d&b", Canonical: "drum-and-bass"},
		{Alias: "", Canonical: "drum-and-bass"},
	}
	result, err := suite.tagService.BulkImportAliases(items)
	suite.Require().NoError(err)
	suite.Assert().Equal(2, result.Imported)
	suite.Require().Len(result.Skipped, 2)
	suite.Assert().Equal(2, result.Skipped[0].Row)
	suite.Assert().Contains(result.Skipped[0].Reason, "nonexistent-genre")
	suite.Assert().Equal(4, result.Skipped[1].Row)
	suite.Assert().Contains(result.Skipped[1].Reason, "required")
}

func (suite *TagServiceIntegrationTestSuite) TestBulkImportAliases_DuplicateInBatch() {
	suite.createTag("drum-and-bass", "genre")

	items := []contracts.BulkAliasImportItem{
		{Alias: "dnb", Canonical: "drum-and-bass"},
		{Alias: "DNB", Canonical: "drum-and-bass"},
	}
	result, err := suite.tagService.BulkImportAliases(items)
	suite.Require().NoError(err)
	suite.Assert().Equal(1, result.Imported)
	suite.Require().Len(result.Skipped, 1)
	suite.Assert().Equal(2, result.Skipped[0].Row)
	suite.Assert().Contains(result.Skipped[0].Reason, "duplicate alias in batch")
}

func (suite *TagServiceIntegrationTestSuite) TestBulkImportAliases_AliasAlreadyExists() {
	tagA := suite.createTag("drum-and-bass", "genre")
	suite.tagService.CreateAlias(tagA.ID, "dnb")

	items := []contracts.BulkAliasImportItem{
		{Alias: "dnb", Canonical: "drum-and-bass"},
	}
	result, err := suite.tagService.BulkImportAliases(items)
	suite.Require().NoError(err)
	suite.Assert().Equal(0, result.Imported)
	suite.Require().Len(result.Skipped, 1)
	suite.Assert().Contains(result.Skipped[0].Reason, "alias already exists")
}

func (suite *TagServiceIntegrationTestSuite) TestBulkImportAliases_CollidesWithTagName() {
	suite.createTag("punk", "genre")
	suite.createTag("rock", "genre")

	items := []contracts.BulkAliasImportItem{
		{Alias: "rock", Canonical: "punk"},
	}
	result, err := suite.tagService.BulkImportAliases(items)
	suite.Require().NoError(err)
	suite.Assert().Equal(0, result.Imported)
	suite.Require().Len(result.Skipped, 1)
	suite.Assert().Contains(result.Skipped[0].Reason, "collides with existing tag name")
}

func (suite *TagServiceIntegrationTestSuite) TestBulkImportAliases_CanonicalBySlug() {
	tag := suite.createTag("Drum And Bass", "genre")
	suite.Assert().Equal("drum-and-bass", tag.Slug)

	items := []contracts.BulkAliasImportItem{
		{Alias: "dnb", Canonical: "drum-and-bass"},
	}
	result, err := suite.tagService.BulkImportAliases(items)
	suite.Require().NoError(err)
	suite.Assert().Equal(1, result.Imported)
}

func (suite *TagServiceIntegrationTestSuite) TestBulkImportAliases_EmptyList() {
	result, err := suite.tagService.BulkImportAliases([]contracts.BulkAliasImportItem{})
	suite.Require().NoError(err)
	suite.Assert().Equal(0, result.Imported)
	suite.Assert().Len(result.Skipped, 0)
}

// ──────────────────────────────────────────────
// Run all integration tests
// ──────────────────────────────────────────────

func TestTagServiceIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	suite.Run(t, new(TagServiceIntegrationTestSuite))
}
