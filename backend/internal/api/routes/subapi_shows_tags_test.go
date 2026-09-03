package routes

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"psychic-homily-backend/internal/api/middleware"
)

// PSY-1598 step 2: the shows and tags groups move off their own humachi.New
// instances onto the main API, so their operations reach the published spec.
//
// The risk here is higher than the report groups the spike converted. These
// endpoints carry BYPASSES, and a bypass that silently stops working is invisible
// from the outside — the endpoint keeps answering, it is just throttled where it
// should not be (the ph CLI doing a bulk import) or unthrottled where it should
// be (anonymous abuse). Each group therefore gets three assertions: it is in the
// spec, the limiter still bites at the documented cap, and the bypass still
// bypasses.
//
// Note the two groups do NOT share a bypass mechanism, which the ticket's plan
// had merged into one:
//
//	POST /shows            rateLimitUnlessValidatedAPIToken  (validated API token only)
//	POST /shows/ai-process plain httprate.Limit               (no bypass)
//	tag create / tag vote  SkipRateLimitForAdmin              (validated API token OR admin JWT)
//
// The tokens these tests send resolve to no row in any database, so this file
// is the NEGATIVE half of the bypass contract: an unvalidated phk_ earns
// nothing. The positive half, that a live token really does bypass through this
// same router, is TestAPITokenBypassThroughRouter.

func newTestRouter(t *testing.T) *chi.Mux {
	t.Helper()
	cfg := testConfig()
	sc := testContainer(cfg)
	router := chi.NewRouter()
	SetupRoutes(router, sc, cfg)
	return router
}

func servedSpecPaths(t *testing.T, router *chi.Mux) map[string]map[string]interface{} {
	t.Helper()
	req := httptest.NewRequest("GET", "/openapi.json", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("GET /openapi.json = %d, want 200", w.Code)
	}
	var spec struct {
		Paths map[string]map[string]interface{} `json:"paths"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &spec); err != nil {
		t.Fatalf("parse spec: %v", err)
	}
	return spec.Paths
}

// The whole point of the change: these five operations were registered on
// separate Huma instances and so were absent from the one published document.
func TestShowsAndTagsOperationsAreInMainSpec(t *testing.T) {
	paths := servedSpecPaths(t, newTestRouter(t))

	for _, want := range []struct{ path, method string }{
		{"/shows", "post"},
		{"/shows/ai-process", "post"},
		{"/entities/{entity_type}/{entity_id}/tags", "post"},
		{"/tags/{tag_id}/entities/{entity_type}/{entity_id}/votes", "post"},
		{"/tags/{tag_id}/entities/{entity_type}/{entity_id}/votes", "delete"},
	} {
		item, ok := paths[want.path]
		if !ok {
			t.Errorf("%s is missing from the served spec", want.path)
			continue
		}
		if _, ok := item[want.method]; !ok {
			t.Errorf("expected a documented %s operation for %s", strings.ToUpper(want.method), want.path)
		}
	}

	// The GET siblings share a path with the converted POSTs and must survive:
	// registering the POST on the main API must not displace them.
	if item, ok := paths["/shows"]; !ok || item["get"] == nil {
		t.Error("GET /shows disappeared from the spec — the POST registration displaced it")
	}
	if item, ok := paths["/entities/{entity_type}/{entity_id}/tags"]; !ok || item["get"] == nil {
		t.Error("GET /entities/{entity_type}/{entity_id}/tags disappeared — the POST displaced it")
	}
}

// send issues one request and returns the status, with a fixed source address so
// httprate keys consistently.
func send(t *testing.T, router *chi.Mux, method, path, ip string, hdrs map[string]string) int {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = ip
	for k, v := range hdrs {
		req.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w.Code
}

// sendWithBody is send plus the response body. Needed because humaFromHTTP's
// failure mode is a bare 200 with an EMPTY body — the operation never runs — and
// every "expect not-429" assertion passes against that. See assertReachedHandler.
func sendWithBody(t *testing.T, router *chi.Mux, method, path, ip string, hdrs map[string]string) (int, string) {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = ip
	for k, v := range hdrs {
		req.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w.Code, w.Body.String()
}

// burstUnderCap sends `limit` requests and fails if any is rejected early — an
// over-tight limiter is a regression too, not just a missing one.
func burstUnderCap(t *testing.T, router *chi.Mux, method, path, ip string, limit int) {
	t.Helper()
	for i := 0; i < limit; i++ {
		if code := send(t, router, method, path, ip, nil); code == http.StatusTooManyRequests {
			t.Fatalf("request %d/%d was rate limited early (429); the limiter is tighter than %d",
				i+1, limit, limit)
		}
	}
}

func TestShowCreateStillRateLimited(t *testing.T) {
	router := newTestRouter(t)
	const ip = "203.0.113.21:1234"
	limit := middleware.ShowCreateRequestsPerHour

	burstUnderCap(t, router, "POST", "/shows", ip, limit)
	if code := send(t, router, "POST", "/shows", ip, nil); code != http.StatusTooManyRequests {
		t.Errorf("request %d returned %d, want 429 — the show-create limit did NOT survive the move to a Huma group",
			limit+1, code)
	}
}

// A phk_ header that names no live token does not open the show-create hatch. Show creation feeds the admin approval queue, so the limiter is the
// only thing standing between a logged-in account and a flooded queue.
func TestShowCreateUnvalidatedAPITokenDoesNotBypass(t *testing.T) {
	const ip = "203.0.113.22:1234"
	limit := middleware.ShowCreateRequestsPerHour

	for _, hdr := range []string{"Bearer phk_deadbeef", "Bearer phk_deadbeef trailing"} {
		t.Run(hdr, func(t *testing.T) {
			// Each case gets its own router: httprate keys by IP, so a shared
			// one would arrive already saturated.
			router := newTestRouter(t)
			hdrs := map[string]string{"Authorization": hdr}
			for i := 0; i < limit; i++ {
				if code := send(t, router, "POST", "/shows", ip, hdrs); code == http.StatusTooManyRequests {
					t.Fatalf("request %d/%d was rate limited early (429)", i+1, limit)
				}
			}
			if code := send(t, router, "POST", "/shows", ip, hdrs); code != http.StatusTooManyRequests {
				t.Errorf("request %d returned %d, want 429: an unvalidated phk_ must not bypass show creation",
					limit+1, code)
			}
		})
	}
}

// The hatch the ph CLI depends on, at the limiter: a token the validator
// accepts stays unthrottled past the cap, or bulk show imports start failing
// partway.
func TestRateLimitUnlessValidatedAPIToken_ValidatedTokenBypasses(t *testing.T) {
	const live = "phk_live"
	mw := rateLimitUnlessValidatedAPIToken(func(token string) bool { return token == live }, 1, time.Hour)
	handler := mw(okRoutesHandler())

	for i := 0; i < 5; i++ {
		req := httptest.NewRequest("POST", "/shows", nil)
		req.Header.Set("Authorization", "Bearer "+live)
		req.RemoteAddr = "203.0.113.28:1234"
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("request %d: status = %d, want 200 (a live API token must bypass)", i+1, rr.Code)
		}
	}
}

func TestAIProcessStillRateLimited(t *testing.T) {
	router := newTestRouter(t)
	const ip = "203.0.113.23:1234"
	limit := middleware.AIProcessRequestsPerMinute

	burstUnderCap(t, router, "POST", "/shows/ai-process", ip, limit)
	if code := send(t, router, "POST", "/shows/ai-process", ip, nil); code != http.StatusTooManyRequests {
		t.Errorf("request %d returned %d, want 429 — the ai-process limit did NOT survive the move", limit+1, code)
	}
}

// ai-process has NO bypass. Pinning that is the point: it calls a paid external
// API, so a bypass appearing here later would be a real cost regression.
func TestAIProcessHasNoAPITokenBypass(t *testing.T) {
	router := newTestRouter(t)
	const ip = "203.0.113.24:1234"
	hdrs := map[string]string{"Authorization": "Bearer phk_deadbeef"}

	limit := middleware.AIProcessRequestsPerMinute
	for i := 0; i < limit; i++ {
		send(t, router, "POST", "/shows/ai-process", ip, hdrs)
	}
	if code := send(t, router, "POST", "/shows/ai-process", ip, hdrs); code != http.StatusTooManyRequests {
		t.Errorf("phk_ request past the cap returned %d, want 429 — ai-process must NOT have a token bypass", code)
	}
}

func TestTagCreateStillRateLimited(t *testing.T) {
	router := newTestRouter(t)
	const ip = "203.0.113.25:1234"
	const path = "/entities/artist/1/tags"
	limit := middleware.TagCreateRequestsPerHour

	burstUnderCap(t, router, "POST", path, ip, limit)
	if code := send(t, router, "POST", path, ip, nil); code != http.StatusTooManyRequests {
		t.Errorf("request %d returned %d, want 429 — the tag-create limit did NOT survive the move", limit+1, code)
	}
}

// Same for tag creation: the prefix is not the hatch.
func TestTagCreateUnvalidatedAPITokenDoesNotBypass(t *testing.T) {
	router := newTestRouter(t)
	const ip = "203.0.113.26:1234"
	const path = "/entities/artist/1/tags"
	hdrs := map[string]string{"Authorization": "Bearer phk_deadbeef"}
	limit := middleware.TagCreateRequestsPerHour

	for i := 0; i < limit; i++ {
		if code := send(t, router, "POST", path, ip, hdrs); code == http.StatusTooManyRequests {
			t.Fatalf("request %d/%d was rate limited early (429)", i+1, limit)
		}
	}
	if code := send(t, router, "POST", path, ip, hdrs); code != http.StatusTooManyRequests {
		t.Errorf("request %d returned %d, want 429: an unvalidated phk_ must not bypass tag creation", limit+1, code)
	}
}

// The POST and DELETE votes shared ONE chi group, so they shared one budget.
// They now share one Huma group; this pins that they still do, rather than
// silently getting a budget each.
func TestTagVoteRateLimitIsSharedAcrossPostAndDelete(t *testing.T) {
	router := newTestRouter(t)
	const ip = "203.0.113.27:1234"
	const path = "/tags/1/entities/artist/1/votes"
	limit := middleware.TagVoteRequestsPerMinute

	half := limit / 2
	burstUnderCap(t, router, "POST", path, ip, half)
	for i := half; i < limit; i++ {
		if code := send(t, router, "DELETE", path, ip, nil); code == http.StatusTooManyRequests {
			t.Fatalf("DELETE %d/%d was rate limited early", i+1, limit)
		}
	}

	if code := send(t, router, "POST", path, ip, nil); code != http.StatusTooManyRequests {
		t.Errorf("request %d returned %d, want 429 — POST and DELETE no longer share one vote budget",
			limit+1, code)
	}
}

// And for tag voting, where the budget guards vote manipulation.
func TestTagVoteUnvalidatedAPITokenDoesNotBypass(t *testing.T) {
	router := newTestRouter(t)
	const ip = "203.0.113.29:1234"
	const path = "/tags/1/entities/artist/1/votes"
	hdrs := map[string]string{"Authorization": "Bearer phk_deadbeef"}
	limit := middleware.TagVoteRequestsPerMinute

	for i := 0; i < limit; i++ {
		if code := send(t, router, "POST", path, ip, hdrs); code == http.StatusTooManyRequests {
			t.Fatalf("request %d/%d was rate limited early (429)", i+1, limit)
		}
	}
	if code := send(t, router, "POST", path, ip, hdrs); code != http.StatusTooManyRequests {
		t.Errorf("request %d returned %d, want 429: an unvalidated phk_ must not bypass tag voting", limit+1, code)
	}
}

// One client exhausting its budget must not lock out another: the key function
// has to survive the bridge, or the limiter becomes global.
func TestConvertedGroupsRateLimitPerIP(t *testing.T) {
	router := newTestRouter(t)
	const path = "/shows"

	for i := 0; i <= middleware.ShowCreateRequestsPerHour; i++ {
		send(t, router, "POST", path, "198.51.100.21:5000", nil)
	}
	if code := send(t, router, "POST", path, "198.51.100.21:5000", nil); code != http.StatusTooManyRequests {
		t.Fatalf("first client should be limited, got %d", code)
	}
	if code := send(t, router, "POST", path, "198.51.100.22:5000", nil); code == http.StatusTooManyRequests {
		t.Error("a different IP was limited by the first client's budget; KeyByClientIP did not survive the move")
	}
}
