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

// PSY-1893: the alert subscription carried by a follow.

func artistShowAlerts() *contracts.FollowAlertSettings {
	return &contracts.FollowAlertSettings{
		EntityType: "artist",
		EntityID:   42,
		Shows: contracts.FollowAlertPreference{
			Enabled: true, InApp: true, Email: false,
			Scope: contracts.FollowAlertScopeNearMe,
		},
		Releases: &contracts.FollowAlertPreference{Enabled: true, InApp: true},
	}
}

// --- GetFollowAlertsHandler ---

func TestGetFollowAlertsHandler_NoAuth(t *testing.T) {
	h := NewFollowHandler(&testhelpers.MockFollowService{})

	_, err := h.GetFollowAlertsHandler(context.Background(),
		&FollowAlertsRequest{EntityType: "artists", EntityID: "42"})
	testhelpers.AssertHumaError(t, err, 401)
}

func TestGetFollowAlertsHandler_InvalidEntityType(t *testing.T) {
	h := NewFollowHandler(&testhelpers.MockFollowService{})
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1})

	_, err := h.GetFollowAlertsHandler(ctx, &FollowAlertsRequest{EntityType: "shows", EntityID: "42"})
	testhelpers.AssertHumaError(t, err, 400)
}

func TestGetFollowAlertsHandler_InvalidID(t *testing.T) {
	h := NewFollowHandler(&testhelpers.MockFollowService{})
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1})

	_, err := h.GetFollowAlertsHandler(ctx, &FollowAlertsRequest{EntityType: "artists", EntityID: "abc"})
	testhelpers.AssertHumaError(t, err, 400)
}

func TestGetFollowAlertsHandler_Success(t *testing.T) {
	mock := &testhelpers.MockFollowService{
		GetFollowAlertSettingsFn: func(userID uint, entityType string, entityID uint) (*contracts.FollowAlertSettings, error) {
			if userID != 7 || entityType != "artist" || entityID != 42 {
				t.Errorf("unexpected args: userID=%d entityType=%s entityID=%d", userID, entityType, entityID)
			}
			return artistShowAlerts(), nil
		},
	}
	h := NewFollowHandler(mock)
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 7})

	resp, err := h.GetFollowAlertsHandler(ctx, &FollowAlertsRequest{EntityType: "artists", EntityID: "42"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.Body.Shows.InApp || resp.Body.Shows.Email {
		t.Errorf("shows = %+v, want in-app on and email off", resp.Body.Shows)
	}
	if resp.Body.Shows.Scope != contracts.FollowAlertScopeNearMe {
		t.Errorf("scope = %q, want near_me", resp.Body.Shows.Scope)
	}
	if resp.CacheControl != "no-store" {
		t.Errorf("Cache-Control = %q, want no-store", resp.CacheControl)
	}
}

// A follow that isn't there is a 404 on the sub-resource, not a silent
// empty subscription.
func TestGetFollowAlertsHandler_NotFollowingIs404(t *testing.T) {
	mock := &testhelpers.MockFollowService{
		GetFollowAlertSettingsFn: func(_ uint, _ string, _ uint) (*contracts.FollowAlertSettings, error) {
			return nil, apperrors.ErrFollowNotFound("artist", 42)
		},
	}
	h := NewFollowHandler(mock)
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1})

	_, err := h.GetFollowAlertsHandler(ctx, &FollowAlertsRequest{EntityType: "artists", EntityID: "42"})
	testhelpers.AssertHumaError(t, err, 404)
}

// A follow type with no alert subscription (label, festival, tag, radio show)
// is a semantic validation failure, not a 404.
func TestGetFollowAlertsHandler_AlertlessEntityTypeIs422(t *testing.T) {
	for _, entityType := range []string{"labels", "festivals", "tags", "radio-shows"} {
		t.Run(entityType, func(t *testing.T) {
			mock := &testhelpers.MockFollowService{
				GetFollowAlertSettingsFn: func(_ uint, et string, _ uint) (*contracts.FollowAlertSettings, error) {
					return nil, apperrors.ErrFollowInvalidEntityType(et)
				},
			}
			h := NewFollowHandler(mock)
			ctx := testhelpers.CtxWithUser(&authm.User{ID: 1})

			_, err := h.GetFollowAlertsHandler(ctx, &FollowAlertsRequest{EntityType: entityType, EntityID: "1"})
			testhelpers.AssertHumaError(t, err, 422)
		})
	}
}

func TestGetFollowAlertsHandler_ServiceError(t *testing.T) {
	mock := &testhelpers.MockFollowService{
		GetFollowAlertSettingsFn: func(_ uint, _ string, _ uint) (*contracts.FollowAlertSettings, error) {
			return nil, fmt.Errorf("db error")
		},
	}
	h := NewFollowHandler(mock)
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1})

	_, err := h.GetFollowAlertsHandler(ctx, &FollowAlertsRequest{EntityType: "artists", EntityID: "42"})
	testhelpers.AssertHumaError(t, err, 500)
}

// The service contract says a nil error carries a settings value; a nil pair
// is a contract violation and must not panic the handler.
func TestGetFollowAlertsHandler_NilSettingsIs500(t *testing.T) {
	h := NewFollowHandler(&testhelpers.MockFollowService{})
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1})

	_, err := h.GetFollowAlertsHandler(ctx, &FollowAlertsRequest{EntityType: "artists", EntityID: "42"})
	testhelpers.AssertHumaError(t, err, 500)
}

// --- UpdateFollowAlertsHandler ---

func TestUpdateFollowAlertsHandler_NoAuth(t *testing.T) {
	h := NewFollowHandler(&testhelpers.MockFollowService{})

	_, err := h.UpdateFollowAlertsHandler(context.Background(),
		&UpdateFollowAlertsRequest{EntityType: "artists", EntityID: "42"})
	testhelpers.AssertHumaError(t, err, 401)
}

// Omitted body fields must arrive as nil updates so the service leaves those
// axes inheriting the account default.
func TestUpdateFollowAlertsHandler_PassesOnlySetFields(t *testing.T) {
	scope := contracts.FollowAlertScopeEverywhere
	var captured contracts.FollowAlertUpdate
	mock := &testhelpers.MockFollowService{
		SetFollowAlertSettingsFn: func(userID uint, entityType string, entityID uint, update contracts.FollowAlertUpdate) (*contracts.FollowAlertSettings, error) {
			if userID != 7 || entityType != "artist" || entityID != 42 {
				t.Errorf("unexpected args: userID=%d entityType=%s entityID=%d", userID, entityType, entityID)
			}
			captured = update
			return artistShowAlerts(), nil
		},
	}
	h := NewFollowHandler(mock)
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 7})

	req := &UpdateFollowAlertsRequest{EntityType: "artists", EntityID: "42"}
	req.Body.Shows = &FollowAlertPreferenceBody{Scope: &scope}

	if _, err := h.UpdateFollowAlertsHandler(ctx, req); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if captured.Releases != nil {
		t.Error("omitted releases must arrive as a nil update")
	}
	if captured.Shows == nil || captured.Shows.Scope == nil || *captured.Shows.Scope != scope {
		t.Errorf("shows update = %+v, want scope %q", captured.Shows, scope)
	}
	if captured.Shows.Enabled != nil || captured.Shows.InApp != nil || captured.Shows.Email != nil {
		t.Errorf("omitted channel fields must stay nil, got %+v", captured.Shows)
	}
}

// An absent alert type arrives as a nil update. What a present-but-empty one
// means is the service's call, so the handler passes it through faithfully
// rather than deciding on the transport's behalf.
func TestUpdateFollowAlertsHandler_MapsBodyFaithfully(t *testing.T) {
	for name, tc := range map[string]struct {
		body    *FollowAlertPreferenceBody
		wantNil bool
	}{
		"absent shows":        {body: nil, wantNil: true},
		"present but all-nil": {body: &FollowAlertPreferenceBody{}, wantNil: false},
	} {
		t.Run(name, func(t *testing.T) {
			var captured contracts.FollowAlertUpdate
			mock := &testhelpers.MockFollowService{
				SetFollowAlertSettingsFn: func(_ uint, _ string, _ uint, update contracts.FollowAlertUpdate) (*contracts.FollowAlertSettings, error) {
					captured = update
					return artistShowAlerts(), nil
				},
			}
			h := NewFollowHandler(mock)
			ctx := testhelpers.CtxWithUser(&authm.User{ID: 1})

			req := &UpdateFollowAlertsRequest{EntityType: "artists", EntityID: "42"}
			req.Body.Shows = tc.body

			if _, err := h.UpdateFollowAlertsHandler(ctx, req); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if captured.Releases != nil {
				t.Errorf("omitted releases must arrive as a nil update, got %+v", captured.Releases)
			}
			if tc.wantNil && captured.Shows != nil {
				t.Errorf("absent shows must arrive as a nil update, got %+v", captured.Shows)
			}
			if !tc.wantNil {
				if captured.Shows == nil {
					t.Fatal("present shows must arrive as a non-nil update")
				}
				if captured.Shows.Enabled != nil || captured.Shows.InApp != nil ||
					captured.Shows.Email != nil || captured.Shows.Scope != nil {
					t.Errorf("no axis was set, so every axis must be nil, got %+v", captured.Shows)
				}
			}
		})
	}
}

func TestUpdateFollowAlertsHandler_NotFollowingIs404(t *testing.T) {
	mock := &testhelpers.MockFollowService{
		SetFollowAlertSettingsFn: func(_ uint, _ string, _ uint, _ contracts.FollowAlertUpdate) (*contracts.FollowAlertSettings, error) {
			return nil, apperrors.ErrFollowNotFound("artist", 42)
		},
	}
	h := NewFollowHandler(mock)
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1})

	_, err := h.UpdateFollowAlertsHandler(ctx,
		&UpdateFollowAlertsRequest{EntityType: "artists", EntityID: "42"})
	testhelpers.AssertHumaError(t, err, 404)
}

func TestUpdateFollowAlertsHandler_RejectedUpdateIs422(t *testing.T) {
	mock := &testhelpers.MockFollowService{
		SetFollowAlertSettingsFn: func(_ uint, _ string, _ uint, _ contracts.FollowAlertUpdate) (*contracts.FollowAlertSettings, error) {
			return nil, apperrors.ErrFollowInvalidEntityType("venue show alerts have no scope axis")
		},
	}
	h := NewFollowHandler(mock)
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1})

	scope := contracts.FollowAlertScopeNearMe
	req := &UpdateFollowAlertsRequest{EntityType: "venues", EntityID: "9"}
	req.Body.Shows = &FollowAlertPreferenceBody{Scope: &scope}

	_, err := h.UpdateFollowAlertsHandler(ctx, req)
	testhelpers.AssertHumaError(t, err, 422)
}

func TestUpdateFollowAlertsHandler_ServiceError(t *testing.T) {
	mock := &testhelpers.MockFollowService{
		SetFollowAlertSettingsFn: func(_ uint, _ string, _ uint, _ contracts.FollowAlertUpdate) (*contracts.FollowAlertSettings, error) {
			return nil, fmt.Errorf("db error")
		},
	}
	h := NewFollowHandler(mock)
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1})

	_, err := h.UpdateFollowAlertsHandler(ctx,
		&UpdateFollowAlertsRequest{EntityType: "artists", EntityID: "42"})
	testhelpers.AssertHumaError(t, err, 500)
}
