package notification

import (
	"fmt"
	"slices"
	"strings"
)

// NotificationEntityTypes is every entity_type a notification_log row can
// carry: the vocabulary a client may name when it narrows an inbox read
// (GET /me/notifications?types=...).
//
// The comment pair is spelled as literals because its constants live in
// services/engagement, which this package cannot import. Those two literals,
// and the completeness of this list, are pinned against the service's
// visibility inventory (inboxEntityTypeDispositions) by
// TestNotificationEntityTypesMatchInboxInventory, so a discriminator added
// there without a line here fails that test instead of answering 422 to every
// client that asks for it.
var NotificationEntityTypes = []string{
	NotificationEntityShow,
	NotificationEntityArtistShowAlert,
	NotificationEntityVenueShowAlert,
	NotificationEntityArtistReleaseDigest,
	NotificationEntityRequestFulfillmentProposed,
	"comment_reply",
	"comment_mention",
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
