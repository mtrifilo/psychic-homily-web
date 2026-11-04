package notification

import (
	"fmt"
	"slices"
	"strings"
)

// NotificationEntityTypes is every entity_type a notification_log row can
// carry: the vocabulary a client may name when it narrows an inbox read
// (GET /me/notifications?types=...), and the declared inventory the inbox's
// show-visibility dispositions are checked against in both directions
// (services/notification inbox_visibility_inventory_test.go).
//
// A new discriminator is added HERE as well as to its const block. Nothing can
// enumerate a package's constants, so a constant left off this list is a 422
// for any client that asks for it and has no visibility disposition checked.
var NotificationEntityTypes = []string{
	NotificationEntityShow,
	NotificationEntityArtistShowAlert,
	NotificationEntityVenueShowAlert,
	NotificationEntityArtistReleaseDigest,
	NotificationEntityRequestFulfillmentProposed,
	NotificationEntityCommentReply,
	NotificationEntityCommentMention,
}

// ParseNotificationEntityTypes reads a comma-separated entity_type list into
// its distinct values, in first-seen order.
//
// A blank string means "no narrowing" and returns nil. Surrounding whitespace
// on each value is trimmed; an empty value (a stray comma) or a value outside
// NotificationEntityTypes is an error, because silently dropping it would
// answer a different question than the one the client asked.
func ParseNotificationEntityTypes(csv string) ([]string, error) {
	if strings.TrimSpace(csv) == "" {
		return nil, nil
	}
	parts := strings.Split(csv, ",")
	types := make([]string, 0, len(parts))
	for _, part := range parts {
		entityType := strings.TrimSpace(part)
		if entityType == "" {
			return nil, fmt.Errorf("types contains an empty value")
		}
		if !slices.Contains(NotificationEntityTypes, entityType) {
			return nil, fmt.Errorf("unknown notification type %q; expected one of %s",
				entityType, strings.Join(NotificationEntityTypes, ", "))
		}
		if !slices.Contains(types, entityType) {
			types = append(types, entityType)
		}
	}
	return types, nil
}
