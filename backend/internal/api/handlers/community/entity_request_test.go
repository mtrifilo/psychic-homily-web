package community

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

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

// PSY-1038: the fulfiller forwards the previously-dropped nullable payload
// fields (image_url + bandcamp_embed_url) onto the CreateArtist contract.
func TestAdminDecide_ApproveArtist_CarriesNullableFields(t *testing.T) {
	img := "https://example.com/boris.jpg"
	embed := "https://boris.bandcamp.com/album/pink"
	payload, err := communitym.MarshalPayload(communitym.ArtistRequestPayload{
		Name:             "Boris",
		ImageURL:         &img,
		BandcampEmbedURL: &embed,
	})
	if err != nil {
		t.Fatalf("marshal artist payload: %v", err)
	}
	decided := pendingRequest(30, "artist")
	decided.Payload = &payload
	decided.DecisionState = communitym.EntityRequestStateApproved

	var gotImage, gotEmbed *string
	h := NewEntityRequestHandler(
		&testhelpers.MockEntityRequestService{
			DecideFn: func(requestID, adminID uint, newState communitym.EntityRequestDecisionState, note *string) (*communitym.EntityRequest, error) {
				return decided, nil
			},
		},
		&testhelpers.MockEntityRequestFulfiller{
			CreateArtistFn: func(req *contracts.CreateArtistRequest) (*contracts.ArtistDetailResponse, error) {
				gotImage = req.ImageURL
				gotEmbed = req.BandcampEmbedURL
				return &contracts.ArtistDetailResponse{ID: 88}, nil
			},
		},
		&testhelpers.MockAuditLogService{},
	)

	req := &AdminDecideEntityRequestRequest{ID: "30"}
	req.Body.Decision = "approved"
	if _, err := h.AdminDecideEntityRequestHandler(erAdminCtx(), req); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotImage == nil || *gotImage != "https://example.com/boris.jpg" {
		t.Errorf("expected image_url forwarded, got %v", gotImage)
	}
	if gotEmbed == nil || *gotEmbed != "https://boris.bandcamp.com/album/pink" {
		t.Errorf("expected bandcamp_embed_url forwarded, got %v", gotEmbed)
	}
}

// PSY-1038: the venue fulfiller branch forwards description + image_url.
func TestAdminDecide_ApproveVenue_CarriesNullableFields(t *testing.T) {
	desc := "All-ages club."
	img := "https://example.com/v.jpg"
	payload, err := communitym.MarshalPayload(communitym.VenueRequestPayload{
		Name: "Rebel Lounge", City: "Phoenix", State: "AZ",
		Description: &desc, ImageURL: &img,
	})
	if err != nil {
		t.Fatalf("marshal venue payload: %v", err)
	}
	decided := pendingRequest(31, "venue")
	decided.Payload = &payload
	decided.DecisionState = communitym.EntityRequestStateApproved

	var gotDesc, gotImage *string
	h := NewEntityRequestHandler(
		&testhelpers.MockEntityRequestService{
			DecideFn: func(requestID, adminID uint, newState communitym.EntityRequestDecisionState, note *string) (*communitym.EntityRequest, error) {
				return decided, nil
			},
		},
		&testhelpers.MockEntityRequestFulfiller{
			CreateVenueFn: func(req *contracts.CreateVenueRequest, isAdmin bool) (*contracts.VenueDetailResponse, error) {
				gotDesc = req.Description
				gotImage = req.ImageURL
				return &contracts.VenueDetailResponse{ID: 71}, nil
			},
		},
		&testhelpers.MockAuditLogService{},
	)
	req := &AdminDecideEntityRequestRequest{ID: "31"}
	req.Body.Decision = "approved"
	if _, err := h.AdminDecideEntityRequestHandler(erAdminCtx(), req); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotDesc == nil || *gotDesc != "All-ages club." {
		t.Errorf("expected description forwarded, got %v", gotDesc)
	}
	if gotImage == nil || *gotImage != "https://example.com/v.jpg" {
		t.Errorf("expected image_url forwarded, got %v", gotImage)
	}
}

// PSY-1038: the label fulfiller branch forwards image_url.
func TestAdminDecide_ApproveLabel_CarriesNullableFields(t *testing.T) {
	img := "https://example.com/l.png"
	payload, err := communitym.MarshalPayload(communitym.LabelRequestPayload{Name: "Hydra Head", ImageURL: &img})
	if err != nil {
		t.Fatalf("marshal label payload: %v", err)
	}
	decided := pendingRequest(32, "label")
	decided.Payload = &payload
	decided.DecisionState = communitym.EntityRequestStateApproved

	var gotImage *string
	h := NewEntityRequestHandler(
		&testhelpers.MockEntityRequestService{
			DecideFn: func(requestID, adminID uint, newState communitym.EntityRequestDecisionState, note *string) (*communitym.EntityRequest, error) {
				return decided, nil
			},
		},
		&testhelpers.MockEntityRequestFulfiller{
			CreateLabelFn: func(req *contracts.CreateLabelRequest) (*contracts.LabelDetailResponse, error) {
				gotImage = req.ImageURL
				return &contracts.LabelDetailResponse{ID: 72}, nil
			},
		},
		&testhelpers.MockAuditLogService{},
	)
	req := &AdminDecideEntityRequestRequest{ID: "32"}
	req.Body.Decision = "approved"
	if _, err := h.AdminDecideEntityRequestHandler(erAdminCtx(), req); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotImage == nil || *gotImage != "https://example.com/l.png" {
		t.Errorf("expected image_url forwarded, got %v", gotImage)
	}
}

// PSY-1038 (adversarial): a stored artist request carrying a hostile-scheme
// image_url that predates the URL validation must be rejected at the fulfill
// boundary (re-validation), NOT mapped onto the created artist. CreateArtist
// must never be called.
func TestAdminDecide_ApproveArtist_RejectsHostileStoredURL(t *testing.T) {
	// Built directly (bypassing create-time validation) as a pre-PSY-1038 row.
	raw := json.RawMessage(`{"name":"Evil","image_url":"javascript:alert(document.cookie)"}`)
	decided := pendingRequest(33, "artist")
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
			CreateArtistFn: func(req *contracts.CreateArtistRequest) (*contracts.ArtistDetailResponse, error) {
				createCalled = true
				return &contracts.ArtistDetailResponse{ID: 99}, nil
			},
		},
		&testhelpers.MockAuditLogService{},
	)
	req := &AdminDecideEntityRequestRequest{ID: "33"}
	req.Body.Decision = "approved"
	_, err := h.AdminDecideEntityRequestHandler(erAdminCtx(), req)
	testhelpers.AssertHumaError(t, err, 500) // stored-payload-invalid → typed 500
	if createCalled {
		t.Error("CreateArtist must NOT be called for a hostile stored image_url")
	}
}

// Approving a show WITHOUT admin-supplied associations is rejected with a 422
// BEFORE the row is claimed (PSY-1037): Decide only re-processes pending rows,
// so a post-claim failure would orphan the request as approved-but-unfulfilled.
func TestAdminDecide_ApproveShow_Unsupported(t *testing.T) {
	pending := pendingRequest(6, "show")

	decideCalled := false
	h := NewEntityRequestHandler(
		&testhelpers.MockEntityRequestService{
			GetRequestFn: func(requestID uint) (*communitym.EntityRequest, error) {
				return pending, nil
			},
			DecideFn: func(requestID, adminID uint, newState communitym.EntityRequestDecisionState, note *string) (*communitym.EntityRequest, error) {
				decideCalled = true
				return pending, nil
			},
		},
		&testhelpers.MockEntityRequestFulfiller{},
		&testhelpers.MockAuditLogService{},
	)

	req := &AdminDecideEntityRequestRequest{ID: "6"}
	req.Body.Decision = "approved"
	_, err := h.AdminDecideEntityRequestHandler(erAdminCtx(), req)
	testhelpers.AssertHumaError(t, err, 422)
	if decideCalled {
		t.Error("Decide must NOT be called when a show is approved without associations (pre-claim guard)")
	}
}

// PSY-1037: approving a show WITH admin-supplied associations creates a real
// show — payload metadata + associations map onto CreateShowRequest, the
// requester keeps attribution, and the admin approval makes it land approved.
func TestAdminDecide_ApproveShow_WithAssociations_CreatesShow(t *testing.T) {
	img := "https://example.com/flyer.jpg"
	city := "Phoenix"
	state := "AZ"
	payload, err := communitym.MarshalPayload(communitym.ShowRequestPayload{
		Title:     "Boris with Earth",
		EventDate: "2026-07-04T21:30:00-07:00",
		City:      &city,
		State:     &state,
		ImageURL:  &img,
	})
	if err != nil {
		t.Fatalf("marshal show payload: %v", err)
	}
	decided := pendingRequest(40, "show")
	decided.Payload = &payload
	decided.RequesterID = 7
	decided.DecisionState = communitym.EntityRequestStateApproved

	var got *contracts.CreateShowRequest
	h := NewEntityRequestHandler(
		&testhelpers.MockEntityRequestService{
			DecideFn: func(requestID, adminID uint, newState communitym.EntityRequestDecisionState, note *string) (*communitym.EntityRequest, error) {
				return decided, nil
			},
		},
		&testhelpers.MockEntityRequestFulfiller{
			CreateShowFn: func(req *contracts.CreateShowRequest) (*contracts.ShowResponse, error) {
				got = req
				return &contracts.ShowResponse{ID: 123}, nil
			},
		},
		&testhelpers.MockAuditLogService{},
	)

	req := &AdminDecideEntityRequestRequest{ID: "40"}
	req.Body.Decision = "approved"
	req.Body.ShowVenue = &ShowVenueInput{Name: "Valley Bar", City: "Phoenix", State: "AZ"}
	headliner := true
	req.Body.ShowArtists = []ShowArtistInput{
		{Name: "Boris", IsHeadliner: &headliner},
		{Name: "Earth"},
	}

	resp, err := h.AdminDecideEntityRequestHandler(erAdminCtx(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil {
		t.Fatal("expected fulfiller CreateShow to be called")
	}
	if got.Title != "Boris with Earth" {
		t.Errorf("title: got %q", got.Title)
	}
	// RFC3339 event_date is taken as-is (compare in UTC).
	wantDate := time.Date(2026, 7, 5, 4, 30, 0, 0, time.UTC)
	if !got.EventDate.UTC().Equal(wantDate) {
		t.Errorf("event_date: got %s, want %s", got.EventDate.UTC(), wantDate)
	}
	if len(got.Venues) != 1 || got.Venues[0].Name != "Valley Bar" || got.Venues[0].City != "Phoenix" {
		t.Errorf("venues: got %+v", got.Venues)
	}
	if len(got.Artists) != 2 || got.Artists[0].Name != "Boris" || got.Artists[1].Name != "Earth" {
		t.Errorf("artists: got %+v", got.Artists)
	}
	if got.Artists[0].IsHeadliner == nil || !*got.Artists[0].IsHeadliner {
		t.Error("expected first artist marked headliner")
	}
	if got.ImageURL == nil || *got.ImageURL != img {
		t.Errorf("image_url: got %v", got.ImageURL)
	}
	if got.SubmittedByUserID == nil || *got.SubmittedByUserID != 7 {
		t.Errorf("expected requester attribution (7), got %v", got.SubmittedByUserID)
	}
	if !got.SubmitterIsAdmin {
		t.Error("expected SubmitterIsAdmin=true on admin approval")
	}
	if resp.Body.CreatedEntityID == nil || *resp.Body.CreatedEntityID != 123 {
		t.Errorf("expected created entity id 123, got %v", resp.Body.CreatedEntityID)
	}
	if resp.Body.CreatedEntityType == nil || *resp.Body.CreatedEntityType != "show" {
		t.Errorf("expected created entity type show, got %v", resp.Body.CreatedEntityType)
	}
}

// PSY-1037: a date-only event_date anchors at 20:00 in the state's assumed
// zone (AZ → America/Phoenix, UTC-7 year-round → 03:00 next day UTC).
func TestAdminDecide_ApproveShow_DateOnlyAnchorsEveningLocal(t *testing.T) {
	state := "AZ"
	payload, err := communitym.MarshalPayload(communitym.ShowRequestPayload{
		Title:     "Date Only Fest",
		EventDate: "2026-07-04",
		State:     &state,
	})
	if err != nil {
		t.Fatalf("marshal show payload: %v", err)
	}
	decided := pendingRequest(41, "show")
	decided.Payload = &payload
	decided.DecisionState = communitym.EntityRequestStateApproved

	var got *contracts.CreateShowRequest
	h := NewEntityRequestHandler(
		&testhelpers.MockEntityRequestService{
			DecideFn: func(requestID, adminID uint, newState communitym.EntityRequestDecisionState, note *string) (*communitym.EntityRequest, error) {
				return decided, nil
			},
		},
		&testhelpers.MockEntityRequestFulfiller{
			CreateShowFn: func(req *contracts.CreateShowRequest) (*contracts.ShowResponse, error) {
				got = req
				return &contracts.ShowResponse{ID: 124}, nil
			},
		},
		&testhelpers.MockAuditLogService{},
	)

	req := &AdminDecideEntityRequestRequest{ID: "41"}
	req.Body.Decision = "approved"
	req.Body.ShowVenue = &ShowVenueInput{Name: "Valley Bar", City: "Phoenix", State: "AZ"}
	req.Body.ShowArtists = []ShowArtistInput{{Name: "Boris"}}

	if _, err := h.AdminDecideEntityRequestHandler(erAdminCtx(), req); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil {
		t.Fatal("expected CreateShow to be called")
	}
	want := time.Date(2026, 7, 5, 3, 0, 0, 0, time.UTC) // 20:00 AZ = 03:00+1d UTC
	if !got.EventDate.UTC().Equal(want) {
		t.Errorf("event_date: got %s, want %s", got.EventDate.UTC(), want)
	}
}

// PSY-1037: malformed association input is a 422 BEFORE the row is claimed —
// Decide must not run, so no approved-but-unfulfilled row is left behind.
func TestAdminDecide_ApproveShow_PartialAssociations422BeforeClaim(t *testing.T) {
	decideCalled := false
	h := NewEntityRequestHandler(
		&testhelpers.MockEntityRequestService{
			DecideFn: func(requestID, adminID uint, newState communitym.EntityRequestDecisionState, note *string) (*communitym.EntityRequest, error) {
				decideCalled = true
				return nil, nil
			},
		},
		&testhelpers.MockEntityRequestFulfiller{},
		&testhelpers.MockAuditLogService{},
	)

	req := &AdminDecideEntityRequestRequest{ID: "42"}
	req.Body.Decision = "approved"
	req.Body.ShowVenue = &ShowVenueInput{Name: "Valley Bar", City: "Phoenix", State: "AZ"}
	// show_artists missing → 422 before claim.
	_, err := h.AdminDecideEntityRequestHandler(erAdminCtx(), req)
	testhelpers.AssertHumaError(t, err, 422)
	if decideCalled {
		t.Error("Decide must NOT be called when association input is malformed")
	}
}

// PSY-1037: a legacy malformed stored show payload is rejected by the fulfill
// re-validation when associations are supplied — CreateShow is never called.
func TestAdminDecide_ApproveShow_MalformedStoredPayloadRejected(t *testing.T) {
	raw := json.RawMessage(`{"title":"Bad Date Fest","event_date":"next summer"}`)
	decided := pendingRequest(43, "show")
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
			CreateShowFn: func(req *contracts.CreateShowRequest) (*contracts.ShowResponse, error) {
				createCalled = true
				return &contracts.ShowResponse{ID: 1}, nil
			},
		},
		&testhelpers.MockAuditLogService{},
	)

	req := &AdminDecideEntityRequestRequest{ID: "43"}
	req.Body.Decision = "approved"
	req.Body.ShowVenue = &ShowVenueInput{Name: "Valley Bar", City: "Phoenix", State: "AZ"}
	req.Body.ShowArtists = []ShowArtistInput{{Name: "Boris"}}
	_, err := h.AdminDecideEntityRequestHandler(erAdminCtx(), req)
	// Stored-payload corruption → typed 500 (clean message), per MapEntityRequestError.
	testhelpers.AssertHumaError(t, err, 500)
	if createCalled {
		t.Error("CreateShow must NOT be called for a malformed stored payload")
	}
}

// PSY-1037: a CreateShow failure (e.g. duplicate headliner at the same
// venue/date) maps to 422 SHOW_CREATE_FAILED, matching the direct handler.
func TestAdminDecide_ApproveShow_CreateFailureIs422(t *testing.T) {
	payload, err := communitym.MarshalPayload(communitym.ShowRequestPayload{
		Title:     "Dup Fest",
		EventDate: "2026-07-04",
	})
	if err != nil {
		t.Fatalf("marshal show payload: %v", err)
	}
	decided := pendingRequest(44, "show")
	decided.Payload = &payload
	decided.DecisionState = communitym.EntityRequestStateApproved

	h := NewEntityRequestHandler(
		&testhelpers.MockEntityRequestService{
			DecideFn: func(requestID, adminID uint, newState communitym.EntityRequestDecisionState, note *string) (*communitym.EntityRequest, error) {
				return decided, nil
			},
		},
		&testhelpers.MockEntityRequestFulfiller{
			CreateShowFn: func(req *contracts.CreateShowRequest) (*contracts.ShowResponse, error) {
				return nil, fmt.Errorf("duplicate headliner at venue on date")
			},
		},
		&testhelpers.MockAuditLogService{},
	)

	req := &AdminDecideEntityRequestRequest{ID: "44"}
	req.Body.Decision = "approved"
	req.Body.ShowVenue = &ShowVenueInput{Name: "Valley Bar", City: "Phoenix", State: "AZ"}
	req.Body.ShowArtists = []ShowArtistInput{{Name: "Boris"}}
	_, err = h.AdminDecideEntityRequestHandler(erAdminCtx(), req)
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

// ============================================================================
// Tests: Admin rescue — POST /admin/entity-requests/{id}/fulfill (PSY-1088)
// ============================================================================

// approvedUnfulfilledRequest is the rescue-target shape: approved, no
// created_entity_id, owned by requester 7 (so attribution can be asserted).
func approvedUnfulfilledRequest(id uint, entityType string) *communitym.EntityRequest {
	r := approvedRequest(id, entityType)
	r.RequesterID = 7
	r.CreatedEntityID = nil
	return r
}

// Fulfilling a non-show approved-but-unfulfilled artist row creates the entity
// and atomically claims the link.
func TestAdminFulfill_Artist_CreatesAndClaims(t *testing.T) {
	orphan := approvedUnfulfilledRequest(10, "artist")
	createCalled := false
	var claimedID uint

	h := NewEntityRequestHandler(
		&testhelpers.MockEntityRequestService{
			GetRequestFn: func(requestID uint) (*communitym.EntityRequest, error) {
				if requestID != 10 {
					t.Errorf("unexpected request id %d", requestID)
				}
				return orphan, nil
			},
			ClaimRescueFulfillmentFn: func(requestID, createdEntityID uint) (bool, error) {
				claimedID = createdEntityID
				return true, nil
			},
		},
		&testhelpers.MockEntityRequestFulfiller{
			CreateArtistFn: func(req *contracts.CreateArtistRequest) (*contracts.ArtistDetailResponse, error) {
				createCalled = true
				return &contracts.ArtistDetailResponse{ID: 55}, nil
			},
		},
		&testhelpers.MockAuditLogService{},
	)

	req := &AdminFulfillEntityRequestRequest{ID: "10"}
	resp, err := h.AdminFulfillEntityRequestHandler(erAdminCtx(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !createCalled {
		t.Error("expected CreateArtist to be called")
	}
	if claimedID != 55 {
		t.Errorf("expected claim of id 55, got %d", claimedID)
	}
	if resp.Body.CreatedEntityID == nil || *resp.Body.CreatedEntityID != 55 {
		t.Errorf("expected created_entity_id 55, got %v", resp.Body.CreatedEntityID)
	}
	if resp.Body.CreatedEntityType == nil || *resp.Body.CreatedEntityType != "artist" {
		t.Errorf("expected type artist, got %v", resp.Body.CreatedEntityType)
	}
}

// Fulfilling a SHOW with admin-supplied associations creates the show; the
// requester keeps attribution.
func TestAdminFulfill_Show_WithAssociations_CreatesShow(t *testing.T) {
	city := "Phoenix"
	state := "AZ"
	payload, err := communitym.MarshalPayload(communitym.ShowRequestPayload{
		Title:     "Deferred Show",
		EventDate: "2026-08-01T21:00:00-07:00",
		City:      &city,
		State:     &state,
	})
	if err != nil {
		t.Fatalf("marshal show payload: %v", err)
	}
	orphan := approvedUnfulfilledRequest(11, "show")
	orphan.Payload = &payload

	var got *contracts.CreateShowRequest
	h := NewEntityRequestHandler(
		&testhelpers.MockEntityRequestService{
			GetRequestFn: func(requestID uint) (*communitym.EntityRequest, error) { return orphan, nil },
			ClaimRescueFulfillmentFn: func(requestID, createdEntityID uint) (bool, error) {
				return true, nil
			},
		},
		&testhelpers.MockEntityRequestFulfiller{
			CreateShowFn: func(req *contracts.CreateShowRequest) (*contracts.ShowResponse, error) {
				got = req
				return &contracts.ShowResponse{ID: 200}, nil
			},
		},
		&testhelpers.MockAuditLogService{},
	)

	req := &AdminFulfillEntityRequestRequest{ID: "11"}
	req.Body.ShowVenue = &ShowVenueInput{Name: "Valley Bar", City: "Phoenix", State: "AZ"}
	req.Body.ShowArtists = []ShowArtistInput{{Name: "Boris"}}

	resp, err := h.AdminFulfillEntityRequestHandler(erAdminCtx(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil {
		t.Fatal("expected CreateShow to be called")
	}
	if len(got.Venues) != 1 || got.Venues[0].Name != "Valley Bar" {
		t.Errorf("venues: got %+v", got.Venues)
	}
	if len(got.Artists) != 1 || got.Artists[0].Name != "Boris" {
		t.Errorf("artists: got %+v", got.Artists)
	}
	if got.SubmittedByUserID == nil || *got.SubmittedByUserID != 7 {
		t.Errorf("expected requester attribution (7), got %v", got.SubmittedByUserID)
	}
	if resp.Body.CreatedEntityID == nil || *resp.Body.CreatedEntityID != 200 {
		t.Errorf("expected created_entity_id 200, got %v", resp.Body.CreatedEntityID)
	}
}

// Fulfilling a SHOW WITHOUT associations is a clean 422 (the payload alone can't
// be fulfilled), and no catalog create or claim is attempted.
func TestAdminFulfill_Show_MissingAssociations_422(t *testing.T) {
	orphan := approvedUnfulfilledRequest(12, "show")
	createCalled := false
	claimCalled := false

	h := NewEntityRequestHandler(
		&testhelpers.MockEntityRequestService{
			GetRequestFn: func(requestID uint) (*communitym.EntityRequest, error) { return orphan, nil },
			ClaimRescueFulfillmentFn: func(requestID, createdEntityID uint) (bool, error) {
				claimCalled = true
				return true, nil
			},
		},
		&testhelpers.MockEntityRequestFulfiller{
			CreateShowFn: func(req *contracts.CreateShowRequest) (*contracts.ShowResponse, error) {
				createCalled = true
				return &contracts.ShowResponse{ID: 1}, nil
			},
		},
		&testhelpers.MockAuditLogService{},
	)

	req := &AdminFulfillEntityRequestRequest{ID: "12"}
	_, err := h.AdminFulfillEntityRequestHandler(erAdminCtx(), req)
	testhelpers.AssertHumaError(t, err, 422)
	if createCalled || claimCalled {
		t.Error("no create or claim should happen when show associations are missing")
	}
}

// Adversarial (mirrors TestAdminDecide_ApproveArtist_RejectsHostileStoredURL):
// an orphan carrying a hostile-scheme image_url that predates URL validation
// must be rejected at the fulfill boundary (fulfillEntity re-validates the
// stored payload), NOT mapped onto the created artist. CreateArtist + claim
// must never fire. Locks the security property the rescue path inherits from
// the shared fulfillEntity dispatcher.
func TestAdminFulfill_RejectsHostileStoredURL(t *testing.T) {
	raw := json.RawMessage(`{"name":"Evil","image_url":"javascript:alert(document.cookie)"}`)
	orphan := approvedUnfulfilledRequest(19, "artist")
	orphan.Payload = &raw
	createCalled := false
	claimCalled := false

	h := NewEntityRequestHandler(
		&testhelpers.MockEntityRequestService{
			GetRequestFn: func(requestID uint) (*communitym.EntityRequest, error) { return orphan, nil },
			ClaimRescueFulfillmentFn: func(requestID, createdEntityID uint) (bool, error) {
				claimCalled = true
				return true, nil
			},
		},
		&testhelpers.MockEntityRequestFulfiller{
			CreateArtistFn: func(req *contracts.CreateArtistRequest) (*contracts.ArtistDetailResponse, error) {
				createCalled = true
				return &contracts.ArtistDetailResponse{ID: 1}, nil
			},
		},
		&testhelpers.MockAuditLogService{},
	)

	req := &AdminFulfillEntityRequestRequest{ID: "19"}
	_, err := h.AdminFulfillEntityRequestHandler(erAdminCtx(), req)
	testhelpers.AssertHumaError(t, err, 500) // stored-payload-invalid → typed 500
	if createCalled {
		t.Error("CreateArtist must NOT be called for a hostile stored image_url")
	}
	if claimCalled {
		t.Error("claim must NOT fire when fulfillment is rejected")
	}
}

// Fulfilling a row that is NOT approved-but-unfulfilled (already fulfilled) →
// 409, no catalog create.
func TestAdminFulfill_AlreadyFulfilled_409(t *testing.T) {
	orphan := approvedUnfulfilledRequest(13, "artist")
	already := uint(99)
	orphan.CreatedEntityID = &already
	createCalled := false

	h := NewEntityRequestHandler(
		&testhelpers.MockEntityRequestService{
			GetRequestFn: func(requestID uint) (*communitym.EntityRequest, error) { return orphan, nil },
		},
		&testhelpers.MockEntityRequestFulfiller{
			CreateArtistFn: func(req *contracts.CreateArtistRequest) (*contracts.ArtistDetailResponse, error) {
				createCalled = true
				return &contracts.ArtistDetailResponse{ID: 1}, nil
			},
		},
		&testhelpers.MockAuditLogService{},
	)

	req := &AdminFulfillEntityRequestRequest{ID: "13"}
	_, err := h.AdminFulfillEntityRequestHandler(erAdminCtx(), req)
	testhelpers.AssertHumaError(t, err, 409)
	if createCalled {
		t.Error("CreateArtist must NOT be called for an already-fulfilled row")
	}
}

// Fulfilling a PENDING row → 409 (only approved-but-unfulfilled rows rescuable).
func TestAdminFulfill_Pending_409(t *testing.T) {
	pending := pendingRequest(14, "artist")
	h := NewEntityRequestHandler(
		&testhelpers.MockEntityRequestService{
			GetRequestFn: func(requestID uint) (*communitym.EntityRequest, error) { return pending, nil },
		},
		&testhelpers.MockEntityRequestFulfiller{},
		&testhelpers.MockAuditLogService{},
	)
	req := &AdminFulfillEntityRequestRequest{ID: "14"}
	_, err := h.AdminFulfillEntityRequestHandler(erAdminCtx(), req)
	testhelpers.AssertHumaError(t, err, 409)
}

// Fulfilling a missing row → 404.
func TestAdminFulfill_NotFound_404(t *testing.T) {
	h := NewEntityRequestHandler(
		&testhelpers.MockEntityRequestService{
			GetRequestFn: func(requestID uint) (*communitym.EntityRequest, error) { return nil, nil },
		},
		&testhelpers.MockEntityRequestFulfiller{},
		&testhelpers.MockAuditLogService{},
	)
	req := &AdminFulfillEntityRequestRequest{ID: "999"}
	_, err := h.AdminFulfillEntityRequestHandler(erAdminCtx(), req)
	testhelpers.AssertHumaError(t, err, 404)
}

// Losing the atomic claim race (entity created, but a concurrent rescue already
// won) → 409 so the admin reconciles the stray, not a silent 200.
func TestAdminFulfill_ClaimRace_409(t *testing.T) {
	orphan := approvedUnfulfilledRequest(15, "artist")
	h := NewEntityRequestHandler(
		&testhelpers.MockEntityRequestService{
			GetRequestFn: func(requestID uint) (*communitym.EntityRequest, error) { return orphan, nil },
			ClaimRescueFulfillmentFn: func(requestID, createdEntityID uint) (bool, error) {
				return false, nil // lost the race
			},
		},
		&testhelpers.MockEntityRequestFulfiller{
			CreateArtistFn: func(req *contracts.CreateArtistRequest) (*contracts.ArtistDetailResponse, error) {
				return &contracts.ArtistDetailResponse{ID: 70}, nil
			},
		},
		&testhelpers.MockAuditLogService{},
	)
	req := &AdminFulfillEntityRequestRequest{ID: "15"}
	_, err := h.AdminFulfillEntityRequestHandler(erAdminCtx(), req)
	testhelpers.AssertHumaError(t, err, 409)
}

// A benign catalog conflict on fulfill (e.g. ArtistExists) maps to 409, not 500.
func TestAdminFulfill_CatalogConflict_409(t *testing.T) {
	orphan := approvedUnfulfilledRequest(16, "artist")
	h := NewEntityRequestHandler(
		&testhelpers.MockEntityRequestService{
			GetRequestFn: func(requestID uint) (*communitym.EntityRequest, error) { return orphan, nil },
		},
		&testhelpers.MockEntityRequestFulfiller{
			CreateArtistFn: func(req *contracts.CreateArtistRequest) (*contracts.ArtistDetailResponse, error) {
				return nil, apperrors.ErrArtistExists("Test")
			},
		},
		&testhelpers.MockAuditLogService{},
	)
	req := &AdminFulfillEntityRequestRequest{ID: "16"}
	_, err := h.AdminFulfillEntityRequestHandler(erAdminCtx(), req)
	testhelpers.AssertHumaError(t, err, 409)
}

// Void on an approved-but-unfulfilled row rejects it (no catalog create).
func TestAdminFulfill_Void_Rejects(t *testing.T) {
	rejected := approvedUnfulfilledRequest(17, "artist")
	rejected.DecisionState = communitym.EntityRequestStateRejected
	voidCalled := false

	h := NewEntityRequestHandler(
		&testhelpers.MockEntityRequestService{
			VoidApprovedUnfulfilledFn: func(requestID, adminID uint, note *string) (bool, error) {
				voidCalled = true
				if requestID != 17 || adminID != 1 {
					t.Errorf("unexpected void params req=%d admin=%d", requestID, adminID)
				}
				if note == nil || *note != "bad data" {
					t.Errorf("expected note 'bad data', got %v", note)
				}
				return true, nil
			},
			GetRequestFn: func(requestID uint) (*communitym.EntityRequest, error) { return rejected, nil },
		},
		&testhelpers.MockEntityRequestFulfiller{},
		&testhelpers.MockAuditLogService{},
	)

	req := &AdminFulfillEntityRequestRequest{ID: "17"}
	req.Body.Action = "void"
	note := "bad data"
	req.Body.Note = &note
	resp, err := h.AdminFulfillEntityRequestHandler(erAdminCtx(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !voidCalled {
		t.Error("expected VoidApprovedUnfulfilled to be called")
	}
	if resp.Body.Request == nil || resp.Body.Request.DecisionState != communitym.EntityRequestStateRejected {
		t.Errorf("expected rejected row, got %+v", resp.Body.Request)
	}
	if resp.Body.CreatedEntityID != nil {
		t.Errorf("void must not create an entity, got id %v", resp.Body.CreatedEntityID)
	}
}

// Void on a row that no longer qualifies (fulfilled/pending/missing) → 409.
func TestAdminFulfill_Void_NotRescuable_409(t *testing.T) {
	h := NewEntityRequestHandler(
		&testhelpers.MockEntityRequestService{
			VoidApprovedUnfulfilledFn: func(requestID, adminID uint, note *string) (bool, error) {
				return false, nil
			},
		},
		&testhelpers.MockEntityRequestFulfiller{},
		&testhelpers.MockAuditLogService{},
	)
	req := &AdminFulfillEntityRequestRequest{ID: "18"}
	req.Body.Action = "void"
	_, err := h.AdminFulfillEntityRequestHandler(erAdminCtx(), req)
	testhelpers.AssertHumaError(t, err, 409)
}

// An unrecognized action → 422.
func TestAdminFulfill_InvalidAction_422(t *testing.T) {
	h := NewEntityRequestHandler(&testhelpers.MockEntityRequestService{}, &testhelpers.MockEntityRequestFulfiller{}, nil)
	req := &AdminFulfillEntityRequestRequest{ID: "1"}
	req.Body.Action = "explode"
	_, err := h.AdminFulfillEntityRequestHandler(erAdminCtx(), req)
	testhelpers.AssertHumaError(t, err, 422)
}

// Non-numeric id → 400.
func TestAdminFulfill_InvalidID_400(t *testing.T) {
	h := NewEntityRequestHandler(&testhelpers.MockEntityRequestService{}, &testhelpers.MockEntityRequestFulfiller{}, nil)
	req := &AdminFulfillEntityRequestRequest{ID: "abc"}
	_, err := h.AdminFulfillEntityRequestHandler(erAdminCtx(), req)
	testhelpers.AssertHumaError(t, err, 400)
}
