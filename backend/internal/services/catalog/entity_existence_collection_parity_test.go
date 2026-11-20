package catalog_test

import (
	"fmt"
	"testing"

	"gorm.io/gorm"

	"psychic-homily-backend/internal/api/handlers/shared/testhelpers"
	"psychic-homily-backend/internal/services/catalog"
	"psychic-homily-backend/internal/services/community"
	"psychic-homily-backend/internal/testutil"
)

// The collection existence probe and the collection detail read answer a caller
// without a session alike, for every creator state and visibility, by slug and
// by numeric id. The probe gates the detail page, so a disagreement either
// hides a page the detail read serves or passes a page it refuses.
func TestCollectionExistsAgreesWithTheAnonymousDetailRead(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	td := testutil.SetupTestPostgres(t)
	defer td.Cleanup()

	probe := catalog.NewEntityExistenceService(td.DB, nil)
	detail := community.NewCollectionService(td.DB)

	creatorStates := []struct {
		name  string
		apply func(t *testing.T, db *gorm.DB, userID uint)
	}{
		{"live, public profile", func(*testing.T, *gorm.DB, uint) {}},
		{"live, private profile", func(t *testing.T, db *gorm.DB, userID uint) {
			testutil.SetProfileVisibility(t, db, userID, "private")
		}},
		{"deactivated", testutil.DeactivateAccount},
		{"soft-deleted", testutil.SoftDeleteAccount},
	}

	n := 0
	for _, state := range creatorStates {
		for _, isPublic := range []bool{true, false} {
			n++
			t.Run(fmt.Sprintf("%s, public=%v", state.name, isPublic), func(t *testing.T) {
				creatorID := testhelpers.CreateTestUser(td.DB).ID
				slug := fmt.Sprintf("probe-parity-%d", n)
				collection := testhelpers.CreateCollection(t, td.DB, creatorID,
					fmt.Sprintf("Probe Parity %d", n), slug, isPublic)
				state.apply(t, td.DB, creatorID)
				id := fmt.Sprint(collection.ID)

				_, bySlugErr := detail.GetBySlug(slug, 0)
				_, byIDErr := detail.GetByID(collection.ID, 0)
				served := bySlugErr == nil
				if (byIDErr == nil) != served {
					t.Fatalf("the detail read disagrees with itself: by slug served=%v, by id served=%v", served, byIDErr == nil)
				}

				for _, segment := range []string{slug, id} {
					exists, err := probe.CollectionExists(segment)
					if err != nil {
						t.Fatalf("CollectionExists(%q): %v", segment, err)
					}
					if exists != served {
						t.Errorf("CollectionExists(%q) = %v, the anonymous detail read served = %v", segment, exists, served)
					}
				}
			})
		}
	}
}
