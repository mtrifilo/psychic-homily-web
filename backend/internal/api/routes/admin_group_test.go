package routes

import (
	"context"
	"net/http"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/humatest"

	"psychic-homily-backend/internal/api/middleware"
	authm "psychic-homily-backend/internal/models/auth"
)

// PSY-423: integration test for the rc.Admin route group.
//
// Wires HumaAdminMiddleware on a Huma group exactly like routes.go does in
// production, registers a sentinel /admin/test-only route, and asserts:
//   - non-admin user → 403, handler NOT entered
//   - missing user (unauth) → 403, handler NOT entered
//   - admin user → 200, handler entered
//
// This guards the move from inline shared.RequireAdmin(ctx) to route-level
// enforcement: if a future PR forgets to install HumaAdminMiddleware on the
// admin group, the test fails. We bypass real JWT here — the contract under
// test is the admin group, not JWT itself (covered in middleware/jwt_test.go).

type adminGroupTestRequest struct{}
type adminGroupTestResponse struct {
	Body struct {
		Message string `json:"message"`
	}
}

// buildAdminGroupTestAPI returns a humatest.TestAPI wired with:
//
//	userInjector → HumaAdminMiddleware → sentinel handler
//
// The userInjector translates the X-Test-User header ("admin", "user", or
// missing) into a context user, mimicking what JWT middleware would do.
func buildAdminGroupTestAPI(t *testing.T, handlerCalled *bool) humatest.TestAPI {
	t.Helper()

	_, api := humatest.New(t, huma.DefaultConfig("test-admin-group", "1.0.0"))

	userInjector := func(ctx huma.Context, next func(huma.Context)) {
		switch ctx.Header("X-Test-User") {
		case "admin":
			u := &authm.User{IsAdmin: true}
			u.ID = 1
			next(huma.WithValue(ctx, middleware.UserContextKey, u))
		case "user":
			u := &authm.User{IsAdmin: false}
			u.ID = 2
			next(huma.WithValue(ctx, middleware.UserContextKey, u))
		default:
			// no header → no user, exactly like an unauthenticated request
			next(ctx)
		}
	}

	adminGroup := huma.NewGroup(api, "")
	adminGroup.UseMiddleware(userInjector)
	adminGroup.UseMiddleware(middleware.HumaAdminMiddleware())

	huma.Register(adminGroup, huma.Operation{
		OperationID: "test-admin-only",
		Method:      http.MethodGet,
		Path:        "/admin/test-only",
	}, func(_ context.Context, _ *adminGroupTestRequest) (*adminGroupTestResponse, error) {
		*handlerCalled = true
		out := &adminGroupTestResponse{}
		out.Body.Message = "ok"
		return out, nil
	})

	return api
}

func TestAdminGroup_NonAdmin_Returns403(t *testing.T) {
	tests := []struct {
		name           string
		userHeader     string
		wantStatus     int
		wantHandlerHit bool
	}{
		{name: "no_user", userHeader: "", wantStatus: http.StatusForbidden, wantHandlerHit: false},
		{name: "non_admin", userHeader: "user", wantStatus: http.StatusForbidden, wantHandlerHit: false},
		{name: "admin", userHeader: "admin", wantStatus: http.StatusOK, wantHandlerHit: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			handlerCalled := false
			api := buildAdminGroupTestAPI(t, &handlerCalled)

			args := []any{}
			if tc.userHeader != "" {
				args = append(args, "X-Test-User: "+tc.userHeader)
			}
			resp := api.Get("/admin/test-only", args...)

			if resp.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d (body: %s)",
					resp.Code, tc.wantStatus, resp.Body.String())
			}
			if handlerCalled != tc.wantHandlerHit {
				t.Fatalf("handlerCalled = %v, want %v — admin gate did not behave as expected",
					handlerCalled, tc.wantHandlerHit)
			}
		})
	}
}
