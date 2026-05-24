package admin

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	"gorm.io/gorm"

	adminm "psychic-homily-backend/internal/models/admin"
	authm "psychic-homily-backend/internal/models/auth"
	"psychic-homily-backend/internal/testutil"
)

// =============================================================================
// INTEGRATION TESTS (Real Postgres via testcontainers)
// =============================================================================

type FeaturedSlotServiceIntegrationTestSuite struct {
	suite.Suite
	testDB              *testutil.TestDatabase
	db                  *gorm.DB
	featuredSlotService *FeaturedSlotService
}

func (suite *FeaturedSlotServiceIntegrationTestSuite) SetupSuite() {
	suite.testDB = testutil.SetupTestPostgres(suite.T())
	suite.db = suite.testDB.DB
	suite.featuredSlotService = NewFeaturedSlotService(suite.testDB.DB)
}

func (suite *FeaturedSlotServiceIntegrationTestSuite) TearDownSuite() {
	suite.testDB.Cleanup()
}

func (suite *FeaturedSlotServiceIntegrationTestSuite) TearDownTest() {
	sqlDB, err := suite.db.DB()
	suite.Require().NoError(err)
	_, _ = sqlDB.Exec("DELETE FROM featured_slots")
	_, _ = sqlDB.Exec("DELETE FROM users")
}

func TestFeaturedSlotServiceIntegrationTestSuite(t *testing.T) {
	suite.Run(t, new(FeaturedSlotServiceIntegrationTestSuite))
}

// =============================================================================
// HELPERS
// =============================================================================

func (suite *FeaturedSlotServiceIntegrationTestSuite) createAdmin(name string) *authm.User {
	email := fmt.Sprintf("%s-%d@test.com", name, time.Now().UnixNano())
	user := &authm.User{
		Email:         &email,
		FirstName:     &name,
		IsActive:      true,
		IsAdmin:       true,
		EmailVerified: true,
	}
	err := suite.db.Create(user).Error
	suite.Require().NoError(err)
	return user
}

// =============================================================================
// GetActiveSlot
// =============================================================================

func (suite *FeaturedSlotServiceIntegrationTestSuite) TestGetActiveSlot_EmptyReturnsNotFound() {
	_, err := suite.featuredSlotService.GetActiveSlot(adminm.FeaturedSlotTypeBill)
	suite.Require().Error(err)
	suite.True(errors.Is(err, ErrFeaturedSlotNotFound))
}

func (suite *FeaturedSlotServiceIntegrationTestSuite) TestGetActiveSlot_InvalidSlotTypeRejected() {
	_, err := suite.featuredSlotService.GetActiveSlot("not-a-slot")
	suite.Require().Error(err)
	suite.False(errors.Is(err, ErrFeaturedSlotNotFound))
}

func (suite *FeaturedSlotServiceIntegrationTestSuite) TestGetActiveSlot_PreloadsCreator() {
	admin := suite.createAdmin("curator")
	note := "First pick"
	_, err := suite.featuredSlotService.SetActiveSlot(adminm.FeaturedSlotTypeBill, 42, &note, admin.ID)
	suite.Require().NoError(err)

	slot, err := suite.featuredSlotService.GetActiveSlot(adminm.FeaturedSlotTypeBill)
	suite.Require().NoError(err)
	suite.Equal(uint(42), slot.EntityID)
	suite.Equal(admin.ID, slot.Creator.ID)
	suite.Equal(*admin.Email, *slot.Creator.Email)
}

// =============================================================================
// SetActiveSlot — atomic retire + insert is the load-bearing behavior
// =============================================================================

func (suite *FeaturedSlotServiceIntegrationTestSuite) TestSetActiveSlot_FirstPick() {
	admin := suite.createAdmin("curator")

	note := "Killer bill"
	slot, err := suite.featuredSlotService.SetActiveSlot(adminm.FeaturedSlotTypeBill, 7, &note, admin.ID)
	suite.Require().NoError(err)
	suite.Require().NotNil(slot)
	suite.Equal(adminm.FeaturedSlotTypeBill, slot.SlotType)
	suite.Equal(uint(7), slot.EntityID)
	suite.Require().NotNil(slot.CuratorNote)
	suite.Equal("Killer bill", *slot.CuratorNote)
	suite.Nil(slot.ActiveUntil, "first active row must have NULL active_until")
	suite.Equal(admin.ID, slot.CreatedBy)
}

func (suite *FeaturedSlotServiceIntegrationTestSuite) TestSetActiveSlot_ReplacesPriorAtomically() {
	admin := suite.createAdmin("curator")

	first, err := suite.featuredSlotService.SetActiveSlot(adminm.FeaturedSlotTypeBill, 1, nil, admin.ID)
	suite.Require().NoError(err)
	firstID := first.ID

	// Replace.
	second, err := suite.featuredSlotService.SetActiveSlot(adminm.FeaturedSlotTypeBill, 2, nil, admin.ID)
	suite.Require().NoError(err)
	suite.NotEqual(firstID, second.ID, "replacement must insert a new row")

	// Verify exactly one active row of this slot_type exists.
	var activeCount int64
	suite.db.Model(&adminm.FeaturedSlot{}).
		Where("slot_type = ? AND active_until IS NULL", adminm.FeaturedSlotTypeBill).
		Count(&activeCount)
	suite.Equal(int64(1), activeCount, "exactly one active row per slot_type")

	// Verify the first row is now retired.
	var prior adminm.FeaturedSlot
	suite.Require().NoError(suite.db.First(&prior, firstID).Error)
	suite.Require().NotNil(prior.ActiveUntil)
	suite.True(prior.ActiveUntil.Before(time.Now().Add(time.Second)) || prior.ActiveUntil.Equal(time.Now()),
		"prior active_until must be set to roughly NOW()")

	// GetActiveSlot returns the new row.
	active, err := suite.featuredSlotService.GetActiveSlot(adminm.FeaturedSlotTypeBill)
	suite.Require().NoError(err)
	suite.Equal(second.ID, active.ID)
	suite.Equal(uint(2), active.EntityID)
}

func (suite *FeaturedSlotServiceIntegrationTestSuite) TestSetActiveSlot_SlotsIndependent() {
	admin := suite.createAdmin("curator")

	bill, err := suite.featuredSlotService.SetActiveSlot(adminm.FeaturedSlotTypeBill, 1, nil, admin.ID)
	suite.Require().NoError(err)
	collection, err := suite.featuredSlotService.SetActiveSlot(adminm.FeaturedSlotTypeCollection, 99, nil, admin.ID)
	suite.Require().NoError(err)
	suite.NotEqual(bill.ID, collection.ID)

	// Both slot types have one active row each.
	var billCount, collectionCount int64
	suite.db.Model(&adminm.FeaturedSlot{}).
		Where("slot_type = ? AND active_until IS NULL", adminm.FeaturedSlotTypeBill).
		Count(&billCount)
	suite.db.Model(&adminm.FeaturedSlot{}).
		Where("slot_type = ? AND active_until IS NULL", adminm.FeaturedSlotTypeCollection).
		Count(&collectionCount)
	suite.Equal(int64(1), billCount)
	suite.Equal(int64(1), collectionCount)
}

func (suite *FeaturedSlotServiceIntegrationTestSuite) TestSetActiveSlot_InvalidSlotType() {
	admin := suite.createAdmin("curator")
	_, err := suite.featuredSlotService.SetActiveSlot("scene", 1, nil, admin.ID)
	suite.Require().Error(err)
}

func (suite *FeaturedSlotServiceIntegrationTestSuite) TestSetActiveSlot_ZeroEntityIDRejected() {
	admin := suite.createAdmin("curator")
	_, err := suite.featuredSlotService.SetActiveSlot(adminm.FeaturedSlotTypeBill, 0, nil, admin.ID)
	suite.Require().Error(err)
}

func (suite *FeaturedSlotServiceIntegrationTestSuite) TestSetActiveSlot_ZeroUserIDRejected() {
	_, err := suite.featuredSlotService.SetActiveSlot(adminm.FeaturedSlotTypeBill, 1, nil, 0)
	suite.Require().Error(err)
}

func (suite *FeaturedSlotServiceIntegrationTestSuite) TestSetActiveSlot_OverLengthNoteRejected() {
	admin := suite.createAdmin("curator")
	huge := make([]byte, adminm.MaxFeaturedSlotCuratorNoteLength+1)
	for i := range huge {
		huge[i] = 'x'
	}
	note := string(huge)
	_, err := suite.featuredSlotService.SetActiveSlot(adminm.FeaturedSlotTypeBill, 1, &note, admin.ID)
	suite.Require().Error(err)
}

// =============================================================================
// Partial unique index — direct DB poke proves the constraint is real
// =============================================================================

func (suite *FeaturedSlotServiceIntegrationTestSuite) TestPartialUniqueIndex_RejectsTwoActiveOfSameType() {
	admin := suite.createAdmin("curator")

	// First active row via the service (one row, NULL active_until).
	_, err := suite.featuredSlotService.SetActiveSlot(adminm.FeaturedSlotTypeBill, 1, nil, admin.ID)
	suite.Require().NoError(err)

	// Direct INSERT bypassing the service to prove the partial unique
	// index is what's holding the invariant, not just the service's
	// transaction wrapper.
	raw := suite.db.Exec(
		"INSERT INTO featured_slots (slot_type, entity_id, created_by) VALUES (?, ?, ?)",
		adminm.FeaturedSlotTypeBill, 2, admin.ID,
	)
	suite.Require().Error(raw.Error, "second active row of same slot_type must fail")
}

func (suite *FeaturedSlotServiceIntegrationTestSuite) TestPartialUniqueIndex_AllowsManyRetiredOfSameType() {
	admin := suite.createAdmin("curator")

	// Three sets in sequence — at the end, one active + two retired.
	for i := uint(1); i <= 3; i++ {
		_, err := suite.featuredSlotService.SetActiveSlot(adminm.FeaturedSlotTypeBill, i, nil, admin.ID)
		suite.Require().NoError(err)
	}

	var total, active int64
	suite.db.Model(&adminm.FeaturedSlot{}).
		Where("slot_type = ?", adminm.FeaturedSlotTypeBill).
		Count(&total)
	suite.db.Model(&adminm.FeaturedSlot{}).
		Where("slot_type = ? AND active_until IS NULL", adminm.FeaturedSlotTypeBill).
		Count(&active)
	suite.Equal(int64(3), total)
	suite.Equal(int64(1), active)
}

// =============================================================================
// RetireActiveSlot
// =============================================================================

func (suite *FeaturedSlotServiceIntegrationTestSuite) TestRetireActiveSlot_Success() {
	admin := suite.createAdmin("curator")
	_, err := suite.featuredSlotService.SetActiveSlot(adminm.FeaturedSlotTypeBill, 1, nil, admin.ID)
	suite.Require().NoError(err)

	suite.Require().NoError(suite.featuredSlotService.RetireActiveSlot(adminm.FeaturedSlotTypeBill, admin.ID))

	_, err = suite.featuredSlotService.GetActiveSlot(adminm.FeaturedSlotTypeBill)
	suite.True(errors.Is(err, ErrFeaturedSlotNotFound))
}

func (suite *FeaturedSlotServiceIntegrationTestSuite) TestRetireActiveSlot_NoActiveReturnsNotFound() {
	admin := suite.createAdmin("curator")
	err := suite.featuredSlotService.RetireActiveSlot(adminm.FeaturedSlotTypeBill, admin.ID)
	suite.True(errors.Is(err, ErrFeaturedSlotNotFound))
}

func (suite *FeaturedSlotServiceIntegrationTestSuite) TestRetireActiveSlot_LeavesHistoryIntact() {
	admin := suite.createAdmin("curator")
	first, err := suite.featuredSlotService.SetActiveSlot(adminm.FeaturedSlotTypeBill, 1, nil, admin.ID)
	suite.Require().NoError(err)
	suite.Require().NoError(suite.featuredSlotService.RetireActiveSlot(adminm.FeaturedSlotTypeBill, admin.ID))

	// History row still exists, with active_until set.
	var prior adminm.FeaturedSlot
	suite.Require().NoError(suite.db.First(&prior, first.ID).Error)
	suite.NotNil(prior.ActiveUntil)
}

// =============================================================================
// ListRecent
// =============================================================================

func (suite *FeaturedSlotServiceIntegrationTestSuite) TestListRecent_OrdersDescByCreatedAt() {
	admin := suite.createAdmin("curator")

	for i := uint(1); i <= 3; i++ {
		_, err := suite.featuredSlotService.SetActiveSlot(adminm.FeaturedSlotTypeBill, i, nil, admin.ID)
		suite.Require().NoError(err)
		time.Sleep(2 * time.Millisecond) // ensure distinct created_at
	}

	history, err := suite.featuredSlotService.ListRecent(adminm.FeaturedSlotTypeBill, 5)
	suite.Require().NoError(err)
	suite.Len(history, 3)
	// Most recent first — entity_id=3 was set last.
	suite.Equal(uint(3), history[0].EntityID)
	suite.Equal(uint(2), history[1].EntityID)
	suite.Equal(uint(1), history[2].EntityID)
}

func (suite *FeaturedSlotServiceIntegrationTestSuite) TestListRecent_RespectsLimit() {
	admin := suite.createAdmin("curator")
	for i := uint(1); i <= 5; i++ {
		_, err := suite.featuredSlotService.SetActiveSlot(adminm.FeaturedSlotTypeBill, i, nil, admin.ID)
		suite.Require().NoError(err)
	}
	history, err := suite.featuredSlotService.ListRecent(adminm.FeaturedSlotTypeBill, 2)
	suite.Require().NoError(err)
	suite.Len(history, 2)
}

func (suite *FeaturedSlotServiceIntegrationTestSuite) TestListRecent_NoCrossContamination() {
	admin := suite.createAdmin("curator")
	_, err := suite.featuredSlotService.SetActiveSlot(adminm.FeaturedSlotTypeBill, 1, nil, admin.ID)
	suite.Require().NoError(err)
	_, err = suite.featuredSlotService.SetActiveSlot(adminm.FeaturedSlotTypeCollection, 99, nil, admin.ID)
	suite.Require().NoError(err)

	bills, err := suite.featuredSlotService.ListRecent(adminm.FeaturedSlotTypeBill, 10)
	suite.Require().NoError(err)
	suite.Len(bills, 1)
	suite.Equal(uint(1), bills[0].EntityID)
}

// =============================================================================
// RenderCuratorNote
// =============================================================================

func (suite *FeaturedSlotServiceIntegrationTestSuite) TestRenderCuratorNote_EmptyAndNil() {
	suite.Equal("", suite.featuredSlotService.RenderCuratorNote(nil))
	empty := ""
	suite.Equal("", suite.featuredSlotService.RenderCuratorNote(&empty))
}

func (suite *FeaturedSlotServiceIntegrationTestSuite) TestRenderCuratorNote_MarkdownToSanitizedHTML() {
	note := "**bold** + *em*"
	html := suite.featuredSlotService.RenderCuratorNote(&note)
	suite.Contains(html, "<strong>bold</strong>")
	suite.Contains(html, "<em>em</em>")
}

func (suite *FeaturedSlotServiceIntegrationTestSuite) TestRenderCuratorNote_StripsScript() {
	note := "good text <script>alert(1)</script>"
	html := suite.featuredSlotService.RenderCuratorNote(&note)
	suite.NotContains(html, "<script>")
}
