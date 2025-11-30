import { createContext, useContext, useEffect, useState, ReactNode } from 'react'
import { useProfile } from '@/lib/hooks/useAuth'

interface User {
    id: string
    email: string
    first_name?: string
    last_name?: string
    email_verified: boolean
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
    const [user, setUser] = useState<User | null>(null)
    const [error, setError] = useState<string | null>(null)

    // Use the useProfile hook to get authentication status
    const { data: profileData, isLoading, error: profileError } = useProfile()
    console.log('profileData', profileData)

    // Update user state when profile data changes
    useEffect(() => {
        if (profileData?.success && profileData?.user) {
            // Convert the API user format to AuthContext User format
            const user = {
                id: profileData.user.id,
                email: profileData.user.email,
                first_name: profileData.user.first_name,
                last_name: profileData.user.last_name,
                email_verified: false, // Default value, update when profile endpoint is available
            }
            setUser(user)
        } else {
            setUser(null)
        }
    }, [profileData])

    // Update error state when profile error changes
    useEffect(() => {
        if (profileError) {
            setError(profileError.message || 'Authentication failed')
        } else {
            setError(null)
        }
    }, [profileError])

    const logout = async () => {
        try {
            setError(null)
            setUser(null)
            // The useProfile hook will automatically handle the logout state
            // when the server clears the HTTP-only cookie
        } catch (err) {
            console.error('Logout failed:', err)
        }
    }

    const clearError = () => {
        setError(null)
    }

    const value: AuthContextType = {
        user,
        isAuthenticated: Boolean(user),
        isLoading,
        error,
        setUser,
        setError,
        clearError,
        logout,
    }

    return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>
}

export function useAuthContext() {
    const context = useContext(AuthContext)
    if (context === undefined) {
        throw new Error('useAuth must be used within an AuthProvider')
    }
    return context
}
