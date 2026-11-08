package engagement

import (
	"bytes"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"psychic-homily-backend/internal/api/handlers/shared/testhelpers"
	authm "psychic-homily-backend/internal/models/auth"
)

func savedShowsDownloadRequest(user *authm.User) *http.Request {
	req := httptest.NewRequest(http.MethodGet, "/saved-shows/calendar.ics", nil)
	if user != nil {
		req = req.WithContext(testhelpers.CtxWithUser(user))
	}
	return req
}

func TestGetSavedShowsCalendarDownload_NoUserIsUnauthorized(t *testing.T) {
	mock := &testhelpers.MockCalendarService{
		RenderSavedShowsCalendarFn: func(uint, string) ([]byte, error) {
			t.Error("rendered a calendar for a request with no user")
			return nil, nil
		},
	}
	h := NewCalendarHandler(mock, &testhelpers.MockUserService{}, testCalendarConfig())

	w := httptest.NewRecorder()
	h.GetSavedShowsCalendarDownloadHandler(w, savedShowsDownloadRequest(nil))

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", w.Code)
	}
}

// The download renders through the uncached path, for the session's user, with
// the configured frontend origin, and never reads the subscription feed.
func TestGetSavedShowsCalendarDownload_RendersUncachedForSessionUser(t *testing.T) {
	payload := []byte("BEGIN:VCALENDAR\r\nEND:VCALENDAR\r\n")
	mock := &testhelpers.MockCalendarService{
		RenderSavedShowsCalendarFn: func(userID uint, frontendURL string) ([]byte, error) {
			if userID != 7 {
				t.Errorf("rendered for user %d, want 7", userID)
			}
			if frontendURL != "http://localhost:3000" {
				t.Errorf("frontendURL = %q", frontendURL)
			}
			return payload, nil
		},
		GenerateICSFeedFn: func(uint, string) ([]byte, error) {
			t.Error("the download read the cached subscription feed")
			return nil, nil
		},
	}
	h := NewCalendarHandler(mock, &testhelpers.MockUserService{}, testCalendarConfig())

	w := httptest.NewRecorder()
	h.GetSavedShowsCalendarDownloadHandler(w, savedShowsDownloadRequest(&authm.User{ID: 7}))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	for header, want := range map[string]string{
		"Content-Type":        "text/calendar; charset=utf-8",
		"Content-Disposition": `attachment; filename="psychic-homily-saved-shows.ics"`,
		"Cache-Control":       "private, no-store",
	} {
		if got := w.Header().Get(header); got != want {
			t.Errorf("%s = %q, want %q", header, got, want)
		}
	}
	if !bytes.Equal(w.Body.Bytes(), payload) {
		t.Errorf("body = %q, want the rendered bytes", w.Body.String())
	}
}

// No validator is sent, so no conditional request can turn the download into a
// bodyless 304, not even If-None-Match: *.
func TestGetSavedShowsCalendarDownload_SendsNoValidator(t *testing.T) {
	mock := &testhelpers.MockCalendarService{
		RenderSavedShowsCalendarFn: func(uint, string) ([]byte, error) {
			return []byte("BEGIN:VCALENDAR\r\nEND:VCALENDAR\r\n"), nil
		},
	}
	h := NewCalendarHandler(mock, &testhelpers.MockUserService{}, testCalendarConfig())

	for _, inm := range []string{"*", `"anything"`} {
		req := savedShowsDownloadRequest(&authm.User{ID: 7})
		req.Header.Set("If-None-Match", inm)
		w := httptest.NewRecorder()
		h.GetSavedShowsCalendarDownloadHandler(w, req)

		if w.Code != http.StatusOK || w.Body.Len() == 0 {
			t.Errorf("If-None-Match %s: status = %d with %d body bytes, want 200 with the file", inm, w.Code, w.Body.Len())
		}
		if etag, ok := w.Header()["Etag"]; ok {
			t.Errorf("If-None-Match %s: ETag header %q sent under no-store", inm, etag)
		}
	}
}

func TestGetSavedShowsCalendarDownload_RenderFailureIs500(t *testing.T) {
	mock := &testhelpers.MockCalendarService{
		RenderSavedShowsCalendarFn: func(uint, string) ([]byte, error) {
			return nil, errors.New("db down")
		},
	}
	h := NewCalendarHandler(mock, &testhelpers.MockUserService{}, testCalendarConfig())

	w := httptest.NewRecorder()
	h.GetSavedShowsCalendarDownloadHandler(w, savedShowsDownloadRequest(&authm.User{ID: 7}))

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", w.Code)
	}
	if strings.Contains(w.Body.String(), "db down") {
		t.Error("the internal error was echoed into the response")
	}
	if w.Header().Get("Content-Type") == "text/calendar; charset=utf-8" {
		t.Error("a failed render was labelled as a calendar file")
	}
}
