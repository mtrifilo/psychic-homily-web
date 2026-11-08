package engagement

import (
	"bytes"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"gorm.io/gorm"

	"psychic-homily-backend/internal/api/handlers/shared/testhelpers"
	authm "psychic-homily-backend/internal/models/auth"
	"psychic-homily-backend/internal/services/contracts"
	engagementsvc "psychic-homily-backend/internal/services/engagement"
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
	h := NewCalendarHandler(mock, testCalendarConfig())

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
	h := NewCalendarHandler(mock, testCalendarConfig())

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
	h := NewCalendarHandler(mock, testCalendarConfig())

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
	h := NewCalendarHandler(mock, testCalendarConfig())

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

// Through the real calendar service: the file carries exactly the upcoming
// approved, not-cancelled saves, on an RFC 5545 wire, including a save made
// after the subscription feed cached its bytes.
func TestGetSavedShowsCalendarDownload_RealServiceFreshAndGated(t *testing.T) {
	stamp := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	show := func(id uint, status string, cancelled bool) *contracts.SavedShowResponse {
		return &contracts.SavedShowResponse{ShowResponse: contracts.ShowResponse{
			ID:          id,
			Title:       fmt.Sprintf("Show %d", id),
			EventDate:   time.Now().Add(72 * time.Hour),
			Status:      status,
			IsCancelled: cancelled,
			CreatedAt:   stamp,
			UpdatedAt:   stamp.Add(time.Duration(id) * time.Hour),
		}}
	}
	saved := []*contracts.SavedShowResponse{
		show(1, "approved", false),
		show(2, "pending", false),
		show(3, "approved", true),
	}
	source := &testhelpers.MockSavedShowService{
		GetUserSavedShowsFn: func(userID uint, _, _ int, timeFilter string) ([]*contracts.SavedShowResponse, int64, error) {
			if userID != 7 || timeFilter != "upcoming" {
				t.Errorf("saved shows read for user %d with filter %q", userID, timeFilter)
			}
			return saved, int64(len(saved)), nil
		},
	}
	svc := engagementsvc.NewCalendarService(&gorm.DB{}, source)
	h := NewCalendarHandler(svc, testCalendarConfig())

	cachedFeed, err := svc.GenerateICSFeed(7, "http://localhost:3000")
	if err != nil {
		t.Fatalf("prime feed cache: %v", err)
	}

	// Saved a moment before the download; nothing clears the feed cache.
	saved = append(saved, show(4, "approved", false))

	w := httptest.NewRecorder()
	h.GetSavedShowsCalendarDownloadHandler(w, savedShowsDownloadRequest(&authm.User{ID: 7}))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	body := w.Body.Bytes()

	assertCRLFCalendarWithStampedEvents(t, body, 2)
	for _, id := range []uint{1, 4} {
		if !bytes.Contains(body, []byte(fmt.Sprintf("UID:show-%d@psychichomily.com\r\n", id))) {
			t.Errorf("download is missing approved upcoming save %d", id)
		}
	}
	for _, id := range []uint{2, 3} {
		if bytes.Contains(body, []byte(fmt.Sprintf("UID:show-%d@", id))) {
			t.Errorf("download carries pending or cancelled save %d", id)
		}
	}

	stillCached, err := svc.GenerateICSFeed(7, "http://localhost:3000")
	if err != nil {
		t.Fatalf("re-read feed: %v", err)
	}
	if !bytes.Equal(stillCached, cachedFeed) || bytes.Contains(stillCached, []byte("UID:show-4@")) {
		t.Error("the subscription feed is not serving its cached bytes, so this test proves no freshness")
	}
}

// assertCRLFCalendarWithStampedEvents is a byte-level RFC 5545 check: every
// line ends in CRLF with no bare CR or LF anywhere, and each of wantEvents
// VEVENTs carries exactly one DTSTAMP and one SEQUENCE.
func assertCRLFCalendarWithStampedEvents(t *testing.T, body []byte, wantEvents int) {
	t.Helper()

	if !bytes.HasSuffix(body, []byte("\r\n")) {
		t.Error("calendar does not end with CRLF")
	}
	if n, crlf := bytes.Count(body, []byte("\n")), bytes.Count(body, []byte("\r\n")); n != crlf {
		t.Errorf("%d bare LF line endings", n-crlf)
	}
	if n, crlf := bytes.Count(body, []byte("\r")), bytes.Count(body, []byte("\r\n")); n != crlf {
		t.Errorf("%d bare CR characters", n-crlf)
	}

	events := 0
	var inEvent bool
	var stamps, sequences int
	for _, line := range strings.Split(strings.TrimSuffix(string(body), "\r\n"), "\r\n") {
		switch {
		case line == "BEGIN:VEVENT":
			inEvent, stamps, sequences = true, 0, 0
		case line == "END:VEVENT":
			events++
			if stamps != 1 || sequences != 1 {
				t.Errorf("VEVENT %d has %d DTSTAMP and %d SEQUENCE properties, want 1 each", events, stamps, sequences)
			}
			inEvent = false
		case inEvent && strings.HasPrefix(line, "DTSTAMP"):
			stamps++
		case inEvent && strings.HasPrefix(line, "SEQUENCE"):
			sequences++
		}
	}
	if events != wantEvents {
		t.Errorf("calendar has %d VEVENTs, want %d", events, wantEvents)
	}
}
