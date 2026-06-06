import { Suspense, cache } from 'react'
import { Metadata } from 'next'
import { notFound } from 'next/navigation'
import { Loader2 } from 'lucide-react'
import * as Sentry from '@sentry/nextjs'
import { HydrationBoundary } from '@tanstack/react-query'
import { ShowDetail } from '@/features/shows'
import type { ShowResponse } from '@/features/shows/types'
import { JsonLd } from '@/components/seo/JsonLd'
import { generateMusicEventSchema, generateBreadcrumbSchema } from '@/lib/seo/jsonld'
import { resolveShowTimezone } from '@/lib/utils/formatters'
import { API_BASE_URL } from '@/lib/api-base'
import { queryKeys } from '@/lib/queryClient'
import { prefetchEntity } from '@/lib/query-hydration'

interface ShowPageProps {
  params: Promise<{ slug: string }>
}

/**
 * Wrapped with `React.cache()` so `generateMetadata` and the page body
 * share ONE backend fetch per request instead of two. The result also
 * seeds the TanStack Query cache via `prefetchEntity` below, eliminating
 * the client-side refetch on first paint.
 */
const getShow = cache(async (slug: string): Promise<ShowResponse | null> => {
  try {
    const res = await fetch(`${API_BASE_URL}/shows/${slug}`, {
      next: { revalidate: 3600 },
    })
    if (res.ok) {
      return res.json()
    }
    // Don't report 404s - they're expected for invalid slugs
    if (res.status >= 500) {
      Sentry.captureMessage(`Show page: API returned ${res.status}`, {
        level: 'error',
        tags: { service: 'show-page' },
        extra: { slug, status: res.status },
      })
    }
  } catch (error) {
    Sentry.captureException(error, {
      level: 'error',
      tags: { service: 'show-page' },
      extra: { slug },
    })
  }
  return null
})

function formatShowDate(
  dateString: string,
  state?: string | null,
  timezone?: string | null
): string {
  const date = new Date(dateString)
  return date.toLocaleDateString('en-US', {
    weekday: 'long',
    year: 'numeric',
    month: 'long',
    day: 'numeric',
    timeZone: resolveShowTimezone(state, timezone), // venue-local for SEO (PSY-986)
  })
}

export async function generateMetadata({ params }: ShowPageProps): Promise<Metadata> {
  const { slug } = await params
  const show = await getShow(slug)

  if (show) {
    const headliner = show.artists?.find(a => a.is_headliner)?.name || show.artists?.[0]?.name || 'Live Music'
    const venueName = show.venues?.[0]?.name || 'TBA'
    const showDate = formatShowDate(show.event_date, show.venues?.[0]?.state, show.venues?.[0]?.timezone)
    const title = `${headliner} at ${venueName}`
    const generatedDesc = `${headliner} live at ${venueName} on ${showDate}`
    const description = show.description
      ? show.description.slice(0, 155) + (show.description.length > 155 ? '...' : '')
      : generatedDesc

    return {
      title,
      description,
      alternates: {
        canonical: `https://psychichomily.com/shows/${slug}`,
      },
      openGraph: {
        title,
        description,
        type: 'website',
        url: `/shows/${slug}`,
      },
    }
  }

  return {
    title: 'Show',
    description: 'View show details',
  }
}

function ShowLoadingFallback() {
  return (
    <div className="flex min-h-[60vh] items-center justify-center">
      <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
    </div>
  )
}

export default async function ShowPage({ params }: ShowPageProps) {
  const { slug } = await params

  if (!slug) {
    notFound()
  }

  const showData = await getShow(slug)

  if (!showData) {
    notFound()
  }

  const dehydratedState = await prefetchEntity(
    queryKeys.shows.detail(slug),
    showData,
  )

  const headliner = showData.artists?.find(a => a.is_headliner)?.name || showData.artists?.[0]?.name || 'Live Music'
  const showName = showData.title || `${headliner} at ${showData.venues?.[0]?.name || 'TBA'}`

  return (
    <>
      <JsonLd data={generateMusicEventSchema({
        name: showData.title,
        date: showData.event_date,
        description: showData.description ?? undefined,
        is_cancelled: showData.is_cancelled,
        is_sold_out: showData.is_sold_out,
        venue: showData.venues?.[0] ? {
          name: showData.venues[0].name,
          slug: showData.venues[0].slug,
          address: showData.venues[0].address ?? undefined,
          city: showData.venues[0].city,
          state: showData.venues[0].state,
        } : undefined,
        artists: showData.artists?.map(a => ({
          name: a.name,
          slug: a.slug,
          is_headliner: a.is_headliner ?? undefined,
          // `ShowArtistSocials` is a struct of named optional fields; spread into a
          // plain object so it satisfies the schema helper's index-signature parameter
          // type without changing the cross-feature type.
          socials: { ...a.socials },
        })),
        price: showData.price ?? undefined,
        slug: showData.slug,
      })} />
      <JsonLd data={generateBreadcrumbSchema([
        { name: 'Home', url: 'https://psychichomily.com' },
        { name: 'Shows', url: 'https://psychichomily.com/shows' },
        { name: showName, url: `https://psychichomily.com/shows/${slug}` },
      ])} />
      <HydrationBoundary state={dehydratedState}>
        <Suspense fallback={<ShowLoadingFallback />}>
          <ShowDetail showId={slug} />
        </Suspense>
      </HydrationBoundary>
    </>
  )
}
