import { Suspense } from 'react'
import { notFound } from 'next/navigation'
import { LoadingSpinner } from '@/components/shared'
import { ChartsPage } from '@/features/charts'
import {
  calendarWindowFromRoute,
  formatArchiveSubtitle,
  formatArchiveTitle,
} from '@/features/charts/calendarWindows'

/**
 * Closed-window immutability is enforced by the charts API's 24h TTL
 * (PSY-1421). Route-segment `revalidate` is incompatible with
 * `cacheComponents` — do not reintroduce it here.
 */

export async function generateMetadata({
  params,
}: {
  params: Promise<{ module: string; period: string }>
}) {
  const { module, period } = await params
  const window = calendarWindowFromRoute(module, period)
  if (!window) return { title: 'Charts' }
  return {
    title: formatArchiveTitle(window),
    description: formatArchiveSubtitle(window),
    alternates: {
      canonical: `https://psychichomily.com/charts/${module}/${period}`,
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
