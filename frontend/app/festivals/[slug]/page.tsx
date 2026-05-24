import { Suspense, cache } from 'react'
import { Metadata } from 'next'
import { notFound } from 'next/navigation'
import * as Sentry from '@sentry/nextjs'
import { Loader2 } from 'lucide-react'
import { HydrationBoundary } from '@tanstack/react-query'
import { FestivalDetail } from '@/features/festivals/components'
import type { FestivalDetail as FestivalDetailData } from '@/features/festivals/types'
import { API_BASE_URL } from '@/lib/api-base'
import { queryKeys } from '@/lib/queryClient'
import { prefetchEntity } from '@/lib/query-hydration'

interface FestivalPageProps {
  params: Promise<{ slug: string }>
}

/**
 * Wrapped with `React.cache()` so `generateMetadata` and the page body
 * share ONE backend fetch per request instead of two. The result also
 * seeds the TanStack Query cache via `prefetchEntity` below, eliminating
 * the client-side refetch on first paint.
 */
const getFestival = cache(async (slug: string): Promise<FestivalDetailData | null> => {
  try {
    const res = await fetch(`${API_BASE_URL}/festivals/${slug}`, {
      next: { revalidate: 3600 },
    })
    if (res.ok) {
      return res.json()
    }
    if (res.status >= 500) {
      Sentry.captureMessage(`Festival page fetch error: ${res.status}`, {
        level: 'error',
        tags: { service: 'festival-page' },
        extra: { slug, status: res.status },
      })
    }
  } catch (error) {
    Sentry.captureException(error, {
      level: 'error',
      tags: { service: 'festival-page' },
      extra: { slug },
    })
  }
  return null
})

export async function generateMetadata({
  params,
}: FestivalPageProps): Promise<Metadata> {
  const { slug } = await params
  const festival = await getFestival(slug)

  if (festival) {
    const locationSuffix =
      festival.city && festival.state ? ` - ${festival.city}, ${festival.state}` : ''
    return {
      title: `${festival.name}${locationSuffix}`,
      description: `${festival.name}${locationSuffix} - festival details on Psychic Homily`,
      alternates: {
        canonical: `https://psychichomily.com/festivals/${slug}`,
      },
      openGraph: {
        title: `${festival.name}${locationSuffix}`,
        description: `View details for ${festival.name}`,
        type: 'website',
        url: `/festivals/${slug}`,
      },
    }
  }

  return {
    title: 'Festival',
    description: 'View festival details',
  }
}

function FestivalLoadingFallback() {
  return (
    <div className="flex min-h-[60vh] items-center justify-center">
      <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
    </div>
  )
}

export default async function FestivalPage({ params }: FestivalPageProps) {
  const { slug } = await params

  if (!slug) {
    notFound()
  }

  const festivalData = await getFestival(slug)

  if (!festivalData) {
    notFound()
  }

  const dehydratedState = await prefetchEntity(
    queryKeys.festivals.detail(slug),
    festivalData,
  )

  return (
    <HydrationBoundary state={dehydratedState}>
      <Suspense fallback={<FestivalLoadingFallback />}>
        <FestivalDetail idOrSlug={slug} />
      </Suspense>
    </HydrationBoundary>
  )
}
