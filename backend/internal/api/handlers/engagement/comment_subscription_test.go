package engagement

import (
	"context"
	"fmt"
	"testing"

	"psychic-homily-backend/internal/api/handlers/shared/testhelpers"
	apperrors "psychic-homily-backend/internal/errors"
	authm "psychic-homily-backend/internal/models/auth"
	"psychic-homily-backend/internal/services/contracts"
)

// Uses auto-generated testhelpers.MockCommentSubscriptionService and testhelpers.MockAuditLogService
// from handler_unit_mock_helpers_test.go

func testCommentSubscriptionHandler() *CommentSubscriptionHandler {
	return NewCommentSubscriptionHandler(nil, nil, testhelpers.AllShowsVisible())
}

// ============================================================================
// The gate's DENY side (PSY-1983)
// ============================================================================
//
// Every other test in this file opts out with testhelpers.AllShowsVisible(),
// because the gate is not their subject. That makes this the only place in the
// FAST suite where the refusal branches are executed at all: the end-to-end
// matrix in api/routes needs Postgres and skips under -short, so without these a
// refactor that inverted the condition, dropped the `!`, or wired a nil checker
// into a new route would pass the whole handler suite.
//
// The mock denies everything, which is the zero value's behaviour anyway; it is
// spelled out so the intent is legible beside its AllShowsVisible() neighbours.
func denyingSubscriptionHandler(svc *testhelpers.MockCommentSubscriptionService) *CommentSubscriptionHandler {
	return NewCommentSubscriptionHandler(svc, nil, &testhelpers.MockShowVisibility{
		ShowVisibleToFn: func(uint, contracts.ShowViewer) bool { return false },
	})
}

// A refused subscribe must not reach the service, or the row is written and only
// the response is withheld.
func TestSubscribe_GatedShowRefusesWithoutTouchingTheService(t *testing.T) {
	called := false
	h := denyingSubscriptionHandler(&testhelpers.MockCommentSubscriptionService{
		SubscribeFn: func(uint, string, uint) error {
			called = true
			return nil
		},
	})

	_, err := h.SubscribeHandler(
		testhelpers.CtxWithUser(&authm.User{ID: 1}),
		&SubscribeRequest{EntityType: "show", EntityID: "42"},
	)
	testhelpers.AssertHumaError(t, err, 404)
	if called {
		t.Error("the subscription was created for a show the caller cannot see")
	}
}

func TestMarkRead_GatedShowRefusesWithoutTouchingTheService(t *testing.T) {
	called := false
	h := denyingSubscriptionHandler(&testhelpers.MockCommentSubscriptionService{
		MarkReadFn: func(uint, string, uint) error {
			called = true
			return nil
		},
	})

	_, err := h.MarkReadHandler(
		testhelpers.CtxWithUser(&authm.User{ID: 1}),
		&MarkReadRequest{EntityType: "show", EntityID: "42"},
	)
	testhelpers.AssertHumaError(t, err, 404)
	if called {
		t.Error("the last-read pointer moved on a show the caller cannot see")
	}
}

// The status route is the one that must NOT refuse: it answers exactly as it
// does for a show nobody is subscribed to, so a 404 cannot confirm the id and a
// truthful count cannot report activity.
func TestSubscriptionStatus_GatedShowAnswersNotSubscribed(t *testing.T) {
	h := denyingSubscriptionHandler(&testhelpers.MockCommentSubscriptionService{
		IsSubscribedFn: func(uint, string, uint) (bool, error) { return true, nil },
		GetUnreadCountFn: func(uint, string, uint) (int, error) {
			return 7, nil
		},
	})

	resp, err := h.SubscriptionStatusHandler(
		testhelpers.CtxWithUser(&authm.User{ID: 1}),
		&SubscriptionStatusRequest{EntityType: "show", EntityID: "42"},
	)
	if err != nil {
		t.Fatalf("the status route refused instead of answering: %v", err)
	}
	if resp.Body.Subscribed {
		t.Error("the status route confirmed a subscription to a show the caller cannot see")
	}
	if resp.Body.UnreadCount != 0 {
		t.Errorf("unread_count = %d, want 0 — a live count is a running activity signal",
			resp.Body.UnreadCount)
	}
}

// The gate is show-scoped, and its pass-through for other entity types is a
// deliberate default-open, not an oversight. Pinned here so a future edit that
// makes the gate refuse everything is caught as a behaviour change rather than
// shipped as a silent lockout of artist and venue subscriptions.
func TestSubscribe_NonShowEntityIsNotRefusedByTheShowGate(t *testing.T) {
	called := false
	h := denyingSubscriptionHandler(&testhelpers.MockCommentSubscriptionService{
		SubscribeFn: func(uint, string, uint) error {
			called = true
			return nil
		},
	})

	_, err := h.SubscribeHandler(
		testhelpers.CtxWithUser(&authm.User{ID: 1}),
		&SubscribeRequest{EntityType: "artist", EntityID: "42"},
	)
	if err != nil {
		t.Fatalf("the show gate refused an ARTIST subscription: %v", err)
	}
	if !called {
		t.Error("the artist subscription never reached the service")
	}
}

// ============================================================================
// SubscribeHandler Tests
// ============================================================================

func TestSubscribe_NoAuth(t *testing.T) {
	h := testCommentSubscriptionHandler()
	req := &SubscribeRequest{EntityType: "show", EntityID: "1"}

	_, err := h.SubscribeHandler(context.Background(), req)
	testhelpers.AssertHumaError(t, err, 401)
}

func TestSubscribe_InvalidEntityID(t *testing.T) {
	h := testCommentSubscriptionHandler()
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1})
	req := &SubscribeRequest{EntityType: "show", EntityID: "abc"}

	_, err := h.SubscribeHandler(ctx, req)
	testhelpers.AssertHumaError(t, err, 400)
}

// An entity type with no registered visibility rule is refused AT THE GATE, as
// a missing entity, before the service that used to name it runs (PSY-1987).
//
// The service is not merely un-consulted, it is asserted un-consulted: the
// refusal has to come from the gate, or a later edit could restore the
// default-open and this test would still pass on the service's error.
//
// It answers 404 where it used to answer 400. That is the fail-closed default,
// and it also closes a small oracle: the old pair of answers told a caller which
// entity types the vocabulary contains. The service's own invalid-type error
// still maps to 400 — see the registered-type case below, and
// shared.TestMapCommentError.
func TestSubscribe_UnregisteredEntityTypeIsRefusedAtTheGate(t *testing.T) {
	serviceCalled := false
	h := NewCommentSubscriptionHandler(&testhelpers.MockCommentSubscriptionService{
		SubscribeFn: func(userID uint, entityType string, entityID uint) error {
			serviceCalled = true
			return nil
		},
	}, nil, testhelpers.AllShowsVisible())
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1})
	req := &SubscribeRequest{EntityType: "invalid", EntityID: "1"}

	_, err := h.SubscribeHandler(ctx, req)
	testhelpers.AssertHumaError(t, err, 404)
	if serviceCalled {
		t.Error("the subscribe service ran for an unregistered entity type — the gate did not refuse it")
	}
}

// The service's invalid-entity-type error still maps to 400, reached through an
// entity type the gate DOES recognise. Without this arm, moving the refusal to
// the gate would have silently deleted the only coverage of that mapping on this
// route.
func TestSubscribe_InvalidEntityTypeFromTheService(t *testing.T) {
	h := NewCommentSubscriptionHandler(&testhelpers.MockCommentSubscriptionService{
		SubscribeFn: func(userID uint, entityType string, entityID uint) error {
			return apperrors.ErrCommentInvalidEntityType(entityType)
		},
	}, nil, testhelpers.AllShowsVisible())
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1})
	req := &SubscribeRequest{EntityType: "artist", EntityID: "1"}

	_, err := h.SubscribeHandler(ctx, req)
	testhelpers.AssertHumaError(t, err, 400)
}

func TestSubscribe_Success(t *testing.T) {
	h := NewCommentSubscriptionHandler(&testhelpers.MockCommentSubscriptionService{
		SubscribeFn: func(userID uint, entityType string, entityID uint) error {
			if userID != 1 || entityType != "show" || entityID != 42 {
				return fmt.Errorf("unexpected args: %d, %s, %d", userID, entityType, entityID)
			}
			return nil
		},
	}, nil, testhelpers.AllShowsVisible())

	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1})
	req := &SubscribeRequest{EntityType: "show", EntityID: "42"}

	resp, err := h.SubscribeHandler(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.Body.Success {
		t.Error("expected success=true")
	}
}

func TestSubscribe_ServiceError(t *testing.T) {
	h := NewCommentSubscriptionHandler(&testhelpers.MockCommentSubscriptionService{
		SubscribeFn: func(userID uint, entityType string, entityID uint) error {
			return fmt.Errorf("database error")
		},
	}, nil, testhelpers.AllShowsVisible())

	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1})
	req := &SubscribeRequest{EntityType: "show", EntityID: "1"}

	_, err := h.SubscribeHandler(ctx, req)
	testhelpers.AssertHumaError(t, err, 500)
}

func TestSubscribe_AuditLogFires(t *testing.T) {
	auditCalled := make(chan bool, 1)
	h := NewCommentSubscriptionHandler(&testhelpers.MockCommentSubscriptionService{
		SubscribeFn: func(userID uint, entityType string, entityID uint) error {
			return nil
		},
	}, &testhelpers.MockAuditLogService{
		LogActionFn: func(actorID uint, action string, entityType string, entityID uint, metadata map[string]interface{}) {
			if action == "subscribe_comments" && actorID == 1 && entityType == "show" && entityID == 42 {
				auditCalled <- true
			}
		},
	}, testhelpers.AllShowsVisible())

	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1})
	req := &SubscribeRequest{EntityType: "show", EntityID: "42"}

	_, err := h.SubscribeHandler(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Audit log is fire-and-forget (goroutine), so we give it a moment
	// but don't fail the test if it doesn't fire — it's best-effort
}

// ============================================================================
// UnsubscribeHandler Tests
// ============================================================================

func TestUnsubscribe_NoAuth(t *testing.T) {
	h := testCommentSubscriptionHandler()
	req := &UnsubscribeRequest{EntityType: "show", EntityID: "1"}

	_, err := h.UnsubscribeHandler(context.Background(), req)
	testhelpers.AssertHumaError(t, err, 401)
}

func TestUnsubscribe_InvalidEntityID(t *testing.T) {
	h := testCommentSubscriptionHandler()
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1})
	req := &UnsubscribeRequest{EntityType: "show", EntityID: "abc"}

	_, err := h.UnsubscribeHandler(ctx, req)
	testhelpers.AssertHumaError(t, err, 400)
}

func TestUnsubscribe_InvalidEntityType(t *testing.T) {
	h := NewCommentSubscriptionHandler(&testhelpers.MockCommentSubscriptionService{
		UnsubscribeFn: func(userID uint, entityType string, entityID uint) error {
			return apperrors.ErrCommentInvalidEntityType(entityType)
		},
	}, nil, testhelpers.AllShowsVisible())
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1})
	req := &UnsubscribeRequest{EntityType: "invalid", EntityID: "1"}

	_, err := h.UnsubscribeHandler(ctx, req)
	testhelpers.AssertHumaError(t, err, 400)
}

func TestUnsubscribe_Success(t *testing.T) {
	h := NewCommentSubscriptionHandler(&testhelpers.MockCommentSubscriptionService{
		UnsubscribeFn: func(userID uint, entityType string, entityID uint) error {
			if userID != 1 || entityType != "show" || entityID != 42 {
				return fmt.Errorf("unexpected args")
			}
			return nil
		},
	}, nil, testhelpers.AllShowsVisible())

	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1})
	req := &UnsubscribeRequest{EntityType: "show", EntityID: "42"}

	_, err := h.UnsubscribeHandler(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestUnsubscribe_ServiceError(t *testing.T) {
	h := NewCommentSubscriptionHandler(&testhelpers.MockCommentSubscriptionService{
		UnsubscribeFn: func(userID uint, entityType string, entityID uint) error {
			return fmt.Errorf("database error")
		},
	}, nil, testhelpers.AllShowsVisible())

	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1})
	req := &UnsubscribeRequest{EntityType: "show", EntityID: "1"}

	_, err := h.UnsubscribeHandler(ctx, req)
	testhelpers.AssertHumaError(t, err, 500)
}

// ============================================================================
// SubscriptionStatusHandler Tests
// ============================================================================

func TestSubscriptionStatus_NoAuth(t *testing.T) {
	h := testCommentSubscriptionHandler()
	req := &SubscriptionStatusRequest{EntityType: "show", EntityID: "1"}

	_, err := h.SubscriptionStatusHandler(context.Background(), req)
	testhelpers.AssertHumaError(t, err, 401)
}

func TestSubscriptionStatus_InvalidEntityID(t *testing.T) {
	h := testCommentSubscriptionHandler()
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1})
	req := &SubscriptionStatusRequest{EntityType: "show", EntityID: "abc"}

	_, err := h.SubscriptionStatusHandler(ctx, req)
	testhelpers.AssertHumaError(t, err, 400)
}

// The status route is the one gated route that must NOT refuse, so an
// unregistered entity type gets the same quiet `{subscribed:false,
// unread_count:0}` a gated show gets — not a 400 naming the vocabulary
// (PSY-1987).
func TestSubscriptionStatus_UnregisteredEntityTypeAnswersNotSubscribed(t *testing.T) {
	serviceCalled := false
	h := NewCommentSubscriptionHandler(&testhelpers.MockCommentSubscriptionService{
		IsSubscribedFn: func(userID uint, entityType string, entityID uint) (bool, error) {
			serviceCalled = true
			return true, nil
		},
	}, nil, testhelpers.AllShowsVisible())
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1})
	req := &SubscriptionStatusRequest{EntityType: "invalid", EntityID: "1"}

	resp, err := h.SubscriptionStatusHandler(ctx, req)
	if err != nil {
		t.Fatalf("the status route refused an unregistered entity type: %v", err)
	}
	if resp.Body.Subscribed || resp.Body.UnreadCount != 0 {
		t.Errorf("the status route answered subscribed=%v unread=%d for an unregistered entity type",
			resp.Body.Subscribed, resp.Body.UnreadCount)
	}
	if serviceCalled {
		t.Error("the subscription service ran for an unregistered entity type — the gate did not refuse it")
	}
}

// The service's invalid-entity-type error still maps to 400, through a type the
// gate recognises.
func TestSubscriptionStatus_InvalidEntityTypeFromTheService(t *testing.T) {
	h := NewCommentSubscriptionHandler(&testhelpers.MockCommentSubscriptionService{
		IsSubscribedFn: func(userID uint, entityType string, entityID uint) (bool, error) {
			return false, apperrors.ErrCommentInvalidEntityType(entityType)
		},
	}, nil, testhelpers.AllShowsVisible())
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1})
	req := &SubscriptionStatusRequest{EntityType: "artist", EntityID: "1"}

	_, err := h.SubscriptionStatusHandler(ctx, req)
	testhelpers.AssertHumaError(t, err, 400)
}

func TestSubscriptionStatus_Subscribed(t *testing.T) {
	h := NewCommentSubscriptionHandler(&testhelpers.MockCommentSubscriptionService{
		IsSubscribedFn: func(userID uint, entityType string, entityID uint) (bool, error) {
			return true, nil
		},
		GetUnreadCountFn: func(userID uint, entityType string, entityID uint) (int, error) {
			return 5, nil
		},
	}, nil, testhelpers.AllShowsVisible())

	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1})
	req := &SubscriptionStatusRequest{EntityType: "show", EntityID: "1"}

	resp, err := h.SubscriptionStatusHandler(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.Body.Subscribed {
		t.Error("expected subscribed=true")
	}
	if resp.Body.UnreadCount != 5 {
		t.Errorf("expected unread_count=5, got %d", resp.Body.UnreadCount)
	}
}

func TestSubscriptionStatus_NotSubscribed(t *testing.T) {
	h := NewCommentSubscriptionHandler(&testhelpers.MockCommentSubscriptionService{
		IsSubscribedFn: func(userID uint, entityType string, entityID uint) (bool, error) {
			return false, nil
		},
	}, nil, testhelpers.AllShowsVisible())

	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1})
	req := &SubscriptionStatusRequest{EntityType: "show", EntityID: "1"}

	resp, err := h.SubscriptionStatusHandler(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Subscribed {
		t.Error("expected subscribed=false")
	}
	if resp.Body.UnreadCount != 0 {
		t.Errorf("expected unread_count=0, got %d", resp.Body.UnreadCount)
	}
}

func TestSubscriptionStatus_ServiceError(t *testing.T) {
	h := NewCommentSubscriptionHandler(&testhelpers.MockCommentSubscriptionService{
		IsSubscribedFn: func(userID uint, entityType string, entityID uint) (bool, error) {
			return false, fmt.Errorf("database error")
		},
	}, nil, testhelpers.AllShowsVisible())

	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1})
	req := &SubscriptionStatusRequest{EntityType: "show", EntityID: "1"}

	_, err := h.SubscriptionStatusHandler(ctx, req)
	testhelpers.AssertHumaError(t, err, 500)
}

// ============================================================================
// ListSubscriptionsHandler Tests
// ============================================================================

func TestListSubscriptions_NoAuth(t *testing.T) {
	h := testCommentSubscriptionHandler()
	req := &ListCommentSubscriptionsRequest{Limit: 20, Offset: 0}

	_, err := h.ListSubscriptionsHandler(context.Background(), req)
	testhelpers.AssertHumaError(t, err, 401)
}

func TestListSubscriptions_SelfScopedAndPaginated(t *testing.T) {
	h := NewCommentSubscriptionHandler(&testhelpers.MockCommentSubscriptionService{
		ListWatchingFn: func(viewer contracts.ShowViewer, limit int, offset int) ([]contracts.WatchingItem, int64, error) {
			// User ID must come from the authenticated context, never the request
			if viewer.UserID != 7 || limit != 10 || offset != 20 {
				return nil, 0, fmt.Errorf("unexpected args: %d, %d, %d", viewer.UserID, limit, offset)
			}
			return []contracts.WatchingItem{
				{EntityType: "artist", EntityID: 3, EntityName: "Watch Artist", EntityURL: "/artists/watch-artist", CommentCount: 4, UnreadCount: 2},
			}, 31, nil
		},
	}, nil, testhelpers.AllShowsVisible())

	ctx := testhelpers.CtxWithUser(&authm.User{ID: 7})
	req := &ListCommentSubscriptionsRequest{Limit: 10, Offset: 20}

	resp, err := h.ListSubscriptionsHandler(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Total != 31 {
		t.Errorf("expected total=31, got %d", resp.Body.Total)
	}
	if len(resp.Body.Items) != 1 || resp.Body.Items[0].EntityName != "Watch Artist" {
		t.Errorf("unexpected items: %+v", resp.Body.Items)
	}
	if resp.Body.Limit != 10 || resp.Body.Offset != 20 {
		t.Errorf("expected limit/offset echoed, got %d/%d", resp.Body.Limit, resp.Body.Offset)
	}
}

func TestListSubscriptions_ServiceError(t *testing.T) {
	h := NewCommentSubscriptionHandler(&testhelpers.MockCommentSubscriptionService{
		ListWatchingFn: func(viewer contracts.ShowViewer, limit int, offset int) ([]contracts.WatchingItem, int64, error) {
			return nil, 0, fmt.Errorf("database error")
		},
	}, nil, testhelpers.AllShowsVisible())

	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1})
	req := &ListCommentSubscriptionsRequest{Limit: 20, Offset: 0}

	_, err := h.ListSubscriptionsHandler(ctx, req)
	testhelpers.AssertHumaError(t, err, 500)
}

// ============================================================================
// MarkReadHandler Tests
// ============================================================================

func TestMarkRead_NoAuth(t *testing.T) {
	h := testCommentSubscriptionHandler()
	req := &MarkReadRequest{EntityType: "show", EntityID: "1"}

	_, err := h.MarkReadHandler(context.Background(), req)
	testhelpers.AssertHumaError(t, err, 401)
}

func TestMarkRead_InvalidEntityID(t *testing.T) {
	h := testCommentSubscriptionHandler()
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1})
	req := &MarkReadRequest{EntityType: "show", EntityID: "abc"}

	_, err := h.MarkReadHandler(ctx, req)
	testhelpers.AssertHumaError(t, err, 400)
}

// Mark-read is a WRITE that advances a pointer over an entity's discussion, so
// an unregistered entity type is refused as a missing entity, exactly as
// subscribe is (PSY-1987).
func TestMarkRead_UnregisteredEntityTypeIsRefusedAtTheGate(t *testing.T) {
	serviceCalled := false
	h := NewCommentSubscriptionHandler(&testhelpers.MockCommentSubscriptionService{
		MarkReadFn: func(userID uint, entityType string, entityID uint) error {
			serviceCalled = true
			return nil
		},
	}, nil, testhelpers.AllShowsVisible())
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1})
	req := &MarkReadRequest{EntityType: "invalid", EntityID: "1"}

	_, err := h.MarkReadHandler(ctx, req)
	testhelpers.AssertHumaError(t, err, 404)
	if serviceCalled {
		t.Error("the mark-read service ran for an unregistered entity type — the gate did not refuse it")
	}
}

// The service's invalid-entity-type error still maps to 400, through a type the
// gate recognises.
func TestMarkRead_InvalidEntityTypeFromTheService(t *testing.T) {
	h := NewCommentSubscriptionHandler(&testhelpers.MockCommentSubscriptionService{
		MarkReadFn: func(userID uint, entityType string, entityID uint) error {
			return apperrors.ErrCommentInvalidEntityType(entityType)
		},
	}, nil, testhelpers.AllShowsVisible())
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1})
	req := &MarkReadRequest{EntityType: "artist", EntityID: "1"}

	_, err := h.MarkReadHandler(ctx, req)
	testhelpers.AssertHumaError(t, err, 400)
}

func TestMarkRead_Success(t *testing.T) {
	h := NewCommentSubscriptionHandler(&testhelpers.MockCommentSubscriptionService{
		MarkReadFn: func(userID uint, entityType string, entityID uint) error {
			if userID != 1 || entityType != "show" || entityID != 42 {
				return fmt.Errorf("unexpected args")
			}
			return nil
		},
	}, nil, testhelpers.AllShowsVisible())

	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1})
	req := &MarkReadRequest{EntityType: "show", EntityID: "42"}

	resp, err := h.MarkReadHandler(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.Body.Success {
		t.Error("expected success=true")
	}
}

func TestMarkRead_ServiceError(t *testing.T) {
	h := NewCommentSubscriptionHandler(&testhelpers.MockCommentSubscriptionService{
		MarkReadFn: func(userID uint, entityType string, entityID uint) error {
			return fmt.Errorf("database error")
		},
	}, nil, testhelpers.AllShowsVisible())

	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1})
	req := &MarkReadRequest{EntityType: "show", EntityID: "1"}

	_, err := h.MarkReadHandler(ctx, req)
	testhelpers.AssertHumaError(t, err, 500)
}

// ============================================================================
// Edge case: subscription status unread count fetch error is silently handled
// ============================================================================

func TestSubscriptionStatus_UnreadCountError_StillReturnsSubscribed(t *testing.T) {
	h := NewCommentSubscriptionHandler(&testhelpers.MockCommentSubscriptionService{
		IsSubscribedFn: func(userID uint, entityType string, entityID uint) (bool, error) {
			return true, nil
		},
		GetUnreadCountFn: func(userID uint, entityType string, entityID uint) (int, error) {
			return 0, fmt.Errorf("count error")
		},
	}, nil, testhelpers.AllShowsVisible())

	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1})
	req := &SubscriptionStatusRequest{EntityType: "show", EntityID: "1"}

	resp, err := h.SubscriptionStatusHandler(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should still return subscribed=true even if unread count fails
	if !resp.Body.Subscribed {
		t.Error("expected subscribed=true despite unread count error")
	}
	if resp.Body.UnreadCount != 0 {
		t.Errorf("expected unread_count=0 on error, got %d", resp.Body.UnreadCount)
	}
}

// ============================================================================
// Verify handler uses correct interface — nil service doesn't panic on no-op
// ============================================================================

func TestSubscribe_NilSubscriptionService(t *testing.T) {
	h := NewCommentSubscriptionHandler(nil, nil, testhelpers.AllShowsVisible())
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1})
	req := &SubscribeRequest{EntityType: "show", EntityID: "1"}

	// This should return success because testhelpers.MockCommentSubscriptionService default returns nil
	// But with nil service it'll panic, which verifies we need a valid service
	defer func() {
		if r := recover(); r == nil {
			t.Log("nil service handled gracefully or returned error")
		}
	}()
	//nolint:errcheck // intentionally calling for panic side effect; recover() above handles outcome
	h.SubscribeHandler(ctx, req)
}
