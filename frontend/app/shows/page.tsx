import { Suspense } from 'react'
import { ShowList, ShowListSkeleton } from '@/components/shows'

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

export default function ShowsPage() {
  return (
    <div className="w-full max-w-4xl mx-auto px-4 py-8 md:px-8">
        <h1 className="text-3xl font-bold text-center mb-8 leading-9">Upcoming Shows</h1>
        <Suspense fallback={<ShowListSkeleton />}>
          <ShowList />
        </Suspense>
    </div>
  )
}
