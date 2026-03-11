package user

import "psychic-homily-backend/internal/services/contracts"

// Compile-time interface satisfaction checks for user services.
var (
	_ contracts.UserServiceInterface               = (*UserService)(nil)
	_ contracts.ContributorProfileServiceInterface = (*ContributorProfileService)(nil)
)
