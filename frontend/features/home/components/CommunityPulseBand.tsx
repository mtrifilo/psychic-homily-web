'use client'

/**
 * CommunityPulseBand (PSY-1431) — full-width hairline stats band under the
 * homepage hero (Figma Product Designs → Home → `1083:7`, locked 2026-07-18).
 *
 * Global pulse: same numbers for every visitor (auth and unauth). Dense
 * mono-numeric StatsList-style strip — not a sidebar box (homepage is
 * single-column max-w-6xl). Self-hides on error so a pulse outage never
 * breaks the discovery landing.
 */

import { statNumberFormatter } from '@/components/shared/StatsList'
import { useCommunityPulse } from '../hooks/useCommunityPulse'

function PulseStat({ value, label }: { value: number; label: string }) {
  return (
    <div className="flex flex-col items-end gap-[3px]">
      <p className="font-mono text-[26px] leading-[34px] text-foreground tabular-nums sm:text-[30px]">
        {statNumberFormatter.format(value)}
      </p>
      <p className="text-xs tracking-[0.2px] text-muted-foreground">{label}</p>
    </div>
  )
}

export function CommunityPulseBand() {
  const { data, isLoading, isError } = useCommunityPulse()

  if (isError) return null

  return (
    <section
      aria-label="Current stats"
      className="flex w-full items-center justify-between gap-4 border-y border-border py-[18px]"
    >
      <p className="shrink-0 font-mono text-[11px] uppercase tracking-[1.4px] text-muted-foreground">
        Current stats
      </p>

      {isLoading || !data ? (
        <div
          aria-hidden
          className="flex gap-10 sm:gap-14"
        >
          <div className="flex flex-col items-end gap-[3px]">
            <span className="h-[34px] w-12 animate-pulse rounded-sm bg-muted" />
            <span className="h-3 w-24 animate-pulse rounded-sm bg-muted" />
          </div>
          <div className="flex flex-col items-end gap-[3px]">
            <span className="h-[34px] w-16 animate-pulse rounded-sm bg-muted" />
            <span className="h-3 w-28 animate-pulse rounded-sm bg-muted" />
          </div>
        </div>
      ) : (
        <div className="flex gap-10 sm:gap-14">
          {/* The count is a ROLLING [today, today+7d) window, not a
              Monday-to-Sunday week, so the label says "next 7 days" (PSY-1732).
              "this week" is reserved for the calendar-week surfaces that read
              `shows_calendar_week` — the /scenes cards and the /shows by-city
              block — whose numbers match the week page they link to. */}
          <PulseStat
            value={data.shows_this_week}
            label="shows in the next 7 days"
          />
          <PulseStat
            value={data.entities_in_graph}
            label="entities in the graph"
          />
        </div>
      )}
    </section>
  )
}
