package engagement

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"psychic-homily-backend/internal/api/handlers/shared/testhelpers"
	authm "psychic-homily-backend/internal/models/auth"
	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/testlog"
)

// personalFeed is one personal feed handler and what it answers with.
type personalFeed struct {
	name        string
	serve       func(h *CalendarHandler, w http.ResponseWriter, r *http.Request)
	invalidLog  string
	contentType string
	filename    string
	// setNotice installs fn as the mock's lookup for this feed's notice.
	setNotice func(m *testhelpers.MockCalendarService, fn func(string) (*contracts.RetiredFeedNotice, error))
}

var personalFeeds = []personalFeed{
	{
		// Serves /feeds/{token}/saved-shows.ics and the legacy /calendar/{token}.
		name:        "saved-shows calendar",
		serve:       (*CalendarHandler).GetCalendarFeedHandler,
		invalidLog:  "calendar_feed_invalid_token",
		contentType: "text/calendar; charset=utf-8",
		filename:    "psychic-homily-saved-shows.ics",
		setNotice: func(m *testhelpers.MockCalendarService, fn func(string) (*contracts.RetiredFeedNotice, error)) {
			m.RetiredSavedShowsNoticeFn = fn
		},
	},
	{
		name:        "follows Atom feed",
		serve:       (*CalendarHandler).GetFollowsActivityFeedHandler,
		invalidLog:  "follows_activity_feed_invalid_token",
		contentType: "application/atom+xml; charset=utf-8",
		filename:    "psychic-homily-follows.atom",
		setNotice: func(m *testhelpers.MockCalendarService, fn func(string) (*contracts.RetiredFeedNotice, error)) {
			m.RetiredFollowsNoticeFn = fn
		},
	},
}

var testNotice = &contracts.RetiredFeedNotice{Body: []byte("NOTICE BODY"), ETag: `W/"0123456789abcdef"`}

// feedAnswer is everything a feed request produced: the response and the log.
type feedAnswer struct {
	status int
	header http.Header
	body   string
	log    string
}

// logTimestamp strips the per-record time so two runs' logs compare equal.
var logTimestamp = regexp.MustCompile(`time=\S+ `)

// logMarker is logged inside every capture, so an empty capture cannot pass
// for a quiet handler.
const logMarker = "retired-link-test-marker"

// askFeed serves one request for token to feed through a handler whose
// service refuses the token with refusal, answers the retired-link lookup
// with lookup, and records nothing after the response unless a read is
// recorded, which it counts in reads.
func askFeed(t *testing.T, feed personalFeed, switchOn bool, lookup func(string) (*contracts.RetiredFeedNotice, error), ifNoneMatch string, reads *int) feedAnswer {
	t.Helper()
	mock := &testhelpers.MockCalendarService{
		ValidateCalendarTokenFn: func(string) (*authm.User, error) {
			return nil, errors.New("invalid calendar token")
		},
		RecordFeedReadFn: func(context.Context, string, string) error {
			*reads++
			return nil
		},
	}
	feed.setNotice(mock, lookup)
	cfg := testCalendarConfig()
	cfg.Calendar.RetiredLinkNoticeEnabled = switchOn
	h := NewCalendarHandler(mock, cfg)
	h.afterFeedResponse = func(work func()) bool { work(); return true }

	w := httptest.NewRecorder()
	log := testlog.Capture(t, func() {
		slog.Info(logMarker)
		r := testlog.Request(t, feedRequestWithToken("phcal_retired"))
		if ifNoneMatch != "" {
			r.Header.Set("If-None-Match", ifNoneMatch)
		}
		feed.serve(h, w, r)
	})
	if !strings.Contains(log, logMarker) {
		t.Fatalf("the capture missed its own marker: %q", log)
	}
	return feedAnswer{
		status: w.Code,
		header: w.Header(),
		body:   w.Body.String(),
		log:    logTimestamp.ReplaceAllString(log, ""),
	}
}

func noticeLookup(calls *int, notice *contracts.RetiredFeedNotice, err error) func(string) (*contracts.RetiredFeedNotice, error) {
	return func(token string) (*contracts.RetiredFeedNotice, error) {
		*calls++
		if token != "phcal_retired" {
			return nil, errors.New("unexpected token " + token)
		}
		return notice, err
	}
}

// requireUnknownTokenAnswer checks that answer is the 401 and log line an
// unknown token gets: exactly what http.Error writes, and one warning.
func requireUnknownTokenAnswer(t *testing.T, feed personalFeed, answer feedAnswer) {
	t.Helper()
	if answer.status != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", answer.status)
	}
	if answer.body != "invalid or expired token\n" {
		t.Errorf("body = %q", answer.body)
	}
	wantHeader := http.Header{
		"Content-Type":           {"text/plain; charset=utf-8"},
		"X-Content-Type-Options": {"nosniff"},
	}
	if len(answer.header) != len(wantHeader) {
		t.Errorf("headers = %v, want %v", answer.header, wantHeader)
	}
	for name, want := range wantHeader {
		if got := answer.header.Values(name); strings.Join(got, ",") != strings.Join(want, ",") {
			t.Errorf("%s = %v, want %v", name, got, want)
		}
	}
	wantLog := `level=WARN msg=` + feed.invalidLog + ` error="invalid calendar token"`
	if strings.Count(answer.log, "level=WARN") != 1 || !strings.Contains(answer.log, wantLog) {
		t.Errorf("log = %q, want one line %q", answer.log, wantLog)
	}
}

// Switched off, a retired address is answered exactly as an unknown one, and
// as an unknown one is answered switched on: the same 401, body, headers and
// warning, and the retired-link lookup is never made.
func TestRetiredLink_SwitchOffAnswersAsAnUnknownToken(t *testing.T) {
	for _, feed := range personalFeeds {
		t.Run(feed.name, func(t *testing.T) {
			var calls, reads int
			retiredOff := askFeed(t, feed, false, noticeLookup(&calls, testNotice, nil), "", &reads)
			if calls != 0 {
				t.Errorf("switched off, the lookup was made %d times", calls)
			}
			unknownOff := askFeed(t, feed, false, noticeLookup(&calls, nil, nil), "", &reads)
			unknownOn := askFeed(t, feed, true, noticeLookup(&calls, nil, nil), "", &reads)

			for name, answer := range map[string]feedAnswer{"retired, off": retiredOff, "unknown, off": unknownOff, "unknown, on": unknownOn} {
				t.Run(name, func(t *testing.T) { requireUnknownTokenAnswer(t, feed, answer) })
			}
			if retiredOff.body != unknownOn.body || retiredOff.log != unknownOn.log {
				t.Errorf("retired switched off differs from unknown switched on:\n%#v\n%#v", retiredOff, unknownOn)
			}
			if reads != 0 {
				t.Errorf("%d feed reads recorded for a refused token", reads)
			}
		})
	}
}

// Switched on, a retired address gets its notice with 200, the feed's content
// type and filename, a private cache lifetime and a validator; no log line
// and no recorded read.
func TestRetiredLink_SwitchOnAnswersTheNotice(t *testing.T) {
	for _, feed := range personalFeeds {
		t.Run(feed.name, func(t *testing.T) {
			var calls, reads int
			answer := askFeed(t, feed, true, noticeLookup(&calls, testNotice, nil), "", &reads)

			if answer.status != http.StatusOK {
				t.Fatalf("status = %d, want 200", answer.status)
			}
			if answer.body != string(testNotice.Body) {
				t.Errorf("body = %q", answer.body)
			}
			for name, want := range map[string]string{
				"Content-Type":        feed.contentType,
				"Content-Disposition": `inline; filename="` + feed.filename + `"`,
				"Cache-Control":       "private, max-age=3600",
				"ETag":                testNotice.ETag,
			} {
				if got := answer.header.Get(name); got != want {
					t.Errorf("%s = %q, want %q", name, got, want)
				}
			}
			if calls != 1 {
				t.Errorf("lookup made %d times, want 1", calls)
			}
			if reads != 0 {
				t.Errorf("%d feed reads recorded for a retired address", reads)
			}
			if strings.Contains(answer.log, "level=WARN") || strings.Contains(answer.log, "level=ERROR") {
				t.Errorf("a retired poll logged: %q", answer.log)
			}
		})
	}
}

// A client holding the notice's validator gets a bodyless 304.
func TestRetiredLink_MatchingValidatorGets304(t *testing.T) {
	for _, feed := range personalFeeds {
		t.Run(feed.name, func(t *testing.T) {
			var calls, reads int
			answer := askFeed(t, feed, true, noticeLookup(&calls, testNotice, nil), testNotice.ETag, &reads)
			if answer.status != http.StatusNotModified {
				t.Fatalf("status = %d, want 304", answer.status)
			}
			if answer.body != "" {
				t.Errorf("a 304 carried a body: %q", answer.body)
			}
			if answer.header.Get("ETag") != testNotice.ETag {
				t.Errorf("ETag = %q", answer.header.Get("ETag"))
			}
		})
	}
}

// A failed lookup is logged as an error, and the address is then answered as
// an unknown one.
func TestRetiredLink_FailedLookupAnswersAsAnUnknownToken(t *testing.T) {
	for _, feed := range personalFeeds {
		t.Run(feed.name, func(t *testing.T) {
			var calls, reads int
			answer := askFeed(t, feed, true, noticeLookup(&calls, nil, errors.New("database down")), "", &reads)
			requireUnknownTokenAnswer(t, feed, answer)
			if !strings.Contains(answer.log, `level=ERROR msg=calendar_feed_retired_link_lookup_failed error="database down"`) {
				t.Errorf("log = %q", answer.log)
			}
		})
	}
}
