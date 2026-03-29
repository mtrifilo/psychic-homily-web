import { Suspense } from 'react'
import { ContributeDashboard } from '@/features/contributions/components/ContributeDashboard'
import { LoadingSpinner } from '@/components/shared'

export const metadata = {
  title: 'Contribute',
  description:
    'Help build the knowledge graph by filling in missing data across artists, venues, and shows.',
  alternates: {
    canonical: 'https://psychichomily.com/contribute',
  },
  openGraph: {
    title: 'Contribute | Psychic Homily',
    description:
      'Help build the knowledge graph by filling in missing data across artists, venues, and shows.',
    url: '/contribute',
    type: 'website',
  },
}

export default function ContributePage() {
  return (
    <div className="flex min-h-screen items-start justify-center">
      <main className="w-full max-w-6xl px-4 py-8 md:px-8">
        <div className="text-center mb-8">
          <h1 className="text-3xl font-bold">Contribute</h1>
          <p className="mt-2 text-muted-foreground">
            Help build the knowledge graph
          </p>
        </div>
        <Suspense
          fallback={
            <div className="flex justify-center items-center py-12">
              <LoadingSpinner />
            </div>
          }
        >
          <ContributeDashboard />
        </Suspense>
      </main>
    </div>
  )
}
