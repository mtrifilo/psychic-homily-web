import { Suspense } from 'react'
import { LoadingSpinner } from '@/components/shared'
import { FeaturedArchivePage } from '@/features/charts'

/**
 * `/charts/featured` (PSY-1501) — the previously-featured collections archive.
 *
 * This is a STATIC route segment, so Next resolves it ahead of the sibling
 * `[module]` dynamic segment (chart drill-downs) and the PSY-1422 numeric-year
 * archive segments. `app/charts/featured/page.test.tsx` pins that contract: the
 * dynamic segment 404s "featured", proving the archive must be — and is — served
 * here. See PSY-1493 board B for the locked layout + copy.
 */
export const metadata = {
  title: 'Previously Featured',
  description: 'Every collection that has held the featured slot.',
  alternates: {
    canonical: 'https://psychichomily.com/charts/featured',
  },
  openGraph: {
    title: 'Previously Featured | Psychic Homily',
    description: 'Every collection that has held the featured slot.',
    url: '/charts/featured',
    type: 'website',
  },
}

export default function ChartsFeaturedRoute() {
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
          <FeaturedArchivePage />
        </Suspense>
      </main>
    </div>
  )
}
