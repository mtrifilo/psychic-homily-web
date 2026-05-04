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

func testRevisionHandler() *RevisionHandler {
	return NewRevisionHandler(nil, nil)
}

func revisionAdminCtx() context.Context {
	return testhelpers.CtxWithUser(&authm.User{ID: 1, IsAdmin: true})
}

func revisionNonAdminCtx() context.Context {
	return testhelpers.CtxWithUser(&authm.User{ID: 2, IsAdmin: false})
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
			GetEntityHistoryFn: func(entityType string, entityID uint, limit, offset int) ([]adminm.Revision, int64, error) {
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
			GetEntityHistoryFn: func(entityType string, entityID uint, limit, offset int) ([]adminm.Revision, int64, error) {
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
			GetEntityHistoryFn: func(entityType string, entityID uint, limit, offset int) ([]adminm.Revision, int64, error) {
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
			GetRevisionFn: func(revisionID uint) (*adminm.Revision, error) {
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
			GetRevisionFn: func(revisionID uint) (*adminm.Revision, error) {
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
			GetRevisionFn: func(revisionID uint) (*adminm.Revision, error) {
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
			GetUserRevisionsFn: func(userID uint, limit, offset int) ([]adminm.Revision, int64, error) {
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
			GetUserRevisionsFn: func(userID uint, limit, offset int) ([]adminm.Revision, int64, error) {
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
			GetUserRevisionsFn: func(userID uint, limit, offset int) ([]adminm.Revision, int64, error) {
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
