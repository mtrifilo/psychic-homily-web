package errors

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"net/http"
	"strconv"
	"strings"
	"testing"
)

// driverText stands in for what a wrapped database error carries. None of it
// may reach a response body.
const driverText = `ERROR: relation "user_bookmarks" does not exist (SQLSTATE 42P01)`

// wantAuthStatus is the deliberate status for every declared code. It is
// spelled out rather than read from authCodeHTTPStatus so that changing a
// status is a change to this table too.
var wantAuthStatus = map[string]int{
	CodeInvalidCredentials:         http.StatusUnauthorized,
	CodeUserNotFound:               http.StatusUnauthorized,
	CodeTokenExpired:               http.StatusUnauthorized,
	CodeTokenInvalid:               http.StatusUnauthorized,
	CodeTokenMissing:               http.StatusUnauthorized,
	CodeServiceUnavailable:         http.StatusServiceUnavailable,
	CodeUserExists:                 http.StatusConflict,
	CodeValidationFailed:           http.StatusBadRequest,
	CodeUnauthorized:               http.StatusForbidden,
	CodeUnknown:                    http.StatusInternalServerError,
	CodeAccountLocked:              http.StatusLocked,
	CodeAccountInactive:            http.StatusForbidden,
	CodeNoPasswordSet:              http.StatusConflict,
	CodeTermsAcceptanceRequired:    http.StatusUnprocessableEntity,
	CodeAgeConfirmationRequired:    http.StatusUnprocessableEntity,
	CodeInvalidReplyPermission:     http.StatusBadRequest,
	CodeUsernameTaken:              http.StatusConflict,
	CodeOAuthLinkRefused:           http.StatusConflict,
	CodeOAuthIdentityInUse:         http.StatusConflict,
	CodeOAuthProviderAlreadyLinked: http.StatusConflict,
	CodeOAuthLinkExpired:           http.StatusBadRequest,
	CodeReauthRequired:             http.StatusForbidden,
	CodeUnknownHomeMetro:           http.StatusUnprocessableEntity,
}

// declaredAuthCodes returns the value of every Code* string constant declared
// in auth.go, so a code added there without a status fails the tests below.
func declaredAuthCodes(t *testing.T) []string {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), "auth.go", nil, 0)
	if err != nil {
		t.Fatalf("parsing auth.go: %v", err)
	}
	var codes []string
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.CONST {
			continue
		}
		for _, spec := range gen.Specs {
			vs := spec.(*ast.ValueSpec)
			for i, name := range vs.Names {
				if !strings.HasPrefix(name.Name, "Code") || i >= len(vs.Values) {
					continue
				}
				lit, ok := vs.Values[i].(*ast.BasicLit)
				if !ok || lit.Kind != token.STRING {
					continue
				}
				value, err := strconv.Unquote(lit.Value)
				if err != nil {
					t.Fatalf("unquoting %s: %v", name.Name, err)
				}
				codes = append(codes, value)
			}
		}
	}
	if len(codes) == 0 {
		t.Fatal("found no Code* constants in auth.go; the parser walk is broken")
	}
	return codes
}

func TestAuthErrorEveryCodeHasADeliberateStatus(t *testing.T) {
	codes := declaredAuthCodes(t)
	for _, code := range codes {
		want, ok := wantAuthStatus[code]
		if !ok {
			t.Errorf("code %s has no expected status in wantAuthStatus", code)
			continue
		}
		if _, ok := authCodeHTTPStatus[code]; !ok {
			t.Errorf("code %s has no entry in authCodeHTTPStatus and would answer 500 by default", code)
		}
		got := (&AuthError{Code: code}).HTTPStatus()
		if got != want {
			t.Errorf("HTTPStatus(%s) = %d, want %d", code, got, want)
		}
	}
	if len(wantAuthStatus) != len(codes) {
		t.Errorf("wantAuthStatus has %d entries, auth.go declares %d codes", len(wantAuthStatus), len(codes))
	}
	if len(authCodeHTTPStatus) != len(codes) {
		t.Errorf("authCodeHTTPStatus has %d entries, auth.go declares %d codes", len(authCodeHTTPStatus), len(codes))
	}
}

func TestAuthErrorUndeclaredCodeAnswers500(t *testing.T) {
	if got := (&AuthError{Code: "NOT_A_DECLARED_CODE"}).HTTPStatus(); got != http.StatusInternalServerError {
		t.Errorf("HTTPStatus for an undeclared code = %d, want 500", got)
	}
}

// huma marshals a returned StatusError as the response body. AuthError must
// not become one, or its exported fields, Internal included, reach the client
// instead of the envelope the API layer builds from it.
func TestAuthErrorIsNotAHumaStatusError(t *testing.T) {
	var err error = ErrServiceUnavailable("export_data", fmt.Errorf("%s", driverText))
	if _, ok := err.(interface{ GetStatus() int }); ok {
		t.Fatal("*AuthError has a GetStatus method; huma would marshal it as the response body")
	}
}

func TestAuthErrorExternalMessage(t *testing.T) {
	for _, code := range declaredAuthCodes(t) {
		authErr := NewAuthError(code, "user-facing copy", fmt.Errorf("%s", driverText))
		want := "user-facing copy"
		if external := ToExternalCode(code); external != code {
			want = ToExternalMessage(external)
		}
		if got := authErr.ExternalMessage(); got != want {
			t.Errorf("ExternalMessage(%s) = %q, want %q", code, got, want)
		}
	}
}

// An unknown address and a wrong password must be indistinguishable to the
// client: same status, same external code, same message.
func TestAuthErrorUserNotFoundIsIndistinguishableFromInvalidCredentials(t *testing.T) {
	notFound := ErrUserNotFound("someone@example.com")
	invalid := ErrInvalidCredentials(nil)

	if notFound.HTTPStatus() != invalid.HTTPStatus() {
		t.Errorf("status differs: user-not-found %d, invalid-credentials %d", notFound.HTTPStatus(), invalid.HTTPStatus())
	}
	if ToExternalCode(notFound.Code) != ToExternalCode(invalid.Code) {
		t.Errorf("external code differs: %q vs %q", ToExternalCode(notFound.Code), ToExternalCode(invalid.Code))
	}
	if notFound.ExternalMessage() != invalid.ExternalMessage() {
		t.Errorf("external message differs: %q vs %q", notFound.ExternalMessage(), invalid.ExternalMessage())
	}
}
