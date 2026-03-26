import { Suspense } from 'react'
import { CrateList } from '@/features/crates/components'
import { LoadingSpinner } from '@/components/shared'

export const metadata = {
  title: 'Crates',
  description: 'Browse curated crates of artists, releases, venues, and more.',
  alternates: {
    canonical: 'https://psychichomily.com/crates',
  },
  openGraph: {
    title: 'Crates | Psychic Homily',
    description: 'Browse curated crates of artists, releases, venues, and more.',
    url: '/crates',
    type: 'website',
  },
}

export default function CratesPage() {
  return (
    <div className="flex min-h-screen items-start justify-center">
      <main className="w-full max-w-6xl px-4 py-8 md:px-8">
        <h1 className="text-3xl font-bold text-center mb-8">Crates</h1>
        <Suspense
          fallback={
            <div className="flex justify-center items-center py-12">
              <LoadingSpinner />
            </div>
          }
        >
          <CrateList />
        </Suspense>
      </main>
    </div>
  )
}
