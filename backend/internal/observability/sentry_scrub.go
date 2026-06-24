// Package observability holds cross-cutting observability-pipeline config that
// is independent of the HTTP request path (the per-request Sentry scope lives in
// internal/api/middleware/sentry.go).
package observability

import (
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/getsentry/sentry-go"
)

// sentryValueLimit caps message / exception strings before they leave for
// Sentry, matching the radio sync-run detail cap (runErrorDetailLimit). Some
// CaptureException sites send err.Error() verbatim, and those errors can embed a
// provider's raw HTTP response body — without a cap one capture can exfiltrate an
// unbounded blob. Rune-based so a multi-byte sequence is never split (PSY-1145).
const sentryValueLimit = 2000

const (
	redactionMarker  = "[REDACTED]"
	truncationMarker = "…[truncated]"
)

var (
	// scheme://user:pass@host → strip URL userinfo (DB/service connection-string
	// passwords ride here, e.g. postgres://user:pw@host/db in a dial error).
	// The class allows '@' (but not /?#␠) so a password with a literal '@'
	// (redis://u:p@ss@host) backtracks to the LAST '@' before the path.
	urlUserinfoRe = regexp.MustCompile(`([a-zA-Z][\w+.\-]*://)[^/\s?#]+@`)
	// scheme://host/path?query → redact the whole query string, the usual token
	// carrier (magic-link ?token=, digest unsubscribe ?sig=, provider ?api_key=).
	urlQueryRe = regexp.MustCompile(`([a-zA-Z][\w+.\-]*://[^\s?]+)\?\S*`)
	// Free-text "key: value" / "key=value" / JSON "key":"value" for known-secret
	// keys (api_key=…, token=…, "Authorization: Basic …", {"access_token":"…"}).
	// The optional quotes around the key/value catch the JSON form (provider error
	// bodies are JSON), and the optional auth-scheme WORD ([a-zA-Z]+) means
	// "Authorization: <scheme> <jwt>" redacts the jwt for ANY scheme (Bearer,
	// Basic, Negotiate, …), not just a fixed list. The value class stops at a
	// quote/comma/brace so it doesn't eat the rest of a JSON object. Over-
	// redaction (e.g. "password: required" → "password=[REDACTED]") is the safe
	// direction; "code" is excluded (too collidey with "error code: 500"). Keys
	// are \b-anchored so a substring won't trigger.
	sensitiveKVRe = regexp.MustCompile(`(?i)(\b(?:authorization|api[_-]?key|apikey|access[_-]?token|refresh[_-]?token|client[_-]?secret|token|secret|password|passwd|sig)\b)"?\s*[:=]\s*"?(?:[a-zA-Z]+\s+)?[^\s",}]+`)
	// "Bearer <token>" anywhere in free text. \b so "forbearer foo" isn't matched.
	bearerRe = regexp.MustCompile(`(?i)(\bbearer\s+)\S+`)
)

// ScrubSentryEvent is the Sentry BeforeSend hook (PSY-1145): on every outgoing
// event it caps oversized values and strips obvious secrets from the message,
// exception values, and request (URL/query/cookies/body/sensitive headers), so
// no individual CaptureException site can leak an unbounded body or a
// token-bearing URL. Registered in cmd/server/main.go's sentry.Init.
//
// NOT scrubbed (deliberate scope): event.Extra / Breadcrumbs / Contexts /
// Threads / stacktrace frames. Today those carry only sanitized values (the
// per-request scope sets a hashed email + method/path, never the query string —
// internal/api/middleware/sentry.go; the few SetExtra sites pass non-secrets). A
// future SetExtra("payload", <secret>) would bypass this hook — scrub at that
// call site, or extend this hook, if that changes.
func ScrubSentryEvent(event *sentry.Event, _ *sentry.EventHint) *sentry.Event {
	if event == nil {
		return nil
	}
	event.Message = scrubText(event.Message)
	for i := range event.Exception {
		event.Exception[i].Value = scrubText(event.Exception[i].Value)
	}
	if event.Request != nil {
		scrubRequest(event.Request)
	}
	return event
}

// scrubText redacts secret patterns THEN caps length — redact first so a secret
// can't survive by sitting just past the length boundary.
func scrubText(s string) string {
	if s == "" {
		return s
	}
	return capRunes(redactSecrets(s), sentryValueLimit)
}

func redactSecrets(s string) string {
	s = urlUserinfoRe.ReplaceAllString(s, `${1}`+redactionMarker+`@`)
	s = urlQueryRe.ReplaceAllString(s, `${1}?`+redactionMarker)
	s = sensitiveKVRe.ReplaceAllString(s, `${1}=`+redactionMarker)
	s = bearerRe.ReplaceAllString(s, `${1}`+redactionMarker)
	return s
}

func capRunes(s string, limit int) string {
	if utf8.RuneCountInString(s) <= limit {
		return s
	}
	return string([]rune(s)[:limit]) + truncationMarker
}

// scrubRequest strips the secret-bearing parts Sentry's request integration
// captures. The sentryhttp middleware's Scope.SetRequest buffers the request
// BODY (≤10KB) and applyToEvent writes it to event.Request.Data UNCONDITIONALLY
// — it is NOT gated by SendDefaultPII (sentry-go scope.go) — so login/register/
// password-change bodies (plaintext passwords) would otherwise ship to Sentry on
// any error or panic during such a request. v0.42.0 has no option to disable the
// body buffer, so redacting Data here in BeforeSend is the only chokepoint. URL
// query, cookies (auth_token), and sensitive headers ride the same request and
// are stripped too.
func scrubRequest(r *sentry.Request) {
	if i := strings.IndexByte(r.URL, '?'); i >= 0 {
		r.URL = r.URL[:i] + "?" + redactionMarker
	}
	if r.QueryString != "" {
		r.QueryString = redactionMarker
	}
	if r.Cookies != "" {
		r.Cookies = redactionMarker
	}
	if r.Data != "" {
		r.Data = redactionMarker
	}
	for k := range r.Headers {
		if isSensitiveHeader(k) {
			r.Headers[k] = redactionMarker
		}
	}
}

func isSensitiveHeader(key string) bool {
	k := strings.ToLower(key)
	switch k {
	case "authorization", "proxy-authorization", "cookie", "set-cookie", "x-api-key", "x-auth-token":
		return true
	}
	return strings.Contains(k, "token") || strings.Contains(k, "secret") ||
		strings.Contains(k, "password") || strings.Contains(k, "apikey")
}
