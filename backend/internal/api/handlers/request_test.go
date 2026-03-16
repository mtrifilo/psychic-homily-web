package handlers

import (
	"context"
	"fmt"
	"testing"

	"psychic-homily-backend/internal/models"
)

// ============================================================================
// Mock: RequestServiceInterface
// ============================================================================

type mockRequestService struct {
	createRequestFn   func(userID uint, title, description, entityType string, requestedEntityID *uint) (*models.Request, error)
	getRequestFn      func(requestID uint) (*models.Request, error)
	listRequestsFn    func(status string, entityType string, sortBy string, limit, offset int) ([]models.Request, int64, error)
	updateRequestFn   func(requestID, userID uint, title, description *string) (*models.Request, error)
	deleteRequestFn   func(requestID, userID uint, isAdmin bool) error
	voteFn            func(requestID, userID uint, isUpvote bool) error
	removeVoteFn      func(requestID, userID uint) error
	fulfillRequestFn  func(requestID, fulfillerID uint, fulfilledEntityID *uint) error
	closeRequestFn    func(requestID, userID uint, isAdmin bool) error
	getUserVoteFn     func(requestID, userID uint) (*models.RequestVote, error)
}

func (m *mockRequestService) CreateRequest(userID uint, title, description, entityType string, requestedEntityID *uint) (*models.Request, error) {
	if m.createRequestFn != nil {
		return m.createRequestFn(userID, title, description, entityType, requestedEntityID)
	}
	desc := description
	return &models.Request{
		ID:          1,
		Title:       title,
		Description: &desc,
		EntityType:  entityType,
		Status:      models.RequestStatusPending,
		RequesterID: userID,
	}, nil
}

func (m *mockRequestService) GetRequest(requestID uint) (*models.Request, error) {
	if m.getRequestFn != nil {
		return m.getRequestFn(requestID)
	}
	return &models.Request{
		ID:          requestID,
		Title:       "Test Request",
		EntityType:  "artist",
		Status:      models.RequestStatusPending,
		RequesterID: 1,
	}, nil
}

func (m *mockRequestService) ListRequests(status string, entityType string, sortBy string, limit, offset int) ([]models.Request, int64, error) {
	if m.listRequestsFn != nil {
		return m.listRequestsFn(status, entityType, sortBy, limit, offset)
	}
	return []models.Request{
		{ID: 1, Title: "Request 1", EntityType: "artist", Status: models.RequestStatusPending, RequesterID: 1},
	}, 1, nil
}

func (m *mockRequestService) UpdateRequest(requestID, userID uint, title, description *string) (*models.Request, error) {
	if m.updateRequestFn != nil {
		return m.updateRequestFn(requestID, userID, title, description)
	}
	t := "Updated"
	return &models.Request{ID: requestID, Title: t, EntityType: "artist", Status: models.RequestStatusPending, RequesterID: userID}, nil
}

func (m *mockRequestService) DeleteRequest(requestID, userID uint, isAdmin bool) error {
	if m.deleteRequestFn != nil {
		return m.deleteRequestFn(requestID, userID, isAdmin)
	}
	return nil
}

func (m *mockRequestService) Vote(requestID, userID uint, isUpvote bool) error {
	if m.voteFn != nil {
		return m.voteFn(requestID, userID, isUpvote)
	}
	return nil
}

func (m *mockRequestService) RemoveVote(requestID, userID uint) error {
	if m.removeVoteFn != nil {
		return m.removeVoteFn(requestID, userID)
	}
	return nil
}

func (m *mockRequestService) FulfillRequest(requestID, fulfillerID uint, fulfilledEntityID *uint) error {
	if m.fulfillRequestFn != nil {
		return m.fulfillRequestFn(requestID, fulfillerID, fulfilledEntityID)
	}
	return nil
}

func (m *mockRequestService) CloseRequest(requestID, userID uint, isAdmin bool) error {
	if m.closeRequestFn != nil {
		return m.closeRequestFn(requestID, userID, isAdmin)
	}
	return nil
}

func (m *mockRequestService) GetUserVote(requestID, userID uint) (*models.RequestVote, error) {
	if m.getUserVoteFn != nil {
		return m.getUserVoteFn(requestID, userID)
	}
	return nil, nil
}

// ============================================================================
// Test helpers
// ============================================================================

func testRequestHandler() *RequestHandler {
	return NewRequestHandler(&mockRequestService{}, nil)
}

func requestUserCtx() context.Context {
	return ctxWithUser(&models.User{ID: 1, IsAdmin: false})
}

func requestAdminCtx() context.Context {
	return ctxWithUser(&models.User{ID: 2, IsAdmin: true})
}

// ============================================================================
// Tests: NewRequestHandler
// ============================================================================

func TestNewRequestHandler(t *testing.T) {
	h := testRequestHandler()
	if h == nil {
		t.Fatal("expected non-nil RequestHandler")
	}
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
		{"CloseRequest", func(ctx context.Context) error {
			_, err := h.CloseRequestHandler(ctx, &CloseRequestHandlerRequest{RequestID: "1"})
			return err
		}},
	}

	for _, tc := range tests {
		t.Run(tc.name+"_NoUser", func(t *testing.T) {
			err := tc.fn(context.Background())
			assertHumaError(t, err, 401)
		})
	}
}

// ============================================================================
// Tests: CreateRequestHandler
// ============================================================================

func TestRequestHandler_Create_Success(t *testing.T) {
	h := NewRequestHandler(&mockRequestService{}, nil)

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
	assertHumaError(t, err, 400)
}

func TestRequestHandler_Create_MissingEntityType(t *testing.T) {
	h := testRequestHandler()

	req := &CreateRequestHandlerRequest{}
	req.Body.Title = "Test"

	_, err := h.CreateRequestHandler(requestUserCtx(), req)
	assertHumaError(t, err, 400)
}

func TestRequestHandler_Create_ServiceError(t *testing.T) {
	h := NewRequestHandler(&mockRequestService{
		createRequestFn: func(userID uint, title, description, entityType string, requestedEntityID *uint) (*models.Request, error) {
			return nil, fmt.Errorf("database error")
		},
	}, nil)

	req := &CreateRequestHandlerRequest{}
	errDesc := "desc"
	req.Body.Title = "Test"
	req.Body.Description = &errDesc
	req.Body.EntityType = "artist"

	_, err := h.CreateRequestHandler(requestUserCtx(), req)
	assertHumaError(t, err, 500)
}

// ============================================================================
// Tests: GetRequestHandler
// ============================================================================

func TestRequestHandler_Get_Success(t *testing.T) {
	h := NewRequestHandler(&mockRequestService{}, nil)

	resp, err := h.GetRequestHandler(requestUserCtx(), &GetRequestHandlerRequest{RequestID: "1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.ID != 1 {
		t.Errorf("expected id=1, got %d", resp.Body.ID)
	}
}

func TestRequestHandler_Get_InvalidID(t *testing.T) {
	h := testRequestHandler()

	_, err := h.GetRequestHandler(requestUserCtx(), &GetRequestHandlerRequest{RequestID: "not-a-number"})
	assertHumaError(t, err, 400)
}

func TestRequestHandler_Get_NotFound(t *testing.T) {
	h := NewRequestHandler(&mockRequestService{
		getRequestFn: func(requestID uint) (*models.Request, error) {
			return nil, nil
		},
	}, nil)

	_, err := h.GetRequestHandler(requestUserCtx(), &GetRequestHandlerRequest{RequestID: "999"})
	assertHumaError(t, err, 404)
}

func TestRequestHandler_Get_ServiceError(t *testing.T) {
	h := NewRequestHandler(&mockRequestService{
		getRequestFn: func(requestID uint) (*models.Request, error) {
			return nil, fmt.Errorf("db error")
		},
	}, nil)

	_, err := h.GetRequestHandler(requestUserCtx(), &GetRequestHandlerRequest{RequestID: "1"})
	assertHumaError(t, err, 500)
}

// ============================================================================
// Tests: ListRequestsHandler
// ============================================================================

func TestRequestHandler_List_Success(t *testing.T) {
	h := NewRequestHandler(&mockRequestService{}, nil)

	resp, err := h.ListRequestsHandler(requestUserCtx(), &ListRequestsHandlerRequest{Limit: 20})
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
	h := NewRequestHandler(&mockRequestService{
		listRequestsFn: func(status string, entityType string, sortBy string, limit, offset int) ([]models.Request, int64, error) {
			receivedLimit = limit
			return []models.Request{}, 0, nil
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
	h := NewRequestHandler(&mockRequestService{
		listRequestsFn: func(status string, entityType string, sortBy string, limit, offset int) ([]models.Request, int64, error) {
			return nil, 0, fmt.Errorf("database error")
		},
	}, nil)

	_, err := h.ListRequestsHandler(requestUserCtx(), &ListRequestsHandlerRequest{})
	assertHumaError(t, err, 500)
}

func TestRequestHandler_List_NoAuth_Succeeds(t *testing.T) {
	// List should work without authentication (public endpoint)
	h := NewRequestHandler(&mockRequestService{}, nil)

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
	h := NewRequestHandler(&mockRequestService{}, nil)

	req := &UpdateRequestHandlerRequest{RequestID: "1"}
	title := "Updated"
	req.Body.Title = &title

	resp, err := h.UpdateRequestHandler(requestUserCtx(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.ID != 1 {
		t.Errorf("expected id=1, got %d", resp.Body.ID)
	}
}

func TestRequestHandler_Update_InvalidID(t *testing.T) {
	h := testRequestHandler()

	req := &UpdateRequestHandlerRequest{RequestID: "abc"}
	_, err := h.UpdateRequestHandler(requestUserCtx(), req)
	assertHumaError(t, err, 400)
}

func TestRequestHandler_Update_ServiceError_NotFound(t *testing.T) {
	h := NewRequestHandler(&mockRequestService{
		updateRequestFn: func(requestID, userID uint, title, description *string) (*models.Request, error) {
			return nil, fmt.Errorf("REQUEST_NOT_FOUND: not found")
		},
	}, nil)

	req := &UpdateRequestHandlerRequest{RequestID: "999"}
	_, err := h.UpdateRequestHandler(requestUserCtx(), req)
	// Falls through to 500 since the plain error doesn't match RequestError type
	assertHumaError(t, err, 500)
}

// ============================================================================
// Tests: DeleteRequestHandler
// ============================================================================

func TestRequestHandler_Delete_Success(t *testing.T) {
	h := NewRequestHandler(&mockRequestService{}, nil)

	_, err := h.DeleteRequestHandler(requestUserCtx(), &DeleteRequestHandlerRequest{RequestID: "1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRequestHandler_Delete_InvalidID(t *testing.T) {
	h := testRequestHandler()

	_, err := h.DeleteRequestHandler(requestUserCtx(), &DeleteRequestHandlerRequest{RequestID: "abc"})
	assertHumaError(t, err, 400)
}

func TestRequestHandler_Delete_ServiceError(t *testing.T) {
	h := NewRequestHandler(&mockRequestService{
		deleteRequestFn: func(requestID, userID uint, isAdmin bool) error {
			return fmt.Errorf("db error")
		},
	}, nil)

	_, err := h.DeleteRequestHandler(requestUserCtx(), &DeleteRequestHandlerRequest{RequestID: "1"})
	assertHumaError(t, err, 500)
}

// ============================================================================
// Tests: VoteRequestHandler
// ============================================================================

func TestRequestHandler_Vote_Success(t *testing.T) {
	h := NewRequestHandler(&mockRequestService{}, nil)

	req := &VoteRequestHandlerRequest{RequestID: "1"}
	req.Body.IsUpvote = true

	_, err := h.VoteRequestHandler(requestUserCtx(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRequestHandler_Vote_InvalidID(t *testing.T) {
	h := testRequestHandler()

	_, err := h.VoteRequestHandler(requestUserCtx(), &VoteRequestHandlerRequest{RequestID: "abc"})
	assertHumaError(t, err, 400)
}

func TestRequestHandler_Vote_ServiceError(t *testing.T) {
	h := NewRequestHandler(&mockRequestService{
		voteFn: func(requestID, userID uint, isUpvote bool) error {
			return fmt.Errorf("db error")
		},
	}, nil)

	req := &VoteRequestHandlerRequest{RequestID: "1"}
	req.Body.IsUpvote = true

	_, err := h.VoteRequestHandler(requestUserCtx(), req)
	assertHumaError(t, err, 500)
}

// ============================================================================
// Tests: RemoveVoteRequestHandler
// ============================================================================

func TestRequestHandler_RemoveVote_Success(t *testing.T) {
	h := NewRequestHandler(&mockRequestService{}, nil)

	_, err := h.RemoveVoteRequestHandler(requestUserCtx(), &RemoveVoteRequestHandlerRequest{RequestID: "1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRequestHandler_RemoveVote_InvalidID(t *testing.T) {
	h := testRequestHandler()

	_, err := h.RemoveVoteRequestHandler(requestUserCtx(), &RemoveVoteRequestHandlerRequest{RequestID: "abc"})
	assertHumaError(t, err, 400)
}

// ============================================================================
// Tests: FulfillRequestHandler
// ============================================================================

func TestRequestHandler_Fulfill_Success(t *testing.T) {
	h := NewRequestHandler(&mockRequestService{}, nil)

	_, err := h.FulfillRequestHandler(requestUserCtx(), &FulfillRequestHandlerRequest{RequestID: "1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRequestHandler_Fulfill_InvalidID(t *testing.T) {
	h := testRequestHandler()

	_, err := h.FulfillRequestHandler(requestUserCtx(), &FulfillRequestHandlerRequest{RequestID: "abc"})
	assertHumaError(t, err, 400)
}

func TestRequestHandler_Fulfill_ServiceError(t *testing.T) {
	h := NewRequestHandler(&mockRequestService{
		fulfillRequestFn: func(requestID, fulfillerID uint, fulfilledEntityID *uint) error {
			return fmt.Errorf("db error")
		},
	}, nil)

	_, err := h.FulfillRequestHandler(requestUserCtx(), &FulfillRequestHandlerRequest{RequestID: "1"})
	assertHumaError(t, err, 500)
}

// ============================================================================
// Tests: CloseRequestHandler
// ============================================================================

func TestRequestHandler_Close_Success(t *testing.T) {
	h := NewRequestHandler(&mockRequestService{}, nil)

	_, err := h.CloseRequestHandler(requestUserCtx(), &CloseRequestHandlerRequest{RequestID: "1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRequestHandler_Close_InvalidID(t *testing.T) {
	h := testRequestHandler()

	_, err := h.CloseRequestHandler(requestUserCtx(), &CloseRequestHandlerRequest{RequestID: "abc"})
	assertHumaError(t, err, 400)
}

func TestRequestHandler_Close_ServiceError(t *testing.T) {
	h := NewRequestHandler(&mockRequestService{
		closeRequestFn: func(requestID, userID uint, isAdmin bool) error {
			return fmt.Errorf("db error")
		},
	}, nil)

	_, err := h.CloseRequestHandler(requestUserCtx(), &CloseRequestHandlerRequest{RequestID: "1"})
	assertHumaError(t, err, 500)
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
				assertHumaError(t, result, tc.expectedStatus)
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
	request := &models.Request{
		ID:          1,
		Title:       "Test",
		EntityType:  "artist",
		Status:      models.RequestStatusPending,
		RequesterID: 10,
		Upvotes:     3,
		Downvotes:   1,
		VoteScore:   2,
		Requester:   models.User{ID: 10, Username: &username},
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
	if resp.UserVote == nil || *resp.UserVote != 1 {
		t.Errorf("expected user_vote=1, got %v", resp.UserVote)
	}
	if resp.WilsonScore <= 0 {
		t.Errorf("expected positive wilson_score, got %f", resp.WilsonScore)
	}
}

func TestBuildRequestResponse_NoVote(t *testing.T) {
	request := &models.Request{
		ID:          1,
		Title:       "Test",
		EntityType:  "artist",
		Status:      models.RequestStatusPending,
		RequesterID: 10,
	}

	resp := buildRequestResponse(request, nil)
	if resp.UserVote != nil {
		t.Errorf("expected user_vote=nil, got %v", resp.UserVote)
	}
}

func TestResolveUserDisplayName(t *testing.T) {
	username := "cooluser"
	firstName := "Jane"
	lastName := "Doe"

	tests := []struct {
		name     string
		user     *models.User
		expected string
	}{
		{"username", &models.User{Username: &username}, "cooluser"},
		{"first+last", &models.User{FirstName: &firstName, LastName: &lastName}, "Jane Doe"},
		{"first only", &models.User{FirstName: &firstName}, "Jane"},
		{"empty", &models.User{}, "Unknown"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := resolveUserDisplayName(tc.user)
			if result != tc.expected {
				t.Errorf("expected %q, got %q", tc.expected, result)
			}
		})
	}
}
