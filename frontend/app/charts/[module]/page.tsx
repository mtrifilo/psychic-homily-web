import { Suspense } from 'react'
import { notFound } from 'next/navigation'
import { LoadingSpinner } from '@/components/shared'
import { ChartDrilldownPage } from '@/features/charts/components/ChartDrilldownPage'
import { isChartModuleSlug } from '@/features/charts/moduleConfig'

export default async function ChartModuleRoute({
  params,
}: {
  params: Promise<{ module: string }>
}) {
  const { module } = await params
  if (!isChartModuleSlug(module)) notFound()

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
