package auth

import "psychic-homily-backend/internal/services/contracts"

// Compile-time interface satisfaction checks for auth sub-package.
var (
	_ contracts.AuthServiceInterface       = (*AuthService)(nil)
	_ contracts.JWTServiceInterface        = (*JWTService)(nil)
	_ contracts.PasswordValidatorInterface = (*PasswordValidator)(nil)
	_ contracts.AppleAuthServiceInterface  = (*AppleAuthService)(nil)
	_ contracts.WebAuthnServiceInterface   = (*WebAuthnService)(nil)
)
