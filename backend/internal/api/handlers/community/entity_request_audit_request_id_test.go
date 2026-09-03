package community

import (
	"testing"
	"time"

	"psychic-homily-backend/internal/api/handlers/shared/testhelpers"
	authm "psychic-homily-backend/internal/models/auth"
	communitym "psychic-homily-backend/internal/models/community"
	"psychic-homily-backend/internal/services/contracts"
)

// EVERY ENTITY-REQUEST AUDIT ROW MUST RECORD THE REQUEST'S OWN ID IN METADATA.
//
// The contributions timeline decides these rows against that key rather than
// against entity_id (services/user/contributor_profile.go). It has to: these
// rows store the REQUESTED catalog type in entity_type, and repointEntityRefs
// (services/catalog/entity_ref_repoint.go) rewrites audit_logs.entity_id keyed
// on (entity_type, entity_id) alone, so a catalog merge whose losing id equals a
// request's id moves that request's rows onto the canonical entity's number.
//
// A writer that stopped recording the key would send the gate to its entity_id
// fallback, which is the merge-corruptible column the key exists to replace. The
// gate cannot detect that, and its own tests hand-write the metadata, so this is
// the only place the invariant is checked against the writers themselves.

// auditRow is one captured fire-and-forget audit write.
type auditRow struct {
	action   string
	entityID uint
	metadata map[string]interface{}
}

// captureEntityRequestAudit runs one handler and returns the audit row it wrote.
// The write happens in a goroutine, so it is read off a channel rather than
// after a sleep.
func captureEntityRequestAudit(t *testing.T, run func(*testhelpers.MockAuditLogService)) auditRow {
	t.Helper()
	written := make(chan auditRow, 4)
	run(&testhelpers.MockAuditLogService{
		LogActionFn: func(_ uint, action, _ string, entityID uint, md map[string]interface{}) {
			written <- auditRow{action: action, entityID: entityID, metadata: md}
		},
	})
	select {
	case row := <-written:
		return row
	case <-time.After(2 * time.Second):
		t.Fatal("the path under test must write an audit row")
		return auditRow{}
	}
}

// assertRecordsRequestID is the invariant itself, checked the way the gate reads
// it: the key is present, and it names the request the row is about rather than
// the entity_id beside it.
func assertRecordsRequestID(t *testing.T, row auditRow, wantRequestID uint) {
	t.Helper()
	raw, ok := row.metadata["request_id"]
	if !ok {
		t.Fatalf("%s recorded no request_id. The contributions timeline decides this row "+
			"against that key, because a catalog merge rewrites its entity_id; without it "+
			"the gate falls back to the very column the key replaces.", row.action)
	}
	got, ok := raw.(uint)
	if !ok {
		t.Fatalf("%s recorded request_id as %T, and the gate reads it as a number", row.action, raw)
	}
	if got != wantRequestID {
		t.Errorf("%s recorded request_id %d, want %d", row.action, got, wantRequestID)
	}
}

func TestEntityRequestAuditRowsRecordTheRequestID(t *testing.T) {
	const requestID uint = 7

	t.Run("queue", func(t *testing.T) {
		row := captureEntityRequestAudit(t, func(audit *testhelpers.MockAuditLogService) {
			h := NewEntityRequestHandler(
				&testhelpers.MockEntityRequestService{
					CreateRequestFn: func(*authm.User, string, []byte, string, []byte, bool) (*communitym.EntityRequest, *communitym.SupersededSubmission, error) {
						return pendingRequest(requestID, "artist"), nil, nil
					},
				},
				nil,
				audit,
			)
			req := &CreateEntityRequestRequest{}
			req.Body.EntityType = "artist"
			req.Body.Payload = artistPayload(t, "Boris")
			req.Body.SourceContext = communitym.EntityRequestSourceManual
			if _, err := h.CreateEntityRequestHandler(erUserCtx(), req); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
		assertRecordsRequestID(t, row, requestID)
	})

	t.Run("decide", func(t *testing.T) {
		rejected := pendingRequest(requestID, "artist")
		rejected.DecisionState = communitym.EntityRequestStateRejected
		row := captureEntityRequestAudit(t, func(audit *testhelpers.MockAuditLogService) {
			h := NewEntityRequestHandler(
				&testhelpers.MockEntityRequestService{
					GetRequestFn: func(uint) (*communitym.EntityRequest, error) {
						return pendingRequest(requestID, "artist"), nil
					},
					DecideFn: func(uint, uint, communitym.EntityRequestDecisionState, *string, *time.Time) (*communitym.EntityRequest, error) {
						return rejected, nil
					},
				},
				&testhelpers.MockEntityRequestFulfiller{},
				audit,
			)
			req := &AdminDecideEntityRequestRequest{ID: "7"}
			req.Body.Decision = "rejected"
			if _, err := h.AdminDecideEntityRequestHandler(erAdminCtx(), req); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
		assertRecordsRequestID(t, row, requestID)
	})

	t.Run("rescue fulfill", func(t *testing.T) {
		orphan := approvedUnfulfilledRequest(requestID, "artist")
		row := captureEntityRequestAudit(t, func(audit *testhelpers.MockAuditLogService) {
			h := NewEntityRequestHandler(
				&testhelpers.MockEntityRequestService{
					GetRequestFn: func(uint) (*communitym.EntityRequest, error) { return orphan, nil },
					ClaimRescueFulfillmentFn: func(uint, uint) (bool, error) {
						return true, nil
					},
				},
				&testhelpers.MockEntityRequestFulfiller{
					CreateArtistFn: func(*contracts.CreateArtistRequest) (*contracts.ArtistDetailResponse, error) {
						return &contracts.ArtistDetailResponse{ID: 501}, nil
					},
				},
				audit,
			)
			req := &AdminFulfillEntityRequestRequest{ID: "7"}
			if _, err := h.AdminFulfillEntityRequestHandler(erAdminCtx(), req); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
		assertRecordsRequestID(t, row, requestID)
	})

	t.Run("rescue void", func(t *testing.T) {
		voided := approvedUnfulfilledRequest(requestID, "artist")
		voided.DecisionState = communitym.EntityRequestStateRejected
		row := captureEntityRequestAudit(t, func(audit *testhelpers.MockAuditLogService) {
			h := NewEntityRequestHandler(
				&testhelpers.MockEntityRequestService{
					GetRequestFn: func(uint) (*communitym.EntityRequest, error) { return voided, nil },
					VoidApprovedUnfulfilledFn: func(uint, uint, *string) (bool, error) {
						return true, nil
					},
				},
				&testhelpers.MockEntityRequestFulfiller{},
				audit,
			)
			req := &AdminFulfillEntityRequestRequest{ID: "7"}
			req.Body.Action = "void"
			if _, err := h.AdminFulfillEntityRequestHandler(erAdminCtx(), req); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
		assertRecordsRequestID(t, row, requestID)
	})
}
