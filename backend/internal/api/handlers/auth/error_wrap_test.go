package auth

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/danielgtaylor/huma/v2"

	"psychic-homily-backend/internal/api/handlers/shared/testhelpers"
	autherrors "psychic-homily-backend/internal/errors"
	authm "psychic-homily-backend/internal/models/auth"
)

// The unhandled-authcode branches must answer 5xx even when the code their
// switch does not handle maps to a 4xx. CodeUserExists maps to 409, and none of these
// three branches routes it.
func TestUnhandledAuthCodeAnswersServiceUnavailable(t *testing.T) {
	unrouted := autherrors.NewAuthError(autherrors.CodeUserExists, "An account with this email already exists", errWireDriver)
	if unrouted.HTTPStatus() >= http.StatusInternalServerError {
		t.Fatalf("the fixture code must map to a 4xx for this test to mean anything, got %d", unrouted.HTTPStatus())
	}

	cases := map[string]func() error{
		"login": func() error {
			h := authHandler(func(ah *AuthHandler) {
				ah.userService = &testhelpers.MockUserService{
					AuthenticateUserWithPasswordFn: func(string, string) (*authm.User, error) { return nil, unrouted },
				}
			})
			req := &LoginRequest{}
			req.Body.Email = "test@example.com"
			req.Body.Password = "any-password"
			_, err := h.LoginHandler(context.Background(), req)
			return err
		},
		"change password": func() error {
			h := authHandler(func(ah *AuthHandler) {
				ah.userService = &testhelpers.MockUserService{
					UpdatePasswordFn: func(uint, string, string) error { return unrouted },
				}
			})
			req := &ChangePasswordRequest{}
			req.Body.CurrentPassword = "old-password-123"
			req.Body.NewPassword = "new-password-456"
			_, err := h.ChangePasswordHandler(testhelpers.CtxWithUser(&authm.User{ID: 1}), req)
			return err
		},
		"update profile": func() error {
			mock := &testhelpers.MockUserService{
				UpdateUserFn: func(uint, map[string]any) (*authm.User, error) { return nil, unrouted },
			}
			h := NewAuthHandler(nil, nil, mock, nil, nil, nil, testConfig())
			req := &UpdateProfileRequest{}
			req.Body.FirstName = strPtr("Jane")
			_, err := h.UpdateProfileHandler(testhelpers.CtxWithUser(&authm.User{ID: 1}), req)
			return err
		},
	}
	for name, run := range cases {
		t.Run(name, func(t *testing.T) {
			var authErr *autherrors.AuthError
			if err := run(); !errors.As(err, &authErr) {
				t.Fatalf("want an *AuthError, got %T (%v)", err, err)
			}
			if authErr.Code != autherrors.CodeServiceUnavailable || authErr.HTTPStatus() != http.StatusServiceUnavailable {
				t.Errorf("outermost AuthError = %s at %d, want %s at 503", authErr.Code, authErr.HTTPStatus(), autherrors.CodeServiceUnavailable)
			}
		})
	}
}

// The deleted-user 401s carry no error argument, so even huma's default
// constructors, which render every argument's Error(), have nothing internal
// to render.
func TestDeletedUser401CarriesNoErrorDetail(t *testing.T) {
	notFound := autherrors.ErrUserNotFoundByID(1, errWireDriver)
	h := authHandler(func(ah *AuthHandler) {
		ah.authService = &testhelpers.MockAuthService{
			GetUserProfileFn: func(uint) (*authm.User, error) { return nil, notFound },
		}
	})
	ctx := testhelpers.CtxWithUser(&authm.User{ID: 1})

	_, refreshErr := h.RefreshTokenHandler(ctx, &struct{}{})
	_, profileErr := h.GetProfileHandler(ctx, &struct{}{})
	for name, err := range map[string]error{"refresh": refreshErr, "get profile": profileErr} {
		var model *huma.ErrorModel
		if !errors.As(err, &model) {
			t.Fatalf("%s: want a *huma.ErrorModel, got %T (%v)", name, err, err)
		}
		if model.Status != http.StatusUnauthorized || len(model.Errors) != 0 {
			t.Errorf("%s: status %d with %d error details, want 401 with none", name, model.Status, len(model.Errors))
		}
	}
}
