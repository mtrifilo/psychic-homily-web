package admin

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"

	"psychic-homily-backend/internal/api/handlers/shared/testhelpers"
	adminm "psychic-homily-backend/internal/models/admin"
	catalogm "psychic-homily-backend/internal/models/catalog"
	communitym "psychic-homily-backend/internal/models/community"
)

// FeaturedSlotHandlerIntegrationSuite exercises the admin featured-slot
// endpoints end-to-end against a real Postgres test container. The
// service layer is unit-tested separately
// (services/admin/featured_slot_test.go); this suite focuses on the HTTP
// boundary — validation, audit-log wiring, and atomic POST replace +
// DELETE retire behaviour from the handler's perspective.
type FeaturedSlotHandlerIntegrationSuite struct {
	suite.Suite
	deps                *testhelpers.IntegrationDeps
	featuredSlotHandler *FeaturedSlotHandler
}

func (s *FeaturedSlotHandlerIntegrationSuite) SetupSuite() {
	s.deps = testhelpers.SetupIntegrationDeps(s.T())
	s.featuredSlotHandler = NewFeaturedSlotHandler(
		s.deps.FeaturedSlotService,
		s.deps.AuditLogService,
	)
}

func (s *FeaturedSlotHandlerIntegrationSuite) TearDownTest() {
	testhelpers.CleanupTables(s.deps.DB)
}

func (s *FeaturedSlotHandlerIntegrationSuite) TearDownSuite() {
	s.deps.TestDB.Cleanup()
}

func TestFeaturedSlotHandlerIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}
	suite.Run(t, new(FeaturedSlotHandlerIntegrationSuite))
}

// =============================================================================
// HELPERS
// =============================================================================

// createCollection seeds a collection with the requested visibility.
// IsPublic is a GORM-bool gotcha (CLAUDE.md): false on Create is the
// zero-value, so GORM skips it and the column default (true) wins.
// Insert as public, then Update to flip private when needed.
func (s *FeaturedSlotHandlerIntegrationSuite) createCollection(creatorID uint, isPublic bool) *communitym.Collection {
	coll := &communitym.Collection{
		Title:     "Test Collection",
		Slug:      fmt.Sprintf("collection-%d", time.Now().UnixNano()),
		CreatorID: creatorID,
		IsPublic:  true,
	}
	s.Require().NoError(s.deps.DB.Create(coll).Error)
	if !isPublic {
		s.Require().NoError(s.deps.DB.Model(coll).Update("is_public", false).Error)
		coll.IsPublic = false
	}
	return coll
}

// =============================================================================
// List
// =============================================================================

func (s *FeaturedSlotHandlerIntegrationSuite) TestList_EmptyReturnsBothSlots() {
	admin := testhelpers.CreateAdminUser(s.deps.DB)
	ctx := testhelpers.CtxWithUser(admin)

	resp, err := s.featuredSlotHandler.ListFeaturedSlotsHandler(ctx, &ListFeaturedSlotsRequest{HistoryLimit: 5})
	s.Require().NoError(err)
	s.Require().Len(resp.Body.Slots, 2, "always returns both slot_type entries")
	s.Equal(adminm.FeaturedSlotTypeBill, resp.Body.Slots[0].SlotType)
	s.Nil(resp.Body.Slots[0].Active)
	s.Empty(resp.Body.Slots[0].History)
	s.Equal(adminm.FeaturedSlotTypeCollection, resp.Body.Slots[1].SlotType)
	s.Nil(resp.Body.Slots[1].Active)
	s.Empty(resp.Body.Slots[1].History)
}

func (s *FeaturedSlotHandlerIntegrationSuite) TestList_AfterSetIncludesActiveAndHistory() {
	admin := testhelpers.CreateAdminUser(s.deps.DB)
	ctx := testhelpers.CtxWithUser(admin)
	show1 := testhelpers.CreateApprovedShow(s.deps.DB, admin.ID, "Show 1")
	show2 := testhelpers.CreateApprovedShow(s.deps.DB, admin.ID, "Show 2")

	// Two bill picks → most-recent active, one in history.
	note1 := "first"
	first := &SetFeaturedSlotRequest{}
	first.Body.SlotType = adminm.FeaturedSlotTypeBill
	first.Body.EntityID = show1.ID
	first.Body.CuratorNote = &note1
	_, err := s.featuredSlotHandler.SetFeaturedSlotHandler(ctx, first)
	s.Require().NoError(err)

	note2 := "second"
	second := &SetFeaturedSlotRequest{}
	second.Body.SlotType = adminm.FeaturedSlotTypeBill
	second.Body.EntityID = show2.ID
	second.Body.CuratorNote = &note2
	_, err = s.featuredSlotHandler.SetFeaturedSlotHandler(ctx, second)
	s.Require().NoError(err)

	resp, err := s.featuredSlotHandler.ListFeaturedSlotsHandler(ctx, &ListFeaturedSlotsRequest{HistoryLimit: 5})
	s.Require().NoError(err)
	s.Require().Len(resp.Body.Slots, 2)

	billSlot := resp.Body.Slots[0]
	s.Require().NotNil(billSlot.Active)
	s.Equal(show2.ID, billSlot.Active.EntityID)
	s.Len(billSlot.History, 2)
}

// =============================================================================
// Set
// =============================================================================

func (s *FeaturedSlotHandlerIntegrationSuite) TestSet_Success_FirstPick() {
	admin := testhelpers.CreateAdminUser(s.deps.DB)
	ctx := testhelpers.CtxWithUser(admin)
	show := testhelpers.CreateApprovedShow(s.deps.DB, admin.ID, "Killer Show")

	note := "**killer** bill"
	req := &SetFeaturedSlotRequest{}
	req.Body.SlotType = adminm.FeaturedSlotTypeBill
	req.Body.EntityID = show.ID
	req.Body.CuratorNote = &note

	resp, err := s.featuredSlotHandler.SetFeaturedSlotHandler(ctx, req)
	s.Require().NoError(err)
	s.Equal(adminm.FeaturedSlotTypeBill, resp.Body.SlotType)
	s.Equal(show.ID, resp.Body.EntityID)
	s.Require().NotNil(resp.Body.CuratorNote)
	s.Equal("**killer** bill", *resp.Body.CuratorNote)
	s.Contains(resp.Body.CuratorNoteHTML, "<strong>killer</strong>")
	s.Nil(resp.Body.ActiveUntil)
	s.Equal(admin.ID, resp.Body.CreatedBy)
}

func (s *FeaturedSlotHandlerIntegrationSuite) TestSet_AtomicallyReplacesPrior() {
	admin := testhelpers.CreateAdminUser(s.deps.DB)
	ctx := testhelpers.CtxWithUser(admin)
	show1 := testhelpers.CreateApprovedShow(s.deps.DB, admin.ID, "Show A")
	show2 := testhelpers.CreateApprovedShow(s.deps.DB, admin.ID, "Show B")

	first := &SetFeaturedSlotRequest{}
	first.Body.SlotType = adminm.FeaturedSlotTypeBill
	first.Body.EntityID = show1.ID
	firstResp, err := s.featuredSlotHandler.SetFeaturedSlotHandler(ctx, first)
	s.Require().NoError(err)
	firstID := firstResp.Body.ID

	second := &SetFeaturedSlotRequest{}
	second.Body.SlotType = adminm.FeaturedSlotTypeBill
	second.Body.EntityID = show2.ID
	secondResp, err := s.featuredSlotHandler.SetFeaturedSlotHandler(ctx, second)
	s.Require().NoError(err)
	s.NotEqual(firstID, secondResp.Body.ID)

	// List shows new active + old in history.
	listResp, err := s.featuredSlotHandler.ListFeaturedSlotsHandler(ctx, &ListFeaturedSlotsRequest{HistoryLimit: 5})
	s.Require().NoError(err)
	billSlot := listResp.Body.Slots[0]
	s.Require().NotNil(billSlot.Active)
	s.Equal(show2.ID, billSlot.Active.EntityID)
	s.Len(billSlot.History, 2)

	// Look up prior row in history — active_until must be set.
	var priorEntry *FeaturedSlotResponse
	for i := range billSlot.History {
		if billSlot.History[i].ID == firstID {
			priorEntry = &billSlot.History[i]
			break
		}
	}
	s.Require().NotNil(priorEntry)
	s.Require().NotNil(priorEntry.ActiveUntil)
}

func (s *FeaturedSlotHandlerIntegrationSuite) TestSet_InvalidSlotTypeRejected() {
	admin := testhelpers.CreateAdminUser(s.deps.DB)
	ctx := testhelpers.CtxWithUser(admin)

	req := &SetFeaturedSlotRequest{}
	req.Body.SlotType = "scene"
	req.Body.EntityID = 1
	_, err := s.featuredSlotHandler.SetFeaturedSlotHandler(ctx, req)
	testhelpers.AssertHumaError(s.T(), err, 400)
}

func (s *FeaturedSlotHandlerIntegrationSuite) TestSet_ZeroEntityIDRejected() {
	admin := testhelpers.CreateAdminUser(s.deps.DB)
	ctx := testhelpers.CtxWithUser(admin)

	req := &SetFeaturedSlotRequest{}
	req.Body.SlotType = adminm.FeaturedSlotTypeBill
	req.Body.EntityID = 0
	_, err := s.featuredSlotHandler.SetFeaturedSlotHandler(ctx, req)
	testhelpers.AssertHumaError(s.T(), err, 400)
}

func (s *FeaturedSlotHandlerIntegrationSuite) TestSet_OverlengthCuratorNoteRejected() {
	admin := testhelpers.CreateAdminUser(s.deps.DB)
	ctx := testhelpers.CtxWithUser(admin)

	huge := make([]byte, adminm.MaxFeaturedSlotCuratorNoteLength+1)
	for i := range huge {
		huge[i] = 'x'
	}
	note := string(huge)
	req := &SetFeaturedSlotRequest{}
	req.Body.SlotType = adminm.FeaturedSlotTypeBill
	// EntityID=1 here is intentionally non-existent — the over-length
	// guard fires before the referent lookup, so the test stays valid
	// without seeding a show.
	req.Body.EntityID = 1
	req.Body.CuratorNote = &note
	_, err := s.featuredSlotHandler.SetFeaturedSlotHandler(ctx, req)
	testhelpers.AssertHumaError(s.T(), err, 400)
}

// =============================================================================
// Set — referent validation (PSY-850)
// =============================================================================
//
// Service-level rejection sentinels translate to specific HTTP status
// codes + copy at the handler boundary. These tests assert the error
// surface (status + Detail string) — the validation logic itself lives
// in services/admin/featured_slot_test.go.

func (s *FeaturedSlotHandlerIntegrationSuite) TestSet_BillRejectsPendingShow() {
	admin := testhelpers.CreateAdminUser(s.deps.DB)
	ctx := testhelpers.CtxWithUser(admin)
	pending := testhelpers.CreatePendingShow(s.deps.DB, admin.ID, "Pending Show")

	req := &SetFeaturedSlotRequest{}
	req.Body.SlotType = adminm.FeaturedSlotTypeBill
	req.Body.EntityID = pending.ID
	_, err := s.featuredSlotHandler.SetFeaturedSlotHandler(ctx, req)
	testhelpers.AssertHumaErrorWithDetail(s.T(), err, 400,
		"show is not approved; only approved shows can be featured")
}

func (s *FeaturedSlotHandlerIntegrationSuite) TestSet_BillRejectsRejectedShow() {
	admin := testhelpers.CreateAdminUser(s.deps.DB)
	ctx := testhelpers.CtxWithUser(admin)
	// CreatePendingShow + manual status flip — there's no Rejected helper.
	rejected := testhelpers.CreatePendingShow(s.deps.DB, admin.ID, "Rejected Show")
	s.Require().NoError(s.deps.DB.Model(rejected).Update("status", catalogm.ShowStatusRejected).Error)

	req := &SetFeaturedSlotRequest{}
	req.Body.SlotType = adminm.FeaturedSlotTypeBill
	req.Body.EntityID = rejected.ID
	_, err := s.featuredSlotHandler.SetFeaturedSlotHandler(ctx, req)
	testhelpers.AssertHumaErrorWithDetail(s.T(), err, 400,
		"show is not approved; only approved shows can be featured")
}

func (s *FeaturedSlotHandlerIntegrationSuite) TestSet_BillRejectsMissingShow() {
	admin := testhelpers.CreateAdminUser(s.deps.DB)
	ctx := testhelpers.CtxWithUser(admin)

	req := &SetFeaturedSlotRequest{}
	req.Body.SlotType = adminm.FeaturedSlotTypeBill
	req.Body.EntityID = 9999
	_, err := s.featuredSlotHandler.SetFeaturedSlotHandler(ctx, req)
	testhelpers.AssertHumaErrorWithDetail(s.T(), err, 404, "show not found")
}

func (s *FeaturedSlotHandlerIntegrationSuite) TestSet_CollectionRejectsPrivate() {
	admin := testhelpers.CreateAdminUser(s.deps.DB)
	ctx := testhelpers.CtxWithUser(admin)
	private := s.createCollection(admin.ID, false)

	req := &SetFeaturedSlotRequest{}
	req.Body.SlotType = adminm.FeaturedSlotTypeCollection
	req.Body.EntityID = private.ID
	_, err := s.featuredSlotHandler.SetFeaturedSlotHandler(ctx, req)
	testhelpers.AssertHumaErrorWithDetail(s.T(), err, 400,
		"collection is private; only public collections can be featured")
}

func (s *FeaturedSlotHandlerIntegrationSuite) TestSet_CollectionRejectsMissing() {
	admin := testhelpers.CreateAdminUser(s.deps.DB)
	ctx := testhelpers.CtxWithUser(admin)

	req := &SetFeaturedSlotRequest{}
	req.Body.SlotType = adminm.FeaturedSlotTypeCollection
	req.Body.EntityID = 9999
	_, err := s.featuredSlotHandler.SetFeaturedSlotHandler(ctx, req)
	testhelpers.AssertHumaErrorWithDetail(s.T(), err, 404, "collection not found")
}

func (s *FeaturedSlotHandlerIntegrationSuite) TestSet_CollectionAcceptsPublic() {
	admin := testhelpers.CreateAdminUser(s.deps.DB)
	ctx := testhelpers.CtxWithUser(admin)
	public := s.createCollection(admin.ID, true)

	req := &SetFeaturedSlotRequest{}
	req.Body.SlotType = adminm.FeaturedSlotTypeCollection
	req.Body.EntityID = public.ID
	resp, err := s.featuredSlotHandler.SetFeaturedSlotHandler(ctx, req)
	s.Require().NoError(err)
	s.Equal(public.ID, resp.Body.EntityID)
}

// =============================================================================
// Delete
// =============================================================================

func (s *FeaturedSlotHandlerIntegrationSuite) TestDelete_RetiresActive() {
	admin := testhelpers.CreateAdminUser(s.deps.DB)
	ctx := testhelpers.CtxWithUser(admin)
	show := testhelpers.CreateApprovedShow(s.deps.DB, admin.ID, "Show To Retire")

	setReq := &SetFeaturedSlotRequest{}
	setReq.Body.SlotType = adminm.FeaturedSlotTypeBill
	setReq.Body.EntityID = show.ID
	_, err := s.featuredSlotHandler.SetFeaturedSlotHandler(ctx, setReq)
	s.Require().NoError(err)

	delResp, err := s.featuredSlotHandler.DeleteFeaturedSlotHandler(ctx, &DeleteFeaturedSlotRequest{SlotType: adminm.FeaturedSlotTypeBill})
	s.Require().NoError(err)
	s.Equal(adminm.FeaturedSlotTypeBill, delResp.Body.SlotType)

	// List shows no active row + one historical row with active_until set.
	listResp, err := s.featuredSlotHandler.ListFeaturedSlotsHandler(ctx, &ListFeaturedSlotsRequest{HistoryLimit: 5})
	s.Require().NoError(err)
	billSlot := listResp.Body.Slots[0]
	s.Nil(billSlot.Active)
	s.Len(billSlot.History, 1)
	s.Require().NotNil(billSlot.History[0].ActiveUntil)
}

func (s *FeaturedSlotHandlerIntegrationSuite) TestDelete_NoActiveReturns404() {
	admin := testhelpers.CreateAdminUser(s.deps.DB)
	ctx := testhelpers.CtxWithUser(admin)

	_, err := s.featuredSlotHandler.DeleteFeaturedSlotHandler(ctx, &DeleteFeaturedSlotRequest{SlotType: adminm.FeaturedSlotTypeBill})
	testhelpers.AssertHumaError(s.T(), err, 404)
}

func (s *FeaturedSlotHandlerIntegrationSuite) TestDelete_InvalidSlotTypeRejected() {
	admin := testhelpers.CreateAdminUser(s.deps.DB)
	ctx := testhelpers.CtxWithUser(admin)

	_, err := s.featuredSlotHandler.DeleteFeaturedSlotHandler(ctx, &DeleteFeaturedSlotRequest{SlotType: "scene"})
	testhelpers.AssertHumaError(s.T(), err, 400)
}
