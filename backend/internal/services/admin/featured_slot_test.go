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
	catalogm "psychic-homily-backend/internal/models/catalog"
	communitym "psychic-homily-backend/internal/models/community"
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
	// Referent-validation tests seed shows + collections; tear them
	// down so each case starts from a clean slate.
	_, _ = sqlDB.Exec("DELETE FROM collections")
	_, _ = sqlDB.Exec("DELETE FROM shows")
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

// createShow seeds a show with the requested status. Bare row — no
// venue/artist joins — because the validation predicate only reads
// the status column.
func (suite *FeaturedSlotServiceIntegrationTestSuite) createShow(status catalogm.ShowStatus) *catalogm.Show {
	show := &catalogm.Show{
		Title:     fmt.Sprintf("Show %d", time.Now().UnixNano()),
		EventDate: time.Now().UTC().AddDate(0, 0, 7),
		Status:    status,
	}
	suite.Require().NoError(suite.db.Create(show).Error)
	return show
}

// createCollection seeds a collection with the requested visibility.
// IsPublic is a GORM-bool gotcha (CLAUDE.md): false on Create is the
// zero-value so GORM skips it and the column default (true) wins.
// Insert as public, then Update to flip private when needed.
func (suite *FeaturedSlotServiceIntegrationTestSuite) createCollection(creatorID uint, isPublic bool) *communitym.Collection {
	coll := &communitym.Collection{
		Title:     fmt.Sprintf("Collection %d", time.Now().UnixNano()),
		Slug:      fmt.Sprintf("collection-%d", time.Now().UnixNano()),
		CreatorID: creatorID,
		IsPublic:  true,
	}
	suite.Require().NoError(suite.db.Create(coll).Error)
	if !isPublic {
		suite.Require().NoError(suite.db.Model(coll).Update("is_public", false).Error)
		coll.IsPublic = false
	}
	return coll
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
	show := suite.createShow(catalogm.ShowStatusApproved)
	note := "First pick"
	_, err := suite.featuredSlotService.SetActiveSlot(adminm.FeaturedSlotTypeBill, show.ID, &note, admin.ID)
	suite.Require().NoError(err)

	slot, err := suite.featuredSlotService.GetActiveSlot(adminm.FeaturedSlotTypeBill)
	suite.Require().NoError(err)
	suite.Equal(show.ID, slot.EntityID)
	suite.Equal(admin.ID, slot.Creator.ID)
	suite.Equal(*admin.Email, *slot.Creator.Email)
}

// =============================================================================
// SetActiveSlot — atomic retire + insert is the load-bearing behavior
// =============================================================================

func (suite *FeaturedSlotServiceIntegrationTestSuite) TestSetActiveSlot_FirstPick() {
	admin := suite.createAdmin("curator")
	show := suite.createShow(catalogm.ShowStatusApproved)

	note := "Killer bill"
	slot, err := suite.featuredSlotService.SetActiveSlot(adminm.FeaturedSlotTypeBill, show.ID, &note, admin.ID)
	suite.Require().NoError(err)
	suite.Require().NotNil(slot)
	suite.Equal(adminm.FeaturedSlotTypeBill, slot.SlotType)
	suite.Equal(show.ID, slot.EntityID)
	suite.Require().NotNil(slot.CuratorNote)
	suite.Equal("Killer bill", *slot.CuratorNote)
	suite.Nil(slot.ActiveUntil, "first active row must have NULL active_until")
	suite.Equal(admin.ID, slot.CreatedBy)
}

func (suite *FeaturedSlotServiceIntegrationTestSuite) TestSetActiveSlot_ReplacesPriorAtomically() {
	admin := suite.createAdmin("curator")
	show1 := suite.createShow(catalogm.ShowStatusApproved)
	show2 := suite.createShow(catalogm.ShowStatusApproved)

	first, err := suite.featuredSlotService.SetActiveSlot(adminm.FeaturedSlotTypeBill, show1.ID, nil, admin.ID)
	suite.Require().NoError(err)
	firstID := first.ID

	// Replace.
	second, err := suite.featuredSlotService.SetActiveSlot(adminm.FeaturedSlotTypeBill, show2.ID, nil, admin.ID)
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
	suite.Equal(show2.ID, active.EntityID)
}

func (suite *FeaturedSlotServiceIntegrationTestSuite) TestSetActiveSlot_SlotsIndependent() {
	admin := suite.createAdmin("curator")
	show := suite.createShow(catalogm.ShowStatusApproved)
	coll := suite.createCollection(admin.ID, true)

	bill, err := suite.featuredSlotService.SetActiveSlot(adminm.FeaturedSlotTypeBill, show.ID, nil, admin.ID)
	suite.Require().NoError(err)
	collection, err := suite.featuredSlotService.SetActiveSlot(adminm.FeaturedSlotTypeCollection, coll.ID, nil, admin.ID)
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
	// EntityID=1 here is intentionally non-existent — the over-length
	// guard fires before the referent lookup, so the test stays valid
	// without seeding a show.
	_, err := suite.featuredSlotService.SetActiveSlot(adminm.FeaturedSlotTypeBill, 1, &note, admin.ID)
	suite.Require().Error(err)
}

// =============================================================================
// Partial unique index — direct DB poke proves the constraint is real
// =============================================================================

func (suite *FeaturedSlotServiceIntegrationTestSuite) TestPartialUniqueIndex_RejectsTwoActiveOfSameType() {
	admin := suite.createAdmin("curator")
	show := suite.createShow(catalogm.ShowStatusApproved)

	// First active row via the service (one row, NULL active_until).
	_, err := suite.featuredSlotService.SetActiveSlot(adminm.FeaturedSlotTypeBill, show.ID, nil, admin.ID)
	suite.Require().NoError(err)

	// Direct INSERT bypassing the service to prove the partial unique
	// index is what's holding the invariant, not just the service's
	// transaction wrapper.
	raw := suite.db.Exec(
		"INSERT INTO featured_slots (slot_type, entity_id, created_by) VALUES (?, ?, ?)",
		adminm.FeaturedSlotTypeBill, 999, admin.ID,
	)
	suite.Require().Error(raw.Error, "second active row of same slot_type must fail")
}

func (suite *FeaturedSlotServiceIntegrationTestSuite) TestPartialUniqueIndex_AllowsManyRetiredOfSameType() {
	admin := suite.createAdmin("curator")

	// Three sets in sequence — at the end, one active + two retired.
	shows := []*catalogm.Show{
		suite.createShow(catalogm.ShowStatusApproved),
		suite.createShow(catalogm.ShowStatusApproved),
		suite.createShow(catalogm.ShowStatusApproved),
	}
	for _, show := range shows {
		_, err := suite.featuredSlotService.SetActiveSlot(adminm.FeaturedSlotTypeBill, show.ID, nil, admin.ID)
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
	show := suite.createShow(catalogm.ShowStatusApproved)
	_, err := suite.featuredSlotService.SetActiveSlot(adminm.FeaturedSlotTypeBill, show.ID, nil, admin.ID)
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
	show := suite.createShow(catalogm.ShowStatusApproved)
	first, err := suite.featuredSlotService.SetActiveSlot(adminm.FeaturedSlotTypeBill, show.ID, nil, admin.ID)
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
	// Seed three approved shows so each SetActiveSlot passes referent
	// validation; keep the order we feed them in so we can assert
	// most-recent-first ordering against the slice below.
	shows := []*catalogm.Show{
		suite.createShow(catalogm.ShowStatusApproved),
		suite.createShow(catalogm.ShowStatusApproved),
		suite.createShow(catalogm.ShowStatusApproved),
	}
	for _, show := range shows {
		_, err := suite.featuredSlotService.SetActiveSlot(adminm.FeaturedSlotTypeBill, show.ID, nil, admin.ID)
		suite.Require().NoError(err)
		time.Sleep(2 * time.Millisecond) // ensure distinct created_at
	}

	history, err := suite.featuredSlotService.ListRecent(adminm.FeaturedSlotTypeBill, 5)
	suite.Require().NoError(err)
	suite.Len(history, 3)
	// Most recent first — last show in the slice was set last.
	suite.Equal(shows[2].ID, history[0].EntityID)
	suite.Equal(shows[1].ID, history[1].EntityID)
	suite.Equal(shows[0].ID, history[2].EntityID)
}

func (suite *FeaturedSlotServiceIntegrationTestSuite) TestListRecent_RespectsLimit() {
	admin := suite.createAdmin("curator")
	for i := 0; i < 5; i++ {
		show := suite.createShow(catalogm.ShowStatusApproved)
		_, err := suite.featuredSlotService.SetActiveSlot(adminm.FeaturedSlotTypeBill, show.ID, nil, admin.ID)
		suite.Require().NoError(err)
	}
	history, err := suite.featuredSlotService.ListRecent(adminm.FeaturedSlotTypeBill, 2)
	suite.Require().NoError(err)
	suite.Len(history, 2)
}

func (suite *FeaturedSlotServiceIntegrationTestSuite) TestListRecent_NoCrossContamination() {
	admin := suite.createAdmin("curator")
	show := suite.createShow(catalogm.ShowStatusApproved)
	coll := suite.createCollection(admin.ID, true)
	_, err := suite.featuredSlotService.SetActiveSlot(adminm.FeaturedSlotTypeBill, show.ID, nil, admin.ID)
	suite.Require().NoError(err)
	_, err = suite.featuredSlotService.SetActiveSlot(adminm.FeaturedSlotTypeCollection, coll.ID, nil, admin.ID)
	suite.Require().NoError(err)

	bills, err := suite.featuredSlotService.ListRecent(adminm.FeaturedSlotTypeBill, 10)
	suite.Require().NoError(err)
	suite.Len(bills, 1)
	suite.Equal(show.ID, bills[0].EntityID)
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

// =============================================================================
// SetActiveSlot — referent validation (PSY-850)
// =============================================================================
//
// Each rejection path returns a distinct sentinel error so the handler
// can map to a specific 4xx + copy. The predicates mirror
// services/explore/explore.go (approved status for shows, is_public for
// collections) — those are the only reason this validation exists. If
// the consumer predicate changes, change validateReferent in the same
// PR.

func (suite *FeaturedSlotServiceIntegrationTestSuite) TestSetActiveSlot_BillRejectsPendingShow() {
	admin := suite.createAdmin("curator")
	pending := suite.createShow(catalogm.ShowStatusPending)

	_, err := suite.featuredSlotService.SetActiveSlot(adminm.FeaturedSlotTypeBill, pending.ID, nil, admin.ID)
	suite.Require().Error(err)
	suite.True(errors.Is(err, ErrFeaturedSlotReferentNotApproved),
		"pending show must surface the not-approved sentinel, got %v", err)

	// No row was created — the validation fires before the transaction.
	var count int64
	suite.db.Model(&adminm.FeaturedSlot{}).Count(&count)
	suite.Equal(int64(0), count)
}

func (suite *FeaturedSlotServiceIntegrationTestSuite) TestSetActiveSlot_BillRejectsRejectedShow() {
	admin := suite.createAdmin("curator")
	rejected := suite.createShow(catalogm.ShowStatusRejected)

	_, err := suite.featuredSlotService.SetActiveSlot(adminm.FeaturedSlotTypeBill, rejected.ID, nil, admin.ID)
	suite.Require().Error(err)
	suite.True(errors.Is(err, ErrFeaturedSlotReferentNotApproved))
}

func (suite *FeaturedSlotServiceIntegrationTestSuite) TestSetActiveSlot_BillRejectsPrivateShow() {
	admin := suite.createAdmin("curator")
	private := suite.createShow(catalogm.ShowStatusPrivate)

	_, err := suite.featuredSlotService.SetActiveSlot(adminm.FeaturedSlotTypeBill, private.ID, nil, admin.ID)
	suite.Require().Error(err)
	suite.True(errors.Is(err, ErrFeaturedSlotReferentNotApproved))
}

func (suite *FeaturedSlotServiceIntegrationTestSuite) TestSetActiveSlot_BillRejectsMissingShow() {
	admin := suite.createAdmin("curator")

	_, err := suite.featuredSlotService.SetActiveSlot(adminm.FeaturedSlotTypeBill, 9999, nil, admin.ID)
	suite.Require().Error(err)
	suite.True(errors.Is(err, ErrFeaturedSlotReferentNotFound),
		"non-existent show must surface the not-found sentinel, got %v", err)
}

func (suite *FeaturedSlotServiceIntegrationTestSuite) TestSetActiveSlot_BillAcceptsApprovedShow() {
	admin := suite.createAdmin("curator")
	approved := suite.createShow(catalogm.ShowStatusApproved)

	slot, err := suite.featuredSlotService.SetActiveSlot(adminm.FeaturedSlotTypeBill, approved.ID, nil, admin.ID)
	suite.Require().NoError(err)
	suite.Equal(approved.ID, slot.EntityID)
}

func (suite *FeaturedSlotServiceIntegrationTestSuite) TestSetActiveSlot_CollectionRejectsPrivate() {
	admin := suite.createAdmin("curator")
	private := suite.createCollection(admin.ID, false)

	_, err := suite.featuredSlotService.SetActiveSlot(adminm.FeaturedSlotTypeCollection, private.ID, nil, admin.ID)
	suite.Require().Error(err)
	suite.True(errors.Is(err, ErrFeaturedSlotReferentNotPublic),
		"private collection must surface the not-public sentinel, got %v", err)

	var count int64
	suite.db.Model(&adminm.FeaturedSlot{}).Count(&count)
	suite.Equal(int64(0), count)
}

func (suite *FeaturedSlotServiceIntegrationTestSuite) TestSetActiveSlot_CollectionRejectsMissing() {
	admin := suite.createAdmin("curator")

	_, err := suite.featuredSlotService.SetActiveSlot(adminm.FeaturedSlotTypeCollection, 9999, nil, admin.ID)
	suite.Require().Error(err)
	suite.True(errors.Is(err, ErrFeaturedSlotReferentNotFound))
}

func (suite *FeaturedSlotServiceIntegrationTestSuite) TestSetActiveSlot_CollectionAcceptsPublic() {
	admin := suite.createAdmin("curator")
	public := suite.createCollection(admin.ID, true)

	slot, err := suite.featuredSlotService.SetActiveSlot(adminm.FeaturedSlotTypeCollection, public.ID, nil, admin.ID)
	suite.Require().NoError(err)
	suite.Equal(public.ID, slot.EntityID)
}

// Replacement that was previously valid should NOT be allowed to no-op
// the active slot when the new referent fails validation — the prior
// row stays active. Guards against partial state if validation fired
// after the retire-then-insert transaction had already started.
func (suite *FeaturedSlotServiceIntegrationTestSuite) TestSetActiveSlot_ReplacementFailsCleanly() {
	admin := suite.createAdmin("curator")
	approved := suite.createShow(catalogm.ShowStatusApproved)
	pending := suite.createShow(catalogm.ShowStatusPending)

	// Establish a valid active row first.
	original, err := suite.featuredSlotService.SetActiveSlot(adminm.FeaturedSlotTypeBill, approved.ID, nil, admin.ID)
	suite.Require().NoError(err)

	// Attempt to replace with a pending show — must fail without
	// retiring the original.
	_, err = suite.featuredSlotService.SetActiveSlot(adminm.FeaturedSlotTypeBill, pending.ID, nil, admin.ID)
	suite.Require().Error(err)
	suite.True(errors.Is(err, ErrFeaturedSlotReferentNotApproved))

	// Original row is still active.
	active, err := suite.featuredSlotService.GetActiveSlot(adminm.FeaturedSlotTypeBill)
	suite.Require().NoError(err)
	suite.Equal(original.ID, active.ID, "failed replacement must leave the prior active row untouched")
	suite.Nil(active.ActiveUntil)
}
