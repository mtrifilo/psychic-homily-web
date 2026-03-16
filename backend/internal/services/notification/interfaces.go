package notification

import "psychic-homily-backend/internal/services/contracts"

// Compile-time interface satisfaction checks for notification services.
var (
	_ contracts.EmailServiceInterface              = (*EmailService)(nil)
	_ contracts.DiscordServiceInterface            = (*DiscordService)(nil)
	_ contracts.NotificationFilterServiceInterface = (*NotificationFilterService)(nil)
)
