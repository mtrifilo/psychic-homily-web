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

// secretKeyPattern matches a known-secret key NAME. The first alternation group —
// [\w-]*(?:token|secret|password|passwd) — carries a leading [\w-]* segment so
// COMPOUND keys match too: auth_token (this app's own session-cookie name),
// session_token, csrf_token, access_token, refresh_token, client_secret,
// new_password. That prefix is needed because a bare \btoken\b boundary does NOT
// fall inside auth_token ('_' is a word char, so there's no boundary before
// "token"). "code" is deliberately absent (too collidey with "error code: 500").
// Keys are \b-anchored so a substring won't trigger.
//
// To ADD a key: a name that also appears as <prefix>_NAME goes in the first
// group (so the leading [\w-]* catches the prefix); a name only ever seen bare
// goes in the second, fixed group (api_key, authorization, …). Don't add a
// collidey common word (see "code").
const secretKeyPattern = `\b(?:[\w-]*(?:token|secret|password|passwd)|api[_-]?key|apikey|authorization|signature|sig|cookie|session|csrf|jwt)\b`

var (
	// scheme://user:pass@host → strip URL userinfo (DB/service connection-string
	// passwords ride here, e.g. postgres://user:pw@host/db in a dial error).
	// The class allows '@' (but not /?#␠) so a password with a literal '@'
	// (redis://u:p@ss@host) backtracks to the LAST '@' before the path.
	urlUserinfoRe = regexp.MustCompile(`([a-zA-Z][\w+.\-]*://)[^/\s?#]+@`)
	// scheme://host/path?query → redact the whole query string, the usual token
	// carrier (magic-link ?token=, digest unsubscribe ?sig=, provider ?api_key=).
	// NOTE: a secret in a URL PATH segment (e.g. the Discord webhook token,
	// https://discord.com/api/webhooks/{id}/{secret}) is NOT caught here — those
	// call sites must redact at the source (utils.RedactErrorURL); see the
	// best-effort caveat on ScrubSentryEvent.
	urlQueryRe = regexp.MustCompile(`([a-zA-Z][\w+.\-]*://[^\s?]+)\?\S*`)
	// known-secret key followed by a QUOTED value — the JSON form (provider error
	// bodies are JSON, a login body echoed into an error): {"access_token":"…"},
	// password: "…". The value class (?:[^"\\]|\\.)* spans the whole quoted value
	// INCLUDING spaces AND escaped quotes (\"), so a multi-word passphrase
	// ("correct horse battery staple", or one containing a quote) is redacted in
	// full, not just up to the first quote. An optional quote on either side of
	// the key is consumed so the surrounding JSON stays readable.
	sensitiveQuotedRe = regexp.MustCompile(`(?i)"?(` + secretKeyPattern + `)"?\s*[:=]\s*"(?:[^"\\]|\\.)*"`)
	// known-secret key followed by an UNQUOTED value — free text (api_key=…,
	// token=…) and auth headers. The optional auth-scheme WORD ([a-zA-Z]+\s+)
	// means "Authorization: <scheme> <cred>" redacts the credential for ANY scheme
	// (Bearer/Basic/Negotiate/…), not just a fixed list. The value class stops at
	// whitespace/quote/comma/brace so it doesn't eat the rest of a JSON object or a
	// following sentence — so an unquoted MULTI-WORD value redacts only the scheme
	// word + one value token; any further words trail UNREDACTED (the disclosed
	// limit on ScrubSentryEvent). Acceptable: real tokens/keys/JWTs have no spaces;
	// a multi-word passphrase arrives either quoted (caught above) or in the
	// request body (Data, fully redacted). Over-redaction ("password: required" →
	// "password=[REDACTED]") is the safe direction.
	sensitiveUnquotedRe = regexp.MustCompile(`(?i)(` + secretKeyPattern + `)"?\s*[:=]\s*(?:[a-zA-Z]+\s+)?[^\s",}]+`)
	// "Bearer <token>" anywhere in free text. \b so "forbearer foo" isn't matched.
	bearerRe = regexp.MustCompile(`(?i)(\bbearer\s+)\S+`)
)

// ScrubSentryEvent is the Sentry BeforeSend hook (PSY-1145): on every outgoing
// event it caps oversized values and strips obvious secrets from the message,
// exception values, and request (URL/query/cookies/body/sensitive headers), so
// no individual CaptureException site can leak an unbounded body or a
// token-bearing URL. Registered in cmd/server/main.go's sentry.Init.
//
// This is a BEST-EFFORT secondary net for common secret SHAPES (token-bearing
// URL userinfo/query, Bearer, key=value / JSON "key":"value" secrets, the
// request body, auth cookies/headers) — NOT a guarantee every secret is caught.
// It does NOT redact: a secret in a URL PATH segment, an unstructured token with
// no recognizable key prefix, or the trailing words of an unquoted multi-word
// value. Call sites that can emit those — notably the Discord webhook URL, whose
// token rides in the path — must still redact at the source (utils.RedactErrorURL)
// and not rely on this hook.
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
	// Quoted form first: it consumes the whole "key":"value" (spaces and all) so
	// the unquoted pass can't grab only the value's first token and leave the rest.
	s = sensitiveQuotedRe.ReplaceAllString(s, `${1}=`+redactionMarker)
	s = sensitiveUnquotedRe.ReplaceAllString(s, `${1}=`+redactionMarker)
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
// body buffer, so redacting Data here in BeforeSend is the only chokepoint.
//
// The Cookies/Headers redaction below is, by contrast, DEFENSE-IN-DEPTH: under
// the pinned SendDefaultPII:false, sentry-go's NewRequest already leaves Cookies
// empty and filters its own sensitive-header allowlist (Authorization, Cookie,
// X-Api-Key, …) BEFORE BeforeSend runs. The loop still earns its place for (a) a
// future SendDefaultPII:true and (b) header names sentry doesn't cover
// (x-auth-token, the fuzzy *token*/*secret*/*password*/*apikey* matches). Don't
// "simplify" the SendDefaultPII:false setting away — it's what keeps Cookies and
// those headers empty in the first place. URL query rides the same request and is
// stripped too.
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
