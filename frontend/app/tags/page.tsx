import { Suspense } from 'react'
import { TagBrowse } from '@/features/tags/components'
import { LoadingSpinner } from '@/components/shared'

export const metadata = {
  title: 'Tags',
  description: 'Browse tags by category — genres, moods, eras, styles, and more.',
  alternates: {
    canonical: 'https://psychichomily.com/tags',
  },
  openGraph: {
    title: 'Tags | Psychic Homily',
    description: 'Browse tags by category — genres, moods, eras, styles, and more.',
    url: '/tags',
    type: 'website',
  },
}

export default function TagsPage() {
  return (
    <div className="flex min-h-screen items-start justify-center">
      <main className="w-full max-w-6xl px-4 py-8 md:px-8">
        <h1 className="text-3xl font-bold text-center mb-8">Tags</h1>
        <Suspense
          fallback={
            <div className="flex justify-center items-center py-12">
              <LoadingSpinner />
            </div>
          }
        >
          <TagBrowse />
        </Suspense>
      </main>
    </div>
  )
}
