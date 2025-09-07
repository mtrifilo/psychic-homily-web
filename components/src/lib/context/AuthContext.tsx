import React, { createContext, useContext, useEffect, useState, ReactNode } from 'react'
import { apiRequest, API_ENDPOINTS } from '@/lib/api'

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
    setLoading: (loading: boolean) => void
    clearError: () => void
    logout: () => void
}

const AuthContext = createContext<AuthContextType | undefined>(undefined)

interface AuthProviderProps {
    children: ReactNode
}

export function AuthProvider({ children }: AuthProviderProps) {
    const [user, setUser] = useState<User | null>(null)
    const [isLoading, setIsLoading] = useState(true)
    const [error, setError] = useState<string | null>(null)

    // Check if user is already logged in on mount
    useEffect(() => {
        checkAuthStatus()
    }, [])

    const checkAuthStatus = async () => {
        try {
            setIsLoading(true)
            const userData = await apiRequest<User>(API_ENDPOINTS.AUTH.PROFILE)
            setUser(userData)
        } catch (err) {
            console.error('Auth check failed:', err)
            setUser(null)
        } finally {
            setIsLoading(false)
        }
    }

    const logout = async () => {
        try {
            setError(null)
            setIsLoading(true)

            await apiRequest(API_ENDPOINTS.AUTH.LOGOUT, {
                method: 'POST',
            })

            setUser(null)
        } catch (err) {
            console.error('Logout failed:', err)
        } finally {
            setIsLoading(false)
        }
    }

    const clearError = () => {
        setError(null)
    }

    const value: AuthContextType = {
        user,
        isAuthenticated: !!user,
        isLoading,
        error,
        setUser,
        setError,
        setLoading: setIsLoading,
        clearError,
        logout,
    }

    return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>
}

export function useAuth() {
    const context = useContext(AuthContext)
    if (context === undefined) {
        throw new Error('useAuth must be used within an AuthProvider')
    }
    return context
}
