import { Suspense } from 'react'
import { FestivalList } from '@/features/festivals/components'
import { LoadingSpinner } from '@/components/shared'

export const metadata = {
  title: 'Festivals',
  description: 'Browse music festivals, lineups, and schedules.',
  alternates: {
    canonical: 'https://psychichomily.com/festivals',
  },
  openGraph: {
    title: 'Festivals | Psychic Homily',
    description: 'Browse music festivals, lineups, and schedules.',
    url: '/festivals',
    type: 'website',
  },
}

export default function FestivalsPage() {
  return (
    <div className="flex min-h-screen items-start justify-center">
      <main className="w-full max-w-6xl px-4 py-8 md:px-8">
        <h1 className="text-3xl font-bold text-center mb-8">Festivals</h1>
        <Suspense
          fallback={
            <div className="flex justify-center items-center py-12">
              <LoadingSpinner />
            </div>
          }
        >
          <FestivalList />
        </Suspense>
      </main>
    </div>
  )
}
