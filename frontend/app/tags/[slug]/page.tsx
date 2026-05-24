import { Suspense, cache } from 'react'
import type { Metadata } from 'next'
import { notFound } from 'next/navigation'
import * as Sentry from '@sentry/nextjs'
import { Loader2 } from 'lucide-react'
import { HydrationBoundary, dehydrate } from '@tanstack/react-query'
import { TagDetail } from '@/features/tags/components'
import type { TagEnrichedDetailResponse } from '@/features/tags/types'
import { getQueryClient, queryKeys } from '@/lib/queryClient'

const API_BASE_URL =
  process.env.NEXT_PUBLIC_API_URL ||
  (process.env.NODE_ENV === 'development'
    ? 'http://localhost:8080'
    : 'https://api.psychichomily.com')

interface TagPageProps {
  params: Promise<{ slug: string }>
}

/**
 * Fetch the enriched tag detail (GET /tags/{slug}/detail) — the same shape
 * the client `useTagDetail` hook consumes. Server hydration eliminates the
 * client refetch, so paying for the enriched shape once on the server is
 * cheaper than the previous lightweight-metadata + client-enriched waterfall.
 *
 * Wrapped in `React.cache()` so `generateMetadata` and the page body share
 * one round-trip per request. Returns null for 404s (expected for invalid
 * slugs) so the page can call `notFound()` for a hard HTTP 404 rather than
 * letting the client render a soft-404 that poisons SEO and monitoring.
 */
const getTagDetail = cache(
  async (slug: string): Promise<TagEnrichedDetailResponse | null> => {
    try {
      const res = await fetch(`${API_BASE_URL}/tags/${slug}/detail`, {
        next: { revalidate: 3600 },
      })
      if (res.ok) {
        return res.json()
      }
      if (res.status >= 500) {
        Sentry.captureMessage(`Tag page fetch error: ${res.status}`, {
          level: 'error',
          tags: { service: 'tag-page' },
          extra: { slug, status: res.status },
        })
      }
    } catch (error) {
      Sentry.captureException(error, {
        level: 'error',
        tags: { service: 'tag-page' },
        extra: { slug },
      })
    }
    return null
  }
)

export async function generateMetadata({
  params,
}: TagPageProps): Promise<Metadata> {
  const { slug } = await params
  const tag = await getTagDetail(slug)

  if (tag) {
    const usageCount = tag.usage_count ?? 0
    // Description ties the tag to the things people actually browse for —
    // shows/artists/releases — so the SERP snippet is concrete rather than
    // generic boilerplate.
    const description =
      usageCount > 0
        ? `Browse ${usageCount} ${usageCount === 1 ? 'entity' : 'entities'} (artists, shows, releases, and more) tagged ${tag.name} on Psychic Homily.`
        : `Discover artists, shows, and releases tagged ${tag.name} on Psychic Homily.`

    return {
      title: `${tag.name} | Tags`,
      description,
      alternates: {
        canonical: `https://psychichomily.com/tags/${slug}`,
      },
      openGraph: {
        title: `${tag.name} | Tags | Psychic Homily`,
        description,
        type: 'website',
        url: `/tags/${slug}`,
      },
    }
  }

  // Missing tag: the page component will call `notFound()` and Next.js will
  // render `app/tags/[slug]/not-found.tsx`, which owns its own metadata.
  // Returning a generic "Tag" title here would show briefly before the
  // not-found page mounts, so use the explicit not-found title instead.
  return {
    title: 'Tag not found',
    description: 'The tag you are looking for does not exist.',
  }
}

function TagLoadingFallback() {
  return (
    <div className="flex min-h-[60vh] items-center justify-center">
      <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
    </div>
  )
}

export default async function TagDetailPage({ params }: TagPageProps) {
  const { slug } = await params

  // Server-side existence check: unknown slugs must return HTTP 404 so the
  // route renders `not-found.tsx`. Without `notFound()` the client component
  // would render a friendly "Tag Not Found" page at HTTP 200 — a soft-404
  // that poisons SEO, monitoring, and crawlers.
  const tag = await getTagDetail(slug)
  if (!tag) {
    notFound()
  }

  // `cache()` above guarantees the network call already happened, so the
  // sync `queryFn` is a no-op cache write that just seeds the dehydrated
  // entry the client `useTagDetail` hook will pick up.
  const queryClient = getQueryClient()
  await queryClient.prefetchQuery({
    queryKey: queryKeys.tags.enrichedDetail(slug),
    queryFn: () => tag,
  })

  return (
    <HydrationBoundary state={dehydrate(queryClient)}>
      <Suspense fallback={<TagLoadingFallback />}>
        <TagDetail slug={slug} />
      </Suspense>
    </HydrationBoundary>
  )
}
