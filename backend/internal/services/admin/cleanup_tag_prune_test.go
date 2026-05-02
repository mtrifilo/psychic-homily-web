package admin

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	"gorm.io/gorm"

	adminm "psychic-homily-backend/internal/models/admin"
	authm "psychic-homily-backend/internal/models/auth"
	catalogm "psychic-homily-backend/internal/models/catalog"
	"psychic-homily-backend/internal/testutil"
)

// discardLogger returns a slog.Logger that writes nowhere — keeps test output clean.
func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// =============================================================================
// INTEGRATION TESTS — Tag Prune (PSY-308)
// =============================================================================

type CleanupTagPruneTestSuite struct {
	suite.Suite
	testDB  *testutil.TestDatabase
	db      *gorm.DB
	service *CleanupService
}

func (s *CleanupTagPruneTestSuite) SetupSuite() {
	s.testDB = testutil.SetupTestPostgres(s.T())
	s.db = s.testDB.DB
}

func (s *CleanupTagPruneTestSuite) TearDownSuite() {
	s.testDB.Cleanup()
}

func (s *CleanupTagPruneTestSuite) SetupTest() {
	s.service = &CleanupService{
		db:               s.db,
		userService:      &stubUserService{},
		interval:         DefaultCleanupInterval,
		tagPruneInterval: DefaultTagPruneInterval,
		tagPruneEnabled:  true,
		tagPruneDryRun:   false,
		stopCh:           make(chan struct{}),
		logger:           discardLogger(),
	}
}

func (s *CleanupTagPruneTestSuite) TearDownTest() {
	sqlDB, err := s.db.DB()
	s.Require().NoError(err)
	_, _ = sqlDB.Exec("DELETE FROM audit_logs")
	_, _ = sqlDB.Exec("DELETE FROM tag_votes")
	_, _ = sqlDB.Exec("DELETE FROM entity_tags")
	_, _ = sqlDB.Exec("DELETE FROM tag_aliases")
	_, _ = sqlDB.Exec("DELETE FROM tags")
	_, _ = sqlDB.Exec("DELETE FROM users")
}

func TestCleanupTagPruneTestSuite(t *testing.T) {
	suite.Run(t, new(CleanupTagPruneTestSuite))
}

// ------------------------------------------------------------
// Helpers
// ------------------------------------------------------------

var userCounter int

func (s *CleanupTagPruneTestSuite) createUser() *authm.User {
	userCounter++
	email := fmt.Sprintf("prune-%d-%d@test.com", time.Now().UnixNano(), userCounter)
	user := &authm.User{
		Email:         &email,
		IsActive:      true,
		EmailVerified: true,
	}
	s.Require().NoError(s.db.Create(user).Error)
	return user
}

func (s *CleanupTagPruneTestSuite) createTag(name string) *catalogm.Tag {
	userCounter++
	tag := &catalogm.Tag{
		Name:     name,
		Slug:     fmt.Sprintf("%s-%d-%d", name, time.Now().UnixNano(), userCounter),
		Category: catalogm.TagCategoryGenre,
	}
	s.Require().NoError(s.db.Create(tag).Error)
	return tag
}

// applyTag creates an entity_tag and casts the given up/down votes against it,
// using unique per-vote users to satisfy the composite primary key.
func (s *CleanupTagPruneTestSuite) applyTag(tag *catalogm.Tag, entityType string, entityID uint, ups, downs int) *catalogm.EntityTag {
	adder := s.createUser()
	et := &catalogm.EntityTag{
		TagID:         tag.ID,
		EntityType:    entityType,
		EntityID:      entityID,
		AddedByUserID: adder.ID,
	}
	s.Require().NoError(s.db.Create(et).Error)

	for i := 0; i < ups; i++ {
		voter := s.createUser()
		vote := &catalogm.TagVote{
			TagID:      tag.ID,
			EntityType: entityType,
			EntityID:   entityID,
			UserID:     voter.ID,
			Vote:       1,
		}
		s.Require().NoError(s.db.Create(vote).Error)
	}
	for i := 0; i < downs; i++ {
		voter := s.createUser()
		vote := &catalogm.TagVote{
			TagID:      tag.ID,
			EntityType: entityType,
			EntityID:   entityID,
			UserID:     voter.ID,
			Vote:       -1,
		}
		s.Require().NoError(s.db.Create(vote).Error)
	}
	return et
}

func (s *CleanupTagPruneTestSuite) entityTagExists(id uint) bool {
	var count int64
	s.db.Model(&catalogm.EntityTag{}).Where("id = ?", id).Count(&count)
	return count == 1
}

func (s *CleanupTagPruneTestSuite) tagRowExists(id uint) bool {
	var count int64
	s.db.Model(&catalogm.Tag{}).Where("id = ?", id).Count(&count)
	return count == 1
}

// ------------------------------------------------------------
// Tests
// ------------------------------------------------------------

// Happy path: downs > ups AND downs >= 2 → pruned.
func (s *CleanupTagPruneTestSuite) TestPrune_HappyPath_2DownsZeroUps() {
	tag := s.createTag("noise")
	et := s.applyTag(tag, "artist", 1, 0, 2)

	deleted, err := s.service.pruneDownvotedEntityTags(context.Background())
	s.Require().NoError(err)
	s.Equal(int64(1), deleted)
	s.False(s.entityTagExists(et.ID))
	// Tag itself must remain.
	s.True(s.tagRowExists(tag.ID))
}

func (s *CleanupTagPruneTestSuite) TestPrune_HappyPath_3DownsOneUp() {
	tag := s.createTag("bad-fit")
	et := s.applyTag(tag, "release", 5, 1, 3)

	deleted, err := s.service.pruneDownvotedEntityTags(context.Background())
	s.Require().NoError(err)
	s.Equal(int64(1), deleted)
	s.False(s.entityTagExists(et.ID))
	s.True(s.tagRowExists(tag.ID))
}

// Tie: downs == ups should NOT prune (strict inequality required).
func (s *CleanupTagPruneTestSuite) TestPrune_TiedVotes_NotPruned() {
	tag := s.createTag("debatable")
	et := s.applyTag(tag, "artist", 2, 2, 2)

	deleted, err := s.service.pruneDownvotedEntityTags(context.Background())
	s.Require().NoError(err)
	s.Equal(int64(0), deleted)
	s.True(s.entityTagExists(et.ID))
}

// Below threshold: downs == 1 should NOT prune.
func (s *CleanupTagPruneTestSuite) TestPrune_SingleDownvote_NotPruned() {
	tag := s.createTag("maybe")
	et := s.applyTag(tag, "artist", 3, 0, 1)

	deleted, err := s.service.pruneDownvotedEntityTags(context.Background())
	s.Require().NoError(err)
	s.Equal(int64(0), deleted)
	s.True(s.entityTagExists(et.ID))
}

// No votes at all: should NOT prune.
func (s *CleanupTagPruneTestSuite) TestPrune_NoVotes_NotPruned() {
	tag := s.createTag("fresh")
	et := s.applyTag(tag, "label", 9, 0, 0)

	deleted, err := s.service.pruneDownvotedEntityTags(context.Background())
	s.Require().NoError(err)
	s.Equal(int64(0), deleted)
	s.True(s.entityTagExists(et.ID))
}

// ups > downs: should NOT prune, even with many downs.
func (s *CleanupTagPruneTestSuite) TestPrune_UpsWinMajority_NotPruned() {
	tag := s.createTag("loved")
	et := s.applyTag(tag, "show", 11, 5, 3)

	deleted, err := s.service.pruneDownvotedEntityTags(context.Background())
	s.Require().NoError(err)
	s.Equal(int64(0), deleted)
	s.True(s.entityTagExists(et.ID))
}

// Idempotency: running twice back-to-back must not over-delete.
func (s *CleanupTagPruneTestSuite) TestPrune_Idempotency() {
	tag := s.createTag("idem")
	s.applyTag(tag, "artist", 100, 0, 3)
	s.applyTag(tag, "artist", 101, 0, 3)

	deleted1, err := s.service.pruneDownvotedEntityTags(context.Background())
	s.Require().NoError(err)
	s.Equal(int64(2), deleted1)

	deleted2, err := s.service.pruneDownvotedEntityTags(context.Background())
	s.Require().NoError(err)
	s.Equal(int64(0), deleted2)
}

// Dry-run mode: reports count but deletes nothing.
func (s *CleanupTagPruneTestSuite) TestPrune_DryRun_NoDelete() {
	s.service.tagPruneDryRun = true

	tag := s.createTag("dry")
	et1 := s.applyTag(tag, "artist", 200, 0, 2)
	et2 := s.applyTag(tag, "release", 200, 1, 4)

	deleted, err := s.service.pruneDownvotedEntityTags(context.Background())
	s.Require().NoError(err)
	s.Equal(int64(2), deleted)

	// Neither row was actually deleted.
	s.True(s.entityTagExists(et1.ID))
	s.True(s.entityTagExists(et2.ID))
}

// Feature disabled: cycle is a no-op, no audit log written.
func (s *CleanupTagPruneTestSuite) TestRunTagPruneCycle_Disabled_NoOp() {
	s.service.tagPruneEnabled = false

	tag := s.createTag("disabled")
	et := s.applyTag(tag, "artist", 300, 0, 2)

	s.service.runTagPruneCycle(context.Background())

	// Row still present.
	s.True(s.entityTagExists(et.ID))

	// No audit log entry.
	var count int64
	s.db.Model(&adminm.AuditLog{}).
		Where("action = ?", AuditActionPruneDownvotedTags).
		Count(&count)
	s.Equal(int64(0), count)
}

// Audit log: cycle writes a summary entry with count + threshold + dry_run metadata.
func (s *CleanupTagPruneTestSuite) TestRunTagPruneCycle_WritesAuditLog() {
	tag := s.createTag("audited")
	s.applyTag(tag, "artist", 400, 0, 2)
	s.applyTag(tag, "artist", 401, 0, 3)

	s.service.runTagPruneCycle(context.Background())

	var log adminm.AuditLog
	err := s.db.
		Where("action = ?", AuditActionPruneDownvotedTags).
		First(&log).Error
	s.Require().NoError(err)
	s.Nil(log.ActorID)
	s.Equal("entity_tags", log.EntityType)
	s.Equal(uint(0), log.EntityID)
	s.Require().NotNil(log.Metadata)

	var meta map[string]interface{}
	s.Require().NoError(json.Unmarshal(*log.Metadata, &meta))
	s.Equal(float64(2), meta["deleted_count"])
	s.Equal(float64(MinDownvotesToPrune), meta["threshold_min_downs"])
	s.Equal(false, meta["dry_run"])
}

// Multiple entities across entity_types pruned in a single pass.
func (s *CleanupTagPruneTestSuite) TestPrune_MultipleEntities_MixedOutcomes() {
	tag := s.createTag("mixed")
	pruneA := s.applyTag(tag, "artist", 500, 0, 2)     // pruned
	pruneB := s.applyTag(tag, "release", 500, 1, 3)    // pruned (downs > ups, downs >= 2)
	keepTie := s.applyTag(tag, "artist", 501, 2, 2)    // tie — keep
	keepOneDown := s.applyTag(tag, "label", 500, 0, 1) // below threshold — keep
	keepLoved := s.applyTag(tag, "show", 600, 10, 1)   // ups dominate — keep

	deleted, err := s.service.pruneDownvotedEntityTags(context.Background())
	s.Require().NoError(err)
	s.Equal(int64(2), deleted)

	s.False(s.entityTagExists(pruneA.ID))
	s.False(s.entityTagExists(pruneB.ID))
	s.True(s.entityTagExists(keepTie.ID))
	s.True(s.entityTagExists(keepOneDown.ID))
	s.True(s.entityTagExists(keepLoved.ID))

	// Tag row itself is never deleted.
	s.True(s.tagRowExists(tag.ID))
}

// Safety: only the downvoted entity_tag is removed; other applications of the
// same tag on different entities are unaffected.
func (s *CleanupTagPruneTestSuite) TestPrune_OnlyTargetApplicationRemoved() {
	tag := s.createTag("selective")
	bad := s.applyTag(tag, "artist", 700, 0, 2)    // pruned
	goodA := s.applyTag(tag, "artist", 701, 3, 0)  // keep
	goodB := s.applyTag(tag, "release", 700, 0, 0) // keep (no votes)

	deleted, err := s.service.pruneDownvotedEntityTags(context.Background())
	s.Require().NoError(err)
	s.Equal(int64(1), deleted)

	s.False(s.entityTagExists(bad.ID))
	s.True(s.entityTagExists(goodA.ID))
	s.True(s.entityTagExists(goodB.ID))
	s.True(s.tagRowExists(tag.ID))
}

// Constructor env-var parsing: flags are picked up correctly.
func TestNewCleanupService_TagPruneDefaults(t *testing.T) {
	svc := NewCleanupService(nil, &stubUserService{})
	if svc.tagPruneInterval != DefaultTagPruneInterval {
		t.Errorf("expected default tag prune interval %v, got %v", DefaultTagPruneInterval, svc.tagPruneInterval)
	}
	if !svc.tagPruneEnabled {
		t.Error("expected tag prune enabled by default")
	}
	if svc.tagPruneDryRun {
		t.Error("expected dry-run disabled by default")
	}
}

func TestNewCleanupService_TagPruneEnvOverrides(t *testing.T) {
	t.Setenv("TAG_PRUNE_INTERVAL_HOURS", "6")
	t.Setenv("TAG_PRUNE_ENABLED", "false")
	t.Setenv("TAG_PRUNE_DRY_RUN", "true")

	svc := NewCleanupService(nil, &stubUserService{})
	if svc.tagPruneInterval != 6*time.Hour {
		t.Errorf("expected 6h interval, got %v", svc.tagPruneInterval)
	}
	if svc.tagPruneEnabled {
		t.Error("expected tag prune disabled")
	}
	if !svc.tagPruneDryRun {
		t.Error("expected dry-run enabled")
	}
}

func TestNewCleanupService_TagPruneInvalidEnvIgnored(t *testing.T) {
	t.Setenv("TAG_PRUNE_INTERVAL_HOURS", "not-a-number")
	t.Setenv("TAG_PRUNE_ENABLED", "not-a-bool")
	t.Setenv("TAG_PRUNE_DRY_RUN", "nope")

	svc := NewCleanupService(nil, &stubUserService{})
	if svc.tagPruneInterval != DefaultTagPruneInterval {
		t.Errorf("expected default interval on invalid env, got %v", svc.tagPruneInterval)
	}
	if !svc.tagPruneEnabled {
		t.Error("expected tag prune enabled on invalid bool env")
	}
	if svc.tagPruneDryRun {
		t.Error("expected dry-run disabled on invalid bool env")
	}
}
