'use client'

import { QueryClientProvider } from '@tanstack/react-query'
import { NuqsAdapter } from 'nuqs/adapters/next/app'
import { getQueryClient } from '@/lib/queryClient'
import { AuthProvider } from '@/lib/context/AuthContext'
// PSY-961: app-level Create-collection drawer, openable from any surface
// (the /collections button + the AddToCollectionButton "Create … with
// {entity}" CTA). Its heavy form is lazy-loaded inside the provider, so this
// import only adds the Sheet shell + context to the root chunk.
import { CreateCollectionDrawerProvider } from '@/features/collections/components/CreateCollectionDrawer'

interface ProvidersProps {
    children: React.ReactNode
}

/**
 * Session-independent providers: URL search-param plumbing and the TanStack
 * Query cache. Everything mounted here must work before the viewer's profile
 * is known, because this sits ABOVE the `<HydrationBoundary>` that seeds the
 * profile cache.
 *
 * Deliberately does NOT mount `AuthProvider` — see `<SessionProviders>`.
 */
export function Providers({ children }: ProvidersProps) {
    const queryClient = getQueryClient()

    return (
        // NuqsAdapter provides the URL-update mechanism for `useQueryState`
        // (the App Router adapter). Outermost so every search-param consumer in
        // the tree — the shows/venues/artists/explore filter surfaces — sits
        // under it.
        <NuqsAdapter>
            <QueryClientProvider client={queryClient}>
                {children}
            </QueryClientProvider>
        </NuqsAdapter>
    )
}

/**
 * Providers that read the viewer's profile out of the query cache.
 *
 * These MUST render below the `<HydrationBoundary>` in `<AuthHydrator>`.
 * `AuthProvider` derives its state from `useProfile()`, so mounting it above
 * the boundary leaves the single server render with an empty cache and the
 * whole tree renders its `isLoading` branch — a spinner shell in the SSR HTML
 * for authenticated and anonymous viewers alike. Below the boundary the seeded
 * cache is already in context, so the server renders the real signed-in (or
 * signed-out) tree.
 *
 * `CreateCollectionDrawerProvider` travels with it because the form it hosts
 * calls `useAuthContext()`; left above the boundary it would throw
 * "useAuthContext must be used within an AuthProvider" the first time a viewer
 * opened the drawer.
 */
export function SessionProviders({ children }: ProvidersProps) {
    return (
        <AuthProvider>
            <CreateCollectionDrawerProvider>
                {children}
            </CreateCollectionDrawerProvider>
        </AuthProvider>
    )
}
