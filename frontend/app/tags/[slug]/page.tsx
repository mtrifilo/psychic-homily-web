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
 * Fetch the enriched tag detail (GET /tags/{slug}/detail). PSY-485 originally
 * used the lightweight /tags/{slug} endpoint here on the theory that the
 * client component would refetch the enriched payload anyway. PSY-798 wires
 * `<HydrationBoundary>` for `useTagDetail`, which inverts the trade: a single
 * enriched server fetch now covers metadata + page render + client hydration,
 * so the lightweight call would just be wasted bytes.
 *
 * Wrapped in `React.cache()` so `generateMetadata` and the page body share
 * one backend round-trip per request. Returns null for 404s (expected for
 * invalid slugs) — the page component uses that signal to call `notFound()`
 * for a hard HTTP 404 instead of a soft-404 (PSY-497).
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

  // Server-side existence check: if the backend doesn't know this slug, call
  // `notFound()` so Next.js returns HTTP 404 and renders the route's
  // `not-found.tsx`. Without this, the client component would render a
  // friendly "Tag Not Found" page but the response status stays 200 — a
  // soft-404 that poisons SEO, monitoring, and crawlers (PSY-497).
  const tag = await getTagDetail(slug)
  if (!tag) {
    notFound()
  }

  // Seed a request-scoped QueryClient with the enriched tag payload the
  // server already fetched, then dehydrate so the client `useTagDetail` hook
  // resolves from the cache instead of refetching. The queryFn returns the
  // cached value synchronously — `cache()` above guarantees the network call
  // has already happened, so this is a no-op cache write rather than a
  // refetch.
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
