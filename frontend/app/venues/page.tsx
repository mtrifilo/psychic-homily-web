import { Suspense } from 'react'
import { VenueList } from '@/components/VenueList'

export const metadata = {
  title: 'Venues',
  description: 'Browse music venues and discover upcoming shows.',
  openGraph: {
    title: 'Venues | Psychic Homily',
    description: 'Browse music venues and discover upcoming shows.',
    url: '/venues',
    type: 'website',
  },
}

function VenueListLoading() {
  return (
    <div className="flex justify-center items-center py-12">
      <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-foreground"></div>
    </div>
  )
}

export default function VenuesPage() {
  return (
    <div className="flex min-h-screen items-start justify-center bg-background">
      <main className="w-full max-w-4xl px-4 py-8 md:px-8">
        <h1 className="text-3xl font-bold text-center mb-8">Venues</h1>
        <Suspense fallback={<VenueListLoading />}>
          <VenueList />
        </Suspense>
      </main>
    </div>
  )
}
