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
			name:        "first auth cookie wins over a later duplicate",
			cookieLines: []string{"auth_token=first", "auth_token=second"},
			wantToken:   "first",
			wantSource:  "cookie",
		},
		{
			name:        "empty auth cookie is no credential",
			cookieLines: []string{"auth_token="},
		},
		{name: "non-bearer header", authHeader: "Basic dXNlcjpwYXNz"},
		{name: "lowercase scheme is not a Bearer credential", authHeader: "bearer session-jwt"},
		{name: "tab separator is not a Bearer credential", authHeader: "Bearer\tsession-jwt"},
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

// credentialReaderFiles are the files allowed to reach into an Authorization
// header or a cookie directly. Everything else asks one of them.
var credentialReaderFiles = map[string]bool{
	"internal/api/middleware/jwt.go": true,
	// validatedAPIToken reads the Authorization header through
	// bearerTokenFromHeader and deliberately does not read the cookie.
	"internal/api/middleware/ratelimit.go": true,
}

// A parse this centralized is worth only as much as its last copy: nothing in
// the compiler stops a new middleware from reading the Authorization header or
// the auth cookie its own way, and the bypass this ticket removed lived in the
// routes package, not this one. This walks both packages' sources and requires
// every such read to sit in a file that owns the shared helpers.
//
// It catches a SECOND PARSE, which is the divergence half of the defect. It
// does not catch prefix trust: code that reads the credential correctly and
// then exempts on strings.HasPrefix passes this and is still the bug. That half
// is held by the behavioral tests, not here.
func TestCredentialReadsGoThroughTheSharedHelpers(t *testing.T) {
	packages := []struct {
		dir    string
		prefix string
	}{
		{dir: ".", prefix: "internal/api/middleware/"},
		{dir: "../routes", prefix: "internal/api/routes/"},
	}

	for _, pkg := range packages {
		entries, err := os.ReadDir(pkg.dir)
		if err != nil {
			t.Fatalf("read %s: %v", pkg.dir, err)
		}
		for _, entry := range entries {
			name := entry.Name()
			if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
				continue
			}
			path := pkg.prefix + name
			body, err := os.ReadFile(filepath.Join(pkg.dir, name))
			if err != nil {
				t.Fatalf("read %s: %v", path, err)
			}
			// Collapse the file so a read split across lines is still one
			// token sequence to the checks below.
			flat := strings.Join(strings.Fields(string(body)), " ")

			if strings.Contains(flat, `"Authorization"`) && !credentialReaderFiles[path] {
				t.Errorf("%s reads the Authorization header; a second parse is how a limiter and an authenticator start disagreeing about the caller. Call credentialFromRequest, credentialFromHumaContext, or bearerTokenFromHeader.", path)
			}
			if strings.Contains(flat, `Cookie(config.AuthCookieName)`) && path != "internal/api/middleware/jwt.go" {
				t.Errorf("%s reads the auth cookie directly; credentialFromRequest and credentialFromHumaContext own that read.", path)
			}
		}
	}
}
