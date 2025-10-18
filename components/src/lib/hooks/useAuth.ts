/**
 * Authentication Hooks
 *
 * TanStack Query hooks for authentication operations with HTTP-only cookies.
 * Uses proper caching, error handling, and optimistic updates.
 */

import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { apiRequest, API_ENDPOINTS } from '../api'
import { queryKeys, invalidateQueries } from '../queryClient'

// Types
interface LoginCredentials {
    email: string
    password: string
}

interface RegisterCredentials {
    email: string
    password: string
    first_name?: string
    last_name?: string
}

interface AuthResponse {
    success: boolean
    message: string
    user?: {
        id: string
        email: string
        name?: string
        first_name?: string
        last_name?: string
    }
}

interface UserProfile {
    success: boolean
    message: string
    user?: {
        id: string
        email: string
        name?: string
        first_name?: string
        last_name?: string
        created_at: string
        updated_at: string
    }
}

// Login mutation
export const useLogin = () => {
    const queryClient = useQueryClient()

    return useMutation({
        mutationFn: async (credentials: LoginCredentials): Promise<AuthResponse> => {
            const response = await apiRequest<AuthResponse>(API_ENDPOINTS.AUTH.LOGIN, {
                method: 'POST',
                body: JSON.stringify(credentials),
                credentials: 'include', // Include cookies in request
            })
            
            // Throw an error if login was unsuccessful
            if (!response.success) {
                const error: any = new Error(response.message || 'Login failed')
                error.status = 401
                error.details = response
                throw error
            }
            
            return response
        },
        onSuccess: (data) => {
            if (data.success && data.user) {
                // Set user data in cache (HTTP-only cookie is automatically set by server)
                queryClient.setQueryData(queryKeys.auth.profile, {
                    success: true,
                    message: data.message,
                    user: data.user,
                })

                // Invalidate and refetch auth queries
                invalidateQueries.auth()
            }
        },
        onError: () => {
            // Clear user data from cache on error
            queryClient.removeQueries({ queryKey: ['auth'] })
        },
    })
}

// Register mutation
export const useRegister = () => {
    const queryClient = useQueryClient()

    return useMutation({
        mutationFn: async (credentials: RegisterCredentials): Promise<AuthResponse> => {
            const response = await apiRequest<AuthResponse>(API_ENDPOINTS.AUTH.REGISTER, {
                method: 'POST',
                body: JSON.stringify(credentials),
                credentials: 'include', // Include cookies in request
            })
            
            // Throw an error if registration was unsuccessful
            if (!response.success) {
                const error: any = new Error(response.message || 'Registration failed')
                error.status = 400
                error.details = response
                throw error
            }
            
            return response
        },
        onSuccess: (data) => {
            if (data.success && data.user) {
                // Set user data in cache (HTTP-only cookie is automatically set by server)
                queryClient.setQueryData(queryKeys.auth.profile, {
                    success: true,
                    message: data.message,
                    user: data.user,
                })

                // Invalidate and refetch auth queries
                invalidateQueries.auth()
            }
        },
        onError: () => {
            // Clear user data from cache on error
            queryClient.removeQueries({ queryKey: ['auth'] })
        },
    })
}

// Logout mutation
export const useLogout = () => {
    const queryClient = useQueryClient()

    return useMutation({
        mutationFn: async (): Promise<{ success: boolean; message: string }> => {
            return apiRequest(API_ENDPOINTS.AUTH.LOGOUT, {
                method: 'POST',
                credentials: 'include', // Include cookies in request
            })
        },
        onSuccess: () => {
            // Clear all cached data (HTTP-only cookie is cleared by server)
            queryClient.clear()
        },
        onError: () => {
            // Clear cached data even on error (in case of network issues)
            queryClient.clear()
        },
    })
}

// Get user profile query
export const useProfile = () => {
    return useQuery({
        queryKey: queryKeys.auth.profile,
        queryFn: async (): Promise<UserProfile> => {
            return apiRequest(API_ENDPOINTS.AUTH.PROFILE, {
                method: 'GET',
                credentials: 'include', // Include cookies in request
            })
        },
        staleTime: 5 * 60 * 1000, // 5 minutes
        retry: (failureCount, error: any) => {
            // Don't retry on 401/403 errors (authentication issues)
            console.log('useProfile - status::', error?.status)
            if (error?.status === 401 || error?.status === 403) {
                return false
            }
            return failureCount < 2
        },
    })
}

// Refresh token mutation
export const useRefreshToken = () => {
    const queryClient = useQueryClient()

    return useMutation({
        mutationFn: async (): Promise<{ success: boolean; token?: string; message: string }> => {
            return apiRequest(API_ENDPOINTS.AUTH.REFRESH, {
                method: 'POST',
                credentials: 'include', // Include cookies in request
            })
        },
        onSuccess: (data) => {
            if (data.success) {
                // Invalidate auth queries to refetch with new token
                // (HTTP-only cookie is automatically updated by server)
                invalidateQueries.auth()
            }
        },
        onError: (error) => {
            console.error('Error refreshing token:', error)
            // Clear all cached data on refresh failure
            queryClient.clear()
        },
    })
}

// Check if user is authenticated
export const useIsAuthenticated = () => {
    const { data: profile, isLoading, error } = useProfile()

    console.log('profile', profile)

    if (error) {
        console.error('Error checking authentication:', error)
    }

    return {
        isAuthenticated: Boolean(profile?.success) && Boolean(profile?.user) && !error,
        isLoading,
        user: profile?.user,
        error,
    }
}
