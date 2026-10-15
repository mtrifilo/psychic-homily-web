package community

import (
	"testing"

	"psychic-homily-backend/internal/testutil"
)

// A username holding a NUL byte returns the empty page without querying, so the
// database never sees a parameter it rejects. The lookup answers an empty page
// for a failed query too, so the query count, not the result, tells the two
// apart.
func TestGetUserPublicCollectionsByUsername_NULUsernameIsEmptyWithoutQuerying(t *testing.T) {
	db, queries := testutil.CountQueries(testutil.UnavailableDB(t))
	svc := NewCollectionService(db)

	collections, total, err := svc.GetUserPublicCollectionsByUsername("no\x00body", 20, 0)
	if err != nil {
		t.Fatalf("GetUserPublicCollectionsByUsername(NUL username) error = %v, want nil", err)
	}
	if collections == nil || len(collections) != 0 || total != 0 {
		t.Fatalf("GetUserPublicCollectionsByUsername(NUL username) = (%v, %d), want an empty non-nil page", collections, total)
	}
	if *queries != 0 {
		t.Fatalf("GetUserPublicCollectionsByUsername(NUL username) ran %d queries, want 0", *queries)
	}

	// Any other username reaches the database.
	if _, _, err := svc.GetUserPublicCollectionsByUsername("nobody", 20, 0); err != nil {
		t.Fatalf("GetUserPublicCollectionsByUsername(plain username) error = %v, want nil", err)
	}
	if *queries == 0 {
		t.Fatal("GetUserPublicCollectionsByUsername(plain username) ran no query")
	}
}
