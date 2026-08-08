package routes

import (
	"compress/gzip"
	"encoding/json"
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
	"psychic-homily-backend/internal/services/contracts"
)

// stubGraphOverviewService serves one fixed snapshot, so a test can assert on
// the transport without a database behind it.
type stubGraphOverviewService struct {
	overview *contracts.GraphOverview
	etag     string
}

func (s *stubGraphOverviewService) GetGraphOverview() (*contracts.GraphOverview, string, error) {
	return s.overview, s.etag, nil
}

// syntheticOverview builds a payload at roughly production scale (PR #1807
// measured ~5k artists / 20k relationships) so the compression numbers this test
// prints are comparable to the ones on the ticket rather than to a toy body.
func syntheticOverview(nodeCount, edgeCount int) *contracts.GraphOverview {
	nodes := contracts.GraphOverviewNodes{
		ID:        make([]uint, nodeCount),
		Kind:      make([]uint8, nodeCount),
		Name:      make([]string, nodeCount),
		Slug:      make([]string, nodeCount),
		X:         make([]int16, nodeCount),
		Y:         make([]int16, nodeCount),
		Community: make([]int32, nodeCount),
		Degree:    make([]int32, nodeCount),
		Rank:      make([]int32, nodeCount),
		Flags:     make([]uint8, nodeCount),
		Appear:    make([]int32, nodeCount),
	}
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
// compression middleware, so the assertions cover the actual Huma → chi
// interaction (notably what Huma leaves in the header map on a 304) rather than
// a hand-rolled stand-in for it.
func newCompressedOverviewRouter(t *testing.T, svc contracts.GraphOverviewServiceInterface) *chi.Mux {
	t.Helper()

	router := chi.NewRouter()
	router.Use(ResponseCompression())
	api := humachi.New(router, huma.DefaultConfig("test", "1.0.0"))
	handler := catalogh.NewGraphOverviewHandler(svc)
	huma.Get(api, "/graph/overview", handler.GetGraphOverviewHandler)
	return router
}

const testOverviewETag = `W/"abc123"`

// The whole point of the ticket: a client that negotiates gzip must get gzip.
// Before this middleware the backend answered identity no matter what, because
// production browsers bypass the frontend's compressing `/api` proxy.
func TestResponseCompressionGzipsOverviewJSON(t *testing.T) {
	router := newCompressedOverviewRouter(t, &stubGraphOverviewService{
		overview: syntheticOverview(5000, 20000),
		etag:     testOverviewETag,
	})

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
	if cl := compressed.Header().Get("Content-Length"); cl != "" {
		t.Errorf("gzip request: Content-Length = %q, want empty — the pre-compression length "+
			"would describe the wrong body", cl)
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
// without the bodyless-status guard this response advertises gzip over nothing.
func TestResponseCompressionPreservesNotModified(t *testing.T) {
	router := newCompressedOverviewRouter(t, &stubGraphOverviewService{
		overview: syntheticOverview(200, 400),
		etag:     testOverviewETag,
	})

	req := httptest.NewRequest(http.MethodGet, "/graph/overview", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	req.Header.Set("If-None-Match", testOverviewETag)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotModified {
		t.Fatalf("status = %d, want 304 — compression must not break revalidation", rr.Code)
	}
	if body := rr.Body.Len(); body != 0 {
		t.Errorf("304 body = %d bytes, want 0", body)
	}
	if enc := rr.Header().Get("Content-Encoding"); enc != "" {
		t.Errorf("304 Content-Encoding = %q, want empty — there is no content to have an "+
			"encoding, and RFC 9110 §15.4.5 limits a 304 to cache-update metadata", enc)
	}
	if got := rr.Header().Get("ETag"); got != testOverviewETag {
		t.Errorf("304 ETag = %q, want %q — the validator must survive the compressor untouched, "+
			"or the client can never revalidate again", got, testOverviewETag)
	}
	if vary := rr.Header().Get("Vary"); !strings.Contains(vary, "Accept-Encoding") {
		t.Errorf("304 Vary = %q, must still contain Accept-Encoding: it describes how the "+
			"resource is selected, which is exactly what a cache updating its entry needs", vary)
	}
}

// A weak validator is compared weakly, so a cache or proxy that forwards our tag
// with an added `W/`, or a client echoing the strong form, must still revalidate.
func TestResponseCompressionNotModifiedAcrossValidatorForms(t *testing.T) {
	router := newCompressedOverviewRouter(t, &stubGraphOverviewService{
		overview: syntheticOverview(200, 400),
		etag:     testOverviewETag,
	})

	for _, ifNoneMatch := range []string{testOverviewETag, `"abc123"`, `"other", W/"abc123"`, "*"} {
		req := httptest.NewRequest(http.MethodGet, "/graph/overview", nil)
		req.Header.Set("Accept-Encoding", "gzip")
		req.Header.Set("If-None-Match", ifNoneMatch)
		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, req)

		if rr.Code != http.StatusNotModified {
			t.Errorf("If-None-Match %q: status = %d, want 304", ifNoneMatch, rr.Code)
		}
		if enc := rr.Header().Get("Content-Encoding"); enc != "" {
			t.Errorf("If-None-Match %q: Content-Encoding = %q, want empty", ifNoneMatch, enc)
		}
	}
}

// A stale validator still has to deliver the new body, compressed.
func TestResponseCompressionStaleValidatorSendsCompressedBody(t *testing.T) {
	router := newCompressedOverviewRouter(t, &stubGraphOverviewService{
		overview: syntheticOverview(200, 400),
		etag:     testOverviewETag,
	})

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
	zr, err := gzip.NewReader(rr.Body)
	if err != nil {
		t.Fatalf("body is not valid gzip: %v", err)
	}
	var decoded map[string]any
	if err := json.NewDecoder(zr).Decode(&decoded); err != nil {
		t.Fatalf("decompressed body is not JSON: %v", err)
	}
	if decoded["node_count"] == nil {
		t.Error("decompressed body is missing node_count — the payload did not survive the round trip")
	}
}

// The ICS feeds sit outside chi's compressible content types, and that is
// load-bearing rather than incidental: they carry STRONG ETags, which a second
// content-coding of the same resource would invalidate. If a future change adds
// text/calendar to the compressible set it has to weaken those tags too, and this
// test is what will say so.
func TestResponseCompressionLeavesCalendarFeedsIdentity(t *testing.T) {
	router := chi.NewRouter()
	router.Use(ResponseCompression())
	router.Get("/feed.ics", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/calendar; charset=utf-8")
		w.Header().Set("ETag", `"strong-ics-tag"`)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(strings.Repeat("BEGIN:VEVENT\r\nEND:VEVENT\r\n", 500)))
	})

	req := httptest.NewRequest(http.MethodGet, "/feed.ics", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	if enc := rr.Header().Get("Content-Encoding"); enc != "" {
		t.Errorf("Content-Encoding = %q, want empty: text/calendar is served identity so its "+
			"strong ETag keeps describing the only representation there is", enc)
	}
	if got := rr.Header().Get("ETag"); got != `"strong-ics-tag"` {
		t.Errorf("ETag = %q, want the strong tag untouched", got)
	}
}

// A 204 has the same no-content constraint as a 304 and goes through the same
// guard, so it is covered here rather than left to be discovered in production.
func TestResponseCompressionLeavesNoContentUnencoded(t *testing.T) {
	router := chi.NewRouter()
	router.Use(ResponseCompression())
	router.Delete("/thing", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNoContent)
	})

	req := httptest.NewRequest(http.MethodDelete, "/thing", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", rr.Code)
	}
	if enc := rr.Header().Get("Content-Encoding"); enc != "" {
		t.Errorf("204 Content-Encoding = %q, want empty", enc)
	}
}
