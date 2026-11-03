package catalog

import (
	"testing"
	"time"

	notificationm "psychic-homily-backend/internal/models/notification"
)

func TestAlertWeekStart(t *testing.T) {
	monday := time.Date(2026, 11, 2, 0, 0, 0, 0, time.UTC)
	cases := []struct {
		name string
		in   time.Time
		want time.Time
	}{
		{"monday midnight", monday, monday},
		{"wednesday afternoon", time.Date(2026, 11, 4, 15, 30, 0, 0, time.UTC), monday},
		{"sunday last second", time.Date(2026, 11, 8, 23, 59, 59, 0, time.UTC), monday},
		{"next monday opens the next week", time.Date(2026, 11, 9, 0, 0, 0, 0, time.UTC),
			time.Date(2026, 11, 9, 0, 0, 0, 0, time.UTC)},
		// Sunday evening in Phoenix is already Monday in UTC.
		{"bucketed in UTC, not the caller's zone",
			time.Date(2026, 11, 8, 20, 0, 0, 0, time.FixedZone("MST", -7*3600)),
			time.Date(2026, 11, 9, 0, 0, 0, 0, time.UTC)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := AlertWeekStart(tc.in); !got.Equal(tc.want) || got.Location() != time.UTC {
				t.Errorf("AlertWeekStart(%v) = %v, want %v in UTC", tc.in, got, tc.want)
			}
		})
	}
}

// A release digest's entity_id is a USER id and its subject_entity_id is NULL,
// so no merge or delete inventory that rewrites catalog ids may name it.
func TestReleaseDigestIsInNoCatalogIDInventory(t *testing.T) {
	for _, typ := range artistSubjectAlertTypes {
		if typ == notificationm.NotificationEntityArtistReleaseDigest {
			t.Error("an artist merge would rewrite a reader's user id on their release digest rows")
		}
	}
	for entity, types := range alertTypesKeyedOnEntity {
		for _, typ := range types {
			if typ == notificationm.NotificationEntityArtistReleaseDigest {
				t.Errorf("deleting a %s would delete inbox rows keyed on a user id", entity)
			}
		}
	}
}
