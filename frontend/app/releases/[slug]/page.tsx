import { Suspense, cache } from 'react'
import { Metadata } from 'next'
import dynamic from 'next/dynamic'
import { notFound } from 'next/navigation'
import * as Sentry from '@sentry/nextjs'
import { Loader2 } from 'lucide-react'
import { HydrationBoundary } from '@tanstack/react-query'
import type { ReleaseDetail as ReleaseDetailData } from '@/features/releases/types'
import {
  creditedReleaseArtists,
  knownReleaseYear,
  releaseListenServiceNames,
  releaseTypeLabel,
} from '@/features/releases/releaseSearchFacts'
import { JsonLd } from '@/components/seo/JsonLd'
import { generateMusicAlbumSchema } from '@/lib/seo/jsonld'
import { releaseSnippet } from '@/lib/seo/entitySnippets'
import { API_BASE_URL } from '@/lib/api-base'
import { reportServerFetchFailure } from '@/lib/ssr/reportServerFetchFailure'
import { serverReadHeaders } from '@/lib/ssr/serverReadHeaders'
import { queryKeys } from '@/lib/queryClient'
import { prefetchEntity } from '@/lib/query-hydration'

// Imported from the component FILE, never the `@/features/releases/components`
// barrel — see the note there for why the barrel would undo this.
// `ssr: true` preserves the prefetchEntity + HydrationBoundary server render;
// the page's own <Suspense> below is the boundary this lazy resolves against.
const ReleaseDetail = dynamic(
  () =>
    import('@/features/releases/components/ReleaseDetail').then(m => ({
      default: m.ReleaseDetail,
    })),
  { ssr: true },
)

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
    const res = await fetch(`${API_BASE_URL}/releases/${encodeURIComponent(slug)}`, {
      headers: serverReadHeaders(),
      next: { revalidate: 3600 },
    })
    if (res.ok) {
      return res.json()
    }
    reportServerFetchFailure({
      response: res,
      service: 'release-page',
      pathFamily: '/releases/{slug}',
      notFoundIsAnswer: true,
      context: { slug },
    })
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
    const { title, description } = releaseSnippet({
      title: release.title,
      artists: creditedReleaseArtists(release.artists).map(artist => artist.name),
      typeLabel: releaseTypeLabel(release.release_type),
      year: knownReleaseYear(release.release_year),
      label: release.labels?.find(label => label.name?.trim())?.name,
      listenServices: releaseListenServiceNames(release.external_links),
    })
    return {
      title,
      description,
      alternates: {
        canonical: `https://psychichomily.com/releases/${slug}`,
      },
      openGraph: {
        title,
        description,
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
    <>
      <JsonLd data={generateMusicAlbumSchema({
        title: releaseData.title,
        slug: releaseData.slug || slug,
        release_year: releaseData.release_year,
        release_type: releaseData.release_type,
        cover_art_url: releaseData.cover_art_url,
        artists: creditedReleaseArtists(releaseData.artists),
      })} />
      <HydrationBoundary state={dehydratedState}>
        <Suspense fallback={<ReleaseLoadingFallback />}>
          <ReleaseDetail idOrSlug={slug} />
        </Suspense>
      </HydrationBoundary>
    </>
  )
}
