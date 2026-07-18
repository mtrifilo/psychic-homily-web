import { Suspense } from 'react'
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

/**
 * Closed calendar archives are immutable — cache the route shell for a day.
 * Open (in-progress) periods still use this; client fetches stay fresh via
 * react-query against the API's shorter open-window TTLs.
 */
export const revalidate = 86400

export async function generateMetadata({
  params,
}: {
  params: Promise<{ module: string }>
}) {
  const { module } = await params
  if (isChartModuleSlug(module)) {
    return { title: 'Charts' }
  }
  const window = calendarWindowFromRoute(module)
  if (!window) return { title: 'Charts' }
  return {
    title: formatArchiveTitle(window),
    description: formatArchiveSubtitle(window),
    alternates: {
      canonical: `https://psychichomily.com/charts/${module}`,
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
