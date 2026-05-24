import { Suspense, cache } from 'react'
import { Metadata } from 'next'
import { notFound } from 'next/navigation'
import * as Sentry from '@sentry/nextjs'
import { Loader2 } from 'lucide-react'
import { HydrationBoundary } from '@tanstack/react-query'
import { LabelDetail } from '@/features/labels/components'
import type { LabelDetail as LabelDetailData } from '@/features/labels/types'
import { API_BASE_URL } from '@/lib/api-base'
import { queryKeys } from '@/lib/queryClient'
import { prefetchEntity } from '@/lib/query-hydration'

interface LabelPageProps {
  params: Promise<{ slug: string }>
}

/**
 * Wrapped with `React.cache()` so `generateMetadata` and the page body
 * share ONE backend fetch per request instead of two. The result also
 * seeds the TanStack Query cache via `prefetchEntity` below, eliminating
 * the client-side refetch on first paint.
 */
const getLabel = cache(async (slug: string): Promise<LabelDetailData | null> => {
  try {
    const res = await fetch(`${API_BASE_URL}/labels/${slug}`, {
      next: { revalidate: 3600 },
    })
    if (res.ok) {
      return res.json()
    }
    if (res.status >= 500) {
      Sentry.captureMessage(`Label page fetch error: ${res.status}`, {
        level: 'error',
        tags: { service: 'label-page' },
        extra: { slug, status: res.status },
      })
    }
  } catch (error) {
    Sentry.captureException(error, {
      level: 'error',
      tags: { service: 'label-page' },
      extra: { slug },
    })
  }
  return null
})

export async function generateMetadata({
  params,
}: LabelPageProps): Promise<Metadata> {
  const { slug } = await params
  const label = await getLabel(slug)

  if (label) {
    const locationSuffix =
      label.city && label.state ? ` - ${label.city}, ${label.state}` : ''
    return {
      title: `${label.name}${locationSuffix}`,
      description: `${label.name}${locationSuffix} - label details on Psychic Homily`,
      alternates: {
        canonical: `https://psychichomily.com/labels/${slug}`,
      },
      openGraph: {
        title: `${label.name}${locationSuffix}`,
        description: `View details for ${label.name}`,
        type: 'website',
        url: `/labels/${slug}`,
      },
    }
  }

  return {
    title: 'Label',
    description: 'View label details',
  }
}

function LabelLoadingFallback() {
  return (
    <div className="flex min-h-[60vh] items-center justify-center">
      <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
    </div>
  )
}

export default async function LabelPage({ params }: LabelPageProps) {
  const { slug } = await params

  if (!slug) {
    notFound()
  }

  const labelData = await getLabel(slug)

  if (!labelData) {
    notFound()
  }

  const dehydratedState = await prefetchEntity(
    queryKeys.labels.detail(slug),
    labelData,
  )

  return (
    <HydrationBoundary state={dehydratedState}>
      <Suspense fallback={<LabelLoadingFallback />}>
        <LabelDetail idOrSlug={slug} />
      </Suspense>
    </HydrationBoundary>
  )
}
