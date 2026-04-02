import { Suspense } from 'react'
import { LoadingSpinner } from '@/components/shared'
import { LeaderboardPage } from '@/features/community'

export const metadata = {
  title: 'Leaderboard',
  description: 'Top contributors to the Psychic Homily music knowledge graph.',
  alternates: {
    canonical: 'https://psychichomily.com/community/leaderboard',
  },
  openGraph: {
    title: 'Leaderboard | Psychic Homily',
    description: 'Top contributors to the Psychic Homily music knowledge graph.',
    url: '/community/leaderboard',
    type: 'website',
  },
}

export default function LeaderboardRoute() {
  return (
    <div className="flex min-h-screen items-start justify-center">
      <main className="w-full max-w-3xl px-4 py-8 md:px-8">
        <h1 className="text-3xl font-bold text-center mb-2">Leaderboard</h1>
        <p className="text-center text-muted-foreground mb-8 max-w-lg mx-auto">
          Top contributors building the music knowledge graph.
        </p>
        <Suspense
          fallback={
            <div className="flex justify-center items-center py-12">
              <LoadingSpinner />
            </div>
          }
        >
          <LeaderboardPage />
        </Suspense>
      </main>
    </div>
  )
}
