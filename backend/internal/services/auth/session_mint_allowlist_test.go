package auth

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// CreateToken stamps auth_at with the current time, so calling it is a claim
// that an authentication factor just completed. A path that issues a session on
// the strength of a session the caller already holds must call
// RenewSessionToken instead; calling CreateToken there hands that caller a way
// to manufacture the freshness the re-authentication gates require.
//
// Nothing in the type system says so, so this test says it: it walks the built
// tree for calls to CreateToken on a JWT service and fails when the set differs
// from the allowlist below.
//
// If you add a call site, choose one:
//   - a real factor completed in this request (password, passkey, magic link,
//     provider sign-in, account recovery) — add it here with the factor named
//   - anything else — use RenewSessionToken and carry the caller's
//     authentication time forward
var sessionFactorMintAllowlist = map[string]string{
	"internal/api/handlers/auth/auth.go:LoginHandler":                  "password",
	"internal/api/handlers/auth/auth.go:RegisterHandler":               "registration sets the password",
	"internal/api/handlers/auth/auth.go:VerifyMagicLinkHandler":        "emailed magic link",
	"internal/api/handlers/auth/auth.go:RecoverAccountHandler":         "email plus password",
	"internal/api/handlers/auth/auth.go:ConfirmAccountRecoveryHandler": "emailed recovery token",
	"internal/api/handlers/auth/passkey.go:FinishLoginHandler":         "webauthn assertion",
	"internal/api/handlers/auth/passkey.go:FinishSignupHandler":        "webauthn registration at signup",
	"internal/services/auth/apple.go:GenerateToken":                    "apple identity token, verified by the caller",
	"internal/services/auth/oauth.go:oauthCallbackInternal":            "provider handshake",
}

func TestCreateTokenIsOnlyCalledByAuthenticationFactors(t *testing.T) {
	backendRoot, err := filepath.Abs("../../..")
	if err != nil {
		t.Fatalf("resolving backend root: %v", err)
	}

	found := map[string]bool{}
	fset := token.NewFileSet()

	walkErr := filepath.Walk(backendRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			// Vendored and generated trees are not ours to police.
			if name := info.Name(); name == "vendor" || name == "node_modules" || name == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		file, parseErr := parser.ParseFile(fset, path, nil, 0)
		if parseErr != nil {
			return parseErr
		}

		rel, relErr := filepath.Rel(backendRoot, path)
		if relErr != nil {
			return relErr
		}
		rel = filepath.ToSlash(rel)

		var enclosing string
		ast.Inspect(file, func(n ast.Node) bool {
			switch node := n.(type) {
			case *ast.FuncDecl:
				enclosing = node.Name.Name
			case *ast.CallExpr:
				sel, ok := node.Fun.(*ast.SelectorExpr)
				if !ok || sel.Sel.Name != "CreateToken" {
					return true
				}
				// Other services expose an unrelated CreateToken (API tokens,
				// calendar feed tokens). Only a JWT-service receiver mints a
				// session.
				if !isJWTServiceReceiver(sel.X) {
					return true
				}
				found[rel+":"+enclosing] = true
			}
			return true
		})
		return nil
	})
	if walkErr != nil {
		t.Fatalf("walking %s: %v", backendRoot, walkErr)
	}

	for site := range found {
		if _, ok := sessionFactorMintAllowlist[site]; !ok {
			t.Errorf("%s calls CreateToken, which stamps a fresh auth_at.\n"+
				"If an authentication factor completed here, add it to sessionFactorMintAllowlist with the factor named.\n"+
				"If this renews a session the caller already holds, call RenewSessionToken and pass the caller's authentication time.", site)
		}
	}

	var stale []string
	for site := range sessionFactorMintAllowlist {
		if !found[site] {
			stale = append(stale, site)
		}
	}
	sort.Strings(stale)
	for _, site := range stale {
		t.Errorf("sessionFactorMintAllowlist lists %s, which no longer calls CreateToken; remove the entry", site)
	}
}

// isJWTServiceReceiver reports whether an expression names the JWT service.
// Matched by identifier rather than by type so the check needs no type
// information: every mint in the tree reaches it through a field or variable
// spelled this way, and a receiver spelled otherwise fails the allowlist as an
// unknown site rather than passing silently.
func isJWTServiceReceiver(x ast.Expr) bool {
	switch e := x.(type) {
	case *ast.Ident:
		return strings.Contains(strings.ToLower(e.Name), "jwt")
	case *ast.SelectorExpr:
		return strings.Contains(strings.ToLower(e.Sel.Name), "jwt")
	}
	return false
}
