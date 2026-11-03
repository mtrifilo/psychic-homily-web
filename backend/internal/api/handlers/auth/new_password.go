package auth

import (
	"context"
	"errors"
	"strings"

	autherrors "psychic-homily-backend/internal/errors"
	"psychic-homily-backend/internal/logger"
)

// newPasswordRefusal judges a new password against the password validator
// for a route that has no other length or strength check. It returns the
// message to refuse the password with, or "" when it passes. A missing
// validator, or one that fails, is a 5xx error rather than a pass, so no
// unchecked string becomes a password. operation names the route in the logs
// and the 5xx.
func (h *AuthHandler) newPasswordRefusal(ctx context.Context, operation string, userID uint, password string) (string, error) {
	if h.passwordValidator == nil {
		logger.AuthError(ctx, operation+"_no_validator", errors.New("password validator not configured"), "user_id", userID)
		return "", autherrors.ErrServiceUnavailable(operation+"_validator", errors.New("not configured"))
	}
	validation, err := h.passwordValidator.ValidatePassword(password)
	if err != nil {
		logger.AuthError(ctx, operation+"_validation_error", err, "user_id", userID)
		return "", autherrors.ErrServiceUnavailable(operation+"_validator", err)
	}
	if validation.Valid {
		return "", nil
	}
	message := "Password does not meet security requirements"
	if len(validation.Errors) > 0 {
		message = validation.Errors[0]
	}
	logger.AuthWarn(ctx, operation+"_weak_password",
		"user_id", userID,
		"all_errors", strings.Join(validation.Errors, "; "),
	)
	return message, nil
}
