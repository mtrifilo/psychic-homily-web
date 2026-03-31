'use client'

import { TrendingUp, TrendingDown, Minus, Users, Building2, Calendar } from 'lucide-react'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import type { ScenePulse as ScenePulseData } from '../types'

interface ScenePulseProps {
  pulse: ScenePulseData
}

/**
 * Get abbreviated month label for N months ago from today.
 * Sets day to 1 before subtracting to avoid month overflow
 * (e.g., March 31 minus 1 month would overflow Feb 28 back to March).
 */
function getMonthLabel(monthsAgo: number): string {
  const date = new Date()
  date.setDate(1)
  date.setMonth(date.getMonth() - monthsAgo)
  return date.toLocaleString('default', { month: 'short' })
}

/**
 * Simple sparkline bar chart using CSS
 */
function Sparkline({ data }: { data: number[] }) {
  const max = Math.max(...data, 1) // Avoid division by zero
  // shows_by_month is last 6 months, index 0 = oldest
  const months = data.map((value, i) => ({
    value,
    label: getMonthLabel(data.length - 1 - i),
    height: (value / max) * 100,
  }))

  return (
    <div className="flex items-end gap-1.5 h-24 mt-2">
      {months.map((month, i) => (
        <div key={i} className="flex flex-col items-center gap-1 flex-1 min-w-0">
          <div className="w-full flex flex-col items-center justify-end h-[72px]">
            <span className="text-[10px] text-muted-foreground mb-1">
              {month.value > 0 ? month.value : ''}
            </span>
            <div
              className="w-full max-w-[28px] rounded-sm bg-primary/80 transition-all"
              style={{ height: `${Math.max(month.height, 2)}%` }}
            />
          </div>
          <span className="text-[10px] text-muted-foreground truncate">
            {month.label}
          </span>
        </div>
      ))}
    </div>
  )
}

function TrendIndicator({ trend }: { trend: number }) {
  if (trend > 0) {
    return (
      <span className="inline-flex items-center gap-1 text-sm font-medium text-green-500">
        <TrendingUp className="h-4 w-4" />
        +{trend}
      </span>
    )
  }
  if (trend < 0) {
    return (
      <span className="inline-flex items-center gap-1 text-sm font-medium text-red-500">
        <TrendingDown className="h-4 w-4" />
        {trend}
      </span>
    )
  }
  return (
    <span className="inline-flex items-center gap-1 text-sm font-medium text-muted-foreground">
      <Minus className="h-4 w-4" />
      0
    </span>
  )
}

export function ScenePulse({ pulse }: ScenePulseProps) {
  return (
    <Card>
      <CardHeader className="pb-3">
        <CardTitle className="text-base">Scene Pulse</CardTitle>
      </CardHeader>
      <CardContent>
        <div className="grid grid-cols-3 gap-4 mb-4">
          {/* Shows this month */}
          <div>
            <div className="flex items-center gap-1.5 text-sm text-muted-foreground mb-1">
              <Calendar className="h-3.5 w-3.5" />
              Shows this month
            </div>
            <div className="flex items-baseline gap-2">
              <span className="text-2xl font-bold">{pulse.shows_this_month}</span>
              <TrendIndicator trend={pulse.shows_trend} />
            </div>
            <p className="text-xs text-muted-foreground mt-0.5">
              vs {pulse.shows_prev_month} last month
            </p>
          </div>

          {/* New artists */}
          <div>
            <div className="flex items-center gap-1.5 text-sm text-muted-foreground mb-1">
              <Users className="h-3.5 w-3.5" />
              New artists
            </div>
            <div className="flex items-baseline gap-2">
              <span className="text-2xl font-bold">{pulse.new_artists_30d}</span>
            </div>
            <p className="text-xs text-muted-foreground mt-0.5">
              past 30 days
            </p>
          </div>

          {/* Active venues */}
          <div>
            <div className="flex items-center gap-1.5 text-sm text-muted-foreground mb-1">
              <Building2 className="h-3.5 w-3.5" />
              Active venues
            </div>
            <div className="flex items-baseline gap-2">
              <span className="text-2xl font-bold">{pulse.active_venues_this_month}</span>
            </div>
            <p className="text-xs text-muted-foreground mt-0.5">
              this month
            </p>
          </div>
        </div>

        {/* Sparkline */}
        {pulse.shows_by_month && pulse.shows_by_month.length > 0 && (
          <div className="border-t border-border/50 pt-3">
            <p className="text-xs text-muted-foreground mb-1">Shows per month (last 6 months)</p>
            <Sparkline data={pulse.shows_by_month} />
          </div>
        )}
      </CardContent>
    </Card>
  )
}
