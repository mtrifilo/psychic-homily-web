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

func sceneFeedRequest(method, slug string) *http.Request {
	req := httptest.NewRequest(method, "/scenes/x/calendar.ics", nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("slug", slug)
	return req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
}

func stubSceneFeed() *contracts.SceneCalendarFeed {
	return &contracts.SceneCalendarFeed{
		SceneName: "Phoenix, AZ",
		SceneSlug: "phoenix-az",
		ICS:       []byte("BEGIN:VCALENDAR\r\nEND:VCALENDAR\r\n"),
		ETag:      `W/"deadbeef"`,
	}
}

func TestGetSceneCalendarFeedHandler_Success(t *testing.T) {
	mock := &testhelpers.MockSceneCalendarService{
		GenerateSceneFeedFn: func(slug, frontendURL string) (*contracts.SceneCalendarFeed, error) {
			if slug != "phoenix-az" {
				t.Errorf("slug = %q, want phoenix-az", slug)
			}
			if frontendURL != "http://localhost:3000" {
				t.Errorf("frontendURL = %q, want the configured frontend URL", frontendURL)
			}
			return stubSceneFeed(), nil
		},
	}
	h := NewSceneCalendarHandler(mock, testCalendarConfig())
	w := httptest.NewRecorder()

	h.GetSceneCalendarFeedHandler(w, sceneFeedRequest(http.MethodGet, "phoenix-az"))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	if ct := w.Header().Get("Content-Type"); ct != "text/calendar; charset=utf-8" {
		t.Errorf("Content-Type = %q, want text/calendar; charset=utf-8", ct)
	}
	// Same posture as the venue feed: anonymous, so every caller gets the same
	// bytes and shared caches SHOULD be allowed to absorb the poll traffic.
	if cc := w.Header().Get("Cache-Control"); cc != publicCalendarCacheControl {
		t.Errorf("Cache-Control = %q, want %q", cc, publicCalendarCacheControl)
	}
	if strings.Contains(w.Header().Get("Cache-Control"), "private") {
		t.Error("public scene feed must not be marked private")
	}
	if et := w.Header().Get("ETag"); et != `W/"deadbeef"` {
		t.Errorf("ETag = %q, want the feed's ETag", et)
	}
	if cd := w.Header().Get("Content-Disposition"); cd != `inline; filename="phoenix-az-shows.ics"` {
		t.Errorf("Content-Disposition = %q", cd)
	}
	if w.Body.String() != "BEGIN:VCALENDAR\r\nEND:VCALENDAR\r\n" {
		t.Errorf("body = %q", w.Body.String())
	}
}

// The filename comes from the feed's CANONICAL slug, not the requested alias, so
// a metro member slug downloads the file its scene is actually named after.
func TestGetSceneCalendarFeedHandler_FilenameUsesCanonicalSlug(t *testing.T) {
	mock := &testhelpers.MockSceneCalendarService{
		GenerateSceneFeedFn: func(string, string) (*contracts.SceneCalendarFeed, error) {
			return stubSceneFeed(), nil
		},
	}
	h := NewSceneCalendarHandler(mock, testCalendarConfig())
	w := httptest.NewRecorder()

	h.GetSceneCalendarFeedHandler(w, sceneFeedRequest(http.MethodGet, "mesa-az"))

	if cd := w.Header().Get("Content-Disposition"); cd != `inline; filename="phoenix-az-shows.ics"` {
		t.Errorf("Content-Disposition = %q, want the canonical scene's filename", cd)
	}
}

// Calendar clients poll on their own schedule and can be aggressive. A matching
// If-None-Match must cost them a 304, not the whole payload.
func TestGetSceneCalendarFeedHandler_NotModified(t *testing.T) {
	mock := &testhelpers.MockSceneCalendarService{
		GenerateSceneFeedFn: func(string, string) (*contracts.SceneCalendarFeed, error) {
			return stubSceneFeed(), nil
		},
	}
	h := NewSceneCalendarHandler(mock, testCalendarConfig())

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
			req := sceneFeedRequest(http.MethodGet, "phoenix-az")
			if tc.header != "" {
				req.Header.Set("If-None-Match", tc.header)
			}
			w := httptest.NewRecorder()
			h.GetSceneCalendarFeedHandler(w, req)

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
func TestGetSceneCalendarFeedHandler_UnknownScene(t *testing.T) {
	mock := &testhelpers.MockSceneCalendarService{
		GenerateSceneFeedFn: func(string, string) (*contracts.SceneCalendarFeed, error) {
			return nil, apperrors.ErrSceneNotFound("scene not found for slug: nowhere-zz")
		},
	}
	h := NewSceneCalendarHandler(mock, testCalendarConfig())
	w := httptest.NewRecorder()

	h.GetSceneCalendarFeedHandler(w, sceneFeedRequest(http.MethodGet, "nowhere-zz"))

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestGetSceneCalendarFeedHandler_GenerationError(t *testing.T) {
	mock := &testhelpers.MockSceneCalendarService{
		GenerateSceneFeedFn: func(string, string) (*contracts.SceneCalendarFeed, error) {
			return nil, fmt.Errorf("db exploded")
		},
	}
	h := NewSceneCalendarHandler(mock, testCalendarConfig())
	w := httptest.NewRecorder()

	h.GetSceneCalendarFeedHandler(w, sceneFeedRequest(http.MethodGet, "phoenix-az"))

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusInternalServerError)
	}
	// Internal failure detail must not reach an anonymous caller.
	if strings.Contains(w.Body.String(), "db exploded") {
		t.Errorf("error body leaked internals: %q", w.Body.String())
	}
}

// The slug is caller-controlled and reaches a database lookup, so the obviously
// bad shapes are rejected before the service is ever called.
func TestGetSceneCalendarFeedHandler_RejectsBadIdentifiers(t *testing.T) {
	called := false
	mock := &testhelpers.MockSceneCalendarService{
		GenerateSceneFeedFn: func(string, string) (*contracts.SceneCalendarFeed, error) {
			called = true
			return stubSceneFeed(), nil
		},
	}
	h := NewSceneCalendarHandler(mock, testCalendarConfig())

	for _, id := range []string{"", "   ", strings.Repeat("a", maxCalendarIdentifierLength+1)} {
		w := httptest.NewRecorder()
		h.GetSceneCalendarFeedHandler(w, sceneFeedRequest(http.MethodGet, id))
		if w.Code != http.StatusBadRequest {
			t.Errorf("identifier %q: status = %d, want %d", id, w.Code, http.StatusBadRequest)
		}
	}
	if called {
		t.Error("a rejected identifier must never reach the service")
	}
}

// The slug is derived from community-created venue rows and flows into a
// response HEADER, where a stray quote or CRLF stops being cosmetic.
func TestSceneFeedFilename_IsHeaderSafe(t *testing.T) {
	for _, tc := range []struct{ slug, want string }{
		{"phoenix-az", "phoenix-az-shows.ics"},
		{"Phoenix AZ", "phoenixaz-shows.ics"},
		{`evil"; attack="1`, "evilattack1-shows.ics"},
		{"bad\r\nX-Injected: yes", "badx-injectedyes-shows.ics"},
		{"", "scene-shows.ics"},
		{"???", "scene-shows.ics"},
		{"--edges--", "edges-shows.ics"},
	} {
		if got := sceneFeedFilename(tc.slug); got != tc.want {
			t.Errorf("sceneFeedFilename(%q) = %q, want %q", tc.slug, got, tc.want)
		}
	}

	// Whatever the input, the result can never break out of the quoted header.
	for _, slug := range []string{`a"b`, "a\r\nb", "a;b", "a\\b"} {
		got := sceneFeedFilename(slug)
		if strings.ContainsAny(got, "\"\r\n;\\ ") {
			t.Errorf("sceneFeedFilename(%q) = %q contains a header-breaking character", slug, got)
		}
	}
}
