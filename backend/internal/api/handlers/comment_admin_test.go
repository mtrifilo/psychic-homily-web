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

func testCommentAdminHandler() *CommentAdminHandler {
	return NewCommentAdminHandler(nil, nil)
}

func commentAdminAdminCtx() context.Context {
	return ctxWithUser(&models.User{ID: 1, IsAdmin: true})
}

func commentAdminUserCtx() context.Context {
	return ctxWithUser(&models.User{ID: 10, IsAdmin: false})
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
		assertHumaError(t, err, 403)
	})
	t.Run("NonAdmin", func(t *testing.T) {
		req := &AdminHideCommentRequest{CommentID: "1"}
		req.Body.Reason = "spam"
		_, err := h.AdminHideCommentHandler(commentAdminUserCtx(), req)
		assertHumaError(t, err, 403)
	})
}

func TestAdminHideComment_InvalidID(t *testing.T) {
	h := testCommentAdminHandler()
	req := &AdminHideCommentRequest{CommentID: "abc"}
	req.Body.Reason = "spam"
	_, err := h.AdminHideCommentHandler(commentAdminAdminCtx(), req)
	assertHumaError(t, err, 400)
}

func TestAdminHideComment_EmptyReason(t *testing.T) {
	h := testCommentAdminHandler()
	req := &AdminHideCommentRequest{CommentID: "1"}
	req.Body.Reason = ""
	_, err := h.AdminHideCommentHandler(commentAdminAdminCtx(), req)
	assertHumaError(t, err, 400)
}

func TestAdminHideComment_NotFound(t *testing.T) {
	h := NewCommentAdminHandler(
		&mockCommentAdminService{
			hideCommentFn: func(adminUserID, commentID uint, reason string) error {
				return fmt.Errorf("comment not found")
			},
		},
		nil,
	)
	req := &AdminHideCommentRequest{CommentID: "99"}
	req.Body.Reason = "spam"
	_, err := h.AdminHideCommentHandler(commentAdminAdminCtx(), req)
	assertHumaError(t, err, 404)
}

func TestAdminHideComment_Success(t *testing.T) {
	h := NewCommentAdminHandler(
		&mockCommentAdminService{
			hideCommentFn: func(adminUserID, commentID uint, reason string) error {
				if adminUserID != 1 || commentID != 5 || reason != "violates guidelines" {
					t.Errorf("unexpected params: admin=%d, comment=%d, reason=%s", adminUserID, commentID, reason)
				}
				return nil
			},
		},
		&mockAuditLogService{},
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
		assertHumaError(t, err, 403)
	})
	t.Run("NonAdmin", func(t *testing.T) {
		_, err := h.AdminRestoreCommentHandler(commentAdminUserCtx(), &AdminRestoreCommentRequest{CommentID: "1"})
		assertHumaError(t, err, 403)
	})
}

func TestAdminRestoreComment_InvalidID(t *testing.T) {
	h := testCommentAdminHandler()
	_, err := h.AdminRestoreCommentHandler(commentAdminAdminCtx(), &AdminRestoreCommentRequest{CommentID: "abc"})
	assertHumaError(t, err, 400)
}

func TestAdminRestoreComment_NotFound(t *testing.T) {
	h := NewCommentAdminHandler(
		&mockCommentAdminService{
			restoreCommentFn: func(adminUserID, commentID uint) error {
				return fmt.Errorf("comment not found")
			},
		},
		nil,
	)
	_, err := h.AdminRestoreCommentHandler(commentAdminAdminCtx(), &AdminRestoreCommentRequest{CommentID: "99"})
	assertHumaError(t, err, 404)
}

func TestAdminRestoreComment_AlreadyVisible(t *testing.T) {
	h := NewCommentAdminHandler(
		&mockCommentAdminService{
			restoreCommentFn: func(adminUserID, commentID uint) error {
				return fmt.Errorf("comment is already visible")
			},
		},
		nil,
	)
	_, err := h.AdminRestoreCommentHandler(commentAdminAdminCtx(), &AdminRestoreCommentRequest{CommentID: "1"})
	assertHumaError(t, err, 409)
}

func TestAdminRestoreComment_Success(t *testing.T) {
	h := NewCommentAdminHandler(
		&mockCommentAdminService{
			restoreCommentFn: func(adminUserID, commentID uint) error {
				if adminUserID != 1 || commentID != 5 {
					t.Errorf("unexpected params: admin=%d, comment=%d", adminUserID, commentID)
				}
				return nil
			},
		},
		&mockAuditLogService{},
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
		assertHumaError(t, err, 403)
	})
	t.Run("NonAdmin", func(t *testing.T) {
		_, err := h.AdminListPendingCommentsHandler(commentAdminUserCtx(), &AdminListPendingCommentsRequest{})
		assertHumaError(t, err, 403)
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
		&mockCommentAdminService{
			listPendingCommentsFn: func(limit, offset int) ([]*contracts.CommentResponse, int64, error) {
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
		&mockCommentAdminService{
			listPendingCommentsFn: func(limit, offset int) ([]*contracts.CommentResponse, int64, error) {
				return nil, 0, fmt.Errorf("database error")
			},
		},
		nil,
	)
	_, err := h.AdminListPendingCommentsHandler(commentAdminAdminCtx(), &AdminListPendingCommentsRequest{})
	assertHumaError(t, err, 500)
}

// ============================================================================
// Tests: Approve Comment — Auth & Validation
// ============================================================================

func TestAdminApproveComment_RequiresAdmin(t *testing.T) {
	h := testCommentAdminHandler()

	t.Run("NoUser", func(t *testing.T) {
		_, err := h.AdminApproveCommentHandler(context.Background(), &AdminApproveCommentRequest{CommentID: "1"})
		assertHumaError(t, err, 403)
	})
	t.Run("NonAdmin", func(t *testing.T) {
		_, err := h.AdminApproveCommentHandler(commentAdminUserCtx(), &AdminApproveCommentRequest{CommentID: "1"})
		assertHumaError(t, err, 403)
	})
}

func TestAdminApproveComment_InvalidID(t *testing.T) {
	h := testCommentAdminHandler()
	_, err := h.AdminApproveCommentHandler(commentAdminAdminCtx(), &AdminApproveCommentRequest{CommentID: "abc"})
	assertHumaError(t, err, 400)
}

func TestAdminApproveComment_NotFound(t *testing.T) {
	h := NewCommentAdminHandler(
		&mockCommentAdminService{
			approveCommentFn: func(adminUserID, commentID uint) error {
				return fmt.Errorf("comment not found")
			},
		},
		nil,
	)
	_, err := h.AdminApproveCommentHandler(commentAdminAdminCtx(), &AdminApproveCommentRequest{CommentID: "99"})
	assertHumaError(t, err, 404)
}

func TestAdminApproveComment_NotPending(t *testing.T) {
	h := NewCommentAdminHandler(
		&mockCommentAdminService{
			approveCommentFn: func(adminUserID, commentID uint) error {
				return fmt.Errorf("comment is not pending review")
			},
		},
		nil,
	)
	_, err := h.AdminApproveCommentHandler(commentAdminAdminCtx(), &AdminApproveCommentRequest{CommentID: "1"})
	assertHumaError(t, err, 409)
}

func TestAdminApproveComment_Success(t *testing.T) {
	h := NewCommentAdminHandler(
		&mockCommentAdminService{
			approveCommentFn: func(adminUserID, commentID uint) error {
				if adminUserID != 1 || commentID != 5 {
					t.Errorf("unexpected params: admin=%d, comment=%d", adminUserID, commentID)
				}
				return nil
			},
		},
		&mockAuditLogService{},
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
		assertHumaError(t, err, 403)
	})
	t.Run("NonAdmin", func(t *testing.T) {
		req := &AdminRejectCommentRequest{CommentID: "1"}
		req.Body.Reason = "spam"
		_, err := h.AdminRejectCommentHandler(commentAdminUserCtx(), req)
		assertHumaError(t, err, 403)
	})
}

func TestAdminRejectComment_InvalidID(t *testing.T) {
	h := testCommentAdminHandler()
	req := &AdminRejectCommentRequest{CommentID: "abc"}
	req.Body.Reason = "spam"
	_, err := h.AdminRejectCommentHandler(commentAdminAdminCtx(), req)
	assertHumaError(t, err, 400)
}

func TestAdminRejectComment_EmptyReason(t *testing.T) {
	h := testCommentAdminHandler()
	req := &AdminRejectCommentRequest{CommentID: "1"}
	req.Body.Reason = ""
	_, err := h.AdminRejectCommentHandler(commentAdminAdminCtx(), req)
	assertHumaError(t, err, 400)
}

func TestAdminRejectComment_NotFound(t *testing.T) {
	h := NewCommentAdminHandler(
		&mockCommentAdminService{
			rejectCommentFn: func(adminUserID, commentID uint, reason string) error {
				return fmt.Errorf("comment not found")
			},
		},
		nil,
	)
	req := &AdminRejectCommentRequest{CommentID: "99"}
	req.Body.Reason = "spam"
	_, err := h.AdminRejectCommentHandler(commentAdminAdminCtx(), req)
	assertHumaError(t, err, 404)
}

func TestAdminRejectComment_NotPending(t *testing.T) {
	h := NewCommentAdminHandler(
		&mockCommentAdminService{
			rejectCommentFn: func(adminUserID, commentID uint, reason string) error {
				return fmt.Errorf("comment is not pending review")
			},
		},
		nil,
	)
	req := &AdminRejectCommentRequest{CommentID: "1"}
	req.Body.Reason = "spam"
	_, err := h.AdminRejectCommentHandler(commentAdminAdminCtx(), req)
	assertHumaError(t, err, 409)
}

func TestAdminRejectComment_Success(t *testing.T) {
	h := NewCommentAdminHandler(
		&mockCommentAdminService{
			rejectCommentFn: func(adminUserID, commentID uint, reason string) error {
				if adminUserID != 1 || commentID != 5 || reason != "spam" {
					t.Errorf("unexpected params: admin=%d, comment=%d, reason=%s", adminUserID, commentID, reason)
				}
				return nil
			},
		},
		&mockAuditLogService{},
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
	mock := &mockCommentService{
		createCommentFn: func(userID uint, req *contracts.CreateCommentRequest) (*contracts.CommentResponse, error) {
			return nil, fmt.Errorf("please wait 60 seconds between comments on the same entity")
		},
	}
	h := NewCommentHandler(mock, mock, nil, nil)
	req := &CreateCommentRequest{EntityType: "show", EntityID: "1"}
	req.Body.Body = "Hello"
	_, err := h.CreateCommentHandler(commentUserCtx(), req)
	assertHumaError(t, err, 429)
}

func TestCreateComment_HourlyLimitError(t *testing.T) {
	mock := &mockCommentService{
		createCommentFn: func(userID uint, req *contracts.CreateCommentRequest) (*contracts.CommentResponse, error) {
			return nil, fmt.Errorf("you've reached your hourly comment limit (5/hour for new users)")
		},
	}
	h := NewCommentHandler(mock, mock, nil, nil)
	req := &CreateCommentRequest{EntityType: "show", EntityID: "1"}
	req.Body.Body = "Hello"
	_, err := h.CreateCommentHandler(commentUserCtx(), req)
	assertHumaError(t, err, 429)
}
