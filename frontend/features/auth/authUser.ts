import type { UserTier } from './types'
import type { NavMode } from '@/lib/nav-mode'

/**
 * The viewer identity every consumer of `useAuthContext()` reads.
 *
 * Reached only through {@link toAuthUser}, which `AuthProvider` runs on both
 * the profile query's payload and the object handed to `setUser`, so the
 * profile and the in-session override cannot describe the same viewer
 * differently.
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
 * and one mapper cover all of them. Fields are optional because this is also
 * the read contract for cached payloads written by older builds.
 *
 * Two fields are declared narrower than the wire actually carries, inherited
 * from the per-endpoint types this replaced: `id` is a JSON number and `email`
 * is nullable (`components['schemas']['User']` in types/api.d.ts, generated
 * from the backend model). The values pass through unconverted, so consumers
 * that need a number call `Number(user.id)`.
 *
 * `user_tier` is a bare string rather than {@link UserTier}: the value is a
 * server-controlled enum, and the cast to it happens in {@link toAuthUser}.
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
 * Fields are enumerated rather than spread: the backend serializes its whole
 * user model on these endpoints, including `preferences`, `privacy_settings`
 * and `is_active`, none of which belongs in the context value every
 * auth-consuming component re-renders on. The list is the allowlist.
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
