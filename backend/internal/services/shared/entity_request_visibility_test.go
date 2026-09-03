package shared_test

import (
	"encoding/json"
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

	for _, v := range viewers {
		cond, args := shared.VisibleEntityRequestExistsSQL("e.entity_id", v.viewer)

		want := wantEntityRequestVisible(v.isRequester, v.isAdmin, true)
		// countEntityRowMatching is the synthetic-one-row probe the show and
		// collection matrices use. Its entity_type column is inert here: this
		// condition reads the id alone, which is what makes an ACTION-keyed
		// family necessary in the first place.
		if got := countEntityRowMatching(t, td.DB, "artist", request.ID, cond, args); (got > 0) != want {
			t.Errorf("VisibleEntityRequestExistsSQL for %s matched %d rows for an existing request, want visible=%v",
				v.name, got, want)
		}

		// A REQUEST THAT IS NOT THERE ANSWERS THE SAME AS A REFUSED ONE. Without
		// this the pair enumerates the request id space: an id that answers
		// "absent" and one that answers "not yours" would be distinguishable.
		missing := request.ID + 100000
		if got := countEntityRowMatching(t, td.DB, "artist", missing, cond, args); got != 0 {
			t.Errorf("VisibleEntityRequestExistsSQL served %s a request id that names no row", v.name)
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
