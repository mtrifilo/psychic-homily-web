// Package middleware_test is an EXTERNAL test package, unlike every other test
// file in internal/api/middleware. It has to be: the compression tests mount the
// real graph overview handler so the assertions cover the actual Huma → chi
// interaction (notably what Huma leaves in the header map on a 304) rather than a
// hand-rolled stand-in for it, and internal/api/handlers/catalog imports this
// package. An in-package test would be an import cycle.
package middleware_test

import (
	"compress/gzip"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humachi"
	"github.com/go-chi/chi/v5"

	catalogh "psychic-homily-backend/internal/api/handlers/catalog"
	"psychic-homily-backend/internal/api/handlers/shared/testhelpers"
	"psychic-homily-backend/internal/api/middleware"
	"psychic-homily-backend/internal/services/contracts"
)

const testOverviewETag = `W/"abc123"`

// productionScaleOverview builds a payload at roughly production scale (PR #1807
// measured ~5k artists / 20k relationships) so the compression ratio this test
// prints is comparable to the figure on the ticket rather than to a toy body.
//
// The scale and the per-field arithmetic both matter: uniform columns would
// compress far better than real data and overstate the win.
func productionScaleOverview() *contracts.GraphOverview {
	const nodeCount, edgeCount = 5000, 20000

	nodes := contracts.GraphOverviewNodes{
		ID:   make([]uint, nodeCount),
		Kind: make([]uint8, nodeCount),
		Name: make([]string, nodeCount),
		Slug: make([]string, nodeCount),
		// The hub caption columns are full-length and overwhelmingly "" — one
		// short value per LOCATED label hub, nothing at any artist index. They
		// are modelled here rather than omitted because this fixture is the
		// instrument the payload-budget figure is quoted from (PSY-1792), and a
		// fixture missing three of the shipped columns understates the body.
		HubCity:    make([]string, nodeCount),
		HubState:   make([]string, nodeCount),
		HubCountry: make([]string, nodeCount),
		X:          make([]int16, nodeCount),
		Y:          make([]int16, nodeCount),
		Community:  make([]int32, nodeCount),
		Degree:     make([]int32, nodeCount),
		Rank:       make([]int32, nodeCount),
		Flags:      make([]uint8, nodeCount),
		Appear:     make([]int32, nodeCount),
	}
	// Label hubs are a small minority of the node set, and only some have each
	// part on file — the shape that decides what these columns actually cost.
	hubCities := []string{"Phoenix", "Tempe", "Brooklyn", "Austin", "London", ""}
	hubStates := []string{"AZ", "AZ", "NY", "TX", "", ""}
	hubCountries := []string{"US", "US", "US", "USA", "England", ""}
	for i := 0; i < nodeCount; i++ {
		nodes.ID[i] = uint(i + 1)
		nodes.Kind[i] = uint8(i % 2)
		nodes.Name[i] = fmt.Sprintf("Test Artist Number %d", i)
		nodes.Slug[i] = fmt.Sprintf("test-artist-number-%d", i)
		nodes.X[i] = int16(i*7%32767 - 16383)
		nodes.Y[i] = int16(i*13%32767 - 16383)
		nodes.Community[i] = int32(i % 40)
		nodes.Degree[i] = int32(i % 25)
		nodes.Rank[i] = int32(i)
		nodes.Flags[i] = uint8(i % 4)
		nodes.Appear[i] = int32(i * 1000)
		// Roughly 1 node in 12 is a hub; every other index stays "".
		if i%12 == 0 {
			nodes.HubCity[i] = hubCities[(i/12)%len(hubCities)]
			nodes.HubState[i] = hubStates[(i/12)%len(hubStates)]
			nodes.HubCountry[i] = hubCountries[(i/12)%len(hubCountries)]
		}
	}

	slots := edgeCount * 2
	edges := contracts.GraphOverviewEdges{
		Offsets: make([]int32, nodeCount+1),
		Targets: make([]int32, slots),
		Kind:    make([]uint8, slots),
		Appear:  make([]int32, slots),
	}
	for i := 0; i <= nodeCount; i++ {
		edges.Offsets[i] = int32(i * slots / (nodeCount + 1))
	}
	for i := 0; i < slots; i++ {
		edges.Targets[i] = int32(i * 31 % nodeCount)
		edges.Kind[i] = uint8(i % 2)
		edges.Appear[i] = int32(i * 500)
	}

	regions := make([]contracts.GraphOverviewRegion, 40)
	for i := range regions {
		hull := make([][2]int16, 12)
		for j := range hull {
			hull[j] = [2]int16{int16(i*100 + j), int16(i*200 - j)}
		}
		regions[i] = contracts.GraphOverviewRegion{
			Community:   int32(i),
			Label:       fmt.Sprintf("Around Test Artist Number %d", i),
			MemberCount: nodeCount / 40,
			Hull:        hull,
		}
	}

	return &contracts.GraphOverview{
		Version:      contracts.GraphOverviewVersion,
		LastMapped:   time.Now().UTC(),
		Epoch:        time.Now().UTC().Add(-10000 * time.Hour),
		Extent:       1000,
		NodeCount:    nodeCount,
		EdgeCount:    edgeCount,
		IsolateCount: 120,
		RankMetric:   contracts.GraphOverviewRankBetweenness,
		HullKind:     contracts.GraphOverviewHullConvex,
		Nodes:        nodes,
		Edges:        edges,
		Regions:      regions,
	}
}

// newCompressedOverviewRouter mounts the REAL overview handler behind the REAL
// compression middleware.
func newCompressedOverviewRouter(t *testing.T, overview *contracts.GraphOverview) *chi.Mux {
	t.Helper()

	svc := &testhelpers.MockGraphOverviewService{
		GetGraphOverviewFn: func() (*contracts.GraphOverview, string, error) {
			return overview, testOverviewETag, nil
		},
	}

	router := chi.NewRouter()
	router.Use(middleware.ResponseCompression())
	api := humachi.New(router, huma.DefaultConfig("test", "1.0.0"))
	handler := catalogh.NewGraphOverviewHandler(svc)
	huma.Get(api, "/graph/overview", handler.GetGraphOverviewHandler)
	return router
}

// The whole point of the ticket: a client that negotiates gzip must get gzip.
// Before this middleware the backend answered identity no matter what, because
// production browsers bypass the frontend's compressing `/api` proxy.
func TestResponseCompressionGzipsOverviewJSON(t *testing.T) {
	router := newCompressedOverviewRouter(t, productionScaleOverview())

	identity := httptest.NewRecorder()
	router.ServeHTTP(identity, httptest.NewRequest(http.MethodGet, "/graph/overview", nil))
	if identity.Code != http.StatusOK {
		t.Fatalf("identity request: status = %d, want 200", identity.Code)
	}
	if enc := identity.Header().Get("Content-Encoding"); enc != "" {
		t.Errorf("identity request: Content-Encoding = %q, want empty — a client that did not "+
			"negotiate an encoding must never be sent one", enc)
	}

	req := httptest.NewRequest(http.MethodGet, "/graph/overview", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	compressed := httptest.NewRecorder()
	router.ServeHTTP(compressed, req)

	if compressed.Code != http.StatusOK {
		t.Fatalf("gzip request: status = %d, want 200", compressed.Code)
	}
	if enc := compressed.Header().Get("Content-Encoding"); enc != "gzip" {
		t.Fatalf("gzip request: Content-Encoding = %q, want gzip", enc)
	}
	if vary := compressed.Header().Get("Vary"); !strings.Contains(vary, "Accept-Encoding") {
		t.Errorf("gzip request: Vary = %q, must contain Accept-Encoding or a shared cache can "+
			"serve a gzip body to a client that cannot decode it", vary)
	}
	// chi must clear any pre-compression Content-Length, since it describes the
	// wrong body. This asserts the HEADER MAP, not the wire: a real net/http
	// server re-derives a correct Content-Length from the buffered compressed
	// body, so production responses are not chunked because of this.
	if cl := compressed.Header().Get("Content-Length"); cl != "" {
		t.Errorf("gzip request: Content-Length = %q, want it cleared — the pre-compression "+
			"length describes the wrong body", cl)
	}

	// Sizes are captured BEFORE anything reads the recorders: Body is a
	// bytes.Buffer, so Len() reports what is still UNREAD, and decompressing
	// first silently turns the compressed size into 0.
	rawBytes, gzBytes := identity.Body.Len(), compressed.Body.Len()

	// The compressed body must decode back to exactly the identity body: a
	// transport optimisation that changes the payload is a data bug.
	zr, err := gzip.NewReader(compressed.Body)
	if err != nil {
		t.Fatalf("gzip request: body is not a valid gzip stream: %v", err)
	}
	decoded, err := io.ReadAll(zr)
	if err != nil {
		t.Fatalf("gzip request: decompressing body: %v", err)
	}
	if err := zr.Close(); err != nil {
		t.Fatalf("gzip request: closing gzip reader (truncated stream?): %v", err)
	}
	if string(decoded) != identity.Body.String() {
		t.Errorf("decompressed body differs from the identity body (%d vs %d bytes)",
			len(decoded), rawBytes)
	}

	// Not an assertion on an exact ratio — that would be a brittle test of gzip's
	// tuning. The floor only has to prove the middleware is doing real work, and
	// the log line is the number quoted on the PR.
	t.Logf("overview payload: identity %d bytes, gzip %d bytes (%.1f%% of identity, %.1fx)",
		rawBytes, gzBytes, 100*float64(gzBytes)/float64(rawBytes), float64(rawBytes)/float64(gzBytes))
	if gzBytes >= rawBytes/2 {
		t.Errorf("gzip body is %d bytes against %d identity — expected at least 2x on a payload "+
			"this repetitive; the compressor is probably not being applied", gzBytes, rawBytes)
	}
}

// A 304 carries no body, so it must not claim a content-coding for one. chi's
// compressor decides on Content-Type alone and never inspects the status, so
// without the bodyless-status guard this response advertises gzip over an empty
// gzip stream's 23 bytes of framing.
//
// Every validator FORM is covered in one place because they must all reach the
// same response: If-None-Match is compared WEAKLY, so our own weak tag, a copy a
// cache has strengthened or weakened, a comma-separated candidate list, and the
// wildcard are all matches.
func TestResponseCompressionNotModified(t *testing.T) {
	// The 304 path never serialises the payload (Huma skips the body for 304), so
	// its size is irrelevant here — only that a snapshot exists.
	router := newCompressedOverviewRouter(t, &contracts.GraphOverview{})

	for _, ifNoneMatch := range []string{
		testOverviewETag,      // our own tag, echoed verbatim
		`"abc123"`,            // strengthened by an intermediary
		`"other", W/"abc123"`, // a candidate list
		"*",                   // the wildcard
	} {
		t.Run(ifNoneMatch, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/graph/overview", nil)
			req.Header.Set("Accept-Encoding", "gzip")
			req.Header.Set("If-None-Match", ifNoneMatch)
			rr := httptest.NewRecorder()
			router.ServeHTTP(rr, req)

			if rr.Code != http.StatusNotModified {
				t.Fatalf("status = %d, want 304 — compression must not break revalidation", rr.Code)
			}
			if body := rr.Body.Len(); body != 0 {
				t.Errorf("304 body = %d bytes, want 0 — the compressor's empty-stream framing "+
					"must not be emitted as content", body)
			}
			if enc := rr.Header().Get("Content-Encoding"); enc != "" {
				t.Errorf("304 Content-Encoding = %q, want empty — there is no content to have an "+
					"encoding, and RFC 9110 §15.4.5 limits a 304 to cache-update metadata", enc)
			}
			if got := rr.Header().Get("ETag"); got != testOverviewETag {
				t.Errorf("304 ETag = %q, want %q — the validator must survive the compressor "+
					"untouched, or the client can never revalidate again", got, testOverviewETag)
			}
			if vary := rr.Header().Get("Vary"); !strings.Contains(vary, "Accept-Encoding") {
				t.Errorf("304 Vary = %q, must still contain Accept-Encoding: it describes how the "+
					"resource is selected, which is what a cache updating its entry needs", vary)
			}
		})
	}
}

// A stale validator still has to deliver the new body, compressed — the 304 guard
// must not swallow a real response.
func TestResponseCompressionStaleValidatorSendsCompressedBody(t *testing.T) {
	router := newCompressedOverviewRouter(t, productionScaleOverview())

	req := httptest.NewRequest(http.MethodGet, "/graph/overview", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	req.Header.Set("If-None-Match", `W/"a-previous-snapshot"`)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 for a non-matching validator", rr.Code)
	}
	if enc := rr.Header().Get("Content-Encoding"); enc != "gzip" {
		t.Fatalf("Content-Encoding = %q, want gzip", enc)
	}
	if rr.Body.Len() == 0 {
		t.Error("body is empty — the bodyless guard must only apply to bodyless statuses")
	}
}

// Content-type negotiation decides what is compressed, and the POSITIVE control
// row is the point: without it, a change that disabled compression entirely would
// leave the two negative rows passing.
func TestResponseCompressionSelectsByContentType(t *testing.T) {
	for _, tc := range []struct {
		name        string
		contentType string
		status      int
		body        string
		wantEncoded bool
		why         string
	}{
		{
			name:        "json is compressed",
			contentType: "application/json",
			status:      http.StatusOK,
			body:        strings.Repeat(`{"k":"v"},`, 500),
			wantEncoded: true,
			why:         "application/json is in chi's default compressible set",
		},
		{
			name:        "calendar is left identity",
			contentType: "text/calendar; charset=utf-8",
			status:      http.StatusOK,
			body:        strings.Repeat("BEGIN:VEVENT\r\nEND:VEVENT\r\n", 500),
			wantEncoded: false,
			why: "text/calendar is outside chi's default set, so the ICS feeds keep going out " +
				"identity-encoded. Adding it later is a fine change to make — the feeds already " +
				"carry WEAK ETags, so nothing about their validators blocks it",
		},
		{
			name:        "204 carries no encoding even for a compressible type",
			contentType: "application/json",
			status:      http.StatusNoContent,
			body:        "",
			wantEncoded: false,
			why:         "a 204 is defined to carry no content, so there is nothing to encode",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			router := chi.NewRouter()
			router.Use(middleware.ResponseCompression())
			router.Get("/thing", func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", tc.contentType)
				w.WriteHeader(tc.status)
				if tc.body != "" {
					_, _ = w.Write([]byte(tc.body))
				}
			})

			req := httptest.NewRequest(http.MethodGet, "/thing", nil)
			req.Header.Set("Accept-Encoding", "gzip")
			rr := httptest.NewRecorder()
			router.ServeHTTP(rr, req)

			if rr.Code != tc.status {
				t.Fatalf("status = %d, want %d", rr.Code, tc.status)
			}
			enc := rr.Header().Get("Content-Encoding")
			if tc.wantEncoded && enc != "gzip" {
				t.Errorf("Content-Encoding = %q, want gzip: %s", enc, tc.why)
			}
			if !tc.wantEncoded && enc != "" {
				t.Errorf("Content-Encoding = %q, want empty: %s", enc, tc.why)
			}
		})
	}
}

// Compression must ADD to Vary, never replace it. The CORS middleware upstream
// sets `Vary: Origin` on every response, and a shared cache that lost it would
// start serving one origin's CORS headers to another. chi appends today; a bump
// that changed the append to a Set would break that silently and everywhere,
// which is worth one assertion to catch.
func TestResponseCompressionPreservesExistingVary(t *testing.T) {
	router := chi.NewRouter()
	router.Use(middleware.ResponseCompression())
	router.Get("/thing", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Vary", "Origin") // what the CORS middleware contributes
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(strings.Repeat(`{"k":"v"},`, 200)))
	})

	req := httptest.NewRequest(http.MethodGet, "/thing", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	vary := rr.Header().Values("Vary")
	joined := strings.Join(vary, ", ")
	if !strings.Contains(joined, "Origin") {
		t.Errorf("Vary = %q, lost the upstream Origin — a shared cache would serve one origin's "+
			"CORS headers to another", joined)
	}
	if !strings.Contains(joined, "Accept-Encoding") {
		t.Errorf("Vary = %q, missing Accept-Encoding", joined)
	}
}

// Hijack must still reach the real connection through the whole stack. This is
// the END-TO-END half of the capability check; the wrapper's own forwarding is
// unit-tested in response_compression_writer_test.go.
//
// The identity path is the one that matters: chi's Hijack delegates to
// `cw.writer()`, which on a non-compressible response IS the wrapper, so a
// wrapper that does not implement http.Hijacker turns a working hijack into an
// error. Asserting through a real server rather than a recorder because
// httptest.ResponseRecorder is not an http.Hijacker and would fail either way.
func TestResponseCompressionHijackStillReachesTheConnection(t *testing.T) {
	hijacked := make(chan error, 1)

	router := chi.NewRouter()
	router.Use(middleware.ResponseCompression())
	router.Get("/hijack", func(w http.ResponseWriter, _ *http.Request) {
		hj, ok := w.(http.Hijacker)
		if !ok {
			hijacked <- fmt.Errorf("response writer is not an http.Hijacker")
			return
		}
		conn, _, err := hj.Hijack()
		if err != nil {
			hijacked <- err
			return
		}
		_, _ = conn.Write([]byte("HTTP/1.1 200 OK\r\nContent-Length: 2\r\n\r\nhi"))
		_ = conn.Close()
		hijacked <- nil
	})

	srv := httptest.NewServer(router)
	defer srv.Close()

	// A request WITHOUT Accept-Encoding, so the response is on the identity path.
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, srv.URL+"/hijack", nil)
	if err != nil {
		t.Fatalf("building request: %v", err)
	}
	req.Header.Set("Accept-Encoding", "")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /hijack: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if err := <-hijacked; err != nil {
		t.Errorf("Hijack through the compression middleware failed: %v — the wrapper dropped a "+
			"capability the real writer has", err)
	}
}
