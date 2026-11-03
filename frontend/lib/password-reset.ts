/**
 * How long an emailed password reset link works after it is sent, in minutes.
 * The backend's token lifetime (contracts.PasswordResetTokenTTL in
 * backend/internal/services/contracts/auth.go) is pinned to this value by a
 * backend test, so the copy that states it cannot drift from it.
 */
export const PASSWORD_RESET_EXPIRY_MINUTES = 15

/** The page an emailed reset link opens; its `token` parameter is the link. */
export const PASSWORD_RESET_PATH = '/auth/reset-password'
