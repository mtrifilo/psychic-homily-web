/**
 * Read and write one stored preference on the cached `/auth/profile` payload,
 * for optimistic preference writes.
 *
 * Preferences hang off `user.preferences`, NOT off the user. Reading one
 * level too high resolves to `undefined` for every viewer, which looks exactly
 * like "never set".
 *
 * Every settle-time write (a success reconcile or an error rollback) is
 * guarded on `readProfileViewerId` still matching the id captured when the
 * write started: `logout()` clears the cache without cancelling in-flight
 * requests, so an unguarded write can land in the next account's profile or
 * rebuild a signed-out viewer's entry.
 */

interface CachedProfile {
  user?: {
    id?: string | number
    preferences?: Record<string, unknown> | null
  } | null
}

/** The id of the viewer the cached payload names, if any. */
export function readProfileViewerId(cached: unknown): unknown {
  return (cached as CachedProfile | undefined)?.user?.id
}

/** One stored preference; absent and null both read as null. */
export function readProfilePreference<T>(cached: unknown, key: string): T | null {
  const profile = cached as CachedProfile | undefined
  return (profile?.user?.preferences?.[key] as T | null | undefined) ?? null
}

/** The payload with one preference replaced and everything else untouched.
 *  An entry that names no user is returned as is: there is no viewer to hold
 *  a preference. */
export function withProfilePreference(
  cached: unknown,
  key: string,
  value: unknown
): unknown {
  const profile = cached as CachedProfile | undefined
  if (!profile?.user) return cached
  return {
    ...profile,
    user: {
      ...profile.user,
      preferences: { ...profile.user.preferences, [key]: value },
    },
  }
}
