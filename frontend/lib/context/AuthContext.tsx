'use client'

import {
  createContext,
  useContext,
  useState,
  useMemo,
  useCallback,
  ReactNode,
} from 'react'
import { useProfile, useLogout } from '@/features/auth'
import type { UserTier } from '@/features/auth'
import type { NavMode } from '@/lib/nav-mode'

interface User {
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

interface AuthState {
  user: User | null
  isAuthenticated: boolean
  isLoading: boolean
  error: string | null
}

interface AuthContextType extends AuthState {
  setUser: (user: User | null) => void
  setError: (error: string | null) => void
  clearError: () => void
  logout: () => void
}

const AuthContext = createContext<AuthContextType | undefined>(undefined)

interface AuthProviderProps {
  children: ReactNode
}

export function AuthProvider({ children }: AuthProviderProps) {
  // Local state for manual user/error overrides (e.g., after signup)
  const [userOverride, setUserOverride] = useState<User | null | undefined>(
    undefined
  )
  const [errorOverride, setErrorOverride] = useState<string | null>(null)

  // Use the useProfile hook to get authentication status
  const { data: profileData, isLoading, error: profileError } = useProfile()
  const logoutMutation = useLogout()

  // Derive user from profile data or override
  const user = useMemo(() => {
    // If there's an explicit user override (truthy), use it.
    // Note: null means "no override" - logout clears via queryClient.clear().
    // Login/signup build the override from the minimal auth response, which
    // omits nav_mode; backfill it from the full profile so the appearance
    // settings control (PSY-1117) seeds from the saved preference for the rest
    // of the SPA session, not the default. The override still wins for every
    // field it actually sets.
    if (userOverride) {
      return {
        ...userOverride,
        nav_mode: userOverride.nav_mode ?? profileData?.user?.nav_mode,
      }
    }

    // Otherwise derive from profile data
    if (profileData?.success && profileData?.user) {
      return {
        id: profileData.user.id,
        email: profileData.user.email,
        username: profileData.user.username,
        display_name: profileData.user.display_name,
        first_name: profileData.user.first_name,
        last_name: profileData.user.last_name,
        bio: profileData.user.bio,
        location: profileData.user.location,
        avatar_url: profileData.user.avatar_url,
        email_verified: profileData.user.email_verified ?? false,
        is_admin: profileData.user.is_admin,
        user_tier: profileData.user.user_tier as UserTier | undefined,
        nav_mode: profileData.user.nav_mode,
      }
    }

    return null
  }, [profileData, userOverride])

  // Derive error from profile error or override
  const error = useMemo(() => {
    if (errorOverride !== null) {
      return errorOverride
    }
    if (profileError) {
      return profileError.message || 'Authentication failed'
    }
    return null
  }, [profileError, errorOverride])

  const setUser = useCallback((newUser: User | null) => {
    setUserOverride(newUser)
    setErrorOverride(null)
  }, [])

  const setError = useCallback((newError: string | null) => {
    setErrorOverride(newError)
  }, [])

  const logout = useCallback(async () => {
    try {
      setErrorOverride(null)
      setUserOverride(null)
      // The useProfile hook will automatically handle the logout state
      // when the server clears the HTTP-only cookie
      await logoutMutation.mutateAsync()
    } catch (err) {
      // Logout failure is non-critical — cookie will expire naturally
    }
  }, [logoutMutation])

  const clearError = useCallback(() => {
    setErrorOverride(null)
  }, [])

  const value: AuthContextType = useMemo(
    () => ({
      user,
      isAuthenticated: Boolean(user),
      isLoading: isLoading || logoutMutation.isPending,
      error,
      setUser,
      setError,
      clearError,
      logout,
    }),
    [
      user,
      isLoading,
      logoutMutation.isPending,
      error,
      setUser,
      setError,
      clearError,
      logout,
    ]
  )

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>
}

export function useAuthContext() {
  const context = useContext(AuthContext)
  if (context === undefined) {
    throw new Error('useAuthContext must be used within an AuthProvider')
  }
  return context
}
