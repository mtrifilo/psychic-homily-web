import type { UserTier } from '@/features/auth/types'
import type { NavMode } from '@/lib/nav-mode'

/**
 * The viewer identity every consumer of `useAuthContext()` reads.
 *
 * Produced from an API payload only through {@link toAuthUser}, so the profile
 * query and the in-session override built at login cannot describe the same
 * viewer differently.
 */
export interface User {
  id: string
  email: string
  username?: string
  display_name?: string
  first_name?: string
  last_name?: string
  bio?: string
  // Free-text "City, state" (PSY-1416). Optional on the public profile meta line.
  location?: string
  // OAuth / profile avatar URL (PSY-1488). Passed through from /auth/profile.
  avatar_url?: string
  email_verified: boolean
  is_admin?: boolean
  user_tier?: UserTier
  // Saved nav-style preference (PSY-1117). Read by the appearance settings
  // toggle to seed its control; the server shell (AppShell) reads it directly
  // from the profile for first-paint rendering.
  nav_mode?: NavMode
}

/**
 * The `user` payload the auth API returns.
 *
 * `/auth/profile` and every endpoint that establishes a session (password
 * login, registration, magic-link verification, account recovery, passkey
 * login, passkey signup) serialize the same backend user model, so one shape
 * and one mapper cover all of them. Fields are optional because this type is
 * also the read contract for cached payloads written by older builds.
 *
 * `user_tier` is a bare string rather than {@link UserTier}: the value is a
 * server-controlled enum this client does not validate, and narrowing it
 * happens once, in {@link toAuthUser}.
 */
export interface AuthApiUser {
  id: string
  email: string
  username?: string
  display_name?: string
  first_name?: string
  last_name?: string
  bio?: string
  location?: string
  avatar_url?: string
  is_admin?: boolean
  email_verified?: boolean
  user_tier?: string
  nav_mode?: NavMode
}

/**
 * The single adapter from an auth API payload to the context {@link User}.
 *
 * Every field the context exposes is mapped here. A call site that builds the
 * object itself can omit a privilege field (`is_admin`) or state a placeholder
 * for one it does not know (`email_verified`), and the override then wins over
 * the real profile for the rest of the SPA session, so the fix for that class
 * is to leave no other place where the object is assembled.
 */
export function toAuthUser(apiUser: AuthApiUser): User {
  return {
    id: apiUser.id,
    email: apiUser.email,
    username: apiUser.username,
    display_name: apiUser.display_name,
    first_name: apiUser.first_name,
    last_name: apiUser.last_name,
    bio: apiUser.bio,
    location: apiUser.location,
    avatar_url: apiUser.avatar_url,
    email_verified: apiUser.email_verified ?? false,
    is_admin: apiUser.is_admin,
    user_tier: apiUser.user_tier as UserTier | undefined,
    nav_mode: apiUser.nav_mode,
  }
}
