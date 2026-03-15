import { Suspense } from 'react'
import { LoadingSpinner } from '@/components/shared'
import { SceneList } from '@/features/scenes'

export const metadata = {
  title: 'Scenes',
  description: 'Explore music scenes by city — venues, artists, shows, and live music activity.',
  alternates: {
    canonical: 'https://psychichomily.com/scenes',
  },
  openGraph: {
    title: 'Scenes | Psychic Homily',
    description: 'Explore music scenes by city — venues, artists, shows, and live music activity.',
    url: '/scenes',
    type: 'website',
  },
}

export default function ScenesPage() {
  return (
    <div className="flex min-h-screen items-start justify-center">
      <main className="w-full max-w-6xl px-4 py-8 md:px-8">
        <h1 className="text-3xl font-bold text-center mb-2">Scenes</h1>
        <p className="text-center text-muted-foreground mb-8 max-w-lg mx-auto">
          City-level music scenes with venue activity, artist trends, and live show data.
        </p>
        <Suspense
          fallback={
            <div className="flex justify-center items-center py-12">
              <LoadingSpinner />
            </div>
          }
        >
          <SceneList />
        </Suspense>
      </main>
    </div>
  )
}
