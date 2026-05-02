package engagement

import (
	"context"
	"fmt"
	"testing"
	"time"

	"psychic-homily-backend/internal/api/handlers/shared/testhelpers"
	authm "psychic-homily-backend/internal/models/auth"
	"psychic-homily-backend/internal/services/contracts"
)

// ============================================================================
// Test helpers
// ============================================================================

func testCommentHandler() *CommentHandler {
	return NewCommentHandler(nil, nil, nil, nil)
}

func commentUserCtx() context.Context {
	return testhelpers.CtxWithUser(&authm.User{ID: 10, IsAdmin: false})
}

func commentAdminCtx() context.Context {
	return testhelpers.CtxWithUser(&authm.User{ID: 1, IsAdmin: true})
}

func makeCommentResponse(id uint, entityType string, entityID uint, userID uint) *contracts.CommentResponse {
	return &contracts.CommentResponse{
		ID:              id,
		EntityType:      entityType,
		EntityID:        entityID,
		Kind:            "comment",
		UserID:          userID,
		Depth:           0,
		Body:            "Test comment body",
		BodyHTML:        "<p>Test comment body</p>",
		Visibility:      "visible",
		ReplyPermission: "anyone",
		Ups:             0,
		Downs:           0,
		Score:           0,
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
	}
}

func makeReplyResponse(id uint, parentID uint, rootID uint, entityType string, entityID uint, userID uint) *contracts.CommentResponse {
	resp := makeCommentResponse(id, entityType, entityID, userID)
	resp.ParentID = &parentID
	resp.RootID = &rootID
	resp.Depth = 1
	return resp
}

// ============================================================================
// Tests: ListComments — Auth & Validation
// ============================================================================

func TestListComments_InvalidEntityID(t *testing.T) {
	h := testCommentHandler()
	_, err := h.ListCommentsHandler(context.Background(), &ListCommentsRequest{
		EntityType: "show",
		EntityID:   "abc",
	})
	testhelpers.AssertHumaError(t, err, 400)
}

func TestListComments_UnsupportedEntityType(t *testing.T) {
	mock := &testhelpers.MockCommentService{
		ListCommentsForEntityFn: func(entityType string, entityID uint, filters contracts.CommentListFilters) (*contracts.CommentListResponse, error) {
			return nil, fmt.Errorf("unsupported entity type: %s", entityType)
		},
	}
	h := NewCommentHandler(mock, mock, nil, nil)
	_, err := h.ListCommentsHandler(context.Background(), &ListCommentsRequest{
		EntityType: "invalid_type",
		EntityID:   "1",
	})
	testhelpers.AssertHumaError(t, err, 400)
}

func TestListComments_Success(t *testing.T) {
	comments := []*contracts.CommentResponse{makeCommentResponse(1, "show", 5, 10)}
	mock := &testhelpers.MockCommentService{
		ListCommentsForEntityFn: func(entityType string, entityID uint, filters contracts.CommentListFilters) (*contracts.CommentListResponse, error) {
			if entityType != "show" || entityID != 5 {
				t.Errorf("unexpected entity: %s/%d", entityType, entityID)
			}
			if filters.Sort != "new" {
				t.Errorf("expected sort=new, got %s", filters.Sort)
			}
			if filters.Limit != 25 {
				t.Errorf("expected limit=25, got %d", filters.Limit)
			}
			return &contracts.CommentListResponse{
				Comments: comments,
				Total:    1,
				HasMore:  false,
			}, nil
		},
	}
	h := NewCommentHandler(mock, mock, nil, nil)
	resp, err := h.ListCommentsHandler(context.Background(), &ListCommentsRequest{
		EntityType: "show",
		EntityID:   "5",
		Sort:       "new",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Total != 1 {
		t.Errorf("expected total=1, got %d", resp.Body.Total)
	}
	if len(resp.Body.Comments) != 1 {
		t.Errorf("expected 1 comment, got %d", len(resp.Body.Comments))
	}
}

func TestListComments_DefaultLimit(t *testing.T) {
	mock := &testhelpers.MockCommentService{
		ListCommentsForEntityFn: func(entityType string, entityID uint, filters contracts.CommentListFilters) (*contracts.CommentListResponse, error) {
			if filters.Limit != 25 {
				t.Errorf("expected default limit=25, got %d", filters.Limit)
			}
			return &contracts.CommentListResponse{Comments: []*contracts.CommentResponse{}, Total: 0}, nil
		},
	}
	h := NewCommentHandler(mock, mock, nil, nil)
	_, err := h.ListCommentsHandler(context.Background(), &ListCommentsRequest{
		EntityType: "artist",
		EntityID:   "1",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestListComments_LimitCap(t *testing.T) {
	mock := &testhelpers.MockCommentService{
		ListCommentsForEntityFn: func(entityType string, entityID uint, filters contracts.CommentListFilters) (*contracts.CommentListResponse, error) {
			if filters.Limit != 100 {
				t.Errorf("expected capped limit=100, got %d", filters.Limit)
			}
			return &contracts.CommentListResponse{Comments: []*contracts.CommentResponse{}, Total: 0}, nil
		},
	}
	h := NewCommentHandler(mock, mock, nil, nil)
	_, err := h.ListCommentsHandler(context.Background(), &ListCommentsRequest{
		EntityType: "artist",
		EntityID:   "1",
		Limit:      500,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestListComments_ServiceError(t *testing.T) {
	mock := &testhelpers.MockCommentService{
		ListCommentsForEntityFn: func(entityType string, entityID uint, filters contracts.CommentListFilters) (*contracts.CommentListResponse, error) {
			return nil, fmt.Errorf("database error")
		},
	}
	h := NewCommentHandler(mock, mock, nil, nil)
	_, err := h.ListCommentsHandler(context.Background(), &ListCommentsRequest{
		EntityType: "show",
		EntityID:   "1",
	})
	testhelpers.AssertHumaError(t, err, 500)
}

func TestListComments_PopulatesUserVote_WhenAuthenticated(t *testing.T) {
	up := 1
	c1 := makeCommentResponse(1, "show", 5, 10)
	c2 := makeCommentResponse(2, "show", 5, 11)
	commentSvc := &testhelpers.MockCommentService{
		ListCommentsForEntityFn: func(_ string, _ uint, _ contracts.CommentListFilters) (*contracts.CommentListResponse, error) {
			return &contracts.CommentListResponse{
				Comments: []*contracts.CommentResponse{c1, c2},
				Total:    2,
			}, nil
		},
	}
	var receivedUserID uint
	var receivedIDs []uint
	voteSvc := &testhelpers.MockCommentVoteService{
		GetUserVotesForCommentsFn: func(userID uint, ids []uint) (map[uint]int, error) {
			receivedUserID = userID
			receivedIDs = ids
			return map[uint]int{1: up}, nil
		},
	}
	h := NewCommentHandler(commentSvc, commentSvc, voteSvc, nil)
	resp, err := h.ListCommentsHandler(commentUserCtx(), &ListCommentsRequest{
		EntityType: "show",
		EntityID:   "5",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if receivedUserID != 10 {
		t.Errorf("expected voteSvc called with userID=10, got %d", receivedUserID)
	}
	if len(receivedIDs) != 2 {
		t.Errorf("expected voteSvc called with 2 ids, got %d", len(receivedIDs))
	}
	if resp.Body.Comments[0].UserVote == nil || *resp.Body.Comments[0].UserVote != 1 {
		t.Errorf("expected UserVote=1 on comment 1, got %v", resp.Body.Comments[0].UserVote)
	}
	if resp.Body.Comments[1].UserVote != nil {
		t.Errorf("expected UserVote=nil on comment 2 (not voted), got %v", resp.Body.Comments[1].UserVote)
	}
}

func TestListComments_DoesNotPopulateUserVote_WhenAnonymous(t *testing.T) {
	c1 := makeCommentResponse(1, "show", 5, 10)
	commentSvc := &testhelpers.MockCommentService{
		ListCommentsForEntityFn: func(_ string, _ uint, _ contracts.CommentListFilters) (*contracts.CommentListResponse, error) {
			return &contracts.CommentListResponse{
				Comments: []*contracts.CommentResponse{c1},
				Total:    1,
			}, nil
		},
	}
	voteCalled := false
	voteSvc := &testhelpers.MockCommentVoteService{
		GetUserVotesForCommentsFn: func(_ uint, _ []uint) (map[uint]int, error) {
			voteCalled = true
			return nil, nil
		},
	}
	h := NewCommentHandler(commentSvc, commentSvc, voteSvc, nil)
	resp, err := h.ListCommentsHandler(context.Background(), &ListCommentsRequest{
		EntityType: "show",
		EntityID:   "5",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if voteCalled {
		t.Error("expected voteSvc not to be called for anonymous request")
	}
	if resp.Body.Comments[0].UserVote != nil {
		t.Errorf("expected UserVote=nil for anonymous request, got %v", resp.Body.Comments[0].UserVote)
	}
}

func TestListComments_SwallowsVoteLookupError(t *testing.T) {
	c1 := makeCommentResponse(1, "show", 5, 10)
	commentSvc := &testhelpers.MockCommentService{
		ListCommentsForEntityFn: func(_ string, _ uint, _ contracts.CommentListFilters) (*contracts.CommentListResponse, error) {
			return &contracts.CommentListResponse{Comments: []*contracts.CommentResponse{c1}, Total: 1}, nil
		},
	}
	voteSvc := &testhelpers.MockCommentVoteService{
		GetUserVotesForCommentsFn: func(_ uint, _ []uint) (map[uint]int, error) {
			return nil, fmt.Errorf("vote lookup failed")
		},
	}
	h := NewCommentHandler(commentSvc, commentSvc, voteSvc, nil)
	resp, err := h.ListCommentsHandler(commentUserCtx(), &ListCommentsRequest{
		EntityType: "show",
		EntityID:   "5",
	})
	if err != nil {
		t.Fatalf("expected vote-lookup error to be swallowed, got %v", err)
	}
	if resp.Body.Comments[0].UserVote != nil {
		t.Errorf("expected UserVote=nil on vote-lookup failure, got %v", resp.Body.Comments[0].UserVote)
	}
}

// ============================================================================
// Tests: GetComment
// ============================================================================

func TestGetComment_InvalidID(t *testing.T) {
	h := testCommentHandler()
	_, err := h.GetCommentHandler(context.Background(), &GetCommentRequest{CommentID: "abc"})
	testhelpers.AssertHumaError(t, err, 400)
}

func TestGetComment_NotFound(t *testing.T) {
	mock := &testhelpers.MockCommentService{
		GetCommentFn: func(commentID uint) (*contracts.CommentResponse, error) {
			return nil, fmt.Errorf("comment not found")
		},
	}
	h := NewCommentHandler(mock, mock, nil, nil)
	_, err := h.GetCommentHandler(context.Background(), &GetCommentRequest{CommentID: "99"})
	testhelpers.AssertHumaError(t, err, 404)
}

func TestGetComment_Success(t *testing.T) {
	expected := makeCommentResponse(1, "show", 5, 10)
	mock := &testhelpers.MockCommentService{
		GetCommentFn: func(commentID uint) (*contracts.CommentResponse, error) {
			if commentID != 1 {
				t.Errorf("expected commentID=1, got %d", commentID)
			}
			return expected, nil
		},
	}
	h := NewCommentHandler(mock, mock, nil, nil)
	resp, err := h.GetCommentHandler(context.Background(), &GetCommentRequest{CommentID: "1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.ID != 1 {
		t.Errorf("expected ID=1, got %d", resp.Body.ID)
	}
}

// ============================================================================
// Tests: GetThread
// ============================================================================

func TestGetThread_InvalidID(t *testing.T) {
	h := testCommentHandler()
	_, err := h.GetThreadHandler(context.Background(), &GetThreadRequest{CommentID: "abc"})
	testhelpers.AssertHumaError(t, err, 400)
}

func TestGetThread_NotFound(t *testing.T) {
	mock := &testhelpers.MockCommentService{
		GetThreadFn: func(rootID uint) ([]*contracts.CommentResponse, error) {
			return nil, fmt.Errorf("thread root comment not found")
		},
	}
	h := NewCommentHandler(mock, mock, nil, nil)
	_, err := h.GetThreadHandler(context.Background(), &GetThreadRequest{CommentID: "99"})
	testhelpers.AssertHumaError(t, err, 404)
}

func TestGetThread_NotARoot(t *testing.T) {
	mock := &testhelpers.MockCommentService{
		GetThreadFn: func(rootID uint) ([]*contracts.CommentResponse, error) {
			return nil, fmt.Errorf("comment is not a thread root")
		},
	}
	h := NewCommentHandler(mock, mock, nil, nil)
	_, err := h.GetThreadHandler(context.Background(), &GetThreadRequest{CommentID: "5"})
	testhelpers.AssertHumaError(t, err, 400)
}

func TestGetThread_Success(t *testing.T) {
	root := makeCommentResponse(1, "show", 5, 10)
	reply := makeReplyResponse(2, 1, 1, "show", 5, 11)
	mock := &testhelpers.MockCommentService{
		GetThreadFn: func(rootID uint) ([]*contracts.CommentResponse, error) {
			if rootID != 1 {
				t.Errorf("expected rootID=1, got %d", rootID)
			}
			return []*contracts.CommentResponse{root, reply}, nil
		},
	}
	h := NewCommentHandler(mock, mock, nil, nil)
	resp, err := h.GetThreadHandler(context.Background(), &GetThreadRequest{CommentID: "1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Body.Comments) != 2 {
		t.Errorf("expected 2 comments, got %d", len(resp.Body.Comments))
	}
}

// ============================================================================
// Tests: CreateComment — Auth & Validation
// ============================================================================

func TestCreateComment_NoUser(t *testing.T) {
	h := testCommentHandler()
	_, err := h.CreateCommentHandler(context.Background(), &CreateCommentRequest{
		EntityType: "show",
		EntityID:   "1",
	})
	testhelpers.AssertHumaError(t, err, 401)
}

func TestCreateComment_InvalidEntityID(t *testing.T) {
	h := testCommentHandler()
	_, err := h.CreateCommentHandler(commentUserCtx(), &CreateCommentRequest{
		EntityType: "show",
		EntityID:   "abc",
	})
	testhelpers.AssertHumaError(t, err, 400)
}

func TestCreateComment_EmptyBody(t *testing.T) {
	h := testCommentHandler()
	req := &CreateCommentRequest{EntityType: "show", EntityID: "1"}
	req.Body.Body = "   "
	_, err := h.CreateCommentHandler(commentUserCtx(), req)
	testhelpers.AssertHumaError(t, err, 400)
}

func TestCreateComment_UnsupportedEntityType(t *testing.T) {
	mock := &testhelpers.MockCommentService{
		CreateCommentFn: func(userID uint, req *contracts.CreateCommentRequest) (*contracts.CommentResponse, error) {
			return nil, fmt.Errorf("unsupported entity type: nope")
		},
	}
	h := NewCommentHandler(mock, mock, nil, nil)
	req := &CreateCommentRequest{EntityType: "nope", EntityID: "1"}
	req.Body.Body = "Hello"
	_, err := h.CreateCommentHandler(commentUserCtx(), req)
	testhelpers.AssertHumaError(t, err, 400)
}

func TestCreateComment_EntityNotFound(t *testing.T) {
	mock := &testhelpers.MockCommentService{
		CreateCommentFn: func(userID uint, req *contracts.CreateCommentRequest) (*contracts.CommentResponse, error) {
			return nil, fmt.Errorf("show with ID 999 not found")
		},
	}
	h := NewCommentHandler(mock, mock, nil, nil)
	req := &CreateCommentRequest{EntityType: "show", EntityID: "999"}
	req.Body.Body = "Hello"
	_, err := h.CreateCommentHandler(commentUserCtx(), req)
	testhelpers.AssertHumaError(t, err, 404)
}

func TestCreateComment_Success(t *testing.T) {
	expected := makeCommentResponse(1, "show", 5, 10)
	mock := &testhelpers.MockCommentService{
		CreateCommentFn: func(userID uint, req *contracts.CreateCommentRequest) (*contracts.CommentResponse, error) {
			if userID != 10 {
				t.Errorf("expected userID=10, got %d", userID)
			}
			if req.EntityType != "show" || req.EntityID != 5 {
				t.Errorf("unexpected entity: %s/%d", req.EntityType, req.EntityID)
			}
			if req.Body != "Great show!" {
				t.Errorf("unexpected body: %s", req.Body)
			}
			return expected, nil
		},
	}
	h := NewCommentHandler(mock, mock, nil, &testhelpers.MockAuditLogService{})
	req := &CreateCommentRequest{EntityType: "show", EntityID: "5"}
	req.Body.Body = "Great show!"
	resp, err := h.CreateCommentHandler(commentUserCtx(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.ID != 1 {
		t.Errorf("expected ID=1, got %d", resp.Body.ID)
	}
}

func TestCreateComment_WithReplyPermission(t *testing.T) {
	mock := &testhelpers.MockCommentService{
		CreateCommentFn: func(userID uint, req *contracts.CreateCommentRequest) (*contracts.CommentResponse, error) {
			if req.ReplyPermission != "author_only" {
				t.Errorf("expected reply_permission=author_only, got %s", req.ReplyPermission)
			}
			return makeCommentResponse(1, "show", 5, 10), nil
		},
	}
	h := NewCommentHandler(mock, mock, nil, nil)
	req := &CreateCommentRequest{EntityType: "show", EntityID: "5"}
	req.Body.Body = "My thoughts"
	req.Body.ReplyPermission = "author_only"
	_, err := h.CreateCommentHandler(commentUserCtx(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCreateComment_ServiceError(t *testing.T) {
	mock := &testhelpers.MockCommentService{
		CreateCommentFn: func(userID uint, req *contracts.CreateCommentRequest) (*contracts.CommentResponse, error) {
			return nil, fmt.Errorf("database error")
		},
	}
	h := NewCommentHandler(mock, mock, nil, nil)
	req := &CreateCommentRequest{EntityType: "show", EntityID: "1"}
	req.Body.Body = "Hello"
	_, err := h.CreateCommentHandler(commentUserCtx(), req)
	testhelpers.AssertHumaError(t, err, 500)
}

// ============================================================================
// Tests: CreateReply — Auth & Validation
// ============================================================================

func TestCreateReply_NoUser(t *testing.T) {
	h := testCommentHandler()
	_, err := h.CreateReplyHandler(context.Background(), &CreateReplyRequest{CommentID: "1"})
	testhelpers.AssertHumaError(t, err, 401)
}

func TestCreateReply_InvalidCommentID(t *testing.T) {
	h := testCommentHandler()
	_, err := h.CreateReplyHandler(commentUserCtx(), &CreateReplyRequest{CommentID: "abc"})
	testhelpers.AssertHumaError(t, err, 400)
}

func TestCreateReply_EmptyBody(t *testing.T) {
	h := testCommentHandler()
	req := &CreateReplyRequest{CommentID: "1"}
	req.Body.Body = ""
	_, err := h.CreateReplyHandler(commentUserCtx(), req)
	testhelpers.AssertHumaError(t, err, 400)
}

func TestCreateReply_ParentNotFound(t *testing.T) {
	mock := &testhelpers.MockCommentService{
		GetCommentFn: func(commentID uint) (*contracts.CommentResponse, error) {
			return nil, fmt.Errorf("comment not found")
		},
	}
	h := NewCommentHandler(mock, mock, nil, nil)
	req := &CreateReplyRequest{CommentID: "99"}
	req.Body.Body = "Replying..."
	_, err := h.CreateReplyHandler(commentUserCtx(), req)
	testhelpers.AssertHumaError(t, err, 404)
}

func TestCreateReply_MaxDepthExceeded(t *testing.T) {
	parent := makeCommentResponse(1, "show", 5, 10)
	mock := &testhelpers.MockCommentService{
		GetCommentFn: func(commentID uint) (*contracts.CommentResponse, error) {
			return parent, nil
		},
		CreateCommentFn: func(userID uint, req *contracts.CreateCommentRequest) (*contracts.CommentResponse, error) {
			return nil, fmt.Errorf("maximum reply depth of 2 exceeded")
		},
	}
	h := NewCommentHandler(mock, mock, nil, nil)
	req := &CreateReplyRequest{CommentID: "1"}
	req.Body.Body = "Deep reply"
	_, err := h.CreateReplyHandler(commentUserCtx(), req)
	testhelpers.AssertHumaError(t, err, 400)
}

func TestCreateReply_Success(t *testing.T) {
	parent := makeCommentResponse(1, "show", 5, 10)
	reply := makeReplyResponse(2, 1, 1, "show", 5, 10)
	mock := &testhelpers.MockCommentService{
		GetCommentFn: func(commentID uint) (*contracts.CommentResponse, error) {
			return parent, nil
		},
		CreateCommentFn: func(userID uint, req *contracts.CreateCommentRequest) (*contracts.CommentResponse, error) {
			if req.ParentID == nil || *req.ParentID != 1 {
				t.Errorf("expected parent_id=1")
			}
			if req.EntityType != "show" || req.EntityID != 5 {
				t.Errorf("expected entity_type=show, entity_id=5")
			}
			return reply, nil
		},
	}
	h := NewCommentHandler(mock, mock, nil, &testhelpers.MockAuditLogService{})
	req := &CreateReplyRequest{CommentID: "1"}
	req.Body.Body = "Nice reply!"
	resp, err := h.CreateReplyHandler(commentUserCtx(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.ID != 2 {
		t.Errorf("expected ID=2, got %d", resp.Body.ID)
	}
	if resp.Body.ParentID == nil || *resp.Body.ParentID != 1 {
		t.Errorf("expected parent_id=1")
	}
}

// ============================================================================
// Tests: UpdateComment — Auth & Validation
// ============================================================================

func TestUpdateComment_NoUser(t *testing.T) {
	h := testCommentHandler()
	_, err := h.UpdateCommentHandler(context.Background(), &UpdateCommentRequest{CommentID: "1"})
	testhelpers.AssertHumaError(t, err, 401)
}

func TestUpdateComment_InvalidID(t *testing.T) {
	h := testCommentHandler()
	_, err := h.UpdateCommentHandler(commentUserCtx(), &UpdateCommentRequest{CommentID: "abc"})
	testhelpers.AssertHumaError(t, err, 400)
}

func TestUpdateComment_EmptyBody(t *testing.T) {
	h := testCommentHandler()
	req := &UpdateCommentRequest{CommentID: "1"}
	req.Body.Body = "  "
	_, err := h.UpdateCommentHandler(commentUserCtx(), req)
	testhelpers.AssertHumaError(t, err, 400)
}

func TestUpdateComment_NotFound(t *testing.T) {
	mock := &testhelpers.MockCommentService{
		UpdateCommentFn: func(userID uint, commentID uint, req *contracts.UpdateCommentRequest) (*contracts.CommentResponse, error) {
			return nil, fmt.Errorf("comment not found")
		},
	}
	h := NewCommentHandler(mock, mock, nil, nil)
	req := &UpdateCommentRequest{CommentID: "99"}
	req.Body.Body = "Updated text"
	_, err := h.UpdateCommentHandler(commentUserCtx(), req)
	testhelpers.AssertHumaError(t, err, 404)
}

func TestUpdateComment_ForbiddenNotAuthor(t *testing.T) {
	mock := &testhelpers.MockCommentService{
		UpdateCommentFn: func(userID uint, commentID uint, req *contracts.UpdateCommentRequest) (*contracts.CommentResponse, error) {
			return nil, fmt.Errorf("only the comment author can edit this comment")
		},
	}
	h := NewCommentHandler(mock, mock, nil, nil)
	req := &UpdateCommentRequest{CommentID: "1"}
	req.Body.Body = "Trying to edit someone else's comment"
	_, err := h.UpdateCommentHandler(commentUserCtx(), req)
	testhelpers.AssertHumaError(t, err, 403)
}

func TestUpdateComment_Success(t *testing.T) {
	updated := makeCommentResponse(1, "show", 5, 10)
	updated.Body = "Updated body"
	updated.IsEdited = true
	updated.EditCount = 1
	mock := &testhelpers.MockCommentService{
		UpdateCommentFn: func(userID uint, commentID uint, req *contracts.UpdateCommentRequest) (*contracts.CommentResponse, error) {
			if userID != 10 {
				t.Errorf("expected userID=10, got %d", userID)
			}
			if commentID != 1 {
				t.Errorf("expected commentID=1, got %d", commentID)
			}
			if req.Body != "Updated body" {
				t.Errorf("unexpected body: %s", req.Body)
			}
			return updated, nil
		},
	}
	h := NewCommentHandler(mock, mock, nil, &testhelpers.MockAuditLogService{})
	req := &UpdateCommentRequest{CommentID: "1"}
	req.Body.Body = "Updated body"
	resp, err := h.UpdateCommentHandler(commentUserCtx(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Body != "Updated body" {
		t.Errorf("expected updated body, got %s", resp.Body.Body)
	}
	if !resp.Body.IsEdited {
		t.Error("expected is_edited=true")
	}
}

// ============================================================================
// Tests: DeleteComment — Auth & Validation
// ============================================================================

func TestDeleteComment_NoUser(t *testing.T) {
	h := testCommentHandler()
	_, err := h.DeleteCommentHandler(context.Background(), &DeleteCommentRequest{CommentID: "1"})
	testhelpers.AssertHumaError(t, err, 401)
}

func TestDeleteComment_InvalidID(t *testing.T) {
	h := testCommentHandler()
	_, err := h.DeleteCommentHandler(commentUserCtx(), &DeleteCommentRequest{CommentID: "abc"})
	testhelpers.AssertHumaError(t, err, 400)
}

func TestDeleteComment_NotFound(t *testing.T) {
	mock := &testhelpers.MockCommentService{
		DeleteCommentFn: func(userID uint, commentID uint, isAdmin bool) error {
			return fmt.Errorf("comment not found")
		},
	}
	h := NewCommentHandler(mock, mock, nil, nil)
	_, err := h.DeleteCommentHandler(commentUserCtx(), &DeleteCommentRequest{CommentID: "99"})
	testhelpers.AssertHumaError(t, err, 404)
}

func TestDeleteComment_ForbiddenNotAuthorOrAdmin(t *testing.T) {
	mock := &testhelpers.MockCommentService{
		DeleteCommentFn: func(userID uint, commentID uint, isAdmin bool) error {
			return fmt.Errorf("only the comment author or an admin can delete this comment")
		},
	}
	h := NewCommentHandler(mock, mock, nil, nil)
	_, err := h.DeleteCommentHandler(commentUserCtx(), &DeleteCommentRequest{CommentID: "1"})
	testhelpers.AssertHumaError(t, err, 403)
}

func TestDeleteComment_SuccessOwn(t *testing.T) {
	mock := &testhelpers.MockCommentService{
		DeleteCommentFn: func(userID uint, commentID uint, isAdmin bool) error {
			if userID != 10 {
				t.Errorf("expected userID=10, got %d", userID)
			}
			if commentID != 1 {
				t.Errorf("expected commentID=1, got %d", commentID)
			}
			if isAdmin {
				t.Error("expected isAdmin=false")
			}
			return nil
		},
	}
	h := NewCommentHandler(mock, mock, nil, &testhelpers.MockAuditLogService{})
	_, err := h.DeleteCommentHandler(commentUserCtx(), &DeleteCommentRequest{CommentID: "1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDeleteComment_SuccessAdmin(t *testing.T) {
	mock := &testhelpers.MockCommentService{
		DeleteCommentFn: func(userID uint, commentID uint, isAdmin bool) error {
			if !isAdmin {
				t.Error("expected isAdmin=true for admin delete")
			}
			return nil
		},
	}
	h := NewCommentHandler(mock, mock, nil, &testhelpers.MockAuditLogService{})
	_, err := h.DeleteCommentHandler(commentAdminCtx(), &DeleteCommentRequest{CommentID: "1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDeleteComment_ServiceError(t *testing.T) {
	mock := &testhelpers.MockCommentService{
		DeleteCommentFn: func(userID uint, commentID uint, isAdmin bool) error {
			return fmt.Errorf("database error")
		},
	}
	h := NewCommentHandler(mock, mock, nil, nil)
	_, err := h.DeleteCommentHandler(commentUserCtx(), &DeleteCommentRequest{CommentID: "1"})
	testhelpers.AssertHumaError(t, err, 500)
}

// ============================================================================
// Tests: UpdateReplyPermission — PSY-296
// ============================================================================

func TestUpdateReplyPermission_NoUser(t *testing.T) {
	h := testCommentHandler()
	req := &UpdateReplyPermissionRequest{CommentID: "1"}
	req.Body.Permission = "anyone"
	_, err := h.UpdateReplyPermissionHandler(context.Background(), req)
	testhelpers.AssertHumaError(t, err, 401)
}

func TestUpdateReplyPermission_InvalidID(t *testing.T) {
	h := testCommentHandler()
	req := &UpdateReplyPermissionRequest{CommentID: "abc"}
	req.Body.Permission = "anyone"
	_, err := h.UpdateReplyPermissionHandler(commentUserCtx(), req)
	testhelpers.AssertHumaError(t, err, 400)
}

func TestUpdateReplyPermission_EmptyPermission(t *testing.T) {
	h := testCommentHandler()
	req := &UpdateReplyPermissionRequest{CommentID: "1"}
	req.Body.Permission = "   "
	_, err := h.UpdateReplyPermissionHandler(commentUserCtx(), req)
	testhelpers.AssertHumaError(t, err, 400)
}

func TestUpdateReplyPermission_InvalidEnum(t *testing.T) {
	mock := &testhelpers.MockCommentService{
		UpdateReplyPermissionFn: func(userID, commentID uint, permission string) (*contracts.CommentResponse, error) {
			return nil, fmt.Errorf("invalid reply_permission: %s", permission)
		},
	}
	h := NewCommentHandler(mock, mock, nil, nil)
	req := &UpdateReplyPermissionRequest{CommentID: "1"}
	req.Body.Permission = "banana"
	_, err := h.UpdateReplyPermissionHandler(commentUserCtx(), req)
	testhelpers.AssertHumaError(t, err, 400)
}

func TestUpdateReplyPermission_Forbidden(t *testing.T) {
	mock := &testhelpers.MockCommentService{
		UpdateReplyPermissionFn: func(userID, commentID uint, permission string) (*contracts.CommentResponse, error) {
			return nil, fmt.Errorf("only the comment author can change reply permission")
		},
	}
	h := NewCommentHandler(mock, mock, nil, nil)
	req := &UpdateReplyPermissionRequest{CommentID: "1"}
	req.Body.Permission = "author_only"
	_, err := h.UpdateReplyPermissionHandler(commentUserCtx(), req)
	testhelpers.AssertHumaError(t, err, 403)
}

func TestUpdateReplyPermission_NotFound(t *testing.T) {
	mock := &testhelpers.MockCommentService{
		UpdateReplyPermissionFn: func(userID, commentID uint, permission string) (*contracts.CommentResponse, error) {
			return nil, fmt.Errorf("comment not found")
		},
	}
	h := NewCommentHandler(mock, mock, nil, nil)
	req := &UpdateReplyPermissionRequest{CommentID: "99"}
	req.Body.Permission = "followers"
	_, err := h.UpdateReplyPermissionHandler(commentUserCtx(), req)
	testhelpers.AssertHumaError(t, err, 404)
}

func TestUpdateReplyPermission_Success(t *testing.T) {
	updated := makeCommentResponse(1, "show", 5, 10)
	updated.ReplyPermission = "followers"
	mock := &testhelpers.MockCommentService{
		UpdateReplyPermissionFn: func(userID, commentID uint, permission string) (*contracts.CommentResponse, error) {
			if userID != 10 {
				t.Errorf("expected userID=10, got %d", userID)
			}
			if commentID != 1 {
				t.Errorf("expected commentID=1, got %d", commentID)
			}
			if permission != "followers" {
				t.Errorf("expected permission=followers, got %q", permission)
			}
			return updated, nil
		},
	}
	h := NewCommentHandler(mock, mock, nil, &testhelpers.MockAuditLogService{})
	req := &UpdateReplyPermissionRequest{CommentID: "1"}
	req.Body.Permission = "followers"
	resp, err := h.UpdateReplyPermissionHandler(commentUserCtx(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.ReplyPermission != "followers" {
		t.Errorf("expected reply_permission=followers, got %q", resp.Body.ReplyPermission)
	}
}

// ============================================================================
// Tests: CreateReply — reply-gate error propagation (PSY-296)
// ============================================================================

func TestCreateReply_RepliesDisabled(t *testing.T) {
	reader := &testhelpers.MockCommentService{
		GetCommentFn: func(id uint) (*contracts.CommentResponse, error) {
			return makeCommentResponse(id, "show", 5, 99), nil
		},
	}
	writer := &testhelpers.MockCommentService{
		CreateCommentFn: func(uid uint, req *contracts.CreateCommentRequest) (*contracts.CommentResponse, error) {
			return nil, fmt.Errorf("replies to this comment are disabled")
		},
	}
	h := NewCommentHandler(reader, writer, nil, nil)
	req := &CreateReplyRequest{CommentID: "1"}
	req.Body.Body = "trying to reply"
	_, err := h.CreateReplyHandler(commentUserCtx(), req)
	testhelpers.AssertHumaError(t, err, 403)
}

func TestCreateReply_FollowersOnlyRejected(t *testing.T) {
	reader := &testhelpers.MockCommentService{
		GetCommentFn: func(id uint) (*contracts.CommentResponse, error) {
			return makeCommentResponse(id, "show", 5, 99), nil
		},
	}
	writer := &testhelpers.MockCommentService{
		CreateCommentFn: func(uid uint, req *contracts.CreateCommentRequest) (*contracts.CommentResponse, error) {
			return nil, fmt.Errorf("only followers of the author can reply to this comment")
		},
	}
	h := NewCommentHandler(reader, writer, nil, nil)
	req := &CreateReplyRequest{CommentID: "1"}
	req.Body.Body = "trying to reply"
	_, err := h.CreateReplyHandler(commentUserCtx(), req)
	testhelpers.AssertHumaError(t, err, 403)
}
