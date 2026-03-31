'use client'

import { useMemo, useState } from 'react'
import { Skeleton } from '@/components/ui/skeleton'
import { useActivityHeatmap } from '@/features/auth'
import type { ActivityDay } from '@/features/auth'

// ============================================================================
// Helpers
// ============================================================================

function getWeeksToShow(): number {
  if (typeof window === 'undefined') return 52
  return window.innerWidth < 640 ? 26 : 52
}

/**
 * Generate the array of dates for the heatmap grid.
 * Starts from a Sunday that gives us `weeksCount` complete weeks ending on today's week.
 */
function generateDayGrid(weeksCount: number): Date[] {
  const today = new Date()
  today.setHours(0, 0, 0, 0)

  // Find the Saturday of this week (end of grid)
  const endDate = new Date(today)

  // Go back weeksCount weeks from this Sunday
  const startDate = new Date(endDate)
  startDate.setDate(startDate.getDate() - startDate.getDay() - (weeksCount - 1) * 7)

  const days: Date[] = []
  const current = new Date(startDate)
  while (current <= today) {
    days.push(new Date(current))
    current.setDate(current.getDate() + 1)
  }

  return days
}

function formatDateKey(date: Date): string {
  const y = date.getFullYear()
  const m = String(date.getMonth() + 1).padStart(2, '0')
  const d = String(date.getDate()).padStart(2, '0')
  return `${y}-${m}-${d}`
}

function formatTooltipDate(date: Date): string {
  return date.toLocaleDateString('en-US', {
    weekday: 'long',
    month: 'long',
    day: 'numeric',
    year: 'numeric',
  })
}

/**
 * Map intensity level (0-4) to a Tailwind class for the cell background.
 * Uses emerald shades with dark mode variants.
 */
const INTENSITY_CLASSES = [
  'bg-muted/40',                                          // 0: no activity
  'bg-emerald-200 dark:bg-emerald-900/70',                // level 1
  'bg-emerald-300 dark:bg-emerald-700',                   // level 2
  'bg-emerald-500 dark:bg-emerald-500',                   // level 3
  'bg-emerald-700 dark:bg-emerald-300',                   // level 4: max
]

function getIntensityLevel(count: number, maxCount: number): number {
  if (count === 0 || maxCount === 0) return 0
  const ratio = count / maxCount
  if (ratio <= 0.25) return 1
  if (ratio <= 0.50) return 2
  if (ratio <= 0.75) return 3
  return 4
}

const DAY_LABELS = ['', 'Mon', '', 'Wed', '', 'Fri', ''] // Sun, Mon, ..., Sat
const MONTH_NAMES = [
  'Jan', 'Feb', 'Mar', 'Apr', 'May', 'Jun',
  'Jul', 'Aug', 'Sep', 'Oct', 'Nov', 'Dec',
]

// ============================================================================
// Component
// ============================================================================

interface ActivityHeatmapProps {
  username: string
}

export function ActivityHeatmap({ username }: ActivityHeatmapProps) {
  const { data, isLoading } = useActivityHeatmap(username)
  const [tooltip, setTooltip] = useState<{
    text: string
    x: number
    y: number
  } | null>(null)

  const weeksCount = useMemo(() => getWeeksToShow(), [])
  const days = useMemo(() => generateDayGrid(weeksCount), [weeksCount])

  const countMap = useMemo(() => {
    const map = new Map<string, number>()
    if (data?.days) {
      for (const day of data.days) {
        map.set(day.date, day.count)
      }
    }
    return map
  }, [data])

  const maxCount = useMemo(() => {
    if (!data?.days || data.days.length === 0) return 0
    return Math.max(...data.days.map((d: ActivityDay) => d.count))
  }, [data])

  const totalContributions = useMemo(() => {
    if (!data?.days) return 0
    return data.days.reduce((sum: number, d: ActivityDay) => sum + d.count, 0)
  }, [data])

  // Group days into weeks (columns), where each week starts on Sunday
  const weeks = useMemo(() => {
    const result: Date[][] = []
    let currentWeek: Date[] = []

    for (const day of days) {
      if (day.getDay() === 0 && currentWeek.length > 0) {
        result.push(currentWeek)
        currentWeek = []
      }
      currentWeek.push(day)
    }
    if (currentWeek.length > 0) {
      result.push(currentWeek)
    }

    return result
  }, [days])

  // Month labels with column positions
  const monthLabels = useMemo(() => {
    const labels: { label: string; col: number }[] = []
    let lastMonth = -1

    for (let w = 0; w < weeks.length; w++) {
      const firstDay = weeks[w][0]
      const month = firstDay.getMonth()
      if (month !== lastMonth) {
        labels.push({ label: MONTH_NAMES[month], col: w })
        lastMonth = month
      }
    }

    return labels
  }, [weeks])

  if (isLoading) {
    return (
      <div className="space-y-2" data-testid="activity-heatmap-skeleton">
        <Skeleton className="h-4 w-48" />
        <Skeleton className="h-[100px] w-full" />
      </div>
    )
  }

  return (
    <div className="space-y-2" data-testid="activity-heatmap">
      {/* Header: total + legend */}
      <div className="flex items-center justify-between flex-wrap gap-2">
        <p className="text-xs text-muted-foreground">
          {totalContributions} contribution{totalContributions !== 1 ? 's' : ''} in the last{' '}
          {weeksCount === 52 ? 'year' : `${weeksCount} weeks`}
        </p>
        <div className="flex items-center gap-1 text-xs text-muted-foreground">
          <span>Less</span>
          {INTENSITY_CLASSES.map((cls, i) => (
            <div
              key={i}
              className={`w-[11px] h-[11px] rounded-[2px] ${cls}`}
            />
          ))}
          <span>More</span>
        </div>
      </div>

      {/* Grid container */}
      <div className="relative overflow-x-auto" role="img" aria-label={`Activity heatmap showing ${totalContributions} contributions`}>
        {/* Month labels row */}
        <div className="flex" style={{ paddingLeft: 28 }}>
          {weeks.map((_week, wIdx) => {
            const label = monthLabels.find((m) => m.col === wIdx)
            return (
              <div
                key={`month-${wIdx}`}
                className="text-[9px] text-muted-foreground leading-none"
                style={{ width: 13, flexShrink: 0 }}
              >
                {label ? label.label : ''}
              </div>
            )
          })}
        </div>

        {/* Grid: day labels + cells */}
        <div className="flex gap-0">
          {/* Day-of-week labels column */}
          <div className="flex flex-col" style={{ width: 28, flexShrink: 0 }}>
            {DAY_LABELS.map((label, i) => (
              <div
                key={`day-label-${i}`}
                className="text-[9px] text-muted-foreground flex items-center"
                style={{ height: 13 }}
              >
                {label}
              </div>
            ))}
          </div>

          {/* Weeks (columns) */}
          <div className="flex gap-0">
            {weeks.map((week, weekIndex) => (
              <div key={`week-${weekIndex}`} className="flex flex-col">
                {/* Pad the first week if it doesn't start on Sunday */}
                {weekIndex === 0 &&
                  Array.from({ length: week[0].getDay() }).map((_, i) => (
                    <div key={`pad-${i}`} style={{ width: 11, height: 11, margin: 1 }} />
                  ))}
                {week.map((day) => {
                  const dateKey = formatDateKey(day)
                  const count = countMap.get(dateKey) || 0
                  const level = getIntensityLevel(count, maxCount)

                  return (
                    <div
                      key={dateKey}
                      className={`rounded-[2px] cursor-pointer ${INTENSITY_CLASSES[level]}`}
                      style={{ width: 11, height: 11, margin: 1 }}
                      data-testid={`heatmap-cell-${dateKey}`}
                      data-count={count}
                      data-date={dateKey}
                      onMouseEnter={(e) => {
                        const rect = e.currentTarget.getBoundingClientRect()
                        const containerRect = e.currentTarget
                          .closest('[data-testid="activity-heatmap"]')
                          ?.getBoundingClientRect()
                        if (containerRect) {
                          setTooltip({
                            text:
                              count === 0
                                ? `No contributions on ${formatTooltipDate(day)}`
                                : `${count} contribution${count !== 1 ? 's' : ''} on ${formatTooltipDate(day)}`,
                            x: rect.left - containerRect.left + rect.width / 2,
                            y: rect.top - containerRect.top - 4,
                          })
                        }
                      }}
                      onMouseLeave={() => setTooltip(null)}
                    />
                  )
                })}
              </div>
            ))}
          </div>
        </div>

        {/* Tooltip */}
        {tooltip && (
          <div
            className="absolute z-10 pointer-events-none px-2 py-1 text-xs bg-popover text-popover-foreground border border-border rounded shadow-md whitespace-nowrap"
            style={{
              left: tooltip.x,
              top: tooltip.y,
              transform: 'translate(-50%, -100%)',
            }}
            data-testid="heatmap-tooltip"
          >
            {tooltip.text}
          </div>
        )}
      </div>
    </div>
  )
}
