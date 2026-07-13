import { Suspense } from 'react'
import type { Metadata } from 'next'
import {
  ShowSubmissionsConsole,
  ShowSubmissionsLoading,
} from '@/features/shows'

export const metadata: Metadata = {
  title: 'Show Submissions',
  description: 'Track and manage the shows you have submitted.',
  robots: { index: false, follow: false },
}

export default function ShowSubmissionsPage() {
  return (
    <Suspense fallback={<ShowSubmissionsLoading />}>
      <ShowSubmissionsConsole />
    </Suspense>
  )
}
