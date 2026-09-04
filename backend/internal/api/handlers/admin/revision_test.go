package admin

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"psychic-homily-backend/internal/api/handlers/shared/testhelpers"
	adminm "psychic-homily-backend/internal/models/admin"
	authm "psychic-homily-backend/internal/models/auth"
	"psychic-homily-backend/internal/services/contracts"
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
			GetEntityHistoryFn: func(entityType string, entityID uint, limit, offset int, _ contracts.RevisionViewer) ([]adminm.Revision, int64, error) {
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
			GetEntityHistoryFn: func(entityType string, entityID uint, limit, offset int, _ contracts.RevisionViewer) ([]adminm.Revision, int64, error) {
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
			GetEntityHistoryFn: func(entityType string, entityID uint, limit, offset int, _ contracts.RevisionViewer) ([]adminm.Revision, int64, error) {
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
			GetEntityHistoryFn: func(entityType string, entityID uint, limit, offset int, _ contracts.RevisionViewer) ([]adminm.Revision, int64, error) {
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
			GetRevisionFn: func(revisionID uint, _ contracts.RevisionViewer) (*adminm.Revision, error) {
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
			GetRevisionFn: func(revisionID uint, _ contracts.RevisionViewer) (*adminm.Revision, error) {
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
			GetRevisionFn: func(revisionID uint, _ contracts.RevisionViewer) (*adminm.Revision, error) {
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
			GetUserRevisionsFn: func(userID uint, limit, offset int, _ contracts.RevisionViewer) ([]adminm.Revision, int64, error) {
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
			GetUserRevisionsFn: func(userID uint, limit, offset int, _ contracts.RevisionViewer) ([]adminm.Revision, int64, error) {
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
			GetUserRevisionsFn: func(userID uint, limit, offset int, _ contracts.RevisionViewer) ([]adminm.Revision, int64, error) {
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
			RollbackFn: func(_ context.Context, revisionID uint, adminUserID uint) (*contracts.RollbackResult, error) {
				receivedRevisionID = revisionID
				receivedAdminID = adminUserID
				return &contracts.RollbackResult{AppliedFields: []string{"name"}}, nil
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
			RollbackFn: func(_ context.Context, revisionID uint, adminUserID uint) (*contracts.RollbackResult, error) {
				return nil, fmt.Errorf("revision not found")
			},
		},
		nil,
	)

	_, err := h.RollbackRevisionHandler(revisionAdminCtx(), &RollbackRevisionRequest{RevisionID: "999"})
	testhelpers.AssertHumaError(t, err, 422)
}

// A partial rollback must reach the caller as one: an admin who sees only
// success believes an edit was undone that was only half undone.
func TestRevisionHandler_Rollback_ReportsSkippedFields(t *testing.T) {
	h := NewRevisionHandler(
		&testhelpers.MockRevisionService{
			RollbackFn: func(_ context.Context, _ uint, _ uint) (*contracts.RollbackResult, error) {
				return &contracts.RollbackResult{
					AppliedFields: []string{"description"},
					SkippedFields: []contracts.RollbackSkippedField{
						{Field: "spotify", Reason: "Spotify URL must be on spotify.com"},
					},
				}, nil
			},
		},
		&testhelpers.MockAuditLogService{},
	)

	resp, err := h.RollbackRevisionHandler(revisionAdminCtx(), &RollbackRevisionRequest{RevisionID: "7"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := resp.Body.AppliedFields; len(got) != 1 || got[0] != "description" {
		t.Errorf("applied_fields = %v, want [description]", got)
	}
	if len(resp.Body.SkippedFields) != 1 {
		t.Fatalf("skipped_fields = %v, want one entry", resp.Body.SkippedFields)
	}
	if resp.Body.SkippedFields[0].Field != "spotify" {
		t.Errorf("skipped field = %q, want spotify", resp.Body.SkippedFields[0].Field)
	}
	if resp.Body.SkippedFields[0].Reason == "" {
		t.Error("a skipped field must carry the reason it was refused")
	}
}

func TestRevisionHandler_Rollback_NilAuditLog(t *testing.T) {
	// Ensure rollback works even when auditLogService is nil
	h := NewRevisionHandler(
		&testhelpers.MockRevisionService{
			RollbackFn: func(_ context.Context, _ uint, _ uint) (*contracts.RollbackResult, error) {
				return &contracts.RollbackResult{AppliedFields: []string{"name"}}, nil
			},
		},
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

// publicViewer is the tier every anonymous, unauthenticated, expired-token and
// merely-logged-in caller lands on. Spelled as the zero value on purpose: that
// is what revisionViewer returns when it cannot prove anything about a caller,
// so a test that constructs it any other way would not be testing the tier the
// public actually gets.
func publicViewer() contracts.RevisionViewer { return contracts.RevisionViewer{} }

func adminViewer() contracts.RevisionViewer {
	return contracts.RevisionViewer{UserID: 1, IsAdmin: true}
}

// hiddenContributionsSettings is the privacy blob of a contributor who turned
// their contributions off. Marshalled from the real contract type rather than
// hand-written JSON so a field rename cannot leave these tests asserting
// against a key the production unmarshal no longer reads.
func hiddenContributionsSettings(t *testing.T) *json.RawMessage {
	t.Helper()
	settings := contracts.DefaultPrivacySettings()
	settings.Contributions = contracts.PrivacyHidden
	encoded, err := json.Marshal(settings)
	if err != nil {
		t.Fatalf("marshal privacy settings: %v", err)
	}
	raw := json.RawMessage(encoded)
	return &raw
}

func TestMapRevisionToResponse_NilFieldChanges(t *testing.T) {
	r := adminm.Revision{
		ID:           1,
		EntityType:   "artist",
		EntityID:     10,
		UserID:       5,
		FieldChanges: nil,
		CreatedAt:    time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC),
	}

	item := mapRevisionToResponse(r, publicViewer())
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

	encoded, err := json.Marshal(mapRevisionToResponse(r, publicViewer()))
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

// ============================================================================
// Tests: author attribution (PSY-560, PSY-1940)
// ============================================================================
//
// PSY-560 pinned this handler's own copy of the resolution chain, which started
// at username and ended at an email local-part. PSY-1940 deleted that copy and
// split the byline in two:
//
//   - the PUBLIC tier resolves through shared.ResolvePublicContributorCredit,
//     which starts at display_name, fails closed on
//     privacy_settings.contributions, and OMITS rather than publishing an
//     email-derived or Anonymous name;
//   - the ADMIN tier resolves through the canonical shared.ResolveUserName,
//     matching the whole-view unmasking PSY-1717 already grants over field
//     values and summaries.
//
// Both tiers are pinned for every branch that differs, because "the public view
// hides it" is only half the claim a reviewer needs — the other half is that a
// moderator can still see who to talk to.

func TestMapRevisionToResponse_Public_PrefersDisplayName(t *testing.T) {
	// The regression PSY-1940 exists for. The deleted local chain started at
	// username, so this same user was named "mtrifilo" here and "Matt T" in the
	// show submitter byline on the SAME line of the same page.
	displayName := "Matt T"
	username := "mtrifilo"
	r := adminm.Revision{
		ID:        1,
		UserID:    5,
		CreatedAt: time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC),
		User: authm.User{
			ID:          5,
			DisplayName: &displayName,
			Username:    &username,
		},
	}

	item := mapRevisionToResponse(r, publicViewer())
	if item.UserName != "Matt T" {
		t.Errorf("expected user_name=\"Matt T\" (display_name wins), got %q", item.UserName)
	}
	// The LINK still comes from the username: display_name is not a URL slug.
	if item.UserUsername == nil || *item.UserUsername != "mtrifilo" {
		t.Errorf("expected user_username=&\"mtrifilo\", got %v", item.UserUsername)
	}
}

func TestMapRevisionToResponse_Public_FallbackToUsername(t *testing.T) {
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

	item := mapRevisionToResponse(r, publicViewer())
	if item.UserName != "asdf" {
		t.Errorf("expected user_name=asdf (username beats first name), got %q", item.UserName)
	}
	if item.UserUsername == nil || *item.UserUsername != "asdf" {
		t.Errorf("expected user_username=&\"asdf\", got %v", item.UserUsername)
	}
}

func TestMapRevisionToResponse_Public_FallbackToFirstName(t *testing.T) {
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

	item := mapRevisionToResponse(r, publicViewer())
	if item.UserName != "John" {
		t.Errorf("expected user_name=John, got %q", item.UserName)
	}
	if item.UserUsername != nil {
		t.Errorf("expected user_username=nil when username unset, got %v", *item.UserUsername)
	}
}

func TestMapRevisionToResponse_Public_FallbackToFirstAndLastName(t *testing.T) {
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

	item := mapRevisionToResponse(r, publicViewer())
	if item.UserName != "Jane Doe" {
		t.Errorf("expected user_name=\"Jane Doe\", got %q", item.UserName)
	}
}

// Empty-string username should not be linkable — the User would have ""
// stored, which is a valid GORM zero-value but a bad URL slug. PSY-560
// guards against this explicitly to mirror resolveCommentAuthorUsername.
func TestMapRevisionToResponse_Public_EmptyUsernameTreatedAsUnset(t *testing.T) {
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

	item := mapRevisionToResponse(r, publicViewer())
	if item.UserName != "Jane" {
		t.Errorf("expected display name to fall through past empty username, got %q", item.UserName)
	}
	if item.UserUsername != nil {
		t.Errorf("expected user_username=nil when username is empty string, got %v", *item.UserUsername)
	}
}

// The email tier. This USED to publish "asdf" from asdf@admin.com on a route a
// logged-out visitor can read.
func TestMapRevisionToResponse_Public_EmailOnlyAuthorIsUnnamed(t *testing.T) {
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

	item := mapRevisionToResponse(r, publicViewer())
	if item.UserName != "" {
		t.Errorf("expected no user_name for an email-only author, got %q", item.UserName)
	}
	if item.UserUsername != nil {
		t.Errorf("expected user_username=nil, got %v", *item.UserUsername)
	}
	// Substring, not equality: the point is that no fragment of the address
	// escapes, whichever tier a future edit might route it through.
	if strings.Contains(item.UserName, "asdf") {
		t.Errorf("email local-part leaked into the byline: %q", item.UserName)
	}
}

// No identity at all. The old chain answered "Anonymous"; that is a claim about
// a person where the honest answer is silence, and the frontend now renders the
// row with no byline instead.
func TestMapRevisionToResponse_Public_NamelessAuthorIsUnnamedNotAnonymous(t *testing.T) {
	r := adminm.Revision{
		ID:        1,
		UserID:    5,
		CreatedAt: time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC),
		User:      authm.User{ID: 5},
	}

	item := mapRevisionToResponse(r, publicViewer())
	if item.UserName != "" {
		t.Errorf("expected no user_name when no identity fields are set, got %q", item.UserName)
	}
}

// The headline gate. An author who turned contributions off keeps the EDIT
// visible — history stays auditable — and loses only the name.
func TestMapRevisionToResponse_Public_HiddenContributionsSuppressesAuthor(t *testing.T) {
	username := "mtrifilo"
	displayName := "Matt T"
	r := makeTestRevision(1)
	r.User = authm.User{
		ID:              5,
		Username:        &username,
		DisplayName:     &displayName,
		PrivacySettings: hiddenContributionsSettings(t),
	}

	item := mapRevisionToResponse(r, publicViewer())
	if item.UserName != "" {
		t.Errorf("expected no user_name for a hidden contributor, got %q", item.UserName)
	}
	if item.UserUsername != nil {
		t.Errorf("expected no user_username for a hidden contributor, got %v", *item.UserUsername)
	}
	// The edit itself is untouched: this gate hides the person, not the history.
	if len(item.Changes) != 1 || item.Changes[0].Field != "name" {
		t.Errorf("expected the field change to survive the author gate, got %+v", item.Changes)
	}
	if item.Summary != "Updated name" {
		t.Errorf("expected the summary to survive the author gate, got %q", item.Summary)
	}
}

// A suppressed byline must leave NO user_name key, not an empty string a client
// could render as a blank byline or a stray "by". Asserted on the JSON because
// omitempty is the part that regresses silently.
func TestMapRevisionToResponse_Public_SuppressedNameOmittedFromPayload(t *testing.T) {
	username := "mtrifilo"
	r := makeTestRevision(1)
	r.User = authm.User{
		ID:              5,
		Username:        &username,
		PrivacySettings: hiddenContributionsSettings(t),
	}

	encoded, err := json.Marshal(mapRevisionToResponse(r, publicViewer()))
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(encoded, &payload); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if _, present := payload["user_name"]; present {
		t.Errorf("expected no user_name key for a hidden contributor, got %s", encoded)
	}
	if payload["user_username"] != nil {
		t.Errorf("expected user_username=null for a hidden contributor, got %s", encoded)
	}
}

// A private profile is NOT a hidden contributor: the person is still credited,
// as plain text, because /users/{username} would 404.
func TestMapRevisionToResponse_Public_PrivateProfileKeepsNameDropsLink(t *testing.T) {
	username := "mtrifilo"
	r := adminm.Revision{
		ID:        1,
		UserID:    5,
		CreatedAt: time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC),
		User: authm.User{
			ID:                5,
			Username:          &username,
			ProfileVisibility: "private",
		},
	}

	item := mapRevisionToResponse(r, publicViewer())
	if item.UserName != "mtrifilo" {
		t.Errorf("expected a private profile to keep its name, got %q", item.UserName)
	}
	if item.UserUsername != nil {
		t.Errorf("expected a private profile to lose its link, got %v", *item.UserUsername)
	}
}

// Nil privacy_settings is near-unreachable (NOT NULL with a default), so this
// pins the defensive branch: it must read as the DEFAULTS, which have
// contributions visible, and not as "unset, therefore hide" — a divergent local
// rule here would be the surprise, and would blank every byline on any
// column-restricted read that forgot the column.
func TestMapRevisionToResponse_Public_NilPrivacySettingsCreditsAuthor(t *testing.T) {
	username := "mtrifilo"
	r := adminm.Revision{
		ID:        1,
		UserID:    5,
		CreatedAt: time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC),
		User: authm.User{
			ID:              5,
			Username:        &username,
			PrivacySettings: nil,
		},
	}

	item := mapRevisionToResponse(r, publicViewer())
	if item.UserName != "mtrifilo" {
		t.Errorf("expected default privacy to credit the author, got %q", item.UserName)
	}
}

// The admin half of the contract. A moderator deciding on a rollback sees who
// made the edit regardless of the contributor's public setting — the same
// whole-view unmasking PSY-1717 grants over values and summaries.
func TestMapRevisionToResponse_Admin_SeesHiddenContributor(t *testing.T) {
	username := "mtrifilo"
	displayName := "Matt T"
	r := adminm.Revision{
		ID:        1,
		UserID:    5,
		CreatedAt: time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC),
		User: authm.User{
			ID:              5,
			Username:        &username,
			DisplayName:     &displayName,
			PrivacySettings: hiddenContributionsSettings(t),
		},
	}

	item := mapRevisionToResponse(r, adminViewer())
	if item.UserName != "Matt T" {
		t.Errorf("expected an admin to see the hidden contributor, got %q", item.UserName)
	}
	if item.UserUsername == nil || *item.UserUsername != "mtrifilo" {
		t.Errorf("expected an admin to get the profile link, got %v", item.UserUsername)
	}
}

// What the admin tier unmasks is the NAME, and only the name. The profile LINK
// is not a privacy grant to waive: /users/{username} 404s for a private profile
// for everyone, admins included, so an admin byline that kept the link would
// just be pointing at a guaranteed 404. Both tiers share
// shared.ContributorProfileLink; this pins that the admin branch did not go
// around it.
func TestMapRevisionToResponse_Admin_PrivateProfileStillLosesTheLink(t *testing.T) {
	username := "mtrifilo"
	r := adminm.Revision{
		ID:        1,
		UserID:    5,
		CreatedAt: time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC),
		User: authm.User{
			ID:                5,
			Username:          &username,
			ProfileVisibility: "private",
		},
	}

	item := mapRevisionToResponse(r, adminViewer())
	if item.UserName != "mtrifilo" {
		t.Errorf("expected an admin to see the name, got %q", item.UserName)
	}
	if item.UserUsername != nil {
		t.Errorf("expected no link to a private profile even for an admin, got %v", *item.UserUsername)
	}
}

// The admin tier keeps the canonical chain whole, email tier included: it is
// the only handle a moderator has on an account that set no public name, and
// nothing on an admin-only view is being published to the web.
func TestMapRevisionToResponse_Admin_SeesEmailDerivedName(t *testing.T) {
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

	item := mapRevisionToResponse(r, adminViewer())
	if item.UserName != "asdf" {
		t.Errorf("expected the admin view to keep the canonical chain, got %q", item.UserName)
	}
}

// ============================================================================
// Tests: viewer identity (PSY-1717/1715)
// ============================================================================
//
// The three read routes decide two things from the caller: whether to serve an
// unverified venue's address history (admin only), and whether to serve a
// non-approved show's history at all (admin, or the show's submitter). Both
// decisions live in the service; what these tests pin is the half the service
// cannot see — WHAT THE HANDLER TELLS IT the caller is — because from below the
// auth boundary every request arrives as nothing but a struct somebody filled
// in.
//
// The whole viewer is asserted, not just the admin bit. UserID is what the show
// gate compares against shows.submitted_by, so a handler that forwarded the tier
// and dropped the id would silently deny every submitter their own show's
// history while every admin case still passed.
//
// Every case asserts the value the handler actually passed down, not the
// response body, so a regression that stopped forwarding it fails here rather
// than only in an end-to-end test that has to seed a show to notice.

type revisionViewerTierCase struct {
	name string
	ctx  context.Context
	want contracts.RevisionViewer
}

// revisionViewerTierCases covers the contexts these handlers must classify.
// Exactly one is the admin tier; the rest are not, and they differ from each
// other by whether an id reached the viewer at all.
//
// The first two and the last are the shapes OptionalHumaJWTMiddleware actually
// produces today. The typed-nil row is NOT one of them and is not a live request
// shape — see its own note.
var revisionViewerTierCases = []revisionViewerTierCase{
	// No credential at all: the middleware calls next(ctx) untouched, so
	// nothing is stored under the user key. This is the common case — these
	// routes are public and most reads of them are anonymous. It also stands in
	// for every credential the middleware REJECTS (bad signature, expired,
	// inactive user, bad API token), all of which fall through to this same
	// no-user context; the end-to-end coverage for those is in
	// routes/revision_viewer_tier_test.go, over real tokens.
	{"anonymous", context.Background(), contracts.RevisionViewer{}},
	// A valid session for an ordinary contributor. Authenticated is NOT admin,
	// and this is the case a check written as a bare nil test would wrongly
	// promote.
	{"authenticated non-admin", testhelpers.CtxWithUser(&authm.User{ID: 7, IsAdmin: false}),
		contracts.RevisionViewer{UserID: 7}},
	// A TYPED nil under the user key: GetUserFromContext's type assertion
	// succeeds on it and returns a nil pointer, so the `user != nil` half of
	// revisionViewer is the only thing between it and a dereference.
	//
	// No production path produces this. Every writer of UserContextKey stores a
	// user obtained from a validated credential, and neither validator can
	// return (nil, nil). It is here as a guard on the CHECK, not a claim about
	// the middleware: a later refactor that stores a *authm.User unconditionally
	// would make it reachable, and this row is what fails at that moment instead
	// of a nil dereference in a handler.
	{"user key present but nil", testhelpers.CtxWithUser(nil), contracts.RevisionViewer{}},
	{"authenticated admin", revisionAdminCtx(), contracts.RevisionViewer{UserID: 1, IsAdmin: true}},
}

func TestRevisionHandler_GetEntityHistory_PassesViewerIdentity(t *testing.T) {
	for _, tc := range revisionViewerTierCases {
		t.Run(tc.name, func(t *testing.T) {
			var got contracts.RevisionViewer
			h := NewRevisionHandler(
				&testhelpers.MockRevisionService{
					GetEntityHistoryFn: func(_ string, _ uint, _, _ int, viewer contracts.RevisionViewer) ([]adminm.Revision, int64, error) {
						got = viewer
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
				t.Errorf("viewer = %+v, want %+v for a %s caller", got, tc.want, tc.name)
			}
		})
	}
}

func TestRevisionHandler_GetRevision_PassesViewerIdentity(t *testing.T) {
	for _, tc := range revisionViewerTierCases {
		t.Run(tc.name, func(t *testing.T) {
			var got contracts.RevisionViewer
			rev := makeTestRevision(1)
			h := NewRevisionHandler(
				&testhelpers.MockRevisionService{
					GetRevisionFn: func(_ uint, viewer contracts.RevisionViewer) (*adminm.Revision, error) {
						got = viewer
						return &rev, nil
					},
				},
				nil,
			)

			if _, err := h.GetRevisionHandler(tc.ctx, &GetRevisionRequest{RevisionID: "1"}); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Errorf("viewer = %+v, want %+v for a %s caller", got, tc.want, tc.name)
			}
		})
	}
}

func TestRevisionHandler_GetUserRevisions_PassesViewerIdentity(t *testing.T) {
	for _, tc := range revisionViewerTierCases {
		t.Run(tc.name, func(t *testing.T) {
			var got contracts.RevisionViewer
			h := NewRevisionHandler(
				&testhelpers.MockRevisionService{
					GetUserRevisionsFn: func(_ uint, _, _ int, viewer contracts.RevisionViewer) ([]adminm.Revision, int64, error) {
						got = viewer
						return nil, 0, nil
					},
				},
				nil,
			)

			if _, err := h.GetUserRevisionsHandler(tc.ctx, &GetUserRevisionsRequest{UserID: "5"}); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Errorf("viewer = %+v, want %+v for a %s caller", got, tc.want, tc.name)
			}
		})
	}
}

// ============================================================================
// Tests: author attribution reaches the RESPONSE on every route (PSY-1940)
// ============================================================================
//
// The mapping tests above prove the policy; these prove the wiring. Each of the
// three read routes has to resolve the caller AND hand that same reading to the
// response mapper, and each does it in its own function body — a route that
// forwarded the viewer to the service (which the tier tests above cover) while
// still mapping the response with a default viewer would publish every hidden
// contributor and pass every test written so far.
//
// Driven through the handlers with the real context shapes, one case per route
// per tier, because the bug being guarded is per route.

// revisionWithHiddenAuthor is a revision by a contributor who has a display
// name, a username, AND contributions switched off — so a suppressed byline
// cannot be mistaken for a user who simply had no name to resolve.
func revisionWithHiddenAuthor(t *testing.T) adminm.Revision {
	t.Helper()
	username := "mtrifilo"
	displayName := "Matt T"
	r := makeTestRevision(1)
	r.User = authm.User{
		ID:              5,
		Username:        &username,
		DisplayName:     &displayName,
		PrivacySettings: hiddenContributionsSettings(t),
	}
	return r
}

func assertAuthorSuppressed(t *testing.T, item RevisionResponseItem) {
	t.Helper()
	if item.UserName != "" {
		t.Errorf("expected no user_name for a hidden contributor, got %q", item.UserName)
	}
	if item.UserUsername != nil {
		t.Errorf("expected no user_username for a hidden contributor, got %v", *item.UserUsername)
	}
	// The id goes with the name. Left in, it is a lookup key against the public
	// payloads that publish an id and a display name together, and the byline
	// is recovered in one more request.
	if item.UserID != nil {
		t.Errorf("expected no user_id for a hidden contributor, got %d", *item.UserID)
	}
	// The edit survives: this gate hides the person, not the history.
	if len(item.Changes) != 1 {
		t.Errorf("expected the edit to remain visible, got %d changes", len(item.Changes))
	}
}

// Asserts the WHOLE credit, not just the name: an admin-branch regression that
// dropped the profile link or the id would otherwise pass every route test.
func assertAuthorNamed(t *testing.T, item RevisionResponseItem) {
	t.Helper()
	if item.UserName != "Matt T" {
		t.Errorf("expected an admin to see the hidden contributor, got %q", item.UserName)
	}
	if item.UserUsername == nil || *item.UserUsername != "mtrifilo" {
		t.Errorf("expected the admin to get the profile link, got %v", item.UserUsername)
	}
	if item.UserID == nil || *item.UserID != 5 {
		t.Errorf("expected the admin to get user_id=5, got %v", item.UserID)
	}
}

func TestRevisionHandler_GetEntityHistory_HiddenAuthorByTier(t *testing.T) {
	for _, tc := range revisionViewerTierCases {
		t.Run(tc.name, func(t *testing.T) {
			rev := revisionWithHiddenAuthor(t)
			h := NewRevisionHandler(
				&testhelpers.MockRevisionService{
					GetEntityHistoryFn: func(_ string, _ uint, _, _ int, _ contracts.RevisionViewer) ([]adminm.Revision, int64, error) {
						return []adminm.Revision{rev}, 1, nil
					},
				},
				nil,
			)

			resp, err := h.GetEntityHistoryHandler(tc.ctx, &GetEntityHistoryRequest{
				EntityType: "artist",
				EntityID:   "10",
			})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(resp.Body.Revisions) != 1 {
				t.Fatalf("expected 1 revision, got %d", len(resp.Body.Revisions))
			}

			if tc.want.IsAdmin {
				assertAuthorNamed(t, resp.Body.Revisions[0])
				return
			}
			assertAuthorSuppressed(t, resp.Body.Revisions[0])
		})
	}
}

func TestRevisionHandler_GetRevision_HiddenAuthorByTier(t *testing.T) {
	for _, tc := range revisionViewerTierCases {
		t.Run(tc.name, func(t *testing.T) {
			rev := revisionWithHiddenAuthor(t)
			h := NewRevisionHandler(
				&testhelpers.MockRevisionService{
					GetRevisionFn: func(_ uint, _ contracts.RevisionViewer) (*adminm.Revision, error) {
						return &rev, nil
					},
				},
				nil,
			)

			resp, err := h.GetRevisionHandler(tc.ctx, &GetRevisionRequest{RevisionID: "1"})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if tc.want.IsAdmin {
				assertAuthorNamed(t, resp.Body)
				return
			}
			assertAuthorSuppressed(t, resp.Body)
		})
	}
}

// The per-user route names its subject in the PATH, so suppressing the byline
// here withholds the NAME and not the fact that user 5 made these edits. That
// residual is deliberate and matches the show submitter byline, which likewise
// keeps submitted_by; the counting-and-differencing family is PSY-1939's. What
// this pins is that the route does not become the one place the name still
// leaks.
func TestRevisionHandler_GetUserRevisions_HiddenAuthorByTier(t *testing.T) {
	for _, tc := range revisionViewerTierCases {
		t.Run(tc.name, func(t *testing.T) {
			rev := revisionWithHiddenAuthor(t)
			h := NewRevisionHandler(
				&testhelpers.MockRevisionService{
					GetUserRevisionsFn: func(_ uint, _, _ int, _ contracts.RevisionViewer) ([]adminm.Revision, int64, error) {
						return []adminm.Revision{rev}, 1, nil
					},
				},
				nil,
			)

			resp, err := h.GetUserRevisionsHandler(tc.ctx, &GetUserRevisionsRequest{UserID: "5"})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(resp.Body.Revisions) != 1 {
				t.Fatalf("expected 1 revision, got %d", len(resp.Body.Revisions))
			}

			if tc.want.IsAdmin {
				assertAuthorNamed(t, resp.Body.Revisions[0])
				return
			}
			assertAuthorSuppressed(t, resp.Body.Revisions[0])
		})
	}
}

// The audit row is the durable record of what the admin's action did, so a row
// naming only the revision would record a full undo that did not happen. The
// write is fire-and-forget inside a goroutine, which is exactly why it needs an
// assertion: a refactor that dropped the fields would be invisible.
func TestRevisionHandler_Rollback_AuditRowNamesTheRefusedFields(t *testing.T) {
	recorded := make(chan map[string]interface{}, 1)
	h := NewRevisionHandler(
		&testhelpers.MockRevisionService{
			RollbackFn: func(_ context.Context, _ uint, _ uint) (*contracts.RollbackResult, error) {
				return &contracts.RollbackResult{
					AppliedFields: []string{"name"},
					SkippedFields: []contracts.RollbackSkippedField{
						{Field: "spotify", Reason: "Spotify URL must be on spotify.com"},
					},
				}, nil
			},
		},
		&testhelpers.MockAuditLogService{
			LogActionFn: func(_ uint, action string, _ string, _ uint, metadata map[string]interface{}) {
				if action == "revision_rollback" {
					recorded <- metadata
				}
			},
		},
	)

	if _, err := h.RollbackRevisionHandler(revisionAdminCtx(), &RollbackRevisionRequest{RevisionID: "7"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var metadata map[string]interface{}
	select {
	case metadata = <-recorded:
	case <-time.After(2 * time.Second):
		t.Fatal("no revision_rollback audit row was written")
	}

	skipped, ok := metadata["skipped_fields"].([]contracts.RollbackSkippedField)
	if !ok || len(skipped) != 1 || skipped[0].Field != "spotify" {
		t.Fatalf("audit metadata skipped_fields = %#v, want the refused spotify field", metadata["skipped_fields"])
	}
	if skipped[0].Reason == "" {
		t.Error("the audit row must carry the reason a field was refused")
	}
	applied, ok := metadata["applied_fields"].([]string)
	if !ok || len(applied) != 1 || applied[0] != "name" {
		t.Fatalf("audit metadata applied_fields = %#v, want [name]", metadata["applied_fields"])
	}
}

// A nil result with no error is not something the service produces, but the
// generated mock's zero value is exactly that, so the handler has to answer
// rather than panic the request.
func TestRevisionHandler_Rollback_NilResultIsAnError(t *testing.T) {
	h := NewRevisionHandler(&testhelpers.MockRevisionService{}, nil)

	_, err := h.RollbackRevisionHandler(revisionAdminCtx(), &RollbackRevisionRequest{RevisionID: "1"})
	testhelpers.AssertHumaError(t, err, 422)
}
