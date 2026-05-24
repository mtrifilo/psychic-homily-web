package admin

import (
	"testing"

	"github.com/stretchr/testify/suite"

	"psychic-homily-backend/internal/api/handlers/shared/testhelpers"
	adminm "psychic-homily-backend/internal/models/admin"
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

	// Two bill picks → most-recent active, one in history.
	note1 := "first"
	first := &SetFeaturedSlotRequest{}
	first.Body.SlotType = adminm.FeaturedSlotTypeBill
	first.Body.EntityID = 1
	first.Body.CuratorNote = &note1
	_, err := s.featuredSlotHandler.SetFeaturedSlotHandler(ctx, first)
	s.Require().NoError(err)

	note2 := "second"
	second := &SetFeaturedSlotRequest{}
	second.Body.SlotType = adminm.FeaturedSlotTypeBill
	second.Body.EntityID = 2
	second.Body.CuratorNote = &note2
	_, err = s.featuredSlotHandler.SetFeaturedSlotHandler(ctx, second)
	s.Require().NoError(err)

	resp, err := s.featuredSlotHandler.ListFeaturedSlotsHandler(ctx, &ListFeaturedSlotsRequest{HistoryLimit: 5})
	s.Require().NoError(err)
	s.Require().Len(resp.Body.Slots, 2)

	billSlot := resp.Body.Slots[0]
	s.Require().NotNil(billSlot.Active)
	s.Equal(uint(2), billSlot.Active.EntityID)
	s.Len(billSlot.History, 2)
}

// =============================================================================
// Set
// =============================================================================

func (s *FeaturedSlotHandlerIntegrationSuite) TestSet_Success_FirstPick() {
	admin := testhelpers.CreateAdminUser(s.deps.DB)
	ctx := testhelpers.CtxWithUser(admin)

	note := "**killer** bill"
	req := &SetFeaturedSlotRequest{}
	req.Body.SlotType = adminm.FeaturedSlotTypeBill
	req.Body.EntityID = 7
	req.Body.CuratorNote = &note

	resp, err := s.featuredSlotHandler.SetFeaturedSlotHandler(ctx, req)
	s.Require().NoError(err)
	s.Equal(adminm.FeaturedSlotTypeBill, resp.Body.SlotType)
	s.Equal(uint(7), resp.Body.EntityID)
	s.Require().NotNil(resp.Body.CuratorNote)
	s.Equal("**killer** bill", *resp.Body.CuratorNote)
	s.Contains(resp.Body.CuratorNoteHTML, "<strong>killer</strong>")
	s.Nil(resp.Body.ActiveUntil)
	s.Equal(admin.ID, resp.Body.CreatedBy)
}

func (s *FeaturedSlotHandlerIntegrationSuite) TestSet_AtomicallyReplacesPrior() {
	admin := testhelpers.CreateAdminUser(s.deps.DB)
	ctx := testhelpers.CtxWithUser(admin)

	first := &SetFeaturedSlotRequest{}
	first.Body.SlotType = adminm.FeaturedSlotTypeBill
	first.Body.EntityID = 1
	firstResp, err := s.featuredSlotHandler.SetFeaturedSlotHandler(ctx, first)
	s.Require().NoError(err)
	firstID := firstResp.Body.ID

	second := &SetFeaturedSlotRequest{}
	second.Body.SlotType = adminm.FeaturedSlotTypeBill
	second.Body.EntityID = 2
	secondResp, err := s.featuredSlotHandler.SetFeaturedSlotHandler(ctx, second)
	s.Require().NoError(err)
	s.NotEqual(firstID, secondResp.Body.ID)

	// List shows new active + old in history.
	listResp, err := s.featuredSlotHandler.ListFeaturedSlotsHandler(ctx, &ListFeaturedSlotsRequest{HistoryLimit: 5})
	s.Require().NoError(err)
	billSlot := listResp.Body.Slots[0]
	s.Require().NotNil(billSlot.Active)
	s.Equal(uint(2), billSlot.Active.EntityID)
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
	req.Body.EntityID = 1
	req.Body.CuratorNote = &note
	_, err := s.featuredSlotHandler.SetFeaturedSlotHandler(ctx, req)
	testhelpers.AssertHumaError(s.T(), err, 400)
}

// =============================================================================
// Delete
// =============================================================================

func (s *FeaturedSlotHandlerIntegrationSuite) TestDelete_RetiresActive() {
	admin := testhelpers.CreateAdminUser(s.deps.DB)
	ctx := testhelpers.CtxWithUser(admin)

	setReq := &SetFeaturedSlotRequest{}
	setReq.Body.SlotType = adminm.FeaturedSlotTypeBill
	setReq.Body.EntityID = 1
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
