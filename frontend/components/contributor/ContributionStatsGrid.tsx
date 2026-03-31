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
  History,
  FilePen,
  ThumbsUp,
  GitFork,
  Vote,
  Library,
  Bell,
  Ticket,
  Flag,
  CheckCircle,
  UserPlus,
  Heart,
} from 'lucide-react'
import type { LucideIcon } from 'lucide-react'
import type { ContributionStats } from '@/features/auth'

interface StatCardProps {
  icon: LucideIcon
  label: string
  value: number | string
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

function formatPercentage(rate: number): string {
  return `${Math.round(rate * 100)}%`
}

export function ContributionStatsGrid({ stats }: ContributionStatsGridProps) {
  const statItems: StatCardProps[] = [
    // Content creation
    { icon: Calendar, label: 'Shows Submitted', value: stats.shows_submitted },
    { icon: MapPin, label: 'Venues Submitted', value: stats.venues_submitted },
    { icon: PenLine, label: 'Venue Edits', value: stats.venue_edits_submitted },
    { icon: Disc3, label: 'Releases Created', value: stats.releases_created },
    { icon: Tag, label: 'Labels Created', value: stats.labels_created },
    { icon: Tent, label: 'Festivals Created', value: stats.festivals_created },
    { icon: Mic2, label: 'Artists Edited', value: stats.artists_edited },
    { icon: History, label: 'Revisions Made', value: stats.revisions_made },
    { icon: FilePen, label: 'Pending Edits', value: stats.pending_edits_submitted },

    // Community participation
    { icon: ThumbsUp, label: 'Tag Votes', value: stats.tag_votes_cast },
    { icon: GitFork, label: 'Relationship Votes', value: stats.relationship_votes_cast },
    { icon: Vote, label: 'Request Votes', value: stats.request_votes_cast },
    { icon: Library, label: 'Collection Items', value: stats.collection_items_added },
    { icon: Bell, label: 'Subscriptions', value: stats.collection_subscriptions },
    { icon: Ticket, label: 'Shows Attended', value: stats.shows_attended },

    // Reports
    { icon: Flag, label: 'Reports Filed', value: stats.reports_filed },
    { icon: CheckCircle, label: 'Reports Resolved', value: stats.reports_resolved },

    // Social
    { icon: UserPlus, label: 'Followers', value: stats.followers_count },
    { icon: Heart, label: 'Following', value: stats.following_count },

    // Moderation
    { icon: Shield, label: 'Moderation Actions', value: stats.moderation_actions },
  ]

  // Add approval rate as a special stat if present
  if (stats.approval_rate != null) {
    statItems.push({
      icon: CheckCircle,
      label: 'Approval Rate',
      value: formatPercentage(stats.approval_rate),
    })
  }

  const visibleStats = statItems.filter(s => s.value !== 0 && s.value !== '0%')

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
