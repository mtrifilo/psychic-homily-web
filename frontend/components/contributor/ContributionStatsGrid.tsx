'use client'

import { Card, CardContent } from '@/components/ui/card'
import {
  Calendar,
  MapPin,
  Disc3,
  Tag,
  Tent,
  Mic2,
  Shield,
  PenLine,
} from 'lucide-react'
import type { LucideIcon } from 'lucide-react'
import type { ContributionStats } from '@/features/auth'

interface StatCardProps {
  icon: LucideIcon
  label: string
  value: number
}

function StatCard({ icon: Icon, label, value }: StatCardProps) {
  if (value === 0) return null

  return (
    <Card className="bg-muted/30 border-border/50">
      <CardContent className="p-4 flex items-center gap-3">
        <div className="flex h-10 w-10 items-center justify-center rounded-lg bg-background">
          <Icon className="h-5 w-5 text-muted-foreground" />
        </div>
        <div>
          <p className="text-2xl font-bold">{value}</p>
          <p className="text-xs text-muted-foreground">{label}</p>
        </div>
      </CardContent>
    </Card>
  )
}

interface ContributionStatsGridProps {
  stats: ContributionStats
}

export function ContributionStatsGrid({ stats }: ContributionStatsGridProps) {
  const statItems: StatCardProps[] = [
    { icon: Calendar, label: 'Shows Submitted', value: stats.shows_submitted },
    { icon: MapPin, label: 'Venues Submitted', value: stats.venues_submitted },
    { icon: PenLine, label: 'Venue Edits', value: stats.venue_edits_submitted },
    { icon: Disc3, label: 'Releases Created', value: stats.releases_created },
    { icon: Tag, label: 'Labels Created', value: stats.labels_created },
    { icon: Tent, label: 'Festivals Created', value: stats.festivals_created },
    { icon: Mic2, label: 'Artists Edited', value: stats.artists_edited },
    { icon: Shield, label: 'Moderation Actions', value: stats.moderation_actions },
  ]

  const visibleStats = statItems.filter(s => s.value > 0)

  if (visibleStats.length === 0) {
    return (
      <p className="text-sm text-muted-foreground">
        No contributions yet.
      </p>
    )
  }

  return (
    <div className="grid grid-cols-2 md:grid-cols-3 lg:grid-cols-4 gap-3">
      {visibleStats.map(stat => (
        <StatCard key={stat.label} {...stat} />
      ))}
    </div>
  )
}
