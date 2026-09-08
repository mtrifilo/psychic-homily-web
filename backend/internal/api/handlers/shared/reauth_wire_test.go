package shared_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/humatest"

	"psychic-homily-backend/internal/api/handlers/shared"
	"psychic-homily-backend/internal/api/middleware"
	autherrors "psychic-homily-backend/internal/errors"
)

// Everything the surfaces do with the refusal hangs on one property of huma:
// that a StatusError is written as the response body at its own status rather
// than wrapped in huma's ErrorModel. Marshalling the value proves encoding/json
// can render it, not that huma puts it on the wire, so this drives a registered
// operation and reads the response.
//
// If huma changed that, every unit test here and every frontend test would
// still pass while the feature was dead: the frontend reads error_code from the
// body, and an ErrorModel carries no such field.
func TestReauthRefusalOverTheWire(t *testing.T) {
	_, api := humatest.New(t, huma.DefaultConfig("reauth-wire", "1.0.0"))

	huma.Register(api, huma.Operation{
		OperationID: "mint",
		Method:      http.MethodPost,
		Path:        "/mint",
		Middlewares: huma.Middlewares{func(ctx huma.Context, next func(huma.Context)) {
			// What the JWT middleware puts in front of a gated handler: a
			// principal, and a session that authenticated two hours ago.
			ctx = huma.WithValue(ctx, middleware.UserContextKey, userWithPassword())
			ctx = huma.WithValue(ctx, middleware.SessionAuthTimeContextKey, time.Now().Add(-2*time.Hour))
			next(ctx)
		}},
	}, func(ctx context.Context, _ *struct{}) (*struct{}, error) {
		if err := shared.RequireRecentSessionAuth(ctx, "wire_test_mint"); err != nil {
			return nil, err
		}
		return &struct{}{}, nil
	})

	resp := api.Post("/mint")

	if resp.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d; body: %s", resp.Code, http.StatusForbidden, resp.Body.String())
	}

	var decoded struct {
		Success   bool   `json:"success"`
		Message   string `json:"message"`
		ErrorCode string `json:"error_code"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &decoded); err != nil {
		t.Fatalf("decoding the response body: %v (body: %s)", err, resp.Body.String())
	}
	if decoded.ErrorCode != autherrors.CodeReauthRequired {
		t.Errorf("error_code = %q, want %q; body: %s", decoded.ErrorCode, autherrors.CodeReauthRequired, resp.Body.String())
	}
	if decoded.Success {
		t.Error("a refusal must not report success")
	}
	if decoded.Message == "" {
		t.Error("a refusal must carry copy for a client with no code map")
	}
}

// The same operation, reached by a session that authenticated a moment ago,
// runs. Without this the test above would pass against a gate that refused
// everything.
func TestReauthGateLetsAFreshSessionThrough(t *testing.T) {
	_, api := humatest.New(t, huma.DefaultConfig("reauth-wire", "1.0.0"))

	huma.Register(api, huma.Operation{
		OperationID: "mint",
		Method:      http.MethodPost,
		Path:        "/mint",
		Middlewares: huma.Middlewares{func(ctx huma.Context, next func(huma.Context)) {
			ctx = huma.WithValue(ctx, middleware.UserContextKey, userWithPassword())
			ctx = huma.WithValue(ctx, middleware.SessionAuthTimeContextKey, time.Now())
			next(ctx)
		}},
	}, func(ctx context.Context, _ *struct{}) (*struct{}, error) {
		if err := shared.RequireRecentSessionAuth(ctx, "wire_test_mint"); err != nil {
			return nil, err
		}
		return &struct{}{}, nil
	})

	if resp := api.Post("/mint"); resp.Code != http.StatusNoContent && resp.Code != http.StatusOK {
		t.Fatalf("status = %d, want a success; body: %s", resp.Code, resp.Body.String())
	}
}
