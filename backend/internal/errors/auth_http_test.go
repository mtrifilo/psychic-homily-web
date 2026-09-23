package errors

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/humatest"
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
		got := (&AuthError{Code: code}).GetStatus()
		if got != want {
			t.Errorf("GetStatus(%s) = %d, want %d", code, got, want)
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
	if got := (&AuthError{Code: "NOT_A_DECLARED_CODE"}).GetStatus(); got != http.StatusInternalServerError {
		t.Errorf("GetStatus for an undeclared code = %d, want 500", got)
	}
}

type decodedAuthBody struct {
	Success   *bool   `json:"success"`
	Message   *string `json:"message"`
	ErrorCode *string `json:"error_code"`
	RequestID *string `json:"request_id"`
}

func TestAuthErrorBodyCarriesOnlyTheExternalCodeAndMessage(t *testing.T) {
	for _, code := range declaredAuthCodes(t) {
		t.Run(code, func(t *testing.T) {
			authErr := NewAuthError(code, "user-facing copy", fmt.Errorf("lookup: %s", driverText))

			raw, err := json.Marshal(authErr)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			body := string(raw)
			for _, leaked := range []string{"user_bookmarks", "SQLSTATE", "lookup:", "internal"} {
				if strings.Contains(body, leaked) {
					t.Errorf("body contains %q: %s", leaked, body)
				}
			}

			var fields map[string]any
			if err := json.Unmarshal(raw, &fields); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			for key := range fields {
				switch key {
				case "success", "message", "error_code":
				default:
					t.Errorf("unexpected body field %q: %s", key, body)
				}
			}

			var decoded decodedAuthBody
			if err := json.Unmarshal(raw, &decoded); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if decoded.Success == nil || *decoded.Success {
				t.Errorf("success must be present and false: %s", body)
			}
			wantCode := ToExternalCode(code)
			if decoded.ErrorCode == nil || *decoded.ErrorCode != wantCode {
				t.Errorf("error_code = %v, want %q", decoded.ErrorCode, wantCode)
			}
			wantMessage := "user-facing copy"
			if wantCode != code {
				wantMessage = ToExternalMessage(wantCode)
			}
			if decoded.Message == nil || *decoded.Message != wantMessage {
				t.Errorf("message = %v, want %q", decoded.Message, wantMessage)
			}
		})
	}
}

func TestAuthErrorBodyCarriesTheRequestIDWhenSet(t *testing.T) {
	raw, err := json.Marshal(ErrServiceUnavailable("export_data", fmt.Errorf("%s", driverText)).WithRequestID("req-123"))
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded decodedAuthBody
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded.RequestID == nil || *decoded.RequestID != "req-123" {
		t.Errorf("request_id = %v, want req-123 (body: %s)", decoded.RequestID, raw)
	}
}

// An unknown address and a wrong password must be indistinguishable on the
// wire: same status, same bytes.
func TestAuthErrorUserNotFoundIsIndistinguishableFromInvalidCredentials(t *testing.T) {
	notFound := ErrUserNotFound("someone@example.com")
	invalid := ErrInvalidCredentials(nil)

	if notFound.GetStatus() != invalid.GetStatus() {
		t.Errorf("status differs: user-not-found %d, invalid-credentials %d", notFound.GetStatus(), invalid.GetStatus())
	}
	notFoundBody, _ := json.Marshal(notFound)
	invalidBody, _ := json.Marshal(invalid)
	if !bytes.Equal(notFoundBody, invalidBody) {
		t.Errorf("body differs:\n user-not-found:      %s\n invalid-credentials: %s", notFoundBody, invalidBody)
	}
}

// MarshalJSON makes AuthError a json.Marshaler, which slog's JSON handler
// prefers over Error(). The internal chain must still reach the log.
func TestAuthErrorLogsKeepTheInternalChain(t *testing.T) {
	authErr := ErrServiceUnavailable("export_data", fmt.Errorf("%s", driverText))

	for name, value := range map[string]any{
		"direct":  authErr,
		"wrapped": fmt.Errorf("handler: %w", authErr),
	} {
		t.Run(name, func(t *testing.T) {
			var buf bytes.Buffer
			slog.New(slog.NewJSONHandler(&buf, nil)).Error("export_data_failed", slog.Any("error", value))
			if !strings.Contains(buf.String(), "user_bookmarks") {
				t.Errorf("JSON log line lost the internal chain: %s", buf.String())
			}
			buf.Reset()
			slog.New(slog.NewTextHandler(&buf, nil)).Error("export_data_failed", "error", value)
			if !strings.Contains(buf.String(), "user_bookmarks") {
				t.Errorf("text log line lost the internal chain: %s", buf.String())
			}
		})
	}
}

// The properties above hold for the value; this drives a registered huma
// operation to prove huma writes that value, at that status, on the wire.
func TestAuthErrorOverTheWire(t *testing.T) {
	_, api := humatest.New(t, huma.DefaultConfig("auth-error-wire", "1.0.0"))

	cases := []struct {
		name       string
		path       string
		err        error
		wantStatus int
		wantCode   string
	}{
		{
			name:       "direct",
			path:       "/direct",
			err:        ErrServiceUnavailable("export_data", fmt.Errorf("%s", driverText)),
			wantStatus: http.StatusServiceUnavailable,
			wantCode:   CodeServiceUnavailable,
		},
		{
			name:       "wrapped",
			path:       "/wrapped",
			err:        fmt.Errorf("handler: %w", ErrUsernameTaken(fmt.Errorf("%s", driverText))),
			wantStatus: http.StatusConflict,
			wantCode:   CodeUsernameTaken,
		},
	}
	for _, tc := range cases {
		huma.Register(api, huma.Operation{
			OperationID: "auth-error-" + tc.name,
			Method:      http.MethodGet,
			Path:        tc.path,
		}, func(context.Context, *struct{}) (*struct{}, error) {
			return nil, tc.err
		})
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp := api.Get(tc.path)
			if resp.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d; body: %s", resp.Code, tc.wantStatus, resp.Body.String())
			}
			if ct := resp.Header().Get("Content-Type"); ct != "application/json" {
				t.Errorf("Content-Type = %q, want application/json", ct)
			}
			body := resp.Body.String()
			for _, leaked := range []string{"user_bookmarks", "SQLSTATE", "internal"} {
				if strings.Contains(body, leaked) {
					t.Errorf("wire body contains %q: %s", leaked, body)
				}
			}
			var decoded decodedAuthBody
			if err := json.Unmarshal(resp.Body.Bytes(), &decoded); err != nil {
				t.Fatalf("decode: %v (body: %s)", err, body)
			}
			if decoded.ErrorCode == nil || *decoded.ErrorCode != tc.wantCode {
				t.Errorf("error_code = %v, want %q (body: %s)", decoded.ErrorCode, tc.wantCode, body)
			}
			if decoded.Message == nil || *decoded.Message == "" {
				t.Errorf("message missing (body: %s)", body)
			}
		})
	}
}
