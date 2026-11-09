package engagement

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"psychic-homily-backend/internal/api/handlers/shared/testhelpers"
	authm "psychic-homily-backend/internal/models/auth"
	engagementm "psychic-homily-backend/internal/models/engagement"
	"psychic-homily-backend/internal/services/contracts"
)

// feedRead is one RecordFeedRead call the handler made.
type feedRead struct {
	token     string
	userAgent string
}

// feedHandlerRecordingReads is a calendar handler serving ics to user 7 that
// records every RecordFeedRead call into reads, answering each with
// recordErr. Work after the response runs synchronously, so a test sees
// exactly what ran once the handler returned.
func feedHandlerRecordingReads(ics []byte, reads *[]feedRead, recordErr error) *CalendarHandler {
	mock := &testhelpers.MockCalendarService{
		ValidateCalendarTokenFn: func(string) (*authm.User, error) {
			return &authm.User{ID: 7}, nil
		},
		GenerateICSFeedFn: func(uint, string) ([]byte, error) {
			return ics, nil
		},
		RecordFeedReadFn: func(_ context.Context, token, userAgent string) error {
			*reads = append(*reads, feedRead{token: token, userAgent: userAgent})
			return recordErr
		},
	}
	h := NewCalendarHandler(mock, testCalendarConfig())
	h.afterFeedResponse = func(work func()) bool { work(); return true }
	return h
}

// A served feed is recorded once, with the token it was served for and the
// caller's User-Agent; the service decides what the User-Agent means.
func TestGetCalendarFeedHandler_RecordsTheReadAfterTheResponse(t *testing.T) {
	ics := []byte("BEGIN:VCALENDAR\r\nEND:VCALENDAR\r\n")
	var reads []feedRead
	h := feedHandlerRecordingReads(ics, &reads, nil)
	req := feedRequestWithToken("phcal_ok")
	req.Header.Set("User-Agent", "Google-Calendar-Importer")
	w := httptest.NewRecorder()
	h.afterFeedResponse = func(work func()) bool {
		if w.Code != http.StatusOK || w.Body.String() != string(ics) {
			t.Errorf("the record was handed off before the feed was written: %d %q", w.Code, w.Body.String())
		}
		work()
		return true
	}

	h.GetCalendarFeedHandler(w, req)

	if w.Code != http.StatusOK || w.Body.String() != string(ics) {
		t.Fatalf("feed response = %d %q, want 200 with the feed", w.Code, w.Body.String())
	}
	want := []feedRead{{token: "phcal_ok", userAgent: "Google-Calendar-Importer"}}
	if len(reads) != 1 || reads[0] != want[0] {
		t.Errorf("RecordFeedRead calls = %+v, want %+v", reads, want)
	}
}

// A failing record changes nothing about the feed response.
func TestGetCalendarFeedHandler_FailingRecordLeavesTheResponse(t *testing.T) {
	ics := []byte("BEGIN:VCALENDAR\r\nEND:VCALENDAR\r\n")
	var okReads, failedReads []feedRead
	ok := httptest.NewRecorder()
	failed := httptest.NewRecorder()

	feedHandlerRecordingReads(ics, &okReads, nil).GetCalendarFeedHandler(ok, feedRequestWithToken("phcal_ok"))
	feedHandlerRecordingReads(ics, &failedReads, errors.New("db down")).GetCalendarFeedHandler(failed, feedRequestWithToken("phcal_ok"))

	if len(failedReads) != 1 {
		t.Fatalf("the failing record must still have been attempted, got %d calls", len(failedReads))
	}
	if failed.Code != ok.Code || failed.Body.String() != ok.Body.String() {
		t.Errorf("failing record changed the response: %d %q, want %d %q", failed.Code, failed.Body.String(), ok.Code, ok.Body.String())
	}
	for _, header := range []string{"Content-Type", "Content-Disposition", "Cache-Control"} {
		if failed.Header().Get(header) != ok.Header().Get(header) {
			t.Errorf("%s = %q, want %q", header, failed.Header().Get(header), ok.Header().Get(header))
		}
	}
}

// Nothing is recorded for a feed that was not served.
func TestGetCalendarFeedHandler_UnservedFeedRecordsNothing(t *testing.T) {
	cases := map[string]*testhelpers.MockCalendarService{
		"invalid token": {
			ValidateCalendarTokenFn: func(string) (*authm.User, error) { return nil, errors.New("invalid calendar token") },
		},
		"generation error": {
			ValidateCalendarTokenFn: func(string) (*authm.User, error) { return &authm.User{ID: 7}, nil },
			GenerateICSFeedFn:       func(uint, string) ([]byte, error) { return nil, errors.New("ics build failed") },
		},
	}
	for name, mock := range cases {
		t.Run(name, func(t *testing.T) {
			var recorded int
			mock.RecordFeedReadFn = func(context.Context, string, string) error {
				recorded++
				return nil
			}
			h := NewCalendarHandler(mock, testCalendarConfig())
			h.afterFeedResponse = func(work func()) bool { work(); return true }

			h.GetCalendarFeedHandler(httptest.NewRecorder(), feedRequestWithToken("phcal_ok"))

			if recorded != 0 {
				t.Errorf("RecordFeedRead called %d times for an unserved feed", recorded)
			}
		})
	}
}

// The record runs on a context the end of the request does not cancel, with
// a deadline of its own.
func TestGetCalendarFeedHandler_RecordOutlivesTheRequestContext(t *testing.T) {
	var called, hasDeadline bool
	var errAtCall error
	mock := &testhelpers.MockCalendarService{
		ValidateCalendarTokenFn: func(string) (*authm.User, error) { return &authm.User{ID: 7}, nil },
		GenerateICSFeedFn:       func(uint, string) ([]byte, error) { return []byte("x"), nil },
		RecordFeedReadFn: func(ctx context.Context, _, _ string) error {
			called = true
			errAtCall = ctx.Err()
			_, hasDeadline = ctx.Deadline()
			return nil
		},
	}
	h := NewCalendarHandler(mock, testCalendarConfig())
	var pending func()
	h.afterFeedResponse = func(work func()) bool { pending = work; return true }
	req := feedRequestWithToken("phcal_ok")
	reqCtx, cancel := context.WithCancel(req.Context())
	req = req.WithContext(reqCtx)

	h.GetCalendarFeedHandler(httptest.NewRecorder(), req)
	cancel()
	if pending == nil {
		t.Fatal("no after-response work was scheduled")
	}
	pending()

	if !called {
		t.Fatal("RecordFeedRead was not called")
	}
	if errAtCall != nil {
		t.Errorf("the record's context ended with the request: %v", errAtCall)
	}
	if !hasDeadline {
		t.Error("the record's context must carry its own deadline")
	}
}

// statusWithReads answers GetTokenStatus with a recoverable token that two
// calendar clients have read.
func statusWithReads(_ uint, apiBaseURL string) (*contracts.CalendarTokenStatusResponse, error) {
	status, _ := recoverableTokenStatus(0, apiBaseURL)
	now := time.Now()
	status.Reads = []contracts.CalendarFeedReadResponse{
		{Client: engagementm.CalendarFeedClientAppleIOS, FirstReadAt: now, LastReadAt: now},
		{Client: engagementm.CalendarFeedClientGoogle, FirstReadAt: now, LastReadAt: now},
	}
	return status, nil
}

// A session caller gets the observed reads; an API-token principal gets none,
// though the service returned them.
func TestGetCalendarTokenStatusHandler_ReadsOnlyForASession(t *testing.T) {
	h := NewCalendarHandler(&testhelpers.MockCalendarService{GetTokenStatusFn: statusWithReads}, testCalendarConfig())

	session, err := h.GetCalendarTokenStatusHandler(
		testhelpers.CtxWithSession(&authm.User{ID: 1}, time.Now().Add(-time.Hour)), &GetCalendarTokenStatusRequest{})
	if err != nil {
		t.Fatalf("session status: %v", err)
	}
	if len(session.Body.Reads) != 2 {
		t.Errorf("a session caller gets the reads, got %+v", session.Body.Reads)
	}

	apiToken, err := h.GetCalendarTokenStatusHandler(
		testhelpers.CtxWithSessionAuthTime(&authm.User{ID: 1}, time.Time{}), &GetCalendarTokenStatusRequest{})
	if err != nil {
		t.Fatalf("API-token status: %v", err)
	}
	if apiToken.Body.Reads != nil {
		t.Errorf("an API-token principal must get no reads, got %+v", apiToken.Body.Reads)
	}
}

// A session caller with no token is told the link was reset and why; an
// API-token principal is not.
func TestGetCalendarTokenStatusHandler_LinkResetOnlyForASession(t *testing.T) {
	resetAt := time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)
	reason := engagementm.CalendarLinkResetSignOutEverywhere
	h := NewCalendarHandler(&testhelpers.MockCalendarService{
		GetTokenStatusFn: func(uint, string) (*contracts.CalendarTokenStatusResponse, error) {
			return &contracts.CalendarTokenStatusResponse{LinkResetAt: &resetAt, LinkResetReason: &reason}, nil
		},
	}, testCalendarConfig())

	session, err := h.GetCalendarTokenStatusHandler(
		testhelpers.CtxWithSession(&authm.User{ID: 1}, time.Now().Add(-time.Hour)), &GetCalendarTokenStatusRequest{})
	if err != nil {
		t.Fatalf("session status: %v", err)
	}
	if session.Body.LinkResetAt == nil || !session.Body.LinkResetAt.Equal(resetAt) ||
		session.Body.LinkResetReason == nil || *session.Body.LinkResetReason != reason {
		t.Errorf("a session caller gets the link reset, got %+v", session.Body)
	}

	apiToken, err := h.GetCalendarTokenStatusHandler(
		testhelpers.CtxWithSessionAuthTime(&authm.User{ID: 1}, time.Time{}), &GetCalendarTokenStatusRequest{})
	if err != nil {
		t.Fatalf("API-token status: %v", err)
	}
	if apiToken.Body.LinkResetAt != nil || apiToken.Body.LinkResetReason != nil {
		t.Errorf("an API-token principal must get no link reset, got %+v", apiToken.Body)
	}
}
