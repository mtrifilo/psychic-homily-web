package admin

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"psychic-homily-backend/internal/api/handlers/shared/testhelpers"
	adminm "psychic-homily-backend/internal/models/admin"
	authm "psychic-homily-backend/internal/models/auth"
)

// ============================================================================
// Test helpers
// ============================================================================

func revisionAdminCtx() context.Context {
	return testhelpers.CtxWithUser(&authm.User{ID: 1, IsAdmin: true})
}

func makeTestRevision(id uint) adminm.Revision {
	changes := []adminm.FieldChange{
		{Field: "name", OldValue: "Old Name", NewValue: "New Name"},
	}
	changesJSON, _ := json.Marshal(changes)
	raw := json.RawMessage(changesJSON)
	summary := "Updated name"
	username := "testuser"

	return adminm.Revision{
		ID:           id,
		EntityType:   "artist",
		EntityID:     10,
		UserID:       5,
		FieldChanges: &raw,
		Summary:      &summary,
		CreatedAt:    time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC),
		User: authm.User{
			ID:       5,
			Username: &username,
		},
	}
}

// ============================================================================
// Tests: Admin Guard (Rollback only)
// ============================================================================

// ============================================================================
// Tests: GetEntityHistoryHandler
// ============================================================================

func TestRevisionHandler_GetEntityHistory_Success(t *testing.T) {
	rev := makeTestRevision(1)
	h := NewRevisionHandler(
		&testhelpers.MockRevisionService{
			GetEntityHistoryFn: func(entityType string, entityID uint, limit, offset int, _ bool) ([]adminm.Revision, int64, error) {
				if entityType != "artist" || entityID != 10 {
					t.Errorf("unexpected params: type=%s, id=%d", entityType, entityID)
				}
				return []adminm.Revision{rev}, 1, nil
			},
		},
		nil,
	)

	resp, err := h.GetEntityHistoryHandler(context.Background(), &GetEntityHistoryRequest{
		EntityType: "artist",
		EntityID:   "10",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Total != 1 {
		t.Errorf("expected total=1, got %d", resp.Body.Total)
	}
	if len(resp.Body.Revisions) != 1 {
		t.Fatalf("expected 1 revision, got %d", len(resp.Body.Revisions))
	}

	r := resp.Body.Revisions[0]
	if r.ID != 1 {
		t.Errorf("expected id=1, got %d", r.ID)
	}
	if r.EntityType != "artist" {
		t.Errorf("expected entity_type=artist, got %s", r.EntityType)
	}
	if r.UserName != "testuser" {
		t.Errorf("expected user_name=testuser, got %s", r.UserName)
	}
	if r.Summary != "Updated name" {
		t.Errorf("expected summary='Updated name', got %s", r.Summary)
	}
	if len(r.Changes) != 1 {
		t.Fatalf("expected 1 change, got %d", len(r.Changes))
	}
	if r.Changes[0].Field != "name" {
		t.Errorf("expected field=name, got %s", r.Changes[0].Field)
	}
}

// TestRevisionHandler_GetEntityHistory_CreatedAtIsUTC is the PSY-604
// regression guard. Before the fix, a revision whose CreatedAt was a local
// time.Time (e.g. served from a DB driver that returns timestamptz in the
// session TZ) was formatted via t.Format("2006-01-02T15:04:05Z") — Format
// does NOT convert to UTC, so the literal "Z" in the layout asserted UTC
// while the value still carried the local clock reading. The frontend
// then parsed the (correctly-marked-but-wrong) UTC value and rendered the
// AttributionLine relative time off by exactly the local UTC offset
// (e.g. "7 hours ago" for Phoenix MST). The fix is to call .UTC() before
// .Format(...) on this specific field. Test asserts the response field
// reflects the UTC equivalent of the input time, not the local clock.
func TestRevisionHandler_GetEntityHistory_CreatedAtIsUTC(t *testing.T) {
	// 13:00 Phoenix MST (UTC-7) == 20:00 UTC
	phoenix, err := time.LoadLocation("America/Phoenix")
	if err != nil {
		t.Fatalf("failed to load Phoenix location: %v", err)
	}
	localTime := time.Date(2026, 5, 4, 13, 0, 0, 0, phoenix)

	rev := makeTestRevision(1)
	rev.CreatedAt = localTime

	h := NewRevisionHandler(
		&testhelpers.MockRevisionService{
			GetEntityHistoryFn: func(entityType string, entityID uint, limit, offset int, _ bool) ([]adminm.Revision, int64, error) {
				return []adminm.Revision{rev}, 1, nil
			},
		},
		nil,
	)

	resp, err := h.GetEntityHistoryHandler(context.Background(), &GetEntityHistoryRequest{
		EntityType: "artist",
		EntityID:   "10",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Body.Revisions) != 1 {
		t.Fatalf("expected 1 revision, got %d", len(resp.Body.Revisions))
	}

	got := resp.Body.Revisions[0].CreatedAt
	want := "2026-05-04T20:00:00Z"
	if got != want {
		t.Errorf("CreatedAt timezone drift: got %q, want %q (input was 13:00 Phoenix == 20:00 UTC)", got, want)
	}
}

func TestRevisionHandler_GetEntityHistory_InvalidEntityType(t *testing.T) {
	h := NewRevisionHandler(&testhelpers.MockRevisionService{}, nil)

	_, err := h.GetEntityHistoryHandler(context.Background(), &GetEntityHistoryRequest{
		EntityType: "invalid",
		EntityID:   "1",
	})
	testhelpers.AssertHumaError(t, err, 422)
}

func TestRevisionHandler_GetEntityHistory_InvalidEntityID(t *testing.T) {
	h := NewRevisionHandler(&testhelpers.MockRevisionService{}, nil)

	_, err := h.GetEntityHistoryHandler(context.Background(), &GetEntityHistoryRequest{
		EntityType: "artist",
		EntityID:   "not-a-number",
	})
	testhelpers.AssertHumaError(t, err, 400)
}

func TestRevisionHandler_GetEntityHistory_ServiceError(t *testing.T) {
	h := NewRevisionHandler(
		&testhelpers.MockRevisionService{
			GetEntityHistoryFn: func(entityType string, entityID uint, limit, offset int, _ bool) ([]adminm.Revision, int64, error) {
				return nil, 0, fmt.Errorf("database error")
			},
		},
		nil,
	)

	_, err := h.GetEntityHistoryHandler(context.Background(), &GetEntityHistoryRequest{
		EntityType: "artist",
		EntityID:   "1",
	})
	testhelpers.AssertHumaError(t, err, 500)
}

func TestRevisionHandler_GetEntityHistory_DefaultLimit(t *testing.T) {
	var receivedLimit int
	h := NewRevisionHandler(
		&testhelpers.MockRevisionService{
			GetEntityHistoryFn: func(entityType string, entityID uint, limit, offset int, _ bool) ([]adminm.Revision, int64, error) {
				receivedLimit = limit
				return nil, 0, nil
			},
		},
		nil,
	)

	_, err := h.GetEntityHistoryHandler(context.Background(), &GetEntityHistoryRequest{
		EntityType: "venue",
		EntityID:   "1",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if receivedLimit != 20 {
		t.Errorf("expected default limit=20, got %d", receivedLimit)
	}
}

func TestRevisionHandler_GetEntityHistory_AllEntityTypes(t *testing.T) {
	h := NewRevisionHandler(&testhelpers.MockRevisionService{}, nil)

	for _, entityType := range []string{"artist", "venue", "show", "release", "label", "festival"} {
		t.Run(entityType, func(t *testing.T) {
			resp, err := h.GetEntityHistoryHandler(context.Background(), &GetEntityHistoryRequest{
				EntityType: entityType,
				EntityID:   "1",
			})
			if err != nil {
				t.Fatalf("unexpected error for entity type %s: %v", entityType, err)
			}
			if resp == nil {
				t.Fatalf("expected non-nil response for entity type %s", entityType)
			}
		})
	}
}

// ============================================================================
// Tests: GetRevisionHandler
// ============================================================================

func TestRevisionHandler_GetRevision_Success(t *testing.T) {
	rev := makeTestRevision(42)
	h := NewRevisionHandler(
		&testhelpers.MockRevisionService{
			GetRevisionFn: func(revisionID uint, _ bool) (*adminm.Revision, error) {
				if revisionID != 42 {
					t.Errorf("expected revisionID=42, got %d", revisionID)
				}
				return &rev, nil
			},
		},
		nil,
	)

	resp, err := h.GetRevisionHandler(context.Background(), &GetRevisionRequest{RevisionID: "42"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.ID != 42 {
		t.Errorf("expected id=42, got %d", resp.Body.ID)
	}
	if resp.Body.EntityType != "artist" {
		t.Errorf("expected entity_type=artist, got %s", resp.Body.EntityType)
	}
}

func TestRevisionHandler_GetRevision_NotFound(t *testing.T) {
	h := NewRevisionHandler(
		&testhelpers.MockRevisionService{
			GetRevisionFn: func(revisionID uint, _ bool) (*adminm.Revision, error) {
				return nil, nil // not found
			},
		},
		nil,
	)

	_, err := h.GetRevisionHandler(context.Background(), &GetRevisionRequest{RevisionID: "999"})
	testhelpers.AssertHumaError(t, err, 404)
}

func TestRevisionHandler_GetRevision_InvalidID(t *testing.T) {
	h := NewRevisionHandler(&testhelpers.MockRevisionService{}, nil)

	_, err := h.GetRevisionHandler(context.Background(), &GetRevisionRequest{RevisionID: "abc"})
	testhelpers.AssertHumaError(t, err, 400)
}

func TestRevisionHandler_GetRevision_ServiceError(t *testing.T) {
	h := NewRevisionHandler(
		&testhelpers.MockRevisionService{
			GetRevisionFn: func(revisionID uint, _ bool) (*adminm.Revision, error) {
				return nil, fmt.Errorf("database error")
			},
		},
		nil,
	)

	_, err := h.GetRevisionHandler(context.Background(), &GetRevisionRequest{RevisionID: "1"})
	testhelpers.AssertHumaError(t, err, 500)
}

// ============================================================================
// Tests: GetUserRevisionsHandler
// ============================================================================

func TestRevisionHandler_GetUserRevisions_Success(t *testing.T) {
	rev := makeTestRevision(1)
	h := NewRevisionHandler(
		&testhelpers.MockRevisionService{
			GetUserRevisionsFn: func(userID uint, limit, offset int, _ bool) ([]adminm.Revision, int64, error) {
				if userID != 5 {
					t.Errorf("expected userID=5, got %d", userID)
				}
				return []adminm.Revision{rev}, 1, nil
			},
		},
		nil,
	)

	resp, err := h.GetUserRevisionsHandler(context.Background(), &GetUserRevisionsRequest{UserID: "5"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Body.Total != 1 {
		t.Errorf("expected total=1, got %d", resp.Body.Total)
	}
	if len(resp.Body.Revisions) != 1 {
		t.Fatalf("expected 1 revision, got %d", len(resp.Body.Revisions))
	}
}

func TestRevisionHandler_GetUserRevisions_InvalidUserID(t *testing.T) {
	h := NewRevisionHandler(&testhelpers.MockRevisionService{}, nil)

	_, err := h.GetUserRevisionsHandler(context.Background(), &GetUserRevisionsRequest{UserID: "abc"})
	testhelpers.AssertHumaError(t, err, 400)
}

func TestRevisionHandler_GetUserRevisions_ServiceError(t *testing.T) {
	h := NewRevisionHandler(
		&testhelpers.MockRevisionService{
			GetUserRevisionsFn: func(userID uint, limit, offset int, _ bool) ([]adminm.Revision, int64, error) {
				return nil, 0, fmt.Errorf("database error")
			},
		},
		nil,
	)

	_, err := h.GetUserRevisionsHandler(context.Background(), &GetUserRevisionsRequest{UserID: "1"})
	testhelpers.AssertHumaError(t, err, 500)
}

func TestRevisionHandler_GetUserRevisions_DefaultLimit(t *testing.T) {
	var receivedLimit int
	h := NewRevisionHandler(
		&testhelpers.MockRevisionService{
			GetUserRevisionsFn: func(userID uint, limit, offset int, _ bool) ([]adminm.Revision, int64, error) {
				receivedLimit = limit
				return nil, 0, nil
			},
		},
		nil,
	)

	_, err := h.GetUserRevisionsHandler(context.Background(), &GetUserRevisionsRequest{UserID: "1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if receivedLimit != 20 {
		t.Errorf("expected default limit=20, got %d", receivedLimit)
	}
}

// ============================================================================
// Tests: RollbackRevisionHandler
// ============================================================================

func TestRevisionHandler_Rollback_Success(t *testing.T) {
	var receivedRevisionID uint
	var receivedAdminID uint
	h := NewRevisionHandler(
		&testhelpers.MockRevisionService{
			RollbackFn: func(revisionID uint, adminUserID uint) error {
				receivedRevisionID = revisionID
				receivedAdminID = adminUserID
				return nil
			},
		},
		&testhelpers.MockAuditLogService{},
	)

	resp, err := h.RollbackRevisionHandler(revisionAdminCtx(), &RollbackRevisionRequest{RevisionID: "42"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.Body.Success {
		t.Error("expected success=true")
	}
	if receivedRevisionID != 42 {
		t.Errorf("expected revisionID=42, got %d", receivedRevisionID)
	}
	if receivedAdminID != 1 {
		t.Errorf("expected adminID=1, got %d", receivedAdminID)
	}
}

func TestRevisionHandler_Rollback_InvalidID(t *testing.T) {
	h := NewRevisionHandler(&testhelpers.MockRevisionService{}, nil)

	_, err := h.RollbackRevisionHandler(revisionAdminCtx(), &RollbackRevisionRequest{RevisionID: "abc"})
	testhelpers.AssertHumaError(t, err, 400)
}

func TestRevisionHandler_Rollback_ServiceError(t *testing.T) {
	h := NewRevisionHandler(
		&testhelpers.MockRevisionService{
			RollbackFn: func(revisionID uint, adminUserID uint) error {
				return fmt.Errorf("revision not found")
			},
		},
		nil,
	)

	_, err := h.RollbackRevisionHandler(revisionAdminCtx(), &RollbackRevisionRequest{RevisionID: "999"})
	testhelpers.AssertHumaError(t, err, 422)
}

func TestRevisionHandler_Rollback_NilAuditLog(t *testing.T) {
	// Ensure rollback works even when auditLogService is nil
	h := NewRevisionHandler(
		&testhelpers.MockRevisionService{},
		nil,
	)

	resp, err := h.RollbackRevisionHandler(revisionAdminCtx(), &RollbackRevisionRequest{RevisionID: "1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.Body.Success {
		t.Error("expected success=true")
	}
}

// ============================================================================
// Tests: mapRevisionToResponse
// ============================================================================

func TestMapRevisionToResponse_NilFieldChanges(t *testing.T) {
	r := adminm.Revision{
		ID:           1,
		EntityType:   "artist",
		EntityID:     10,
		UserID:       5,
		FieldChanges: nil,
		CreatedAt:    time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC),
	}

	item := mapRevisionToResponse(r)
	if len(item.Changes) != 0 {
		t.Errorf("expected empty changes for nil FieldChanges, got %d", len(item.Changes))
	}
}

// The last link in the withholding chain. RevisionService hands the handler a
// nil Summary for a gated venue; this pins that the serialized payload then
// carries no summary KEY at all, rather than an empty string a client could
// still render as a blank line. Asserted on the JSON, not on the struct, because
// omitempty is the part that could regress silently.
//
// Built from the fully-populated fixture with only Summary cleared, so it also
// shows summary is the ONLY key that disappears. A payload that dropped several
// keys would not distinguish withholding from a broken response.
func TestMapRevisionToResponse_NilSummaryOmittedFromPayload(t *testing.T) {
	r := makeTestRevision(1)
	r.Summary = nil

	encoded, err := json.Marshal(mapRevisionToResponse(r))
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal(encoded, &payload); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if _, present := payload["summary"]; present {
		t.Errorf("expected no summary key in payload, got %s", encoded)
	}
	for _, key := range []string{"id", "entity_type", "entity_id", "user_id", "user_name", "changes", "created_at"} {
		if _, present := payload[key]; !present {
			t.Errorf("expected %q to survive, got %s", key, encoded)
		}
	}
}

func TestMapRevisionToResponse_FallbackToFirstName(t *testing.T) {
	firstName := "John"
	r := adminm.Revision{
		ID:           1,
		EntityType:   "artist",
		EntityID:     10,
		UserID:       5,
		FieldChanges: nil,
		CreatedAt:    time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC),
		User: authm.User{
			ID:        5,
			Username:  nil,
			FirstName: &firstName,
		},
	}

	item := mapRevisionToResponse(r)
	if item.UserName != "John" {
		t.Errorf("expected user_name=John, got %s", item.UserName)
	}
	if item.UserUsername != nil {
		t.Errorf("expected user_username=nil when username unset, got %v", *item.UserUsername)
	}
}

// PSY-560: full resolveUserName chain (username → first/last → email-prefix
// → "Anonymous") + linkable user_username for /users/:username profile
// links. Mirrors PSY-552's resolveCommentAuthorName.

func TestMapRevisionToResponse_PrefersUsername(t *testing.T) {
	username := "asdf"
	firstName := "John"
	r := adminm.Revision{
		ID:        1,
		UserID:    5,
		CreatedAt: time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC),
		User: authm.User{
			ID:        5,
			Username:  &username,
			FirstName: &firstName,
		},
	}

	item := mapRevisionToResponse(r)
	if item.UserName != "asdf" {
		t.Errorf("expected user_name=asdf (username wins), got %s", item.UserName)
	}
	if item.UserUsername == nil || *item.UserUsername != "asdf" {
		t.Errorf("expected user_username=&\"asdf\", got %v", item.UserUsername)
	}
}

func TestMapRevisionToResponse_FallbackToFirstAndLastName(t *testing.T) {
	first := "Jane"
	last := "Doe"
	r := adminm.Revision{
		ID:        1,
		UserID:    5,
		CreatedAt: time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC),
		User: authm.User{
			ID:        5,
			FirstName: &first,
			LastName:  &last,
		},
	}

	item := mapRevisionToResponse(r)
	if item.UserName != "Jane Doe" {
		t.Errorf("expected user_name=\"Jane Doe\", got %s", item.UserName)
	}
}

func TestMapRevisionToResponse_FallbackToEmailPrefix(t *testing.T) {
	email := "asdf@admin.com"
	r := adminm.Revision{
		ID:        1,
		UserID:    5,
		CreatedAt: time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC),
		User: authm.User{
			ID:    5,
			Email: &email,
		},
	}

	item := mapRevisionToResponse(r)
	if item.UserName != "asdf" {
		t.Errorf("expected user_name=asdf (email local-part), got %s", item.UserName)
	}
	if item.UserUsername != nil {
		t.Errorf("expected user_username=nil (no username set), got %v", *item.UserUsername)
	}
}

func TestMapRevisionToResponse_FallbackToAnonymous(t *testing.T) {
	r := adminm.Revision{
		ID:        1,
		UserID:    5,
		CreatedAt: time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC),
		User:      authm.User{ID: 5},
	}

	item := mapRevisionToResponse(r)
	if item.UserName != "Anonymous" {
		t.Errorf("expected user_name=Anonymous when no identity fields set, got %s", item.UserName)
	}
}

// Empty-string username should not be linkable — the User would have ""
// stored, which is a valid GORM zero-value but a bad URL slug. PSY-560
// guards against this explicitly to mirror resolveCommentAuthorUsername.
func TestMapRevisionToResponse_EmptyUsernameTreatedAsUnset(t *testing.T) {
	emptyUsername := ""
	firstName := "Jane"
	r := adminm.Revision{
		ID:        1,
		UserID:    5,
		CreatedAt: time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC),
		User: authm.User{
			ID:        5,
			Username:  &emptyUsername,
			FirstName: &firstName,
		},
	}

	item := mapRevisionToResponse(r)
	if item.UserName != "Jane" {
		t.Errorf("expected display name to fall through past empty username, got %s", item.UserName)
	}
	if item.UserUsername != nil {
		t.Errorf("expected user_username=nil when username is empty string, got %v", *item.UserUsername)
	}
}

// ============================================================================
// Tests: viewer tier (PSY-1717)
// ============================================================================
//
// The three read routes serve an unverified venue's address history to an admin
// and mask it for everyone else. The masking itself lives in the service; what
// these tests pin is the half the service cannot see — WHICH CALLER resolves to
// which tier — because from below the auth boundary an anonymous request and an
// authenticated contributor are the same single false.
//
// Every case asserts the bool the handler actually passed down, not the response
// body, so a regression that stopped forwarding the tier fails here rather than
// only in an end-to-end test that has to seed a venue to notice.

type revisionViewerTierCase struct {
	name string
	ctx  context.Context
	want bool
}

// revisionViewerTierCases enumerates every way a caller can arrive at these
// routes. Exactly one of them is the admin tier; the rest are the public one,
// and each is a distinct way of failing to prove admin that
// OptionalHumaJWTMiddleware can produce.
var revisionViewerTierCases = []revisionViewerTierCase{
	// No credential at all: the middleware calls next(ctx) untouched, so
	// nothing is stored under the user key. This is the common case — these
	// routes are public and most reads of them are anonymous.
	{"anonymous", context.Background(), false},
	// A valid session for an ordinary contributor. Authenticated is NOT admin,
	// and this is the case a check written as a bare nil test would wrongly
	// promote.
	{"authenticated non-admin", testhelpers.CtxWithUser(&authm.User{ID: 7, IsAdmin: false}), false},
	// A TYPED nil under the user key. GetUserFromContext's type assertion
	// succeeds on it, so the nil check in revisionViewerIsAdmin is what stops
	// this dereferencing — dropping that check turns this row into a panic on a
	// request an attacker can shape.
	{"user key present but nil", testhelpers.CtxWithUser(nil), false},
	{"authenticated admin", revisionAdminCtx(), true},
}

func TestRevisionHandler_GetEntityHistory_PassesViewerTier(t *testing.T) {
	for _, tc := range revisionViewerTierCases {
		t.Run(tc.name, func(t *testing.T) {
			var got bool
			h := NewRevisionHandler(
				&testhelpers.MockRevisionService{
					GetEntityHistoryFn: func(_ string, _ uint, _, _ int, viewerIsAdmin bool) ([]adminm.Revision, int64, error) {
						got = viewerIsAdmin
						return nil, 0, nil
					},
				},
				nil,
			)

			if _, err := h.GetEntityHistoryHandler(tc.ctx, &GetEntityHistoryRequest{
				EntityType: "venue",
				EntityID:   "10",
			}); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Errorf("viewerIsAdmin = %t, want %t for a %s caller", got, tc.want, tc.name)
			}
		})
	}
}

func TestRevisionHandler_GetRevision_PassesViewerTier(t *testing.T) {
	for _, tc := range revisionViewerTierCases {
		t.Run(tc.name, func(t *testing.T) {
			var got bool
			rev := makeTestRevision(1)
			h := NewRevisionHandler(
				&testhelpers.MockRevisionService{
					GetRevisionFn: func(_ uint, viewerIsAdmin bool) (*adminm.Revision, error) {
						got = viewerIsAdmin
						return &rev, nil
					},
				},
				nil,
			)

			if _, err := h.GetRevisionHandler(tc.ctx, &GetRevisionRequest{RevisionID: "1"}); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Errorf("viewerIsAdmin = %t, want %t for a %s caller", got, tc.want, tc.name)
			}
		})
	}
}

func TestRevisionHandler_GetUserRevisions_PassesViewerTier(t *testing.T) {
	for _, tc := range revisionViewerTierCases {
		t.Run(tc.name, func(t *testing.T) {
			var got bool
			h := NewRevisionHandler(
				&testhelpers.MockRevisionService{
					GetUserRevisionsFn: func(_ uint, _, _ int, viewerIsAdmin bool) ([]adminm.Revision, int64, error) {
						got = viewerIsAdmin
						return nil, 0, nil
					},
				},
				nil,
			)

			if _, err := h.GetUserRevisionsHandler(tc.ctx, &GetUserRevisionsRequest{UserID: "5"}); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Errorf("viewerIsAdmin = %t, want %t for a %s caller", got, tc.want, tc.name)
			}
		})
	}
}
