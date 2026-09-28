package humaerr

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/humatest"

	autherrors "psychic-homily-backend/internal/errors"
	"psychic-homily-backend/internal/logger"
	"psychic-homily-backend/internal/testlog"
)

// driverText stands in for what a wrapped database error carries. None of it
// may reach a response body, and all of it must reach the log.
const driverText = `ERROR: relation "user_bookmarks" does not exist (SQLSTATE 42P01)`

const testRequestID = "req-humaerr-test"

// testSecret stands in for a credential an upstream error can echo. It may
// reach neither the body nor the log.
const testSecret = "hunter2-humaerr-secret"

func errDriver() error { return fmt.Errorf("lookup: %s", driverText) }

// installForTest installs the constructors and restores huma's defaults when
// the test ends, so the process-global swap does not leak into other tests.
func installForTest(t *testing.T) {
	t.Helper()
	prevNewError, prevNewErrorWithContext := huma.NewError, huma.NewErrorWithContext
	t.Cleanup(func() {
		huma.NewError = prevNewError
		huma.NewErrorWithContext = prevNewErrorWithContext
	})
	Install()
}

type createBody struct {
	Body struct {
		Name string `json:"name" minLength:"1"`
	}
}

// newTestAPI registers one operation per behaviour under test. Each request
// carries what middleware.HumaRequestIDMiddleware gives it in production, a
// request ID and a logger bound to it, except that the logger writes wherever
// slog's default points, which is the capture buffer inside testlog.Capture.
func newTestAPI(t *testing.T) humatest.TestAPI {
	t.Helper()
	installForTest(t)
	_, api := humatest.New(t, huma.DefaultConfig("humaerr-test", "1.0.0"))
	api.UseMiddleware(func(ctx huma.Context, next func(huma.Context)) {
		reqCtx := logger.SetRequestID(ctx.Context(), testRequestID)
		reqCtx = logger.NewContext(reqCtx, logger.WithRequestID(slog.Default(), testRequestID))
		next(huma.WithContext(ctx, reqCtx))
	})

	// Each handler builds its error per request, as production handlers do, so
	// the huma.ErrorNNN helpers run inside the request and its log capture.
	register := func(path string, build func() error) {
		huma.Register(api, huma.Operation{
			OperationID: strings.TrimPrefix(path, "/"),
			Method:      http.MethodGet,
			Path:        path,
		}, func(context.Context, *struct{}) (*struct{}, error) {
			return nil, build()
		})
	}
	register("/raw", errDriver)
	register("/raw-secret", func() error {
		return fmt.Errorf("dial upstream: password=%s", testSecret)
	})
	register("/error500-detail", func() error {
		return huma.Error500InternalServerError("Failed to load things", errDriver())
	})
	register("/error400-detail", func() error {
		return huma.Error400BadRequest("Bad things", errDriver())
	})
	register("/structured-detail", func() error {
		return huma.Error422UnprocessableEntity("validation failed", &huma.ErrorDetail{
			Location: "body.music_at",
			Message:  "music_at cannot be before doors_at",
		})
	})
	register("/auth-direct", func() error {
		return autherrors.ErrServiceUnavailable("export_data", errDriver())
	})
	register("/auth-wrapped", func() error {
		return fmt.Errorf("handler: %w", autherrors.ErrUsernameTaken(errDriver()))
	})
	register("/auth-user-not-found", func() error {
		return autherrors.ErrUserNotFoundByID(7, errDriver())
	})

	// huma.WriteErr carries its caller's status and message, so an AuthError
	// argument there is withheld like any other, not turned into the envelope.
	// Each path shares one half of huma's unresolved-handler-error arguments
	// with it, so each half of that check is pinned on its own.
	for path, call := range map[string]struct {
		status int
		msg    string
	}{
		"/write-err":                  {http.StatusUnprocessableEntity, "validation failed"},
		"/write-err-500":              {http.StatusInternalServerError, "Unable to get resource"},
		"/write-err-fallback-message": {http.StatusUnprocessableEntity, unresolvedHandlerErrorMessage},
	} {
		huma.Register(api, huma.Operation{
			OperationID: strings.TrimPrefix(path, "/"),
			Method:      http.MethodGet,
			Path:        path,
			Middlewares: huma.Middlewares{func(ctx huma.Context, _ func(huma.Context)) {
				_ = huma.WriteErr(api, ctx, call.status, call.msg,
					autherrors.ErrUserExists("someone@example.com"))
			}},
		}, func(context.Context, *struct{}) (*struct{}, error) {
			return &struct{}{}, nil
		})
	}

	huma.Register(api, huma.Operation{
		OperationID: "create",
		Method:      http.MethodPost,
		Path:        "/create",
	}, func(context.Context, *createBody) (*struct{}, error) {
		return &struct{}{}, nil
	})
	return api
}

// get performs the request inside a log capture and returns the response and
// everything logged while it ran.
func get(t *testing.T, api humatest.TestAPI, path string) (code int, contentType, body, logs string) {
	t.Helper()
	logs = testlog.Capture(t, func() {
		resp := api.Get(path)
		code, contentType, body = resp.Code, resp.Header().Get("Content-Type"), resp.Body.String()
	})
	return code, contentType, body, logs
}

func assertNoDriverText(t *testing.T, body string) {
	t.Helper()
	for _, leaked := range []string{"user_bookmarks", "SQLSTATE", "lookup:", "(internal:"} {
		if strings.Contains(body, leaked) {
			t.Errorf("body contains %q: %s", leaked, body)
		}
	}
}

func assertLoggedWithRequestID(t *testing.T, logs string) {
	t.Helper()
	if !strings.Contains(logs, "user_bookmarks") {
		t.Errorf("the withheld error text was not logged; logs:\n%s", logs)
	}
	if !strings.Contains(logs, testRequestID) {
		t.Errorf("the log line does not carry the request ID; logs:\n%s", logs)
	}
}

func decodeModel(t *testing.T, body string) huma.ErrorModel {
	t.Helper()
	var model huma.ErrorModel
	if err := json.Unmarshal([]byte(body), &model); err != nil {
		t.Fatalf("decode: %v (body: %s)", err, body)
	}
	return model
}

func TestUnmappedHandlerErrorIsWithheldAndLogged(t *testing.T) {
	api := newTestAPI(t)
	code, contentType, body, logs := get(t, api, "/raw")

	if code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500; body: %s", code, body)
	}
	if contentType != "application/problem+json" {
		t.Errorf("Content-Type = %q, want application/problem+json", contentType)
	}
	assertNoDriverText(t, body)
	model := decodeModel(t, body)
	if model.Detail != "unexpected error occurred" || len(model.Errors) != 0 {
		t.Errorf("want huma's generic detail and no errors[]; body: %s", body)
	}
	assertLoggedWithRequestID(t, logs)
}

// Withheld text is logged through observability.ScrubText, so a credential an
// upstream error echoes stays out of the log as well as the body.
func TestWithheldTextIsScrubbedBeforeLogging(t *testing.T) {
	api := newTestAPI(t)
	code, _, body, logs := get(t, api, "/raw-secret")

	if code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500; body: %s", code, body)
	}
	if strings.Contains(body, testSecret) {
		t.Errorf("body carries the secret: %s", body)
	}
	if !strings.Contains(logs, "dial upstream") {
		t.Fatalf("the withheld error was not logged; logs:\n%s", logs)
	}
	if strings.Contains(logs, testSecret) {
		t.Errorf("log carries the secret unscrubbed; logs:\n%s", logs)
	}
}

// The huma.ErrorNNN helpers run in handler code with no request context, so
// only the text itself is asserted in the log.
func TestRawErrorDetailIsWithheldAndLogged(t *testing.T) {
	api := newTestAPI(t)

	for path, want := range map[string]int{
		"/error500-detail": http.StatusInternalServerError,
		"/error400-detail": http.StatusBadRequest,
	} {
		t.Run(path, func(t *testing.T) {
			code, _, body, logs := get(t, api, path)

			if code != want {
				t.Fatalf("status = %d, want %d; body: %s", code, want, body)
			}
			assertNoDriverText(t, body)
			model := decodeModel(t, body)
			if model.Detail == "" || len(model.Errors) != 0 {
				t.Errorf("want the handler's detail message and no errors[]; body: %s", body)
			}
			if !strings.Contains(logs, "user_bookmarks") {
				t.Errorf("the withheld error text was not logged; logs:\n%s", logs)
			}
		})
	}
}

func TestStructuredDetailsReachTheBody(t *testing.T) {
	api := newTestAPI(t)

	t.Run("handler-built ErrorDetail", func(t *testing.T) {
		code, _, body, _ := get(t, api, "/structured-detail")
		if code != http.StatusUnprocessableEntity {
			t.Fatalf("status = %d, want 422; body: %s", code, body)
		}
		model := decodeModel(t, body)
		if len(model.Errors) != 1 || model.Errors[0].Location != "body.music_at" {
			t.Errorf("want the handler's ErrorDetail in errors[]; body: %s", body)
		}
	})

	t.Run("huma request validation", func(t *testing.T) {
		resp := api.Post("/create", map[string]any{"name": ""})
		if resp.Code != http.StatusUnprocessableEntity {
			t.Fatalf("status = %d, want 422; body: %s", resp.Code, resp.Body.String())
		}
		model := decodeModel(t, resp.Body.String())
		if len(model.Errors) == 0 || model.Errors[0].Location == "" {
			t.Errorf("want huma's located validation details; body: %s", resp.Body.String())
		}
	})
}

func TestWriteErrKeepsItsStatusAndWithholdsAnAuthError(t *testing.T) {
	api := newTestAPI(t)

	for path, want := range map[string]struct {
		status int
		detail string
	}{
		"/write-err":                  {http.StatusUnprocessableEntity, "validation failed"},
		"/write-err-500":              {http.StatusInternalServerError, "Unable to get resource"},
		"/write-err-fallback-message": {http.StatusUnprocessableEntity, unresolvedHandlerErrorMessage},
	} {
		t.Run(path, func(t *testing.T) {
			code, contentType, body, logs := get(t, api, path)

			if code != want.status {
				t.Fatalf("status = %d, want the caller's %d; body: %s", code, want.status, body)
			}
			if contentType != "application/problem+json" {
				t.Errorf("Content-Type = %q, want application/problem+json", contentType)
			}
			model := decodeModel(t, body)
			if model.Detail != want.detail || len(model.Errors) != 0 {
				t.Errorf("want the caller's detail and no errors[]; body: %s", body)
			}
			if strings.Contains(body, "USER_EXISTS") || strings.Contains(body, "already exists") {
				t.Errorf("body carries the AuthError: %s", body)
			}
			if !strings.Contains(logs, "USER_EXISTS") {
				t.Errorf("the withheld AuthError was not logged; logs:\n%s", logs)
			}
		})
	}
}

// A context without a logger falls back to slog.Default() itself, not to
// logger.Default(), which would run logger.Init and replace slog.Default().
func TestRequestLoggerFallsBackToSlogDefault(t *testing.T) {
	before := slog.Default()
	if got := requestLogger(context.Background()); got != before {
		t.Error("requestLogger without a context logger did not return slog.Default()")
	}
	if slog.Default() != before {
		t.Error("requestLogger replaced slog.Default()")
	}
}

type authEnvelope struct {
	Success   *bool   `json:"success"`
	Message   *string `json:"message"`
	ErrorCode *string `json:"error_code"`
	RequestID *string `json:"request_id"`
}

func TestAuthErrorAnswersWithItsStatusAndTheAuthEnvelope(t *testing.T) {
	api := newTestAPI(t)

	cases := []struct {
		path        string
		wantStatus  int
		wantCode    string
		wantMessage string
	}{
		{"/auth-direct", http.StatusServiceUnavailable, autherrors.CodeServiceUnavailable, "Service temporarily unavailable"},
		{"/auth-wrapped", http.StatusConflict, autherrors.CodeUsernameTaken, "Username is already taken"},
		{"/auth-user-not-found", http.StatusUnauthorized, autherrors.CodeInvalidCredentials, "Invalid email or password"},
	}
	for _, tc := range cases {
		t.Run(tc.path, func(t *testing.T) {
			code, contentType, body, logs := get(t, api, tc.path)

			if code != tc.wantStatus {
				t.Fatalf("status = %d, want %d; body: %s", code, tc.wantStatus, body)
			}
			if contentType != "application/json" {
				t.Errorf("Content-Type = %q, want application/json", contentType)
			}
			assertNoDriverText(t, body)
			if strings.Contains(body, "USER_NOT_FOUND") || strings.Contains(body, "User not found") {
				t.Errorf("body names the concealed code: %s", body)
			}

			var fields map[string]any
			if err := json.Unmarshal([]byte(body), &fields); err != nil {
				t.Fatalf("decode: %v (body: %s)", err, body)
			}
			for key := range fields {
				switch key {
				case "success", "message", "error_code", "request_id":
				default:
					t.Errorf("unexpected body field %q: %s", key, body)
				}
			}

			var env authEnvelope
			if err := json.Unmarshal([]byte(body), &env); err != nil {
				t.Fatalf("decode: %v (body: %s)", err, body)
			}
			if env.Success == nil || *env.Success {
				t.Errorf("success must be present and false: %s", body)
			}
			if env.ErrorCode == nil || *env.ErrorCode != tc.wantCode {
				t.Errorf("error_code = %v, want %q", env.ErrorCode, tc.wantCode)
			}
			if env.Message == nil || *env.Message != tc.wantMessage {
				t.Errorf("message = %v, want %q", env.Message, tc.wantMessage)
			}
			if env.RequestID == nil || *env.RequestID != testRequestID {
				t.Errorf("request_id = %v, want %q", env.RequestID, testRequestID)
			}
			assertLoggedWithRequestID(t, logs)
		})
	}
}

// huma derives the documented error schema from what huma.NewError returns, so
// it must stay *huma.ErrorModel or the published spec changes.
func TestDocumentedErrorSchemaIsHumaErrorModel(t *testing.T) {
	installForTest(t)
	if _, ok := huma.NewError(0, "").(*huma.ErrorModel); !ok {
		t.Fatalf("huma.NewError returns %T, want *huma.ErrorModel", huma.NewError(0, ""))
	}
}
