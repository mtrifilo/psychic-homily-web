import type { AuthStatus } from '@/lib/context/AuthContext'

/**
 * The shape a suite mocking `useAuthContext()` returns.
 *
 * `TUser` is the suite's own minimal user shape, so each suite states only the
 * fields its component reads rather than building a whole `User`.
 */
export type MockAuthContextValue<TUser> = {
  user: TUser | null
  isAuthenticated: boolean
  authStatus: AuthStatus
  isLoading: boolean
  logout: () => void
}

/**
 * Build the `authFixture` a suite hands to its mocked `useAuthContext()`.
 *
 * Lives here rather than in each suite because the builder encodes invariants
 * of the real provider, and a copy that drifts from them keeps passing while
 * describing a viewer the provider cannot produce:
 *
 *   - `isAuthenticated` is DERIVED from `authStatus` and is not overridable,
 *     so no test can describe a viewer whose two auth signals disagree.
 *   - A non-null `user` and `authStatus === 'authenticated'` imply each other.
 *     `AuthProvider` derives the status from the user with `if (user) return
 *     'authenticated'` as its first clause, and no other clause reaches that
 *     value, so "a stale user beside an unsettled status" is not a state a
 *     component can be handed. Asking for it throws rather than passing
 *     vacuously.
 *   - `isLoading` IS overridable, because it partitions neither status. It
 *     is TanStack's `isPending && isFetching`, so 'pending' carries it true
 *     while a fetch is open and false both before one starts and after one
 *     fails, and 'authenticated' carries it true while a logout is in flight.
 *
 * Read the `AuthStatus` docblock before gating a component on any of these.
 */
export function makeAuthFixture<TUser>(logout: () => void) {
  return (
    overrides: Partial<Omit<MockAuthContextValue<TUser>, 'isAuthenticated'>> = {}
  ): MockAuthContextValue<TUser> => {
    const authStatus = overrides.authStatus ?? 'anonymous'
    const user = overrides.user ?? null
    if ((user !== null) !== (authStatus === 'authenticated')) {
      throw new Error(
        `authFixture: a ${user === null ? 'null' : 'non-null'} user cannot ` +
          `accompany authStatus '${authStatus}'. AuthProvider derives one from ` +
          `the other, so this viewer is unreachable and a test asserting ` +
          `against it proves nothing.`
      )
    }
    return {
      user: null,
      authStatus,
      isLoading: authStatus === 'pending',
      logout,
      ...overrides,
      isAuthenticated: authStatus === 'authenticated',
    }
  }
}
