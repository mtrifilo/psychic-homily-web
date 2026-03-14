import { Suspense } from 'react'
import { RequestList } from '@/features/requests/components'
import { LoadingSpinner } from '@/components/shared'

export const metadata = {
  title: 'Requests',
  description: 'Browse and create requests for artists, releases, labels, and more.',
  alternates: {
    canonical: 'https://psychichomily.com/requests',
  },
  openGraph: {
    title: 'Requests | Psychic Homily',
    description: 'Browse and create requests for artists, releases, labels, and more.',
    url: '/requests',
    type: 'website',
  },
}

export default function RequestsPage() {
  return (
    <div className="flex min-h-screen items-start justify-center">
      <main className="w-full max-w-6xl px-4 py-8 md:px-8">
        <h1 className="text-3xl font-bold text-center mb-8">Requests</h1>
        <Suspense
          fallback={
            <div className="flex justify-center items-center py-12">
              <LoadingSpinner />
            </div>
          }
        >
          <RequestList />
        </Suspense>
      </main>
    </div>
  )
}
