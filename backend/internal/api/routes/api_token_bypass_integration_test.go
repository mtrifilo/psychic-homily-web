package routes

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"psychic-homily-backend/internal/api/middleware"
	authm "psychic-homily-backend/internal/models/auth"
	"psychic-homily-backend/internal/services"
	adminsvc "psychic-homily-backend/internal/services/admin"
	"psychic-homily-backend/internal/testutil"
)

// The rate-limiter hatches, driven through the ROUTER against a real database.
//
// The router's own limiters read RouteContext.ValidateAPIToken, which every
// bypass treats as "no usable token" when it is absent. A suite that only
// asserts requests are limited would therefore stay green if the wiring were
// deleted, and the ph CLI would silently drop to 10 shows an hour. These cases
// mint a real token and require the hatch to OPEN.
func TestAPITokenBypassThroughRouter(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	td := testutil.SetupTestPostgres(t)
	defer td.Cleanup()

	cfg := testConfig()
	sc := services.NewServiceContainer(td.DB, cfg)

	adminEmail := "api-token-bypass-admin@test.com"
	admin := &authm.User{Email: &adminEmail, IsActive: true, EmailVerified: true, IsAdmin: true}
	if err := td.DB.Create(admin).Error; err != nil {
		t.Fatalf("create admin: %v", err)
	}
	created, err := adminsvc.NewAPITokenService(td.DB).CreateToken(admin.ID, nil, 30)
	if err != nil {
		t.Fatalf("create api token: %v", err)
	}
	adminSession, err := sc.JWT.CreateToken(admin)
	if err != nil {
		t.Fatalf("create admin session token: %v", err)
	}

	// A router per case: the limiters keep their counters in memory, so a
	// shared one would let an earlier case spend a later case's budget and
	// every failure would read as "already throttled".
	newRouter := func() *chi.Mux {
		router := chi.NewRouter()
		SetupRoutes(router, sc, cfg)
		return router
	}

	post := func(router *chi.Mux, path, header, ip string) int {
		req := httptest.NewRequest("POST", path, strings.NewReader(`{}`))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", header)
		req.RemoteAddr = ip
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		return w.Code
	}

	liveToken := "Bearer " + created.Token

	// Each case walks well past its cap; a single 429 means the hatch is shut.
	hatches := []struct {
		name  string
		path  string
		limit int
	}{
		{"show create", "/shows", middleware.ShowCreateRequestsPerHour},
		{"tag create", "/entities/artist/1/tags", middleware.TagCreateRequestsPerHour},
		{"tag vote", "/tags/1/entities/artist/1/votes", middleware.TagVoteRequestsPerMinute},
	}
	for _, h := range hatches {
		t.Run(h.name, func(t *testing.T) {
			router := newRouter()
			for i := 0; i < h.limit+5; i++ {
				if code := post(router, h.path, liveToken, "198.51.100.77:4444"); code == http.StatusTooManyRequests {
					t.Fatalf("request %d returned 429: the live API token is not reaching the %s bypass, so RouteContext.ValidateAPIToken is not wired", i+1, h.name)
				}
			}
		})
	}

	// Show creation withholds the admin-JWT hatch that tag creation grants, so
	// an admin session is metered here. Nothing else pins that asymmetry.
	t.Run("admin session is still limited on show create", func(t *testing.T) {
		router := newRouter()
		limit := middleware.ShowCreateRequestsPerHour
		for i := 0; i < limit; i++ {
			if code := post(router, "/shows", "Bearer "+adminSession, "198.51.100.78:4444"); code == http.StatusTooManyRequests {
				t.Fatalf("request %d/%d was rate limited early on a fresh router", i+1, limit)
			}
		}
		if code := post(router, "/shows", "Bearer "+adminSession, "198.51.100.78:4444"); code != http.StatusTooManyRequests {
			t.Errorf("request %d returned %d, want 429: show creation has no admin-JWT hatch", limit+1, code)
		}
	})

	// Tag creation does grant it.
	t.Run("admin session bypasses tag create", func(t *testing.T) {
		router := newRouter()
		for i := 0; i < middleware.TagCreateRequestsPerHour+5; i++ {
			if code := post(router, "/entities/artist/1/tags", "Bearer "+adminSession, "198.51.100.79:4444"); code == http.StatusTooManyRequests {
				t.Fatalf("request %d returned 429: the admin-JWT hatch on tag creation is shut", i+1)
			}
		}
	})
}
