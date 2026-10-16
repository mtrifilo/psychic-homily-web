package auth

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/require"
)

// callerMark is planted in every caller-controlled value of the tokens below.
// It is seven runes, so a key id made of it survives a prefix bound of eight
// runes or more whole.
const callerMark = "Qz7mark"

// rawToken joins a header and claims of any shape with sig as the signature
// segment. The result is not signed.
func rawToken(t *testing.T, header, claims any, sig string) string {
	t.Helper()
	seg := func(v any) string {
		if s, ok := v.(string); ok {
			return base64.RawURLEncoding.EncodeToString([]byte(s))
		}
		out, err := json.Marshal(v)
		require.NoError(t, err)
		return base64.RawURLEncoding.EncodeToString(out)
	}
	return seg(header) + "." + seg(claims) + "." + sig
}

// No ValidateIdentityToken error text carries caller-supplied text: not the
// token, not one of its segments, and not a value from its header or claims,
// the key id included. The handler logs these texts at warn.
func TestValidateIdentityToken_ErrorTextCarriesNoCallerText(t *testing.T) {
	key := newRSAKey(t)
	otherKey := newRSAKey(t)
	header := map[string]any{"alg": "RS256", "typ": "JWT", "kid": "known-kid"}
	liveClaims := func() map[string]any {
		return map[string]any{
			"iss":   appleIssuer,
			"aud":   keysTestBundleID,
			"sub":   "sub-" + callerMark,
			"email": callerMark + "@example.com",
			"exp":   time.Now().Add(time.Hour).Unix(),
			"iat":   time.Now().Unix(),
		}
	}
	sign := func(t *testing.T, method jwt.SigningMethod, signKey any, kid string, mutate func(map[string]any)) string {
		t.Helper()
		claims := liveClaims()
		if mutate != nil {
			mutate(claims)
		}
		tok := jwt.NewWithClaims(method, jwt.MapClaims(claims))
		tok.Header["kid"] = kid
		out, err := tok.SignedString(signKey)
		require.NoError(t, err)
		return out
	}
	// withHeader is header with key set to value, or removed when value is nil.
	withHeader := func(key string, value any) map[string]any {
		h := maps.Clone(header)
		if value == nil {
			delete(h, key)
		} else {
			h[key] = value
		}
		return h
	}

	cases := []struct {
		name string
		// token builds the token under test; prime may run lookups first.
		token func(t *testing.T) string
		prime func(t *testing.T, svc *AppleAuthService, ks *keyServer)
		// cancelled runs the validation under a context that has ended.
		cancelled bool
		// kind, when set, is the jwt sentinel the error wraps.
		kind error
	}{
		{name: "not three segments", token: func(*testing.T) string { return "not-a-jwt-" + callerMark }},
		{name: "header not base64url", token: func(*testing.T) string { return "!" + callerMark + "!.e30.c2ln" }},
		{name: "header not JSON", token: func(t *testing.T) string { return rawToken(t, "{"+callerMark, map[string]any{}, "c2ln") }},
		{name: "claims not base64url", token: func(t *testing.T) string {
			return strings.Split(rawToken(t, header, map[string]any{}, ""), ".")[0] + ".!" + callerMark + "!.c2ln"
		}},
		{name: "claims not JSON", token: func(t *testing.T) string { return rawToken(t, header, "{"+callerMark, "c2ln") }},
		{name: "exp a string", kind: jwt.ErrTokenMalformed, token: func(t *testing.T) string {
			return rawToken(t, header, map[string]any{"exp": callerMark}, "c2ln")
		}},
		{name: "aud an object", token: func(t *testing.T) string {
			return rawToken(t, header, map[string]any{"aud": map[string]any{callerMark: callerMark}}, "c2ln")
		}},
		{name: "email an object", token: func(t *testing.T) string {
			return rawToken(t, header, map[string]any{"email": map[string]any{callerMark: callerMark}}, "c2ln")
		}},
		{name: "unknown alg", token: func(t *testing.T) string {
			return rawToken(t, withHeader("alg", callerMark), liveClaims(), "c2ln")
		}},
		{name: "no alg", token: func(t *testing.T) string {
			return rawToken(t, withHeader("alg", nil), liveClaims(), "c2ln")
		}},
		{name: "no kid", token: func(t *testing.T) string {
			return rawToken(t, withHeader("kid", nil), liveClaims(), "c2ln")
		}},
		{name: "kid an object", token: func(t *testing.T) string {
			return rawToken(t, withHeader("kid", map[string]any{callerMark: callerMark}), liveClaims(), "c2ln")
		}},
		{name: "unknown kid, fetched", token: func(t *testing.T) string { return sign(t, jwt.SigningMethodRS256, key, callerMark, nil) }},
		{
			name:  "unknown kid, refresh cooling down",
			token: func(t *testing.T) string { return sign(t, jwt.SigningMethodRS256, key, callerMark, nil) },
			prime: func(t *testing.T, svc *AppleAuthService, _ *keyServer) {
				_, err := svc.getApplePublicKey(t.Context(), "spend-the-refresh")
				require.Error(t, err)
			},
		},
		{
			name:  "key set fetch failing",
			token: func(t *testing.T) string { return sign(t, jwt.SigningMethodRS256, key, callerMark, nil) },
			prime: func(t *testing.T, _ *AppleAuthService, ks *keyServer) {
				ks.serve(http.StatusInternalServerError, []byte(callerMark))
			},
		},
		{
			name:  "backing off after a failed fetch",
			token: func(t *testing.T) string { return sign(t, jwt.SigningMethodRS256, key, callerMark, nil) },
			prime: func(t *testing.T, svc *AppleAuthService, ks *keyServer) {
				ks.serve(http.StatusInternalServerError, nil)
				_, err := svc.getApplePublicKey(t.Context(), "cold-lookup")
				require.Error(t, err)
			},
		},
		{
			name:      "caller context ended",
			token:     func(t *testing.T) string { return sign(t, jwt.SigningMethodRS256, key, callerMark, nil) },
			cancelled: true,
		},
		{name: "HMAC algorithm", kind: jwt.ErrTokenUnverifiable, token: func(t *testing.T) string {
			return sign(t, jwt.SigningMethodHS256, []byte(callerMark), "known-kid", nil)
		}},
		{name: "signature not base64url", token: func(t *testing.T) string {
			parts := strings.Split(sign(t, jwt.SigningMethodRS256, key, "known-kid", nil), ".")
			return parts[0] + "." + parts[1] + ".!" + callerMark + "!"
		}},
		{name: "signed by another key", kind: jwt.ErrTokenSignatureInvalid, token: func(t *testing.T) string {
			return sign(t, jwt.SigningMethodRS256, otherKey, "known-kid", nil)
		}},
		{name: "other issuer", kind: jwt.ErrTokenInvalidIssuer, token: func(t *testing.T) string {
			return sign(t, jwt.SigningMethodRS256, key, "known-kid", func(c map[string]any) { c["iss"] = "https://" + callerMark })
		}},
		{name: "other audience", kind: jwt.ErrTokenInvalidAudience, token: func(t *testing.T) string {
			return sign(t, jwt.SigningMethodRS256, key, "known-kid", func(c map[string]any) { c["aud"] = callerMark })
		}},
		{name: "expired", kind: jwt.ErrTokenExpired, token: func(t *testing.T) string {
			return sign(t, jwt.SigningMethodRS256, key, "known-kid", func(c map[string]any) { c["exp"] = time.Now().Add(-time.Hour).Unix() })
		}},
		{name: "expired and other audience", kind: jwt.ErrTokenExpired, token: func(t *testing.T) string {
			return sign(t, jwt.SigningMethodRS256, key, "known-kid", func(c map[string]any) {
				c["exp"] = time.Now().Add(-time.Hour).Unix()
				c["aud"] = callerMark
			})
		}},
		{name: "no exp", kind: jwt.ErrTokenRequiredClaimMissing, token: func(t *testing.T) string {
			return sign(t, jwt.SigningMethodRS256, key, "known-kid", func(c map[string]any) { delete(c, "exp") })
		}},
		{name: "exp null", kind: jwt.ErrTokenRequiredClaimMissing, token: func(t *testing.T) string {
			return sign(t, jwt.SigningMethodRS256, key, "known-kid", func(c map[string]any) { c["exp"] = nil })
		}},
		{name: "not yet valid", kind: jwt.ErrTokenNotValidYet, token: func(t *testing.T) string {
			return sign(t, jwt.SigningMethodRS256, key, "known-kid", func(c map[string]any) { c["nbf"] = time.Now().Add(time.Hour).Unix() })
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ks := newKeyServer(t, jwks(key, "known-kid"))
			svc, _, _ := newKeysTestService(ks.URL)
			_, err := svc.getApplePublicKey(t.Context(), "known-kid")
			require.NoError(t, err)
			if tc.prime != nil {
				tc.prime(t, svc, ks)
			}
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			if tc.cancelled {
				cancel()
			}
			token := tc.token(t)

			_, err = svc.ValidateIdentityToken(ctx, token)
			require.Error(t, err)
			text := err.Error()
			if tc.kind != nil {
				require.ErrorIs(t, err, tc.kind)
			}
			require.NotContains(t, text, callerMark)
			require.NotContains(t, text, token)
			for _, seg := range strings.Split(token, ".") {
				if len(seg) >= 8 {
					require.NotContains(t, text, seg)
				}
			}
		})
	}
}

// jwtFailureKind keeps the most specific sentinel of a joined jwt error, and
// an error that wraps no jwt sentinel becomes errJWTUnclassified.
func TestJWTFailureKind(t *testing.T) {
	joined := fmt.Errorf("%w: %w", jwt.ErrTokenInvalidClaims, errors.Join(jwt.ErrTokenInvalidAudience, jwt.ErrTokenExpired))
	require.Equal(t, jwt.ErrTokenExpired, jwtFailureKind(joined))
	require.Equal(t, errJWTUnclassified, jwtFailureKind(errors.New(callerMark)))
}
