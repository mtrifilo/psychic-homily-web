package services

import (
	"testing"

	"github.com/stretchr/testify/require"

	"psychic-homily-backend/internal/config"
)

// The challenge sweep needs only the database, so the container builds it even
// when the WebAuthn service fails to construct.
func TestNewServiceContainer_ChallengeSweepWithoutWebAuthn(t *testing.T) {
	cfg := &config.Config{
		JWT:      config.JWTConfig{SecretKey: "test-secret-key-32-chars-minimum", Expiry: 24},
		OAuth:    config.OAuthConfig{SecretKey: "test-oauth-secret-key-32-chars"},
		WebAuthn: config.WebAuthnConfig{RPID: "%zz", RPOrigins: []string{"http://localhost:3000"}},
	}

	sc := NewServiceContainer(nil, cfg)

	require.Nil(t, sc.WebAuthn, "an RPID that is not a valid URI fails WebAuthn construction")
	require.NotNil(t, sc.ChallengeSweep)
}
