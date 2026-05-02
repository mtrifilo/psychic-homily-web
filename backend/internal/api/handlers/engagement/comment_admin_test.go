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

func testCommentAdminHandler() *CommentAdminHandler {
	return NewCommentAdminHandler(nil, nil)
}

func commentAdminAdminCtx() context.Context {
	return testhelpers.CtxWithUser(&authm.User{ID: 1, IsAdmin: true})
}

func commentAdminUserCtx() context.Context {
	return testhelpers.CtxWithUser(&authm.User{ID: 10, IsAdmin: false})
}

// ============================================================================
// Tests: Hide Comment — Auth & Validation
// ============================================================================

func TestAdminHideComment_RequiresAdmin(t *testing.T) {
	h := testCommentAdminHandler()

	t.Run("NoUser", func(t *testing.T) {
		req := &AdminHideCommentRequest{CommentID: "1"}
		req.Body.Reason = "spam"
		_, err := h.AdminHideCommentHandler(context.Background(), req)
		testhelpers.AssertHumaError(t, err, 403)
	})
	t.Run("NonAdmin", func(t *testing.T) {
		req := &AdminHideCommentRequest{CommentID: "1"}
		req.Body.Reason = "spam"
		_, err := h.AdminHideCommentHandler(commentAdminUserCtx(), req)
		testhelpers.AssertHumaError(t, err, 403)
	})
}

func TestAdminHideComment_InvalidID(t *testing.T) {
	h := testCommentAdminHandler()
	req := &AdminHideCommentRequest{CommentID: "abc"}
	req.Body.Reason = "spam"
	_, err := h.AdminHideCommentHandler(commentAdminAdminCtx(), req)
	testhelpers.AssertHumaError(t, err, 400)
}

func TestAdminHideComment_EmptyReason(t *testing.T) {
	h := testCommentAdminHandler()
	req := &AdminHideCommentRequest{CommentID: "1"}
	req.Body.Reason = ""
	_, err := h.AdminHideCommentHandler(commentAdminAdminCtx(), req)
	testhelpers.AssertHumaError(t, err, 400)
}

func TestAdminHideComment_NotFound(t *testing.T) {
	h := NewCommentAdminHandler(
		&testhelpers.MockCommentAdminService{
			HideCommentFn: func(adminUserID, commentID uint, reason string) error {
				return fmt.Errorf("comment not found")
			},
		},
		nil,
	)
	req := &AdminHideCommentRequest{CommentID: "99"}
	req.Body.Reason = "spam"
	_, err := h.AdminHideCommentHandler(commentAdminAdminCtx(), req)
	testhelpers.AssertHumaError(t, err, 404)
}

func TestAdminHideComment_Success(t *testing.T) {
	h := NewCommentAdminHandler(
		&testhelpers.MockCommentAdminService{
			HideCommentFn: func(adminUserID, commentID uint, reason string) error {
				if adminUserID != 1 || commentID != 5 || reason != "violates guidelines" {
					t.Errorf("unexpected params: admin=%d, comment=%d, reason=%s", adminUserID, commentID, reason)
				}
				return nil
			},
		},
		&testhelpers.MockAuditLogService{},
	)
	req := &AdminHideCommentRequest{CommentID: "5"}
	req.Body.Reason = "violates guidelines"
	_, err := h.AdminHideCommentHandler(commentAdminAdminCtx(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// ============================================================================
// Tests: Restore Comment — Auth & Validation
// ============================================================================

func TestAdminRestoreComment_RequiresAdmin(t *testing.T) {
	h := testCommentAdminHandler()

	t.Run("NoUser", func(t *testing.T) {
		_, err := h.AdminRestoreCommentHandler(context.Background(), &AdminRestoreCommentRequest{CommentID: "1"})
		testhelpers.AssertHumaError(t, err, 403)
	})
	t.Run("NonAdmin", func(t *testing.T) {
		_, err := h.AdminRestoreCommentHandler(commentAdminUserCtx(), &AdminRestoreCommentRequest{CommentID: "1"})
		testhelpers.AssertHumaError(t, err, 403)
	})
}

func TestAdminRestoreComment_InvalidID(t *testing.T) {
	h := testCommentAdminHandler()
	_, err := h.AdminRestoreCommentHandler(commentAdminAdminCtx(), &AdminRestoreCommentRequest{CommentID: "abc"})
	testhelpers.AssertHumaError(t, err, 400)
}

func TestAdminRestoreComment_NotFound(t *testing.T) {
	h := NewCommentAdminHandler(
		&testhelpers.MockCommentAdminService{
			RestoreCommentFn: func(adminUserID, commentID uint) error {
				return fmt.Errorf("comment not found")
			},
		},
		nil,
	)
	_, err := h.AdminRestoreCommentHandler(commentAdminAdminCtx(), &AdminRestoreCommentRequest{CommentID: "99"})
	testhelpers.AssertHumaError(t, err, 404)
}

func TestAdminRestoreComment_AlreadyVisible(t *testing.T) {
	h := NewCommentAdminHandler(
		&testhelpers.MockCommentAdminService{
			RestoreCommentFn: func(adminUserID, commentID uint) error {
				return fmt.Errorf("comment is already visible")
			},
		},
		nil,
	)
	_, err := h.AdminRestoreCommentHandler(commentAdminAdminCtx(), &AdminRestoreCommentRequest{CommentID: "1"})
	testhelpers.AssertHumaError(t, err, 409)
}

func TestAdminRestoreComment_Success(t *testing.T) {
	h := NewCommentAdminHandler(
		&testhelpers.MockCommentAdminService{
			RestoreCommentFn: func(adminUserID, commentID uint) error {
				if adminUserID != 1 || commentID != 5 {
					t.Errorf("unexpected params: admin=%d, comment=%d", adminUserID, commentID)
				}
				return nil
			},
		},
		&testhelpers.MockAuditLogService{},
	)
	_, err := h.AdminRestoreCommentHandler(commentAdminAdminCtx(), &AdminRestoreCommentRequest{CommentID: "5"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// ============================================================================
// Tests: List Pending Comments — Auth & Pagination
// ============================================================================

func TestAdminListPendingComments_RequiresAdmin(t *testing.T) {
	h := testCommentAdminHandler()

	t.Run("NoUser", func(t *testing.T) {
		_, err := h.AdminListPendingCommentsHandler(context.Background(), &AdminListPendingCommentsRequest{})
		testhelpers.AssertHumaError(t, err, 403)
	})
	t.Run("NonAdmin", func(t *testing.T) {
		_, err := h.AdminListPendingCommentsHandler(commentAdminUserCtx(), &AdminListPendingCommentsRequest{})
		testhelpers.AssertHumaError(t, err, 403)
	})
}

func TestAdminListPendingComments_Success(t *testing.T) {
	pendingComments := []*contracts.CommentResponse{
		{
			ID:         1,
			EntityType: "artist",
			EntityID:   10,
			Kind:       "comment",
			UserID:     5,
			Body:       "Pending comment",
			Visibility: "pending_review",
			CreatedAt:  time.Now(),
			UpdatedAt:  time.Now(),
		},
	}
	h := NewCommentAdminHandler(
		&testhelpers.MockCommentAdminService{
			ListPendingCommentsFn: func(limit, offset int) ([]*contracts.CommentResponse, int64, error) {
				if limit != 20 || offset != 0 {
					t.Errorf("unexpected pagination: limit=%d, offset=%d", limit, offset)
				}
				return pendingComments, 1, nil
			},
		},
		nil,
	)
	resp, err := h.AdminListPendingCommentsHandler(commentAdminAdminCtx(), &AdminListPendingCommentsRequest{})
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

func TestAdminListPendingComments_ServiceError(t *testing.T) {
	h := NewCommentAdminHandler(
		&testhelpers.MockCommentAdminService{
			ListPendingCommentsFn: func(limit, offset int) ([]*contracts.CommentResponse, int64, error) {
				return nil, 0, fmt.Errorf("database error")
			},
		},
		nil,
	)
	_, err := h.AdminListPendingCommentsHandler(commentAdminAdminCtx(), &AdminListPendingCommentsRequest{})
	testhelpers.AssertHumaError(t, err, 500)
}

// ============================================================================
// Tests: Approve Comment — Auth & Validation
// ============================================================================

func TestAdminApproveComment_RequiresAdmin(t *testing.T) {
	h := testCommentAdminHandler()

	t.Run("NoUser", func(t *testing.T) {
		_, err := h.AdminApproveCommentHandler(context.Background(), &AdminApproveCommentRequest{CommentID: "1"})
		testhelpers.AssertHumaError(t, err, 403)
	})
	t.Run("NonAdmin", func(t *testing.T) {
		_, err := h.AdminApproveCommentHandler(commentAdminUserCtx(), &AdminApproveCommentRequest{CommentID: "1"})
		testhelpers.AssertHumaError(t, err, 403)
	})
}

func TestAdminApproveComment_InvalidID(t *testing.T) {
	h := testCommentAdminHandler()
	_, err := h.AdminApproveCommentHandler(commentAdminAdminCtx(), &AdminApproveCommentRequest{CommentID: "abc"})
	testhelpers.AssertHumaError(t, err, 400)
}

func TestAdminApproveComment_NotFound(t *testing.T) {
	h := NewCommentAdminHandler(
		&testhelpers.MockCommentAdminService{
			ApproveCommentFn: func(adminUserID, commentID uint) error {
				return fmt.Errorf("comment not found")
			},
		},
		nil,
	)
	_, err := h.AdminApproveCommentHandler(commentAdminAdminCtx(), &AdminApproveCommentRequest{CommentID: "99"})
	testhelpers.AssertHumaError(t, err, 404)
}

func TestAdminApproveComment_NotPending(t *testing.T) {
	h := NewCommentAdminHandler(
		&testhelpers.MockCommentAdminService{
			ApproveCommentFn: func(adminUserID, commentID uint) error {
				return fmt.Errorf("comment is not pending review")
			},
		},
		nil,
	)
	_, err := h.AdminApproveCommentHandler(commentAdminAdminCtx(), &AdminApproveCommentRequest{CommentID: "1"})
	testhelpers.AssertHumaError(t, err, 409)
}

func TestAdminApproveComment_Success(t *testing.T) {
	h := NewCommentAdminHandler(
		&testhelpers.MockCommentAdminService{
			ApproveCommentFn: func(adminUserID, commentID uint) error {
				if adminUserID != 1 || commentID != 5 {
					t.Errorf("unexpected params: admin=%d, comment=%d", adminUserID, commentID)
				}
				return nil
			},
		},
		&testhelpers.MockAuditLogService{},
	)
	_, err := h.AdminApproveCommentHandler(commentAdminAdminCtx(), &AdminApproveCommentRequest{CommentID: "5"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// ============================================================================
// Tests: Reject Comment — Auth & Validation
// ============================================================================

func TestAdminRejectComment_RequiresAdmin(t *testing.T) {
	h := testCommentAdminHandler()

	t.Run("NoUser", func(t *testing.T) {
		req := &AdminRejectCommentRequest{CommentID: "1"}
		req.Body.Reason = "spam"
		_, err := h.AdminRejectCommentHandler(context.Background(), req)
		testhelpers.AssertHumaError(t, err, 403)
	})
	t.Run("NonAdmin", func(t *testing.T) {
		req := &AdminRejectCommentRequest{CommentID: "1"}
		req.Body.Reason = "spam"
		_, err := h.AdminRejectCommentHandler(commentAdminUserCtx(), req)
		testhelpers.AssertHumaError(t, err, 403)
	})
}

func TestAdminRejectComment_InvalidID(t *testing.T) {
	h := testCommentAdminHandler()
	req := &AdminRejectCommentRequest{CommentID: "abc"}
	req.Body.Reason = "spam"
	_, err := h.AdminRejectCommentHandler(commentAdminAdminCtx(), req)
	testhelpers.AssertHumaError(t, err, 400)
}

func TestAdminRejectComment_EmptyReason(t *testing.T) {
	h := testCommentAdminHandler()
	req := &AdminRejectCommentRequest{CommentID: "1"}
	req.Body.Reason = ""
	_, err := h.AdminRejectCommentHandler(commentAdminAdminCtx(), req)
	testhelpers.AssertHumaError(t, err, 400)
}

func TestAdminRejectComment_NotFound(t *testing.T) {
	h := NewCommentAdminHandler(
		&testhelpers.MockCommentAdminService{
			RejectCommentFn: func(adminUserID, commentID uint, reason string) error {
				return fmt.Errorf("comment not found")
			},
		},
		nil,
	)
	req := &AdminRejectCommentRequest{CommentID: "99"}
	req.Body.Reason = "spam"
	_, err := h.AdminRejectCommentHandler(commentAdminAdminCtx(), req)
	testhelpers.AssertHumaError(t, err, 404)
}

func TestAdminRejectComment_NotPending(t *testing.T) {
	h := NewCommentAdminHandler(
		&testhelpers.MockCommentAdminService{
			RejectCommentFn: func(adminUserID, commentID uint, reason string) error {
				return fmt.Errorf("comment is not pending review")
			},
		},
		nil,
	)
	req := &AdminRejectCommentRequest{CommentID: "1"}
	req.Body.Reason = "spam"
	_, err := h.AdminRejectCommentHandler(commentAdminAdminCtx(), req)
	testhelpers.AssertHumaError(t, err, 409)
}

func TestAdminRejectComment_Success(t *testing.T) {
	h := NewCommentAdminHandler(
		&testhelpers.MockCommentAdminService{
			RejectCommentFn: func(adminUserID, commentID uint, reason string) error {
				if adminUserID != 1 || commentID != 5 || reason != "spam" {
					t.Errorf("unexpected params: admin=%d, comment=%d, reason=%s", adminUserID, commentID, reason)
				}
				return nil
			},
		},
		&testhelpers.MockAuditLogService{},
	)
	req := &AdminRejectCommentRequest{CommentID: "5"}
	req.Body.Reason = "spam"
	_, err := h.AdminRejectCommentHandler(commentAdminAdminCtx(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// ============================================================================
// Tests: CreateComment — Rate limiting error mapping
// ============================================================================

func TestCreateComment_RateLimitError(t *testing.T) {
	mock := &testhelpers.MockCommentService{
		CreateCommentFn: func(userID uint, req *contracts.CreateCommentRequest) (*contracts.CommentResponse, error) {
			return nil, fmt.Errorf("please wait 60 seconds between comments on the same entity")
		},
	}
	h := NewCommentHandler(mock, mock, nil, nil)
	req := &CreateCommentRequest{EntityType: "show", EntityID: "1"}
	req.Body.Body = "Hello"
	_, err := h.CreateCommentHandler(commentUserCtx(), req)
	testhelpers.AssertHumaError(t, err, 429)
}

func TestCreateComment_HourlyLimitError(t *testing.T) {
	mock := &testhelpers.MockCommentService{
		CreateCommentFn: func(userID uint, req *contracts.CreateCommentRequest) (*contracts.CommentResponse, error) {
			return nil, fmt.Errorf("you've reached your hourly comment limit (5/hour for new users)")
		},
	}
	h := NewCommentHandler(mock, mock, nil, nil)
	req := &CreateCommentRequest{EntityType: "show", EntityID: "1"}
	req.Body.Body = "Hello"
	_, err := h.CreateCommentHandler(commentUserCtx(), req)
	testhelpers.AssertHumaError(t, err, 429)
}

// ============================================================================
// Tests: Admin Get Comment Edit History — Auth & Response (PSY-297)
// ============================================================================

func TestAdminGetCommentEditHistory_RequiresAdmin(t *testing.T) {
	h := testCommentAdminHandler()

	t.Run("NoUser", func(t *testing.T) {
		_, err := h.AdminGetCommentEditHistoryHandler(context.Background(), &AdminGetCommentEditHistoryRequest{CommentID: "1"})
		testhelpers.AssertHumaError(t, err, 403)
	})
	t.Run("NonAdmin", func(t *testing.T) {
		_, err := h.AdminGetCommentEditHistoryHandler(commentAdminUserCtx(), &AdminGetCommentEditHistoryRequest{CommentID: "1"})
		testhelpers.AssertHumaError(t, err, 403)
	})
}

func TestAdminGetCommentEditHistory_InvalidID(t *testing.T) {
	h := testCommentAdminHandler()
	_, err := h.AdminGetCommentEditHistoryHandler(commentAdminAdminCtx(), &AdminGetCommentEditHistoryRequest{CommentID: "not-a-number"})
	testhelpers.AssertHumaError(t, err, 400)
}

func TestAdminGetCommentEditHistory_NotFound(t *testing.T) {
	h := NewCommentAdminHandler(
		&testhelpers.MockCommentAdminService{
			GetCommentEditHistoryFn: func(requesterID, commentID uint) (*contracts.CommentEditHistoryResponse, error) {
				return nil, fmt.Errorf("comment not found")
			},
		},
		nil,
	)
	_, err := h.AdminGetCommentEditHistoryHandler(commentAdminAdminCtx(), &AdminGetCommentEditHistoryRequest{CommentID: "99"})
	testhelpers.AssertHumaError(t, err, 404)
}

func TestAdminGetCommentEditHistory_ServiceAdminRejection(t *testing.T) {
	// Service should be able to bubble up an admin-access error and we map it to 403.
	h := NewCommentAdminHandler(
		&testhelpers.MockCommentAdminService{
			GetCommentEditHistoryFn: func(requesterID, commentID uint) (*contracts.CommentEditHistoryResponse, error) {
				return nil, fmt.Errorf("admin access required")
			},
		},
		nil,
	)
	_, err := h.AdminGetCommentEditHistoryHandler(commentAdminAdminCtx(), &AdminGetCommentEditHistoryRequest{CommentID: "5"})
	testhelpers.AssertHumaError(t, err, 403)
}

func TestAdminGetCommentEditHistory_Success(t *testing.T) {
	editorID := uint(42)
	h := NewCommentAdminHandler(
		&testhelpers.MockCommentAdminService{
			GetCommentEditHistoryFn: func(requesterID, commentID uint) (*contracts.CommentEditHistoryResponse, error) {
				if requesterID != 1 || commentID != 7 {
					t.Errorf("unexpected params: requester=%d, comment=%d", requesterID, commentID)
				}
				return &contracts.CommentEditHistoryResponse{
					CommentID:   7,
					CurrentBody: "third",
					Edits: []contracts.CommentEditHistoryEntry{
						{ID: 1, CommentID: 7, OldBody: "first", EditedAt: time.Now().Add(-2 * time.Hour), EditorUserID: &editorID, EditorUsername: "author"},
						{ID: 2, CommentID: 7, OldBody: "second", EditedAt: time.Now().Add(-1 * time.Hour), EditorUserID: &editorID, EditorUsername: "author"},
					},
				}, nil
			},
		},
		nil,
	)
	resp, err := h.AdminGetCommentEditHistoryHandler(commentAdminAdminCtx(), &AdminGetCommentEditHistoryRequest{CommentID: "7"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.CommentID != 7 {
		t.Errorf("expected comment_id=7, got %d", resp.Body.CommentID)
	}
	if resp.Body.CurrentBody != "third" {
		t.Errorf("expected current_body=third, got %s", resp.Body.CurrentBody)
	}
	if len(resp.Body.Edits) != 2 {
		t.Errorf("expected 2 edits, got %d", len(resp.Body.Edits))
	}
	if resp.Body.Edits[0].OldBody != "first" {
		t.Errorf("expected oldest entry first, got %q", resp.Body.Edits[0].OldBody)
	}
}
