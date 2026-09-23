package middleware

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"psychic-homily-backend/internal/logger"
)

// logCapture collects the JSON lines a request's context logger writes.
type logCapture struct {
	t   *testing.T
	buf bytes.Buffer
	log *slog.Logger
}

func newLogCapture(t *testing.T) *logCapture {
	t.Helper()
	c := &logCapture{t: t}
	c.log = slog.New(slog.NewJSONHandler(&c.buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	return c
}

// attach returns r with this capture as its context logger, the same slot
// RequestIDMiddleware fills in production.
func (c *logCapture) attach(r *http.Request) *http.Request {
	return r.WithContext(logger.NewContext(r.Context(), c.log))
}

// events returns the captured lines whose event attribute is event.
func (c *logCapture) events(event string) []map[string]any {
	c.t.Helper()
	var out []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(c.buf.String()), "\n") {
		if line == "" {
			continue
		}
		var rec map[string]any
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			c.t.Fatalf("log line is not JSON: %v: %s", err, line)
		}
		if rec["event"] == event {
			out = append(out, rec)
		}
	}
	return out
}

func always() bool { return true }
func never() bool  { return false }

// The client address sits at the trusted hop of a proxied chain; every other
// address in the request is decoy material the log must not carry either.
const (
	observedClientIP = "203.0.113.77"
	spoofedLeftIP    = "198.51.100.200"
	proxyHopIP       = "10.9.9.9"
	proxyRemoteIP    = "10.1.2.3"
	secretSlug       = "secret-slug-xyz"
	bearerToken      = APITokenPrefix + "secret-token-value"
)

func proxiedRequest(c *logCapture) *http.Request {
	req := httptest.NewRequest(http.MethodGet, "/artists/"+secretSlug+"/shows?cursor="+secretSlug, nil)
	req.RemoteAddr = proxyRemoteIP + ":4444"
	req.Header.Set("X-Forwarded-For", spoofedLeftIP+", "+observedClientIP+", "+proxyHopIP)
	req.Header.Set("Authorization", "Bearer "+bearerToken)
	return c.attach(req)
}

func TestFingerprintWithSalt(t *testing.T) {
	saltA := bytes.Repeat([]byte{0xA5}, sha256.Size)
	saltB := bytes.Repeat([]byte{0x5A}, sha256.Size)
	const key = "198.51.100.4"

	first := fingerprintWithSalt(saltA, key)

	if again := fingerprintWithSalt(saltA, key); again != first {
		t.Errorf("same key and salt gave %q then %q; cardinality counts need a stable tag", first, again)
	}
	if other := fingerprintWithSalt(saltB, key); other == first {
		t.Errorf("two salts gave the same fingerprint %q; the salt is not keying the hash", first)
	}
	if other := fingerprintWithSalt(saltA, "198.51.100.5"); other == first {
		t.Errorf("two keys collided on %q", first)
	}
	if first == key || strings.Contains(first, key) {
		t.Errorf("fingerprint %q carries the key it was derived from", first)
	}
	if len(first) != keyFingerprintHexLen {
		t.Errorf("fingerprint length = %d, want %d", len(first), keyFingerprintHexLen)
	}
	if _, err := hex.DecodeString(first); err != nil {
		t.Errorf("fingerprint %q is not hex: %v", first, err)
	}
}

// An unkeyed hash of an IPv4 address is reversed by enumerating the address
// space, so the fingerprint the process writes must not be one.
func TestFingerprint_IsKeyedByProcessSalt(t *testing.T) {
	const key = "198.51.100.4"
	sum := sha256.Sum256([]byte(key))
	unkeyed := hex.EncodeToString(sum[:])[:keyFingerprintHexLen]

	got := fingerprint(key)
	if got == unkeyed {
		t.Fatalf("fingerprint(%q) = the unkeyed SHA-256 prefix; it is reversible by enumeration", key)
	}
	if got != fingerprint(key) {
		t.Error("fingerprint is not stable within the process")
	}
	if got == key {
		t.Error("fingerprint equals its input")
	}
}

// The 429 line names the limiter, its window, the path family and the bucket
// fingerprint, and carries no client address, slug, or credential.
func TestRateLimitRejection_LogLineCarriesNoRawAddress(t *testing.T) {
	t.Setenv(TrustedProxyHopsEnvVar, "2")
	logs := newLogCapture(t)
	handler := limiterSpec{
		name: LimiterPublicReadAnonymous, limit: 1, window: time.Minute, key: KeyByClientIP,
	}.handler()(okHandler())

	for i, want := range []int{http.StatusOK, http.StatusTooManyRequests} {
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, proxiedRequest(logs))
		if rr.Code != want {
			t.Fatalf("request %d: status = %d, want %d", i+1, rr.Code, want)
		}
	}

	assertNoRawIdentifiers(t, logs.buf.String())

	lines := logs.events(rateLimitRejectedEvent)
	if len(lines) != 1 {
		t.Fatalf("got %d %s lines, want 1:\n%s", len(lines), rateLimitRejectedEvent, logs.buf.String())
	}
	line := lines[0]
	for field, want := range map[string]any{
		"msg":               "rate limit exceeded",
		"level":             "WARN",
		"limiter":           string(LimiterPublicReadAnonymous),
		"window_seconds":    float64(60),
		"path_family":       pathFamilyArtist,
		"method":            http.MethodGet,
		"key_fingerprint":   fingerprint(observedClientIP),
		"fingerprint_epoch": fingerprintEpoch,
		"origin_present":    false,
	} {
		if line[field] != want {
			t.Errorf("%s = %v, want %v", field, line[field], want)
		}
	}
	for _, field := range []string{"remote_addr", "path"} {
		if _, ok := line[field]; ok {
			t.Errorf("429 line carries %q: %v", field, line[field])
		}
	}
}

// assertNoRawIdentifiers fails when captured log output carries any address,
// slug, or credential from proxiedRequest.
func assertNoRawIdentifiers(t *testing.T, out string) {
	t.Helper()
	if out == "" {
		t.Fatal("no log output captured")
	}
	for _, raw := range []string{observedClientIP, spoofedLeftIP, proxyHopIP, proxyRemoteIP, secretSlug, bearerToken, "secret-token-value"} {
		if strings.Contains(out, raw) {
			t.Errorf("log output carries %q:\n%s", raw, out)
		}
	}
}

func TestRateLimitRejection_ResponseNamesTheLimiterWindow(t *testing.T) {
	for _, tc := range []struct {
		window time.Duration
		want   string
	}{
		{time.Minute, "60"},
		{time.Hour, "3600"},
	} {
		rr := httptest.NewRecorder()
		req := newLogCapture(t).attach(httptest.NewRequest(http.MethodGet, "/shows/x", nil))
		limiterSpec{name: "test", window: tc.window, key: KeyByClientIP}.rejection()(rr, req)

		if rr.Code != http.StatusTooManyRequests {
			t.Errorf("window %s: status = %d, want 429", tc.window, rr.Code)
		}
		if got := rr.Header().Get("Retry-After"); got != tc.want {
			t.Errorf("window %s: Retry-After = %q, want %q", tc.window, got, tc.want)
		}
		if !strings.Contains(rr.Body.String(), tc.want+" seconds") {
			t.Errorf("window %s: body %q does not name %s seconds", tc.window, rr.Body.String(), tc.want)
		}
	}
}

// Each allowed request the sampler selects writes one line reporting the
// bucket's state after that request; a rejected request writes the 429 line
// instead, never a sample.
func TestSampledHandler_LogsBucketStateOfSelectedRequests(t *testing.T) {
	t.Setenv(TrustedProxyHopsEnvVar, "2")
	logs := newLogCapture(t)
	handler := limiterSpec{
		name: LimiterPublicReadAnonymous, limit: 3, window: time.Minute, key: KeyByClientIP,
	}.sampledHandler(authStateAnonymous, always)(okHandler())

	for i := 0; i < 4; i++ {
		handler.ServeHTTP(httptest.NewRecorder(), proxiedRequest(logs))
	}

	assertNoRawIdentifiers(t, logs.buf.String())

	samples := logs.events(rateLimitAllowedSampleEvent)
	if len(samples) != 3 {
		t.Fatalf("got %d sample lines, want 3 (one per allowed request):\n%s", len(samples), logs.buf.String())
	}
	for i, line := range samples {
		for field, want := range map[string]any{
			"msg":             "rate limit sample",
			"level":           "INFO",
			"limiter":         string(LimiterPublicReadAnonymous),
			"auth_state":      authStateAnonymous,
			"window_seconds":  float64(60),
			"path_family":     pathFamilyArtist,
			"key_fingerprint": fingerprint(observedClientIP),
			"limit":           float64(3),
			"remaining":       float64(2 - i),
		} {
			if line[field] != want {
				t.Errorf("sample %d: %s = %v, want %v", i+1, field, line[field], want)
			}
		}
	}
	if got := len(logs.events(rateLimitRejectedEvent)); got != 1 {
		t.Errorf("got %d rejection lines, want 1", got)
	}
}

// The sampler is consulted once per allowed request, and only its selections
// are logged.
func TestSampledHandler_LogsOnlyWhatTheSamplerSelects(t *testing.T) {
	logs := newLogCapture(t)
	calls := 0
	everyThird := func() bool {
		calls++
		return calls%3 == 0
	}
	handler := limiterSpec{
		name: LimiterPublicReadIPCeiling, limit: 100, window: time.Minute, key: KeyByClientIP,
	}.sampledHandler(authStateAuthenticated, everyThird)(okHandler())

	for i := 0; i < 9; i++ {
		req := httptest.NewRequest(http.MethodGet, "/scenes/phoenix-az", nil)
		req.RemoteAddr = "192.0.2.10:1234"
		handler.ServeHTTP(httptest.NewRecorder(), logs.attach(req))
	}

	if calls != 9 {
		t.Errorf("sampler consulted %d times, want 9", calls)
	}
	samples := logs.events(rateLimitAllowedSampleEvent)
	if len(samples) != 3 {
		t.Fatalf("got %d sample lines, want 3", len(samples))
	}
	for i, line := range samples {
		if want := float64(100 - 3*(i+1)); line["remaining"] != want {
			t.Errorf("sample %d: remaining = %v, want %v", i+1, line["remaining"], want)
		}
		if line["auth_state"] != authStateAuthenticated || line["path_family"] != pathFamilyScene {
			t.Errorf("sample %d: auth_state=%v path_family=%v", i+1, line["auth_state"], line["path_family"])
		}
	}

	quiet := newLogCapture(t)
	silent := limiterSpec{
		name: LimiterPublicReadIPCeiling, limit: 100, window: time.Minute, key: KeyByClientIP,
	}.sampledHandler(authStateAuthenticated, never)(okHandler())
	silent.ServeHTTP(httptest.NewRecorder(), quiet.attach(httptest.NewRequest(http.MethodGet, "/", nil)))
	if got := len(quiet.events(rateLimitAllowedSampleEvent)); got != 0 {
		t.Errorf("a sampler that selects nothing produced %d lines", got)
	}
}

func TestSampleAt_HonoursRate(t *testing.T) {
	count := func(sample func() bool, draws int) int {
		n := 0
		for i := 0; i < draws; i++ {
			if sample() {
				n++
			}
		}
		return n
	}

	if got := count(sampleAt(0), 10_000); got != 0 {
		t.Errorf("rate 0 selected %d of 10000", got)
	}
	if got := count(sampleAt(1), 10_000); got != 10_000 {
		t.Errorf("rate 1 selected %d of 10000", got)
	}

	// 200,000 draws at p=0.1 have a standard deviation of 0.00067 in the
	// observed fraction, so 0.005 is about 7.5 standard deviations.
	const draws = 200_000
	got := float64(count(sampleAt(allowedReadSampleRate), draws)) / draws
	if diff := got - allowedReadSampleRate; diff > 0.005 || diff < -0.005 {
		t.Errorf("rate %v selected fraction %v of %d draws", allowedReadSampleRate, got, draws)
	}
}

// The public-read limiters carry their own names and windows on the 429 line.
func TestLimiterFactories_NameTheirRejections(t *testing.T) {
	for _, tc := range []struct {
		name    string
		mw      func(http.Handler) http.Handler
		limit   int
		limiter LimiterName
		window  float64
	}{
		{"anonymous public read", RateLimitPublicReadAnonymousEndpoints(), APIRequestsPerMinute, LimiterPublicReadAnonymous, 60},
		{"authenticated ip ceiling", RateLimitPublicReadAuthenticatedIPCeiling(), PublicReadAuthenticatedIPCeilingPerMinute, LimiterPublicReadIPCeiling, 60},
	} {
		t.Run(tc.name, func(t *testing.T) {
			logs := newLogCapture(t)
			handler := tc.mw(okHandler())
			var last *httptest.ResponseRecorder
			for i := 0; i <= tc.limit; i++ {
				req := httptest.NewRequest(http.MethodGet, "/venues/some-venue", nil)
				req.RemoteAddr = "192.0.2.44:1234"
				last = httptest.NewRecorder()
				handler.ServeHTTP(last, logs.attach(req))
			}
			if last.Code != http.StatusTooManyRequests {
				t.Fatalf("request %d: status = %d, want 429", tc.limit+1, last.Code)
			}
			if got := last.Header().Get("Retry-After"); got != strconv.Itoa(int(tc.window)) {
				t.Errorf("Retry-After = %q, want %v", got, tc.window)
			}
			lines := logs.events(rateLimitRejectedEvent)
			if len(lines) != 1 {
				t.Fatalf("got %d rejection lines, want 1", len(lines))
			}
			if lines[0]["limiter"] != string(tc.limiter) || lines[0]["window_seconds"] != tc.window {
				t.Errorf("limiter=%v window_seconds=%v, want %s %v",
					lines[0]["limiter"], lines[0]["window_seconds"], tc.limiter, tc.window)
			}
			for _, s := range logs.events(rateLimitAllowedSampleEvent) {
				if s["limiter"] != string(tc.limiter) {
					t.Errorf("sample line names limiter %v, want %s", s["limiter"], tc.limiter)
				}
			}
		})
	}
}

// The two sampled public-read limiters report the auth state of the traffic
// they meter.
func TestPublicReadLimiters_SampleWithTheirAuthState(t *testing.T) {
	for _, tc := range []struct {
		limiter   LimiterName
		mw        func(http.Handler) http.Handler
		authState string
	}{
		{LimiterPublicReadAnonymous, publicReadAnonymousLimiter(always), authStateAnonymous},
		{LimiterPublicReadIPCeiling, publicReadIPCeilingLimiter(always), authStateAuthenticated},
	} {
		logs := newLogCapture(t)
		handler := tc.mw(okHandler())
		for i := 0; i < 3; i++ {
			req := httptest.NewRequest(http.MethodGet, "/shows/some-show", nil)
			req.RemoteAddr = "192.0.2.55:1234"
			handler.ServeHTTP(httptest.NewRecorder(), logs.attach(req))
		}
		samples := logs.events(rateLimitAllowedSampleEvent)
		if len(samples) != 3 {
			t.Fatalf("%s: got %d sampled lines from 3 allowed requests, want 3", tc.limiter, len(samples))
		}
		for _, s := range samples {
			if s["limiter"] != string(tc.limiter) || s["auth_state"] != tc.authState {
				t.Errorf("%s: sample carries limiter=%v auth_state=%v", tc.limiter, s["limiter"], s["auth_state"])
			}
		}
	}
}

func TestRateLimitPathFamily(t *testing.T) {
	for _, tc := range []struct {
		path string
		want string
	}{
		{"/artists/radiohead", pathFamilyArtist},
		{"/artists/42", pathFamilyArtist},
		{"/artists/radiohead/shows/years", pathFamilyArtist},
		{"/artists/radiohead/", pathFamilyArtist},
		{"/shows/desert-doom-night", pathFamilyShow},
		{"/shows/desert-doom-night/also-tonight", pathFamilyShow},
		{"/venues/the-rebel-lounge/shows/2024/exists", pathFamilyVenue},
		{"/scenes/phoenix-az/week/2026-W39", pathFamilyScene},
		{"/search", pathFamilySearch},
		{"/search/", pathFamilySearch},
		{"/artists/search", pathFamilySearch},
		{"/shows/search", pathFamilySearch},
		{"/labels/search", pathFamilySearch},
		{"/artists", PathFamilyOther},
		{"/shows/", PathFamilyOther},
		{"/artists/cities", PathFamilyOther},
		{"/artists/relationships/1/2/vote", PathFamilyOther},
		{"/shows/upcoming", PathFamilyOther},
		{"/shows/calendar/range", PathFamilyOther},
		{"/shows/saves/batch", PathFamilyOther},
		{"/venues/listing", PathFamilyOther},
		{"/artists//shows", PathFamilyOther},
		{"/labels/sub-pop", PathFamilyOther},
		{"/search/radiohead", PathFamilyOther},
		{"/", PathFamilyOther},
		{"", PathFamilyOther},
		{"/health", PathFamilyOther},
	} {
		if got := RateLimitPathFamily(tc.path); got != tc.want {
			t.Errorf("RateLimitPathFamily(%q) = %q, want %q", tc.path, got, tc.want)
		}
	}
}

// origin_present reports whether the request carried an Origin header, and
// nothing about its value.
func TestLogAttrs_OriginPresentCarriesNoOriginValue(t *testing.T) {
	const origin = "https://secret-origin.example"
	spec := limiterSpec{name: "test", limit: 1, window: time.Minute, key: KeyByClientIP}
	for _, tc := range []struct {
		origin string
		want   bool
	}{{"", false}, {origin, true}} {
		logs := newLogCapture(t)
		handler := spec.sampledHandler(authStateAnonymous, always)(okHandler())
		for i := 0; i < 2; i++ {
			req := httptest.NewRequest(http.MethodGet, "/artists/x", nil)
			req.RemoteAddr = "192.0.2.66:1234"
			if tc.origin != "" {
				req.Header.Set("Origin", tc.origin)
			}
			handler.ServeHTTP(httptest.NewRecorder(), logs.attach(req))
		}
		if strings.Contains(logs.buf.String(), "secret-origin") {
			t.Errorf("log output carries the Origin value:\n%s", logs.buf.String())
		}
		for _, event := range []string{rateLimitAllowedSampleEvent, rateLimitRejectedEvent} {
			lines := logs.events(event)
			if len(lines) != 1 {
				t.Fatalf("origin %q: got %d %s lines, want 1", tc.origin, len(lines), event)
			}
			if lines[0]["origin_present"] != tc.want {
				t.Errorf("origin %q: %s origin_present = %v, want %v", tc.origin, event, lines[0]["origin_present"], tc.want)
			}
		}
	}
}

// Log queries group on the limiter attribute, so two limiters sharing a name
// would merge their counts silently.
func TestLimiterNamesAreDistinct(t *testing.T) {
	names := []LimiterName{
		LimiterPublicReadAnonymous, LimiterPublicReadUser, LimiterPublicReadIPCeiling,
		LimiterEngagementMutationBurst, LimiterEngagementMutationSustained,
		LimiterEntityRequestBatchBurst, LimiterEntityRequestBatchSustained,
		LimiterAuth, LimiterPasskey, LimiterVerificationResend, LimiterPasswordConfirm,
		LimiterOAuthLinkToken, LimiterTagCreate, LimiterTagVote, LimiterShowReport,
		LimiterEntityReport, LimiterShowCreate, LimiterAIProcess,
	}
	seen := make(map[LimiterName]bool, len(names))
	for _, name := range names {
		if name == "" || seen[name] {
			t.Errorf("limiter name %q is empty or repeated", name)
		}
		seen[name] = true
	}
}
