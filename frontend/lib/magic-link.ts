/**
 * How long an emailed sign-in link works after it is sent, in minutes. The
 * backend's token lifetime (contracts.MagicLinkTokenTTL in
 * backend/internal/services/contracts/auth.go) is pinned to this value by a
 * backend test, so the copy that states it cannot drift from it.
 */
export const MAGIC_LINK_EXPIRY_MINUTES = 15
