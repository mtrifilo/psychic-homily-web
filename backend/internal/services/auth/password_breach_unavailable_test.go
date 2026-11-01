package auth

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const unavailableTestPassword = "xK9$mPq2vL!nR8wJ"

// logRecorder is a slog handler that keeps every record it receives.
type logRecorder struct {
	mu      sync.Mutex
	records []slog.Record
}

func (h *logRecorder) Enabled(context.Context, slog.Level) bool { return true }
func (h *logRecorder) Handle(_ context.Context, r slog.Record) error {
	h.mu.Lock()
	h.records = append(h.records, r.Clone())
	h.mu.Unlock()
	return nil
}
func (h *logRecorder) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h *logRecorder) WithGroup(string) slog.Handler      { return h }

func (h *logRecorder) all() []slog.Record {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]slog.Record(nil), h.records...)
}

func recordAttrs(r slog.Record) map[string]slog.Value {
	attrs := map[string]slog.Value{}
	r.Attrs(func(a slog.Attr) bool {
		attrs[a.Key] = a.Value
		return true
	})
	return attrs
}

// transportTo sends every request to srv, whatever host the request names.
func transportTo(t *testing.T, srv *httptest.Server) http.RoundTripper {
	t.Helper()
	target, err := url.Parse(srv.URL)
	require.NoError(t, err)
	base := srv.Client().Transport
	return roundTripFunc(func(req *http.Request) (*http.Response, error) {
		redirected := req.Clone(req.Context())
		redirected.URL.Scheme = target.Scheme
		redirected.URL.Host = target.Host
		redirected.Host = target.Host
		return base.RoundTrip(redirected)
	})
}

// blockingServer answers every request by calling respond, then blocks until
// the client goes away or the test ends.
func blockingServer(t *testing.T, respond func(http.ResponseWriter)) *httptest.Server {
	t.Helper()
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respond(w)
		select {
		case <-r.Context().Done():
		case <-release:
		}
	}))
	t.Cleanup(srv.Close)
	t.Cleanup(func() { close(release) })
	return srv
}

func statusServer(t *testing.T, status int, body string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func recordingValidator(transport http.RoundTripper) (*PasswordValidator, *logRecorder) {
	rec := &logRecorder{}
	v := NewPasswordValidatorWithTransport(transport)
	v.logger = slog.New(rec)
	return v, rec
}

// assertFailedOpen checks that result is the fail-open outcome of an
// unavailable breach check on a password that passes every other check.
func assertFailedOpen(t *testing.T, v *PasswordValidator) {
	t.Helper()
	result, err := v.ValidatePassword(unavailableTestPassword)
	require.NoError(t, err)
	assert.True(t, result.Valid, "an unavailable breach check accepts the password")
	assert.Empty(t, result.Errors)
	require.Len(t, result.Warnings, 1)
	assert.Contains(t, result.Warnings[0], "breach database")
}

func TestValidatePassword_UnavailableBreachCheckLogsReasonClass(t *testing.T) {
	const shortTimeout = 100 * time.Millisecond

	closedServer := func(t *testing.T) *httptest.Server {
		srv := httptest.NewServer(http.NotFoundHandler())
		srv.Close()
		return srv
	}

	cases := []struct {
		name          string
		server        func(t *testing.T) *httptest.Server
		clientTimeout time.Duration
		wantReason    string
	}{
		{
			name:       "503",
			server:     func(t *testing.T) *httptest.Server { return statusServer(t, http.StatusServiceUnavailable, "") },
			wantReason: "status_503",
		},
		{
			name:       "429",
			server:     func(t *testing.T) *httptest.Server { return statusServer(t, http.StatusTooManyRequests, "") },
			wantReason: "status_429",
		},
		{
			name: "no response before the deadline",
			server: func(t *testing.T) *httptest.Server {
				return blockingServer(t, func(http.ResponseWriter) {})
			},
			clientTimeout: shortTimeout,
			wantReason:    "timeout",
		},
		{
			name: "body not finished before the deadline",
			server: func(t *testing.T) *httptest.Server {
				return blockingServer(t, func(w http.ResponseWriter) {
					w.WriteHeader(http.StatusOK)
					_, _ = w.Write([]byte("0018A45C4D1DEF81644B54AB7F969B88D65:1\r\n"))
					w.(http.Flusher).Flush()
				})
			},
			clientTimeout: shortTimeout,
			wantReason:    "timeout",
		},
		{
			name:       "connection refused",
			server:     closedServer,
			wantReason: "network",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			v, rec := recordingValidator(transportTo(t, tc.server(t)))
			if tc.clientTimeout > 0 {
				v.httpClient.Timeout = tc.clientTimeout
			}

			assertFailedOpen(t, v)

			records := rec.all()
			require.Len(t, records, 1)
			r := records[0]
			assert.Equal(t, "password_breach_check_unavailable", r.Message)
			assert.Equal(t, slog.LevelWarn, r.Level)

			attrs := recordAttrs(r)
			assert.ElementsMatch(t, []string{"reason", "elapsed_ms"}, keysOf(attrs),
				"the line carries the reason class and elapsed time only")
			assert.Equal(t, tc.wantReason, attrs["reason"].String())
			require.Equal(t, slog.KindInt64, attrs["elapsed_ms"].Kind())
			elapsed := attrs["elapsed_ms"].Int64()
			assert.GreaterOrEqual(t, elapsed, int64(0))
			if tc.clientTimeout > 0 {
				assert.GreaterOrEqual(t, elapsed, tc.clientTimeout.Milliseconds(),
					"elapsed covers the wait for the deadline")
			}
		})
	}
}

func keysOf(attrs map[string]slog.Value) []string {
	keys := make([]string, 0, len(attrs))
	for k := range attrs {
		keys = append(keys, k)
	}
	return keys
}

func TestValidatePassword_UnavailableBreachCheckLogCarriesNoPasswordMaterial(t *testing.T) {
	// The network failure's error text names the request URL, which carries
	// the hash prefix; none of it may reach the line.
	srv := httptest.NewServer(http.NotFoundHandler())
	srv.Close()
	v, rec := recordingValidator(transportTo(t, srv))

	assertFailedOpen(t, v)

	sum := sha1.Sum([]byte(unavailableTestPassword))
	fullHash := strings.ToUpper(hex.EncodeToString(sum[:]))
	forbidden := []string{unavailableTestPassword, fullHash, fullHash[:5], fullHash[5:], "/range/"}

	records := rec.all()
	require.Len(t, records, 1)
	line := records[0].Message
	for k, val := range recordAttrs(records[0]) {
		line += " " + k + "=" + val.String()
	}
	for _, f := range forbidden {
		assert.NotContains(t, strings.ToUpper(line), strings.ToUpper(f))
	}
}

func TestValidatePassword_UnavailableBreachCheckLogIsThrottled(t *testing.T) {
	clock := &fakeClock{now: time.Date(2026, 10, 31, 12, 0, 0, 0, time.UTC)}
	v, rec := recordingValidator(transportTo(t, statusServer(t, http.StatusServiceUnavailable, "")))
	v.now = clock.Now

	assertFailedOpen(t, v)
	assert.Len(t, rec.all(), 1)

	assertFailedOpen(t, v)
	assert.Len(t, rec.all(), 1, "a second failure at the same instant writes no second line")

	clock.advance(breachCheckUnavailableLogInterval - time.Second)
	assertFailedOpen(t, v)
	assert.Len(t, rec.all(), 1, "a failure inside the interval writes no second line")

	clock.advance(time.Second)
	assertFailedOpen(t, v)
	assert.Len(t, rec.all(), 2, "the first failure once the interval has passed writes a line")
}

func TestValidatePassword_UnavailableBreachCheckLogIsThrottledAcrossGoroutines(t *testing.T) {
	clock := &fakeClock{now: time.Date(2026, 10, 31, 12, 0, 0, 0, time.UTC)}
	v, rec := recordingValidator(mockHIBPResponse(http.StatusServiceUnavailable, ""))
	v.now = clock.Now

	var wg sync.WaitGroup
	for range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			result, err := v.ValidatePassword(unavailableTestPassword)
			assert.NoError(t, err)
			assert.True(t, result.Valid)
		}()
	}
	wg.Wait()

	assert.Len(t, rec.all(), 1)
}

func TestValidatePassword_CompletedBreachCheckWritesNoUnavailableLine(t *testing.T) {
	sum := sha1.Sum([]byte(unavailableTestPassword))
	suffix := strings.ToUpper(hex.EncodeToString(sum[:]))[5:]

	t.Run("not breached", func(t *testing.T) {
		v, rec := recordingValidator(transportTo(t, statusServer(t, http.StatusOK, "0018A45C4D1DEF81644B54AB7F969B88D65:1\r\n")))

		result, err := v.ValidatePassword(unavailableTestPassword)
		require.NoError(t, err)
		assert.True(t, result.Valid)
		assert.Empty(t, result.Warnings)
		assert.Empty(t, rec.all())
	})

	t.Run("breached", func(t *testing.T) {
		v, rec := recordingValidator(transportTo(t, statusServer(t, http.StatusOK, suffix+":42\r\n")))

		result, err := v.ValidatePassword(unavailableTestPassword)
		require.NoError(t, err)
		assert.False(t, result.Valid)
		assert.Empty(t, result.Warnings)
		assert.Empty(t, rec.all())
	})
}
