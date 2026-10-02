/**
 * Read and write one stored preference on the cached `/auth/profile` payload,
 * for optimistic preference writes.
 *
 * Preferences hang off `user.preferences`, NOT off the user. Reading one
 * level too high resolves to `undefined` for every viewer, which looks exactly
 * like "never set".
 *
 * `withProfilePreference` is for the optimistic write a mutation makes as it
 * starts. Anything written when a request SETTLES (a success reconcile, an
 * error rollback) goes through `settleProfilePreference`, which refuses to
 * write into a different viewer's payload.
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

/**
 * `withProfilePreference`, but only while the cached payload still names the
 * viewer captured when the write started; otherwise the payload is returned
 * as is. A request can outlive its session: `logout()` clears the cache
 * without cancelling in-flight requests, so an unguarded settle can land in
 * the next account's profile or rebuild a signed-out viewer's entry.
 */
export function settleProfilePreference(
  cached: unknown,
  viewerId: unknown,
  key: string,
  value: unknown
): unknown {
  return readProfileViewerId(cached) === viewerId
    ? withProfilePreference(cached, key, value)
    : cached
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
