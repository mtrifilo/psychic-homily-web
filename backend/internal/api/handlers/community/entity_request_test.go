package community

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"psychic-homily-backend/internal/api/handlers/shared/testhelpers"
	apperrors "psychic-homily-backend/internal/errors"
	authm "psychic-homily-backend/internal/models/auth"
	communitym "psychic-homily-backend/internal/models/community"
	"psychic-homily-backend/internal/services/contracts"
)

// ============================================================================
// Test helpers
// ============================================================================

func erAdminCtx() context.Context {
	return testhelpers.CtxWithUser(&authm.User{ID: 1, IsAdmin: true})
}

func erUserCtx() context.Context {
	return testhelpers.CtxWithUser(&authm.User{ID: 2, IsAdmin: false, UserTier: "contributor"})
}

func artistPayload(t *testing.T, name string) json.RawMessage {
	t.Helper()
	raw, err := communitym.MarshalPayload(communitym.ArtistRequestPayload{Name: name})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	return raw
}

func pendingRequest(id uint, entityType string) *communitym.EntityRequest {
	raw := json.RawMessage(`{"name":"Test"}`)
	return &communitym.EntityRequest{
		ID:            id,
		EntityType:    entityType,
		Payload:       &raw,
		RequesterID:   2,
		SourceContext: communitym.EntityRequestSourceManual,
		DecisionState: communitym.EntityRequestStatePending,
	}
}

// ============================================================================
// Tests: Queue-create — auth & validation
// ============================================================================

func TestCreateEntityRequest_NoUser(t *testing.T) {
	h := NewEntityRequestHandler(nil, nil, nil)
	req := &CreateEntityRequestRequest{}
	req.Body.EntityType = "artist"
	req.Body.Payload = artistPayload(t, "Boris")
	_, err := h.CreateEntityRequestHandler(context.Background(), req)
	testhelpers.AssertHumaError(t, err, 401)
}

func TestCreateEntityRequest_InvalidEntityType(t *testing.T) {
	h := NewEntityRequestHandler(nil, nil, nil)
	req := &CreateEntityRequestRequest{}
	req.Body.EntityType = "wizard"
	req.Body.Payload = json.RawMessage(`{"name":"x"}`)
	_, err := h.CreateEntityRequestHandler(erUserCtx(), req)
	testhelpers.AssertHumaError(t, err, 422)
}

func TestCreateEntityRequest_InvalidSource(t *testing.T) {
	h := NewEntityRequestHandler(nil, nil, nil)
	req := &CreateEntityRequestRequest{}
	req.Body.EntityType = "artist"
	req.Body.Payload = artistPayload(t, "Boris")
	req.Body.SourceContext = "carrier_pigeon"
	_, err := h.CreateEntityRequestHandler(erUserCtx(), req)
	testhelpers.AssertHumaError(t, err, 422)
}

func TestCreateEntityRequest_EmptyPayload(t *testing.T) {
	h := NewEntityRequestHandler(nil, nil, nil)
	req := &CreateEntityRequestRequest{}
	req.Body.EntityType = "artist"
	req.Body.Payload = json.RawMessage("   ")
	_, err := h.CreateEntityRequestHandler(erUserCtx(), req)
	testhelpers.AssertHumaError(t, err, 422)
}

// ============================================================================
// Tests: Queue-create — success
// ============================================================================

// contributor tier files a PENDING request (no autonomous creation).
func TestCreateEntityRequest_ContributorQueuesPending(t *testing.T) {
	want := pendingRequest(7, "artist")
	h := NewEntityRequestHandler(
		&testhelpers.MockEntityRequestService{
			CreateRequestFn: func(user *authm.User, entityType string, payload []byte, sourceContext string, confirmed bool) (*communitym.EntityRequest, error) {
				if user.ID != 2 {
					t.Errorf("expected requester 2, got %d", user.ID)
				}
				if entityType != "artist" {
					t.Errorf("expected artist, got %s", entityType)
				}
				if sourceContext != communitym.EntityRequestSourceManual {
					t.Errorf("expected default source manual, got %s", sourceContext)
				}
				return want, nil
			},
		},
		nil,
		&testhelpers.MockAuditLogService{},
	)

	req := &CreateEntityRequestRequest{}
	req.Body.EntityType = "artist"
	req.Body.Payload = artistPayload(t, "Boris")

	resp, err := h.CreateEntityRequestHandler(erUserCtx(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.ID != 7 {
		t.Errorf("expected id 7, got %d", resp.Body.ID)
	}
	if resp.Body.DecisionState != communitym.EntityRequestStatePending {
		t.Errorf("expected pending, got %s", resp.Body.DecisionState)
	}
}

func TestCreateEntityRequest_ServiceError(t *testing.T) {
	h := NewEntityRequestHandler(
		&testhelpers.MockEntityRequestService{
			CreateRequestFn: func(user *authm.User, entityType string, payload []byte, sourceContext string, confirmed bool) (*communitym.EntityRequest, error) {
				return nil, fmt.Errorf("db down")
			},
		},
		nil,
		nil,
	)
	req := &CreateEntityRequestRequest{}
	req.Body.EntityType = "artist"
	req.Body.Payload = artistPayload(t, "Boris")
	_, err := h.CreateEntityRequestHandler(erUserCtx(), req)
	testhelpers.AssertHumaError(t, err, 500)
}

func TestCreateEntityRequest_MapsTypedError(t *testing.T) {
	h := NewEntityRequestHandler(
		&testhelpers.MockEntityRequestService{
			CreateRequestFn: func(user *authm.User, entityType string, payload []byte, sourceContext string, confirmed bool) (*communitym.EntityRequest, error) {
				return nil, apperrors.ErrEntityRequestEmptyPayload("artist")
			},
		},
		nil,
		nil,
	)
	req := &CreateEntityRequestRequest{}
	req.Body.EntityType = "artist"
	req.Body.Payload = artistPayload(t, "Boris")
	_, err := h.CreateEntityRequestHandler(erUserCtx(), req)
	testhelpers.AssertHumaError(t, err, 422)
}

// ============================================================================
// Tests: Admin list
// ============================================================================

func TestAdminListEntityRequests_Success(t *testing.T) {
	rows := []communitym.EntityRequest{*pendingRequest(1, "artist")}
	h := NewEntityRequestHandler(
		&testhelpers.MockEntityRequestService{
			ListRequestsFn: func(f *contracts.EntityRequestFilters) ([]communitym.EntityRequest, int64, error) {
				if f.EntityType != "artist" {
					t.Errorf("expected entity_type filter artist, got %s", f.EntityType)
				}
				if f.SourceContext != communitym.EntityRequestSourcePasteMode {
					t.Errorf("expected source paste_mode, got %s", f.SourceContext)
				}
				return rows, 1, nil
			},
		},
		nil,
		nil,
	)

	resp, err := h.AdminListEntityRequestsHandler(erAdminCtx(), &AdminListEntityRequestsRequest{
		EntityType:    "artist",
		SourceContext: communitym.EntityRequestSourcePasteMode,
		State:         "pending",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Total != 1 || len(resp.Body.Requests) != 1 {
		t.Errorf("expected 1 request total=1, got total=%d len=%d", resp.Body.Total, len(resp.Body.Requests))
	}
}

func TestAdminListEntityRequests_InvalidEntityType(t *testing.T) {
	h := NewEntityRequestHandler(nil, nil, nil)
	_, err := h.AdminListEntityRequestsHandler(erAdminCtx(), &AdminListEntityRequestsRequest{EntityType: "wizard"})
	testhelpers.AssertHumaError(t, err, 422)
}

func TestAdminListEntityRequests_InvalidState(t *testing.T) {
	h := NewEntityRequestHandler(nil, nil, nil)
	_, err := h.AdminListEntityRequestsHandler(erAdminCtx(), &AdminListEntityRequestsRequest{State: "limbo"})
	testhelpers.AssertHumaError(t, err, 422)
}

func TestAdminListEntityRequests_InvalidSource(t *testing.T) {
	h := NewEntityRequestHandler(nil, nil, nil)
	_, err := h.AdminListEntityRequestsHandler(erAdminCtx(), &AdminListEntityRequestsRequest{SourceContext: "smoke_signal"})
	testhelpers.AssertHumaError(t, err, 422)
}

func TestAdminListEntityRequests_ServiceError(t *testing.T) {
	h := NewEntityRequestHandler(
		&testhelpers.MockEntityRequestService{
			ListRequestsFn: func(f *contracts.EntityRequestFilters) ([]communitym.EntityRequest, int64, error) {
				return nil, 0, fmt.Errorf("db down")
			},
		},
		nil,
		nil,
	)
	_, err := h.AdminListEntityRequestsHandler(erAdminCtx(), &AdminListEntityRequestsRequest{})
	testhelpers.AssertHumaError(t, err, 500)
}

// ============================================================================
// Tests: Admin decide — validation
// ============================================================================

func TestAdminDecide_InvalidID(t *testing.T) {
	h := NewEntityRequestHandler(nil, nil, nil)
	req := &AdminDecideEntityRequestRequest{ID: "abc"}
	req.Body.Decision = "approved"
	_, err := h.AdminDecideEntityRequestHandler(erAdminCtx(), req)
	testhelpers.AssertHumaError(t, err, 400)
}

func TestAdminDecide_InvalidDecision(t *testing.T) {
	h := NewEntityRequestHandler(nil, nil, nil)
	req := &AdminDecideEntityRequestRequest{ID: "1"}
	req.Body.Decision = "maybe"
	_, err := h.AdminDecideEntityRequestHandler(erAdminCtx(), req)
	testhelpers.AssertHumaError(t, err, 422)
}

// ============================================================================
// Tests: Admin decide — approve (creates entity)
// ============================================================================

func TestAdminDecide_ApproveArtist_CreatesEntity(t *testing.T) {
	decided := pendingRequest(5, "artist")
	decided.DecisionState = communitym.EntityRequestStateApproved
	createCalled := false

	h := NewEntityRequestHandler(
		&testhelpers.MockEntityRequestService{
			DecideFn: func(requestID, adminID uint, newState communitym.EntityRequestDecisionState, note *string) (*communitym.EntityRequest, error) {
				if requestID != 5 || adminID != 1 {
					t.Errorf("unexpected decide params: req=%d admin=%d", requestID, adminID)
				}
				if newState != communitym.EntityRequestStateApproved {
					t.Errorf("expected approved, got %s", newState)
				}
				return decided, nil
			},
		},
		&testhelpers.MockEntityRequestFulfiller{
			CreateArtistFn: func(req *contracts.CreateArtistRequest) (*contracts.ArtistDetailResponse, error) {
				createCalled = true
				if req.Name != "Test" {
					t.Errorf("expected name Test from payload, got %s", req.Name)
				}
				return &contracts.ArtistDetailResponse{ID: 99}, nil
			},
		},
		&testhelpers.MockAuditLogService{},
	)

	req := &AdminDecideEntityRequestRequest{ID: "5"}
	req.Body.Decision = "approved"

	resp, err := h.AdminDecideEntityRequestHandler(erAdminCtx(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !createCalled {
		t.Error("expected fulfiller CreateArtist to be called on approve")
	}
	if resp.Body.CreatedEntityID == nil || *resp.Body.CreatedEntityID != 99 {
		t.Errorf("expected created entity id 99, got %v", resp.Body.CreatedEntityID)
	}
	if resp.Body.CreatedEntityType == nil || *resp.Body.CreatedEntityType != "artist" {
		t.Errorf("expected created entity type artist, got %v", resp.Body.CreatedEntityType)
	}
	if resp.Body.Request.DecisionState != communitym.EntityRequestStateApproved {
		t.Errorf("expected request approved, got %s", resp.Body.Request.DecisionState)
	}
}

// Approving a show is unsupported (payload lacks venues + artists) → 422.
func TestAdminDecide_ApproveShow_Unsupported(t *testing.T) {
	decided := pendingRequest(6, "show")
	decided.DecisionState = communitym.EntityRequestStateApproved

	h := NewEntityRequestHandler(
		&testhelpers.MockEntityRequestService{
			DecideFn: func(requestID, adminID uint, newState communitym.EntityRequestDecisionState, note *string) (*communitym.EntityRequest, error) {
				return decided, nil
			},
		},
		&testhelpers.MockEntityRequestFulfiller{},
		&testhelpers.MockAuditLogService{},
	)

	req := &AdminDecideEntityRequestRequest{ID: "6"}
	req.Body.Decision = "approved"
	_, err := h.AdminDecideEntityRequestHandler(erAdminCtx(), req)
	// The row was claimed (approved) but fulfillment is unsupported → 422.
	testhelpers.AssertHumaError(t, err, 422)
}

// Fulfillment failure after claim surfaces a 500 (entity NOT created).
func TestAdminDecide_ApproveFulfillFails(t *testing.T) {
	decided := pendingRequest(8, "artist")
	decided.DecisionState = communitym.EntityRequestStateApproved

	h := NewEntityRequestHandler(
		&testhelpers.MockEntityRequestService{
			DecideFn: func(requestID, adminID uint, newState communitym.EntityRequestDecisionState, note *string) (*communitym.EntityRequest, error) {
				return decided, nil
			},
		},
		&testhelpers.MockEntityRequestFulfiller{
			CreateArtistFn: func(req *contracts.CreateArtistRequest) (*contracts.ArtistDetailResponse, error) {
				return nil, fmt.Errorf("slug collision")
			},
		},
		&testhelpers.MockAuditLogService{},
	)

	req := &AdminDecideEntityRequestRequest{ID: "8"}
	req.Body.Decision = "approved"
	_, err := h.AdminDecideEntityRequestHandler(erAdminCtx(), req)
	testhelpers.AssertHumaError(t, err, 500)
}

// ============================================================================
// Tests: Admin decide — reject (no entity created)
// ============================================================================

func TestAdminDecide_Reject(t *testing.T) {
	rejected := pendingRequest(9, "artist")
	rejected.DecisionState = communitym.EntityRequestStateRejected
	fulfillCalled := false

	h := NewEntityRequestHandler(
		&testhelpers.MockEntityRequestService{
			DecideFn: func(requestID, adminID uint, newState communitym.EntityRequestDecisionState, note *string) (*communitym.EntityRequest, error) {
				if newState != communitym.EntityRequestStateRejected {
					t.Errorf("expected rejected, got %s", newState)
				}
				if note == nil || *note != "not notable" {
					t.Errorf("expected trimmed note 'not notable', got %v", note)
				}
				return rejected, nil
			},
		},
		&testhelpers.MockEntityRequestFulfiller{
			CreateArtistFn: func(req *contracts.CreateArtistRequest) (*contracts.ArtistDetailResponse, error) {
				fulfillCalled = true
				return &contracts.ArtistDetailResponse{ID: 1}, nil
			},
		},
		&testhelpers.MockAuditLogService{},
	)

	note := "  not notable  "
	req := &AdminDecideEntityRequestRequest{ID: "9"}
	req.Body.Decision = "rejected"
	req.Body.Note = &note

	resp, err := h.AdminDecideEntityRequestHandler(erAdminCtx(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fulfillCalled {
		t.Error("reject must NOT create an entity")
	}
	if resp.Body.CreatedEntityID != nil {
		t.Errorf("reject must not report a created entity, got %v", resp.Body.CreatedEntityID)
	}
	if resp.Body.Request.DecisionState != communitym.EntityRequestStateRejected {
		t.Errorf("expected rejected, got %s", resp.Body.Request.DecisionState)
	}
}

// ============================================================================
// Tests: Admin decide — error mapping
// ============================================================================

func TestAdminDecide_NotFound(t *testing.T) {
	h := NewEntityRequestHandler(
		&testhelpers.MockEntityRequestService{
			DecideFn: func(requestID, adminID uint, newState communitym.EntityRequestDecisionState, note *string) (*communitym.EntityRequest, error) {
				return nil, apperrors.ErrEntityRequestNotFound(404)
			},
		},
		nil,
		nil,
	)
	req := &AdminDecideEntityRequestRequest{ID: "404"}
	req.Body.Decision = "approved"
	_, err := h.AdminDecideEntityRequestHandler(erAdminCtx(), req)
	testhelpers.AssertHumaError(t, err, 404)
}

// Re-deciding an already-resolved request → 409 conflict.
func TestAdminDecide_AlreadyDecided(t *testing.T) {
	h := NewEntityRequestHandler(
		&testhelpers.MockEntityRequestService{
			DecideFn: func(requestID, adminID uint, newState communitym.EntityRequestDecisionState, note *string) (*communitym.EntityRequest, error) {
				return nil, apperrors.ErrEntityRequestInvalidState(1, "approved")
			},
		},
		nil,
		nil,
	)
	req := &AdminDecideEntityRequestRequest{ID: "1"}
	req.Body.Decision = "approved"
	_, err := h.AdminDecideEntityRequestHandler(erAdminCtx(), req)
	testhelpers.AssertHumaError(t, err, 409)
}

func TestAdminDecide_ServiceError(t *testing.T) {
	h := NewEntityRequestHandler(
		&testhelpers.MockEntityRequestService{
			DecideFn: func(requestID, adminID uint, newState communitym.EntityRequestDecisionState, note *string) (*communitym.EntityRequest, error) {
				return nil, fmt.Errorf("db down")
			},
		},
		nil,
		nil,
	)
	req := &AdminDecideEntityRequestRequest{ID: "1"}
	req.Body.Decision = "rejected"
	_, err := h.AdminDecideEntityRequestHandler(erAdminCtx(), req)
	testhelpers.AssertHumaError(t, err, 500)
}
