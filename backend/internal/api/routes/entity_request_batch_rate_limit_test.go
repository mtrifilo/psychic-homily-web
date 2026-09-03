package routes

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"psychic-homily-backend/internal/api/middleware"
)

func TestIsEntityRequestBatchRequest(t *testing.T) {
	cases := []struct {
		method string
		path   string
		want   bool
	}{
		{http.MethodPost, EntityRequestBatchPath, true},
		{http.MethodGet, EntityRequestBatchPath, false},
		{http.MethodPost, "/entity-requests", false},
		{http.MethodPost, "/entity-requests/42/withdraw", false},
		{http.MethodPost, "/entity-requests/batch/extra", false},
		{http.MethodPost, "/admin/entity-requests", false},
	}
	for _, tc := range cases {
		req := httptest.NewRequest(tc.method, tc.path, nil)
		if got := isEntityRequestBatchRequest(req); got != tc.want {
			t.Errorf("isEntityRequestBatchRequest(%s %s) = %v, want %v", tc.method, tc.path, got, tc.want)
		}
	}
}

// The batch budget rides the engagement limiter's opt-in flag, so both
// entity-request budgets arrive in an environment together.
func TestEntityRequestBatchRateLimiter_NotEnabledIsNoop(t *testing.T) {
	jwtService := newEngagementJWTService()
	token := engagementToken(t, jwtService, 1)
	mw := EntityRequestBatchRateLimiter(jwtService, noAPITokens, func(string) string { return "" })
	handler := mw(okRoutesHandler())

	for i := 0; i < middleware.EntityRequestBatchBurstPerMinute+5; i++ {
		req := httptest.NewRequest(http.MethodPost, EntityRequestBatchPath, nil)
		req.Header.Set("Authorization", "Bearer "+token)
		req.RemoteAddr = "7.7.8.1:100"
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("request %d: status = %d, want 200 (disabled → noop)", i, rr.Code)
		}
	}
}

func TestEntityRequestBatchRateLimiter_LimitsPastBurstCap(t *testing.T) {
	jwtService := newEngagementJWTService()
	token := engagementToken(t, jwtService, 1)
	mw := EntityRequestBatchRateLimiter(jwtService, noAPITokens, enableEngagementEnv)
	handler := mw(okRoutesHandler())

	file := func() *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, EntityRequestBatchPath, nil)
		req.Header.Set("Authorization", "Bearer "+token)
		req.RemoteAddr = "7.7.8.2:100"
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		return rr
	}

	for i := 0; i < middleware.EntityRequestBatchBurstPerMinute; i++ {
		if rr := file(); rr.Code != http.StatusOK {
			t.Fatalf("batch %d within cap: status = %d, want 200", i, rr.Code)
		}
	}
	rr := file()
	if rr.Code != http.StatusTooManyRequests {
		t.Errorf("batch past cap: status = %d, want 429", rr.Code)
	}
	if rr.Header().Get("Retry-After") != "60" {
		t.Errorf("Retry-After = %q, want 60", rr.Header().Get("Retry-After"))
	}
}

// The picker splits a paste into chunks of the endpoint's 200-item cap, so a
// 600-line paste is three back-to-back requests and a full retry of it is three
// more. The whole flow must clear the burst window with room to spare, which is
// the constraint the cap was chosen against.
func TestEntityRequestBatchRateLimiter_SixHundredLinePasteAndRetryComplete(t *testing.T) {
	const pasteLines = 600
	const pickerChunkSize = 200
	chunks := pasteLines / pickerChunkSize

	jwtService := newEngagementJWTService()
	token := engagementToken(t, jwtService, 1)
	mw := EntityRequestBatchRateLimiter(jwtService, noAPITokens, enableEngagementEnv)
	handler := mw(okRoutesHandler())

	for i := 0; i < chunks*2; i++ {
		req := httptest.NewRequest(http.MethodPost, EntityRequestBatchPath, nil)
		req.Header.Set("Authorization", "Bearer "+token)
		req.RemoteAddr = "7.7.8.3:100"
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("chunk %d of a %d-line paste plus retry: status = %d, want 200", i, pasteLines, rr.Code)
		}
	}
}

// The batch budget is its OWN bucket: a contributor who has spent it still has
// their engagement allowance, and vice versa. Pinning both directions is what
// keeps a later "just reuse the shared limiter" edit from passing review.
func TestEntityRequestBatchRateLimiter_DoesNotShareTheEngagementBudget(t *testing.T) {
	jwtService := newEngagementJWTService()
	token := engagementToken(t, jwtService, 1)
	batch := EntityRequestBatchRateLimiter(jwtService, noAPITokens, enableEngagementEnv)
	engagement := EngagementMutationRateLimiter(jwtService, noAPITokens, enableEngagementEnv)
	handler := batch(engagement(okRoutesHandler()))

	send := func(path string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, path, nil)
		req.Header.Set("Authorization", "Bearer "+token)
		req.RemoteAddr = "7.7.8.4:100"
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		return rr
	}

	for i := 0; i < middleware.EntityRequestBatchBurstPerMinute; i++ {
		if rr := send(EntityRequestBatchPath); rr.Code != http.StatusOK {
			t.Fatalf("batch %d within cap: status = %d, want 200", i, rr.Code)
		}
	}
	if rr := send(EntityRequestBatchPath); rr.Code != http.StatusTooManyRequests {
		t.Fatalf("batch past its own cap: status = %d, want 429", rr.Code)
	}
	if rr := send("/saved-shows/1"); rr.Code != http.StatusOK {
		t.Errorf("save after the batch budget is spent: status = %d, want 200 (separate buckets)", rr.Code)
	}
	if rr := send("/entity-requests"); rr.Code != http.StatusOK {
		t.Errorf("single file after the batch budget is spent: status = %d, want 200 (separate buckets)", rr.Code)
	}
}

// Only the batch route is metered here; everything else passes through, so the
// batch budget can never 429 an unrelated endpoint.
func TestEntityRequestBatchRateLimiter_OtherPathsPassThrough(t *testing.T) {
	jwtService := newEngagementJWTService()
	token := engagementToken(t, jwtService, 1)
	mw := EntityRequestBatchRateLimiter(jwtService, noAPITokens, enableEngagementEnv)
	handler := mw(okRoutesHandler())

	for _, path := range []string{"/entity-requests", "/entity-requests/42/withdraw", "/artists/1/follow"} {
		for i := 0; i < middleware.EntityRequestBatchBurstPerMinute+5; i++ {
			req := httptest.NewRequest(http.MethodPost, path, nil)
			req.Header.Set("Authorization", "Bearer "+token)
			req.RemoteAddr = "7.7.8.5:100"
			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)
			if rr.Code != http.StatusOK {
				t.Fatalf("%s request %d: status = %d, want 200 (not the batch route)", path, i, rr.Code)
			}
		}
	}
}

// Two users never collide: the budget keys by user id, not by IP, so a shared
// egress does not make one contributor's paste session spend another's.
func TestEntityRequestBatchRateLimiter_UsersDoNotCollide(t *testing.T) {
	jwtService := newEngagementJWTService()
	first := engagementToken(t, jwtService, 1)
	second := engagementToken(t, jwtService, 2)
	mw := EntityRequestBatchRateLimiter(jwtService, noAPITokens, enableEngagementEnv)
	handler := mw(okRoutesHandler())

	send := func(token string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, EntityRequestBatchPath, nil)
		req.Header.Set("Authorization", "Bearer "+token)
		req.RemoteAddr = "7.7.8.6:100"
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		return rr
	}

	for i := 0; i < middleware.EntityRequestBatchBurstPerMinute; i++ {
		if rr := send(first); rr.Code != http.StatusOK {
			t.Fatalf("first user batch %d: status = %d, want 200", i, rr.Code)
		}
	}
	if rr := send(first); rr.Code != http.StatusTooManyRequests {
		t.Fatalf("first user past cap: status = %d, want 429", rr.Code)
	}
	if rr := send(second); rr.Code != http.StatusOK {
		t.Errorf("second user's first batch: status = %d, want 200 (per-user buckets)", rr.Code)
	}
}

// A validated API token bypasses, an unvalidated phk_ prefix does not: the same
// rule PSY-2004 pinned on every other limiter.
func TestEntityRequestBatchRateLimiter_APITokenBypassRequiresValidation(t *testing.T) {
	const live = "phk_live"
	jwtService := newEngagementJWTService()
	validate := func(tok string) bool { return tok == live }
	mw := EntityRequestBatchRateLimiter(jwtService, validate, enableEngagementEnv)
	handler := mw(okRoutesHandler())

	send := func(bearer, remote string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, EntityRequestBatchPath, nil)
		req.Header.Set("Authorization", "Bearer "+bearer)
		req.RemoteAddr = remote
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		return rr
	}

	for i := 0; i < middleware.EntityRequestBatchBurstPerMinute+5; i++ {
		if rr := send(live, "7.7.8.7:100"); rr.Code != http.StatusOK {
			t.Fatalf("live token batch %d: status = %d, want 200 (exempt)", i, rr.Code)
		}
	}

	// A forged prefix carries no session either, so it never reaches the
	// per-user bucket: the limiter passes it through to the handler behind,
	// where authentication rejects it. What must NOT happen is the bypass.
	forgedValidated := false
	countingValidate := func(tok string) bool {
		if tok == "phk_forged" {
			forgedValidated = true
		}
		return tok == live
	}
	forgedHandler := EntityRequestBatchRateLimiter(jwtService, countingValidate, enableEngagementEnv)(okRoutesHandler())
	req := httptest.NewRequest(http.MethodPost, EntityRequestBatchPath, nil)
	req.Header.Set("Authorization", "Bearer phk_forged")
	req.RemoteAddr = "7.7.8.8:100"
	forgedHandler.ServeHTTP(httptest.NewRecorder(), req)
	if !forgedValidated {
		t.Error("a phk_ bearer reached the bypass without being validated")
	}
}
