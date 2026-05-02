package routes

import (
	"github.com/danielgtaylor/huma/v2"

	notificationh "psychic-homily-backend/internal/api/handlers/notification"
)

// setupNotificationFilterRoutes configures notification filter and notification log endpoints.
// CRUD and notifications require authentication. Unsubscribe is public (HMAC-signed).
func setupNotificationFilterRoutes(rc RouteContext) {
	filterHandler := notificationh.NewNotificationFilterHandler(rc.SC.NotificationFilter, rc.Cfg.JWT.SecretKey)

	// Protected: filter CRUD
	huma.Get(rc.Protected, "/me/notification-filters", filterHandler.ListFiltersHandler)
	huma.Post(rc.Protected, "/me/notification-filters", filterHandler.CreateFilterHandler)
	huma.Patch(rc.Protected, "/me/notification-filters/{id}", filterHandler.UpdateFilterHandler)
	huma.Delete(rc.Protected, "/me/notification-filters/{id}", filterHandler.DeleteFilterHandler)
	huma.Post(rc.Protected, "/me/notification-filters/quick", filterHandler.QuickCreateFilterHandler)

	// Protected: notification log
	huma.Get(rc.Protected, "/me/notifications", filterHandler.GetNotificationsHandler)

	// Public: HMAC-signed unsubscribe
	huma.Post(rc.API, "/unsubscribe/filter/{id}", filterHandler.UnsubscribeFilterHandler)
}
