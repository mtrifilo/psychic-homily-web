'use client'

import {
  createContext,
  useContext,
  useState,
  useRef,
  useMemo,
  useCallback,
  ReactNode,
} from 'react'
import { useProfile, useLogout } from '@/lib/hooks/useAuth'

interface User {
  id: string
  email: string
  first_name?: string
  last_name?: string
  email_verified: boolean
  is_admin?: boolean
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
  const logoutRef = useRef(logoutMutation)
  logoutRef.current = logoutMutation

  // Derive user from profile data or override
  const user = useMemo(() => {
    // If there's an explicit user override (truthy), use it
    // Note: null means "no override" - logout clears via queryClient.clear()
    if (userOverride) {
      return userOverride
    }

    // Otherwise derive from profile data
    if (profileData?.success && profileData?.user) {
      return {
        id: profileData.user.id,
        email: profileData.user.email,
        first_name: profileData.user.first_name,
        last_name: profileData.user.last_name,
        email_verified: profileData.user.email_verified ?? false,
        is_admin: profileData.user.is_admin,
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
      await logoutRef.current.mutateAsync()
    } catch (err) {
      console.error('Logout failed:', err)
    }
  }, [])

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
