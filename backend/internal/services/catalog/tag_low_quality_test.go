package catalog

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	"gorm.io/gorm"

	"psychic-homily-backend/internal/models"
	"psychic-homily-backend/internal/testutil"
)

type TagLowQualityIntegrationSuite struct {
	suite.Suite
	testDB     *testutil.TestDatabase
	db         *gorm.DB
	tagService *TagService
}

func (s *TagLowQualityIntegrationSuite) SetupSuite() {
	s.testDB = testutil.SetupTestPostgres(s.T())
	s.db = s.testDB.DB
	s.tagService = NewTagService(s.db)
}

func (s *TagLowQualityIntegrationSuite) TearDownSuite() {
	s.testDB.Cleanup()
}

func (s *TagLowQualityIntegrationSuite) SetupTest() {
	sqlDB, _ := s.db.DB()
	_, _ = sqlDB.Exec("DELETE FROM audit_logs")
	_, _ = sqlDB.Exec("DELETE FROM tag_votes")
	_, _ = sqlDB.Exec("DELETE FROM entity_tags")
	_, _ = sqlDB.Exec("DELETE FROM tag_aliases")
	_, _ = sqlDB.Exec("DELETE FROM tags")
	_, _ = sqlDB.Exec("DELETE FROM artists")
	_, _ = sqlDB.Exec("DELETE FROM users")
}

func TestTagLowQualityIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	suite.Run(t, new(TagLowQualityIntegrationSuite))
}

// ──────────────────────────────────────────────
// Helpers
// ──────────────────────────────────────────────

func (s *TagLowQualityIntegrationSuite) createUser(name string) *models.User {
	email := fmt.Sprintf("%s-%d@test.com", name, time.Now().UnixNano())
	u := &models.User{Email: &email, FirstName: &name, IsActive: true, EmailVerified: true}
	s.Require().NoError(s.db.Create(u).Error)
	return u
}

func (s *TagLowQualityIntegrationSuite) createArtist(name string) uint {
	slug := fmt.Sprintf("%s-%d", name, time.Now().UnixNano())
	a := &models.Artist{Name: name, Slug: &slug}
	s.Require().NoError(s.db.Create(a).Error)
	return a.ID
}

// createTagRaw inserts a tag bypassing the service's validations so we can set
// arbitrary created_at, name lengths, usage_count, etc. for queue criteria.
func (s *TagLowQualityIntegrationSuite) createTagRaw(name string, opts tagOpts) *models.Tag {
	createdAt := time.Now().UTC()
	if !opts.createdAt.IsZero() {
		createdAt = opts.createdAt
	}
	slug := fmt.Sprintf("%s-%d", name, time.Now().UnixNano())
	tag := &models.Tag{
		Name:       name,
		Slug:       slug,
		Category:   "other",
		IsOfficial: opts.official,
		UsageCount: opts.usageCount,
		CreatedAt:  createdAt,
		UpdatedAt:  createdAt,
		ReviewedAt: opts.reviewedAt,
	}
	s.Require().NoError(s.db.Create(tag).Error)
	return tag
}

type tagOpts struct {
	official   bool
	usageCount int
	createdAt  time.Time
	reviewedAt *time.Time
}

// castVote inserts a tag_vote row. Uses a dedicated artist per call so the
// (tag, entity, user) PK is never violated across different test scenarios.
func (s *TagLowQualityIntegrationSuite) castVote(tagID uint, userID uint, value int) {
	artistID := s.createArtist(fmt.Sprintf("artist-%d-%d", tagID, userID))
	v := &models.TagVote{
		TagID:      tagID,
		EntityType: "artist",
		EntityID:   artistID,
		UserID:     userID,
		Vote:       value,
	}
	s.Require().NoError(s.db.Create(v).Error)
}

// ──────────────────────────────────────────────
// Criteria — individual
// ──────────────────────────────────────────────

func (s *TagLowQualityIntegrationSuite) TestCriterion_Orphaned() {
	tag := s.createTagRaw("orphaned-tag", tagOpts{usageCount: 0})

	resp, err := s.tagService.GetLowQualityTagQueue(20, 0)
	s.Require().NoError(err)
	s.Require().Len(resp.Tags, 1)
	s.Assert().Equal(tag.ID, resp.Tags[0].ID)
	s.Assert().Contains(resp.Tags[0].Reasons, "orphaned")
}

func (s *TagLowQualityIntegrationSuite) TestCriterion_AgingUnused() {
	// Created 10 days ago, usage 1 → aging_unused
	old := time.Now().UTC().AddDate(0, 0, -10)
	tag := s.createTagRaw("aging-tag", tagOpts{usageCount: 1, createdAt: old})

	resp, err := s.tagService.GetLowQualityTagQueue(20, 0)
	s.Require().NoError(err)
	s.Require().Len(resp.Tags, 1)
	s.Assert().Equal(tag.ID, resp.Tags[0].ID)
	s.Assert().Contains(resp.Tags[0].Reasons, "aging_unused")
}

func (s *TagLowQualityIntegrationSuite) TestCriterion_AgingUnused_RecentTagExcluded() {
	// Created 1 day ago, usage 1 → NOT aging_unused (too fresh)
	recent := time.Now().UTC().AddDate(0, 0, -1)
	s.createTagRaw("fresh-tag", tagOpts{usageCount: 1, createdAt: recent})

	resp, err := s.tagService.GetLowQualityTagQueue(20, 0)
	s.Require().NoError(err)
	s.Assert().Empty(resp.Tags)
}

func (s *TagLowQualityIntegrationSuite) TestCriterion_Downvoted() {
	tag := s.createTagRaw("hated-tag", tagOpts{usageCount: 5, createdAt: time.Now().UTC()})
	// Ensure aging_unused doesn't kick in by giving enough usage and recent created_at
	u1 := s.createUser("voter1")
	u2 := s.createUser("voter2")
	u3 := s.createUser("voter3")
	s.castVote(tag.ID, u1.ID, -1)
	s.castVote(tag.ID, u2.ID, -1)
	s.castVote(tag.ID, u3.ID, 1)

	resp, err := s.tagService.GetLowQualityTagQueue(20, 0)
	s.Require().NoError(err)
	s.Require().Len(resp.Tags, 1)
	s.Assert().Equal(tag.ID, resp.Tags[0].ID)
	s.Assert().Contains(resp.Tags[0].Reasons, "downvoted")
	s.Assert().EqualValues(1, resp.Tags[0].Upvotes)
	s.Assert().EqualValues(2, resp.Tags[0].Downvotes)
}

func (s *TagLowQualityIntegrationSuite) TestCriterion_NetPositiveExcluded() {
	tag := s.createTagRaw("liked-tag", tagOpts{usageCount: 5, createdAt: time.Now().UTC()})
	u1 := s.createUser("voter1")
	u2 := s.createUser("voter2")
	s.castVote(tag.ID, u1.ID, 1)
	s.castVote(tag.ID, u2.ID, 1)

	resp, err := s.tagService.GetLowQualityTagQueue(20, 0)
	s.Require().NoError(err)
	s.Assert().Empty(resp.Tags, "tag with net-positive votes and healthy usage should not be flagged")
}

func (s *TagLowQualityIntegrationSuite) TestCriterion_ShortName() {
	// LENGTH(name) < 3 — "ab" qualifies
	tag := s.createTagRaw("ab", tagOpts{usageCount: 5, createdAt: time.Now().UTC()})

	resp, err := s.tagService.GetLowQualityTagQueue(20, 0)
	s.Require().NoError(err)
	s.Require().Len(resp.Tags, 1)
	s.Assert().Equal(tag.ID, resp.Tags[0].ID)
	s.Assert().Contains(resp.Tags[0].Reasons, "short_name")
}

func (s *TagLowQualityIntegrationSuite) TestCriterion_LongName() {
	// LENGTH(name) > 40 — 41 chars
	long := "this-is-an-absurdly-long-tag-name-to-test!!"
	s.Require().Greater(len(long), 40)
	tag := s.createTagRaw(long, tagOpts{usageCount: 5, createdAt: time.Now().UTC()})

	resp, err := s.tagService.GetLowQualityTagQueue(20, 0)
	s.Require().NoError(err)
	s.Require().Len(resp.Tags, 1)
	s.Assert().Equal(tag.ID, resp.Tags[0].ID)
	s.Assert().Contains(resp.Tags[0].Reasons, "long_name")
}

// ──────────────────────────────────────────────
// Criteria — combinations and exclusions
// ──────────────────────────────────────────────

func (s *TagLowQualityIntegrationSuite) TestMultipleReasons() {
	// Orphaned AND short name — should return both reasons.
	tag := s.createTagRaw("x", tagOpts{usageCount: 0})

	resp, err := s.tagService.GetLowQualityTagQueue(20, 0)
	s.Require().NoError(err)
	s.Require().Len(resp.Tags, 1)
	s.Assert().Equal(tag.ID, resp.Tags[0].ID)
	s.Assert().Contains(resp.Tags[0].Reasons, "orphaned")
	s.Assert().Contains(resp.Tags[0].Reasons, "short_name")
	s.Assert().Len(resp.Tags[0].Reasons, 2)
}

func (s *TagLowQualityIntegrationSuite) TestOfficialTagsExcluded() {
	// Official tags are exempt even if they would otherwise qualify.
	s.createTagRaw("ab", tagOpts{usageCount: 0, official: true})
	s.createTagRaw("orphaned-admin-tag", tagOpts{usageCount: 0, official: true})

	resp, err := s.tagService.GetLowQualityTagQueue(20, 0)
	s.Require().NoError(err)
	s.Assert().Empty(resp.Tags)
}

func (s *TagLowQualityIntegrationSuite) TestSnoozedRecently_Excluded() {
	// Snoozed 10 days ago — still within the 30-day window, excluded.
	snoozed := time.Now().UTC().AddDate(0, 0, -10)
	s.createTagRaw("snoozed-tag", tagOpts{usageCount: 0, reviewedAt: &snoozed})

	resp, err := s.tagService.GetLowQualityTagQueue(20, 0)
	s.Require().NoError(err)
	s.Assert().Empty(resp.Tags)
}

func (s *TagLowQualityIntegrationSuite) TestSnoozedExpired_Included() {
	// Snoozed 40 days ago — past the 30-day window, re-surfaces.
	snoozed := time.Now().UTC().AddDate(0, 0, -40)
	tag := s.createTagRaw("old-snooze-tag", tagOpts{usageCount: 0, reviewedAt: &snoozed})

	resp, err := s.tagService.GetLowQualityTagQueue(20, 0)
	s.Require().NoError(err)
	s.Require().Len(resp.Tags, 1)
	s.Assert().Equal(tag.ID, resp.Tags[0].ID)
}

// ──────────────────────────────────────────────
// Pagination and ordering
// ──────────────────────────────────────────────

func (s *TagLowQualityIntegrationSuite) TestPagination() {
	// Create 5 flagged tags spaced by timestamp so order is predictable.
	base := time.Now().UTC()
	for i := 0; i < 5; i++ {
		s.createTagRaw(fmt.Sprintf("orphan-%d", i), tagOpts{
			usageCount: 0,
			createdAt:  base.Add(-time.Duration(i) * time.Hour), // Newer first
		})
	}

	resp, err := s.tagService.GetLowQualityTagQueue(2, 0)
	s.Require().NoError(err)
	s.Assert().Len(resp.Tags, 2)
	s.Assert().EqualValues(5, resp.Total)

	// Next page
	resp2, err := s.tagService.GetLowQualityTagQueue(2, 2)
	s.Require().NoError(err)
	s.Assert().Len(resp2.Tags, 2)
	s.Assert().EqualValues(5, resp2.Total)

	// Ensure page 2 is different from page 1
	s.Assert().NotEqual(resp.Tags[0].ID, resp2.Tags[0].ID)
}

func (s *TagLowQualityIntegrationSuite) TestNewestFirstOrder() {
	base := time.Now().UTC()
	older := s.createTagRaw("orphan-older", tagOpts{usageCount: 0, createdAt: base.Add(-2 * time.Hour)})
	newer := s.createTagRaw("orphan-newer", tagOpts{usageCount: 0, createdAt: base.Add(-1 * time.Hour)})

	resp, err := s.tagService.GetLowQualityTagQueue(20, 0)
	s.Require().NoError(err)
	s.Require().Len(resp.Tags, 2)
	s.Assert().Equal(newer.ID, resp.Tags[0].ID, "newer tag should come first")
	s.Assert().Equal(older.ID, resp.Tags[1].ID)
}

// ──────────────────────────────────────────────
// Snooze
// ──────────────────────────────────────────────

func (s *TagLowQualityIntegrationSuite) TestSnoozeLowQualityTag_SetsReviewedAt() {
	actor := s.createUser("admin")
	tag := s.createTagRaw("snooze-me", tagOpts{usageCount: 0})

	err := s.tagService.SnoozeLowQualityTag(tag.ID, actor.ID)
	s.Require().NoError(err)

	var refreshed models.Tag
	s.Require().NoError(s.db.First(&refreshed, tag.ID).Error)
	s.Require().NotNil(refreshed.ReviewedAt)
	s.Assert().WithinDuration(time.Now().UTC(), *refreshed.ReviewedAt, 10*time.Second)

	// And the queue should now skip it.
	resp, err := s.tagService.GetLowQualityTagQueue(20, 0)
	s.Require().NoError(err)
	s.Assert().Empty(resp.Tags)
}

func (s *TagLowQualityIntegrationSuite) TestSnoozeLowQualityTag_NotFound() {
	actor := s.createUser("admin")
	err := s.tagService.SnoozeLowQualityTag(99999, actor.ID)
	s.Assert().Error(err)
}

// ──────────────────────────────────────────────
// Bulk action (PSY-487)
// ──────────────────────────────────────────────

func (s *TagLowQualityIntegrationSuite) TestBulkAction_Snooze_HappyPath() {
	t1 := s.createTagRaw("orphan-bulk-1", tagOpts{usageCount: 0})
	t2 := s.createTagRaw("orphan-bulk-2", tagOpts{usageCount: 0})
	t3 := s.createTagRaw("orphan-bulk-3", tagOpts{usageCount: 0})

	res, err := s.tagService.BulkActionLowQualityTags(BulkActionSnooze, []uint{t1.ID, t2.ID, t3.ID})
	s.Require().NoError(err)
	s.Require().NotNil(res)
	s.Assert().Equal(BulkActionSnooze, res.Action)
	s.Assert().EqualValues(3, res.Requested)
	s.Assert().EqualValues(3, res.Affected)
	s.Assert().EqualValues(0, res.NotFound)

	// reviewed_at should now be set on all three.
	for _, id := range []uint{t1.ID, t2.ID, t3.ID} {
		var refreshed models.Tag
		s.Require().NoError(s.db.First(&refreshed, id).Error)
		s.Require().NotNil(refreshed.ReviewedAt)
	}

	// Queue should be empty now (snoozed within window).
	q, err := s.tagService.GetLowQualityTagQueue(20, 0)
	s.Require().NoError(err)
	s.Assert().Empty(q.Tags)
}

func (s *TagLowQualityIntegrationSuite) TestBulkAction_Delete_RemovesTags() {
	t1 := s.createTagRaw("delete-me-1", tagOpts{usageCount: 0})
	t2 := s.createTagRaw("delete-me-2", tagOpts{usageCount: 0})

	res, err := s.tagService.BulkActionLowQualityTags(BulkActionDelete, []uint{t1.ID, t2.ID})
	s.Require().NoError(err)
	s.Assert().EqualValues(2, res.Affected)

	var count int64
	s.Require().NoError(s.db.Model(&models.Tag{}).Where("id IN ?", []uint{t1.ID, t2.ID}).Count(&count).Error)
	s.Assert().EqualValues(0, count)
}

func (s *TagLowQualityIntegrationSuite) TestBulkAction_MarkOfficial_FlipsFlag() {
	t1 := s.createTagRaw("promote-1", tagOpts{usageCount: 0})
	t2 := s.createTagRaw("promote-2", tagOpts{usageCount: 0})

	res, err := s.tagService.BulkActionLowQualityTags(BulkActionMarkOfficial, []uint{t1.ID, t2.ID})
	s.Require().NoError(err)
	s.Assert().EqualValues(2, res.Affected)

	for _, id := range []uint{t1.ID, t2.ID} {
		var refreshed models.Tag
		s.Require().NoError(s.db.First(&refreshed, id).Error)
		s.Assert().True(refreshed.IsOfficial)
	}

	// Queue should be empty (official tags are excluded).
	q, err := s.tagService.GetLowQualityTagQueue(20, 0)
	s.Require().NoError(err)
	s.Assert().Empty(q.Tags)
}

func (s *TagLowQualityIntegrationSuite) TestBulkAction_NotFoundCounted() {
	t1 := s.createTagRaw("real-1", tagOpts{usageCount: 0})

	res, err := s.tagService.BulkActionLowQualityTags(BulkActionSnooze, []uint{t1.ID, 999998, 999999})
	s.Require().NoError(err)
	s.Assert().EqualValues(3, res.Requested)
	s.Assert().EqualValues(1, res.Affected)
	s.Assert().EqualValues(2, res.NotFound)
}

func (s *TagLowQualityIntegrationSuite) TestBulkAction_DedupesIDs() {
	t1 := s.createTagRaw("dupe-target", tagOpts{usageCount: 0})

	// Same ID three times — should count as 1 requested, 1 affected.
	res, err := s.tagService.BulkActionLowQualityTags(BulkActionSnooze, []uint{t1.ID, t1.ID, t1.ID})
	s.Require().NoError(err)
	s.Assert().EqualValues(1, res.Requested)
	s.Assert().EqualValues(1, res.Affected)
}

func (s *TagLowQualityIntegrationSuite) TestBulkAction_RejectsEmptyIDs() {
	_, err := s.tagService.BulkActionLowQualityTags(BulkActionSnooze, []uint{})
	s.Require().Error(err)
}

func (s *TagLowQualityIntegrationSuite) TestBulkAction_RejectsAllZeroIDs() {
	_, err := s.tagService.BulkActionLowQualityTags(BulkActionSnooze, []uint{0, 0, 0})
	s.Require().Error(err)
}

func (s *TagLowQualityIntegrationSuite) TestBulkAction_RejectsOverLimit() {
	ids := make([]uint, BulkActionMaxTagIDs+1)
	for i := range ids {
		ids[i] = uint(i + 1)
	}
	_, err := s.tagService.BulkActionLowQualityTags(BulkActionSnooze, ids)
	s.Require().Error(err)
}

func (s *TagLowQualityIntegrationSuite) TestBulkAction_RejectsUnknownAction() {
	t1 := s.createTagRaw("victim", tagOpts{usageCount: 0})
	_, err := s.tagService.BulkActionLowQualityTags("nuke-from-orbit", []uint{t1.ID})
	s.Require().Error(err)
}
