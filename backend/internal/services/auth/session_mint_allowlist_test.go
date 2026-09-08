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

// CreateToken stamps auth_at with the current time, so naming it asserts that
// an authentication factor just completed. RenewSessionToken takes the time
// from its caller, so passing time.Now() there asserts the same thing while
// looking like a renewal. A path that issues a session on the strength of a
// session the caller already holds must do neither, or it hands that caller a
// way to manufacture the freshness the re-authentication gates require.
//
// Nothing in the type system says so, so this test does: it parses every
// non-test .go file under backend/ for mentions of either method and fails when
// the set differs from the allowlist below.
//
// The walk matches the method NAME alone and makes no guess about the receiver,
// so a mention it cannot classify is reported rather than skipped. That is why
// unrelated services' CreateToken sites are listed here too: naming them is
// what lets the check be total. It matches selectors whether or not they are
// immediately called, so taking a method value does not evade it.
//
// What it does NOT do is follow calls: it sees the site that names the method,
// not every path that can reach that site. A new caller of an already-listed
// wrapper (AppleAuthService.GenerateToken, AuthService.OAuthCallback) is not
// reported here and is on the author.
//
// If you add a site, choose one:
//   - an authentication factor completed in this request (password, passkey,
//     magic link, provider sign-in, account recovery): name CreateToken and add
//     the site here with the factor
//   - anything else: name RenewSessionToken and pass the authentication time
//     the caller's own credential established, never time.Now()
var sessionMintAllowlist = map[string]string{
	// Session mints behind a real factor.
	"internal/api/handlers/auth/auth.go:AuthHandler.LoginHandler:CreateToken":                  "password",
	"internal/api/handlers/auth/auth.go:AuthHandler.RegisterHandler:CreateToken":               "registration sets the password",
	"internal/api/handlers/auth/auth.go:AuthHandler.VerifyMagicLinkHandler:CreateToken":        "emailed magic link",
	"internal/api/handlers/auth/auth.go:AuthHandler.RecoverAccountHandler:CreateToken":         "email plus password",
	"internal/api/handlers/auth/auth.go:AuthHandler.ChangePasswordHandler:CreateToken":         "current password verified in this request",
	"internal/api/handlers/auth/auth.go:AuthHandler.ConfirmAccountRecoveryHandler:CreateToken": "emailed recovery token",
	"internal/api/handlers/auth/passkey.go:PasskeyHandler.FinishLoginHandler:CreateToken":      "webauthn assertion",
	"internal/api/handlers/auth/passkey.go:PasskeyHandler.FinishSignupHandler:CreateToken":     "webauthn registration at signup",
	"internal/services/auth/apple.go:AppleAuthService.GenerateToken:CreateToken":               "apple identity token, verified by AppleCallbackHandler before this runs",
	"internal/services/auth/oauth.go:AuthService.oauthCallbackInternal:CreateToken":            "provider handshake",

	// Session renewals: the authentication time comes from the caller's own
	// credential, so these mint no freshness.
	"internal/api/handlers/auth/auth.go:AuthHandler.GenerateCLITokenHandler:RenewSessionToken": "renewal, time from the request context",
	"internal/services/auth/oauth.go:AuthService.RefreshUserToken:RenewSessionToken":           "renewal, time from the caller",

	// The one deliberate time.Now() stamp, which is what CreateToken means.
	"internal/services/auth/jwt.go:JWTService.CreateToken:RenewSessionToken": "the factor stamp itself",

	// A test helper, which the walk sees because it lives in a non-test file so
	// that every handler package can import it. It renews with a time its
	// caller supplies and reaches no request.
	"internal/api/handlers/shared/testhelpers/session_auth_time.go:SessionAuthTimeAfterRenewal:RenewSessionToken": "test helper, time from the caller",

	// Unrelated services that happen to expose a CreateToken. Named so the walk
	// needs no receiver heuristic to leave them alone.
	"cmd/gen-api-token/main.go:main:CreateToken":                                                          "admin API token, not a session",
	"internal/api/handlers/admin/admin_tokens.go:AdminTokenHandler.CreateAPITokenHandler:CreateToken":     "admin API token, not a session",
	"internal/api/handlers/engagement/calendar.go:CalendarHandler.CreateCalendarTokenHandler:CreateToken": "calendar feed token, not a session",
}

// mintedNames are the methods the walk looks for.
var mintedNames = map[string]bool{"CreateToken": true, "RenewSessionToken": true}

// factorStampSite is the single site allowed to pass time.Now() as an
// authentication time, because that is the definition of a factor stamp.
const factorStampSite = "internal/services/auth/jwt.go:JWTService.CreateToken:RenewSessionToken"

func TestSessionMintsAreOnlyMadeByAuthenticationFactors(t *testing.T) {
	backendRoot, err := filepath.Abs("../../..")
	if err != nil {
		t.Fatalf("resolving backend root: %v", err)
	}

	found := map[string]bool{}
	stampsNow := map[string]bool{}
	fset := token.NewFileSet()

	walkErr := filepath.Walk(backendRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			// testdata is skipped the way the go tool skips it: fixtures there
			// need not parse, and a broken one is not this test's business.
			if name := info.Name(); name == "vendor" || name == "node_modules" || name == ".git" || name == "testdata" {
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

		// Walked per declaration, so the enclosing name is always the function
		// the mention sits in rather than whichever one was seen last.
		for _, decl := range file.Decls {
			enclosing := "<file scope>"
			if fn, ok := decl.(*ast.FuncDecl); ok {
				enclosing = funcDeclName(fn)
			}
			ast.Inspect(decl, func(n ast.Node) bool {
				// Selectors, not calls: `mint := x.CreateToken` names the
				// method just as surely as calling it.
				if sel, ok := n.(*ast.SelectorExpr); ok && mintedNames[sel.Sel.Name] {
					found[rel+":"+enclosing+":"+sel.Sel.Name] = true
				}
				if call, ok := n.(*ast.CallExpr); ok {
					if sel, ok := call.Fun.(*ast.SelectorExpr); ok &&
						sel.Sel.Name == "RenewSessionToken" && callStampsNow(call) {
						stampsNow[rel+":"+enclosing+":"+sel.Sel.Name] = true
					}
				}
				return true
			})
		}
		return nil
	})
	if walkErr != nil {
		t.Fatalf("walking %s: %v", backendRoot, walkErr)
	}

	// A walk that found nothing means the root is wrong, not that every mint was
	// deleted. Without this the stale-entry loop below would tell a reader to
	// delete the whole allowlist.
	if len(found) == 0 {
		t.Fatalf("no session-minting sites found under %s; the walk root is wrong", backendRoot)
	}

	var unknown []string
	for site := range found {
		if _, ok := sessionMintAllowlist[site]; !ok {
			unknown = append(unknown, site)
		}
	}
	sort.Strings(unknown)
	for _, site := range unknown {
		t.Errorf("%s names a session-minting method and is not in sessionMintAllowlist.\n"+
			"If an authentication factor completed here, add it with the factor named.\n"+
			"If this renews a session the caller already holds, name RenewSessionToken\n"+
			"and pass the authentication time from that caller's credential.", site)
	}

	var nowStamps []string
	for site := range stampsNow {
		if site != factorStampSite {
			nowStamps = append(nowStamps, site)
		}
	}
	sort.Strings(nowStamps)
	for _, site := range nowStamps {
		t.Errorf("%s calls RenewSessionToken with time.Now(), which stamps a fresh\n"+
			"authentication time onto a session that completed no factor. Pass the time\n"+
			"the caller's own credential established, or call CreateToken if a factor\n"+
			"really did complete here.", site)
	}

	var stale []string
	for site := range sessionMintAllowlist {
		if !found[site] {
			stale = append(stale, site)
		}
	}
	sort.Strings(stale)
	for _, site := range stale {
		t.Errorf("sessionMintAllowlist lists %s, which no longer names a session-minting method; remove the entry", site)
	}
}

// funcDeclName renders a declaration as Receiver.Name, or just Name for a plain
// function, so two methods of the same name in one file stay distinct.
func funcDeclName(fn *ast.FuncDecl) string {
	if fn.Recv == nil || len(fn.Recv.List) == 0 {
		return fn.Name.Name
	}
	return receiverTypeName(fn.Recv.List[0].Type) + "." + fn.Name.Name
}

func receiverTypeName(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.StarExpr:
		return receiverTypeName(t.X)
	case *ast.Ident:
		return t.Name
	case *ast.IndexExpr:
		return receiverTypeName(t.X)
	}
	return "?"
}

// callStampsNow reports whether the call's authAt argument is time.Now().
func callStampsNow(call *ast.CallExpr) bool {
	if len(call.Args) < 2 {
		return false
	}
	inner, ok := call.Args[1].(*ast.CallExpr)
	if !ok {
		return false
	}
	sel, ok := inner.Fun.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "Now" {
		return false
	}
	pkg, ok := sel.X.(*ast.Ident)
	return ok && pkg.Name == "time"
}
