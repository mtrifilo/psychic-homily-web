package community

import (
	"testing"

	"psychic-homily-backend/internal/testutil"
)

// A username Postgres rejects as text returns the empty page without querying,
// so the database never sees a parameter it refuses. Over a failing database a
// username that reaches the lookup returns the error, so the empty page and the
// zero query count both show the guard answered first.
func TestGetUserPublicCollectionsByUsername_RejectedTextUsernameIsEmptyWithoutQuerying(t *testing.T) {
	for name, username := range map[string]string{
		"NUL":           "no\x00body",
		"invalid UTF-8": "no\xffbody",
	} {
		t.Run(name, func(t *testing.T) {
			db, queries := testutil.CountQueries(testutil.UnavailableDB(t))
			svc := NewCollectionService(db)

			collections, total, err := svc.GetUserPublicCollectionsByUsername(username, 20, 0)
			if err != nil {
				t.Fatalf("GetUserPublicCollectionsByUsername(%q) error = %v, want nil", username, err)
			}
			if collections == nil || len(collections) != 0 || total != 0 {
				t.Fatalf("GetUserPublicCollectionsByUsername(%q) = (%v, %d), want an empty non-nil page", username, collections, total)
			}
			if *queries != 0 {
				t.Fatalf("GetUserPublicCollectionsByUsername(%q) ran %d queries, want 0", username, *queries)
			}

			// Any other username reaches the database.
			if _, _, err := svc.GetUserPublicCollectionsByUsername("nobody", 20, 0); err == nil {
				t.Fatal("GetUserPublicCollectionsByUsername(plain username) over a failing database error = nil, want the lookup's error")
			}
			if *queries == 0 {
				t.Fatal("GetUserPublicCollectionsByUsername(plain username) ran no query")
			}
		})
	}
}
