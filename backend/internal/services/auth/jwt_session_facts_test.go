package auth

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The session's age is its iat, read on the same parse as everything else.
func TestValidateSessionFacts_ReadsIssuedAt(t *testing.T) {
	svc := authTimeTestService(t, authTimeTestSecret, 24)
	issued := time.Now().Add(-30 * time.Hour).Truncate(time.Second)
	token := signSessionClaims(t, authTimeTestSecret, jwt.MapClaims{
		"iat": issued.Unix(),
		"exp": time.Now().Add(time.Hour).Unix(),
	})

	user, facts, err := svc.ValidateSessionFacts(token)
	require.NoError(t, err)
	assert.Equal(t, authTimeTestUser().ID, user.ID)
	assert.True(t, facts.IssuedAt.Equal(issued), "IssuedAt = %v, want %v", facts.IssuedAt, issued)

	// The narrow reader agrees with the wide one.
	_, authAt, err := svc.ValidateSession(token)
	require.NoError(t, err)
	assert.True(t, authAt.Equal(facts.AuthAt))
}

func TestValidateSessionFacts_IssuedAtAbsentOrUnbelievableIsZero(t *testing.T) {
	svc := authTimeTestService(t, authTimeTestSecret, 24)
	cases := map[string]jwt.MapClaims{
		"no iat":                {"iat": nil},
		"iat of zero":           {"iat": 0},
		"iat in the far future": {"iat": time.Now().Add(time.Hour).Unix()},
	}
	for name, overrides := range cases {
		t.Run(name, func(t *testing.T) {
			claims := sessionTestClaims()
			for k, v := range overrides {
				if v == nil {
					delete(claims, k)
				} else {
					claims[k] = v
				}
			}
			signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(authTimeTestSecret))
			require.NoError(t, err)
			_, facts, err := svc.ValidateSessionFacts(signed)
			require.NoError(t, err)
			assert.True(t, facts.IssuedAt.IsZero(), "IssuedAt = %v, want zero", facts.IssuedAt)
		})
	}
}

// A token inside the refresh grace still reports how old it is.
func TestValidateSessionLenientFacts_ReadsIssuedAtAcrossExpiry(t *testing.T) {
	svc := authTimeTestService(t, authTimeTestSecret, 24)
	issued := time.Now().Add(-26 * time.Hour).Truncate(time.Second)
	token := signSessionClaims(t, authTimeTestSecret, jwt.MapClaims{
		"iat": issued.Unix(),
		"exp": time.Now().Add(-time.Hour).Unix(),
	})

	_, _, err := svc.ValidateSessionFacts(token)
	require.Error(t, err, "the strict reader refuses an expired token")
	_, facts, err := svc.ValidateSessionLenientFacts(token, 7*24*time.Hour)
	require.NoError(t, err)
	assert.True(t, facts.IssuedAt.Equal(issued))
}
