package notification

import (
	"slices"
	"sort"
	"testing"

	notificationm "psychic-homily-backend/internal/models/notification"
)

// The vocabulary GET /me/notifications?types= accepts must name exactly the
// entity types the inbox can hold. inboxEntityTypeDispositions is the
// inventory every new discriminator is forced to join (by its own completeness
// test), so binding the vocabulary to it means a new type cannot ship
// filterable-by-nobody: a client asking for it would get a 422.
//
// It also pins the two comment literals the models package cannot import.
func TestNotificationEntityTypesMatchInboxInventory(t *testing.T) {
	inventory := make([]string, 0, len(inboxEntityTypeDispositions))
	for entityType := range inboxEntityTypeDispositions {
		inventory = append(inventory, entityType)
	}
	sort.Strings(inventory)

	vocabulary := slices.Clone(notificationm.NotificationEntityTypes)
	sort.Strings(vocabulary)

	if !slices.Equal(vocabulary, inventory) {
		t.Errorf("models/notification.NotificationEntityTypes = %v\n"+
			"but the inbox inventory holds                 %v\n"+
			"Add the missing discriminator to NotificationEntityTypes (or remove the stale one) "+
			"so the types filter accepts exactly what the inbox can return.",
			vocabulary, inventory)
	}
}
