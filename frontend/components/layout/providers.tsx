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

export function Providers({ children }: ProvidersProps) {
    const queryClient = getQueryClient()

    return (
        // NuqsAdapter provides the URL-update mechanism for `useQueryState`
        // (the App Router adapter). Outermost so every search-param consumer in
        // the tree — the shows/venues/artists/explore filter surfaces — sits
        // under it.
        <NuqsAdapter>
            <QueryClientProvider client={queryClient}>
                <AuthProvider>
                    <CreateCollectionDrawerProvider>
                        {children}
                    </CreateCollectionDrawerProvider>
                </AuthProvider>
            </QueryClientProvider>
        </NuqsAdapter>
    )
}

