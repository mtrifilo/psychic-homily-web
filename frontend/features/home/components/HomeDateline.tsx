'use client'

import { useState } from 'react'
import { statNumberFormatter } from '@/components/shared/StatsList'
import { useHydrated } from '@/lib/hooks/common/useHydrated'
import { cn } from '@/lib/utils'
import { useCommunityPulse } from '../hooks/useCommunityPulse'
import type { CommunityPulseResponse } from '../types'

/**
 * Each form's date, copy (singular then plural), type size and placeholder
 * width. No `timeZone` on the dates: the date is the viewer's own, which is
 * why the line renders only after hydration. The counts are site-wide, so no
 * city: a city beside the show count would read as that city's count. The
 * show count is a rolling seven-day window (see
 * `CommunityPulseResponse.shows_this_week`), so both forms say "in the next
 * 7 days", never "this week".
 */
const SHORT_DATE = new Intl.DateTimeFormat('en-US', {
  weekday: 'short',
  month: 'short',
  day: 'numeric',
})
const LONG_DATE = new Intl.DateTimeFormat('en-US', {
  weekday: 'long',
  month: 'long',
  day: 'numeric',
})

const FORMS = {
  short: {
    // "Wed, Nov 4" without the comma.
    date: (date: Date) => SHORT_DATE.format(date).replace(',', ''),
    shows: ['show in the next 7 days', 'shows in the next 7 days'],
    entities: ['entity', 'entities'],
    className: 'text-xs leading-4',
    placeholderClassName: 'w-64',
  },
  long: {
    date: (date: Date) => LONG_DATE.format(date),
    shows: ['show in the next 7 days', 'shows in the next 7 days'],
    entities: ['entity in the graph', 'entities in the graph'],
    className: 'text-[13px] leading-[18px]',
    placeholderClassName: 'w-[28rem]',
  },
} as const

export type DatelineForm = keyof typeof FORMS

function counted(value: number, [one, many]: readonly [string, string]): string {
  return `${statNumberFormatter.format(value)} ${value === 1 ? one : many}`
}

/** "Wed Nov 4 · 1,204 shows in the next 7 days · 25,838 entities" (short) or
 *  "Wednesday, November 4 · 1,204 shows in the next 7 days · 25,838 entities
 *  in the graph" (long). Without counts, the date alone. */
export function formatDateline(
  date: Date,
  pulse: CommunityPulseResponse | undefined,
  form: DatelineForm
): string {
  const spec = FORMS[form]
  const day = spec.date(date)
  if (!pulse) return day
  return [
    day,
    counted(pulse.shows_this_week, spec.shows),
    counted(pulse.entities_in_graph, spec.entities),
  ].join(' · ')
}

/**
 * The signed-in home's dateline: today's date and the site-wide pulse counts,
 * in the data face, uppercase. Reads the pulse query the stats band used, so
 * it costs no new request. Until the browser has both the date and an answer
 * from the pulse, a placeholder holds the line's height; a failed pulse read
 * leaves the date alone.
 */
export function HomeDateline({
  form,
  className,
}: {
  form: DatelineForm
  className?: string
}) {
  const hydrated = useHydrated()
  // Read once, at mount: the line names the day the viewer arrived.
  const [today] = useState(() => new Date())
  const { data, isError } = useCommunityPulse()
  const ready = hydrated && (data !== undefined || isError)
  const spec = FORMS[form]

  return (
    <p
      className={cn(
        'font-data uppercase text-muted-foreground',
        spec.className,
        className
      )}
    >
      {ready ? (
        formatDateline(today, data, form)
      ) : (
        <span
          aria-hidden
          className={cn(
            'inline-block h-3 animate-pulse rounded-sm bg-muted align-middle',
            spec.placeholderClassName
          )}
        />
      )}
    </p>
  )
}
