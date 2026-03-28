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
	"psychic-homily-backend/internal/testutil"
)

// =============================================================================
// UNIT TESTS (No Database Required)
// =============================================================================

func TestNewTagService(t *testing.T) {
	svc := NewTagService(nil)
	assert.NotNil(t, svc)
}

func TestTagService_NilDatabase(t *testing.T) {
	svc := &TagService{db: nil}

	t.Run("CreateTag", func(t *testing.T) {
		testutil.AssertNilDBErrorWithResult(t, func() (interface{}, error) {
			return svc.CreateTag("test", nil, nil, "genre", false)
		})
	})

	t.Run("GetTag", func(t *testing.T) {
		testutil.AssertNilDBErrorWithResult(t, func() (interface{}, error) {
			return svc.GetTag(1)
		})
	})

	t.Run("GetTagBySlug", func(t *testing.T) {
		testutil.AssertNilDBErrorWithResult(t, func() (interface{}, error) {
			return svc.GetTagBySlug("test")
		})
	})

	t.Run("ListTags", func(t *testing.T) {
		testutil.AssertNilDBError(t, func() error {
			_, _, err := svc.ListTags("", "", nil, "usage", 20, 0)
			return err
		})
	})

	t.Run("UpdateTag", func(t *testing.T) {
		name := "updated"
		testutil.AssertNilDBErrorWithResult(t, func() (interface{}, error) {
			return svc.UpdateTag(1, &name, nil, nil, nil, nil)
		})
	})

	t.Run("DeleteTag", func(t *testing.T) {
		testutil.AssertNilDBError(t, func() error {
			return svc.DeleteTag(1)
		})
	})

	t.Run("AddTagToEntity", func(t *testing.T) {
		testutil.AssertNilDBErrorWithResult(t, func() (interface{}, error) {
			return svc.AddTagToEntity(1, "", "artist", 1, 1)
		})
	})

	t.Run("RemoveTagFromEntity", func(t *testing.T) {
		testutil.AssertNilDBError(t, func() error {
			return svc.RemoveTagFromEntity(1, "artist", 1)
		})
	})

	t.Run("ListEntityTags", func(t *testing.T) {
		testutil.AssertNilDBErrorWithResult(t, func() (interface{}, error) {
			return svc.ListEntityTags("artist", 1, 0)
		})
	})

	t.Run("VoteOnTag", func(t *testing.T) {
		testutil.AssertNilDBError(t, func() error {
			return svc.VoteOnTag(1, "artist", 1, 1, true)
		})
	})

	t.Run("RemoveTagVote", func(t *testing.T) {
		testutil.AssertNilDBError(t, func() error {
			return svc.RemoveTagVote(1, "artist", 1, 1)
		})
	})

	t.Run("CreateAlias", func(t *testing.T) {
		testutil.AssertNilDBErrorWithResult(t, func() (interface{}, error) {
			return svc.CreateAlias(1, "test")
		})
	})

	t.Run("DeleteAlias", func(t *testing.T) {
		testutil.AssertNilDBError(t, func() error {
			return svc.DeleteAlias(1)
		})
	})

	t.Run("ListAliases", func(t *testing.T) {
		testutil.AssertNilDBErrorWithResult(t, func() (interface{}, error) {
			return svc.ListAliases(1)
		})
	})

	t.Run("ResolveAlias", func(t *testing.T) {
		testutil.AssertNilDBErrorWithResult(t, func() (interface{}, error) {
			return svc.ResolveAlias("test")
		})
	})

	t.Run("SearchTags", func(t *testing.T) {
		testutil.AssertNilDBErrorWithResult(t, func() (interface{}, error) {
			return svc.SearchTags("test", 10)
		})
	})

	t.Run("GetTrendingTags", func(t *testing.T) {
		testutil.AssertNilDBErrorWithResult(t, func() (interface{}, error) {
			return svc.GetTrendingTags(10, "")
		})
	})

	t.Run("PruneDownvotedTags", func(t *testing.T) {
		testutil.AssertNilDBErrorWithResult(t, func() (interface{}, error) {
			return svc.PruneDownvotedTags()
		})
	})
}

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
	tag, err := suite.tagService.CreateTag(name, nil, nil, category, false)
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
	tag, err := suite.tagService.CreateTag("Post-Punk", &desc, nil, "genre", true)
	suite.Require().NoError(err)
	suite.Require().NotNil(tag)

	suite.Assert().Equal("Post-Punk", tag.Name)
	suite.Assert().Equal("post-punk", tag.Slug)
	suite.Assert().Equal("genre", tag.Category)
	suite.Assert().True(tag.IsOfficial)
	suite.Assert().NotNil(tag.Description)
	suite.Assert().Equal("A subgenre of punk rock", *tag.Description)
	suite.Assert().Equal(0, tag.UsageCount)
}

func (suite *TagServiceIntegrationTestSuite) TestCreateTag_InvalidCategory() {
	tag, err := suite.tagService.CreateTag("Test", nil, nil, "invalid", false)
	suite.Assert().Error(err)
	suite.Assert().Contains(err.Error(), "invalid tag category")
	suite.Assert().Nil(tag)
}

func (suite *TagServiceIntegrationTestSuite) TestCreateTag_DuplicateName() {
	suite.createTag("rock", "genre")

	tag, err := suite.tagService.CreateTag("Rock", nil, nil, "genre", false)
	suite.Assert().Error(err)
	var tagErr *apperrors.TagError
	suite.Assert().ErrorAs(err, &tagErr)
	suite.Assert().Equal(apperrors.CodeTagExists, tagErr.Code)
	suite.Assert().Nil(tag)
}

func (suite *TagServiceIntegrationTestSuite) TestCreateTag_WithParent() {
	parent := suite.createTag("rock", "genre")

	child, err := suite.tagService.CreateTag("post-punk", nil, &parent.ID, "genre", false)
	suite.Require().NoError(err)
	suite.Assert().NotNil(child.ParentID)
	suite.Assert().Equal(parent.ID, *child.ParentID)
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
	suite.tagService.CreateTag("post-rock", nil, &parent.ID, "genre", false)

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

	et, err := suite.tagService.AddTagToEntity(tag.ID, "", "artist", artistID, user.ID)
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

	et, err := suite.tagService.AddTagToEntity(0, "ambient", "artist", artistID, user.ID)
	suite.Require().NoError(err)
	suite.Assert().Equal(tag.ID, et.TagID)
}

func (suite *TagServiceIntegrationTestSuite) TestAddTagToEntity_ByAlias() {
	user := suite.createTestUser("tagger")
	tag := suite.createTag("post-punk", "genre")
	_, err := suite.tagService.CreateAlias(tag.ID, "post punk")
	suite.Require().NoError(err)
	artistID := suite.createArtist("Post Punk Band")

	et, err := suite.tagService.AddTagToEntity(0, "post punk", "artist", artistID, user.ID)
	suite.Require().NoError(err)
	suite.Assert().Equal(tag.ID, et.TagID)
}

func (suite *TagServiceIntegrationTestSuite) TestAddTagToEntity_Duplicate() {
	user := suite.createTestUser("tagger")
	tag := suite.createTag("rock", "genre")
	artistID := suite.createArtist("Rock Band")

	_, err := suite.tagService.AddTagToEntity(tag.ID, "", "artist", artistID, user.ID)
	suite.Require().NoError(err)

	_, err = suite.tagService.AddTagToEntity(tag.ID, "", "artist", artistID, user.ID)
	suite.Assert().Error(err)
	var tagErr *apperrors.TagError
	suite.Assert().ErrorAs(err, &tagErr)
	suite.Assert().Equal(apperrors.CodeEntityTagExists, tagErr.Code)
}

func (suite *TagServiceIntegrationTestSuite) TestAddTagToEntity_InvalidEntityType() {
	user := suite.createTestUser("tagger")
	tag := suite.createTag("rock", "genre")

	_, err := suite.tagService.AddTagToEntity(tag.ID, "", "invalid", 1, user.ID)
	suite.Assert().Error(err)
	suite.Assert().Contains(err.Error(), "invalid entity type")
}

func (suite *TagServiceIntegrationTestSuite) TestRemoveTagFromEntity_Success() {
	user := suite.createTestUser("tagger")
	tag := suite.createTag("metal", "genre")
	artistID := suite.createArtist("Metal Band")

	_, err := suite.tagService.AddTagToEntity(tag.ID, "", "artist", artistID, user.ID)
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

	suite.tagService.AddTagToEntity(tag1.ID, "", "artist", artistID, user.ID)
	suite.tagService.AddTagToEntity(tag2.ID, "", "artist", artistID, user.ID)

	tags, err := suite.tagService.ListEntityTags("artist", artistID, 0)
	suite.Require().NoError(err)
	suite.Assert().Len(tags, 2)
}

func (suite *TagServiceIntegrationTestSuite) TestListEntityTags_WithUserVote() {
	user := suite.createTestUser("voter")
	tag := suite.createTag("punk", "genre")
	artistID := suite.createArtist("Punk Band")

	suite.tagService.AddTagToEntity(tag.ID, "", "artist", artistID, user.ID)
	suite.tagService.VoteOnTag(tag.ID, "artist", artistID, user.ID, true)

	tags, err := suite.tagService.ListEntityTags("artist", artistID, user.ID)
	suite.Require().NoError(err)
	suite.Require().Len(tags, 1)
	suite.Assert().NotNil(tags[0].UserVote)
	suite.Assert().Equal(1, *tags[0].UserVote)
	suite.Assert().Equal(1, tags[0].Upvotes)
}

// ──────────────────────────────────────────────
// Voting Tests
// ──────────────────────────────────────────────

func (suite *TagServiceIntegrationTestSuite) TestVoteOnTag_Upvote() {
	user := suite.createTestUser("voter")
	tag := suite.createTag("synth", "genre")
	artistID := suite.createArtist("Synth Band")

	suite.tagService.AddTagToEntity(tag.ID, "", "artist", artistID, user.ID)

	err := suite.tagService.VoteOnTag(tag.ID, "artist", artistID, user.ID, true)
	suite.Assert().NoError(err)
}

func (suite *TagServiceIntegrationTestSuite) TestVoteOnTag_ChangeVote() {
	user := suite.createTestUser("voter")
	tag := suite.createTag("grunge", "genre")
	artistID := suite.createArtist("Grunge Band")

	suite.tagService.AddTagToEntity(tag.ID, "", "artist", artistID, user.ID)
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

	suite.tagService.AddTagToEntity(tag.ID, "", "artist", artistID, user.ID)
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

	tags, err := suite.tagService.SearchTags("post", 10)
	suite.Require().NoError(err)
	suite.Assert().Len(tags, 2)
}

func (suite *TagServiceIntegrationTestSuite) TestSearchTags_ByAlias() {
	tag := suite.createTag("post-punk", "genre")
	suite.tagService.CreateAlias(tag.ID, "post punk revival")

	tags, err := suite.tagService.SearchTags("revival", 10)
	suite.Require().NoError(err)
	suite.Assert().Len(tags, 1)
	suite.Assert().Equal(tag.ID, tags[0].ID)
}

func (suite *TagServiceIntegrationTestSuite) TestGetTrendingTags() {
	user := suite.createTestUser("tagger")
	tag1 := suite.createTag("rock", "genre")
	tag2 := suite.createTag("jazz", "genre")
	artistID := suite.createArtist("Multi-Genre")

	// Apply tags to entities to increase usage count
	suite.tagService.AddTagToEntity(tag1.ID, "", "artist", artistID, user.ID)
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

	suite.tagService.AddTagToEntity(tag1.ID, "", "artist", artistID, user.ID)
	artist2 := suite.createArtist("Band2")
	suite.tagService.AddTagToEntity(tag2.ID, "", "artist", artist2, user.ID)

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
	suite.tagService.AddTagToEntity(tag.ID, "", "artist", artistID, user1.ID)
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
	tag, _ := suite.tagService.CreateTag("official-tag", nil, nil, "genre", true)
	artistID := suite.createArtist("Some Official Band")

	suite.tagService.AddTagToEntity(tag.ID, "", "artist", artistID, user1.ID)
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
// Run all integration tests
// ──────────────────────────────────────────────

func TestTagServiceIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	suite.Run(t, new(TagServiceIntegrationTestSuite))
}
