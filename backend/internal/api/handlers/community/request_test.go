package community

import (
	"context"
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

func testRequestHandler() *RequestHandler {
	return NewRequestHandler(&testhelpers.MockRequestService{}, nil)
}

func requestUserCtx() context.Context {
	return testhelpers.CtxWithUser(&authm.User{ID: 1, IsAdmin: false})
}

// ============================================================================
// Tests: Auth Guard — all mutating endpoints require auth
// ============================================================================

func TestRequestHandler_RequiresAuth(t *testing.T) {
	h := testRequestHandler()

	tests := []struct {
		name string
		fn   func(ctx context.Context) error
	}{
		{"CreateRequest", func(ctx context.Context) error {
			_, err := h.CreateRequestHandler(ctx, &CreateRequestHandlerRequest{})
			return err
		}},
		{"UpdateRequest", func(ctx context.Context) error {
			_, err := h.UpdateRequestHandler(ctx, &UpdateRequestHandlerRequest{RequestID: "1"})
			return err
		}},
		{"DeleteRequest", func(ctx context.Context) error {
			_, err := h.DeleteRequestHandler(ctx, &DeleteRequestHandlerRequest{RequestID: "1"})
			return err
		}},
		{"Vote", func(ctx context.Context) error {
			_, err := h.VoteRequestHandler(ctx, &VoteRequestHandlerRequest{RequestID: "1"})
			return err
		}},
		{"RemoveVote", func(ctx context.Context) error {
			_, err := h.RemoveVoteRequestHandler(ctx, &RemoveVoteRequestHandlerRequest{RequestID: "1"})
			return err
		}},
		{"FulfillRequest", func(ctx context.Context) error {
			_, err := h.FulfillRequestHandler(ctx, &FulfillRequestHandlerRequest{RequestID: "1"})
			return err
		}},
		{"ApproveFulfillment", func(ctx context.Context) error {
			_, err := h.ApproveFulfillmentHandler(ctx, &ApproveFulfillmentHandlerRequest{RequestID: "1"})
			return err
		}},
		{"RejectFulfillment", func(ctx context.Context) error {
			_, err := h.RejectFulfillmentHandler(ctx, &RejectFulfillmentHandlerRequest{RequestID: "1"})
			return err
		}},
		{"CloseRequest", func(ctx context.Context) error {
			_, err := h.CloseRequestHandler(ctx, &CloseRequestHandlerRequest{RequestID: "1"})
			return err
		}},
	}

	for _, tc := range tests {
		t.Run(tc.name+"_NoUser", func(t *testing.T) {
			err := tc.fn(context.Background())
			testhelpers.AssertHumaError(t, err, 401)
		})
	}
}

// ============================================================================
// Tests: CreateRequestHandler
// ============================================================================

func TestRequestHandler_Create_Success(t *testing.T) {
	h := NewRequestHandler(&testhelpers.MockRequestService{
		CreateRequestFn: func(userID uint, title, description, entityType string, requestedEntityID *uint) (*communitym.Request, error) {
			if userID != 1 {
				t.Errorf("expected userID=1, got %d", userID)
			}
			if title != "Add Band XYZ" {
				t.Errorf("expected title='Add Band XYZ', got %q", title)
			}
			if description != "They play shows" {
				t.Errorf("expected description='They play shows', got %q", description)
			}
			if entityType != "artist" {
				t.Errorf("expected entityType='artist', got %q", entityType)
			}
			if requestedEntityID != nil {
				t.Errorf("expected requestedEntityID=nil, got %v", requestedEntityID)
			}
			desc := description
			return &communitym.Request{
				ID:          1,
				Title:       title,
				Description: &desc,
				EntityType:  entityType,
				Status:      communitym.RequestStatusPending,
				RequesterID: userID,
			}, nil
		},
	}, nil)

	req := &CreateRequestHandlerRequest{}
	desc := "They play shows"
	req.Body.Title = "Add Band XYZ"
	req.Body.Description = &desc
	req.Body.EntityType = "artist"

	resp, err := h.CreateRequestHandler(requestUserCtx(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Title != "Add Band XYZ" {
		t.Errorf("expected title 'Add Band XYZ', got %s", resp.Body.Title)
	}
}

func TestRequestHandler_Create_MissingTitle(t *testing.T) {
	h := testRequestHandler()

	req := &CreateRequestHandlerRequest{}
	req.Body.EntityType = "artist"

	_, err := h.CreateRequestHandler(requestUserCtx(), req)
	testhelpers.AssertHumaError(t, err, 422)
}

func TestRequestHandler_Create_MissingEntityType(t *testing.T) {
	h := testRequestHandler()

	req := &CreateRequestHandlerRequest{}
	req.Body.Title = "Test"

	_, err := h.CreateRequestHandler(requestUserCtx(), req)
	testhelpers.AssertHumaError(t, err, 422)
}

func TestRequestHandler_Create_ServiceError(t *testing.T) {
	h := NewRequestHandler(&testhelpers.MockRequestService{
		CreateRequestFn: func(userID uint, title, description, entityType string, requestedEntityID *uint) (*communitym.Request, error) {
			return nil, fmt.Errorf("database error")
		},
	}, nil)

	req := &CreateRequestHandlerRequest{}
	errDesc := "desc"
	req.Body.Title = "Test"
	req.Body.Description = &errDesc
	req.Body.EntityType = "artist"

	_, err := h.CreateRequestHandler(requestUserCtx(), req)
	testhelpers.AssertHumaError(t, err, 500)
}

// ============================================================================
// Tests: GetRequestHandler
// ============================================================================

func TestRequestHandler_Get_Success(t *testing.T) {
	h := NewRequestHandler(&testhelpers.MockRequestService{
		GetRequestFn: func(requestID uint) (*communitym.Request, error) {
			if requestID != 42 {
				t.Errorf("expected requestID=42, got %d", requestID)
			}
			return &communitym.Request{
				ID:          requestID,
				Title:       "Test Request",
				EntityType:  "artist",
				Status:      communitym.RequestStatusPending,
				RequesterID: 1,
			}, nil
		},
	}, nil)

	resp, err := h.GetRequestHandler(requestUserCtx(), &GetRequestHandlerRequest{RequestID: "42"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.ID != 42 {
		t.Errorf("expected id=42, got %d", resp.Body.ID)
	}
}

func TestRequestHandler_Get_InvalidID(t *testing.T) {
	h := testRequestHandler()

	_, err := h.GetRequestHandler(requestUserCtx(), &GetRequestHandlerRequest{RequestID: "not-a-number"})
	testhelpers.AssertHumaError(t, err, 400)
}

func TestRequestHandler_Get_NotFound(t *testing.T) {
	h := NewRequestHandler(&testhelpers.MockRequestService{
		GetRequestFn: func(requestID uint) (*communitym.Request, error) {
			return nil, nil
		},
	}, nil)

	_, err := h.GetRequestHandler(requestUserCtx(), &GetRequestHandlerRequest{RequestID: "999"})
	testhelpers.AssertHumaError(t, err, 404)
}

func TestRequestHandler_Get_ServiceError(t *testing.T) {
	h := NewRequestHandler(&testhelpers.MockRequestService{
		GetRequestFn: func(requestID uint) (*communitym.Request, error) {
			return nil, fmt.Errorf("db error")
		},
	}, nil)

	_, err := h.GetRequestHandler(requestUserCtx(), &GetRequestHandlerRequest{RequestID: "1"})
	testhelpers.AssertHumaError(t, err, 500)
}

// PSY-917: when the request references an entity, the detail handler resolves
// it to a slug + name so the review panel can render a "View proposed" link.
func TestRequestHandler_Get_ResolvesProposedEntity(t *testing.T) {
	entityID := uint(7)
	slug := "kendrick-lamar"
	var resolveCalls int
	h := NewRequestHandler(&testhelpers.MockRequestService{
		GetRequestFn: func(requestID uint) (*communitym.Request, error) {
			return &communitym.Request{
				ID:                requestID,
				Title:             "Test Request",
				EntityType:        "artist",
				RequestedEntityID: &entityID,
				Status:            communitym.RequestStatusPendingFulfillment,
				RequesterID:       1,
			}, nil
		},
		ResolveEntityRefFn: func(entityType string, id uint) (*contracts.EntityRef, error) {
			resolveCalls++
			if entityType != "artist" || id != entityID {
				t.Errorf("expected ResolveEntityRef(artist, %d), got (%s, %d)", entityID, entityType, id)
			}
			return &contracts.EntityRef{Slug: &slug, Name: "Kendrick Lamar"}, nil
		},
	}, nil)

	resp, err := h.GetRequestHandler(requestUserCtx(), &GetRequestHandlerRequest{RequestID: "42"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resolveCalls != 1 {
		t.Errorf("expected ResolveEntityRef to be called once, got %d", resolveCalls)
	}
	if resp.Body.RequestedEntitySlug == nil || *resp.Body.RequestedEntitySlug != slug {
		t.Errorf("expected slug %q, got %v", slug, resp.Body.RequestedEntitySlug)
	}
	if resp.Body.RequestedEntityName == nil || *resp.Body.RequestedEntityName != "Kendrick Lamar" {
		t.Errorf("expected name %q, got %v", "Kendrick Lamar", resp.Body.RequestedEntityName)
	}
}

// PSY-917: a NULL-slug entity (catalog slug is nullable) resolves with a name
// but no slug — the handler leaves the slug nil so the frontend suppresses
// the broken link.
func TestRequestHandler_Get_ProposedEntityNullSlug(t *testing.T) {
	entityID := uint(7)
	h := NewRequestHandler(&testhelpers.MockRequestService{
		GetRequestFn: func(requestID uint) (*communitym.Request, error) {
			return &communitym.Request{
				ID:                requestID,
				EntityType:        "artist",
				RequestedEntityID: &entityID,
				Status:            communitym.RequestStatusPendingFulfillment,
				RequesterID:       1,
			}, nil
		},
		ResolveEntityRefFn: func(entityType string, id uint) (*contracts.EntityRef, error) {
			return &contracts.EntityRef{Slug: nil, Name: "No Slug Artist"}, nil
		},
	}, nil)

	resp, err := h.GetRequestHandler(requestUserCtx(), &GetRequestHandlerRequest{RequestID: "42"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.RequestedEntitySlug != nil {
		t.Errorf("expected nil slug, got %v", *resp.Body.RequestedEntitySlug)
	}
	if resp.Body.RequestedEntityName == nil || *resp.Body.RequestedEntityName != "No Slug Artist" {
		t.Errorf("expected name to survive, got %v", resp.Body.RequestedEntityName)
	}
}

// PSY-917: a stale RequestedEntityID (entity since deleted → resolver returns
// nil) must not crash the detail fetch; the request renders link-less.
func TestRequestHandler_Get_ProposedEntityResolveFailureNonFatal(t *testing.T) {
	entityID := uint(7)
	h := NewRequestHandler(&testhelpers.MockRequestService{
		GetRequestFn: func(requestID uint) (*communitym.Request, error) {
			return &communitym.Request{
				ID:                requestID,
				EntityType:        "artist",
				RequestedEntityID: &entityID,
				Status:            communitym.RequestStatusPendingFulfillment,
				RequesterID:       1,
			}, nil
		},
		ResolveEntityRefFn: func(entityType string, id uint) (*contracts.EntityRef, error) {
			return nil, fmt.Errorf("db blew up")
		},
	}, nil)

	resp, err := h.GetRequestHandler(requestUserCtx(), &GetRequestHandlerRequest{RequestID: "42"})
	if err != nil {
		t.Fatalf("resolver failure should not fail the request fetch, got: %v", err)
	}
	if resp.Body.RequestedEntitySlug != nil || resp.Body.RequestedEntityName != nil {
		t.Errorf("expected no entity ref on resolver failure, got slug=%v name=%v",
			resp.Body.RequestedEntitySlug, resp.Body.RequestedEntityName)
	}
}

// ============================================================================
// Tests: ListRequestsHandler
// ============================================================================

func TestRequestHandler_List_Success(t *testing.T) {
	h := NewRequestHandler(&testhelpers.MockRequestService{
		ListRequestsFn: func(status string, entityType string, sortBy string, limit, offset int) ([]communitym.Request, int64, error) {
			if limit != 20 {
				t.Errorf("expected limit=20, got %d", limit)
			}
			if offset != 10 {
				t.Errorf("expected offset=10, got %d", offset)
			}
			if status != "pending" {
				t.Errorf("expected status='pending', got %q", status)
			}
			if entityType != "artist" {
				t.Errorf("expected entityType='artist', got %q", entityType)
			}
			if sortBy != "votes" {
				t.Errorf("expected sortBy='votes', got %q", sortBy)
			}
			return []communitym.Request{
				{ID: 1, Title: "Request 1", EntityType: "artist", Status: communitym.RequestStatusPending, RequesterID: 1},
			}, 1, nil
		},
	}, nil)

	resp, err := h.ListRequestsHandler(requestUserCtx(), &ListRequestsHandlerRequest{
		Limit:      20,
		Offset:     10,
		Status:     "pending",
		EntityType: "artist",
		Sort:       "votes",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Total != 1 {
		t.Errorf("expected total=1, got %d", resp.Body.Total)
	}
	if len(resp.Body.Requests) != 1 {
		t.Fatalf("expected 1 request, got %d", len(resp.Body.Requests))
	}
}

func TestRequestHandler_List_DefaultLimit(t *testing.T) {
	var receivedLimit int
	h := NewRequestHandler(&testhelpers.MockRequestService{
		ListRequestsFn: func(status string, entityType string, sortBy string, limit, offset int) ([]communitym.Request, int64, error) {
			receivedLimit = limit
			return []communitym.Request{}, 0, nil
		},
	}, nil)

	_, err := h.ListRequestsHandler(requestUserCtx(), &ListRequestsHandlerRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if receivedLimit != 20 {
		t.Errorf("expected default limit=20, got %d", receivedLimit)
	}
}

func TestRequestHandler_List_ServiceError(t *testing.T) {
	h := NewRequestHandler(&testhelpers.MockRequestService{
		ListRequestsFn: func(status string, entityType string, sortBy string, limit, offset int) ([]communitym.Request, int64, error) {
			return nil, 0, fmt.Errorf("database error")
		},
	}, nil)

	_, err := h.ListRequestsHandler(requestUserCtx(), &ListRequestsHandlerRequest{})
	testhelpers.AssertHumaError(t, err, 500)
}

func TestRequestHandler_List_NoAuth_Succeeds(t *testing.T) {
	// List should work without authentication (public endpoint)
	h := NewRequestHandler(&testhelpers.MockRequestService{}, nil)

	resp, err := h.ListRequestsHandler(context.Background(), &ListRequestsHandlerRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp == nil {
		t.Fatal("expected non-nil response")
	}
}

// ============================================================================
// Tests: UpdateRequestHandler
// ============================================================================

func TestRequestHandler_Update_Success(t *testing.T) {
	h := NewRequestHandler(&testhelpers.MockRequestService{
		UpdateRequestFn: func(requestID, userID uint, title, description *string) (*communitym.Request, error) {
			if requestID != 7 {
				t.Errorf("expected requestID=7, got %d", requestID)
			}
			if userID != 1 {
				t.Errorf("expected userID=1, got %d", userID)
			}
			if title == nil || *title != "Updated" {
				t.Errorf("expected title='Updated', got %v", title)
			}
			if description != nil {
				t.Errorf("expected description=nil, got %v", description)
			}
			return &communitym.Request{ID: requestID, Title: *title, EntityType: "artist", Status: communitym.RequestStatusPending, RequesterID: userID}, nil
		},
	}, nil)

	req := &UpdateRequestHandlerRequest{RequestID: "7"}
	title := "Updated"
	req.Body.Title = &title

	resp, err := h.UpdateRequestHandler(requestUserCtx(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.ID != 7 {
		t.Errorf("expected id=7, got %d", resp.Body.ID)
	}
}

func TestRequestHandler_Update_InvalidID(t *testing.T) {
	h := testRequestHandler()

	req := &UpdateRequestHandlerRequest{RequestID: "abc"}
	_, err := h.UpdateRequestHandler(requestUserCtx(), req)
	testhelpers.AssertHumaError(t, err, 400)
}

func TestRequestHandler_Update_ServiceError_NotFound(t *testing.T) {
	h := NewRequestHandler(&testhelpers.MockRequestService{
		UpdateRequestFn: func(requestID, userID uint, title, description *string) (*communitym.Request, error) {
			return nil, fmt.Errorf("REQUEST_NOT_FOUND: not found")
		},
	}, nil)

	req := &UpdateRequestHandlerRequest{RequestID: "999"}
	_, err := h.UpdateRequestHandler(requestUserCtx(), req)
	// Falls through to 500 since the plain error doesn't match RequestError type
	testhelpers.AssertHumaError(t, err, 500)
}

// ============================================================================
// Tests: DeleteRequestHandler
// ============================================================================

func TestRequestHandler_Delete_Success(t *testing.T) {
	h := NewRequestHandler(&testhelpers.MockRequestService{
		DeleteRequestFn: func(requestID, userID uint, isAdmin bool) error {
			if requestID != 3 {
				t.Errorf("expected requestID=3, got %d", requestID)
			}
			if userID != 1 {
				t.Errorf("expected userID=1, got %d", userID)
			}
			if isAdmin {
				t.Errorf("expected isAdmin=false, got true")
			}
			return nil
		},
	}, nil)

	_, err := h.DeleteRequestHandler(requestUserCtx(), &DeleteRequestHandlerRequest{RequestID: "3"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRequestHandler_Delete_InvalidID(t *testing.T) {
	h := testRequestHandler()

	_, err := h.DeleteRequestHandler(requestUserCtx(), &DeleteRequestHandlerRequest{RequestID: "abc"})
	testhelpers.AssertHumaError(t, err, 400)
}

func TestRequestHandler_Delete_ServiceError(t *testing.T) {
	h := NewRequestHandler(&testhelpers.MockRequestService{
		DeleteRequestFn: func(requestID, userID uint, isAdmin bool) error {
			return fmt.Errorf("db error")
		},
	}, nil)

	_, err := h.DeleteRequestHandler(requestUserCtx(), &DeleteRequestHandlerRequest{RequestID: "1"})
	testhelpers.AssertHumaError(t, err, 500)
}

// ============================================================================
// Tests: VoteRequestHandler
// ============================================================================

func TestRequestHandler_Vote_Success(t *testing.T) {
	h := NewRequestHandler(&testhelpers.MockRequestService{
		VoteFn: func(requestID, userID uint, isUpvote bool) error {
			if requestID != 5 {
				t.Errorf("expected requestID=5, got %d", requestID)
			}
			if userID != 1 {
				t.Errorf("expected userID=1, got %d", userID)
			}
			if !isUpvote {
				t.Errorf("expected isUpvote=true, got false")
			}
			return nil
		},
	}, nil)

	req := &VoteRequestHandlerRequest{RequestID: "5"}
	req.Body.IsUpvote = true

	_, err := h.VoteRequestHandler(requestUserCtx(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRequestHandler_Vote_InvalidID(t *testing.T) {
	h := testRequestHandler()

	_, err := h.VoteRequestHandler(requestUserCtx(), &VoteRequestHandlerRequest{RequestID: "abc"})
	testhelpers.AssertHumaError(t, err, 400)
}

func TestRequestHandler_Vote_ServiceError(t *testing.T) {
	h := NewRequestHandler(&testhelpers.MockRequestService{
		VoteFn: func(requestID, userID uint, isUpvote bool) error {
			return fmt.Errorf("db error")
		},
	}, nil)

	req := &VoteRequestHandlerRequest{RequestID: "1"}
	req.Body.IsUpvote = true

	_, err := h.VoteRequestHandler(requestUserCtx(), req)
	testhelpers.AssertHumaError(t, err, 500)
}

// ============================================================================
// Tests: RemoveVoteRequestHandler
// ============================================================================

func TestRequestHandler_RemoveVote_Success(t *testing.T) {
	h := NewRequestHandler(&testhelpers.MockRequestService{
		RemoveVoteFn: func(requestID, userID uint) error {
			if requestID != 8 {
				t.Errorf("expected requestID=8, got %d", requestID)
			}
			if userID != 1 {
				t.Errorf("expected userID=1, got %d", userID)
			}
			return nil
		},
	}, nil)

	_, err := h.RemoveVoteRequestHandler(requestUserCtx(), &RemoveVoteRequestHandlerRequest{RequestID: "8"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRequestHandler_RemoveVote_InvalidID(t *testing.T) {
	h := testRequestHandler()

	_, err := h.RemoveVoteRequestHandler(requestUserCtx(), &RemoveVoteRequestHandlerRequest{RequestID: "abc"})
	testhelpers.AssertHumaError(t, err, 400)
}

// ============================================================================
// Tests: FulfillRequestHandler
// ============================================================================

func TestRequestHandler_Fulfill_Success(t *testing.T) {
	h := NewRequestHandler(&testhelpers.MockRequestService{
		FulfillRequestFn: func(requestID, fulfillerID uint, fulfilledEntityID *uint) error {
			if requestID != 15 {
				t.Errorf("expected requestID=15, got %d", requestID)
			}
			if fulfillerID != 1 {
				t.Errorf("expected fulfillerID=1, got %d", fulfillerID)
			}
			return nil
		},
	}, nil)

	_, err := h.FulfillRequestHandler(requestUserCtx(), &FulfillRequestHandlerRequest{RequestID: "15"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRequestHandler_Fulfill_InvalidID(t *testing.T) {
	h := testRequestHandler()

	_, err := h.FulfillRequestHandler(requestUserCtx(), &FulfillRequestHandlerRequest{RequestID: "abc"})
	testhelpers.AssertHumaError(t, err, 400)
}

func TestRequestHandler_Fulfill_ServiceError(t *testing.T) {
	h := NewRequestHandler(&testhelpers.MockRequestService{
		FulfillRequestFn: func(requestID, fulfillerID uint, fulfilledEntityID *uint) error {
			return fmt.Errorf("db error")
		},
	}, nil)

	_, err := h.FulfillRequestHandler(requestUserCtx(), &FulfillRequestHandlerRequest{RequestID: "1"})
	testhelpers.AssertHumaError(t, err, 500)
}

// PSY-748: typed errors from the service must surface as the right HTTP
// status. EntityTypeMismatch + EntityNotFound → 400 (caller payload
// problem); the request itself is fine and still exists.
func TestRequestHandler_Fulfill_EntityTypeMismatch_400(t *testing.T) {
	h := NewRequestHandler(&testhelpers.MockRequestService{
		FulfillRequestFn: func(requestID, fulfillerID uint, fulfilledEntityID *uint) error {
			return apperrors.ErrRequestEntityTypeMismatch(requestID, "artist", "venue")
		},
	}, nil)

	_, err := h.FulfillRequestHandler(requestUserCtx(), &FulfillRequestHandlerRequest{RequestID: "1"})
	testhelpers.AssertHumaError(t, err, 400)
}

func TestRequestHandler_Fulfill_EntityNotFound_400(t *testing.T) {
	h := NewRequestHandler(&testhelpers.MockRequestService{
		FulfillRequestFn: func(requestID, fulfillerID uint, fulfilledEntityID *uint) error {
			return apperrors.ErrRequestEntityNotFound(requestID, "artist", 9999)
		},
	}, nil)

	_, err := h.FulfillRequestHandler(requestUserCtx(), &FulfillRequestHandlerRequest{RequestID: "1"})
	testhelpers.AssertHumaError(t, err, 400)
}

// ============================================================================
// Tests: ApproveFulfillmentHandler (PSY-748)
// ============================================================================

func TestRequestHandler_ApproveFulfillment_Success(t *testing.T) {
	var sawIsAdmin bool
	h := NewRequestHandler(&testhelpers.MockRequestService{
		ApproveFulfillmentFn: func(requestID, userID uint, isAdmin bool) error {
			if requestID != 42 {
				t.Errorf("expected requestID=42, got %d", requestID)
			}
			if userID != 1 {
				t.Errorf("expected userID=1, got %d", userID)
			}
			sawIsAdmin = isAdmin
			return nil
		},
	}, nil)

	_, err := h.ApproveFulfillmentHandler(requestUserCtx(), &ApproveFulfillmentHandlerRequest{RequestID: "42"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sawIsAdmin {
		t.Errorf("expected isAdmin=false (default test user is non-admin), got true")
	}
}

func TestRequestHandler_ApproveFulfillment_AdminFlagPropagates(t *testing.T) {
	// Admin users approve via the same handler — the IsAdmin flag from
	// the context user must thread through to the service so admin
	// take-overs work.
	var sawIsAdmin bool
	h := NewRequestHandler(&testhelpers.MockRequestService{
		ApproveFulfillmentFn: func(requestID, userID uint, isAdmin bool) error {
			sawIsAdmin = isAdmin
			return nil
		},
	}, nil)

	adminCtx := testhelpers.CtxWithUser(&authm.User{ID: 7, IsAdmin: true})
	_, err := h.ApproveFulfillmentHandler(adminCtx, &ApproveFulfillmentHandlerRequest{RequestID: "1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !sawIsAdmin {
		t.Errorf("expected isAdmin=true, got false")
	}
}

func TestRequestHandler_ApproveFulfillment_InvalidID(t *testing.T) {
	h := testRequestHandler()

	_, err := h.ApproveFulfillmentHandler(requestUserCtx(), &ApproveFulfillmentHandlerRequest{RequestID: "abc"})
	testhelpers.AssertHumaError(t, err, 400)
}

func TestRequestHandler_ApproveFulfillment_Forbidden_403(t *testing.T) {
	h := NewRequestHandler(&testhelpers.MockRequestService{
		ApproveFulfillmentFn: func(requestID, userID uint, isAdmin bool) error {
			return apperrors.ErrRequestForbidden(requestID)
		},
	}, nil)

	_, err := h.ApproveFulfillmentHandler(requestUserCtx(), &ApproveFulfillmentHandlerRequest{RequestID: "1"})
	testhelpers.AssertHumaError(t, err, 403)
}

func TestRequestHandler_ApproveFulfillment_InvalidState_409(t *testing.T) {
	h := NewRequestHandler(&testhelpers.MockRequestService{
		ApproveFulfillmentFn: func(requestID, userID uint, isAdmin bool) error {
			return apperrors.ErrRequestInvalidState(requestID, "pending", "pending_fulfillment")
		},
	}, nil)

	_, err := h.ApproveFulfillmentHandler(requestUserCtx(), &ApproveFulfillmentHandlerRequest{RequestID: "1"})
	testhelpers.AssertHumaError(t, err, 409)
}

func TestRequestHandler_ApproveFulfillment_NotFound_404(t *testing.T) {
	h := NewRequestHandler(&testhelpers.MockRequestService{
		ApproveFulfillmentFn: func(requestID, userID uint, isAdmin bool) error {
			return apperrors.ErrRequestNotFound(requestID)
		},
	}, nil)

	_, err := h.ApproveFulfillmentHandler(requestUserCtx(), &ApproveFulfillmentHandlerRequest{RequestID: "999"})
	testhelpers.AssertHumaError(t, err, 404)
}

func TestRequestHandler_ApproveFulfillment_ServiceError_500(t *testing.T) {
	h := NewRequestHandler(&testhelpers.MockRequestService{
		ApproveFulfillmentFn: func(requestID, userID uint, isAdmin bool) error {
			return fmt.Errorf("db error")
		},
	}, nil)

	_, err := h.ApproveFulfillmentHandler(requestUserCtx(), &ApproveFulfillmentHandlerRequest{RequestID: "1"})
	testhelpers.AssertHumaError(t, err, 500)
}

// ============================================================================
// Tests: RejectFulfillmentHandler (PSY-748)
// ============================================================================

func TestRequestHandler_RejectFulfillment_Success(t *testing.T) {
	h := NewRequestHandler(&testhelpers.MockRequestService{
		RejectFulfillmentFn: func(requestID, userID uint, isAdmin bool) error {
			if requestID != 33 {
				t.Errorf("expected requestID=33, got %d", requestID)
			}
			if userID != 1 {
				t.Errorf("expected userID=1, got %d", userID)
			}
			return nil
		},
	}, nil)

	_, err := h.RejectFulfillmentHandler(requestUserCtx(), &RejectFulfillmentHandlerRequest{RequestID: "33"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRequestHandler_RejectFulfillment_InvalidID(t *testing.T) {
	h := testRequestHandler()

	_, err := h.RejectFulfillmentHandler(requestUserCtx(), &RejectFulfillmentHandlerRequest{RequestID: "abc"})
	testhelpers.AssertHumaError(t, err, 400)
}

func TestRequestHandler_RejectFulfillment_Forbidden_403(t *testing.T) {
	h := NewRequestHandler(&testhelpers.MockRequestService{
		RejectFulfillmentFn: func(requestID, userID uint, isAdmin bool) error {
			return apperrors.ErrRequestForbidden(requestID)
		},
	}, nil)

	_, err := h.RejectFulfillmentHandler(requestUserCtx(), &RejectFulfillmentHandlerRequest{RequestID: "1"})
	testhelpers.AssertHumaError(t, err, 403)
}

func TestRequestHandler_RejectFulfillment_InvalidState_409(t *testing.T) {
	h := NewRequestHandler(&testhelpers.MockRequestService{
		RejectFulfillmentFn: func(requestID, userID uint, isAdmin bool) error {
			return apperrors.ErrRequestInvalidState(requestID, "pending", "pending_fulfillment")
		},
	}, nil)

	_, err := h.RejectFulfillmentHandler(requestUserCtx(), &RejectFulfillmentHandlerRequest{RequestID: "1"})
	testhelpers.AssertHumaError(t, err, 409)
}

func TestRequestHandler_RejectFulfillment_NotFound_404(t *testing.T) {
	h := NewRequestHandler(&testhelpers.MockRequestService{
		RejectFulfillmentFn: func(requestID, userID uint, isAdmin bool) error {
			return apperrors.ErrRequestNotFound(requestID)
		},
	}, nil)

	_, err := h.RejectFulfillmentHandler(requestUserCtx(), &RejectFulfillmentHandlerRequest{RequestID: "999"})
	testhelpers.AssertHumaError(t, err, 404)
}

func TestRequestHandler_RejectFulfillment_ServiceError_500(t *testing.T) {
	h := NewRequestHandler(&testhelpers.MockRequestService{
		RejectFulfillmentFn: func(requestID, userID uint, isAdmin bool) error {
			return fmt.Errorf("db error")
		},
	}, nil)

	_, err := h.RejectFulfillmentHandler(requestUserCtx(), &RejectFulfillmentHandlerRequest{RequestID: "1"})
	testhelpers.AssertHumaError(t, err, 500)
}

// ============================================================================
// Tests: CloseRequestHandler
// ============================================================================

func TestRequestHandler_Close_Success(t *testing.T) {
	h := NewRequestHandler(&testhelpers.MockRequestService{
		CloseRequestFn: func(requestID, userID uint, isAdmin bool) error {
			if requestID != 20 {
				t.Errorf("expected requestID=20, got %d", requestID)
			}
			if userID != 1 {
				t.Errorf("expected userID=1, got %d", userID)
			}
			if isAdmin {
				t.Errorf("expected isAdmin=false, got true")
			}
			return nil
		},
	}, nil)

	_, err := h.CloseRequestHandler(requestUserCtx(), &CloseRequestHandlerRequest{RequestID: "20"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRequestHandler_Close_InvalidID(t *testing.T) {
	h := testRequestHandler()

	_, err := h.CloseRequestHandler(requestUserCtx(), &CloseRequestHandlerRequest{RequestID: "abc"})
	testhelpers.AssertHumaError(t, err, 400)
}

func TestRequestHandler_Close_ServiceError(t *testing.T) {
	h := NewRequestHandler(&testhelpers.MockRequestService{
		CloseRequestFn: func(requestID, userID uint, isAdmin bool) error {
			return fmt.Errorf("db error")
		},
	}, nil)

	_, err := h.CloseRequestHandler(requestUserCtx(), &CloseRequestHandlerRequest{RequestID: "1"})
	testhelpers.AssertHumaError(t, err, 500)
}

// ============================================================================
// Tests: mapRequestError
// ============================================================================

func TestMapRequestError(t *testing.T) {
	tests := []struct {
		name           string
		err            error
		expectedStatus int
		expectNil      bool
	}{
		{
			name:           "not found",
			err:            &errorWithCode{code: "REQUEST_NOT_FOUND", message: "not found"},
			expectedStatus: 0,
			expectNil:      true, // Not a *RequestError, so mapRequestError returns nil
		},
		{
			name:      "generic error",
			err:       fmt.Errorf("some error"),
			expectNil: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := mapRequestError(tc.err)
			if tc.expectNil {
				if result != nil {
					t.Errorf("expected nil, got %v", result)
				}
			} else {
				testhelpers.AssertHumaError(t, result, tc.expectedStatus)
			}
		})
	}
}

// errorWithCode is a test helper for errors that have a code but aren't RequestError
type errorWithCode struct {
	code    string
	message string
}

func (e *errorWithCode) Error() string { return e.code + ": " + e.message }

// ============================================================================
// Tests: buildRequestResponse
// ============================================================================

func TestBuildRequestResponse(t *testing.T) {
	username := "testuser"
	request := &communitym.Request{
		ID:          1,
		Title:       "Test",
		EntityType:  "artist",
		Status:      communitym.RequestStatusPending,
		RequesterID: 10,
		Upvotes:     3,
		Downvotes:   1,
		VoteScore:   2,
		Requester:   authm.User{ID: 10, Username: &username},
	}

	userVote := 1
	resp := buildRequestResponse(request, &userVote)

	if resp.ID != 1 {
		t.Errorf("expected id=1, got %d", resp.ID)
	}
	if resp.Title != "Test" {
		t.Errorf("expected title=Test, got %s", resp.Title)
	}
	if resp.RequesterName != "testuser" {
		t.Errorf("expected requester_name=testuser, got %s", resp.RequesterName)
	}
	// PSY-619: RequesterUsername populated when the user has a username set —
	// frontend uses this to render the byline as a /users/:username link.
	if resp.RequesterUsername == nil {
		t.Errorf("expected requester_username=&%q, got nil", username)
	} else if *resp.RequesterUsername != username {
		t.Errorf("expected requester_username=%q, got %q", username, *resp.RequesterUsername)
	}
	if resp.UserVote == nil || *resp.UserVote != 1 {
		t.Errorf("expected user_vote=1, got %v", resp.UserVote)
	}
	if resp.WilsonScore <= 0 {
		t.Errorf("expected positive wilson_score, got %f", resp.WilsonScore)
	}
}

// TestBuildRequestResponse_NoUsername asserts the unlinked-byline path (PSY-619):
// when the requester has no username on file, RequesterUsername is nil so the
// frontend renders plain text rather than a broken /users/null link.
func TestBuildRequestResponse_NoUsername(t *testing.T) {
	firstName := "Jane"
	request := &communitym.Request{
		ID:          1,
		Title:       "Test",
		EntityType:  "artist",
		Status:      communitym.RequestStatusPending,
		RequesterID: 10,
		Requester:   authm.User{ID: 10, FirstName: &firstName},
	}

	resp := buildRequestResponse(request, nil)
	if resp.RequesterName != "Jane" {
		t.Errorf("expected requester_name=Jane (first-name fallback), got %s", resp.RequesterName)
	}
	if resp.RequesterUsername != nil {
		t.Errorf("expected requester_username=nil, got %q", *resp.RequesterUsername)
	}
}

func TestBuildRequestResponse_NoVote(t *testing.T) {
	request := &communitym.Request{
		ID:          1,
		Title:       "Test",
		EntityType:  "artist",
		Status:      communitym.RequestStatusPending,
		RequesterID: 10,
	}

	resp := buildRequestResponse(request, nil)
	if resp.UserVote != nil {
		t.Errorf("expected user_vote=nil, got %v", resp.UserVote)
	}
}
