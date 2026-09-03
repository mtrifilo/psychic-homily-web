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
// hand-written truth table rather than against a sibling: requester or admin,
// and a request that does not exist is refused to both.
//
// The table is written out here rather than derived from the implementation for
// the reason the show and collection files give. A test that recomputed the rule
// would agree with a bug.
func wantEntityRequestVisible(viewerIsRequester, viewerIsAdmin, requestExists bool) bool {
	if !requestExists {
		return false
	}
	return viewerIsRequester || viewerIsAdmin
}

// entityRequestCase is one id state every viewer tier is checked against. The
// missing id runs through the SAME truth table as the real one, so "refused to
// both" is a row of the table rather than a hardcoded assertion beside it.
type entityRequestCase struct {
	name   string
	exists bool
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

	request := createEntityRequest(t, td.DB, requesterID, "artist")

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
	cases := []entityRequestCase{
		{"an existing request", true},
		// A REQUEST THAT IS NOT THERE MUST ANSWER LIKE A REFUSED ONE, or the
		// pair enumerates the request id space: an id that answers "absent" and
		// one that answers "not yours" would be distinguishable.
		{"a request id that names no row", false},
	}

	for _, v := range viewers {
		cond, args := shared.VisibleEntityRequestTextIDExistsSQL("meta.value", v.viewer)

		for _, c := range cases {
			id := request.ID
			if !c.exists {
				id = request.ID + 100000
			}
			want := wantEntityRequestVisible(v.isRequester, v.isAdmin, c.exists)
			got := countTextExprMatching(t, td.DB, fmt.Sprint(id), cond, args)
			if (got > 0) != want {
				t.Errorf("VisibleEntityRequestTextIDExistsSQL for %s and %s matched %d rows, want visible=%v",
					v.name, c.name, got, want)
			}
		}

		// A NON-NUMERIC id answers "no such request" rather than raising inside
		// the statement it is spliced into.
		if got := countTextExprMatching(t, td.DB, "not-an-id", cond, args); got != 0 {
			t.Errorf("VisibleEntityRequestTextIDExistsSQL served %s a non-numeric id", v.name)
		}
	}
}

func createEntityRequest(t *testing.T, db *gorm.DB, requesterID uint, entityType string) *communitym.EntityRequest {
	t.Helper()
	payload := json.RawMessage(`{"name":"Matrix Subject"}`)
	request := &communitym.EntityRequest{
		EntityType:    entityType,
		Payload:       &payload,
		RequesterID:   requesterID,
		SourceContext: communitym.EntityRequestSourceManual,
		DecisionState: communitym.EntityRequestStatePending,
		CreatedAt:     time.Now().UTC(),
		UpdatedAt:     time.Now().UTC(),
	}
	if err := db.Create(request).Error; err != nil {
		t.Fatalf("create entity request: %v", err)
	}
	return request
}
