package observability

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/getsentry/sentry-go"
	"github.com/stretchr/testify/require"
)

func TestScrubSentryEvent_CapsAndRedactsMessageAndException(t *testing.T) {
	long := strings.Repeat("x", sentryValueLimit+500)
	event := &sentry.Event{
		Message: "fetch failed https://wfmu.org/api?api_key=SECRETTOKEN&u=1 with Bearer abc.DEF-123",
		Exception: []sentry.Exception{
			{Value: "provider 500: " + long},
		},
	}

	got := ScrubSentryEvent(event, nil)
	require.NotNil(t, got)

	// URL query string + Bearer token are redacted, the rest of the message kept.
	require.Contains(t, got.Message, "https://wfmu.org/api?"+redactionMarker)
	require.NotContains(t, got.Message, "SECRETTOKEN")
	require.NotContains(t, got.Message, "abc.DEF-123")
	require.Contains(t, got.Message, "Bearer "+redactionMarker)
	require.Contains(t, got.Message, "fetch failed")

	// Over-length exception value is capped (rune-safe) with the marker.
	val := got.Exception[0].Value
	require.LessOrEqual(t, utf8.RuneCountInString(val), sentryValueLimit+utf8.RuneCountInString(truncationMarker))
	require.True(t, strings.HasSuffix(val, truncationMarker))
	require.Contains(t, val, "provider 500:")
}

func TestScrubSentryEvent_ScrubsRequest(t *testing.T) {
	event := &sentry.Event{
		Request: &sentry.Request{
			URL:         "https://psychichomily.com/verify?token=MAGICLINK",
			QueryString: "token=MAGICLINK&next=/home",
			Cookies:     "auth_token=eyJhbGciOi...; other=1",
			// The sentryhttp middleware buffers the body into Data unconditionally
			// (not gated by SendDefaultPII) — a login body would leak plaintext.
			Data: `{"email":"a@b.com","password":"hunter2"}`,
			Headers: map[string]string{
				"Authorization": "Bearer eyJ...",
				"Cookie":        "auth_token=eyJ...",
				"X-Api-Key":     "live_sk_123",
				"User-Agent":    "Mozilla/5.0",
			},
		},
	}

	got := ScrubSentryEvent(event, nil)
	require.NotNil(t, got.Request)
	require.Equal(t, "https://psychichomily.com/verify?"+redactionMarker, got.Request.URL)
	require.NotContains(t, got.Request.URL, "MAGICLINK")
	require.Equal(t, redactionMarker, got.Request.QueryString)
	require.Equal(t, redactionMarker, got.Request.Cookies)
	require.Equal(t, redactionMarker, got.Request.Data, "request body (login password) must be redacted")
	require.NotContains(t, got.Request.Data, "hunter2")
	require.Equal(t, redactionMarker, got.Request.Headers["Authorization"])
	require.Equal(t, redactionMarker, got.Request.Headers["Cookie"])
	require.Equal(t, redactionMarker, got.Request.Headers["X-Api-Key"])
	// Non-sensitive headers are preserved for triage.
	require.Equal(t, "Mozilla/5.0", got.Request.Headers["User-Agent"])
}

func TestScrubSentryEvent_RedactsURLUserinfoAndFreeTextSecrets(t *testing.T) {
	cases := []struct {
		name, in, mustNotContain string
		mustContain              string
	}{
		{
			name:           "postgres DSN userinfo",
			in:             "dial postgres://app:hunter2@db:5432/main?sslmode=require failed",
			mustNotContain: "hunter2",
			mustContain:    "postgres://" + redactionMarker + "@",
		},
		{
			name:           "basic-auth https URL, no query",
			in:             "GET https://admin:s3cr3t@db.internal/health -> 500",
			mustNotContain: "s3cr3t",
			mustContain:    "https://" + redactionMarker + "@db.internal/health",
		},
		{
			name:           "free-text api_key",
			in:             "provider rejected api_key=live_sk_ABC123 (401)",
			mustNotContain: "live_sk_ABC123",
			mustContain:    "api_key=" + redactionMarker,
		},
		{
			name:           "Authorization: Basic <b64>",
			in:             "upstream auth failed: Authorization: Basic dXNlcjpwYXNzd29yZA==",
			mustNotContain: "dXNlcjpwYXNzd29yZA==",
			mustContain:    "Authorization=" + redactionMarker,
		},
		{
			name:           "schemeless token=",
			in:             "could not parse token=DEADBEEF&u=1",
			mustNotContain: "DEADBEEF",
			mustContain:    "token=" + redactionMarker,
		},
		{
			// Provider error bodies (spotify/radio) are JSON — the quoted-key form.
			name:           "json-quoted access_token",
			in:             `token endpoint 400: {"access_token":"BQxyz123","scope":"read"}`,
			mustNotContain: "BQxyz123",
			mustContain:    "access_token=" + redactionMarker,
		},
		{
			name:           "json-quoted password",
			in:             `invalid body {"email":"a@b.com","password":"hunter2"}`,
			mustNotContain: "hunter2",
			mustContain:    "password=" + redactionMarker,
		},
		{
			// Password with a literal '@' must redact past it to the host '@'.
			name:           "DSN password containing @",
			in:             "redis dial: redis://default:p@ssw0rd@cache:6379/0 refused",
			mustNotContain: "ssw0rd",
			mustContain:    "redis://" + redactionMarker + "@",
		},
		{
			// Any auth scheme, not just Bearer/Basic/Digest.
			name:           "Authorization: Negotiate <token>",
			in:             "upstream: Authorization: Negotiate ABC123XYZ failed",
			mustNotContain: "ABC123XYZ",
			mustContain:    "Authorization=" + redactionMarker,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ScrubSentryEvent(&sentry.Event{Message: tc.in}, nil)
			require.NotContains(t, got.Message, tc.mustNotContain)
			require.Contains(t, got.Message, tc.mustContain)
		})
	}
}

func TestScrubSentryEvent_BearerWordBoundary(t *testing.T) {
	// A real Bearer token is redacted...
	got := ScrubSentryEvent(&sentry.Event{Message: "request used Bearer abc.DEF-123 now"}, nil)
	require.Contains(t, got.Message, "Bearer "+redactionMarker)
	require.NotContains(t, got.Message, "abc.DEF-123")

	// ...but a word merely ending in "bearer" must NOT eat the following word.
	got = ScrubSentryEvent(&sentry.Event{Message: "the forbearer walked in"}, nil)
	require.Equal(t, "the forbearer walked in", got.Message)
}

func TestScrubSentryEvent_LeavesNormalEventIntact(t *testing.T) {
	event := &sentry.Event{
		Message:   "database connection refused on startup",
		Exception: []sentry.Exception{{Value: "dial tcp 127.0.0.1:5432: connect: connection refused"}},
	}
	got := ScrubSentryEvent(event, nil)
	require.Equal(t, "database connection refused on startup", got.Message)
	require.Equal(t, "dial tcp 127.0.0.1:5432: connect: connection refused", got.Exception[0].Value)
}

func TestScrubSentryEvent_NilSafe(t *testing.T) {
	require.Nil(t, ScrubSentryEvent(nil, nil))

	// An event with no Request must not panic.
	require.NotPanics(t, func() {
		ScrubSentryEvent(&sentry.Event{Message: "no request here"}, nil)
	})
}
