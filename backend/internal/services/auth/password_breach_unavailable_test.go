package auth

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"log/slog"
	"maps"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"sync"
	"syscall"
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

// statusServer answers every request with status and an empty body.
func statusServer(t *testing.T, status int) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(status)
	}))
	t.Cleanup(srv.Close)
	return srv
}

// refusedTransport fails every request the way a refused dial does.
var refusedTransport = mockHIBPError(&net.OpError{Op: "dial", Net: "tcp", Err: syscall.ECONNREFUSED})

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

	cases := []struct {
		name          string
		transport     func(t *testing.T) http.RoundTripper
		clientTimeout time.Duration
		wantReason    string
	}{
		{
			name: "503",
			transport: func(t *testing.T) http.RoundTripper {
				return transportTo(t, statusServer(t, http.StatusServiceUnavailable))
			},
			wantReason: "status_503",
		},
		{
			name:       "429",
			transport:  func(*testing.T) http.RoundTripper { return mockHIBPResponse(http.StatusTooManyRequests, "") },
			wantReason: "status_429",
		},
		{
			name: "no response before the deadline",
			transport: func(t *testing.T) http.RoundTripper {
				return transportTo(t, blockingServer(t, func(http.ResponseWriter) {}))
			},
			clientTimeout: shortTimeout,
			wantReason:    "timeout",
		},
		{
			name: "body not finished before the deadline",
			transport: func(t *testing.T) http.RoundTripper {
				return transportTo(t, blockingServer(t, func(w http.ResponseWriter) {
					w.WriteHeader(http.StatusOK)
					_, _ = w.Write([]byte("0018A45C4D1DEF81644B54AB7F969B88D65:1\r\n"))
					w.(http.Flusher).Flush()
				}))
			},
			clientTimeout: shortTimeout,
			wantReason:    "timeout",
		},
		{
			name:       "connection refused",
			transport:  func(*testing.T) http.RoundTripper { return refusedTransport },
			wantReason: "network",
		},
		{
			name: "200 with an empty body",
			transport: func(t *testing.T) http.RoundTripper {
				return transportTo(t, statusServer(t, http.StatusOK))
			},
			wantReason: "bad_body",
		},
		{
			name: "200 with a captive portal page",
			transport: func(*testing.T) http.RoundTripper {
				return mockHIBPResponse(http.StatusOK, "<html><body>Accept the terms to continue</body></html>")
			},
			wantReason: "bad_body",
		},
		{
			name: "200 with a line too long to scan",
			transport: func(*testing.T) http.RoundTripper {
				return mockHIBPResponse(http.StatusOK, strings.Repeat("A", 70000))
			},
			wantReason: "bad_body",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			v, rec := recordingValidator(tc.transport(t))
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
			assert.ElementsMatch(t, []string{"reason", "elapsed_ms"}, slices.Collect(maps.Keys(attrs)),
				"the line carries the reason class and elapsed time only")
			assert.Equal(t, tc.wantReason, attrs["reason"].String())
			require.Equal(t, slog.KindInt64, attrs["elapsed_ms"].Kind())
			if tc.clientTimeout > 0 {
				assert.GreaterOrEqual(t, attrs["elapsed_ms"].Int64(), tc.clientTimeout.Milliseconds(),
					"elapsed covers the wait for the deadline")
			}
		})
	}
}

func TestValidatePassword_UnavailableBreachCheckLogCarriesNoPasswordMaterial(t *testing.T) {
	sum := sha1.Sum([]byte(unavailableTestPassword))
	fullHash := strings.ToUpper(hex.EncodeToString(sum[:]))
	forbidden := []string{unavailableTestPassword, fullHash, fullHash[:5], fullHash[5:], "/range/"}

	cases := []struct {
		name      string
		transport http.RoundTripper
	}{
		{
			// The client wraps a transport failure in an error naming the
			// request URL, which carries the hash prefix.
			name:      "transport failure",
			transport: refusedTransport,
		},
		{
			// A body with no range record that echoes the request and the
			// password material back.
			name: "200 with a garbage body",
			transport: mockHIBPResponse(http.StatusOK,
				"<html>Blocked: https://api.pwnedpasswords.com/range/"+fullHash[:5]+"</html>\r\n"+
					fullHash[5:]+"\r\n"+fullHash+":1:"+unavailableTestPassword+"\r\n"),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			v, rec := recordingValidator(tc.transport)

			assertFailedOpen(t, v)

			records := rec.all()
			require.Len(t, records, 1)
			line := records[0].Message
			for k, val := range recordAttrs(records[0]) {
				line += " " + k + "=" + val.String()
			}
			for _, f := range forbidden {
				assert.NotContains(t, strings.ToUpper(line), strings.ToUpper(f))
			}
		})
	}
}

func TestValidatePassword_UnavailableBreachCheckLogIsThrottled(t *testing.T) {
	clock := &fakeClock{now: time.Date(2026, 10, 31, 12, 0, 0, 0, time.UTC)}
	v, rec := recordingValidator(mockHIBPResponse(http.StatusServiceUnavailable, ""))
	v.now = clock.Now

	assertFailedOpen(t, v)
	assert.Len(t, rec.all(), 1)

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
	t.Run("not breached", func(t *testing.T) {
		v, rec := recordingValidator(mockHIBPResponse(http.StatusOK, "0018A45C4D1DEF81644B54AB7F969B88D65:1\r\n"))

		result, err := v.ValidatePassword(unavailableTestPassword)
		require.NoError(t, err)
		assert.True(t, result.Valid)
		assert.Empty(t, result.Warnings)
		assert.Empty(t, rec.all())
	})

	t.Run("breached", func(t *testing.T) {
		v, rec := recordingValidator(mockHIBPResponse(http.StatusOK, hibpSuffix(unavailableTestPassword)+":42\r\n"))

		result, err := v.ValidatePassword(unavailableTestPassword)
		require.NoError(t, err)
		assert.False(t, result.Valid)
		assert.Empty(t, result.Warnings)
		assert.Empty(t, rec.all())
	})
}
