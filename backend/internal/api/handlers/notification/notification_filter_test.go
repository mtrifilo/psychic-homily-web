package notification

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"testing"

	"psychic-homily-backend/internal/api/handlers/shared/testhelpers"
	apperrors "psychic-homily-backend/internal/errors"
	"psychic-homily-backend/internal/services/contracts"

	authm "psychic-homily-backend/internal/models/auth"
	notificationm "psychic-homily-backend/internal/models/notification"

	"github.com/lib/pq"
)

func testNotificationFilterHandler() *NotificationFilterHandler {
	return NewNotificationFilterHandler(nil, "test-secret")
}

// --- ListFiltersHandler ---

func TestListFiltersHandler_NoAuth(t *testing.T) {
	h := testNotificationFilterHandler()
	_, err := h.ListFiltersHandler(context.Background(), &ListFiltersRequest{})
	testhelpers.AssertHumaError(t, err, 401)
}

func TestListFiltersHandler_Success(t *testing.T) {
	mock := &testhelpers.MockNotificationFilterService{
		GetUserFiltersFn: func(userID uint) ([]notificationm.NotificationFilter, error) {
			return []notificationm.NotificationFilter{
				{ID: 1, Name: "Test filter", IsActive: true, ArtistIDs: pq.Int64Array{1, 2}},
			}, nil
		},
	}
	h := NewNotificationFilterHandler(mock, "test-secret")
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1})

	resp, err := h.ListFiltersHandler(ctx, &ListFiltersRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Body.Filters) != 1 {
		t.Errorf("expected 1 filter, got %d", len(resp.Body.Filters))
	}
	if resp.Body.Filters[0].Name != "Test filter" {
		t.Errorf("expected 'Test filter', got '%s'", resp.Body.Filters[0].Name)
	}
}

func TestListFiltersHandler_ServiceError(t *testing.T) {
	mock := &testhelpers.MockNotificationFilterService{
		GetUserFiltersFn: func(_ uint) ([]notificationm.NotificationFilter, error) {
			return nil, fmt.Errorf("db error")
		},
	}
	h := NewNotificationFilterHandler(mock, "test-secret")
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1})

	_, err := h.ListFiltersHandler(ctx, &ListFiltersRequest{})
	testhelpers.AssertHumaError(t, err, 500)
}

// --- CreateFilterHandler ---

func TestCreateFilterHandler_NoAuth(t *testing.T) {
	h := testNotificationFilterHandler()
	_, err := h.CreateFilterHandler(context.Background(), &CreateFilterRequest{})
	testhelpers.AssertHumaError(t, err, 401)
}

func TestCreateFilterHandler_Success(t *testing.T) {
	mock := &testhelpers.MockNotificationFilterService{
		CreateFilterFn: func(userID uint, input contracts.CreateFilterInput) (*notificationm.NotificationFilter, error) {
			if userID != 1 {
				return nil, fmt.Errorf("wrong user")
			}
			if input.Name != "PHX punk" {
				return nil, fmt.Errorf("wrong name: %s", input.Name)
			}
			return &notificationm.NotificationFilter{
				ID:       1,
				UserID:   userID,
				Name:     input.Name,
				IsActive: true,
			}, nil
		},
	}
	h := NewNotificationFilterHandler(mock, "test-secret")
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1})

	req := &CreateFilterRequest{}
	req.Body.Name = "PHX punk"
	req.Body.ArtistIDs = []int64{1, 2}

	resp, err := h.CreateFilterHandler(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Name != "PHX punk" {
		t.Errorf("expected 'PHX punk', got '%s'", resp.Body.Name)
	}
}

func TestCreateFilterHandler_ValidationError(t *testing.T) {
	mock := &testhelpers.MockNotificationFilterService{
		CreateFilterFn: func(_ uint, _ contracts.CreateFilterInput) (*notificationm.NotificationFilter, error) {
			return nil, apperrors.ErrFilterValidation("at least one filter criteria is required")
		},
	}
	h := NewNotificationFilterHandler(mock, "test-secret")
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1})

	req := &CreateFilterRequest{}
	req.Body.Name = "Empty"

	_, err := h.CreateFilterHandler(ctx, req)
	testhelpers.AssertHumaError(t, err, 422)
}

func TestCreateFilterHandler_ServiceError(t *testing.T) {
	mock := &testhelpers.MockNotificationFilterService{
		CreateFilterFn: func(_ uint, _ contracts.CreateFilterInput) (*notificationm.NotificationFilter, error) {
			return nil, apperrors.ErrFilterInternal(fmt.Errorf("db down"))
		},
	}
	h := NewNotificationFilterHandler(mock, "test-secret")
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1})

	req := &CreateFilterRequest{}
	req.Body.Name = "PHX punk"
	req.Body.ArtistIDs = []int64{1}

	_, err := h.CreateFilterHandler(ctx, req)
	testhelpers.AssertHumaError(t, err, 500)
}

// --- UpdateFilterHandler ---

func TestUpdateFilterHandler_NoAuth(t *testing.T) {
	h := testNotificationFilterHandler()
	_, err := h.UpdateFilterHandler(context.Background(), &UpdateFilterRequest{ID: "1"})
	testhelpers.AssertHumaError(t, err, 401)
}

func TestUpdateFilterHandler_InvalidID(t *testing.T) {
	h := NewNotificationFilterHandler(&testhelpers.MockNotificationFilterService{}, "test-secret")
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1})

	_, err := h.UpdateFilterHandler(ctx, &UpdateFilterRequest{ID: "abc"})
	testhelpers.AssertHumaError(t, err, 400)
}

func TestUpdateFilterHandler_NotFound(t *testing.T) {
	mock := &testhelpers.MockNotificationFilterService{
		UpdateFilterFn: func(_ uint, _ uint, _ contracts.UpdateFilterInput) (*notificationm.NotificationFilter, error) {
			return nil, apperrors.ErrFilterNotFound()
		},
	}
	h := NewNotificationFilterHandler(mock, "test-secret")
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1})

	_, err := h.UpdateFilterHandler(ctx, &UpdateFilterRequest{ID: "99"})
	testhelpers.AssertHumaError(t, err, 404)
}

func TestUpdateFilterHandler_ServiceError(t *testing.T) {
	mock := &testhelpers.MockNotificationFilterService{
		UpdateFilterFn: func(_ uint, _ uint, _ contracts.UpdateFilterInput) (*notificationm.NotificationFilter, error) {
			return nil, apperrors.ErrFilterInternal(fmt.Errorf("db down"))
		},
	}
	h := NewNotificationFilterHandler(mock, "test-secret")
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1})

	_, err := h.UpdateFilterHandler(ctx, &UpdateFilterRequest{ID: "42"})
	testhelpers.AssertHumaError(t, err, 500)
}

func TestUpdateFilterHandler_Success(t *testing.T) {
	mock := &testhelpers.MockNotificationFilterService{
		UpdateFilterFn: func(_ uint, filterID uint, input contracts.UpdateFilterInput) (*notificationm.NotificationFilter, error) {
			return &notificationm.NotificationFilter{
				ID:       filterID,
				Name:     *input.Name,
				IsActive: true,
			}, nil
		},
	}
	h := NewNotificationFilterHandler(mock, "test-secret")
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1})

	req := &UpdateFilterRequest{ID: "42"}
	name := "Updated"
	req.Body.Name = &name

	resp, err := h.UpdateFilterHandler(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Name != "Updated" {
		t.Errorf("expected 'Updated', got '%s'", resp.Body.Name)
	}
}

// --- DeleteFilterHandler ---

func TestDeleteFilterHandler_NoAuth(t *testing.T) {
	h := testNotificationFilterHandler()
	_, err := h.DeleteFilterHandler(context.Background(), &DeleteFilterRequest{ID: "1"})
	testhelpers.AssertHumaError(t, err, 401)
}

func TestDeleteFilterHandler_InvalidID(t *testing.T) {
	h := NewNotificationFilterHandler(&testhelpers.MockNotificationFilterService{}, "test-secret")
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1})

	_, err := h.DeleteFilterHandler(ctx, &DeleteFilterRequest{ID: "abc"})
	testhelpers.AssertHumaError(t, err, 400)
}

func TestDeleteFilterHandler_Success(t *testing.T) {
	mock := &testhelpers.MockNotificationFilterService{
		DeleteFilterFn: func(userID, filterID uint) error {
			if userID != 1 || filterID != 42 {
				return fmt.Errorf("wrong args")
			}
			return nil
		},
	}
	h := NewNotificationFilterHandler(mock, "test-secret")
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1})

	resp, err := h.DeleteFilterHandler(ctx, &DeleteFilterRequest{ID: "42"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.Body.Success {
		t.Error("expected success=true")
	}
}

func TestDeleteFilterHandler_NotFound(t *testing.T) {
	mock := &testhelpers.MockNotificationFilterService{
		DeleteFilterFn: func(_, _ uint) error {
			return apperrors.ErrFilterNotFound()
		},
	}
	h := NewNotificationFilterHandler(mock, "test-secret")
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1})

	_, err := h.DeleteFilterHandler(ctx, &DeleteFilterRequest{ID: "99"})
	testhelpers.AssertHumaError(t, err, 404)
}

func TestDeleteFilterHandler_ServiceError(t *testing.T) {
	mock := &testhelpers.MockNotificationFilterService{
		DeleteFilterFn: func(_, _ uint) error {
			return apperrors.ErrFilterInternal(fmt.Errorf("db down"))
		},
	}
	h := NewNotificationFilterHandler(mock, "test-secret")
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1})

	_, err := h.DeleteFilterHandler(ctx, &DeleteFilterRequest{ID: "42"})
	testhelpers.AssertHumaError(t, err, 500)
}

// --- QuickCreateFilterHandler ---

func TestQuickCreateFilterHandler_NoAuth(t *testing.T) {
	h := testNotificationFilterHandler()
	_, err := h.QuickCreateFilterHandler(context.Background(), &QuickCreateFilterRequest{})
	testhelpers.AssertHumaError(t, err, 401)
}

func TestQuickCreateFilterHandler_InvalidEntityID(t *testing.T) {
	h := NewNotificationFilterHandler(&testhelpers.MockNotificationFilterService{}, "test-secret")
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1})

	req := &QuickCreateFilterRequest{}
	req.Body.EntityType = "artist"
	req.Body.EntityID = 0

	_, err := h.QuickCreateFilterHandler(ctx, req)
	testhelpers.AssertHumaError(t, err, 400)
}

func TestQuickCreateFilterHandler_Success(t *testing.T) {
	mock := &testhelpers.MockNotificationFilterService{
		QuickCreateFilterFn: func(userID uint, entityType string, entityID uint) (*notificationm.NotificationFilter, error) {
			return &notificationm.NotificationFilter{
				ID:       1,
				Name:     "Deafheaven shows",
				IsActive: true,
			}, nil
		},
	}
	h := NewNotificationFilterHandler(mock, "test-secret")
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1})

	req := &QuickCreateFilterRequest{}
	req.Body.EntityType = "artist"
	req.Body.EntityID = 42

	resp, err := h.QuickCreateFilterHandler(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Name != "Deafheaven shows" {
		t.Errorf("expected 'Deafheaven shows', got '%s'", resp.Body.Name)
	}
}

// --- GetNotificationsHandler ---

func TestGetNotificationsHandler_NoAuth(t *testing.T) {
	h := testNotificationFilterHandler()
	_, err := h.GetNotificationsHandler(context.Background(), &GetNotificationsRequest{})
	testhelpers.AssertHumaError(t, err, 401)
}

func TestGetNotificationsHandler_Success(t *testing.T) {
	mock := &testhelpers.MockNotificationFilterService{
		GetUserNotificationsFn: func(userID uint, limit, offset int) ([]contracts.NotificationLogEntry, error) {
			return []contracts.NotificationLogEntry{
				{ID: 1, EntityType: "show", EntityID: 42, Channel: "email"},
			}, nil
		},
		GetUnreadCountFn: func(userID uint) (int64, error) {
			return 3, nil
		},
	}
	h := NewNotificationFilterHandler(mock, "test-secret")
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1})

	resp, err := h.GetNotificationsHandler(ctx, &GetNotificationsRequest{Limit: 20, Offset: 0})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Body.Notifications) != 1 {
		t.Errorf("expected 1 notification, got %d", len(resp.Body.Notifications))
	}
	if resp.Body.UnreadCount != 3 {
		t.Errorf("expected unread count 3, got %d", resp.Body.UnreadCount)
	}
}

// --- UnsubscribeFilterHandler ---

func TestUnsubscribeFilterHandler_InvalidID(t *testing.T) {
	h := testNotificationFilterHandler()
	req := &UnsubscribeFilterRequest{ID: "abc"}
	req.Body.Sig = "something"

	_, err := h.UnsubscribeFilterHandler(context.Background(), req)
	testhelpers.AssertHumaError(t, err, 400)
}

func TestUnsubscribeFilterHandler_InvalidSignature(t *testing.T) {
	h := NewNotificationFilterHandler(&testhelpers.MockNotificationFilterService{}, "test-secret")
	req := &UnsubscribeFilterRequest{ID: "42"}
	req.Body.Sig = "bad-signature"

	_, err := h.UnsubscribeFilterHandler(context.Background(), req)
	testhelpers.AssertHumaError(t, err, 403)
}

func TestUnsubscribeFilterHandler_Success(t *testing.T) {
	mock := &testhelpers.MockNotificationFilterService{
		PauseFilterFn: func(filterID uint) error {
			if filterID != 42 {
				return fmt.Errorf("wrong filter ID: %d", filterID)
			}
			return nil
		},
	}
	h := NewNotificationFilterHandler(mock, "test-secret")

	// Compute valid HMAC signature — mirrors ComputeFilterUnsubscribeSignature
	sig := computeTestFilterSig(42, "test-secret")
	req := &UnsubscribeFilterRequest{ID: "42"}
	req.Body.Sig = sig

	resp, err := h.UnsubscribeFilterHandler(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.Body.Success {
		t.Error("expected success=true")
	}
}

// --- MarkNotificationsReadHandler (PSY-595) ---

func TestMarkNotificationsReadHandler_NoAuth(t *testing.T) {
	h := testNotificationFilterHandler()
	_, err := h.MarkNotificationsReadHandler(context.Background(), &MarkNotificationsReadRequest{})
	testhelpers.AssertHumaError(t, err, 401)
}

func TestMarkNotificationsReadHandler_MarkAll(t *testing.T) {
	var capturedUserID uint
	mock := &testhelpers.MockNotificationFilterService{
		MarkAllNotificationsReadFn: func(userID uint) (int64, error) {
			capturedUserID = userID
			return 5, nil
		},
		GetUnreadCountFn: func(_ uint) (int64, error) { return 0, nil },
	}
	h := NewNotificationFilterHandler(mock, "test-secret")
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 99})

	req := &MarkNotificationsReadRequest{}
	resp, err := h.MarkNotificationsReadHandler(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedUserID != 99 {
		t.Errorf("expected user 99, got %d", capturedUserID)
	}
	if resp.Body.UpdatedCount != 5 {
		t.Errorf("expected updated 5, got %d", resp.Body.UpdatedCount)
	}
	if resp.Body.UnreadCount != 0 {
		t.Errorf("expected unread 0, got %d", resp.Body.UnreadCount)
	}
}

func TestMarkNotificationsReadHandler_MarkSpecific(t *testing.T) {
	var capturedIDs []uint
	mock := &testhelpers.MockNotificationFilterService{
		MarkNotificationsReadFn: func(_ uint, ids []uint) (int64, error) {
			capturedIDs = ids
			return int64(len(ids)), nil
		},
		GetUnreadCountFn: func(_ uint) (int64, error) { return 2, nil },
	}
	h := NewNotificationFilterHandler(mock, "test-secret")
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1})

	req := &MarkNotificationsReadRequest{}
	req.Body.IDs = []uint{10, 20, 30}
	resp, err := h.MarkNotificationsReadHandler(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(capturedIDs) != 3 {
		t.Errorf("expected 3 IDs, got %d", len(capturedIDs))
	}
	if resp.Body.UpdatedCount != 3 {
		t.Errorf("expected updated 3, got %d", resp.Body.UpdatedCount)
	}
	if resp.Body.UnreadCount != 2 {
		t.Errorf("expected unread 2, got %d", resp.Body.UnreadCount)
	}
}

func TestMarkNotificationsReadHandler_ServiceError(t *testing.T) {
	mock := &testhelpers.MockNotificationFilterService{
		MarkAllNotificationsReadFn: func(_ uint) (int64, error) {
			return 0, fmt.Errorf("db down")
		},
	}
	h := NewNotificationFilterHandler(mock, "test-secret")
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1})

	_, err := h.MarkNotificationsReadHandler(ctx, &MarkNotificationsReadRequest{})
	testhelpers.AssertHumaError(t, err, 500)
}

// --- filterToResponse helper ---

func TestFilterToResponse(t *testing.T) {
	f := &notificationm.NotificationFilter{
		ID:        1,
		Name:      "Test",
		Source:    notificationm.FilterSourceManaged,
		IsActive:  true,
		ArtistIDs: pq.Int64Array{1, 2},
	}
	resp := filterToResponse(f)
	if resp.ID != 1 {
		t.Errorf("expected ID=1, got %d", resp.ID)
	}
	if resp.Name != "Test" {
		t.Errorf("expected name=Test, got %s", resp.Name)
	}
	if resp.Source != notificationm.FilterSourceManaged {
		t.Errorf("expected source=managed, got %s", resp.Source)
	}
	if len(resp.ArtistIDs) != 2 {
		t.Errorf("expected 2 artist IDs, got %d", len(resp.ArtistIDs))
	}
}

func TestFilterToResponse_EmptySourceDefaultsToUser(t *testing.T) {
	f := &notificationm.NotificationFilter{
		ID:   1,
		Name: "Legacy",
	}
	resp := filterToResponse(f)
	if resp.Source != notificationm.FilterSourceUser {
		t.Errorf("expected source=user for empty, got %s", resp.Source)
	}
}

func TestInt64ArrayToSlice(t *testing.T) {
	if int64ArrayToSlice(nil) != nil {
		t.Error("expected nil for nil input")
	}
	if int64ArrayToSlice([]int64{}) != nil {
		t.Error("expected nil for empty input")
	}
	result := int64ArrayToSlice([]int64{1, 2, 3})
	if len(result) != 3 {
		t.Errorf("expected 3, got %d", len(result))
	}
}

// =============================================================================
// Mock service and helper
// =============================================================================

// computeTestFilterSig mirrors notification.ComputeFilterUnsubscribeSignature.
// We recompute it here to avoid importing the notification package (which would
// create a circular dependency in test setup).
func computeTestFilterSig(filterID uint, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	// hash.Hash.Write never returns an error; the drop is intentional.
	_, _ = fmt.Fprintf(mac, "unsubscribe:filter:%d", filterID)
	return hex.EncodeToString(mac.Sum(nil))
}
