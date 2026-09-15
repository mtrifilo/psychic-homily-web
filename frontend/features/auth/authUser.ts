import type { UserTier } from './types'
import type { NavMode } from '@/lib/nav-mode'

/**
 * The viewer identity every consumer of `useAuthContext()` reads.
 *
 * `AuthProvider` builds it with {@link toAuthUser} on both the profile query's
 * payload and the object handed to `setUser`, so the two sources produce the
 * same shape from the same fields.
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
 * and one mapper cover all of them.
 *
 * This is a hand-written mirror of `components['schemas']['User']` in
 * types/api.d.ts and does not match it: `email` and the seven other string
 * fields are nullable there, and every field the backend declares non-pointer
 * arrives on every response rather than being optional. Deriving it from the
 * generated schema instead is a change with its own blast radius, not a
 * rename.
 *
 * `id` is declared WIDER than the wire, which sends a JSON number
 * (`components['schemas']['User']` declares `id: number`). The union is not a
 * claim that a string arrives; it is what makes the narrowing in
 * {@link toAuthUser} a written step rather than an assumption, and it keeps
 * {@link toAuthUser} total over either spelling should a second serializer
 * ever answer these endpoints. `string` is what the context {@link User}
 * exposes, and this mapper is the only place the two meet.
 *
 * `user_tier` is a bare string rather than {@link UserTier}: the value is a
 * server-controlled enum, and {@link toAuthUser} asserts the union without
 * validating it. `nav_mode` is declared as its union here and is asserted the
 * same way, without a validating parse.
 */
export interface AuthApiUser {
  id: string | number
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
 * user model on these endpoints, and the fields below are the ones the context
 * exposes. The rest of what it sends — `preferences`, `privacy_settings`,
 * `is_active`, `profile_visibility`, `created_at`, `updated_at`, `deleted_at`,
 * `oauth_accounts`, `passkey_credentials` — stays out of the context value
 * every auth-consuming component re-renders on. The list is the allowlist.
 *
 * `id` is narrowed to `string` HERE, once, so every consumer of the context
 * reads one type and one spelling of the viewer's identity. A consumer that
 * converts an id itself is scoping a cache key or gating a control on a
 * conversion its neighbours may not make, and two spellings of one viewer are
 * two cache entries and two answers to "is this mine".
 *
 * An absent id maps to `''`, never to `String(undefined)`, which is the truthy
 * string `'undefined'`. `''` is falsy, so a viewer with no id matches nobody
 * and enables no viewer-scoped query: every gate on this value tests it for
 * truth, not against `undefined`.
 */
export function toAuthUser(apiUser: AuthApiUser): User {
  return {
    id: apiUser.id == null ? '' : String(apiUser.id),
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

/**
 * Any spelling of a user id a caller can hold: the context {@link User}'s
 * `string`, an entity payload's numeric owner column, or nothing at all.
 */
export type UserIdLike = string | number | null | undefined

/**
 * Whether two ids name the same user.
 *
 * ONE identity test, because "is this row mine" is asked on comments, field
 * notes, collections, requests, venues, shows and the leaderboard, and a
 * second copy of the rule is a surface that answers differently about the same
 * viewer.
 *
 * Compared as strings, not as numbers. `Number('')` is 0 and `Number(' 7 ')`
 * is 7, so a numeric comparison accepts blanks and padding as ids; string
 * equality accepts only the same digits. The coercion is here because the
 * viewer's id is a string and an entity's owner column is a JSON number.
 *
 * A falsy id on either side is an absent id, never a match: no user row has id
 * 0, and an anonymous viewer must not match an unowned entity.
 */
export function isSameUserId(a: UserIdLike, b: UserIdLike): boolean {
  return !!(a && b && String(a) === String(b))
}
