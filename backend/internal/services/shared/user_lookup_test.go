package shared

import (
	"testing"

	"gorm.io/gorm"

	authm "psychic-homily-backend/internal/models/auth"
	"psychic-homily-backend/internal/testutil"
)

// Each users lookup answers a value Postgres rejects as text as not found
// without querying, so the database never sees a parameter it refuses. The
// database here fails every query, which makes a query that does run visible
// as an error.
func TestUserLookups_RejectedTextIsNotFoundWithoutQuerying(t *testing.T) {
	lookups := map[string]func(*gorm.DB, string) (*authm.User, error){
		"LiveUserByUsername":            LiveUserByUsername,
		"UserByUsernameIncludingClosed": UserByUsernameIncludingClosed,
		"UserByEmailIncludingClosed":    UserByEmailIncludingClosed,
	}
	for lookupName, lookup := range lookups {
		for valueName, value := range map[string]string{
			"NUL":           "no\x00body",
			"invalid UTF-8": "no\xffbody",
		} {
			t.Run(lookupName+"/"+valueName, func(t *testing.T) {
				db, queries := testutil.CountQueries(testutil.UnavailableDB(t))

				user, err := lookup(db, value)
				if err != nil || user != nil {
					t.Fatalf("%s(%q) = (%v, %v), want (nil, nil)", lookupName, value, user, err)
				}
				if *queries != 0 {
					t.Fatalf("%s(%q) ran %d queries, want 0", lookupName, value, *queries)
				}

				// Any other value reaches the database and returns its error.
				if _, err := lookup(db, "nobody"); err == nil {
					t.Fatalf("%s(plain value) on an unavailable database succeeded, want the query's error", lookupName)
				}
				if *queries == 0 {
					t.Fatalf("%s(plain value) ran no query", lookupName)
				}
			})
		}
	}
}

// A lookup never changes the *gorm.DB it reads through, so one chain carrying
// the caller's Preload serves any number of lookups.
func TestUserLookups_ReuseOfTheCallersChain(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	td := testutil.SetupTestPostgres(t)
	defer td.Cleanup()

	usernames := []string{"lookup-chain-first", "lookup-chain-second"}
	for _, username := range usernames {
		email := username + "@example.com"
		name := username
		if err := td.DB.Create(&authm.User{Email: &email, Username: &name, IsActive: true}).Error; err != nil {
			t.Fatalf("create %s: %v", username, err)
		}
	}

	chain := td.DB.Preload("Preferences")
	lookups := map[string]func(*gorm.DB, string) (*authm.User, error){
		"LiveUserByUsername":            LiveUserByUsername,
		"UserByUsernameIncludingClosed": UserByUsernameIncludingClosed,
	}
	for name, lookup := range lookups {
		for _, username := range usernames {
			user, err := lookup(chain, username)
			if err != nil || user == nil || user.Username == nil || *user.Username != username {
				t.Errorf("%s(%q) through a reused chain = (%v, %v), want the account", name, username, user, err)
			}
		}
	}
	for _, username := range usernames {
		user, err := UserByEmailIncludingClosed(chain, username+"@example.com")
		if err != nil || user == nil {
			t.Errorf("UserByEmailIncludingClosed(%q) through a reused chain = (%v, %v), want the account", username, user, err)
		}
	}
}
