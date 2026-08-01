package engagement

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	apperrors "psychic-homily-backend/internal/errors"
	"psychic-homily-backend/internal/api/handlers/shared/testhelpers"
	"psychic-homily-backend/internal/services/contracts"
)

func venueFeedRequest(method, venueID string) *http.Request {
	req := httptest.NewRequest(method, "/venues/x/calendar.ics", nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("venue_id", venueID)
	return req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
}

func stubVenueFeed() *contracts.VenueCalendarFeed {
	return &contracts.VenueCalendarFeed{
		VenueName: "The Rebel Lounge",
		VenueSlug: "the-rebel-lounge",
		ICS:       []byte("BEGIN:VCALENDAR\r\nEND:VCALENDAR\r\n"),
		ETag:      `W/"deadbeef"`,
	}
}

func TestGetVenueCalendarFeedHandler_Success(t *testing.T) {
	mock := &testhelpers.MockVenueCalendarService{
		GenerateVenueFeedFn: func(idOrSlug, frontendURL string) (*contracts.VenueCalendarFeed, error) {
			if idOrSlug != "the-rebel-lounge" {
				t.Errorf("idOrSlug = %q, want the-rebel-lounge", idOrSlug)
			}
			if frontendURL != "http://localhost:3000" {
				t.Errorf("frontendURL = %q, want the configured frontend URL", frontendURL)
			}
			return stubVenueFeed(), nil
		},
	}
	h := NewVenueCalendarHandler(mock, testCalendarConfig())
	w := httptest.NewRecorder()

	h.GetVenueCalendarFeedHandler(w, venueFeedRequest(http.MethodGet, "the-rebel-lounge"))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	if ct := w.Header().Get("Content-Type"); ct != "text/calendar; charset=utf-8" {
		t.Errorf("Content-Type = %q, want text/calendar; charset=utf-8", ct)
	}
	// A public feed must be cacheable by SHARED caches — every caller gets the
	// same bytes, and absorbing poll traffic upstream is the whole point.
	if cc := w.Header().Get("Cache-Control"); cc != publicCalendarCacheControl {
		t.Errorf("Cache-Control = %q, want %q", cc, publicCalendarCacheControl)
	}
	if strings.Contains(w.Header().Get("Cache-Control"), "private") {
		t.Error("public venue feed must not be marked private")
	}
	if et := w.Header().Get("ETag"); et != `W/"deadbeef"` {
		t.Errorf("ETag = %q, want the feed's ETag", et)
	}
	if cd := w.Header().Get("Content-Disposition"); cd != `inline; filename="the-rebel-lounge-shows.ics"` {
		t.Errorf("Content-Disposition = %q", cd)
	}
	if w.Body.String() != "BEGIN:VCALENDAR\r\nEND:VCALENDAR\r\n" {
		t.Errorf("body = %q", w.Body.String())
	}
}

// Calendar clients poll on their own schedule and can be aggressive. A matching
// If-None-Match must cost them a 304, not the whole payload.
func TestGetVenueCalendarFeedHandler_NotModified(t *testing.T) {
	mock := &testhelpers.MockVenueCalendarService{
		GenerateVenueFeedFn: func(string, string) (*contracts.VenueCalendarFeed, error) {
			return stubVenueFeed(), nil
		},
	}
	h := NewVenueCalendarHandler(mock, testCalendarConfig())

	for _, tc := range []struct {
		name       string
		header     string
		wantStatus int
	}{
		{"exact match", `W/"deadbeef"`, http.StatusNotModified},
		{"wildcard", "*", http.StatusNotModified},
		{"in a list", `W/"other", W/"deadbeef"`, http.StatusNotModified},
		{"stale validator", `W/"stale"`, http.StatusOK},
		{"absent", "", http.StatusOK},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := venueFeedRequest(http.MethodGet, "the-rebel-lounge")
			if tc.header != "" {
				req.Header.Set("If-None-Match", tc.header)
			}
			w := httptest.NewRecorder()
			h.GetVenueCalendarFeedHandler(w, req)

			if w.Code != tc.wantStatus {
				t.Errorf("status = %d, want %d", w.Code, tc.wantStatus)
			}
			if tc.wantStatus == http.StatusNotModified {
				if w.Body.Len() != 0 {
					t.Errorf("304 must have an empty body, got %q", w.Body.String())
				}
				if w.Header().Get("ETag") == "" {
					t.Error("304 must still carry the ETag")
				}
			}
		})
	}
}

// An unknown slug must 404, not 500 — a public endpoint that 500s on every
// mistyped URL is indistinguishable from one that is actually broken.
func TestGetVenueCalendarFeedHandler_UnknownVenue(t *testing.T) {
	mock := &testhelpers.MockVenueCalendarService{
		GenerateVenueFeedFn: func(string, string) (*contracts.VenueCalendarFeed, error) {
			return nil, apperrors.ErrVenueNotFound(0)
		},
	}
	h := NewVenueCalendarHandler(mock, testCalendarConfig())
	w := httptest.NewRecorder()

	h.GetVenueCalendarFeedHandler(w, venueFeedRequest(http.MethodGet, "no-such-room"))

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestGetVenueCalendarFeedHandler_GenerationError(t *testing.T) {
	mock := &testhelpers.MockVenueCalendarService{
		GenerateVenueFeedFn: func(string, string) (*contracts.VenueCalendarFeed, error) {
			return nil, fmt.Errorf("db exploded")
		},
	}
	h := NewVenueCalendarHandler(mock, testCalendarConfig())
	w := httptest.NewRecorder()

	h.GetVenueCalendarFeedHandler(w, venueFeedRequest(http.MethodGet, "the-rebel-lounge"))

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusInternalServerError)
	}
	// Internal failure detail must not reach an anonymous caller.
	if strings.Contains(w.Body.String(), "db exploded") {
		t.Errorf("error body leaked internals: %q", w.Body.String())
	}
}

// The identifier is caller-controlled and reaches a database lookup, so the
// obviously-bad shapes are rejected before the service is ever called.
func TestGetVenueCalendarFeedHandler_RejectsBadIdentifiers(t *testing.T) {
	called := false
	mock := &testhelpers.MockVenueCalendarService{
		GenerateVenueFeedFn: func(string, string) (*contracts.VenueCalendarFeed, error) {
			called = true
			return stubVenueFeed(), nil
		},
	}
	h := NewVenueCalendarHandler(mock, testCalendarConfig())

	for _, id := range []string{"", "   ", strings.Repeat("a", maxCalendarIdentifierLength+1)} {
		w := httptest.NewRecorder()
		h.GetVenueCalendarFeedHandler(w, venueFeedRequest(http.MethodGet, id))
		if w.Code != http.StatusBadRequest {
			t.Errorf("identifier %q: status = %d, want %d", id, w.Code, http.StatusBadRequest)
		}
	}
	if called {
		t.Error("a rejected identifier must never reach the service")
	}
}

// The slug is community-created data flowing into a response HEADER, where a
// stray quote or CRLF stops being cosmetic.
func TestVenueFeedFilename_IsHeaderSafe(t *testing.T) {
	for _, tc := range []struct{ slug, want string }{
		{"the-rebel-lounge", "the-rebel-lounge-shows.ics"},
		{"The Rebel Lounge", "therebellounge-shows.ics"},
		{`evil"; attack="1`, "evilattack1-shows.ics"},
		{"bad\r\nX-Injected: yes", "badx-injectedyes-shows.ics"},
		{"", "venue-shows.ics"},
		{"???", "venue-shows.ics"},
		{"--edges--", "edges-shows.ics"},
	} {
		if got := venueFeedFilename(tc.slug); got != tc.want {
			t.Errorf("venueFeedFilename(%q) = %q, want %q", tc.slug, got, tc.want)
		}
	}

	// Whatever the input, the result can never break out of the quoted header.
	for _, slug := range []string{`a"b`, "a\r\nb", "a;b", "a\\b"} {
		got := venueFeedFilename(slug)
		if strings.ContainsAny(got, "\"\r\n;\\ ") {
			t.Errorf("venueFeedFilename(%q) = %q contains a header-breaking character", slug, got)
		}
	}
}
