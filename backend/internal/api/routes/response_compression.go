package routes

import (
	"net/http"

	chimw "github.com/go-chi/chi/v5/middleware"
)

// compressionLevel is the gzip level applied to compressible responses.
//
// 5 is chi's documented "sensible value" and the knee of the curve for JSON:
// past it, each extra level buys single-digit percentages of size for
// super-linear CPU, and this runs on a shared Railway container that also serves
// every other request. Below it, the payloads this exists for stay needlessly
// large. Kept a named constant so the tradeoff is one edit, not a magic number.
const compressionLevel = 5

// ResponseCompression negotiates gzip for compressible response bodies.
//
// WHY THIS IS NEEDED AT THE BACKEND, not the proxy
// ------------------------------------------------
// The frontend has a Vercel `/api` proxy that DOES gzip what it forwards
// (measured), but production browsers do not go through it: `lib/api-base.ts`
// resolves API_BASE_URL to the absolute backend origin outside development, and
// `next.config.ts` whitelists that origin in the CSP `connect-src` precisely
// because the fetches are cross-origin. So the proxy's compression covers local
// development and nothing else — every production byte is served straight from
// here, and before this middleware existed it was served as identity encoding no
// matter what the client negotiated.
//
// SCOPE IS GLOBAL, deliberately. The endpoint that motivated this (the nightly
// graph overview) is not special; it is just the first response big enough to
// make the gap visible. Every JSON read on this API travels the same
// uncompressed path, so scoping the fix to one route would leave the same defect
// everywhere else and add a per-route config surface to maintain. Content-type
// negotiation already keeps the cost off responses that cannot benefit.
//
// WHAT GETS COMPRESSED is chi's default compressible set — notably
// `application/json` (every Huma read) and `application/atom+xml` (the follows
// activity feed). Deliberately NOT enumerated here: an explicit list would be a
// second place to forget a content type. `text/calendar` (the ICS feeds) is
// outside that set and stays identity-encoded, which is what keeps their STRONG
// ETags valid — see the weakening note in the graph overview reader for why a
// compressed representation cannot keep a strong validator.
//
// MOUNTING ORDER: this must sit INSIDE the rate limiters (registered after them
// in cmd/server/main.go). A 429 body is a few dozen bytes, so compressing it
// spends CPU and a `Vary` header to make the response bigger; keeping the
// limiters outside means a rejected request never reaches the compressor at all.
// It does not participate in the layered-limiter counting order, which is a
// limiter-to-limiter concern.
func ResponseCompression() func(http.Handler) http.Handler {
	compress := chimw.Compress(compressionLevel)
	return func(next http.Handler) http.Handler {
		return stripEncodingFromBodylessResponses(compress(next))
	}
}

// stripEncodingFromBodylessResponses keeps responses that are defined to carry
// no body free of both a content-coding header and the compressor's stream
// framing.
//
// chi's compressor decides purely on Content-Type and never looks at the status
// code. Huma stamps `application/json` before it knows the handler will answer
// 304, so on the revalidation path the compressor concludes the response is
// compressible and does two things it should not: it advertises
// `Content-Encoding: gzip`, and — because a deferred `Close()` flushes the
// encoder whether or not anything was written — it emits an empty gzip stream's
// header and trailer as the 304's body. Measured at 23 bytes (see the
// TestResponseCompressionPreservesNotModified fixture, which fails on both counts
// without this wrapper).
//
// Neither is caught by anything downstream that we should be relying on. Go's
// server does refuse body bytes on a bodyless status (`bodyAllowedForStatus`), so
// the 23 bytes do not reach a real client — but leaning on that means the
// middleware is only correct because a lower layer cleans up after it, and it
// leaves the bogus header untouched regardless. RFC 9110 §15.4.5 limits a 304 to
// the metadata a cache needs to update a stored response; a content-coding for
// content that was never sent is not in that set, and a cache that believes it
// holds a gzip representation of a resource it holds only headers for is a very
// hard bug to find later. The overview endpoint is precisely the one whose repeat
// traffic is almost entirely 304s, so this is its common path, not its edge case.
//
// `Vary: Accept-Encoding` is deliberately LEFT IN PLACE. It describes how the
// resource is SELECTED, not how this particular response was encoded, and a
// shared cache needs it to keep the encoded and identity variants apart.
// Stripping it would be the actual cache-poisoning bug.
//
// This wraps AROUND chi's compressor, and the direction is load-bearing. Header()
// is one shared map all the way down while WriteHeader propagates OUTWARD, so
// chi's writer sets Content-Encoding and then calls through to this one — which
// removes it before the real ResponseWriter commits the headers. Nested the other
// way the deletion would run first and chi would re-add the header afterwards,
// leaving a silently useless middleware.
func stripEncodingFromBodylessResponses(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(&bodylessEncodingStripper{ResponseWriter: w}, r)
	})
}

type bodylessEncodingStripper struct {
	http.ResponseWriter
	wroteHeader bool
	bodyless    bool
}

func (w *bodylessEncodingStripper) WriteHeader(code int) {
	if !w.wroteHeader {
		w.wroteHeader = true
		// The two statuses this API actually returns without content. 1xx never
		// reaches here (Go handles informational responses on its own path), and
		// HEAD is handled by the server suppressing the body while still needing
		// the encoding header to describe what a GET would have returned.
		w.bodyless = code == http.StatusNotModified || code == http.StatusNoContent
		if w.bodyless {
			w.Header().Del("Content-Encoding")
		}
	}
	w.ResponseWriter.WriteHeader(code)
}

// Write drops anything written under a bodyless status. In practice the only
// caller is the compressor flushing an empty stream's framing on Close, which is
// transport artefact rather than content. It reports the bytes as accepted rather
// than erroring because the writer it lies to is that same deferred Close, whose
// error is discarded — an error would change nothing except to make the failure
// mode harder to read.
func (w *bodylessEncodingStripper) Write(p []byte) (int, error) {
	if w.bodyless {
		return len(p), nil
	}
	return w.ResponseWriter.Write(p)
}

// Unwrap keeps the wrapper transparent to http.ResponseController and to chi's
// own writer introspection, so nothing downstream loses Flush/Hijack.
func (w *bodylessEncodingStripper) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}
