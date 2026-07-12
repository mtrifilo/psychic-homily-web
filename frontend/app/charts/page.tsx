import { Suspense } from 'react'
import { LoadingSpinner } from '@/components/shared'
import { ChartsPage } from '@/features/charts'

export const metadata = {
  title: 'Charts',
  description:
    'The ledger of what’s moving — artists, shows, venues, releases, and airwaves.',
  alternates: {
    canonical: 'https://psychichomily.com/charts',
  },
  openGraph: {
    title: 'Charts | Psychic Homily',
    description:
      'The ledger of what’s moving — artists, shows, venues, releases, and airwaves.',
    url: '/charts',
    type: 'website',
  },
}

export default function ChartsRoute() {
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
          <ChartsPage />
        </Suspense>
      </main>
    </div>
  )
}
