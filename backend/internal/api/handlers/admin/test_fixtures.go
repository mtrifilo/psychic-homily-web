package admin

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/danielgtaylor/huma/v2"
	"gorm.io/gorm"

	"psychic-homily-backend/internal/api/middleware"
	"psychic-homily-backend/internal/logger"
	authm "psychic-homily-backend/internal/models/auth"
	catalogm "psychic-homily-backend/internal/models/catalog"
	communitym "psychic-homily-backend/internal/models/community"
	engagementm "psychic-homily-backend/internal/models/engagement"
)

// Env flag + allowed environment list for the test-fixtures reset endpoint.
// See PSY-432 for the layered-defense rationale. The check is default-deny:
// `ENABLE_TEST_FIXTURES=1` is only honored when ENVIRONMENT is one of the
// allowed values. Unset and unknown values both refuse.
const (
	EnableTestFixturesEnvVar = "ENABLE_TEST_FIXTURES"
	TestFixturesHeader       = "X-Test-Fixtures"
	TestUserEmailSuffix      = "@test.local"
)

// TestFixturesAllowedEnvironments is the whitelist of ENVIRONMENT values that
// may set ENABLE_TEST_FIXTURES=1. Blacklisting "production" would miss
// staging/preview and other envs that host real data, so allow-list instead.
var TestFixturesAllowedEnvironments = map[string]bool{
	"test":        true,
	"ci":          true,
	"development": true,
}

// testFixtureScope is the subset of a delete query that varies per allowlisted
// "table" (some entries map to a logical scope, e.g. pending shows, rather
// than a plain TRUNCATE). Each scope owns its table name and predicate; the
// user_id is bound at call time.
type testFixtureScope struct {
	// displayName is the key callers pass in `tables` and we return in the
	// response counts. Not always the literal DB table name (see pending_shows).
	displayName string
	// delete executes the scope's DELETE for the given user in the given tx.
	// Returns rows affected.
	delete func(tx *gorm.DB, userID uint) (int64, error)
}

// testFixtureAllowlist is the set of scopes this endpoint can reset. Aligned
// with the physical schema as of 2026-04:
//
//   - user_bookmarks backs saves, favorite venues, follows (artist/venue/
//     label/festival), and going/interested. Resetting it wipes all five
//     bookmark-backed flows at once. Coarser than per-action deletes, but
//     exactly what a fresh-slate E2E run wants.
//   - collections cascades to collection_items via FK ON DELETE CASCADE, but
//     tests also add items to OTHER users' collections; we clear those via
//     collection_items separately.
//   - pending_shows is exposed as a virtual scope so tests can't accidentally
//     wipe approved seed shows. The predicate scopes to submitted-by-user +
//     pending status.
//
// If you add a new user-owned table, add it here AND extend the allowlist
// contract test (test_fixtures_allowlist_test.go) or explicitly skip it with
// a justifying comment.
var testFixtureAllowlist = []testFixtureScope{
	{
		displayName: "user_bookmarks",
		delete: func(tx *gorm.DB, userID uint) (int64, error) {
			res := tx.Where("user_id = ?", userID).Delete(&engagementm.UserBookmark{})
			return res.RowsAffected, res.Error
		},
	},
	{
		displayName: "collection_items",
		delete: func(tx *gorm.DB, userID uint) (int64, error) {
			res := tx.Where("added_by_user_id = ?", userID).Delete(&communitym.CollectionItem{})
			return res.RowsAffected, res.Error
		},
	},
	{
		displayName: "collection_subscribers",
		delete: func(tx *gorm.DB, userID uint) (int64, error) {
			res := tx.Table("collection_subscribers").Where("user_id = ?", userID).Delete(nil)
			return res.RowsAffected, res.Error
		},
	},
	{
		// PSY-352: per-user likes; FK to users with ON DELETE CASCADE.
		// Reset alongside subscribers so a fresh-slate E2E run doesn't
		// inherit prior likes from a previous run.
		displayName: "collection_likes",
		delete: func(tx *gorm.DB, userID uint) (int64, error) {
			res := tx.Where("user_id = ?", userID).Delete(&communitym.CollectionLike{})
			return res.RowsAffected, res.Error
		},
	},
	{
		displayName: "collections",
		delete: func(tx *gorm.DB, userID uint) (int64, error) {
			res := tx.Where("creator_id = ?", userID).Delete(&communitym.Collection{})
			return res.RowsAffected, res.Error
		},
	},
	{
		displayName: "pending_shows",
		delete: func(tx *gorm.DB, userID uint) (int64, error) {
			res := tx.Where("submitted_by = ? AND status = ?", userID, catalogm.ShowStatusPending).Delete(&catalogm.Show{})
			return res.RowsAffected, res.Error
		},
	},
}

// testFixtureScopeByName returns the scope for a given display name, or false
// if the name is unknown.
func testFixtureScopeByName(name string) (testFixtureScope, bool) {
	for _, s := range testFixtureAllowlist {
		if s.displayName == name {
			return s, true
		}
	}
	return testFixtureScope{}, false
}

// IsTestFixturesEnabled reports whether the endpoint should be registered.
// Callers should also invoke ValidateTestFixturesEnvironment at startup to
// panic if the flag is combined with a non-allowed ENVIRONMENT.
func IsTestFixturesEnabled(getenv func(string) string) bool {
	return getenv(EnableTestFixturesEnvVar) == "1"
}

// ValidateTestFixturesEnvironment panics unless the combination of
// ENABLE_TEST_FIXTURES and ENVIRONMENT is safe. Rules:
//   - ENABLE_TEST_FIXTURES=0 (or unset): always safe, returns nil.
//   - ENABLE_TEST_FIXTURES=1 AND ENVIRONMENT in {test, ci, development}: safe.
//   - Any other combination: returns an error describing the mismatch.
//
// Call this from cmd/server/main.go before route setup. A returned error
// should cause the server to refuse to boot.
func ValidateTestFixturesEnvironment(getenv func(string) string) error {
	if !IsTestFixturesEnabled(getenv) {
		return nil
	}
	env := getenv("ENVIRONMENT")
	if !TestFixturesAllowedEnvironments[env] {
		allowed := make([]string, 0, len(TestFixturesAllowedEnvironments))
		for k := range TestFixturesAllowedEnvironments {
			allowed = append(allowed, k)
		}
		return fmt.Errorf(
			"%s=1 requires ENVIRONMENT to be one of %v (got %q). Refusing to boot.",
			EnableTestFixturesEnvVar, allowed, env,
		)
	}
	return nil
}

// TestFixtureHandler serves the admin-only, test-env-only reset endpoint.
// It is only registered when ENABLE_TEST_FIXTURES=1 (see routes.go).
type TestFixtureHandler struct {
	db *gorm.DB
}

// NewTestFixtureHandler constructs the handler. The caller (routes.go) is
// responsible for only instantiating this when IsTestFixturesEnabled is true.
func NewTestFixtureHandler(db *gorm.DB) *TestFixtureHandler {
	return &TestFixtureHandler{db: db}
}

// ResetTestFixturesRequest is the body for POST /admin/test-fixtures/reset.
type ResetTestFixturesRequest struct {
	TestFixturesToken string `header:"X-Test-Fixtures" doc:"Required: 'X-Test-Fixtures: 1' — belt for the admin JWT suspenders."`
	Body              struct {
		UserID uint     `json:"user_id" doc:"Target user whose rows will be deleted. Must have a @test.local email."`
		Tables []string `json:"tables" doc:"Allowlisted display names to reset."`
	}
}

// ResetTestFixturesResponse reports per-scope row counts deleted.
type ResetTestFixturesResponse struct {
	Body struct {
		Deleted map[string]int64 `json:"deleted"`
	}
}

// Reset deletes the target user's rows across the requested allowlisted
// scopes. All deletes run in one transaction; any error rolls back.
//
// Defenses (layered; any one failure aborts):
//  1. Route is only registered when ENABLE_TEST_FIXTURES=1 (routes.go).
//  2. Startup panic if ENABLE_TEST_FIXTURES=1 + ENVIRONMENT not in the
//     allowed list (ValidateTestFixturesEnvironment).
//  3. Caller must be an admin (checked below).
//  4. Header "X-Test-Fixtures: 1" required.
//  5. Target user must have a @test.local email.
func (h *TestFixtureHandler) Reset(ctx context.Context, req *ResetTestFixturesRequest) (*ResetTestFixturesResponse, error) {
	if h.db == nil {
		return nil, huma.Error500InternalServerError("database not initialized")
	}

	// Defense 4: require the test header independent of auth.
	if req.TestFixturesToken != "1" {
		return nil, huma.Error400BadRequest(fmt.Sprintf("%s header is required", TestFixturesHeader))
	}

	// Defense 3: admin only. Route lives on the rc.Admin group (PSY-423) so
	// JWT auth + IsAdmin are both enforced upstream by middleware. Pull the
	// user for audit-log attribution below.
	user := middleware.GetUserFromContext(ctx)

	if len(req.Body.Tables) == 0 {
		return nil, huma.Error400BadRequest("tables: at least one scope is required")
	}

	// Validate each requested scope against the allowlist before doing any DB
	// work. Unknown -> 400, no partial work.
	scopes := make([]testFixtureScope, 0, len(req.Body.Tables))
	for _, name := range req.Body.Tables {
		scope, ok := testFixtureScopeByName(name)
		if !ok {
			known := make([]string, 0, len(testFixtureAllowlist))
			for _, s := range testFixtureAllowlist {
				known = append(known, s.displayName)
			}
			return nil, huma.Error400BadRequest(fmt.Sprintf(
				"unknown table %q; allowed: %s", name, strings.Join(known, ", "),
			))
		}
		scopes = append(scopes, scope)
	}

	// Defense 5: target user must exist and have a @test.local email. Cheap
	// sanity check with real teeth if the flag ever leaks into an env with
	// real users.
	var target authm.User
	if err := h.db.Select("id, email").First(&target, req.Body.UserID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, huma.Error404NotFound("target user not found")
		}
		return nil, huma.Error500InternalServerError("failed to look up target user")
	}
	if target.Email == nil || !strings.HasSuffix(strings.ToLower(*target.Email), TestUserEmailSuffix) {
		return nil, huma.Error403Forbidden("target user is not a test user")
	}

	// Audit-adjacent: log every hit at warn so an accidental exposure in a
	// real env gets loud regardless of outcome.
	logger.FromContext(ctx).Warn("test_fixtures_reset",
		"admin_id", user.ID,
		"target_user_id", req.Body.UserID,
		"tables", req.Body.Tables,
	)

	deleted := make(map[string]int64, len(scopes))
	err := h.db.Transaction(func(tx *gorm.DB) error {
		for _, scope := range scopes {
			n, err := scope.delete(tx, req.Body.UserID)
			if err != nil {
				return fmt.Errorf("%s: %w", scope.displayName, err)
			}
			deleted[scope.displayName] = n
		}
		return nil
	})
	if err != nil {
		return nil, huma.Error500InternalServerError(fmt.Sprintf("reset failed: %v", err))
	}

	resp := &ResetTestFixturesResponse{}
	resp.Body.Deleted = deleted
	return resp, nil
}
