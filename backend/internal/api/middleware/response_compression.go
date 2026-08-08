package middleware

import (
	"bufio"
	"errors"
	"net"
	"net/http"

	chimw "github.com/go-chi/chi/v5/middleware"
)

// compressionLevel is the gzip level applied to compressible responses.
//
// 5 is chi's documented "sensible value", and past it the size curve flattens
// while CPU keeps climbing — level 6 measured marginally WORSE than 5 on the
// overview payload.
//
// THE REAL COST OF ANY LEVEL ABOVE 1 IS A FIXED PER-REQUEST ONE, not compression
// throughput, and it is worth writing down because it is invisible from here.
// chi's `selectEncoder` runs at the top of EVERY request that sends
// `Accept-Encoding: gzip` — which is every browser, and Go's own http.Client —
// and calls `Reset` on a pooled writer before it knows the content type or the
// status. For flate levels 2-9 `(*compressor).reset` clears `hashHead` and
// `hashPrev`: 512 KB + 128 KB = 640 KB memset, benchmarked at ~21 µs. Level 1
// takes flate's BestSpeed branch, which resets an offset counter and touches
// nothing. So a 304 costs ~22 µs at level 5 against ~1 µs at level 1, and the
// overview's repeat traffic is almost entirely 304s.
//
// Level 5 is still the right call, and the tradeoff is a byte-vs-CPU one rather
// than a free lunch: level 1 would cost ~15% more bytes on the payload this
// exists for (measured 223,590 vs 190,673) and 2-9% on ordinary list responses.
// Those bytes are on a public, anonymous, mobile-first surface where they are
// user-perceived latency, while ~21 µs of server CPU is not — it is roughly 0.2%
// of one core at 100 req/s. REVISIT THIS if CPU on the shared Railway container
// ever becomes the binding constraint; level 1 is the lever, and level 2 does not
// help because only level 1 escapes the memset.
//
// Fixed rather than configurable on purpose: an env knob for a compression level
// is mostly a way to accidentally ship level 9 to a shared container.
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
// everywhere else and add a per-route config surface to maintain.
//
// WHAT GETS COMPRESSED is chi's default compressible set — notably
// `application/json` (every Huma read) and `application/atom+xml` (the follows
// activity feed). Deliberately NOT enumerated here: an explicit list would be a
// second place to forget a content type. The set is a floor rather than a
// perfect fit — `text/calendar` (the ICS feeds) and `text/markdown` (the show
// export) are both outside it and would compress well — but both are
// low-traffic attachment downloads, and widening the set is a separate decision
// from establishing that compression happens at all.
//
// NO `ENABLE_*` FLAG, which is a deviation from the rollout convention its
// neighbours in the mounting chain follow (see routes.PublicReadRateLimiter) and
// therefore a deliberate choice rather than an oversight. Those flags exist
// because a limiter can REJECT a request, so it needs to be observed on stage
// before it can 429 a real user. Compression cannot reject anything: it changes
// the encoding of a body every HTTP client has been able to decode for twenty
// years, and its failure mode is CPU, which is a dashboard reading rather than a
// user-visible break. A flag would add a permanently-on config knob and a second
// code path to keep tested.
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
// code. Huma stamps `application/json` unconditionally before it knows the
// handler will answer 304, so on the revalidation path the compressor concludes
// the response is compressible and does two things it should not: it advertises
// `Content-Encoding: gzip`, and — because a deferred `Close()` flushes the
// encoder whether or not anything was written — it emits an empty gzip stream's
// header and trailer as the 304's body. Measured at 23 bytes; the
// TestResponseCompressionNotModified fixture fails on both counts without this
// wrapper, and the header was reproduced against a real net/http server.
//
// Neither is caught by anything downstream that we should be relying on. Go's
// server does refuse body bytes on a bodyless status (`bodyAllowedForStatus`), so
// the 23 bytes do not reach a real client — but leaning on that means the
// middleware is only correct because a lower layer cleans up after it, and it
// leaves the bogus header untouched regardless. RFC 9110 §15.4.5 limits a 304 to
// the metadata a cache needs to update a stored response; a content-coding for
// content that was never sent is not in that set, and a cache that believes it
// holds a gzip representation of a resource it holds only headers for is a very
// hard bug to find later. The graph overview is precisely the endpoint whose
// repeat traffic is almost entirely 304s, so this is its common path.
//
// The fix belongs upstream in chi, whose compressor should consult the status
// code. DELETE THIS WRAPPER if it ever does.
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

// bodylessEncodingStripper is a generic "a bodyless status must not carry a
// content-coding" writer. It is not compression-specific beyond being the thing
// that cleans up after a compressor.
type bodylessEncodingStripper struct {
	http.ResponseWriter
	// status is the committed status code, or 0 for "not written yet" — no valid
	// HTTP status is 0, so the one field carries both facts.
	status int
}

func (w *bodylessEncodingStripper) WriteHeader(code int) {
	// Only the FIRST call decides. chi's compressor deliberately propagates
	// repeat WriteHeader calls, and without this guard a handler that wrote 200
	// and then called WriteHeader(304) would flip this writer into discarding a
	// body it had already committed.
	if w.status == 0 {
		w.status = code
		if w.isBodyless() {
			w.Header().Del("Content-Encoding")
		}
	}
	w.ResponseWriter.WriteHeader(code)
}

// isBodyless reports whether the committed status is defined to carry no
// content. 1xx never reaches here (Go handles informational responses on its own
// path), and HEAD is deliberately absent: the server suppresses a HEAD body
// itself while the encoding header still has to describe what a GET would have
// returned.
func (w *bodylessEncodingStripper) isBodyless() bool {
	return w.status == http.StatusNotModified || w.status == http.StatusNoContent
}

// Write drops anything written under a bodyless status. In practice the only
// caller is the compressor flushing an empty stream's framing on Close, which is
// transport artefact rather than content. It reports the bytes as accepted rather
// than erroring because the writer it lies to is that same deferred Close, whose
// error is discarded — an error would change nothing except to make the failure
// mode harder to read.
func (w *bodylessEncodingStripper) Write(p []byte) (int, error) {
	if w.isBodyless() {
		return len(p), nil
	}
	return w.ResponseWriter.Write(p)
}

// Flush, Hijack and Push forward the optional ResponseWriter interfaces.
//
// These are NOT redundant with Unwrap. chi's compressor reaches for them by
// DIRECT TYPE ASSERTION (`cw.ResponseWriter.(http.Flusher)`), not through
// http.ResponseController, so a wrapper that embeds only the http.ResponseWriter
// interface silently removes capabilities the real writer had: on the identity
// path chi would find no Flusher and no Hijacker at all. Nothing in this API
// streams or hijacks today, so the loss would be latent — which is exactly the
// kind of defect that surfaces months later as "SSE doesn't flush" with no
// obvious connection to a compression change. A wrapper should be transparent to
// everything it does not deliberately change.
func (w *bodylessEncodingStripper) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (w *bodylessEncodingStripper) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if hj, ok := w.ResponseWriter.(http.Hijacker); ok {
		return hj.Hijack()
	}
	return nil, nil, errors.New("middleware: http.Hijacker is unavailable on the writer")
}

func (w *bodylessEncodingStripper) Push(target string, opts *http.PushOptions) error {
	if p, ok := w.ResponseWriter.(http.Pusher); ok {
		return p.Push(target, opts)
	}
	return errors.New("middleware: http.Pusher is unavailable on the writer")
}

// Unwrap keeps the wrapper transparent to http.ResponseController for any
// capability not forwarded explicitly above.
func (w *bodylessEncodingStripper) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}
