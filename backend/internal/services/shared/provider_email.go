package shared

import (
	apperrors "psychic-homily-backend/internal/errors"
	"psychic-homily-backend/internal/logger"
	authm "psychic-homily-backend/internal/models/auth"
)

// ValidateProviderEmailClaim refuses an identity provider's email claim that
// Postgres rejects as text (authm.PostgresRejectsText), which fails every query
// that binds it and every insert that stores it, with
// apperrors.ErrProviderEmailUnstorable. A sign-in calls it before any query.
// Neither the refusal nor its log record carries the claim.
func ValidateProviderEmailClaim(provider, email string) error {
	if !authm.PostgresRejectsText(email) {
		return nil
	}
	logger.Default().Warn("oauth_signin_refused_provider_email_unstorable", "provider", provider)
	return apperrors.ErrProviderEmailUnstorable()
}
