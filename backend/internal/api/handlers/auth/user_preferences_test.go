package auth

import (
	"context"
	"errors"
	"testing"

	"psychic-homily-backend/internal/api/handlers/shared/testhelpers"
	autherrors "psychic-homily-backend/internal/errors"
	authm "psychic-homily-backend/internal/models/auth"
	"psychic-homily-backend/internal/services/engagement"
)

// --- SetFavoriteCitiesHandler ---

func TestSetFavoriteCitiesHandler_NoAuth(t *testing.T) {
	h := NewUserPreferencesHandler(&testhelpers.MockUserService{}, "secret")
	req := &SetFavoriteCitiesRequest{}

	_, err := h.SetFavoriteCitiesHandler(context.Background(), req)
	testhelpers.AssertHumaError(t, err, 401)
}

func TestSetFavoriteCitiesHandler_Success(t *testing.T) {
	var calledWith []authm.FavoriteCity
	mock := &testhelpers.MockUserService{
		SetFavoriteCitiesFn: func(userID uint, cities []authm.FavoriteCity) error {
			calledWith = cities
			return nil
		},
	}
	h := NewUserPreferencesHandler(mock, "secret")
	user := &authm.User{ID: 1, IsActive: true}
	ctx := testhelpers.CtxWithUser(user)

	req := &SetFavoriteCitiesRequest{}
	req.Body.Cities = []authm.FavoriteCity{{City: "Phoenix", State: "AZ"}}

	resp, err := h.SetFavoriteCitiesHandler(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.Body.Success {
		t.Fatal("expected success=true")
	}
	if len(calledWith) != 1 || calledWith[0].City != "Phoenix" {
		t.Fatalf("expected Phoenix, got %+v", calledWith)
	}
}

func TestSetFavoriteCitiesHandler_NilCities(t *testing.T) {
	mock := &testhelpers.MockUserService{
		SetFavoriteCitiesFn: func(userID uint, cities []authm.FavoriteCity) error {
			if cities == nil {
				return errors.New("expected non-nil slice")
			}
			return nil
		},
	}
	h := NewUserPreferencesHandler(mock, "secret")
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1})

	req := &SetFavoriteCitiesRequest{}
	// Body.Cities is nil — handler should default to empty slice

	resp, err := h.SetFavoriteCitiesHandler(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.Body.Success {
		t.Fatal("expected success=true")
	}
}

func TestSetFavoriteCitiesHandler_ServiceError(t *testing.T) {
	mock := &testhelpers.MockUserService{
		SetFavoriteCitiesFn: func(userID uint, cities []authm.FavoriteCity) error {
			return errors.New("db error")
		},
	}
	h := NewUserPreferencesHandler(mock, "secret")
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1})
	req := &SetFavoriteCitiesRequest{}

	_, err := h.SetFavoriteCitiesHandler(ctx, req)
	testhelpers.AssertHumaError(t, err, 422)
}

// --- SetShowRemindersHandler ---

func TestSetShowRemindersHandler_NoAuth(t *testing.T) {
	h := NewUserPreferencesHandler(&testhelpers.MockUserService{}, "secret")
	req := &SetShowRemindersRequest{}

	_, err := h.SetShowRemindersHandler(context.Background(), req)
	testhelpers.AssertHumaError(t, err, 401)
}

func TestSetShowRemindersHandler_Success_Enable(t *testing.T) {
	var calledEnabled bool
	mock := &testhelpers.MockUserService{
		SetShowRemindersFn: func(userID uint, enabled bool) error {
			calledEnabled = enabled
			return nil
		},
	}
	h := NewUserPreferencesHandler(mock, "secret")
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1})
	req := &SetShowRemindersRequest{}
	req.Body.Enabled = true

	resp, err := h.SetShowRemindersHandler(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.Body.Success || !resp.Body.ShowReminders {
		t.Fatal("expected success=true and show_reminders=true")
	}
	if !calledEnabled {
		t.Fatal("expected service called with enabled=true")
	}
}

func TestSetShowRemindersHandler_Success_Disable(t *testing.T) {
	mock := &testhelpers.MockUserService{
		SetShowRemindersFn: func(userID uint, enabled bool) error {
			return nil
		},
	}
	h := NewUserPreferencesHandler(mock, "secret")
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1})
	req := &SetShowRemindersRequest{}
	req.Body.Enabled = false

	resp, err := h.SetShowRemindersHandler(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.Body.Success || resp.Body.ShowReminders {
		t.Fatal("expected success=true and show_reminders=false")
	}
}

func TestSetShowRemindersHandler_ServiceError(t *testing.T) {
	mock := &testhelpers.MockUserService{
		SetShowRemindersFn: func(userID uint, enabled bool) error {
			return errors.New("db error")
		},
	}
	h := NewUserPreferencesHandler(mock, "secret")
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1})
	req := &SetShowRemindersRequest{}
	req.Body.Enabled = true

	_, err := h.SetShowRemindersHandler(ctx, req)
	testhelpers.AssertHumaError(t, err, 422)
}

// --- UnsubscribeShowRemindersHandler ---

func TestUnsubscribeShowRemindersHandler_InvalidSignature(t *testing.T) {
	h := NewUserPreferencesHandler(&testhelpers.MockUserService{}, "secret")
	req := &UnsubscribeShowRemindersRequest{}
	req.Body.UID = 1
	req.Body.Sig = "invalid-sig"

	_, err := h.UnsubscribeShowRemindersHandler(context.Background(), req)
	testhelpers.AssertHumaError(t, err, 403)
}

func TestUnsubscribeShowRemindersHandler_ServiceError(t *testing.T) {
	// Use the real HMAC function to generate a valid signature
	secret := "test-jwt-secret"
	uid := uint(42)
	sig := computeTestUnsubscribeSig(uid, secret)

	mock := &testhelpers.MockUserService{
		SetShowRemindersFn: func(userID uint, enabled bool) error {
			return errors.New("db error")
		},
	}
	h := NewUserPreferencesHandler(mock, secret)
	req := &UnsubscribeShowRemindersRequest{}
	req.Body.UID = uid
	req.Body.Sig = sig

	_, err := h.UnsubscribeShowRemindersHandler(context.Background(), req)
	testhelpers.AssertHumaError(t, err, 500)
}

// computeTestUnsubscribeSig generates a valid HMAC signature for testing
func computeTestUnsubscribeSig(uid uint, secret string) string {
	return engagement.ComputeScopedUnsubscribeSignature(uid, engagement.UnsubscribeScopeShowReminders, secret)
}

// ──────────────────────────────────────────────
// PSY-289: comment-notifications preference + unsubscribe handlers
// ──────────────────────────────────────────────

func TestSetCommentNotificationsHandler_NoAuth(t *testing.T) {
	h := NewUserPreferencesHandler(&testhelpers.MockUserService{}, "secret")
	req := &SetCommentNotificationsRequest{}
	enabled := false
	req.Body.NotifyOnMention = &enabled
	_, err := h.SetCommentNotificationsHandler(context.Background(), req)
	testhelpers.AssertHumaError(t, err, 401)
}

func TestSetCommentNotificationsHandler_NoFieldsRejected(t *testing.T) {
	h := NewUserPreferencesHandler(&testhelpers.MockUserService{}, "secret")
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1})
	req := &SetCommentNotificationsRequest{}
	_, err := h.SetCommentNotificationsHandler(ctx, req)
	testhelpers.AssertHumaError(t, err, 422)
}

func TestSetCommentNotificationsHandler_Success(t *testing.T) {
	var commentEnabled, mentionEnabled *bool
	mock := &testhelpers.MockUserService{
		SetNotifyOnCommentSubscriptionFn: func(userID uint, enabled bool) error {
			e := enabled
			commentEnabled = &e
			return nil
		},
		SetNotifyOnMentionFn: func(userID uint, enabled bool) error {
			e := enabled
			mentionEnabled = &e
			return nil
		},
		GetUserByIDFn: func(uid uint) (*authm.User, error) {
			return &authm.User{
				ID: uid,
				Preferences: &authm.UserPreferences{
					UserID:                      uid,
					NotifyOnCommentSubscription: false,
					NotifyOnMention:             true,
				},
			}, nil
		},
	}
	h := NewUserPreferencesHandler(mock, "secret")
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1})

	no := false
	yes := true
	req := &SetCommentNotificationsRequest{}
	req.Body.NotifyOnCommentSubscription = &no
	req.Body.NotifyOnMention = &yes

	resp, err := h.SetCommentNotificationsHandler(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.Body.Success {
		t.Fatal("expected success=true")
	}
	if commentEnabled == nil || *commentEnabled {
		t.Fatalf("expected SetNotifyOnCommentSubscription(false); got %v", commentEnabled)
	}
	if mentionEnabled == nil || !*mentionEnabled {
		t.Fatalf("expected SetNotifyOnMention(true); got %v", mentionEnabled)
	}
	// Response mirrors DB state via GetUserByID.
	if resp.Body.NotifyOnCommentSubscription {
		t.Fatal("expected response to reflect DB state (false)")
	}
	if !resp.Body.NotifyOnMention {
		t.Fatal("expected response to reflect DB state (true)")
	}
}

func TestUnsubscribeCommentSubscriptionHandler_InvalidSignature(t *testing.T) {
	h := NewUserPreferencesHandler(&testhelpers.MockUserService{}, "secret")
	req := &UnsubscribeCommentSubscriptionRequest{}
	req.Body.UID = 1
	req.Body.EntityType = "artist"
	req.Body.EntityID = 1
	req.Body.Sig = "nope"
	_, err := h.UnsubscribeCommentSubscriptionHandler(context.Background(), req)
	testhelpers.AssertHumaError(t, err, 403)
}

func TestUnsubscribeCommentSubscriptionHandler_Success(t *testing.T) {
	secret := "hmac-secret"
	uid := uint(7)
	entityType := "release"
	entityID := uint(999)
	sig := engagement.ComputeCommentSubscriptionUnsubscribeSignature(uid, entityType, entityID, secret)

	var called bool
	var receivedEnabled *bool
	mock := &testhelpers.MockUserService{
		SetNotifyOnCommentSubscriptionFn: func(userID uint, enabled bool) error {
			called = true
			e := enabled
			receivedEnabled = &e
			return nil
		},
	}
	h := NewUserPreferencesHandler(mock, secret)
	req := &UnsubscribeCommentSubscriptionRequest{}
	req.Body.UID = uid
	req.Body.EntityType = entityType
	req.Body.EntityID = entityID
	req.Body.Sig = sig

	resp, err := h.UnsubscribeCommentSubscriptionHandler(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.Body.Success {
		t.Fatal("expected success=true")
	}
	if !called || receivedEnabled == nil || *receivedEnabled {
		t.Fatalf("expected SetNotifyOnCommentSubscription(false); called=%v got=%v", called, receivedEnabled)
	}
}

func TestUnsubscribeMentionHandler_InvalidSignature(t *testing.T) {
	h := NewUserPreferencesHandler(&testhelpers.MockUserService{}, "secret")
	req := &UnsubscribeMentionRequest{}
	req.Body.UID = 1
	req.Body.Sig = "bad"
	_, err := h.UnsubscribeMentionHandler(context.Background(), req)
	testhelpers.AssertHumaError(t, err, 403)
}

func TestUnsubscribeMentionHandler_Success(t *testing.T) {
	secret := "hmac-secret"
	uid := uint(8)
	sig := engagement.ComputeScopedUnsubscribeSignature(uid, engagement.UnsubscribeScopeMention, secret)

	var called bool
	mock := &testhelpers.MockUserService{
		SetNotifyOnMentionFn: func(userID uint, enabled bool) error {
			if enabled {
				t.Fatalf("expected enabled=false, got true")
			}
			called = true
			return nil
		},
	}
	h := NewUserPreferencesHandler(mock, secret)
	req := &UnsubscribeMentionRequest{}
	req.Body.UID = uid
	req.Body.Sig = sig

	resp, err := h.UnsubscribeMentionHandler(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.Body.Success {
		t.Fatal("expected success=true")
	}
	if !called {
		t.Fatal("expected SetNotifyOnMention to be called")
	}
}

// ──────────────────────────────────────────────
// PSY-621: SetDefaultReplyPermissionHandler validation contract
// ──────────────────────────────────────────────
//
// Mirrors the three-case shape used by PUT /comments/{id}/reply-permission
// (PSY-592 in comment_test.go: _EmptyPermission / _InvalidEnum /
// _AcceptsAllValidEnumValues). PSY-621 unifies the validation contract:
// missing/empty -> 400 "reply_permission is required"; unrecognized ->
// 400 with the canonical `shared.InvalidReplyPermissionMessage`. Previously
// invalid-enum returned 422 with a different message.

func TestSetDefaultReplyPermission_EmptyPermission(t *testing.T) {
	mock := &testhelpers.MockUserService{
		SetDefaultReplyPermissionFn: func(userID uint, permission string) error {
			t.Fatalf("service must not be invoked for empty permission; got call with permission=%q", permission)
			return nil
		},
	}
	h := NewUserPreferencesHandler(mock, "secret")
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1})
	req := &SetDefaultReplyPermissionRequest{}
	req.Body.Permission = "   "
	_, err := h.SetDefaultReplyPermissionHandler(ctx, req)
	testhelpers.AssertHumaErrorWithDetail(t, err, 400, "reply_permission is required")
}

// TestSetDefaultReplyPermission_InvalidEnum: an unrecognized value must
// be rejected with the explicit-list message, NOT
// "reply_permission is required" (which implies the field was absent).
// The service mock fails the test if invoked — the handler-level enum
// check must short-circuit before the service is called.
func TestSetDefaultReplyPermission_InvalidEnum(t *testing.T) {
	mock := &testhelpers.MockUserService{
		SetDefaultReplyPermissionFn: func(userID uint, permission string) error {
			t.Fatalf("service must not be invoked for invalid enum value; got call with permission=%q", permission)
			return nil
		},
	}
	h := NewUserPreferencesHandler(mock, "secret")
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1})
	req := &SetDefaultReplyPermissionRequest{}
	req.Body.Permission = "garbage"
	_, err := h.SetDefaultReplyPermissionHandler(ctx, req)
	testhelpers.AssertHumaErrorWithDetail(t, err, 400, "permission must be one of: anyone, followers, author_only")
}

// TestSetDefaultReplyPermission_AcceptsAllValidEnumValues: all three
// recognized enum values must clear the handler-level enum check and
// reach the service layer. Complements the _InvalidEnum and
// _EmptyPermission negative cases above.
func TestSetDefaultReplyPermission_AcceptsAllValidEnumValues(t *testing.T) {
	for _, perm := range []string{"anyone", "followers", "author_only"} {
		t.Run(perm, func(t *testing.T) {
			var called bool
			mock := &testhelpers.MockUserService{
				SetDefaultReplyPermissionFn: func(userID uint, permission string) error {
					called = true
					if permission != perm {
						t.Errorf("expected permission=%q, got %q", perm, permission)
					}
					return nil
				},
			}
			h := NewUserPreferencesHandler(mock, "secret")
			ctx := testhelpers.CtxWithUser(&authm.User{ID: 1})
			req := &SetDefaultReplyPermissionRequest{}
			req.Body.Permission = perm
			resp, err := h.SetDefaultReplyPermissionHandler(ctx, req)
			if err != nil {
				t.Fatalf("unexpected error for permission=%q: %v", perm, err)
			}
			if !called {
				t.Fatalf("expected service called for permission=%q", perm)
			}
			if !resp.Body.Success {
				t.Fatalf("expected success=true for permission=%q", perm)
			}
			if resp.Body.DefaultReplyPermission != perm {
				t.Errorf("expected default_reply_permission=%q, got %q", perm, resp.Body.DefaultReplyPermission)
			}
		})
	}
}

// TestSetDefaultReplyPermission_ServiceTypedInvalid: PSY-792 defence-in-depth.
// If the service-layer enum check is somehow reached (it surfaces a typed
// CodeInvalidReplyPermission), the handler maps it to 400 with the canonical
// message rather than string-matching. We bypass the handler's own enum guard
// by mocking the service to reject a value the handler considers valid.
func TestSetDefaultReplyPermission_ServiceTypedInvalid(t *testing.T) {
	mock := &testhelpers.MockUserService{
		SetDefaultReplyPermissionFn: func(_ uint, permission string) error {
			return autherrors.ErrInvalidReplyPermission(permission)
		},
	}
	h := NewUserPreferencesHandler(mock, "secret")
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1})
	req := &SetDefaultReplyPermissionRequest{}
	req.Body.Permission = "anyone"
	_, err := h.SetDefaultReplyPermissionHandler(ctx, req)
	testhelpers.AssertHumaErrorWithDetail(t, err, 400, "permission must be one of: anyone, followers, author_only")
}

// TestSetDefaultReplyPermission_ServiceError: a non-typed service fault (e.g.
// DB down) maps to 500, not 422 — PSY-792 corrects the prior 422-for-everything
// fallthrough to match the engagement (PSY-761) convention.
func TestSetDefaultReplyPermission_ServiceError(t *testing.T) {
	mock := &testhelpers.MockUserService{
		SetDefaultReplyPermissionFn: func(_ uint, _ string) error {
			return errors.New("db down")
		},
	}
	h := NewUserPreferencesHandler(mock, "secret")
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1})
	req := &SetDefaultReplyPermissionRequest{}
	req.Body.Permission = "anyone"
	_, err := h.SetDefaultReplyPermissionHandler(ctx, req)
	testhelpers.AssertHumaError(t, err, 500)
}

// --- SetCollectionDigestHandler ---

func TestSetCollectionDigestHandler_NoAuth(t *testing.T) {
	h := NewUserPreferencesHandler(&testhelpers.MockUserService{}, "secret")
	_, err := h.SetCollectionDigestHandler(context.Background(), &SetCollectionDigestRequest{})
	testhelpers.AssertHumaError(t, err, 401)
}

func TestSetCollectionDigestHandler_Success_Enable(t *testing.T) {
	var calledEnabled bool
	mock := &testhelpers.MockUserService{
		SetNotifyOnCollectionDigestFn: func(userID uint, enabled bool) error {
			if userID != 1 {
				t.Errorf("expected userID=1, got %d", userID)
			}
			calledEnabled = enabled
			return nil
		},
	}
	h := NewUserPreferencesHandler(mock, "secret")
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1})
	req := &SetCollectionDigestRequest{}
	req.Body.Enabled = true

	resp, err := h.SetCollectionDigestHandler(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.Body.Success || !resp.Body.NotifyOnCollectionDigest {
		t.Fatal("expected success=true and notify_on_collection_digest=true")
	}
	if !calledEnabled {
		t.Fatal("expected service called with enabled=true")
	}
}

func TestSetCollectionDigestHandler_Success_Disable(t *testing.T) {
	mock := &testhelpers.MockUserService{
		SetNotifyOnCollectionDigestFn: func(_ uint, _ bool) error { return nil },
	}
	h := NewUserPreferencesHandler(mock, "secret")
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1})
	req := &SetCollectionDigestRequest{}
	req.Body.Enabled = false

	resp, err := h.SetCollectionDigestHandler(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.Body.Success || resp.Body.NotifyOnCollectionDigest {
		t.Fatal("expected success=true and notify_on_collection_digest=false")
	}
}

func TestSetCollectionDigestHandler_ServiceError(t *testing.T) {
	mock := &testhelpers.MockUserService{
		SetNotifyOnCollectionDigestFn: func(_ uint, _ bool) error {
			return errors.New("db error")
		},
	}
	h := NewUserPreferencesHandler(mock, "secret")
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1})
	req := &SetCollectionDigestRequest{}
	req.Body.Enabled = true

	_, err := h.SetCollectionDigestHandler(ctx, req)
	testhelpers.AssertHumaError(t, err, 500)
}

// ──────────────────────────────────────────────
// PSY-756: tier-edit-notifications preference handler
// ──────────────────────────────────────────────

func TestSetTierEditNotificationsHandler_NoAuth(t *testing.T) {
	h := NewUserPreferencesHandler(&testhelpers.MockUserService{}, "secret")
	req := &SetTierEditNotificationsRequest{}
	enabled := false
	req.Body.NotifyOnTierNotifications = &enabled
	_, err := h.SetTierEditNotificationsHandler(context.Background(), req)
	testhelpers.AssertHumaError(t, err, 401)
}

func TestSetTierEditNotificationsHandler_NoFieldsRejected(t *testing.T) {
	h := NewUserPreferencesHandler(&testhelpers.MockUserService{}, "secret")
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1})
	req := &SetTierEditNotificationsRequest{}
	_, err := h.SetTierEditNotificationsHandler(ctx, req)
	testhelpers.AssertHumaError(t, err, 422)
}

func TestSetTierEditNotificationsHandler_Success(t *testing.T) {
	var tierEnabled, editEnabled *bool
	mock := &testhelpers.MockUserService{
		SetNotifyOnTierNotificationsFn: func(_ uint, enabled bool) error {
			e := enabled
			tierEnabled = &e
			return nil
		},
		SetNotifyOnEditNotificationsFn: func(_ uint, enabled bool) error {
			e := enabled
			editEnabled = &e
			return nil
		},
		GetUserByIDFn: func(uid uint) (*authm.User, error) {
			return &authm.User{
				ID: uid,
				Preferences: &authm.UserPreferences{
					UserID:                    uid,
					NotifyOnTierNotifications: false,
					NotifyOnEditNotifications: true,
				},
			}, nil
		},
	}
	h := NewUserPreferencesHandler(mock, "secret")
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1})

	no := false
	yes := true
	req := &SetTierEditNotificationsRequest{}
	req.Body.NotifyOnTierNotifications = &no
	req.Body.NotifyOnEditNotifications = &yes

	resp, err := h.SetTierEditNotificationsHandler(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.Body.Success {
		t.Fatal("expected success=true")
	}
	if tierEnabled == nil || *tierEnabled {
		t.Fatalf("expected SetNotifyOnTierNotifications(false); got %v", tierEnabled)
	}
	if editEnabled == nil || !*editEnabled {
		t.Fatalf("expected SetNotifyOnEditNotifications(true); got %v", editEnabled)
	}
	// Response mirrors DB state via GetUserByID.
	if resp.Body.NotifyOnTierNotifications {
		t.Fatal("expected response to reflect DB state (false)")
	}
	if !resp.Body.NotifyOnEditNotifications {
		t.Fatal("expected response to reflect DB state (true)")
	}
}

// --- SetSceneDigestHandler (PSY-1342) ---

func TestSetSceneDigestHandler_NoAuth(t *testing.T) {
	h := NewUserPreferencesHandler(&testhelpers.MockUserService{}, "secret")
	_, err := h.SetSceneDigestHandler(context.Background(), &SetSceneDigestRequest{})
	testhelpers.AssertHumaError(t, err, 401)
}

func TestSetSceneDigestHandler_Success_Enable(t *testing.T) {
	var calledEnabled bool
	mock := &testhelpers.MockUserService{
		SetNotifyOnSceneDigestFn: func(userID uint, enabled bool) error {
			if userID != 1 {
				t.Errorf("expected userID=1, got %d", userID)
			}
			calledEnabled = enabled
			return nil
		},
	}
	h := NewUserPreferencesHandler(mock, "secret")
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1})
	req := &SetSceneDigestRequest{}
	req.Body.Enabled = true

	resp, err := h.SetSceneDigestHandler(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.Body.Success || !resp.Body.NotifyOnSceneDigest {
		t.Fatal("expected success=true and notify_on_scene_digest=true")
	}
	if !calledEnabled {
		t.Fatal("expected service called with enabled=true")
	}
}

func TestSetSceneDigestHandler_ServiceError(t *testing.T) {
	mock := &testhelpers.MockUserService{
		SetNotifyOnSceneDigestFn: func(_ uint, _ bool) error { return errors.New("db error") },
	}
	h := NewUserPreferencesHandler(mock, "secret")
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1})
	req := &SetSceneDigestRequest{}
	req.Body.Enabled = true
	_, err := h.SetSceneDigestHandler(ctx, req)
	testhelpers.AssertHumaError(t, err, 500)
}
