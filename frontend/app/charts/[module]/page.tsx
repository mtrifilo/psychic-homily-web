import { Suspense } from 'react'
import type { Metadata } from 'next'
import { notFound } from 'next/navigation'
import { LoadingSpinner } from '@/components/shared'
import { ChartsPage } from '@/features/charts'
import {
  calendarWindowFromRoute,
  formatArchiveSubtitle,
  formatArchiveTitle,
} from '@/features/charts/calendarWindows'
import { isChartModuleSlug } from '@/features/charts/moduleConfig'
import { ChartDrilldownPage } from '@/features/charts/components/ChartDrilldownPage'
import { listRootCanonical } from '@/lib/seo/siteMetadata'

/**
 * Aggressive closed-window caching lives on the charts API (24h TTL for
 * ended calendar periods — PSY-1421). Route-segment `revalidate` is
 * incompatible with `cacheComponents` (Next 16); the Broadsheet client
 * still pins via `pinnedWindow` and hits that API cache.
 */

/**
 * Both branches canonicalize to their own path, which is the list root, per the
 * site-wide pagination indexing policy documented on `listRootCanonical`.
 *
 * The drilldown branch had NO canonical at all before PSY-1767, so its
 * `?page=`, `?window=` and `?scene=` variants were each offered to crawlers as
 * a document in their own right. That is the gap this closes. The drilldown is
 * one chart; the query string picks which slice of it is on screen.
 *
 * `module` reaches the canonical only after `isChartModuleSlug` or
 * `calendarWindowFromRoute` has vouched for it, so no unvalidated path segment
 * can be reflected into a canonical URL. That ordering is load bearing: the
 * unknown-slug case returns before either branch.
 */
export async function generateMetadata({
  params,
}: {
  params: Promise<{ module: string }>
}): Promise<Metadata> {
  const { module } = await params
  if (isChartModuleSlug(module)) {
    return {
      title: 'Charts',
      alternates: {
        canonical: listRootCanonical(`/charts/${module}`),
      },
    }
  }
  const window = calendarWindowFromRoute(module)
  if (!window) return { title: 'Charts' }
  return {
    title: formatArchiveTitle(window),
    description: formatArchiveSubtitle(window),
    alternates: {
      canonical: listRootCanonical(`/charts/${module}`),
    },
    openGraph: {
      title: `${formatArchiveTitle(window)} | Psychic Homily`,
      description: formatArchiveSubtitle(window),
      url: `/charts/${module}`,
      type: 'website',
    },
  }
}

export default async function ChartModuleOrArchiveRoute({
  params,
}: {
  params: Promise<{ module: string }>
}) {
  const { module } = await params

  if (isChartModuleSlug(module)) {
    return (
      <div className="flex min-h-screen items-start justify-center">
        <main className="w-full max-w-6xl px-4 py-8 md:px-8">
          <Suspense
            fallback={
              <div className="flex items-center justify-center py-12">
                <LoadingSpinner />
              </div>
            }
          >
            <ChartDrilldownPage module={module} />
          </Suspense>
        </main>
      </div>
    )
  }

  const window = calendarWindowFromRoute(module)
  if (!window) notFound()

  return (
    <div className="flex min-h-screen items-start justify-center">
      <main className="w-full max-w-6xl px-4 py-8 md:px-8">
        <Suspense
          fallback={
            <div className="flex justify-center items-center py-12">
              <LoadingSpinner />
            </div>
          }
        >
          <ChartsPage pinnedWindow={window} />
        </Suspense>
      </main>
    </div>
  )
}
