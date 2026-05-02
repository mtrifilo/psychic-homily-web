package catalog

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	"gorm.io/gorm"

	apperrors "psychic-homily-backend/internal/errors"
	adminm "psychic-homily-backend/internal/models/admin"
	authm "psychic-homily-backend/internal/models/auth"
	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/testutil"
)

type TagMergeIntegrationSuite struct {
	suite.Suite
	testDB     *testutil.TestDatabase
	db         *gorm.DB
	tagService *TagService
}

func (s *TagMergeIntegrationSuite) SetupSuite() {
	s.testDB = testutil.SetupTestPostgres(s.T())
	s.db = s.testDB.DB
	s.tagService = NewTagService(s.db)
}

func (s *TagMergeIntegrationSuite) TearDownSuite() {
	s.testDB.Cleanup()
}

func (s *TagMergeIntegrationSuite) SetupTest() {
	sqlDB, _ := s.db.DB()
	_, _ = sqlDB.Exec("DELETE FROM audit_logs")
	_, _ = sqlDB.Exec("DELETE FROM tag_votes")
	_, _ = sqlDB.Exec("DELETE FROM entity_tags")
	_, _ = sqlDB.Exec("DELETE FROM tag_aliases")
	_, _ = sqlDB.Exec("DELETE FROM tags")
	_, _ = sqlDB.Exec("DELETE FROM users")
}

func TestTagMergeIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	suite.Run(t, new(TagMergeIntegrationSuite))
}

// ──────────────────────────────────────────────
// Helpers
// ──────────────────────────────────────────────

func (s *TagMergeIntegrationSuite) createUser(name string) *authm.User {
	email := fmt.Sprintf("%s-%d@test.com", name, time.Now().UnixNano())
	u := &authm.User{Email: &email, FirstName: &name, IsActive: true, EmailVerified: true}
	s.Require().NoError(s.db.Create(u).Error)
	return u
}

func (s *TagMergeIntegrationSuite) createTagWithOfficial(name string, official bool) *catalogm.Tag {
	tag, err := s.tagService.CreateTag(name, nil, nil, catalogm.TagCategoryGenre, official, nil)
	s.Require().NoError(err)
	return tag
}

func (s *TagMergeIntegrationSuite) createTag(name string) *catalogm.Tag {
	return s.createTagWithOfficial(name, false)
}

func (s *TagMergeIntegrationSuite) applyTag(tagID uint, entityType string, entityID, userID uint) *catalogm.EntityTag {
	et := &catalogm.EntityTag{
		TagID:         tagID,
		EntityType:    entityType,
		EntityID:      entityID,
		AddedByUserID: userID,
	}
	s.Require().NoError(s.db.Create(et).Error)
	// Mirror usage_count increment used by AddTagToEntity so counts stay sane.
	s.db.Model(&catalogm.Tag{}).Where("id = ?", tagID).
		Update("usage_count", gorm.Expr("usage_count + 1"))
	return et
}

// rawInsertAlias bypasses CreateAlias's friendly validation (which forbids
// aliases matching an existing tag name), so tests can simulate DB states
// that arise from migrations / out-of-band inserts.
func (s *TagMergeIntegrationSuite) rawInsertAlias(tagID uint, alias string) {
	s.Require().NoError(s.db.Exec("INSERT INTO tag_aliases (tag_id, alias) VALUES (?, ?)", tagID, alias).Error)
}

func (s *TagMergeIntegrationSuite) vote(tagID uint, entityType string, entityID, userID uint, up bool) {
	v := 1
	if !up {
		v = -1
	}
	s.Require().NoError(s.db.Create(&catalogm.TagVote{
		TagID: tagID, EntityType: entityType, EntityID: entityID, UserID: userID, Vote: v,
	}).Error)
}

func (s *TagMergeIntegrationSuite) usageCount(tagID uint) int {
	var t catalogm.Tag
	s.Require().NoError(s.db.First(&t, tagID).Error)
	return t.UsageCount
}

// ──────────────────────────────────────────────
// Happy path
// ──────────────────────────────────────────────

func (s *TagMergeIntegrationSuite) TestMerge_HappyPath_MovesEverything() {
	admin := s.createUser("admin")
	u1 := s.createUser("u1")
	u2 := s.createUser("u2")

	source := s.createTag("shoe-gaze")
	target := s.createTag("shoegaze")

	// Distinct entities — no conflicts.
	s.applyTag(source.ID, "artist", 1, u1.ID)
	s.applyTag(source.ID, "artist", 2, u1.ID)
	s.applyTag(target.ID, "artist", 3, u1.ID)

	// Distinct votes — no conflicts.
	s.vote(source.ID, "artist", 1, u1.ID, true)
	s.vote(source.ID, "artist", 2, u2.ID, true)
	s.vote(target.ID, "artist", 3, u2.ID, false)

	result, err := s.tagService.MergeTags(source.ID, target.ID, admin.ID)
	s.Require().NoError(err)
	s.Require().NotNil(result)

	s.Equal(int64(2), result.MovedEntityTags)
	s.Equal(int64(0), result.SkippedEntityTags)
	s.Equal(int64(2), result.MovedVotes)
	s.Equal(int64(0), result.SkippedVotes)
	s.True(result.AliasCreated)

	// Source is gone.
	var srcCount int64
	s.db.Model(&catalogm.Tag{}).Where("id = ?", source.ID).Count(&srcCount)
	s.Equal(int64(0), srcCount)

	// usage_count is the real count of entity_tags on target (3).
	s.Equal(3, s.usageCount(target.ID))

	// Alias source.name → target exists.
	var alias catalogm.TagAlias
	s.Require().NoError(s.db.Where("tag_id = ? AND LOWER(alias) = LOWER(?)", target.ID, source.Name).First(&alias).Error)
}

// ──────────────────────────────────────────────
// Entity-tag conflict
// ──────────────────────────────────────────────

func (s *TagMergeIntegrationSuite) TestMerge_EntityTagConflict_SourceRowDropped() {
	admin := s.createUser("admin")
	u1 := s.createUser("u1")

	source := s.createTag("shoe-gaze")
	target := s.createTag("shoegaze")

	// Same (entity_type, entity_id) on both — target wins.
	s.applyTag(source.ID, "artist", 1, u1.ID)
	s.applyTag(target.ID, "artist", 1, u1.ID)
	// One unique on source → should move.
	s.applyTag(source.ID, "artist", 2, u1.ID)

	result, err := s.tagService.MergeTags(source.ID, target.ID, admin.ID)
	s.Require().NoError(err)

	s.Equal(int64(1), result.MovedEntityTags)
	s.Equal(int64(1), result.SkippedEntityTags)

	// Target has 2 rows (its original + the moved one), not 3.
	var tgtCount int64
	s.db.Model(&catalogm.EntityTag{}).Where("tag_id = ?", target.ID).Count(&tgtCount)
	s.Equal(int64(2), tgtCount)
	s.Equal(2, s.usageCount(target.ID))
}

// ──────────────────────────────────────────────
// Vote conflict
// ──────────────────────────────────────────────

func (s *TagMergeIntegrationSuite) TestMerge_VoteConflict_TargetWins() {
	admin := s.createUser("admin")
	u1 := s.createUser("u1")

	source := s.createTag("shoe-gaze")
	target := s.createTag("shoegaze")

	// Both tags applied to the same entity, but with a twist: move source's
	// vote. To set up, apply target to entity 1 and source to entity 2 so
	// their rows don't conflict on entity_tags, then place votes on both for
	// the same user/entity/type combination.
	s.applyTag(target.ID, "artist", 1, u1.ID)
	s.applyTag(source.ID, "artist", 1, u1.ID)    // will collide on entity_tags
	s.vote(source.ID, "artist", 1, u1.ID, true)  // upvote on source
	s.vote(target.ID, "artist", 1, u1.ID, false) // downvote on target (wins)

	result, err := s.tagService.MergeTags(source.ID, target.ID, admin.ID)
	s.Require().NoError(err)

	s.Equal(int64(0), result.MovedVotes)
	s.Equal(int64(1), result.SkippedVotes)

	// Target's downvote survives.
	var survivor catalogm.TagVote
	s.Require().NoError(s.db.Where("tag_id = ? AND entity_type = 'artist' AND entity_id = 1 AND user_id = ?", target.ID, u1.ID).First(&survivor).Error)
	s.Equal(-1, survivor.Vote)
}

// ──────────────────────────────────────────────
// Guards
// ──────────────────────────────────────────────

func (s *TagMergeIntegrationSuite) TestMerge_SelfMerge_Rejected() {
	admin := s.createUser("admin")
	t := s.createTag("shoegaze")

	_, err := s.tagService.MergeTags(t.ID, t.ID, admin.ID)
	s.Require().Error(err)
	var tagErr *apperrors.TagError
	s.Require().ErrorAs(err, &tagErr)
	s.Equal(apperrors.CodeTagMergeInvalid, tagErr.Code)
}

func (s *TagMergeIntegrationSuite) TestMerge_NonexistentSource() {
	admin := s.createUser("admin")
	t := s.createTag("shoegaze")

	_, err := s.tagService.MergeTags(99999, t.ID, admin.ID)
	s.Require().Error(err)
	var tagErr *apperrors.TagError
	s.Require().ErrorAs(err, &tagErr)
	s.Equal(apperrors.CodeTagNotFound, tagErr.Code)
}

func (s *TagMergeIntegrationSuite) TestMerge_NonexistentTarget() {
	admin := s.createUser("admin")
	t := s.createTag("shoegaze")

	_, err := s.tagService.MergeTags(t.ID, 99999, admin.ID)
	s.Require().Error(err)
	var tagErr *apperrors.TagError
	s.Require().ErrorAs(err, &tagErr)
	s.Equal(apperrors.CodeTagNotFound, tagErr.Code)
}

// Source already an alias of target → reject (this would be a circular merge).
func (s *TagMergeIntegrationSuite) TestMerge_CircularAlias_Rejected() {
	admin := s.createUser("admin")
	target := s.createTag("shoegaze")
	source := s.createTag("shoe-gaze")

	// Seed source.name as an alias already pointing at target. Using the
	// direct-GORM insert bypasses CreateAlias's "alias collides with tag name"
	// guard so we can simulate an already-conflicting DB state.
	s.rawInsertAlias(target.ID, source.Name)

	_, err := s.tagService.MergeTags(source.ID, target.ID, admin.ID)
	s.Require().Error(err)
	var tagErr *apperrors.TagError
	s.Require().ErrorAs(err, &tagErr)
	s.Equal(apperrors.CodeTagMergeInvalid, tagErr.Code)
}

// ──────────────────────────────────────────────
// Aliases
// ──────────────────────────────────────────────

func (s *TagMergeIntegrationSuite) TestMerge_SourceAliasesRepointToTarget() {
	admin := s.createUser("admin")
	source := s.createTag("shoe-gaze")
	target := s.createTag("shoegaze")

	_, err := s.tagService.CreateAlias(source.ID, "shoegazing")
	s.Require().NoError(err)
	_, err = s.tagService.CreateAlias(source.ID, "nu-gaze")
	s.Require().NoError(err)

	result, err := s.tagService.MergeTags(source.ID, target.ID, admin.ID)
	s.Require().NoError(err)
	s.Equal(int64(2), result.MovedAliases)

	aliases, err := s.tagService.ListAliases(target.ID)
	s.Require().NoError(err)

	names := map[string]bool{}
	for _, a := range aliases {
		names[a.Alias] = true
	}
	s.True(names["shoegazing"])
	s.True(names["nu-gaze"])
	// source.name also becomes an alias on target.
	s.True(names[source.Name])
}

// The tag_aliases table has a global unique index on LOWER(alias), so two
// tags can't hold the same alias at rest. But during a merge, the DELETE for
// source's conflicting aliases runs BEFORE the UPDATE that re-points the
// rest — and we want that DELETE to pick up any case-insensitive collision
// with target's existing aliases. Simulate by seeding target with an alias,
// then seeding source with the same text via raw SQL (which succeeds only
// because we briefly drop the unique index below — or, simpler, don't seed
// source at all and instead verify that moveAliases is a no-op collision
// when the alias is ALREADY on target).
func (s *TagMergeIntegrationSuite) TestMerge_AliasCollisionOnTarget_TargetWins() {
	admin := s.createUser("admin")
	source := s.createTag("shoe-gaze")
	target := s.createTag("shoegaze")

	// Target owns "nu-gaze" already. Source also "owns" it via raw SQL after
	// we drop the unique index, simulating a legacy dataset that pre-dates
	// the index. moveAliases should drop the source row and keep target's.
	s.rawInsertAlias(target.ID, "Nu-Gaze") // target casing
	s.Require().NoError(s.db.Exec("DROP INDEX IF EXISTS idx_tag_aliases_alias_lower").Error)
	defer s.db.Exec("CREATE UNIQUE INDEX idx_tag_aliases_alias_lower ON tag_aliases(LOWER(alias))")
	s.rawInsertAlias(source.ID, "nu-gaze")

	_, err := s.tagService.MergeTags(source.ID, target.ID, admin.ID)
	s.Require().NoError(err)

	// Only target's row survives, with its original casing.
	var aliases []catalogm.TagAlias
	s.Require().NoError(s.db.Where("LOWER(alias) = LOWER(?)", "nu-gaze").Find(&aliases).Error)
	s.Require().Len(aliases, 1)
	s.Equal(target.ID, aliases[0].TagID)
	s.Equal("Nu-Gaze", aliases[0].Alias)
}

// If source.name matches an alias already pointing at a DIFFERENT tag, abort.
func (s *TagMergeIntegrationSuite) TestMerge_SourceNameCollidesWithForeignAlias_Aborts() {
	admin := s.createUser("admin")
	source := s.createTag("shoe-gaze")
	target := s.createTag("shoegaze")
	other := s.createTag("other-genre")

	// Seed a foreign alias pointing source.name → `other` via direct insert,
	// bypassing CreateAlias's tag-name collision guard.
	s.rawInsertAlias(other.ID, source.Name)

	_, err := s.tagService.MergeTags(source.ID, target.ID, admin.ID)
	s.Require().Error(err)
	var tagErr *apperrors.TagError
	s.Require().ErrorAs(err, &tagErr)
	s.Equal(apperrors.CodeTagMergeAliasConflict, tagErr.Code)

	// Source still exists (transaction rolled back).
	var stillThere catalogm.Tag
	s.Require().NoError(s.db.First(&stillThere, source.ID).Error)
}

// ──────────────────────────────────────────────
// Official tag handling
// ──────────────────────────────────────────────

func (s *TagMergeIntegrationSuite) TestMerge_OfficialSource_TargetBecomesOfficial() {
	admin := s.createUser("admin")
	source := s.createTagWithOfficial("shoe-gaze", true)
	target := s.createTagWithOfficial("shoegaze", false)

	_, err := s.tagService.MergeTags(source.ID, target.ID, admin.ID)
	s.Require().NoError(err)

	var merged catalogm.Tag
	s.Require().NoError(s.db.First(&merged, target.ID).Error)
	s.True(merged.IsOfficial, "official flag should carry forward from source")
}

func (s *TagMergeIntegrationSuite) TestMerge_OfficialTarget_StaysOfficial() {
	admin := s.createUser("admin")
	source := s.createTagWithOfficial("shoe-gaze", false)
	target := s.createTagWithOfficial("shoegaze", true)

	_, err := s.tagService.MergeTags(source.ID, target.ID, admin.ID)
	s.Require().NoError(err)

	var merged catalogm.Tag
	s.Require().NoError(s.db.First(&merged, target.ID).Error)
	s.True(merged.IsOfficial)
}

// ──────────────────────────────────────────────
// Idempotency + usage_count integrity
// ──────────────────────────────────────────────

func (s *TagMergeIntegrationSuite) TestMerge_ReRunWithDeletedSource_ReturnsNotFound() {
	admin := s.createUser("admin")
	source := s.createTag("shoe-gaze")
	target := s.createTag("shoegaze")

	_, err := s.tagService.MergeTags(source.ID, target.ID, admin.ID)
	s.Require().NoError(err)

	_, err = s.tagService.MergeTags(source.ID, target.ID, admin.ID)
	s.Require().Error(err)
	var tagErr *apperrors.TagError
	s.Require().ErrorAs(err, &tagErr)
	s.Equal(apperrors.CodeTagNotFound, tagErr.Code)
}

func (s *TagMergeIntegrationSuite) TestMerge_UsageCount_ReflectsActualCount() {
	admin := s.createUser("admin")
	u1 := s.createUser("u1")

	source := s.createTag("shoe-gaze")
	target := s.createTag("shoegaze")

	// Set usage_count values that are OUT OF SYNC with real rows, so we can
	// verify recompute-from-actual-count rather than simple addition.
	s.applyTag(source.ID, "artist", 1, u1.ID)
	s.applyTag(source.ID, "artist", 2, u1.ID)
	s.applyTag(target.ID, "artist", 3, u1.ID)
	// Manually scramble the stored counts.
	s.db.Model(&catalogm.Tag{}).Where("id = ?", source.ID).Update("usage_count", 999)
	s.db.Model(&catalogm.Tag{}).Where("id = ?", target.ID).Update("usage_count", 77)

	_, err := s.tagService.MergeTags(source.ID, target.ID, admin.ID)
	s.Require().NoError(err)

	// After merge, target has 3 entity_tag rows, so usage_count = 3.
	s.Equal(3, s.usageCount(target.ID))
}

// ──────────────────────────────────────────────
// Audit log
// ──────────────────────────────────────────────

func (s *TagMergeIntegrationSuite) TestMerge_WritesAuditLog() {
	admin := s.createUser("admin")
	u1 := s.createUser("u1")
	source := s.createTag("shoe-gaze")
	target := s.createTag("shoegaze")
	s.applyTag(source.ID, "artist", 1, u1.ID)

	_, err := s.tagService.MergeTags(source.ID, target.ID, admin.ID)
	s.Require().NoError(err)

	// Audit log write is fire-and-forget; poll briefly so the goroutine wins.
	// Filter on entity_id so a log from a prior test can't satisfy the query.
	var log adminm.AuditLog
	for i := 0; i < 40; i++ {
		err := s.db.Where("action = ? AND entity_id = ?", AuditActionMergeTags, target.ID).First(&log).Error
		if err == nil {
			break
		}
		time.Sleep(25 * time.Millisecond)
	}
	s.Require().NotZero(log.ID, "audit log was not written in time")

	s.Equal("tag", log.EntityType)
	s.Equal(target.ID, log.EntityID)
	s.Require().NotNil(log.ActorID)
	s.Equal(admin.ID, *log.ActorID)

	s.Require().NotNil(log.Metadata)
	var meta map[string]interface{}
	s.Require().NoError(json.Unmarshal(*log.Metadata, &meta))
	s.Equal("shoe-gaze", meta["source_tag_name"])
	s.Equal("shoegaze", meta["target_tag_name"])
	s.Equal(float64(1), meta["moved_entity_tags"])
}

// ──────────────────────────────────────────────
// Transaction rollback
// ──────────────────────────────────────────────

// When an in-transaction error occurs AFTER moves have happened, the whole
// merge must be rolled back — nothing should be visible to callers.
func (s *TagMergeIntegrationSuite) TestMerge_TransactionRollback_OnAliasCollision() {
	admin := s.createUser("admin")
	u1 := s.createUser("u1")

	source := s.createTag("shoe-gaze")
	target := s.createTag("shoegaze")
	other := s.createTag("unrelated")

	// Apply entity_tags to source to verify none leak through on rollback.
	s.applyTag(source.ID, "artist", 1, u1.ID)
	s.applyTag(source.ID, "artist", 2, u1.ID)

	// Seed a foreign alias collision to force abort.
	s.rawInsertAlias(other.ID, source.Name)

	_, err := s.tagService.MergeTags(source.ID, target.ID, admin.ID)
	s.Require().Error(err)

	// Source still exists with its original 2 entity_tags.
	var still catalogm.Tag
	s.Require().NoError(s.db.First(&still, source.ID).Error)
	var srcEntityCount int64
	s.db.Model(&catalogm.EntityTag{}).Where("tag_id = ?", source.ID).Count(&srcEntityCount)
	s.Equal(int64(2), srcEntityCount)

	// Target has nothing on it.
	var tgtEntityCount int64
	s.db.Model(&catalogm.EntityTag{}).Where("tag_id = ?", target.ID).Count(&tgtEntityCount)
	s.Equal(int64(0), tgtEntityCount)
}

// ──────────────────────────────────────────────
// Preview endpoint
// ──────────────────────────────────────────────

func (s *TagMergeIntegrationSuite) TestPreviewMerge_CountsMatchActualMerge() {
	admin := s.createUser("admin")
	u1 := s.createUser("u1")
	u2 := s.createUser("u2")

	source := s.createTag("shoe-gaze")
	target := s.createTag("shoegaze")

	s.applyTag(source.ID, "artist", 1, u1.ID)
	s.applyTag(source.ID, "artist", 2, u1.ID)
	s.applyTag(target.ID, "artist", 2, u1.ID) // conflict on entity 2
	s.vote(source.ID, "artist", 1, u2.ID, true)
	s.vote(source.ID, "artist", 2, u1.ID, true)
	s.vote(target.ID, "artist", 2, u1.ID, false) // conflict
	_, err := s.tagService.CreateAlias(source.ID, "nu-gaze-preview")
	s.Require().NoError(err)

	preview, err := s.tagService.PreviewMergeTags(source.ID, target.ID)
	s.Require().NoError(err)
	s.Equal(int64(1), preview.MovedEntityTags)
	s.Equal(int64(1), preview.SkippedEntityTags)
	s.Equal(int64(1), preview.MovedVotes)
	s.Equal(int64(1), preview.SkippedVotes)
	s.Equal(int64(1), preview.SourceAliasesCount)
	s.Equal("shoe-gaze", preview.SourceName)
	s.Equal("shoegaze", preview.TargetName)

	// Now actually merge and verify the preview matched.
	result, err := s.tagService.MergeTags(source.ID, target.ID, admin.ID)
	s.Require().NoError(err)
	s.Equal(preview.MovedEntityTags, result.MovedEntityTags)
	s.Equal(preview.SkippedEntityTags, result.SkippedEntityTags)
	s.Equal(preview.MovedVotes, result.MovedVotes)
	s.Equal(preview.SkippedVotes, result.SkippedVotes)
}
