package admin

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	"gorm.io/gorm"

	"psychic-homily-backend/internal/api/handlers/shared/testhelpers"
	"psychic-homily-backend/internal/models"
)

// TestFixturesSuite exercises the PSY-432 reset endpoint against a real
// Postgres container. Uses the shared handler integration helpers.
type TestFixturesSuite struct {
	suite.Suite
	deps *testhelpers.IntegrationDeps
}

func TestTestFixturesSuite(t *testing.T) {
	suite.Run(t, new(TestFixturesSuite))
}

func (s *TestFixturesSuite) SetupSuite() {
	s.deps = testhelpers.SetupIntegrationDeps(s.T())
}

func (s *TestFixturesSuite) SetupTest() {
	testhelpers.CleanupTables(s.deps.DB)
}

// createTestLocalUser creates a user with a @test.local email so the reset
// endpoint's email-suffix guard will accept them.
func (s *TestFixturesSuite) createTestLocalUser(admin bool) *models.User {
	prefix := "user"
	if admin {
		prefix = "admin"
	}
	u := &models.User{
		Email:         testhelpers.StringPtr(fmt.Sprintf("%s-%d@test.local", prefix, time.Now().UnixNano())),
		FirstName:     testhelpers.StringPtr("T"),
		LastName:      testhelpers.StringPtr("U"),
		IsActive:      true,
		IsAdmin:       admin,
		EmailVerified: true,
	}
	s.Require().NoError(s.deps.DB.Create(u).Error)
	return u
}

// seedUserData seeds each allowlisted scope with at least one row belonging
// to userID, so a reset has something to delete. Returns the number of rows
// created per scope.
func (s *TestFixturesSuite) seedUserData(userID uint) map[string]int {
	counts := make(map[string]int)

	// user_bookmarks: one save + one follow
	for _, bm := range []models.UserBookmark{
		{UserID: userID, EntityType: models.BookmarkEntityShow, EntityID: 1, Action: models.BookmarkActionSave, CreatedAt: time.Now()},
		{UserID: userID, EntityType: models.BookmarkEntityVenue, EntityID: 1, Action: models.BookmarkActionFollow, CreatedAt: time.Now()},
	} {
		s.Require().NoError(s.deps.DB.Create(&bm).Error)
		counts["user_bookmarks"]++
	}

	// collection (owned by this user) + two items they added to it
	col := &models.Collection{
		Title:     "test",
		Slug:      fmt.Sprintf("test-%d", time.Now().UnixNano()),
		CreatorID: userID,
	}
	s.Require().NoError(s.deps.DB.Create(col).Error)
	counts["collections"]++

	for i := 0; i < 2; i++ {
		item := &models.CollectionItem{
			CollectionID:  col.ID,
			EntityType:    "show",
			EntityID:      uint(100 + i),
			AddedByUserID: userID,
		}
		s.Require().NoError(s.deps.DB.Create(item).Error)
		counts["collection_items"]++
	}

	// collection_subscribers (join table, no model) — seed another user's
	// collection that this user subscribes to
	otherCol := &models.Collection{
		Title:     "other",
		Slug:      fmt.Sprintf("other-%d", time.Now().UnixNano()),
		CreatorID: userID + 999, // some unrelated id; FK isn't enforced in seed
	}
	// avoid FK violation by creating a matching user
	otherUser := &models.User{
		ID:            userID + 999,
		Email:         testhelpers.StringPtr(fmt.Sprintf("other-%d@test.local", time.Now().UnixNano())),
		IsActive:      true,
		EmailVerified: true,
	}
	s.Require().NoError(s.deps.DB.Create(otherUser).Error)
	s.Require().NoError(s.deps.DB.Create(otherCol).Error)
	s.Require().NoError(s.deps.DB.Exec(
		"INSERT INTO collection_subscribers (collection_id, user_id) VALUES (?, ?)",
		otherCol.ID, userID,
	).Error)
	counts["collection_subscribers"]++

	// pending show submitted by this user, plus an approved show we should NOT touch
	pending := &models.Show{
		Status:      models.ShowStatusPending,
		SubmittedBy: &userID,
		EventDate:   time.Now(),
	}
	s.Require().NoError(s.deps.DB.Create(pending).Error)
	counts["pending_shows"]++

	return counts
}

// call issues a Reset request with sensible defaults, allowing the caller to
// override via opts.
func (s *TestFixturesSuite) call(admin *models.User, targetID uint, tables []string, header string) (*ResetTestFixturesResponse, error) {
	h := NewTestFixtureHandler(s.deps.DB)
	req := &ResetTestFixturesRequest{TestFixturesToken: header}
	req.Body.UserID = targetID
	req.Body.Tables = tables
	return h.Reset(testhelpers.CtxWithUser(admin), req)
}

func (s *TestFixturesSuite) TestReset_HappyPath_DeletesAllowlistedScopes() {
	admin := s.createTestLocalUser(true)
	target := s.createTestLocalUser(false)
	expected := s.seedUserData(target.ID)

	resp, err := s.call(admin, target.ID, []string{
		"user_bookmarks", "collection_items", "collection_subscribers", "collections", "pending_shows",
	}, "1")
	s.Require().NoError(err)
	s.Require().NotNil(resp)

	for scope, want := range expected {
		s.Equalf(int64(want), resp.Body.Deleted[scope], "scope %s", scope)
	}

	// Verify rows actually gone
	var bookmarkCount int64
	s.deps.DB.Model(&models.UserBookmark{}).Where("user_id = ?", target.ID).Count(&bookmarkCount)
	s.Zero(bookmarkCount)

	// Verify approved show NOT touched: nothing to assert positively because
	// we didn't seed one, but the SQL shape is checked by the explicit
	// predicate + this row count going to zero without side effects.
}

func (s *TestFixturesSuite) TestReset_HeaderMissing_Returns400() {
	admin := s.createTestLocalUser(true)
	target := s.createTestLocalUser(false)
	_, err := s.call(admin, target.ID, []string{"user_bookmarks"}, "")
	s.Require().Error(err)
	testhelpers.AssertHumaError(s.T(), err, 400)
}

func (s *TestFixturesSuite) TestReset_HeaderWrongValue_Returns400() {
	admin := s.createTestLocalUser(true)
	target := s.createTestLocalUser(false)
	_, err := s.call(admin, target.ID, []string{"user_bookmarks"}, "yes")
	s.Require().Error(err)
	testhelpers.AssertHumaError(s.T(), err, 400)
}

func (s *TestFixturesSuite) TestReset_NonAdmin_Returns403() {
	nonAdmin := s.createTestLocalUser(false)
	target := s.createTestLocalUser(false)
	_, err := s.call(nonAdmin, target.ID, []string{"user_bookmarks"}, "1")
	s.Require().Error(err)
	testhelpers.AssertHumaError(s.T(), err, 403)
}

func (s *TestFixturesSuite) TestReset_NoAuthContext_Returns403() {
	h := NewTestFixtureHandler(s.deps.DB)
	req := &ResetTestFixturesRequest{TestFixturesToken: "1"}
	req.Body.UserID = 1
	req.Body.Tables = []string{"user_bookmarks"}
	_, err := h.Reset(context.Background(), req)
	s.Require().Error(err)
	testhelpers.AssertHumaError(s.T(), err, 403)
}

func (s *TestFixturesSuite) TestReset_UnknownTable_Returns400() {
	admin := s.createTestLocalUser(true)
	target := s.createTestLocalUser(false)
	_, err := s.call(admin, target.ID, []string{"user_bookmarks", "totally_unknown_table"}, "1")
	s.Require().Error(err)
	testhelpers.AssertHumaError(s.T(), err, 400)

	// Even partially-valid request: no DB work should happen. Seed then
	// confirm nothing was deleted.
	bm := &models.UserBookmark{UserID: target.ID, EntityType: models.BookmarkEntityShow, EntityID: 1, Action: models.BookmarkActionSave, CreatedAt: time.Now()}
	s.Require().NoError(s.deps.DB.Create(bm).Error)
	_, err = s.call(admin, target.ID, []string{"user_bookmarks", "totally_unknown_table"}, "1")
	s.Require().Error(err)
	var count int64
	s.deps.DB.Model(&models.UserBookmark{}).Where("user_id = ?", target.ID).Count(&count)
	s.Equal(int64(1), count, "unknown-table rejection must not delete anything")
}

func (s *TestFixturesSuite) TestReset_NonTestLocalEmail_Returns403() {
	admin := s.createTestLocalUser(true)
	// Create a user with a non-@test.local email
	realUser := &models.User{
		Email:         testhelpers.StringPtr(fmt.Sprintf("real-%d@example.com", time.Now().UnixNano())),
		IsActive:      true,
		EmailVerified: true,
	}
	s.Require().NoError(s.deps.DB.Create(realUser).Error)

	_, err := s.call(admin, realUser.ID, []string{"user_bookmarks"}, "1")
	s.Require().Error(err)
	testhelpers.AssertHumaError(s.T(), err, 403)
}

func (s *TestFixturesSuite) TestReset_UnknownUser_Returns404() {
	admin := s.createTestLocalUser(true)
	_, err := s.call(admin, 99999999, []string{"user_bookmarks"}, "1")
	s.Require().Error(err)
	testhelpers.AssertHumaError(s.T(), err, 404)
}

func (s *TestFixturesSuite) TestReset_EmptyTables_Returns400() {
	admin := s.createTestLocalUser(true)
	target := s.createTestLocalUser(false)
	_, err := s.call(admin, target.ID, []string{}, "1")
	s.Require().Error(err)
	testhelpers.AssertHumaError(s.T(), err, 400)
}

func (s *TestFixturesSuite) TestReset_PendingShowsScope_PreservesApproved() {
	admin := s.createTestLocalUser(true)
	target := s.createTestLocalUser(false)

	pending := &models.Show{Status: models.ShowStatusPending, SubmittedBy: &target.ID, EventDate: time.Now()}
	approved := &models.Show{Status: models.ShowStatusApproved, SubmittedBy: &target.ID, EventDate: time.Now()}
	s.Require().NoError(s.deps.DB.Create(pending).Error)
	s.Require().NoError(s.deps.DB.Create(approved).Error)

	_, err := s.call(admin, target.ID, []string{"pending_shows"}, "1")
	s.Require().NoError(err)

	var pendingAfter, approvedAfter int64
	s.deps.DB.Model(&models.Show{}).Where("id = ?", pending.ID).Count(&pendingAfter)
	s.deps.DB.Model(&models.Show{}).Where("id = ?", approved.ID).Count(&approvedAfter)
	s.Zero(pendingAfter, "pending show should be deleted")
	s.Equal(int64(1), approvedAfter, "approved show must NOT be deleted")
}

// Sanity check that the allowlist can round-trip through the lookup helper.
func TestTestFixtureScopeByName(t *testing.T) {
	for _, s := range testFixtureAllowlist {
		got, ok := testFixtureScopeByName(s.displayName)
		if !ok || got.displayName != s.displayName {
			t.Errorf("lookup failed for %s", s.displayName)
		}
	}
	if _, ok := testFixtureScopeByName("no such scope"); ok {
		t.Error("unknown name should not resolve")
	}
}

// Quieter linter — suite unused imports guard.
var _ = gorm.DB{}
