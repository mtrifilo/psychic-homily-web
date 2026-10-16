package user

import (
	"testing"

	"psychic-homily-backend/internal/services/contracts"
	"psychic-homily-backend/internal/testutil"
)

// A username Postgres rejects as text is not found without querying, so the
// database never sees a parameter it refuses. The database here fails every
// query, which makes a query that does run visible as an error.
func TestGetPublicProfile_RejectedTextUsernameIsNotFoundWithoutQuerying(t *testing.T) {
	for name, username := range map[string]string{
		"NUL":           "no\x00body",
		"invalid UTF-8": "no\xffbody",
	} {
		t.Run(name, func(t *testing.T) {
			db, queries := testutil.CountQueries(testutil.UnavailableDB(t))
			svc := NewContributorProfileService(db)

			profile, err := svc.GetPublicProfile(username, contracts.ShowViewer{})
			if err != nil {
				t.Fatalf("GetPublicProfile(%q) error = %v, want nil", username, err)
			}
			if profile != nil {
				t.Fatalf("GetPublicProfile(%q) = %+v, want nil", username, profile)
			}
			if *queries != 0 {
				t.Fatalf("GetPublicProfile(%q) ran %d queries, want 0", username, *queries)
			}

			// Any other username reaches the database.
			if _, err := svc.GetPublicProfile("nobody", contracts.ShowViewer{}); err == nil {
				t.Fatal("GetPublicProfile(plain username) on an unavailable database succeeded, want the lookup's error")
			}
			if *queries == 0 {
				t.Fatal("GetPublicProfile(plain username) ran no query")
			}
		})
	}
}
