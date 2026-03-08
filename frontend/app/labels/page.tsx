import { Suspense } from 'react'
import { LabelList } from '@/components/labels'
import { LoadingSpinner } from '@/components/shared'

export const metadata = {
  title: 'Labels',
  description: 'Browse record labels and their rosters and catalogs.',
  alternates: {
    canonical: 'https://psychichomily.com/labels',
  },
  openGraph: {
    title: 'Labels | Psychic Homily',
    description: 'Browse record labels and their rosters and catalogs.',
    url: '/labels',
    type: 'website',
  },
}

export default function LabelsPage() {
  return (
    <div className="flex min-h-screen items-start justify-center">
      <main className="w-full max-w-6xl px-4 py-8 md:px-8">
        <h1 className="text-3xl font-bold text-center mb-8">Labels</h1>
        <Suspense
          fallback={
            <div className="flex justify-center items-center py-12">
              <LoadingSpinner />
            </div>
          }
        >
          <LabelList />
        </Suspense>
      </main>
    </div>
  )
}
