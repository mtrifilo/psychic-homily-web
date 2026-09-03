package shared_test

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"gorm.io/gorm"

	"psychic-homily-backend/internal/api/handlers/shared/testhelpers"
	communitym "psychic-homily-backend/internal/models/community"
	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/services/shared"
	"psychic-homily-backend/internal/testutil"
)

// The entity-request rule has ONE spelling, so this file checks it against a
// hand-written truth table rather than against a sibling: a FULFILLED request is
// public, an unfulfilled one is requester-or-admin, and a request that does not
// exist is refused to everybody.
//
// The table is written out here rather than derived from the implementation for
// the reason the show and collection files give. A test that recomputed the rule
// would agree with a bug.
//
// FULFILLED IS READ FROM created_entity_id, not from decision_state, and the
// matrix carries a row where the two disagree: an approved request whose
// fulfilment deferred is approved and has created nothing, and it must answer
// like a pending one.
func wantEntityRequestVisible(viewerIsRequester, viewerIsAdmin, requestExists, fulfilled bool) bool {
	if !requestExists {
		return false
	}
	if fulfilled {
		return true
	}
	return viewerIsRequester || viewerIsAdmin
}

// entityRequestCase is one request state every viewer tier is checked against.
// The missing id runs through the SAME truth table as the real ones, so "refused
// to everybody" is a row of the table rather than a hardcoded assertion beside
// it.
type entityRequestCase struct {
	name          string
	exists        bool
	fulfilled     bool
	decisionState communitym.EntityRequestDecisionState
}

func TestEntityRequestVisibilityMatrix(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	td := testutil.SetupTestPostgres(t)
	defer td.Cleanup()

	// entity_requests.requester_id carries a foreign key, so the matrix is built
	// on real user rows. The admin viewer carries a STRANGER's id as well as the
	// admin flag, so the admin row fails if the rule started answering on the id
	// rather than on the flag.
	requesterID := testhelpers.CreateTestUser(td.DB).ID
	strangerID := testhelpers.CreateTestUser(td.DB).ID

	// One row per state the rule can meet, each a real row so the SQL decides it
	// rather than a fixture standing in for one.
	pending := createEntityRequest(t, td.DB, requesterID, "artist", nil,
		communitym.EntityRequestStatePending)
	rejected := createEntityRequest(t, td.DB, requesterID, "artist", nil,
		communitym.EntityRequestStateRejected)
	// APPROVED AND FULFILLED: the catalog entity exists, so the row is public.
	createdID := uint(4242)
	fulfilled := createEntityRequest(t, td.DB, requesterID, "artist", &createdID,
		communitym.EntityRequestStateApproved)
	// APPROVED AND NOT FULFILLED, which is the state the two possible rules
	// disagree about: a show approval whose fulfilment deferred. It has created
	// nothing, so it stays private.
	orphan := createEntityRequest(t, td.DB, requesterID, "show", nil,
		communitym.EntityRequestStateApproved)

	viewers := []struct {
		name        string
		viewer      contracts.ShowViewer
		isRequester bool
		isAdmin     bool
	}{
		{"anonymous", contracts.ShowViewer{}, false, false},
		{"an authenticated stranger", contracts.ShowViewer{UserID: strangerID}, false, false},
		{"the requester", contracts.ShowViewer{UserID: requesterID}, true, false},
		{"an admin", contracts.ShowViewer{UserID: strangerID, IsAdmin: true}, false, true},
	}

	// THE ID ARRIVES AS TEXT, which is the shape the timeline evaluates it in:
	// the gate reads the request id out of the audit row's JSON metadata,
	// because catalog merges rewrite the entity_id column for these rows.
	cases := []struct {
		entityRequestCase
		id uint
	}{
		{entityRequestCase{"a pending request", true, false,
			communitym.EntityRequestStatePending}, pending.ID},
		{entityRequestCase{"a rejected request", true, false,
			communitym.EntityRequestStateRejected}, rejected.ID},
		{entityRequestCase{"an approved and fulfilled request", true, true,
			communitym.EntityRequestStateApproved}, fulfilled.ID},
		{entityRequestCase{"an approved request that created nothing", true, false,
			communitym.EntityRequestStateApproved}, orphan.ID},
		// A REQUEST THAT IS NOT THERE MUST ANSWER LIKE A REFUSED ONE, or the
		// pair enumerates the request id space: an id that answers "absent" and
		// one that answers "not yours" would be distinguishable.
		{entityRequestCase{"a request id that names no row", false, false, ""},
			pending.ID + 100000},
	}

	for _, v := range viewers {
		cond, args := shared.VisibleEntityRequestTextIDExistsSQL("meta.value", v.viewer)

		for _, c := range cases {
			want := wantEntityRequestVisible(v.isRequester, v.isAdmin, c.exists, c.fulfilled)
			got := countTextExprMatching(t, td.DB, fmt.Sprint(c.id), cond, args)
			if (got > 0) != want {
				t.Errorf("VisibleEntityRequestTextIDExistsSQL for %s and %s (state %q) matched %d rows, want visible=%v",
					v.name, c.name, c.decisionState, got, want)
			}
		}

		// A NON-NUMERIC id answers "no such request" rather than raising inside
		// the statement it is spliced into.
		if got := countTextExprMatching(t, td.DB, "not-an-id", cond, args); got != 0 {
			t.Errorf("VisibleEntityRequestTextIDExistsSQL served %s a non-numeric id", v.name)
		}
	}
}

func createEntityRequest(
	t *testing.T, db *gorm.DB, requesterID uint, entityType string,
	createdEntityID *uint, state communitym.EntityRequestDecisionState,
) *communitym.EntityRequest {
	t.Helper()
	payload := json.RawMessage(`{"name":"Matrix Subject"}`)
	request := &communitym.EntityRequest{
		EntityType:      entityType,
		Payload:         &payload,
		RequesterID:     requesterID,
		SourceContext:   communitym.EntityRequestSourceManual,
		DecisionState:   state,
		CreatedEntityID: createdEntityID,
		CreatedAt:       time.Now().UTC(),
		UpdatedAt:       time.Now().UTC(),
	}
	if err := db.Create(request).Error; err != nil {
		t.Fatalf("create entity request: %v", err)
	}
	return request
}
