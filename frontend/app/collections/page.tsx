import { Suspense } from 'react'
import { CollectionList } from '@/features/collections/components'
import { LoadingSpinner } from '@/components/shared'

export const metadata = {
  title: 'Collections',
  description: 'Browse curated collections of artists, releases, venues, and more.',
  alternates: {
    canonical: 'https://psychichomily.com/collections',
  },
  openGraph: {
    title: 'Collections | Psychic Homily',
    description: 'Browse curated collections of artists, releases, venues, and more.',
    url: '/collections',
    type: 'website',
  },
}

export default function CollectionsPage() {
  return (
    <div className="flex min-h-screen items-start justify-center">
      <main className="w-full max-w-6xl px-4 py-8 md:px-8">
        <h1 className="text-3xl font-bold text-center mb-8">Collections</h1>
        <Suspense
          fallback={
            <div className="flex justify-center items-center py-12">
              <LoadingSpinner />
            </div>
          }
        >
          <CollectionList />
        </Suspense>
      </main>
    </div>
  )
}
