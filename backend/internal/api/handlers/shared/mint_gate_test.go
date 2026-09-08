package shared_test

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

// Issuing a credential to whoever presents a session is the operation
// RequireRecentSessionAuth guards, and a handler that forgets to call it is not
// a compile error. This test is what catches that: it parses every non-test .go
// file under the handler tree, finds every function that names a
// credential-issuing service method, and fails when that function does not also
// name RequireRecentSessionAuth.
//
// What it checks is that the NAME appears in the same function. It does not
// check that the gate is called, that its error is returned, or that it runs
// before the issuance, so a site that reached for it and dropped the result
// passes. Read as a spelling check on a list of names, not as proof that the
// gate runs.
//
// The sibling walk in internal/services/auth/session_mint_allowlist_test.go
// asks a different question about the same method names: whether a mint claims
// an authentication factor that did not happen. That one is about what the
// minted token says; this one is about who is allowed to ask for it.
//
// Adding a site means choosing one:
//   - the request completed an authentication factor of its own (a sign-in, a
//     verified password), so the freshness the gate asks for is what this
//     request just established: list it below with the factor
//   - anything else issues on the strength of a credential the caller already
//     holds: call shared.RequireRecentSessionAuth first
//
// It also does not follow calls: it sees the function that names the method,
// not every path that reaches it, so a new caller of an already-listed helper
// is on the author.
var mintGateExemptions = map[string]string{
	"auth/auth.go:AuthHandler.LoginHandler":                    "password verified in this request",
	"auth/auth.go:AuthHandler.RegisterHandler":                 "registration sets the password",
	"auth/auth.go:AuthHandler.VerifyMagicLinkHandler":          "emailed magic link",
	"auth/auth.go:AuthHandler.RecoverAccountHandler":           "email plus password",
	"auth/auth.go:AuthHandler.ConfirmAccountRecoveryHandler":   "emailed recovery token",
	"auth/auth.go:AuthHandler.ChangePasswordHandler":           "current password verified in this request",
	"auth/passkey.go:PasskeyHandler.FinishLoginHandler":        "webauthn assertion",
	"auth/passkey.go:PasskeyHandler.FinishSignupHandler":       "webauthn registration at signup",
	"auth/apple_auth.go:AppleAuthHandler.AppleCallbackHandler": "apple identity token, verified in this request",
}

// credentialIssuingMethods are the service methods a handler names when it
// hands a caller something that can act as the account afterwards: a session
// token, a bearer token, a feed token, a passkey. Matched by name alone, with
// no guess about the receiver, so a mention the walk cannot classify is
// reported rather than skipped.
//
// This is a HAND-MAINTAINED list of names, which is the check's real limit. A
// service that spells its issuance "Issue", "Mint" or "Rotate" is outside the
// walk until someone adds it here, and renaming an existing one out of the list
// makes its site silently disappear rather than fail. The list is the thing to
// review when a new credential type lands.
//
// AuthService.RefreshUserToken is deliberately absent: POST /auth/refresh
// carries the presented session's authentication time forward unchanged, so it
// hands back a token that grants exactly what the one it replaced granted. The
// sibling walk in services/auth is what holds that property.
var credentialIssuingMethods = map[string]bool{
	"CreateToken":                       true,
	"RenewSessionToken":                 true,
	"GenerateToken":                     true,
	"FinishRegistration":                true,
	"FinishSignupRegistrationWithLegal": true,
}

const gateFunc = "RequireRecentSessionAuth"

func TestCredentialMintsAskTheReauthGate(t *testing.T) {
	handlersRoot, err := filepath.Abs("..")
	if err != nil {
		t.Fatalf("resolving the handler tree: %v", err)
	}

	ungated := map[string]string{}
	gatedOrExempt := 0
	fset := token.NewFileSet()

	walkErr := filepath.Walk(handlersRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			if info.Name() == "testdata" {
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
		rel, relErr := filepath.Rel(handlersRoot, path)
		if relErr != nil {
			return relErr
		}
		rel = filepath.ToSlash(rel)

		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok {
				continue
			}

			var issues bool
			var gated bool
			var method string
			ast.Inspect(fn, func(n ast.Node) bool {
				sel, ok := n.(*ast.SelectorExpr)
				if !ok {
					return true
				}
				if credentialIssuingMethods[sel.Sel.Name] {
					issues, method = true, sel.Sel.Name
				}
				if sel.Sel.Name == gateFunc {
					gated = true
				}
				return true
			})
			// The gate is called unqualified inside this package, so the
			// selector walk above does not see it there.
			if !gated {
				gated = callsUnqualified(fn, gateFunc)
			}
			if !issues {
				continue
			}

			site := rel + ":" + funcName(fn)
			if _, exempt := mintGateExemptions[site]; exempt || gated {
				gatedOrExempt++
				continue
			}
			ungated[site] = method
		}
		return nil
	})
	if walkErr != nil {
		t.Fatalf("walking %s: %v", handlersRoot, walkErr)
	}

	// A walk that found nothing means the root is wrong, not that every mint was
	// deleted. Without this the stale-entry loop below would tell a reader to
	// delete the whole exemption list.
	if gatedOrExempt == 0 && len(ungated) == 0 {
		t.Fatalf("no credential-issuing sites found under %s; the walk root is wrong", handlersRoot)
	}

	var sites []string
	for site := range ungated {
		sites = append(sites, site)
	}
	sort.Strings(sites)
	for _, site := range sites {
		t.Errorf("%s names %s and does not call shared.%s.\n"+
			"A credential issued on the strength of a session the caller already holds\n"+
			"must ask the gate first. If an authentication factor completed in this\n"+
			"request instead, add the site to mintGateExemptions with the factor named.",
			site, ungated[site], gateFunc)
	}

	for site := range mintGateExemptions {
		if !siteExists(t, handlersRoot, site) {
			t.Errorf("mintGateExemptions names %s, which no longer issues a credential. Remove the entry.", site)
		}
	}
}

// callsUnqualified reports whether fn names ident as a bare function call,
// which is how a function in this package reaches the gate.
func callsUnqualified(fn *ast.FuncDecl, ident string) bool {
	found := false
	ast.Inspect(fn, func(n ast.Node) bool {
		if id, ok := n.(*ast.Ident); ok && id.Name == ident {
			found = true
		}
		return true
	})
	return found
}

// funcName is "Receiver.Name" for a method and "Name" for a plain function, so
// a site reads the way a stack trace does.
func funcName(fn *ast.FuncDecl) string {
	if fn.Recv == nil || len(fn.Recv.List) == 0 {
		return fn.Name.Name
	}
	t := fn.Recv.List[0].Type
	if star, ok := t.(*ast.StarExpr); ok {
		t = star.X
	}
	if id, ok := t.(*ast.Ident); ok {
		return id.Name + "." + fn.Name.Name
	}
	return fn.Name.Name
}

// siteExists reports whether a "file:Receiver.Name" site still names a
// credential-issuing method, so a stale exemption is reported rather than
// silently excusing nothing.
func siteExists(t *testing.T, root, site string) bool {
	t.Helper()
	rel, name, ok := strings.Cut(site, ":")
	if !ok {
		return false
	}
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, filepath.Join(root, rel), nil, 0)
	if err != nil {
		return false
	}
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || funcName(fn) != name {
			continue
		}
		issues := false
		ast.Inspect(fn, func(n ast.Node) bool {
			if sel, ok := n.(*ast.SelectorExpr); ok && credentialIssuingMethods[sel.Sel.Name] {
				issues = true
			}
			return true
		})
		return issues
	}
	return false
}
