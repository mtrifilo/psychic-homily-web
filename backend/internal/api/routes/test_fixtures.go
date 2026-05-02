package routes

import (
	"os"

	"github.com/danielgtaylor/huma/v2"

	appdb "psychic-homily-backend/db"
	adminh "psychic-homily-backend/internal/api/handlers/admin"
)

// setupTestFixtureRoutes registers the admin-only test-fixtures reset
// endpoint ONLY when ENABLE_TEST_FIXTURES=1. In any other environment the
// route is not registered at all — requests return 404, not 403.
// cmd/server/main.go additionally refuses to boot if the flag is set in a
// non-allowed ENVIRONMENT (adminh.ValidateTestFixturesEnvironment).
//
// PSY-423: registered on rc.Admin so the admin gate is enforced by
// HumaAdminMiddleware at the route level.
func setupTestFixtureRoutes(rc RouteContext) {
	if !adminh.IsTestFixturesEnabled(os.Getenv) {
		return
	}
	database := appdb.GetDB()
	if database == nil {
		return
	}
	h := adminh.NewTestFixtureHandler(database)
	huma.Post(rc.Admin, "/admin/test-fixtures/reset", h.Reset)
}
