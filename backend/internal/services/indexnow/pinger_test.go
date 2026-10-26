package indexnow

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"sync"
	"testing"
	"time"

	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/testutil/netguard"
)

func TestMain(m *testing.M) { os.Exit(netguard.RefuseNonLoopbackNetwork(m)) }

const testKey = "abcdef0123456789"

// memoryMarks is a markStore on a fake clock.
type memoryMarks struct {
	now      time.Time
	mark     time.Time
	hasMark  bool
	advances int
}

func (m *memoryMarks) Begin(context.Context) (time.Time, time.Time, bool, error) {
	return m.now, m.mark, m.hasMark, nil
}

func (m *memoryMarks) Advance(_ context.Context, start time.Time) error {
	m.mark, m.hasMark = start, true
	m.advances++
	return nil
}

// fixedSource returns the same entries for every since, and records the since
// it was asked for.
type fixedSource struct {
	entries *contracts.SitemapEntries
	asked   []time.Time
}

func (f *fixedSource) EntriesChangedSince(_ context.Context, since time.Time) (*contracts.SitemapEntries, error) {
	f.asked = append(f.asked, since)
	return f.entries, nil
}

// stub is a loopback IndexNow endpoint answering statuses in order, then the
// last one forever, and recording every body it received.
type stub struct {
	mu       sync.Mutex
	statuses []int
	bodies   []submission
	server   *httptest.Server
}

func newStub(t *testing.T, statuses ...int) *stub {
	t.Helper()
	s := &stub{statuses: statuses}
	s.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var body submission
		if err := json.Unmarshal(raw, &body); err != nil {
			t.Errorf("stub: undecodable body %q: %v", raw, err)
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/json; charset=utf-8" {
			t.Errorf("stub: Content-Type = %q", ct)
		}
		s.mu.Lock()
		s.bodies = append(s.bodies, body)
		status := s.statuses[min(len(s.bodies), len(s.statuses))-1]
		s.mu.Unlock()
		if status == http.StatusTemporaryRedirect {
			w.Header().Set("Location", "/elsewhere")
		}
		w.WriteHeader(status)
		_, _ = fmt.Fprintf(w, "stub answered %d", status)
	}))
	t.Cleanup(s.server.Close)
	return s
}

func (s *stub) received() []submission {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.bodies)
}

func entriesOf(slugs ...string) []contracts.SitemapEntry {
	out := make([]contracts.SitemapEntry, 0, len(slugs))
	for _, slug := range slugs {
		out = append(out, contracts.SitemapEntry{Slug: slug})
	}
	return out
}

func testPinger(t *testing.T, endpoint string, source ChangeSource, marks markStore) *Pinger {
	t.Helper()
	cfg, enabled, reason := ConfigFromEnv("https://www.psychichomily.com", func(key string) string {
		switch key {
		case KeyEnvVar:
			return testKey
		case EndpointOverrideEnvVar:
			return endpoint
		case environmentEnvVar:
			return "test"
		}
		return ""
	})
	if !enabled {
		t.Fatalf("config disabled: %s", reason)
	}
	p := newPinger(cfg, source, marks, slog.New(slog.NewTextHandler(io.Discard, nil)))
	p.submitter.retryBackoff = time.Millisecond
	return p
}

var (
	markAt  = time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC)
	startAt = markAt.Add(10 * time.Minute)
)

func TestConfigFromEnv(t *testing.T) {
	env := func(vars map[string]string) func(string) string {
		return func(key string) string { return vars[key] }
	}
	for _, tc := range []struct {
		name        string
		frontendURL string
		vars        map[string]string
		enabled     bool
		host        string
		endpoint    string
	}{
		{name: "key unset", frontendURL: "https://psychichomily.com", vars: map[string]string{}},
		{name: "key blank", frontendURL: "https://psychichomily.com", vars: map[string]string{KeyEnvVar: "  "}},
		{name: "key too short", frontendURL: "https://psychichomily.com", vars: map[string]string{KeyEnvVar: "abc123"}},
		{name: "key with a slash", frontendURL: "https://psychichomily.com", vars: map[string]string{KeyEnvVar: "abcdef01/../x"}},
		{name: "frontend URL without a scheme", frontendURL: "psychichomily.com", vars: map[string]string{KeyEnvVar: testKey}},
		{
			name: "override on a public host", frontendURL: "https://psychichomily.com",
			vars: map[string]string{KeyEnvVar: testKey, environmentEnvVar: "test", EndpointOverrideEnvVar: "https://evil.test/indexnow"},
		},
		{
			name: "override on a host that only starts like loopback", frontendURL: "https://psychichomily.com",
			vars: map[string]string{KeyEnvVar: testKey, environmentEnvVar: "test", EndpointOverrideEnvVar: "http://localhost.evil.test/indexnow"},
		},
		{
			name: "apex", frontendURL: "https://psychichomily.com", vars: map[string]string{KeyEnvVar: testKey},
			enabled: true, host: "psychichomily.com", endpoint: "https://api.indexnow.org/indexnow",
		},
		{
			name: "www is read as the apex", frontendURL: "https://www.psychichomily.com/", vars: map[string]string{KeyEnvVar: testKey},
			enabled: true, host: "psychichomily.com", endpoint: "https://api.indexnow.org/indexnow",
		},
		{
			name: "stage", frontendURL: "https://stage.psychichomily.com", vars: map[string]string{KeyEnvVar: testKey},
			enabled: true, host: "stage.psychichomily.com", endpoint: "https://api.indexnow.org/indexnow",
		},
		{
			name: "loopback override", frontendURL: "http://localhost:3000",
			vars:    map[string]string{KeyEnvVar: testKey, environmentEnvVar: "test", EndpointOverrideEnvVar: "http://127.0.0.1:9999/indexnow"},
			enabled: true, host: "localhost:3000", endpoint: "http://127.0.0.1:9999/indexnow",
		},
		{
			name: "loopback override in production", frontendURL: "https://www.psychichomily.com",
			vars: map[string]string{KeyEnvVar: testKey, environmentEnvVar: "production", EndpointOverrideEnvVar: "http://127.0.0.1:9999/indexnow"},
		},
		{
			name: "loopback override with ENVIRONMENT unset", frontendURL: "https://psychichomily.com",
			vars: map[string]string{KeyEnvVar: testKey, EndpointOverrideEnvVar: "http://127.0.0.1:9999/indexnow"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg, enabled, reason := ConfigFromEnv(tc.frontendURL, env(tc.vars))
			if enabled != tc.enabled {
				t.Fatalf("enabled = %v (%s), want %v", enabled, reason, tc.enabled)
			}
			if !enabled {
				if reason == "" {
					t.Error("disabled with no reason")
				}
				return
			}
			if cfg.Host != tc.host || cfg.Endpoint != tc.endpoint {
				t.Errorf("host %q endpoint %q, want %q %q", cfg.Host, cfg.Endpoint, tc.host, tc.endpoint)
			}
		})
	}
}

func TestNewIsOffWithoutAKey(t *testing.T) {
	t.Setenv(KeyEnvVar, "")
	p, reason := New(nil, &fixedSource{}, "https://psychichomily.com")
	if p != nil || reason != KeyEnvVar+" is not set" {
		t.Fatalf("New = %v, %q; want nil and the unset-key reason", p, reason)
	}
	t.Setenv(KeyEnvVar, testKey)
	t.Setenv(EndpointOverrideEnvVar, "")
	if p, reason := New(nil, &fixedSource{}, "https://psychichomily.com"); p == nil {
		t.Fatalf("New with a key = nil (%s)", reason)
	}
}

func TestKeyLocationIsTheSiteRoot(t *testing.T) {
	cfg, _, _ := ConfigFromEnv("https://www.psychichomily.com", func(key string) string {
		if key == KeyEnvVar {
			return testKey
		}
		return ""
	})
	if got, want := cfg.KeyLocation(), "https://psychichomily.com/"+testKey+".txt"; got != want {
		t.Errorf("KeyLocation = %q, want %q", got, want)
	}
}

func TestRunSubmitsEveryFamilyAndAdvancesTheMark(t *testing.T) {
	s := newStub(t, http.StatusOK)
	source := &fixedSource{entries: &contracts.SitemapEntries{
		Shows:    entriesOf("a-show"),
		Artists:  entriesOf("an-artist"),
		Venues:   entriesOf("a-venue"),
		Releases: entriesOf("a release"),
		Labels:   entriesOf("not-submitted"),
	}}
	marks := &memoryMarks{now: startAt, mark: markAt, hasMark: true}

	stats, err := testPinger(t, s.server.URL, source, marks).run(context.Background())
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if !slices.Equal(source.asked, []time.Time{markAt}) {
		t.Errorf("selected since %v, want the mark %v", source.asked, markAt)
	}
	got := s.received()
	if len(got) != 1 {
		t.Fatalf("stub received %d batches, want 1", len(got))
	}
	want := submission{
		Host:        "psychichomily.com",
		Key:         testKey,
		KeyLocation: "https://psychichomily.com/" + testKey + ".txt",
		URLList: []string{
			"https://psychichomily.com/shows/a-show",
			"https://psychichomily.com/artists/an-artist",
			"https://psychichomily.com/venues/a-venue",
			"https://psychichomily.com/releases/a%20release",
		},
	}
	if got[0].Host != want.Host || got[0].Key != want.Key || got[0].KeyLocation != want.KeyLocation ||
		!slices.Equal(got[0].URLList, want.URLList) {
		t.Errorf("body = %+v, want %+v", got[0], want)
	}
	if stats != (runStats{selected: 4, submitted: 4, batches: 1, status: http.StatusOK}) {
		t.Errorf("stats = %+v", stats)
	}
	if !marks.mark.Equal(startAt) {
		t.Errorf("mark = %v, want the run's start %v", marks.mark, startAt)
	}
}

func TestRunBatchesAtTheProtocolLimit(t *testing.T) {
	s := newStub(t, http.StatusAccepted)
	slugs := make([]string, 2*maxURLsPerRequest+1)
	for i := range slugs {
		slugs[i] = fmt.Sprintf("show-%d", i)
	}
	source := &fixedSource{entries: &contracts.SitemapEntries{Shows: entriesOf(slugs...)}}
	marks := &memoryMarks{now: startAt, mark: markAt, hasMark: true}

	stats, err := testPinger(t, s.server.URL, source, marks).run(context.Background())
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	var sizes []int
	var all []string
	for _, body := range s.received() {
		sizes = append(sizes, len(body.URLList))
		all = append(all, body.URLList...)
	}
	if !slices.Equal(sizes, []int{maxURLsPerRequest, maxURLsPerRequest, 1}) {
		t.Errorf("batch sizes = %v", sizes)
	}
	if len(all) != len(slugs) || all[0] != "https://psychichomily.com/shows/show-0" ||
		all[len(all)-1] != fmt.Sprintf("https://psychichomily.com/shows/show-%d", len(slugs)-1) {
		t.Errorf("URLs not submitted once each in order: %d submitted", len(all))
	}
	if stats.batches != 3 || stats.submitted != len(slugs) || !marks.mark.Equal(startAt) {
		t.Errorf("stats = %+v, mark = %v", stats, marks.mark)
	}
}

func TestRunWithNothingChangedPostsNothing(t *testing.T) {
	s := newStub(t, http.StatusOK)
	marks := &memoryMarks{now: startAt, mark: markAt, hasMark: true}
	stats, err := testPinger(t, s.server.URL, &fixedSource{entries: &contracts.SitemapEntries{}}, marks).run(context.Background())
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if n := len(s.received()); n != 0 {
		t.Errorf("stub received %d batches, want 0", n)
	}
	if stats != (runStats{}) || !marks.mark.Equal(startAt) {
		t.Errorf("stats = %+v, mark = %v", stats, marks.mark)
	}
}

func TestFirstRunRecordsAMarkAndSubmitsNothing(t *testing.T) {
	s := newStub(t, http.StatusOK)
	source := &fixedSource{entries: &contracts.SitemapEntries{Shows: entriesOf("a-show")}}
	marks := &memoryMarks{now: startAt}
	stats, err := testPinger(t, s.server.URL, source, marks).run(context.Background())
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if len(s.received()) != 0 || len(source.asked) != 0 {
		t.Errorf("first run selected %d times and posted %d batches, want 0 and 0", len(source.asked), len(s.received()))
	}
	if !stats.firstRun || !marks.hasMark || !marks.mark.Equal(startAt) {
		t.Errorf("stats = %+v, mark = %v (has %v)", stats, marks.mark, marks.hasMark)
	}
}

func TestFailedRunsLeaveTheMark(t *testing.T) {
	for _, tc := range []struct {
		name     string
		statuses []int
		requests int
		status   int
	}{
		{"4xx is not retried", []int{http.StatusForbidden}, 1, http.StatusForbidden},
		{"429 is not retried", []int{http.StatusTooManyRequests}, 1, http.StatusTooManyRequests},
		{"a redirect is not followed", []int{http.StatusTemporaryRedirect}, 1, http.StatusTemporaryRedirect},
		{"5xx is retried once, then fails", []int{http.StatusInternalServerError}, 2, http.StatusInternalServerError},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := newStub(t, tc.statuses...)
			source := &fixedSource{entries: &contracts.SitemapEntries{Shows: entriesOf("a-show")}}
			marks := &memoryMarks{now: startAt, mark: markAt, hasMark: true}
			stats, err := testPinger(t, s.server.URL, source, marks).run(context.Background())
			if err == nil {
				t.Fatal("run succeeded, want an error")
			}
			if n := len(s.received()); n != tc.requests {
				t.Errorf("stub received %d requests, want %d", n, tc.requests)
			}
			if stats.status != tc.status || stats.submitted != 0 {
				t.Errorf("stats = %+v", stats)
			}
			if marks.advances != 0 || !marks.mark.Equal(markAt) {
				t.Errorf("mark moved to %v", marks.mark)
			}
		})
	}
}

func TestA5xxThatRecoversOnRetrySucceeds(t *testing.T) {
	s := newStub(t, http.StatusServiceUnavailable, http.StatusOK)
	marks := &memoryMarks{now: startAt, mark: markAt, hasMark: true}
	source := &fixedSource{entries: &contracts.SitemapEntries{Shows: entriesOf("a-show")}}
	stats, err := testPinger(t, s.server.URL, source, marks).run(context.Background())
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if len(s.received()) != 2 || stats.submitted != 1 || !marks.mark.Equal(startAt) {
		t.Errorf("requests %d, stats %+v, mark %v", len(s.received()), stats, marks.mark)
	}
}

// failingTransport fails every request without a response and counts them.
type failingTransport struct{ attempts int }

func (f *failingTransport) RoundTrip(*http.Request) (*http.Response, error) {
	f.attempts++
	return nil, errors.New("connection reset by peer")
}

func TestANetworkFailureIsRetriedOnceAndLeavesTheMark(t *testing.T) {
	s := newStub(t, http.StatusOK)
	marks := &memoryMarks{now: startAt, mark: markAt, hasMark: true}
	source := &fixedSource{entries: &contracts.SitemapEntries{Shows: entriesOf("a-show")}}
	p := testPinger(t, s.server.URL, source, marks)
	transport := &failingTransport{}
	p.submitter.client.Transport = transport
	stats, err := p.run(context.Background())
	if err == nil {
		t.Fatal("run succeeded with every request failing")
	}
	if transport.attempts != 2 {
		t.Errorf("attempts = %d, want the request and one retry", transport.attempts)
	}
	if stats.status != 0 || stats.submitted != 0 || marks.advances != 0 {
		t.Errorf("stats %+v, advances %d", stats, marks.advances)
	}
}

func TestALaterBatchFailingLeavesTheMark(t *testing.T) {
	s := newStub(t, http.StatusOK, http.StatusBadRequest)
	slugs := make([]string, maxURLsPerRequest+1)
	for i := range slugs {
		slugs[i] = fmt.Sprintf("show-%d", i)
	}
	marks := &memoryMarks{now: startAt, mark: markAt, hasMark: true}
	source := &fixedSource{entries: &contracts.SitemapEntries{Shows: entriesOf(slugs...)}}
	stats, err := testPinger(t, s.server.URL, source, marks).run(context.Background())
	if err == nil {
		t.Fatal("run succeeded, want the second batch's error")
	}
	if stats.batches != 2 || stats.submitted != maxURLsPerRequest || marks.advances != 0 {
		t.Errorf("stats %+v, advances %d", stats, marks.advances)
	}
}

type failingMarks struct{ memoryMarks }

func (f *failingMarks) Begin(context.Context) (time.Time, time.Time, bool, error) {
	return time.Time{}, time.Time{}, false, errors.New("database down")
}

func TestAMarkReadFailureSubmitsNothing(t *testing.T) {
	s := newStub(t, http.StatusOK)
	source := &fixedSource{entries: &contracts.SitemapEntries{Shows: entriesOf("a-show")}}
	if _, err := testPinger(t, s.server.URL, source, &failingMarks{}).run(context.Background()); err == nil {
		t.Fatal("run succeeded without a mark")
	}
	if len(s.received()) != 0 || len(source.asked) != 0 {
		t.Errorf("selected %d times, posted %d batches", len(source.asked), len(s.received()))
	}
}
