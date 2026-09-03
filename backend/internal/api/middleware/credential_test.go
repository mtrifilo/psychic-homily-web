package middleware

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The two credential readers must answer identically for the same request, or a
// limiter and an authenticator can disagree about who is calling.
func TestCredentialReadersAgree(t *testing.T) {
	cases := []struct {
		name        string
		authHeader  string
		cookieLines []string
		wantToken   string
		wantSource  string
	}{
		{name: "no credential"},
		{name: "bearer header", authHeader: "Bearer session-jwt", wantToken: "session-jwt", wantSource: "header"},
		{
			name:        "header wins over cookie",
			authHeader:  "Bearer " + APITokenPrefix + "live",
			cookieLines: []string{"auth_token=session-jwt"},
			wantToken:   APITokenPrefix + "live",
			wantSource:  "header",
		},
		{
			// A header with extra fields is not a Bearer credential, so the
			// request presents its cookie. This is the shape that used to be
			// read one way by the limiter and another by the authenticator.
			name:        "malformed header falls back to cookie",
			authHeader:  "Bearer " + APITokenPrefix + "live trailing",
			cookieLines: []string{"auth_token=session-jwt"},
			wantToken:   "session-jwt",
			wantSource:  "cookie",
		},
		{
			// Multiple Cookie header lines are legal. net/http reads them all,
			// so the huma reader must too.
			name:        "auth cookie on a second Cookie line",
			cookieLines: []string{"junk=1", "auth_token=session-jwt"},
			wantToken:   "session-jwt",
			wantSource:  "cookie",
		},
		{
			name:        "empty auth cookie is no credential",
			cookieLines: []string{"auth_token="},
		},
		{name: "non-bearer header", authHeader: "Basic dXNlcjpwYXNz"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
			if tc.authHeader != "" {
				req.Header.Set("Authorization", tc.authHeader)
			}
			for _, line := range tc.cookieLines {
				req.Header.Add("Cookie", line)
			}

			gotToken, gotSource := credentialFromRequest(req)
			if gotToken != tc.wantToken || gotSource != tc.wantSource {
				t.Errorf("credentialFromRequest = (%q, %q), want (%q, %q)",
					gotToken, gotSource, tc.wantToken, tc.wantSource)
			}

			ctx, _ := newHumaContext(t, req)
			humaToken, humaSource := credentialFromHumaContext(ctx)
			if humaToken != gotToken || humaSource != gotSource {
				t.Errorf("credentialFromHumaContext = (%q, %q), want (%q, %q) to match credentialFromRequest",
					humaToken, humaSource, gotToken, gotSource)
			}
		})
	}
}

// The centralization is only worth anything while it holds, and nothing in the
// compiler holds it: a new middleware can read the Authorization header or the
// auth cookie its own way and still build. This walks the package source and
// requires every such read to go through the shared helpers.
func TestCredentialReadsGoThroughTheSharedHelpers(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read package dir: %v", err)
	}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		body, err := os.ReadFile(filepath.Clean(name))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		for i, line := range strings.Split(string(body), "\n") {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "//") {
				continue
			}
			if strings.Contains(line, `Header.Get("Authorization")`) || strings.Contains(line, `Header("Authorization")`) {
				if !strings.Contains(line, "bearerTokenFromHeader") {
					t.Errorf("%s:%d reads the Authorization header without bearerTokenFromHeader; a second parse is how a limiter and an authenticator start disagreeing about the caller", name, i+1)
				}
			}
			if strings.Contains(line, ".Cookie(") && name != "jwt.go" {
				t.Errorf("%s:%d reads a cookie directly; the auth cookie is read by credentialFromRequest / credentialFromHumaContext in jwt.go", name, i+1)
			}
		}
	}
}
