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
 *   - `isLoading` IS overridable, because 'pending' covers two windows that
 *     differ in it: the profile in flight (true), and a profile that failed
 *     on a non-definitive error (false).
 *
 * Read the `AuthStatus` docblock before gating a component on any of these.
 */
export function makeAuthFixture<TUser>(logout: () => void) {
  return (
    overrides: Partial<Omit<MockAuthContextValue<TUser>, 'isAuthenticated'>> = {}
  ): MockAuthContextValue<TUser> => {
    const authStatus = overrides.authStatus ?? 'anonymous'
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
