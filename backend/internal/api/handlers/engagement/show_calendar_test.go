package engagement

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"psychic-homily-backend/internal/api/handlers/shared/testhelpers"
	apperrors "psychic-homily-backend/internal/errors"
	"psychic-homily-backend/internal/services/contracts"
)

func showCalendarRequest(method, showID string) *http.Request {
	req := httptest.NewRequest(method, "/shows/x/calendar.ics", nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("show_id", showID)
	return req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
}

func stubShowEvent() *contracts.ShowCalendarEvent {
	return &contracts.ShowCalendarEvent{
		ShowSlug: "desert-doom-night",
		ICS:      []byte("BEGIN:VCALENDAR\r\nEND:VCALENDAR\r\n"),
		ETag:     `W/"cafef00d"`,
	}
}

func TestGetShowCalendarHandler_Success(t *testing.T) {
	mock := &testhelpers.MockShowCalendarService{
		GenerateShowEventFn: func(idOrSlug, frontendURL string) (*contracts.ShowCalendarEvent, error) {
			if idOrSlug != "desert-doom-night" {
				t.Errorf("idOrSlug = %q, want desert-doom-night", idOrSlug)
			}
			if frontendURL != "http://localhost:3000" {
				t.Errorf("frontendURL = %q, want the configured frontend URL", frontendURL)
			}
			return stubShowEvent(), nil
		},
	}
	h := NewShowCalendarHandler(mock, testCalendarConfig())
	w := httptest.NewRecorder()

	h.GetShowCalendarHandler(w, showCalendarRequest(http.MethodGet, "desert-doom-night"))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	if ct := w.Header().Get("Content-Type"); ct != "text/calendar; charset=utf-8" {
		t.Errorf("Content-Type = %q, want text/calendar; charset=utf-8", ct)
	}
	if cc := w.Header().Get("Cache-Control"); cc != publicCalendarCacheControl {
		t.Errorf("Cache-Control = %q, want %q", cc, publicCalendarCacheControl)
	}
	// attachment, not inline: this endpoint IS the download path.
	if cd := w.Header().Get("Content-Disposition"); cd != `attachment; filename="desert-doom-night.ics"` {
		t.Errorf("Content-Disposition = %q", cd)
	}
	if w.Body.String() != "BEGIN:VCALENDAR\r\nEND:VCALENDAR\r\n" {
		t.Errorf("body = %q", w.Body.String())
	}
}

func TestGetShowCalendarHandler_ETagRevalidation(t *testing.T) {
	mock := &testhelpers.MockShowCalendarService{
		GenerateShowEventFn: func(_, _ string) (*contracts.ShowCalendarEvent, error) {
			return stubShowEvent(), nil
		},
	}
	h := NewShowCalendarHandler(mock, testCalendarConfig())

	cases := []struct {
		ifNoneMatch string
		wantStatus  int
	}{
		{`W/"cafef00d"`, http.StatusNotModified},
		{"*", http.StatusNotModified},
		{`W/"stale"`, http.StatusOK},
		{"", http.StatusOK},
	}
	for _, tc := range cases {
		w := httptest.NewRecorder()
		req := showCalendarRequest(http.MethodGet, "42")
		if tc.ifNoneMatch != "" {
			req.Header.Set("If-None-Match", tc.ifNoneMatch)
		}
		h.GetShowCalendarHandler(w, req)
		if w.Code != tc.wantStatus {
			t.Errorf("If-None-Match %q: status = %d, want %d", tc.ifNoneMatch, w.Code, tc.wantStatus)
		}
		if tc.wantStatus == http.StatusNotModified && w.Body.Len() != 0 {
			t.Errorf("If-None-Match %q: 304 must carry no body", tc.ifNoneMatch)
		}
	}
}

func TestGetShowCalendarHandler_HeadServesHeaders(t *testing.T) {
	mock := &testhelpers.MockShowCalendarService{
		GenerateShowEventFn: func(_, _ string) (*contracts.ShowCalendarEvent, error) {
			return stubShowEvent(), nil
		},
	}
	h := NewShowCalendarHandler(mock, testCalendarConfig())
	w := httptest.NewRecorder()

	h.GetShowCalendarHandler(w, showCalendarRequest(http.MethodHead, "42"))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	if et := w.Header().Get("ETag"); et != `W/"cafef00d"` {
		t.Errorf("ETag = %q", et)
	}
}

func TestGetShowCalendarHandler_InvalidIdentifier(t *testing.T) {
	h := NewShowCalendarHandler(&testhelpers.MockShowCalendarService{}, testCalendarConfig())

	for _, identifier := range []string{"", "  ", strings.Repeat("a", maxCalendarIdentifierLength+1)} {
		w := httptest.NewRecorder()
		h.GetShowCalendarHandler(w, showCalendarRequest(http.MethodGet, identifier))
		if w.Code != http.StatusBadRequest {
			t.Errorf("identifier %q: status = %d, want %d", identifier, w.Code, http.StatusBadRequest)
		}
	}
}

func TestGetShowCalendarHandler_NotFound(t *testing.T) {
	mock := &testhelpers.MockShowCalendarService{
		GenerateShowEventFn: func(_, _ string) (*contracts.ShowCalendarEvent, error) {
			return nil, apperrors.ErrShowNotFound(0)
		},
	}
	h := NewShowCalendarHandler(mock, testCalendarConfig())
	w := httptest.NewRecorder()

	h.GetShowCalendarHandler(w, showCalendarRequest(http.MethodGet, "no-such-show"))

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestGetShowCalendarHandler_InternalErrorDoesNotEchoIdentifier(t *testing.T) {
	mock := &testhelpers.MockShowCalendarService{
		GenerateShowEventFn: func(_, _ string) (*contracts.ShowCalendarEvent, error) {
			return nil, fmt.Errorf("db exploded")
		},
	}
	h := NewShowCalendarHandler(mock, testCalendarConfig())
	w := httptest.NewRecorder()

	hostile := "evil-slug-that-must-not-echo"
	h.GetShowCalendarHandler(w, showCalendarRequest(http.MethodGet, hostile))

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusInternalServerError)
	}
	if strings.Contains(w.Body.String(), hostile) {
		t.Errorf("500 body must not echo the caller-controlled identifier")
	}
}

func TestShowEventFilename_SuffixAndFallback(t *testing.T) {
	// filenameSafeSlug's character filtering is pinned by the venue feed's
	// filename test; this covers only what differs here.
	if got := showEventFilename("desert-doom-night"); got != "desert-doom-night.ics" {
		t.Errorf("showEventFilename = %q", got)
	}
	if got := showEventFilename(""); got != "show.ics" {
		t.Errorf("empty slug: showEventFilename = %q, want show.ics", got)
	}
}
