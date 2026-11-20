import type { Metadata } from 'next'
import { CalendarStepsPage } from '@/features/collections/components/saved-shows-calendar/CalendarStepsPage'
import { STEPS_PAGE_COPY } from '@/features/collections/components/saved-shows-calendar/calendarStepsPageCopy'

// The page "Email me the steps" opens. Private to the signed-in reader and
// acting on their private calendar link, so it is kept out of search here,
// whatever robots.txt says about the path.
export const metadata: Metadata = {
  title: STEPS_PAGE_COPY.heading,
  robots: { index: false, follow: false },
}

export default function LibraryCalendarPage() {
  return <CalendarStepsPage />
}
