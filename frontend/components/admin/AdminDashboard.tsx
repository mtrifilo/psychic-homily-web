'use client'

import {
  Clock,
  MapPin,
  Flag,
  BadgeCheck,
  Music,
  Building2,
  Mic2,
  Users,
  TrendingUp,
  UserPlus,
  type LucideIcon,
} from 'lucide-react'
import { Card, CardContent } from '@/components/ui/card'
import { useAdminStats } from '@/lib/hooks/useAdminStats'
import { Loader2 } from 'lucide-react'

interface StatCardProps {
  label: string
  value: number
  icon: LucideIcon
  highlight?: boolean
}

function StatCard({ label, value, icon: Icon, highlight }: StatCardProps) {
  return (
    <Card className="py-4">
      <CardContent className="flex items-center gap-4">
        <div
          className={`flex h-10 w-10 shrink-0 items-center justify-center rounded-lg ${
            highlight && value > 0
              ? 'bg-amber-500/15 text-amber-600 dark:text-amber-400'
              : 'bg-muted text-muted-foreground'
          }`}
        >
          <Icon className="h-5 w-5" />
        </div>
        <div className="min-w-0">
          <p
            className={`text-2xl font-bold tabular-nums ${
              highlight && value > 0
                ? 'text-amber-600 dark:text-amber-400'
                : ''
            }`}
          >
            {value.toLocaleString()}
          </p>
          <p className="text-sm text-muted-foreground truncate">{label}</p>
        </div>
      </CardContent>
    </Card>
  )
}

export function AdminDashboard() {
  const { data: stats, isLoading, error } = useAdminStats()

  if (isLoading) {
    return (
      <div className="flex items-center justify-center py-12">
        <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
      </div>
    )
  }

  if (error) {
    return (
      <div className="rounded-lg border border-destructive/50 bg-destructive/10 p-4 text-center">
        <p className="text-destructive">
          {error instanceof Error
            ? error.message
            : 'Failed to load dashboard stats. Please try again.'}
        </p>
      </div>
    )
  }

  if (!stats) return null

  return (
    <div className="space-y-8">
      {/* Needs Attention */}
      <section>
        <h2 className="text-sm font-medium text-muted-foreground uppercase tracking-wider mb-3">
          Needs Attention
        </h2>
        <div className="grid grid-cols-2 gap-3 sm:grid-cols-4">
          <StatCard
            label="Pending Shows"
            value={stats.pending_shows}
            icon={Clock}
            highlight
          />
          <StatCard
            label="Pending Venue Edits"
            value={stats.pending_venue_edits}
            icon={MapPin}
            highlight
          />
          <StatCard
            label="Pending Reports"
            value={stats.pending_reports}
            icon={Flag}
            highlight
          />
          <StatCard
            label="Unverified Venues"
            value={stats.unverified_venues}
            icon={BadgeCheck}
            highlight
          />
        </div>
      </section>

      {/* Platform */}
      <section>
        <h2 className="text-sm font-medium text-muted-foreground uppercase tracking-wider mb-3">
          Platform
        </h2>
        <div className="grid grid-cols-2 gap-3 sm:grid-cols-4">
          <StatCard
            label="Approved Shows"
            value={stats.total_shows}
            icon={Music}
          />
          <StatCard
            label="Verified Venues"
            value={stats.total_venues}
            icon={Building2}
          />
          <StatCard
            label="Artists"
            value={stats.total_artists}
            icon={Mic2}
          />
          <StatCard
            label="Users"
            value={stats.total_users}
            icon={Users}
          />
        </div>
      </section>

      {/* Recent Activity */}
      <section>
        <h2 className="text-sm font-medium text-muted-foreground uppercase tracking-wider mb-3">
          Last 7 Days
        </h2>
        <div className="grid grid-cols-2 gap-3">
          <StatCard
            label="Shows Submitted"
            value={stats.shows_submitted_last_7_days}
            icon={TrendingUp}
          />
          <StatCard
            label="Users Registered"
            value={stats.users_registered_last_7_days}
            icon={UserPlus}
          />
        </div>
      </section>
    </div>
  )
}
