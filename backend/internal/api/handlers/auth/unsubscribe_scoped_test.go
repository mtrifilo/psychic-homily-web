package auth

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"psychic-homily-backend/internal/api/handlers/shared/testhelpers"
	"psychic-homily-backend/internal/services/engagement"
)

// PSY-756: tests for the scoped unsubscribe handlers backing
// /unsubscribe/tier-notifications and /unsubscribe/edit-notifications. Same
// dual GET/POST shape as the collection-digest handler.

func TestUnsubscribeTierNotifications_GET_Success(t *testing.T) {
	secret := "test-secret"
	uid := uint(99)
	sig := engagement.ComputeScopedUnsubscribeSignature(uid, engagement.UnsubscribeScopeTierNotifications, secret)

	var gotUID uint
	var gotEnabled = true
	mock := &testhelpers.MockUserService{
		SetNotifyOnTierNotificationsFn: func(userID uint, enabled bool) error {
			gotUID = userID
			gotEnabled = enabled
			return nil
		},
	}
	h := NewUserPreferencesHandler(mock, secret)

	req := httptest.NewRequest(http.MethodGet, "/unsubscribe/tier-notifications?uid=99&sig="+sig, nil)
	w := httptest.NewRecorder()
	h.UnsubscribeTierNotificationsPageHandler(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Fatalf("expected text/html on GET, got %q", ct)
	}
	body := w.Body.String()
	if !strings.Contains(body, "unsubscribed") {
		t.Errorf("confirmation page must mention being unsubscribed, body was: %s", body)
	}
	if !strings.Contains(body, "tier-change emails") {
		t.Errorf("confirmation page must name the category, body was: %s", body)
	}
	if gotUID != uid || gotEnabled {
		t.Errorf("expected SetNotifyOnTierNotifications(uid=%d, false), got (uid=%d, enabled=%v)", uid, gotUID, gotEnabled)
	}
}

func TestUnsubscribeEditNotifications_POST_OneClick_Success(t *testing.T) {
	secret := "test-secret"
	uid := uint(7)
	sig := engagement.ComputeScopedUnsubscribeSignature(uid, engagement.UnsubscribeScopeEditNotifications, secret)

	var called bool
	mock := &testhelpers.MockUserService{
		SetNotifyOnEditNotificationsFn: func(_ uint, enabled bool) error {
			called = true
			if enabled {
				t.Errorf("POST one-click must always disable; got enabled=true")
			}
			return nil
		},
	}
	h := NewUserPreferencesHandler(mock, secret)

	req := httptest.NewRequest(http.MethodPost,
		"/unsubscribe/edit-notifications?uid=7&sig="+sig, strings.NewReader("List-Unsubscribe=One-Click"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	h.UnsubscribeEditNotificationsPageHandler(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (body=%s)", w.Code, w.Body.String())
	}
	if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("POST response should be JSON, got %q", ct)
	}
	if !strings.Contains(w.Body.String(), `"unsubscribed":true`) {
		t.Errorf("POST response should contain unsubscribed:true, body was %q", w.Body.String())
	}
	if !called {
		t.Error("expected SetNotifyOnEditNotifications to be called")
	}
}

func TestUnsubscribeScoped_InvalidSignature(t *testing.T) {
	h := NewUserPreferencesHandler(&testhelpers.MockUserService{}, "secret")

	// Tier handler, GET, bad sig -> 403 HTML.
	req := httptest.NewRequest(http.MethodGet, "/unsubscribe/tier-notifications?uid=42&sig=bogus", nil)
	w := httptest.NewRecorder()
	h.UnsubscribeTierNotificationsPageHandler(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", w.Code)
	}

	// Edit handler, POST, bad sig -> 403 JSON.
	req = httptest.NewRequest(http.MethodPost, "/unsubscribe/edit-notifications?uid=42&sig=bogus",
		strings.NewReader("List-Unsubscribe=One-Click"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w = httptest.NewRecorder()
	h.UnsubscribeEditNotificationsPageHandler(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("POST error should be JSON, got %q", ct)
	}
}

func TestUnsubscribeScoped_CrossScopeSignatureRejected(t *testing.T) {
	// A signature minted for the tier scope must not unsubscribe edit emails.
	secret := "test-secret"
	uid := uint(11)
	tierSig := engagement.ComputeScopedUnsubscribeSignature(uid, engagement.UnsubscribeScopeTierNotifications, secret)

	mock := &testhelpers.MockUserService{
		SetNotifyOnEditNotificationsFn: func(uint, bool) error {
			t.Error("edit preference must not be flipped by a tier-scoped signature")
			return nil
		},
	}
	h := NewUserPreferencesHandler(mock, secret)

	req := httptest.NewRequest(http.MethodGet, "/unsubscribe/edit-notifications?uid=11&sig="+tierSig, nil)
	w := httptest.NewRecorder()
	h.UnsubscribeEditNotificationsPageHandler(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for cross-scope signature, got %d", w.Code)
	}
}

func TestUnsubscribeScoped_MissingParams(t *testing.T) {
	h := NewUserPreferencesHandler(&testhelpers.MockUserService{}, "secret")

	req := httptest.NewRequest(http.MethodGet, "/unsubscribe/tier-notifications", nil)
	w := httptest.NewRecorder()
	h.UnsubscribeTierNotificationsPageHandler(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Fatalf("GET error should render HTML, got Content-Type %q", ct)
	}
}

func TestUnsubscribeScoped_ServiceError(t *testing.T) {
	secret := "test-secret"
	uid := uint(50)
	sig := engagement.ComputeScopedUnsubscribeSignature(uid, engagement.UnsubscribeScopeTierNotifications, secret)

	mock := &testhelpers.MockUserService{
		SetNotifyOnTierNotificationsFn: func(uint, bool) error {
			return errors.New("db unavailable")
		},
	}
	h := NewUserPreferencesHandler(mock, secret)

	req := httptest.NewRequest(http.MethodGet, "/unsubscribe/tier-notifications?uid=50&sig="+sig, nil)
	w := httptest.NewRecorder()
	h.UnsubscribeTierNotificationsPageHandler(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}
}
