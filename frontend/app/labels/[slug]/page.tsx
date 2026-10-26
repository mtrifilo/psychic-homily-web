import { Suspense, cache } from 'react'
import { Metadata } from 'next'
import { notFound } from 'next/navigation'
import * as Sentry from '@sentry/nextjs'
import { Loader2 } from 'lucide-react'
import { HydrationBoundary } from '@tanstack/react-query'
import { LabelDetail } from '@/features/labels/components'
import { labelEndpoints } from '@/features/labels/api'
import type {
  LabelArtistsResponse,
  LabelDetail as LabelDetailData,
  LabelReleasesResponse,
} from '@/features/labels/types'
import { API_BASE_URL } from '@/lib/api-base'
import { fetchListPayload } from '@/lib/ssr/fetchListPayload'
import { reportServerFetchFailure } from '@/lib/ssr/reportServerFetchFailure'
import { serverReadHeaders } from '@/lib/ssr/serverReadHeaders'
import { queryKeys } from '@/lib/queryClient'
import { prefetchEntity, seedFirstScreen, type QuerySeed } from '@/lib/query-hydration'
import { unpadNumericId } from '@/lib/utils/unpadNumericId'

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
    const res = await fetch(`${API_BASE_URL}/labels/${encodeURIComponent(slug)}`, {
      headers: serverReadHeaders(),
      next: { revalidate: 3600 },
    })
    if (res.ok) {
      return res.json()
    }
    reportServerFetchFailure({
      response: res,
      service: 'label-page',
      pathFamily: '/labels/{slug}',
      notFoundIsAnswer: true,
      context: { slug },
    })
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

  // Every read takes the slug from `params`, so none waits on another. The
  // lists are thrown away for a label that turns out not to exist, which the
  // proxy's existence check already filters before this route renders.
  //
  // The list reads ask the endpoints `useLabelRoster` and `useLabelCatalog`
  // read: each one unpaginated request with no query parameters. The slug is
  // encoded because route params arrive decoded, and a bare `/`, `?` or `#`
  // would re-point a server-side fetch at another backend path; a padded
  // numeric id is unpadded so every padding shares one Data Cache entry. Null
  // on a failed read; a 404 says the label no longer exists, which is an
  // answer rather than a defect.
  const listSlug = encodeURIComponent(unpadNumericId(slug))
  const [labelData, roster, catalog] = await Promise.all([
    getLabel(slug),
    fetchListPayload<LabelArtistsResponse>({
      url: labelEndpoints.ARTISTS(listSlug),
      collection: 'artists',
      service: 'label-page-roster',
      pathFamily: '/labels/{slug}/artists',
      notFoundIsAnswer: true,
    }),
    fetchListPayload<LabelReleasesResponse>({
      url: labelEndpoints.RELEASES(listSlug),
      collection: 'releases',
      service: 'label-page-catalog',
      pathFamily: '/labels/{slug}/releases',
      notFoundIsAnswer: true,
    }),
  ])

  if (!labelData) {
    notFound()
  }

  const dehydratedState = await prefetchEntity(
    queryKeys.labels.detail(slug),
    labelData,
  )

  // Keyed on the route slug, which is what `LabelDetail` hands both hooks.
  // Stamped stale (`seedFirstScreen`), unlike the detail above: the rows paint
  // from the server and the browser then re-asks the live endpoints on mount,
  // so a Data Cache entry older than a roster or catalog change is corrected
  // after paint, and a stale stamp never overwrites a fresher entry the client
  // already holds. A failed read seeds nothing, which leaves that section to
  // its own client fetch behind its loader.
  const listSeeds: QuerySeed[] = []
  if (roster) listSeeds.push({ queryKey: queryKeys.labels.roster(slug), data: roster })
  if (catalog) listSeeds.push({ queryKey: queryKeys.labels.catalog(slug), data: catalog })
  const listsState = await seedFirstScreen(listSeeds)

  return (
    <HydrationBoundary state={dehydratedState}>
      <HydrationBoundary state={listsState}>
        <Suspense fallback={<LabelLoadingFallback />}>
          <LabelDetail idOrSlug={slug} />
        </Suspense>
      </HydrationBoundary>
    </HydrationBoundary>
  )
}
