/**
 * TanStack Query Configuration
 *
 * This module configures TanStack Query with environment-aware settings
 * and provides query client utilities for the application.
 */

import { QueryClient, DefaultOptions } from '@tanstack/react-query'
import { ArtistSearchParams } from './types/artist'

// Default query options for all queries
const defaultQueryOptions: DefaultOptions = {
    queries: {
        // Stale time: how long data is considered fresh
        staleTime: 5 * 60 * 1000, // 5 minutes
        // Cache time: how long data stays in cache after last use
        gcTime: 10 * 60 * 1000, // 10 minutes (formerly cacheTime)
        // Retry configuration
        retry: (failureCount, error: any) => {
            // Don't retry on 4xx errors (client errors)
            if (error?.status >= 400 && error?.status < 500) {
                return false
            }
            // Retry up to 3 times for other errors
            return failureCount < 3
        },
        // Refetch on window focus (useful for development)
        refetchOnWindowFocus: import.meta.env.DEV,
        // Refetch on reconnect
        refetchOnReconnect: true,
    },
    mutations: {
        // Retry mutations once on failure
        retry: 1,
    },
}

// Create the query client with environment-aware configuration
export const queryClient = new QueryClient({
    defaultOptions: defaultQueryOptions,
})

// Query key factory for consistent key generation
export const queryKeys = {
    // Authentication queries
    auth: {
        profile: ['auth', 'profile'] as const,
        user: (id: string) => ['auth', 'user', id] as const,
    },

    // Show queries
    shows: {
        all: ['shows'] as const,
        list: (filters?: Record<string, any>) => ['shows', 'list', filters] as const,
        detail: (id: string) => ['shows', 'detail', id] as const,
        userShows: (userId: string) => ['shows', 'user', userId] as const,
    },

    // Venue queries
    venues: {
        all: ['venues'] as const,
        list: (filters?: Record<string, any>) => ['venues', 'list', filters] as const,
        detail: (id: string) => ['venues', 'detail', id] as const,
    },

    artists: {
        search: (params: ArtistSearchParams) => ['artists', 'search', params.query.toLowerCase()],
    },

    // System queries
    system: {
        health: ['system', 'health'] as const,
    },
} as const

// Utility function to invalidate related queries
export const invalidateQueries = {
    // Invalidate all auth-related queries
    auth: () => queryClient.invalidateQueries({ queryKey: ['auth'] }),

    // Invalidate all show-related queries
    shows: () => queryClient.invalidateQueries({ queryKey: ['shows'] }),

    // Invalidate specific show queries
    show: (id: string) => queryClient.invalidateQueries({ queryKey: ['shows', 'detail', id] }),

    artists: () => queryClient.invalidateQueries({ queryKey: ['artists'] }),

    // Invalidate all venue-related queries
    venues: () => queryClient.invalidateQueries({ queryKey: ['venues'] }),
}
