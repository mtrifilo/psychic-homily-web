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
import { listRootCanonical } from '@/lib/seo/siteMetadata'

/**
 * Closed-window immutability is enforced by the charts API's 24h TTL
 * (PSY-1421). Route-segment `revalidate` is incompatible with
 * `cacheComponents` — do not reintroduce it here.
 *
 * The canonical is this quarter's own path, which is the root of this archive,
 * per the site-wide pagination indexing policy on `listRootCanonical`. It was
 * already in that posture before PSY-1767; routing it through the helper is
 * what ties it to the written policy instead of leaving it as a literal that
 * happens to agree.
 */

export async function generateMetadata({
  params,
}: {
  params: Promise<{ module: string; period: string }>
}): Promise<Metadata> {
  const { module, period } = await params
  const window = calendarWindowFromRoute(module, period)
  if (!window) return { title: 'Charts' }
  return {
    title: formatArchiveTitle(window),
    description: formatArchiveSubtitle(window),
    alternates: {
      canonical: listRootCanonical(`/charts/${module}/${period}`),
    },
    openGraph: {
      title: `${formatArchiveTitle(window)} | Psychic Homily`,
      description: formatArchiveSubtitle(window),
      url: `/charts/${module}/${period}`,
      type: 'website',
    },
  }
}

export default async function ChartQuarterArchiveRoute({
  params,
}: {
  params: Promise<{ module: string; period: string }>
}) {
  const { module, period } = await params
  const window = calendarWindowFromRoute(module, period)
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
