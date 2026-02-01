import { Suspense } from 'react'
import { ShowList } from '@/components/shows'
import { LoadingSpinner } from '@/components/shared'

export const metadata = {
  title: 'Upcoming Shows',
  description: 'Discover upcoming live music shows in Phoenix and beyond.',
  openGraph: {
    title: 'Upcoming Shows | Psychic Homily',
    description: 'Discover upcoming live music shows in Phoenix and beyond.',
    url: '/shows',
    type: 'website',
  },
}

function ShowListFallback() {
  return (
    <div className="flex justify-center items-center py-12">
      <LoadingSpinner />
    </div>
  )
}

export default function ShowsPage() {
  return (
    <div className="flex min-h-screen items-start justify-center bg-background">
      <main className="w-full max-w-4xl px-4 py-8 md:px-8">
        <h1 className="text-3xl font-bold text-center mb-8">Upcoming Shows</h1>
        <Suspense fallback={<ShowListFallback />}>
          <ShowList />
        </Suspense>
      </main>
    </div>
  )
}
