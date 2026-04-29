package handlers

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"psychic-homily-backend/internal/services/engagement"
)

// PSY-350: Tests for the chi-level unsubscribe handler that backs the
// `/unsubscribe/collection-digest` route. Both GET (manual click) and POST
// (RFC 8058 one-click) hit the same handler; we verify each branch returns
// the right Content-Type, status, and side effect.

func TestUnsubscribeCollectionDigestPage_MissingParams(t *testing.T) {
	h := NewUserPreferencesHandler(&mockUserService{}, "secret")

	req := httptest.NewRequest(http.MethodGet, "/unsubscribe/collection-digest", nil)
	w := httptest.NewRecorder()
	h.UnsubscribeCollectionDigestPageHandler(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Fatalf("GET error should render HTML, got Content-Type %q", ct)
	}
}

func TestUnsubscribeCollectionDigestPage_InvalidSignature(t *testing.T) {
	h := NewUserPreferencesHandler(&mockUserService{}, "secret")

	req := httptest.NewRequest(http.MethodGet, "/unsubscribe/collection-digest?uid=42&sig=bogus", nil)
	w := httptest.NewRecorder()
	h.UnsubscribeCollectionDigestPageHandler(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", w.Code)
	}
}

func TestUnsubscribeCollectionDigestPage_GET_Success(t *testing.T) {
	secret := "test-secret"
	uid := uint(99)
	sig := engagement.ComputeCollectionDigestUnsubscribeSignature(uid, secret)

	var receivedUID uint
	var receivedEnabled bool
	mock := &mockUserService{
		setNotifyOnCollectionDigestFn: func(userID uint, enabled bool) error {
			receivedUID = userID
			receivedEnabled = enabled
			return nil
		},
	}
	h := NewUserPreferencesHandler(mock, secret)

	req := httptest.NewRequest(http.MethodGet,
		"/unsubscribe/collection-digest?uid=99&sig="+sig, nil)
	w := httptest.NewRecorder()
	h.UnsubscribeCollectionDigestPageHandler(w, req)

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
	if !strings.Contains(body, "/settings") {
		t.Error("confirmation page must include a link back to notification settings")
	}
	if receivedUID != uid {
		t.Errorf("expected SetNotifyOnCollectionDigest called with uid=%d, got %d", uid, receivedUID)
	}
	if receivedEnabled {
		t.Error("expected SetNotifyOnCollectionDigest called with enabled=false (unsubscribe), got true")
	}
}

func TestUnsubscribeCollectionDigestPage_POST_OneClick_Success(t *testing.T) {
	secret := "test-secret"
	uid := uint(7)
	sig := engagement.ComputeCollectionDigestUnsubscribeSignature(uid, secret)

	var called bool
	mock := &mockUserService{
		setNotifyOnCollectionDigestFn: func(userID uint, enabled bool) error {
			called = true
			if enabled {
				t.Errorf("POST one-click must always disable; got enabled=true")
			}
			return nil
		},
	}
	h := NewUserPreferencesHandler(mock, secret)

	body := strings.NewReader("List-Unsubscribe=One-Click")
	req := httptest.NewRequest(http.MethodPost,
		"/unsubscribe/collection-digest?uid=7&sig="+sig, body)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	h.UnsubscribeCollectionDigestPageHandler(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (body=%s)", w.Code, w.Body.String())
	}
	if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("POST response should be JSON, got Content-Type %q", ct)
	}
	if !strings.Contains(w.Body.String(), `"unsubscribed":true`) {
		t.Errorf("POST response should contain unsubscribed:true, body was %q", w.Body.String())
	}
	if !called {
		t.Error("expected SetNotifyOnCollectionDigest to be called")
	}
}

func TestUnsubscribeCollectionDigestPage_POST_InvalidSig_JSON(t *testing.T) {
	h := NewUserPreferencesHandler(&mockUserService{}, "secret")

	req := httptest.NewRequest(http.MethodPost,
		"/unsubscribe/collection-digest?uid=42&sig=bogus", strings.NewReader("List-Unsubscribe=One-Click"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	h.UnsubscribeCollectionDigestPageHandler(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", w.Code)
	}
	// POST errors must be JSON so mailbox provider machines can parse them.
	if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("POST error should be JSON, got %q", ct)
	}
}

func TestUnsubscribeCollectionDigestPage_ServiceError_GET(t *testing.T) {
	secret := "test-secret"
	uid := uint(50)
	sig := engagement.ComputeCollectionDigestUnsubscribeSignature(uid, secret)

	mock := &mockUserService{
		setNotifyOnCollectionDigestFn: func(uint, bool) error {
			return errors.New("db unavailable")
		},
	}
	h := NewUserPreferencesHandler(mock, secret)

	req := httptest.NewRequest(http.MethodGet,
		"/unsubscribe/collection-digest?uid=50&sig="+sig, nil)
	w := httptest.NewRecorder()
	h.UnsubscribeCollectionDigestPageHandler(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Errorf("GET error should be HTML, got %q", ct)
	}
}

func TestUnsubscribeCollectionDigestPage_ServiceError_POST(t *testing.T) {
	secret := "test-secret"
	uid := uint(50)
	sig := engagement.ComputeCollectionDigestUnsubscribeSignature(uid, secret)

	mock := &mockUserService{
		setNotifyOnCollectionDigestFn: func(uint, bool) error {
			return errors.New("db unavailable")
		},
	}
	h := NewUserPreferencesHandler(mock, secret)

	req := httptest.NewRequest(http.MethodPost,
		"/unsubscribe/collection-digest?uid=50&sig="+sig, strings.NewReader("List-Unsubscribe=One-Click"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	h.UnsubscribeCollectionDigestPageHandler(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("POST error should be JSON, got %q", ct)
	}
}
