'use client'

import { useMemo } from 'react'
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
  TrendingDown,
  CircleCheck,
  CheckCircle,
  XCircle,
  PlusCircle,
  Pencil,
  Trash2,
  UserPlus,
  GitMerge,
  History,
  Star,
  Tag,
  Link2,
  Activity,
  type LucideIcon,
} from 'lucide-react'
import { Card, CardContent } from '@/components/ui/card'
import { useAdminStats, useAdminActivity } from '@/lib/hooks/admin/useAdminStats'
import { Loader2 } from 'lucide-react'
import type { ActivityEvent } from '@/lib/types/adminStats'

interface StatCardProps {
  label: string
  value: number
  icon: LucideIcon
  highlight?: boolean
  trend?: number
  onClick?: () => void
}

function StatCard({ label, value, icon: Icon, highlight, trend, onClick }: StatCardProps) {
  const isZeroHighlight = highlight && value === 0
  return (
    <Card
      className={`py-4${isZeroHighlight ? ' opacity-50' : ''}${onClick ? ' cursor-pointer transition-shadow hover:shadow-md hover:bg-muted/50' : ''}`}
      onClick={onClick}
      role={onClick ? 'button' : undefined}
      tabIndex={onClick ? 0 : undefined}
      onKeyDown={onClick ? (e) => { if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); onClick() } } : undefined}
    >
      <CardContent className="flex items-center gap-3">
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
          <p className="text-sm text-muted-foreground leading-tight">{label}</p>
          {trend !== undefined && trend !== 0 && (
            <span className={`flex items-center gap-1 text-xs ${
              trend > 0 ? 'text-emerald-600 dark:text-emerald-400' : 'text-red-500 dark:text-red-400'
            }`}>
              {trend > 0 ? <TrendingUp className="h-3 w-3" /> : <TrendingDown className="h-3 w-3" />}
              {trend > 0 ? '+' : ''}{trend} this week
            </span>
          )}
        </div>
      </CardContent>
    </Card>
  )
}

function AllClearMessage() {
  return (
    <Card className="py-6 border-emerald-500/30 bg-emerald-500/5">
      <CardContent className="flex items-center justify-center gap-3">
        <div className="flex h-10 w-10 shrink-0 items-center justify-center rounded-full bg-emerald-500/15">
          <CircleCheck className="h-5 w-5 text-emerald-600 dark:text-emerald-400" />
        </div>
        <div>
          <p className="font-medium text-emerald-600 dark:text-emerald-400">
            All caught up!
          </p>
          <p className="text-sm text-muted-foreground">
            No pending items need your attention.
          </p>
        </div>
      </CardContent>
    </Card>
  )
}

function getEventIcon(eventType: string): LucideIcon {
  if (eventType.includes('approved') || eventType.includes('verified')) return CheckCircle
  if (eventType.includes('rejected') || eventType.includes('dismissed')) return XCircle
  if (eventType.includes('created')) return PlusCircle
  if (eventType.includes('edited') || eventType.includes('updated')) return Pencil
  if (eventType.includes('deleted')) return Trash2
  if (eventType.includes('registered')) return UserPlus
  if (eventType.includes('merged')) return GitMerge
  if (eventType.includes('rolled_back')) return History
  if (eventType.includes('featured')) return Star
  if (eventType.includes('tag')) return Tag
  if (eventType.includes('relationship') || eventType.includes('alias')) return Link2
  if (eventType.includes('fulfilled') || eventType.includes('closed')) return CheckCircle
  if (eventType.includes('resolved')) return CheckCircle
  return Activity
}

function getEventIconColor(eventType: string): string {
  if (eventType.includes('approved') || eventType.includes('verified') || eventType.includes('fulfilled') || eventType.includes('resolved')) {
    return 'text-emerald-600 dark:text-emerald-400 bg-emerald-500/10'
  }
  if (eventType.includes('rejected') || eventType.includes('dismissed') || eventType.includes('deleted')) {
    return 'text-red-500 dark:text-red-400 bg-red-500/10'
  }
  if (eventType.includes('created')) {
    return 'text-blue-600 dark:text-blue-400 bg-blue-500/10'
  }
  if (eventType.includes('edited') || eventType.includes('updated')) {
    return 'text-amber-600 dark:text-amber-400 bg-amber-500/10'
  }
  if (eventType.includes('merged') || eventType.includes('rolled_back')) {
    return 'text-violet-600 dark:text-violet-400 bg-violet-500/10'
  }
  return 'text-muted-foreground bg-muted'
}

function getEntityUrl(entityType: string | undefined, entitySlug: string | undefined): string | null {
  if (!entityType || !entitySlug) return null
  switch (entityType) {
    case 'show': return `/shows/${entitySlug}`
    case 'venue': return `/venues/${entitySlug}`
    case 'artist': return `/artists/${entitySlug}`
    default: return null
  }
}

function formatRelativeTime(timestamp: string): string {
  const date = new Date(timestamp)
  const now = new Date()
  const diffMs = now.getTime() - date.getTime()
  const diffSec = Math.floor(diffMs / 1000)
  const diffMin = Math.floor(diffSec / 60)
  const diffHr = Math.floor(diffMin / 60)
  const diffDays = Math.floor(diffHr / 24)

  if (diffSec < 60) return 'just now'
  if (diffMin < 60) return `${diffMin}m ago`
  if (diffHr < 24) return `${diffHr}h ago`
  if (diffDays < 7) return `${diffDays}d ago`
  return date.toLocaleDateString()
}

function ActivityFeedItem({ event }: { event: ActivityEvent }) {
  const Icon = getEventIcon(event.event_type)
  const iconColor = getEventIconColor(event.event_type)
  const url = getEntityUrl(event.entity_type, event.entity_slug)

  return (
    <div className="flex items-start gap-3 py-2.5 px-1">
      <div className={`flex h-7 w-7 shrink-0 items-center justify-center rounded-full ${iconColor}`}>
        <Icon className="h-3.5 w-3.5" />
      </div>
      <div className="min-w-0 flex-1">
        <p className="text-sm leading-snug">
          {url ? (
            <a href={url} className="hover:underline text-foreground">
              {event.description}
            </a>
          ) : (
            <span className="text-foreground">{event.description}</span>
          )}
        </p>
        <p className="text-xs text-muted-foreground mt-0.5">
          {event.actor_name && <span>{event.actor_name} &middot; </span>}
          {formatRelativeTime(event.timestamp)}
        </p>
      </div>
    </div>
  )
}

function ActivityFeed() {
  const { data, isLoading, error } = useAdminActivity()

  const events = useMemo(() => data?.events ?? [], [data])

  if (isLoading) {
    return (
      <div className="flex items-center justify-center py-8">
        <Loader2 className="h-5 w-5 animate-spin text-muted-foreground" />
      </div>
    )
  }

  if (error) {
    return (
      <p className="text-sm text-muted-foreground py-4 text-center">
        Failed to load activity feed.
      </p>
    )
  }

  if (events.length === 0) {
    return (
      <p className="text-sm text-muted-foreground py-8 text-center">
        No recent activity
      </p>
    )
  }

  return (
    <div className="max-h-80 overflow-y-auto divide-y divide-border">
      {events.map((event) => (
        <ActivityFeedItem key={event.id} event={event} />
      ))}
    </div>
  )
}

interface AdminDashboardProps {
  onNavigate?: (tab: string) => void
}

export function AdminDashboard({ onNavigate }: AdminDashboardProps) {
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

  const allAttentionClear =
    stats.pending_shows === 0 &&
    stats.pending_venue_edits === 0 &&
    stats.pending_reports === 0 &&
    stats.unverified_venues === 0

  return (
    <div className="space-y-8">
      {/* Needs Attention */}
      <section>
        <h2 className="text-sm font-medium text-muted-foreground uppercase tracking-wider mb-3">
          Needs Attention
        </h2>
        {allAttentionClear ? (
          <AllClearMessage />
        ) : (
          <div className="grid grid-cols-2 gap-3 sm:grid-cols-4">
            <StatCard
              label="Pending Shows"
              value={stats.pending_shows}
              icon={Clock}
              highlight
              onClick={onNavigate ? () => onNavigate('pending-shows') : undefined}
            />
            <StatCard
              label="Pending Venue Edits"
              value={stats.pending_venue_edits}
              icon={MapPin}
              highlight
              onClick={onNavigate ? () => onNavigate('pending-venue-edits') : undefined}
            />
            <StatCard
              label="Pending Reports"
              value={stats.pending_reports}
              icon={Flag}
              highlight
              onClick={onNavigate ? () => onNavigate('reports') : undefined}
            />
            <StatCard
              label="Unverified Venues"
              value={stats.unverified_venues}
              icon={BadgeCheck}
              highlight
              onClick={onNavigate ? () => onNavigate('unverified-venues') : undefined}
            />
          </div>
        )}
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
            trend={stats.total_shows_trend}
            onClick={onNavigate ? () => onNavigate('pending-shows') : undefined}
          />
          <StatCard
            label="Verified Venues"
            value={stats.total_venues}
            icon={Building2}
            trend={stats.total_venues_trend}
            onClick={onNavigate ? () => onNavigate('unverified-venues') : undefined}
          />
          <StatCard
            label="Artists"
            value={stats.total_artists}
            icon={Mic2}
            trend={stats.total_artists_trend}
            onClick={onNavigate ? () => onNavigate('artists-admin') : undefined}
          />
          <StatCard
            label="Users"
            value={stats.total_users}
            icon={Users}
            trend={stats.total_users_trend}
            onClick={onNavigate ? () => onNavigate('users') : undefined}
          />
        </div>
      </section>

      {/* Recent Activity Feed */}
      <section>
        <h2 className="text-sm font-medium text-muted-foreground uppercase tracking-wider mb-3">
          Recent Activity
        </h2>
        <Card>
          <CardContent className="pt-2 pb-1">
            <ActivityFeed />
          </CardContent>
        </Card>
      </section>
    </div>
  )
}
