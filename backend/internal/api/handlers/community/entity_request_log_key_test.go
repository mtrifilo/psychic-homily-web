package community

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"testing"
	"time"

	"psychic-homily-backend/internal/api/handlers/shared/testhelpers"
	"psychic-homily-backend/internal/logger"
	communitym "psychic-homily-backend/internal/models/community"
	"psychic-homily-backend/internal/testlog"
)

// In a structured log line, request_id is the HTTP correlation id the request
// middleware attaches to the context logger. A community row's database id
// travels under its own key, so a search for one never matches the other.

const httpCorrelationID = "http-correlation-id-for-log-key-tests"

// logKeyRowID is distinct from every id the handlers' other tests use, so a
// match in the captured line can only come from the row under test.
const logKeyRowID uint = 7041

// withCapturedLogger returns ctx carrying a JSON logger shaped like the one the
// request middleware installs: request_id already bound to the HTTP id.
func withCapturedLogger(ctx context.Context) (context.Context, *bytes.Buffer) {
	l, buf := testlog.NewJSONCapture(slog.LevelDebug)
	return logger.NewContext(ctx, logger.WithRequestID(l, httpCorrelationID)), buf
}

// assertLogKeys finds the record whose msg is event and checks both keys. A
// record that carried request_id twice decodes to the LAST value, so a row id
// logged under request_id fails the correlation-id check.
func assertLogKeys(t *testing.T, buf *bytes.Buffer, event string, rowID uint) {
	t.Helper()
	scanner := bufio.NewScanner(bytes.NewReader(buf.Bytes()))
	for scanner.Scan() {
		var rec map[string]any
		if err := json.Unmarshal(scanner.Bytes(), &rec); err != nil {
			t.Fatalf("captured line is not JSON: %v\n%s", err, scanner.Text())
		}
		if rec["msg"] != event {
			continue
		}
		if got, ok := rec["entity_request_id"].(float64); !ok || uint(got) != rowID {
			t.Errorf("%s: entity_request_id = %v, want %d", event, rec["entity_request_id"], rowID)
		}
		if got := rec["request_id"]; got != httpCorrelationID {
			t.Errorf("%s: request_id = %v, want the HTTP correlation id %q", event, got, httpCorrelationID)
		}
		return
	}
	t.Fatalf("no %q record in the capture:\n%s", event, buf.String())
}

var errLogKeyStore = errors.New("store unavailable")

func TestEntityRequestLogKey_DecideFailure(t *testing.T) {
	h := NewEntityRequestHandler(
		&testhelpers.MockEntityRequestService{
			DecideFn: func(uint, uint, communitym.EntityRequestDecisionState, *string, *time.Time) (*communitym.EntityRequest, error) {
				return nil, errLogKeyStore
			},
		},
		nil, nil,
	)
	ctx, buf := withCapturedLogger(erAdminCtx())
	req := &AdminDecideEntityRequestRequest{ID: "7041"}
	req.Body.Decision = string(communitym.EntityRequestStateRejected)

	_, err := h.AdminDecideEntityRequestHandler(ctx, req)
	testhelpers.AssertHumaError(t, err, 500)
	assertLogKeys(t, buf, "entity_request_decide_failed", logKeyRowID)
}

func TestEntityRequestLogKey_RescueVoidFailure(t *testing.T) {
	h := NewEntityRequestHandler(
		&testhelpers.MockEntityRequestService{
			VoidApprovedUnfulfilledFn: func(uint, uint, *string) (bool, error) {
				return false, errLogKeyStore
			},
		},
		nil, nil,
	)
	ctx, buf := withCapturedLogger(erAdminCtx())
	req := &AdminFulfillEntityRequestRequest{ID: "7041"}
	req.Body.Action = rescueActionVoid

	_, err := h.AdminFulfillEntityRequestHandler(ctx, req)
	testhelpers.AssertHumaError(t, err, 500)
	assertLogKeys(t, buf, "entity_request_rescue_void_failed", logKeyRowID)
}

func TestEntityRequestLogKey_WithdrawFailure(t *testing.T) {
	h := NewEntityRequestHandler(
		&testhelpers.MockEntityRequestService{
			WithdrawFn: func(uint, uint) (*communitym.EntityRequest, error) {
				return nil, errLogKeyStore
			},
		},
		nil, nil,
	)
	ctx, buf := withCapturedLogger(erUserCtx())

	_, err := h.WithdrawEntityRequestHandler(ctx, withdrawRequest("7041"))
	testhelpers.AssertHumaError(t, err, 500)
	assertLogKeys(t, buf, "entity_request_withdraw_failed", logKeyRowID)
}
