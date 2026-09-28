package auth

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/humatest"

	"psychic-homily-backend/internal/api/handlers/shared/testhelpers"
	"psychic-homily-backend/internal/api/humaerr"
	"psychic-homily-backend/internal/api/middleware"
	autherrors "psychic-homily-backend/internal/errors"
	authm "psychic-homily-backend/internal/models/auth"
)

// errWireDriver is shaped like what a failed query returns from the driver.
// None of its text may reach a response body.
var errWireDriver = fmt.Errorf(`ERROR: relation "user_bookmarks" does not exist (SQLSTATE 42P01)`)

// Handler unit tests call handlers directly, so they never see what huma puts
// on the wire for a returned error. These register the real handlers on a
// huma API with humaerr installed, as routes.SetupRoutes installs it, and read
// the response.
func newWireAPI(t *testing.T, h *AuthHandler) humatest.TestAPI {
	t.Helper()
	prevNewError, prevNewErrorWithContext := huma.NewError, huma.NewErrorWithContext
	t.Cleanup(func() {
		huma.NewError = prevNewError
		huma.NewErrorWithContext = prevNewErrorWithContext
	})
	humaerr.Install()
	_, api := humatest.New(t, huma.DefaultConfig("auth-error-wire", "1.0.0"))

	withUser := huma.Middlewares{func(ctx huma.Context, next func(huma.Context)) {
		next(huma.WithValue(ctx, middleware.UserContextKey, &authm.User{ID: 1}))
	}}
	huma.Register(api, huma.Operation{
		OperationID: "export-data",
		Method:      http.MethodGet,
		Path:        "/auth/account/export",
		Middlewares: withUser,
	}, h.ExportDataHandler)
	huma.Register(api, huma.Operation{
		OperationID: "get-profile",
		Method:      http.MethodGet,
		Path:        "/auth/profile",
		Middlewares: withUser,
	}, h.GetProfileHandler)
	huma.Register(api, huma.Operation{
		OperationID: "login",
		Method:      http.MethodPost,
		Path:        "/auth/login",
	}, h.LoginHandler)
	return api
}

func assertNoInternalText(t *testing.T, body string, fragments ...string) {
	t.Helper()
	for _, leaked := range append([]string{"user_bookmarks", "SQLSTATE", "internal"}, fragments...) {
		if strings.Contains(body, leaked) {
			t.Errorf("response body contains %q: %s", leaked, body)
		}
	}
}

func decodeErrorCode(t *testing.T, raw []byte) string {
	t.Helper()
	var decoded struct {
		ErrorCode string `json:"error_code"`
		Message   string `json:"message"`
	}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("decode: %v (body: %s)", err, raw)
	}
	if decoded.Message == "" {
		t.Errorf("body carries no message: %s", raw)
	}
	return decoded.ErrorCode
}

func TestExportDataServiceFailureBodyOverTheWire(t *testing.T) {
	h := authHandler(func(ah *AuthHandler) {
		ah.userService = &testhelpers.MockUserService{
			ExportUserDataJSONFn: func(uint) ([]byte, error) {
				return nil, errWireDriver
			},
		}
	})

	resp := newWireAPI(t, h).Get("/auth/account/export")

	if resp.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503; body: %s", resp.Code, resp.Body.String())
	}
	assertNoInternalText(t, resp.Body.String(), "export_data")
	if code := decodeErrorCode(t, resp.Body.Bytes()); code != autherrors.CodeServiceUnavailable {
		t.Errorf("error_code = %q, want %q", code, autherrors.CodeServiceUnavailable)
	}
}

func TestLoginServiceFailureBodyOverTheWire(t *testing.T) {
	cases := []struct {
		name    string
		failure error
	}{
		{"raw service error", errWireDriver},
		// An AuthError code the login switch does not route: its own status
		// is 409, and the branch must still answer 5xx.
		{"unrouted auth code", autherrors.NewAuthError(autherrors.CodeUserExists, "An account with this email already exists", errWireDriver)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := authHandler(func(ah *AuthHandler) {
				ah.userService = &testhelpers.MockUserService{
					AuthenticateUserWithPasswordFn: func(string, string) (*authm.User, error) {
						return nil, tc.failure
					},
				}
			})

			resp := newWireAPI(t, h).Post("/auth/login", map[string]any{
				"email":    "test@example.com",
				"password": "any-password",
			})

			if resp.Code != http.StatusServiceUnavailable {
				t.Fatalf("status = %d, want 503; body: %s", resp.Code, resp.Body.String())
			}
			assertNoInternalText(t, resp.Body.String(), "login_unhandled_authcode")
			if code := decodeErrorCode(t, resp.Body.Bytes()); code != autherrors.CodeServiceUnavailable {
				t.Errorf("error_code = %q, want %q", code, autherrors.CodeServiceUnavailable)
			}
		})
	}
}

func TestGetProfileDeletedUserBodyOverTheWire(t *testing.T) {
	h := authHandler(func(ah *AuthHandler) {
		ah.authService = &testhelpers.MockAuthService{
			GetUserProfileFn: func(userID uint) (*authm.User, error) {
				return nil, autherrors.ErrUserNotFoundByID(userID, errWireDriver)
			},
		}
	})

	resp := newWireAPI(t, h).Get("/auth/profile")

	if resp.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401; body: %s", resp.Code, resp.Body.String())
	}
	assertNoInternalText(t, resp.Body.String(), "no user with id", "USER_NOT_FOUND")
}
