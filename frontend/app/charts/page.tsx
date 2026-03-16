import { Suspense } from 'react'
import { LoadingSpinner } from '@/components/shared'
import { ChartsPage } from '@/features/charts'

export const metadata = {
  title: 'Charts',
  description: 'Top charts — trending shows, popular artists, active venues, and hot releases.',
  alternates: {
    canonical: 'https://psychichomily.com/charts',
  },
  openGraph: {
    title: 'Charts | Psychic Homily',
    description: 'Top charts — trending shows, popular artists, active venues, and hot releases.',
    url: '/charts',
    type: 'website',
  },
}

export default function ChartsRoute() {
  return (
    <div className="flex min-h-screen items-start justify-center">
      <main className="w-full max-w-6xl px-4 py-8 md:px-8">
        <h1 className="text-3xl font-bold text-center mb-2">Charts</h1>
        <p className="text-center text-muted-foreground mb-8 max-w-lg mx-auto">
          Trending shows, popular artists, active venues, and hot releases.
        </p>
        <Suspense
          fallback={
            <div className="flex justify-center items-center py-12">
              <LoadingSpinner />
            </div>
          }
        >
          <ChartsPage />
        </Suspense>
      </main>
    </div>
  )
}
