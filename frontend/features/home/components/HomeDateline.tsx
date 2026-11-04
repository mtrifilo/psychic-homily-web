'use client'

import { useState } from 'react'
import { statNumberFormatter } from '@/components/shared/StatsList'
import { useHydrated } from '@/lib/hooks/common/useHydrated'
import { cn } from '@/lib/utils'
import { useCommunityPulse } from '../hooks/useCommunityPulse'
import type { CommunityPulseResponse } from '../types'

export type DatelineForm = 'short' | 'long'

// No `timeZone`: the date is the viewer's own, which is why it renders only
// after hydration.
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

/** Copy per form, singular then plural. Site-wide counts, so no city: a city
 *  beside the show count would read as that city's count. The short form's
 *  "this week" is the owner's phone copy for the rolling seven-day count; the
 *  long form says what the count is. */
const COPY = {
  short: {
    shows: ['show this week', 'shows this week'],
    entities: ['entity', 'entities'],
  },
  long: {
    shows: ['show in the next 7 days', 'shows in the next 7 days'],
    entities: ['entity in the graph', 'entities in the graph'],
  },
} as const

function counted(value: number, [one, many]: readonly [string, string]): string {
  return `${statNumberFormatter.format(value)} ${value === 1 ? one : many}`
}

/** "Wed Nov 4 · 1,204 shows this week · 25,838 entities" (short) or
 *  "Wednesday, November 4 · 1,204 shows in the next 7 days · 25,838 entities
 *  in the graph" (long). Without counts, the date alone. */
export function formatDateline(
  date: Date,
  pulse: CommunityPulseResponse | undefined,
  form: DatelineForm
): string {
  const day =
    form === 'short'
      ? SHORT_DATE.formatToParts(date)
          .filter(part => part.type !== 'literal')
          .map(part => part.value)
          .join(' ')
      : LONG_DATE.format(date)
  if (!pulse) return day
  return [
    day,
    counted(pulse.shows_this_week, COPY[form].shows),
    counted(pulse.entities_in_graph, COPY[form].entities),
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

  return (
    <p
      className={cn(
        'font-data uppercase text-muted-foreground',
        form === 'short' ? 'text-xs leading-4' : 'text-[13px] leading-[18px]',
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
            form === 'short' ? 'w-64' : 'w-[28rem]'
          )}
        />
      )}
    </p>
  )
}
