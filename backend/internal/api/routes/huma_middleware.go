package routes

import (
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humachi"
)

// humaFromHTTP adapts a standard net/http middleware into a Huma middleware
// (PSY-1598).
//
// # Why this exists
//
// Rate-limited endpoints used to live on their own humachi.New instance so
// they could sit inside a chi.Group carrying the limiter. That works for
// routing, but each humachi.New owns a SEPARATE OpenAPI document, so those
// operations never appeared in the published spec — 27 of them, including the
// entire auth surface (PSY-1554).
//
// Registering them on the main API instead puts them in the one document. The
// limiter then has to travel with the operations rather than with a chi group,
// and Huma middleware has a different shape than net/http middleware:
//
//	net/http: func(http.Handler) http.Handler
//	huma:     func(huma.Context, func(huma.Context))
//
// humachi.Unwrap recovers the underlying request/response from a huma.Context,
// which is what makes the bridge possible without reimplementing the limiter.
// That matters: the existing limiter carries real behaviour — the API-token
// bypass, the JSON 429 body, the layered-ordering constraint from PSY-1397 —
// and a reimplementation would be a second thing to keep correct.
//
// # How the short-circuit is detected
//
// A net/http middleware signals "allowed" by invoking the handler it wraps, and
// "rejected" by writing a response and NOT invoking it. There is no return
// value to inspect. So we pass a sentinel handler that only records whether it
// ran, and use that to decide whether to continue the Huma chain.
//
// If the middleware rejected the request it has already written its own
// response (the 429 body and Retry-After header) to the ResponseWriter, so
// returning without calling next is correct and complete — Huma must not also
// write a response.
//
// # How this fails, and why "expect not-429" tests miss it
//
// If the sentinel detection ever breaks, this returns WITHOUT calling next. Huma
// never runs the operation and emits a bare 200 with an EMPTY BODY. It does not
// error, it does not 404 — it looks like a successful request that silently did
// nothing, which is the worst shape a failure can take on an auth endpoint.
//
// Every assertion of the form "the response was not 429" passes against that. So
// a test for a BYPASS or a no-op limiter (PSY-475) is not evidence on its own:
// it has to prove the handler actually ran. See assertReachedHandler in
// subapi_auth_test.go, which checks for the empty-200 signature; it was added
// after a negative control showed the naive version passing against a
// deliberately broken bridge.
func humaFromHTTP(mw func(http.Handler) http.Handler) func(huma.Context, func(huma.Context)) {
	return func(ctx huma.Context, next func(huma.Context)) {
		r, w := humachi.Unwrap(ctx)

		allowed := false
		sentinel := http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
			allowed = true
		})

		mw(sentinel).ServeHTTP(w, r)

		if allowed {
			next(ctx)
		}
	}
}
