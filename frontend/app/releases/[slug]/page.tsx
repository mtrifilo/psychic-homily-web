import { Suspense, cache } from 'react'
import { Metadata } from 'next'
import { notFound } from 'next/navigation'
import * as Sentry from '@sentry/nextjs'
import { Loader2 } from 'lucide-react'
import { HydrationBoundary } from '@tanstack/react-query'
import { ReleaseDetail } from '@/features/releases/components'
import type { ReleaseDetail as ReleaseDetailData } from '@/features/releases/types'
import { API_BASE_URL } from '@/lib/api-base'
import { queryKeys } from '@/lib/queryClient'
import { prefetchEntity } from '@/lib/query-hydration'

interface ReleasePageProps {
  params: Promise<{ slug: string }>
}

/**
 * Wrapped with `React.cache()` so `generateMetadata` and the page body
 * share ONE backend fetch per request instead of two. The result also
 * seeds the TanStack Query cache via `prefetchEntity` below, eliminating
 * the client-side refetch on first paint.
 */
const getRelease = cache(async (slug: string): Promise<ReleaseDetailData | null> => {
  try {
    const res = await fetch(`${API_BASE_URL}/releases/${slug}`, {
      next: { revalidate: 3600 },
    })
    if (res.ok) {
      return res.json()
    }
    if (res.status >= 500) {
      Sentry.captureMessage(`Release page fetch error: ${res.status}`, {
        level: 'error',
        tags: { service: 'release-page' },
        extra: { slug, status: res.status },
      })
    }
  } catch (error) {
    Sentry.captureException(error, {
      level: 'error',
      tags: { service: 'release-page' },
      extra: { slug },
    })
  }
  return null
})

export async function generateMetadata({
  params,
}: ReleasePageProps): Promise<Metadata> {
  const { slug } = await params
  const release = await getRelease(slug)

  if (release) {
    const yearSuffix = release.release_year ? ` (${release.release_year})` : ''
    return {
      title: `${release.title}${yearSuffix}`,
      description: `${release.title}${yearSuffix} - release details on Psychic Homily`,
      alternates: {
        canonical: `https://psychichomily.com/releases/${slug}`,
      },
      openGraph: {
        title: `${release.title}${yearSuffix}`,
        description: `View details for ${release.title}`,
        type: 'website',
        url: `/releases/${slug}`,
      },
    }
  }

  return {
    title: 'Release',
    description: 'View release details',
  }
}

function ReleaseLoadingFallback() {
  return (
    <div className="flex min-h-[60vh] items-center justify-center">
      <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
    </div>
  )
}

export default async function ReleasePage({ params }: ReleasePageProps) {
  const { slug } = await params

  if (!slug) {
    notFound()
  }

  const releaseData = await getRelease(slug)

  if (!releaseData) {
    notFound()
  }

  const dehydratedState = await prefetchEntity(
    queryKeys.releases.detail(slug),
    releaseData,
  )

  return (
    <HydrationBoundary state={dehydratedState}>
      <Suspense fallback={<ReleaseLoadingFallback />}>
        <ReleaseDetail idOrSlug={slug} />
      </Suspense>
    </HydrationBoundary>
  )
}
