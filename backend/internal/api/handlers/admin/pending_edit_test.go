package handlers

import (
	"context"
	"fmt"
	"testing"
	"time"

	"psychic-homily-backend/internal/models"
	"psychic-homily-backend/internal/services/contracts"
)

// ============================================================================
// Test helpers
// ============================================================================

func testPendingEditHandler() *PendingEditHandler {
	return NewPendingEditHandler(nil, nil)
}

func pendingEditAdminCtx() context.Context {
	return ctxWithUser(&models.User{ID: 1, IsAdmin: true, UserTier: "new_user"})
}

func pendingEditTrustedCtx() context.Context {
	return ctxWithUser(&models.User{ID: 2, IsAdmin: false, UserTier: "trusted_contributor"})
}

func pendingEditNewUserCtx() context.Context {
	return ctxWithUser(&models.User{ID: 3, IsAdmin: false, UserTier: "new_user"})
}

func pendingEditContributorCtx() context.Context {
	return ctxWithUser(&models.User{ID: 4, IsAdmin: false, UserTier: "contributor"})
}

func pendingEditLocalAmbassadorCtx() context.Context {
	return ctxWithUser(&models.User{ID: 5, IsAdmin: false, UserTier: "local_ambassador"})
}

func makePendingEditResponse(id uint) *contracts.PendingEditResponse {
	now := time.Now()
	return &contracts.PendingEditResponse{
		ID:          id,
		EntityType:  "artist",
		EntityID:    10,
		SubmittedBy: 3,
		SubmitterName: "testuser",
		FieldChanges: []models.FieldChange{
			{Field: "name", OldValue: "Old", NewValue: "New"},
		},
		Summary:   "Fix name",
		Status:    models.PendingEditStatusPending,
		CreatedAt: now,
		UpdatedAt: now,
	}
}

// ============================================================================
// Tests: SuggestEdit — Auth & Validation
// ============================================================================

func TestSuggestEdit_NoUser(t *testing.T) {
	h := testPendingEditHandler()
	_, err := h.SuggestArtistEditHandler(context.Background(), &SuggestEntityEditRequest{
		EntityID: "1",
	})
	assertHumaError(t, err, 401)
}

func TestSuggestEdit_InvalidEntityID(t *testing.T) {
	h := testPendingEditHandler()
	_, err := h.SuggestArtistEditHandler(pendingEditNewUserCtx(), &SuggestEntityEditRequest{
		EntityID: "abc",
	})
	assertHumaError(t, err, 400)
}

func TestSuggestEdit_NoChanges(t *testing.T) {
	h := testPendingEditHandler()
	req := &SuggestEntityEditRequest{EntityID: "1"}
	req.Body.Changes = []models.FieldChange{}
	req.Body.Summary = "test"
	_, err := h.SuggestArtistEditHandler(pendingEditNewUserCtx(), req)
	assertHumaError(t, err, 400)
}

func TestSuggestEdit_NoSummary(t *testing.T) {
	h := testPendingEditHandler()
	req := &SuggestEntityEditRequest{EntityID: "1"}
	req.Body.Changes = []models.FieldChange{{Field: "name", OldValue: "a", NewValue: "b"}}
	req.Body.Summary = ""
	_, err := h.SuggestArtistEditHandler(pendingEditNewUserCtx(), req)
	assertHumaError(t, err, 400)
}

func TestSuggestEdit_DisallowedField(t *testing.T) {
	h := testPendingEditHandler()
	req := &SuggestEntityEditRequest{EntityID: "1"}
	req.Body.Changes = []models.FieldChange{{Field: "is_active", OldValue: true, NewValue: false}}
	req.Body.Summary = "hack"
	_, err := h.SuggestArtistEditHandler(pendingEditNewUserCtx(), req)
	assertHumaError(t, err, 400)
}

func TestSuggestEdit_VenueDisallowedField(t *testing.T) {
	h := testPendingEditHandler()
	req := &SuggestEntityEditRequest{EntityID: "1"}
	req.Body.Changes = []models.FieldChange{{Field: "verified", OldValue: false, NewValue: true}}
	req.Body.Summary = "verify"
	_, err := h.SuggestVenueEditHandler(pendingEditNewUserCtx(), req)
	assertHumaError(t, err, 400)
}

func TestSuggestEdit_FestivalDisallowedField(t *testing.T) {
	h := testPendingEditHandler()
	req := &SuggestEntityEditRequest{EntityID: "1"}
	req.Body.Changes = []models.FieldChange{{Field: "status", OldValue: "announced", NewValue: "cancelled"}}
	req.Body.Summary = "cancel"
	_, err := h.SuggestFestivalEditHandler(pendingEditNewUserCtx(), req)
	assertHumaError(t, err, 400)
}

// ============================================================================
// Tests: SuggestEdit — New User (creates pending)
// ============================================================================

func TestSuggestEdit_NewUser_CreatesPending(t *testing.T) {
	expected := makePendingEditResponse(1)
	h := NewPendingEditHandler(
		&mockPendingEditService{
			createPendingEditFn: func(req *contracts.CreatePendingEditRequest) (*contracts.PendingEditResponse, error) {
				if req.EntityType != "artist" || req.EntityID != 10 || req.UserID != 3 {
					t.Errorf("unexpected params: %+v", req)
				}
				return expected, nil
			},
		},
		nil,
	)

	req := &SuggestEntityEditRequest{EntityID: "10"}
	req.Body.Changes = []models.FieldChange{{Field: "name", OldValue: "Old", NewValue: "New"}}
	req.Body.Summary = "Fix name"

	resp, err := h.SuggestArtistEditHandler(pendingEditNewUserCtx(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Applied {
		t.Error("expected applied=false for new_user")
	}
	if resp.Body.PendingEdit == nil {
		t.Fatal("expected pending edit in response")
	}
	if resp.Body.PendingEdit.ID != 1 {
		t.Errorf("expected edit ID=1, got %d", resp.Body.PendingEdit.ID)
	}
}

func TestSuggestEdit_Contributor_CreatesPending(t *testing.T) {
	expected := makePendingEditResponse(2)
	expected.SubmittedBy = 4
	h := NewPendingEditHandler(
		&mockPendingEditService{
			createPendingEditFn: func(req *contracts.CreatePendingEditRequest) (*contracts.PendingEditResponse, error) {
				return expected, nil
			},
		},
		nil,
	)

	req := &SuggestEntityEditRequest{EntityID: "10"}
	req.Body.Changes = []models.FieldChange{{Field: "city", OldValue: "", NewValue: "Phoenix"}}
	req.Body.Summary = "Add city"

	resp, err := h.SuggestVenueEditHandler(pendingEditContributorCtx(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Applied {
		t.Error("expected applied=false for contributor")
	}
}

// ============================================================================
// Tests: SuggestEdit — Trusted User (auto-applies)
// ============================================================================

func TestSuggestEdit_TrustedContributor_AutoApplies(t *testing.T) {
	created := makePendingEditResponse(3)
	approved := makePendingEditResponse(3)
	approved.Status = models.PendingEditStatusApproved

	h := NewPendingEditHandler(
		&mockPendingEditService{
			createPendingEditFn: func(req *contracts.CreatePendingEditRequest) (*contracts.PendingEditResponse, error) {
				return created, nil
			},
			approvePendingEditFn: func(editID, reviewerID uint) (*contracts.PendingEditResponse, error) {
				if editID != 3 {
					t.Errorf("expected approve editID=3, got %d", editID)
				}
				return approved, nil
			},
		},
		nil,
	)

	req := &SuggestEntityEditRequest{EntityID: "10"}
	req.Body.Changes = []models.FieldChange{{Field: "name", OldValue: "Old", NewValue: "New"}}
	req.Body.Summary = "Fix name"

	resp, err := h.SuggestArtistEditHandler(pendingEditTrustedCtx(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.Body.Applied {
		t.Error("expected applied=true for trusted_contributor")
	}
	if resp.Body.Message != "Changes applied directly" {
		t.Errorf("expected direct apply message, got: %s", resp.Body.Message)
	}
}

func TestSuggestEdit_LocalAmbassador_AutoApplies(t *testing.T) {
	created := makePendingEditResponse(4)
	approved := makePendingEditResponse(4)
	approved.Status = models.PendingEditStatusApproved

	h := NewPendingEditHandler(
		&mockPendingEditService{
			createPendingEditFn: func(req *contracts.CreatePendingEditRequest) (*contracts.PendingEditResponse, error) {
				return created, nil
			},
			approvePendingEditFn: func(editID, reviewerID uint) (*contracts.PendingEditResponse, error) {
				return approved, nil
			},
		},
		nil,
	)

	req := &SuggestEntityEditRequest{EntityID: "10"}
	req.Body.Changes = []models.FieldChange{{Field: "name", OldValue: "Old", NewValue: "New"}}
	req.Body.Summary = "Fix name"

	resp, err := h.SuggestArtistEditHandler(pendingEditLocalAmbassadorCtx(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.Body.Applied {
		t.Error("expected applied=true for local_ambassador")
	}
}

func TestSuggestEdit_Admin_AutoApplies(t *testing.T) {
	created := makePendingEditResponse(5)
	approved := makePendingEditResponse(5)
	approved.Status = models.PendingEditStatusApproved

	h := NewPendingEditHandler(
		&mockPendingEditService{
			createPendingEditFn: func(req *contracts.CreatePendingEditRequest) (*contracts.PendingEditResponse, error) {
				return created, nil
			},
			approvePendingEditFn: func(editID, reviewerID uint) (*contracts.PendingEditResponse, error) {
				return approved, nil
			},
		},
		nil,
	)

	req := &SuggestEntityEditRequest{EntityID: "10"}
	req.Body.Changes = []models.FieldChange{{Field: "name", OldValue: "Old", NewValue: "New"}}
	req.Body.Summary = "Fix name"

	resp, err := h.SuggestArtistEditHandler(pendingEditAdminCtx(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.Body.Applied {
		t.Error("expected applied=true for admin")
	}
}

// ============================================================================
// Tests: SuggestEdit — Error Cases
// ============================================================================

func TestSuggestEdit_EntityNotFound(t *testing.T) {
	h := NewPendingEditHandler(
		&mockPendingEditService{
			createPendingEditFn: func(req *contracts.CreatePendingEditRequest) (*contracts.PendingEditResponse, error) {
				return nil, fmt.Errorf("entity not found: artist 99999")
			},
		},
		nil,
	)

	req := &SuggestEntityEditRequest{EntityID: "99999"}
	req.Body.Changes = []models.FieldChange{{Field: "name", OldValue: "a", NewValue: "b"}}
	req.Body.Summary = "test"

	_, err := h.SuggestArtistEditHandler(pendingEditNewUserCtx(), req)
	assertHumaError(t, err, 404)
}

func TestSuggestEdit_DuplicatePending(t *testing.T) {
	h := NewPendingEditHandler(
		&mockPendingEditService{
			createPendingEditFn: func(req *contracts.CreatePendingEditRequest) (*contracts.PendingEditResponse, error) {
				return nil, fmt.Errorf("failed to create pending edit: duplicate key value violates unique constraint")
			},
		},
		nil,
	)

	req := &SuggestEntityEditRequest{EntityID: "10"}
	req.Body.Changes = []models.FieldChange{{Field: "name", OldValue: "a", NewValue: "b"}}
	req.Body.Summary = "test"

	_, err := h.SuggestArtistEditHandler(pendingEditNewUserCtx(), req)
	assertHumaError(t, err, 409)
}

// ============================================================================
// Tests: GetMyPendingEdits
// ============================================================================

func TestGetMyPendingEdits_NoUser(t *testing.T) {
	h := testPendingEditHandler()
	_, err := h.GetMyPendingEditsHandler(context.Background(), &GetMyPendingEditsRequest{})
	assertHumaError(t, err, 401)
}

func TestGetMyPendingEdits_Success(t *testing.T) {
	edits := []contracts.PendingEditResponse{*makePendingEditResponse(1), *makePendingEditResponse(2)}
	h := NewPendingEditHandler(
		&mockPendingEditService{
			getUserPendingEditsFn: func(userID uint, limit, offset int) ([]contracts.PendingEditResponse, int64, error) {
				if userID != 3 {
					t.Errorf("expected userID=3, got %d", userID)
				}
				return edits, 2, nil
			},
		},
		nil,
	)

	resp, err := h.GetMyPendingEditsHandler(pendingEditNewUserCtx(), &GetMyPendingEditsRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Total != 2 {
		t.Errorf("expected total=2, got %d", resp.Body.Total)
	}
	if len(resp.Body.Edits) != 2 {
		t.Errorf("expected 2 edits, got %d", len(resp.Body.Edits))
	}
}

// ============================================================================
// Tests: CancelMyPendingEdit
// ============================================================================

func TestCancelMyPendingEdit_NoUser(t *testing.T) {
	h := testPendingEditHandler()
	_, err := h.CancelMyPendingEditHandler(context.Background(), &CancelMyPendingEntityEditRequest{EditID: "1"})
	assertHumaError(t, err, 401)
}

func TestCancelMyPendingEdit_InvalidID(t *testing.T) {
	h := testPendingEditHandler()
	_, err := h.CancelMyPendingEditHandler(pendingEditNewUserCtx(), &CancelMyPendingEntityEditRequest{EditID: "abc"})
	assertHumaError(t, err, 400)
}

func TestCancelMyPendingEdit_Success(t *testing.T) {
	h := NewPendingEditHandler(
		&mockPendingEditService{
			cancelPendingEditFn: func(editID, userID uint) error {
				if editID != 5 || userID != 3 {
					t.Errorf("unexpected params: editID=%d, userID=%d", editID, userID)
				}
				return nil
			},
		},
		nil,
	)

	resp, err := h.CancelMyPendingEditHandler(pendingEditNewUserCtx(), &CancelMyPendingEntityEditRequest{EditID: "5"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.Body.Success {
		t.Error("expected success=true")
	}
}

func TestCancelMyPendingEdit_NotFound(t *testing.T) {
	h := NewPendingEditHandler(
		&mockPendingEditService{
			cancelPendingEditFn: func(editID, userID uint) error {
				return fmt.Errorf("pending edit not found")
			},
		},
		nil,
	)

	_, err := h.CancelMyPendingEditHandler(pendingEditNewUserCtx(), &CancelMyPendingEntityEditRequest{EditID: "99"})
	assertHumaError(t, err, 404)
}

func TestCancelMyPendingEdit_WrongUser(t *testing.T) {
	h := NewPendingEditHandler(
		&mockPendingEditService{
			cancelPendingEditFn: func(editID, userID uint) error {
				return fmt.Errorf("only the submitter can cancel")
			},
		},
		nil,
	)

	_, err := h.CancelMyPendingEditHandler(pendingEditNewUserCtx(), &CancelMyPendingEntityEditRequest{EditID: "5"})
	assertHumaError(t, err, 403)
}

// ============================================================================
// Tests: Admin — List Pending Edits
// ============================================================================

func TestAdminListPendingEdits_RequiresAdmin(t *testing.T) {
	h := testPendingEditHandler()

	t.Run("NoUser", func(t *testing.T) {
		_, err := h.AdminListPendingEditsHandler(context.Background(), &AdminListPendingEditsRequest{})
		assertHumaError(t, err, 403)
	})
	t.Run("NonAdmin", func(t *testing.T) {
		_, err := h.AdminListPendingEditsHandler(pendingEditNewUserCtx(), &AdminListPendingEditsRequest{})
		assertHumaError(t, err, 403)
	})
}

func TestAdminListPendingEdits_Success(t *testing.T) {
	edits := []contracts.PendingEditResponse{*makePendingEditResponse(1)}
	h := NewPendingEditHandler(
		&mockPendingEditService{
			listPendingEditsFn: func(filters *contracts.PendingEditFilters) ([]contracts.PendingEditResponse, int64, error) {
				if filters.Status != "pending" {
					t.Errorf("expected status=pending, got %s", filters.Status)
				}
				return edits, 1, nil
			},
		},
		nil,
	)

	resp, err := h.AdminListPendingEditsHandler(pendingEditAdminCtx(), &AdminListPendingEditsRequest{
		Status: "pending",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Total != 1 {
		t.Errorf("expected total=1, got %d", resp.Body.Total)
	}
}

// ============================================================================
// Tests: Admin — Get Pending Edit
// ============================================================================

func TestAdminGetPendingEdit_RequiresAdmin(t *testing.T) {
	h := testPendingEditHandler()
	_, err := h.AdminGetPendingEditHandler(pendingEditNewUserCtx(), &AdminGetPendingEditRequest{EditID: "1"})
	assertHumaError(t, err, 403)
}

func TestAdminGetPendingEdit_NotFound(t *testing.T) {
	h := NewPendingEditHandler(
		&mockPendingEditService{
			getPendingEditFn: func(editID uint) (*contracts.PendingEditResponse, error) {
				return nil, nil
			},
		},
		nil,
	)

	_, err := h.AdminGetPendingEditHandler(pendingEditAdminCtx(), &AdminGetPendingEditRequest{EditID: "99"})
	assertHumaError(t, err, 404)
}

func TestAdminGetPendingEdit_Success(t *testing.T) {
	expected := makePendingEditResponse(1)
	h := NewPendingEditHandler(
		&mockPendingEditService{
			getPendingEditFn: func(editID uint) (*contracts.PendingEditResponse, error) {
				if editID != 1 {
					t.Errorf("expected editID=1, got %d", editID)
				}
				return expected, nil
			},
		},
		nil,
	)

	resp, err := h.AdminGetPendingEditHandler(pendingEditAdminCtx(), &AdminGetPendingEditRequest{EditID: "1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.ID != 1 {
		t.Errorf("expected ID=1, got %d", resp.Body.ID)
	}
}

// ============================================================================
// Tests: Admin — Approve
// ============================================================================

func TestAdminApprove_RequiresAdmin(t *testing.T) {
	h := testPendingEditHandler()
	_, err := h.AdminApprovePendingEditHandler(pendingEditNewUserCtx(), &AdminApprovePendingEditRequest{EditID: "1"})
	assertHumaError(t, err, 403)
}

func TestAdminApprove_Success(t *testing.T) {
	approved := makePendingEditResponse(1)
	approved.Status = models.PendingEditStatusApproved
	reviewerID := uint(1)
	approved.ReviewedBy = &reviewerID

	h := NewPendingEditHandler(
		&mockPendingEditService{
			approvePendingEditFn: func(editID, rID uint) (*contracts.PendingEditResponse, error) {
				if editID != 1 || rID != 1 {
					t.Errorf("unexpected params: editID=%d, reviewerID=%d", editID, rID)
				}
				return approved, nil
			},
		},
		&mockAuditLogService{},
	)

	resp, err := h.AdminApprovePendingEditHandler(pendingEditAdminCtx(), &AdminApprovePendingEditRequest{EditID: "1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Status != models.PendingEditStatusApproved {
		t.Errorf("expected approved status, got %s", resp.Body.Status)
	}
}

func TestAdminApprove_NotFound(t *testing.T) {
	h := NewPendingEditHandler(
		&mockPendingEditService{
			approvePendingEditFn: func(editID, rID uint) (*contracts.PendingEditResponse, error) {
				return nil, fmt.Errorf("pending edit not found")
			},
		},
		nil,
	)

	_, err := h.AdminApprovePendingEditHandler(pendingEditAdminCtx(), &AdminApprovePendingEditRequest{EditID: "99"})
	assertHumaError(t, err, 404)
}

func TestAdminApprove_AlreadyReviewed(t *testing.T) {
	h := NewPendingEditHandler(
		&mockPendingEditService{
			approvePendingEditFn: func(editID, rID uint) (*contracts.PendingEditResponse, error) {
				return nil, fmt.Errorf("edit is not pending (status: approved)")
			},
		},
		nil,
	)

	_, err := h.AdminApprovePendingEditHandler(pendingEditAdminCtx(), &AdminApprovePendingEditRequest{EditID: "1"})
	assertHumaError(t, err, 409)
}

// ============================================================================
// Tests: Admin — Reject
// ============================================================================

func TestAdminReject_RequiresAdmin(t *testing.T) {
	h := testPendingEditHandler()
	req := &AdminRejectPendingEditRequest{EditID: "1"}
	req.Body.Reason = "bad"
	_, err := h.AdminRejectPendingEditHandler(pendingEditNewUserCtx(), req)
	assertHumaError(t, err, 403)
}

func TestAdminReject_EmptyReason(t *testing.T) {
	h := testPendingEditHandler()
	req := &AdminRejectPendingEditRequest{EditID: "1"}
	req.Body.Reason = ""
	_, err := h.AdminRejectPendingEditHandler(pendingEditAdminCtx(), req)
	assertHumaError(t, err, 400)
}

func TestAdminReject_Success(t *testing.T) {
	rejected := makePendingEditResponse(1)
	rejected.Status = models.PendingEditStatusRejected
	reason := "Name should be 'The Rebel Lounge' not 'Rebel Lounge'"
	rejected.RejectionReason = &reason

	h := NewPendingEditHandler(
		&mockPendingEditService{
			rejectPendingEditFn: func(editID, rID uint, r string) (*contracts.PendingEditResponse, error) {
				if r != reason {
					t.Errorf("expected reason=%q, got %q", reason, r)
				}
				return rejected, nil
			},
		},
		&mockAuditLogService{},
	)

	req := &AdminRejectPendingEditRequest{EditID: "1"}
	req.Body.Reason = reason

	resp, err := h.AdminRejectPendingEditHandler(pendingEditAdminCtx(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Status != models.PendingEditStatusRejected {
		t.Errorf("expected rejected status, got %s", resp.Body.Status)
	}
	if resp.Body.RejectionReason == nil || *resp.Body.RejectionReason != reason {
		t.Errorf("expected rejection reason")
	}
}

func TestAdminReject_NotFound(t *testing.T) {
	h := NewPendingEditHandler(
		&mockPendingEditService{
			rejectPendingEditFn: func(editID, rID uint, r string) (*contracts.PendingEditResponse, error) {
				return nil, fmt.Errorf("pending edit not found")
			},
		},
		nil,
	)

	req := &AdminRejectPendingEditRequest{EditID: "99"}
	req.Body.Reason = "bad"
	_, err := h.AdminRejectPendingEditHandler(pendingEditAdminCtx(), req)
	assertHumaError(t, err, 404)
}

// ============================================================================
// Tests: Admin — Get Entity Pending Edits
// ============================================================================

func TestAdminGetEntityPendingEdits_RequiresAdmin(t *testing.T) {
	h := testPendingEditHandler()
	_, err := h.AdminGetEntityPendingEditsHandler(pendingEditNewUserCtx(), &AdminGetEntityPendingEditsRequest{
		EntityType: "artist", EntityID: "1",
	})
	assertHumaError(t, err, 403)
}

func TestAdminGetEntityPendingEdits_InvalidEntityType(t *testing.T) {
	h := testPendingEditHandler()
	_, err := h.AdminGetEntityPendingEditsHandler(pendingEditAdminCtx(), &AdminGetEntityPendingEditsRequest{
		EntityType: "show", EntityID: "1",
	})
	assertHumaError(t, err, 400)
}

func TestAdminGetEntityPendingEdits_Success(t *testing.T) {
	edits := []contracts.PendingEditResponse{*makePendingEditResponse(1)}
	h := NewPendingEditHandler(
		&mockPendingEditService{
			getPendingEditsForEntityFn: func(entityType string, entityID uint) ([]contracts.PendingEditResponse, error) {
				if entityType != "artist" || entityID != 10 {
					t.Errorf("unexpected params: type=%s, id=%d", entityType, entityID)
				}
				return edits, nil
			},
		},
		nil,
	)

	resp, err := h.AdminGetEntityPendingEditsHandler(pendingEditAdminCtx(), &AdminGetEntityPendingEditsRequest{
		EntityType: "artist", EntityID: "10",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Body.Edits) != 1 {
		t.Errorf("expected 1 edit, got %d", len(resp.Body.Edits))
	}
}

// ============================================================================
// Tests: canEditDirectly
// ============================================================================

func TestCanEditDirectly(t *testing.T) {
	tests := []struct {
		name     string
		user     *models.User
		expected bool
	}{
		{"admin", &models.User{IsAdmin: true, UserTier: "new_user"}, true},
		{"trusted_contributor", &models.User{IsAdmin: false, UserTier: "trusted_contributor"}, true},
		{"local_ambassador", &models.User{IsAdmin: false, UserTier: "local_ambassador"}, true},
		{"new_user", &models.User{IsAdmin: false, UserTier: "new_user"}, false},
		{"contributor", &models.User{IsAdmin: false, UserTier: "contributor"}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := canEditDirectly(tt.user); got != tt.expected {
				t.Errorf("canEditDirectly(%s) = %v, want %v", tt.name, got, tt.expected)
			}
		})
	}
}

// ============================================================================
// Tests: Allowed fields validation
// ============================================================================

func TestAllowedEditFields(t *testing.T) {
	// Artist allowed
	for _, f := range []string{"name", "city", "state", "instagram", "bandcamp", "description"} {
		if !allowedEditFields["artist"][f] {
			t.Errorf("expected %s to be allowed for artist", f)
		}
	}
	// Artist disallowed
	for _, f := range []string{"slug", "is_active", "data_source", "created_at"} {
		if allowedEditFields["artist"][f] {
			t.Errorf("expected %s to be disallowed for artist", f)
		}
	}

	// Venue allowed
	for _, f := range []string{"name", "address", "city", "zipcode", "website"} {
		if !allowedEditFields["venue"][f] {
			t.Errorf("expected %s to be allowed for venue", f)
		}
	}
	// Venue disallowed
	for _, f := range []string{"verified", "submitted_by", "auto_approve"} {
		if allowedEditFields["venue"][f] {
			t.Errorf("expected %s to be disallowed for venue", f)
		}
	}

	// Festival allowed
	for _, f := range []string{"name", "description", "website", "ticket_url", "flyer_url"} {
		if !allowedEditFields["festival"][f] {
			t.Errorf("expected %s to be allowed for festival", f)
		}
	}
	// Festival disallowed
	for _, f := range []string{"status", "slug", "series_slug", "edition_year"} {
		if allowedEditFields["festival"][f] {
			t.Errorf("expected %s to be disallowed for festival", f)
		}
	}
}
