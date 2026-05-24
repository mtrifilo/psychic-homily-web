'use client'

import {
    HydrationBoundary,
    QueryClientProvider,
    type DehydratedState,
} from '@tanstack/react-query'
import { getQueryClient } from '@/lib/queryClient'
import { AuthProvider } from '@/lib/context/AuthContext'

interface ProvidersProps {
    children: React.ReactNode
    /**
     * Pre-hydrated TanStack Query state from the root layout. Today this
     * only carries the `/auth/profile` cache entry so `useProfile`
     * resolves from cache on first paint, eliminating the race where
     * auth-gated buttons render interactive before the client profile
     * fetch settles. Optional so callers that don't prefetch (e.g. unit
     * tests) keep working without a noop placeholder.
     */
    authState?: DehydratedState
}

export function Providers({ children, authState }: ProvidersProps) {
    const queryClient = getQueryClient()

    // HydrationBoundary must live inside QueryClientProvider so it seeds
    // the same client the AuthProvider's useProfile hook will read from.
    return (
        <QueryClientProvider client={queryClient}>
            <HydrationBoundary state={authState}>
                <AuthProvider>{children}</AuthProvider>
            </HydrationBoundary>
        </QueryClientProvider>
    )
}

