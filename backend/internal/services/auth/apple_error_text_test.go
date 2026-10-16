package auth

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/require"
)

// callerMark is planted in every caller-controlled value of the tokens below.
// It is short enough that no prefix bound on a key id could hide it.
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
	withHeader := func(change map[string]any) map[string]any {
		h := map[string]any{}
		for k, v := range header {
			h[k] = v
		}
		for k, v := range change {
			if v == nil {
				delete(h, k)
			} else {
				h[k] = v
			}
		}
		return h
	}

	cases := []struct {
		name string
		// token builds the token under test; prime may run lookups first and
		// returns the context the validation runs under.
		token func(t *testing.T) string
		prime func(t *testing.T, svc *AppleAuthService, ks *keyServer) context.Context
	}{
		{name: "not three segments", token: func(*testing.T) string { return "not-a-jwt-" + callerMark }},
		{name: "header not base64url", token: func(*testing.T) string { return "!" + callerMark + "!.e30.c2ln" }},
		{name: "header not JSON", token: func(t *testing.T) string { return rawToken(t, "{"+callerMark, map[string]any{}, "c2ln") }},
		{name: "claims not base64url", token: func(t *testing.T) string {
			return strings.Split(rawToken(t, header, map[string]any{}, ""), ".")[0] + ".!" + callerMark + "!.c2ln"
		}},
		{name: "claims not JSON", token: func(t *testing.T) string { return rawToken(t, header, "{"+callerMark, "c2ln") }},
		{name: "exp a string", token: func(t *testing.T) string {
			return rawToken(t, header, map[string]any{"exp": callerMark}, "c2ln")
		}},
		{name: "aud an object", token: func(t *testing.T) string {
			return rawToken(t, header, map[string]any{"aud": map[string]any{callerMark: callerMark}}, "c2ln")
		}},
		{name: "email an object", token: func(t *testing.T) string {
			return rawToken(t, header, map[string]any{"email": map[string]any{callerMark: callerMark}}, "c2ln")
		}},
		{name: "unknown alg", token: func(t *testing.T) string {
			return rawToken(t, withHeader(map[string]any{"alg": callerMark}), liveClaims(), "c2ln")
		}},
		{name: "no alg", token: func(t *testing.T) string {
			return rawToken(t, withHeader(map[string]any{"alg": nil}), liveClaims(), "c2ln")
		}},
		{name: "no kid", token: func(t *testing.T) string {
			return rawToken(t, withHeader(map[string]any{"kid": nil}), liveClaims(), "c2ln")
		}},
		{name: "kid an object", token: func(t *testing.T) string {
			return rawToken(t, withHeader(map[string]any{"kid": map[string]any{callerMark: callerMark}}), liveClaims(), "c2ln")
		}},
		{name: "unknown kid, fetched", token: func(t *testing.T) string { return sign(t, jwt.SigningMethodRS256, key, callerMark, nil) }},
		{
			name:  "unknown kid, refresh cooling down",
			token: func(t *testing.T) string { return sign(t, jwt.SigningMethodRS256, key, callerMark, nil) },
			prime: func(t *testing.T, svc *AppleAuthService, _ *keyServer) context.Context {
				_, err := svc.getApplePublicKey(t.Context(), "spend-the-refresh")
				require.Error(t, err)
				return t.Context()
			},
		},
		{
			name:  "key set fetch failing",
			token: func(t *testing.T) string { return sign(t, jwt.SigningMethodRS256, key, callerMark, nil) },
			prime: func(t *testing.T, _ *AppleAuthService, ks *keyServer) context.Context {
				ks.serve(http.StatusInternalServerError, []byte(callerMark))
				return t.Context()
			},
		},
		{
			name:  "backing off after a failed fetch",
			token: func(t *testing.T) string { return sign(t, jwt.SigningMethodRS256, key, callerMark, nil) },
			prime: func(t *testing.T, svc *AppleAuthService, ks *keyServer) context.Context {
				ks.serve(http.StatusInternalServerError, nil)
				_, err := svc.getApplePublicKey(t.Context(), "cold-lookup")
				require.Error(t, err)
				return t.Context()
			},
		},
		{
			name:  "caller context ended",
			token: func(t *testing.T) string { return sign(t, jwt.SigningMethodRS256, key, callerMark, nil) },
			prime: func(t *testing.T, _ *AppleAuthService, _ *keyServer) context.Context {
				ctx, cancel := context.WithCancel(t.Context())
				cancel()
				return ctx
			},
		},
		{name: "HMAC algorithm", token: func(t *testing.T) string {
			return sign(t, jwt.SigningMethodHS256, []byte(callerMark), "known-kid", nil)
		}},
		{name: "signature not base64url", token: func(t *testing.T) string {
			parts := strings.Split(sign(t, jwt.SigningMethodRS256, key, "known-kid", nil), ".")
			return parts[0] + "." + parts[1] + ".!" + callerMark + "!"
		}},
		{name: "signed by another key", token: func(t *testing.T) string {
			return sign(t, jwt.SigningMethodRS256, otherKey, "known-kid", nil)
		}},
		{name: "other issuer", token: func(t *testing.T) string {
			return sign(t, jwt.SigningMethodRS256, key, "known-kid", func(c map[string]any) { c["iss"] = "https://" + callerMark })
		}},
		{name: "other audience", token: func(t *testing.T) string {
			return sign(t, jwt.SigningMethodRS256, key, "known-kid", func(c map[string]any) { c["aud"] = callerMark })
		}},
		{name: "expired", token: func(t *testing.T) string {
			return sign(t, jwt.SigningMethodRS256, key, "known-kid", func(c map[string]any) { c["exp"] = time.Now().Add(-time.Hour).Unix() })
		}},
		{name: "not yet valid", token: func(t *testing.T) string {
			return sign(t, jwt.SigningMethodRS256, key, "known-kid", func(c map[string]any) { c["nbf"] = time.Now().Add(time.Hour).Unix() })
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ks := newKeyServer(t, jwks(key, "known-kid"))
			svc, _, _ := newKeysTestService(ks.URL)
			_, err := svc.getApplePublicKey(t.Context(), "known-kid")
			require.NoError(t, err)
			ctx := t.Context()
			if tc.prime != nil {
				ctx = tc.prime(t, svc, ks)
			}
			token := tc.token(t)

			_, err = svc.ValidateIdentityToken(ctx, token)
			require.Error(t, err)
			text := err.Error()
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
