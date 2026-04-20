package catalog

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	"gorm.io/gorm"

	apperrors "psychic-homily-backend/internal/errors"
	"psychic-homily-backend/internal/models"
	"psychic-homily-backend/internal/testutil"
)

type TagHierarchyIntegrationSuite struct {
	suite.Suite
	testDB     *testutil.TestDatabase
	db         *gorm.DB
	tagService *TagService
}

func (s *TagHierarchyIntegrationSuite) SetupSuite() {
	s.testDB = testutil.SetupTestPostgres(s.T())
	s.db = s.testDB.DB
	s.tagService = NewTagService(s.db)
}

func (s *TagHierarchyIntegrationSuite) TearDownSuite() {
	s.testDB.Cleanup()
}

func (s *TagHierarchyIntegrationSuite) SetupTest() {
	sqlDB, _ := s.db.DB()
	_, _ = sqlDB.Exec("DELETE FROM audit_logs")
	_, _ = sqlDB.Exec("DELETE FROM tag_votes")
	_, _ = sqlDB.Exec("DELETE FROM entity_tags")
	_, _ = sqlDB.Exec("DELETE FROM tag_aliases")
	_, _ = sqlDB.Exec("DELETE FROM tags")
	_, _ = sqlDB.Exec("DELETE FROM users")
}

func TestTagHierarchyIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	suite.Run(t, new(TagHierarchyIntegrationSuite))
}

// ──────────────────────────────────────────────
// Helpers
// ──────────────────────────────────────────────

func (s *TagHierarchyIntegrationSuite) createUser(name string) *models.User {
	email := fmt.Sprintf("%s-%d@test.com", name, time.Now().UnixNano())
	u := &models.User{Email: &email, FirstName: &name, IsActive: true, EmailVerified: true}
	s.Require().NoError(s.db.Create(u).Error)
	return u
}

// createGenre creates a genre tag (no parent).
func (s *TagHierarchyIntegrationSuite) createGenre(name string) *models.Tag {
	tag, err := s.tagService.CreateTag(name, nil, nil, models.TagCategoryGenre, false, nil)
	s.Require().NoError(err)
	return tag
}

// createTagWithCategory creates a tag in a specific category (for rejection tests).
func (s *TagHierarchyIntegrationSuite) createTagWithCategory(name, category string) *models.Tag {
	tag, err := s.tagService.CreateTag(name, nil, nil, category, false, nil)
	s.Require().NoError(err)
	return tag
}

// ──────────────────────────────────────────────
// Happy path: set and clear
// ──────────────────────────────────────────────

func (s *TagHierarchyIntegrationSuite) TestSetTagParent_Simple() {
	admin := s.createUser("admin")
	parent := s.createGenre("post-punk")
	child := s.createGenre("shoegaze")

	err := s.tagService.SetTagParent(child.ID, &parent.ID, admin.ID)
	s.Require().NoError(err)

	var reloaded models.Tag
	s.Require().NoError(s.db.First(&reloaded, child.ID).Error)
	s.Require().NotNil(reloaded.ParentID)
	s.Equal(parent.ID, *reloaded.ParentID)
}

func (s *TagHierarchyIntegrationSuite) TestSetTagParent_Clear() {
	admin := s.createUser("admin")
	parent := s.createGenre("post-punk")
	child := s.createGenre("shoegaze")

	// Set first, then clear.
	s.Require().NoError(s.tagService.SetTagParent(child.ID, &parent.ID, admin.ID))
	s.Require().NoError(s.tagService.SetTagParent(child.ID, nil, admin.ID))

	var reloaded models.Tag
	s.Require().NoError(s.db.First(&reloaded, child.ID).Error)
	s.Nil(reloaded.ParentID, "parent_id should be NULL after clearing")
}

// ──────────────────────────────────────────────
// Cycle detection
// ──────────────────────────────────────────────

// Direct self-parent: tag cannot be its own parent.
func (s *TagHierarchyIntegrationSuite) TestSetTagParent_SelfParent_Rejected() {
	admin := s.createUser("admin")
	tag := s.createGenre("shoegaze")

	err := s.tagService.SetTagParent(tag.ID, &tag.ID, admin.ID)
	s.Require().Error(err)
	var tagErr *apperrors.TagError
	s.Require().ErrorAs(err, &tagErr)
	s.Equal(apperrors.CodeTagHierarchyCycle, tagErr.Code)
}

// Depth-2 cycle: A is the parent of B, then try setting A's parent = B.
func (s *TagHierarchyIntegrationSuite) TestSetTagParent_Cycle_Depth2_Rejected() {
	admin := s.createUser("admin")
	a := s.createGenre("a")
	b := s.createGenre("b")

	// A → B (B's parent is A)
	s.Require().NoError(s.tagService.SetTagParent(b.ID, &a.ID, admin.ID))

	// Try: A's parent = B (would create cycle A → B → A)
	err := s.tagService.SetTagParent(a.ID, &b.ID, admin.ID)
	s.Require().Error(err)
	var tagErr *apperrors.TagError
	s.Require().ErrorAs(err, &tagErr)
	s.Equal(apperrors.CodeTagHierarchyCycle, tagErr.Code)
}

// Depth-3 cycle: A → B → C, then try setting A's parent = C.
func (s *TagHierarchyIntegrationSuite) TestSetTagParent_Cycle_Depth3_Rejected() {
	admin := s.createUser("admin")
	a := s.createGenre("a")
	b := s.createGenre("b")
	c := s.createGenre("c")

	s.Require().NoError(s.tagService.SetTagParent(b.ID, &a.ID, admin.ID))
	s.Require().NoError(s.tagService.SetTagParent(c.ID, &b.ID, admin.ID))

	// A's parent = C would make: C → B → A → C
	err := s.tagService.SetTagParent(a.ID, &c.ID, admin.ID)
	s.Require().Error(err)
	var tagErr *apperrors.TagError
	s.Require().ErrorAs(err, &tagErr)
	s.Equal(apperrors.CodeTagHierarchyCycle, tagErr.Code)
}

// Depth-4 cycle: A → B → C → D, then try setting A's parent = D.
func (s *TagHierarchyIntegrationSuite) TestSetTagParent_Cycle_Depth4_Rejected() {
	admin := s.createUser("admin")
	a := s.createGenre("a")
	b := s.createGenre("b")
	c := s.createGenre("c")
	d := s.createGenre("d")

	s.Require().NoError(s.tagService.SetTagParent(b.ID, &a.ID, admin.ID))
	s.Require().NoError(s.tagService.SetTagParent(c.ID, &b.ID, admin.ID))
	s.Require().NoError(s.tagService.SetTagParent(d.ID, &c.ID, admin.ID))

	err := s.tagService.SetTagParent(a.ID, &d.ID, admin.ID)
	s.Require().Error(err)
	var tagErr *apperrors.TagError
	s.Require().ErrorAs(err, &tagErr)
	s.Equal(apperrors.CodeTagHierarchyCycle, tagErr.Code)
}

// Sibling-ish: A → B and A → C both OK. Setting C's parent = B (uncle) is fine,
// and NOT a cycle — verifies we don't over-reject.
func (s *TagHierarchyIntegrationSuite) TestSetTagParent_SiblingMove_Allowed() {
	admin := s.createUser("admin")
	a := s.createGenre("a")
	b := s.createGenre("b")
	c := s.createGenre("c")

	s.Require().NoError(s.tagService.SetTagParent(b.ID, &a.ID, admin.ID))
	s.Require().NoError(s.tagService.SetTagParent(c.ID, &a.ID, admin.ID))

	// C moves under B: A → B → C. No cycle.
	err := s.tagService.SetTagParent(c.ID, &b.ID, admin.ID)
	s.Require().NoError(err)
}

// ──────────────────────────────────────────────
// Category enforcement
// ──────────────────────────────────────────────

// Tag is not a genre → reject.
func (s *TagHierarchyIntegrationSuite) TestSetTagParent_NonGenreSource_Rejected() {
	admin := s.createUser("admin")
	parent := s.createGenre("post-punk")
	locale := s.createTagWithCategory("arizona", models.TagCategoryLocale)

	err := s.tagService.SetTagParent(locale.ID, &parent.ID, admin.ID)
	s.Require().Error(err)
	var tagErr *apperrors.TagError
	s.Require().ErrorAs(err, &tagErr)
	s.Equal(apperrors.CodeTagHierarchyNotGenre, tagErr.Code)
}

// Proposed parent is not a genre → reject.
func (s *TagHierarchyIntegrationSuite) TestSetTagParent_NonGenreParent_Rejected() {
	admin := s.createUser("admin")
	child := s.createGenre("shoegaze")
	locale := s.createTagWithCategory("arizona", models.TagCategoryLocale)

	err := s.tagService.SetTagParent(child.ID, &locale.ID, admin.ID)
	s.Require().Error(err)
	var tagErr *apperrors.TagError
	s.Require().ErrorAs(err, &tagErr)
	s.Equal(apperrors.CodeTagHierarchyNotGenre, tagErr.Code)
}

// ──────────────────────────────────────────────
// Not found
// ──────────────────────────────────────────────

func (s *TagHierarchyIntegrationSuite) TestSetTagParent_TagNotFound() {
	admin := s.createUser("admin")
	parent := s.createGenre("post-punk")

	err := s.tagService.SetTagParent(99999, &parent.ID, admin.ID)
	s.Require().Error(err)
	var tagErr *apperrors.TagError
	s.Require().ErrorAs(err, &tagErr)
	s.Equal(apperrors.CodeTagNotFound, tagErr.Code)
}

func (s *TagHierarchyIntegrationSuite) TestSetTagParent_ParentNotFound() {
	admin := s.createUser("admin")
	child := s.createGenre("shoegaze")

	err := s.tagService.SetTagParent(child.ID, uintPtr(99999), admin.ID)
	s.Require().Error(err)
	var tagErr *apperrors.TagError
	s.Require().ErrorAs(err, &tagErr)
	s.Equal(apperrors.CodeTagNotFound, tagErr.Code)
}

// ──────────────────────────────────────────────
// GetGenreHierarchy / GetTagAncestors / GetTagChildren
// ──────────────────────────────────────────────

func (s *TagHierarchyIntegrationSuite) TestGetGenreHierarchy_OnlyGenres() {
	admin := s.createUser("admin")
	g1 := s.createGenre("rock")
	g2 := s.createGenre("post-rock")
	// Non-genre tags must not appear.
	_ = s.createTagWithCategory("arizona", models.TagCategoryLocale)
	_ = s.createTagWithCategory("misc", models.TagCategoryOther)

	// Wire up a parent link.
	s.Require().NoError(s.tagService.SetTagParent(g2.ID, &g1.ID, admin.ID))

	tags, err := s.tagService.GetGenreHierarchy()
	s.Require().NoError(err)
	s.Len(tags, 2, "hierarchy should include only genre tags")

	byName := map[string]*models.Tag{}
	for _, t := range tags {
		byName[t.Name] = t
	}
	s.Require().Contains(byName, "rock")
	s.Require().Contains(byName, "post-rock")
	s.Nil(byName["rock"].ParentID)
	s.Require().NotNil(byName["post-rock"].ParentID)
	s.Equal(g1.ID, *byName["post-rock"].ParentID)
}

func (s *TagHierarchyIntegrationSuite) TestGetTagAncestors_WalksChain() {
	admin := s.createUser("admin")
	a := s.createGenre("a")
	b := s.createGenre("b")
	c := s.createGenre("c")
	s.Require().NoError(s.tagService.SetTagParent(b.ID, &a.ID, admin.ID))
	s.Require().NoError(s.tagService.SetTagParent(c.ID, &b.ID, admin.ID))

	ancestors, err := s.tagService.GetTagAncestors(c.ID)
	s.Require().NoError(err)
	s.Require().Len(ancestors, 2)
	// Closest-first ordering.
	s.Equal(b.ID, ancestors[0].ID)
	s.Equal(a.ID, ancestors[1].ID)
}

func (s *TagHierarchyIntegrationSuite) TestGetTagAncestors_Root_Empty() {
	root := s.createGenre("rock")
	ancestors, err := s.tagService.GetTagAncestors(root.ID)
	s.Require().NoError(err)
	s.Empty(ancestors)
}

func (s *TagHierarchyIntegrationSuite) TestGetTagChildren() {
	admin := s.createUser("admin")
	parent := s.createGenre("rock")
	c1 := s.createGenre("post-rock")
	c2 := s.createGenre("math-rock")
	_ = s.createGenre("orphan") // no parent — shouldn't appear

	s.Require().NoError(s.tagService.SetTagParent(c1.ID, &parent.ID, admin.ID))
	s.Require().NoError(s.tagService.SetTagParent(c2.ID, &parent.ID, admin.ID))

	children, err := s.tagService.GetTagChildren(parent.ID)
	s.Require().NoError(err)
	s.Require().Len(children, 2)
	names := map[string]bool{}
	for _, c := range children {
		names[c.Name] = true
	}
	s.True(names["post-rock"])
	s.True(names["math-rock"])
}

// ──────────────────────────────────────────────
// UpdateTag parent-path: also protected
// ──────────────────────────────────────────────

// UpdateTag is the legacy CRUD entry point. Setting parent_id via UpdateTag
// must go through the same cycle check as SetTagParent.
func (s *TagHierarchyIntegrationSuite) TestUpdateTag_SetsParent_WithCycleGuard() {
	admin := s.createUser("admin")
	a := s.createGenre("a")
	b := s.createGenre("b")
	// A → B via UpdateTag.
	_, err := s.tagService.UpdateTag(b.ID, nil, nil, &a.ID, nil, nil)
	s.Require().NoError(err)

	// Now B → A via UpdateTag would be a cycle.
	_, err = s.tagService.UpdateTag(a.ID, nil, nil, &b.ID, nil, nil)
	s.Require().Error(err)
	var tagErr *apperrors.TagError
	s.Require().ErrorAs(err, &tagErr)
	s.Equal(apperrors.CodeTagHierarchyCycle, tagErr.Code)
	_ = admin
}

// UpdateTag on a non-genre tag that tries to set parent_id should be rejected
// too — the hierarchy is genre-only, end-to-end.
func (s *TagHierarchyIntegrationSuite) TestUpdateTag_NonGenreWithParentID_Rejected() {
	admin := s.createUser("admin")
	genre := s.createGenre("rock")
	locale := s.createTagWithCategory("arizona", models.TagCategoryLocale)

	_, err := s.tagService.UpdateTag(locale.ID, nil, nil, &genre.ID, nil, nil)
	s.Require().Error(err)
	var tagErr *apperrors.TagError
	s.Require().ErrorAs(err, &tagErr)
	s.Equal(apperrors.CodeTagHierarchyNotGenre, tagErr.Code)
	_ = admin
}

// ──────────────────────────────────────────────
// Audit log
// ──────────────────────────────────────────────

func (s *TagHierarchyIntegrationSuite) TestSetTagParent_WritesAuditLog() {
	admin := s.createUser("admin")
	parent := s.createGenre("post-punk")
	child := s.createGenre("shoegaze")

	err := s.tagService.SetTagParent(child.ID, &parent.ID, admin.ID)
	s.Require().NoError(err)

	// Fire-and-forget audit log — poll briefly so the goroutine wins.
	var log models.AuditLog
	for i := 0; i < 40; i++ {
		err := s.db.Where("action = ? AND entity_id = ?", AuditActionSetTagParent, child.ID).First(&log).Error
		if err == nil {
			break
		}
		time.Sleep(25 * time.Millisecond)
	}
	s.Require().NotZero(log.ID, "audit log was not written in time")
	s.Equal("tag", log.EntityType)
	s.Equal(child.ID, log.EntityID)
	s.Require().NotNil(log.ActorID)
	s.Equal(admin.ID, *log.ActorID)

	s.Require().NotNil(log.Metadata)
	var meta map[string]interface{}
	s.Require().NoError(json.Unmarshal(*log.Metadata, &meta))
	s.Equal(float64(child.ID), meta["tag_id"])
	s.Equal("shoegaze", meta["tag_name"])
	s.Equal(float64(parent.ID), meta["parent_id"])
	s.Equal("post-punk", meta["parent_name"])
}

func (s *TagHierarchyIntegrationSuite) TestSetTagParent_ClearParent_AuditReflectsNull() {
	admin := s.createUser("admin")
	parent := s.createGenre("post-punk")
	child := s.createGenre("shoegaze")

	s.Require().NoError(s.tagService.SetTagParent(child.ID, &parent.ID, admin.ID))
	// Poll until the first audit log arrives so we don't race the clear below.
	for i := 0; i < 40; i++ {
		var count int64
		s.db.Model(&models.AuditLog{}).
			Where("action = ? AND entity_id = ?", AuditActionSetTagParent, child.ID).
			Count(&count)
		if count > 0 {
			break
		}
		time.Sleep(25 * time.Millisecond)
	}

	s.Require().NoError(s.tagService.SetTagParent(child.ID, nil, admin.ID))

	// Wait for the second audit entry (the clear).
	var logs []models.AuditLog
	for i := 0; i < 40; i++ {
		s.db.Where("action = ? AND entity_id = ?", AuditActionSetTagParent, child.ID).
			Order("id ASC").Find(&logs)
		if len(logs) >= 2 {
			break
		}
		time.Sleep(25 * time.Millisecond)
	}
	s.Require().GreaterOrEqual(len(logs), 2)

	var meta map[string]interface{}
	s.Require().NoError(json.Unmarshal(*logs[len(logs)-1].Metadata, &meta))
	s.Nil(meta["parent_id"], "cleared parent should serialize as JSON null")
	s.Nil(meta["parent_name"])
}
