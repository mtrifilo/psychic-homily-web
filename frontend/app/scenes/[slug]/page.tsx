import { Suspense, cache } from 'react'
import type { Metadata } from 'next'
import { notFound } from 'next/navigation'
import { Loader2 } from 'lucide-react'
import * as Sentry from '@sentry/nextjs'
import { HydrationBoundary } from '@tanstack/react-query'
import { SceneDetailView } from '@/features/scenes'
import type { SceneDetail } from '@/features/scenes'
import { API_BASE_URL } from '@/lib/api-base'
import { queryKeys } from '@/lib/queryClient'
import { prefetchEntity } from '@/lib/query-hydration'

interface ScenePageProps {
  params: Promise<{ slug: string }>
}

/**
 * Scenes are DERIVED from location data (verified venues + the artists/shows
 * at them), not a stored slug entity, so any string could otherwise be
 * title-cased into a real-looking "City, ST Music Scene" page (PSY-906).
 *
 * `GET /scenes/{slug}` is the authoritative existence check: the backend
 * resolves the slug against verified venues and returns 404 for an
 * unparseable slug OR a location below the scene threshold (the same guard
 * every sub-fetch the page renders — active artists, scene graph — already
 * enforces). Fetching it here, server-side, lets the route return a real
 * HTTP 404 (rendering the root `not-found.tsx`) instead of the soft-404 the
 * client `SceneDetailView` would paint at HTTP 200.
 *
 * Wrapped in `React.cache()` so `generateMetadata` and the page body share
 * ONE backend round-trip per request. The result also seeds the TanStack
 * Query cache via `prefetchEntity` below so the matching `useSceneDetail`
 * hook resolves from cache instead of refetching on first paint. Returns
 * null for non-2xx (404 expected for bogus slugs) so the page can call
 * `notFound()`.
 */
const getScene = cache(async (slug: string): Promise<SceneDetail | null> => {
  try {
    const res = await fetch(`${API_BASE_URL}/scenes/${slug}`, {
      next: { revalidate: 3600 },
    })
    if (res.ok) {
      return res.json()
    }
    // Don't report 404s — they're the expected response for invalid /
    // below-threshold slugs (the whole point of this check).
    if (res.status >= 500) {
      Sentry.captureMessage(`Scene page: API returned ${res.status}`, {
        level: 'error',
        tags: { service: 'scene-page' },
        extra: { slug, status: res.status },
      })
    }
  } catch (error) {
    Sentry.captureException(error, {
      level: 'error',
      tags: { service: 'scene-page' },
      extra: { slug },
    })
  }
  return null
})

export async function generateMetadata({
  params,
}: ScenePageProps): Promise<Metadata> {
  const { slug } = await params
  const scene = await getScene(slug)

  // Resolve the title from the real scene record rather than title-casing the
  // slug — a nonexistent scene must NOT emit a fabricated "City, ST Music
  // Scene" title (PSY-906). The page body calls `notFound()` for the missing
  // case, so return an explicit not-found title to avoid flashing a generic
  // one before the not-found page mounts.
  if (!scene) {
    return {
      title: 'Scene not found',
      description: 'The music scene you are looking for does not exist.',
    }
  }

  const title = `${scene.city}, ${scene.state} Music Scene`
  const description = `Explore the ${scene.city}, ${scene.state} music scene — venues, active artists, upcoming shows, and scene pulse.`

  return {
    title,
    description,
    alternates: {
      canonical: `https://psychichomily.com/scenes/${slug}`,
    },
    openGraph: {
      title: `${title} | Psychic Homily`,
      description,
      url: `/scenes/${slug}`,
      type: 'website',
    },
  }
}

function SceneLoadingFallback() {
  return (
    <div className="flex min-h-[60vh] items-center justify-center">
      <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
    </div>
  )
}

export default async function ScenePage({ params }: ScenePageProps) {
  const { slug } = await params

  if (!slug) {
    notFound()
  }

  // Server-side existence check: a slug that doesn't resolve to a qualifying
  // scene must return HTTP 404 so the route renders `not-found.tsx`. Without
  // this, `SceneDetailView` renders a friendly "Scene not found" message at
  // HTTP 200 — a soft-404 that poisons SEO, monitoring, and crawlers.
  const scene = await getScene(slug)
  if (!scene) {
    notFound()
  }

  // `cache()` above guarantees the fetch already happened, so this is a no-op
  // cache write that seeds the entry `useSceneDetail` will pick up.
  const dehydratedState = await prefetchEntity(
    queryKeys.scenes.detail(slug),
    scene,
  )

  return (
    <div className="flex min-h-screen items-start justify-center">
      <main className="w-full max-w-6xl px-4 py-8 md:px-8">
        <HydrationBoundary state={dehydratedState}>
          <Suspense fallback={<SceneLoadingFallback />}>
            <SceneDetailView slug={slug} />
          </Suspense>
        </HydrationBoundary>
      </main>
    </div>
  )
}
