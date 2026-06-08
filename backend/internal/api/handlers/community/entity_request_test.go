package community

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
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

// approvedRequest is a pendingRequest already flipped to approved — the shape an
// auto-approve tier's CreateRequest returns (the service stamps the decision).
func approvedRequest(id uint, entityType string) *communitym.EntityRequest {
	r := pendingRequest(id, entityType)
	r.DecisionState = communitym.EntityRequestStateApproved
	return r
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

// Payload missing the type's required field (empty name) → 422 at create time,
// not deferred to fulfillment.
func TestCreateEntityRequest_PayloadMissingRequiredField(t *testing.T) {
	h := NewEntityRequestHandler(
		&testhelpers.MockEntityRequestService{
			CreateRequestFn: func(user *authm.User, entityType string, payload []byte, sourceContext string, sourceDetail []byte, confirmed bool) (*communitym.EntityRequest, error) {
				t.Fatal("service must NOT be called for an invalid payload")
				return nil, nil
			},
		},
		nil, nil,
	)
	req := &CreateEntityRequestRequest{}
	req.Body.EntityType = "artist"
	req.Body.Payload = json.RawMessage(`{"name":""}`)
	_, err := h.CreateEntityRequestHandler(erUserCtx(), req)
	testhelpers.AssertHumaError(t, err, 422)
}

// Payload with an unknown field (schema drift) → 422 (UnmarshalPayload's
// DisallowUnknownFields guard surfaces at the boundary).
func TestCreateEntityRequest_PayloadUnknownField(t *testing.T) {
	h := NewEntityRequestHandler(
		&testhelpers.MockEntityRequestService{
			CreateRequestFn: func(user *authm.User, entityType string, payload []byte, sourceContext string, sourceDetail []byte, confirmed bool) (*communitym.EntityRequest, error) {
				t.Fatal("service must NOT be called for an invalid payload")
				return nil, nil
			},
		},
		nil, nil,
	)
	req := &CreateEntityRequestRequest{}
	req.Body.EntityType = "artist"
	req.Body.Payload = json.RawMessage(`{"name":"Boris","sneaky":"x"}`)
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
			CreateRequestFn: func(user *authm.User, entityType string, payload []byte, sourceContext string, sourceDetail []byte, confirmed bool) (*communitym.EntityRequest, error) {
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

// ============================================================================
// Tests: Queue-create — auto-approve fulfillment + source detail (PSY-1008)
// ============================================================================

// An auto-approve tier's request is fulfilled inline: the catalog entity is
// created, created_entity_id is persisted (RecordFulfillment) AND rides back on
// the response body so the frontend can stage it (true inline create-and-add).
func TestCreateEntityRequest_AutoApprove_FulfillsAndReturnsID(t *testing.T) {
	approved := approvedRequest(11, "artist")
	createCalled := false
	recordedID := uint(0)

	h := NewEntityRequestHandler(
		&testhelpers.MockEntityRequestService{
			CreateRequestFn: func(user *authm.User, entityType string, payload []byte, sourceContext string, sourceDetail []byte, confirmed bool) (*communitym.EntityRequest, error) {
				return approved, nil
			},
			RecordFulfillmentFn: func(requestID, createdEntityID uint) error {
				if requestID != 11 {
					t.Errorf("expected RecordFulfillment for request 11, got %d", requestID)
				}
				recordedID = createdEntityID
				return nil
			},
		},
		&testhelpers.MockEntityRequestFulfiller{
			CreateArtistFn: func(req *contracts.CreateArtistRequest) (*contracts.ArtistDetailResponse, error) {
				createCalled = true
				return &contracts.ArtistDetailResponse{ID: 77}, nil
			},
		},
		&testhelpers.MockAuditLogService{},
	)

	req := &CreateEntityRequestRequest{}
	req.Body.EntityType = "artist"
	req.Body.Payload = artistPayload(t, "Auto Band")

	resp, err := h.CreateEntityRequestHandler(erAdminCtx(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !createCalled {
		t.Error("expected fulfiller CreateArtist to be called on auto-approve")
	}
	if recordedID != 77 {
		t.Errorf("expected created_entity_id 77 persisted via RecordFulfillment, got %d", recordedID)
	}
	if resp.Body.CreatedEntityID == nil || *resp.Body.CreatedEntityID != 77 {
		t.Errorf("expected created_entity_id 77 on response body, got %v", resp.Body.CreatedEntityID)
	}
	if resp.Body.DecisionState != communitym.EntityRequestStateApproved {
		t.Errorf("expected approved, got %s", resp.Body.DecisionState)
	}
}

// Auto-approving a show is gracefully DEFERRED (not an error): the request is
// filed-and-approved, but show fulfillment needs associations the payload lacks
// (PSY-998), so no entity is created, no created_entity_id is returned, the
// link-back is not recorded, and the create still succeeds (200).
func TestCreateEntityRequest_AutoApprove_ShowDeferredNotError(t *testing.T) {
	approved := approvedRequest(12, "show")

	h := NewEntityRequestHandler(
		&testhelpers.MockEntityRequestService{
			CreateRequestFn: func(user *authm.User, entityType string, payload []byte, sourceContext string, sourceDetail []byte, confirmed bool) (*communitym.EntityRequest, error) {
				return approved, nil
			},
			RecordFulfillmentFn: func(requestID, createdEntityID uint) error {
				t.Fatal("RecordFulfillment must NOT be called when fulfillment is unsupported")
				return nil
			},
		},
		&testhelpers.MockEntityRequestFulfiller{},
		&testhelpers.MockAuditLogService{},
	)

	showPayload, err := communitym.MarshalPayload(communitym.ShowRequestPayload{Title: "Big Fest", EventDate: "2026-07-01"})
	if err != nil {
		t.Fatalf("marshal show payload: %v", err)
	}
	req := &CreateEntityRequestRequest{}
	req.Body.EntityType = "show"
	req.Body.Payload = showPayload

	resp, err := h.CreateEntityRequestHandler(erAdminCtx(), req)
	if err != nil {
		t.Fatalf("auto-approve of an unsupported type must not error, got %v", err)
	}
	if resp.Body.CreatedEntityID != nil {
		t.Errorf("show fulfillment is deferred; expected no created_entity_id, got %v", resp.Body.CreatedEntityID)
	}
	if resp.Body.DecisionState != communitym.EntityRequestStateApproved {
		t.Errorf("expected the request to remain approved, got %s", resp.Body.DecisionState)
	}
}

// A real fulfillment failure (catalog error, not unsupported) on the auto-approve
// path surfaces a 500 so the requester learns the entity was NOT created.
func TestCreateEntityRequest_AutoApprove_FulfillFails(t *testing.T) {
	approved := approvedRequest(13, "artist")

	h := NewEntityRequestHandler(
		&testhelpers.MockEntityRequestService{
			CreateRequestFn: func(user *authm.User, entityType string, payload []byte, sourceContext string, sourceDetail []byte, confirmed bool) (*communitym.EntityRequest, error) {
				return approved, nil
			},
		},
		&testhelpers.MockEntityRequestFulfiller{
			CreateArtistFn: func(req *contracts.CreateArtistRequest) (*contracts.ArtistDetailResponse, error) {
				return nil, fmt.Errorf("slug collision")
			},
		},
		&testhelpers.MockAuditLogService{},
	)

	req := &CreateEntityRequestRequest{}
	req.Body.EntityType = "artist"
	req.Body.Payload = artistPayload(t, "Boom")
	_, err := h.CreateEntityRequestHandler(erAdminCtx(), req)
	testhelpers.AssertHumaError(t, err, 500)
}

// A catalog "already exists" conflict on the auto-approve create path maps to
// 409, NOT 500 — inline create-and-add of an entity that already exists is a
// benign conflict, not a server fault. (Regression guard for the adversarial
// finding that catalog errors fell through to 500.)
func TestCreateEntityRequest_AutoApprove_ExistsConflictIs409(t *testing.T) {
	approved := approvedRequest(16, "artist")

	h := NewEntityRequestHandler(
		&testhelpers.MockEntityRequestService{
			CreateRequestFn: func(user *authm.User, entityType string, payload []byte, sourceContext string, sourceDetail []byte, confirmed bool) (*communitym.EntityRequest, error) {
				return approved, nil
			},
		},
		&testhelpers.MockEntityRequestFulfiller{
			CreateArtistFn: func(req *contracts.CreateArtistRequest) (*contracts.ArtistDetailResponse, error) {
				return nil, apperrors.ErrArtistExists("Boris")
			},
		},
		&testhelpers.MockAuditLogService{},
	)

	req := &CreateEntityRequestRequest{}
	req.Body.EntityType = "artist"
	req.Body.Payload = artistPayload(t, "Boris")
	_, err := h.CreateEntityRequestHandler(erAdminCtx(), req)
	testhelpers.AssertHumaError(t, err, 409)
}

// source_detail is normalized (trimmed, empties dropped) and marshalled through
// to the service so the admin queue can show the AI source context.
func TestCreateEntityRequest_SourceDetailPassthrough(t *testing.T) {
	want := pendingRequest(14, "artist")
	var gotDetail []byte

	h := NewEntityRequestHandler(
		&testhelpers.MockEntityRequestService{
			CreateRequestFn: func(user *authm.User, entityType string, payload []byte, sourceContext string, sourceDetail []byte, confirmed bool) (*communitym.EntityRequest, error) {
				gotDetail = sourceDetail
				return want, nil
			},
		},
		nil,
		&testhelpers.MockAuditLogService{},
	)

	url := "  https://example.com/show  "
	excerpt := "  Boris at The Venue  "
	req := &CreateEntityRequestRequest{}
	req.Body.EntityType = "artist"
	req.Body.Payload = artistPayload(t, "Boris")
	req.Body.SourceContext = communitym.EntityRequestSourceAIExtraction
	req.Body.SourceDetail = &communitym.EntityRequestSourceDetail{URL: &url, Excerpt: &excerpt}

	if _, err := h.CreateEntityRequestHandler(erUserCtx(), req); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotDetail == nil {
		t.Fatal("expected source_detail to be passed to the service")
	}
	var sd communitym.EntityRequestSourceDetail
	if err := json.Unmarshal(gotDetail, &sd); err != nil {
		t.Fatalf("source_detail is not valid json: %v", err)
	}
	if sd.URL == nil || *sd.URL != "https://example.com/show" {
		t.Errorf("expected trimmed url, got %v", sd.URL)
	}
	if sd.Excerpt == nil || *sd.Excerpt != "Boris at The Venue" {
		t.Errorf("expected trimmed excerpt, got %v", sd.Excerpt)
	}
}

// An empty (all-blank) source_detail is normalized to nil so the row stores
// NULL, not an empty object.
func TestCreateEntityRequest_EmptySourceDetailDropped(t *testing.T) {
	want := pendingRequest(15, "artist")
	sawDetail := true

	h := NewEntityRequestHandler(
		&testhelpers.MockEntityRequestService{
			CreateRequestFn: func(user *authm.User, entityType string, payload []byte, sourceContext string, sourceDetail []byte, confirmed bool) (*communitym.EntityRequest, error) {
				sawDetail = sourceDetail != nil
				return want, nil
			},
		},
		nil,
		&testhelpers.MockAuditLogService{},
	)

	blank := "   "
	req := &CreateEntityRequestRequest{}
	req.Body.EntityType = "artist"
	req.Body.Payload = artistPayload(t, "Boris")
	req.Body.SourceDetail = &communitym.EntityRequestSourceDetail{URL: &blank}

	if _, err := h.CreateEntityRequestHandler(erUserCtx(), req); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sawDetail {
		t.Error("expected an all-blank source_detail to normalize to nil")
	}
}

// An over-long source_detail excerpt is rejected at the trust boundary (422)
// before the service is called.
func TestCreateEntityRequest_SourceDetailTooLong(t *testing.T) {
	h := NewEntityRequestHandler(
		&testhelpers.MockEntityRequestService{
			CreateRequestFn: func(user *authm.User, entityType string, payload []byte, sourceContext string, sourceDetail []byte, confirmed bool) (*communitym.EntityRequest, error) {
				t.Fatal("service must NOT be called when source_detail is invalid")
				return nil, nil
			},
		},
		nil, nil,
	)

	huge := strings.Repeat("x", maxSourceExcerptLen+1)
	req := &CreateEntityRequestRequest{}
	req.Body.EntityType = "artist"
	req.Body.Payload = artistPayload(t, "Boris")
	req.Body.SourceDetail = &communitym.EntityRequestSourceDetail{Excerpt: &huge}
	_, err := h.CreateEntityRequestHandler(erUserCtx(), req)
	testhelpers.AssertHumaError(t, err, 422)
}

func TestCreateEntityRequest_ServiceError(t *testing.T) {
	h := NewEntityRequestHandler(
		&testhelpers.MockEntityRequestService{
			CreateRequestFn: func(user *authm.User, entityType string, payload []byte, sourceContext string, sourceDetail []byte, confirmed bool) (*communitym.EntityRequest, error) {
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
			CreateRequestFn: func(user *authm.User, entityType string, payload []byte, sourceContext string, sourceDetail []byte, confirmed bool) (*communitym.EntityRequest, error) {
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

// The admin list resolves the requester's display name/username (the raw model
// serializes Requester as json:"-") and carries the payload so the moderation
// card can attribute + preview each request. PSY-871.
func TestAdminListEntityRequests_ResolvesRequesterAndPayload(t *testing.T) {
	uname := "alice"
	row := pendingRequest(3, "artist")
	row.RequesterID = 42
	row.Requester = authm.User{ID: 42, Username: &uname}

	h := NewEntityRequestHandler(
		&testhelpers.MockEntityRequestService{
			ListRequestsFn: func(f *contracts.EntityRequestFilters) ([]communitym.EntityRequest, int64, error) {
				return []communitym.EntityRequest{*row}, 1, nil
			},
		},
		nil, nil,
	)

	resp, err := h.AdminListEntityRequestsHandler(erAdminCtx(), &AdminListEntityRequestsRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Body.Requests) != 1 {
		t.Fatalf("expected 1 request, got %d", len(resp.Body.Requests))
	}
	v := resp.Body.Requests[0]
	if v.RequesterID != 42 {
		t.Errorf("expected requester_id 42, got %d", v.RequesterID)
	}
	if v.RequesterName != "alice" {
		t.Errorf("expected requester_name alice, got %q", v.RequesterName)
	}
	if v.RequesterUsername == nil || *v.RequesterUsername != "alice" {
		t.Errorf("expected requester_username alice, got %v", v.RequesterUsername)
	}
	if v.EntityType != "artist" || v.Payload == nil {
		t.Errorf("expected entity_type artist + non-nil payload, got %s payload=%v", v.EntityType, v.Payload)
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

// Approving a festival creates a real festival: the fulfiller derives the two
// fields the payload doesn't carry — series_slug (slugified name) and
// edition_year — then calls CreateFestival. PSY-998.
func TestAdminDecide_ApproveFestival_CreatesEntity(t *testing.T) {
	festPayload, err := communitym.MarshalPayload(communitym.FestivalRequestPayload{
		Name:        "Best Coast Fest",
		EditionYear: 2027,
		StartDate:   "2027-05-01",
		EndDate:     "2027-05-03",
	})
	if err != nil {
		t.Fatalf("marshal festival payload: %v", err)
	}
	decided := pendingRequest(20, "festival")
	decided.Payload = &festPayload
	decided.DecisionState = communitym.EntityRequestStateApproved
	createCalled := false

	h := NewEntityRequestHandler(
		&testhelpers.MockEntityRequestService{
			DecideFn: func(requestID, adminID uint, newState communitym.EntityRequestDecisionState, note *string) (*communitym.EntityRequest, error) {
				return decided, nil
			},
		},
		&testhelpers.MockEntityRequestFulfiller{
			CreateFestivalFn: func(req *contracts.CreateFestivalRequest) (*contracts.FestivalDetailResponse, error) {
				createCalled = true
				if req.Name != "Best Coast Fest" {
					t.Errorf("expected name from payload, got %q", req.Name)
				}
				if req.SeriesSlug != "best-coast-fest" {
					t.Errorf("expected series_slug derived from name, got %q", req.SeriesSlug)
				}
				if req.EditionYear != 2027 {
					t.Errorf("expected edition_year 2027 from payload, got %d", req.EditionYear)
				}
				return &contracts.FestivalDetailResponse{ID: 77}, nil
			},
		},
		&testhelpers.MockAuditLogService{},
	)

	req := &AdminDecideEntityRequestRequest{ID: "20"}
	req.Body.Decision = "approved"

	resp, err := h.AdminDecideEntityRequestHandler(erAdminCtx(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !createCalled {
		t.Error("expected fulfiller CreateFestival to be called on approve")
	}
	if resp.Body.CreatedEntityID == nil || *resp.Body.CreatedEntityID != 77 {
		t.Errorf("expected created entity id 77, got %v", resp.Body.CreatedEntityID)
	}
	if resp.Body.CreatedEntityType == nil || *resp.Body.CreatedEntityType != "festival" {
		t.Errorf("expected created entity type festival, got %v", resp.Body.CreatedEntityType)
	}
}

// When the payload omits edition_year, the fulfiller falls back to the
// start_date's calendar year so the festival edition stays meaningful. PSY-998.
func TestAdminDecide_ApproveFestival_EditionYearFromStartDate(t *testing.T) {
	festPayload, err := communitym.MarshalPayload(communitym.FestivalRequestPayload{
		Name:      "Yearless Fest",
		StartDate: "2028-04-10",
		EndDate:   "2028-04-12",
	})
	if err != nil {
		t.Fatalf("marshal festival payload: %v", err)
	}
	decided := pendingRequest(21, "festival")
	decided.Payload = &festPayload
	decided.DecisionState = communitym.EntityRequestStateApproved

	gotYear := -1
	h := NewEntityRequestHandler(
		&testhelpers.MockEntityRequestService{
			DecideFn: func(requestID, adminID uint, newState communitym.EntityRequestDecisionState, note *string) (*communitym.EntityRequest, error) {
				return decided, nil
			},
		},
		&testhelpers.MockEntityRequestFulfiller{
			CreateFestivalFn: func(req *contracts.CreateFestivalRequest) (*contracts.FestivalDetailResponse, error) {
				gotYear = req.EditionYear
				return &contracts.FestivalDetailResponse{ID: 78}, nil
			},
		},
		&testhelpers.MockAuditLogService{},
	)

	req := &AdminDecideEntityRequestRequest{ID: "21"}
	req.Body.Decision = "approved"
	if _, err := h.AdminDecideEntityRequestHandler(erAdminCtx(), req); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotYear != 2028 {
		t.Errorf("expected edition_year derived from start_date (2028), got %d", gotYear)
	}
}

// A duplicate-edition conflict on festival approve maps to 409, not 500
// (FESTIVAL_EXISTS → 409; mirrors the artist regression guard). PSY-998.
func TestAdminDecide_ApproveFestival_ExistsConflictIs409(t *testing.T) {
	festPayload, err := communitym.MarshalPayload(communitym.FestivalRequestPayload{
		Name:        "Dup Fest",
		EditionYear: 2026,
		StartDate:   "2026-05-01",
		EndDate:     "2026-05-03",
	})
	if err != nil {
		t.Fatalf("marshal festival payload: %v", err)
	}
	decided := pendingRequest(22, "festival")
	decided.Payload = &festPayload
	decided.DecisionState = communitym.EntityRequestStateApproved

	h := NewEntityRequestHandler(
		&testhelpers.MockEntityRequestService{
			DecideFn: func(requestID, adminID uint, newState communitym.EntityRequestDecisionState, note *string) (*communitym.EntityRequest, error) {
				return decided, nil
			},
		},
		&testhelpers.MockEntityRequestFulfiller{
			CreateFestivalFn: func(req *contracts.CreateFestivalRequest) (*contracts.FestivalDetailResponse, error) {
				return nil, apperrors.ErrFestivalExists("Dup Fest")
			},
		},
		&testhelpers.MockAuditLogService{},
	)

	req := &AdminDecideEntityRequestRequest{ID: "22"}
	req.Body.Decision = "approved"
	_, err = h.AdminDecideEntityRequestHandler(erAdminCtx(), req)
	testhelpers.AssertHumaError(t, err, 409)
}

// A festival request with a malformed start_date that predates the create-time
// date validation must be re-validated and rejected cleanly at the fulfill
// boundary — a typed 500 with a generic message — instead of reaching INSERT
// and leaking a raw DB date-parse error. The key guard is that CreateFestival
// is never called (no DB round-trip, no leak, no corrupt festival). PSY-998.
func TestAdminDecide_ApproveFestival_MalformedStoredDateRejected(t *testing.T) {
	// Build the row directly (bypassing create-time validation, as a legacy
	// pre-PSY-998 queued row would).
	raw := json.RawMessage(`{"name":"Stale Fest","start_date":"next summer","end_date":"2026-01-03"}`)
	decided := pendingRequest(23, "festival")
	decided.Payload = &raw
	decided.DecisionState = communitym.EntityRequestStateApproved

	createCalled := false
	h := NewEntityRequestHandler(
		&testhelpers.MockEntityRequestService{
			DecideFn: func(requestID, adminID uint, newState communitym.EntityRequestDecisionState, note *string) (*communitym.EntityRequest, error) {
				return decided, nil
			},
		},
		&testhelpers.MockEntityRequestFulfiller{
			CreateFestivalFn: func(req *contracts.CreateFestivalRequest) (*contracts.FestivalDetailResponse, error) {
				createCalled = true
				return &contracts.FestivalDetailResponse{ID: 99}, nil
			},
		},
		&testhelpers.MockAuditLogService{},
	)

	req := &AdminDecideEntityRequestRequest{ID: "23"}
	req.Body.Decision = "approved"
	_, err := h.AdminDecideEntityRequestHandler(erAdminCtx(), req)
	// Stored-payload corruption is a server-side data fault → typed 500 (clean
	// message, no raw DB error), per MapEntityRequestError.
	testhelpers.AssertHumaError(t, err, 500)
	if createCalled {
		t.Error("CreateFestival must NOT be called when the stored date is malformed")
	}
}

// A festival request carrying an explicit non-positive edition_year must be
// rejected at the fulfill boundary rather than persisting a year-0/negative
// edition. PSY-998.
func TestAdminDecide_ApproveFestival_NonPositiveYearRejected(t *testing.T) {
	festPayload, err := communitym.MarshalPayload(communitym.FestivalRequestPayload{
		Name:        "Negative Fest",
		EditionYear: -5,
		StartDate:   "2026-05-01",
		EndDate:     "2026-05-03",
	})
	if err != nil {
		t.Fatalf("marshal festival payload: %v", err)
	}
	decided := pendingRequest(24, "festival")
	decided.Payload = &festPayload
	decided.DecisionState = communitym.EntityRequestStateApproved

	h := NewEntityRequestHandler(
		&testhelpers.MockEntityRequestService{
			DecideFn: func(requestID, adminID uint, newState communitym.EntityRequestDecisionState, note *string) (*communitym.EntityRequest, error) {
				return decided, nil
			},
		},
		&testhelpers.MockEntityRequestFulfiller{
			CreateFestivalFn: func(req *contracts.CreateFestivalRequest) (*contracts.FestivalDetailResponse, error) {
				t.Error("CreateFestival must NOT be called for a non-positive edition_year")
				return nil, nil
			},
		},
		&testhelpers.MockAuditLogService{},
	)

	req := &AdminDecideEntityRequestRequest{ID: "24"}
	req.Body.Decision = "approved"
	_, err = h.AdminDecideEntityRequestHandler(erAdminCtx(), req)
	testhelpers.AssertHumaError(t, err, 500)
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

// A catalog "already exists" conflict on admin approve maps to 409, not 500
// (regression guard for the adversarial finding).
func TestAdminDecide_ApproveExistsConflictIs409(t *testing.T) {
	decided := pendingRequest(18, "artist")
	decided.DecisionState = communitym.EntityRequestStateApproved

	h := NewEntityRequestHandler(
		&testhelpers.MockEntityRequestService{
			DecideFn: func(requestID, adminID uint, newState communitym.EntityRequestDecisionState, note *string) (*communitym.EntityRequest, error) {
				return decided, nil
			},
		},
		&testhelpers.MockEntityRequestFulfiller{
			CreateArtistFn: func(req *contracts.CreateArtistRequest) (*contracts.ArtistDetailResponse, error) {
				return nil, apperrors.ErrArtistExists("Boris")
			},
		},
		&testhelpers.MockAuditLogService{},
	)

	req := &AdminDecideEntityRequestRequest{ID: "18"}
	req.Body.Decision = "approved"
	_, err := h.AdminDecideEntityRequestHandler(erAdminCtx(), req)
	testhelpers.AssertHumaError(t, err, 409)
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
