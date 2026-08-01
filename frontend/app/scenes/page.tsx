import { Suspense } from 'react'
import { HydrationBoundary } from '@tanstack/react-query'
import { LoadingSpinner } from '@/components/shared'
import { SceneList } from '@/features/scenes'
import type { SceneListResponse } from '@/features/scenes/types'
import { API_ENDPOINTS } from '@/lib/api'
import { queryKeys } from '@/lib/queryClient'
import { seedFirstScreen } from '@/lib/query-hydration'
import { fetchListPayload } from '@/lib/ssr/fetchListPayload'

export const metadata = {
  title: 'Scenes',
  description: 'Explore music scenes by city — venues, artists, shows, and live music activity.',
  alternates: {
    canonical: 'https://psychichomily.com/scenes',
  },
  openGraph: {
    title: 'Scenes | Psychic Homily',
    description: 'Explore music scenes by city — venues, artists, shows, and live music activity.',
    url: '/scenes',
    type: 'website',
  },
}

function SceneListLoading() {
  return (
    <div className="flex justify-center items-center py-12">
      <LoadingSpinner />
    </div>
  )
}

/**
 * Seed the cache `useScenes` reads, so the scene cards are in the server HTML
 * instead of appearing only after the client's own fetch resolves (PSY-1624).
 *
 * `SceneList` reads no URL state and no per-visitor state, so the server and
 * the client's hydration render agree unconditionally — this page is the
 * simple end of the three the ticket covers.
 *
 * A failed fetch deliberately renders `<SceneList />` UNSEEDED rather than
 * throwing: the component fetches for itself and owns the error state. See
 * `fetchListPayload`.
 */
async function HydratedSceneList() {
  const scenes = await fetchListPayload<SceneListResponse>({
    url: API_ENDPOINTS.SCENES.LIST,
    collection: 'scenes',
    service: 'scenes-listing',
  })

  if (!scenes) {
    return <SceneList />
  }

  const dehydratedState = await seedFirstScreen([
    { queryKey: queryKeys.scenes.list, data: scenes },
  ])

  return (
    <HydrationBoundary state={dehydratedState}>
      <SceneList />
    </HydrationBoundary>
  )
}

export default function ScenesPage() {
  return (
    <div className="flex min-h-screen items-start justify-center">
      <main className="w-full max-w-6xl px-4 py-8 md:px-8">
        <h1 className="text-3xl font-bold text-center mb-2">Scenes</h1>
        <p className="text-center text-muted-foreground mb-8 max-w-lg mx-auto">
          City-level music scenes with venue activity, artist trends, and live show data.
        </p>
        <Suspense fallback={<SceneListLoading />}>
          <HydratedSceneList />
        </Suspense>
      </main>
    </div>
  )
}
